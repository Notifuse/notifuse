package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An A/B test broadcast can be paused and cancelled during its test (testing), while it
// waits for its winner (test_completed) and while the winner is sent (winner_selected), as a
// broadcast sending a single template can; the circuit breaker pauses it in those statuses
// too. Resumed, it goes back to the step it was at.
//
// Plan: Notifuse/architecture plans/notifuse/fix-scheduled-ab-broadcasts-plan.md

var abSendingStatuses = []domain.BroadcastStatus{
	domain.BroadcastStatusTesting,
	domain.BroadcastStatusTestCompleted,
	domain.BroadcastStatusWinnerSelected,
}

func abTestBroadcast(status domain.BroadcastStatus) *domain.Broadcast {
	b := testBroadcast("w1", "b1")
	b.Status = status
	b.TestSettings = domain.BroadcastTestSettings{
		Enabled:          true,
		SamplePercentage: 20,
		Variations: []domain.BroadcastVariation{
			{VariationName: "A", TemplateID: "tplA"},
			{VariationName: "B", TemplateID: "tplB"},
		},
	}
	started := time.Now().UTC().Add(-time.Hour)
	b.StartedAt = &started
	return b
}

func inTransaction(d *broadcastSvcDeps, workspaceID string) {
	d.repo.EXPECT().WithTransaction(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
}

func ackEvent(d *broadcastSvcDeps, got *domain.EventPayload) {
	d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, event domain.EventPayload, ack domain.EventAckCallback) {
			if got != nil {
				*got = event
			}
			ack(nil)
		},
	)
}

func TestBroadcastService_PauseBroadcast_ABTest(t *testing.T) {
	for _, status := range abSendingStatuses {
		t.Run("pauses a broadcast in "+string(status)+" and its queue", func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			ctx := context.Background()
			req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
			authOK(d.authService, ctx, req.WorkspaceID)
			inTransaction(d, req.WorkspaceID)

			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(abTestBroadcast(status), nil)
			var written *domain.Broadcast
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
					written = b
					return nil
				})
			d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(3), nil)
			var event domain.EventPayload
			ackEvent(d, &event)

			require.NoError(t, d.svc.PauseBroadcast(ctx, req))
			require.NotNil(t, written)
			assert.Equal(t, domain.BroadcastStatusPaused, written.Status)
			assert.NotNil(t, written.PausedAt)
			assert.Equal(t, domain.EventBroadcastPaused, event.Type)
		})
	}

	for _, status := range []domain.BroadcastStatus{
		domain.BroadcastStatusDraft,
		domain.BroadcastStatusScheduled,
		domain.BroadcastStatusPaused,
		domain.BroadcastStatusCancelled,
		domain.BroadcastStatusFailed,
	} {
		t.Run("refuses a broadcast in "+string(status), func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			ctx := context.Background()
			req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
			authOK(d.authService, ctx, req.WorkspaceID)
			inTransaction(d, req.WorkspaceID)
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(abTestBroadcast(status), nil)

			err := d.svc.PauseBroadcast(ctx, req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "only broadcasts that are sending can be paused")
			assert.Contains(t, err.Error(), "current status: "+string(status))
		})
	}
}

func TestBroadcastService_CancelBroadcast_ABTest(t *testing.T) {
	for _, status := range abSendingStatuses {
		t.Run("cancels a broadcast in "+string(status)+" and removes its queue", func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			ctx := context.Background()
			req := &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
			authOK(d.authService, ctx, req.WorkspaceID)
			inTransaction(d, req.WorkspaceID)

			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(abTestBroadcast(status), nil)
			var written *domain.Broadcast
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
					written = b
					return nil
				})
			d.emailQueueRepo.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(5), nil)
			var event domain.EventPayload
			ackEvent(d, &event)

			require.NoError(t, d.svc.CancelBroadcast(ctx, req))
			require.NotNil(t, written)
			assert.Equal(t, domain.BroadcastStatusCancelled, written.Status)
			assert.NotNil(t, written.CancelledAt)
			assert.Equal(t, domain.EventBroadcastCancelled, event.Type)
		})
	}

	for _, status := range []domain.BroadcastStatus{
		domain.BroadcastStatusDraft,
		domain.BroadcastStatusCancelled,
		domain.BroadcastStatusFailed,
	} {
		t.Run("refuses a broadcast in "+string(status), func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			ctx := context.Background()
			req := &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
			authOK(d.authService, ctx, req.WorkspaceID)
			inTransaction(d, req.WorkspaceID)
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(abTestBroadcast(status), nil)

			err := d.svc.CancelBroadcast(ctx, req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "only broadcasts that are scheduled, paused or sending can be cancelled")
			assert.Contains(t, err.Error(), "current status: "+string(status))
		})
	}
}

func TestBroadcastService_PauseForCircuitBreaker_ABTest(t *testing.T) {
	for _, status := range abSendingStatuses {
		t.Run("pauses a broadcast in "+string(status), func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()

			d.emailQueueRepo.EXPECT().
				ListActiveSourcesByIntegration(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "intg-1").
				Return([]string{"b1"}, nil)
			d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(abTestBroadcast(status), nil)
			inTransaction(d, "w1")
			var written *domain.Broadcast
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
					written = b
					return nil
				})
			d.emailQueueRepo.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(4), nil)
			d.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(2)

			require.NoError(t, d.svc.PauseForCircuitBreaker(context.Background(), "w1", "intg-1", "quota exceeded"))
			require.NotNil(t, written)
			assert.Equal(t, domain.BroadcastStatusPaused, written.Status)
		})
	}

	t.Run("leaves a scheduled broadcast alone", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()

		d.emailQueueRepo.EXPECT().
			ListActiveSourcesByIntegration(gomock.Any(), "w1", domain.EmailQueueSourceBroadcast, "intg-1").
			Return([]string{"b1"}, nil)
		d.repo.EXPECT().GetBroadcast(gomock.Any(), "w1", "b1").Return(abTestBroadcast(domain.BroadcastStatusScheduled), nil)

		require.NoError(t, d.svc.PauseForCircuitBreaker(context.Background(), "w1", "intg-1", "quota exceeded"))
	})
}

func TestBroadcastService_ResumeBroadcast_ABTest(t *testing.T) {
	winner := "tplB"
	testSent := time.Now().UTC().Add(-30 * time.Minute)
	future := time.Now().UTC().Add(48 * time.Hour)

	cases := []struct {
		name         string
		broadcast    func() *domain.Broadcast
		wantStatus   domain.BroadcastStatus
		wantStartNow bool
	}{
		{
			name: "a winner chosen: winner_selected",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.WinningTemplate, b.TestSentAt = &winner, &testSent
				return b
			},
			wantStatus: domain.BroadcastStatusWinnerSelected, wantStartNow: true,
		},
		{
			name: "a winner chosen and no start date: winner_selected",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.WinningTemplate, b.TestSentAt, b.StartedAt = &winner, &testSent, nil
				return b
			},
			wantStatus: domain.BroadcastStatusWinnerSelected, wantStartNow: true,
		},
		{
			name: "the test sent and no winner yet: test_completed",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.TestSentAt = &testSent
				return b
			},
			wantStatus: domain.BroadcastStatusTestCompleted, wantStartNow: true,
		},
		{
			name: "the test sent, no winner and no start date: test_completed",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.TestSentAt, b.StartedAt = &testSent, nil
				return b
			},
			wantStatus: domain.BroadcastStatusTestCompleted, wantStartNow: true,
		},
		{
			name: "the test under way: testing",
			broadcast: func() *domain.Broadcast {
				return abTestBroadcast(domain.BroadcastStatusPaused)
			},
			wantStatus: domain.BroadcastStatusTesting, wantStartNow: true,
		},
		{
			name: "the test under way of a broadcast scheduled for a date to come: testing",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.Schedule = domain.ScheduleSettings{
					IsScheduled:   true,
					ScheduledDate: future.Format("2006-01-02"),
					ScheduledTime: future.Format("15:04"),
					Timezone:      "UTC",
				}
				return b
			},
			wantStatus: domain.BroadcastStatusTesting, wantStartNow: true,
		},
		{
			name: "never started, scheduled for a date to come: scheduled",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.StartedAt = nil
				b.Schedule = domain.ScheduleSettings{
					IsScheduled:   true,
					ScheduledDate: future.Format("2006-01-02"),
					ScheduledTime: future.Format("15:04"),
					Timezone:      "UTC",
				}
				return b
			},
			wantStatus: domain.BroadcastStatusScheduled, wantStartNow: false,
		},
		{
			name: "never started, sent right away: processing",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.StartedAt = nil
				return b
			},
			wantStatus: domain.BroadcastStatusProcessing, wantStartNow: true,
		},
		{
			name: "every recipient queued: processed",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.WinningTemplate, b.TestSentAt = &winner, &testSent
				completed := time.Now().UTC().Add(-time.Minute)
				b.CompletedAt = &completed
				return b
			},
			wantStatus: domain.BroadcastStatusProcessed, wantStartNow: false,
		},
		{
			name: "no A/B test, started: processing",
			broadcast: func() *domain.Broadcast {
				b := abTestBroadcast(domain.BroadcastStatusPaused)
				b.TestSettings.Enabled = false
				b.TestSentAt = &testSent
				return b
			},
			wantStatus: domain.BroadcastStatusProcessing, wantStartNow: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			ctx := context.Background()
			req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
			authOK(d.authService, ctx, req.WorkspaceID)
			noTaskStateYet(d)
			inTransaction(d, req.WorkspaceID)

			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(tc.broadcast(), nil)
			var written *domain.Broadcast
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
					written = b
					return nil
				})
			d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(2), nil)
			var event domain.EventPayload
			ackEvent(d, &event)

			require.NoError(t, d.svc.ResumeBroadcast(ctx, req))
			require.NotNil(t, written)
			assert.Equal(t, tc.wantStatus, written.Status)
			assert.Nil(t, written.PausedAt)
			assert.Equal(t, domain.EventBroadcastResumed, event.Type)
			startNow, _ := event.Data["start_now"].(bool)
			assert.Equal(t, tc.wantStartNow, startNow)
		})
	}
}
