package smtp_relay

import (
	"errors"
	"io"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// MessageHandler is a function that processes incoming SMTP messages
// The first parameter is the authenticated user ID (from the API key)
type MessageHandler func(userID string, from string, to []string, data []byte) error

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
	backend *Backend
	logger  logger.Logger
	userID  string // API key user ID set after successful authentication
	from    string
	to      []string
}

// AuthMechanisms returns a list of available auth mechanisms
// This is required by the AuthSession interface to advertise authentication
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

// Auth returns a SASL server for the specified mechanism
// This is required by the AuthSession interface
func (s *Session) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		s.logger.WithFields(map[string]interface{}{
			"username": username,
		}).Debug("SMTP relay: AUTH PLAIN attempt")

		// Authenticate using api_email as username and api_key as password
		userID, err := s.backend.authenticator(username, password)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"username": username,
				"error":    err.Error(),
			}).Warn("SMTP relay: Authentication failed")
			return errors.New("invalid credentials")
		}

		s.userID = userID
		s.logger.WithFields(map[string]interface{}{
			"user_id": userID,
		}).Info("SMTP relay: Authentication successful")

		return nil
	}), nil
}

// AuthPlain implements PLAIN authentication mechanism (legacy compatibility)
func (s *Session) AuthPlain(username, password string) error {
	s.logger.WithFields(map[string]interface{}{
		"username": username,
	}).Debug("SMTP relay: AUTH PLAIN attempt (legacy)")

	// Authenticate using api_email as username and api_key as password
	userID, err := s.backend.authenticator(username, password)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Warn("SMTP relay: Authentication failed")
		return errors.New("invalid credentials")
	}

	s.userID = userID
	s.logger.WithFields(map[string]interface{}{
		"user_id": userID,
	}).Info("SMTP relay: Authentication successful")

	return nil
}

// Mail is called when the client sends a MAIL FROM command
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if s.userID == "" {
		return errors.New("not authenticated")
	}

	s.logger.WithFields(map[string]interface{}{
		"from":    from,
		"user_id": s.userID,
	}).Debug("SMTP relay: MAIL FROM")

	s.from = from
	return nil
}

// Rcpt is called when the client sends a RCPT TO command
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if s.userID == "" {
		return errors.New("not authenticated")
	}

	s.logger.WithFields(map[string]interface{}{
		"to":      to,
		"user_id": s.userID,
	}).Debug("SMTP relay: RCPT TO")

	s.to = append(s.to, to)
	return nil
}

// Data is called when the client sends the message data
func (s *Session) Data(r io.Reader) error {
	if s.userID == "" {
		return errors.New("not authenticated")
	}

	s.logger.WithFields(map[string]interface{}{
		"from":    s.from,
		"to":      s.to,
		"user_id": s.userID,
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
	// The handler will extract workspace_id from the JSON payload
	err = s.backend.handler(s.userID, s.from, s.to, data)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":   err.Error(),
			"user_id": s.userID,
		}).Error("SMTP relay: Failed to process message")
		return err
	}

	s.logger.WithFields(map[string]interface{}{
		"user_id":      s.userID,
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
