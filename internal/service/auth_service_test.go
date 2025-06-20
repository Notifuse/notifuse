package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func setupAuthTest(t *testing.T) (
	*mocks.MockAuthRepository,
	*mocks.MockWorkspaceRepository,
	*pkgmocks.MockLogger,
	*AuthService,
) {
	ctrl := gomock.NewController(t)
	mockAuthRepo := mocks.NewMockAuthRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Generate test keys
	privateKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := privateKey.Public()

	service, err := NewAuthService(AuthServiceConfig{
		Repository:          mockAuthRepo,
		WorkspaceRepository: mockWorkspaceRepo,
		PrivateKey:          privateKey.ExportBytes(),
		PublicKey:           publicKey.ExportBytes(),
		Logger:              mockLogger,
	})
	require.NoError(t, err)

	return mockAuthRepo, mockWorkspaceRepo, mockLogger, service
}

func TestAuthService_AuthenticateUserFromContext(t *testing.T) {
	mockAuthRepo, _, _, service := setupAuthTest(t)

	userID := "user123"
	sessionID := "session123"

	t.Run("successful authentication with user type", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		expiresAt := time.Now().Add(1 * time.Hour)

		ctx := context.WithValue(
			context.WithValue(
				context.WithValue(
					context.Background(),
					domain.UserIDKey,
					userID,
				),
				domain.SessionIDKey,
				sessionID,
			),
			domain.UserTypeKey,
			string(domain.UserTypeUser),
		)

		mockAuthRepo.EXPECT().
			GetSessionByID(ctx, sessionID, userID).
			Return(&expiresAt, nil)

		mockAuthRepo.EXPECT().
			GetUserByID(ctx, userID).
			Return(user, nil)

		result, err := service.AuthenticateUserFromContext(ctx)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)
	})

	t.Run("successful authentication with API key type", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		ctx := context.WithValue(
			context.WithValue(
				context.Background(),
				domain.UserIDKey,
				userID,
			),
			domain.UserTypeKey,
			string(domain.UserTypeAPIKey),
		)

		mockAuthRepo.EXPECT().
			GetUserByID(ctx, userID).
			Return(user, nil)

		result, err := service.AuthenticateUserFromContext(ctx)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)
	})

	t.Run("missing user_id in context", func(t *testing.T) {
		ctx := context.WithValue(
			context.WithValue(
				context.Background(),
				domain.SessionIDKey,
				sessionID,
			),
			domain.UserTypeKey,
			string(domain.UserTypeUser),
		)

		result, err := service.AuthenticateUserFromContext(ctx)

		require.Error(t, err)
		require.Equal(t, ErrUserNotFound, err)
		require.Nil(t, result)
	})

	t.Run("missing user type in context", func(t *testing.T) {
		ctx := context.WithValue(
			context.Background(),
			domain.UserIDKey,
			userID,
		)

		result, err := service.AuthenticateUserFromContext(ctx)

		require.Error(t, err)
		require.Equal(t, ErrUserNotFound, err)
		require.Nil(t, result)
	})

	t.Run("missing session_id in context for user type", func(t *testing.T) {
		ctx := context.WithValue(
			context.WithValue(
				context.Background(),
				domain.UserIDKey,
				userID,
			),
			domain.UserTypeKey,
			string(domain.UserTypeUser),
		)

		result, err := service.AuthenticateUserFromContext(ctx)

		require.Error(t, err)
		require.Equal(t, ErrUserNotFound, err)
		require.Nil(t, result)
	})

	t.Run("invalid user type in context", func(t *testing.T) {
		ctx := context.WithValue(
			context.WithValue(
				context.Background(),
				domain.UserIDKey,
				userID,
			),
			domain.UserTypeKey,
			"invalid_type",
		)

		result, err := service.AuthenticateUserFromContext(ctx)

		require.Error(t, err)
		require.Equal(t, ErrUserNotFound, err)
		require.Nil(t, result)
	})
}

func TestAuthService_AuthenticateUserForWorkspace(t *testing.T) {
	mockAuthRepo, mockWorkspaceRepo, _, service := setupAuthTest(t)

	userID := "user123"
	sessionID := "session123"
	workspaceID := "workspace123"

	t.Run("successful authentication", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		expiresAt := time.Now().Add(1 * time.Hour)

		ctx := context.WithValue(
			context.WithValue(
				context.WithValue(
					context.Background(),
					domain.UserIDKey,
					userID,
				),
				domain.SessionIDKey,
				sessionID,
			),
			domain.UserTypeKey,
			string(domain.UserTypeUser),
		)

		mockAuthRepo.EXPECT().
			GetSessionByID(ctx, sessionID, userID).
			Return(&expiresAt, nil)

		mockAuthRepo.EXPECT().
			GetUserByID(ctx, userID).
			Return(user, nil)

		mockWorkspaceRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(workspace, nil)

		mockWorkspaceRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}, nil)

		newCtx, result, err := service.AuthenticateUserForWorkspace(ctx, workspaceID)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)

		// Verify that the user is stored in the context
		storedUser, ok := newCtx.Value(domain.WorkspaceUserKey(workspaceID)).(*domain.User)
		require.True(t, ok)
		require.Equal(t, userID, storedUser.ID)
	})

	t.Run("successful authentication with API key", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		ctx := context.WithValue(
			context.WithValue(
				context.Background(),
				domain.UserIDKey,
				userID,
			),
			domain.UserTypeKey,
			string(domain.UserTypeAPIKey),
		)

		mockAuthRepo.EXPECT().
			GetUserByID(ctx, userID).
			Return(user, nil)

		mockWorkspaceRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(workspace, nil)

		mockWorkspaceRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}, nil)

		newCtx, result, err := service.AuthenticateUserForWorkspace(ctx, workspaceID)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)

		// Verify that the user is stored in the context
		storedUser, ok := newCtx.Value(domain.WorkspaceUserKey(workspaceID)).(*domain.User)
		require.True(t, ok)
		require.Equal(t, userID, storedUser.ID)
	})

	t.Run("user already in context", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		// Create a context with the user already stored for this workspace
		ctx := context.WithValue(context.Background(), domain.WorkspaceUserKey(workspaceID), user)

		// No mock expectations should be called since the user is already in context

		newCtx, result, err := service.AuthenticateUserForWorkspace(ctx, workspaceID)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)
		require.Equal(t, ctx, newCtx) // Context should be unchanged
	})

	t.Run("user not in workspace", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		workspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
		}

		expiresAt := time.Now().Add(1 * time.Hour)

		ctx := context.WithValue(
			context.WithValue(
				context.WithValue(
					context.Background(),
					domain.UserIDKey,
					userID,
				),
				domain.SessionIDKey,
				sessionID,
			),
			domain.UserTypeKey,
			string(domain.UserTypeUser),
		)

		mockAuthRepo.EXPECT().
			GetSessionByID(ctx, sessionID, userID).
			Return(&expiresAt, nil)

		mockAuthRepo.EXPECT().
			GetUserByID(ctx, userID).
			Return(user, nil)

		mockWorkspaceRepo.EXPECT().
			GetByID(ctx, workspaceID).
			Return(workspace, nil)

		mockWorkspaceRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(nil, errors.New("not found"))

		newCtx, result, err := service.AuthenticateUserForWorkspace(ctx, workspaceID)

		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, ctx, newCtx) // Context should be unchanged on error
	})
}

func TestAuthService_VerifyUserSession(t *testing.T) {
	mockAuthRepo, _, mockLogger, service := setupAuthTest(t)

	userID := "user123"
	sessionID := "session123"

	t.Run("successful verification", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		expiresAt := time.Now().Add(1 * time.Hour)

		mockAuthRepo.EXPECT().
			GetSessionByID(context.Background(), sessionID, userID).
			Return(&expiresAt, nil)

		mockAuthRepo.EXPECT().
			GetUserByID(context.Background(), userID).
			Return(user, nil)

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)
	})

	t.Run("session not found", func(t *testing.T) {
		mockAuthRepo.EXPECT().
			GetSessionByID(context.Background(), sessionID, userID).
			Return(nil, sql.ErrNoRows)

		mockLogger.EXPECT().
			WithField(string(domain.UserIDKey), userID).
			Return(mockLogger)

		mockLogger.EXPECT().
			WithField(string(domain.SessionIDKey), sessionID).
			Return(mockLogger)

		mockLogger.EXPECT().
			Error("Session not found")

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Equal(t, ErrSessionExpired, err)
		require.Nil(t, result)
	})

	t.Run("session expired", func(t *testing.T) {
		expiresAt := time.Now().Add(-1 * time.Hour)

		mockAuthRepo.EXPECT().
			GetSessionByID(context.Background(), sessionID, userID).
			Return(&expiresAt, nil)

		mockLogger.EXPECT().
			WithField(string(domain.UserIDKey), userID).
			Return(mockLogger)

		mockLogger.EXPECT().
			WithField(string(domain.SessionIDKey), sessionID).
			Return(mockLogger)

		mockLogger.EXPECT().
			WithField("expires_at", &expiresAt).
			Return(mockLogger)

		mockLogger.EXPECT().
			Error("Session expired")

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Equal(t, ErrSessionExpired, err)
		require.Nil(t, result)
	})

	t.Run("user not found", func(t *testing.T) {
		expiresAt := time.Now().Add(1 * time.Hour)

		mockAuthRepo.EXPECT().
			GetSessionByID(context.Background(), sessionID, userID).
			Return(&expiresAt, nil)

		mockAuthRepo.EXPECT().
			GetUserByID(context.Background(), userID).
			Return(nil, sql.ErrNoRows)

		mockLogger.EXPECT().
			WithField(string(domain.UserIDKey), userID).
			Return(mockLogger)

		mockLogger.EXPECT().
			Error("User not found")

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Equal(t, ErrUserNotFound, err)
		require.Nil(t, result)
	})
}

func TestAuthService_GenerateAuthToken(t *testing.T) {
	mockAuthRepo, _, _, service := setupAuthTest(t)

	userID := "user123"
	sessionID := "session123"
	expiresAt := time.Now().Add(1 * time.Hour)

	t.Run("successful token generation", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		token := service.GenerateUserAuthToken(user, sessionID, expiresAt)

		require.NotEmpty(t, token)
		require.NotNil(t, token)
	})

	t.Run("failed token generation", func(t *testing.T) {
		// Create a service with invalid key length
		_, err := NewAuthService(AuthServiceConfig{
			Repository:          mockAuthRepo,
			WorkspaceRepository: nil,
			PrivateKey:          []byte("invalid"),
			PublicKey:           []byte("invalid"),
			Logger:              nil,
		})
		require.Error(t, err)
	})
}

func TestAuthService_GenerateInvitationToken(t *testing.T) {
	mockAuthRepo, _, _, service := setupAuthTest(t)

	invitationID := "invitation123"
	workspaceID := "workspace123"
	inviterID := "inviter123"
	email := "test@example.com"

	t.Run("successful token generation", func(t *testing.T) {
		invitation := &domain.WorkspaceInvitation{
			ID:          invitationID,
			WorkspaceID: workspaceID,
			InviterID:   inviterID,
			Email:       email,
			ExpiresAt:   time.Now().Add(15 * 24 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		token := service.GenerateInvitationToken(invitation)

		require.NotEmpty(t, token)
		require.NotNil(t, token)
	})

	t.Run("failed token generation", func(t *testing.T) {
		// Create a service with invalid key length
		_, err := NewAuthService(AuthServiceConfig{
			Repository:          mockAuthRepo,
			WorkspaceRepository: nil,
			PrivateKey:          []byte("invalid"),
			PublicKey:           []byte("invalid"),
			Logger:              nil,
		})
		require.Error(t, err)
	})
}

func TestAuthService_GetUserByID(t *testing.T) {
	mockAuthRepo, _, mockLogger, service := setupAuthTest(t)

	userID := "user123"

	t.Run("successful user retrieval", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		mockAuthRepo.EXPECT().
			GetUserByID(context.Background(), userID).
			Return(user, nil)

		result, err := service.GetUserByID(context.Background(), userID)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)
	})

	t.Run("user not found", func(t *testing.T) {
		mockAuthRepo.EXPECT().
			GetUserByID(context.Background(), userID).
			Return(nil, sql.ErrNoRows)

		result, err := service.GetUserByID(context.Background(), userID)

		require.Error(t, err)
		require.Equal(t, ErrUserNotFound, err)
		require.Nil(t, result)
	})

	t.Run("error retrieving user", func(t *testing.T) {
		mockAuthRepo.EXPECT().
			GetUserByID(context.Background(), userID).
			Return(nil, errors.New("database error"))

		mockLogger.EXPECT().
			WithField("error", "database error").
			Return(mockLogger)

		mockLogger.EXPECT().
			WithField(string(domain.UserIDKey), userID).
			Return(mockLogger)

		mockLogger.EXPECT().
			Error("Failed to get user by ID")

		result, err := service.GetUserByID(context.Background(), userID)

		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestAuthService_GenerateAPIAuthToken(t *testing.T) {
	_, _, _, service := setupAuthTest(t)

	userID := "user123"
	email := "test@example.com"

	t.Run("successful API token generation", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: email,
		}

		token := service.GenerateAPIAuthToken(user)

		require.NotEmpty(t, token)
		require.NotNil(t, token)

		// Verify the token can be parsed and contains expected claims
		parser := paseto.NewParser()
		publicKey := service.GetPrivateKey().Public()

		parsedToken, err := parser.ParseV4Public(publicKey, token, nil)
		require.NoError(t, err)
		require.NotNil(t, parsedToken)

		// Verify token claims
		userIDClaim, err := parsedToken.GetString("user_id")
		require.NoError(t, err)
		require.Equal(t, userID, userIDClaim)

		typeClaim, err := parsedToken.GetString("type")
		require.NoError(t, err)
		require.Equal(t, string(domain.UserTypeAPIKey), typeClaim)

		// Verify expiration is set to 10 years from now (approximately)
		expiration, err := parsedToken.GetExpiration()
		require.NoError(t, err)
		expectedExpiration := time.Now().Add(time.Hour * 24 * 365 * 10)
		require.WithinDuration(t, expectedExpiration, expiration, time.Minute)

		// Verify issued at and not before are set
		issuedAt, err := parsedToken.GetIssuedAt()
		require.NoError(t, err)
		require.WithinDuration(t, time.Now(), issuedAt, time.Minute)

		notBefore, err := parsedToken.GetNotBefore()
		require.NoError(t, err)
		require.WithinDuration(t, time.Now(), notBefore, time.Minute)
	})

	t.Run("token generation with invalid key format", func(t *testing.T) {
		// Test that service construction fails with invalid key format
		mockLoggerForInvalid := pkgmocks.NewMockLogger(gomock.NewController(t))
		mockLoggerForInvalid.EXPECT().
			WithField("error", gomock.Any()).
			Return(mockLoggerForInvalid)
		mockLoggerForInvalid.EXPECT().
			Error("Error creating PASETO private key")

		_, err := NewAuthService(AuthServiceConfig{
			Repository:          nil,
			WorkspaceRepository: nil,
			PrivateKey:          make([]byte, 32), // Invalid key format
			PublicKey:           make([]byte, 32), // Invalid key format
			Logger:              mockLoggerForInvalid,
		})
		require.Error(t, err) // Should fail during construction

		// Test with valid service to ensure token generation works
		user := &domain.User{
			ID:    userID,
			Email: email,
		}

		token := service.GenerateAPIAuthToken(user)
		require.NotEmpty(t, token)
	})

	t.Run("token generation with nil user", func(t *testing.T) {
		// This test verifies that the method panics with nil user (current behavior)
		// In a production system, this should be handled more gracefully
		require.Panics(t, func() {
			service.GenerateAPIAuthToken(nil)
		})
	})
}

func TestAuthService_GetPrivateKey(t *testing.T) {
	_, _, _, service := setupAuthTest(t)

	t.Run("successful private key retrieval", func(t *testing.T) {
		privateKey := service.GetPrivateKey()

		require.NotNil(t, privateKey)

		// Verify the key can be used for signing
		token := paseto.NewToken()
		token.SetString("test", "value")
		token.SetExpiration(time.Now().Add(time.Hour)) // Add expiration to make token valid

		signed := token.V4Sign(privateKey, nil)
		require.NotEmpty(t, signed)

		// Verify the key can be used to derive public key
		publicKey := privateKey.Public()
		require.NotNil(t, publicKey)

		// Verify we can parse the token with the public key
		parser := paseto.NewParser()
		parsedToken, err := parser.ParseV4Public(publicKey, signed, nil)
		require.NoError(t, err)
		require.NotNil(t, parsedToken)

		testValue, err := parsedToken.GetString("test")
		require.NoError(t, err)
		require.Equal(t, "value", testValue)
	})

	t.Run("private key consistency", func(t *testing.T) {
		// Verify that multiple calls return the same key
		key1 := service.GetPrivateKey()
		key2 := service.GetPrivateKey()

		require.Equal(t, key1.ExportBytes(), key2.ExportBytes())
	})

	t.Run("private key security", func(t *testing.T) {
		privateKey := service.GetPrivateKey()

		// Verify the key has the expected length for V4 asymmetric keys
		keyBytes := privateKey.ExportBytes()
		require.Len(t, keyBytes, 64) // Ed25519 private key is 64 bytes

		// Verify the key is not all zeros (basic sanity check)
		allZeros := make([]byte, 64)
		require.NotEqual(t, allZeros, keyBytes)
	})
}
