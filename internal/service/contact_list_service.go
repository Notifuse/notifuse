package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactListService struct {
	repo        domain.ContactListRepository
	authService domain.AuthService
	contactRepo domain.ContactRepository
	listRepo    domain.ListRepository
	logger      logger.Logger
}

func NewContactListService(
	repo domain.ContactListRepository,
	_ domain.WorkspaceRepository,
	authService domain.AuthService,
	contactRepo domain.ContactRepository,
	listRepo domain.ListRepository,
	_ domain.ContactListRepository,
	logger logger.Logger,
) *ContactListService {
	return &ContactListService{
		repo:        repo,
		authService: authService,
		contactRepo: contactRepo,
		listRepo:    listRepo,
		logger:      logger,
	}
}

func (s *ContactListService) GetContactListByIDs(ctx context.Context, workspaceID string, email, listID string) (*domain.ContactList, error) {
	var err error
	var userWorkspace *domain.UserWorkspace
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to lists required",
		)
	}

	contactList, err := s.repo.GetContactListByIDs(ctx, workspaceID, email, listID)
	if err != nil {
		if _, ok := err.(*domain.ErrContactListNotFound); ok {
			return nil, err
		}
		s.logger.WithField("email", email).
			WithField("list_id", listID).
			Error(fmt.Sprintf("Failed to get contact list: %v", err))
		return nil, fmt.Errorf("failed to get contact list: %w", err)
	}

	return contactList, nil
}

func (s *ContactListService) GetContactsByListID(ctx context.Context, workspaceID string, listID string) ([]*domain.ContactList, error) {
	var err error
	var userWorkspace *domain.UserWorkspace
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to lists required",
		)
	}

	// The response enumerates every subscriber address on the list, so lists:read
	// alone would be a full contact dump to a caller granted no contact access.
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	// Verify list exists
	_, err = s.listRepo.GetListByID(ctx, workspaceID, listID)
	if err != nil {
		return nil, fmt.Errorf("list not found: %w", err)
	}

	contactLists, err := s.repo.GetContactsByListID(ctx, workspaceID, listID)
	if err != nil {
		s.logger.WithField("list_id", listID).
			Error(fmt.Sprintf("Failed to get contacts for list: %v", err))
		return nil, fmt.Errorf("failed to get contacts for list: %w", err)
	}

	return contactLists, nil
}

func (s *ContactListService) GetListsByEmail(ctx context.Context, workspaceID string, email string) ([]*domain.ContactList, error) {
	// Verify contact exists by email
	var err error
	var userWorkspace *domain.UserWorkspace
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to lists required",
		)
	}

	// Answering for an arbitrary address confirms which contacts exist and what
	// they are subscribed to, so it needs contact access as well as list access.
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	_, err = s.contactRepo.GetContactByEmail(ctx, workspaceID, email)
	if err != nil {
		return nil, fmt.Errorf("contact not found: %w", err)
	}

	contactLists, err := s.repo.GetListsByEmail(ctx, workspaceID, email)
	if err != nil {
		s.logger.WithField("email", email).
			Error(fmt.Sprintf("Failed to get lists for contact: %v", err))
		return nil, fmt.Errorf("failed to get lists for contact: %w", err)
	}

	return contactLists, nil
}

// UpdateContactListStatus changes a contact's status on a list. A contact who opted out
// keeps their opt-out: a plain change may only make it stronger
// (domain.ContactListStatusTransitionAllowed). With resubscribe, the row goes back to
// active from unsubscribed, and from bounced for a console user (a mailbox fixed since),
// never from complained. The route is audited only when that happens.
func (s *ContactListService) UpdateContactListStatus(ctx context.Context, workspaceID string, email, listID string, status domain.ContactListStatus, resubscribe bool) (*domain.UpdateContactListStatusResult, error) {
	// Marks the audit record when the contact actually leaves an opt-out: the handler
	// keeps the entry only then.
	audit := domain.AuditFromContext(ctx)
	resubscribed := false
	defer func() {
		if resubscribed {
			audit.AddMetadata(domain.AuditResubscribedListIDs, []string{listID})
		}
	}()

	// Verify contact list exists
	var err error
	var user *domain.User
	var userWorkspace *domain.UserWorkspace
	ctx, user, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to lists required",
		)
	}

	// The subscription status lives on the contact as much as on the list, so
	// changing it requires write access to both.
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required",
		)
	}

	existing, err := s.repo.GetContactListByIDs(ctx, workspaceID, email, listID)
	if err != nil {
		// If contact is not in the list, return success with message
		if _, ok := err.(*domain.ErrContactListNotFound); ok {
			s.logger.WithField("email", email).
				WithField("list_id", listID).
				Info("Contact not in list, treating as successful operation")
			return &domain.UpdateContactListStatusResult{
				Success: true,
				Message: "contact not in list",
				Found:   false,
			}, nil
		}
		return nil, fmt.Errorf("contact list not found: %w", err)
	}

	if resubscribe && domain.IsOptOutContactListStatus(existing.Status) {
		// A console user may clear a bounce (the mailbox was fixed); an API key only an
		// unsubscribe. A complaint is never cleared.
		clearBounce := user != nil && user.Type == domain.UserTypeUser
		if existing.Status == domain.ContactListStatusComplained ||
			(existing.Status == domain.ContactListStatusBounced && !clearBounce) {
			return nil, &domain.ErrContactListOptedOut{Status: existing.Status}
		}
		written, err := s.repo.ResubscribeContactToList(ctx, workspaceID, &domain.ContactList{
			Email:  email,
			ListID: listID,
			Status: domain.ContactListStatusActive,
		}, clearBounce)
		if err != nil {
			return nil, fmt.Errorf("failed to resubscribe contact to list: %w", err)
		}
		if !written {
			// A complaint (or, for an API key, a bounce) was recorded since the read. The
			// row may also have been removed meanwhile, so it is read whatever deleted_at.
			stored, err := s.repo.GetContactListIncludingRemoved(ctx, workspaceID, email, listID)
			if err != nil {
				return nil, fmt.Errorf("failed to read contact list: %w", err)
			}
			return nil, &domain.ErrContactListOptedOut{Status: stored.Status}
		}
		resubscribed = true
		return &domain.UpdateContactListStatusResult{
			Success: true,
			Message: "contact resubscribed",
			Found:   true,
		}, nil
	}

	if !domain.ContactListStatusTransitionAllowed(existing.Status, status) {
		return nil, &domain.ErrContactListOptedOut{Status: existing.Status}
	}

	if err := s.repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status); err != nil {
		var optedOut *domain.ErrContactListOptedOut
		if errors.As(err, &optedOut) {
			// Opted out since the read above.
			return nil, err
		}
		s.logger.WithField("email", email).
			WithField("list_id", listID).
			Error(fmt.Sprintf("Failed to update contact list status: %v", err))
		return nil, fmt.Errorf("failed to update contact list status: %w", err)
	}

	return &domain.UpdateContactListStatusResult{
		Success: true,
		Message: "status updated successfully",
		Found:   true,
	}, nil
}

func (s *ContactListService) RemoveContactFromList(ctx context.Context, workspaceID string, email, listID string) error {
	var err error
	var userWorkspace *domain.UserWorkspace
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to lists required",
		)
	}

	// Removing the membership row edits the contact's subscriptions, so it
	// requires write access to both resources.
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required",
		)
	}

	if err := s.repo.RemoveContactFromList(ctx, workspaceID, email, listID); err != nil {
		s.logger.WithField("email", email).
			WithField("list_id", listID).
			Error(fmt.Sprintf("Failed to remove contact from list: %v", err))
		return fmt.Errorf("failed to remove contact from list: %w", err)
	}

	return nil
}
