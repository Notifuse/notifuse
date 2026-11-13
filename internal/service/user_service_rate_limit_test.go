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
	assert.True(t, record.Failed(), "the refusal is audited as a failure")

	// Another address keeps its own attempts.
	assert.True(t, service.rateLimiter.Allow("signin", "other@example.com"))
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
	assert.True(t, record.Failed())
}
