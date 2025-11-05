package smtp_relay

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"

	"github.com/Notifuse/notifuse/pkg/logger"
)

// TLSConfig holds configuration for TLS certificate management
type TLSConfig struct {
	CertBase64 string // Base64 encoded certificate
	KeyBase64  string // Base64 encoded key
	Logger     logger.Logger
}

// SetupTLS configures TLS from base64-encoded certificates
// Returns a *tls.Config that can be used with the SMTP server
func SetupTLS(cfg TLSConfig) (*tls.Config, error) {
	// Check if certificates are provided
	if cfg.CertBase64 == "" || cfg.KeyBase64 == "" {
		cfg.Logger.Warn("SMTP relay: No TLS configuration provided - server will run without TLS (NOT recommended for production)")
		return nil, nil
	}

	cfg.Logger.Info("SMTP relay: Configuring TLS from base64-encoded certificates")

	// Decode certificate
	certPEM, err := base64.StdEncoding.DecodeString(cfg.CertBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 certificate: %w", err)
	}

	// Decode key
	keyPEM, err := base64.StdEncoding.DecodeString(cfg.KeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 key: %w", err)
	}

	// Load certificate and key
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate from base64: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	cfg.Logger.Info("SMTP relay: TLS configured successfully from base64 certificates")

	return tlsConfig, nil
}
