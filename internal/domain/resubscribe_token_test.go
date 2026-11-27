package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResubscribeToken_MintAndVerify(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	secret := "workspace-secret-key"
	ws := "ws123"
	email := "alice@example.com"
	listID := "vip"

	token, err := MintResubscribeTokenAt(secret, ws, email, listID, now)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	assert.NoError(t, VerifyResubscribeTokenAt(secret, ws, email, listID, token, now))
	assert.NoError(t, VerifyResubscribeTokenAt(secret, ws, email, listID, token, now.Add(ResubscribeTokenTTL)))

	// Expired one second past TTL
	assert.ErrorIs(t,
		VerifyResubscribeTokenAt(secret, ws, email, listID, token, now.Add(ResubscribeTokenTTL+time.Second)),
		ErrResubscribeTokenExpired,
	)

	// Mismatched secret, workspace, email, or list must be rejected
	assert.ErrorIs(t, VerifyResubscribeTokenAt("other-secret", ws, email, listID, token, now), ErrResubscribeTokenInvalid)
	assert.ErrorIs(t, VerifyResubscribeTokenAt(secret, "other-ws", email, listID, token, now), ErrResubscribeTokenInvalid)
	assert.ErrorIs(t, VerifyResubscribeTokenAt(secret, ws, "bob@example.com", listID, token, now), ErrResubscribeTokenInvalid)
	assert.ErrorIs(t, VerifyResubscribeTokenAt(secret, ws, email, "other-list", token, now), ErrResubscribeTokenInvalid)

	// Malformed tokens and replay of email_hmac must be rejected
	assert.ErrorIs(t, VerifyResubscribeTokenAt(secret, ws, email, listID, "", now), ErrResubscribeTokenInvalid)
	assert.ErrorIs(t, VerifyResubscribeTokenAt(secret, ws, email, listID, "not-a-token", now), ErrResubscribeTokenInvalid)
	assert.ErrorIs(t, VerifyResubscribeTokenAt(secret, ws, email, listID, ComputeEmailHMAC(email, secret), now), ErrResubscribeTokenInvalid)
}
