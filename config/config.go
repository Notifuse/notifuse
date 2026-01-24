package config

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/spf13/viper"
)

const VERSION = "26.12"

type Config struct {
	Server              ServerConfig
	Database            DatabaseConfig
	Security            SecurityConfig
	Tracing             TracingConfig
	SMTP                SMTPConfig
	SMTPRelay           SMTPRelayConfig
	Demo                DemoConfig
	Broadcast           BroadcastConfig
	TaskScheduler       TaskSchedulerConfig
	AutomationScheduler AutomationSchedulerConfig
	Telemetry           bool
	CheckForUpdates     bool
	RootEmail           string
	Environment         string
	APIEndpoint         string
	WebhookEndpoint     string
	LogLevel            string
	Version             string
	IsInstalled         bool // NEW: Indicates if setup wizard has been completed

	// Track which values came from actual environment variables (not database, not generated)
	EnvValues EnvValues
}

// EnvValues tracks configuration that came from actual environment variables
type EnvValues struct {
	RootEmail              string
	APIEndpoint            string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFromEmail          string
	SMTPFromName           string
	SMTPUseTLS             string // "true", "false", or "" (empty = not set, defaults to true)
	SMTPRelayEnabled       string // "true", "false", or "" (empty = not set, allows setup wizard to configure)
	SMTPRelayDomain        string
	SMTPRelayPort          int
	SMTPRelayTLSCertBase64 string
	SMTPRelayTLSKeyBase64  string
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
	Host                  string
	Port                  int
	User                  string
	Password              string
	DBName                string
	Prefix                string
	SSLMode               string
	MaxConnections        int           // Total max connections across all databases
	MaxConnectionsPerDB   int           // Max connections per individual workspace database
	ConnectionMaxLifetime time.Duration // Maximum lifetime of a connection
	ConnectionMaxIdleTime time.Duration // Maximum idle time before closing
}

type SecurityConfig struct {
	// JWTSecret for token signing (derived from SecretKey)
	JWTSecret []byte

	// SecretKey for DB encryption AND JWT signing
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
	UseTLS    bool
}

type SMTPRelayConfig struct {
	Enabled       bool   // Enable SMTP relay server for receiving emails
	Port          int    // Port to listen on (default: 587)
	Host          string // Host to bind to (default: "0.0.0.0")
	Domain        string // Server domain name for SMTP greeting
	TLSCertBase64 string // Base64 encoded TLS certificate
	TLSKeyBase64  string // Base64 encoded TLS private key
}

type BroadcastConfig struct {
	DefaultRateLimit int // Default rate limit per minute for broadcasts (0 means use service default)
}

type TaskSchedulerConfig struct {
	Enabled  bool          // Enable/disable internal scheduler
	Interval time.Duration // Tick interval (default: 20s)
	MaxTasks int           // Max tasks per execution (default: 100)
}

type AutomationSchedulerConfig struct {
	Delay     time.Duration // Delay before scheduler starts (default: 30s)
	Interval  time.Duration // Polling interval (default: 10s)
	BatchSize int           // Contacts per batch (default: 50)
}

// LoadOptions contains options for loading configuration
type LoadOptions struct {
	EnvFile string // Optional environment file to load (e.g., ".env", ".env.test")
}

// SystemSettings holds configuration loaded from database
type SystemSettings struct {
	IsInstalled            bool
	RootEmail              string
	APIEndpoint            string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFromEmail          string
	SMTPFromName           string
	SMTPUseTLS             bool
	TelemetryEnabled       bool
	CheckForUpdates        bool
	SMTPRelayEnabled       bool
	SMTPRelayDomain        string
	SMTPRelayPort          int
	SMTPRelayTLSCertBase64 string
	SMTPRelayTLSKeyBase64  string
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
		SMTPUseTLS:  true,  // Default to TLS enabled
	}

	// Load all settings from database
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		// If settings table doesn't exist yet, return default settings
		return settings, nil
	}
	defer func() { _ = rows.Close() }()

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

		// Load SMTP settings
		settings.SMTPHost = settingsMap["smtp_host"]
		if port, ok := settingsMap["smtp_port"]; ok && port != "" {
			_, _ = fmt.Sscanf(port, "%d", &settings.SMTPPort)
		}
		settings.SMTPFromEmail = settingsMap["smtp_from_email"]
		settings.SMTPFromName = settingsMap["smtp_from_name"]

		// Load SMTP TLS setting (default to true if not set)
		if smtpUseTLS, ok := settingsMap["smtp_use_tls"]; ok {
			settings.SMTPUseTLS = smtpUseTLS != "false"
		}

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

		// Load telemetry setting
		if telemetry, ok := settingsMap["telemetry_enabled"]; ok {
			settings.TelemetryEnabled = telemetry == "true"
		}

		// Load check for updates setting
		if checkUpdates, ok := settingsMap["check_for_updates"]; ok {
			settings.CheckForUpdates = checkUpdates == "true"
		}

		// Load SMTP Relay settings
		if smtpRelayEnabled, ok := settingsMap["smtp_relay_enabled"]; ok {
			settings.SMTPRelayEnabled = smtpRelayEnabled == "true"
		}

		settings.SMTPRelayDomain = settingsMap["smtp_relay_domain"]
		if port, ok := settingsMap["smtp_relay_port"]; ok && port != "" {
			_, _ = fmt.Sscanf(port, "%d", &settings.SMTPRelayPort)
		}

		// Decrypt SMTP Relay TLS certificate if present
		if encryptedCert, ok := settingsMap["encrypted_smtp_relay_tls_cert_base64"]; ok && encryptedCert != "" {
			if decrypted, err := crypto.DecryptFromHexString(encryptedCert, secretKey); err == nil {
				settings.SMTPRelayTLSCertBase64 = decrypted
			}
		}

		// Decrypt SMTP Relay TLS key if present
		if encryptedKey, ok := settingsMap["encrypted_smtp_relay_tls_key_base64"]; ok && encryptedKey != "" {
			if decrypted, err := crypto.DecryptFromHexString(encryptedKey, secretKey); err == nil {
				settings.SMTPRelayTLSKeyBase64 = decrypted
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
	v.SetDefault("DB_MAX_CONNECTIONS", 100)
	v.SetDefault("DB_MAX_CONNECTIONS_PER_DB", 3)
	v.SetDefault("DB_CONNECTION_MAX_LIFETIME", "10m")
	v.SetDefault("DB_CONNECTION_MAX_IDLE_TIME", "5m")
	v.SetDefault("ENVIRONMENT", "production")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("VERSION", VERSION)

	// SMTP defaults
	v.SetDefault("SMTP_FROM_NAME", "Notifuse")

	// SMTP Relay defaults
	// NOTE: Don't set default for SMTP_RELAY_ENABLED - we need to detect when it's truly unset
	v.SetDefault("SMTP_RELAY_PORT", 587)

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

	// Task scheduler defaults
	v.SetDefault("TASK_SCHEDULER_ENABLED", true)
	v.SetDefault("TASK_SCHEDULER_INTERVAL", "20s")
	v.SetDefault("TASK_SCHEDULER_MAX_TASKS", 100)

	// Automation scheduler defaults
	v.SetDefault("AUTOMATION_SCHEDULER_DELAY", "30s")
	v.SetDefault("AUTOMATION_SCHEDULER_INTERVAL", "10s")
	v.SetDefault("AUTOMATION_SCHEDULER_BATCH_SIZE", 50)

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
		Host:                  v.GetString("DB_HOST"),
		Port:                  v.GetInt("DB_PORT"),
		User:                  v.GetString("DB_USER"),
		Password:              v.GetString("DB_PASSWORD"),
		DBName:                v.GetString("DB_NAME"),
		Prefix:                v.GetString("DB_PREFIX"),
		SSLMode:               v.GetString("DB_SSLMODE"),
		MaxConnections:        v.GetInt("DB_MAX_CONNECTIONS"),
		MaxConnectionsPerDB:   v.GetInt("DB_MAX_CONNECTIONS_PER_DB"),
		ConnectionMaxLifetime: v.GetDuration("DB_CONNECTION_MAX_LIFETIME"),
		ConnectionMaxIdleTime: v.GetDuration("DB_CONNECTION_MAX_IDLE_TIME"),
	}

	// Validate database connection settings
	if dbConfig.MaxConnections < 20 {
		return nil, fmt.Errorf("DB_MAX_CONNECTIONS must be at least 20 (got %d)", dbConfig.MaxConnections)
	}
	if dbConfig.MaxConnections > 10000 {
		return nil, fmt.Errorf("DB_MAX_CONNECTIONS cannot exceed 10000 (got %d)", dbConfig.MaxConnections)
	}
	if dbConfig.MaxConnectionsPerDB < 1 {
		return nil, fmt.Errorf("DB_MAX_CONNECTIONS_PER_DB must be at least 1 (got %d)", dbConfig.MaxConnectionsPerDB)
	}
	if dbConfig.MaxConnectionsPerDB > 50 {
		return nil, fmt.Errorf("DB_MAX_CONNECTIONS_PER_DB cannot exceed 50 (got %d)", dbConfig.MaxConnectionsPerDB)
	}

	// SECRET_KEY resolution (CRITICAL for decryption and JWT signing)
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
		defer func() { _ = db.Close() }()
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
	var smtpUseTLSStr string
	if v.IsSet("SMTP_USE_TLS") {
		smtpUseTLSStr = v.GetString("SMTP_USE_TLS")
	} // else: leave empty string (not set, defaults to true)

	var smtpRelayEnabledStr string
	if v.IsSet("SMTP_RELAY_ENABLED") {
		smtpRelayEnabledStr = v.GetString("SMTP_RELAY_ENABLED")
	} // else: leave empty string (not set)

	envVals := EnvValues{
		RootEmail:              v.GetString("ROOT_EMAIL"),
		APIEndpoint:            v.GetString("API_ENDPOINT"),
		SMTPHost:               v.GetString("SMTP_HOST"),
		SMTPPort:               v.GetInt("SMTP_PORT"),
		SMTPUsername:           v.GetString("SMTP_USERNAME"),
		SMTPPassword:           v.GetString("SMTP_PASSWORD"),
		SMTPFromEmail:          v.GetString("SMTP_FROM_EMAIL"),
		SMTPFromName:           v.GetString("SMTP_FROM_NAME"),
		SMTPUseTLS:             smtpUseTLSStr,       // "true", "false", or "" (empty = not set, defaults to true)
		SMTPRelayEnabled:       smtpRelayEnabledStr, // "true", "false", or "" (empty = not set)
		SMTPRelayDomain:        v.GetString("SMTP_RELAY_DOMAIN"),
		SMTPRelayPort:          v.GetInt("SMTP_RELAY_PORT"),
		SMTPRelayTLSCertBase64: v.GetString("SMTP_RELAY_TLS_CERT_BASE64"),
		SMTPRelayTLSKeyBase64:  v.GetString("SMTP_RELAY_TLS_KEY_BASE64"),
	}

	// Derive JWT secret from SECRET_KEY
	// Try base64 decode first (for PASETO_PRIVATE_KEY compatibility), otherwise use raw bytes
	var jwtSecret []byte
	decoded, err := base64.StdEncoding.DecodeString(secretKey)
	if err == nil && len(decoded) >= 32 {
		// Valid base64-encoded key (likely from PASETO_PRIVATE_KEY backward compatibility)
		jwtSecret = decoded
	} else {
		// Use raw string bytes
		jwtSecret = []byte(secretKey)
	}

	// Warn if secret is less than recommended length
	if len(jwtSecret) < 32 {
		fmt.Fprintf(os.Stderr, "⚠️  WARNING: SECRET_KEY is only %d bytes. For production use, it should be at least 32 bytes (256 bits) for secure JWT signing.\n", len(jwtSecret))
		fmt.Fprintf(os.Stderr, "   Generate a secure key with: openssl rand -base64 32\n")
	}

	// Load config values with database override logic
	var rootEmail, apiEndpoint string
	var smtpConfig SMTPConfig
	var smtpRelayConfig SMTPRelayConfig

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
			UseTLS:    envVals.SMTPUseTLS != "false", // Default to true unless explicitly set to false
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
		// Use database value for TLS if env var is not set
		if envVals.SMTPUseTLS == "" {
			smtpConfig.UseTLS = systemSettings.SMTPUseTLS
		}

		// SMTP Relay settings - env vars override database
		smtpRelayConfig = SMTPRelayConfig{
			Enabled:       envVals.SMTPRelayEnabled == "true",
			Port:          envVals.SMTPRelayPort,
			Host:          "0.0.0.0",
			Domain:        envVals.SMTPRelayDomain,
			TLSCertBase64: envVals.SMTPRelayTLSCertBase64,
			TLSKeyBase64:  envVals.SMTPRelayTLSKeyBase64,
		}

		// Use database values as fallback
		if envVals.SMTPRelayEnabled == "" {
			// Only use DB value if env var is not set (empty string)
			smtpRelayConfig.Enabled = systemSettings.SMTPRelayEnabled
		}
		if smtpRelayConfig.Domain == "" {
			smtpRelayConfig.Domain = systemSettings.SMTPRelayDomain
		}
		if smtpRelayConfig.Port == 0 {
			smtpRelayConfig.Port = systemSettings.SMTPRelayPort
		}
		if smtpRelayConfig.Port == 0 {
			smtpRelayConfig.Port = 587 // Default
		}
		if smtpRelayConfig.TLSCertBase64 == "" {
			smtpRelayConfig.TLSCertBase64 = systemSettings.SMTPRelayTLSCertBase64
		}
		if smtpRelayConfig.TLSKeyBase64 == "" {
			smtpRelayConfig.TLSKeyBase64 = systemSettings.SMTPRelayTLSKeyBase64
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
			UseTLS:    envVals.SMTPUseTLS != "false", // Default to true unless explicitly set to false
		}
		// Apply defaults for first-run
		if smtpConfig.Port == 0 {
			smtpConfig.Port = 587
		}
		if smtpConfig.FromName == "" {
			smtpConfig.FromName = "Notifuse"
		}

		smtpRelayConfig = SMTPRelayConfig{
			Enabled:       envVals.SMTPRelayEnabled == "true",
			Port:          envVals.SMTPRelayPort,
			Host:          "0.0.0.0",
			Domain:        envVals.SMTPRelayDomain,
			TLSCertBase64: envVals.SMTPRelayTLSCertBase64,
			TLSKeyBase64:  envVals.SMTPRelayTLSKeyBase64,
		}
		// Apply defaults for first-run
		if smtpRelayConfig.Port == 0 {
			smtpRelayConfig.Port = 587
		}
		if smtpRelayConfig.Domain == "" {
			smtpRelayConfig.Domain = "localhost"
		}
	}

	// Telemetry and check for updates settings - env var overrides database
	var telemetryEnabled, checkForUpdates bool
	if isInstalled && systemSettings != nil {
		// Check if env var is set (IsSet checks if the key exists, not if it's true)
		if v.IsSet("TELEMETRY") {
			telemetryEnabled = v.GetBool("TELEMETRY")
		} else {
			telemetryEnabled = systemSettings.TelemetryEnabled
		}

		if v.IsSet("CHECK_FOR_UPDATES") {
			checkForUpdates = v.GetBool("CHECK_FOR_UPDATES")
		} else {
			checkForUpdates = systemSettings.CheckForUpdates
		}
	} else {
		// First-run: use env vars only (defaults to false if not set)
		telemetryEnabled = v.GetBool("TELEMETRY")
		checkForUpdates = v.GetBool("CHECK_FOR_UPDATES")
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
		Database:  dbConfig,
		SMTP:      smtpConfig,
		SMTPRelay: smtpRelayConfig,
		Security: SecurityConfig{
			JWTSecret: jwtSecret,
			SecretKey: secretKey,
		},
		Demo: DemoConfig{
			FileManagerEndpoint:  v.GetString("DEMO_FILE_MANAGER_ENDPOINT"),
			FileManagerBucket:    v.GetString("DEMO_FILE_MANAGER_BUCKET"),
			FileManagerAccessKey: v.GetString("DEMO_FILE_MANAGER_ACCESS_KEY"),
			FileManagerSecretKey: v.GetString("DEMO_FILE_MANAGER_SECRET_KEY"),
		},
		Telemetry:       telemetryEnabled,
		CheckForUpdates: checkForUpdates,
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
		Broadcast: BroadcastConfig{
			DefaultRateLimit: v.GetInt("BROADCAST_DEFAULT_RATE_LIMIT"),
		},
		TaskScheduler: TaskSchedulerConfig{
			Enabled:  v.GetBool("TASK_SCHEDULER_ENABLED"),
			Interval: v.GetDuration("TASK_SCHEDULER_INTERVAL"),
			MaxTasks: v.GetInt("TASK_SCHEDULER_MAX_TASKS"),
		},
		AutomationScheduler: AutomationSchedulerConfig{
			Delay:     v.GetDuration("AUTOMATION_SCHEDULER_DELAY"),
			Interval:  v.GetDuration("AUTOMATION_SCHEDULER_INTERVAL"),
			BatchSize: v.GetInt("AUTOMATION_SCHEDULER_BATCH_SIZE"),
		},

		RootEmail:       rootEmail,
		Environment:     v.GetString("ENVIRONMENT"),
		APIEndpoint:     apiEndpoint,
		WebhookEndpoint: v.GetString("WEBHOOK_ENDPOINT"),
		LogLevel:        v.GetString("LOG_LEVEL"),
		Version:         v.GetString("VERSION"),
		IsInstalled:     isInstalled,
		EnvValues:       envVals, // Store env values for setup service
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
func (c *Config) GetEnvValues() (rootEmail, apiEndpoint, smtpHost, smtpUsername, smtpPassword, smtpFromEmail, smtpFromName string, smtpPort int, smtpUseTLS string, smtpRelayEnabled string, smtpRelayDomain, smtpRelayTLSCertBase64, smtpRelayTLSKeyBase64 string, smtpRelayPort int) {
	return c.EnvValues.RootEmail,
		c.EnvValues.APIEndpoint,
		c.EnvValues.SMTPHost,
		c.EnvValues.SMTPUsername,
		c.EnvValues.SMTPPassword,
		c.EnvValues.SMTPFromEmail,
		c.EnvValues.SMTPFromName,
		c.EnvValues.SMTPPort,
		c.EnvValues.SMTPUseTLS,
		c.EnvValues.SMTPRelayEnabled,
		c.EnvValues.SMTPRelayDomain,
		c.EnvValues.SMTPRelayTLSCertBase64,
		c.EnvValues.SMTPRelayTLSKeyBase64,
		c.EnvValues.SMTPRelayPort
}
