package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to automations required",
		)
	}

	// Create runs no DDL, so an automation created as live would be a row claiming to be
	// live with nothing listening on contact_timeline — it would show a Live badge, enrol
	// nobody, and refuse activation as "already live". Overwritten rather than rejected,
	// as in Update: clients echo the field back without meaning to set it.
	automation.Status = domain.AutomationStatusDraft

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
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to automations required",
		)
	}

	automation, err := s.repo.GetByID(ctx, workspaceID, automationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get automation: %w", err)
	}

	return automation, nil
}

// List retrieves automations with optional filters
func (s *AutomationService) List(ctx context.Context, workspaceID string, filter domain.AutomationFilter) ([]*domain.Automation, int, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeRead) {
		return nil, 0, domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to automations required",
		)
	}

	automations, count, err := s.repo.List(ctx, workspaceID, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list automations: %w", err)
	}

	return automations, count, nil
}

// Update updates an existing automation
func (s *AutomationService) Update(ctx context.Context, workspaceID string, automation *domain.Automation) error {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to automations required",
		)
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

	existing, err := s.repo.GetByID(ctx, workspaceID, automation.ID)
	if err != nil {
		return fmt.Errorf("failed to get automation: %w", err)
	}

	// Status is not the caller's to set here. Transitions belong to activate and pause,
	// which install and drop the trigger; honouring one would persist a live automation
	// with no trigger installed — one that never fires and that nothing ever repairs.
	//
	// The stored value is kept rather than the request rejected, because the whole object
	// is overwritten on update: every read-modify-write client sends back the status it
	// read, and the console sends it on every save. Erroring would fail those saves for a
	// field the caller never meant to change.
	automation.Status = existing.Status

	if err := s.repo.Update(ctx, workspaceID, automation); err != nil {
		s.logger.WithField("automation_id", automation.ID).Error(fmt.Sprintf("failed to update automation: %v", err))
		return fmt.Errorf("failed to update automation: %w", err)
	}

	// The installed trigger is compiled from the trigger config and the root node, so a
	// live automation whose config just changed is running a trigger that no longer
	// matches it.
	//
	// The row is written first and the trigger install compensated on failure, rather
	// than the reverse. Installing first and then failing to write the row would leave a
	// trigger compiled from a configuration the database does not store — and nothing
	// would ever repair it, because the next edit compares against that stale stored row,
	// finds no change, and skips regeneration.
	if existing.Status != domain.AutomationStatusLive || !triggerInputsChanged(existing, automation) {
		return nil
	}

	if err := s.repo.CreateAutomationTrigger(ctx, workspaceID, automation); err != nil {
		// Detached from the request context: a client disconnect is one of the ways the
		// install fails, and the compensation would then be cancelled by the very thing it
		// exists to compensate for.
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if rollbackErr := s.repo.Update(restoreCtx, workspaceID, existing); rollbackErr != nil {
			// The row now describes a configuration the installed trigger does not
			// implement. Nothing detects that on its own, so it has to be said out loud —
			// with the workspace, since each one is its own database.
			s.logger.WithField("automation_id", automation.ID).
				WithField("workspace_id", workspaceID).
				Error(fmt.Sprintf("failed to restore automation after trigger update failed, stored config no longer matches the installed trigger: %v", rollbackErr))
		}
		return fmt.Errorf("failed to update automation trigger: %w", err)
	}

	return nil
}

// triggerInputsChanged reports whether anything the trigger generator reads has changed.
// The generated function and WHEN clause are a function of the automation's id, root
// node and trigger config, and of nothing else in the record — so comparing those is
// enough to decide whether the installed trigger has to be rebuilt. It is worth the
// comparison: DROP/CREATE TRIGGER takes ACCESS EXCLUSIVE on contact_timeline, which
// every contact event in the workspace passes through.
func triggerInputsChanged(existing, updated *domain.Automation) bool {
	if existing.RootNodeID != updated.RootNodeID {
		return true
	}

	existingTrigger, err := json.Marshal(existing.Trigger)
	if err != nil {
		return true
	}
	updatedTrigger, err := json.Marshal(updated.Trigger)
	if err != nil {
		return true
	}

	return !bytes.Equal(existingTrigger, updatedTrigger)
}

// Delete soft-deletes an automation (can delete live automations)
// The repository handles dropping triggers and exiting active contacts
func (s *AutomationService) Delete(ctx context.Context, workspaceID, automationID string) error {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to automations required",
		)
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
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to automations required",
		)
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

	// Validate what is stored before generating DDL from it. A row written before a
	// validation rule existed can still be structurally unusable, and the generator
	// dereferences the trigger config without checking.
	if err := automation.Validate(); err != nil {
		return domain.NewTriggerConditionError(fmt.Sprintf("cannot activate automation: %v", err))
	}

	// Update status to live
	previousStatus := automation.Status
	automation.Status = domain.AutomationStatusLive
	if err := s.repo.Update(ctx, workspaceID, automation); err != nil {
		return fmt.Errorf("failed to update automation status: %w", err)
	}

	// Create the database trigger
	if err := s.repo.CreateAutomationTrigger(ctx, workspaceID, automation); err != nil {
		// Roll the status back to where it was, not unconditionally to draft: a failed
		// re-activation of a paused automation should leave it paused.
		automation.Status = previousStatus

		// Detached, for the same reason as in Update: a cancelled request must not also
		// cancel the write that undoes it. The residue here is worse — a live row with no
		// trigger installed.
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if rollbackErr := s.repo.Update(restoreCtx, workspaceID, automation); rollbackErr != nil {
			// The row is now live with no trigger installed. Say so — silently
			// discarding this leaves an automation that will never fire and no trace.
			s.logger.WithField("automation_id", automationID).
				WithField("workspace_id", workspaceID).
				Error(fmt.Sprintf("failed to roll back automation status after trigger creation failed: %v", rollbackErr))
		}
		return fmt.Errorf("failed to create automation trigger: %w", err)
	}

	return nil
}

// Pause pauses a live automation (changes status to paused and drops trigger)
func (s *AutomationService) Pause(ctx context.Context, workspaceID, automationID string) error {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to automations required",
		)
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
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeRead) {
		return nil, nil, domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to automations required",
		)
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
