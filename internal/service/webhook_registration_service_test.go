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

// ownerOf is the membership the ESP-side registration methods require: they read
// the workspace's un-redacted provider credentials, so they are owner-only rather
// than gated on a permission resource.
func ownerOf(workspaceID, userID string) *domain.UserWorkspace {
	return &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
}

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
					Return(ctx, user, ownerOf(workspaceID, userID), nil).
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

// fakeInboundProvider implements both WebhookProvider and InboundRouteRegistrar so we
// can verify the orchestration layer invokes EnsureInboundRoute after RegisterWebhooks.
type fakeInboundProvider struct {
	status          *domain.WebhookRegistrationStatus
	ensureErr       error
	ensureCallCount int
	gotInboundURL   string
}

func (f *fakeInboundProvider) RegisterWebhooks(_ context.Context, _, _ string, _ string, _ []domain.EmailEventType, _ *domain.EmailProvider) (*domain.WebhookRegistrationStatus, error) {
	return f.status, nil
}
func (f *fakeInboundProvider) GetWebhookStatus(_ context.Context, _, _ string, _ *domain.EmailProvider) (*domain.WebhookRegistrationStatus, error) {
	return f.status, nil
}
func (f *fakeInboundProvider) UnregisterWebhooks(_ context.Context, _, _ string, _ *domain.EmailProvider) error {
	return nil
}
func (f *fakeInboundProvider) EnsureInboundRoute(_ context.Context, _ *domain.EmailProvider, inboundURL string) error {
	f.ensureCallCount++
	f.gotInboundURL = inboundURL
	return f.ensureErr
}

func TestWebhookRegistrationService_RegisterWebhooks_RegistersInboundRoute(t *testing.T) {
	ctx := context.Background()
	const workspaceID, integrationID, apiEndpoint = "ws-1", "int-1", "https://api.notifuse.com"

	newSvc := func(ctrl *gomock.Controller, provider domain.WebhookProvider) *WebhookRegistrationService {
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "u"}, ownerOf(workspaceID, "u"), nil)
		workspace := &domain.Workspace{
			ID:           workspaceID,
			Integrations: domain.Integrations{{ID: integrationID, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindMailgun}}},
		}
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		return &WebhookRegistrationService{
			workspaceRepo:    mockWorkspaceRepo,
			authService:      mockAuthService,
			logger:           mockLogger,
			apiEndpoint:      apiEndpoint,
			webhookProviders: map[domain.EmailProviderKind]domain.WebhookProvider{domain.EmailProviderKindMailgun: provider},
		}
	}
	config := &domain.WebhookRegistrationConfig{IntegrationID: integrationID, EventTypes: []domain.EmailEventType{domain.EmailEventDelivered}}

	t.Run("invokes EnsureInboundRoute with the inbound URL", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		fake := &fakeInboundProvider{status: &domain.WebhookRegistrationStatus{IsRegistered: true}}
		svc := newSvc(ctrl, fake)

		result, err := svc.RegisterWebhooks(ctx, workspaceID, config)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 1, fake.ensureCallCount, "inbound route registration must run for a provider that supports it")
		assert.Equal(t, domain.GenerateInboundWebhookURL(apiEndpoint, workspaceID, integrationID), fake.gotInboundURL)
	})

	t.Run("inbound route failure does NOT fail the primary registration (best-effort)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		fake := &fakeInboundProvider{status: &domain.WebhookRegistrationStatus{IsRegistered: true}, ensureErr: errors.New("routes API down")}
		svc := newSvc(ctrl, fake)

		result, err := svc.RegisterWebhooks(ctx, workspaceID, config)

		// Inbound provisioning uses a broader permission surface; its failure must not roll back
		// the already-succeeded delivery/bounce/complaint registration. It is logged, not fatal.
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsRegistered)
		assert.Equal(t, 1, fake.ensureCallCount, "inbound route registration was attempted")
	})
}

func TestWebhookRegistrationService_persistSESInboundTopicARN(t *testing.T) {
	ctx := context.Background()
	newWS := func() *domain.Workspace {
		return &domain.Workspace{ID: "ws-1", Integrations: domain.Integrations{
			{ID: "int-1", EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{}}},
		}}
	}

	t.Run("persists the ARN onto the SES integration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(newWS(), nil)
		// Written through the atomic single-statement patch, not a full-row rewrite, so a
		// concurrent integration edit cannot silently drop it.
		mockRepo.EXPECT().
			PatchIntegrationSESSettings(gomock.Any(), "ws-1", "int-1", map[string]interface{}{
				"inbound_topic_arn": "arn:topic",
			}).
			Return(nil)
		svc := &WebhookRegistrationService{workspaceRepo: mockRepo}
		require.NoError(t, svc.persistSESInboundTopicARN(ctx, "ws-1", "int-1", "arn:topic"))
	})

	t.Run("no-op when the ARN is already persisted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
		ws := newWS()
		ws.Integrations[0].EmailProvider.SES.InboundTopicARN = "arn:topic"
		mockRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(ws, nil)
		// No Update expected — already in sync.
		svc := &WebhookRegistrationService{workspaceRepo: mockRepo}
		require.NoError(t, svc.persistSESInboundTopicARN(ctx, "ws-1", "int-1", "arn:topic"))
	})
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
					Return(ctx, user, ownerOf(workspaceID, userID), nil).
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
					Return(ctx, user, ownerOf(workspaceID, userID), nil).
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
	mockSendGridService := mocks.NewMockSendGridServiceInterface(ctrl)

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
		mockSendGridService,
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

// TestWebhookRegistrationService_OwnerOnly pins decision 8: ESP-side registration
// gets no permission resource. Every method here reads the workspace's un-redacted
// provider credentials and calls the ESP with them, so a member is refused however
// widely they are granted — the caller below holds read and write on all resources.
// No workspace-repository expectation is set, so gomock also fails if the
// credentials are loaded before the role is checked.
func TestWebhookRegistrationService_OwnerOnly(t *testing.T) {
	const workspaceID = "ws-1"
	const integrationID = "int-1"

	newService := func(t *testing.T, userWorkspace *domain.UserWorkspace) *WebhookRegistrationService {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				return ctx, &domain.User{ID: "u1"}, userWorkspace, nil
			})

		return &WebhookRegistrationService{
			workspaceRepo:    mocks.NewMockWorkspaceRepository(ctrl),
			authService:      mockAuthService,
			logger:           mockLogger,
			apiEndpoint:      "https://api.notifuse.com",
			webhookProviders: map[domain.EmailProviderKind]domain.WebhookProvider{},
		}
	}

	fullyGrantedMember := func() *domain.UserWorkspace {
		return &domain.UserWorkspace{
			UserID:      "u1",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.NewFullPermissions(),
		}
	}

	cases := []struct {
		name string
		call func(context.Context, *WebhookRegistrationService) error
	}{
		{"RegisterWebhooks", func(ctx context.Context, s *WebhookRegistrationService) error {
			_, err := s.RegisterWebhooks(ctx, workspaceID, &domain.WebhookRegistrationConfig{
				IntegrationID: integrationID,
				EventTypes:    []domain.EmailEventType{domain.EmailEventBounce},
			})
			return err
		}},
		{"GetWebhookStatus", func(ctx context.Context, s *WebhookRegistrationService) error {
			_, err := s.GetWebhookStatus(ctx, workspaceID, integrationID)
			return err
		}},
		{"UnregisterWebhooks", func(ctx context.Context, s *WebhookRegistrationService) error {
			return s.UnregisterWebhooks(ctx, workspaceID, integrationID)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name+" rejects a member", func(t *testing.T) {
			svc := newService(t, fullyGrantedMember())

			err := tc.call(context.Background(), svc)
			require.Error(t, err)
			assert.IsType(t, &domain.ErrUnauthorized{}, err)
			assert.Contains(t, err.Error(), "owner")
		})

		t.Run(tc.name+" rejects a nil membership", func(t *testing.T) {
			svc := newService(t, nil)

			err := tc.call(context.Background(), svc)
			require.Error(t, err)
			assert.IsType(t, &domain.ErrUnauthorized{}, err)
		})
	}
}
