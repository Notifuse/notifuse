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

// GetNotificationCenter returns the notification center data for a contact
// It returns public lists and public transactional notifications
func (s *NotificationCenterService) GetNotificationCenter(ctx context.Context, workspaceID string, email string, emailHMAC string) (*domain.NotificationCenterResponse, error) {
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
			if list.IsPublic {
				publicLists = append(publicLists, list)
			}
		}
	}

	return &domain.NotificationCenterResponse{
		Contact:      contact,
		PublicLists:  publicLists,
		ContactLists: contact.ContactLists,
		LogoURL:      workspace.Settings.LogoURL,
		WebsiteURL:   workspace.Settings.WebsiteURL,
	}, nil
}
