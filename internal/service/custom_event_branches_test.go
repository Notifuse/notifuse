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

// Import/list branches: imports are all-or-nothing validated before the
// batch write (one bad event must not leave a partial import behind), zero
// timestamps default to the import instant, an omitted Properties stays nil
// (the merge semantics depend on it), and listing requires exactly one
// selector.

func customEventAuth(authCtx context.Context, read, write bool) *domain.UserWorkspace {
	return &domain.UserWorkspace{
		WorkspaceID: "ws-1",
		UserID:      "user-1",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: read, Write: write},
		},
	}
}

func TestCustomEventService_ImportEvents_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("auth failure is wrapped", func(t *testing.T) {
		_, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		sentinel := errors.New("expired")
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(nil, nil, nil, sentinel).Times(1)
		_, err := service.ImportEvents(ctx, &domain.ImportCustomEventsRequest{WorkspaceID: "ws-1"})
		require.ErrorContains(t, err, "failed to authenticate user:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("read-only users cannot import", func(t *testing.T) {
		_, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "user-1"}, customEventAuth(ctx, true, false), nil).Times(1)
		_, err := service.ImportEvents(ctx, &domain.ImportCustomEventsRequest{WorkspaceID: "ws-1"})
		require.EqualError(t, err,
			"Insufficient permissions: write access to contacts required for custom events")
	})

	t.Run("a missing external_id names its index and writes nothing", func(t *testing.T) {
		_, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "user-1"}, customEventAuth(ctx, true, true), nil).Times(1)
		// No BatchUpsert expectation: validation of the whole batch precedes
		// any write, so event 0 must not land either.
		req := &domain.ImportCustomEventsRequest{WorkspaceID: "ws-1", Events: []*domain.CustomEvent{
			{ExternalID: "ok-1", Email: "a@example.com", EventName: "orders/fulfilled"},
			{Email: "b@example.com", EventName: "orders/fulfilled"},
		}}
		_, err := service.ImportEvents(ctx, req)
		require.ErrorContains(t, err, "event at index 1: external_id is required")
		var validationErr domain.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("zero fields default to the import instant and source api", func(t *testing.T) {
		repo, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "user-1"}, customEventAuth(ctx, true, true), nil).Times(1)
		before := time.Now()
		repo.EXPECT().BatchUpsert(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, events []*domain.CustomEvent) error {
				require.Len(t, events, 1)
				ev := events[0]
				assert.False(t, ev.CreatedAt.Before(before))
				assert.False(t, ev.OccurredAt.Before(before))
				assert.Equal(t, "api", ev.Source)
				assert.Nil(t, ev.Properties, "an omitted properties map stays nil: nil means 'keep stored'")
				return nil
			}).Times(1)

		ids, err := service.ImportEvents(ctx, &domain.ImportCustomEventsRequest{
			WorkspaceID: "ws-1",
			Events:      []*domain.CustomEvent{{ExternalID: "e-1", Email: "a@example.com", EventName: "orders/fulfilled"}},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"e-1"}, ids)
	})

	t.Run("batch failure is wrapped with the operation", func(t *testing.T) {
		repo, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "user-1"}, customEventAuth(ctx, true, true), nil).Times(1)
		sentinel := errors.New("insert failed")
		repo.EXPECT().BatchUpsert(gomock.Any(), "ws-1", gomock.Any()).Return(sentinel).Times(1)

		_, err := service.ImportEvents(ctx, &domain.ImportCustomEventsRequest{
			WorkspaceID: "ws-1",
			Events:      []*domain.CustomEvent{{ExternalID: "e-1", Email: "a@example.com", EventName: "orders/fulfilled"}},
		})
		require.ErrorContains(t, err, "failed to import custom events:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestCustomEventService_ListEvents_Branches(t *testing.T) {
	ctx := context.Background()
	name := "orders/fulfilled"

	t.Run("event_name selector dereferences and forwards pagination", func(t *testing.T) {
		repo, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "user-1"}, customEventAuth(ctx, true, false), nil).Times(1)
		want := []*domain.CustomEvent{{ExternalID: "e-1"}}
		repo.EXPECT().ListByEventName(gomock.Any(), "ws-1", "orders/fulfilled", 10, 20).
			Return(want, nil).Times(1)

		got, err := service.ListEvents(ctx, &domain.ListCustomEventsRequest{
			WorkspaceID: "ws-1", EventName: &name, Limit: 10, Offset: 20,
		})
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("no selector is a plain error, not an empty page", func(t *testing.T) {
		_, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "user-1"}, customEventAuth(ctx, true, false), nil).Times(1)

		_, err := service.ListEvents(ctx, &domain.ListCustomEventsRequest{WorkspaceID: "ws-1"})
		require.EqualError(t, err, "validation error: invalid request: either email or event_name is required")
		var validationErr domain.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("write-only users cannot list", func(t *testing.T) {
		_, _, authService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "user-1"}, customEventAuth(ctx, false, true), nil).Times(1)

		_, err := service.ListEvents(ctx, &domain.ListCustomEventsRequest{
			WorkspaceID: "ws-1", Email: "a@example.com",
		})
		require.EqualError(t, err, "Insufficient permissions: read access to contacts required")
	})
}
