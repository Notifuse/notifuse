package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_EnvironmentPredicates(t *testing.T) {
	// Each predicate must match exactly one environment string: a typo'd
	// comparison would silently flip demo/production behaviour (demo data
	// seeding, production guards), so pin all three against each other.
	cases := []struct {
		env        string
		demo, prod bool
	}{
		{"demo", true, false},
		{"production", false, true},
		{"development", false, false},
		{"", false, false},
		{"Demo", false, false},       // case-sensitive: env values are lowercase by convention
		{"PRODUCTION", false, false}, // case-sensitive
	}
	for _, tc := range cases {
		cfg := &Config{Environment: tc.env}
		assert.Equal(t, tc.demo, cfg.IsDemo(), "IsDemo for env %q", tc.env)
		assert.Equal(t, tc.prod, cfg.IsProduction(), "IsProduction for env %q", tc.env)
	}
}

func TestConfig_GetEnvValues(t *testing.T) {
	cfg := &Config{}
	cfg.EnvValues = EnvValues{
		RootEmail:               "root@example.com",
		APIEndpoint:             "https://api.example.com",
		SMTPHost:                "smtp.example.com",
		SMTPUsername:            "smtp-user",
		SMTPPassword:            "smtp-pass",
		SMTPFromEmail:           "from@example.com",
		SMTPFromName:            "Example Sender",
		SMTPPort:                587,
		SMTPUseTLS:              "true",
		SMTPBridgeEnabled:       "false",
		SMTPBridgeDomain:        "bridge.example.com",
		SMTPBridgeTLSCertBase64: "cert-b64",
		SMTPBridgeTLSKeyBase64:  "key-b64",
		SMTPBridgePort:          465,
	}

	rootEmail, apiEndpoint, smtpHost, smtpUsername, smtpPassword, smtpFromEmail, smtpFromName,
		smtpPort, smtpUseTLS, smtpBridgeEnabled, smtpBridgeDomain, smtpBridgeTLSCert, smtpBridgeTLSKey, smtpBridgePort := cfg.GetEnvValues()

	// Positional contract: the setup service consumes these positionally, so
	// every value carries a distinct sentinel and a swapped return fails.
	assert.Equal(t, "root@example.com", rootEmail)
	assert.Equal(t, "https://api.example.com", apiEndpoint)
	assert.Equal(t, "smtp.example.com", smtpHost)
	assert.Equal(t, "smtp-user", smtpUsername)
	assert.Equal(t, "smtp-pass", smtpPassword)
	assert.Equal(t, "from@example.com", smtpFromEmail)
	assert.Equal(t, "Example Sender", smtpFromName)
	assert.Equal(t, 587, smtpPort)
	assert.Equal(t, "true", smtpUseTLS)
	assert.Equal(t, "false", smtpBridgeEnabled)
	assert.Equal(t, "bridge.example.com", smtpBridgeDomain)
	assert.Equal(t, "cert-b64", smtpBridgeTLSCert)
	assert.Equal(t, "key-b64", smtpBridgeTLSKey)
	assert.Equal(t, 465, smtpBridgePort)
}

func TestConfig_GetEnvValues_ZeroConfig(t *testing.T) {
	// A config with no env-sourced values must report empties/zeros, which is
	// how the setup wizard decides nothing is pre-configured.
	cfg := &Config{}
	rootEmail, _, smtpHost, _, _, _, _, smtpPort, _, _, _, _, _, smtpBridgePort := cfg.GetEnvValues()
	assert.Equal(t, "", rootEmail)
	assert.Equal(t, "", smtpHost)
	assert.Equal(t, 0, smtpPort)
	assert.Equal(t, 0, smtpBridgePort)
}

func TestConfig_IsRootEmailInsensitive_Method(t *testing.T) {
	cfg := &Config{RootEmail: "Admin@Example.com, ops@example.com"}

	require.True(t, cfg.IsRootEmailInsensitive("admin@example.com"),
		"the OIDC path relies on case-insensitive matching")
	require.True(t, cfg.IsRootEmailInsensitive("OPS@EXAMPLE.COM"))
	assert.False(t, cfg.IsRootEmailInsensitive("other@example.com"))
	assert.False(t, cfg.IsRootEmailInsensitive(""), "an empty candidate never matches")

	empty := &Config{}
	assert.False(t, empty.IsRootEmailInsensitive("admin@example.com"))
}
