package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unscheduleDeps sets up a service whose two transactions run their functions with no real
// transaction: the Tx methods are mocked.
func unscheduleDeps(t *testing.T) (*broadcastSvcDeps, context.Context, *domain.UnscheduleBroadcastRequest) {
	d := setupBroadcastSvc(t)
	ctx := context.Background()
	req := &domain.UnscheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)
	d.taskRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fn func(*sql.Tx) error) error { return fn(nil) },
	).AnyTimes()
	d.repo.EXPECT().WithTransaction(gomock.Any(), req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	).AnyTimes()
	return d, ctx, req
}

// scheduledBroadcast is a broadcast booked for a date, not started.
func scheduledBroadcast() *domain.Broadcast {
	b := testBroadcast("w1", "b1")
	b.Status = domain.BroadcastStatusScheduled
	b.Schedule = domain.ScheduleSettings{
		IsScheduled:   true,
		ScheduledDate: "2030-01-15",
		ScheduledTime: "09:30",
		Timezone:      "Europe/Paris",
	}
	return b
}

// pendingTask is the task a schedule leaves: waiting for its date, never run.
func pendingTask() *domain.Task {
	broadcastID := "b1"
	return &domain.Task{
		ID:          "t1",
		WorkspaceID: "w1",
		Type:        "send_broadcast",
		Status:      domain.TaskStatusPending,
		BroadcastID: &broadcastID,
		CreatedAt:   time.Now().Add(-time.Hour),
	}
}

func requireSendStarted(t *testing.T, err error) {
	t.Helper()
	var validation domain.ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Contains(t, validation.Message, "cancel it instead")
}

func TestBroadcastService_UnscheduleBroadcast(t *testing.T) {
	t.Run("sends a scheduled broadcast back to draft, keeping its date, then deletes its task", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(scheduledBroadcast(), nil)
		d.repo.EXPECT().HasSendActivityTx(gomock.Any(), gomock.Any(), "b1").Return(false, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(pendingTask(), nil)
		// The tasks go while the broadcast is locked, before its draft is written; both commit
		// after, the workspace first.
		gomock.InOrder(
			d.taskRepo.EXPECT().DeleteBroadcastTasksTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(int64(1), nil),
			d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
					assert.Equal(t, domain.BroadcastStatusDraft, b.Status)
					assert.False(t, b.Schedule.IsScheduled)
					assert.Equal(t, "2030-01-15", b.Schedule.ScheduledDate)
					assert.Equal(t, "09:30", b.Schedule.ScheduledTime)
					assert.Equal(t, "Europe/Paris", b.Schedule.Timezone)
					return nil
				}),
		)

		require.NoError(t, d.svc.UnscheduleBroadcast(ctx, req))
	})

	t.Run("goes back to draft when the broadcast has no task", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(scheduledBroadcast(), nil)
		d.repo.EXPECT().HasSendActivityTx(gomock.Any(), gomock.Any(), "b1").Return(false, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, domain.ErrTaskNotFound)
		d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		d.taskRepo.EXPECT().DeleteBroadcastTasksTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(int64(0), nil)

		require.NoError(t, d.svc.UnscheduleBroadcast(ctx, req))
	})

	t.Run("refuses a scheduled broadcast with a start date", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		b := scheduledBroadcast()
		started := time.Now()
		b.StartedAt = &started
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)

		requireSendStarted(t, d.svc.UnscheduleBroadcast(ctx, req))
	})

	t.Run("refuses a scheduled broadcast with emails queued or sent, though it has no start date", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(scheduledBroadcast(), nil)
		d.repo.EXPECT().HasSendActivityTx(gomock.Any(), gomock.Any(), "b1").Return(true, nil)

		requireSendStarted(t, d.svc.UnscheduleBroadcast(ctx, req))
	})

	startedTasks := map[string]func(*domain.Task){
		"running":                    func(task *domain.Task) { task.Status = domain.TaskStatusRunning },
		"completed":                  func(task *domain.Task) { task.Status = domain.TaskStatusCompleted },
		"waiting under a live lease": func(task *domain.Task) { lease := time.Now().Add(time.Minute); task.TimeoutAfter = &lease },
		"run since it was scheduled": func(task *domain.Task) { run := time.Now(); task.LastRunAt = &run },
		"with recipients passed": func(task *domain.Task) {
			task.State = &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{RecipientOffset: 10}}
		},
		"with emails already enqueued": func(task *domain.Task) {
			task.State = &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{EnqueuedCount: 3}}
		},
	}
	for name, change := range startedTasks {
		t.Run("refuses when its task is "+name, func(t *testing.T) {
			d, ctx, req := unscheduleDeps(t)
			task := pendingTask()
			change(task)
			d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(scheduledBroadcast(), nil)
			d.repo.EXPECT().HasSendActivityTx(gomock.Any(), gomock.Any(), "b1").Return(false, nil)
			d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(task, nil)
			// No write, no deletion: the mocks fail on any UpdateBroadcastTx or DeleteBroadcastTasksTx.

			requireSendStarted(t, d.svc.UnscheduleBroadcast(ctx, req))
		})
	}

	t.Run("ignores a run older than the schedule, as a rescheduled task keeps it", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		task := pendingTask()
		earlier := task.CreatedAt.Add(-time.Hour)
		task.LastRunAt = &earlier
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(scheduledBroadcast(), nil)
		d.repo.EXPECT().HasSendActivityTx(gomock.Any(), gomock.Any(), "b1").Return(false, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(task, nil)
		d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		d.taskRepo.EXPECT().DeleteBroadcastTasksTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(int64(1), nil)

		require.NoError(t, d.svc.UnscheduleBroadcast(ctx, req))
	})

	t.Run("deletes the task left for a draft by an earlier call, without writing the broadcast", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		b := scheduledBroadcast()
		b.Status = domain.BroadcastStatusDraft
		b.Schedule.IsScheduled = false
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(pendingTask(), nil)
		d.taskRepo.EXPECT().DeleteBroadcastTasksTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(int64(1), nil)

		require.NoError(t, d.svc.UnscheduleBroadcast(ctx, req))
	})

	t.Run("refuses a draft with no task: it is not scheduled", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusDraft
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, domain.ErrTaskNotFound)

		err := d.svc.UnscheduleBroadcast(ctx, req)
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Contains(t, validation.Message, "not scheduled")
	})

	// A broadcast whose sending started (a scheduled one leaves scheduled when its sending
	// starts) is refused with the error that sends the console to Cancel.
	for _, status := range []domain.BroadcastStatus{
		domain.BroadcastStatusPaused,
		domain.BroadcastStatusProcessing,
		domain.BroadcastStatusProcessed,
		domain.BroadcastStatusTesting,
		domain.BroadcastStatusTestCompleted,
		domain.BroadcastStatusWinnerSelected,
	} {
		t.Run("refuses a broadcast "+string(status)+" as started", func(t *testing.T) {
			d, ctx, req := unscheduleDeps(t)
			b := testBroadcast("w1", "b1")
			b.Status = status
			started := time.Now().UTC().Add(-time.Hour)
			b.StartedAt = &started
			d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)

			err := d.svc.UnscheduleBroadcast(ctx, req)
			var validation domain.ValidationError
			require.ErrorAs(t, err, &validation)
			assert.Contains(t, validation.Message, "started sending")
		})
	}

	// A run's last attempt can pause a broadcast that never started: it is not told it did.
	t.Run("refuses a paused broadcast that never started, pointing to cancel", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		b := testBroadcast("w1", "b1")
		b.Status, b.StartedAt = domain.BroadcastStatusPaused, nil
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)

		err := d.svc.UnscheduleBroadcast(ctx, req)
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Contains(t, validation.Message, "cancel it instead")
		assert.NotContains(t, validation.Message, "started sending")
	})

	// Cancel refuses these too, so the error does not send the console there.
	for _, status := range []domain.BroadcastStatus{
		domain.BroadcastStatusCancelled,
		domain.BroadcastStatusFailed,
	} {
		t.Run("refuses a broadcast "+string(status), func(t *testing.T) {
			d, ctx, req := unscheduleDeps(t)
			b := testBroadcast("w1", "b1")
			b.Status = status
			d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(b, nil)

			err := d.svc.UnscheduleBroadcast(ctx, req)
			var validation domain.ValidationError
			require.ErrorAs(t, err, &validation)
			assert.Contains(t, validation.Message, string(status))
			assert.NotContains(t, validation.Message, "started sending")
		})
	}

	t.Run("returns a failed draft write: both transactions roll back, the task with them", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(scheduledBroadcast(), nil)
		d.repo.EXPECT().HasSendActivityTx(gomock.Any(), gomock.Any(), "b1").Return(false, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(pendingTask(), nil)
		d.taskRepo.EXPECT().DeleteBroadcastTasksTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(int64(1), nil)
		d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("db down"))

		err := d.svc.UnscheduleBroadcast(ctx, req)
		require.EqualError(t, err, "db down")
	})

	t.Run("writes no draft when the task deletion fails", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(scheduledBroadcast(), nil)
		d.repo.EXPECT().HasSendActivityTx(gomock.Any(), gomock.Any(), "b1").Return(false, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(pendingTask(), nil)
		d.taskRepo.EXPECT().DeleteBroadcastTasksTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(int64(0), errors.New("system db down"))
		// No UpdateBroadcastTx: the mock fails on it.

		require.EqualError(t, d.svc.UnscheduleBroadcast(ctx, req), "system db down")
	})

	t.Run("passes on a broadcast that does not exist", func(t *testing.T) {
		d, ctx, req := unscheduleDeps(t)
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, &domain.ErrBroadcastNotFound{ID: "b1"})

		var notFound *domain.ErrBroadcastNotFound
		require.ErrorAs(t, d.svc.UnscheduleBroadcast(ctx, req), &notFound)
	})
}

func TestBroadcastService_UnscheduleBroadcast_Access(t *testing.T) {
	t.Run("requires write access to broadcasts", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		authAs(d.authService, ctx, "w1", "member", domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true, Write: false},
		})

		err := d.svc.UnscheduleBroadcast(ctx, &domain.UnscheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1"})
		var permission *domain.PermissionError
		require.ErrorAs(t, err, &permission)
	})

	t.Run("passes on a failed authentication", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

		err := d.svc.UnscheduleBroadcast(ctx, &domain.UnscheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1"})
		require.ErrorContains(t, err, "failed to authenticate user")
	})

	t.Run("refuses a request without an id", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")

		err := d.svc.UnscheduleBroadcast(ctx, &domain.UnscheduleBroadcastRequest{WorkspaceID: "w1"})
		require.ErrorContains(t, err, "broadcast id is required")
	})
}
