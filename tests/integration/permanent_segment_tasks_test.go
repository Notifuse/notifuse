package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The segment queue and recompute tasks run for ever, one of each per workspace. One that
// stopped for good left its workspace's segments frozen, with nothing to run it again; these
// tests drive the restore and the retry rule against the real database.

type permanentTaskRow struct {
	ID           string
	Status       string
	RetryCount   int
	NextRunAfter sql.NullTime
	TimeoutAfter sql.NullTime
	ErrorMessage sql.NullString
}

func permanentTasksOf(t *testing.T, db *sql.DB, workspaceID, taskType string) []permanentTaskRow {
	t.Helper()
	rows, err := db.Query(`SELECT id, status, retry_count, next_run_after, timeout_after, error_message
		FROM tasks WHERE workspace_id = $1 AND type = $2 ORDER BY created_at`, workspaceID, taskType)
	require.NoError(t, err)
	defer rows.Close()
	var out []permanentTaskRow
	for rows.Next() {
		var r permanentTaskRow
		require.NoError(t, rows.Scan(&r.ID, &r.Status, &r.RetryCount, &r.NextRunAfter, &r.TimeoutAfter, &r.ErrorMessage))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// onlyPermanentTask returns the one task of the type, failing when there are more or none.
func onlyPermanentTask(t *testing.T, db *sql.DB, workspaceID, taskType string) permanentTaskRow {
	t.Helper()
	tasks := permanentTasksOf(t, db, workspaceID, taskType)
	require.Len(t, tasks, 1, "%s of %s", taskType, workspaceID)
	return tasks[0]
}

// insertPermanentTask writes a permanent task in the given state, as a past version left it.
func insertPermanentTask(t *testing.T, db *sql.DB, workspaceID, taskType, status string, nextRunAfter, timeoutAfter *time.Time) string {
	t.Helper()
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := db.Exec(`INSERT INTO tasks (id, workspace_id, type, status, progress, state, error_message,
			created_at, updated_at, next_run_after, timeout_after, max_runtime, max_retries, retry_count, retry_interval)
		VALUES ($1, $2, $3, $4, 0, '{}'::jsonb, 'old failure', $5, $5, $6, $7, 50, 3, 4, 60)`,
		id, workspaceID, taskType, status, now, nextRunAfter, timeoutAfter)
	require.NoError(t, err)
	return id
}

func utcAfter(d time.Duration) *time.Time {
	ts := time.Now().UTC().Add(d)
	return &ts
}

// assertDueNow checks a restored task: pending, due now, its failures forgotten.
func assertDueNow(t *testing.T, task permanentTaskRow) {
	t.Helper()
	assert.Equal(t, "pending", task.Status)
	assert.Equal(t, 0, task.RetryCount)
	assert.False(t, task.TimeoutAfter.Valid)
	assert.False(t, task.ErrorMessage.Valid)
	require.True(t, task.NextRunAfter.Valid)
	assert.WithinDuration(t, time.Now().UTC(), task.NextRunAfter.Time, 5*time.Second)
}

func batchIDs(t *testing.T, repo domain.TaskRepository) map[string]bool {
	t.Helper()
	batch, err := repo.GetNextBatch(context.Background(), 100)
	require.NoError(t, err)
	ids := make(map[string]bool, len(batch))
	for _, task := range batch {
		ids[task.ID] = true
	}
	return ids
}

func TestPermanentSegmentTasks_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	ctx := context.Background()
	factory := suite.DataFactory
	sysDB := suite.DBManager.GetDB()
	taskRepo := suite.ServerManager.GetApp().GetTaskRepository()
	queueType := domain.TaskTypeProcessContactSegmentQueue
	recomputeType := domain.TaskTypeCheckSegmentRecompute

	newWorkspace := func(t *testing.T) string {
		t.Helper()
		ws, err := factory.CreateWorkspace()
		require.NoError(t, err)
		return ws.ID
	}

	t.Run("a permanent task keeps being retried, later each time, and a success resets it", func(t *testing.T) {
		wsID := newWorkspace(t)
		_, err := taskRepo.RestorePermanentTasks(ctx, wsID)
		require.NoError(t, err)
		task := onlyPermanentTask(t, sysDB, wsID, queueType)

		// Three failures within the retries, then two past them: the fourth and fifth in a row
		// used to leave the task failed for good.
		wantDelays := []time.Duration{time.Minute, time.Minute, time.Minute, 2 * time.Minute, 4 * time.Minute}
		for i, want := range wantDelays {
			require.NoError(t, taskRepo.MarkAsFailed(ctx, wsID, task.ID, fmt.Sprintf("failure %d", i+1)))
			got := onlyPermanentTask(t, sysDB, wsID, queueType)
			assert.Equal(t, "pending", got.Status, "after failure %d", i+1)
			assert.Equal(t, i+1, got.RetryCount)
			require.True(t, got.NextRunAfter.Valid)
			assert.WithinDuration(t, time.Now().UTC().Add(want), got.NextRunAfter.Time, 5*time.Second, "after failure %d", i+1)
		}

		require.NoError(t, taskRepo.MarkAsPending(ctx, wsID, task.ID, time.Now().UTC(), 0, nil))
		assert.Equal(t, 0, onlyPermanentTask(t, sysDB, wsID, queueType).RetryCount)

		// Another type past its retries still fails for good.
		other := insertPermanentTask(t, sysDB, wsID, "send_broadcast", "running", nil, nil)
		_, err = sysDB.Exec(`UPDATE tasks SET retry_count = 3 WHERE id = $1`, other)
		require.NoError(t, err)
		require.NoError(t, taskRepo.MarkAsFailed(ctx, wsID, other, "boom"))
		var status string
		require.NoError(t, sysDB.QueryRow(`SELECT status FROM tasks WHERE id = $1`, other).Scan(&status))
		assert.Equal(t, "failed", status)
	})

	t.Run("the restore revives what is stuck, leaves the rest, creates the missing and removes orphans", func(t *testing.T) {
		stuck1 := newWorkspace(t)
		stuck2 := newWorkspace(t)
		healthy := newWorkspace(t)
		bare := newWorkspace(t)

		failed := insertPermanentTask(t, sysDB, stuck1, queueType, "failed", nil, nil)
		completed := insertPermanentTask(t, sysDB, stuck1, recomputeType, "completed", nil, nil)
		pausedNoDate := insertPermanentTask(t, sysDB, stuck2, queueType, "paused", nil, nil)
		runningNoLease := insertPermanentTask(t, sysDB, stuck2, recomputeType, "running", utcAfter(-time.Minute), nil)
		_, err := sysDB.Exec(`UPDATE tasks SET updated_at = $2 WHERE id = $1`, runningNoLease, *utcAfter(-20 * time.Minute))
		require.NoError(t, err)
		lease := utcAfter(time.Minute)
		runningLeased := insertPermanentTask(t, sysDB, healthy, queueType, "running", utcAfter(-time.Minute), lease)
		later := utcAfter(time.Hour)
		pendingLater := insertPermanentTask(t, sysDB, healthy, recomputeType, "pending", later, nil)
		orphan := insertPermanentTask(t, sysDB, "ghostws", queueType, "pending", utcAfter(0), nil)
		// A run with no lease that wrote a moment ago may still be under way: held, as notHeld has it.
		fresh := newWorkspace(t)
		freshRunning := insertPermanentTask(t, sysDB, fresh, queueType, "running", utcAfter(-time.Minute), nil)

		restored, err := taskRepo.RestorePermanentTasks(ctx, "")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, restored.Revived, int64(4))
		assert.GreaterOrEqual(t, restored.Created, int64(2))
		assert.GreaterOrEqual(t, restored.Removed, int64(1))

		for _, c := range []struct{ ws, taskType, id string }{
			{stuck1, queueType, failed}, {stuck1, recomputeType, completed},
			{stuck2, queueType, pausedNoDate}, {stuck2, recomputeType, runningNoLease},
		} {
			got := onlyPermanentTask(t, sysDB, c.ws, c.taskType)
			assert.Equal(t, c.id, got.ID, "revived in place, not replaced")
			assertDueNow(t, got)
		}

		leased := onlyPermanentTask(t, sysDB, healthy, queueType)
		assert.Equal(t, runningLeased, leased.ID)
		assert.Equal(t, "running", leased.Status, "a run holding its lease is left to it")
		assert.Equal(t, 4, leased.RetryCount)
		waiting := onlyPermanentTask(t, sysDB, healthy, recomputeType)
		assert.Equal(t, pendingLater, waiting.ID)
		assert.WithinDuration(t, *later, waiting.NextRunAfter.Time, time.Second, "a pending task keeps its date")

		held := onlyPermanentTask(t, sysDB, fresh, queueType)
		assert.Equal(t, freshRunning, held.ID)
		assert.Equal(t, "running", held.Status)

		assertDueNow(t, onlyPermanentTask(t, sysDB, bare, queueType))
		assertDueNow(t, onlyPermanentTask(t, sysDB, bare, recomputeType))

		var orphans int
		require.NoError(t, sysDB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = $1`, orphan).Scan(&orphans))
		assert.Zero(t, orphans)

		// Every revived and created task is picked up by the next pass.
		due := batchIDs(t, taskRepo)
		for _, id := range []string{failed, completed, pausedNoDate, runningNoLease,
			onlyPermanentTask(t, sysDB, bare, queueType).ID, onlyPermanentTask(t, sysDB, bare, recomputeType).ID} {
			assert.True(t, due[id], "task %s is due", id)
		}
		assert.False(t, due[pendingLater])

		again, err := taskRepo.RestorePermanentTasks(ctx, "")
		require.NoError(t, err)
		assert.False(t, again.Changed(), "a second restore has nothing to do: %+v", again)
	})

	t.Run("concurrent restores create one task of each type", func(t *testing.T) {
		wsIDs := []string{newWorkspace(t), newWorkspace(t), newWorkspace(t)}

		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				scope := ""
				if i%2 == 1 {
					scope = wsIDs[i%len(wsIDs)] // as a workspace creation does
				}
				_, err := taskRepo.RestorePermanentTasks(ctx, scope)
				errs <- err
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		for _, wsID := range wsIDs {
			onlyPermanentTask(t, sysDB, wsID, queueType)
			onlyPermanentTask(t, sysDB, wsID, recomputeType)
		}
	})

	t.Run("a session outside UTC still makes the tasks due now", func(t *testing.T) {
		cfg := suite.DBManager.GetConfig()
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s timezone=Pacific/Kiritimati",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
		tzDB, err := sql.Open("postgres", dsn)
		require.NoError(t, err)
		defer tzDB.Close()
		var tz string
		require.NoError(t, tzDB.QueryRow(`SHOW TimeZone`).Scan(&tz))
		require.Equal(t, "Pacific/Kiritimati", tz, "the session runs 14 hours ahead of UTC")
		tzRepo := repository.NewTaskRepository(tzDB)

		wsID := newWorkspace(t)
		failed := insertPermanentTask(t, sysDB, wsID, queueType, "failed", nil, nil)

		_, err = tzRepo.RestorePermanentTasks(ctx, wsID)
		require.NoError(t, err)

		revived := onlyPermanentTask(t, sysDB, wsID, queueType)
		assertDueNow(t, revived)
		created := onlyPermanentTask(t, sysDB, wsID, recomputeType)
		assertDueNow(t, created)
		due := batchIDs(t, taskRepo)
		assert.True(t, due[failed])
		assert.True(t, due[created.ID])
	})

	t.Run("a workspace deletion removes tasks a restore created meanwhile", func(t *testing.T) {
		wsID := newWorkspace(t)
		wsRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()

		// WorkspaceService.DeleteWorkspace deletes the tasks first, then the database and the
		// row; a restore can run in between.
		require.NoError(t, taskRepo.DeleteAll(ctx, wsID))
		restored, err := taskRepo.RestorePermanentTasks(ctx, "")
		require.NoError(t, err)
		require.GreaterOrEqual(t, restored.Created, int64(2))
		require.NoError(t, wsRepo.Delete(ctx, wsID))

		assert.Empty(t, permanentTasksOf(t, sysDB, wsID, queueType))
		assert.Empty(t, permanentTasksOf(t, sysDB, wsID, recomputeType))

		_, err = taskRepo.RestorePermanentTasks(ctx, "")
		require.NoError(t, err)
		assert.Empty(t, permanentTasksOf(t, sysDB, wsID, queueType), "nothing is created for a deleted workspace")
	})
}

// TestPermanentSegmentTasks_CronRestores_Integration runs the restore the way a deployment
// without the internal scheduler does: through /api/cron.
func TestPermanentSegmentTasks_CronRestores_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		require.False(t, cfg.TaskScheduler.Enabled, "the harness runs no scheduler")
		cfg.TaskScheduler.SkipPermanentTaskRestore = false
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	sysDB := suite.DBManager.GetDB()
	ws, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	failed := insertPermanentTask(t, sysDB, ws.ID, domain.TaskTypeProcessContactSegmentQueue, "failed", nil, nil)

	resp, err := http.Get(suite.ServerManager.GetURL() + "/api/cron?max_tasks=10")
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	// The cron run works in the background: the failed task comes back and runs, and the
	// missing recompute task is created.
	require.Eventually(t, func() bool {
		var status string
		var retries int
		if err := sysDB.QueryRow(`SELECT status, retry_count FROM tasks WHERE id = $1`, failed).Scan(&status, &retries); err != nil {
			return false
		}
		return status != "failed" && retries == 0
	}, 20*time.Second, 200*time.Millisecond, "the failed queue task is restored")
	require.Eventually(t, func() bool {
		return len(permanentTasksOf(t, sysDB, ws.ID, domain.TaskTypeCheckSegmentRecompute)) == 1
	}, 20*time.Second, 200*time.Millisecond, "the missing recompute task is created")
}
