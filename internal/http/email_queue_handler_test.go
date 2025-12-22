package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		"/api/email_queue.dead_letter.list",
		"/api/email_queue.dead_letter.cleanup",
		"/api/email_queue.dead_letter.retry",
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
			DeadLetter: 1,
		}

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			GetStats(gomock.Any(), "workspace-123").
			Return(expectedStats, nil)

		req := httptest.NewRequest("GET", "/api/email_queue.stats?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleStats(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.NotNil(t, response["stats"])
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

func TestEmailQueueHandler_handleDeadLetterList(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		entries := []*domain.EmailQueueDeadLetter{
			{
				ID:              "dl-1",
				OriginalEntryID: "entry-1",
				SourceType:      domain.EmailQueueSourceBroadcast,
				SourceID:        "broadcast-1",
				ContactEmail:    "test@example.com",
				FinalError:      "permanent failure",
				Attempts:        3,
			},
		}

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			GetDeadLetterEntries(gomock.Any(), "workspace-123", 50, 0).
			Return(entries, int64(1), nil)

		req := httptest.NewRequest("GET", "/api/email_queue.dead_letter.list?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterList(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.NotNil(t, response["entries"])
		assert.Equal(t, float64(1), response["total"])
	})

	t.Run("With pagination", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			GetDeadLetterEntries(gomock.Any(), "workspace-123", 10, 20).
			Return([]*domain.EmailQueueDeadLetter{}, int64(0), nil)

		req := httptest.NewRequest("GET", "/api/email_queue.dead_letter.list?workspace_id=workspace-123&limit=10&offset=20", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterList(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.list?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterList(rec, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/api/email_queue.dead_letter.list", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterList(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		handler, _, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthFailure(mockAuthService, "workspace-123")

		req := httptest.NewRequest("GET", "/api/email_queue.dead_letter.list?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterList(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("Repository error", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			GetDeadLetterEntries(gomock.Any(), "workspace-123", 50, 0).
			Return(nil, int64(0), errors.New("database error"))

		req := httptest.NewRequest("GET", "/api/email_queue.dead_letter.list?workspace_id=workspace-123", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterList(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestEmailQueueHandler_handleDeadLetterCleanup(t *testing.T) {
	t.Run("Success with default retention", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			CleanupDeadLetter(gomock.Any(), "workspace-123", 720*time.Hour).
			Return(int64(5), nil)

		body := `{"workspace_id": "workspace-123"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.cleanup", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterCleanup(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, float64(5), response["deleted"])
		assert.Equal(t, float64(720), response["retention_hours"])
	})

	t.Run("Success with custom retention", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			CleanupDeadLetter(gomock.Any(), "workspace-123", 24*time.Hour).
			Return(int64(10), nil)

		body := `{"workspace_id": "workspace-123", "retention_hours": 24}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.cleanup", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterCleanup(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, float64(10), response["deleted"])
		assert.Equal(t, float64(24), response["retention_hours"])
	})

	t.Run("Method not allowed", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/api/email_queue.dead_letter.cleanup", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterCleanup(rec, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		body := `{invalid json}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.cleanup", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterCleanup(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		body := `{"retention_hours": 24}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.cleanup", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterCleanup(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		handler, _, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthFailure(mockAuthService, "workspace-123")

		body := `{"workspace_id": "workspace-123"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.cleanup", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterCleanup(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("Repository error", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			CleanupDeadLetter(gomock.Any(), "workspace-123", 720*time.Hour).
			Return(int64(0), errors.New("database error"))

		body := `{"workspace_id": "workspace-123"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.cleanup", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterCleanup(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestEmailQueueHandler_handleDeadLetterRetry(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			RetryDeadLetter(gomock.Any(), "workspace-123", "dl-456").
			Return(nil)

		body := `{"workspace_id": "workspace-123", "dead_letter_id": "dl-456"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.retry", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterRetry(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, true, response["success"])
	})

	t.Run("Method not allowed", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/api/email_queue.dead_letter.retry", nil)
		rec := httptest.NewRecorder()

		handler.handleDeadLetterRetry(rec, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		body := `{invalid json}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.retry", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterRetry(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		body := `{"dead_letter_id": "dl-456"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.retry", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterRetry(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Missing dead_letter_id", func(t *testing.T) {
		handler, _, _, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		body := `{"workspace_id": "workspace-123"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.retry", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterRetry(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		handler, _, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthFailure(mockAuthService, "workspace-123")

		body := `{"workspace_id": "workspace-123", "dead_letter_id": "dl-456"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.retry", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterRetry(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("Repository error", func(t *testing.T) {
		handler, mockRepo, mockAuthService, ctrl := setupEmailQueueHandlerTest(t)
		defer ctrl.Finish()

		mockAuthSuccess(mockAuthService, "workspace-123")
		mockRepo.EXPECT().
			RetryDeadLetter(gomock.Any(), "workspace-123", "dl-456").
			Return(errors.New("database error"))

		body := `{"workspace_id": "workspace-123", "dead_letter_id": "dl-456"}`
		req := httptest.NewRequest("POST", "/api/email_queue.dead_letter.retry", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleDeadLetterRetry(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
