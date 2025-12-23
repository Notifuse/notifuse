package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGetJWTSecret returns a test JWT secret function
func testGetJWTSecret() ([]byte, error) {
	return []byte("test-jwt-secret"), nil
}

// setupEmailQueueHandlerTest creates a handler with mock dependencies
func setupEmailQueueHandlerTest(t *testing.T) (*EmailQueueHandler, *mocks.MockEmailQueueRepository, *mocks.MockAuthService, *gomock.Controller) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	log := logger.NewLogger()
	handler := NewEmailQueueHandler(mockRepo, mockAuthService, testGetJWTSecret, log)
	return handler, mockRepo, mockAuthService, ctrl
}

// mockAuthSuccess sets up the mock auth service to return success
func mockAuthSuccess(mockAuthService *mocks.MockAuthService, workspaceID string) {
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(context.Background(), &domain.User{ID: "user-1"}, &domain.UserWorkspace{}, nil)
}

// mockAuthFailure sets up the mock auth service to return an error
func mockAuthFailure(mockAuthService *mocks.MockAuthService, workspaceID string) {
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(nil, nil, nil, errors.New("unauthorized"))
}

func TestNewEmailQueueHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	log := logger.NewLogger()

	handler := NewEmailQueueHandler(mockRepo, mockAuthService, testGetJWTSecret, log)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.repo)
	assert.NotNil(t, handler.authService)
	assert.NotNil(t, handler.logger)
	assert.NotNil(t, handler.getJWTSecret)
}

func TestEmailQueueHandler_RegisterRoutes(t *testing.T) {
	handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
	defer ctrl.Finish()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Verify routes are registered by checking they don't return 404
	routes := []string{
		"/api/email_queue.stats",
	}

	for _, route := range routes {
		_, pattern := mux.Handler(httptest.NewRequest("GET", route, nil))
		assert.NotEmpty(t, pattern, "Route %s should be registered", route)
	}
}

func TestEmailQueueHandler_handleStats(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		expectedStats := &domain.EmailQueueStats{
			Pending:    10,
			Processing: 5,
			Failed:     2,
		}

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			GetStats(gomock.Any(), "workspace-123").
			Return(expectedStats, nil)

		req := httptest.NewRequest("GET", "/api/email_queue.stats?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleStats(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		require.NotEmpty(t, rec.Body.String())
	})

	t.Run("Method not allowed", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("POST", "/api/email_queue.stats?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleStats(rec, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/api/email_queue.stats", nil)
		rec := httptest.NewRecorder()

		handler.handleStats(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		handler, _, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthFailure(mockAuthService, "workspace-123")

		req := httptest.NewRequest("GET", "/api/email_queue.stats?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleStats(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("Repository error", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			GetStats(gomock.Any(), "workspace-123").
			Return(nil, errors.New("database error"))

		req := httptest.NewRequest("GET", "/api/email_queue.stats?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleStats(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
