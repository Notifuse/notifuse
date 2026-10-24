package repository

import (
	"context"
	"errors"
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// automationUpdateBuilder owns the column allowlist and both WHERE keys for
// automation updates: dropping workspace_id from the predicate would let an
// update cross workspaces, and a marshal failure must surface instead of
// writing a zero blob.

func TestAutomationUpdateBuilder(t *testing.T) {
	t.Run("builds a workspace-scoped update with marshaled trigger and nodes", func(t *testing.T) {
		automation := &domain.Automation{
			ID: "auto-1", Name: "Flow", Status: domain.AutomationStatusLive,
			Trigger: &domain.TimelineTriggerConfig{EventKind: "contact.created"},
			Nodes:   []*domain.AutomationNode{{ID: "n1"}},
		}
		builder, err := automationUpdateBuilder("ws-1", automation)
		require.NoError(t, err)
		query, args, err := builder.ToSql()
		require.NoError(t, err)

		assert.Contains(t, query, "UPDATE automations SET")
		assert.Contains(t, query, "WHERE id = $10 AND workspace_id = $11",
			"both keys scope the update; placeholders follow the nine SET columns")
		require.Len(t, args, 11)
		assert.Equal(t, "Flow", args[0])
		assert.Equal(t, "auto-1", args[9])
		assert.Equal(t, "ws-1", args[10])
		assert.False(t, automation.UpdatedAt.IsZero(), "the builder stamps UpdatedAt")
		assert.Equal(t, time.UTC, automation.UpdatedAt.Location())
	})

	t.Run("an unmarshalable trigger fails before any SQL is built", func(t *testing.T) {
		automation := &domain.Automation{
			ID:      "auto-1",
			Trigger: &domain.TimelineTriggerConfig{Conditions: &domain.TreeNode{}},
		}
		// A NaN hiding in the condition tree cannot be marshaled.
		bad := &domain.Automation{
			ID: "auto-1",
			Trigger: &domain.TimelineTriggerConfig{
				EventKind: "contact.created",
				Conditions: &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
					Source: "contacts",
					Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
						{FieldName: "score", NumberValues: []float64{math.NaN()}},
					}},
				}},
			},
		}
		_ = automation
		_, err := automationUpdateBuilder("ws-1", bad)
		require.ErrorContains(t, err, "failed to marshal trigger config:")
	})
}

func TestAutomationRepository_UpdateTx(t *testing.T) {
	ctx := context.Background()

	newAutomation := func() *domain.Automation {
		return &domain.Automation{
			ID: "auto-1", Name: "Flow", Status: domain.AutomationStatusDraft,
			Trigger: &domain.TimelineTriggerConfig{EventKind: "contact.created"},
		}
	}

	t.Run("executes the built update inside the caller's transaction", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE automations SET")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		require.NoError(t, repo.UpdateTx(ctx, tx, "ws-1", newAutomation()))
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec failure is wrapped", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE automations SET")).
			WillReturnError(errors.New("deadlock"))
		mock.ExpectRollback()

		tx, err := db.Begin()
		require.NoError(t, err)
		err = repo.UpdateTx(ctx, tx, "ws-1", newAutomation())
		require.ErrorContains(t, err, "failed to update automation:")
		_ = tx.Rollback()
	})
}
