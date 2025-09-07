package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookRegistrationService_RegisterWebhooks(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWebhookProvider := mocks.NewMockWebhookProvider(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Test constants
	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"
	userID := "user-789"
	apiEndpoint := "https://api.notifuse.com"

	// Create a mock user
	user := &domain.User{ID: userID}

	tests := []struct {
		name               string
		emailProviderKind  domain.EmailProviderKind
		eventTypes         []domain.EmailEventType
		providerResponse   *domain.WebhookRegistrationStatus
		expectedError      string
		authError          error
		workspaceRepoError error
		providerError      error
	}{
		{
			name:              "Successfully register webhooks",
			emailProviderKind: domain.EmailProviderKindPostmark,
			eventTypes: []domain.EmailEventType{
				domain.EmailEventDelivered,
				domain.EmailEventBounce,
			},
			providerResponse: &domain.WebhookRegistrationStatus{
				EmailProviderKind: domain.EmailProviderKindPostmark,
				IsRegistered:      true,
				Endpoints: []domain.WebhookEndpointStatus{
					{
						WebhookID: "webhook-123",
						URL:       "https://api.notifuse.com/webhooks/email?provider=postmark&workspace_id=workspace-123&integration_id=integration-456",
						EventType: domain.EmailEventDelivered,
						Active:    true,
					},
				},
			},
			expectedError: "",
		},
		{
			name:              "Failed authentication",
			emailProviderKind: domain.EmailProviderKindPostmark,
			eventTypes:        []domain.EmailEventType{domain.EmailEventDelivered},
			providerResponse:  nil,
			expectedError:     "failed to authenticate user: authentication error",
			authError:         errors.New("authentication error"),
		},
		{
			name:               "Failed to get workspace",
			emailProviderKind:  domain.EmailProviderKindPostmark,
			eventTypes:         []domain.EmailEventType{domain.EmailEventDelivered},
			providerResponse:   nil,
			expectedError:      "failed to get email provider configuration: failed to get workspace: workspace not found",
			workspaceRepoError: errors.New("workspace not found"),
		},
		{
			name:              "Provider not implemented",
			emailProviderKind: "unknown-provider",
			eventTypes:        []domain.EmailEventType{domain.EmailEventDelivered},
			providerResponse:  nil,
			expectedError:     "webhook registration not implemented for provider: unknown-provider",
		},
		{
			name:              "Provider error",
			emailProviderKind: domain.EmailProviderKindPostmark,
			eventTypes:        []domain.EmailEventType{domain.EmailEventDelivered},
			providerResponse:  nil,
			expectedError:     "failed to register webhooks",
			providerError:     errors.New("failed to register webhooks"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a map of webhook providers
			webhookProviders := map[domain.EmailProviderKind]domain.WebhookProvider{
				domain.EmailProviderKindPostmark: mockWebhookProvider,
			}

			// Create service with the mocks
			svc := &WebhookRegistrationService{
				workspaceRepo:    mockWorkspaceRepo,
				authService:      mockAuthService,
				logger:           mockLogger,
				apiEndpoint:      apiEndpoint,
				webhookProviders: webhookProviders,
			}

			// Setup test-specific mock expectations
			config := &domain.WebhookRegistrationConfig{
				IntegrationID: integrationID,
				EventTypes:    tt.eventTypes,
			}

			if tt.authError != nil {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
					Return(nil, nil, nil, tt.authError).
					MaxTimes(1)
			} else {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
					Return(ctx, user, nil, nil).
					MaxTimes(1)

				if tt.workspaceRepoError != nil {
					mockWorkspaceRepo.EXPECT().
						GetByID(gomock.Any(), workspaceID).
						Return(nil, tt.workspaceRepoError).
						MaxTimes(1)
				} else {
					// Create an integration with the mock email provider
					integration := domain.Integration{
						ID: integrationID,
						EmailProvider: domain.EmailProvider{
							Kind: tt.emailProviderKind,
						},
					}

					// Create a workspace with integrations
					integrations := domain.Integrations{integration}
					workspace := &domain.Workspace{
						ID:           workspaceID,
						Integrations: integrations,
					}

					mockWorkspaceRepo.EXPECT().
						GetByID(gomock.Any(), workspaceID).
						Return(workspace, nil).
						MaxTimes(1)

					// Setup provider mock if we've passed workspace retrieval
					if tt.emailProviderKind == domain.EmailProviderKindPostmark {
						if tt.providerError != nil {
							mockWebhookProvider.EXPECT().
								RegisterWebhooks(
									gomock.Any(),
									workspaceID,
									integrationID,
									apiEndpoint,
									tt.eventTypes,
									gomock.Any(), // The email provider config
								).
								Return(nil, tt.providerError).
								MaxTimes(1)
						} else {
							mockWebhookProvider.EXPECT().
								RegisterWebhooks(
									gomock.Any(),
									workspaceID,
									integrationID,
									apiEndpoint,
									tt.eventTypes,
									gomock.Any(), // The email provider config
								).
								Return(tt.providerResponse, nil).
								MaxTimes(1)
						}
					}
				}
			}

			// Call the method under test
			result, err := svc.RegisterWebhooks(ctx, workspaceID, config)

			// Assert the result
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.providerResponse, result)
			}
		})
	}
}

func TestWebhookRegistrationService_GetWebhookStatus(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWebhookProvider := mocks.NewMockWebhookProvider(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Test constants
	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"
	userID := "user-789"
	apiEndpoint := "https://api.notifuse.com"

	// Create a mock user
	user := &domain.User{ID: userID}

	tests := []struct {
		name               string
		emailProviderKind  domain.EmailProviderKind
		providerResponse   *domain.WebhookRegistrationStatus
		expectedError      string
		authError          error
		workspaceRepoError error
		providerError      error
	}{
		{
			name:              "Successfully get webhook status",
			emailProviderKind: domain.EmailProviderKindMailgun,
			providerResponse: &domain.WebhookRegistrationStatus{
				EmailProviderKind: domain.EmailProviderKindMailgun,
				IsRegistered:      true,
				Endpoints: []domain.WebhookEndpointStatus{
					{
						WebhookID: "webhook-123",
						URL:       "https://api.notifuse.com/webhooks/email?provider=mailgun&workspace_id=workspace-123&integration_id=integration-456",
						EventType: domain.EmailEventDelivered,
						Active:    true,
					},
				},
			},
			expectedError: "",
		},
		{
			name:              "Failed authentication",
			emailProviderKind: domain.EmailProviderKindMailgun,
			providerResponse:  nil,
			expectedError:     "failed to authenticate user: authentication error",
			authError:         errors.New("authentication error"),
		},
		{
			name:               "Failed to get workspace",
			emailProviderKind:  domain.EmailProviderKindMailgun,
			providerResponse:   nil,
			expectedError:      "failed to get email provider configuration: failed to get workspace: workspace not found",
			workspaceRepoError: errors.New("workspace not found"),
		},
		{
			name:              "Provider not implemented",
			emailProviderKind: "unknown-provider",
			providerResponse:  nil,
			expectedError:     "webhook status check not implemented for provider: unknown-provider",
		},
		{
			name:              "Provider error",
			emailProviderKind: domain.EmailProviderKindMailgun,
			providerResponse:  nil,
			expectedError:     "failed to get webhook status",
			providerError:     errors.New("failed to get webhook status"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a map of webhook providers
			webhookProviders := map[domain.EmailProviderKind]domain.WebhookProvider{
				domain.EmailProviderKindMailgun: mockWebhookProvider,
			}

			// Create service with the mocks
			svc := &WebhookRegistrationService{
				workspaceRepo:    mockWorkspaceRepo,
				authService:      mockAuthService,
				logger:           mockLogger,
				apiEndpoint:      apiEndpoint,
				webhookProviders: webhookProviders,
			}

			if tt.authError != nil {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
					Return(nil, nil, nil, tt.authError).
					MaxTimes(1)
			} else {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
					Return(ctx, user, nil, nil).
					MaxTimes(1)

				if tt.workspaceRepoError != nil {
					mockWorkspaceRepo.EXPECT().
						GetByID(gomock.Any(), workspaceID).
						Return(nil, tt.workspaceRepoError).
						MaxTimes(1)
				} else {
					// Create an integration with the mock email provider
					integration := domain.Integration{
						ID: integrationID,
						EmailProvider: domain.EmailProvider{
							Kind: tt.emailProviderKind,
						},
					}

					// Create a workspace with integrations
					integrations := domain.Integrations{integration}
					workspace := &domain.Workspace{
						ID:           workspaceID,
						Integrations: integrations,
					}

					mockWorkspaceRepo.EXPECT().
						GetByID(gomock.Any(), workspaceID).
						Return(workspace, nil).
						MaxTimes(1)

					// Setup provider mock if we've passed workspace retrieval
					if tt.emailProviderKind == domain.EmailProviderKindMailgun {
						if tt.providerError != nil {
							mockWebhookProvider.EXPECT().
								GetWebhookStatus(
									gomock.Any(),
									workspaceID,
									integrationID,
									gomock.Any(), // The email provider config
								).
								Return(nil, tt.providerError).
								MaxTimes(1)
						} else {
							mockWebhookProvider.EXPECT().
								GetWebhookStatus(
									gomock.Any(),
									workspaceID,
									integrationID,
									gomock.Any(), // The email provider config
								).
								Return(tt.providerResponse, nil).
								MaxTimes(1)
						}
					}
				}
			}

			// Call the method under test
			result, err := svc.GetWebhookStatus(ctx, workspaceID, integrationID)

			// Assert the result
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.providerResponse, result)
			}
		})
	}
}

func TestWebhookRegistrationService_UnregisterWebhooks(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWebhookProvider := mocks.NewMockWebhookProvider(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Test constants
	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"
	userID := "user-789"
	apiEndpoint := "https://api.notifuse.com"

	// Create a mock user
	user := &domain.User{ID: userID}

	tests := []struct {
		name               string
		emailProviderKind  domain.EmailProviderKind
		expectedError      string
		authError          error
		workspaceRepoError error
		providerError      error
	}{
		{
			name:              "Successfully unregister webhooks",
			emailProviderKind: domain.EmailProviderKindSparkPost,
			expectedError:     "",
		},
		{
			name:              "Failed authentication",
			emailProviderKind: domain.EmailProviderKindSparkPost,
			expectedError:     "failed to authenticate user: authentication error",
			authError:         errors.New("authentication error"),
		},
		{
			name:               "Failed to get workspace",
			emailProviderKind:  domain.EmailProviderKindSparkPost,
			expectedError:      "failed to get email provider configuration: failed to get workspace: workspace not found",
			workspaceRepoError: errors.New("workspace not found"),
		},
		{
			name:              "Provider not implemented",
			emailProviderKind: "unknown-provider",
			expectedError:     "webhook unregistration not implemented for provider: unknown-provider",
		},
		{
			name:              "Provider error",
			emailProviderKind: domain.EmailProviderKindSparkPost,
			expectedError:     "failed to unregister webhooks",
			providerError:     errors.New("failed to unregister webhooks"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a map of webhook providers
			webhookProviders := map[domain.EmailProviderKind]domain.WebhookProvider{
				domain.EmailProviderKindSparkPost: mockWebhookProvider,
			}

			// Create service with the mocks
			svc := &WebhookRegistrationService{
				workspaceRepo:    mockWorkspaceRepo,
				authService:      mockAuthService,
				logger:           mockLogger,
				apiEndpoint:      apiEndpoint,
				webhookProviders: webhookProviders,
			}

			if tt.authError != nil {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
					Return(nil, nil, nil, tt.authError).
					MaxTimes(1)
			} else {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
					Return(ctx, user, nil, nil).
					MaxTimes(1)

				if tt.workspaceRepoError != nil {
					mockWorkspaceRepo.EXPECT().
						GetByID(gomock.Any(), workspaceID).
						Return(nil, tt.workspaceRepoError).
						MaxTimes(1)
				} else {
					// Create an integration with the mock email provider
					integration := domain.Integration{
						ID: integrationID,
						EmailProvider: domain.EmailProvider{
							Kind: tt.emailProviderKind,
						},
					}

					// Create a workspace with integrations
					integrations := domain.Integrations{integration}
					workspace := &domain.Workspace{
						ID:           workspaceID,
						Integrations: integrations,
					}

					mockWorkspaceRepo.EXPECT().
						GetByID(gomock.Any(), workspaceID).
						Return(workspace, nil).
						MaxTimes(1)

					// Setup provider mock if we've passed workspace retrieval
					if tt.emailProviderKind == domain.EmailProviderKindSparkPost {
						if tt.providerError != nil {
							mockWebhookProvider.EXPECT().
								UnregisterWebhooks(
									gomock.Any(),
									workspaceID,
									integrationID,
									gomock.Any(), // The email provider config
								).
								Return(tt.providerError).
								MaxTimes(1)
						} else {
							mockWebhookProvider.EXPECT().
								UnregisterWebhooks(
									gomock.Any(),
									workspaceID,
									integrationID,
									gomock.Any(), // The email provider config
								).
								Return(nil).
								MaxTimes(1)
						}
					}
				}
			}

			// Call the method under test
			err := svc.UnregisterWebhooks(ctx, workspaceID, integrationID)

			// Assert the result
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWebhookRegistrationService_GetEmailProviderConfig(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Test constants
	ctx := context.Background()
	workspaceID := "workspace-123"
	integrationID := "integration-456"
	apiEndpoint := "https://api.notifuse.com"

	tests := []struct {
		name                string
		emailProviderKind   domain.EmailProviderKind
		expectedErrorPrefix string
		workspaceRepoError  error
		integrationMissing  bool
	}{
		{
			name:                "Successfully get email provider config",
			emailProviderKind:   domain.EmailProviderKindMailjet,
			expectedErrorPrefix: "",
		},
		{
			name:                "Failed to get workspace",
			emailProviderKind:   domain.EmailProviderKindMailjet,
			expectedErrorPrefix: "failed to get workspace",
			workspaceRepoError:  errors.New("workspace not found"),
		},
		{
			name:                "Integration not found",
			emailProviderKind:   domain.EmailProviderKindMailjet,
			expectedErrorPrefix: "integration with ID integration-not-found not found",
			integrationMissing:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create service with the mocks
			svc := &WebhookRegistrationService{
				workspaceRepo: mockWorkspaceRepo,
				authService:   mockAuthService,
				logger:        mockLogger,
				apiEndpoint:   apiEndpoint,
			}

			testIntegrationID := integrationID
			if tt.integrationMissing {
				testIntegrationID = "integration-not-found"
			}

			if tt.workspaceRepoError != nil {
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), workspaceID).
					Return(nil, tt.workspaceRepoError).
					MaxTimes(1)
			} else {
				// Create an integration with the mock email provider
				integration := domain.Integration{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: tt.emailProviderKind,
					},
				}

				// Create a workspace with integrations
				integrations := domain.Integrations{integration}
				workspace := &domain.Workspace{
					ID:           workspaceID,
					Integrations: integrations,
				}

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), workspaceID).
					Return(workspace, nil).
					MaxTimes(1)
			}

			// Call the method under test using the unexported getEmailProviderConfig
			result, err := svc.getEmailProviderConfig(ctx, workspaceID, testIntegrationID)

			// Assert the result
			if tt.expectedErrorPrefix != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorPrefix)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.emailProviderKind, result.Kind)
			}
		})
	}
}

func TestNewWebhookRegistrationService(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Create provider mocks that implement WebhookProvider
	mockSparkPostService := mocks.NewMockSparkPostServiceInterface(ctrl)
	mockPostmarkService := mocks.NewMockPostmarkServiceInterface(ctrl)
	mockMailgunService := mocks.NewMockMailgunServiceInterface(ctrl)
	mockMailjetService := mocks.NewMockMailjetServiceInterface(ctrl)
	mockSESService := mocks.NewMockSESServiceInterface(ctrl)

	// Test constants
	apiEndpoint := "https://api.notifuse.com"

	// Create service with the mocks
	svc := NewWebhookRegistrationService(
		mockWorkspaceRepo,
		mockAuthService,
		mockPostmarkService,
		mockMailgunService,
		mockMailjetService,
		mockSparkPostService,
		mockSESService,
		mockLogger,
		apiEndpoint,
	)

	// Assertions
	require.NotNil(t, svc)
	assert.Equal(t, mockWorkspaceRepo, svc.workspaceRepo)
	assert.Equal(t, mockAuthService, svc.authService)
	assert.Equal(t, mockLogger, svc.logger)
	assert.Equal(t, apiEndpoint, svc.apiEndpoint)
	assert.NotNil(t, svc.webhookProviders)

	// The webhook providers map should be empty since our mocks don't implement the WebhookProvider interface
	assert.Equal(t, 0, len(svc.webhookProviders))
}
