package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AuthMethod says how a session was opened. It is stored in user_sessions.auth_method;
// sessions opened before 45.0 have none.
type AuthMethod string

const (
	AuthMethodMagicCode     AuthMethod = "magic_code"
	AuthMethodSSO           AuthMethod = "sso"
	AuthMethodRootSignature AuthMethod = "root_signature"
	AuthMethodInvitation    AuthMethod = "invitation"
	// AuthMethodDemo is the session the demo seed opens for itself after a call signed
	// with SECRET_KEY. No token is ever issued for it.
	AuthMethodDemo AuthMethod = "demo"
)

// SecondFactor says which second factor a session passed, stored in
// user_sessions.second_factor.
type SecondFactor string

const (
	SecondFactorTOTP         SecondFactor = "totp"
	SecondFactorRecoveryCode SecondFactor = "recovery_code"
)

const (
	// RecoveryCodeCount is how many recovery codes a set holds.
	RecoveryCodeCount = 10
	// recoveryCodeLength is the length of a normalized recovery code: 24 base32 characters,
	// 120 bits.
	recoveryCodeLength = 24
	// ChallengeTTL bounds the life of a sign-in challenge.
	ChallengeTTL = 10 * time.Minute
	// MaxChallengeAttempts is how many codes one challenge accepts.
	MaxChallengeAttempts = 5
	// PendingSetupTTL bounds the life of a secret being set up.
	PendingSetupTTL = 15 * time.Minute
	// MaxPendingSetupAttempts is how many wrong codes a setup takes before its secret is
	// dropped.
	MaxPendingSetupAttempts = 5
	// FreshSessionWindow is how recent a session must be to turn on two-factor
	// authentication for the first time.
	FreshSessionWindow = 15 * time.Minute
	// lockoutStart is the consecutive failure that starts the waits.
	lockoutStart = 5
	// TOTPHardLockThreshold is the consecutive failure count at which authenticator codes
	// are refused until a recovery code or a reset by the root user.
	TOTPHardLockThreshold = 50
	// maxLockout caps the wait between two attempts.
	maxLockout = time.Hour
)

// SessionAuth is what a request knows about its session beyond the user: how it was
// opened, the second factor it passed and when it became usable.
type SessionAuth struct {
	ExpiresAt       time.Time
	AuthMethod      AuthMethod
	SecondFactor    SecondFactor
	AuthenticatedAt *time.Time
}

// SessionSatisfiesTwoFactor reports whether a session meets the instance's two-factor
// requirement: it passed a second factor, or it was opened by single sign-on (the identity
// provider owns multi-factor authentication), by the root signature (it proves possession
// of SECRET_KEY) or by the demo seed.
func SessionSatisfiesTwoFactor(s SessionAuth) bool {
	if s.SecondFactor != "" {
		return true
	}
	switch s.AuthMethod {
	case AuthMethodSSO, AuthMethodRootSignature, AuthMethodDemo:
		return true
	}
	return false
}

// TwoFactorChallenge is what a sign-in answers instead of a token when the account has
// two-factor authentication: an opaque token for user.twoFactor.verify, and its deadline.
type TwoFactorChallenge struct {
	Token     string
	ExpiresAt time.Time
}

// TOTPAAD is the associated data an authenticator secret is sealed with: it binds the
// ciphertext to its user, so one copied onto another user's row does not open. The user ID
// is canonical (lower-case UUID), so a differently written ID cannot change it.
func TOTPAAD(userID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return "", fmt.Errorf("totp aad: invalid user id: %w", err)
	}
	return "notifuse:totp:" + id.String(), nil
}

// crockford is the Crockford base32 alphabet in lower case: digits and letters without i,
// l, o and u, so a code read aloud or copied by hand stays unambiguous.
const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

// GenerateRecoveryCodes returns RecoveryCodeCount codes of 120 bits each, read from r, in
// the form shown to the user: four groups of six characters joined by dashes.
func GenerateRecoveryCodes(r io.Reader) ([]string, error) {
	codes := make([]string, 0, RecoveryCodeCount)
	for len(codes) < RecoveryCodeCount {
		raw := make([]byte, 15)
		if _, err := io.ReadFull(r, raw); err != nil {
			return nil, fmt.Errorf("generate recovery codes: %w", err)
		}
		code := encodeCrockford(raw)
		codes = append(codes, code[0:6]+"-"+code[6:12]+"-"+code[12:18]+"-"+code[18:24])
	}
	return codes, nil
}

// encodeCrockford encodes 15 bytes as 24 Crockford base32 characters.
func encodeCrockford(raw []byte) string {
	var b strings.Builder
	var buffer uint64
	bits := 0
	for _, c := range raw {
		buffer = buffer<<8 | uint64(c)
		bits += 8
		for bits >= 5 {
			bits -= 5
			b.WriteByte(crockford[(buffer>>uint(bits))&31])
		}
	}
	return b.String()
}

// ErrInvalidRecoveryCode is returned for input that cannot be a recovery code. It is
// refused before any database lookup.
var ErrInvalidRecoveryCode = errors.New("invalid recovery code")

// NormalizeRecoveryCode turns what a person typed into the canonical code: spaces and
// dashes removed, lower case, o read as 0 and i or l as 1. Anything else outside the
// alphabet, u included, or a length other than 24, is refused.
func NormalizeRecoveryCode(input string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToLower(input) {
		switch {
		case r == ' ' || r == '-' || r == '\t':
			continue
		case r == 'o':
			b.WriteByte('0')
		case r == 'i' || r == 'l':
			b.WriteByte('1')
		case r < 128 && strings.IndexByte(crockford, byte(r)) >= 0:
			b.WriteRune(r)
		default:
			return "", ErrInvalidRecoveryCode
		}
	}
	if b.Len() != recoveryCodeLength {
		return "", ErrInvalidRecoveryCode
	}
	return b.String(), nil
}

// HashRecoveryCode is the stored form of a normalized recovery code: SHA-256 of the code
// bound to its user. A code of 120 bits needs no salt nor slow hash (NIST SP 800-63B-4
// §3.1.2), and the hash does not depend on SECRET_KEY, so a key rotation leaves the codes
// working.
func HashRecoveryCode(userID, normalized string) string {
	sum := sha256.Sum256([]byte("notifuse:recovery:" + strings.ToLower(userID) + ":" + normalized))
	return hex.EncodeToString(sum[:])
}

// NextLockout returns until when the next code must wait after failedAttempts consecutive
// failures, or nil when it need not: no wait below 5 failures, 1 minute at the 5th, then
// doubled at each failure (2, 4, 8 … minutes) up to an hour.
func NextLockout(failedAttempts int, now time.Time) *time.Time {
	if failedAttempts < lockoutStart {
		return nil
	}
	wait := time.Minute
	for i := lockoutStart; i < failedAttempts && wait < maxLockout; i++ {
		wait *= 2
	}
	if wait > maxLockout {
		wait = maxLockout
	}
	until := now.Add(wait)
	return &until
}

// Errors of two-factor authentication. The HTTP layer maps each one to its status; none of
// them maps to 401, which makes the console discard its token.
var (
	// ErrTwoFactorRequired: the instance requires two-factor authentication and the
	// session did not pass it. Its message must not contain text that handlers match
	// before writePermissionError ("not found", "already exists").
	ErrTwoFactorRequired = errors.New("two-factor authentication required")
	// ErrTwoFactorInvalidCode: a wrong or already used code.
	ErrTwoFactorInvalidCode = errors.New("invalid two-factor code")
	// ErrTwoFactorChallengeInvalid: the challenge is unknown, expired or used up.
	ErrTwoFactorChallengeInvalid = errors.New("two-factor challenge is invalid or expired")
	// ErrTwoFactorSetupExpired: no secret is being set up, or it expired.
	ErrTwoFactorSetupExpired = errors.New("two-factor setup expired")
	// ErrTwoFactorTOTPLocked: too many wrong codes; only a recovery code is accepted.
	ErrTwoFactorTOTPLocked = errors.New("authenticator codes are locked, use a recovery code")
	// ErrReauthenticationRequired: turning on two-factor authentication needs a recent
	// sign-in.
	ErrReauthenticationRequired = errors.New("sign in again to continue")
	// ErrTwoFactorRequiredByInstance: two-factor authentication cannot be turned off while
	// the instance requires it.
	ErrTwoFactorRequiredByInstance = errors.New("two-factor authentication is required on this instance")
)

// TwoFactorLockedError: a code must wait until RetryAfter after repeated failures.
type TwoFactorLockedError struct {
	RetryAfter time.Duration
}

func (e *TwoFactorLockedError) Error() string {
	return fmt.Sprintf("too many wrong codes, try again in %s", e.RetryAfter.Round(time.Second))
}

// commitAndFail marks a business error whose transaction must still commit: a wrong code
// answers an error, and the failure counters it incremented must persist.
type commitAndFail struct {
	err error
}

func (c *commitAndFail) Error() string { return c.err.Error() }
func (c *commitAndFail) Unwrap() error { return c.err }

// CommitAndFail wraps err so that a system transaction commits before returning it.
func CommitAndFail(err error) error {
	return &commitAndFail{err: err}
}

// AsCommitAndFail reports whether err asks for a commit, and returns the error to give the
// caller once the commit succeeded.
func AsCommitAndFail(err error) (error, bool) {
	var c *commitAndFail
	if errors.As(err, &c) {
		return c.err, true
	}
	return err, false
}
