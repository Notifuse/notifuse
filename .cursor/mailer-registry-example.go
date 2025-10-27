package example

// This is a reference implementation showing how to use a registry pattern
// to avoid forgetting to update mailer references in services.

import (
	"github.com/Notifuse/notifuse/pkg/mailer"
)

// MailerConsumer is implemented by any service that uses a mailer
type MailerConsumer interface {
	SetMailer(mailer.Mailer)
}

// MailerRegistry manages all services that need mailer updates
type MailerRegistry struct {
	consumers []MailerConsumer
}

// NewMailerRegistry creates a new registry
func NewMailerRegistry() *MailerRegistry {
	return &MailerRegistry{
		consumers: make([]MailerConsumer, 0),
	}
}

// Register adds a service to the registry
func (r *MailerRegistry) Register(consumer MailerConsumer) {
	r.consumers = append(r.consumers, consumer)
}

// UpdateAll updates the mailer for all registered services
func (r *MailerRegistry) UpdateAll(m mailer.Mailer) {
	for _, consumer := range r.consumers {
		consumer.SetMailer(m)
	}
}

// Count returns the number of registered consumers
func (r *MailerRegistry) Count() int {
	return len(r.consumers)
}

// Example: How to use in App

/*
type App struct {
    config  *config.Config
    logger  logger.Logger
    mailer  mailer.Mailer
    
    // Services
    userService             *service.UserService
    workspaceService        *service.WorkspaceService
    systemNotificationService *service.SystemNotificationService
    
    // Registry for mailer updates
    mailerRegistry *MailerRegistry
}

func (a *App) InitServices() error {
    // Create registry
    a.mailerRegistry = NewMailerRegistry()
    
    // Initialize services
    a.userService = service.NewUserService(...)
    a.workspaceService = service.NewWorkspaceService(...)
    a.systemNotificationService = service.NewSystemNotificationService(...)
    
    // Register all services that need mailer updates
    // Compile-time error if service doesn't implement SetMailer!
    a.mailerRegistry.Register(mailerConsumerAdapter{a.userService})
    a.mailerRegistry.Register(a.workspaceService)
    a.mailerRegistry.Register(a.systemNotificationService)
    
    return nil
}

func (a *App) ReloadConfig(ctx context.Context) error {
    a.logger.Info("Reloading configuration from database...")
    
    if err := a.config.ReloadDatabaseSettings(); err != nil {
        return fmt.Errorf("failed to reload database settings: %w", err)
    }
    
    a.isInstalled = a.config.IsInstalled
    
    // Reinitialize mailer with new SMTP settings
    if err := a.InitMailer(); err != nil {
        return fmt.Errorf("failed to reinitialize mailer: %w", err)
    }
    
    // Single call updates ALL services - no way to forget!
    a.mailerRegistry.UpdateAll(a.mailer)
    
    a.authService.InvalidateKeyCache()
    
    a.logger.Info("Configuration reloaded successfully")
    return nil
}

// Adapter pattern if service uses different method name
type mailerConsumerAdapter struct {
    service interface{ SetEmailSender(mailer.Mailer) }
}

func (a mailerConsumerAdapter) SetMailer(m mailer.Mailer) {
    a.service.SetEmailSender(m)
}
*/

// Alternative: Type-safe registry with generics (Go 1.18+)

/*
type TypedMailerRegistry[T MailerConsumer] struct {
    consumers []T
}

func NewTypedMailerRegistry[T MailerConsumer]() *TypedMailerRegistry[T] {
    return &TypedMailerRegistry[T]{
        consumers: make([]T, 0),
    }
}

func (r *TypedMailerRegistry[T]) Register(consumer T) {
    r.consumers = append(r.consumers, consumer)
}

func (r *TypedMailerRegistry[T]) UpdateAll(m mailer.Mailer) {
    for _, consumer := range r.consumers {
        consumer.SetMailer(m)
    }
}
*/

// Example test to verify all services are registered

/*
func TestMailerRegistry_AllServicesRegistered(t *testing.T) {
    app := setupTestApp(t)
    
    // Count how many services should have mailer
    expectedCount := 3  // UserService, WorkspaceService, SystemNotificationService
    
    actualCount := app.mailerRegistry.Count()
    
    assert.Equal(t, expectedCount, actualCount,
        "All services using mailer must be registered. "+
        "If you added a new service using mailer, register it in InitServices()")
}
*/
