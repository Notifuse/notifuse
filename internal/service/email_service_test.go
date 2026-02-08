package service

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/preslavrachev/gomjml/mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailService_NewEmailService(t *testing.T) {
	// Setup the controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockMessageRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)

	secretKey := "test-secret-key"
	webhookEndpoint := "https://webhook.test"
	apiEndpoint := "https://api.test"
	isDemo := false

	t.Run("successful service creation with all dependencies", func(t *testing.T) {
		// Call the constructor
		service := NewEmailService(
			mockLogger,
			mockAuthService,
			secretKey,
			isDemo,
			mockWorkspaceRepo,
			mockTemplateRepo,
			mockTemplateService,
			mockMessageRepo,
			mockHTTPClient,
			webhookEndpoint,
			apiEndpoint,
		)

		// Verify the service is created and all fields are properly set
		require.NotNil(t, service)
		require.Equal(t, mockLogger, service.logger)
		require.Equal(t, mockAuthService, service.authService)
		require.Equal(t, secretKey, service.secretKey)
		require.Equal(t, mockWorkspaceRepo, service.workspaceRepo)
		require.Equal(t, mockTemplateRepo, service.templateRepo)
		require.Equal(t, mockTemplateService, service.templateService)
		require.Equal(t, mockMessageRepo, service.messageRepo)
		require.Equal(t, mockHTTPClient, service.httpClient)
		require.Equal(t, webhookEndpoint, service.webhookEndpoint)
		require.Equal(t, apiEndpoint, service.apiEndpoint)

		// Verify all provider services are initialized
		require.NotNil(t, service.smtpService)
		require.NotNil(t, service.sesService)
		require.NotNil(t, service.sparkPostService)
		require.NotNil(t, service.postmarkService)
		require.NotNil(t, service.mailgunService)
		require.NotNil(t, service.mailjetService)
	})

	t.Run("service creation with nil dependencies", func(t *testing.T) {
		// Test that the constructor handles nil dependencies gracefully
		service := NewEmailService(
			nil, // nil logger
			nil, // nil authService
			"",  // empty secretKey
			false,
			nil, // nil workspaceRepo
			nil, // nil templateRepo
			nil, // nil templateService
			nil, // nil messageRepo
			nil, // nil httpClient
			"",  // empty webhookEndpoint
			"",  // empty apiEndpoint
		)

		// Verify the service is still created (constructor doesn't validate inputs)
		require.NotNil(t, service)
		require.Nil(t, service.logger)
		require.Nil(t, service.authService)
		require.Equal(t, "", service.secretKey)
		require.Nil(t, service.workspaceRepo)
		require.Nil(t, service.templateRepo)
		require.Nil(t, service.templateService)
		require.Nil(t, service.messageRepo)
		require.Nil(t, service.httpClient)
		require.Equal(t, "", service.webhookEndpoint)
		require.Equal(t, "", service.apiEndpoint)

		// Provider services should still be initialized (they handle nil dependencies internally)
		require.NotNil(t, service.smtpService)
		require.NotNil(t, service.sesService)
		require.NotNil(t, service.sparkPostService)
		require.NotNil(t, service.postmarkService)
		require.NotNil(t, service.mailgunService)
		require.NotNil(t, service.mailjetService)
	})

	t.Run("service creation with empty string parameters", func(t *testing.T) {
		// Test with empty strings for string parameters
		service := NewEmailService(
			mockLogger,
			mockAuthService,
			"", // empty secretKey
			false,
			mockWorkspaceRepo,
			mockTemplateRepo,
			mockTemplateService,
			mockMessageRepo,
			mockHTTPClient,
			"", // empty webhookEndpoint
			"", // empty apiEndpoint
		)

		require.NotNil(t, service)
		require.Equal(t, "", service.secretKey)
		require.Equal(t, "", service.webhookEndpoint)
		require.Equal(t, "", service.apiEndpoint)

		// Other dependencies should be set correctly
		require.Equal(t, mockLogger, service.logger)
		require.Equal(t, mockAuthService, service.authService)
		require.Equal(t, mockWorkspaceRepo, service.workspaceRepo)
	})

	t.Run("verify provider service initialization", func(t *testing.T) {
		service := NewEmailService(
			mockLogger,
			mockAuthService,
			secretKey,
			false,
			mockWorkspaceRepo,
			mockTemplateRepo,
			mockTemplateService,
			mockMessageRepo,
			mockHTTPClient,
			webhookEndpoint,
			apiEndpoint,
		)

		// Test that getProviderService works for all provider types
		smtpService, err := service.getProviderService(domain.EmailProviderKindSMTP)
		require.NoError(t, err)
		require.NotNil(t, smtpService)
		require.Equal(t, service.smtpService, smtpService)

		sesService, err := service.getProviderService(domain.EmailProviderKindSES)
		require.NoError(t, err)
		require.NotNil(t, sesService)
		require.Equal(t, service.sesService, sesService)

		sparkPostService, err := service.getProviderService(domain.EmailProviderKindSparkPost)
		require.NoError(t, err)
		require.NotNil(t, sparkPostService)
		require.Equal(t, service.sparkPostService, sparkPostService)

		postmarkService, err := service.getProviderService(domain.EmailProviderKindPostmark)
		require.NoError(t, err)
		require.NotNil(t, postmarkService)
		require.Equal(t, service.postmarkService, postmarkService)

		mailgunService, err := service.getProviderService(domain.EmailProviderKindMailgun)
		require.NoError(t, err)
		require.NotNil(t, mailgunService)
		require.Equal(t, service.mailgunService, mailgunService)

		mailjetService, err := service.getProviderService(domain.EmailProviderKindMailjet)
		require.NoError(t, err)
		require.NotNil(t, mailjetService)
		require.Equal(t, service.mailjetService, mailjetService)
	})

	t.Run("verify service type and interface compliance", func(t *testing.T) {
		service := NewEmailService(
			mockLogger,
			mockAuthService,
			secretKey,
			false,
			mockWorkspaceRepo,
			mockTemplateRepo,
			mockTemplateService,
			mockMessageRepo,
			mockHTTPClient,
			webhookEndpoint,
			apiEndpoint,
		)

		// Verify the service implements the expected interface (compile-time check)
		var _ domain.EmailServiceInterface = service

		// Verify the service is of the correct type
		require.IsType(t, &EmailService{}, service)
	})

	t.Run("verify constructor parameters are used correctly", func(t *testing.T) {
		// Use specific values to verify they're set correctly
		specificSecretKey := "specific-secret-key-12345"
		specificWebhookEndpoint := "https://specific-webhook.example.com/webhook"
		specificAPIEndpoint := "https://specific-api.example.com/api"

		service := NewEmailService(
			mockLogger,
			mockAuthService,
			specificSecretKey,
			false,
			mockWorkspaceRepo,
			mockTemplateRepo,
			mockTemplateService,
			mockMessageRepo,
			mockHTTPClient,
			specificWebhookEndpoint,
			specificAPIEndpoint,
		)

		// Verify specific values are set correctly
		require.Equal(t, specificSecretKey, service.secretKey)
		require.Equal(t, specificWebhookEndpoint, service.webhookEndpoint)
		require.Equal(t, specificAPIEndpoint, service.apiEndpoint)
	})
}

func TestEmailService_CreateSESClient(t *testing.T) {
	t.Run("successful SES client creation", func(t *testing.T) {
		region := "us-east-1"
		accessKey := "test-access-key"
		secretKey := "test-secret-key"

		client := CreateSESClient(region, accessKey, secretKey)

		// Verify the client is created
		require.NotNil(t, client)

		// Verify the client is of the expected type
		require.Implements(t, (*domain.SESClient)(nil), client)
	})

	t.Run("SES client creation with empty parameters", func(t *testing.T) {
		// Test with empty parameters
		client := CreateSESClient("", "", "")

		// Client should still be created (AWS SDK handles validation)
		require.NotNil(t, client)
		require.Implements(t, (*domain.SESClient)(nil), client)
	})

	t.Run("SES client creation with different regions", func(t *testing.T) {
		regions := []string{"us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"}

		for _, region := range regions {
			client := CreateSESClient(region, "test-key", "test-secret")
			require.NotNil(t, client)
			require.Implements(t, (*domain.SESClient)(nil), client)
		}
	})

	t.Run("verify client independence", func(t *testing.T) {
		// Create multiple clients to verify they are independent
		client1 := CreateSESClient("us-east-1", "key1", "secret1")
		client2 := CreateSESClient("us-west-2", "key2", "secret2")

		require.NotNil(t, client1)
		require.NotNil(t, client2)

		// Verify they are different instances
		require.NotEqual(t, client1, client2)
	})
}

func TestEmailService_TestEmailProvider(t *testing.T) {
	// Setup the controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)

	// Create a mock email provider service using the generated mock
	mockSESService := mocks.NewMockEmailProviderService(ctrl)

	secretKey := "test-secret-key"
	webhookEndpoint := "https://webhook.test"

	// Create the email service with the generated mock
	emailService := EmailService{
		logger:          mockLogger,
		authService:     mockAuthService,
		secretKey:       secretKey,
		workspaceRepo:   mockWorkspaceRepo,
		templateRepo:    mockTemplateRepo,
		templateService: mockTemplateService,
		httpClient:      mockHTTPClient,
		webhookEndpoint: webhookEndpoint,
		sesService:      mockSESService,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"
	toEmail := "test@example.com"

	t.Run("Success with SES provider", func(t *testing.T) {
		// Create a provider for testing
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			Senders: []domain.EmailSender{
				{
					Email: "sender@example.com",
					Name:  "Test Sender",
				},
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		// Set up authentication mock
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, nil, nil)

		// Provider should send an email - use gomock's Any matcher to be flexible
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(nil)

		// Call method under test
		err := emailService.TestEmailProvider(ctx, workspaceID, provider, toEmail)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Authentication failure", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			Senders: []domain.EmailSender{
				{
					Email: "sender@example.com",
					Name:  "Test Sender",
				},
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, nil, nil, assert.AnError)

		// Call method under test
		err := emailService.TestEmailProvider(ctx, workspaceID, provider, toEmail)

		// Assertions
		require.Error(t, err)
	})

	t.Run("Provider validation failure", func(t *testing.T) {
		// Create an invalid provider with no senders
		provider := domain.EmailProvider{
			Kind:    domain.EmailProviderKindSES,
			Senders: []domain.EmailSender{}, // No senders at all
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, nil, nil)

		// Call method under test
		err := emailService.TestEmailProvider(ctx, workspaceID, provider, toEmail)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one sender is required")
	})

	t.Run("Email sending failure", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			Senders: []domain.EmailSender{
				{
					Email: "sender@example.com",
					Name:  "Test Sender",
				},
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user-123"}, nil, nil)

		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(assert.AnError)

		// Call method under test
		err := emailService.TestEmailProvider(ctx, workspaceID, provider, toEmail)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to test provider")
	})
}

func TestEmailService_SendEmail(t *testing.T) {
	// Setup the controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)

	// Create mocks for each email provider service using generated mocks
	mockSESService := mocks.NewMockEmailProviderService(ctrl)

	// Create the email service
	emailService := EmailService{
		logger:          mockLogger,
		authService:     mockAuthService,
		secretKey:       "test-secret-key",
		workspaceRepo:   mockWorkspaceRepo,
		templateRepo:    mockTemplateRepo,
		templateService: mockTemplateService,
		httpClient:      mockHTTPClient,
		webhookEndpoint: "https://webhook.test",
		sesService:      mockSESService,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"
	fromAddress := "sender@example.com"
	fromName := "Test Sender"
	toEmail := "recipient@example.com"
	subject := "Test Subject"
	content := "<html><body>Test content</body></html>"
	messageID := uuid.New().String()
	options := domain.EmailOptions{
		ReplyTo: "",
		CC:      nil,
		BCC:     nil,
	}

	t.Run("Basic SES provider", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			Senders: []domain.EmailSender{
				{
					ID:    uuid.New().String(),
					Email: "default@example.com",
					Name:  "Default Sender",
				},
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		// Set expectation
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(nil)

		// Call method under test
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            toEmail,
			Subject:       subject,
			Content:       content,
			Provider:      &provider,
			EmailOptions:  options,
		}
		err := emailService.SendEmail(ctx, request, false)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Unsupported provider kind", func(t *testing.T) {
		provider := domain.EmailProvider{
			Kind: "unsupported",
		}

		// Call method under test
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            toEmail,
			Subject:       subject,
			Content:       content,
			Provider:      &provider,
			EmailOptions:  options,
		}
		err := emailService.SendEmail(ctx, request, false)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported provider kind")
	})
}

func TestEmailService_getProviderService(t *testing.T) {
	// Setup the controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)

	// Email provider services using generated mocks
	mockSMTPService := mocks.NewMockEmailProviderService(ctrl)
	mockSESService := mocks.NewMockEmailProviderService(ctrl)
	mockSparkPostService := mocks.NewMockEmailProviderService(ctrl)
	mockPostmarkService := mocks.NewMockEmailProviderService(ctrl)
	mockMailgunService := mocks.NewMockEmailProviderService(ctrl)
	mockMailjetService := mocks.NewMockEmailProviderService(ctrl)

	// Create the email service
	emailService := EmailService{
		logger:           mockLogger,
		authService:      mockAuthService,
		workspaceRepo:    mockWorkspaceRepo,
		templateRepo:     mockTemplateRepo,
		templateService:  mockTemplateService,
		httpClient:       mockHTTPClient,
		smtpService:      mockSMTPService,
		sesService:       mockSESService,
		sparkPostService: mockSparkPostService,
		postmarkService:  mockPostmarkService,
		mailgunService:   mockMailgunService,
		mailjetService:   mockMailjetService,
	}

	tests := []struct {
		name         string
		providerKind domain.EmailProviderKind
		expected     domain.EmailProviderService
		expectError  bool
	}{
		{
			name:         "SMTP provider",
			providerKind: domain.EmailProviderKindSMTP,
			expected:     mockSMTPService,
			expectError:  false,
		},
		{
			name:         "SES provider",
			providerKind: domain.EmailProviderKindSES,
			expected:     mockSESService,
			expectError:  false,
		},
		{
			name:         "SparkPost provider",
			providerKind: domain.EmailProviderKindSparkPost,
			expected:     mockSparkPostService,
			expectError:  false,
		},
		{
			name:         "Postmark provider",
			providerKind: domain.EmailProviderKindPostmark,
			expected:     mockPostmarkService,
			expectError:  false,
		},
		{
			name:         "Mailgun provider",
			providerKind: domain.EmailProviderKindMailgun,
			expected:     mockMailgunService,
			expectError:  false,
		},
		{
			name:         "Mailjet provider",
			providerKind: domain.EmailProviderKindMailjet,
			expected:     mockMailjetService,
			expectError:  false,
		},
		{
			name:         "Unsupported provider",
			providerKind: "unsupported",
			expected:     nil,
			expectError:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			providerService, err := emailService.getProviderService(tc.providerKind)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, providerService)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, providerService)
			}
		})
	}
}

func TestEmailService_VisitLink(t *testing.T) {
	// Setup the controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockMessageRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Create the email service
	emailService := EmailService{
		logger:          mockLogger,
		authService:     mockAuthService,
		workspaceRepo:   mockWorkspaceRepo,
		templateRepo:    mockTemplateRepo,
		templateService: mockTemplateService,
		httpClient:      mockHTTPClient,
		messageRepo:     mockMessageRepo,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "message-456"

	t.Run("Successfully sets message as clicked", func(t *testing.T) {
		// Setup message repository mock to expect SetClicked
		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, timestamp time.Time) error {
				// Verify the timestamp is close to now
				assert.True(t, time.Since(timestamp) < time.Second)
				return nil
			})

		// No logger error expected

		// Call method under test
		err := emailService.VisitLink(ctx, messageID, workspaceID)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Error setting clicked status", func(t *testing.T) {
		// Setup message repository mock to return an error
		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any()).
			Return(assert.AnError)

		// Should log the error
		mockLogger.EXPECT().Error(gomock.Any())

		// Call method under test
		err := emailService.VisitLink(ctx, messageID, workspaceID)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set clicked")
	})
}

func TestEmailService_OpenEmail(t *testing.T) {
	// Setup the controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockMessageRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Create the email service
	emailService := EmailService{
		logger:          mockLogger,
		authService:     mockAuthService,
		workspaceRepo:   mockWorkspaceRepo,
		templateRepo:    mockTemplateRepo,
		templateService: mockTemplateService,
		httpClient:      mockHTTPClient,
		messageRepo:     mockMessageRepo,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "message-456"

	t.Run("Successfully sets message as opened", func(t *testing.T) {
		// Setup message repository mock to expect SetOpened
		mockMessageRepo.EXPECT().
			SetOpened(ctx, workspaceID, messageID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, timestamp time.Time) error {
				// Verify the timestamp is close to now
				assert.True(t, time.Since(timestamp) < time.Second)
				return nil
			})

		// No logger error expected

		// Call method under test
		err := emailService.OpenEmail(ctx, messageID, workspaceID)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Error setting opened status", func(t *testing.T) {
		// Setup message repository mock to return an error
		mockMessageRepo.EXPECT().
			SetOpened(ctx, workspaceID, messageID, gomock.Any()).
			Return(assert.AnError)

		// Setup logger mock to expect Error call
		mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().
			WithFields(gomock.Any()).
			Return(mockLoggerWithFields).
			AnyTimes()
		mockLoggerWithFields.EXPECT().
			Error(gomock.Any()).
			AnyTimes()

		// Call method under test
		err := emailService.OpenEmail(ctx, messageID, workspaceID)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update message opened")
	})
}

func TestEmailService_SendEmailForTemplate(t *testing.T) {
	// Setup the controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockMessageRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Email provider services using generated mocks
	mockSMTPService := mocks.NewMockEmailProviderService(ctrl)
	mockSESService := mocks.NewMockEmailProviderService(ctrl)
	mockSparkPostService := mocks.NewMockEmailProviderService(ctrl)
	mockPostmarkService := mocks.NewMockEmailProviderService(ctrl)
	mockMailgunService := mocks.NewMockEmailProviderService(ctrl)
	mockMailjetService := mocks.NewMockEmailProviderService(ctrl)

	// Create the email service
	emailService := EmailService{
		logger:           mockLogger,
		authService:      mockAuthService,
		workspaceRepo:    mockWorkspaceRepo,
		templateRepo:     mockTemplateRepo,
		templateService:  mockTemplateService,
		httpClient:       mockHTTPClient,
		messageRepo:      mockMessageRepo,
		webhookEndpoint:  "https://webhook.test",
		smtpService:      mockSMTPService,
		sesService:       mockSESService,
		sparkPostService: mockSparkPostService,
		postmarkService:  mockPostmarkService,
		mailgunService:   mockMailgunService,
		mailjetService:   mockMailjetService,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "message-456"

	// Create a contact
	contact := &domain.Contact{
		Email:     "test@example.com",
		FirstName: &domain.NullableString{String: "Test", IsNull: false},
		LastName:  &domain.NullableString{String: "User", IsNull: false},
	}

	// Create template config
	templateConfig := domain.ChannelTemplate{
		TemplateID: "template-789",
	}

	// Create message data
	messageData := domain.MessageData{
		Data: map[string]interface{}{
			"name": "Test User",
			"link": "https://example.com/test",
		},
	}

	// Create tracking settings
	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:       "https://track.example.com",
		EnableTracking: true,
		UTMSource:      "newsletter",
		UTMMedium:      "email",
		UTMCampaign:    "welcome",
		UTMContent:     "template-789",
		UTMTerm:        "new-user",
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender Name")

	// Create email provider
	emailProvider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSES,
		Senders: []domain.EmailSender{
			emailSender,
		},
		SES: &domain.AmazonSESSettings{
			Region:    "us-east-1",
			AccessKey: "access-key",
			SecretKey: "secret-key",
		},
	}

	// Set up common mock expectations for logger
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create email template
	emailTemplate := &domain.Template{
		ID:   "template-789",
		Name: "Welcome Email",
		Email: &domain.EmailTemplate{
			Subject:  "Welcome to Our Service",
			SenderID: emailSender.ID,
			ReplyTo:  "support@example.com",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
		},
	}

	// Create compile template result
	compiledHTML := "<h1>Welcome!</h1><p>Hello Test User, welcome to our service!</p>"
	compileResult := &domain.CompileTemplateResponse{
		Success: true,
		HTML:    &compiledHTML,
	}

	options := domain.EmailOptions{
		ReplyTo: emailTemplate.Email.ReplyTo,
		CC:      nil,
		BCC:     nil,
	}

	t.Run("Successfully sends email template", func(t *testing.T) {
		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil, // No custom endpoint for this test
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compileResult, nil)

		// Setup message repository mock
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, wsID string, _ string, msgHistory *domain.MessageHistory) error {
				// Verify message history properties
				assert.Equal(t, messageID, msgHistory.ID)
				assert.Equal(t, contact.Email, msgHistory.ContactEmail)
				assert.Equal(t, templateConfig.TemplateID, msgHistory.TemplateID)
				assert.Equal(t, "email", msgHistory.Channel)
				assert.Equal(t, messageData, msgHistory.MessageData)

				return nil
			})

		// Setup email provider mock
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(nil)

		// Call method under test
		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     options,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Error getting template", func(t *testing.T) {
		// Setup template service mock to return an error
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(nil, assert.AnError)

		// Logger should log the error
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call method under test
		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     options,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get template")
	})

	t.Run("Error compiling template", func(t *testing.T) {
		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock to return an error
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(nil, assert.AnError)

		// Logger should log the error
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call method under test
		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     options,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to compile template")
	})

	t.Run("Template compilation unsuccessful", func(t *testing.T) {
		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Create unsuccessful compile result
		unsuccessfulResult := &domain.CompileTemplateResponse{
			Success: false,
			Error: &mjml.Error{
				Message: "Template compilation error",
			},
		}

		// Setup compile template mock to return unsuccessful result
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(unsuccessfulResult, nil)

		// Logger should log the error
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call method under test
		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     options,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template compilation failed")
	})

	t.Run("Error creating message history", func(t *testing.T) {
		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compileResult, nil)

		// Setup message repository mock to return an error
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(assert.AnError)

		// Logger should log the error
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call method under test
		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     options,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create message history")
	})

	t.Run("Error sending email", func(t *testing.T) {
		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compileResult, nil)

		// Setup message repository mock
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)

		// Setup email provider mock to return an error
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(assert.AnError)

		// Setup message repository mock to update with error status
		mockMessageRepo.EXPECT().
			Update(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, wsID string, msgHistory *domain.MessageHistory) error {
				// Verify message history error properties
				assert.Equal(t, messageID, msgHistory.ID)
				assert.NotNil(t, msgHistory.StatusInfo)

				return nil
			})

		// Logger should log the error
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call method under test
		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     options,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("Error updating message history after failed email", func(t *testing.T) {
		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compileResult, nil)

		// Setup message repository mock
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)

		// Setup email provider mock to return an error
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(assert.AnError)

		// Setup message repository mock to fail updating with error status
		mockMessageRepo.EXPECT().
			Update(gomock.Any(), workspaceID, gomock.Any()).
			Return(assert.AnError)

		// Logger should log both errors
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call method under test
		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     options,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("Successfully sends email with from_name override", func(t *testing.T) {
		// Create custom from_name
		customFromName := "Custom Support Team"

		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compileResult, nil)

		// Setup message repository mock
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)

		// Setup email provider mock with custom matcher to verify from_name override
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest) error {
			// Verify that from_name was overridden
			assert.Equal(t, customFromName, req.FromName, "FromName should be overridden with custom value")
			assert.Equal(t, emailSender.Email, req.FromAddress, "FromAddress should remain unchanged")
			return nil
		})

		// Call method under test with from_name override
		optionsWithOverride := domain.EmailOptions{
			FromName: &customFromName,
			ReplyTo:  emailTemplate.Email.ReplyTo,
		}

		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     optionsWithOverride,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Uses default from_name when override is nil", func(t *testing.T) {
		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compileResult, nil)

		// Setup message repository mock
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)

		// Setup email provider mock to verify default from_name is used
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest) error {
			// Verify that default sender name is used
			assert.Equal(t, emailSender.Name, req.FromName, "FromName should use default sender name")
			assert.Equal(t, emailSender.Email, req.FromAddress, "FromAddress should remain unchanged")
			return nil
		})

		// Call method under test without from_name override
		optionsWithoutOverride := domain.EmailOptions{
			FromName: nil, // Explicitly nil
			ReplyTo:  emailTemplate.Email.ReplyTo,
		}

		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     optionsWithoutOverride,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Uses default from_name when override is empty string", func(t *testing.T) {
		emptyFromName := ""

		// Setup workspace mock
		workspace := &domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				CustomEndpointURL: nil,
			},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Setup template service mock
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// Setup compile template mock
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			Return(compileResult, nil)

		// Setup message repository mock
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)

		// Setup email provider mock to verify default from_name is used when empty string
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest) error {
			// Verify that default sender name is used (empty string should not override)
			assert.Equal(t, emailSender.Name, req.FromName, "FromName should use default sender name when override is empty string")
			return nil
		})

		// Call method under test with empty string from_name
		optionsWithEmptyOverride := domain.EmailOptions{
			FromName: &emptyFromName, // Empty string pointer
			ReplyTo:  emailTemplate.Email.ReplyTo,
		}

		request := domain.SendEmailRequest{
			WorkspaceID:      workspaceID,
			IntegrationID:    "test-integration-id",
			MessageID:        messageID,
			ExternalID:       nil,
			Contact:          contact,
			TemplateConfig:   templateConfig,
			MessageData:      messageData,
			TrackingSettings: trackingSettings,
			EmailProvider:    emailProvider,
			EmailOptions:     optionsWithEmptyOverride,
		}
		err := emailService.SendEmailForTemplate(ctx, request)

		// Assertions
		require.NoError(t, err)
	})
}
