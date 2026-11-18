package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveSMTPConfig(t *testing.T) {
	stored := &SystemSettings{
		IsInstalled:      true,
		SMTPHost:         "smtp.stored.com",
		SMTPPort:         2525,
		SMTPUsername:     "stored-user",
		SMTPPassword:     "stored-pass",
		SMTPFromEmail:    "stored@acme.com",
		SMTPFromName:     "Acme",
		SMTPUseTLS:       false,
		SMTPEHLOHostname: "mail.acme.com",
	}

	t.Run("installed without env uses every stored value", func(t *testing.T) {
		got := resolveSMTPConfig(EnvValues{}, stored, true)
		assert.Equal(t, SMTPConfig{
			Host: "smtp.stored.com", Port: 2525, Username: "stored-user", Password: "stored-pass",
			FromEmail: "stored@acme.com", FromName: "Acme", UseTLS: false, EHLOHostname: "mail.acme.com",
		}, got)
	})

	t.Run("a non-empty env var wins field by field", func(t *testing.T) {
		got := resolveSMTPConfig(EnvValues{SMTPHost: "smtp.env.com", SMTPFromName: "Env name", SMTPUseTLS: "true"}, stored, true)
		assert.Equal(t, "smtp.env.com", got.Host)
		assert.Equal(t, "Env name", got.FromName)
		assert.True(t, got.UseTLS)
		assert.Equal(t, 2525, got.Port, "port not set by env comes from the database")
		assert.Equal(t, "stored@acme.com", got.FromEmail)
	})

	t.Run("installed with nothing stored falls back to the built-in values", func(t *testing.T) {
		got := resolveSMTPConfig(EnvValues{}, &SystemSettings{IsInstalled: true, SMTPUseTLS: true}, true)
		assert.Equal(t, defaultSMTPPort, got.Port)
		assert.Equal(t, "Notifuse", got.FromName)
		assert.True(t, got.UseTLS)
	})

	t.Run("first run ignores the database and applies the built-in values", func(t *testing.T) {
		got := resolveSMTPConfig(EnvValues{SMTPHost: "smtp.env.com"}, stored, false)
		assert.Equal(t, "smtp.env.com", got.Host)
		assert.Equal(t, defaultSMTPPort, got.Port)
		assert.Equal(t, "Notifuse", got.FromName)
		assert.True(t, got.UseTLS, "TLS is on unless SMTP_USE_TLS=false")
		assert.Empty(t, got.FromEmail)
	})

	t.Run("SMTP_USE_TLS=false wins over a stored true", func(t *testing.T) {
		got := resolveSMTPConfig(EnvValues{SMTPUseTLS: "false"}, &SystemSettings{IsInstalled: true, SMTPUseTLS: true}, true)
		assert.False(t, got.UseTLS)
	})
}

func TestResolveSMTPBridgeConfig(t *testing.T) {
	stored := &SystemSettings{
		IsInstalled:             true,
		SMTPBridgeEnabled:       true,
		SMTPBridgeDomain:        "bridge.acme.com",
		SMTPBridgePort:          2526,
		SMTPBridgeTLSCertBase64: "stored-cert",
		SMTPBridgeTLSKeyBase64:  "stored-key",
	}

	t.Run("installed without env uses the stored values, but not the stored port", func(t *testing.T) {
		// compose publishes the container port from SMTP_BRIDGE_PORT: a stored port
		// would move the bridge off it.
		got := resolveSMTPBridgeConfig(EnvValues{}, stored, true)
		assert.Equal(t, SMTPBridgeConfig{
			Enabled: true, Port: defaultSMTPBridgePort, Host: "0.0.0.0", Domain: "bridge.acme.com",
			TLSCertBase64: "stored-cert", TLSKeyBase64: "stored-key",
		}, got)
	})

	t.Run("SMTP_BRIDGE_ENABLED=false wins over a stored true", func(t *testing.T) {
		got := resolveSMTPBridgeConfig(EnvValues{SMTPBridgeEnabled: "false"}, stored, true)
		assert.False(t, got.Enabled)
	})

	t.Run("env port and TLS mode win, the mode never comes from the database", func(t *testing.T) {
		got := resolveSMTPBridgeConfig(EnvValues{SMTPBridgePort: 2600, SMTPBridgeTLSMode: "implicit"}, stored, true)
		assert.Equal(t, 2600, got.Port)
		assert.Equal(t, "implicit", got.TLSMode)
	})

	t.Run("installed with nothing stored uses the built-in port and no domain", func(t *testing.T) {
		got := resolveSMTPBridgeConfig(EnvValues{}, &SystemSettings{IsInstalled: true}, true)
		assert.Equal(t, defaultSMTPBridgePort, got.Port)
		assert.Empty(t, got.Domain)
	})

	t.Run("first run uses the built-in port and greets as localhost", func(t *testing.T) {
		got := resolveSMTPBridgeConfig(EnvValues{}, stored, false)
		assert.False(t, got.Enabled)
		assert.Equal(t, defaultSMTPBridgePort, got.Port)
		assert.Equal(t, "localhost", got.Domain)
	})
}
