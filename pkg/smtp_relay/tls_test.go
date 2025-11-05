package smtp_relay

import (
	"encoding/base64"
	"testing"

	"github.com/Notifuse/notifuse/pkg/logger"
)

func TestSetupTLS_NoConfig(t *testing.T) {
	log := logger.NewLogger()

	cfg := TLSConfig{
		Logger: log,
	}

	tlsConfig, err := SetupTLS(cfg)

	if err != nil {
		t.Errorf("Expected no error for empty config, got: %v", err)
	}

	if tlsConfig != nil {
		t.Error("Expected nil TLS config when no configuration provided")
	}
}

func TestSetupTLS_OnlyCertProvided(t *testing.T) {
	log := logger.NewLogger()

	cfg := TLSConfig{
		CertBase64: base64.StdEncoding.EncodeToString([]byte("cert")),
		Logger:     log,
	}

	tlsConfig, err := SetupTLS(cfg)

	if err != nil {
		t.Errorf("Expected no error when only cert provided: %v", err)
	}

	if tlsConfig != nil {
		t.Error("Expected nil TLS config when only cert provided")
	}
}

func TestSetupTLS_OnlyKeyProvided(t *testing.T) {
	log := logger.NewLogger()

	cfg := TLSConfig{
		KeyBase64: base64.StdEncoding.EncodeToString([]byte("key")),
		Logger:    log,
	}

	tlsConfig, err := SetupTLS(cfg)

	if err != nil {
		t.Errorf("Expected no error when only key provided: %v", err)
	}

	if tlsConfig != nil {
		t.Error("Expected nil TLS config when only key provided")
	}
}

func TestSetupTLS_InvalidBase64Cert(t *testing.T) {
	log := logger.NewLogger()

	cfg := TLSConfig{
		CertBase64: "invalid-base64!!!",
		KeyBase64:  base64.StdEncoding.EncodeToString([]byte("key")),
		Logger:     log,
	}

	_, err := SetupTLS(cfg)

	if err == nil {
		t.Error("Expected error for invalid base64 cert")
	}
}

func TestSetupTLS_InvalidBase64Key(t *testing.T) {
	log := logger.NewLogger()

	cfg := TLSConfig{
		CertBase64: base64.StdEncoding.EncodeToString([]byte("cert")),
		KeyBase64:  "invalid-base64!!!",
		Logger:     log,
	}

	_, err := SetupTLS(cfg)

	if err == nil {
		t.Error("Expected error for invalid base64 key")
	}
}

func TestSetupTLS_ValidBase64ButInvalidCert(t *testing.T) {
	log := logger.NewLogger()

	cfg := TLSConfig{
		CertBase64: base64.StdEncoding.EncodeToString([]byte("not a cert")),
		KeyBase64:  base64.StdEncoding.EncodeToString([]byte("not a key")),
		Logger:     log,
	}

	_, err := SetupTLS(cfg)

	if err == nil {
		t.Error("Expected error for invalid certificate data")
	}

	if err != nil && err.Error() != "failed to load TLS certificate from base64: tls: failed to find any PEM data in certificate input" {
		// Error message may vary, just check we got an error
		t.Logf("Got expected error: %v", err)
	}
}
