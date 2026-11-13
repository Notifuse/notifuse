package service

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Without a usable secret nothing is signed and nothing is accepted: an empty key
// would let anyone forge a token.
func TestAuthService_UnusableSecret_SignsAndAcceptsNothing(t *testing.T) {
	secrets := []struct {
		name      string
		getSecret func() ([]byte, error)
		wantErr   string
	}{
		{name: "empty secret", getSecret: func() ([]byte, error) { return []byte{}, nil }, wantErr: "cannot be empty"},
		{name: "secret unavailable", getSecret: func() ([]byte, error) { return nil, errors.New("vault down") }, wantErr: "vault down"},
	}
	for _, tc := range secrets {
		t.Run(tc.name, func(t *testing.T) {
			service := NewAuthService(AuthServiceConfig{GetSecret: tc.getSecret, Logger: logger.NewLogger()})
			user := &domain.User{ID: "user-1", Email: "user@example.com"}

			assert.Empty(t, service.GenerateUserAuthToken(user, "session-1", time.Now().Add(time.Hour)))
			assert.Empty(t, service.GenerateAPIAuthToken(user))
			assert.Empty(t, service.GenerateInvitationToken(&domain.WorkspaceInvitation{
				ID: "inv-1", WorkspaceID: "ws-1", Email: "invitee@example.com", ExpiresAt: time.Now().Add(time.Hour),
			}))

			// A token signed with an empty key is refused rather than verified with one.
			forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, InvitationClaims{
				InvitationID: "inv-1", WorkspaceID: "ws-1", Email: "invitee@example.com",
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			}).SignedString([]byte{})
			require.NoError(t, err)
			invitationID, workspaceID, email, err := service.ValidateInvitationToken(forged)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "secret not available")
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, invitationID)
			assert.Empty(t, workspaceID)
			assert.Empty(t, email)
		})
	}
}

// An unsigned invitation token (alg none) is refused even when its claims are well formed.
func TestAuthService_ValidateInvitationToken_RefusesUnsignedToken(t *testing.T) {
	service := NewAuthService(AuthServiceConfig{
		GetSecret: func() ([]byte, error) { return []byte("test-secret-key-for-jwt-signing-minimum-32-chars"), nil },
		Logger:    logger.NewLogger(),
	})
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, InvitationClaims{
		InvitationID: "inv-1", WorkspaceID: "ws-1", Email: "invitee@example.com",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	invitationID, workspaceID, email, err := service.ValidateInvitationToken(unsigned)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid invitation token")
	assert.Contains(t, err.Error(), "unexpected signing method")
	assert.Empty(t, invitationID)
	assert.Empty(t, workspaceID)
	assert.Empty(t, email)
}
