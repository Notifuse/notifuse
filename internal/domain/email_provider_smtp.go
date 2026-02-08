package domain

import (
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// SMTPWebhookPayload represents an SMTP webhook payload
// SMTP doesn't typically have a built-in webhook system, so this is a generic structure
// that could be used with a third-party SMTP provider that offers webhooks
type SMTPWebhookPayload struct {
	Event          string            `json:"event"`
	Timestamp      string            `json:"timestamp"`
	MessageID      string            `json:"message_id"`
	Recipient      string            `json:"recipient"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Reason         string            `json:"reason,omitempty"`
	Description    string            `json:"description,omitempty"`
	BounceCategory string            `json:"bounce_category,omitempty"`
	DiagnosticCode string            `json:"diagnostic_code,omitempty"`
	ComplaintType  string            `json:"complaint_type,omitempty"`
}

// SMTPSettings contains configuration for SMTP email server
type SMTPSettings struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	EncryptedUsername string `json:"encrypted_username,omitempty"`
	EncryptedPassword string `json:"encrypted_password,omitempty"`
	UseTLS            bool   `json:"use_tls"`

	// decoded username, not stored in the database
	// decoded password , not stored in the database
	Username string `json:"username"`
	Password string `json:"password,omitempty"`

	// Authentication type: "basic" (default) or "oauth2"
	AuthType string `json:"auth_type,omitempty"`

	// OAuth2 fields
	OAuth2Provider              string `json:"oauth2_provider,omitempty"`                // "microsoft" or "google"
	OAuth2TenantID              string `json:"oauth2_tenant_id,omitempty"`               // Microsoft only
	OAuth2ClientID              string `json:"oauth2_client_id,omitempty"`               // Client ID from OAuth2 app
	EncryptedOAuth2ClientSecret string `json:"encrypted_oauth2_client_secret,omitempty"` // Encrypted client secret
	EncryptedOAuth2RefreshToken string `json:"encrypted_oauth2_refresh_token,omitempty"` // Encrypted refresh token (Google)

	// Runtime decrypted OAuth2 secrets (not stored in database)
	OAuth2ClientSecret string `json:"oauth2_client_secret,omitempty"` // Decrypted client secret
	OAuth2RefreshToken string `json:"oauth2_refresh_token,omitempty"` // Decrypted refresh token (Google)
}

func (s *SMTPSettings) DecryptUsername(passphrase string) error {
	username, err := crypto.DecryptFromHexString(s.EncryptedUsername, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt SMTP username: %w", err)
	}
	s.Username = username
	return nil
}

func (s *SMTPSettings) EncryptUsername(passphrase string) error {
	encryptedUsername, err := crypto.EncryptString(s.Username, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt SMTP username: %w", err)
	}
	s.EncryptedUsername = encryptedUsername
	return nil
}

func (s *SMTPSettings) DecryptPassword(passphrase string) error {
	password, err := crypto.DecryptFromHexString(s.EncryptedPassword, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt SMTP password: %w", err)
	}
	s.Password = password
	return nil
}

func (s *SMTPSettings) EncryptPassword(passphrase string) error {
	encryptedPassword, err := crypto.EncryptString(s.Password, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt SMTP password: %w", err)
	}
	s.EncryptedPassword = encryptedPassword
	return nil
}

// OAuth2 encryption/decryption methods

func (s *SMTPSettings) DecryptOAuth2ClientSecret(passphrase string) error {
	clientSecret, err := crypto.DecryptFromHexString(s.EncryptedOAuth2ClientSecret, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt OAuth2 client secret: %w", err)
	}
	s.OAuth2ClientSecret = clientSecret
	return nil
}

func (s *SMTPSettings) EncryptOAuth2ClientSecret(passphrase string) error {
	encryptedSecret, err := crypto.EncryptString(s.OAuth2ClientSecret, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt OAuth2 client secret: %w", err)
	}
	s.EncryptedOAuth2ClientSecret = encryptedSecret
	return nil
}

func (s *SMTPSettings) DecryptOAuth2RefreshToken(passphrase string) error {
	refreshToken, err := crypto.DecryptFromHexString(s.EncryptedOAuth2RefreshToken, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt OAuth2 refresh token: %w", err)
	}
	s.OAuth2RefreshToken = refreshToken
	return nil
}

func (s *SMTPSettings) EncryptOAuth2RefreshToken(passphrase string) error {
	encryptedToken, err := crypto.EncryptString(s.OAuth2RefreshToken, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt OAuth2 refresh token: %w", err)
	}
	s.EncryptedOAuth2RefreshToken = encryptedToken
	return nil
}

func (s *SMTPSettings) Validate(passphrase string) error {
	if s.Host == "" {
		return fmt.Errorf("host is required for SMTP configuration")
	}

	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("invalid port number for SMTP configuration: %d", s.Port)
	}

	// Handle OAuth2 authentication
	if s.AuthType == "oauth2" {
		return s.validateOAuth2(passphrase)
	}

	// Basic authentication (default)
	// Username is optional - only encrypt if provided
	if s.Username != "" {
		if err := s.EncryptUsername(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt SMTP username: %w", err)
		}
	}

	// Only encrypt password if it's not empty
	if s.Password != "" {
		if err := s.EncryptPassword(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt SMTP password: %w", err)
		}
	}

	return nil
}

// validateOAuth2 validates OAuth2 specific fields and encrypts secrets
func (s *SMTPSettings) validateOAuth2(passphrase string) error {
	// Provider is required
	if s.OAuth2Provider == "" {
		return fmt.Errorf("oauth2_provider is required for OAuth2 authentication")
	}

	// Provider must be valid
	if s.OAuth2Provider != "microsoft" && s.OAuth2Provider != "google" {
		return fmt.Errorf("oauth2_provider must be 'microsoft' or 'google'")
	}

	// Client ID is always required
	if s.OAuth2ClientID == "" {
		return fmt.Errorf("oauth2_client_id is required for OAuth2 authentication")
	}

	// Client secret is always required
	if s.OAuth2ClientSecret == "" {
		return fmt.Errorf("oauth2_client_secret is required for OAuth2 authentication")
	}

	// Microsoft-specific validation
	if s.OAuth2Provider == "microsoft" {
		if s.OAuth2TenantID == "" {
			return fmt.Errorf("oauth2_tenant_id is required for Microsoft OAuth2")
		}
	}

	// Google-specific validation
	if s.OAuth2Provider == "google" {
		if s.OAuth2RefreshToken == "" {
			return fmt.Errorf("oauth2_refresh_token is required for Google OAuth2")
		}
	}

	// Encrypt OAuth2 client secret
	if err := s.EncryptOAuth2ClientSecret(passphrase); err != nil {
		return fmt.Errorf("failed to encrypt OAuth2 client secret: %w", err)
	}

	// Encrypt OAuth2 refresh token if present (Google)
	if s.OAuth2RefreshToken != "" {
		if err := s.EncryptOAuth2RefreshToken(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt OAuth2 refresh token: %w", err)
		}
	}

	return nil
}
