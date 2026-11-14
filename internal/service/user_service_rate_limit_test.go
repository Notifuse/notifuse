package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// spendSignInAttempts uses up the sign-in attempts the limiter grants email.
func spendSignInAttempts(t *testing.T, service *UserService, email string) {
	t.Helper()
	for service.rateLimiter.Allow("signin", email) {
	}
}

// Once an address has spent its attempts, a sign-in is refused before any
// account lookup (strict repository mock) and audited as rate limited.
func TestUserService_SignIn_RateLimitedBeforeLookup(t *testing.T) {
	_, _, service, _ := setupUserTest(t)
	spendSignInAttempts(t, service, "victim@example.com")
	record := domain.NewAuditRecord("203.0.113.7", "test", "req-1")
	ctx := domain.WithAuditRecord(context.Background(), record)

	code, err := service.SignIn(ctx, domain.SignInInput{Email: "victim@example.com"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many sign-in attempts")
	assert.Empty(t, code)
	assertAuditReason(t, record, "rate_limited")
}

// assertAuditReason checks the request is audited as a failure for reason.
func assertAuditReason(t *testing.T, record *domain.AuditRecord, reason string) {
	t.Helper()
	assert.True(t, record.Failed(), "the refusal is audited as a failure")
	event, skipped := record.Finalize(429)
	require.False(t, skipped)
	assert.Equal(t, reason, event.Metadata["reason"])
}

func TestUserService_RootSignin_RateLimitedBeforeSignatureCheck(t *testing.T) {
	_, _, service, _ := setupUserTestWithRootEmail(t, "root@example.com")
	spendSignInAttempts(t, service, "root@example.com")
	record := domain.NewAuditRecord("203.0.113.7", "test", "req-1")
	ctx := domain.WithAuditRecord(context.Background(), record)

	resp, err := service.RootSignin(ctx, domain.RootSigninInput{Email: "root@example.com", Signature: "anything"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many sign-in attempts")
	assert.Nil(t, resp)
	assertAuditReason(t, record, "rate_limited")
}
