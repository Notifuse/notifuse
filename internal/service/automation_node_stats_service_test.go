package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nodeStatsServiceMocks struct {
	repo *mocks.MockAutomationRepository
	auth *mocks.MockAuthService
	svc  *AutomationService
}

func newNodeStatsServiceMocks(t *testing.T) *nodeStatsServiceMocks {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	m := &nodeStatsServiceMocks{
		repo: mocks.NewMockAutomationRepository(ctrl),
		auth: mocks.NewMockAuthService(ctrl),
	}
	m.svc = NewAutomationService(m.repo, m.auth, pkgmocks.NewMockLogger(ctrl), false)
	return m
}

// as authenticates the caller with these permissions.
func (m *nodeStatsServiceMocks) as(permissions domain.UserPermissions) {
	m.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{},
		&domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws1", Role: "member", Permissions: permissions}, nil)
}

func permissionsWithout(resources ...domain.PermissionResource) domain.UserPermissions {
	perms := domain.NewFullPermissions()
	for _, r := range resources {
		perms[r] = domain.ResourcePermissions{}
	}
	return perms
}

func TestAutomationService_CountJourneys(t *testing.T) {
	t.Run("zeros for an automation without journey", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.as(domain.NewFullPermissions())
		m.repo.EXPECT().CountJourneysByStatus(gomock.Any(), "ws1", []string{"a1", "a2"}).
			Return(map[string]domain.JourneyCounts{"a1": {Active: 2, Total: 2}}, nil)

		got, err := m.svc.CountJourneys(context.Background(), "ws1", []string{"a1", "a2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]domain.JourneyCounts{"a1": {Active: 2, Total: 2}, "a2": {}}, got)
	})

	t.Run("no automation: an empty map and no query", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.as(domain.NewFullPermissions())

		got, err := m.svc.CountJourneys(context.Background(), "ws1", nil)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("refused without automations read", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.as(permissionsWithout(domain.PermissionResourceAutomations))

		_, err := m.svc.CountJourneys(context.Background(), "ws1", []string{"a1"})
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
	})

	t.Run("authentication and repository errors returned", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), nil, nil, errors.New("bad token"))
		_, err := m.svc.CountJourneys(context.Background(), "ws1", []string{"a1"})
		require.Error(t, err)

		m.as(domain.NewFullPermissions())
		m.repo.EXPECT().CountJourneysByStatus(gomock.Any(), "ws1", []string{"a1"}).Return(nil, errors.New("db down"))
		_, err = m.svc.CountJourneys(context.Background(), "ws1", []string{"a1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}

func nodeStatsAutomation() *domain.Automation {
	next := "delay-1"
	return &domain.Automation{
		ID:          "auto1",
		WorkspaceID: "ws1",
		Nodes: []*domain.AutomationNode{
			{ID: "trigger-1", Type: domain.NodeTypeTrigger, NextNodeID: stringPtrForNodeStats("email-1")},
			{ID: "email-1", Type: domain.NodeTypeEmail, NextNodeID: &next},
			{ID: "delay-1", Type: domain.NodeTypeDelay},
			{ID: "email-2", Type: domain.NodeTypeEmail},
		},
	}
}

func stringPtrForNodeStats(s string) *string { return &s }

func TestAutomationService_GetNodeStats(t *testing.T) {
	since := time.Date(2026, 10, 11, 22, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 10, 22, 59, 59, 0, time.UTC)

	t.Run("every node of the graph, typed from the graph, zero when never visited", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.as(domain.NewFullPermissions())
		m.repo.EXPECT().GetByID(gomock.Any(), "ws1", "auto1").Return(nodeStatsAutomation(), nil)

		visited := domain.NewNodeStats("")
		visited.Visits = 5
		visited.Email = &domain.NodeEmailStats{Executions: 5, Sent: 4}
		removed := domain.NewNodeStats("")
		removed.Visits = 9
		m.repo.EXPECT().GetNodeStats(gomock.Any(), "ws1", "auto1", &since, &until, true).
			Return(&domain.NodeStatsRaw{Nodes: map[string]*domain.NodeStats{
				"email-1":      visited,
				"step-removed": removed,
				"delay-1":      {NodeType: domain.NodeTypeTrigger}, // an error row's type, ignored
			}}, nil)

		got, err := m.svc.GetNodeStats(context.Background(), "ws1", "auto1", &since, &until)
		require.NoError(t, err)
		assert.Equal(t, "auto1", got.AutomationID)
		assert.Equal(t, &since, got.Since)
		assert.Equal(t, &until, got.Until)

		require.Len(t, got.Nodes, 4, "the four nodes of the graph, the removed step dropped")
		assert.NotContains(t, got.Nodes, "step-removed")
		assert.Equal(t, domain.NodeTypeEmail, got.Nodes["email-1"].NodeType)
		assert.Equal(t, int64(5), got.Nodes["email-1"].Visits)
		assert.Equal(t, int64(4), got.Nodes["email-1"].Email.Sent)
		assert.Equal(t, domain.NodeTypeDelay, got.Nodes["delay-1"].NodeType, "the type comes from the graph")
		assert.NotNil(t, got.Nodes["delay-1"].Branches)
		assert.NotNil(t, got.Nodes["delay-1"].SkipReasons)

		require.NotNil(t, got.Nodes["email-2"].Email, "an email step never visited has zeroed counters")
		assert.Equal(t, domain.NodeEmailStats{}, *got.Nodes["email-2"].Email)
		assert.Nil(t, got.Nodes["delay-1"].Email, "a step that is not an email has none")
		assert.Nil(t, got.Nodes["trigger-1"].Email)
		assert.Equal(t, int64(0), got.Nodes["trigger-1"].Visits)
	})

	t.Run("without message history read: no email query and no email counters", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.as(permissionsWithout(domain.PermissionResourceMessageHistory))
		m.repo.EXPECT().GetByID(gomock.Any(), "ws1", "auto1").Return(nodeStatsAutomation(), nil)
		m.repo.EXPECT().GetNodeStats(gomock.Any(), "ws1", "auto1", nil, nil, false).
			Return(&domain.NodeStatsRaw{Nodes: map[string]*domain.NodeStats{}}, nil)

		got, err := m.svc.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil)
		require.NoError(t, err)
		for id, n := range got.Nodes {
			assert.Nil(t, n.Email, id)
		}
	})

	t.Run("refused without automations read", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.as(permissionsWithout(domain.PermissionResourceAutomations))

		_, err := m.svc.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil)
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
	})

	t.Run("an unknown automation is a not found error", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.as(domain.NewFullPermissions())
		m.repo.EXPECT().GetByID(gomock.Any(), "ws1", "auto1").
			Return(nil, fmt.Errorf("lookup: %w", &domain.ErrNotFound{Entity: "automation", ID: "auto1"}))

		_, err := m.svc.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil)
		var notFound *domain.ErrNotFound
		require.ErrorAs(t, err, &notFound)
	})

	t.Run("authentication, lookup and repository errors returned", func(t *testing.T) {
		m := newNodeStatsServiceMocks(t)
		m.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), nil, nil, errors.New("bad token"))
		_, err := m.svc.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil)
		require.Error(t, err)

		m.as(domain.NewFullPermissions())
		m.repo.EXPECT().GetByID(gomock.Any(), "ws1", "auto1").Return(nil, errors.New("db down"))
		_, err = m.svc.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil)
		require.Error(t, err)
		var notFound *domain.ErrNotFound
		assert.False(t, errors.As(err, &notFound))

		m.as(domain.NewFullPermissions())
		m.repo.EXPECT().GetByID(gomock.Any(), "ws1", "auto1").Return(nodeStatsAutomation(), nil)
		m.repo.EXPECT().GetNodeStats(gomock.Any(), "ws1", "auto1", nil, nil, true).Return(nil, errors.New("stats down"))
		_, err = m.svc.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stats down")
	})
}
