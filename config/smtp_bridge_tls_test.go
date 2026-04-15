package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Direct tests for the resolver. It's a pure function, so no DB/env plumbing needed. ---

func TestResolveSMTPBridgeTLSMode_ExplicitOff(t *testing.T) {
	got, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{TLSMode: "off"}, "production")
	require.NoError(t, err)
	assert.Equal(t, "off", got)
}

func TestResolveSMTPBridgeTLSMode_ExplicitOff_WithCerts(t *testing.T) {
	// Certs are ignored in off mode, no error.
	got, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{
		TLSMode:       "off",
		TLSCertBase64: "cert",
		TLSKeyBase64:  "key",
	}, "production")
	require.NoError(t, err)
	assert.Equal(t, "off", got)
}

func TestResolveSMTPBridgeTLSMode_STARTTLS_MissingCert_Errors(t *testing.T) {
	_, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{TLSMode: "starttls"}, "production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starttls")
	assert.Contains(t, err.Error(), "SMTP_BRIDGE_TLS_CERT_BASE64")
}

func TestResolveSMTPBridgeTLSMode_Implicit_MissingKey_Errors(t *testing.T) {
	_, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{
		TLSMode:       "implicit",
		TLSCertBase64: "cert",
		// key missing
	}, "production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "implicit")
}

func TestResolveSMTPBridgeTLSMode_Implicit_WithCerts(t *testing.T) {
	got, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{
		TLSMode:       "implicit",
		TLSCertBase64: "cert",
		TLSKeyBase64:  "key",
	}, "production")
	require.NoError(t, err)
	assert.Equal(t, "implicit", got)
}

func TestResolveSMTPBridgeTLSMode_UnknownValue_Errors(t *testing.T) {
	_, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{TLSMode: "bogus"}, "production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP_BRIDGE_TLS")
}

func TestResolveSMTPBridgeTLSMode_AutoResolve_WithCerts(t *testing.T) {
	got, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{
		TLSCertBase64: "cert",
		TLSKeyBase64:  "key",
	}, "production")
	require.NoError(t, err)
	assert.Equal(t, "starttls", got)
}

func TestResolveSMTPBridgeTLSMode_AutoResolve_NoCerts_Production_Errors(t *testing.T) {
	_, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{}, "production")
	require.Error(t, err)
	// The message should point operators at the escape hatch.
	assert.Contains(t, err.Error(), "SMTP_BRIDGE_TLS=off")
}

func TestResolveSMTPBridgeTLSMode_AutoResolve_NoCerts_Development(t *testing.T) {
	got, err := resolveSMTPBridgeTLSMode(SMTPBridgeConfig{}, "development")
	require.NoError(t, err)
	assert.Equal(t, "off", got)
}

// --- End-to-end through LoadWithOptions. Exercises the viper binding + legacy fallback. ---

// setSMTPBridgeTestEnv sets the base env vars required for LoadWithOptions to succeed,
// so individual tests only need to add the SMTP_BRIDGE_* vars they care about.
func setSMTPBridgeTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
	t.Setenv("ROOT_EMAIL", "test@example.com")
	t.Setenv("DB_HOST", "nonexistent.invalid") // connection will fail → first-run path
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "x")
	t.Setenv("DB_PASSWORD", "x")
	t.Setenv("DB_PREFIX", "x")
	t.Setenv("DB_NAME", "x")
	t.Setenv("ENVIRONMENT", "development") // safe default; override per test
}

func TestLoad_SMTPBridgeTLS_ExplicitOff(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	t.Setenv("SMTP_BRIDGE_TLS", "off")

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "off", cfg.SMTPBridge.TLSMode)
	assert.Equal(t, "off", cfg.EnvValues.SMTPBridgeTLSMode)
}

func TestLoad_SMTPBridgeTLS_STARTTLS_NoCert_Errors(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	t.Setenv("SMTP_BRIDGE_TLS", "starttls")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starttls")
}

func TestLoad_SMTPBridgeTLS_Implicit_NoCert_Errors(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	t.Setenv("SMTP_BRIDGE_TLS", "implicit")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "implicit")
}

func TestLoad_SMTPBridgeTLS_Unknown_Errors(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	t.Setenv("SMTP_BRIDGE_TLS", "not-a-mode")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "SMTP_BRIDGE_TLS") ||
			strings.Contains(err.Error(), "invalid"),
		"error should mention SMTP_BRIDGE_TLS: %v", err,
	)
}

func TestLoad_SMTPBridgeTLS_LegacyFallback(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	t.Setenv("SMTP_RELAY_TLS", "off") // legacy var only

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "off", cfg.SMTPBridge.TLSMode)
}

func TestLoad_SMTPBridgeTLS_AutoResolve_WithCerts(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	t.Setenv("SMTP_BRIDGE_TLS_CERT_BASE64", "cert")
	t.Setenv("SMTP_BRIDGE_TLS_KEY_BASE64", "key")
	// SMTP_BRIDGE_TLS unset

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "starttls", cfg.SMTPBridge.TLSMode)
}

func TestLoad_SMTPBridgeTLS_AutoResolve_NoCert_Production_Errors(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	// No cert, no SMTP_BRIDGE_TLS

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP_BRIDGE_TLS=off")
}

func TestLoad_SMTPBridgeTLS_AutoResolve_NoCert_Development(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("SMTP_BRIDGE_ENABLED", "true")
	// ENVIRONMENT stays development from setSMTPBridgeTestEnv

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "off", cfg.SMTPBridge.TLSMode)
}

func TestLoad_SMTPBridgeTLS_Disabled_SkipsValidation(t *testing.T) {
	setSMTPBridgeTestEnv(t)
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("SMTP_BRIDGE_ENABLED", "false")
	// No cert, no mode — but bridge disabled, so no validation runs.

	_, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
}
