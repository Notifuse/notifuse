package testutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/testkeys"
)

// ServerManager manages test server lifecycle
type ServerManager struct {
	app       AppInterface
	server    *http.Server
	url       string
	listener  net.Listener
	isStarted bool
	config    *config.Config
	dbManager *DatabaseManager
}

// AppInterface defines the interface for the App (to avoid circular imports)
type AppInterface interface {
	Initialize() error
	Start() error
	Shutdown(ctx context.Context) error
	GetConfig() *config.Config
	GetLogger() logger.Logger
	GetMux() *http.ServeMux

	// Repository getters for testing
	GetUserRepository() domain.UserRepository
	GetWorkspaceRepository() domain.WorkspaceRepository
	GetContactRepository() domain.ContactRepository
	GetListRepository() domain.ListRepository
	GetTemplateRepository() domain.TemplateRepository
	GetBroadcastRepository() domain.BroadcastRepository
	GetMessageHistoryRepository() domain.MessageHistoryRepository
	GetContactListRepository() domain.ContactListRepository
	GetTransactionalNotificationRepository() domain.TransactionalNotificationRepository
}

// NewServerManager creates a new server manager for testing
func NewServerManager(appFactory func(*config.Config) AppInterface, dbManager *DatabaseManager) *ServerManager {
	// Get test keys from pkg/testkeys
	keys, err := testkeys.GetHardcodedTestKeys()
	if err != nil {
		panic(fmt.Sprintf("Failed to get test keys: %v", err))
	}

	// Create PASETO keys
	privateKey, err := paseto.NewV4AsymmetricSecretKeyFromBytes(keys.PrivateKeyBytes)
	if err != nil {
		panic(fmt.Sprintf("Failed to create PASETO private key: %v", err))
	}

	publicKey, err := paseto.NewV4AsymmetricPublicKeyFromBytes(keys.PublicKeyBytes)
	if err != nil {
		panic(fmt.Sprintf("Failed to create PASETO public key: %v", err))
	}

	// Create test configuration
	cfg := &config.Config{
		Environment: "test",
		RootEmail:   "test@example.com",
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0, // Use random available port
		},
		Database: *dbManager.GetConfig(),
		Security: config.SecurityConfig{
			SecretKey:             "test-secret-key-for-integration-tests-only",
			PasetoPrivateKey:      privateKey,
			PasetoPublicKey:       publicKey,
			PasetoPrivateKeyBytes: keys.PrivateKeyBytes,
			PasetoPublicKeyBytes:  keys.PublicKeyBytes,
		},
		SMTP: config.SMTPConfig{
			Host:      "localhost",
			Port:      1025,
			FromEmail: "test@example.com",
			FromName:  "Test Notifuse",
		},
		Tracing: config.TracingConfig{
			Enabled: false,
		},
	}

	app := appFactory(cfg)

	return &ServerManager{
		app:       app,
		config:    cfg,
		dbManager: dbManager,
	}
}

// Start starts the test server
func (sm *ServerManager) Start() error {
	if sm.isStarted {
		return nil
	}

	// Initialize the app
	if err := sm.app.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}

	// Create listener on random port
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", sm.config.Server.Host))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	sm.listener = listener

	// Get the actual port
	port := listener.Addr().(*net.TCPAddr).Port
	sm.url = fmt.Sprintf("http://%s:%d", sm.config.Server.Host, port)

	// Create HTTP server
	sm.server = &http.Server{
		Handler:      sm.app.GetMux(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in background
	go func() {
		if err := sm.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			sm.app.GetLogger().WithField("error", err.Error()).Error("Server error")
		}
	}()

	// Wait for server to be ready
	if err := sm.waitForReady(10 * time.Second); err != nil {
		return fmt.Errorf("server not ready: %w", err)
	}

	sm.isStarted = true
	return nil
}

// Stop stops the test server
func (sm *ServerManager) Stop() error {
	if !sm.isStarted {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sm.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	if sm.listener != nil {
		sm.listener.Close()
	}

	sm.isStarted = false
	return nil
}

// GetURL returns the server URL
func (sm *ServerManager) GetURL() string {
	return sm.url
}

// IsStarted returns whether the server is started
func (sm *ServerManager) IsStarted() bool {
	return sm.isStarted
}

// GetApp returns the app instance
func (sm *ServerManager) GetApp() AppInterface {
	return sm.app
}

// waitForReady waits for the server to be ready to accept requests
func (sm *ServerManager) waitForReady(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for server to be ready")
		case <-ticker.C:
			// Try to make a request to the health endpoint
			resp, err := client.Get(sm.url + "/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 500 {
					return nil
				}
			}
		}
	}
}

// WaitForReady waits for the server to be ready with custom timeout
func (sm *ServerManager) WaitForReady(timeout time.Duration) error {
	if !sm.isStarted {
		return fmt.Errorf("server not started")
	}
	return sm.waitForReady(timeout)
}

// Restart stops and starts the server
func (sm *ServerManager) Restart() error {
	if err := sm.Stop(); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	time.Sleep(100 * time.Millisecond) // Brief pause

	if err := sm.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}
