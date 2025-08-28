package broadcast

import (
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Factory creates and wires together all the broadcast components
type Factory struct {
	broadcastRepo      domain.BroadcastRepository
	messageHistoryRepo domain.MessageHistoryRepository
	templateRepo       domain.TemplateRepository
	emailService       domain.EmailServiceInterface
	contactRepo        domain.ContactRepository
	taskRepo           domain.TaskRepository
	workspaceRepo      domain.WorkspaceRepository
	logger             logger.Logger
	config             *Config
	apiEndpoint        string
}

// NewFactory creates a new factory for broadcast components
func NewFactory(
	broadcastRepo domain.BroadcastRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	templateRepo domain.TemplateRepository,
	emailService domain.EmailServiceInterface,
	contactRepo domain.ContactRepository,
	taskRepo domain.TaskRepository,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
	config *Config,
	apiEndpoint string,
) *Factory {
	if config == nil {
		config = DefaultConfig()
	}

	return &Factory{
		broadcastRepo:      broadcastRepo,
		messageHistoryRepo: messageHistoryRepo,
		templateRepo:       templateRepo,
		emailService:       emailService,
		contactRepo:        contactRepo,
		taskRepo:           taskRepo,
		workspaceRepo:      workspaceRepo,
		logger:             logger,
		config:             config,
		apiEndpoint:        apiEndpoint,
	}
}

// CreateMessageSender creates a new message sender
func (f *Factory) CreateMessageSender() MessageSender {
	return NewMessageSender(
		f.broadcastRepo,
		f.messageHistoryRepo,
		f.templateRepo,
		f.emailService,
		f.logger,
		f.config,
		f.apiEndpoint,
	)
}

// CreateOrchestrator creates a new broadcast orchestrator
func (f *Factory) CreateOrchestrator() BroadcastOrchestratorInterface {
	messageSender := f.CreateMessageSender()
	timeProvider := NewRealTimeProvider()

	// Create AB test evaluator
	abTestEvaluator := NewABTestEvaluator(
		f.messageHistoryRepo,
		f.broadcastRepo,
		f.logger,
	)

	return NewBroadcastOrchestrator(
		messageSender,
		f.broadcastRepo,
		f.templateRepo,
		f.contactRepo,
		f.taskRepo,
		f.workspaceRepo,
		abTestEvaluator,
		f.logger,
		f.config,
		timeProvider,
		f.apiEndpoint,
	)
}

// RegisterWithTaskService registers the orchestrator with the task service
func (f *Factory) RegisterWithTaskService(taskService domain.TaskService) {
	orchestrator := f.CreateOrchestrator()
	taskService.RegisterProcessor(orchestrator)

	f.logger.Info("Broadcast orchestrator registered with task service")
}
