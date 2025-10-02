package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactService struct {
	repo                domain.ContactRepository
	workspaceRepo       domain.WorkspaceRepository
	authService         domain.AuthService
	messageHistoryRepo  domain.MessageHistoryRepository
	webhookEventRepo    domain.WebhookEventRepository
	contactListRepo     domain.ContactListRepository
	contactTimelineRepo domain.ContactTimelineRepository
	logger              logger.Logger
}

func NewContactService(
	repo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	authService domain.AuthService,
	messageHistoryRepo domain.MessageHistoryRepository,
	webhookEventRepo domain.WebhookEventRepository,
	contactListRepo domain.ContactListRepository,
	contactTimelineRepo domain.ContactTimelineRepository,
	logger logger.Logger,
) *ContactService {
	return &ContactService{
		repo:                repo,
		workspaceRepo:       workspaceRepo,
		authService:         authService,
		messageHistoryRepo:  messageHistoryRepo,
		webhookEventRepo:    webhookEventRepo,
		contactListRepo:     contactListRepo,
		contactTimelineRepo: contactTimelineRepo,
		logger:              logger,
	}
}

func (s *ContactService) GetContactByEmail(ctx context.Context, workspaceID string, email string) (*domain.Contact, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	contact, err := s.repo.GetContactByEmail(ctx, workspaceID, email)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			return nil, err
		}
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to get contact by email: %v", err))
		return nil, fmt.Errorf("failed to get contact by email: %w", err)
	}

	return contact, nil
}

func (s *ContactService) GetContactByExternalID(ctx context.Context, workspaceID string, externalID string) (*domain.Contact, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	contact, err := s.repo.GetContactByExternalID(ctx, workspaceID, externalID)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			return nil, err
		}
		s.logger.WithField("external_id", externalID).Error(fmt.Sprintf("Failed to get contact by external ID: %v", err))
		return nil, fmt.Errorf("failed to get contact by external ID: %w", err)
	}

	return contact, nil
}

func (s *ContactService) GetContacts(ctx context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	response, err := s.repo.GetContacts(ctx, req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get contacts: %v", err))
		return nil, fmt.Errorf("failed to get contacts: %w", err)
	}

	return response, nil
}

func (s *ContactService) DeleteContact(ctx context.Context, workspaceID string, email string) error {
	var err error
	log.Println("DeleteContact", email, workspaceID)
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required",
		)
	}

	// Delete related data first
	if err := s.messageHistoryRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete message history: %v", err))
		return fmt.Errorf("failed to delete message history: %w", err)
	}

	if err := s.webhookEventRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete webhook events: %v", err))
		return fmt.Errorf("failed to delete webhook events: %w", err)
	}

	if err := s.contactListRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete contact list relationships: %v", err))
		return fmt.Errorf("failed to delete contact list relationships: %w", err)
	}

	if err := s.contactTimelineRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete contact timeline: %v", err))
		return fmt.Errorf("failed to delete contact timeline: %w", err)
	}

	// Finally delete the contact
	if err := s.repo.DeleteContact(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete contact: %v", err))
		return fmt.Errorf("failed to delete contact: %w", err)
	}

	return nil
}

func (s *ContactService) BatchImportContacts(ctx context.Context, workspaceID string, contacts []*domain.Contact) *domain.BatchImportContactsResponse {
	response := &domain.BatchImportContactsResponse{
		Operations: make([]*domain.UpsertContactOperation, len(contacts)),
	}

	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		response.Error = fmt.Sprintf("failed to authenticate user: %v", err)
		return response
	}

	// Check permission for writing contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		response.Error = "Insufficient permissions: write access to contacts required"
		return response
	}

	// Validate and upsert
	for i, contact := range contacts {
		now := time.Now().UTC()
		// Set created_at to now if not provided
		if contact.CreatedAt.IsZero() {
			contact.CreatedAt = now
		}
		contact.UpdatedAt = now

		// init operation
		operation := &domain.UpsertContactOperation{
			Email:  contact.Email,
			Action: domain.UpsertContactOperationCreate,
		}

		if err := contact.Validate(); err != nil {
			operation.Action = domain.UpsertContactOperationError
			operation.Error = fmt.Sprintf("invalid contact at index %d: %v", i, err)
			response.Operations = append(response.Operations, operation)
			continue
		}

		isNew, err := s.repo.UpsertContact(ctx, workspaceID, contact)
		if err != nil {
			operation.Action = domain.UpsertContactOperationError
			operation.Error = fmt.Sprintf("failed to upsert contact at index %d: %v", i, err)
			response.Operations = append(response.Operations, operation)
			continue
		}

		if !isNew {
			operation.Action = domain.UpsertContactOperationUpdate
		}

		response.Operations = append(response.Operations, operation)
	}

	return response
}

func (s *ContactService) UpsertContact(ctx context.Context, workspaceID string, contact *domain.Contact) domain.UpsertContactOperation {
	operation := domain.UpsertContactOperation{
		Email:  contact.Email,
		Action: domain.UpsertContactOperationCreate,
	}

	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		operation.Action = domain.UpsertContactOperationError
		operation.Error = err.Error()
		s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Failed to authenticate user: %v", err))
		return operation
	}

	// Check permission for writing contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		operation.Action = domain.UpsertContactOperationError
		operation.Error = "Insufficient permissions: write access to contacts required"
		s.logger.WithField("email", contact.Email).Error("Insufficient permissions: write access to contacts required")
		return operation
	}

	if err := contact.Validate(); err != nil {
		operation.Action = domain.UpsertContactOperationError
		operation.Error = err.Error()
		s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Invalid contact: %v", err))
		return operation
	}

	// Set created_at to now if not provided
	if contact.CreatedAt.IsZero() {
		contact.CreatedAt = time.Now().UTC()
	}

	// Always update the updated_at timestamp
	contact.UpdatedAt = time.Now().UTC()

	isNew, err := s.repo.UpsertContact(ctx, workspaceID, contact)
	if err != nil {
		operation.Action = domain.UpsertContactOperationError
		operation.Error = err.Error()
		s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Failed to upsert contact: %v", err))
		return operation
	}

	if !isNew {
		operation.Action = domain.UpsertContactOperationUpdate
	}

	return operation
}
