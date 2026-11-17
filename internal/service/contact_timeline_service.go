package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ContactTimelineService implements domain.ContactTimelineService
type ContactTimelineService struct {
	repo          domain.ContactTimelineRepository
	authService   domain.AuthService
	workspaceRepo domain.WorkspaceRepository
	logger        logger.Logger
}

// NewContactTimelineService creates a new contact timeline service
func NewContactTimelineService(repo domain.ContactTimelineRepository, authService domain.AuthService, workspaceRepo domain.WorkspaceRepository, logger logger.Logger) *ContactTimelineService {
	return &ContactTimelineService{
		repo:          repo,
		authService:   authService,
		workspaceRepo: workspaceRepo,
		logger:        logger,
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

// timelinePublicEntityTypes are the entity types that carry nothing beyond the contact, so
// reading contacts is enough. Every type of domain.TimelineEntityTypes is in this list or in
// timelineEntityPermissions (TestTimelineEntityTypes_AllClassified).
var timelinePublicEntityTypes = []string{
	domain.TimelineEntityContact,
	domain.TimelineEntityContactList,
	domain.TimelineEntityContactSegment,
	domain.TimelineEntityCustomEvent,
	domain.TimelineEntityAutomation,
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

	return s.repo.List(ctx, workspaceID, s.messageDataKeyLoader(workspaceID, filter), email, limit, cursor, filter)
}

// messageDataKeyLoader gives the repository the workspace's key, which decrypts the data of the
// messages the entries carry. It is nil when the filter keeps no message_history entry, and the
// repository calls it only on meeting encrypted data, so a timeline without messages never loads
// the workspace (a system database read that also decrypts every integration secret). A failed
// load leaves the messages' data out rather than failing the timeline.
func (s *ContactTimelineService) messageDataKeyLoader(workspaceID string, filter domain.TimelineEntityTypeFilter) domain.TimelineSecretKeyLoader {
	if !timelineFilterKeeps(filter, domain.TimelineEntityMessageHistory) {
		return nil
	}
	return func(ctx context.Context) (string, error) {
		workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"workspace_id": workspaceID,
				"error":        err.Error(),
			}).Error("Failed to load the workspace key for the timeline: messages are listed without their data")
			return "", err
		}
		return workspace.Settings.SecretKey, nil
	}
}

// timelineFilterKeeps tells whether entries of entityType pass the filter.
func timelineFilterKeeps(filter domain.TimelineEntityTypeFilter, entityType string) bool {
	if len(filter.Only) > 0 {
		return slices.Contains(filter.Only, entityType)
	}
	return !slices.Contains(filter.Except, entityType)
}
