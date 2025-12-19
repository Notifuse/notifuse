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

func (s *SMTPSettings) Validate(passphrase string) error {
	if s.Host == "" {
		return fmt.Errorf("host is required for SMTP configuration")
	}

	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("invalid port number for SMTP configuration: %d", s.Port)
	}

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
