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

// timelineEntityPermissions names, for the entity types that carry more than the contact,
// the resource a member must be able to read to see them. Provider events (bounces,
// complaints) carry the message's template and ID, so they go with message history.
var timelineEntityPermissions = map[string]domain.PermissionResource{
	domain.TimelineEntityMessageHistory:      domain.PermissionResourceMessageHistory,
	domain.TimelineEntityInboundWebhookEvent: domain.PermissionResourceMessageHistory,
	domain.TimelineEntityWebhookEvent:        domain.PermissionResourceMessageHistory,
	domain.TimelineEntityWebSession:          domain.PermissionResourceWebAnalytics,
	domain.TimelineEntityWebPage:             domain.PermissionResourceWebAnalytics,
}

// List retrieves timeline entries for a contact with pagination
func (s *ContactTimelineService) List(ctx context.Context, workspaceID string, email string, limit int, cursor *string, entityTypes []string) ([]*domain.ContactTimelineEntry, *string, error) {
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

	readable := func(entityType string) bool {
		resource, gated := timelineEntityPermissions[entityType]
		return !gated || userWorkspace.HasPermission(resource, domain.PermissionTypeRead)
	}

	var filter domain.TimelineEntityTypeFilter
	if len(entityTypes) > 0 {
		for _, t := range entityTypes {
			if readable(t) {
				filter.Only = append(filter.Only, t)
			}
		}
		if len(filter.Only) == 0 {
			return []*domain.ContactTimelineEntry{}, nil, nil
		}
	} else {
		// A deny-list rather than the readable types, so an entity type added later is
		// shown unless it is gated here.
		for _, t := range domain.TimelineEntityTypes {
			if !readable(t) {
				filter.Except = append(filter.Except, t)
			}
		}
	}

	return s.repo.List(ctx, workspaceID, email, limit, cursor, filter)
}
