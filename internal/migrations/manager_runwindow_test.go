package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dispatch window (currentDBVersion, codeVersion] is the safety core of
// the migration system: selecting one version too many replays DDL, one too
// few silently skips schema. These tests drive RunMigrations at the window
// edge against the real registry (db one version behind => exactly v42 is
// selected) with a fake workspace connector.

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

	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)
	expectMigrationLockGranted(mock)
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(fmt.Sprintf("%.0f", codeVersion-1)))
	return NewManager(logger.NewLoggerWithLevel("disabled")), mock, db, codeVersion
}

func TestManager_RunMigrations_WindowFailures(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("workspace listing failure names the migration version", func(t *testing.T) {
		manager, mock, db, codeVersion := managerOneVersionBehind(t)
		sentinel := errors.New("workspaces table gone")
		mock.ExpectBegin()
		mock.ExpectQuery("FROM workspaces").WillReturnError(sentinel)
		mock.ExpectRollback()
		mock.ExpectRollback() // the migration lock

		err := manager.RunMigrations(ctx, cfg, db)
		require.ErrorContains(t, err, fmt.Sprintf("migration failed for version %.0f:", codeVersion))
		require.ErrorContains(t, err, "failed to get workspaces:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection failure names the workspace", func(t *testing.T) {
		manager, mock, db, _ := managerOneVersionBehind(t)
		sentinel := errors.New("workspace db unreachable")
		manager.connector = fakeWorkspaceConnector{err: sentinel}

		mock.ExpectBegin()
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
		manager, mock, db, codeVersion := managerOneVersionBehind(t)

		wsDB, wsMock, err := sqlmock.New()
		require.NoError(t, err)
		manager.connector = fakeWorkspaceConnector{db: wsDB}

		mock.ExpectBegin()
		mock.ExpectQuery("FROM workspaces").WillReturnRows(
			sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"}).
				AddRow("ws-1", "Demo", []byte("{}"), []byte("[]"), time.Now(), time.Now()))
		mock.ExpectRollback()

		sentinel := errors.New("cannot set local")
		expectWorkspaceHoldsTables(wsMock, true)
		wsMock.ExpectBegin()
		wsMock.ExpectExec("SET LOCAL statement_timeout").WillReturnError(sentinel)
		wsMock.ExpectRollback()
		mock.ExpectRollback() // the migration lock

		err = manager.RunMigrations(ctx, cfg, db)
		require.ErrorContains(t, err, fmt.Sprintf("migration failed for version %.0f:", codeVersion))
		require.ErrorContains(t, err, "workspace migration failed for workspace ws-1:")
		require.ErrorContains(t, err, "v42: failed to disable statement_timeout:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
		require.NoError(t, wsMock.ExpectationsWereMet())
	})
}
