package service

import (
	"context"
	"errors"
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
	assert.Contains(t, err.Error(), "LLM provider configuration is missing")
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
			name:           "Opus 4.5 - 1M input, 500K output",
			model:          "claude-opus-4-5-20251101",
			inputTokens:    1_000_000,
			outputTokens:   500_000,
			wantInputCost:  5.0,  // 1M * $5/MTok
			wantOutputCost: 12.5, // 500K * $25/MTok
			wantTotalCost:  17.5,
		},
		{
			name:           "Sonnet 4.5 - 1M input, 500K output",
			model:          "claude-sonnet-4-5-20250929",
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
			model:          "claude-sonnet-4-5-20250929",
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
