package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type CustomEventService struct {
	repo        domain.CustomEventRepository
	contactRepo domain.ContactRepository
	authService domain.AuthService
	logger      logger.Logger
}

func NewCustomEventService(
	repo domain.CustomEventRepository,
	contactRepo domain.ContactRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *CustomEventService {
	return &CustomEventService{
		repo:        repo,
		contactRepo: contactRepo,
		authService: authService,
		logger:      logger,
	}
}

// UpsertEvent creates or updates a custom event with goal tracking and soft-delete support
func (s *CustomEventService) UpsertEvent(ctx context.Context, req *domain.UpsertCustomEventRequest) (*domain.CustomEvent, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing custom events
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required for custom events",
		)
	}

	if err := req.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	// Verify contact exists (or create if it doesn't) - only if not soft-deleting
	if req.DeletedAt == nil {
		contact, err := s.contactRepo.GetContactByEmail(ctx, req.WorkspaceID, req.Email)
		if err != nil {
			// Create contact if it doesn't exist
			contact = &domain.Contact{
				Email:     req.Email,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			_, err = s.contactRepo.UpsertContact(ctx, req.WorkspaceID, contact)
			if err != nil {
				return nil, fmt.Errorf("failed to create contact for custom event: %w", err)
			}
		}
		_ = contact // suppress unused warning
	}

	// Create or update custom event
	now := time.Now()
	occurredAt := now
	if req.OccurredAt != nil {
		occurredAt = *req.OccurredAt
	}

	event := &domain.CustomEvent{
		ExternalID:    req.ExternalID,
		Email:         req.Email,
		EventName:     req.EventName,
		Properties:    req.Properties,
		OccurredAt:    occurredAt,
		Source:        "api",
		IntegrationID: req.IntegrationID,
		GoalName:      req.GoalName,
		GoalType:      req.GoalType,
		GoalValue:     req.GoalValue,
		DeletedAt:     req.DeletedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := event.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid custom event: %s", err.Error()))
	}

	// A soft-delete has to read the row before hiding it: GetByID filters deleted
	// rows out, so once the write lands there is nothing left to describe.
	var rowBeforeDeletion *domain.CustomEvent
	if req.DeletedAt != nil {
		rowBeforeDeletion, _ = s.repo.GetByID(ctx, req.WorkspaceID, req.EventName, req.ExternalID)
	}

	if err := s.repo.Upsert(ctx, req.WorkspaceID, event); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"event_name": event.EventName,
		}).Error("Failed to upsert custom event")
		return nil, fmt.Errorf("failed to upsert custom event: %w", err)
	}

	action := "upserted"
	if req.DeletedAt != nil {
		action = "soft-deleted"
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": req.WorkspaceID,
		"email":        req.Email,
		"event_name":   event.EventName,
		"external_id":  event.ExternalID,
		"action":       action,
	}).Info("Custom event " + action + " successfully")

	return s.describeStoredEvent(ctx, req, event, rowBeforeDeletion), nil
}

// describeStoredEvent answers with the row the upsert left behind rather than with
// the request that produced it. The two part company on any partial write: the
// ON CONFLICT clause keeps the properties, integration link and goal fields a body
// left out, never moves a row's source, and discards a write whose occurred_at is
// not newer than the stored one — equal counts as not newer, so a body repeating
// the stored timestamp changes nothing while still answering 200. Echoing the request would put "properties": null in a
// 200 body over a row that still holds them — and null is also how this endpoint
// reads "say nothing about properties", so the caller could not even send the
// response back to reconcile.
func (s *CustomEventService) describeStoredEvent(
	ctx context.Context,
	req *domain.UpsertCustomEventRequest,
	written *domain.CustomEvent,
	rowBeforeDeletion *domain.CustomEvent,
) *domain.CustomEvent {
	if req.DeletedAt == nil {
		stored, err := s.repo.GetByID(ctx, req.WorkspaceID, req.EventName, req.ExternalID)
		if err == nil && stored != nil {
			return stored
		}
		s.logger.WithFields(map[string]interface{}{
			"event_name":  req.EventName,
			"external_id": req.ExternalID,
		}).Warn("Custom event written but not readable back; answering from the request")
	}

	// The write succeeded, so what is left is describing it without the row in
	// hand. Only the two things a caller cannot state come from the read taken
	// beforehand; everything else is what the request said it was.
	if rowBeforeDeletion != nil {
		if written.Properties == nil {
			written.Properties = rowBeforeDeletion.Properties
		}
		written.Source = rowBeforeDeletion.Source
	}
	if written.Properties == nil {
		// Nothing stored to preserve, so the row went in carrying an empty object.
		written.Properties = map[string]interface{}{}
	}
	return written
}

func (s *CustomEventService) ImportEvents(ctx context.Context, req *domain.ImportCustomEventsRequest) ([]string, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required for custom events",
		)
	}

	if err := req.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	// Validate and prepare all events
	now := time.Now()
	for i, event := range req.Events {
		if event.ExternalID == "" {
			return nil, domain.NewValidationError(fmt.Sprintf("event at index %d: external_id is required", i))
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = now
		}
		if event.UpdatedAt.IsZero() {
			event.UpdatedAt = now
		}
		if event.OccurredAt.IsZero() {
			event.OccurredAt = now
		}
		if event.Source == "" {
			event.Source = "api"
		}
		// Properties stays nil when the import entry omitted it: the batch writes
		// through the same ON CONFLICT clause as the single upsert, so filling in
		// an empty map here would empty the state of every event the import
		// re-mentions without restating its properties.

		if err := event.Validate(); err != nil {
			return nil, domain.NewValidationError(fmt.Sprintf("invalid event at index %d: %s", i, err.Error()))
		}
	}

	// Batch upsert (supports goal fields and soft-delete)
	if err := s.repo.BatchUpsert(ctx, req.WorkspaceID, req.Events); err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to import custom events")
		return nil, fmt.Errorf("failed to import custom events: %w", err)
	}

	// Extract external IDs
	externalIDs := make([]string, len(req.Events))
	for i, event := range req.Events {
		externalIDs[i] = event.ExternalID
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": req.WorkspaceID,
		"count":        len(externalIDs),
	}).Info("Custom events imported successfully")

	return externalIDs, nil
}

func (s *CustomEventService) GetEvent(ctx context.Context, workspaceID, eventName, externalID string) (*domain.CustomEvent, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading custom events
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	return s.repo.GetByID(ctx, workspaceID, eventName, externalID)
}

func (s *CustomEventService) ListEvents(ctx context.Context, req *domain.ListCustomEventsRequest) ([]*domain.CustomEvent, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading custom events
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	if err := req.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	// Query by email or event name
	if req.Email != "" {
		return s.repo.ListByEmail(ctx, req.WorkspaceID, req.Email, req.Limit, req.Offset)
	}
	if req.EventName != nil {
		return s.repo.ListByEventName(ctx, req.WorkspaceID, *req.EventName, req.Limit, req.Offset)
	}

	return nil, fmt.Errorf("either email or event_name must be provided")
}
