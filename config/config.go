package config

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/pkg/crypto"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/spf13/viper"
)

const VERSION = "13.2"

type Config struct {
	Server          ServerConfig
	Database        DatabaseConfig
	Security        SecurityConfig
	Tracing         TracingConfig
	SMTP            SMTPConfig
	Demo            DemoConfig
	Broadcast       BroadcastConfig
	Telemetry       bool
	RootEmail       string
	Environment     string
	APIEndpoint     string
	WebhookEndpoint string
	LogLevel        string
	Version         string
	IsInstalled     bool // NEW: Indicates if setup wizard has been completed

	// Track which values came from actual environment variables (not database, not generated)
	envValues envValues
}

// envValues tracks configuration that came from actual environment variables
type envValues struct {
	RootEmail        string
	APIEndpoint      string
	PasetoPublicKey  string
	PasetoPrivateKey string
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPassword     string
	SMTPFromEmail    string
	SMTPFromName     string
}

type DemoConfig struct {
	FileManagerEndpoint  string
	FileManagerBucket    string
	FileManagerAccessKey string
	FileManagerSecretKey string
}

type ServerConfig struct {
	Port int
	Host string
	SSL  SSLConfig
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	Prefix   string
	SSLMode  string
}

type SecurityConfig struct {
	// PASETO key types
	PasetoPrivateKey paseto.V4AsymmetricSecretKey
	PasetoPublicKey  paseto.V4AsymmetricPublicKey

	// Raw decoded bytes for compatibility
	PasetoPrivateKeyBytes []byte
	PasetoPublicKeyBytes  []byte

	// Secret passphrase for workspace settings encryption
	SecretKey string
}

type SSLConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

type TracingConfig struct {
	Enabled             bool
	ServiceName         string
	SamplingProbability float64

	// Trace exporter configuration
	TraceExporter string // "jaeger", "stackdriver", "zipkin", "azure", "datadog", "xray", "none"

	// Jaeger settings
	JaegerEndpoint string

	// Zipkin settings
	ZipkinEndpoint string

	// Stackdriver settings
	StackdriverProjectID string

	// Azure Monitor settings
	AzureInstrumentationKey string

	// Datadog settings
	DatadogAgentAddress string
	DatadogAPIKey       string

	// AWS X-Ray settings
	XRayRegion string

	// General agent endpoint (for exporters that support a common agent)
	AgentEndpoint string

	// Metrics exporter configuration
	MetricsExporter string // "prometheus", "stackdriver", "datadog", "none" or comma-separated list
	PrometheusPort  int
}

type SMTPConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

type BroadcastConfig struct {
	DefaultRateLimit int // Default rate limit per minute for broadcasts (0 means use service default)
}

// LoadOptions contains options for loading configuration
type LoadOptions struct {
	EnvFile string // Optional environment file to load (e.g., ".env", ".env.test")
}

// SystemSettings holds configuration loaded from database
type SystemSettings struct {
	IsInstalled      bool
	RootEmail        string
	APIEndpoint      string
	PasetoPublicKey  string // Base64 encoded
	PasetoPrivateKey string // Base64 encoded
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPassword     string
	SMTPFromEmail    string
	SMTPFromName     string
}

// getSystemDSN constructs the database connection string for the system database
func getSystemDSN(cfg *DatabaseConfig) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}

	// Build DSN, omitting password if empty
	var dsn string
	if cfg.Password == "" {
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s dbname=%s sslmode=%s",
			cfg.Host,
			cfg.Port,
			cfg.User,
			cfg.DBName,
			sslMode,
		)
	} else {
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host,
			cfg.Port,
			cfg.User,
			cfg.Password,
			cfg.DBName,
			sslMode,
		)
	}

	return dsn
}

// loadSystemSettings loads configuration from the database settings table
func loadSystemSettings(db *sql.DB, secretKey string) (*SystemSettings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	settings := &SystemSettings{
		IsInstalled: false, // Default to false if not found
		SMTPPort:    587,   // Default SMTP port
	}

	// Load all settings from database
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		// If settings table doesn't exist yet, return default settings
		return settings, nil
	}
	defer rows.Close()

	settingsMap := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settingsMap[key] = value
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating settings: %w", err)
	}

	// Parse is_installed
	if val, ok := settingsMap["is_installed"]; ok && val == "true" {
		settings.IsInstalled = true
	}

	// Load other settings if installed
	if settings.IsInstalled {
		settings.RootEmail = settingsMap["root_email"]
		settings.APIEndpoint = settingsMap["api_endpoint"]

		// Decrypt PASETO keys if present
		if encryptedPrivateKey, ok := settingsMap["encrypted_paseto_private_key"]; ok && encryptedPrivateKey != "" {
			if decrypted, err := crypto.DecryptFromHexString(encryptedPrivateKey, secretKey); err == nil {
				settings.PasetoPrivateKey = decrypted
			}
		}

		if encryptedPublicKey, ok := settingsMap["encrypted_paseto_public_key"]; ok && encryptedPublicKey != "" {
			if decrypted, err := crypto.DecryptFromHexString(encryptedPublicKey, secretKey); err == nil {
				settings.PasetoPublicKey = decrypted
			}
		}

		// Load SMTP settings
		settings.SMTPHost = settingsMap["smtp_host"]
		if port, ok := settingsMap["smtp_port"]; ok && port != "" {
			fmt.Sscanf(port, "%d", &settings.SMTPPort)
		}
		settings.SMTPFromEmail = settingsMap["smtp_from_email"]
		settings.SMTPFromName = settingsMap["smtp_from_name"]

		// Decrypt SMTP username if present
		if encryptedUsername, ok := settingsMap["encrypted_smtp_username"]; ok && encryptedUsername != "" {
			if decrypted, err := crypto.DecryptFromHexString(encryptedUsername, secretKey); err == nil {
				settings.SMTPUsername = decrypted
			}
		}

		// Decrypt SMTP password if present
		if encryptedPassword, ok := settingsMap["encrypted_smtp_password"]; ok && encryptedPassword != "" {
			if decrypted, err := crypto.DecryptFromHexString(encryptedPassword, secretKey); err == nil {
				settings.SMTPPassword = decrypted
			}
		}
	}

	return settings, nil
}

// Load loads the configuration with default options
func Load() (*Config, error) {
	// Try to load .env file but don't require it
	return LoadWithOptions(LoadOptions{EnvFile: ".env"})
}

// LoadWithOptions loads the configuration with the specified options
func LoadWithOptions(opts LoadOptions) (*Config, error) {
	v := viper.New()

	// Set default values
	v.SetDefault("SERVER_PORT", 8080)
	v.SetDefault("SERVER_HOST", "0.0.0.0")
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", 5432)
	v.SetDefault("DB_USER", "postgres")
	v.SetDefault("DB_PASSWORD", "postgres")
	v.SetDefault("DB_PREFIX", "notifuse")
	v.SetDefault("DB_NAME", "notifuse_system")
	v.SetDefault("DB_SSLMODE", "require")
	v.SetDefault("ENVIRONMENT", "production")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("VERSION", VERSION)

	// SMTP defaults
	v.SetDefault("SMTP_FROM_NAME", "Notifuse")

	// Default tracing config
	v.SetDefault("TRACING_ENABLED", false)
	v.SetDefault("TRACING_SERVICE_NAME", "notifuse-api")
	v.SetDefault("TRACING_SAMPLING_PROBABILITY", 0.1)

	// Default trace exporter config
	v.SetDefault("TRACING_TRACE_EXPORTER", "none")

	// Jaeger settings
	v.SetDefault("TRACING_JAEGER_ENDPOINT", "http://localhost:14268/api/traces")

	// Zipkin settings
	v.SetDefault("TRACING_ZIPKIN_ENDPOINT", "http://localhost:9411/api/v2/spans")

	// Stackdriver settings
	v.SetDefault("TRACING_STACKDRIVER_PROJECT_ID", "")

	// Azure Monitor settings
	v.SetDefault("TRACING_AZURE_INSTRUMENTATION_KEY", "")

	// Datadog settings
	v.SetDefault("TRACING_DATADOG_AGENT_ADDRESS", "localhost:8126")
	v.SetDefault("TRACING_DATADOG_API_KEY", "")

	// AWS X-Ray settings
	v.SetDefault("TRACING_XRAY_REGION", "us-west-2")

	// General agent endpoint (for exporters that support a common agent)
	v.SetDefault("TRACING_AGENT_ENDPOINT", "localhost:8126")

	// Default metrics exporter config
	v.SetDefault("TRACING_METRICS_EXPORTER", "none")
	v.SetDefault("TRACING_PROMETHEUS_PORT", 9464)

	// Default telemetry config
	v.SetDefault("TELEMETRY", true)

	// Load environment file if specified
	if opts.EnvFile != "" {
		v.SetConfigName(opts.EnvFile)
		v.SetConfigType("env")

		currentPath, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("error getting current directory: %w", err)
		}

		v.AddConfigPath(currentPath)

		if err := v.ReadInConfig(); err != nil {
			// It's okay if config file doesn't exist
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("error reading config file: %w", err)
			}
		}
	}

	// Read environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Build database config first (needed to load system settings)
	dbConfig := DatabaseConfig{
		Host:     v.GetString("DB_HOST"),
		Port:     v.GetInt("DB_PORT"),
		User:     v.GetString("DB_USER"),
		Password: v.GetString("DB_PASSWORD"),
		DBName:   v.GetString("DB_NAME"),
		Prefix:   v.GetString("DB_PREFIX"),
		SSLMode:  v.GetString("DB_SSLMODE"),
	}

	// SECRET_KEY resolution (CRITICAL for decryption)
	secretKey := v.GetString("SECRET_KEY")
	if secretKey == "" {
		// Fallback for backward compatibility
		secretKey = v.GetString("PASETO_PRIVATE_KEY")
	}
	if secretKey == "" {
		// REQUIRED - fail fast if both are empty
		return nil, fmt.Errorf("SECRET_KEY (or PASETO_PRIVATE_KEY for backward compatibility) must be set")
	}

	// Try to load system settings from database
	var systemSettings *SystemSettings
	var isInstalled bool

	db, err := sql.Open("postgres", getSystemDSN(&dbConfig))
	if err == nil {
		defer db.Close()
		if err := db.Ping(); err == nil {
			// Database is accessible, try to load settings
			systemSettings, err = loadSystemSettings(db, secretKey)
			if err == nil && systemSettings != nil {
				isInstalled = systemSettings.IsInstalled
			}
		}
	}

	// Track env var values from viper (before any database fallbacks are applied)
	// Note: These come from environment variables or .env file, not from defaults or database
	envVals := envValues{
		RootEmail:        v.GetString("ROOT_EMAIL"),
		APIEndpoint:      v.GetString("API_ENDPOINT"),
		PasetoPublicKey:  v.GetString("PASETO_PUBLIC_KEY"),
		PasetoPrivateKey: v.GetString("PASETO_PRIVATE_KEY"),
		SMTPHost:         v.GetString("SMTP_HOST"),
		SMTPPort:         v.GetInt("SMTP_PORT"),
		SMTPUsername:     v.GetString("SMTP_USERNAME"),
		SMTPPassword:     v.GetString("SMTP_PASSWORD"),
		SMTPFromEmail:    v.GetString("SMTP_FROM_EMAIL"),
		SMTPFromName:     v.GetString("SMTP_FROM_NAME"),
	}

	// Get PASETO keys from env vars or database
	var privateKeyBase64, publicKeyBase64 string

	if isInstalled && systemSettings != nil {
		// Prefer env vars, fall back to database
		privateKeyBase64 = envVals.PasetoPrivateKey
		if privateKeyBase64 == "" {
			privateKeyBase64 = systemSettings.PasetoPrivateKey
		}

		publicKeyBase64 = envVals.PasetoPublicKey
		if publicKeyBase64 == "" {
			publicKeyBase64 = systemSettings.PasetoPublicKey
		}
	} else {
		// First-run or database not accessible: use env vars if available
		privateKeyBase64 = envVals.PasetoPrivateKey
		publicKeyBase64 = envVals.PasetoPublicKey
	}

	// Initialize PASETO keys if available (optional for first-run)
	var privateKey paseto.V4AsymmetricSecretKey
	var publicKey paseto.V4AsymmetricPublicKey
	var privateKeyBytes, publicKeyBytes []byte

	if privateKeyBase64 != "" && publicKeyBase64 != "" {
		// Decode base64 keys
		privateKeyBytes, err = base64.StdEncoding.DecodeString(privateKeyBase64)
		if err != nil {
			return nil, fmt.Errorf("error decoding PASETO_PRIVATE_KEY: %w", err)
		}

		publicKeyBytes, err = base64.StdEncoding.DecodeString(publicKeyBase64)
		if err != nil {
			return nil, fmt.Errorf("error decoding PASETO_PUBLIC_KEY: %w", err)
		}

		// Convert bytes to paseto key types
		privateKey, err = paseto.NewV4AsymmetricSecretKeyFromBytes(privateKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("error creating PASETO private key (expected 64 bytes, got %d): %w", len(privateKeyBytes), err)
		}

		publicKey, err = paseto.NewV4AsymmetricPublicKeyFromBytes(publicKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("error creating PASETO public key: %w", err)
		}
	} else {
		// No PASETO keys available - this is OK if system is not installed yet
		// Keys will be loaded on-demand by AuthService after setup completes
		// Leave privateKeyBytes and publicKeyBytes as nil
	}

	// Load config values with database override logic
	var rootEmail, apiEndpoint string
	var smtpConfig SMTPConfig

	if isInstalled && systemSettings != nil {
		// Prefer env vars, fall back to database
		rootEmail = envVals.RootEmail
		if rootEmail == "" {
			rootEmail = systemSettings.RootEmail
		}

		apiEndpoint = envVals.APIEndpoint
		if apiEndpoint == "" {
			apiEndpoint = systemSettings.APIEndpoint
		}

		// SMTP settings - env vars override database
		smtpConfig = SMTPConfig{
			Host:      envVals.SMTPHost,
			Port:      envVals.SMTPPort,
			Username:  envVals.SMTPUsername,
			Password:  envVals.SMTPPassword,
			FromEmail: envVals.SMTPFromEmail,
			FromName:  envVals.SMTPFromName,
		}

		// Use database values as fallback
		if smtpConfig.Host == "" {
			smtpConfig.Host = systemSettings.SMTPHost
		}
		if smtpConfig.Port == 0 {
			smtpConfig.Port = systemSettings.SMTPPort
		}
		if smtpConfig.Port == 0 {
			smtpConfig.Port = 587 // Default
		}
		if smtpConfig.Username == "" {
			smtpConfig.Username = systemSettings.SMTPUsername
		}
		if smtpConfig.Password == "" {
			smtpConfig.Password = systemSettings.SMTPPassword
		}
		if smtpConfig.FromEmail == "" {
			smtpConfig.FromEmail = systemSettings.SMTPFromEmail
		}
		if smtpConfig.FromName == "" {
			smtpConfig.FromName = systemSettings.SMTPFromName
		}
		if smtpConfig.FromName == "" {
			smtpConfig.FromName = "Notifuse" // Default
		}
	} else {
		// First-run: use env vars only
		rootEmail = envVals.RootEmail
		apiEndpoint = envVals.APIEndpoint
		smtpConfig = SMTPConfig{
			Host:      envVals.SMTPHost,
			Port:      envVals.SMTPPort,
			Username:  envVals.SMTPUsername,
			Password:  envVals.SMTPPassword,
			FromEmail: envVals.SMTPFromEmail,
			FromName:  envVals.SMTPFromName,
		}
		// Apply defaults for first-run
		if smtpConfig.Port == 0 {
			smtpConfig.Port = 587
		}
		if smtpConfig.FromName == "" {
			smtpConfig.FromName = "Notifuse"
		}
	}

	config := &Config{
		Server: ServerConfig{
			Port: v.GetInt("SERVER_PORT"),
			Host: v.GetString("SERVER_HOST"),
			SSL: SSLConfig{
				Enabled:  v.GetBool("SSL_ENABLED"),
				CertFile: v.GetString("SSL_CERT_FILE"),
				KeyFile:  v.GetString("SSL_KEY_FILE"),
			},
		},
		Database: dbConfig,
		SMTP:     smtpConfig,
		Security: SecurityConfig{
			PasetoPrivateKey:      privateKey,
			PasetoPublicKey:       publicKey,
			PasetoPrivateKeyBytes: privateKeyBytes,
			PasetoPublicKeyBytes:  publicKeyBytes,
			SecretKey:             secretKey,
		},
		Demo: DemoConfig{
			FileManagerEndpoint:  v.GetString("DEMO_FILE_MANAGER_ENDPOINT"),
			FileManagerBucket:    v.GetString("DEMO_FILE_MANAGER_BUCKET"),
			FileManagerAccessKey: v.GetString("DEMO_FILE_MANAGER_ACCESS_KEY"),
			FileManagerSecretKey: v.GetString("DEMO_FILE_MANAGER_SECRET_KEY"),
		},
		Telemetry: v.GetBool("TELEMETRY"),
		Tracing: TracingConfig{
			Enabled:             v.GetBool("TRACING_ENABLED"),
			ServiceName:         v.GetString("TRACING_SERVICE_NAME"),
			SamplingProbability: v.GetFloat64("TRACING_SAMPLING_PROBABILITY"),

			// Trace exporter configuration
			TraceExporter: v.GetString("TRACING_TRACE_EXPORTER"),

			// Jaeger settings
			JaegerEndpoint: v.GetString("TRACING_JAEGER_ENDPOINT"),

			// Zipkin settings
			ZipkinEndpoint: v.GetString("TRACING_ZIPKIN_ENDPOINT"),

			// Stackdriver settings
			StackdriverProjectID: v.GetString("TRACING_STACKDRIVER_PROJECT_ID"),

			// Azure Monitor settings
			AzureInstrumentationKey: v.GetString("TRACING_AZURE_INSTRUMENTATION_KEY"),

			// Datadog settings
			DatadogAgentAddress: v.GetString("TRACING_DATADOG_AGENT_ADDRESS"),
			DatadogAPIKey:       v.GetString("TRACING_DATADOG_API_KEY"),

			// AWS X-Ray settings
			XRayRegion: v.GetString("TRACING_XRAY_REGION"),

			// General agent endpoint (for exporters that support a common agent)
			AgentEndpoint: v.GetString("TRACING_AGENT_ENDPOINT"),

			// Metrics exporter configuration
			MetricsExporter: v.GetString("TRACING_METRICS_EXPORTER"),
			PrometheusPort:  v.GetInt("TRACING_PROMETHEUS_PORT"),
		},

		RootEmail:       rootEmail,
		Environment:     v.GetString("ENVIRONMENT"),
		APIEndpoint:     apiEndpoint,
		WebhookEndpoint: v.GetString("WEBHOOK_ENDPOINT"),
		LogLevel:        v.GetString("LOG_LEVEL"),
		Version:         v.GetString("VERSION"),
		IsInstalled:     isInstalled,
		envValues:       envVals, // Store env values for setup service
	}

	if config.WebhookEndpoint == "" {
		config.WebhookEndpoint = config.APIEndpoint
	}

	return config, nil
}

// IsDevelopment returns true if the environment is set to development
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

func (c *Config) IsDemo() bool {
	return c.Environment == "demo"
}

func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}

// GetEnvValues returns configuration values that came from actual environment variables
// This is used by the setup service to determine which settings are already configured
func (c *Config) GetEnvValues() (rootEmail, apiEndpoint, pasetoPublicKey, pasetoPrivateKey, smtpHost, smtpUsername, smtpPassword, smtpFromEmail, smtpFromName string, smtpPort int) {
	return c.envValues.RootEmail,
		c.envValues.APIEndpoint,
		c.envValues.PasetoPublicKey,
		c.envValues.PasetoPrivateKey,
		c.envValues.SMTPHost,
		c.envValues.SMTPUsername,
		c.envValues.SMTPPassword,
		c.envValues.SMTPFromEmail,
		c.envValues.SMTPFromName,
		c.envValues.SMTPPort
}
