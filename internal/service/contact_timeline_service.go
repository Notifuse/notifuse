package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// ContactTimelineService implements domain.ContactTimelineService
type ContactTimelineService struct {
	repo        domain.ContactTimelineRepository
	authService domain.AuthService
}

// NewContactTimelineService creates a new contact timeline service
func NewContactTimelineService(repo domain.ContactTimelineRepository, authService domain.AuthService) *ContactTimelineService {
	return &ContactTimelineService{
		repo:        repo,
		authService: authService,
	}
}

// List retrieves timeline entries for a contact with pagination
func (s *ContactTimelineService) List(ctx context.Context, workspaceID string, email string, limit int, cursor *string) ([]*domain.ContactTimelineEntry, *string, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// The timeline replays a contact's attribute changes, list subscriptions and
	// message history keyed by their email, so reading it is reading the contact.
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	return s.repo.List(ctx, workspaceID, email, limit, cursor)
}
