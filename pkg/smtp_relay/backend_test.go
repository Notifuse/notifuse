package smtp_relay

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/emersion/go-smtp"
)

func TestBackend_NewSession(t *testing.T) {
	log := logger.NewLogger()

	authHandler := func(username, password string) (string, error) {
		return "user123", nil
	}

	messageHandler := func(userID string, from string, to []string, data []byte) error {
		return nil
	}

	backend := NewBackend(authHandler, messageHandler, log)

	session, err := backend.NewSession(nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	if session == nil {
		t.Fatal("Expected session to be created, got nil")
	}
}

func TestSession_AuthPlain(t *testing.T) {
	log := logger.NewLogger()

	tests := []struct {
		name     string
		username string
		password string
		authFunc AuthHandler
		wantErr  bool
	}{
		{
			name:     "successful authentication",
			username: "api@example.com",
			password: "api_key_token",
			authFunc: func(username, password string) (string, error) {
				if username == "api@example.com" && password == "api_key_token" {
					return "user123", nil
				}
				return "", smtp.ErrAuthFailed
			},
			wantErr: false,
		},
		{
			name:     "failed authentication",
			username: "api@example.com",
			password: "wrong_token",
			authFunc: func(username, password string) (string, error) {
				return "", smtp.ErrAuthFailed
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messageHandler := func(userID string, from string, to []string, data []byte) error {
				return nil
			}

			backend := NewBackend(tt.authFunc, messageHandler, log)
			session, _ := backend.NewSession(nil)

			err := session.(*Session).AuthPlain(tt.username, tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("AuthPlain() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSession_Mail(t *testing.T) {
	log := logger.NewLogger()

	authHandler := func(username, password string) (string, error) {
		return "user123", nil
	}

	messageHandler := func(userID string, from string, to []string, data []byte) error {
		return nil
	}

	backend := NewBackend(authHandler, messageHandler, log)
	session, _ := backend.NewSession(nil)
	s := session.(*Session)

	// Should fail without authentication
	err := s.Mail("sender@example.com", nil)
	if err == nil {
		t.Error("Expected error when not authenticated, got nil")
	}

	// Authenticate
	s.AuthPlain("api@example.com", "token")

	// Should succeed after authentication
	err = s.Mail("sender@example.com", nil)
	if err != nil {
		t.Errorf("Mail() failed after authentication: %v", err)
	}

	if s.from != "sender@example.com" {
		t.Errorf("Expected from to be 'sender@example.com', got '%s'", s.from)
	}
}

func TestSession_Rcpt(t *testing.T) {
	log := logger.NewLogger()

	authHandler := func(username, password string) (string, error) {
		return "user123", nil
	}

	messageHandler := func(userID string, from string, to []string, data []byte) error {
		return nil
	}

	backend := NewBackend(authHandler, messageHandler, log)
	session, _ := backend.NewSession(nil)
	s := session.(*Session)

	// Should fail without authentication
	err := s.Rcpt("recipient@example.com", nil)
	if err == nil {
		t.Error("Expected error when not authenticated, got nil")
	}

	// Authenticate
	s.AuthPlain("api@example.com", "token")

	// Should succeed after authentication
	err = s.Rcpt("recipient1@example.com", nil)
	if err != nil {
		t.Errorf("Rcpt() failed after authentication: %v", err)
	}

	err = s.Rcpt("recipient2@example.com", nil)
	if err != nil {
		t.Errorf("Rcpt() failed for second recipient: %v", err)
	}

	if len(s.to) != 2 {
		t.Errorf("Expected 2 recipients, got %d", len(s.to))
	}
}

func TestSession_Data(t *testing.T) {
	log := logger.NewLogger()

	authHandler := func(username, password string) (string, error) {
		return "user123", nil
	}

	var capturedUserID string
	var capturedFrom string
	var capturedTo []string
	var capturedData []byte

	messageHandler := func(userID string, from string, to []string, data []byte) error {
		capturedUserID = userID
		capturedFrom = from
		capturedTo = to
		capturedData = data
		return nil
	}

	backend := NewBackend(authHandler, messageHandler, log)
	session, _ := backend.NewSession(nil)
	s := session.(*Session)

	// Authenticate and set up message
	s.AuthPlain("api@example.com", "token")
	s.Mail("sender@example.com", nil)
	s.Rcpt("recipient@example.com", nil)

	// Send data
	messageData := []byte(`{"workspace_id": "workspace123", "notification": {"id": "test"}}`)
	reader := io.NopCloser(bytes.NewReader(messageData))

	err := s.Data(reader)
	if err != nil {
		t.Errorf("Data() failed: %v", err)
	}

	// Verify handler was called with correct parameters
	if capturedUserID != "user123" {
		t.Errorf("Expected user_id 'user123', got '%s'", capturedUserID)
	}

	if capturedFrom != "sender@example.com" {
		t.Errorf("Expected from 'sender@example.com', got '%s'", capturedFrom)
	}

	if len(capturedTo) != 1 || capturedTo[0] != "recipient@example.com" {
		t.Errorf("Expected to contain ['recipient@example.com'], got %v", capturedTo)
	}

	if !bytes.Equal(capturedData, messageData) {
		t.Errorf("Expected data to match, got different data")
	}
}

func TestSession_Reset(t *testing.T) {
	log := logger.NewLogger()

	authHandler := func(username, password string) (string, error) {
		return "user123", nil
	}

	messageHandler := func(userID string, from string, to []string, data []byte) error {
		return nil
	}

	backend := NewBackend(authHandler, messageHandler, log)
	session, _ := backend.NewSession(nil)
	s := session.(*Session)

	// Set up some state
	s.AuthPlain("api@example.com", "token")
	s.Mail("sender@example.com", nil)
	s.Rcpt("recipient@example.com", nil)

	// Reset
	s.Reset()

	// Verify state was cleared
	if s.from != "" {
		t.Errorf("Expected from to be empty after Reset, got '%s'", s.from)
	}

	if len(s.to) != 0 {
		t.Errorf("Expected to to be empty after Reset, got %v", s.to)
	}

	// userID should still be set
	if s.userID != "user123" {
		t.Errorf("Expected userID to remain 'user123', got '%s'", s.userID)
	}
}

func TestSession_Logout(t *testing.T) {
	log := logger.NewLogger()

	authHandler := func(username, password string) (string, error) {
		return "user123", nil
	}

	messageHandler := func(userID string, from string, to []string, data []byte) error {
		return nil
	}

	backend := NewBackend(authHandler, messageHandler, log)
	session, _ := backend.NewSession(nil)

	err := session.Logout()
	if err != nil {
		t.Errorf("Logout() returned error: %v", err)
	}
}

func TestSession_DataWithoutAuth(t *testing.T) {
	log := logger.NewLogger()

	authHandler := func(username, password string) (string, error) {
		return "user123", nil
	}

	messageHandler := func(userID string, from string, to []string, data []byte) error {
		return nil
	}

	backend := NewBackend(authHandler, messageHandler, log)
	session, _ := backend.NewSession(nil)
	s := session.(*Session)

	// Try to send data without authentication
	messageData := []byte(`test`)
	reader := io.NopCloser(strings.NewReader(string(messageData)))

	err := s.Data(reader)
	if err == nil {
		t.Error("Expected error when calling Data() without authentication, got nil")
	}

	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("Expected 'not authenticated' error, got: %v", err)
	}
}
