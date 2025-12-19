package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMTPSettings_EncryptDecryptUsername(t *testing.T) {
	passphrase := "test-passphrase"
	username := "user@example.com"

	settings := domain.SMTPSettings{
		Host:     "smtp.example.com",
		Port:     587,
		Username: username,
		Password: "test-password",
		UseTLS:   true,
	}

	// Test encryption
	err := settings.EncryptUsername(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedUsername)
	assert.Equal(t, username, settings.Username) // Original username should be unchanged

	// Save encrypted username
	encryptedUsername := settings.EncryptedUsername

	// Test decryption
	settings.Username = "" // Clear username
	err = settings.DecryptUsername(passphrase)
	require.NoError(t, err)
	assert.Equal(t, username, settings.Username)

	// Test decryption with wrong passphrase
	settings.Username = "" // Clear username
	settings.EncryptedUsername = encryptedUsername
	err = settings.DecryptUsername("wrong-passphrase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt SMTP username")
}

func TestSMTPSettings_EncryptDecryptPassword(t *testing.T) {
	passphrase := "test-passphrase"
	password := "test-password"

	settings := domain.SMTPSettings{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user@example.com",
		Password: password,
		UseTLS:   true,
	}

	// Test encryption
	err := settings.EncryptPassword(passphrase)
	require.NoError(t, err)
	assert.NotEmpty(t, settings.EncryptedPassword)
	assert.Equal(t, password, settings.Password) // Original password should be unchanged

	// Save encrypted password
	encryptedPassword := settings.EncryptedPassword

	// Test decryption
	settings.Password = "" // Clear password
	err = settings.DecryptPassword(passphrase)
	require.NoError(t, err)
	assert.Equal(t, password, settings.Password)

	// Test decryption with wrong passphrase
	settings.Password = "" // Clear password
	settings.EncryptedPassword = encryptedPassword
	err = settings.DecryptPassword("wrong-passphrase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt SMTP password")
}

// We need to test edge cases for passphrase encryption
func TestSMTPSettings_PassphraseEdgeCases(t *testing.T) {
	// Following pattern from TestEncryptDecrypt_PassphraseEdgeCases
	t.Run("Empty vs non-empty passphrase", func(t *testing.T) {
		// Encrypt with empty passphrase
		emptyPassphrase := ""
		nonEmptyPassphrase := "test-passphrase"

		smtp1 := domain.SMTPSettings{
			Password: "test-password",
		}

		smtp2 := domain.SMTPSettings{
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

		smtp := domain.SMTPSettings{
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
}

func TestSMTPSettings_Validate(t *testing.T) {
	passphrase := "test-passphrase"

	tests := []struct {
		name     string
		settings domain.SMTPSettings
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid settings",
			settings: domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
			wantErr: false,
		},
		{
			name: "missing host",
			settings: domain.SMTPSettings{
				Port:     587,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name: "invalid port (zero)",
			settings: domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     0,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
			wantErr: true,
			errMsg:  "invalid port number",
		},
		{
			name: "invalid port (negative)",
			settings: domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     -1,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
			wantErr: true,
			errMsg:  "invalid port number",
		},
		{
			name: "missing username (should be valid - username is optional)",
			settings: domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "",
				Password: "password",
				UseTLS:   true,
			},
			wantErr: false,
		},
		{
			name: "invalid port (too large)",
			settings: domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     70000,
				Username: "user@example.com",
				Password: "password",
				UseTLS:   true,
			},
			wantErr: true,
			errMsg:  "invalid port number",
		},
		{
			name: "missing username (should be valid - username is optional)",
			settings: domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Password: "password",
				UseTLS:   true,
			},
			wantErr: false,
		},
		{
			name: "empty password",
			settings: domain.SMTPSettings{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "",
				UseTLS:   true,
			},
			wantErr: false, // Empty password is allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate(passphrase)
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

func TestSMTPWebhookPayload(t *testing.T) {
	// Test struct mapping with JSON
	payload := domain.SMTPWebhookPayload{
		Event:          "bounce",
		Timestamp:      "2023-01-01T12:00:00Z",
		MessageID:      "test-message-id",
		Recipient:      "recipient@example.com",
		Metadata:       map[string]string{"key1": "value1", "key2": "value2"},
		Tags:           []string{"tag1", "tag2"},
		Reason:         "mailbox full",
		Description:    "The recipient's mailbox is full",
		BounceCategory: "soft_bounce",
		DiagnosticCode: "452 4.2.2 The email account is over quota",
		ComplaintType:  "",
	}

	// Convert to JSON and back
	jsonData, err := json.Marshal(payload)
	require.NoError(t, err)

	var decodedPayload domain.SMTPWebhookPayload
	err = json.Unmarshal(jsonData, &decodedPayload)
	require.NoError(t, err)

	// Verify all fields are correctly mapped
	assert.Equal(t, payload.Event, decodedPayload.Event)
	assert.Equal(t, payload.Timestamp, decodedPayload.Timestamp)
	assert.Equal(t, payload.MessageID, decodedPayload.MessageID)
	assert.Equal(t, payload.Recipient, decodedPayload.Recipient)
	assert.Equal(t, payload.Metadata, decodedPayload.Metadata)
	assert.Equal(t, payload.Tags, decodedPayload.Tags)
	assert.Equal(t, payload.Reason, decodedPayload.Reason)
	assert.Equal(t, payload.Description, decodedPayload.Description)
	assert.Equal(t, payload.BounceCategory, decodedPayload.BounceCategory)
	assert.Equal(t, payload.DiagnosticCode, decodedPayload.DiagnosticCode)
	assert.Equal(t, payload.ComplaintType, decodedPayload.ComplaintType)
}
