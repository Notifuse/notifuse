package testutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/internal/service/queue"
	"github.com/Notifuse/notifuse/pkg/logger"
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
	GetEmailQueueRepository() domain.EmailQueueRepository

	// Service getters for testing
	GetAuthService() interface{} // Returns *service.AuthService but defined as interface{} to avoid import cycle
	GetTransactionalNotificationService() domain.TransactionalNotificationService
	GetEmailQueueWorker() *queue.EmailQueueWorker
	GetAutomationScheduler() *service.AutomationScheduler
}

// NewServerManager creates a new server manager for testing
func NewServerManager(appFactory func(*config.Config) AppInterface, dbManager *DatabaseManager) *ServerManager {
	// Create test JWT secret (32+ bytes for HS256)
	jwtSecret := []byte("test-jwt-secret-key-for-integration-tests-only-32bytes")

	// Create test configuration
	// Use "development" to enable features like returning invitation tokens in responses
	cfg := &config.Config{
		Environment: "development",
		RootEmail:   "test@example.com",
		APIEndpoint: "",   // Empty to trigger direct task execution instead of HTTP callbacks
		IsInstalled: true, // Mark as installed for tests
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0, // Use random available port
		},
		Database: *dbManager.GetConfig(),
		Security: config.SecurityConfig{
			SecretKey: "test-secret-key-for-integration-tests-only",
			JWTSecret: jwtSecret,
		},
		SMTP: config.SMTPConfig{
			Host:      "localhost",
			Port:      1025,
			FromEmail: "test@example.com",
			FromName:  "Test Notifuse",
		},
		Broadcast: config.BroadcastConfig{
			DefaultRateLimit: 6000, // 6000 per minute = 100 per second (no rate limiting for tests)
		},
		TaskScheduler: config.TaskSchedulerConfig{
			Enabled: false, // Disable task scheduler and autoExecuteImmediate to prevent background goroutines
		},
		AutomationScheduler: config.AutomationSchedulerConfig{
			Delay:     0,                      // No delay for tests
			Interval:  500 * time.Millisecond, // Fast polling for tests
			BatchSize: 50,
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

	// Note: We intentionally do NOT update the API endpoint in tests
	// This keeps it empty, which triggers direct task execution instead of HTTP callbacks
	// Direct execution is faster and more reliable for tests

	sm.isStarted = true
	return nil
}

// StartBackgroundWorkers starts the email queue worker and other background services
// Call this after Start() when you need workers to process queued items
func (sm *ServerManager) StartBackgroundWorkers(ctx context.Context) error {
	worker := sm.app.GetEmailQueueWorker()
	if worker != nil {
		if err := worker.Start(ctx); err != nil {
			return fmt.Errorf("failed to start email queue worker: %w", err)
		}
	}

	// Start automation scheduler for tests (no delay since config has Delay=0)
	// Note: Start() spawns its own goroutine internally, no need for `go`
	if scheduler := sm.app.GetAutomationScheduler(); scheduler != nil {
		scheduler.Start(ctx)
	}

	return nil
}

// Stop stops the test server
func (sm *ServerManager) Stop() error {
	if !sm.isStarted {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First, shutdown the app (closes database connections, etc.)
	if err := sm.app.Shutdown(ctx); err != nil {
		sm.app.GetLogger().WithField("error", err.Error()).Warn("Failed to shutdown app gracefully")
	}

	// Then shutdown the HTTP server
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
