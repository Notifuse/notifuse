package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"aidanwoods.dev/go-paseto"
	"github.com/spf13/viper"
)

const VERSION = "3.13"

type Config struct {
	Server          ServerConfig
	Database        DatabaseConfig
	Security        SecurityConfig
	Tracing         TracingConfig
	SMTP            SMTPConfig
	Demo            DemoConfig
	Telemetry       bool
	RootEmail       string
	Environment     string
	APIEndpoint     string
	WebhookEndpoint string
	LogLevel        string
	Version         string
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

// LoadOptions contains options for loading configuration
type LoadOptions struct {
	EnvFile string // Optional environment file to load (e.g., ".env", ".env.test")
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
	v.SetDefault("SMTP_PORT", 587)
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

	// Get base64 encoded keys
	privateKeyBase64 := v.GetString("PASETO_PRIVATE_KEY")
	publicKeyBase64 := v.GetString("PASETO_PUBLIC_KEY")

	// Validate required configuration
	if privateKeyBase64 == "" {
		return nil, fmt.Errorf("PASETO_PRIVATE_KEY is required")
	}
	if publicKeyBase64 == "" {
		return nil, fmt.Errorf("PASETO_PUBLIC_KEY is required")
	}

	// Decode base64 keys
	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("error decoding PASETO_PRIVATE_KEY: %w", err)
	}

	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("error decoding PASETO_PUBLIC_KEY: %w", err)
	}

	// Convert bytes to paseto key types
	privateKey, err := paseto.NewV4AsymmetricSecretKeyFromBytes(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("error creating PASETO private key: %w", err)
	}

	publicKey, err := paseto.NewV4AsymmetricPublicKeyFromBytes(publicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("error creating PASETO public key: %w", err)
	}

	// Use PASETO private key as secret key if SECRET_KEY is not provided
	secretKey := v.GetString("SECRET_KEY")
	if secretKey == "" {
		// Use base64 encoded PASETO private key as the secret key
		secretKey = privateKeyBase64
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
		Database: DatabaseConfig{
			Host:     v.GetString("DB_HOST"),
			Port:     v.GetInt("DB_PORT"),
			User:     v.GetString("DB_USER"),
			Password: v.GetString("DB_PASSWORD"),
			DBName:   v.GetString("DB_NAME"),
			Prefix:   v.GetString("DB_PREFIX"),
			SSLMode:  v.GetString("DB_SSLMODE"),
		},
		SMTP: SMTPConfig{
			Host:      v.GetString("SMTP_HOST"),
			Port:      v.GetInt("SMTP_PORT"),
			Username:  v.GetString("SMTP_USERNAME"),
			Password:  v.GetString("SMTP_PASSWORD"),
			FromEmail: v.GetString("SMTP_FROM_EMAIL"),
			FromName:  v.GetString("SMTP_FROM_NAME"),
		},
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

		RootEmail:       v.GetString("ROOT_EMAIL"),
		Environment:     v.GetString("ENVIRONMENT"),
		APIEndpoint:     v.GetString("API_ENDPOINT"),
		WebhookEndpoint: v.GetString("WEBHOOK_ENDPOINT"),
		LogLevel:        v.GetString("LOG_LEVEL"),
		Version:         v.GetString("VERSION"),
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
