package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// CreateTx owns the task row's birth defaults: a task inserted without an
// ID, status, or timestamps must come out pending, identified, and stamped —
// the scheduler claims on exactly those fields.

func TestTaskRepository_CreateTx_DefaultsAndUTCNorm(t *testing.T) {
	_, mock, repo := setupTaskMock(t)
	ctx := context.Background()

	zone := time.FixedZone("UTC+5", 5*3600)
	nextRun := time.Date(2026, 6, 1, 10, 0, 0, 0, zone)
	task := &domain.Task{Type: "send_broadcast", NextRunAfter: &nextRun}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tasks")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.WithTransaction(ctx, func(tx *sql.Tx) error {
		return repo.CreateTx(ctx, tx, "ws-1", task)
	})
	require.NoError(t, err)

	assert.NotEmpty(t, task.ID, "an ID is generated for an unidentified task")
	assert.Equal(t, domain.TaskStatusPending, task.Status)
	assert.False(t, task.CreatedAt.IsZero())
	assert.Equal(t, task.CreatedAt, task.UpdatedAt, "both stamps come from one now")
	require.NotNil(t, task.NextRunAfter)
	assert.Equal(t, time.UTC, task.NextRunAfter.Location(), "stored times are normalized to UTC")
	assert.Equal(t, 5, task.NextRunAfter.Hour(), "10:00 at UTC+5 is 05:00 UTC")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_CreateTx_InsertFailureRollsBack(t *testing.T) {
	_, mock, repo := setupTaskMock(t)
	ctx := context.Background()

	sentinel := errors.New("tasks table locked")
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tasks")).WillReturnError(sentinel)
	mock.ExpectRollback()

	task := &domain.Task{Type: "send_broadcast", Status: domain.TaskStatusPending}
	err := repo.WithTransaction(ctx, func(tx *sql.Tx) error {
		return repo.CreateTx(ctx, tx, "ws-1", task)
	})
	require.ErrorContains(t, err, "failed to insert task:")
	assert.ErrorIs(t, err, sentinel)
	require.NoError(t, mock.ExpectationsWereMet())
}
