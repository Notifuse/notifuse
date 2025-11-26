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

func (s *CustomEventService) CreateEvent(ctx context.Context, req *domain.CreateCustomEventRequest) (*domain.CustomEvent, error) {
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
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Verify contact exists (or create if it doesn't)
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
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid custom event: %w", err)
	}

	if err := s.repo.Create(ctx, req.WorkspaceID, event); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"event_name": event.EventName,
		}).Error("Failed to create custom event")
		return nil, fmt.Errorf("failed to create custom event: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": req.WorkspaceID,
		"email":        req.Email,
		"event_name":   event.EventName,
		"external_id":  event.ExternalID,
	}).Info("Custom event created successfully")

	return event, nil
}

func (s *CustomEventService) BatchCreateEvents(ctx context.Context, req *domain.BatchCreateCustomEventsRequest) ([]string, error) {
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
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate and prepare all events
	now := time.Now()
	for i, event := range req.Events {
		if event.ExternalID == "" {
			return nil, fmt.Errorf("event at index %d: external_id is required", i)
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
		if event.Properties == nil {
			event.Properties = make(map[string]interface{})
		}

		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("invalid event at index %d: %w", i, err)
		}
	}

	// Batch create/update
	if err := s.repo.BatchCreate(ctx, req.WorkspaceID, req.Events); err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to batch create custom events")
		return nil, fmt.Errorf("failed to batch create custom events: %w", err)
	}

	// Extract external IDs
	externalIDs := make([]string, len(req.Events))
	for i, event := range req.Events {
		externalIDs[i] = event.ExternalID
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": req.WorkspaceID,
		"count":        len(externalIDs),
	}).Info("Custom events batch created successfully")

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
		return nil, fmt.Errorf("invalid request: %w", err)
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
