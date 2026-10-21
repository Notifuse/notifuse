//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rules that keep a second execution of a task from starting while one
// holds it, checked against real PostgreSQL timestamps. A run holds its task
// while the task is running and its lease (timeout_after) has not expired.
// Nothing outside the run may wake or pause such a task, and the launch gate
// refuses a waiting task that still carries a live lease.
//
// Plan: plans/fix-broadcast-duplicate-sends-plan.md

type taskLeaseHarness struct {
	suite       *testutil.IntegrationTestSuite
	repo        domain.TaskRepository
	workspaceID string
}

func setupTaskLease(t *testing.T) *taskLeaseHarness {
	t.Helper()
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	t.Cleanup(testutil.CleanupTestEnvironment)

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	t.Cleanup(suite.Cleanup)

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	return &taskLeaseHarness{
		suite:       suite,
		repo:        suite.ServerManager.GetApp().GetTaskRepository(),
		workspaceID: workspace.ID,
	}
}

// newTask creates a pending task with a cursor in its state, so a test can tell
// whether a write kept or replaced what a run saved.
func (h *taskLeaseHarness) newTask(t *testing.T) string {
	t.Helper()
	now := time.Now().UTC()
	task := &domain.Task{
		WorkspaceID:   h.workspaceID,
		Type:          "lease-test",
		Status:        domain.TaskStatusPending,
		NextRunAfter:  &now,
		MaxRuntime:    50,
		MaxRetries:    3,
		RetryInterval: 60,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{LastProcessedEmail: "start@example.com"},
		},
	}
	require.NoError(t, h.repo.Create(context.Background(), h.workspaceID, task))
	return task.ID
}

// set writes the columns that define who holds a task, as an earlier write
// (a run's claim, a crash, an external update) would have left them.
func (h *taskLeaseHarness) set(t *testing.T, id string, status domain.TaskStatus, lease *time.Time) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Minute)
	_, err := h.suite.DBManager.GetDB().Exec(
		`UPDATE tasks SET status = $1, timeout_after = $2, next_run_after = $3 WHERE id = $4`,
		string(status), lease, past, id)
	require.NoError(t, err)
}

func (h *taskLeaseHarness) get(t *testing.T, id string) *domain.Task {
	t.Helper()
	task, err := h.repo.Get(context.Background(), h.workspaceID, id)
	require.NoError(t, err)
	return task
}

// selected reports whether the scheduler's next batch would pick the task.
func (h *taskLeaseHarness) selected(t *testing.T, id string) bool {
	t.Helper()
	batch, err := h.repo.GetNextBatch(context.Background(), 1000)
	require.NoError(t, err)
	for _, task := range batch {
		if task.ID == id {
			return true
		}
	}
	return false
}

func liveLease() *time.Time {
	lease := time.Now().UTC().Add(time.Minute)
	return &lease
}

func expiredLease() *time.Time {
	lease := time.Now().UTC().Add(-time.Second)
	return &lease
}

func TestTaskLease_HeldTaskIsNeitherWokenNorPausedNorTakenAgain(t *testing.T) {
	h := setupTaskLease(t)
	ctx := context.Background()
	id := h.newTask(t)

	require.NoError(t, h.repo.MarkAsRunning(ctx, h.workspaceID, id, *liveLease()))
	claimed := h.get(t, id)
	require.NotNil(t, claimed.TimeoutAfter)

	woken, err := h.repo.WakeTask(ctx, h.workspaceID, id, time.Now().UTC())
	require.NoError(t, err)
	assert.False(t, woken, "a task whose run holds it must not be woken")

	err = h.repo.MarkAsPaused(ctx, h.workspaceID, id, time.Now().UTC().Add(24*time.Hour), 0, claimed.State)
	var alreadyRunning *domain.ErrTaskAlreadyRunning
	assert.ErrorAs(t, err, &alreadyRunning, "a task whose run holds it must not be paused")

	err = h.repo.MarkAsRunning(ctx, h.workspaceID, id, *liveLease())
	assert.ErrorAs(t, err, &alreadyRunning, "a held task must not be launched a second time")

	after := h.get(t, id)
	assert.Equal(t, domain.TaskStatusRunning, after.Status)
	require.NotNil(t, after.TimeoutAfter)
	assert.WithinDuration(t, *claimed.TimeoutAfter, *after.TimeoutAfter, time.Millisecond,
		"the run's lease must be left as it was")

	// Once the lease has expired the run is presumed dead and the task can be
	// woken or paused like any waiting task.
	h.set(t, id, domain.TaskStatusRunning, expiredLease())
	woken, err = h.repo.WakeTask(ctx, h.workspaceID, id, time.Now().UTC())
	require.NoError(t, err)
	assert.True(t, woken)
	assert.Equal(t, domain.TaskStatusPending, h.get(t, id).Status)

	h.set(t, id, domain.TaskStatusRunning, expiredLease())
	require.NoError(t, h.repo.MarkAsPaused(ctx, h.workspaceID, id, time.Now().UTC().Add(24*time.Hour), 0, claimed.State))
	paused := h.get(t, id)
	assert.Equal(t, domain.TaskStatusPaused, paused.Status)
	assert.Nil(t, paused.TimeoutAfter)
}

func TestTaskLease_RunningTaskWithoutALeaseCountsAsHeld(t *testing.T) {
	h := setupTaskLease(t)
	ctx := context.Background()
	id := h.newTask(t)
	h.set(t, id, domain.TaskStatusRunning, nil)

	woken, err := h.repo.WakeTask(ctx, h.workspaceID, id, time.Now().UTC())
	require.NoError(t, err)
	assert.False(t, woken, "with no lease there is no proof the run has ended")

	err = h.repo.MarkAsPaused(ctx, h.workspaceID, id, time.Now().UTC().Add(24*time.Hour), 0, &domain.TaskState{})
	var alreadyRunning *domain.ErrTaskAlreadyRunning
	assert.ErrorAs(t, err, &alreadyRunning)
	assert.Equal(t, domain.TaskStatusRunning, h.get(t, id).Status)
}

// TestTaskLease_WaitingTaskWithALiveLeaseIsNotLaunched covers a write that puts
// a running task back to pending or paused and copies the lease it read, as
// Update does: the scheduler must neither pick nor launch it until the run's
// own end clears the lease.
func TestTaskLease_WaitingTaskWithALiveLeaseIsNotLaunched(t *testing.T) {
	h := setupTaskLease(t)
	ctx := context.Background()

	for _, status := range []domain.TaskStatus{domain.TaskStatusPending, domain.TaskStatusPaused} {
		t.Run(string(status), func(t *testing.T) {
			id := h.newTask(t)
			h.set(t, id, status, liveLease())

			assert.False(t, h.selected(t, id), "the scheduler must not pick a task whose lease is live")
			err := h.repo.MarkAsRunning(ctx, h.workspaceID, id, *liveLease())
			var alreadyRunning *domain.ErrTaskAlreadyRunning
			assert.ErrorAs(t, err, &alreadyRunning, "the launch gate must refuse a task whose lease is live")

			// The run that held it ends its slice: its MarkAsPending clears the lease.
			require.NoError(t, h.repo.MarkAsPending(ctx, h.workspaceID, id, time.Now().UTC().Add(-time.Second), 0, h.get(t, id).State))
			assert.True(t, h.selected(t, id))
			require.NoError(t, h.repo.MarkAsRunning(ctx, h.workspaceID, id, *liveLease()))
		})
	}

	t.Run("expired lease", func(t *testing.T) {
		id := h.newTask(t)
		h.set(t, id, domain.TaskStatusPending, expiredLease())
		assert.True(t, h.selected(t, id))
		require.NoError(t, h.repo.MarkAsRunning(ctx, h.workspaceID, id, *liveLease()))
	})
}

func TestTaskLease_WakeTask(t *testing.T) {
	h := setupTaskLease(t)
	ctx := context.Background()

	for _, status := range []domain.TaskStatus{domain.TaskStatusPending, domain.TaskStatusPaused, domain.TaskStatusFailed} {
		t.Run("wakes a "+string(status)+" task", func(t *testing.T) {
			id := h.newTask(t)
			h.set(t, id, status, nil)
			_, err := h.suite.DBManager.GetDB().Exec(
				`UPDATE tasks SET retry_count = 2, error_message = 'boom' WHERE id = $1`, id)
			require.NoError(t, err)

			due := time.Now().UTC().Add(time.Second).Truncate(time.Microsecond)
			woken, err := h.repo.WakeTask(ctx, h.workspaceID, id, due)
			require.NoError(t, err)
			assert.True(t, woken)

			task := h.get(t, id)
			assert.Equal(t, domain.TaskStatusPending, task.Status)
			require.NotNil(t, task.NextRunAfter)
			assert.WithinDuration(t, due, *task.NextRunAfter, time.Millisecond)
			assert.Zero(t, task.RetryCount, "a woken task gets a fresh retry budget")
			assert.Nil(t, task.ErrorMessage)
			assert.Nil(t, task.TimeoutAfter)
		})
	}

	t.Run("leaves a completed task completed", func(t *testing.T) {
		id := h.newTask(t)
		h.set(t, id, domain.TaskStatusCompleted, nil)
		woken, err := h.repo.WakeTask(ctx, h.workspaceID, id, time.Now().UTC())
		require.NoError(t, err)
		assert.False(t, woken)
		assert.Equal(t, domain.TaskStatusCompleted, h.get(t, id).Status)
	})

	t.Run("keeps the state a run saved", func(t *testing.T) {
		id := h.newTask(t)
		require.NoError(t, h.repo.MarkAsRunning(ctx, h.workspaceID, id, *liveLease()))
		saved := &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{LastProcessedEmail: "reached@example.com"},
		}
		require.NoError(t, h.repo.SaveState(ctx, h.workspaceID, id, 42, saved))
		h.set(t, id, domain.TaskStatusRunning, expiredLease())

		woken, err := h.repo.WakeTask(ctx, h.workspaceID, id, time.Now().UTC())
		require.NoError(t, err)
		require.True(t, woken)

		task := h.get(t, id)
		assert.Equal(t, float64(42), task.Progress)
		require.NotNil(t, task.State)
		require.NotNil(t, task.State.SendBroadcast)
		assert.Equal(t, "reached@example.com", task.State.SendBroadcast.LastProcessedEmail,
			"a wake must not rewind the cursor the run saved")
	})

	t.Run("reports a missing task", func(t *testing.T) {
		woken, err := h.repo.WakeTask(ctx, h.workspaceID, uuid.New().String(), time.Now().UTC())
		assert.ErrorIs(t, err, domain.ErrTaskNotFound)
		assert.False(t, woken)
	})
}
