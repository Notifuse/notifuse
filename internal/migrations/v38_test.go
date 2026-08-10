package migrations

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV38Migration_Metadata(t *testing.T) {
	m := &V38Migration{}
	assert.Equal(t, 38.0, m.GetMajorVersion())
	assert.True(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV38Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(38.0)
	require.True(t, ok, "v38 must be registered so it runs on startup")
	assert.IsType(t, &V38Migration{}, migration)
}

// Both statements must grant web_analytics and keep the two guards: the
// object check (a scalar would concatenate into an array and corrupt the row)
// and the "already granted" check (so a re-run cannot revoke a narrowed grant).
const (
	v38UserWorkspacesGrant = `(?s)UPDATE user_workspaces.*` +
		`"web_analytics": \{"read": true, "write": true\}.*` +
		`jsonb_typeof\(permissions\) = 'object'.*` +
		`NOT permissions \? 'web_analytics'`
	v38InvitationsGrant = `(?s)UPDATE workspace_invitations.*` +
		`"web_analytics": \{"read": true, "write": true\}.*` +
		`jsonb_typeof\(permissions\) = 'object'.*` +
		`NOT permissions \? 'web_analytics'`
)

func TestV38Migration_UpdateSystem(t *testing.T) {
	m := &V38Migration{}
	ctx := context.Background()

	t.Run("grants web_analytics to existing members and pending invitations", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Without this, HasPermission denies web_analytics for every non-owner
		// member: their stored permission map predates the resource.
		mock.ExpectExec(v38UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(v38InvitationsGrant).WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, m.UpdateSystem(ctx, &config.Config{}, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports a failed user_workspaces backfill", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v38UserWorkspacesGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user workspaces")
	})

	t.Run("reports a failed invitations backfill", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v38UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(v38InvitationsGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace invitations")
	})
}

func expectV38WorkspaceDDL(mock sqlmock.Sqlmock) {
	for range schema.WebAnalyticsTableDefinitions() {
		mock.ExpectExec("(?s)CREATE (TABLE|INDEX) IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			mock.ExpectExec(regexp.QuoteMeta(schema.WebAnalyticsPartitionName(table, month)) + " PARTITION OF").
				WillReturnResult(sqlmock.NewResult(0, 0))
		}
	}
}

func TestV38Migration_UpdateWorkspace(t *testing.T) {
	workspace := &domain.Workspace{ID: "ws1"}

	t.Run("creates parents, indexes, and current+next partitions", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV38WorkspaceDDL(mock)

		m := &V38Migration{}
		require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("re-run is harmless (every statement is IF NOT EXISTS)", func(t *testing.T) {
		for _, stmt := range schema.WebAnalyticsTableDefinitions() {
			assert.Contains(t, stmt, "IF NOT EXISTS")
		}
		assert.Contains(t, schema.WebAnalyticsPartitionDDL("web_sessions", time.Now()), "IF NOT EXISTS")

		// And executing twice issues the same idempotent statements again.
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		expectV38WorkspaceDDL(mock)
		expectV38WorkspaceDDL(mock)

		m := &V38Migration{}
		require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db))
		require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("surfaces DDL failures with the workspace id", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("(?s)CREATE (TABLE|INDEX) IF NOT EXISTS").WillReturnError(errors.New("boom"))

		m := &V38Migration{}
		err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ws1")
		assert.Contains(t, err.Error(), "boom")
	})
}
