package service

import (
	"context"
	"fmt"
	"time"

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
	authService domain.AuthService,
	contactRepo domain.ContactRepository,
	listRepo domain.ListRepository,
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

func (s *ContactListService) AddContactToList(ctx context.Context, workspaceID string, contactList *domain.ContactList) error {
	// Verify contact exists by email
	var err error
	ctx, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	_, err = s.contactRepo.GetContactByEmail(ctx, workspaceID, contactList.Email)
	if err != nil {
		return fmt.Errorf("contact not found: %w", err)
	}

	// Verify list exists
	_, err = s.listRepo.GetListByID(ctx, workspaceID, contactList.ListID)
	if err != nil {
		return fmt.Errorf("list not found: %w", err)
	}

	now := time.Now().UTC()
	contactList.CreatedAt = now
	contactList.UpdatedAt = now

	if err := contactList.Validate(); err != nil {
		return fmt.Errorf("invalid contact list: %w", err)
	}

	if err := s.repo.AddContactToList(ctx, workspaceID, contactList); err != nil {
		s.logger.WithField("email", contactList.Email).
			WithField("list_id", contactList.ListID).
			Error(fmt.Sprintf("Failed to add contact to list: %v", err))
		return fmt.Errorf("failed to add contact to list: %w", err)
	}

	if err := s.listRepo.IncrementTotal(ctx, workspaceID, contactList.ListID, domain.TotalTypeActive); err != nil {
		s.logger.WithField("list_id", contactList.ListID).
			Error(fmt.Sprintf("Failed to increment total active: %v", err))
	}

	return nil
}

func (s *ContactListService) GetContactListByIDs(ctx context.Context, workspaceID string, email, listID string) (*domain.ContactList, error) {
	var err error
	ctx, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
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
	ctx, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
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
	ctx, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
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

func (s *ContactListService) UpdateContactListStatus(ctx context.Context, workspaceID string, email, listID string, status domain.ContactListStatus) error {
	// Verify contact list exists
	var err error
	ctx, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	_, err = s.repo.GetContactListByIDs(ctx, workspaceID, email, listID)
	if err != nil {
		return fmt.Errorf("contact list not found: %w", err)
	}

	if err := s.repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status); err != nil {
		s.logger.WithField("email", email).
			WithField("list_id", listID).
			Error(fmt.Sprintf("Failed to update contact list status: %v", err))
		return fmt.Errorf("failed to update contact list status: %w", err)
	}

	return nil
}

func (s *ContactListService) RemoveContactFromList(ctx context.Context, workspaceID string, email, listID string) error {
	var err error
	ctx, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	if err := s.repo.RemoveContactFromList(ctx, workspaceID, email, listID); err != nil {
		s.logger.WithField("email", email).
			WithField("list_id", listID).
			Error(fmt.Sprintf("Failed to remove contact from list: %v", err))
		return fmt.Errorf("failed to remove contact from list: %w", err)
	}

	return nil
}
