package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestBroadcastService_EstimateAudience(t *testing.T) {
	ctx := context.Background()
	readable := func(resources ...domain.PermissionResource) domain.UserPermissions {
		perms := domain.UserPermissions{}
		for _, r := range resources {
			perms[r] = domain.ResourcePermissions{Read: true}
		}
		return perms
	}
	all := readable(domain.PermissionResourceBroadcasts, domain.PermissionResourceLists, domain.PermissionResourceSegments)
	withSegments := domain.AudienceSettings{List: "l1", Segments: []string{"s1"}, ExcludeUnsubscribed: true}
	listOnly := domain.AudienceSettings{List: "l1", ExcludeUnsubscribed: false}

	authenticate := func(d *broadcastSvcDeps, perms domain.UserPermissions) {
		d.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws").
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws", Role: "member", Permissions: perms}, nil)
	}

	for name, tc := range map[string]struct {
		perms    domain.UserPermissions
		audience domain.AudienceSettings
		resource domain.PermissionResource
	}{
		"broadcasts.read":             {readable(domain.PermissionResourceLists, domain.PermissionResourceSegments), withSegments, domain.PermissionResourceBroadcasts},
		"lists.read":                  {readable(domain.PermissionResourceBroadcasts, domain.PermissionResourceSegments), withSegments, domain.PermissionResourceLists},
		"segments.read with segments": {readable(domain.PermissionResourceBroadcasts, domain.PermissionResourceLists), withSegments, domain.PermissionResourceSegments},
	} {
		t.Run("refused without "+name, func(t *testing.T) {
			d := setupBroadcastSvc(t)
			authenticate(d, tc.perms)
			_, err := d.svc.EstimateAudience(ctx, "ws", tc.audience)
			var permErr *domain.PermissionError
			require.ErrorAs(t, err, &permErr)
			assert.Equal(t, tc.resource, permErr.Resource)
			assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
		})
	}

	t.Run("segments.read not needed without segments; audience passed as is", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		authenticate(d, readable(domain.PermissionResourceBroadcasts, domain.PermissionResourceLists))
		d.contactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), "ws", listOnly).Return(17, nil)
		n, err := d.svc.EstimateAudience(ctx, "ws", listOnly)
		require.NoError(t, err)
		assert.Equal(t, 17, n)
	})

	t.Run("with segments and every permission", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		authenticate(d, all)
		d.contactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), "ws", withSegments).Return(3, nil)
		n, err := d.svc.EstimateAudience(ctx, "ws", withSegments)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
	})

	t.Run("repository error wrapped", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		authenticate(d, all)
		d.contactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), "ws", listOnly).Return(0, errors.New("db down"))
		_, err := d.svc.EstimateAudience(ctx, "ws", listOnly)
		require.ErrorContains(t, err, "failed to estimate audience: db down")
	})

	t.Run("authentication error returned", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		d.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws").Return(ctx, nil, nil, errors.New("bad token"))
		_, err := d.svc.EstimateAudience(ctx, "ws", listOnly)
		require.ErrorContains(t, err, "failed to authenticate user")
	})
}

func TestBroadcastService_ListBroadcasts_PassesNewParams(t *testing.T) {
	ctx := context.Background()
	d := setupBroadcastSvc(t)
	d.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws").
		Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "member", Permissions: domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true},
		}}, nil)
	startedAfter := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	params := domain.ListBroadcastsParams{WorkspaceID: "ws", WithStatusCounts: true, StartedAfter: &startedAfter, Limit: 10}
	d.repo.EXPECT().ListBroadcasts(gomock.Any(), params).Return(&domain.BroadcastListResponse{StatusCounts: map[domain.BroadcastStatus]int{}}, nil)

	got, err := d.svc.ListBroadcasts(ctx, params)
	require.NoError(t, err)
	assert.NotNil(t, got.StatusCounts)
}
