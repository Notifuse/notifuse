package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAmazonSESValidation(t *testing.T) {
	passphrase := "test-passphrase"

	tests := []struct {
		name    string
		ses     AmazonSESSettings
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Empty SES config",
			ses:     AmazonSESSettings{},
			wantErr: false,
		},
		{
			name: "Valid SES config",
			ses: AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
				SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			wantErr: false,
		},
		{
			name: "Missing region",
			ses: AmazonSESSettings{
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
				SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "Missing access key",
			ses: AmazonSESSettings{
				Region:    "us-east-1",
				SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			wantErr: true,
			errMsg:  "access key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ses.Validate(passphrase)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSMTPSettingsValidation(t *testing.T) {
	passphrase := "test-passphrase"

	tests := []struct {
		name    string
		smtp    SMTPSettings
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid SMTP settings",
			smtp: SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
			wantErr: false,
		},
		{
			name: "Missing host",
			smtp: SMTPSettings{
				Port:     587,
				Username: "user@example.com",
				Password: "password",
			},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name: "Invalid port - zero",
			smtp: SMTPSettings{
				Host:     "smtp.example.com",
				Port:     0,
				Username: "user@example.com",
				Password: "password",
			},
			wantErr: true,
			errMsg:  "invalid port number",
		},
		{
			name: "Invalid port - too large",
			smtp: SMTPSettings{
				Host:     "smtp.example.com",
				Port:     70000,
				Username: "user@example.com",
				Password: "password",
			},
			wantErr: true,
			errMsg:  "invalid port number",
		},
		{
			name: "Missing username (should be valid - username is optional)",
			smtp: SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Password: "password",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.smtp.Validate(passphrase)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSparkPostSettingsValidation(t *testing.T) {
	passphrase := "test-passphrase"

	tests := []struct {
		name      string
		sparkpost SparkPostSettings
		wantErr   bool
		errMsg    string
	}{
		{
			name: "Valid SparkPost settings",
			sparkpost: SparkPostSettings{
				APIKey:      "test-api-key",
				Endpoint:    "https://api.sparkpost.com",
				SandboxMode: false,
			},
			wantErr: false,
		},
		{
			name: "Missing endpoint",
			sparkpost: SparkPostSettings{
				APIKey:      "test-api-key",
				SandboxMode: false,
			},
			wantErr: true,
			errMsg:  "endpoint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sparkpost.Validate(passphrase)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEmailProviderValidation(t *testing.T) {
	passphrase := "test-passphrase"

	tests := []struct {
		name     string
		provider EmailProvider
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "Empty provider",
			provider: EmailProvider{},
			wantErr:  false,
		},
		{
			name: "Valid SMTP provider",
			provider: EmailProvider{
				Kind: EmailProviderKindSMTP,
				Senders: []EmailSender{
					NewEmailSender("default@example.com", "Default Sender"),
				},
				SMTP: &SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "user@example.com",
					Password: "password",
					UseTLS:   true,
				},
				RateLimitPerMinute: 25,
			},
			wantErr: false,
		},
		{
			name: "Valid SES provider with empty ID",
			provider: EmailProvider{
				Kind: EmailProviderKindSES,
				Senders: []EmailSender{
					NewEmailSender("default@example.com", "Default Sender"),
				},
				SES: &AmazonSESSettings{
					Region:    "us-east-1",
					AccessKey: "AKIAIOSFODNN7EXAMPLE",
					SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				},
				RateLimitPerMinute: 25,
			},
			wantErr: false,
		},
		{
			name: "Valid SparkPost provider",
			provider: EmailProvider{
				Kind: EmailProviderKindSparkPost,
				Senders: []EmailSender{
					NewEmailSender("default@example.com", "Default Sender"),
				},
				SparkPost: &SparkPostSettings{
					APIKey:   "test-api-key",
					Endpoint: "https://api.sparkpost.com",
				},
				RateLimitPerMinute: 25,
			},
			wantErr: false,
		},
		{
			name: "No senders",
			provider: EmailProvider{
				Kind:               EmailProviderKindSMTP,
				RateLimitPerMinute: 25,
				SMTP: &SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "user@example.com",
					Password: "password",
				},
			},
			wantErr: true,
			errMsg:  "at least one sender is required",
		},
		{
			name: "Invalid sender email",
			provider: EmailProvider{
				Kind: EmailProviderKindSMTP,
				Senders: []EmailSender{
					NewEmailSender("invalid-email", "Default Sender"),
				},
				SMTP: &SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "user@example.com",
					Password: "password",
				},
				RateLimitPerMinute: 25,
			},
			wantErr: true,
			errMsg:  "invalid sender email",
		},
		{
			name: "Missing sender name",
			provider: EmailProvider{
				Kind: EmailProviderKindSMTP,
				Senders: []EmailSender{
					NewEmailSender("default@example.com", ""),
				},
				SMTP: &SMTPSettings{
					Host:     "smtp.example.com",
					Port:     587,
					Username: "user@example.com",
					Password: "password",
				},
				RateLimitPerMinute: 25,
			},
			wantErr: true,
			errMsg:  "sender name is required",
		},
		{
			name: "Invalid kind",
			provider: EmailProvider{
				Kind: "invalid",
				Senders: []EmailSender{
					NewEmailSender("default@example.com", "Default Sender"),
				},
				RateLimitPerMinute: 25,
			},
			wantErr: true,
			errMsg:  "invalid email provider kind",
		},
		{
			name: "SMTP provider with nil SMTP settings",
			provider: EmailProvider{
				Kind:               EmailProviderKindSMTP,
				RateLimitPerMinute: 25,
				Senders: []EmailSender{
					NewEmailSender("default@example.com", "Default Sender"),
				},
			},
			wantErr: true,
			errMsg:  "SMTP settings required",
		},
		{
			name: "SES provider with nil SES settings",
			provider: EmailProvider{
				Kind:               EmailProviderKindSES,
				RateLimitPerMinute: 25,
				Senders: []EmailSender{
					NewEmailSender("default@example.com", "Default Sender"),
				},
			},
			wantErr: true,
			errMsg:  "SES settings required",
		},
		{
			name: "SparkPost provider with nil SparkPost settings",
			provider: EmailProvider{
				Kind:               EmailProviderKindSparkPost,
				RateLimitPerMinute: 25,
				Senders: []EmailSender{
					NewEmailSender("default@example.com", "Default Sender"),
				},
			},
			wantErr: true,
			errMsg:  "SparkPost settings required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Validate(passphrase)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEncryptionDecryption(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("SMTP password encryption/decryption", func(t *testing.T) {
		originalPassword := "test-password"
		smtp := SMTPSettings{
			Password: originalPassword,
		}

		// Encrypt
		err := smtp.EncryptPassword(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, smtp.EncryptedPassword)
		assert.NotEqual(t, originalPassword, smtp.EncryptedPassword)

		// Clear password
		smtp.Password = ""

		// Decrypt
		err = smtp.DecryptPassword(passphrase)
		require.NoError(t, err)
		assert.Equal(t, originalPassword, smtp.Password)
	})

	t.Run("SES secret key encryption/decryption", func(t *testing.T) {
		originalSecretKey := "test-secret-key"
		ses := AmazonSESSettings{
			SecretKey: originalSecretKey,
		}

		// Encrypt
		err := ses.EncryptSecretKey(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, ses.EncryptedSecretKey)
		assert.NotEqual(t, originalSecretKey, ses.EncryptedSecretKey)

		// Clear secret key
		ses.SecretKey = ""

		// Decrypt
		err = ses.DecryptSecretKey(passphrase)
		require.NoError(t, err)
		assert.Equal(t, originalSecretKey, ses.SecretKey)
	})

	t.Run("SparkPost API key encryption/decryption", func(t *testing.T) {
		originalAPIKey := "test-api-key"
		sparkpost := SparkPostSettings{
			APIKey: originalAPIKey,
		}

		// Encrypt
		err := sparkpost.EncryptAPIKey(passphrase)
		require.NoError(t, err)
		assert.NotEmpty(t, sparkpost.EncryptedAPIKey)
		assert.NotEqual(t, originalAPIKey, sparkpost.EncryptedAPIKey)

		// Clear API key
		sparkpost.APIKey = ""

		// Decrypt
		err = sparkpost.DecryptAPIKey(passphrase)
		require.NoError(t, err)
		assert.Equal(t, originalAPIKey, sparkpost.APIKey)
	})
}

func TestEmailProviderEncryptDecryptSecretKeys(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("SMTP provider secret keys", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindSMTP,
			SMTP: &SMTPSettings{
				Password: "test-password",
			},
		}

		// Encrypt all secret keys
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.SMTP.Password)
		assert.NotEmpty(t, provider.SMTP.EncryptedPassword)

		// Decrypt all secret keys
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-password", provider.SMTP.Password)
	})

	t.Run("SES provider secret keys", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindSES,
			SES: &AmazonSESSettings{
				SecretKey: "test-secret-key",
			},
		}

		// Encrypt all secret keys
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.SES.SecretKey)
		assert.NotEmpty(t, provider.SES.EncryptedSecretKey)

		// Decrypt all secret keys
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-secret-key", provider.SES.SecretKey)
	})

	t.Run("SparkPost provider secret keys", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindSparkPost,
			SparkPost: &SparkPostSettings{
				APIKey: "test-api-key",
			},
		}

		// Encrypt all secret keys
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.SparkPost.APIKey)
		assert.NotEmpty(t, provider.SparkPost.EncryptedAPIKey)

		// Decrypt all secret keys
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-api-key", provider.SparkPost.APIKey)
	})
}

func TestMailgunSettings_Validate(t *testing.T) {
	tests := []struct {
		name          string
		settings      MailgunSettings
		passphrase    string
		expectedError bool
	}{
		{
			name: "valid settings with API key",
			settings: MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
			passphrase:    "test-passphrase",
			expectedError: false,
		},
		{
			name: "valid settings without API key",
			settings: MailgunSettings{
				Domain: "example.com",
				Region: "EU",
			},
			passphrase:    "test-passphrase",
			expectedError: false,
		},
		{
			name: "missing domain",
			settings: MailgunSettings{
				APIKey: "test-api-key",
				Region: "US",
			},
			passphrase:    "test-passphrase",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate(tt.passphrase)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.settings.APIKey != "" {
					assert.NotEmpty(t, tt.settings.EncryptedAPIKey)
					assert.Empty(t, tt.settings.APIKey) // API key should be cleared after encryption
				}
			}
		})
	}
}

func TestMailgunSettings_EncryptDecryptAPIKey(t *testing.T) {
	settings := MailgunSettings{
		Domain: "example.com",
		APIKey: "test-api-key",
		Region: "US",
	}
	passphrase := "test-passphrase"

	// Test encryption
	err := settings.EncryptAPIKey(passphrase)
	assert.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedAPIKey)
	assert.NotEqual(t, "test-api-key", settings.EncryptedAPIKey)

	// Clear original API key
	originalAPIKey := settings.APIKey
	settings.APIKey = ""

	// Test decryption
	err = settings.DecryptAPIKey(passphrase)
	assert.NoError(t, err)
	assert.Equal(t, originalAPIKey, settings.APIKey)
}

func TestEmailProvider_ValidateWithMailgun(t *testing.T) {
	provider := EmailProvider{
		Kind:               EmailProviderKindMailgun,
		RateLimitPerMinute: 25,
		Senders: []EmailSender{
			NewEmailSender("sender@example.com", "Test Sender"),
		},
		Mailgun: &MailgunSettings{
			Domain: "example.com",
			APIKey: "test-api-key",
			Region: "US",
		},
	}

	err := provider.Validate("test-passphrase")
	assert.NoError(t, err)
	assert.NotEmpty(t, provider.Mailgun.EncryptedAPIKey)
	assert.Empty(t, provider.Mailgun.APIKey) // API key should be cleared after encryption
}

func TestMailjetSettings_Validate(t *testing.T) {
	tests := []struct {
		name          string
		settings      MailjetSettings
		passphrase    string
		expectedError bool
	}{
		{
			name: "valid settings with API key and Secret key",
			settings: MailjetSettings{
				APIKey:      "test-api-key",
				SecretKey:   "test-secret-key",
				SandboxMode: true,
			},
			passphrase:    "test-passphrase",
			expectedError: false,
		},
		{
			name: "valid settings with empty API key",
			settings: MailjetSettings{
				SandboxMode: true,
			},
			passphrase:    "test-passphrase",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate(tt.passphrase)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// If API key was provided, it should be encrypted
			if tt.settings.APIKey != "" {
				assert.NotEmpty(t, tt.settings.EncryptedAPIKey)
				assert.Empty(t, tt.settings.APIKey)
			}

			// If Secret key was provided, it should be encrypted
			if tt.settings.SecretKey != "" {
				assert.NotEmpty(t, tt.settings.EncryptedSecretKey)
				assert.Empty(t, tt.settings.SecretKey)
			}
		})
	}
}

func TestMailjetSettings_EncryptDecryptAPIKey(t *testing.T) {
	// Setup
	settings := MailjetSettings{
		APIKey: "test-api-key",
	}
	passphrase := "test-passphrase"

	// Test encryption
	err := settings.EncryptAPIKey(passphrase)
	assert.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedAPIKey)
	assert.NotEqual(t, "test-api-key", settings.EncryptedAPIKey)

	// Clear the API key and test decryption
	settings.APIKey = ""
	err = settings.DecryptAPIKey(passphrase)
	assert.NoError(t, err)
	assert.Equal(t, "test-api-key", settings.APIKey)
}

func TestMailjetSettings_EncryptDecryptSecretKey(t *testing.T) {
	// Setup
	settings := MailjetSettings{
		SecretKey: "test-secret-key",
	}
	passphrase := "test-passphrase"

	// Test encryption
	err := settings.EncryptSecretKey(passphrase)
	assert.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedSecretKey)
	assert.NotEqual(t, "test-secret-key", settings.EncryptedSecretKey)

	// Clear the Secret key and test decryption
	settings.SecretKey = ""
	err = settings.DecryptSecretKey(passphrase)
	assert.NoError(t, err)
	assert.Equal(t, "test-secret-key", settings.SecretKey)
}

func TestEmailProvider_ValidateWithMailjet(t *testing.T) {
	// Valid provider with Mailjet
	provider := EmailProvider{
		Kind:               EmailProviderKindMailjet,
		RateLimitPerMinute: 25,
		Senders: []EmailSender{
			NewEmailSender("from@example.com", "Test Sender"),
		},
		Mailjet: &MailjetSettings{
			APIKey:      "test-api-key",
			SecretKey:   "test-secret-key",
			SandboxMode: true,
		},
	}

	// Should validate without error
	err := provider.Validate("test-passphrase")
	assert.NoError(t, err)

	// Provider with missing Mailjet settings
	invalidProvider := EmailProvider{
		Kind:               EmailProviderKindMailjet,
		RateLimitPerMinute: 25,
		Senders: []EmailSender{
			NewEmailSender("from@example.com", "Test Sender"),
		},
	}

	// Should fail validation
	err = invalidProvider.Validate("test-passphrase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mailjet settings required")
}

func TestPostmarkSettings_Validate(t *testing.T) {
	tests := []struct {
		name          string
		settings      PostmarkSettings
		passphrase    string
		expectedError bool
	}{
		{
			name: "valid settings with server token",
			settings: PostmarkSettings{
				ServerToken: "test-server-token",
			},
			passphrase:    "test-passphrase",
			expectedError: false,
		},
		{
			name:          "valid settings with empty server token",
			settings:      PostmarkSettings{},
			passphrase:    "test-passphrase",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the settings to test
			settings := tt.settings

			err := settings.Validate(tt.passphrase)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.settings.ServerToken != "" {
					assert.NotEmpty(t, settings.EncryptedServerToken)
					// Unlike other providers, PostmarkSettings.Validate doesn't clear ServerToken
					// so we don't check for empty ServerToken here
				}
			}
		})
	}
}

func TestPostmarkSettings_EncryptDecryptServerToken(t *testing.T) {
	settings := PostmarkSettings{
		ServerToken: "test-server-token",
	}
	passphrase := "test-passphrase"

	// Test encryption
	err := settings.EncryptServerToken(passphrase)
	assert.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedServerToken)
	assert.NotEqual(t, "test-server-token", settings.EncryptedServerToken)

	// Clear original server token
	originalServerToken := settings.ServerToken
	settings.ServerToken = ""

	// Test decryption
	err = settings.DecryptServerToken(passphrase)
	assert.NoError(t, err)
	assert.Equal(t, originalServerToken, settings.ServerToken)
}

// Expand the existing test to cover all providers
func TestEmailProviderEncryptDecryptSecretKeys_AllProviders(t *testing.T) {
	passphrase := "test-passphrase"

	// Test Postmark provider
	t.Run("Postmark provider secret keys", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindPostmark,
			Postmark: &PostmarkSettings{
				ServerToken: "test-server-token",
			},
		}

		// Encrypt all secret keys
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.Postmark.ServerToken)
		assert.NotEmpty(t, provider.Postmark.EncryptedServerToken)

		// Decrypt all secret keys
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-server-token", provider.Postmark.ServerToken)
	})

	// Test Mailgun provider
	t.Run("Mailgun provider secret keys", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindMailgun,
			Mailgun: &MailgunSettings{
				APIKey: "test-api-key",
				Domain: "example.com",
			},
		}

		// Encrypt all secret keys
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.Mailgun.APIKey)
		assert.NotEmpty(t, provider.Mailgun.EncryptedAPIKey)

		// Decrypt all secret keys
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-api-key", provider.Mailgun.APIKey)
	})

	// Test Mailjet provider with both keys
	t.Run("Mailjet provider both secret keys", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindMailjet,
			Mailjet: &MailjetSettings{
				APIKey:    "test-api-key",
				SecretKey: "test-secret-key",
			},
		}

		// Encrypt all secret keys
		err := provider.EncryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Empty(t, provider.Mailjet.APIKey)
		assert.Empty(t, provider.Mailjet.SecretKey)
		assert.NotEmpty(t, provider.Mailjet.EncryptedAPIKey)
		assert.NotEmpty(t, provider.Mailjet.EncryptedSecretKey)

		// Decrypt all secret keys
		err = provider.DecryptSecretKeys(passphrase)
		require.NoError(t, err)
		assert.Equal(t, "test-api-key", provider.Mailjet.APIKey)
		assert.Equal(t, "test-secret-key", provider.Mailjet.SecretKey)
	})
}

func TestEmailProvider_ValidateWithPostmark(t *testing.T) {
	// Valid provider with Postmark
	provider := EmailProvider{
		Kind:               EmailProviderKindPostmark,
		RateLimitPerMinute: 25,
		Senders: []EmailSender{
			NewEmailSender("from@example.com", "Test Sender"),
		},
		Postmark: &PostmarkSettings{
			ServerToken: "test-server-token",
		},
	}

	// Should validate without error
	err := provider.Validate("test-passphrase")
	assert.NoError(t, err)
	assert.NotEmpty(t, provider.Postmark.EncryptedServerToken)
	// Unlike other providers, PostmarkSettings.Validate doesn't clear ServerToken
	// so we don't check for empty ServerToken here

	// Provider with missing Postmark settings
	invalidProvider := EmailProvider{
		Kind:               EmailProviderKindPostmark,
		RateLimitPerMinute: 25,
		Senders: []EmailSender{
			NewEmailSender("from@example.com", "Test Sender"),
		},
	}

	// Should fail validation
	err = invalidProvider.Validate("test-passphrase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "postmark settings required")
}

// Add decryption error tests that don't rely on mocking
func TestDecryptionErrors(t *testing.T) {
	// Test decryption errors by using invalid encrypted values

	// SES decryption error
	t.Run("SES decryption error", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindSES,
			SES: &AmazonSESSettings{
				// Set invalid encrypted data
				EncryptedSecretKey: "invalid-encrypted-data",
			},
		}

		err := provider.DecryptSecretKeys("any-passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt SES secret key")
	})

	// SMTP decryption error
	t.Run("SMTP decryption error", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindSMTP,
			SMTP: &SMTPSettings{
				// Set invalid encrypted data
				EncryptedPassword: "invalid-encrypted-data",
			},
		}

		err := provider.DecryptSecretKeys("any-passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt SMTP password")
	})

	// SparkPost decryption error
	t.Run("SparkPost decryption error", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindSparkPost,
			SparkPost: &SparkPostSettings{
				// Set invalid encrypted data
				EncryptedAPIKey: "invalid-encrypted-data",
			},
		}

		err := provider.DecryptSecretKeys("any-passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt SparkPost API key")
	})

	// Postmark decryption error
	t.Run("Postmark decryption error", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindPostmark,
			Postmark: &PostmarkSettings{
				// Set invalid encrypted data
				EncryptedServerToken: "invalid-encrypted-data",
			},
		}

		err := provider.DecryptSecretKeys("any-passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt Postmark server token")
	})

	// Mailgun decryption error
	t.Run("Mailgun decryption error", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindMailgun,
			Mailgun: &MailgunSettings{
				// Set invalid encrypted data
				EncryptedAPIKey: "invalid-encrypted-data",
			},
		}

		err := provider.DecryptSecretKeys("any-passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt Mailgun API key")
	})

	// Mailjet decryption errors
	t.Run("Mailjet API key decryption error", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindMailjet,
			Mailjet: &MailjetSettings{
				// Set invalid encrypted data
				EncryptedAPIKey: "invalid-encrypted-data",
			},
		}

		err := provider.DecryptSecretKeys("any-passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt Mailjet API key")
	})

	t.Run("Mailjet Secret key decryption error", func(t *testing.T) {
		provider := EmailProvider{
			Kind: EmailProviderKindMailjet,
			Mailjet: &MailjetSettings{
				// Set invalid encrypted data for secret key only
				EncryptedSecretKey: "invalid-encrypted-data",
			},
		}

		err := provider.DecryptSecretKeys("any-passphrase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt Mailjet Secret key")
	})
}

// Add more validations for edge cases and missing settings
func TestEmailProvider_AdditionalValidation(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Invalid kind with valid settings", func(t *testing.T) {
		provider := EmailProvider{
			Kind:               "invalid",
			RateLimitPerMinute: 25,
			Senders: []EmailSender{
				NewEmailSender("default@example.com", "Default Sender"),
			},
			// Add all possible settings to ensure they don't override kind validation
			SMTP: &SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "password",
			},
			SES: &AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
			SparkPost: &SparkPostSettings{
				APIKey:   "test-api-key",
				Endpoint: "https://api.sparkpost.com",
			},
		}

		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email provider kind")
	})

	t.Run("Multiple provider errors", func(t *testing.T) {
		// Test that validation fails even when multiple providers have valid settings
		// if the Kind doesn't match
		provider := EmailProvider{
			Kind:               EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders: []EmailSender{
				NewEmailSender("default@example.com", "Default Sender"),
			},
			// Missing SMTP settings but have SES settings
			SES: &AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
		}

		err := provider.Validate(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SMTP settings required")
	})

	t.Run("Empty provider", func(t *testing.T) {
		provider := EmailProvider{}
		err := provider.Validate(passphrase)
		assert.NoError(t, err)
	})
}

// Test encryption/decryption with invalid passphrase formats
func TestEncryptDecrypt_PassphraseEdgeCases(t *testing.T) {
	t.Run("Empty vs non-empty passphrase", func(t *testing.T) {
		// Encrypt with empty passphrase
		emptyPassphrase := ""
		nonEmptyPassphrase := "test-passphrase"

		smtp1 := SMTPSettings{
			Password: "test-password",
		}

		smtp2 := SMTPSettings{
			Password: "test-password",
		}

		// Encrypt both with different passphrases
		err1 := smtp1.EncryptPassword(emptyPassphrase)
		err2 := smtp2.EncryptPassword(nonEmptyPassphrase)

		// Both should succeed
		assert.NoError(t, err1)
		assert.NoError(t, err2)

		// But they should produce different encrypted values
		assert.NotEqual(t, smtp1.EncryptedPassword, smtp2.EncryptedPassword)

		// Decrypt with wrong passphrase should fail
		smtp1.Password = ""
		err := smtp1.DecryptPassword(nonEmptyPassphrase)
		assert.Error(t, err)
	})

	t.Run("Very long passphrase", func(t *testing.T) {
		// Using a valid long passphrase should still work
		longPassphrase := string(make([]byte, 1000))
		for i := range longPassphrase {
			longPassphrase = longPassphrase[:i] + "a" + longPassphrase[i+1:]
		}

		smtp := SMTPSettings{
			Password: "test-password",
		}

		// Should still work with a long passphrase
		err := smtp.EncryptPassword(longPassphrase)
		assert.NoError(t, err)

		// Should be able to decrypt with the same long passphrase
		originalPassword := smtp.Password
		smtp.Password = ""
		err = smtp.DecryptPassword(longPassphrase)
		assert.NoError(t, err)
		assert.Equal(t, originalPassword, smtp.Password)
	})

	t.Run("Wrong passphrase for decryption", func(t *testing.T) {
		// First encrypt with the correct passphrase
		correctPassphrase := "correct-passphrase"
		wrongPassphrase := "wrong-passphrase"

		sparkpost := SparkPostSettings{
			APIKey: "test-api-key",
		}

		err := sparkpost.EncryptAPIKey(correctPassphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, sparkpost.EncryptedAPIKey)

		// Now try to decrypt with the wrong passphrase
		sparkpost.APIKey = ""
		err = sparkpost.DecryptAPIKey(wrongPassphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt SparkPost API key")
	})
}

func TestMailjetSettings_EmptyEncryptedValues(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("Empty encrypted API key", func(t *testing.T) {
		settings := MailjetSettings{
			EncryptedAPIKey: "",
		}

		// Decrypting an empty encrypted value should fail
		err := settings.DecryptAPIKey(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DecryptFromHexString empty string")
	})

	t.Run("Empty encrypted Secret key", func(t *testing.T) {
		settings := MailjetSettings{
			EncryptedSecretKey: "",
		}

		// Decrypting an empty encrypted value should fail
		err := settings.DecryptSecretKey(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DecryptFromHexString empty string")
	})
}

func TestMailjetSettings_MultiKeyEncryptionWithSamePassphrase(t *testing.T) {
	passphrase := "test-passphrase"
	settings := MailjetSettings{
		APIKey:      "test-api-key",
		SecretKey:   "test-secret-key",
		SandboxMode: true,
	}

	// Encrypt both keys
	err := settings.EncryptAPIKey(passphrase)
	assert.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedAPIKey)

	err = settings.EncryptSecretKey(passphrase)
	assert.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedSecretKey)

	// Save encrypted values
	apiKeyEncrypted := settings.EncryptedAPIKey
	secretKeyEncrypted := settings.EncryptedSecretKey

	// Clear keys
	settings.APIKey = ""
	settings.SecretKey = ""

	// Decrypt with same passphrase
	err = settings.DecryptAPIKey(passphrase)
	assert.NoError(t, err)
	assert.Equal(t, "test-api-key", settings.APIKey)

	err = settings.DecryptSecretKey(passphrase)
	assert.NoError(t, err)
	assert.Equal(t, "test-secret-key", settings.SecretKey)

	// Now try with a wrong passphrase
	settings.EncryptedAPIKey = apiKeyEncrypted
	settings.EncryptedSecretKey = secretKeyEncrypted
	settings.APIKey = ""
	settings.SecretKey = ""

	err = settings.DecryptAPIKey("wrong-passphrase")
	assert.Error(t, err)

	err = settings.DecryptSecretKey("wrong-passphrase")
	assert.Error(t, err)
}

func TestEmailProvider_GetSender_Behavior(t *testing.T) {
	defaultSender := EmailSender{ID: "id-default", Email: "default@example.com", Name: "Default", IsDefault: true}
	otherSender := EmailSender{ID: "id-1", Email: "one@example.com", Name: "One", IsDefault: false}

	ep := &EmailProvider{Senders: []EmailSender{defaultSender, otherSender}}

	// by id
	s := ep.GetSender("id-1")
	assert.NotNil(t, s)
	assert.Equal(t, "id-1", s.ID)

	// default when id empty
	s = ep.GetSender("")
	assert.NotNil(t, s)
	assert.Equal(t, "id-default", s.ID)

	// default when id not found
	s = ep.GetSender("missing")
	assert.NotNil(t, s)
	assert.Equal(t, "id-default", s.ID)

	// nil when no senders
	empty := &EmailProvider{}
	assert.Nil(t, empty.GetSender(""))
}

func TestSendEmailRequest_Validate_Cases(t *testing.T) {
	validContact := &Contact{Email: "user@example.com"}
	validProvider := &EmailProvider{Kind: EmailProviderKindSMTP, SMTP: &SMTPSettings{Host: "smtp.example.com", Port: 25, Username: "u"}}

	tests := []struct {
		name    string
		req     SendEmailRequest
		wantErr bool
	}{
		{"missing workspace", SendEmailRequest{}, true},
		{"missing message", SendEmailRequest{WorkspaceID: "w"}, true},
		{"missing integration", SendEmailRequest{WorkspaceID: "w", MessageID: "m"}, true},
		{"missing contact", SendEmailRequest{WorkspaceID: "w", IntegrationID: "integration-123", MessageID: "m"}, true},
		{"missing provider", SendEmailRequest{WorkspaceID: "w", IntegrationID: "integration-123", MessageID: "m", Contact: validContact}, true},
		{"missing template id", SendEmailRequest{WorkspaceID: "w", IntegrationID: "integration-123", MessageID: "m", Contact: validContact, EmailProvider: validProvider}, true},
		{"valid", SendEmailRequest{WorkspaceID: "w", IntegrationID: "integration-123", MessageID: "m", Contact: validContact, EmailProvider: validProvider, TemplateConfig: ChannelTemplate{TemplateID: "tpl"}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSendEmailProviderRequest_Validate(t *testing.T) {
	validProvider := &EmailProvider{
		Kind: EmailProviderKindSES,
		SES: &AmazonSESSettings{
			Region:    "us-east-1",
			AccessKey: "test-key",
			SecretKey: "test-secret",
		},
	}

	tests := []struct {
		name    string
		req     SendEmailProviderRequest
		wantErr bool
	}{
		{"missing workspace", SendEmailProviderRequest{}, true},
		{"missing integration", SendEmailProviderRequest{WorkspaceID: "w"}, true},
		{"missing message", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i"}, true},
		{"missing from address", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i", MessageID: "m"}, true},
		{"missing from name", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i", MessageID: "m", FromAddress: "from@example.com"}, true},
		{"missing to", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i", MessageID: "m", FromAddress: "from@example.com", FromName: "From"}, true},
		{"missing subject", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i", MessageID: "m", FromAddress: "from@example.com", FromName: "From", To: "to@example.com"}, true},
		{"missing content", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i", MessageID: "m", FromAddress: "from@example.com", FromName: "From", To: "to@example.com", Subject: "Subject"}, true},
		{"missing provider", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i", MessageID: "m", FromAddress: "from@example.com", FromName: "From", To: "to@example.com", Subject: "Subject", Content: "Content"}, true},
		{"valid", SendEmailProviderRequest{WorkspaceID: "w", IntegrationID: "i", MessageID: "m", FromAddress: "from@example.com", FromName: "From", To: "to@example.com", Subject: "Subject", Content: "Content", Provider: validProvider}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEmailOptions_FromNameField(t *testing.T) {
	t.Run("EmailOptions with no from_name", func(t *testing.T) {
		options := EmailOptions{
			CC:      []string{"cc@example.com"},
			BCC:     []string{"bcc@example.com"},
			ReplyTo: "reply@example.com",
		}

		assert.Nil(t, options.FromName)
		assert.Equal(t, 1, len(options.CC))
		assert.Equal(t, 1, len(options.BCC))
	})

	t.Run("EmailOptions with from_name set", func(t *testing.T) {
		fromName := "Custom Sender Name"
		options := EmailOptions{
			FromName: &fromName,
			CC:       []string{"cc@example.com"},
		}

		assert.NotNil(t, options.FromName)
		assert.Equal(t, "Custom Sender Name", *options.FromName)
	})

	t.Run("EmailOptions with empty from_name", func(t *testing.T) {
		fromName := ""
		options := EmailOptions{
			FromName: &fromName,
		}

		assert.NotNil(t, options.FromName)
		assert.Equal(t, "", *options.FromName)
	})
}

func TestEmailOptions_JSONMarshaling(t *testing.T) {
	t.Run("JSON marshal without from_name", func(t *testing.T) {
		options := EmailOptions{
			CC:      []string{"cc@example.com"},
			ReplyTo: "reply@example.com",
		}

		jsonBytes, err := json.Marshal(options)
		require.NoError(t, err)

		// Unmarshal to verify
		var unmarshaled EmailOptions
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		require.NoError(t, err)

		assert.Nil(t, unmarshaled.FromName)
		assert.Equal(t, options.CC, unmarshaled.CC)
		assert.Equal(t, options.ReplyTo, unmarshaled.ReplyTo)
	})

	t.Run("JSON marshal with from_name", func(t *testing.T) {
		fromName := "Test Sender"
		options := EmailOptions{
			FromName: &fromName,
			CC:       []string{"cc@example.com"},
			ReplyTo:  "reply@example.com",
		}

		jsonBytes, err := json.Marshal(options)
		require.NoError(t, err)

		// Verify from_name is in JSON
		jsonString := string(jsonBytes)
		assert.Contains(t, jsonString, "from_name")
		assert.Contains(t, jsonString, "Test Sender")

		// Unmarshal to verify
		var unmarshaled EmailOptions
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		require.NoError(t, err)

		assert.NotNil(t, unmarshaled.FromName)
		assert.Equal(t, "Test Sender", *unmarshaled.FromName)
		assert.Equal(t, options.CC, unmarshaled.CC)
		assert.Equal(t, options.ReplyTo, unmarshaled.ReplyTo)
	})

	t.Run("JSON unmarshal with null from_name", func(t *testing.T) {
		jsonString := `{"from_name": null, "cc": ["cc@example.com"]}`

		var options EmailOptions
		err := json.Unmarshal([]byte(jsonString), &options)
		require.NoError(t, err)

		assert.Nil(t, options.FromName)
		assert.Equal(t, 1, len(options.CC))
	})

	t.Run("JSON unmarshal without from_name field", func(t *testing.T) {
		jsonString := `{"cc": ["cc@example.com"], "reply_to": "reply@example.com"}`

		var options EmailOptions
		err := json.Unmarshal([]byte(jsonString), &options)
		require.NoError(t, err)

		assert.Nil(t, options.FromName)
		assert.Equal(t, 1, len(options.CC))
		assert.Equal(t, "reply@example.com", options.ReplyTo)
	})

	t.Run("JSON unmarshal with empty string from_name", func(t *testing.T) {
		jsonString := `{"from_name": "", "cc": ["cc@example.com"]}`

		var options EmailOptions
		err := json.Unmarshal([]byte(jsonString), &options)
		require.NoError(t, err)

		assert.NotNil(t, options.FromName)
		assert.Equal(t, "", *options.FromName)
	})
}

func TestEmailOptions_IsEmpty(t *testing.T) {
	t.Run("Empty EmailOptions", func(t *testing.T) {
		options := EmailOptions{}
		assert.True(t, options.IsEmpty())
	})

	t.Run("EmailOptions with FromName", func(t *testing.T) {
		fromName := "Test Sender"
		options := EmailOptions{
			FromName: &fromName,
		}
		assert.False(t, options.IsEmpty())
	})

	t.Run("EmailOptions with CC", func(t *testing.T) {
		options := EmailOptions{
			CC: []string{"cc@example.com"},
		}
		assert.False(t, options.IsEmpty())
	})

	t.Run("EmailOptions with BCC", func(t *testing.T) {
		options := EmailOptions{
			BCC: []string{"bcc@example.com"},
		}
		assert.False(t, options.IsEmpty())
	})

	t.Run("EmailOptions with ReplyTo", func(t *testing.T) {
		options := EmailOptions{
			ReplyTo: "reply@example.com",
		}
		assert.False(t, options.IsEmpty())
	})

	t.Run("EmailOptions with all fields", func(t *testing.T) {
		fromName := "Test Sender"
		options := EmailOptions{
			FromName: &fromName,
			CC:       []string{"cc@example.com"},
			BCC:      []string{"bcc@example.com"},
			ReplyTo:  "reply@example.com",
		}
		assert.False(t, options.IsEmpty())
	})
}

func TestEmailOptions_ToChannelOptions(t *testing.T) {
	t.Run("Empty EmailOptions returns nil", func(t *testing.T) {
		options := EmailOptions{}
		channelOptions := options.ToChannelOptions()
		assert.Nil(t, channelOptions)
	})

	t.Run("EmailOptions with FromName only", func(t *testing.T) {
		fromName := "Test Sender"
		options := EmailOptions{
			FromName: &fromName,
		}
		channelOptions := options.ToChannelOptions()
		require.NotNil(t, channelOptions)
		assert.Equal(t, fromName, *channelOptions.FromName)
		assert.Empty(t, channelOptions.CC)
		assert.Empty(t, channelOptions.BCC)
		assert.Empty(t, channelOptions.ReplyTo)
	})

	t.Run("EmailOptions with all fields", func(t *testing.T) {
		fromName := "Test Sender"
		options := EmailOptions{
			FromName: &fromName,
			CC:       []string{"cc1@example.com", "cc2@example.com"},
			BCC:      []string{"bcc@example.com"},
			ReplyTo:  "reply@example.com",
		}
		channelOptions := options.ToChannelOptions()
		require.NotNil(t, channelOptions)
		assert.Equal(t, fromName, *channelOptions.FromName)
		assert.Equal(t, 2, len(channelOptions.CC))
		assert.Contains(t, channelOptions.CC, "cc1@example.com")
		assert.Contains(t, channelOptions.CC, "cc2@example.com")
		assert.Equal(t, 1, len(channelOptions.BCC))
		assert.Contains(t, channelOptions.BCC, "bcc@example.com")
		assert.Equal(t, "reply@example.com", channelOptions.ReplyTo)
	})

	t.Run("EmailOptions with only CC and BCC", func(t *testing.T) {
		options := EmailOptions{
			CC:  []string{"cc@example.com"},
			BCC: []string{"bcc@example.com"},
		}
		channelOptions := options.ToChannelOptions()
		require.NotNil(t, channelOptions)
		assert.Nil(t, channelOptions.FromName)
		assert.Equal(t, 1, len(channelOptions.CC))
		assert.Equal(t, 1, len(channelOptions.BCC))
		assert.Empty(t, channelOptions.ReplyTo)
	})
}
