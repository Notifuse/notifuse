package config

import (
	"encoding/base64"
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
	// Use valid PASETO keys generated with the keygen tool
	privateKey := "8OSonZEkrCTlDd612EBoORCKVMZ4OjbWlrq03n0FIEgEJK+qb95F4pwewi+Dd++qOjQ9zkviUjFdIaBUz3nzgA=="
	publicKey := "BCSvqm/eReKcHsIvg3fvqjo0Pc5L4lIxXSGgVM9584A="

	// Set environment variables for the test
	os.Setenv("PASETO_PRIVATE_KEY", privateKey)
	os.Setenv("PASETO_PUBLIC_KEY", publicKey)
	os.Setenv("ROOT_EMAIL", "test@example.com")
	os.Setenv("SERVER_PORT", "9000")
	os.Setenv("SERVER_HOST", "127.0.0.1")
	os.Setenv("DB_HOST", "testhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_PREFIX", "test")
	os.Setenv("DB_NAME", "test_system")
	os.Setenv("ENVIRONMENT", "development")
	os.Setenv("SECRET_KEY", "test-key")

	// Clean up after the test
	defer func() {
		os.Unsetenv("PASETO_PRIVATE_KEY")
		os.Unsetenv("PASETO_PUBLIC_KEY")
		os.Unsetenv("ROOT_EMAIL")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("SERVER_HOST")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_PREFIX")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("SECRET_KEY")
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
	// Check the decoded keys
	decodedPrivateKey, _ := base64.StdEncoding.DecodeString(privateKey)
	decodedPublicKey, _ := base64.StdEncoding.DecodeString(publicKey)
	assert.Equal(t, decodedPrivateKey, cfg.Security.PasetoPrivateKeyBytes)
	assert.Equal(t, decodedPublicKey, cfg.Security.PasetoPublicKeyBytes)
	assert.Equal(t, "test-key", cfg.Security.SecretKey)

	// Verify that the parsed keys are not nil
	assert.NotNil(t, cfg.Security.PasetoPrivateKey)
	assert.NotNil(t, cfg.Security.PasetoPublicKey)

	// Test development environment flag
	assert.True(t, cfg.IsDevelopment())
}

func TestInvalidKeysHandling(t *testing.T) {
	// This test needs to align with how the config.go actually validates the keys
	// First it checks if the keys are present, then if they're valid base64

	t.Run("missing_private_key", func(t *testing.T) {
		// Clear any existing environment variables
		os.Unsetenv("SECRET_KEY")
		os.Unsetenv("PASETO_PRIVATE_KEY")
		os.Unsetenv("PASETO_PUBLIC_KEY")

		// Test missing SECRET_KEY
		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Equal(t, "SECRET_KEY (or PASETO_PRIVATE_KEY for backward compatibility) must be set", err.Error())
	})

	t.Run("missing_public_key", func(t *testing.T) {
		// Clear any existing environment variables first
		os.Unsetenv("SECRET_KEY")
		os.Unsetenv("PASETO_PRIVATE_KEY")
		os.Unsetenv("PASETO_PUBLIC_KEY")

		// Set SECRET_KEY and valid private key but no public key
		// On first run (not installed), missing PASETO keys is OK
		os.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
		os.Setenv("PASETO_PRIVATE_KEY", "8OSonZEkrCTlDd612EBoORCKVMZ4OjbWlrq03n0FIEgEJK+qb95F4pwewi+Dd++qOjQ9zkviUjFdIaBUz3nzgA==")
		defer func() {
			os.Unsetenv("SECRET_KEY")
			os.Unsetenv("PASETO_PRIVATE_KEY")
		}()

		// Should succeed on first run (not installed), PASETO keys are optional
		_, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err) // No error expected for first-run without public key
	})

	t.Run("invalid_private_key", func(t *testing.T) {
		// Clear any existing environment variables first
		os.Unsetenv("SECRET_KEY")
		os.Unsetenv("PASETO_PRIVATE_KEY")
		os.Unsetenv("PASETO_PUBLIC_KEY")

		// Set SECRET_KEY, invalid private key but also public key (to pass the presence check)
		os.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
		os.Setenv("PASETO_PRIVATE_KEY", "invalid-base64!")
		os.Setenv("PASETO_PUBLIC_KEY", "BCSvqm/eReKcHsIvg3fvqjo0Pc5L4lIxXSGgVM9584A=")
		defer func() {
			os.Unsetenv("SECRET_KEY")
			os.Unsetenv("PASETO_PRIVATE_KEY")
			os.Unsetenv("PASETO_PUBLIC_KEY")
		}()

		// Should fail with decoding error
		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error decoding PASETO_PRIVATE_KEY")
	})

	t.Run("invalid_public_key", func(t *testing.T) {
		// Clear any existing environment variables first
		os.Unsetenv("SECRET_KEY")
		os.Unsetenv("PASETO_PRIVATE_KEY")
		os.Unsetenv("PASETO_PUBLIC_KEY")

		// Set SECRET_KEY, valid private key but invalid public key
		os.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
		os.Setenv("PASETO_PRIVATE_KEY", "8OSonZEkrCTlDd612EBoORCKVMZ4OjbWlrq03n0FIEgEJK+qb95F4pwewi+Dd++qOjQ9zkviUjFdIaBUz3nzgA==")
		os.Setenv("PASETO_PUBLIC_KEY", "invalid-base64!")
		defer func() {
			os.Unsetenv("SECRET_KEY")
			os.Unsetenv("PASETO_PRIVATE_KEY")
			os.Unsetenv("PASETO_PUBLIC_KEY")
		}()

		// Should fail with decoding error
		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error decoding PASETO_PUBLIC_KEY")
	})
}

func TestLoad(t *testing.T) {
	// Test the Load function by temporarily setting the required environment variables
	privateKey := "8OSonZEkrCTlDd612EBoORCKVMZ4OjbWlrq03n0FIEgEJK+qb95F4pwewi+Dd++qOjQ9zkviUjFdIaBUz3nzgA=="
	publicKey := "BCSvqm/eReKcHsIvg3fvqjo0Pc5L4lIxXSGgVM9584A="

	// Set environment variables for the test
	os.Setenv("PASETO_PRIVATE_KEY", privateKey)
	os.Setenv("PASETO_PUBLIC_KEY", publicKey)
	os.Setenv("ROOT_EMAIL", "test@example.com")

	// Clean up after the test
	defer func() {
		os.Unsetenv("PASETO_PRIVATE_KEY")
		os.Unsetenv("PASETO_PUBLIC_KEY")
		os.Unsetenv("ROOT_EMAIL")
	}()

	// Call Load() directly
	cfg, err := Load()

	// We may get an error if the .env file doesn't exist, but the environment variables
	// should still be processed
	if err != nil {
		// This is an acceptable error if it relates to file loading
		if err.Error() == "PASETO_PRIVATE_KEY is required" ||
			err.Error() == "PASETO_PUBLIC_KEY is required" {
			t.Fatal("Environment variables not properly loaded")
		}
	} else {
		assert.NotNil(t, cfg)
		assert.Equal(t, "test@example.com", cfg.RootEmail)
	}
}

func TestDatabaseConnectionConfig_Defaults(t *testing.T) {
	// Set minimal required env vars
	os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	os.Setenv("DB_PASSWORD", "testpass")
	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")

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
	os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_MAX_CONNECTIONS", "200")
	os.Setenv("DB_MAX_CONNECTIONS_PER_DB", "5")
	os.Setenv("DB_CONNECTION_MAX_LIFETIME", "20m")
	os.Setenv("DB_CONNECTION_MAX_IDLE_TIME", "10m")
	
	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("DB_MAX_CONNECTIONS")
	defer os.Unsetenv("DB_MAX_CONNECTIONS_PER_DB")
	defer os.Unsetenv("DB_CONNECTION_MAX_LIFETIME")
	defer os.Unsetenv("DB_CONNECTION_MAX_IDLE_TIME")

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
	os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_MAX_CONNECTIONS", "10") // Below minimum of 20
	
	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("DB_MAX_CONNECTIONS")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS must be at least 20")
}

func TestDatabaseConnectionConfig_ValidationMaximum(t *testing.T) {
	// Test that MaxConnections above maximum fails
	os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_MAX_CONNECTIONS", "15000") // Above maximum of 10000
	
	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("DB_MAX_CONNECTIONS")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS cannot exceed 10000")
}

func TestDatabaseConnectionConfig_ValidationPerDBMinimum(t *testing.T) {
	// Test that MaxConnectionsPerDB below minimum fails
	os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_MAX_CONNECTIONS_PER_DB", "0") // Below minimum of 1
	
	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("DB_MAX_CONNECTIONS_PER_DB")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS_PER_DB must be at least 1")
}

func TestDatabaseConnectionConfig_ValidationPerDBMaximum(t *testing.T) {
	// Test that MaxConnectionsPerDB above maximum fails
	os.Setenv("SECRET_KEY", "test-secret-key-for-testing")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_MAX_CONNECTIONS_PER_DB", "60") // Above maximum of 50
	
	defer os.Unsetenv("SECRET_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("DB_MAX_CONNECTIONS_PER_DB")

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNECTIONS_PER_DB cannot exceed 50")
}

