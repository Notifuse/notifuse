package integration

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/lib/pq"
)

// TestReloadDatabaseSettings_EnvVarPrecedence verifies that environment variables
// always take precedence over database values when reloading configuration
func TestReloadDatabaseSettings_EnvVarPrecedence(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test")
	}

	// Setup test database
	dbHost := os.Getenv("TEST_DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPortStr := os.Getenv("TEST_DB_PORT")
	if dbPortStr == "" {
		dbPortStr = "5433"
	}
	
	// Parse port for both DSN (string) and Config struct (int)
	dbPortInt := 5433
	if p, err := strconv.Atoi(dbPortStr); err == nil {
		dbPortInt = p
	}

	testDBName := fmt.Sprintf("test_config_reload_%d", time.Now().UnixNano())
	systemDSN := fmt.Sprintf("host=%s port=%s user=notifuse_test password=test_password dbname=postgres sslmode=disable",
		dbHost, dbPortStr)

	db, err := sql.Open("postgres", systemDSN)
	require.NoError(t, err)
	defer db.Close()

	// Create test database
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
	require.NoError(t, err)
	defer db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))

	// Connect to test database
	testDSN := fmt.Sprintf("host=%s port=%s user=notifuse_test password=test_password dbname=%s sslmode=disable",
		dbHost, dbPortStr, testDBName)
	testDB, err := sql.Open("postgres", testDSN)
	require.NoError(t, err)
	defer testDB.Close()

	// Create settings table
	_, err = testDB.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key VARCHAR(255) PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Encrypt SMTP credentials
	secretKey := "test-secret-key-32-characters!"
	encryptedUsername, err := crypto.EncryptString("db_user", secretKey)
	require.NoError(t, err)
	encryptedPassword, err := crypto.EncryptString("db_pass", secretKey)
	require.NoError(t, err)

	// Insert test settings into database
	settings := map[string]string{
		"is_installed":            "true",
		"root_email":              "db@example.com",
		"api_endpoint":            "https://db.example.com",
		"smtp_host":               "smtp.db.example.com",
		"smtp_port":               "587",
		"encrypted_smtp_username": encryptedUsername, // Encrypted!
		"encrypted_smtp_password": encryptedPassword, // Encrypted!
		"smtp_from_email":         "db@example.com",
		"smtp_from_name":          "DB Mailer",
	}

	for key, value := range settings {
		_, err := testDB.Exec("INSERT INTO settings (key, value) VALUES ($1, $2)", key, value)
		require.NoError(t, err)
	}

	// Create config with ENV VALUES set (simulating environment variables)
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:     dbHost,
			Port:     dbPortInt,
			User:     "notifuse_test",
			Password: "test_password",
			DBName:   testDBName,
			SSLMode:  "disable",
		},
		Security: config.SecurityConfig{
			SecretKey: "test-secret-key-32-characters!",
		},
		// Simulate env values being set
		EnvValues: config.EnvValues{
			RootEmail:      "env@example.com",       // ENV VAR SET
			APIEndpoint:    "https://env.example.com", // ENV VAR SET
			SMTPHost:       "smtp.env.example.com",    // ENV VAR SET
			SMTPPort:       2525,                      // ENV VAR SET
			SMTPFromEmail:  "env@example.com",         // ENV VAR SET
			// Note: SMTPUsername, SMTPPassword, SMTPFromName NOT set in env
		},
		// Initial config values (from env vars)
		RootEmail:   "env@example.com",
		APIEndpoint: "https://env.example.com",
		SMTP: config.SMTPConfig{
			Host:      "smtp.env.example.com",
			Port:      2525,
			FromEmail: "env@example.com",
			// Username, Password, FromName will be empty initially
		},
	}

	// Reload database settings
	err = cfg.ReloadDatabaseSettings()
	require.NoError(t, err)

	// Verify ENV VARS are PRESERVED (not overwritten by database)
	assert.Equal(t, "env@example.com", cfg.RootEmail, 
		"Root email should preserve env var value")
	assert.Equal(t, "https://env.example.com", cfg.APIEndpoint, 
		"API endpoint should preserve env var value")
	assert.Equal(t, "smtp.env.example.com", cfg.SMTP.Host, 
		"SMTP host should preserve env var value")
	assert.Equal(t, 2525, cfg.SMTP.Port, 
		"SMTP port should preserve env var value")
	assert.Equal(t, "env@example.com", cfg.SMTP.FromEmail, 
		"SMTP from email should preserve env var value")

	// Verify DATABASE VALUES are used for fields NOT set in env
	assert.Equal(t, "db_user", cfg.SMTP.Username, 
		"SMTP username should use database value when env var not set")
	assert.Equal(t, "db_pass", cfg.SMTP.Password, 
		"SMTP password should use database value when env var not set")
	assert.Equal(t, "DB Mailer", cfg.SMTP.FromName, 
		"SMTP from name should use database value when env var not set")

	// Verify IsInstalled is always updated (it's not an env var)
	assert.True(t, cfg.IsInstalled, "IsInstalled should be updated from database")
}

// TestReloadDatabaseSettings_DatabaseOnlyValues verifies that database values
// are correctly loaded when NO environment variables are set
func TestReloadDatabaseSettings_DatabaseOnlyValues(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test")
	}

	// Setup test database (similar setup as above)
	dbHost := os.Getenv("TEST_DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPortStr := os.Getenv("TEST_DB_PORT")
	if dbPortStr == "" {
		dbPortStr = "5433"
	}
	
	// Parse port for both DSN (string) and Config struct (int)
	dbPortInt := 5433
	if p, err := strconv.Atoi(dbPortStr); err == nil {
		dbPortInt = p
	}

	testDBName := fmt.Sprintf("test_config_reload_db_%d", time.Now().UnixNano())
	systemDSN := fmt.Sprintf("host=%s port=%s user=notifuse_test password=test_password dbname=postgres sslmode=disable",
		dbHost, dbPortStr)

	db, err := sql.Open("postgres", systemDSN)
	require.NoError(t, err)
	defer db.Close()

	// Create and setup test database
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
	require.NoError(t, err)
	defer db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))

	testDSN := fmt.Sprintf("host=%s port=%s user=notifuse_test password=test_password dbname=%s sslmode=disable",
		dbHost, dbPortStr, testDBName)
	testDB, err := sql.Open("postgres", testDSN)
	require.NoError(t, err)
	defer testDB.Close()

	_, err = testDB.Exec(`CREATE TABLE IF NOT EXISTS settings (key VARCHAR(255) PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)

	// Insert test settings
	settings := map[string]string{
		"is_installed":     "true",
		"root_email":       "dbonly@example.com",
		"smtp_from_email":  "dbonly@example.com",
		"smtp_from_name":   "DB Only Mailer",
	}

	for key, value := range settings {
		_, err := testDB.Exec("INSERT INTO settings (key, value) VALUES ($1, $2)", key, value)
		require.NoError(t, err)
	}

	// Create config with NO env values set
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:     dbHost,
			Port:     dbPortInt,
			User:     "notifuse_test",
			Password: "test_password",
			DBName:   testDBName,
			SSLMode:  "disable",
		},
		Security: config.SecurityConfig{
			SecretKey: "test-secret-key-32-characters!",
		},
		EnvValues: config.EnvValues{}, // NO environment variables set
	}

	// Reload database settings
	err = cfg.ReloadDatabaseSettings()
	require.NoError(t, err)

	// Verify all values come from database
	assert.Equal(t, "dbonly@example.com", cfg.RootEmail, 
		"Root email should use database value when env var not set")
	assert.Equal(t, "dbonly@example.com", cfg.SMTP.FromEmail, 
		"SMTP from email should use database value when env var not set")
	assert.Equal(t, "DB Only Mailer", cfg.SMTP.FromName, 
		"SMTP from name should use database value when env var not set")

	// Verify defaults are applied for missing values
	assert.Equal(t, 587, cfg.SMTP.Port, 
		"SMTP port should use default when neither env var nor database value set")
}
