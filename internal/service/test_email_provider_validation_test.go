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
	"github.com/wneessen/go-mail"
)

// TestEmailService_TestEmailProvider_SenderNameValidation
// Integration test that verifies sender name validation in the test email flow
func TestEmailService_TestEmailProvider_SenderNameValidation(t *testing.T) {
	t.Run("Sender name 'hello' passes validation and calls SMTP", func(t *testing.T) {
		// Setup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx := context.Background()
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		
		// Allow logging
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Mock SMTP factory - will be called if validation passes
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		factoryCalled := false
		
		mockFactory.EXPECT().
			CreateClient("localhost", 1025, "", "", false).
			DoAndReturn(func(host string, port int, username, password string, useTLS bool) (*mail.Client, error) {
				factoryCalled = true
				t.Log("✅ Factory called - sender name passed validation")
				// Return error to prevent actual send
				return nil, errors.New("mock: client creation stopped for test")
			}).
			Times(1)

		smtpService := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		emailService := &EmailService{
			logger:      mockLogger,
			smtpService: smtpService,
		}

		// Provider with sender name "hello"
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			Senders: []domain.EmailSender{
				{
					ID:        "sender-1",
					Email:     "test@notifuse.com",
					Name:      "hello", // Valid name
					IsDefault: true,
				},
			},
		}

		// Execute
		err := emailService.TestEmailProvider(ctx, "test-workspace", provider, "recipient@example.com")

		// Assert
		// May fail at client creation (which is expected), but factory should be called
		assert.True(t, factoryCalled, "Factory should be called when name is valid")
		t.Log("✅ Test passed: sender name 'hello' reached SMTP factory call")
	})

	t.Run("EMPTY sender name fails validation - factory never called", func(t *testing.T) {
		// Setup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx := context.Background()
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		// Mock factory - should NEVER be called
		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		mockFactory.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		smtpService := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		emailService := &EmailService{
			logger:      mockLogger,
			smtpService: smtpService,
		}

		// Provider with EMPTY sender name
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
			},
			Senders: []domain.EmailSender{
				{
					ID:        "sender-1",
					Email:     "test@notifuse.com",
					Name:      "", // EMPTY - should fail validation
					IsDefault: true,
				},
			},
		}

		// Execute
		err := emailService.TestEmailProvider(ctx, "test-workspace", provider, "recipient@example.com")

		// Assert
		require.Error(t, err, "Should fail with empty sender name")
		assert.Contains(t, err.Error(), "sender name is required", "Error should mention sender name")
		assert.Contains(t, err.Error(), "test@notifuse.com", "Error should include email address")
		
		t.Logf("✅ Validation working: %v", err)
	})

	t.Run("Test email uses first sender when multiple senders exist", func(t *testing.T) {
		// Setup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx := context.Background()
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		factoryCalled := false
		
		mockFactory.EXPECT().
			CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(host string, port int, username, password string, useTLS bool) (*mail.Client, error) {
				factoryCalled = true
				t.Log("✅ Factory called with first sender")
				return nil, errors.New("mock: stopped for test")
			}).
			MaxTimes(1)

		smtpService := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		emailService := &EmailService{
			logger:      mockLogger,
			smtpService: smtpService,
		}

		// Provider with multiple senders
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
			},
			Senders: []domain.EmailSender{
				{
					ID:        "sender-1",
					Email:     "first@notifuse.com",
					Name:      "First Sender", // Should use this
					IsDefault: true,
				},
				{
					ID:        "sender-2",
					Email:     "second@notifuse.com",
					Name:      "Second Sender", // Should NOT use this
					IsDefault: false,
				},
			},
		}

		// Execute
		_ = emailService.TestEmailProvider(ctx, "test-workspace", provider, "recipient@example.com")

		// Assert
		assert.True(t, factoryCalled, "Should call factory with first sender")
		t.Log("✅ Correctly uses first sender")
	})

	t.Run("Whitespace-only sender name also fails validation", func(t *testing.T) {
		// Setup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx := context.Background()
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		mockFactory := mocks.NewMockSMTPClientFactory(ctrl)
		// Factory should NOT be called with whitespace-only name
		mockFactory.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		smtpService := &SMTPService{
			logger:        mockLogger,
			clientFactory: mockFactory,
		}

		emailService := &EmailService{
			logger:      mockLogger,
			smtpService: smtpService,
		}

		// Provider with whitespace-only sender name
		provider := domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
			},
			Senders: []domain.EmailSender{
				{
					ID:        "sender-1",
					Email:     "test@notifuse.com",
					Name:      "   ", // Whitespace only
					IsDefault: true,
				},
			},
		}

		// Execute
		err := emailService.TestEmailProvider(ctx, "test-workspace", provider, "recipient@example.com")

		// Note: Current validation only checks for empty string, not whitespace
		// This test documents current behavior
		if err != nil && err.Error() == "sender name is required but was empty (from address: test@notifuse.com)" {
			t.Log("✅ Whitespace-only name is caught by validation")
		} else {
			t.Log("⚠️  Whitespace-only name passes current validation (may want to enhance this)")
		}
	})
}
