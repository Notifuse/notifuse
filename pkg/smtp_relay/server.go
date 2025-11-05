package smtp_relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/emersion/go-smtp"
)

// Server represents an SMTP relay server for receiving emails
type Server struct {
	server  *smtp.Server
	backend *Backend
	logger  logger.Logger
	addr    string
}

// ServerConfig holds the configuration for the SMTP server
type ServerConfig struct {
	Host       string
	Port       int
	Domain     string
	TLSConfig  *tls.Config // Pre-configured TLS config (if provided)
	RequireTLS bool        // Enforce TLS in production
	Logger     logger.Logger
}

// NewServer creates a new SMTP server with the given configuration
func NewServer(cfg ServerConfig, backend *Backend) (*Server, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Create the SMTP server
	s := smtp.NewServer(backend)
	s.Addr = addr
	s.Domain = cfg.Domain
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 10 * 1024 * 1024 // 10 MB max
	s.MaxRecipients = 50

	// Configure TLS
	if cfg.TLSConfig != nil {
		s.TLSConfig = cfg.TLSConfig
		s.AllowInsecureAuth = false // Require TLS for authentication
		cfg.Logger.Info("SMTP relay: TLS enabled")
	} else {
		// Check if running in production
		if cfg.RequireTLS {
			return nil, fmt.Errorf("SMTP relay: TLS is required in production environment")
		}
		s.AllowInsecureAuth = true // Allow auth without TLS (development only)
		cfg.Logger.Warn("⚠️  SMTP relay: Running without TLS - authentication will be insecure (FOR DEVELOPMENT ONLY)")
	}

	cfg.Logger.WithFields(map[string]interface{}{
		"addr":   addr,
		"domain": cfg.Domain,
		"tls":    cfg.TLSConfig != nil,
	}).Info("SMTP relay server initialized")

	return &Server{
		server:  s,
		backend: backend,
		logger:  cfg.Logger,
		addr:    addr,
	}, nil
}

// Start starts the SMTP server
func (s *Server) Start() error {
	s.logger.WithField("addr", s.addr).Info("Starting SMTP relay server")

	// Listen on the specified address
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	s.logger.WithField("addr", s.addr).Info("SMTP relay server listening")

	// Start serving
	if err := s.server.Serve(listener); err != nil {
		return fmt.Errorf("SMTP server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the SMTP server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down SMTP relay server")

	// Create a channel to signal completion
	done := make(chan error, 1)

	go func() {
		done <- s.server.Close()
	}()

	// Wait for shutdown to complete or context to be canceled
	select {
	case err := <-done:
		if err != nil {
			s.logger.WithField("error", err.Error()).Error("Error during SMTP server shutdown")
			return err
		}
		s.logger.Info("SMTP relay server shut down successfully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("SMTP server shutdown timeout exceeded")
		return ctx.Err()
	}
}
