package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timeAround matches a time.Time argument within a second of now + offset.
type timeAround struct{ offset time.Duration }

func (a timeAround) Match(v driver.Value) bool {
	ts, ok := v.(time.Time)
	if !ok {
		return false
	}
	want := time.Now().UTC().Add(a.offset)
	return ts.After(want.Add(-time.Second)) && ts.Before(want.Add(time.Second))
}

// utcTime matches a time.Time argument in UTC, within a second of now: the tasks columns have
// no time zone, so a local time would be stored as if it were UTC.
type utcTime struct{}

func (utcTime) Match(v driver.Value) bool {
	ts, ok := v.(time.Time)
	return ok && ts.Location() == time.UTC && time.Since(ts) < time.Second && time.Since(ts) > -time.Second
}

// typesArray matches the pq.Array of the permanent task types.
type typesArray struct{}

func (typesArray) Match(v driver.Value) bool {
	want, _ := pq.Array(domain.PermanentTaskTypes).Value()
	return v == want
}

func expectMarkAsFailed(t *testing.T, mock sqlmock.Sqlmock, task *domain.Task, status domain.TaskStatus, nextRunAfter interface{}) {
	t.Helper()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM tasks WHERE id = .* AND workspace_id = .*").
		WithArgs(task.ID, task.WorkspaceID).
		WillReturnRows(taskToMockRows(t, task))
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(string(status), "boom", sqlmock.AnyArg(), nextRunAfter, nil, task.ID, task.WorkspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestTaskRepository_MarkAsFailed_PermanentTask(t *testing.T) {
	ctx := context.Background()

	for _, taskType := range domain.PermanentTaskTypes {
		t.Run(taskType+" past its retries stays pending, later", func(t *testing.T) {
			db, mock, repo := setupTaskMock(t)
			defer func() { _ = db.Close() }()

			task := createTestTask("", "ws1")
			task.Type = taskType
			task.MaxRetries = 3
			task.RetryCount = 3 // the fourth failure in a row, which used to mark it failed
			task.RetryInterval = 60
			expectMarkAsFailed(t, mock, task, domain.TaskStatusPending, timeAround{2 * time.Minute})

			require.NoError(t, repo.MarkAsFailed(ctx, "ws1", task.ID, "boom"))
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("with retries left it waits RetryInterval", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()

		task := createTestTask("", "ws1")
		task.Type = domain.TaskTypeProcessContactSegmentQueue
		task.MaxRetries = 3
		task.RetryCount = 1
		task.RetryInterval = 60
		expectMarkAsFailed(t, mock, task, domain.TaskStatusPending, timeAround{time.Minute})

		require.NoError(t, repo.MarkAsFailed(ctx, "ws1", task.ID, "boom"))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a task that is not permanent still fails for good", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()

		task := createTestTask("", "ws1")
		task.Type = "send_broadcast"
		task.MaxRetries = 3
		task.RetryCount = 3
		expectMarkAsFailed(t, mock, task, domain.TaskStatusFailed, nil)

		require.NoError(t, repo.MarkAsFailed(ctx, "ws1", task.ID, "boom"))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func expectCreateMissing(mock sqlmock.Sqlmock, workspaceID string, created map[string]int64) {
	for _, taskType := range domain.PermanentTaskTypes {
		state, _ := json.Marshal(&domain.TaskState{Message: domain.PermanentTaskMessage(taskType)})
		mock.ExpectExec(regexp.QuoteMeta(createMissingPermanentTasksSQL)).
			WithArgs(taskType, string(state), utcTime{}, workspaceID,
				domain.PermanentTaskMaxRuntime, domain.PermanentTaskMaxRetries, domain.PermanentTaskRetryInterval).
			WillReturnResult(sqlmock.NewResult(0, created[taskType]))
	}
}

func TestTaskRepository_RestorePermanentTasks(t *testing.T) {
	ctx := context.Background()

	t.Run("every workspace: revives, removes orphans, creates the missing ones", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).
			WithArgs(permanentTaskRestoreLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(restoreStuckPermanentTasksSQL)).
			WithArgs(typesArray{}, utcTime{}, "").WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(regexp.QuoteMeta(removeOrphanPermanentTasksSQL)).
			WithArgs(typesArray{}).WillReturnResult(sqlmock.NewResult(0, 1))
		expectCreateMissing(mock, "", map[string]int64{
			domain.TaskTypeProcessContactSegmentQueue: 1,
			domain.TaskTypeCheckSegmentRecompute:      3,
		})
		mock.ExpectCommit()

		got, err := repo.RestorePermanentTasks(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, domain.PermanentTaskRestore{Revived: 2, Created: 4, Removed: 1}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("one workspace: no orphan cleanup", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).
			WithArgs(permanentTaskRestoreLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(restoreStuckPermanentTasksSQL)).
			WithArgs(typesArray{}, utcTime{}, "ws1").WillReturnResult(sqlmock.NewResult(0, 0))
		expectCreateMissing(mock, "ws1", map[string]int64{
			domain.TaskTypeProcessContactSegmentQueue: 1,
			domain.TaskTypeCheckSegmentRecompute:      1,
		})
		mock.ExpectCommit()

		got, err := repo.RestorePermanentTasks(ctx, "ws1")
		require.NoError(t, err)
		assert.Equal(t, domain.PermanentTaskRestore{Created: 2}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the statements say what they touch", func(t *testing.T) {
		// Running tasks with a lease, pending and paused ones with a date are left alone.
		assert.Contains(t, restoreStuckPermanentTasksSQL, "status IN ('failed', 'completed')")
		assert.Contains(t, restoreStuckPermanentTasksSQL, "(status = 'paused' AND next_run_after IS NULL)")
		assert.Contains(t, restoreStuckPermanentTasksSQL, "(status = 'running' AND timeout_after IS NULL AND updated_at <= $2::timestamp - INTERVAL '10 minutes')")
		assert.NotContains(t, restoreStuckPermanentTasksSQL, "CURRENT_TIMESTAMP")
		assert.Contains(t, createMissingPermanentTasksSQL, "FOR KEY SHARE OF w")
		assert.NotContains(t, createMissingPermanentTasksSQL, "CURRENT_TIMESTAMP")
		assert.Equal(t, int64(4_242_000_004), permanentTaskRestoreLockID)
	})

	steps := []struct {
		name  string
		setup func(mock sqlmock.Sqlmock, fail error)
	}{
		{"lock", func(mock sqlmock.Sqlmock, fail error) {
			mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WillReturnError(fail)
		}},
		{"revive", func(mock sqlmock.Sqlmock, fail error) {
			mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(regexp.QuoteMeta(restoreStuckPermanentTasksSQL)).WillReturnError(fail)
		}},
		{"remove orphans", func(mock sqlmock.Sqlmock, fail error) {
			mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(regexp.QuoteMeta(restoreStuckPermanentTasksSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta(removeOrphanPermanentTasksSQL)).WillReturnError(fail)
		}},
		{"create", func(mock sqlmock.Sqlmock, fail error) {
			mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(regexp.QuoteMeta(restoreStuckPermanentTasksSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta(removeOrphanPermanentTasksSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(regexp.QuoteMeta(createMissingPermanentTasksSQL)).WillReturnError(fail)
		}},
	}
	for _, step := range steps {
		t.Run("a failed "+step.name+" rolls everything back", func(t *testing.T) {
			db, mock, repo := setupTaskMock(t)
			defer func() { _ = db.Close() }()

			fail := fmt.Errorf("%s failed", step.name)
			mock.ExpectBegin()
			step.setup(mock, fail)
			mock.ExpectRollback()

			_, err := repo.RestorePermanentTasks(ctx, "")
			require.Error(t, err)
			assert.True(t, errors.Is(err, fail))
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("a failed commit reports nothing changed", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(restoreStuckPermanentTasksSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(regexp.QuoteMeta(removeOrphanPermanentTasksSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		expectCreateMissing(mock, "", nil)
		mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

		got, err := repo.RestorePermanentTasks(ctx, "")
		require.Error(t, err)
		assert.Equal(t, domain.PermanentTaskRestore{}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
