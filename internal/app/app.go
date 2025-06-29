package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	httpHandler "github.com/Notifuse/notifuse/internal/http"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/mailer"
	"github.com/Notifuse/notifuse/pkg/tracing"

	"contrib.go.opencensus.io/integrations/ocsql"
)

// AppInterface defines the interface for the App
type AppInterface interface {
	Initialize() error
	Start() error
	Shutdown(ctx context.Context) error

	// Getters for app components accessed in tests
	GetConfig() *config.Config
	GetLogger() logger.Logger
	GetMux() *http.ServeMux
	GetDB() *sql.DB
	GetMailer() mailer.Mailer

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

	// Server status methods
	IsServerCreated() bool
	WaitForServerStart(ctx context.Context) bool

	// Methods for initialization steps
	InitDB() error
	InitMailer() error
	InitTracing() error
	InitRepositories() error
	InitServices() error
	InitHandlers() error
}

// App encapsulates the application dependencies and configuration
type App struct {
	config   *config.Config
	logger   logger.Logger
	db       *sql.DB
	mailer   mailer.Mailer
	eventBus domain.EventBus

	// Repositories
	userRepo                      domain.UserRepository
	workspaceRepo                 domain.WorkspaceRepository
	authRepo                      domain.AuthRepository
	contactRepo                   domain.ContactRepository
	listRepo                      domain.ListRepository
	contactListRepo               domain.ContactListRepository
	templateRepo                  domain.TemplateRepository
	broadcastRepo                 domain.BroadcastRepository
	taskRepo                      domain.TaskRepository
	transactionalNotificationRepo domain.TransactionalNotificationRepository
	messageHistoryRepo            domain.MessageHistoryRepository
	webhookEventRepo              domain.WebhookEventRepository

	// Services
	authService                      *service.AuthService
	userService                      *service.UserService
	workspaceService                 *service.WorkspaceService
	contactService                   *service.ContactService
	listService                      *service.ListService
	contactListService               *service.ContactListService
	templateService                  *service.TemplateService
	emailService                     *service.EmailService
	broadcastService                 *service.BroadcastService
	taskService                      *service.TaskService
	transactionalNotificationService *service.TransactionalNotificationService
	webhookEventService              *service.WebhookEventService
	webhookRegistrationService       *service.WebhookRegistrationService
	messageHistoryService            *service.MessageHistoryService
	notificationCenterService        *service.NotificationCenterService
	demoService                      *service.DemoService
	// providers
	postmarkService  *service.PostmarkService
	mailgunService   *service.MailgunService
	mailjetService   *service.MailjetService
	sparkPostService *service.SparkPostService
	sesService       *service.SESService

	// HTTP handlers
	mux    *http.ServeMux
	server *http.Server

	// Server synchronization
	serverMu      sync.RWMutex
	serverStarted chan struct{}
}

// AppOption defines a functional option for configuring the App
type AppOption func(*App)

// WithMockDB configures the app to use a mock database
func WithMockDB(db *sql.DB) AppOption {
	return func(a *App) {
		a.db = db
	}
}

// WithMockMailer configures the app to use a mock mailer
func WithMockMailer(m mailer.Mailer) AppOption {
	return func(a *App) {
		a.mailer = m
	}
}

// WithLogger sets a custom logger
func WithLogger(logger logger.Logger) AppOption {
	return func(a *App) {
		a.logger = logger
	}
}

// NewApp creates a new application instance
func NewApp(cfg *config.Config, opts ...AppOption) AppInterface {
	app := &App{
		config:        cfg,
		logger:        logger.NewLoggerWithLevel(cfg.LogLevel), // Use configured log level
		mux:           http.NewServeMux(),
		serverStarted: make(chan struct{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(app)
	}

	return app
}

// InitTracing initializes OpenCensus tracing
func (a *App) InitTracing() error {
	tracingConfig := &a.config.Tracing

	if err := tracing.InitTracing(tracingConfig); err != nil {
		return fmt.Errorf("failed to initialize tracing: %w", err)
	}

	if tracingConfig.Enabled {
		exporter := tracingConfig.TraceExporter
		if exporter == "" {
			exporter = "jaeger" // Default
		}

		metricsExporter := tracingConfig.MetricsExporter
		if metricsExporter == "" {
			metricsExporter = "prometheus" // Default
		}

		a.logger.WithField("trace_exporter", exporter).
			WithField("metrics_exporter", metricsExporter).
			WithField("sampling_rate", tracingConfig.SamplingProbability).
			Info("Tracing initialized successfully")
	}

	return nil
}

// InitDB initializes the database connection
func (a *App) InitDB() error {

	password := a.config.Database.Password
	maskedPassword := ""
	if len(password) > 0 {
		maskedPassword = fmt.Sprintf("%c...%c", password[0], password[len(password)-1])
	}
	a.logger.Info(fmt.Sprintf("Connecting to database %s:%d, user %s, sslmode %s, password: %s, dbname: %s", a.config.Database.Host, a.config.Database.Port, a.config.Database.User, a.config.Database.SSLMode, maskedPassword, a.config.Database.DBName))

	// Ensure system database exists
	if err := database.EnsureSystemDatabaseExists(database.GetPostgresDSN(&a.config.Database), a.config.Database.DBName); err != nil {
		a.logger.Error(err.Error())
		return fmt.Errorf("failed to ensure system database exists: %w", err)
	}

	a.logger.Info("System database check completed")

	// If tracing is enabled, wrap the postgres driver
	driverName := "postgres"
	if a.config.Tracing.Enabled {
		var err error
		driverName, err = ocsql.Register(driverName, ocsql.WithAllTraceOptions())
		if err != nil {
			return fmt.Errorf("failed to register opencensus sql driver: %w", err)
		}
		a.logger.Info("Database driver wrapped with OpenCensus tracing")
	}

	// Connect to system database
	db, err := sql.Open(driverName, database.GetSystemDSN(&a.config.Database))
	if err != nil {
		return fmt.Errorf("failed to connect to system database: %w", err)
	}

	// Test database connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping system database: %w", err)
	}

	// Initialize database schema if needed
	if err := database.InitializeDatabase(db, a.config.RootEmail); err != nil {
		db.Close()
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// Set connection pool settings based on environment
	maxOpen, maxIdle, maxLifetime := database.GetConnectionPoolSettings()
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)

	a.db = db
	return nil
}

// InitMailer initializes the mailer service
func (a *App) InitMailer() error {
	// Skip if mailer already set (e.g., by mock)
	if a.mailer != nil {
		return nil
	}

	if a.config.IsDevelopment() {
		// Use console mailer in development
		a.mailer = mailer.NewConsoleMailer()
		a.logger.Info("Using console mailer for development")
	} else {
		// Use SMTP mailer in production
		a.mailer = mailer.NewSMTPMailer(&mailer.Config{
			SMTPHost:     a.config.SMTP.Host,
			SMTPPort:     a.config.SMTP.Port,
			SMTPUsername: a.config.SMTP.Username,
			SMTPPassword: a.config.SMTP.Password,
			FromEmail:    a.config.SMTP.FromEmail,
			FromName:     a.config.SMTP.FromName,
			APIEndpoint:  a.config.APIEndpoint,
		})
		a.logger.Info("Using SMTP mailer for production")
	}

	return nil
}

// InitRepositories initializes all repositories
func (a *App) InitRepositories() error {
	if a.db == nil {
		return fmt.Errorf("database must be initialized before repositories")
	}

	a.userRepo = repository.NewUserRepository(a.db)
	a.taskRepo = repository.NewTaskRepository(a.db)
	a.authRepo = repository.NewSQLAuthRepository(a.db)
	a.workspaceRepo = repository.NewWorkspaceRepository(a.db, &a.config.Database, a.config.Security.SecretKey)
	a.contactRepo = repository.NewContactRepository(a.workspaceRepo)
	a.listRepo = repository.NewListRepository(a.workspaceRepo)
	a.contactListRepo = repository.NewContactListRepository(a.workspaceRepo)
	a.templateRepo = repository.NewTemplateRepository(a.workspaceRepo)
	a.broadcastRepo = repository.NewBroadcastRepository(a.workspaceRepo)
	a.transactionalNotificationRepo = repository.NewTransactionalNotificationRepository(a.workspaceRepo)
	a.messageHistoryRepo = repository.NewMessageHistoryRepository(a.workspaceRepo)
	a.webhookEventRepo = repository.NewWebhookEventRepository(a.workspaceRepo)

	return nil
}

// InitServices initializes all application services
func (a *App) InitServices() error {
	// Initialize event bus first
	a.eventBus = domain.NewInMemoryEventBus()

	// Initialize auth service
	authServiceConfig := service.AuthServiceConfig{
		Repository:          a.authRepo,
		WorkspaceRepository: a.workspaceRepo,
		PrivateKey:          a.config.Security.PasetoPrivateKeyBytes,
		PublicKey:           a.config.Security.PasetoPublicKeyBytes,
	}

	var err error
	a.authService, err = service.NewAuthService(authServiceConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize auth service: %w", err)
	}

	// Initialize user service
	userServiceConfig := service.UserServiceConfig{
		Repository:    a.userRepo,
		AuthService:   a.authService,
		EmailSender:   a.mailer,
		SessionExpiry: 30 * 24 * time.Hour, // 30 days
		IsProduction:  a.config.IsProduction(),
		Logger:        a.logger,
		Tracer:        tracing.GetTracer(),
	}

	a.userService, err = service.NewUserService(userServiceConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize user service: %w", err)
	}

	// Initialize template service
	a.templateService = service.NewTemplateService(
		a.templateRepo,
		a.authService,
		a.logger,
		a.config.APIEndpoint,
	)

	// Initialize contact service
	a.contactService = service.NewContactService(
		a.contactRepo,
		a.workspaceRepo,
		a.authService,
		a.logger,
	)

	// Initialize contact list service
	a.contactListService = service.NewContactListService(
		a.contactListRepo,
		a.workspaceRepo,
		a.authService,
		a.contactRepo,
		a.listRepo,
		a.contactListRepo,
		a.logger,
	)

	// Initialize http client
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Wrap HTTP client with tracing if enabled
	if a.config.Tracing.Enabled {
		httpClient = tracing.WrapHTTPClient(httpClient)
		a.logger.Info("HTTP client wrapped with OpenCensus tracing")
	}

	// Initialize email provider services
	a.postmarkService = service.NewPostmarkService(httpClient, a.authService, a.logger)
	a.mailgunService = service.NewMailgunService(httpClient, a.authService, a.logger, a.config.WebhookEndpoint)
	a.mailjetService = service.NewMailjetService(httpClient, a.authService, a.logger)
	a.sparkPostService = service.NewSparkPostService(httpClient, a.authService, a.logger)
	a.sesService = service.NewSESService(a.authService, a.logger)

	// Initialize email service
	a.emailService = service.NewEmailService(
		a.logger,
		a.authService,
		a.config.Security.SecretKey,
		a.config.IsDemo(),
		a.workspaceRepo,
		a.templateRepo,
		a.templateService,
		a.messageHistoryRepo,
		httpClient,
		a.config.WebhookEndpoint,
		a.config.APIEndpoint,
	)

	// Initialize webhook registration service
	a.webhookRegistrationService = service.NewWebhookRegistrationService(
		a.workspaceRepo,
		a.authService,
		a.postmarkService,
		a.mailgunService,
		a.mailjetService,
		a.sparkPostService,
		a.sesService,
		a.logger,
		a.config.WebhookEndpoint,
	)

	// Initialize list service after webhook registration service
	a.listService = service.NewListService(
		a.listRepo,
		a.workspaceRepo,
		a.contactListRepo,
		a.contactRepo,
		a.authService,
		a.emailService,
		a.logger,
		a.config.APIEndpoint,
	)

	// Initialize workspace service
	a.workspaceService = service.NewWorkspaceService(
		a.workspaceRepo,
		a.userRepo,
		a.logger,
		a.userService,
		a.authService,
		a.mailer,
		a.config,
		a.contactService,
		a.listService,
		a.contactListService,
		a.templateService,
		a.webhookRegistrationService,
		a.config.Security.SecretKey,
	)

	// Initialize task service
	a.taskService = service.NewTaskService(a.taskRepo, a.logger, a.authService, a.config.APIEndpoint)

	// Initialize transactional notification service
	a.transactionalNotificationService = service.NewTransactionalNotificationService(
		a.transactionalNotificationRepo,
		a.messageHistoryRepo,
		a.templateService,
		a.contactService,
		a.emailService,
		a.authService,
		a.logger,
		a.workspaceRepo,
		a.config.APIEndpoint,
	)

	a.webhookEventService = service.NewWebhookEventService(
		a.webhookEventRepo,
		a.authService,
		a.logger,
		a.workspaceRepo,
		a.messageHistoryRepo,
	)

	// Initialize broadcast service
	a.broadcastService = service.NewBroadcastService(
		a.logger,
		a.broadcastRepo,
		a.workspaceRepo,
		a.emailService,
		a.contactRepo,
		a.templateService,
		nil,        // No taskService yet
		a.taskRepo, // Task repository
		a.authService,
		a.eventBus,           // Pass the event bus
		a.messageHistoryRepo, // Message history repository
		a.config.APIEndpoint, // API endpoint for tracking URLs
	)

	// Create broadcast factory with refactored components
	broadcastConfig := broadcast.DefaultConfig()
	broadcastFactory := broadcast.NewFactory(
		a.broadcastRepo,
		a.messageHistoryRepo,
		a.templateRepo,
		a.emailService,
		a.contactRepo,
		a.taskRepo,
		a.workspaceRepo,
		a.logger,
		broadcastConfig,
		a.config.APIEndpoint,
	)

	// Register the broadcast factory with the task service
	broadcastFactory.RegisterWithTaskService(a.taskService)

	// Register task service to listen for broadcast events
	a.taskService.SubscribeToBroadcastEvents(a.eventBus)

	// Set the task service on the broadcast service
	a.broadcastService.SetTaskService(a.taskService)

	// Initialize message history service
	a.messageHistoryService = service.NewMessageHistoryService(a.messageHistoryRepo, a.logger, a.authService)

	// Initialize notification center service
	a.notificationCenterService = service.NewNotificationCenterService(
		a.contactRepo,
		a.workspaceRepo,
		a.listRepo,
		a.logger,
	)

	// Initialize demo service
	a.demoService = service.NewDemoService(
		a.logger,
		a.config,
		a.workspaceService,
		a.userService,
		a.contactService,
		a.listService,
		a.contactListService,
		a.templateService,
		a.emailService,
		a.broadcastService,
		a.taskService,
		a.transactionalNotificationService,
		a.webhookEventService,
		a.webhookRegistrationService,
		a.messageHistoryService,
		a.notificationCenterService,
		a.workspaceRepo,
		a.taskRepo,
	)

	return nil
}

// InitHandlers initializes all HTTP handlers and routes
func (a *App) InitHandlers() error {
	// Create a new ServeMux to avoid route conflicts on restart
	a.mux = http.NewServeMux()

	// Initialize handlers
	userHandler := httpHandler.NewUserHandler(
		a.userService,
		a.workspaceService,
		a.config,
		a.config.Security.PasetoPublicKey,
		a.logger)
	rootHandler := httpHandler.NewRootHandler("console/dist", "notification_center/dist", a.logger, a.config.APIEndpoint)
	workspaceHandler := httpHandler.NewWorkspaceHandler(
		a.workspaceService,
		a.authService,
		a.config.Security.PasetoPublicKey,
		a.logger,
		a.config.Security.SecretKey,
	)
	faviconHandler := httpHandler.NewFaviconHandler()
	contactHandler := httpHandler.NewContactHandler(a.contactService, a.config.Security.PasetoPublicKey, a.logger)
	listHandler := httpHandler.NewListHandler(a.listService, a.config.Security.PasetoPublicKey, a.logger)
	contactListHandler := httpHandler.NewContactListHandler(a.contactListService, a.config.Security.PasetoPublicKey, a.logger)
	templateHandler := httpHandler.NewTemplateHandler(a.templateService, a.config.Security.PasetoPublicKey, a.logger)
	emailHandler := httpHandler.NewEmailHandler(a.emailService, a.config.Security.PasetoPublicKey, a.logger, a.config.Security.SecretKey)
	broadcastHandler := httpHandler.NewBroadcastHandler(a.broadcastService, a.templateService, a.config.Security.PasetoPublicKey, a.logger, a.config.IsDemo())
	taskHandler := httpHandler.NewTaskHandler(
		a.taskService,
		a.config.Security.PasetoPublicKey,
		a.logger,
		a.config.Security.SecretKey,
	)
	transactionalHandler := httpHandler.NewTransactionalNotificationHandler(a.transactionalNotificationService, a.config.Security.PasetoPublicKey, a.logger, a.config.IsDemo())
	webhookEventHandler := httpHandler.NewWebhookEventHandler(a.webhookEventService, a.config.Security.PasetoPublicKey, a.logger)
	webhookRegistrationHandler := httpHandler.NewWebhookRegistrationHandler(a.webhookRegistrationService, a.config.Security.PasetoPublicKey, a.logger)
	messageHistoryHandler := httpHandler.NewMessageHistoryHandler(
		a.messageHistoryService,
		a.authService,
		a.config.Security.PasetoPublicKey,
		a.logger,
	)
	notificationCenterHandler := httpHandler.NewNotificationCenterHandler(
		a.notificationCenterService,
		a.listService,
		a.logger,
	)
	if !a.config.IsProduction() {
		demoHandler := httpHandler.NewDemoHandler(a.demoService, a.logger)
		demoHandler.RegisterRoutes(a.mux)
	}

	// Register routes
	userHandler.RegisterRoutes(a.mux)
	workspaceHandler.RegisterRoutes(a.mux)
	rootHandler.RegisterRoutes(a.mux)
	contactHandler.RegisterRoutes(a.mux)
	listHandler.RegisterRoutes(a.mux)
	contactListHandler.RegisterRoutes(a.mux)
	templateHandler.RegisterRoutes(a.mux)
	emailHandler.RegisterRoutes(a.mux)
	broadcastHandler.RegisterRoutes(a.mux)
	taskHandler.RegisterRoutes(a.mux)
	transactionalHandler.RegisterRoutes(a.mux)
	webhookEventHandler.RegisterRoutes(a.mux)
	webhookRegistrationHandler.RegisterRoutes(a.mux)
	messageHistoryHandler.RegisterRoutes(a.mux)
	notificationCenterHandler.RegisterRoutes(a.mux)
	a.mux.HandleFunc("/api/detect-favicon", faviconHandler.DetectFavicon)

	return nil
}

// Start starts the HTTP server
func (a *App) Start() error {
	// Create server with wrapped handler for CORS and tracing
	var handler http.Handler = a.mux

	// Apply tracing middleware if enabled
	if a.config.Tracing.Enabled {
		handler = middleware.TracingMiddleware(handler)
		a.logger.Info("OpenCensus tracing middleware enabled")
	}

	// Apply CORS middleware
	handler = middleware.CORSMiddleware(handler)

	addr := fmt.Sprintf("%s:%d", a.config.Server.Host, a.config.Server.Port)
	a.logger.WithField("address", addr).Info(fmt.Sprintf("Server starting on %s", addr))

	// Create a fresh notification channel and update the server
	a.serverMu.Lock()
	// Close the existing channel if it exists
	if a.serverStarted != nil {
		close(a.serverStarted)
	}
	a.serverStarted = make(chan struct{})

	// Create the server
	a.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Get a reference to the channel before unlocking
	serverStarted := a.serverStarted
	a.serverMu.Unlock()

	// Signal that the server has been created and is about to start
	close(serverStarted)

	// Start the server based on SSL configuration
	if a.config.Server.SSL.Enabled {
		a.logger.WithField("cert_file", a.config.Server.SSL.CertFile).Info("SSL enabled")
		return a.server.ListenAndServeTLS(a.config.Server.SSL.CertFile, a.config.Server.SSL.KeyFile)
	}

	return a.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (a *App) Shutdown(ctx context.Context) error {
	a.serverMu.RLock()
	server := a.server
	a.serverMu.RUnlock()

	if server != nil {
		if err := server.Shutdown(ctx); err != nil {
			return err
		}
	}

	// Close database connection if it exists
	if a.db != nil {
		// If tracing is enabled, unwrap the driver to properly close tracing
		if a.config.Tracing.Enabled {
			if err := ocsql.RecordStats(a.db, 5*time.Second); err != nil {
				a.logger.WithField("error", err).Error("Failed to record final database stats for tracing")
			}
		}
		a.db.Close()
	}

	return nil
}

// IsServerCreated safely checks if the server has been created
func (a *App) IsServerCreated() bool {
	a.serverMu.RLock()
	defer a.serverMu.RUnlock()
	return a.server != nil
}

// WaitForServerStart waits for the server to be created and initialized
// Returns true if the server started successfully, false if context expired
func (a *App) WaitForServerStart(ctx context.Context) bool {
	// Get the current channel under lock
	a.serverMu.RLock()
	started := a.serverStarted
	a.serverMu.RUnlock()

	// If the channel is nil, that's a logic error - just wait on the context
	if started == nil {
		a.logger.Error("serverStarted channel is nil - server initialization error")
		select {
		case <-ctx.Done():
			return false
		}
	}

	// Wait for signal or timeout
	select {
	case <-started:
		return a.IsServerCreated() // Double-check server was created
	case <-ctx.Done():
		return false
	}
}

// Initialize sets up all components of the application
func (a *App) Initialize() error {
	if err := a.InitTracing(); err != nil {
		return err
	}

	if err := a.InitDB(); err != nil {
		return err
	}

	if err := a.InitMailer(); err != nil {
		return err
	}

	if err := a.InitRepositories(); err != nil {
		return err
	}

	if err := a.InitServices(); err != nil {
		return err
	}

	if err := a.InitHandlers(); err != nil {
		return err
	}

	a.logger.Info("Application successfully initialized")

	return nil
}

// GetConfig returns the app's configuration
func (a *App) GetConfig() *config.Config {
	return a.config
}

// GetLogger returns the app's logger
func (a *App) GetLogger() logger.Logger {
	return a.logger
}

// GetMux returns the app's HTTP multiplexer
func (a *App) GetMux() *http.ServeMux {
	return a.mux
}

// GetDB returns the app's database connection
func (a *App) GetDB() *sql.DB {
	return a.db
}

// GetMailer returns the app's mailer
func (a *App) GetMailer() mailer.Mailer {
	return a.mailer
}

// Repository getters for testing
func (a *App) GetUserRepository() domain.UserRepository {
	return a.userRepo
}

func (a *App) GetWorkspaceRepository() domain.WorkspaceRepository {
	return a.workspaceRepo
}

func (a *App) GetContactRepository() domain.ContactRepository {
	return a.contactRepo
}

func (a *App) GetListRepository() domain.ListRepository {
	return a.listRepo
}

func (a *App) GetTemplateRepository() domain.TemplateRepository {
	return a.templateRepo
}

func (a *App) GetBroadcastRepository() domain.BroadcastRepository {
	return a.broadcastRepo
}

func (a *App) GetMessageHistoryRepository() domain.MessageHistoryRepository {
	return a.messageHistoryRepo
}

func (a *App) GetContactListRepository() domain.ContactListRepository {
	return a.contactListRepo
}

func (a *App) GetTransactionalNotificationRepository() domain.TransactionalNotificationRepository {
	return a.transactionalNotificationRepo
}

// SetHandler allows setting a custom HTTP handler
func (a *App) SetHandler(handler http.Handler) {
	a.mux = handler.(*http.ServeMux)
}

// Ensure App implements AppInterface
var _ AppInterface = (*App)(nil)
