package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type NotificationCenterService struct {
	contactRepo   domain.ContactRepository
	workspaceRepo domain.WorkspaceRepository
	listRepo      domain.ListRepository
	logger        logger.Logger
}

func NewNotificationCenterService(
	contactRepo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	listRepo domain.ListRepository,
	logger logger.Logger,
) *NotificationCenterService {
	return &NotificationCenterService{
		contactRepo:   contactRepo,
		workspaceRepo: workspaceRepo,
		listRepo:      listRepo,
		logger:        logger,
	}
}

// GetContactPreferences returns the notification center data for a contact
// It returns public lists and public transactional notifications
func (s *NotificationCenterService) GetContactPreferences(ctx context.Context, workspaceID string, email string, emailHMAC string) (*domain.ContactPreferencesResponse, error) {
	// Verify that the email HMAC is valid
	// This is a simple security measure to verify the request is legitimate
	// The workspace should have a secret key that is used to generate the HMAC
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get workspace: %v", err))
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Using the workspace's settings secret key to verify the HMAC
	secretKey := workspace.Settings.SecretKey

	if !domain.VerifyEmailHMAC(email, emailHMAC, secretKey) {
		return nil, fmt.Errorf("invalid email verification")
	}

	// Get the contact
	contact, err := s.contactRepo.GetContactByEmail(ctx, workspaceID, email)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			return nil, err
		}
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to get contact: %v", err))
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}

	// Get public lists for this workspace
	var publicLists []*domain.List

	// Get lists using the list service

	lists, err := s.listRepo.GetLists(ctx, workspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get lists: %v", err))
	} else {
		// Filter to only include public lists
		for _, list := range lists {
			if list.IsPublic && list.DeletedAt == nil {
				publicLists = append(publicLists, list)
			}
		}
	}

	// The contact object carries the same rows, so both are filtered
	contact.ContactLists = visibleContactLists(contact.ContactLists, publicLists)

	return &domain.ContactPreferencesResponse{
		Contact:      contact,
		PublicLists:  publicLists,
		ContactLists: contact.ContactLists,
		LogoURL:      workspace.Settings.LogoURL,
		WebsiteURL:   workspace.Settings.WebsiteURL,
	}, nil
}

// visibleContactLists keeps the rows the contact can act on in their preferences: every
// row on a public list, and rows on private lists they are still on (active, or awaiting
// a double opt-in confirmation). A private list they left is the workspace's to manage,
// and showing it would only reveal its name. Removed rows keep their last status, so a
// removed member would show as subscribed; they are dropped everywhere.
func visibleContactLists(rows []*domain.ContactList, publicLists []*domain.List) []*domain.ContactList {
	if rows == nil {
		return nil
	}

	public := make(map[string]bool, len(publicLists))
	for _, list := range publicLists {
		public[list.ID] = true
	}

	visible := make([]*domain.ContactList, 0, len(rows))
	for _, row := range rows {
		if row.DeletedAt != nil {
			continue
		}
		if !public[row.ListID] &&
			row.Status != domain.ContactListStatusActive && row.Status != domain.ContactListStatusPending {
			continue
		}
		visible = append(visible, row)
	}
	return visible
}

// UpdateContactPreferences updates a contact's language and/or timezone
func (s *NotificationCenterService) UpdateContactPreferences(ctx context.Context, req *domain.UpdateContactPreferencesRequest) error {
	workspace, err := s.workspaceRepo.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get workspace: %v", err))
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	if !domain.VerifyEmailHMAC(req.Email, req.EmailHMAC, workspace.Settings.SecretKey) {
		return fmt.Errorf("invalid email verification")
	}

	contact := &domain.Contact{Email: req.Email}
	if req.Language != "" {
		contact.Language = &domain.NullableString{String: req.Language, IsNull: false}
	}
	if req.Timezone != "" {
		contact.Timezone = &domain.NullableString{String: req.Timezone, IsNull: false}
	}

	_, err = s.contactRepo.UpsertContact(ctx, req.WorkspaceID, contact)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to upsert contact preferences: %v", err))
		return fmt.Errorf("failed to update contact preferences: %w", err)
	}

	return nil
}
