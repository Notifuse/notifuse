package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

func TestWebAnalyticsContactVisitsService(t *testing.T) {
	ctx := context.Background()
	both := domain.UserPermissions{
		domain.PermissionResourceWebAnalytics: {Read: true},
		domain.PermissionResourceContacts:     {Read: true},
	}
	enabled := &domain.Workspace{ID: "ws", Settings: domain.WorkspaceSettings{WebAnalytics: &domain.WebAnalyticsSettings{Enabled: true}}}

	type deps struct {
		svc       *WebAnalyticsContactVisitsService
		repo      *mocks.MockWebAnalyticsContactVisitsRepository
		workspace *mocks.MockWorkspaceRepository
	}
	setup := func(t *testing.T, perms domain.UserPermissions) deps {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockWebAnalyticsContactVisitsRepository(ctrl)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		auth := mocks.NewMockAuthService(ctrl)
		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws").
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws", Role: "member", Permissions: perms}, nil)
		return deps{svc: NewWebAnalyticsContactVisitsService(repo, workspaceRepo, auth), repo: repo, workspace: workspaceRepo}
	}
	visits := domain.ContactVisitsRequest{WorkspaceID: "ws", Email: "Ann@Example.com", Filter: domain.ContactVisitsFilterAll, Limit: 20}
	pages := domain.ContactSessionPagesRequest{WorkspaceID: "ws", Email: "Ann@Example.com", SessionID: "0190d5a4-1c2b-7000-8000-000000000001", SessionDate: "2026-11-03"}

	for name, perms := range map[string]domain.UserPermissions{
		"web_analytics.read": {domain.PermissionResourceContacts: {Read: true}},
		"contacts.read":      {domain.PermissionResourceWebAnalytics: {Read: true}},
	} {
		t.Run("refused without "+name, func(t *testing.T) {
			d := setup(t, perms)
			_, err := d.svc.ContactVisits(ctx, visits)
			var permErr *domain.PermissionError
			require.ErrorAs(t, err, &permErr)
			assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)

			d = setup(t, perms)
			_, err = d.svc.ContactSessionPages(ctx, pages)
			require.ErrorAs(t, err, &permErr)
		})
	}

	for name, ws := range map[string]*domain.Workspace{
		"settings absent": {ID: "ws"},
		"disabled":        {ID: "ws", Settings: domain.WorkspaceSettings{WebAnalytics: &domain.WebAnalyticsSettings{Enabled: false}}},
	} {
		t.Run("web analytics "+name+": the dedicated error", func(t *testing.T) {
			d := setup(t, both)
			d.workspace.EXPECT().GetByID(gomock.Any(), "ws").Return(ws, nil)
			_, err := d.svc.ContactVisits(ctx, visits)
			require.ErrorIs(t, err, domain.ErrWebAnalyticsNotEnabled)
		})
	}

	t.Run("workspace read error returned", func(t *testing.T) {
		d := setup(t, both)
		d.workspace.EXPECT().GetByID(gomock.Any(), "ws").Return(nil, errors.New("db down"))
		_, err := d.svc.ContactSessionPages(ctx, pages)
		require.ErrorContains(t, err, "db down")
	})

	t.Run("first page: email normalized, summary and sessions", func(t *testing.T) {
		d := setup(t, both)
		d.workspace.EXPECT().GetByID(gomock.Any(), "ws").Return(enabled, nil)
		summary := &domain.ContactVisitsSummary{Sessions: 2}
		d.repo.EXPECT().ContactVisitsSummary(gomock.Any(), "ws", "ann@example.com").Return(summary, nil)
		d.repo.EXPECT().ListContactSessions(gomock.Any(), "ws", "ann@example.com", "all", 20, "").
			Return([]domain.ContactVisitSession{{ID: "s1"}}, "next", nil)

		got, err := d.svc.ContactVisits(ctx, visits)
		require.NoError(t, err)
		assert.Same(t, summary, got.Summary)
		assert.Equal(t, []domain.ContactVisitSession{{ID: "s1"}}, got.Sessions)
		assert.Equal(t, "next", got.NextCursor)
	})

	t.Run("a later page: no summary", func(t *testing.T) {
		d := setup(t, both)
		d.workspace.EXPECT().GetByID(gomock.Any(), "ws").Return(enabled, nil)
		d.repo.EXPECT().ContactVisitsSummary(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		d.repo.EXPECT().ListContactSessions(gomock.Any(), "ws", "ann@example.com", "goal", 20, "cur").
			Return([]domain.ContactVisitSession{}, "", nil)

		req := visits
		req.Cursor, req.Filter = "cur", domain.ContactVisitsFilterGoal
		got, err := d.svc.ContactVisits(ctx, req)
		require.NoError(t, err)
		assert.Nil(t, got.Summary)
	})

	t.Run("repository error returned", func(t *testing.T) {
		d := setup(t, both)
		d.workspace.EXPECT().GetByID(gomock.Any(), "ws").Return(enabled, nil)
		d.repo.EXPECT().ContactVisitsSummary(gomock.Any(), "ws", "ann@example.com").Return(nil, errors.New("boom"))
		_, err := d.svc.ContactVisits(ctx, visits)
		require.ErrorContains(t, err, "boom")
	})

	t.Run("session pages: email normalized before the repository call", func(t *testing.T) {
		d := setup(t, both)
		d.workspace.EXPECT().GetByID(gomock.Any(), "ws").Return(enabled, nil)
		want := &domain.ContactSessionPagesResponse{}
		d.repo.EXPECT().ContactSessionPages(gomock.Any(), "ws", "ann@example.com", "2026-11-03", pages.SessionID).Return(want, nil)

		got, err := d.svc.ContactSessionPages(ctx, pages)
		require.NoError(t, err)
		assert.Same(t, want, got)
	})
}
