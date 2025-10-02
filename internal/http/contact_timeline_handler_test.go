package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/trace"
)

func TestContactTimelineHandler_handleList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockService := mocks.NewMockContactTimelineService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTracer := pkgmocks.NewMockTracer(ctrl)

	// Mock tracer expectations for all test cases
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "ContactTimelineHandler.handleList").
		Return(context.Background(), mockSpan).
		AnyTimes()
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil).
		AnyTimes()
	mockTracer.EXPECT().
		MarkSpanError(gomock.Any(), gomock.Any()).
		AnyTimes()

	// Generate test keys
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewContactTimelineHandlerWithTracer(
		mockService,
		mockAuthService,
		publicKey,
		mockLogger,
		mockTracer,
	)

	t.Run("Success - List timeline entries", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?workspace_id=ws1&email=user@example.com&limit=10", nil)
		w := httptest.NewRecorder()

		// Mock authentication
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), "ws1").
			Return(context.Background(), &domain.User{ID: "user1"}, &domain.UserWorkspace{WorkspaceID: "ws1"}, nil)

		// Mock service response
		cursor := "cursor123"
		entries := []*domain.ContactTimelineEntry{
			{
				ID:         "entry1",
				Email:      "user@example.com",
				Operation:  "insert",
				EntityType: "contact",
				Changes:    nil,
				CreatedAt:  time.Now(),
			},
			{
				ID:         "entry2",
				Email:      "user@example.com",
				Operation:  "update",
				EntityType: "contact",
				Changes:    map[string]interface{}{"first_name": map[string]interface{}{"old": "John", "new": "Jane"}},
				CreatedAt:  time.Now(),
			},
		}
		mockService.EXPECT().
			List(gomock.Any(), "ws1", "user@example.com", 10, (*string)(nil)).
			Return(entries, &cursor, nil)

		handler.handleList(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response domain.TimelineListResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Len(t, response.Timeline, 2)
		assert.NotNil(t, response.NextCursor)
		assert.Equal(t, "cursor123", *response.NextCursor)
		assert.Equal(t, "entry1", response.Timeline[0].ID)
		assert.Equal(t, "insert", response.Timeline[0].Operation)
	})

	t.Run("Success - With cursor pagination", func(t *testing.T) {
		cursor := "existing_cursor"
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?workspace_id=ws1&email=user@example.com&cursor="+cursor, nil)
		w := httptest.NewRecorder()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), "ws1").
			Return(context.Background(), &domain.User{ID: "user1"}, &domain.UserWorkspace{WorkspaceID: "ws1"}, nil)

		entries := []*domain.ContactTimelineEntry{}
		mockService.EXPECT().
			List(gomock.Any(), "ws1", "user@example.com", 50, &cursor).
			Return(entries, nil, nil)

		handler.handleList(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response domain.TimelineListResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Empty(t, response.Timeline)
		assert.Nil(t, response.NextCursor)
	})

	t.Run("Error - Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/timeline.list?workspace_id=ws1&email=user@example.com", nil)
		w := httptest.NewRecorder()

		handler.handleList(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Error - Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?email=user@example.com", nil)
		w := httptest.NewRecorder()

		handler.handleList(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "workspace_id is required")
	})

	t.Run("Error - Missing email", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?workspace_id=ws1", nil)
		w := httptest.NewRecorder()

		handler.handleList(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "email is required")
	})

	t.Run("Error - Invalid limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?workspace_id=ws1&email=user@example.com&limit=invalid", nil)
		w := httptest.NewRecorder()

		handler.handleList(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid limit parameter")
	})

	t.Run("Error - Limit exceeds maximum", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?workspace_id=ws1&email=user@example.com&limit=150", nil)
		w := httptest.NewRecorder()

		handler.handleList(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "limit cannot exceed 100")
	})

	t.Run("Error - Authentication failed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?workspace_id=ws1&email=user@example.com", nil)
		w := httptest.NewRecorder()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), "ws1").
			Return(context.Background(), nil, nil, assert.AnError)

		mockLogger.EXPECT().Error(gomock.Any())

		handler.handleList(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Error - Service error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/timeline.list?workspace_id=ws1&email=user@example.com", nil)
		w := httptest.NewRecorder()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), "ws1").
			Return(context.Background(), &domain.User{ID: "user1"}, &domain.UserWorkspace{WorkspaceID: "ws1"}, nil)

		mockService.EXPECT().
			List(gomock.Any(), "ws1", "user@example.com", 50, (*string)(nil)).
			Return(nil, nil, assert.AnError)

		mockLogger.EXPECT().Error(gomock.Any())

		handler.handleList(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to list timeline entries")
	})
}

func TestContactTimelineHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockContactTimelineService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewContactTimelineHandler(
		mockService,
		mockAuthService,
		publicKey,
		mockLogger,
	)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that route is registered
	req := httptest.NewRequest(http.MethodGet, "/api/timeline.list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should not be 404 (route exists)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}
