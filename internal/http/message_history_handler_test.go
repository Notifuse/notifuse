package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func setupMessageHistoryHandlerTest(t *testing.T) (*MessageHistoryHandler, *mocks.MockMessageHistoryService, *mocks.MockAuthService, *pkgmocks.MockTracer, paseto.V4AsymmetricSecretKey) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockMessageHistoryService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTracer := pkgmocks.NewMockTracer(ctrl)

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewMessageHistoryHandlerWithTracer(
		mockService,
		mockAuthService,
		func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil },
		mockLogger,
		mockTracer,
	)

	return handler, mockService, mockAuthService, mockTracer, secretKey
}

func createMessageHistoryTestToken(t *testing.T, secretKey paseto.V4AsymmetricSecretKey, userID string) string {
	token := paseto.NewToken()
	token.SetExpiration(time.Now().Add(time.Hour))
	token.SetString(string(domain.UserIDKey), userID)
	token.SetString(string(domain.UserTypeKey), string(domain.UserTypeUser))
	token.SetString(string(domain.SessionIDKey), "test-session")

	signedToken := token.V4Sign(secretKey, nil)
	require.NotEmpty(t, signedToken)
	return signedToken
}

func TestMessageHistoryHandler_handleList_MethodNotAllowed(t *testing.T) {
	handler, _, _, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a non-GET request
	req := httptest.NewRequest(http.MethodPost, "/api/messages.list", nil)
	w := httptest.NewRecorder()

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestMessageHistoryHandler_handleList_MissingWorkspaceID(t *testing.T) {
	handler, _, _, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with no workspace_id
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list", nil)
	w := httptest.NewRecorder()

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Missing workspace ID", response["error"])
}

func TestMessageHistoryHandler_handleList_AuthenticationError(t *testing.T) {
	// Setup test with controller that doesn't finish early
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockMessageHistoryService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTracer := pkgmocks.NewMockTracer(ctrl)

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewMessageHistoryHandlerWithTracer(
		mockService,
		mockAuthService,
		func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil },
		mockLogger,
		mockTracer,
	)

	// Create a request with workspace_id
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123", nil)
	w := httptest.NewRecorder()

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)
	mockTracer.EXPECT().
		MarkSpanError(gomock.Any(), assert.AnError)

	// Set up logger mock to expect error call
	mockLogger.EXPECT().Error(gomock.Any()).Times(1)

	// Mock auth service to return error
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(nil, nil, nil, assert.AnError)

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Unauthorized", response["error"])
}

func TestMessageHistoryHandler_handleList_ValidationError(t *testing.T) {
	handler, _, mockAuthService, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with an invalid channel value
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123&channel=invalid", nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "invalid channel")
}

func TestMessageHistoryHandler_handleList_Success(t *testing.T) {
	handler, mockService, mockAuthService, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with valid parameters
	now := time.Now()
	sentAfter := now.Add(-24 * time.Hour)
	sentBefore := now

	// Format the times in RFC3339
	sentAfterStr := sentAfter.Format(time.RFC3339)
	sentBeforeStr := sentBefore.Format(time.RFC3339)

	// Create request URL with multiple parameters
	url := fmt.Sprintf(
		"/api/messages.list?workspace_id=ws123&limit=10&channel=email&status=sent&sent_after=%s&sent_before=%s",
		url.QueryEscape(sentAfterStr),
		url.QueryEscape(sentBeforeStr),
	)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Mock messages result
	messages := []*domain.MessageHistory{
		{
			ID:           "msg1",
			ContactEmail: "contact1",
			TemplateID:   "template1",
			Channel:      "email",
			SentAt:       time.Now().Add(-time.Hour),
			CreatedAt:    time.Now().Add(-time.Hour),
			UpdatedAt:    time.Now().Add(-time.Hour),
		},
	}

	result := &domain.MessageListResult{
		Messages:   messages,
		NextCursor: "next-cursor",
		HasMore:    true,
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Mock message history service
	mockService.EXPECT().
		ListMessages(gomock.Any(), "ws123", gomock.Any()).
		DoAndReturn(func(_ context.Context, workspaceID string, params domain.MessageListParams) (*domain.MessageListResult, error) {
			assert.Equal(t, "ws123", workspaceID)
			assert.Equal(t, 10, params.Limit)
			assert.Equal(t, "email", params.Channel)

			// Verify time parameters were parsed correctly - with approximate comparison
			assert.NotNil(t, params.SentAfter)
			assert.NotNil(t, params.SentBefore)

			// Compare times with a small tolerance
			if params.SentAfter != nil && sentAfter.Unix() != 0 {
				assert.WithinDuration(t, sentAfter, *params.SentAfter, 2*time.Second)
			}

			if params.SentBefore != nil && sentBefore.Unix() != 0 {
				assert.WithinDuration(t, sentBefore, *params.SentBefore, 2*time.Second)
			}

			return result, nil
		})

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var response domain.MessageListResult
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "next-cursor", response.NextCursor)
	assert.True(t, response.HasMore)
	assert.Len(t, response.Messages, 1)
	assert.Equal(t, "msg1", response.Messages[0].ID)
	assert.Equal(t, "contact1", response.Messages[0].ContactEmail)
}

func TestMessageHistoryHandler_handleList_ServiceError(t *testing.T) {
	// Setup test with controller that doesn't finish early
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockMessageHistoryService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTracer := pkgmocks.NewMockTracer(ctrl)

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewMessageHistoryHandlerWithTracer(
		mockService,
		mockAuthService,
		func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil },
		mockLogger,
		mockTracer,
	)

	// Create a request with valid parameters
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123", nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)
	mockTracer.EXPECT().
		MarkSpanError(gomock.Any(), assert.AnError)

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Set up logger mock to expect error call
	mockLogger.EXPECT().Error(gomock.Any()).Times(1)

	// Mock message history service to return error
	mockService.EXPECT().
		ListMessages(gomock.Any(), "ws123", gomock.Any()).
		Return(nil, assert.AnError)

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to list messages", response["error"])
}

func TestMessageHistoryHandler_handleList_InvalidTimeFormat(t *testing.T) {
	handler, _, mockAuthService, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with an invalid time format
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123&sent_after=invalid-time", nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "invalid sent_after time format")
}

func TestMessageHistoryHandler_handleList_InvalidBooleanFormat(t *testing.T) {
	handler, _, mockAuthService, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with an invalid boolean format
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123&is_sent=invalid-bool", nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "invalid is_sent value")
}

func TestMessageHistoryHandler_handleList_WithBooleanParameter(t *testing.T) {
	handler, mockService, mockAuthService, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with is_sent=true parameter
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123&is_sent=true", nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Create expected result
	result := &domain.MessageListResult{
		Messages:   []*domain.MessageHistory{},
		NextCursor: "",
		HasMore:    false,
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Mock message history service
	mockService.EXPECT().
		ListMessages(gomock.Any(), "ws123", gomock.Any()).
		DoAndReturn(func(_ context.Context, workspaceID string, params domain.MessageListParams) (*domain.MessageListResult, error) {
			assert.Equal(t, "ws123", workspaceID)
			assert.NotNil(t, params.IsSent)
			assert.True(t, *params.IsSent)
			return result, nil
		})

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var response domain.MessageListResult
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.HasMore)
}

func TestMessageHistoryHandler_RegisterRoutes(t *testing.T) {
	handler, _, _, _, _ := setupMessageHistoryHandlerTest(t)

	// Create a new test ServeMux
	mux := http.NewServeMux()

	// Register the routes
	handler.RegisterRoutes(mux)

	// Check that the route was registered
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list", nil)
	w := httptest.NewRecorder()

	// Should not panic
	mux.ServeHTTP(w, req)
}

func TestMessageHistoryHandler_handleList_NilTracer(t *testing.T) {
	// Setup with controller that doesn't finish early
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockMessageHistoryService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	// Create handler with standard constructor (using global tracer)
	handler := NewMessageHistoryHandler(
		mockService,
		mockAuthService,
		func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil },
		mockLogger,
	)

	// Create a request with valid parameters
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123", nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Create expected result
	result := &domain.MessageListResult{
		Messages:   []*domain.MessageHistory{},
		NextCursor: "",
		HasMore:    false,
	}

	// Mock message history service
	mockService.EXPECT().
		ListMessages(gomock.Any(), "ws123", gomock.Any()).
		Return(result, nil)

	// Should not panic when calling handleList
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMessageHistoryHandler_handleList_TracerErrorHandling(t *testing.T) {
	// Setup test
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockMessageHistoryService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTracer := pkgmocks.NewMockTracer(ctrl)

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewMessageHistoryHandlerWithTracer(
		mockService,
		mockAuthService,
		func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil },
		mockLogger,
		mockTracer,
	)

	// Create a request with valid parameters
	req := httptest.NewRequest(http.MethodGet, "/api/messages.list?workspace_id=ws123", nil)
	w := httptest.NewRecorder()

	// Create a user for authentication
	user := &domain.User{
		ID:    "user123",
		Email: "test@example.com",
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleList").
		Return(context.Background(), mockSpan)
	// Expect error marking on the span
	mockTracer.EXPECT().
		MarkSpanError(gomock.Any(), gomock.Any())
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Mock auth service to return success
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		Return(context.Background(), user, nil, nil)

	// Mock logger to expect error log
	mockLogger.EXPECT().Error(gomock.Any())

	// Mock message history service to return an error
	expectedErr := errors.New("service error")
	mockService.EXPECT().
		ListMessages(gomock.Any(), "ws123", gomock.Any()).
		Return(nil, expectedErr)

	// Call the handler
	handler.handleList(w, req)

	// Check response - should return internal server error
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to list messages", response["error"])
}

func TestMessageHistoryHandler_handleBroadcastStats_MethodNotAllowed(t *testing.T) {
	handler, _, _, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a non-GET request
	req := httptest.NewRequest(http.MethodPost, "/api/messages.broadcastStats", nil)
	w := httptest.NewRecorder()

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleBroadcastStats").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Call the handler
	handler.handleBroadcastStats(w, req)

	// Check response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestMessageHistoryHandler_handleBroadcastStats_MissingBroadcastID(t *testing.T) {
	handler, _, _, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with no broadcast_id
	req := httptest.NewRequest(http.MethodGet, "/api/messages.broadcastStats?workspace_id=ws123", nil)
	w := httptest.NewRecorder()

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleBroadcastStats").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Call the handler
	handler.handleBroadcastStats(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "broadcast_id is required", response["error"])
}

func TestMessageHistoryHandler_handleBroadcastStats_MissingWorkspaceID(t *testing.T) {
	handler, _, _, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with no workspace_id
	req := httptest.NewRequest(http.MethodGet, "/api/messages.broadcastStats?broadcast_id=bc123", nil)
	w := httptest.NewRecorder()

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleBroadcastStats").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Call the handler
	handler.handleBroadcastStats(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "workspace_id is required", response["error"])
}

func TestMessageHistoryHandler_handleBroadcastStats_ServiceError(t *testing.T) {
	// Setup test with controller that doesn't finish early
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockMessageHistoryService(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTracer := pkgmocks.NewMockTracer(ctrl)

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewMessageHistoryHandlerWithTracer(
		mockService,
		mockAuthService,
		func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil },
		mockLogger,
		mockTracer,
	)

	// Create a request with valid parameters
	req := httptest.NewRequest(http.MethodGet, "/api/messages.broadcastStats?workspace_id=ws123&broadcast_id=bc123", nil)
	w := httptest.NewRecorder()

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleBroadcastStats").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Set up logger mock to expect error call
	mockLogger.EXPECT().WithField("error", gomock.Any()).Return(mockLogger)
	mockLogger.EXPECT().Error("Failed to get stats").Times(1)

	// Mock message history service to return error
	mockService.EXPECT().
		GetBroadcastStats(gomock.Any(), "ws123", "bc123").
		Return(nil, errors.New("service error"))

	// Call the handler
	handler.handleBroadcastStats(w, req)

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to get stats", response["error"])
}

func TestMessageHistoryHandler_handleBroadcastStats_Success(t *testing.T) {
	handler, mockService, _, mockTracer, _ := setupMessageHistoryHandlerTest(t)

	// Create a request with valid parameters
	req := httptest.NewRequest(http.MethodGet, "/api/messages.broadcastStats?workspace_id=ws123&broadcast_id=bc123", nil)
	w := httptest.NewRecorder()

	// Mock the stats result
	stats := &domain.MessageHistoryStatusSum{
		TotalSent:         100,
		TotalDelivered:    90,
		TotalBounced:      5,
		TotalComplained:   1,
		TotalFailed:       4,
		TotalOpened:       50,
		TotalClicked:      30,
		TotalUnsubscribed: 2,
	}

	// Mock the tracer
	mockSpan := &trace.Span{}
	mockTracer.EXPECT().
		StartSpan(gomock.Any(), "MessageHistoryHandler.handleBroadcastStats").
		Return(context.Background(), mockSpan)
	mockTracer.EXPECT().
		EndSpan(mockSpan, nil)

	// Mock message history service
	mockService.EXPECT().
		GetBroadcastStats(gomock.Any(), "ws123", "bc123").
		Return(stats, nil)

	// Call the handler
	handler.handleBroadcastStats(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "bc123", response["broadcast_id"])

	// Check stats in response
	statsMap, ok := response["stats"].(map[string]interface{})
	require.True(t, ok, "Stats should be a map")
	assert.Equal(t, float64(100), statsMap["total_sent"])
	assert.Equal(t, float64(90), statsMap["total_delivered"])
	assert.Equal(t, float64(5), statsMap["total_bounced"])
	assert.Equal(t, float64(1), statsMap["total_complained"])
	assert.Equal(t, float64(4), statsMap["total_failed"])
	assert.Equal(t, float64(50), statsMap["total_opened"])
	assert.Equal(t, float64(30), statsMap["total_clicked"])
	assert.Equal(t, float64(2), statsMap["total_unsubscribed"])
}
