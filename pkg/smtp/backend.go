package smtp

import (
	"errors"
	"io"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/emersion/go-smtp"
)

// MessageHandler is a function that processes incoming SMTP messages
type MessageHandler func(workspaceID string, from string, to []string, data []byte) error

// Backend implements smtp.Backend interface for handling SMTP connections
type Backend struct {
	authenticator AuthHandler
	handler       MessageHandler
	logger        logger.Logger
}

// NewBackend creates a new SMTP backend
func NewBackend(authenticator AuthHandler, handler MessageHandler, logger logger.Logger) *Backend {
	return &Backend{
		authenticator: authenticator,
		handler:       handler,
		logger:        logger,
	}
}

// NewSession creates a new SMTP session
// This is called when a client connects to the SMTP server
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		backend: b,
		logger:  b.logger,
	}, nil
}

// Session represents an SMTP session for a single connection
type Session struct {
	backend     *Backend
	logger      logger.Logger
	workspaceID string // Set after successful authentication
	from        string
	to          []string
}

// AuthPlain implements PLAIN authentication mechanism
func (s *Session) AuthPlain(username, password string) error {
	s.logger.WithFields(map[string]interface{}{
		"username": username,
	}).Debug("SMTP relay: AUTH PLAIN attempt")

	// Authenticate using workspace_id as username and api_key as password
	workspaceID, err := s.backend.authenticator(username, password)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Warn("SMTP relay: Authentication failed")
		return errors.New("invalid credentials")
	}

	s.workspaceID = workspaceID
	s.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
	}).Info("SMTP relay: Authentication successful")

	return nil
}

// Mail is called when the client sends a MAIL FROM command
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if s.workspaceID == "" {
		return errors.New("not authenticated")
	}

	s.logger.WithFields(map[string]interface{}{
		"from":         from,
		"workspace_id": s.workspaceID,
	}).Debug("SMTP relay: MAIL FROM")

	s.from = from
	return nil
}

// Rcpt is called when the client sends a RCPT TO command
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if s.workspaceID == "" {
		return errors.New("not authenticated")
	}

	s.logger.WithFields(map[string]interface{}{
		"to":           to,
		"workspace_id": s.workspaceID,
	}).Debug("SMTP relay: RCPT TO")

	s.to = append(s.to, to)
	return nil
}

// Data is called when the client sends the message data
func (s *Session) Data(r io.Reader) error {
	if s.workspaceID == "" {
		return errors.New("not authenticated")
	}

	s.logger.WithFields(map[string]interface{}{
		"from":         s.from,
		"to":           s.to,
		"workspace_id": s.workspaceID,
	}).Debug("SMTP relay: DATA")

	// Read the message data
	data, err := io.ReadAll(r)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error": err.Error(),
		}).Error("SMTP relay: Failed to read message data")
		return errors.New("failed to read message")
	}

	// Process the message using the handler
	err = s.backend.handler(s.workspaceID, s.from, s.to, data)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":        err.Error(),
			"workspace_id": s.workspaceID,
		}).Error("SMTP relay: Failed to process message")
		return err
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": s.workspaceID,
		"message_size": len(data),
	}).Info("SMTP relay: Message processed successfully")

	return nil
}

// Reset is called when the client sends a RSET command
func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

// Logout is called when the client disconnects
func (s *Session) Logout() error {
	return nil
}
