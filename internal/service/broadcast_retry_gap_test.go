package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Gap coverage for RetryFailedRecipients / Pause / Resume / Cancel /
// PauseForCircuitBreaker uncovered branches. Guard tests set NO expectations
// on downstream mocks (queue, workspace, event bus, status update) — gomock's
// Finish then proves the guard rejected before touching them.

func TestBroadcastGap_RetryStatusGuardsNoDownstream(t *testing.T) {
	ctx := context.Background()
	for _, status := range []domain.BroadcastStatus{
		domain.BroadcastStatusDraft, domain.BroadcastStatusScheduled,
		domain.BroadcastStatusProcessing, domain.BroadcastStatusPaused,
		domain.BroadcastStatusCancelled, domain.BroadcastStatusTesting,
		domain.BroadcastStatusTestCompleted, domain.BroadcastStatusWinnerSelected,
	} {
		t.Run("status "+string(status), func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			authOK(d.authService, ctx, "w1")
			b := testBroadcast("w1", "b1")
			b.Status = status
			// Only GetBroadcast is expected: no GetSourceCounts, no GetByID,
			// no ResetTerminallyFailedBySource.
			d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)

			n, err := d.svc.RetryFailedRecipients(ctx, &domain.RetryFailedBroadcastRequest{WorkspaceID: "w1", ID: "b1"})
			assert.Equal(t, int64(0), n)
			var notAllowed *domain.ErrBroadcastRetryNotAllowed
			require.ErrorAs(t, err, &notAllowed)
			assert.Equal(t,
				"only a broadcast that has finished sending can be retried, current status: "+string(status),
				notAllowed.Reason)
			assert.EqualError(t, err, notAllowed.Reason)
		})
	}
}

func TestBroadcastGap_RetryFailedStatusRequeuesExactIntegration(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()
	ctx := context.Background()
	authOK(d.authService, ctx, "w1")
	b := testBroadcast("w1", "b1")
	b.Status = domain.BroadcastStatusFailed // second allowed status, not just Processed
	d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
	d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
		Return(&domain.EmailQueueSourceCounts{FailedTerminal: 7}, nil)
	d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(retryWorkspace("w1"), nil)
	d.emailQueueRepo.EXPECT().ResetTerminallyFailedBySource(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1", "integration-1").
		Return(int64(7), nil)

	n, err := d.svc.RetryFailedRecipients(ctx, &domain.RetryFailedBroadcastRequest{WorkspaceID: "w1", ID: "b1"})
	require.NoError(t, err)
	assert.Equal(t, int64(7), n)
}

func TestBroadcastGap_RetryDownstreamErrors(t *testing.T) {
	ctx := context.Background()
	req := &domain.RetryFailedBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	t.Run("auth failure wraps sentinel and touches nothing else", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		sentinel := errors.New("auth boom")
		d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, sentinel)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to authenticate user: auth boom")
	})

	t.Run("validation failure after auth touches no repository", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		n, err := d.svc.RetryFailedRecipients(ctx, &domain.RetryFailedBroadcastRequest{WorkspaceID: "w1", ID: ""})
		assert.Equal(t, int64(0), n)
		assert.EqualError(t, err, "broadcast id is required")
	})

	t.Run("permission denied is PermissionError and touches no repository", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authReadOnlyBroadcast(d.authService, ctx, "w1")
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
		assert.Equal(t, domain.PermissionResourceBroadcasts, permErr.Resource)
		assert.Equal(t, domain.PermissionTypeWrite, permErr.Permission)
		assert.EqualError(t, err, "Insufficient permissions: write access to broadcasts required")
	})

	t.Run("get broadcast failure propagates sentinel", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		sentinel := errors.New("broadcast gone")
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(nil, sentinel)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("queue counts failure wraps exactly and does not requeue", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
		sentinel := errors.New("counts boom")
		d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
			Return(nil, sentinel)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to read broadcast queue counts: counts boom")
	})

	t.Run("in-flight boundary processing=1 blocks with exact count", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
		// InFlight = Pending+Processing+Paused+FailedRetrying; here exactly 1.
		d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
			Return(&domain.EmailQueueSourceCounts{Processing: 1, FailedTerminal: 5}, nil)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		var notAllowed *domain.ErrBroadcastRetryNotAllowed
		require.ErrorAs(t, err, &notAllowed)
		assert.Equal(t, "this broadcast is still sending: 1 messages are still queued", notAllowed.Reason)
	})

	t.Run("paused queue rows also count as in-flight", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusFailed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
		d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
			Return(&domain.EmailQueueSourceCounts{Paused: 2, FailedRetrying: 1, FailedTerminal: 5}, nil)
		_, err := d.svc.RetryFailedRecipients(ctx, req)
		var notAllowed *domain.ErrBroadcastRetryNotAllowed
		require.ErrorAs(t, err, &notAllowed)
		assert.Equal(t, "this broadcast is still sending: 3 messages are still queued", notAllowed.Reason)
	})

	t.Run("workspace lookup failure propagates and does not requeue", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
		d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
			Return(&domain.EmailQueueSourceCounts{FailedTerminal: 3}, nil)
		sentinel := errors.New("workspace gone")
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(nil, sentinel)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("missing integration wraps provider error exactly", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
		d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
			Return(&domain.EmailQueueSourceCounts{FailedTerminal: 3}, nil)
		ws := &domain.Workspace{ID: "w1", Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "missing-9"}}
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(ws, nil)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		assert.EqualError(t, err, "failed to get email provider: integration with ID missing-9 not found")
	})

	t.Run("no provider is typed not-allowed with exact reason", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
		d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
			Return(&domain.EmailQueueSourceCounts{FailedTerminal: 3}, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(&domain.Workspace{ID: "w1"}, nil)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		var notAllowed *domain.ErrBroadcastRetryNotAllowed
		require.ErrorAs(t, err, &notAllowed)
		assert.Equal(t, "no marketing email provider configured for this workspace", notAllowed.Reason)
	})

	t.Run("requeue failure wraps exactly", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b, nil)
		d.emailQueueRepo.EXPECT().GetSourceCounts(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1").
			Return(&domain.EmailQueueSourceCounts{FailedTerminal: 3}, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(retryWorkspace("w1"), nil)
		sentinel := errors.New("reset boom")
		d.emailQueueRepo.EXPECT().ResetTerminallyFailedBySource(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "b1", "integration-1").
			Return(int64(0), sentinel)
		n, err := d.svc.RetryFailedRecipients(ctx, req)
		assert.Equal(t, int64(0), n)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to requeue failed recipients: reset boom")
	})
}

func TestBroadcastGap_PauseDownstreamFailures(t *testing.T) {
	ctx := context.Background()
	req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	t.Run("validation failure never opens a transaction", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		err := d.svc.PauseBroadcast(ctx, &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: ""})
		assert.EqualError(t, err, "broadcast id is required")
	})

	t.Run("status update failure prevents queue pause and event", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessing
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		sentinel := errors.New("update boom")
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(sentinel)
		// No PauseBySourceTx / PublishWithAck expectations.
		err := d.svc.PauseBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("event ack failure surfaces exact wrapped error", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessing
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, u *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusPaused, u.Status)
				assert.NotNil(t, u.PausedAt)
				return nil
			})
		d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(2), nil)
		sentinel := errors.New("evt boom")
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, p domain.EventPayload, ack domain.EventAckCallback) {
				assert.Equal(t, domain.EventBroadcastPaused, p.Type)
				assert.Equal(t, "w1", p.WorkspaceID)
				assert.Equal(t, "b1", p.EntityID)
				ack(sentinel)
			})
		err := d.svc.PauseBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to process pause event: evt boom")
	})
}

func TestBroadcastGap_ResumeBranchesAndFailures(t *testing.T) {
	ctx := context.Background()
	req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	t.Run("status guards reject without mutation", func(t *testing.T) {
		for _, status := range []domain.BroadcastStatus{
			domain.BroadcastStatusDraft, domain.BroadcastStatusProcessing,
			domain.BroadcastStatusProcessed, domain.BroadcastStatusCancelled,
			domain.BroadcastStatusFailed, domain.BroadcastStatusScheduled,
		} {
			d := setupBroadcastSvc(t)
			authOK(d.authService, ctx, "w1")
			inTx(d, ctx)
			b := testBroadcast("w1", "b1")
			b.Status = status
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
			err := d.svc.ResumeBroadcast(ctx, req)
			assert.EqualError(t, err, "only broadcasts with paused status can be resumed, current status: "+string(status))
			d.ctrl.Finish()
		}
	})

	t.Run("future schedule restores scheduled with start_now false", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusPaused
		b.Schedule.IsScheduled = true
		require.NoError(t, b.Schedule.SetScheduledDateTime(time.Now().UTC().Add(3*time.Hour), "UTC"))
		pausedAt := time.Now().UTC().Add(-time.Hour)
		b.PausedAt = &pausedAt
		reason := "manual"
		b.PauseReason = &reason
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, u *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusScheduled, u.Status)
				assert.Nil(t, u.PausedAt)
				assert.Nil(t, u.PauseReason)
				assert.Nil(t, u.StartedAt)
				return nil
			})
		d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(4), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, p domain.EventPayload, ack domain.EventAckCallback) {
				assert.Equal(t, domain.EventBroadcastResumed, p.Type)
				assert.Equal(t, false, p.Data["start_now"])
				ack(nil)
			})
		require.NoError(t, d.svc.ResumeBroadcast(ctx, req))
	})

	t.Run("past schedule starts now and stamps StartedAt", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusPaused
		b.Schedule.IsScheduled = true
		require.NoError(t, b.Schedule.SetScheduledDateTime(time.Now().UTC().Add(-2*time.Hour), "UTC"))
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, u *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusProcessing, u.Status)
				assert.NotNil(t, u.StartedAt)
				return nil
			})
		d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(1), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, p domain.EventPayload, ack domain.EventAckCallback) {
				assert.Equal(t, true, p.Data["start_now"])
				ack(nil)
			})
		require.NoError(t, d.svc.ResumeBroadcast(ctx, req))
	})

	t.Run("future schedule but already started resumes to processing", func(t *testing.T) {
		// Pin: isScheduledInFuture requires StartedAt==nil; a started broadcast
		// must not be put back to scheduled even if its time is in the future.
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusPaused
		b.Schedule.IsScheduled = true
		require.NoError(t, b.Schedule.SetScheduledDateTime(time.Now().UTC().Add(3*time.Hour), "UTC"))
		started := time.Now().UTC().Add(-time.Hour)
		b.StartedAt = &started
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, u *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusProcessing, u.Status)
				assert.Equal(t, started, *u.StartedAt) // not overwritten
				return nil
			})
		d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(1), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, p domain.EventPayload, ack domain.EventAckCallback) {
				assert.Equal(t, true, p.Data["start_now"])
				ack(nil)
			})
		require.NoError(t, d.svc.ResumeBroadcast(ctx, req))
	})

	t.Run("queue resume failure aborts before event", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusPaused
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		sentinel := errors.New("queue locked")
		d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").
			Return(int64(0), sentinel)
		err := d.svc.ResumeBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("event ack failure surfaces exact wrapped error", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusPaused
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(1), nil)
		sentinel := errors.New("evt boom")
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(sentinel) })
		err := d.svc.ResumeBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to process resume event: evt boom")
	})
}

func TestBroadcastGap_CancelBranchesAndFailures(t *testing.T) {
	ctx := context.Background()
	req := &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	t.Run("status guards reject without mutation", func(t *testing.T) {
		for _, status := range []domain.BroadcastStatus{
			domain.BroadcastStatusDraft, domain.BroadcastStatusFailed,
			domain.BroadcastStatusTesting, domain.BroadcastStatusTestCompleted,
			domain.BroadcastStatusWinnerSelected,
		} {
			d := setupBroadcastSvc(t)
			authOK(d.authService, ctx, "w1")
			inTx(d, ctx)
			b := testBroadcast("w1", "b1")
			b.Status = status
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
			err := d.svc.CancelBroadcast(ctx, req)
			assert.EqualError(t, err,
				"only broadcasts with scheduled, paused, processing, or processed status can be cancelled, current status: "+string(status))
			d.ctrl.Finish()
		}
	})

	t.Run("paused broadcast cancels with exact queue mutation and event", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusPaused
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, u *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusCancelled, u.Status)
				assert.NotNil(t, u.CancelledAt)
				return nil
			})
		d.emailQueueRepo.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(9), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, p domain.EventPayload, ack domain.EventAckCallback) {
				assert.Equal(t, domain.EventBroadcastCancelled, p.Type)
				assert.Equal(t, "b1", p.Data["broadcast_id"])
				ack(nil)
			})
		require.NoError(t, d.svc.CancelBroadcast(ctx, req))
	})

	t.Run("status update failure prevents queue delete and event", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusProcessing
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		sentinel := errors.New("update boom")
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(sentinel)
		err := d.svc.CancelBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("event ack failure surfaces exact wrapped error", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		inTx(d, ctx)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusScheduled
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		d.emailQueueRepo.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(1), nil)
		sentinel := errors.New("evt boom")
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(sentinel) })
		err := d.svc.CancelBroadcast(ctx, req)
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to process cancel event: evt boom")
	})

	t.Run("validation failure never opens a transaction", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		authOK(d.authService, ctx, "w1")
		err := d.svc.CancelBroadcast(ctx, &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: ""})
		assert.EqualError(t, err, "broadcast id is required")
	})
}

func TestBroadcastGap_PauseForCircuitBreakerFailures(t *testing.T) {
	t.Run("list failure wraps exactly and loads nothing", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		sentinel := errors.New("list boom")
		d.emailQueueRepo.EXPECT().ListActiveSourcesByIntegration(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "intg-1").
			Return(nil, sentinel)
		err := d.svc.PauseForCircuitBreaker(context.Background(), "w1", "intg-1", "down")
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to list broadcasts affected by the open circuit: list boom")
	})

	t.Run("non-sending statuses are skipped without transaction or events", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		d.emailQueueRepo.EXPECT().ListActiveSourcesByIntegration(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "intg-1").
			Return([]string{"b1", "b2", "b3"}, nil)
		for i, status := range []domain.BroadcastStatus{
			domain.BroadcastStatusDraft, domain.BroadcastStatusScheduled, domain.BroadcastStatusCancelled,
		} {
			b := testBroadcast("w1", []string{"b1", "b2", "b3"}[i])
			b.Status = status
			d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", b.ID).Return(b, nil)
		}
		// No WithTransaction / Publish expectations.
		err := d.svc.PauseForCircuitBreaker(context.Background(), "w1", "intg-1", "down")
		require.NoError(t, err)
	})

	t.Run("transaction failure for one broadcast does not stop the next", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		d.emailQueueRepo.EXPECT().ListActiveSourcesByIntegration(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "intg-1").
			Return([]string{"b1", "b2"}, nil)
		b1 := testBroadcast("w1", "b1")
		b1.Status = domain.BroadcastStatusProcessing
		b2 := testBroadcast("w1", "b2")
		b2.Status = domain.BroadcastStatusProcessed
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b1, nil)
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b2").Return(b2, nil)
		d.repo.EXPECT().WithTransaction(gomock.Any(), "w1", gomock.Any()).Times(2).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
		sentinel := errors.New("update boom")
		gomock.InOrder(
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(sentinel),
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ *sql.Tx, u *domain.Broadcast) error {
					assert.Equal(t, "b2", u.ID)
					assert.Equal(t, domain.BroadcastStatusPaused, u.Status)
					require.NotNil(t, u.PauseReason)
					assert.Equal(t, "Circuit breaker triggered: down", *u.PauseReason)
					return nil
				}),
		)
		// Only b2 reaches the queue pause.
		d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b2").
			Return(int64(1), nil)
		var published []domain.EventPayload
		d.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(2).
			Do(func(_ context.Context, p domain.EventPayload) { published = append(published, p) })
		err := d.svc.PauseForCircuitBreaker(context.Background(), "w1", "intg-1", "down")
		require.NoError(t, err)
		require.Len(t, published, 2)
		assert.Equal(t, domain.EventBroadcastCircuitBreaker, published[0].Type)
		assert.Equal(t, "b2", published[0].EntityID)
		assert.Equal(t, "Circuit breaker triggered: down", published[0].Data["reason"])
		assert.Equal(t, "down", published[0].Data["error"])
		assert.Equal(t, domain.EventBroadcastPaused, published[1].Type)
		assert.Equal(t, "circuit_breaker", published[1].Data["reason"])
	})

	t.Run("queue pause failure inside tx is swallowed and next broadcast proceeds", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		d.emailQueueRepo.EXPECT().ListActiveSourcesByIntegration(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "intg-1").
			Return([]string{"b1"}, nil)
		b1 := testBroadcast("w1", "b1")
		b1.Status = domain.BroadcastStatusProcessing
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b1, nil)
		d.repo.EXPECT().WithTransaction(gomock.Any(), "w1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		sentinel := errors.New("queue locked")
		d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").
			Return(int64(0), sentinel)
		// No Publish: pause failed, so no alert.
		err := d.svc.PauseForCircuitBreaker(context.Background(), "w1", "intg-1", "down")
		require.NoError(t, err)
	})

	t.Run("nil event bus still pauses without alerting", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		d.svc.eventBus = nil
		d.emailQueueRepo.EXPECT().ListActiveSourcesByIntegration(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "intg-1").
			Return([]string{"b1"}, nil)
		b1 := testBroadcast("w1", "b1")
		b1.Status = domain.BroadcastStatusProcessing
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(b1, nil)
		d.repo.EXPECT().WithTransaction(gomock.Any(), "w1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, u *domain.Broadcast) error {
				assert.Equal(t, domain.BroadcastStatusPaused, u.Status)
				assert.NotNil(t, u.PausedAt)
				return nil
			})
		d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").
			Return(int64(3), nil)
		err := d.svc.PauseForCircuitBreaker(context.Background(), "w1", "intg-1", "down")
		require.NoError(t, err)
	})
}
