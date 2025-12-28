package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func setupLLMHandlerTest(t *testing.T) (*LLMHandler, *mocks.MockLLMService) {
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockLLMService(ctrl)
	mockLogger := logger.NewLoggerWithLevel("disabled")

	getJWTSecret := func() ([]byte, error) {
		return []byte("test-secret"), nil
	}

	handler := NewLLMHandler(mockService, getJWTSecret, mockLogger)
	return handler, mockService
}

func TestLLMHandler_HandleChat_MethodNotAllowed(t *testing.T) {
	handler, _ := setupLLMHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/llm.chat", nil)
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestLLMHandler_HandleChat_InvalidJSON(t *testing.T) {
	handler, _ := setupLLMHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/llm.chat", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLLMHandler_HandleChat_ValidationError_MissingWorkspaceID(t *testing.T) {
	handler, _ := setupLLMHandlerTest(t)

	body := map[string]interface{}{
		"integration_id": "integration456",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/llm.chat", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "workspace_id is required")
}

func TestLLMHandler_HandleChat_ValidationError_MissingIntegrationID(t *testing.T) {
	handler, _ := setupLLMHandlerTest(t)

	body := map[string]interface{}{
		"workspace_id": "workspace123",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/llm.chat", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "integration_id is required")
}

func TestLLMHandler_HandleChat_ValidationError_EmptyMessages(t *testing.T) {
	handler, _ := setupLLMHandlerTest(t)

	body := map[string]interface{}{
		"workspace_id":   "workspace123",
		"integration_id": "integration456",
		"messages":       []map[string]string{},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/llm.chat", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least one message is required")
}

func TestLLMHandler_HandleChat_ServiceError(t *testing.T) {
	handler, mockService := setupLLMHandlerTest(t)

	body := map[string]interface{}{
		"workspace_id":   "workspace123",
		"integration_id": "integration456",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	mockService.EXPECT().
		StreamChat(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("service error")).
		Times(1)

	req := httptest.NewRequest(http.MethodPost, "/api/llm.chat", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	// The response should contain SSE error event
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "\"type\":\"error\"")
	assert.Contains(t, w.Body.String(), "service error")
}

func TestLLMHandler_HandleChat_Success(t *testing.T) {
	handler, mockService := setupLLMHandlerTest(t)

	body := map[string]interface{}{
		"workspace_id":   "workspace123",
		"integration_id": "integration456",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
		"system_prompt": "You are a helpful assistant.",
		"max_tokens":    1024,
	}
	bodyBytes, _ := json.Marshal(body)

	// Mock successful streaming - capture the onEvent callback and call it
	mockService.EXPECT().
		StreamChat(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx interface{}, req *domain.LLMChatRequest, onEvent func(domain.LLMChatEvent) error) error {
			// Verify request parameters
			assert.Equal(t, "workspace123", req.WorkspaceID)
			assert.Equal(t, "integration456", req.IntegrationID)
			assert.Equal(t, 1, len(req.Messages))
			assert.Equal(t, "user", req.Messages[0].Role)
			assert.Equal(t, "Hello", req.Messages[0].Content)
			assert.Equal(t, "You are a helpful assistant.", req.SystemPrompt)
			assert.Equal(t, 1024, req.MaxTokens)

			// Simulate streaming events
			onEvent(domain.LLMChatEvent{Type: "text", Content: "Hello"})
			onEvent(domain.LLMChatEvent{Type: "text", Content: " there!"})
			onEvent(domain.LLMChatEvent{Type: "done"})
			return nil
		}).
		Times(1)

	req := httptest.NewRequest(http.MethodPost, "/api/llm.chat", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	// Verify SSE response headers
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", w.Header().Get("Connection"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))

	// Verify SSE content
	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "data: {\"type\":\"text\",\"content\":\"Hello\"}")
	assert.Contains(t, responseBody, "data: {\"type\":\"text\",\"content\":\" there!\"}")
	assert.Contains(t, responseBody, "data: {\"type\":\"done\"}")
}

func TestLLMHandler_HandleChat_WithToolUse(t *testing.T) {
	handler, mockService := setupLLMHandlerTest(t)

	body := map[string]interface{}{
		"workspace_id":   "workspace123",
		"integration_id": "integration456",
		"messages": []map[string]string{
			{"role": "user", "content": "Write a blog post"},
		},
		"tools": []map[string]interface{}{
			{
				"name":         "update_blog_content",
				"description":  "Update blog content",
				"input_schema": map[string]interface{}{"type": "object"},
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	mockService.EXPECT().
		StreamChat(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx interface{}, req *domain.LLMChatRequest, onEvent func(domain.LLMChatEvent) error) error {
			// Verify tools are passed
			assert.Equal(t, 1, len(req.Tools))
			assert.Equal(t, "update_blog_content", req.Tools[0].Name)

			// Simulate tool use event
			onEvent(domain.LLMChatEvent{
				Type:      "tool_use",
				ToolName:  "update_blog_content",
				ToolInput: map[string]interface{}{"content": map[string]interface{}{"type": "doc"}, "message": "Updated"},
			})
			onEvent(domain.LLMChatEvent{Type: "done"})
			return nil
		}).
		Times(1)

	req := httptest.NewRequest(http.MethodPost, "/api/llm.chat", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler.handleChat(w, req)

	responseBody := w.Body.String()
	assert.Contains(t, responseBody, `"type":"tool_use"`)
	assert.Contains(t, responseBody, `"tool_name":"update_blog_content"`)
}

func TestLLMHandler_RegisterRoutes(t *testing.T) {
	handler, _ := setupLLMHandlerTest(t)
	mux := http.NewServeMux()

	// Should not panic
	require.NotPanics(t, func() {
		handler.RegisterRoutes(mux)
	})
}

func TestNewLLMHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockLLMService(ctrl)
	mockLogger := logger.NewLoggerWithLevel("disabled")

	getJWTSecret := func() ([]byte, error) {
		return []byte("test-secret"), nil
	}

	handler := NewLLMHandler(mockService, getJWTSecret, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockService, handler.service)
	assert.Equal(t, mockLogger, handler.logger)
	assert.NotNil(t, handler.getJWTSecret)
}
