package repository

import (
	"context"
	"errors"
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// BatchInsertNew must use ON CONFLICT DO NOTHING (never DO UPDATE): the
// custom_events timeline trigger fires on UPDATE too, so re-writing an
// existing row would duplicate timeline entries and re-enrol contacts.

func batchInsertEvent(id string) *domain.CustomEvent {
	return &domain.CustomEvent{
		ExternalID: id,
		Email:      "user@example.com",
		EventName:  "shopify.order",
		Properties: map[string]interface{}{"total": 42.0},
		OccurredAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Source:     "integration",
	}
}

func TestCustomEventRepository_BatchInsertNew(t *testing.T) {
	ctx := context.Background()

	t.Run("empty batch is a no-op that never touches the database", func(t *testing.T) {
		_, repo, mock, _, cleanup := setupCustomEventTest(t)
		defer cleanup()
		require.NoError(t, repo.BatchInsertNew(ctx, "ws-1", nil))
		require.NoError(t, repo.BatchInsertNew(ctx, "ws-1", []*domain.CustomEvent{}))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("connection failure is wrapped", func(t *testing.T) {
		wsRepo, repo, _, _, cleanup := setupCustomEventTest(t)
		defer cleanup()
		sentinel := errors.New("workspace db missing")
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel).Times(1)
		err := repo.BatchInsertNew(ctx, "ws-1", []*domain.CustomEvent{batchInsertEvent("e1")})
		require.ErrorContains(t, err, "failed to get workspace connection:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("inserts every event in one transaction and commits", func(t *testing.T) {
		wsRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
		defer cleanup()
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).Times(1)

		mock.ExpectBegin()
		// The DO NOTHING clause is the contract: pin it in the prepared SQL.
		prepared := mock.ExpectPrepare(regexp.QuoteMeta("ON CONFLICT (event_name, external_id) DO NOTHING"))
		for _, id := range []string{"e1", "e2"} {
			prepared.ExpectExec().WithArgs(
				"shopify.order", id, "user@example.com", []byte(`{"total":42}`),
				sqlmock.AnyArg(), "integration", nil, nil, nil, nil, nil,
				sqlmock.AnyArg(), sqlmock.AnyArg(),
			).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectCommit()

		err := repo.BatchInsertNew(ctx, "ws-1", []*domain.CustomEvent{
			batchInsertEvent("e1"), batchInsertEvent("e2"),
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an exec failure aborts without committing", func(t *testing.T) {
		wsRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
		defer cleanup()
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).Times(1)

		sentinel := errors.New("insert exploded")
		mock.ExpectBegin()
		prepared := mock.ExpectPrepare("INSERT INTO custom_events")
		prepared.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))
		prepared.ExpectExec().WillReturnError(sentinel)
		mock.ExpectRollback()

		err := repo.BatchInsertNew(ctx, "ws-1", []*domain.CustomEvent{
			batchInsertEvent("e1"), batchInsertEvent("e2"),
		})
		require.ErrorContains(t, err, "failed to insert custom event:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet(), "rollback, never commit, after a failed insert")
	})

	t.Run("unmarshalable properties fail before any exec", func(t *testing.T) {
		wsRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
		defer cleanup()
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).Times(1)

		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO custom_events")
		mock.ExpectRollback()

		bad := batchInsertEvent("e1")
		bad.Properties = map[string]interface{}{"total": math.NaN()} // json.Marshal rejects NaN
		err := repo.BatchInsertNew(ctx, "ws-1", []*domain.CustomEvent{bad})
		require.ErrorContains(t, err, "failed to marshal properties:")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nil properties are stored as an empty object", func(t *testing.T) {
		wsRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
		defer cleanup()
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).Times(1)

		mock.ExpectBegin()
		prepared := mock.ExpectPrepare("INSERT INTO custom_events")
		prepared.ExpectExec().WithArgs(
			"shopify.order", "e1", "user@example.com", []byte(`{}`),
			sqlmock.AnyArg(), "integration", nil, nil, nil, nil, nil,
			sqlmock.AnyArg(), sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		ev := batchInsertEvent("e1")
		ev.Properties = nil
		require.NoError(t, repo.BatchInsertNew(ctx, "ws-1", []*domain.CustomEvent{ev}))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
