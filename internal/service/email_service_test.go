package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
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

// emailFullAccess builds a member row — role "member", not "owner", so
// HasPermission actually consults the grants — holding read+write on every
// resource, for the cases that exercise the code past the permission gate.
func emailFullAccess() *domain.UserWorkspace {
	return &domain.UserWorkspace{
		UserID:      "user-123",
		WorkspaceID: "workspace-123",
		Role:        "member",
		Permissions: domain.NewFullPermissions(),
	}
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
			Return(ctx, &domain.User{ID: "user-123"}, emailFullAccess(), nil)

		// Provider should send an email - use gomock's Any matcher to be flexible
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(nil)

		// Call method under test
		err := emailService.TestEmailProvider(ctx, workspaceID, "", provider, toEmail)

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
		err := emailService.TestEmailProvider(ctx, workspaceID, "", provider, toEmail)

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
			Return(ctx, &domain.User{ID: "user-123"}, emailFullAccess(), nil)

		// Call method under test
		err := emailService.TestEmailProvider(ctx, workspaceID, "", provider, toEmail)

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
			Return(ctx, &domain.User{ID: "user-123"}, emailFullAccess(), nil)

		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).Return(assert.AnError)

		// Call method under test
		err := emailService.TestEmailProvider(ctx, workspaceID, "", provider, toEmail)

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
		apiEndpoint:     "https://api.notifuse.test",
	}

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "message-456"
	requestHost := "click.notifuse.test"

	t.Run("Successfully sets message as clicked without URL", func(t *testing.T) {
		// Setup message repository mock to expect SetClicked
		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), "").
			DoAndReturn(func(_ context.Context, _, _ string, timestamp time.Time, _ string) error {
				// Verify the timestamp is close to now
				assert.True(t, time.Since(timestamp) < time.Second)
				return nil
			})

		// No logger error expected

		// Call method under test
		err := emailService.VisitLink(ctx, messageID, workspaceID, "", requestHost)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("Records the clicked URL", func(t *testing.T) {
		clickedURL := "https://shop.example.com/product?utm_source=news"

		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), clickedURL).
			Return(nil)

		err := emailService.VisitLink(ctx, messageID, workspaceID, clickedURL, requestHost)
		require.NoError(t, err)
	})

	t.Run("Non-http scheme degrades to aggregate-only", func(t *testing.T) {
		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), "").
			Return(nil)

		err := emailService.VisitLink(ctx, messageID, workspaceID, "mailto:someone@example.com", requestHost)
		require.NoError(t, err)
	})

	t.Run("Overlong URL degrades to aggregate-only", func(t *testing.T) {
		longURL := "https://example.com/?q=" + strings.Repeat("a", 2048)

		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), "").
			Return(nil)

		err := emailService.VisitLink(ctx, messageID, workspaceID, longURL, requestHost)
		require.NoError(t, err)
	})

	t.Run("URL pointing back at the request host degrades to aggregate-only", func(t *testing.T) {
		// Unsubscribe/notification-center URLs embed the recipient's raw email
		// and are built on the same endpoint that serves the click redirect.
		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), "").
			Return(nil)

		err := emailService.VisitLink(ctx, messageID, workspaceID,
			"https://click.notifuse.test/unsubscribe?email=someone@example.com", requestHost)
		require.NoError(t, err)
	})

	t.Run("Host comparison is case-insensitive", func(t *testing.T) {
		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), "").
			Return(nil)

		err := emailService.VisitLink(ctx, messageID, workspaceID,
			"https://Click.Notifuse.Test/notification-center?email=someone@example.com", requestHost)
		require.NoError(t, err)
	})

	t.Run("Empty request host still records external URLs", func(t *testing.T) {
		clickedURL := "https://shop.example.com/product"

		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), clickedURL).
			Return(nil)

		err := emailService.VisitLink(ctx, messageID, workspaceID, clickedURL, "")
		require.NoError(t, err)
	})

	t.Run("Error setting clicked status", func(t *testing.T) {
		// Setup message repository mock to return an error
		mockMessageRepo.EXPECT().
			SetClicked(ctx, workspaceID, messageID, gomock.Any(), "").
			Return(assert.AnError)

		// Should log the error
		mockLogger.EXPECT().Error(gomock.Any())

		// Call method under test
		err := emailService.VisitLink(ctx, messageID, workspaceID, "", requestHost)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set clicked")
	})
}

// TestSanitizeClickedURL_CollapsesPerRecipientIdentityTokens is the assertion the
// whole strip exists for. The recorded URL becomes a JSONB key in clicked_links
// and per-link reporting groups by that key, so a token that varies per
// recipient would turn one row per link into one row per recipient.
func TestSanitizeClickedURL_CollapsesPerRecipientIdentityTokens(t *testing.T) {
	const secretKey = "workspace-secret-key-32-chars-min!"
	const link = "https://shop.example.com/product?utm_source=news"
	const requestHost = "click.notifuse.test"

	now := time.Now()

	t.Run("Two recipients clicking the same link record one key", func(t *testing.T) {
		aliceToken, err := domain.BuildWebIdentifyToken("alice@example.com", secretKey, domain.WebIdentifyTokenTTL, now)
		require.NoError(t, err)
		bobToken, err := domain.BuildWebIdentifyToken("bob@example.com", secretKey, domain.WebIdentifyTokenTTL, now)
		require.NoError(t, err)

		aliceURL := link + "&" + domain.WebIdentifyQueryParam + "=" + aliceToken
		bobURL := link + "&" + domain.WebIdentifyQueryParam + "=" + bobToken
		require.NotEqual(t, aliceURL, bobURL, "the two recipients must not share a destination URL, or the test proves nothing")

		recordedForAlice := sanitizeClickedURL(aliceURL, requestHost)
		recordedForBob := sanitizeClickedURL(bobURL, requestHost)

		assert.Equal(t, link, recordedForAlice)
		assert.Equal(t, recordedForAlice, recordedForBob, "both clicks must aggregate under a single clicked_links key")
		assert.NotContains(t, recordedForAlice, domain.WebIdentifyQueryParam, "no bearer identity credential may reach the workspace database")
	})

	t.Run("Two sends to the same recipient also record one key", func(t *testing.T) {
		// The token is encrypted over a fresh random nonce per call, so even one
		// recipient receiving the same campaign twice carries two distinct URLs.
		first, err := domain.BuildWebIdentifyToken("alice@example.com", secretKey, domain.WebIdentifyTokenTTL, now)
		require.NoError(t, err)
		second, err := domain.BuildWebIdentifyToken("alice@example.com", secretKey, domain.WebIdentifyTokenTTL, now)
		require.NoError(t, err)
		require.NotEqual(t, first, second, "EncryptString must seed a fresh nonce per call")

		assert.Equal(t,
			sanitizeClickedURL(link+"&"+domain.WebIdentifyQueryParam+"="+first, requestHost),
			sanitizeClickedURL(link+"&"+domain.WebIdentifyQueryParam+"="+second, requestHost))
	})
}

func TestSanitizeClickedURL_IdentifyTokenStripping(t *testing.T) {
	const requestHost = "click.notifuse.test"

	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL without an identify token is recorded byte-identical",
			url:      "https://shop.example.com/product?utm_source=news&utm_medium=email",
			expected: "https://shop.example.com/product?utm_source=news&utm_medium=email",
		},
		{
			name:     "Identify token as the only parameter leaves no dangling question mark",
			url:      "https://shop.example.com/product?nf_id=abc123",
			expected: "https://shop.example.com/product",
		},
		{
			name:     "Identify token among other parameters keeps their order",
			url:      "https://shop.example.com/product?utm_source=news&nf_id=abc123&utm_medium=email",
			expected: "https://shop.example.com/product?utm_source=news&utm_medium=email",
		},
		{
			name:     "Identify token first",
			url:      "https://shop.example.com/product?nf_id=abc123&utm_source=news",
			expected: "https://shop.example.com/product?utm_source=news",
		},
		{
			name:     "Identify token without a value",
			url:      "https://shop.example.com/product?nf_id&utm_source=news",
			expected: "https://shop.example.com/product?utm_source=news",
		},
		{
			name:     "A parameter merely starting with the token name is kept",
			url:      "https://shop.example.com/product?nf_ident=1&nf_id=abc123",
			expected: "https://shop.example.com/product?nf_ident=1",
		},
		{
			name:     "The fragment is not a query string",
			url:      "https://shop.example.com/product?utm_source=news#nf_id=abc123",
			expected: "https://shop.example.com/product?utm_source=news#nf_id=abc123",
		},
		{
			name:     "The fragment survives the strip",
			url:      "https://shop.example.com/product?nf_id=abc123#reviews",
			expected: "https://shop.example.com/product#reviews",
		},
		{
			name:     "An unrendered Liquid placeholder is preserved verbatim",
			url:      "https://shop.example.com/{{ contact.first_name }}?nf_id=abc123&utm_source=news",
			expected: "https://shop.example.com/{{ contact.first_name }}?utm_source=news",
		},
		{
			name:     "Userinfo and port are preserved",
			url:      "http://user:pw@shop.example.com:8443/product?nf_id=abc123",
			expected: "http://user:pw@shop.example.com:8443/product",
		},
		{
			name:     "Unparseable URL degrades to aggregate-only",
			url:      "https://shop.example.com/%zz?nf_id=abc123",
			expected: "",
		},
		{
			name:     "A token on a link back to the request host does not rescue it",
			url:      "https://click.notifuse.test/unsubscribe?email=someone@example.com&nf_id=abc123",
			expected: "",
		},
		{
			name:     "A token on a non-http scheme does not rescue it",
			url:      "ftp://files.example.com/report.pdf?nf_id=abc123",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, sanitizeClickedURL(tc.url, requestHost))
		})
	}
}

func TestSanitizeClickedURL_LengthCapIgnoresTheIdentifyToken(t *testing.T) {
	const secretKey = "workspace-secret-key-32-chars-min!"
	const requestHost = "click.notifuse.test"

	token, err := domain.BuildWebIdentifyToken("alice@example.com", secretKey, domain.WebIdentifyTokenTTL, time.Now())
	require.NoError(t, err)

	t.Run("A URL only over the cap because of the token is still recorded", func(t *testing.T) {
		prefix := "https://shop.example.com/product?q="
		atCap := prefix + strings.Repeat("a", maxRecordedClickedURLLength-len(prefix))
		withToken := atCap + "&" + domain.WebIdentifyQueryParam + "=" + token

		require.Len(t, atCap, maxRecordedClickedURLLength)
		require.Greater(t, len(withToken), maxRecordedClickedURLLength, "the token must be what pushes this URL over the cap")

		assert.Equal(t, atCap, sanitizeClickedURL(withToken, requestHost))
	})

	t.Run("A URL still over the cap once stripped degrades to aggregate-only", func(t *testing.T) {
		prefix := "https://shop.example.com/product?q="
		overCap := prefix + strings.Repeat("a", maxRecordedClickedURLLength-len(prefix)+1)
		withToken := overCap + "&" + domain.WebIdentifyQueryParam + "=" + token

		assert.Empty(t, sanitizeClickedURL(withToken, requestHost))
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
				// TransactionalNotificationID should be nil when not set in request
				assert.Nil(t, msgHistory.TransactionalNotificationID)

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

	t.Run("TrackingMode survives the compile-request rebuild", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:       workspaceID,
			Settings: domain.WorkspaceSettings{},
		}
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID).
			Return(workspace, nil)
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), workspaceID, templateConfig.TemplateID, int64(0)).
			Return(emailTemplate, nil)

		// The compile request's TrackingSettings are rebuilt field by field —
		// the per-notification opt-out must reach TrackLinks or opted-out auth
		// URLs get UTM-rewritten (the UTMContent default always sets a UTM field).
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, compileReq domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
				assert.Equal(t, notifuse_mjml.TrackingModeDisabled, compileReq.TrackingSettings.TrackingMode,
					"tracking_mode must be propagated into the compile request")
				assert.False(t, compileReq.TrackingSettings.EnableTracking)
				return compileResult, nil
			})
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)
		mockSESService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any()).
			Return(nil)

		request := domain.SendEmailRequest{
			WorkspaceID:    workspaceID,
			IntegrationID:  "test-integration-id",
			MessageID:      messageID,
			Contact:        contact,
			TemplateConfig: templateConfig,
			MessageData:    messageData,
			TrackingSettings: notifuse_mjml.TrackingSettings{
				EnableTracking: false,
				TrackingMode:   notifuse_mjml.TrackingModeDisabled,
			},
			EmailProvider: emailProvider,
			EmailOptions:  options,
		}
		require.NoError(t, emailService.SendEmailForTemplate(ctx, request))
	})

	t.Run("sends email with subject override processed through Liquid", func(t *testing.T) {
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

		// Setup email provider mock - capture the request to verify subject
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).DoAndReturn(func(ctx context.Context, req domain.SendEmailProviderRequest) error {
			// Verify the subject was overridden and Liquid processed
			assert.Equal(t, "Override Test User", req.Subject)
			return nil
		})

		// Call method under test with subject override
		overrideSubject := "Override {{ name }}"
		subjectOptions := domain.EmailOptions{
			Subject: &overrideSubject,
			ReplyTo: emailTemplate.Email.ReplyTo,
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
			EmailOptions:     subjectOptions,
		}
		err := emailService.SendEmailForTemplate(ctx, request)
		require.NoError(t, err)
	})

	t.Run("empty subject override uses template default", func(t *testing.T) {
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

		// Setup email provider mock - capture the request to verify subject uses template default
		mockSESService.EXPECT().
			SendEmail(
				gomock.Any(),
				gomock.Any(),
			).DoAndReturn(func(ctx context.Context, req domain.SendEmailProviderRequest) error {
			// Verify the subject is the template default (processed through Liquid)
			assert.Equal(t, "Welcome to Our Service", req.Subject)
			return nil
		})

		// Call method under test with empty subject override (should use template default)
		emptySubject := ""
		subjectOptions := domain.EmailOptions{
			Subject: &emptySubject,
			ReplyTo: emailTemplate.Email.ReplyTo,
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
			EmailOptions:     subjectOptions,
		}
		err := emailService.SendEmailForTemplate(ctx, request)
		require.NoError(t, err)
	})

	t.Run("sends email with subject_preview override passed to compilation", func(t *testing.T) {
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

		// Setup compile template mock - verify SubjectPreviewOverride is set
		mockTemplateService.EXPECT().
			CompileTemplate(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
				// Verify the SubjectPreviewOverride was passed to the compile request
				require.NotNil(t, req.SubjectPreviewOverride)
				assert.Equal(t, "Override preview", *req.SubjectPreviewOverride)
				return compileResult, nil
			})

		// Setup message repository mock
		mockMessageRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			Return(nil)

		// Setup email provider mock
		mockSESService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any()).
			Return(nil)

		// Call method under test with subject_preview override
		overridePreview := "Override preview"
		previewOptions := domain.EmailOptions{
			SubjectPreview: &overridePreview,
			ReplyTo:        emailTemplate.Email.ReplyTo,
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
			EmailOptions:     previewOptions,
		}
		err := emailService.SendEmailForTemplate(ctx, request)
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

		// The failure is recorded as a status write, never a row rewrite: the
		// in-memory copy still holds the plaintext template data, so anything
		// carrying message_data back to the row would undo the encryption Create
		// applied. gomock fails the test if any other repository call is made.
		mockMessageRepo.EXPECT().
			SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, wsID string, updates []domain.MessageEventUpdate) error {
				require.Len(t, updates, 1)
				assert.Equal(t, messageID, updates[0].ID)
				assert.Equal(t, domain.MessageEventFailed, updates[0].Event)
				assert.False(t, updates[0].Timestamp.IsZero())
				require.NotNil(t, updates[0].StatusInfo)
				assert.Contains(t, *updates[0].StatusInfo, assert.AnError.Error())

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

		// Setup message repository mock to fail recording the error status
		mockMessageRepo.EXPECT().
			SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).
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

// resolveIdentifyToken is the server side of the round trip: it answers who a
// minted token identifies, exactly as /track does on the next beat.
func resolveIdentifyToken(t *testing.T, token string, secretKey string) (string, bool) {
	t.Helper()
	return domain.ResolveWebIdentity(&domain.WebTrackPayload{IdentifyToken: &token}, secretKey, time.Now())
}

// capturedTrackingEndpoint is the API endpoint the capturing helper's service
// runs on, and therefore the prefix of the /r/{token} links TrackLinks builds
// from the settings it captures.
const capturedTrackingEndpoint = "https://track.notifuse.test"

// sendForTemplateCapturingTracking runs one send through SendEmailForTemplate
// against a workspace carrying the given web analytics settings, and returns the
// TrackingSettings the compiler was handed — the only place the identity fields
// are observable, since they are request-scoped and never persisted.
func sendForTemplateCapturingTracking(t *testing.T, webAnalytics *domain.WebAnalyticsSettings, contactEmail string, secretKey string) notifuse_mjml.TrackingSettings {
	t.Helper()
	return sendForTemplateCapturingTrackingWithOptions(t, webAnalytics, contactEmail, secretKey, domain.EmailOptions{})
}

// sendForTemplateCapturingTrackingWithOptions is the same send with the caller's
// EmailOptions attached, so a test can state who else the message goes to.
func sendForTemplateCapturingTrackingWithOptions(t *testing.T, webAnalytics *domain.WebAnalyticsSettings, contactEmail string, secretKey string, emailOptions domain.EmailOptions) notifuse_mjml.TrackingSettings {
	t.Helper()
	return sendForTemplateCapturingTrackingWithRequestSettings(t, webAnalytics, contactEmail, secretKey, emailOptions,
		notifuse_mjml.TrackingSettings{EnableTracking: true})
}

// sendForTemplateCapturingTrackingWithRequestSettings is the same send with the
// caller's own request-level TrackingSettings, so a test can state the
// per-notification tracking mode the transactional service resolved before the
// send — the value SendEmailForTemplate carries into the compiler.
func sendForTemplateCapturingTrackingWithRequestSettings(t *testing.T, webAnalytics *domain.WebAnalyticsSettings, contactEmail string, secretKey string, emailOptions domain.EmailOptions, requestTracking notifuse_mjml.TrackingSettings) notifuse_mjml.TrackingSettings {
	t.Helper()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockMessageRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockSESService := mocks.NewMockEmailProviderService(ctrl)

	emailService := EmailService{
		logger:          mockLogger,
		workspaceRepo:   mockWorkspaceRepo,
		templateService: mockTemplateService,
		messageRepo:     mockMessageRepo,
		sesService:      mockSESService,
		// The workspace below declares no custom endpoint, so this is what
		// ResolveEndpoint hands the compiler and what the /r/ links are built on.
		apiEndpoint: capturedTrackingEndpoint,
	}

	const workspaceID = "workspace-123"
	emailSender := domain.NewEmailSender("sender@example.com", "Sender Name")
	emailTemplate := &domain.Template{
		ID:   "template-789",
		Name: "Welcome Email",
		Email: &domain.EmailTemplate{
			Subject:  "Welcome",
			SenderID: emailSender.ID,
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
		},
	}
	compiledHTML := "<h1>Welcome!</h1>"

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), workspaceID).
		Return(&domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				SecretKey:    secretKey,
				WebAnalytics: webAnalytics,
			},
		}, nil)
	mockTemplateService.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, emailTemplate.ID, int64(0)).
		Return(emailTemplate, nil)

	var captured notifuse_mjml.TrackingSettings
	mockTemplateService.EXPECT().
		CompileTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			captured = req.TrackingSettings
			return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
		})
	mockMessageRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		Return(nil)
	mockSESService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		Return(nil)

	request := domain.SendEmailRequest{
		WorkspaceID:      workspaceID,
		IntegrationID:    "test-integration-id",
		MessageID:        "message-456",
		Contact:          &domain.Contact{Email: contactEmail},
		TemplateConfig:   domain.ChannelTemplate{TemplateID: emailTemplate.ID},
		TrackingSettings: requestTracking,
		EmailProvider: &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSES,
			Senders: []domain.EmailSender{emailSender},
			SES:     &domain.AmazonSESSettings{Region: "us-east-1"},
		},
		EmailOptions: emailOptions,
	}
	require.NoError(t, emailService.SendEmailForTemplate(context.Background(), request))

	return captured
}

// TestEmailService_SendEmailForTemplate_WebIdentity covers the "identified with
// no code" path: the transactional/automation send has to hand the compiler a
// credential for THIS recipient, or the tracked links land on the customer's
// site anonymously and the visit is never attributed.
func TestEmailService_SendEmailForTemplate_WebIdentity(t *testing.T) {
	const secretKey = "workspace-secret-key-32-chars-min!"
	const contactEmail = "test@example.com"

	t.Run("Mints the recipient's token when web analytics runs on declared domains", func(t *testing.T) {
		tracking := sendForTemplateCapturingTracking(t, &domain.WebAnalyticsSettings{
			Enabled:                true,
			IdentifyFromEmailLinks: true,
			AllowedDomains:         []string{"shop.example.com", "*.blog.example.com"},
		}, contactEmail, secretKey)

		require.NotEmpty(t, tracking.IdentifyToken)
		assert.Equal(t, []string{"shop.example.com", "*.blog.example.com"}, tracking.IdentifyAllowedHosts,
			"the token may only be handed to the hosts the workspace declared")

		resolved, ok := resolveIdentifyToken(t, tracking.IdentifyToken, secretKey)
		require.True(t, ok, "the token must survive the round trip through the workspace secret")
		assert.Equal(t, contactEmail, resolved)
	})

	t.Run("Mints nothing when the workspace has no web analytics settings", func(t *testing.T) {
		tracking := sendForTemplateCapturingTracking(t, nil, contactEmail, secretKey)

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Mints nothing when web analytics is configured but disabled", func(t *testing.T) {
		tracking := sendForTemplateCapturingTracking(t, &domain.WebAnalyticsSettings{
			Enabled:                false,
			IdentifyFromEmailLinks: true,
			AllowedDomains:         []string{"shop.example.com"},
		}, contactEmail, secretKey)

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Mints nothing until the workspace asks for email-link identification", func(t *testing.T) {
		// The gate that makes this path the workspace's decision rather than
		// ours. It was enforced on the broadcast sender alone for a while, which
		// left transactional and automation sends identifying every recipient
		// regardless — so it is worth a case on each path, not just one.
		tracking := sendForTemplateCapturingTracking(t, &domain.WebAnalyticsSettings{
			Enabled:        true,
			AllowedDomains: []string{"shop.example.com"},
		}, contactEmail, secretKey)

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Mints nothing when no destination domain is declared", func(t *testing.T) {
		// The one case worth stating twice: an empty allowlist means "accept
		// beats from any host" to the tracker. If the mint site read it the same
		// way, every link in the email — including third-party ones — would carry
		// a bearer identity for the recipient.
		tracking := sendForTemplateCapturingTracking(t, &domain.WebAnalyticsSettings{
			Enabled:        true,
			AllowedDomains: nil,
		}, contactEmail, secretKey)

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Each recipient gets their own token", func(t *testing.T) {
		webAnalytics := &domain.WebAnalyticsSettings{
			Enabled:                true,
			IdentifyFromEmailLinks: true,
			AllowedDomains:         []string{"shop.example.com"},
		}

		aliceTracking := sendForTemplateCapturingTracking(t, webAnalytics, "alice@example.com", secretKey)
		bobTracking := sendForTemplateCapturingTracking(t, webAnalytics, "bob@example.com", secretKey)

		require.NotEqual(t, aliceTracking.IdentifyToken, bobTracking.IdentifyToken,
			"a token hoisted above the per-recipient work would identify everyone as the first recipient")

		alice, ok := resolveIdentifyToken(t, aliceTracking.IdentifyToken, secretKey)
		require.True(t, ok)
		assert.Equal(t, "alice@example.com", alice)

		bob, ok := resolveIdentifyToken(t, bobTracking.IdentifyToken, secretKey)
		require.True(t, ok)
		assert.Equal(t, "bob@example.com", bob)
	})

	t.Run("A workspace without a secret key still sends, without identity", func(t *testing.T) {
		// Minting is the only thing that fails here, and it must cost the visit
		// its attribution rather than the email: sendForTemplateCapturingTracking
		// asserts the send itself succeeded.
		tracking := sendForTemplateCapturingTracking(t, &domain.WebAnalyticsSettings{
			Enabled:                true,
			IdentifyFromEmailLinks: true,
			AllowedDomains:         []string{"shop.example.com"},
		}, contactEmail, "")

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})
}

// TestEmailService_SendEmailForTemplate_WebIdentityAdditionalRecipients covers
// who else receives the message. The identity token names one contact and rides
// on every tracked link in the body, so any inbox that receives that body holds
// a working credential for the recipient — and CC/BCC arrive verbatim from the
// transactional.send request, which means a single misconfigured notification
// hands the token out automatically, to people neither party can see.
func TestEmailService_SendEmailForTemplate_WebIdentityAdditionalRecipients(t *testing.T) {
	const secretKey = "workspace-secret-key-32-chars-min!"
	const contactEmail = "test@example.com"

	webAnalytics := func() *domain.WebAnalyticsSettings {
		return &domain.WebAnalyticsSettings{
			Enabled:                true,
			IdentifyFromEmailLinks: true,
			AllowedDomains:         []string{"shop.example.com"},
		}
	}

	t.Run("Mints nothing when the send carries a CC", func(t *testing.T) {
		tracking := sendForTemplateCapturingTrackingWithOptions(t, webAnalytics(), contactEmail, secretKey,
			domain.EmailOptions{CC: []string{"colleague@example.com"}})

		assert.Empty(t, tracking.IdentifyToken, "a CC'd inbox must not receive a bearer identity for the recipient")
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Mints nothing when the send carries a BCC", func(t *testing.T) {
		// Worse than CC: the recipient cannot even see who else was handed their
		// identity.
		tracking := sendForTemplateCapturingTrackingWithOptions(t, webAnalytics(), contactEmail, secretKey,
			domain.EmailOptions{BCC: []string{"archive@example.com"}})

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Mints nothing when both are set", func(t *testing.T) {
		tracking := sendForTemplateCapturingTrackingWithOptions(t, webAnalytics(), contactEmail, secretKey,
			domain.EmailOptions{CC: []string{"a@example.com"}, BCC: []string{"b@example.com"}})

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Other email options do not suppress the identity", func(t *testing.T) {
		// The gate is about additional RECIPIENTS, not about EmailOptions being
		// populated: a send that only overrides the subject still reaches exactly
		// one mailbox.
		subject := "Overridden"
		tracking := sendForTemplateCapturingTrackingWithOptions(t, webAnalytics(), contactEmail, secretKey,
			domain.EmailOptions{Subject: &subject, ReplyTo: "support@example.com"})

		require.NotEmpty(t, tracking.IdentifyToken)
		resolved, ok := resolveIdentifyToken(t, tracking.IdentifyToken, secretKey)
		require.True(t, ok)
		assert.Equal(t, contactEmail, resolved)
	})

	t.Run("An empty CC slice is not an additional recipient", func(t *testing.T) {
		tracking := sendForTemplateCapturingTrackingWithOptions(t, webAnalytics(), contactEmail, secretKey,
			domain.EmailOptions{CC: []string{}, BCC: []string{}})

		assert.NotEmpty(t, tracking.IdentifyToken)
	})
}

// TestEmailService_SendEmailForTemplate_WebIdentityTrackingMode covers the
// per-notification opt-out. TrackingModeDisabled makes TrackLinks return the
// HTML untouched — no redirect, no UTM, no nf_id — so a token minted for such a
// send is a live 7-day bearer identity for the recipient that no link can ever
// carry. It is minted, encrypted, handed to the compiler and dropped.
//
// Not hypothetical: every Supabase auth notification (signup confirmation, magic
// link, recovery, invite, email change, reauth) is configured with this mode, so
// on a workspace running web analytics each of those sends used to mint one.
func TestEmailService_SendEmailForTemplate_WebIdentityTrackingMode(t *testing.T) {
	const secretKey = "workspace-secret-key-32-chars-min!"
	const contactEmail = "test@example.com"

	webAnalytics := func() *domain.WebAnalyticsSettings {
		return &domain.WebAnalyticsSettings{
			Enabled:                true,
			IdentifyFromEmailLinks: true,
			AllowedDomains:         []string{"shop.example.com"},
		}
	}

	send := func(t *testing.T, requestTracking notifuse_mjml.TrackingSettings) notifuse_mjml.TrackingSettings {
		t.Helper()
		return sendForTemplateCapturingTrackingWithRequestSettings(t, webAnalytics(), contactEmail, secretKey,
			domain.EmailOptions{}, requestTracking)
	}

	t.Run("Mints nothing when the notification opts out of tracking", func(t *testing.T) {
		// What the transactional service actually hands this method for an
		// opted-out notification: effectiveTracking resolves EnableTracking to
		// false, and the mode rides along.
		tracking := send(t, notifuse_mjml.TrackingSettings{
			TrackingMode:   notifuse_mjml.TrackingModeDisabled,
			EnableTracking: false,
		})

		assert.Empty(t, tracking.IdentifyToken, "an email that carries no tracked link must not mint a credential")
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Mints nothing when the opt-out arrives with tracking still flagged on", func(t *testing.T) {
		// The mode is the veto on its own: TrackLinks returns the HTML untouched
		// on TrackingModeDisabled before it ever reads EnableTracking, so a caller
		// that did not resolve the two against each other must not mint either.
		tracking := send(t, notifuse_mjml.TrackingSettings{
			TrackingMode:   notifuse_mjml.TrackingModeDisabled,
			EnableTracking: true,
		})

		assert.Empty(t, tracking.IdentifyToken)
		assert.Empty(t, tracking.IdentifyAllowedHosts)
	})

	t.Run("Mints on an explicit inherit", func(t *testing.T) {
		tracking := send(t, notifuse_mjml.TrackingSettings{
			TrackingMode:   notifuse_mjml.TrackingModeInherit,
			EnableTracking: true,
		})

		require.NotEmpty(t, tracking.IdentifyToken, "inherit defers to the workspace; it is not an opt-out")
		resolved, ok := resolveIdentifyToken(t, tracking.IdentifyToken, secretKey)
		require.True(t, ok)
		assert.Equal(t, contactEmail, resolved)
	})

	t.Run("Mints on the absent mode", func(t *testing.T) {
		// "" is what an inherit is stored as (canonicalTrackingMode) and what every
		// notification that never set a mode carries, so this is the common path.
		tracking := send(t, notifuse_mjml.TrackingSettings{EnableTracking: true})

		require.NotEmpty(t, tracking.IdentifyToken)
		resolved, ok := resolveIdentifyToken(t, tracking.IdentifyToken, secretKey)
		require.True(t, ok)
		assert.Equal(t, contactEmail, resolved)
	})

	t.Run("Mints when click tracking is off but the notification did not opt out", func(t *testing.T) {
		// A workspace can run web analytics with email click tracking off, and the
		// recipient still has to be identified on landing: TrackLinks' second early
		// return counts an identity token as a reason to rewrite the links. Gating
		// the mint on EnableTracking would silently undo that.
		for _, mode := range []string{"", notifuse_mjml.TrackingModeInherit} {
			tracking := send(t, notifuse_mjml.TrackingSettings{
				TrackingMode:   mode,
				EnableTracking: false,
			})

			require.NotEmpty(t, tracking.IdentifyToken, "mode %q with click tracking off must still identify the recipient", mode)
			resolved, ok := resolveIdentifyToken(t, tracking.IdentifyToken, secretKey)
			require.True(t, ok)
			assert.Equal(t, contactEmail, resolved)
		}
	})
}

// TestSanitizeClickedURL_StripsWhatTrackLinksWrites is the cross-package
// assertion. Every other strip test builds its URLs by concatenating this
// package's idea of the parameter name, so it can only prove the strip is
// self-consistent. The name that matters is the one notifuse_mjml WRITES onto a
// tracked link, one package away, and the only way to prove the two agree is to
// run the real chain: mint through SendEmailForTemplate, rewrite through
// TrackLinks, decrypt the /r/ token the recipient's browser would follow, then
// record it. If the two constants ever drift apart, this is the test that fails
// instead of a per-recipient credential quietly becoming a clicked_links key.
func TestSanitizeClickedURL_StripsWhatTrackLinksWrites(t *testing.T) {
	const secretKey = "workspace-secret-key-32-chars-min!"
	const requestHost = "track.notifuse.test"
	const linkHTML = `<a href="https://shop.example.com/product?utm_source=news">Buy now</a>`

	webAnalytics := &domain.WebAnalyticsSettings{
		Enabled:                true,
		IdentifyFromEmailLinks: true,
		AllowedDomains:         []string{"shop.example.com"},
	}

	// destinationFor sends to one recipient and returns the URL that recipient's
	// browser is redirected to — the string /r/ hands to VisitLink as clickedURL.
	destinationFor := func(recipient string) string {
		t.Helper()

		tracking := sendForTemplateCapturingTracking(t, webAnalytics, recipient, secretKey)
		require.NotEmpty(t, tracking.IdentifyToken, "the send must have minted a token, or this test proves nothing")

		trackedHTML, err := notifuse_mjml.TrackLinks(linkHTML, tracking)
		require.NoError(t, err)

		tokenRegex := regexp.MustCompile(regexp.QuoteMeta(capturedTrackingEndpoint) + `/r/([^"']+)`)
		m := tokenRegex.FindStringSubmatch(trackedHTML)
		require.NotNil(t, m, "expected a /r/{token} tracking link, got: %s", trackedHTML)

		// Payload format: messageID\nworkspaceID\ntimestamp\ndestinationURL
		plaintext, err := crypto.DecryptTrackingToken(m[1])
		require.NoError(t, err)
		parts := strings.Split(plaintext, "\n")
		require.Len(t, parts, 4, "unexpected token payload: %q", plaintext)
		return parts[3]
	}

	aliceDestination := destinationFor("alice@example.com")
	bobDestination := destinationFor("bob@example.com")

	require.Contains(t, aliceDestination, domain.WebIdentifyQueryParam+"=",
		"TrackLinks must have written the identity parameter onto the destination")
	require.NotEqual(t, aliceDestination, bobDestination,
		"the two recipients must land on different URLs, or the strip is not being exercised")

	aliceKey := sanitizeClickedURL(aliceDestination, requestHost)
	bobKey := sanitizeClickedURL(bobDestination, requestHost)

	require.NotEmpty(t, aliceKey, "the destination is on a third-party host and must stay recordable")
	assert.Equal(t, aliceKey, bobKey, "both clicks must aggregate under a single clicked_links key")
	assert.NotContains(t, aliceKey, domain.WebIdentifyQueryParam,
		"no bearer identity credential may reach the workspace database")
	assert.Contains(t, aliceKey, "https://shop.example.com/product", "the destination itself must survive the strip")
	assert.Contains(t, aliceKey, "utm_source=news", "the campaign parameters must survive the strip")
}

// TestEmailService_TestEmailProvider_PermissionEnforcement verifies that testing a
// provider requires transactional write: it sends a real email using the
// workspace's stored credentials, so read access is not enough. The member is
// granted the OPPOSITE permission, so the test fails both if the check is missing
// and if it is gated on read. No provider expectation is set, so gomock also fails
// if the send runs.
//
// The refusal has to name transactional:write specifically. The fixture grants a
// single resource, so a gate on any OTHER resource denies this member too — only
// reading the resource and verb back off the error tells the two apart, and both
// travel to the client through writePermissionError.
func TestEmailService_TestEmailProvider_PermissionEnforcement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockSESService := mocks.NewMockEmailProviderService(ctrl)

	emailService := EmailService{
		logger:        mockLogger,
		authService:   mockAuthService,
		workspaceRepo: mockWorkspaceRepo,
		sesService:    mockSESService,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"

	// role "member" (not "owner") so HasPermission actually consults the grants.
	readOnlyMember := &domain.UserWorkspace{
		UserID:      "user-123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTransactional: {Read: true},
		},
	}

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user-123"}, readOnlyMember, nil)

	provider := domain.EmailProvider{
		Kind:    domain.EmailProviderKindSES,
		Senders: []domain.EmailSender{{Email: "sender@example.com", Name: "Test Sender"}},
		SES: &domain.AmazonSESSettings{
			Region:    "us-east-1",
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
		},
	}

	err := emailService.TestEmailProvider(ctx, workspaceID, "", provider, "test@example.com")
	require.Error(t, err)

	var permErr *domain.PermissionError
	require.True(t, errors.As(err, &permErr), "expected a *domain.PermissionError, got %T: %v", err, err)
	assert.Equal(t, domain.PermissionResourceTransactional, permErr.Resource)
	assert.Equal(t, domain.PermissionTypeWrite, permErr.Permission)
}
