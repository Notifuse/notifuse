package domain

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionSatisfiesTwoFactor(t *testing.T) {
	cases := []struct {
		session SessionAuth
		want    bool
	}{
		{SessionAuth{AuthMethod: AuthMethodMagicCode}, false},
		{SessionAuth{AuthMethod: AuthMethodMagicCode, SecondFactor: SecondFactorTOTP}, true},
		{SessionAuth{AuthMethod: AuthMethodMagicCode, SecondFactor: SecondFactorRecoveryCode}, true},
		{SessionAuth{AuthMethod: AuthMethodInvitation}, false},
		{SessionAuth{AuthMethod: AuthMethodSSO}, true},
		{SessionAuth{AuthMethod: AuthMethodRootSignature}, true},
		{SessionAuth{AuthMethod: AuthMethodDemo}, true},
		{SessionAuth{}, false}, // opened before 45.0
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, SessionSatisfiesTwoFactor(tc.session), "%+v", tc.session)
	}
}

func TestTOTPAAD(t *testing.T) {
	lower, err := TOTPAAD("6f1c2a7e-0b5d-4c1e-9a3f-2d8e7b6c5a41")
	require.NoError(t, err)
	upper, err := TOTPAAD("6F1C2A7E-0B5D-4C1E-9A3F-2D8E7B6C5A41")
	require.NoError(t, err)
	assert.Equal(t, "notifuse:totp:6f1c2a7e-0b5d-4c1e-9a3f-2d8e7b6c5a41", lower)
	assert.Equal(t, lower, upper, "the same user whatever the case of the ID")

	_, err = TOTPAAD("not-a-uuid")
	assert.Error(t, err)
	_, err = TOTPAAD("")
	assert.Error(t, err)
}

func TestGenerateRecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes(rand.Reader)
	require.NoError(t, err)
	require.Len(t, codes, RecoveryCodeCount)

	seen := map[string]bool{}
	for _, code := range codes {
		assert.Len(t, code, 27, "24 characters in four groups of six")
		assert.Equal(t, 3, strings.Count(code, "-"))
		normalized, err := NormalizeRecoveryCode(code)
		require.NoError(t, err, "a generated code normalizes: %s", code)
		assert.Len(t, normalized, 24)
		assert.False(t, seen[normalized], "codes are unique")
		seen[normalized] = true
	}

	_, err = GenerateRecoveryCodes(bytes.NewReader(make([]byte, 20)))
	assert.Error(t, err, "a short source fails rather than repeating codes")
}

func TestEncodeCrockfordUsesEveryBit(t *testing.T) {
	assert.Equal(t, strings.Repeat("0", 24), encodeCrockford(make([]byte, 15)))
	assert.Equal(t, strings.Repeat("z", 24), encodeCrockford(bytes.Repeat([]byte{0xff}, 15)))
}

func TestNormalizeRecoveryCode(t *testing.T) {
	const canonical = "7k2m9px4c8rdq1vt6h3nbw5e"
	for _, input := range []string{
		"7k2m9p-x4c8rd-q1vt6h-3nbw5e",
		"7K2M9P-X4C8RD-Q1VT6H-3NBW5E",
		" 7k2m9p x4c8rd q1vt6h 3nbw5e ",
		"7k2m9px4c8rdqivt6h3nbw5e", // i read as 1
		"7k2m9px4c8rdqLvt6h3nbw5e", // l read as 1
	} {
		got, err := NormalizeRecoveryCode(input)
		require.NoError(t, err, input)
		assert.Equal(t, canonical, got, input)
	}

	got, err := NormalizeRecoveryCode("o00000000000000000000000")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("0", 24), got, "o read as 0")

	for _, input := range []string{
		"",
		"7k2m9px4c8rdq1vt6h3nbw5",   // 23
		"7k2m9px4c8rdq1vt6h3nbw5e0", // 25
		"7k2m9px4c8rdq1vt6h3nbw5u",  // u is not in the alphabet
		"7k2m9px4c8rdq1vt6h3nbw5é",
		"7k2m9px4c8rdq1vt6h3nbw5!",
	} {
		_, err := NormalizeRecoveryCode(input)
		assert.ErrorIs(t, err, ErrInvalidRecoveryCode, "%q", input)
	}
}

func TestHashRecoveryCode(t *testing.T) {
	const code = "7k2m9px4c8rdq1vt6h3nbw5e"
	a := HashRecoveryCode("6f1c2a7e-0b5d-4c1e-9a3f-2d8e7b6c5a41", code)
	b := HashRecoveryCode("0a9b8c7d-6e5f-4a3b-2c1d-0e9f8a7b6c5d", code)

	assert.Len(t, a, 64)
	assert.NotEqual(t, a, b, "the same code hashes differently for another user")
	assert.Equal(t, a, HashRecoveryCode("6F1C2A7E-0B5D-4C1E-9A3F-2D8E7B6C5A41", code))
	assert.NotEqual(t, a, HashRecoveryCode("6f1c2a7e-0b5d-4c1e-9a3f-2d8e7b6c5a41", "7k2m9px4c8rdq1vt6h3nbw50"))
}

func TestNextLockout(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		failures int
		wait     time.Duration
	}{
		{5, time.Minute},
		{6, 2 * time.Minute},
		{7, 4 * time.Minute},
		{8, 8 * time.Minute},
		{10, 32 * time.Minute},
		{11, time.Hour},
		{49, time.Hour},
	}
	for _, failures := range []int{0, 1, 4} {
		assert.Nil(t, NextLockout(failures, now), "%d failures", failures)
	}
	for _, tc := range cases {
		until := NextLockout(tc.failures, now)
		require.NotNil(t, until, "%d failures", tc.failures)
		assert.Equal(t, now.Add(tc.wait), *until, "%d failures", tc.failures)
	}
}

func TestErrTwoFactorRequiredMessage(t *testing.T) {
	// Some handlers match text before writePermissionError: this error must not look like a
	// not-found or a conflict to them.
	for _, text := range []string{"not found", "already exists", "invalid template", "not active"} {
		assert.NotContains(t, ErrTwoFactorRequired.Error(), text)
	}
}

func TestCommitAndFail(t *testing.T) {
	marked := CommitAndFail(ErrTwoFactorInvalidCode)
	assert.ErrorIs(t, marked, ErrTwoFactorInvalidCode)

	inner, ok := AsCommitAndFail(fmt.Errorf("verify: %w", marked))
	assert.True(t, ok)
	assert.Equal(t, ErrTwoFactorInvalidCode, inner, "the caller gets the business error, unwrapped")

	plain := errors.New("database down")
	inner, ok = AsCommitAndFail(plain)
	assert.False(t, ok)
	assert.Equal(t, plain, inner)
}

func TestTwoFactorLockedError(t *testing.T) {
	err := &TwoFactorLockedError{RetryAfter: 4*time.Minute + 300*time.Millisecond}
	assert.Equal(t, "too many wrong codes, try again in 4m0s", err.Error())
}

func TestAuthResponseNeverSerializesTheChallenge(t *testing.T) {
	resp := AuthResponse{Token: "jwt", Challenge: &TwoFactorChallenge{Token: "secret-challenge", ExpiresAt: time.Now()}}
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "secret-challenge")
	assert.NotContains(t, string(body), "challenge")
}

func TestSessionNeverSerializesItsChallenge(t *testing.T) {
	hash := "challenge-hash"
	body, err := json.Marshal(Session{ID: "s1", ChallengeHash: &hash, AuthMethod: AuthMethodMagicCode})
	require.NoError(t, err)
	assert.NotContains(t, string(body), hash)
	assert.NotContains(t, string(body), "auth_method")
}
