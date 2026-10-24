package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LoadWithOptions is the first code that runs at startup; its validation
// errors are the operator's only guidance when an env var is out of range,
// so exact messages and boundary values are pinned. SECRET_KEY handling is
// security-critical: the PASETO fallback exists for old installs and must
// keep working, and a missing key must fail fast rather than boot with a
// zero key.

const loadTestSecret = "test-secret-key-for-config-load-0123456789abcdef"

func loadWithEnv(t *testing.T, env map[string]string) (*Config, error) {
	t.Helper()
	// Isolate from the developer's real environment for the keys under test.
	for _, key := range []string{
		"SECRET_KEY", "PASETO_PRIVATE_KEY", "DB_HOST", "DB_PORT",
		"DB_MAX_CONNECTIONS", "DB_MAX_CONNECTIONS_PER_DB",
		"EMAIL_QUEUE_CONCURRENCY", "EMAIL_QUEUE_SMTP_CONCURRENCY", "ENVIRONMENT",
	} {
		t.Setenv(key, "")
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	return LoadWithOptions(LoadOptions{EnvFile: ""})
}

func TestLoadWithOptions_Validation(t *testing.T) {
	t.Run("DB_MAX_CONNECTIONS bounds are enforced with the value named", func(t *testing.T) {
		_, err := loadWithEnv(t, map[string]string{
			"SECRET_KEY": loadTestSecret, "DB_MAX_CONNECTIONS": "19",
		})
		require.EqualError(t, err, "DB_MAX_CONNECTIONS must be at least 20 (got 19)")

		_, err = loadWithEnv(t, map[string]string{
			"SECRET_KEY": loadTestSecret, "DB_MAX_CONNECTIONS": "10001",
		})
		require.EqualError(t, err, "DB_MAX_CONNECTIONS cannot exceed 10000 (got 10001)")

		for _, boundary := range []string{"20", "10000"} {
			_, err = loadWithEnv(t, map[string]string{
				"SECRET_KEY": loadTestSecret, "DB_MAX_CONNECTIONS": boundary,
			})
			require.NoError(t, err, "boundary %s must be accepted", boundary)
		}
	})

	t.Run("DB_MAX_CONNECTIONS_PER_DB bounds are enforced", func(t *testing.T) {
		_, err := loadWithEnv(t, map[string]string{
			"SECRET_KEY": loadTestSecret, "DB_MAX_CONNECTIONS_PER_DB": "0",
		})
		require.EqualError(t, err, "DB_MAX_CONNECTIONS_PER_DB must be at least 1 (got 0)")

		_, err = loadWithEnv(t, map[string]string{
			"SECRET_KEY": loadTestSecret, "DB_MAX_CONNECTIONS_PER_DB": "51",
		})
		require.EqualError(t, err, "DB_MAX_CONNECTIONS_PER_DB cannot exceed 50 (got 51)")
	})

	t.Run("email queue concurrency bounds name their variable", func(t *testing.T) {
		_, err := loadWithEnv(t, map[string]string{
			"SECRET_KEY": loadTestSecret, "EMAIL_QUEUE_CONCURRENCY": "0",
		})
		require.EqualError(t, err, "EMAIL_QUEUE_CONCURRENCY must be at least 1 (got 0)")

		_, err = loadWithEnv(t, map[string]string{
			"SECRET_KEY": loadTestSecret, "EMAIL_QUEUE_SMTP_CONCURRENCY": "51",
		})
		require.EqualError(t, err, "EMAIL_QUEUE_SMTP_CONCURRENCY cannot exceed 50 (got 51)")
	})
}

func TestLoadWithOptions_SecretKeyResolution(t *testing.T) {
	t.Run("no key at all fails fast", func(t *testing.T) {
		_, err := loadWithEnv(t, map[string]string{})
		require.EqualError(t, err,
			"SECRET_KEY (or PASETO_PRIVATE_KEY for backward compatibility) must be set")
	})

	t.Run("the legacy PASETO variable still works and is credited", func(t *testing.T) {
		cfg, err := loadWithEnv(t, map[string]string{
			"PASETO_PRIVATE_KEY": loadTestSecret,
		})
		require.NoError(t, err)
		assert.Equal(t, loadTestSecret, cfg.Security.SecretKey)
		assert.Equal(t, "PASETO_PRIVATE_KEY", cfg.Security.KeyVariable,
			"operators must be told which variable supplied the key")
	})

	t.Run("SECRET_KEY wins over the legacy variable", func(t *testing.T) {
		cfg, err := loadWithEnv(t, map[string]string{
			"SECRET_KEY":         loadTestSecret,
			"PASETO_PRIVATE_KEY": "legacy-key-that-must-not-win-0123456789",
		})
		require.NoError(t, err)
		assert.Equal(t, loadTestSecret, cfg.Security.SecretKey)
		assert.Equal(t, "SECRET_KEY", cfg.Security.KeyVariable)
	})
}

func TestLoadWithOptions_DefaultsAndEnvPrecedence(t *testing.T) {
	cfg, err := loadWithEnv(t, map[string]string{"SECRET_KEY": loadTestSecret})
	require.NoError(t, err)

	// Defaults an operator gets with a bare SECRET_KEY.
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, 0, cfg.Plan.MaxActiveContacts, "plan limits default to unlimited")

	cfg, err = loadWithEnv(t, map[string]string{
		"SECRET_KEY":  loadTestSecret,
		"DB_HOST":     "db.internal",
		"DB_PORT":     "6543",
		"ENVIRONMENT": "demo",
	})
	require.NoError(t, err)
	assert.Equal(t, "db.internal", cfg.Database.Host)
	assert.Equal(t, 6543, cfg.Database.Port)
	assert.Equal(t, "demo", cfg.Environment)
	assert.True(t, cfg.IsDemo())
}
