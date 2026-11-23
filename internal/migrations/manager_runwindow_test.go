package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dispatch window (currentDBVersion, codeVersion] is the safety core of
// the migration system: selecting one version too many replays DDL, one too
// few silently skips schema. These tests drive RunMigrations at the window
// edge against the real registry (db one version behind => exactly the latest
// migration is selected) with a fake workspace connector.

type fakeWorkspaceConnector struct {
	db  *sql.DB
	err error
}

func (f fakeWorkspaceConnector) connectToWorkspace(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	return f.db, f.err
}

func managerOneVersionBehind(t *testing.T) (*Manager, sqlmock.Sqlmock, *sql.DB, float64) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	target := latestWorkspaceMigrationVersion(t)
	expectMigrationLockGranted(mock)
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(fmt.Sprintf("%.0f", target-1)))
	return NewManager(logger.NewLoggerWithLevel("disabled")), mock, db, target
}

// latestWorkspaceMigrationVersion is the highest registered migration that updates the
// workspace databases: these tests drive the dispatch window through that migration. A
// migration registered after it (v45 while 45.0 is being assembled) is expected by
// expectLaterMigrations.
func latestWorkspaceMigrationVersion(t *testing.T) float64 {
	t.Helper()
	var latest float64
	for _, m := range GetRegisteredMigrations() {
		if m.HasWorkspaceUpdate() && m.GetMajorVersion() > latest {
			latest = m.GetMajorVersion()
		}
	}
	require.NotZero(t, latest, "no registered migration updates the workspace databases")
	return latest
}

// expectLaterMigrations fails when a migration is registered above after, the latest workspace
// migration: the tests cannot predict its SQL, so a migration added later must be expected
// here, case by case.
func expectLaterMigrations(t *testing.T, mock sqlmock.Sqlmock, after float64) {
	t.Helper()
	for _, m := range GetRegisteredMigrations() {
		version := m.GetMajorVersion()
		if version <= after {
			continue
		}
		require.Failf(t, "unexpected migration", "v%.0f runs after v%.0f: expect its SQL in the manager tests", version, after)
	}
}

// expectLatestSystemUpdate expects the system update of the latest workspace migration. That is
// v45, whose system part is two-factor authentication (schema.TwoFactorTableDefinitions). A
// project that adds another system part to v45 adds its SQL here.
func expectLatestSystemUpdate(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	m, ok := GetRegisteredMigration(latestWorkspaceMigrationVersion(t))
	require.True(t, ok)
	switch m.GetMajorVersion() {
	case 45.0:
		require.True(t, m.HasSystemUpdate())
		for _, stmt := range schema.TwoFactorTableDefinitions() {
			mock.ExpectExec(regexp.QuoteMeta(stmt)).WillReturnResult(sqlmock.NewResult(0, 0))
		}
	default:
		require.Falsef(t, m.HasSystemUpdate(), "v%.0f now has a system part: expect its SQL here", m.GetMajorVersion())
	}
}

func TestManager_RunMigrations_WindowFailures(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("workspace listing failure names the migration version", func(t *testing.T) {
		manager, mock, db, migrationVersion := managerOneVersionBehind(t)
		sentinel := errors.New("workspaces table gone")
		mock.ExpectBegin()
		expectLatestSystemUpdate(t, mock)
		mock.ExpectQuery("FROM workspaces").WillReturnError(sentinel)
		mock.ExpectRollback()
		mock.ExpectRollback() // the migration lock

		err := manager.RunMigrations(ctx, cfg, db)
		require.ErrorContains(t, err, fmt.Sprintf("migration failed for version %.0f:", migrationVersion))
		require.ErrorContains(t, err, "failed to get workspaces:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection failure names the workspace", func(t *testing.T) {
		manager, mock, db, _ := managerOneVersionBehind(t)
		sentinel := errors.New("workspace db unreachable")
		manager.connector = fakeWorkspaceConnector{err: sentinel}

		mock.ExpectBegin()
		expectLatestSystemUpdate(t, mock)
		mock.ExpectQuery("FROM workspaces").WillReturnRows(
			sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"}).
				AddRow("ws-1", "Demo", []byte("{}"), []byte("[]"), time.Now(), time.Now()))
		mock.ExpectRollback()
		mock.ExpectRollback() // the migration lock

		err := manager.RunMigrations(ctx, cfg, db)
		require.ErrorContains(t, err, "failed to connect to workspace database ws-1:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failing workspace migration rolls back and names both", func(t *testing.T) {
		manager, mock, db, migrationVersion := managerOneVersionBehind(t)

		wsDB, wsMock, err := sqlmock.New()
		require.NoError(t, err)
		manager.connector = fakeWorkspaceConnector{db: wsDB}

		mock.ExpectBegin()
		expectLatestSystemUpdate(t, mock)
		mock.ExpectQuery("FROM workspaces").WillReturnRows(
			sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"}).
				AddRow("ws-1", "Demo", []byte("{}"), []byte("[]"), time.Now(), time.Now()))
		mock.ExpectRollback()

		sentinel := errors.New("cannot replace function")
		expectWorkspaceHoldsTables(wsMock, true)
		wsMock.ExpectBegin()
		wsMock.ExpectExec("CREATE OR REPLACE FUNCTION track_message_history_changes").WillReturnError(sentinel)
		wsMock.ExpectRollback()
		mock.ExpectRollback() // the migration lock

		err = manager.RunMigrations(ctx, cfg, db)
		require.ErrorContains(t, err, fmt.Sprintf("migration failed for version %.0f:", migrationVersion))
		require.ErrorContains(t, err, "workspace migration failed for workspace ws-1:")
		require.ErrorContains(t, err, "data retention (v45):")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
		require.NoError(t, wsMock.ExpectationsWereMet())
	})
}
