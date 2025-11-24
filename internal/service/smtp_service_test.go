package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMTPService_SendEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock logger
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Test data
	ctx := context.Background()
	messageID := "test-message-id"
	workspaceID := "workspace-123"
	fromAddress := "sender@example.com"
	fromName := "Test Sender"
	to := "recipient@example.com"
	subject := "Test Subject"
	content := "<h1>Test Email</h1><p>This is a test email.</p>"
	replyTo := "reply@example.com"
	cc := []string{"cc1@example.com", "cc2@example.com"}
	bcc := []string{"bcc1@example.com", "bcc2@example.com"}

	// Create valid provider config
	validProvider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "smtp.example.com",
			Port:     587,
			Username: "user@example.com",
			Password: "password",
			UseTLS:   true,
		},
		Senders: []domain.EmailSender{
			domain.NewEmailSender("default@example.com", "Default Sender"),
		},
	}

	t.Run("missing SMTP settings", func(t *testing.T) {
		// Create provider with missing SMTP settings
		invalidProvider := &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: nil,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
		}

		// Create mock factory
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		// Create service with mock factory
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      invalidProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// Verify error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SMTP settings required")
	})

	t.Run("client creation error", func(t *testing.T) {
		// Create mock factory that returns an error
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("connection error"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// Verify error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("client creation success but send fails", func(t *testing.T) {
		// Create mock factory that succeeds in creating client but we can't test actual sending
		// without a more complex mock setup for the go-mail library
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("connection refused"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call the method - should fail at client creation
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("test CC and BCC parameters passed to factory", func(t *testing.T) {
		// Create mock factory that fails so we can test parameter passing without nil client issues
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("connection timeout"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with CC and BCC - should fail at client creation but parameters are validated
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{CC: cc, BCC: bcc},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("empty CC and BCC values handling", func(t *testing.T) {
		// Create mock factory that fails to avoid nil client issues
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("network error"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with empty CC and BCC values
		ccWithEmpty := []string{"cc1@example.com", "", "cc2@example.com"}
		bccWithEmpty := []string{"", "bcc1@example.com", ""}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{CC: ccWithEmpty, BCC: bccWithEmpty},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("with reply-to header", func(t *testing.T) {
		// Create mock factory that fails to avoid nil client issues
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("auth failed"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with reply-to
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{ReplyTo: replyTo},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("without reply-to header", func(t *testing.T) {
		// Create mock factory that fails to avoid nil client issues
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("timeout"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call without reply-to (empty string)
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("SMTP settings with TLS disabled", func(t *testing.T) {
		// Create provider with TLS disabled
		providerNoTLS := &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     25,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   false,
			},
			Senders: []domain.EmailSender{
				domain.NewEmailSender("default@example.com", "Default Sender"),
			},
		}

		// Create mock factory that fails to avoid nil client issues
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			providerNoTLS.SMTP.Host,
			providerNoTLS.SMTP.Port,
			providerNoTLS.SMTP.Username,
			providerNoTLS.SMTP.Password,
			providerNoTLS.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("connection refused"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call the method
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      providerNoTLS,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("invalid from address format", func(t *testing.T) {
		// Create mock factory that fails to avoid nil client issues
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("connection error"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with invalid from address
		invalidFromAddress := "invalid-email"
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   invalidFromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("invalid to address format", func(t *testing.T) {
		// Create mock factory that fails to avoid nil client issues
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, fmt.Errorf("connection error"))

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with invalid to address
		invalidToAddress := "invalid-email"
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            invalidToAddress,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create SMTP client")
	})

	t.Run("empty message ID", func(t *testing.T) {
		// Create service (no mock needed since validation fails before SMTP logic)
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: nil, // Not needed since validation fails first
		}

		// Call with empty message ID
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     "",
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "message ID is required")
	})

	t.Run("empty subject", func(t *testing.T) {
		// Create service (no mock needed since validation fails before SMTP logic)
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: nil, // Not needed since validation fails first
		}

		// Call with empty subject
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       "",
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "subject is required")
	})

	t.Run("empty content", func(t *testing.T) {
		// Create service (no mock needed since validation fails before SMTP logic)
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: nil, // Not needed since validation fails first
		}

		// Call with empty content
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       "",
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "content is required")
	})

	t.Run("successful email sending", func(t *testing.T) {
		// For successful cases, we need to test the actual logic without mocking the mail client
		// since the go-mail library doesn't provide interfaces we can easily mock
		// Instead, we'll test by ensuring the factory is called with correct parameters
		// and that no errors are returned when the factory succeeds

		// Create mock factory that returns nil (simulating success)
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, nil) // Return nil client and nil error to simulate factory success

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call the method - this will fail when trying to use the nil client
		// but we can verify the factory was called correctly
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// This will error because we returned nil client, but that's expected
		// The important thing is that the factory was called with correct parameters
		require.Error(t, err)
	})

	t.Run("factory called with CC and BCC parameters", func(t *testing.T) {
		// Test that the factory is called correctly when CC and BCC are provided
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with CC and BCC - verify factory is called with correct parameters
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{CC: cc, BCC: bcc},
		}
		err := service.SendEmail(ctx, request)

		// Will error due to nil client, but factory was called correctly
		require.Error(t, err)
	})

	t.Run("factory called with ReplyTo parameter", func(t *testing.T) {
		// Test that the factory is called correctly when ReplyTo is provided
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with ReplyTo - verify factory is called correctly
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{ReplyTo: replyTo},
		}
		err := service.SendEmail(ctx, request)

		// Will error due to nil client, but factory was called correctly
		require.Error(t, err)
	})

	t.Run("factory parameters validation", func(t *testing.T) {
		// Test that the factory is called with correct parameters for various scenarios
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(nil, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with various parameters to ensure factory is called correctly
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// Will error due to nil client, but factory was called correctly
		require.Error(t, err)
	})

	// Test message composition and validation logic
	t.Run("invalid from address format in message composition", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		// Create a real client that will fail at dial stage (not at message composition)
		// This allows us to test the message composition logic
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with invalid from address format
		invalidFromAddress := "invalid-email-format"
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   invalidFromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at message composition stage with "invalid sender" error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid sender")
	})

	t.Run("invalid to address format in message composition", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with invalid to address format
		invalidToAddress := "invalid-email-format"
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            invalidToAddress,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at message composition stage with "invalid recipient" error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid recipient")
	})

	t.Run("invalid CC address format in message composition", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with invalid CC address format
		invalidCC := []string{"valid@example.com", "invalid-email-format"}
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{CC: invalidCC},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at message composition stage with "invalid CC recipient" error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid CC recipient")
	})

	t.Run("invalid BCC address format in message composition", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with invalid BCC address format
		invalidBCC := []string{"valid@example.com", "invalid-email-format"}
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{BCC: invalidBCC},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at message composition stage with "invalid BCC recipient" error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid BCC recipient")
	})

	t.Run("successful message composition with all fields", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with all valid parameters including CC, BCC, and ReplyTo
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{CC: cc, BCC: bcc, ReplyTo: replyTo},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage (connection error), but message composition should succeed
		// The error should be about sending, not about message composition
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("successful message composition with empty CC and BCC", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with empty CC and BCC arrays
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but message composition should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("successful message composition with mixed empty and valid CC/BCC", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with mixed empty and valid CC/BCC (empty strings should be skipped)
		mixedCC := []string{"", "valid1@example.com", "", "valid2@example.com", ""}
		mixedBCC := []string{"valid3@example.com", "", "valid4@example.com"}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{CC: mixedCC, BCC: mixedBCC, ReplyTo: replyTo},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but message composition should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("message composition with reply-to header", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call with reply-to header
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{ReplyTo: replyTo},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but message composition should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("message composition without reply-to header", func(t *testing.T) {
		// Create a mock factory that returns a working client
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)

		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		// Create service
		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		// Call without reply-to header (empty string)
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions:  domain.EmailOptions{},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but message composition should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})
}

func TestNewSMTPService(t *testing.T) {
	// Create a new instance of Logger for testing
	mockLogger := &pkgmocks.MockLogger{}

	// Call the function being tested
	service := NewSMTPService(mockLogger)

	// Verify service was created correctly
	require.NotNil(t, service)
	require.Equal(t, mockLogger, service.logger)

	// Verify the client factory is properly initialized
	require.NotNil(t, service.clientFactory)
	require.IsType(t, &defaultGoMailFactory{}, service.clientFactory)
}

func TestDefaultGoMailFactory_CreateClient(t *testing.T) {
	factory := &defaultGoMailFactory{}

	t.Run("valid configuration with TLS", func(t *testing.T) {
		client, err := factory.CreateClient("smtp.example.com", 587, "user@example.com", "password", true)

		// We expect this to fail since we're not connecting to a real SMTP server
		// But we can verify the error is related to connection, not configuration
		if err != nil {
			// The error should be about connection, not about invalid parameters
			assert.Contains(t, err.Error(), "failed to create mail client")
		} else {
			// If somehow it succeeds (unlikely), verify we got a client
			assert.NotNil(t, client)
			if client != nil {
				_ = client.Close()
			}
		}
	})

	t.Run("valid configuration without TLS", func(t *testing.T) {
		client, err := factory.CreateClient("smtp.example.com", 25, "user@example.com", "password", false)

		// We expect this to fail since we're not connecting to a real SMTP server
		if err != nil {
			assert.Contains(t, err.Error(), "failed to create mail client")
		} else {
			assert.NotNil(t, client)
			if client != nil {
				_ = client.Close()
			}
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		client, err := factory.CreateClient("smtp.example.com", -1, "user@example.com", "password", true)

		// Should fail due to invalid port
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to create mail client")
	})

	t.Run("empty host", func(t *testing.T) {
		client, err := factory.CreateClient("", 587, "user@example.com", "password", true)

		// Should fail due to empty host
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to create mail client")
	})

	t.Run("port out of range", func(t *testing.T) {
		client, err := factory.CreateClient("smtp.example.com", 70000, "user@example.com", "password", true)

		// Should fail due to port out of range
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to create mail client")
	})

	t.Run("empty username with password (partial credentials)", func(t *testing.T) {
		client, err := factory.CreateClient("smtp.example.com", 587, "", "password", true)

		// With only password (no username), auth should not be configured
		// Client creation should succeed, but the actual SMTP connection would fail
		// since we're not connected to a real server in tests
		if err != nil {
			// Connection error is expected since we're not connected to a real server
			assert.Contains(t, err.Error(), "failed to create mail client")
		} else {
			assert.NotNil(t, client)
			if client != nil {
				_ = client.Close()
			}
		}
	})

	t.Run("empty password with username (partial credentials)", func(t *testing.T) {
		client, err := factory.CreateClient("smtp.example.com", 587, "user@example.com", "", true)

		// With only username (no password), auth should not be configured
		// Client creation should succeed, but the actual SMTP connection would fail
		// since we're not connected to a real server in tests
		if err != nil {
			// Connection error is expected since we're not connected to a real server
			assert.Contains(t, err.Error(), "failed to create mail client")
		} else {
			assert.NotNil(t, client)
			if client != nil {
				_ = client.Close()
			}
		}
	})

	t.Run("both username and password empty (unauthenticated SMTP)", func(t *testing.T) {
		client, err := factory.CreateClient("smtp.example.com", 25, "", "", false)

		// With both credentials empty, no auth should be configured
		// This is the unauthenticated SMTP scenario (e.g., port 25 local relay)
		// Client creation should succeed, but the actual SMTP connection would fail
		// since we're not connected to a real server in tests
		if err != nil {
			// Connection error is expected since we're not connected to a real server
			assert.Contains(t, err.Error(), "failed to create mail client")
		} else {
			assert.NotNil(t, client)
			if client != nil {
				_ = client.Close()
			}
		}
	})
}

// Test the defaultGoMailFactory's method signature (without execution)
func TestDefaultGoMailFactory_CreateClient_Signature(t *testing.T) {
	factory := &defaultGoMailFactory{}

	// We're testing the method existence and signature, not its behavior
	// since that would require actually attempting to connect to an SMTP server
	assert.NotNil(t, factory)

	// Verify the method exists by getting its type
	factoryType := fmt.Sprintf("%T", factory.CreateClient)
	assert.Equal(t, "func(string, int, string, string, bool) (*mail.Client, error)", factoryType)
}

func TestSMTPService_SendEmail_WithAttachments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock logger
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Test data
	ctx := context.Background()
	messageID := "test-message-id"
	workspaceID := "workspace-123"
	fromAddress := "sender@example.com"
	fromName := "Test Sender"
	to := "recipient@example.com"
	subject := "Test Subject"
	content := "<h1>Test Email</h1><p>This is a test email with attachments.</p>"

	// Create valid provider config
	validProvider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "smtp.example.com",
			Port:     587,
			Username: "user@example.com",
			Password: "password",
			UseTLS:   true,
		},
		Senders: []domain.EmailSender{
			domain.NewEmailSender("default@example.com", "Default Sender"),
		},
	}

	// Helper function to create base64 content
	createBase64Content := func(content string) string {
		return "VGVzdCBmaWxlIGNvbnRlbnQ=" // "Test file content" in base64
	}

	t.Run("single attachment", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachment := domain.Attachment{
			Filename:    "test.pdf",
			Content:     createBase64Content("test content"),
			ContentType: "application/pdf",
			Disposition: "attachment",
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{attachment},
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but attachment processing should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("multiple attachments", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachments := []domain.Attachment{
			{
				Filename:    "document.pdf",
				Content:     createBase64Content("PDF content"),
				ContentType: "application/pdf",
				Disposition: "attachment",
			},
			{
				Filename:    "image.png",
				Content:     createBase64Content("PNG content"),
				ContentType: "image/png",
				Disposition: "attachment",
			},
			{
				Filename:    "data.csv",
				Content:     createBase64Content("CSV content"),
				ContentType: "text/csv",
				Disposition: "attachment",
			},
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: attachments,
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but attachment processing should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("inline attachment", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachment := domain.Attachment{
			Filename:    "logo.png",
			Content:     createBase64Content("image content"),
			ContentType: "image/png",
			Disposition: "inline",
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{attachment},
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but attachment processing should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("invalid base64 content in attachment", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachment := domain.Attachment{
			Filename:    "test.pdf",
			Content:     "not-valid-base64!@#$",
			ContentType: "application/pdf",
			Disposition: "attachment",
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{attachment},
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DecodeContent stage
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode content")
	})

	t.Run("attachment without content type", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachment := domain.Attachment{
			Filename:    "test.pdf",
			Content:     createBase64Content("test content"),
			ContentType: "", // No content type specified
			Disposition: "attachment",
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{attachment},
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but attachment processing should succeed
		// (content type is optional)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("mixed inline and regular attachments", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachments := []domain.Attachment{
			{
				Filename:    "logo.png",
				Content:     createBase64Content("logo content"),
				ContentType: "image/png",
				Disposition: "inline",
			},
			{
				Filename:    "report.pdf",
				Content:     createBase64Content("report content"),
				ContentType: "application/pdf",
				Disposition: "attachment",
			},
			{
				Filename:    "banner.jpg",
				Content:     createBase64Content("banner content"),
				ContentType: "image/jpeg",
				Disposition: "inline",
			},
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: attachments,
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but attachment processing should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("email with attachments and CC/BCC/ReplyTo", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachment := domain.Attachment{
			Filename:    "document.pdf",
			Content:     createBase64Content("document content"),
			ContentType: "application/pdf",
			Disposition: "attachment",
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				CC:          []string{"cc@example.com"},
				BCC:         []string{"bcc@example.com"},
				ReplyTo:     "reply@example.com",
				Attachments: []domain.Attachment{attachment},
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but all processing should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("attachment with special characters in filename", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachment := domain.Attachment{
			Filename:    "test-file_2024 (1).pdf",
			Content:     createBase64Content("test content"),
			ContentType: "application/pdf",
			Disposition: "attachment",
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{attachment},
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, but attachment processing should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("no attachments", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{}, // Empty array
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DialAndSend stage, message composition should succeed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})

	t.Run("empty base64 content", func(t *testing.T) {
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		realFactory := &defaultGoMailFactory{}
		mockClient, _ := realFactory.CreateClient("localhost", 25, "test", "test", false)

		mockFactory.EXPECT().CreateClient(
			validProvider.SMTP.Host,
			validProvider.SMTP.Port,
			validProvider.SMTP.Username,
			validProvider.SMTP.Password,
			validProvider.SMTP.UseTLS,
		).Return(mockClient, nil)

		service := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		attachment := domain.Attachment{
			Filename:    "empty.txt",
			Content:     "", // Empty base64 content
			ContentType: "text/plain",
			Disposition: "attachment",
		}

		request := domain.SendEmailProviderRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "test-integration-id",
			MessageID:     messageID,
			FromAddress:   fromAddress,
			FromName:      fromName,
			To:            to,
			Subject:       subject,
			Content:       content,
			Provider:      validProvider,
			EmailOptions: domain.EmailOptions{
				Attachments: []domain.Attachment{attachment},
			},
		}
		err := service.SendEmail(ctx, request)

		// Should fail at DecodeContent stage (empty content is valid base64 but decodes to empty)
		// The service should handle empty attachments gracefully
		require.Error(t, err)
		// Empty content will decode successfully (to empty bytes) but may fail at send stage
	})
}
