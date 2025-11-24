package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/emersion/go-smtp"
)

// Server represents an SMTP relay server for receiving emails
type Server struct {
	server   *smtp.Server
	backend  *Backend
	logger   logger.Logger
	addr     string
	listener net.Listener
	mu       sync.Mutex
}

// ServerConfig holds the configuration for the SMTP server
type ServerConfig struct {
	Host        string
	Port        int
	Domain      string
	TLSCertFile string
	TLSKeyFile  string
	Logger      logger.Logger
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
	s.AllowInsecureAuth = false // Require TLS for authentication

	// Configure TLS
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
		}

		s.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		cfg.Logger.WithFields(map[string]interface{}{
			"cert_file": cfg.TLSCertFile,
			"key_file":  cfg.TLSKeyFile,
		}).Info("SMTP relay: TLS configured")
	} else {
		cfg.Logger.Warn("SMTP relay: No TLS certificates provided - TLS will not be available")
	}

	cfg.Logger.WithFields(map[string]interface{}{
		"addr":   addr,
		"domain": cfg.Domain,
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

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	s.logger.WithField("addr", s.addr).Info("SMTP relay server listening")

	// Start serving - Serve will return when listener is closed
	err = s.server.Serve(listener)

	s.mu.Lock()
	s.listener = nil
	s.mu.Unlock()

	// If the listener was closed (e.g., during shutdown), Serve returns nil
	// This is expected behavior, so we return nil in that case
	if err != nil {
		return fmt.Errorf("SMTP server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the SMTP server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down SMTP relay server")

	// Close the listener first, which will cause Serve() to return
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()

	if listener != nil {
		// Close the listener to cause Serve() to return
		// Ignore errors from closing an already-closed listener
		_ = listener.Close()
	}

	// Close the server (closes any remaining connections)
	// Note: server.Close() may return an error if the listener was already closed,
	// which is expected and can be ignored since we've already closed the listener
	done := make(chan error, 1)
	go func() {
		done <- s.server.Close()
	}()

	// Wait for shutdown to complete or context to be canceled
	select {
	case err := <-done:
		// If we closed the listener ourselves, ignore "use of closed network connection" errors
		// from server.Close() since the listener is already closed
		if err != nil && listener != nil {
			errStr := err.Error()
			// Check if it's a "closed network connection" error, which is expected
			if strings.Contains(errStr, "use of closed network connection") {
				// Expected error when listener was already closed, ignore it
				s.logger.Info("SMTP relay server shut down successfully")
				return nil
			}
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
