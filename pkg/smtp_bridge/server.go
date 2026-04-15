package smtp_bridge

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/emersion/go-smtp"
)

// Server represents an SMTP bridge server for receiving emails
type Server struct {
	server            *smtp.Server
	backend           *Backend
	logger            logger.Logger
	addr              string
	tlsListenerConfig *tls.Config // set only in implicit mode, used to wrap the listener
}

// ServerConfig holds the configuration for the SMTP server
type ServerConfig struct {
	Host      string
	Port      int
	Domain    string
	Mode      string      // ModeOff, ModeSTARTTLS, or ModeImplicit
	TLSConfig *tls.Config // required for ModeSTARTTLS and ModeImplicit
	Logger    logger.Logger
}

// NewServer creates a new SMTP server with the given configuration
func NewServer(cfg ServerConfig, backend *Backend) (*Server, error) {
	if err := ValidateMode(cfg.Mode); err != nil {
		return nil, err
	}

	if (cfg.Mode == ModeSTARTTLS || cfg.Mode == ModeImplicit) && cfg.TLSConfig == nil {
		return nil, fmt.Errorf("SMTP bridge: TLSConfig is required for mode %q", cfg.Mode)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	s := smtp.NewServer(backend)
	s.Addr = addr
	s.Domain = cfg.Domain
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 10 * 1024 * 1024 // 10 MB max
	s.MaxRecipients = 50

	srv := &Server{
		server:  s,
		backend: backend,
		logger:  cfg.Logger,
		addr:    addr,
	}

	switch cfg.Mode {
	case ModeOff:
		s.TLSConfig = nil
		s.AllowInsecureAuth = true
		cfg.Logger.Warn("SMTP bridge: TLS mode is 'off' — running plaintext (authentication will be sent unencrypted; only safe on a trusted network behind a TLS-terminating proxy)")

	case ModeSTARTTLS:
		s.TLSConfig = cfg.TLSConfig
		s.AllowInsecureAuth = false
		cfg.Logger.Info("SMTP bridge: TLS mode is 'starttls'")

	case ModeImplicit:
		// TLS is terminated at the listener layer via tls.NewListener.
		// s.TLSConfig stays nil so STARTTLS is not advertised; go-smtp
		// detects the *tls.Conn automatically and allows AUTH.
		s.TLSConfig = nil
		s.AllowInsecureAuth = false
		srv.tlsListenerConfig = cfg.TLSConfig
		cfg.Logger.Info("SMTP bridge: TLS mode is 'implicit' (SMTPS)")
	}

	cfg.Logger.WithFields(map[string]interface{}{
		"addr":   addr,
		"domain": cfg.Domain,
		"mode":   cfg.Mode,
	}).Info("SMTP bridge server initialized")

	return srv, nil
}

// Start starts the SMTP server
func (s *Server) Start() error {
	s.logger.WithField("addr", s.addr).Info("Starting SMTP bridge server")

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	if s.tlsListenerConfig != nil {
		listener = tls.NewListener(listener, s.tlsListenerConfig)
	}

	s.logger.WithField("addr", s.addr).Info("SMTP bridge server listening")

	if err := s.server.Serve(listener); err != nil {
		return fmt.Errorf("SMTP server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the SMTP server, draining in-flight
// SMTP sessions until they complete or the context expires.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down SMTP bridge server")

	err := s.server.Shutdown(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.logger.Warn("SMTP server shutdown timeout exceeded")
		} else {
			s.logger.WithField("error", err.Error()).Error("Error during SMTP server shutdown")
		}
		return err
	}

	s.logger.Info("SMTP bridge server shut down successfully")
	return nil
}
