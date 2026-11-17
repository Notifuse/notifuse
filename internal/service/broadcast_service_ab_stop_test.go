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

// A test waiting for its winner does not count the time it spent paused: the resume adds it
// to test_paused_seconds, through a write that holds only while the broadcast is still the
// paused one this resume read, so two resumes add it once.
func TestBroadcastService_ResumeBroadcast_ABTestCountsItsPause(t *testing.T) {
	testSent := time.Now().UTC().Add(-3 * time.Hour)
	pausedAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	winner := "tplB"
	completed := time.Now().UTC().Add(-time.Minute)

	for name, tc := range map[string]struct {
		edit    func(b *domain.Broadcast)
		counted bool
	}{
		"waiting for its winner: the pause is counted":  {func(b *domain.Broadcast) {}, true},
		"a winner chosen: not counted":                  {func(b *domain.Broadcast) { b.WinningTemplate = &winner }, false},
		"the test under way: its clock has not started": {func(b *domain.Broadcast) { b.TestSentAt = nil }, false},
		"every recipient queued: not counted":           {func(b *domain.Broadcast) { b.CompletedAt = &completed }, false},
		"the test turned off: not counted":              {func(b *domain.Broadcast) { b.TestSettings.Enabled = false }, false},
		"no pause date: nothing to count":               {func(b *domain.Broadcast) { b.PausedAt = nil }, false},
	} {
		t.Run(name, func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			ctx := context.Background()
			req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
			authOK(d.authService, ctx, req.WorkspaceID)
			noTaskStateYet(d)
			inTransaction(d, req.WorkspaceID)

			b := abTestBroadcast(domain.BroadcastStatusPaused)
			b.TestSettings.AutoSendWinner, b.TestSettings.TestDurationHours = true, 1
			sent, paused := testSent, pausedAt
			b.TestSentAt, b.PausedAt = &sent, &paused
			tc.edit(b)
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
			var seconds int
			if tc.counted {
				d.repo.EXPECT().AddTestPauseTx(gomock.Any(), gomock.Any(), "w1", "b1", pausedAt, gomock.Any()).DoAndReturn(
					func(_ context.Context, _ *sql.Tx, _, _ string, _ time.Time, s int) (bool, error) {
						seconds = s
						return true, nil
					})
			} else {
				d.repo.EXPECT().AddTestPauseTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			}
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(2), nil)
			ackEvent(d, nil)

			require.NoError(t, d.svc.ResumeBroadcast(ctx, req))
			if tc.counted {
				assert.InDelta(t, (2 * time.Hour).Seconds(), float64(seconds), 60, "the time paused, in seconds")
				assert.Equal(t, seconds, b.TestPausedSeconds, "the copy carries it")
				assert.Equal(t, testSent, *b.TestSentAt, "the test date is kept")
			}
		})
	}
}

// The resume reads the broadcast without its lock: a second resume finds the pause already
// counted and is refused, with nothing else written.
func TestBroadcastService_ResumeBroadcast_ABTestPauseCountedOnce(t *testing.T) {
	for name, tc := range map[string]struct {
		written bool
		err     error
	}{
		"another resume went first": {false, nil},
		"the write fails":           {false, assert.AnError},
	} {
		t.Run(name, func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()
			ctx := context.Background()
			req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
			authOK(d.authService, ctx, req.WorkspaceID)
			noTaskStateYet(d)
			inTransaction(d, req.WorkspaceID)

			b := abTestBroadcast(domain.BroadcastStatusPaused)
			testSent, paused := time.Now().UTC().Add(-3*time.Hour), time.Now().UTC().Add(-time.Hour)
			b.TestSentAt, b.PausedAt = &testSent, &paused
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
			d.repo.EXPECT().AddTestPauseTx(gomock.Any(), gomock.Any(), "w1", "b1", paused, gomock.Any()).Return(tc.written, tc.err)
			d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			err := d.svc.ResumeBroadcast(ctx, req)
			require.Error(t, err)
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
			} else {
				var validation domain.ValidationError
				require.ErrorAs(t, err, &validation)
				assert.Contains(t, validation.Message, "no longer paused")
			}
		})
	}
}
