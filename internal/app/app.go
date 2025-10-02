package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	httpHandler "github.com/Notifuse/notifuse/internal/http"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/migrations"
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
	GetTelemetryRepository() domain.TelemetryRepository

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

	// Graceful shutdown methods
	SetShutdownTimeout(timeout time.Duration)
	GetActiveRequestCount() int64
	GetShutdownContext() context.Context
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
	settingRepo                   domain.SettingRepository
	contactRepo                   domain.ContactRepository
	listRepo                      domain.ListRepository
	contactListRepo               domain.ContactListRepository
	templateRepo                  domain.TemplateRepository
	broadcastRepo                 domain.BroadcastRepository
	taskRepo                      domain.TaskRepository
	transactionalNotificationRepo domain.TransactionalNotificationRepository
	messageHistoryRepo            domain.MessageHistoryRepository
	webhookEventRepo              domain.WebhookEventRepository
	telemetryRepo                 domain.TelemetryRepository
	analyticsRepo                 domain.AnalyticsRepository
	contactTimelineRepo           domain.ContactTimelineRepository

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
	systemNotificationService        *service.SystemNotificationService
	webhookEventService              *service.WebhookEventService
	webhookRegistrationService       *service.WebhookRegistrationService
	messageHistoryService            *service.MessageHistoryService
	notificationCenterService        *service.NotificationCenterService
	demoService                      *service.DemoService
	telemetryService                 *service.TelemetryService
	analyticsService                 *service.AnalyticsService
	contactTimelineService           domain.ContactTimelineService
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

	// Graceful shutdown management
	shutdownCtx     context.Context
	shutdownCancel  context.CancelFunc
	activeRequests  int64          // atomic counter for active HTTP requests
	requestWg       sync.WaitGroup // wait group for active requests
	shutdownTimeout time.Duration  // configurable shutdown timeout
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
	// Create shutdown context
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	app := &App{
		config:          cfg,
		logger:          logger.NewLoggerWithLevel(cfg.LogLevel), // Use configured log level
		mux:             http.NewServeMux(),
		serverStarted:   make(chan struct{}),
		shutdownCtx:     shutdownCtx,
		shutdownCancel:  shutdownCancel,
		shutdownTimeout: 60 * time.Second, // Default 60 seconds shutdown timeout (5 seconds buffer for 55-second tasks)
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

	// Run migrations separately
	migrationManager := migrations.NewManager(a.logger)
	ctx := context.Background()
	if err := migrationManager.RunMigrations(ctx, a.config, db); err != nil {
		db.Close()
		return fmt.Errorf("failed to run migrations: %w", err)
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
	a.settingRepo = repository.NewSQLSettingRepository(a.db)
	a.workspaceRepo = repository.NewWorkspaceRepository(a.db, &a.config.Database, a.config.Security.SecretKey)
	a.contactRepo = repository.NewContactRepository(a.workspaceRepo)
	a.listRepo = repository.NewListRepository(a.workspaceRepo)
	a.contactListRepo = repository.NewContactListRepository(a.workspaceRepo)
	a.templateRepo = repository.NewTemplateRepository(a.workspaceRepo)
	a.broadcastRepo = repository.NewBroadcastRepository(a.workspaceRepo)
	a.transactionalNotificationRepo = repository.NewTransactionalNotificationRepository(a.workspaceRepo)
	a.messageHistoryRepo = repository.NewMessageHistoryRepository(a.workspaceRepo)
	a.webhookEventRepo = repository.NewWebhookEventRepository(a.workspaceRepo)
	a.telemetryRepo = repository.NewTelemetryRepository(a.workspaceRepo)
	a.analyticsRepo = repository.NewAnalyticsRepository(a.workspaceRepo, a.logger)
	a.contactTimelineRepo = repository.NewContactTimelineRepository(a.workspaceRepo)

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
		a.messageHistoryRepo,
		a.webhookEventRepo,
		a.contactListRepo,
		a.contactTimelineRepo,
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
	a.taskService = service.NewTaskService(a.taskRepo, a.settingRepo, a.logger, a.authService, a.config.APIEndpoint)

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
		a.eventBus,
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

	// Initialize system notification service
	a.systemNotificationService = service.NewSystemNotificationService(
		a.workspaceRepo,
		a.broadcastRepo,
		a.mailer,
		a.logger,
	)

	// Register system notification service with event bus
	a.systemNotificationService.RegisterWithEventBus(a.eventBus)

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
		a.messageHistoryRepo,
		a.webhookEventRepo,
	)

	// Initialize telemetry service
	telemetryConfig := service.TelemetryServiceConfig{
		Enabled:       a.config.Telemetry,
		APIEndpoint:   a.config.APIEndpoint,
		WorkspaceRepo: a.workspaceRepo,
		TelemetryRepo: a.telemetryRepo,
		Logger:        a.logger,
		HTTPClient:    httpClient, // Reuse the HTTP client created above
	}
	a.telemetryService = service.NewTelemetryService(telemetryConfig)

	// Initialize analytics service
	a.analyticsService = service.NewAnalyticsService(
		a.analyticsRepo,
		a.authService,
		a.logger,
	)

	// Initialize contact timeline service
	a.contactTimelineService = service.NewContactTimelineService(a.contactTimelineRepo)

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
	rootHandler := httpHandler.NewRootHandler("console/dist", "notification_center/dist", a.logger, a.config.APIEndpoint, a.config.Version, a.config.RootEmail)
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
	analyticsHandler := httpHandler.NewAnalyticsHandler(
		a.analyticsService,
		a.config.Security.PasetoPublicKey,
		a.logger,
	)
	contactTimelineHandler := httpHandler.NewContactTimelineHandler(
		a.contactTimelineService,
		a.authService,
		a.config.Security.PasetoPublicKey,
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
	analyticsHandler.RegisterRoutes(a.mux)
	contactTimelineHandler.RegisterRoutes(a.mux)
	a.mux.HandleFunc("/api/detect-favicon", faviconHandler.DetectFavicon)

	return nil
}

// Start starts the HTTP server
func (a *App) Start() error {
	// Create server with wrapped handler for CORS and tracing
	var handler http.Handler = a.mux

	// Apply graceful shutdown middleware first (outermost)
	handler = a.gracefulShutdownMiddleware(handler)
	a.logger.Info("Graceful shutdown middleware enabled")

	// Apply tracing middleware if enabled
	if a.config.Tracing.Enabled {
		handler = middleware.TracingMiddleware(handler)
		a.logger.Info("OpenCensus tracing middleware enabled")
	}

	// Apply CORS middleware
	handler = middleware.CORSMiddleware(handler)

	addr := fmt.Sprintf("%s:%d", a.config.Server.Host, a.config.Server.Port)
	a.logger.WithField("address", addr).
		WithField("api_endpoint", a.config.APIEndpoint).
		WithField("port", a.config.Server.Port).
		Info(fmt.Sprintf("Server starting on %s with API endpoint: %s", addr, a.config.APIEndpoint))

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

	// Start daily telemetry scheduler
	if a.telemetryService != nil {
		ctx := context.Background()
		a.telemetryService.StartDailyScheduler(ctx)
	}

	// Start the server based on SSL configuration
	if a.config.Server.SSL.Enabled {
		a.logger.WithField("cert_file", a.config.Server.SSL.CertFile).Info("SSL enabled")
		return a.server.ListenAndServeTLS(a.config.Server.SSL.CertFile, a.config.Server.SSL.KeyFile)
	}

	return a.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("Starting graceful shutdown...")

	// Signal shutdown to all components
	a.shutdownCancel()

	// Get server reference
	a.serverMu.RLock()
	server := a.server
	a.serverMu.RUnlock()

	if server == nil {
		a.logger.Info("No server to shutdown")
		return a.cleanupResources(ctx)
	}

	// Log current active requests
	activeCount := a.getActiveRequestCount()
	a.logger.WithField("active_requests", activeCount).Info("Active requests at shutdown start")

	// Create a timeout context for shutdown operations
	shutdownTimeout := a.shutdownTimeout
	if deadline, ok := ctx.Deadline(); ok {
		// Use the provided context deadline if it's sooner than our default timeout
		if remaining := time.Until(deadline); remaining < shutdownTimeout {
			shutdownTimeout = remaining - time.Second // Leave 1 second buffer
			if shutdownTimeout < 0 {
				shutdownTimeout = 0
			}
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// Start HTTP server shutdown in a goroutine
	serverShutdownDone := make(chan error, 1)
	go func() {
		a.logger.WithField("timeout", shutdownTimeout).Info("Starting HTTP server shutdown")
		serverShutdownDone <- server.Shutdown(shutdownCtx)
	}()

	// Wait for active requests to complete in another goroutine
	requestsDone := make(chan struct{}, 1)
	go func() {
		defer close(requestsDone)

		// Wait for all active requests to complete
		a.logger.Info("Waiting for active requests to complete...")
		done := make(chan struct{})

		go func() {
			a.requestWg.Wait()
			close(done)
		}()

		// Monitor progress
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				a.logger.Info("All requests completed")
				return
			case <-ticker.C:
				activeCount := a.getActiveRequestCount()
				a.logger.WithField("active_requests", activeCount).Info("Still waiting for requests to complete...")
			case <-shutdownCtx.Done():
				activeCount := a.getActiveRequestCount()
				a.logger.WithField("active_requests", activeCount).Warn("Shutdown timeout reached, forcing shutdown")
				return
			}
		}
	}()

	// Wait for both server shutdown and requests to complete
	var shutdownErr error

	select {
	case err := <-serverShutdownDone:
		shutdownErr = err
		a.logger.Info("HTTP server shutdown completed")
	case <-shutdownCtx.Done():
		a.logger.Warn("Shutdown timeout reached")
		shutdownErr = fmt.Errorf("shutdown timeout exceeded")
	}

	// Wait a bit more for requests to finish if server shutdown completed quickly
	if shutdownErr == nil {
		select {
		case <-requestsDone:
			// All requests completed
		case <-time.After(2 * time.Second):
			// Give up after 2 more seconds
			activeCount := a.getActiveRequestCount()
			if activeCount > 0 {
				a.logger.WithField("active_requests", activeCount).Warn("Some requests still active, proceeding with shutdown")
			}
		}
	}

	// Cleanup resources
	if cleanupErr := a.cleanupResources(ctx); cleanupErr != nil {
		a.logger.WithField("error", cleanupErr).Error("Error during resource cleanup")
		if shutdownErr == nil {
			shutdownErr = cleanupErr
		}
	}

	if shutdownErr != nil {
		a.logger.WithField("error", shutdownErr).Error("Graceful shutdown completed with errors")
	} else {
		a.logger.Info("Graceful shutdown completed successfully")
	}

	return shutdownErr
}

// cleanupResources handles cleanup of database and other resources
func (a *App) cleanupResources(ctx context.Context) error {
	a.logger.Info("Cleaning up resources...")

	// Close database connection if it exists
	if a.db != nil {
		// If tracing is enabled, record final stats
		if a.config.Tracing.Enabled {
			if err := ocsql.RecordStats(a.db, 5*time.Second); err != nil {
				a.logger.WithField("error", err).Error("Failed to record final database stats for tracing")
			}
		}

		a.logger.Info("Closing database connection")
		if err := a.db.Close(); err != nil {
			a.logger.WithField("error", err).Error("Error closing database connection")
			return err
		}
	}

	// Stop telemetry service if it exists
	if a.telemetryService != nil {
		a.logger.Info("Stopping telemetry service")
		// The telemetry service should respect context cancellation
	}

	a.logger.Info("Resource cleanup completed")
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
		<-ctx.Done()
		return false
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
	a.logger.WithField("version", a.config.Version).Info("Starting Notifuse application")

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

	// Send startup telemetry metrics
	if a.telemetryService != nil {
		go func() {
			ctx := context.Background()
			if err := a.telemetryService.SendMetricsForAllWorkspaces(ctx); err != nil {
				a.logger.WithField("error", err).Error("Failed to send startup telemetry metrics")
			}
		}()
	}

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

func (a *App) GetTelemetryRepository() domain.TelemetryRepository {
	return a.telemetryRepo
}

// SetHandler allows setting a custom HTTP handler
func (a *App) SetHandler(handler http.Handler) {
	a.mux = handler.(*http.ServeMux)
}

// incrementActiveRequests atomically increments the active request counter
func (a *App) incrementActiveRequests() {
	atomic.AddInt64(&a.activeRequests, 1)
	a.requestWg.Add(1)
}

// decrementActiveRequests atomically decrements the active request counter
func (a *App) decrementActiveRequests() {
	atomic.AddInt64(&a.activeRequests, -1)
	a.requestWg.Done()
}

// getActiveRequestCount returns the current number of active requests
func (a *App) getActiveRequestCount() int64 {
	return atomic.LoadInt64(&a.activeRequests)
}

// GetActiveRequestCount returns the current number of active requests (public interface method)
func (a *App) GetActiveRequestCount() int64 {
	return a.getActiveRequestCount()
}

// SetShutdownTimeout sets the timeout for graceful shutdown
func (a *App) SetShutdownTimeout(timeout time.Duration) {
	a.shutdownTimeout = timeout
	a.logger.WithField("shutdown_timeout", timeout).Info("Shutdown timeout configured")
}

// GetShutdownContext returns the shutdown context for components that need to watch for shutdown
func (a *App) GetShutdownContext() context.Context {
	return a.shutdownCtx
}

// isShuttingDown returns true if the application is in shutdown mode
func (a *App) isShuttingDown() bool {
	select {
	case <-a.shutdownCtx.Done():
		return true
	default:
		return false
	}
}

// gracefulShutdownMiddleware wraps HTTP handlers to track active requests
func (a *App) gracefulShutdownMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if we're shutting down
		if a.isShuttingDown() {
			// Return 503 Service Unavailable if we're shutting down
			http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
			return
		}

		// Track this request
		a.incrementActiveRequests()
		defer a.decrementActiveRequests()

		// Add shutdown context to request context
		ctx := r.Context()
		ctx = context.WithValue(ctx, "shutdown_ctx", a.shutdownCtx)
		r = r.WithContext(ctx)

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// Ensure App implements AppInterface
var _ AppInterface = (*App)(nil)
