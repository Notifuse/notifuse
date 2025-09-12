package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactListService struct {
	repo            domain.ContactListRepository
	workspaceRepo   domain.WorkspaceRepository
	authService     domain.AuthService
	contactRepo     domain.ContactRepository
	listRepo        domain.ListRepository
	contactListRepo domain.ContactListRepository
	logger          logger.Logger
}

func NewContactListService(
	repo domain.ContactListRepository,
	workspaceRepo domain.WorkspaceRepository,
	authService domain.AuthService,
	contactRepo domain.ContactRepository,
	listRepo domain.ListRepository,
	contactListRepo domain.ContactListRepository,
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
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
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
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
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
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
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

func (s *ContactListService) UpdateContactListStatus(ctx context.Context, workspaceID string, email, listID string, status domain.ContactListStatus) (*domain.UpdateContactListStatusResult, error) {
	// Verify contact list exists
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	_, err = s.repo.GetContactListByIDs(ctx, workspaceID, email, listID)
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

	if err := s.repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status); err != nil {
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
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
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
