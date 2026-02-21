package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsDevelopment(t *testing.T) {
	// Test development environment
	cfg := &Config{
		Environment: "development",
	}
	assert.True(t, cfg.IsDevelopment())

	// Test production environment
	cfg = &Config{
		Environment: "production",
	}
	assert.False(t, cfg.IsDevelopment())

	// Test staging environment
	cfg = &Config{
		Environment: "staging",
	}
	assert.False(t, cfg.IsDevelopment())
}

func TestLoadWithOptions(t *testing.T) {
	// Set environment variables for the test
	_ = os.Setenv("SECRET_KEY", "test-secret-key-1234567890123456") // 32 bytes
	_ = os.Setenv("ROOT_EMAIL", "test@example.com")
	_ = os.Setenv("SERVER_PORT", "9000")
	_ = os.Setenv("SERVER_HOST", "127.0.0.1")
	_ = os.Setenv("DB_HOST", "testhost")
	_ = os.Setenv("DB_PORT", "5432")
	_ = os.Setenv("DB_USER", "testuser")
	_ = os.Setenv("DB_PASSWORD", "testpass")
	_ = os.Setenv("DB_PREFIX", "test")
	_ = os.Setenv("DB_NAME", "test_system")
	_ = os.Setenv("ENVIRONMENT", "development")

	// Clean up after the test
	defer func() {
		_ = os.Unsetenv("SECRET_KEY")
		_ = os.Unsetenv("ROOT_EMAIL")
		_ = os.Unsetenv("SERVER_PORT")
		_ = os.Unsetenv("SERVER_HOST")
		_ = os.Unsetenv("DB_HOST")
		_ = os.Unsetenv("DB_PORT")
		_ = os.Unsetenv("DB_USER")
		_ = os.Unsetenv("DB_PASSWORD")
		_ = os.Unsetenv("DB_PREFIX")
		_ = os.Unsetenv("DB_NAME")
		_ = os.Unsetenv("ENVIRONMENT")
	}()

	// Load config with env vars
	cfg, err := LoadWithOptions(LoadOptions{
		// Don't specify EnvFile to force it to use environment variables
	})
	require.NoError(t, err)

	// Verify loaded config values
	assert.Equal(t, 9000, cfg.Server.Port)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, "testhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "testuser", cfg.Database.User)
	assert.Equal(t, "testpass", cfg.Database.Password)
	assert.Equal(t, "test", cfg.Database.Prefix)
	assert.Equal(t, "test_system", cfg.Database.DBName)
	assert.Equal(t, "test@example.com", cfg.RootEmail)
	assert.Equal(t, "development", cfg.Environment)

	// Verify JWT secret and SecretKey
	assert.Equal(t, "test-secret-key-1234567890123456", cfg.Security.SecretKey)
	assert.NotNil(t, cfg.Security.JWTSecret)
	assert.GreaterOrEqual(t, len(cfg.Security.JWTSecret), 32)

	// Test development environment flag
	assert.True(t, cfg.IsDevelopment())
}

func TestInvalidKeysHandling(t *testing.T) {
	t.Run("missing_secret_key", func(t *testing.T) {
		// Clear any existing environment variables
		_ = os.Unsetenv("SECRET_KEY")
		_ = os.Unsetenv("PASETO_PRIVATE_KEY")

		// Test missing SECRET_KEY
		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Equal(t, "SECRET_KEY (or PASETO_PRIVATE_KEY for backward compatibility) must be set", err.Error())
	})

	t.Run("valid_secret_key", func(t *testing.T) {
		// Clear any existing environment variables first
		_ = os.Unsetenv("SECRET_KEY")
		_ = os.Unsetenv("PASETO_PRIVATE_KEY")

		// Set SECRET_KEY with valid length
		_ = os.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
		defer func() { _ = os.Unsetenv("SECRET_KEY") }()

		// Should succeed
		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.NotNil(t, cfg.Security.JWTSecret)
		assert.Equal(t, "test-secret-key-1234567890123456", cfg.Security.SecretKey)
	})

	t.Run("backward_compatibility_paseto_private_key", func(t *testing.T) {
		// Clear any existing environment variables first
		_ = os.Unsetenv("SECRET_KEY")
		_ = os.Unsetenv("PASETO_PRIVATE_KEY")

		// Set only PASETO_PRIVATE_KEY (backward compatibility)
		_ = os.Setenv("PASETO_PRIVATE_KEY", "8OSonZEkrCTlDd612EBoORCKVMZ4OjbWlrq03n0FIEgEJK+qb95F4pwewi+Dd++qOjQ9zkviUjFdIaBUz3nzgA==")
		defer func() { _ = os.Unsetenv("PASETO_PRIVATE_KEY") }()

		// Should succeed with base64-decoded secret
		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.NotNil(t, cfg.Security.JWTSecret)
		assert.GreaterOrEqual(t, len(cfg.Security.JWTSecret), 32)
	})
}

func TestLoad(t *testing.T) {
	// Test the Load function by temporarily setting the required environment variables
	// Set environment variables for the test
	_ = os.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
	_ = os.Setenv("ROOT_EMAIL", "test@example.com")

	// Clean up after the test
	defer func() {
		_ = os.Unsetenv("SECRET_KEY")
		_ = os.Unsetenv("ROOT_EMAIL")
	}()

	// Call Load() directly
	cfg, err := Load()

	// We may get an error if the .env file doesn't exist, but the environment variables
	// should still be processed
	if err != nil {
		// This is an acceptable error if it relates to file loading
		if err.Error() == "SECRET_KEY (or PASETO_PRIVATE_KEY for backward compatibility) must be set" {
			t.Fatal("Environment variables not properly loaded")
		}
	} else {
		assert.NotNil(t, cfg)
		assert.Equal(t, "test@example.com", cfg.RootEmail)
		assert.NotNil(t, cfg.Security.JWTSecret)
	}
}

func TestDatabaseConnectionConfig_Defaults(t *testing.T) {
	// Set minimal required env vars
	_ = os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	_ = os.Setenv("DB_PASSWORD", "testpass")
	defer func() { _ = os.Unsetenv("SECRET_KEY") }()
	defer func() { _ = os.Unsetenv("DB_PASSWORD") }()

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)

	// Test default values
	assert.Equal(t, 100, cfg.Database.MaxConnections)
	assert.Equal(t, 3, cfg.Database.MaxConnectionsPerDB)
	assert.Equal(t, 10*time.Minute, cfg.Database.ConnectionMaxLifetime)
	assert.Equal(t, 5*time.Minute, cfg.Database.ConnectionMaxIdleTime)
}

func TestDatabaseConnectionConfig_CustomValues(t *testing.T) {
	// Set custom connection configuration
	_ = os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	_ = os.Setenv("DB_PASSWORD", "testpass")
	_ = os.Setenv("DB_MAX_CONNECTIONS", "200")
	_ = os.Setenv("DB_MAX_CONNECTIONS_PER_DB", "5")
	_ = os.Setenv("DB_CONNECTION_MAX_LIFETIME", "20m")
	_ = os.Setenv("DB_CONNECTION_MAX_IDLE_TIME", "10m")

	defer func() { _ = os.Unsetenv("SECRET_KEY") }()
	defer func() { _ = os.Unsetenv("DB_PASSWORD") }()
	defer func() { _ = os.Unsetenv("DB_MAX_CONNECTIONS") }()
	defer func() { _ = os.Unsetenv("DB_MAX_CONNECTIONS_PER_DB") }()
	defer func() { _ = os.Unsetenv("DB_CONNECTION_MAX_LIFETIME") }()
	defer func() { _ = os.Unsetenv("DB_CONNECTION_MAX_IDLE_TIME") }()

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)

	// Test custom values
	assert.Equal(t, 200, cfg.Database.MaxConnections)
	assert.Equal(t, 5, cfg.Database.MaxConnectionsPerDB)
	assert.Equal(t, 20*time.Minute, cfg.Database.ConnectionMaxLifetime)
	assert.Equal(t, 10*time.Minute, cfg.Database.ConnectionMaxIdleTime)
}

func TestDatabaseConnectionConfig_ValidationMinimum(t *testing.T) {
	// Test that MaxConnections below minimum fails
	_ = os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	_ = os.Setenv("DB_PASSWORD", "testpass")
	_ = os.Setenv("DB_MAX_CONNECTIONS", "10") // Below minimum of 20

	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("DB_MAX_CONNECTIONS")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS must be at least 20")
}

func TestDatabaseConnectionConfig_ValidationMaximum(t *testing.T) {
	// Test that MaxConnections above maximum fails
	_ = os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	_ = os.Setenv("DB_PASSWORD", "testpass")
	_ = os.Setenv("DB_MAX_CONNECTIONS", "15000") // Above maximum of 10000

	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("DB_MAX_CONNECTIONS")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS cannot exceed 10000")
}

func TestDatabaseConnectionConfig_ValidationPerDBMinimum(t *testing.T) {
	// Test that MaxConnectionsPerDB below minimum fails
	_ = os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	_ = os.Setenv("DB_PASSWORD", "testpass")
	_ = os.Setenv("DB_MAX_CONNECTIONS_PER_DB", "0") // Below minimum of 1

	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer func() { _ = os.Unsetenv("DB_MAX_CONNECTIONS_PER_DB") }()

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS_PER_DB must be at least 1")
}

func TestDatabaseConnectionConfig_ValidationPerDBMaximum(t *testing.T) {
	// Test that MaxConnectionsPerDB above maximum fails
	_ = os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	_ = os.Setenv("DB_PASSWORD", "testpass")
	_ = os.Setenv("DB_MAX_CONNECTIONS_PER_DB", "60") // Above maximum of 50

	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer func() { _ = os.Unsetenv("DB_MAX_CONNECTIONS_PER_DB") }()

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS_PER_DB cannot exceed 50")
}

func TestAPIEndpointTrailingSlashStripped(t *testing.T) {
	_ = os.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
	_ = os.Setenv("API_ENDPOINT", "http://localhost:8081/")
	defer func() { _ = os.Unsetenv("SECRET_KEY") }()
	defer func() { _ = os.Unsetenv("API_ENDPOINT") }()

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8081", cfg.APIEndpoint)
}
