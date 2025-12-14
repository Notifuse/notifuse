package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// AutomationService handles automation business logic
type AutomationService struct {
	repo        domain.AutomationRepository
	authService domain.AuthService
	logger      logger.Logger
}

// NewAutomationService creates a new AutomationService
func NewAutomationService(
	repo domain.AutomationRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *AutomationService {
	return &AutomationService{
		repo:        repo,
		authService: authService,
		logger:      logger,
	}
}

// Create creates a new automation
func (s *AutomationService) Create(ctx context.Context, workspaceID string, automation *domain.Automation) error {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if err := automation.Validate(); err != nil {
		return fmt.Errorf("invalid automation: %w", err)
	}

	if err := s.repo.Create(ctx, workspaceID, automation); err != nil {
		s.logger.WithField("automation_id", automation.ID).Error(fmt.Sprintf("failed to create automation: %v", err))
		return fmt.Errorf("failed to create automation: %w", err)
	}

	return nil
}

// Get retrieves an automation by ID
func (s *AutomationService) Get(ctx context.Context, workspaceID, automationID string) (*domain.Automation, error) {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	automation, err := s.repo.GetByID(ctx, workspaceID, automationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get automation: %w", err)
	}

	return automation, nil
}

// List retrieves automations with optional filters
func (s *AutomationService) List(ctx context.Context, workspaceID string, filter domain.AutomationFilter) ([]*domain.Automation, int, error) {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to authenticate: %w", err)
	}

	automations, count, err := s.repo.List(ctx, workspaceID, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list automations: %w", err)
	}

	return automations, count, nil
}

// Update updates an existing automation
func (s *AutomationService) Update(ctx context.Context, workspaceID string, automation *domain.Automation) error {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if err := automation.Validate(); err != nil {
		return fmt.Errorf("invalid automation: %w", err)
	}

	// If list_id is being removed/empty, check that there are no email nodes in the embedded nodes
	if automation.HasEmailNodeRestriction() {
		if domain.HasEmailNodes(automation.Nodes) {
			return fmt.Errorf("cannot remove list_id from automation with email nodes - remove email nodes first")
		}
	}

	if err := s.repo.Update(ctx, workspaceID, automation); err != nil {
		s.logger.WithField("automation_id", automation.ID).Error(fmt.Sprintf("failed to update automation: %v", err))
		return fmt.Errorf("failed to update automation: %w", err)
	}

	return nil
}

// Delete soft-deletes an automation (can delete live automations)
// The repository handles dropping triggers and exiting active contacts
func (s *AutomationService) Delete(ctx context.Context, workspaceID, automationID string) error {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Repository handles:
	// 1. Dropping the DB trigger (if automation was live)
	// 2. Marking all active contact_automations as 'exited'
	// 3. Soft-deleting the automation (setting deleted_at)
	if err := s.repo.Delete(ctx, workspaceID, automationID); err != nil {
		s.logger.WithField("automation_id", automationID).Error(fmt.Sprintf("failed to delete automation: %v", err))
		return fmt.Errorf("failed to delete automation: %w", err)
	}

	return nil
}

// Activate activates an automation (changes status to live and creates trigger)
func (s *AutomationService) Activate(ctx context.Context, workspaceID, automationID string) error {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Get existing automation
	automation, err := s.repo.GetByID(ctx, workspaceID, automationID)
	if err != nil {
		return fmt.Errorf("failed to get automation: %w", err)
	}

	// Check if already live
	if automation.Status == domain.AutomationStatusLive {
		return fmt.Errorf("automation is already live")
	}

	// If no list_id, check that there are no email nodes in the embedded nodes
	if automation.HasEmailNodeRestriction() {
		if domain.HasEmailNodes(automation.Nodes) {
			return fmt.Errorf("cannot activate automation with email nodes when list_id is not set")
		}
	}

	// Update status to live
	automation.Status = domain.AutomationStatusLive
	if err := s.repo.Update(ctx, workspaceID, automation); err != nil {
		return fmt.Errorf("failed to update automation status: %w", err)
	}

	// Create the database trigger
	if err := s.repo.CreateAutomationTrigger(ctx, workspaceID, automation); err != nil {
		// Rollback status change
		automation.Status = domain.AutomationStatusDraft
		_ = s.repo.Update(ctx, workspaceID, automation)
		return fmt.Errorf("failed to create automation trigger: %w", err)
	}

	return nil
}

// Pause pauses a live automation (changes status to paused and drops trigger)
func (s *AutomationService) Pause(ctx context.Context, workspaceID, automationID string) error {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Get existing automation
	automation, err := s.repo.GetByID(ctx, workspaceID, automationID)
	if err != nil {
		return fmt.Errorf("failed to get automation: %w", err)
	}

	// Check if live
	if automation.Status != domain.AutomationStatusLive {
		return fmt.Errorf("automation is not live")
	}

	// Drop the database trigger first
	if err := s.repo.DropAutomationTrigger(ctx, workspaceID, automationID); err != nil {
		return fmt.Errorf("failed to drop automation trigger: %w", err)
	}

	// Update status to paused
	automation.Status = domain.AutomationStatusPaused
	if err := s.repo.Update(ctx, workspaceID, automation); err != nil {
		return fmt.Errorf("failed to update automation status: %w", err)
	}

	return nil
}

// GetContactNodeExecutions retrieves the node executions of a contact through an automation
func (s *AutomationService) GetContactNodeExecutions(ctx context.Context, workspaceID, automationID, email string) (*domain.ContactAutomation, []*domain.NodeExecution, error) {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	// Get the contact automation record
	contactAutomation, err := s.repo.GetContactAutomationByEmail(ctx, workspaceID, automationID, email)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get contact automation: %w", err)
	}

	// Get the node executions
	entries, err := s.repo.GetNodeExecutions(ctx, workspaceID, contactAutomation.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node executions: %w", err)
	}

	return contactAutomation, entries, nil
}
