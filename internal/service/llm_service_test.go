package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func setupLLMServiceTest(t *testing.T) (
	*LLMService,
	*mocks.MockAuthService,
	*mocks.MockWorkspaceRepository,
) {
	ctrl := gomock.NewController(t)

	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLoggerWithLevel("disabled")

	// Create Firecrawl service and tool registry for testing
	firecrawlService := NewFirecrawlService(mockLogger)
	toolRegistry := NewServerSideToolRegistry(firecrawlService, mockLogger)

	service := NewLLMService(LLMServiceConfig{
		AuthService:   mockAuthService,
		WorkspaceRepo: mockWorkspaceRepo,
		Logger:        mockLogger,
		ToolRegistry:  toolRegistry,
	})

	return service, mockAuthService, mockWorkspaceRepo
}

func setupLLMContextWithAuth(mockAuthService *mocks.MockAuthService, workspaceID string, readPerm, writePerm bool) context.Context {
	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, workspaceID)

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceLLM: domain.ResourcePermissions{
				Read:  readPerm,
				Write: writePerm,
			},
		},
	}

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil).
		Times(1)

	return ctx
}

func TestLLMService_StreamChat_AuthenticationError(t *testing.T) {
	service, mockAuthService, _ := setupLLMServiceTest(t)

	ctx := context.Background()
	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "integration456",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "workspace123").
		Return(ctx, nil, nil, errors.New("authentication failed")).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestLLMService_StreamChat_PermissionDenied(t *testing.T) {
	service, mockAuthService, _ := setupLLMServiceTest(t)

	ctx := context.Background()
	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "integration456",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	// User without write permission
	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: "workspace123",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceLLM: domain.ResourcePermissions{
				Read:  true,
				Write: false, // No write permission
			},
		},
	}

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "workspace123").
		Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	permErr, ok := err.(*domain.PermissionError)
	assert.True(t, ok, "error should be a PermissionError")
	assert.Equal(t, domain.PermissionResourceLLM, permErr.Resource)
	assert.Equal(t, domain.PermissionTypeWrite, permErr.Permission)
}

func TestLLMService_StreamChat_WorkspaceNotFound(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "integration456",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(nil, errors.New("workspace not found")).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace")
}

func TestLLMService_StreamChat_IntegrationNotFound(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "nonexistent-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "other-integration",
				Name: "Other Integration",
				Type: domain.IntegrationTypeLLM,
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "integration not found")
}

func TestLLMService_StreamChat_NotLLMIntegration(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "email-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "email-integration",
				Name: "Email Provider",
				Type: domain.IntegrationTypeEmail, // Not LLM type
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "integration is not an LLM integration")
}

func TestLLMService_StreamChat_MissingLLMProvider(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:          "llm-integration",
				Name:        "LLM Provider",
				Type:        domain.IntegrationTypeLLM,
				LLMProvider: nil, // Missing provider config
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM provider configuration is missing")
}

func TestLLMService_StreamChat_MissingAnthropicConfig(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind:      "anthropic",
					Anthropic: nil, // Missing Anthropic config
				},
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Anthropic configuration is missing")
}

func TestLLMService_StreamChat_MissingOpenAIConfig(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind:   domain.LLMProviderKindOpenAI,
					OpenAI: nil, // Missing OpenAI config
				},
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI configuration is missing")
}

func TestLLMService_StreamChat_EmptyAPIKey_OpenAI(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind: domain.LLMProviderKindOpenAI,
					OpenAI: &domain.OpenAISettings{
						APIKey: "", // Empty API key
						Model:  "gpt-4.1",
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key is not configured")
}

// sseChatServer returns an httptest server that streams the given OpenAI
// chat.completion.chunk JSON payloads as Server-Sent Events, terminated by [DONE].
func sseChatServer(t *testing.T, chunks ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

// When a reasoning model exhausts its budget (finish_reason == "length"), the
// truncated tool call is dropped; the service must flag truncation on the terminal
// "done" event (a non-destructive signal) rather than a silent no-op or a terminal
// error that would wipe already-streamed content on the client.
func TestLLMService_StreamChat_OpenAI_TruncationEmitsError(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	srv := sseChatServer(t,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"gpt-4.1","choices":[{"index":0,"delta":{"role":"assistant","content":"partial..."},"finish_reason":null}]}`,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"gpt-4.1","choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`,
	)
	defer srv.Close()

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages:      []domain.LLMMessage{{Role: "user", Content: "Design an email"}},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind: domain.LLMProviderKindOpenAI,
					OpenAI: &domain.OpenAISettings{
						APIKey:  "test-key",
						Model:   "gpt-4.1",
						BaseURL: srv.URL,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	var events []domain.LLMChatEvent
	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		events = append(events, event)
		return nil
	})
	assert.NoError(t, err)

	// Truncation must be reported as a flag on the single terminal "done" event,
	// and must NOT be emitted as a terminal "error" (which the client uses to
	// replace already-streamed content).
	var doneEvents, errorEvents int
	var doneTruncated bool
	for _, e := range events {
		switch e.Type {
		case "done":
			doneEvents++
			doneTruncated = e.Truncated
		case "error":
			errorEvents++
		}
	}
	assert.Equal(t, 1, doneEvents, "expected exactly one terminal done event")
	assert.Zero(t, errorEvents, "truncation must not be reported as a terminal error event")
	assert.True(t, doneTruncated, "done event should be flagged truncated when finish_reason is 'length'")
}

// DeepSeek and other OpenAI-compatible reasoning models return their chain of
// thought in a non-standard "reasoning_content" delta field. The service must
// stream it as a separate "thinking" event, never mixed into the answer text.
func TestLLMService_StreamChat_OpenAI_ReasoningEmitsThinking(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	srv := sseChatServer(t,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think about the layout..."},"finish_reason":null}]}`,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":"Here is your email."},"finish_reason":null}]}`,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`,
	)
	defer srv.Close()

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages:      []domain.LLMMessage{{Role: "user", Content: "Design an email"}},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind: domain.LLMProviderKindOpenAI,
					OpenAI: &domain.OpenAISettings{
						APIKey:  "test-key",
						Model:   "deepseek-reasoner",
						BaseURL: srv.URL,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	var thinking, text string
	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		switch event.Type {
		case "thinking":
			thinking += event.Content
		case "text":
			text += event.Content
		}
		return nil
	})
	assert.NoError(t, err)

	assert.Contains(t, thinking, "think about the layout", "reasoning_content must stream as a thinking event")
	assert.Equal(t, "Here is your email.", text, "answer text must not contain the reasoning")
	assert.NotContains(t, text, "layout", "reasoning must not leak into the answer text")
}

// A configured reasoning_effort must be sent to the OpenAI API; an empty one must
// be omitted (some models/providers 400 on it).
func TestLLMService_StreamChat_OpenAI_ReasoningEffortPassthrough(t *testing.T) {
	cases := []struct {
		name        string
		effort      string
		wantInBody  bool
		wantLiteral string
	}{
		{name: "configured", effort: "medium", wantInBody: true, wantLiteral: `"reasoning_effort":"medium"`},
		{name: "empty omitted", effort: "", wantInBody: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

			var body string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"o4-mini","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`+"\n\n")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer srv.Close()

			req := &domain.LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "llm-integration",
				Messages:      []domain.LLMMessage{{Role: "user", Content: "Hi"}},
			}
			ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)
			workspace := &domain.Workspace{
				ID: "workspace123",
				Integrations: []domain.Integration{{
					ID:   "llm-integration",
					Type: domain.IntegrationTypeLLM,
					LLMProvider: &domain.LLMProvider{
						Kind: domain.LLMProviderKindOpenAI,
						OpenAI: &domain.OpenAISettings{
							APIKey:          "test-key",
							Model:           "o4-mini",
							BaseURL:         srv.URL,
							ReasoningEffort: tc.effort,
						},
					},
				}},
			}
			mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace123").Return(workspace, nil).Times(1)

			err := service.StreamChat(ctx, req, func(domain.LLMChatEvent) error { return nil })
			assert.NoError(t, err)

			if tc.wantInBody {
				assert.Contains(t, body, tc.wantLiteral)
			} else {
				assert.NotContains(t, body, "reasoning_effort")
			}
		})
	}
}

func TestLLMService_StreamChat_UnsupportedProvider(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind: "unsupported-provider",
				},
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported LLM provider")
}

func TestLLMService_StreamChat_EmptyAPIKey(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind: "anthropic",
					Anthropic: &domain.AnthropicSettings{
						APIKey: "", // Empty API key
						Model:  "claude-sonnet-4-20250514",
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key is not configured")
}

func TestNewLLMService(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLoggerWithLevel("disabled")

	config := LLMServiceConfig{
		AuthService:   mockAuthService,
		WorkspaceRepo: mockWorkspaceRepo,
		Logger:        mockLogger,
	}

	service := NewLLMService(config)

	assert.NotNil(t, service)
	assert.Equal(t, mockAuthService, service.authService)
	assert.Equal(t, mockWorkspaceRepo, service.workspaceRepo)
	assert.Equal(t, mockLogger, service.logger)
}

func TestCalculateCost(t *testing.T) {
	testCases := []struct {
		name           string
		model          string
		inputTokens    int64
		outputTokens   int64
		wantInputCost  float64
		wantOutputCost float64
		wantTotalCost  float64
	}{
		{
			name:           "Opus 4.6 - 1M input, 500K output",
			model:          "claude-opus-4-6",
			inputTokens:    1_000_000,
			outputTokens:   500_000,
			wantInputCost:  5.0,  // 1M * $5/MTok
			wantOutputCost: 12.5, // 500K * $25/MTok
			wantTotalCost:  17.5,
		},
		{
			name:           "Sonnet 4.6 - 1M input, 500K output",
			model:          "claude-sonnet-4-6",
			inputTokens:    1_000_000,
			outputTokens:   500_000,
			wantInputCost:  3.0, // 1M * $3/MTok
			wantOutputCost: 7.5, // 500K * $15/MTok
			wantTotalCost:  10.5,
		},
		{
			name:           "Haiku 4.5 - 1000 input, 500 output",
			model:          "claude-haiku-4-5-20251001",
			inputTokens:    1000,
			outputTokens:   500,
			wantInputCost:  0.001,  // 1K/1M * $1 = $0.001
			wantOutputCost: 0.0025, // 500/1M * $5 = $0.0025
			wantTotalCost:  0.0035,
		},
		{
			name:           "Unknown model - returns zero",
			model:          "unknown-model",
			inputTokens:    1000,
			outputTokens:   500,
			wantInputCost:  0,
			wantOutputCost: 0,
			wantTotalCost:  0,
		},
		{
			name:           "Zero tokens",
			model:          "claude-sonnet-4-6",
			inputTokens:    0,
			outputTokens:   0,
			wantInputCost:  0,
			wantOutputCost: 0,
			wantTotalCost:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inputCost, outputCost, totalCost := calculateCost(tc.model, tc.inputTokens, tc.outputTokens)
			assert.InDelta(t, tc.wantInputCost, inputCost, 0.0001, "input cost mismatch")
			assert.InDelta(t, tc.wantOutputCost, outputCost, 0.0001, "output cost mismatch")
			assert.InDelta(t, tc.wantTotalCost, totalCost, 0.0001, "total cost mismatch")
		})
	}
}
