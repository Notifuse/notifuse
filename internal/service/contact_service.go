package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactService struct {
	repo                domain.ContactRepository
	workspaceRepo       domain.WorkspaceRepository
	authService         domain.AuthService
	messageHistoryRepo  domain.MessageHistoryRepository
	inboundWebhookEventRepo    domain.InboundWebhookEventRepository
	contactListRepo     domain.ContactListRepository
	contactTimelineRepo domain.ContactTimelineRepository
	logger              logger.Logger
}

func NewContactService(
	repo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	authService domain.AuthService,
	messageHistoryRepo domain.MessageHistoryRepository,
	inboundWebhookEventRepo domain.InboundWebhookEventRepository,
	contactListRepo domain.ContactListRepository,
	contactTimelineRepo domain.ContactTimelineRepository,
	logger logger.Logger,
) *ContactService {
	return &ContactService{
		repo:                repo,
		workspaceRepo:       workspaceRepo,
		authService:         authService,
		messageHistoryRepo:  messageHistoryRepo,
		inboundWebhookEventRepo:    inboundWebhookEventRepo,
		contactListRepo:     contactListRepo,
		contactTimelineRepo: contactTimelineRepo,
		logger:              logger,
	}
}

func (s *ContactService) GetContactByEmail(ctx context.Context, workspaceID string, email string) (*domain.Contact, error) {
	// Normalize email for consistent lookups
	email = domain.NormalizeEmail(email)

	// Check if this is a system call (e.g., from Supabase webhook)
	isSystemCall := ctx.Value(domain.SystemCallKey) != nil

	// Only authenticate and check permissions for non-system calls
	if !isSystemCall {
		var err error
		var userWorkspace *domain.UserWorkspace
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
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
	// Normalize email for consistent lookups
	email = domain.NormalizeEmail(email)

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

	if err := s.inboundWebhookEventRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
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

func (s *ContactService) BatchImportContacts(ctx context.Context, workspaceID string, contacts []*domain.Contact, listIDs []string) *domain.BatchImportContactsResponse {
	response := &domain.BatchImportContactsResponse{
		Operations: make([]*domain.UpsertContactOperation, 0, len(contacts)),
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

	// If listIDs are provided, also check permission for writing lists
	if len(listIDs) > 0 {
		if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
			response.Error = "Insufficient permissions: write access to lists required"
			return response
		}
	}

	// Pre-validate all contacts and separate valid from invalid
	// This allows us to provide immediate feedback on validation errors
	// while still processing valid contacts in bulk
	validContacts := make([]*domain.Contact, 0, len(contacts))
	validContactIndices := make([]int, 0, len(contacts))

	for i, contact := range contacts {
		// CreatedAt and UpdatedAt are optional - if not provided, DB will use CURRENT_TIMESTAMP
		// If provided, the values will be used (allows historical imports)

		if err := contact.Validate(); err != nil {
			// Record validation error
			operation := &domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationError,
				Error:  fmt.Sprintf("invalid contact at index %d: %v", i, err),
			}
			response.Operations = append(response.Operations, operation)
		} else {
			// Add to valid contacts for bulk processing
			validContacts = append(validContacts, contact)
			validContactIndices = append(validContactIndices, i)
		}
	}

	// Deduplicate contacts by email - keep the last occurrence
	// This prevents PostgreSQL "ON CONFLICT DO UPDATE cannot affect row a second time" error
	// when the same email appears multiple times in a single batch
	if len(validContacts) > 1 {
		seen := make(map[string]bool)
		deduplicatedContacts := make([]*domain.Contact, 0, len(validContacts))
		deduplicatedIndices := make([]int, 0, len(validContacts))

		// Iterate in reverse to keep last occurrence at its original position
		for i := len(validContacts) - 1; i >= 0; i-- {
			email := validContacts[i].Email
			if !seen[email] {
				seen[email] = true
				deduplicatedContacts = append([]*domain.Contact{validContacts[i]}, deduplicatedContacts...)
				deduplicatedIndices = append([]int{validContactIndices[i]}, deduplicatedIndices...)
			}
		}

		validContacts = deduplicatedContacts
		validContactIndices = deduplicatedIndices
	}

	// If there are valid contacts, perform bulk upsert
	if len(validContacts) > 0 {
		bulkResults, err := s.repo.BulkUpsertContacts(ctx, workspaceID, validContacts)
		if err != nil {
			// Bulk operation failed - mark all valid contacts as errors
			s.logger.Error(fmt.Sprintf("Bulk upsert failed: %v", err))
			for i, contact := range validContacts {
				operation := &domain.UpsertContactOperation{
					Email:  contact.Email,
					Action: domain.UpsertContactOperationError,
					Error:  fmt.Sprintf("failed to upsert contact at index %d: %v", validContactIndices[i], err),
				}
				response.Operations = append(response.Operations, operation)
			}
		} else {
			// Map bulk results to individual operations
			for _, result := range bulkResults {
				action := domain.UpsertContactOperationCreate
				if !result.IsNew {
					action = domain.UpsertContactOperationUpdate
				}

				operation := &domain.UpsertContactOperation{
					Email:  result.Email,
					Action: action,
				}
				response.Operations = append(response.Operations, operation)
			}

			// If listIDs were provided, bulk subscribe contacts to lists
			if len(listIDs) > 0 {
				emails := make([]string, len(validContacts))
				for i, contact := range validContacts {
					emails[i] = contact.Email
				}

				// Bulk add all valid contacts to all specified lists
				err := s.contactListRepo.BulkAddContactsToLists(ctx, workspaceID, emails, listIDs, domain.ContactListStatusActive)
				if err != nil {
					s.logger.Error(fmt.Sprintf("Failed to bulk add contacts to lists: %v", err))
					// Note: We don't fail the entire operation if list subscription fails
					// The contacts were successfully created/updated
				}
			}
		}
	}

	return response
}

func (s *ContactService) UpsertContact(ctx context.Context, workspaceID string, contact *domain.Contact) domain.UpsertContactOperation {
	operation := domain.UpsertContactOperation{
		Email:  contact.Email,
		Action: domain.UpsertContactOperationCreate,
	}

	// Check if this is a system call (e.g., from Supabase webhook)
	isSystemCall := ctx.Value(domain.SystemCallKey) != nil

	// Only authenticate and check permissions for non-system calls
	if !isSystemCall {
		var err error
		var userWorkspace *domain.UserWorkspace
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
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
	}

	if err := contact.Validate(); err != nil {
		operation.Action = domain.UpsertContactOperationError
		operation.Error = err.Error()
		s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Invalid contact: %v", err))
		return operation
	}

	// CreatedAt and UpdatedAt are optional - if not provided, DB will use CURRENT_TIMESTAMP
	// If provided, the values will be used (allows historical imports)

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

func (s *ContactService) CountContacts(ctx context.Context, workspaceID string) (int, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return 0, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	count, err := s.repo.Count(ctx, workspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to count contacts: %v", err))
		return 0, fmt.Errorf("failed to count contacts: %w", err)
	}

	return count, nil
}
