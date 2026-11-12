package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// WebAnalyticsContactVisitsService serves a contact's web visits. Reading them is reading
// both the contact and the workspace's web analytics, so both permissions are required.
type WebAnalyticsContactVisitsService struct {
	repo          domain.WebAnalyticsContactVisitsRepository
	workspaceRepo domain.WorkspaceRepository
	authService   domain.AuthService
}

// NewWebAnalyticsContactVisitsService creates the service.
func NewWebAnalyticsContactVisitsService(repo domain.WebAnalyticsContactVisitsRepository, workspaceRepo domain.WorkspaceRepository, authService domain.AuthService) *WebAnalyticsContactVisitsService {
	return &WebAnalyticsContactVisitsService{repo: repo, workspaceRepo: workspaceRepo, authService: authService}
}

func (s *WebAnalyticsContactVisitsService) authorize(ctx context.Context, workspaceID string) (context.Context, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to authenticate user: %w", err)
	}
	for _, resource := range []domain.PermissionResource{domain.PermissionResourceWebAnalytics, domain.PermissionResourceContacts} {
		if !userWorkspace.HasPermission(resource, domain.PermissionTypeRead) {
			return ctx, domain.NewPermissionError(resource, domain.PermissionTypeRead,
				fmt.Sprintf("Insufficient permissions: read access to %s required", resource))
		}
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to get workspace: %w", err)
	}
	if workspace.Settings.WebAnalytics == nil || !workspace.Settings.WebAnalytics.Enabled {
		return ctx, domain.ErrWebAnalyticsNotEnabled
	}
	return ctx, nil
}

// ContactVisits returns a page of the contact's sessions, with the summary on the first page.
func (s *WebAnalyticsContactVisitsService) ContactVisits(ctx context.Context, req domain.ContactVisitsRequest) (*domain.ContactVisitsResponse, error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	email := domain.NormalizeEmail(req.Email)

	response := &domain.ContactVisitsResponse{}
	if req.Cursor == "" {
		if response.Summary, err = s.repo.ContactVisitsSummary(ctx, req.WorkspaceID, email); err != nil {
			return nil, fmt.Errorf("failed to summarize contact visits: %w", err)
		}
	}
	response.Sessions, response.NextCursor, err = s.repo.ListContactSessions(ctx, req.WorkspaceID, email, req.Filter, req.Limit, req.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to list contact visits: %w", err)
	}
	return response, nil
}

// ContactSessionPages returns the pages and goals of one of the contact's sessions.
func (s *WebAnalyticsContactVisitsService) ContactSessionPages(ctx context.Context, req domain.ContactSessionPagesRequest) (*domain.ContactSessionPagesResponse, error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	pages, err := s.repo.ContactSessionPages(ctx, req.WorkspaceID, domain.NormalizeEmail(req.Email), req.SessionDate, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to read session pages: %w", err)
	}
	return pages, nil
}
