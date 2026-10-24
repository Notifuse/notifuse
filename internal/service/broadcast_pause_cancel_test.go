package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
)

// Pause/cancel state guards: only in-flight broadcasts may pause, only
// not-yet-terminal ones may cancel, and the queue mutation happens inside
// the same transaction as the status change — a failure anywhere aborts
// the whole operation rather than leaving status and queue disagreeing.

func authReadOnlyBroadcast(auth *domainmocks.MockAuthService, ctx context.Context, workspaceID string) {
	userWorkspace := &domain.UserWorkspace{
		UserID:      "user1",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true, Write: false},
		},
	}
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
		Return(ctx, &domain.User{ID: "user1"}, userWorkspace, nil)
}

func inTx(d *broadcastSvcDeps, ctx context.Context) {
	d.repo.EXPECT().WithTransaction(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
}

func TestBroadcastService_PauseBroadcast_StatusGuard(t *testing.T) {
	ctx := context.Background()
	req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	for _, status := range []domain.BroadcastStatus{
		domain.BroadcastStatusDraft, domain.BroadcastStatusScheduled,
		domain.BroadcastStatusPaused, domain.BroadcastStatusCancelled,
		domain.BroadcastStatusProcessed, // pauseable: excluded below
	} {
		if status == domain.BroadcastStatusProcessed {
			continue
		}
		t.Run("status "+string(status)+" cannot pause", func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			authOK(d.authService, ctx, "w1")
			inTx(d, ctx)
			b := testBroadcast("w1", "b1")
			b.Status = status
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)

			err := d.svc.PauseBroadcast(ctx, req)
			require.EqualError(t, err,
				"only broadcasts with sending or processed status can be paused, current status: "+string(status))
		})
	}

	t.Run("a processed broadcast with queued remainder can pause", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusPaused, updated.Status)
				assert.NotNil(t, updated.PausedAt)
				return nil
			})
		d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(),
			domain.EmailQueueSourceBroadcast, "b1").Return(int64(3), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })

		require.NoError(t, d.svc.PauseBroadcast(ctx, req))
	})
}

func TestBroadcastService_PauseBroadcast_Failures(t *testing.T) {
	ctx := context.Background()
	req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	t.Run("read-only users cannot pause", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authReadOnlyBroadcast(d.authService, ctx, "w1")
		err := d.svc.PauseBroadcast(ctx, req)
		require.EqualError(t, err, "Insufficient permissions: write access to broadcasts required")
	})

	t.Run("missing broadcast aborts before any mutation", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		sentinel := errors.New("not found")
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, sentinel)
		err := d.svc.PauseBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("queue pause failure aborts the transaction", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessing
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		sentinel := errors.New("queue locked")
		d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(),
			domain.EmailQueueSourceBroadcast, "b1").Return(int64(0), sentinel)

		err := d.svc.PauseBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel,
			"status change and queue pause commit or fail together")
	})
}

func TestBroadcastService_CancelBroadcast_StatusGuard(t *testing.T) {
	ctx := context.Background()
	req := &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	t.Run("a completed broadcast cannot be cancelled", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusCancelled
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)

		err := d.svc.CancelBroadcast(ctx, req)
		require.EqualError(t, err,
			"only broadcasts with scheduled, paused, processing, or processed status can be cancelled, current status: cancelled")
	})

	t.Run("a scheduled broadcast cancels and its queue rows are deleted in-tx", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusScheduled
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusCancelled, updated.Status)
				assert.NotNil(t, updated.CancelledAt)
				return nil
			})
		d.emailQueueRepo.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(),
			domain.EmailQueueSourceBroadcast, "b1").Return(int64(5), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })

		require.NoError(t, d.svc.CancelBroadcast(ctx, req))
	})

	t.Run("queue deletion failure aborts the cancel", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusPaused
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		sentinel := errors.New("queue locked")
		d.emailQueueRepo.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(),
			domain.EmailQueueSourceBroadcast, "b1").Return(int64(0), sentinel)

		err := d.svc.CancelBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
	})
}
