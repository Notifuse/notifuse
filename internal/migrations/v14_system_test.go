package migrations

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Notifuse/notifuse/internal/domain"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// V14 seeds telemetry_enabled / check_for_updates exactly once, only on
// installed systems, honoring the operator's env choice at migration time.
// Rewriting an existing value — or seeding a fresh install that will set
// its own during setup — would clobber an explicit opt-out.

func v14InstalledSystem(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
}

func TestV14Migration_UpdateSystem_SeedingRules(t *testing.T) {
	ctx := context.Background()
	migration := &V14Migration{}
	cfg := &config.Config{}

	newDB := func(t *testing.T) (sqlmock.Sqlmock, DBExecutor) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		return mock, db
	}

	t.Run("uninstalled system is skipped without writes", func(t *testing.T) {
		mock, db := newDB(t)
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnError(sql.ErrNoRows)
		require.NoError(t, migration.UpdateSystem(ctx, cfg, db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("is_installed other than true is skipped without writes", func(t *testing.T) {
		mock, db := newDB(t)
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
		require.NoError(t, migration.UpdateSystem(ctx, cfg, db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("installed check failure is wrapped", func(t *testing.T) {
		mock, db := newDB(t)
		sentinel := errors.New("settings gone")
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'is_installed'").
			WillReturnError(sentinel)
		err := migration.UpdateSystem(ctx, cfg, db)
		require.ErrorContains(t, err, "failed to check is_installed:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("missing settings are seeded with env honored", func(t *testing.T) {
		t.Setenv("TELEMETRY", "false")
		t.Setenv("CHECK_FOR_UPDATES", "0")
		mock, db := newDB(t)
		v14InstalledSystem(mock)
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'telemetry_enabled'").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("INSERT INTO settings").
			WithArgs("telemetry_enabled", "false").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'check_for_updates'").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("INSERT INTO settings").
			WithArgs("check_for_updates", "false").
			WillReturnResult(sqlmock.NewResult(1, 1))

		require.NoError(t, migration.UpdateSystem(ctx, cfg, db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("existing settings are left untouched", func(t *testing.T) {
		mock, db := newDB(t)
		v14InstalledSystem(mock)
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'telemetry_enabled'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'check_for_updates'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

		require.NoError(t, migration.UpdateSystem(ctx, cfg, db),
			"an operator's explicit opt-out must survive the migration")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("only the missing setting is inserted", func(t *testing.T) {
		mock, db := newDB(t)
		v14InstalledSystem(mock)
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'telemetry_enabled'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'check_for_updates'").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("INSERT INTO settings").
			WithArgs("check_for_updates", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		require.NoError(t, migration.UpdateSystem(ctx, cfg, db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an insert failure names the setting", func(t *testing.T) {
		mock, db := newDB(t)
		v14InstalledSystem(mock)
		sentinel := errors.New("read-only")
		mock.ExpectQuery("SELECT value FROM settings WHERE key = 'telemetry_enabled'").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("INSERT INTO settings").WillReturnError(sentinel)

		err := migration.UpdateSystem(ctx, cfg, db)
		require.ErrorContains(t, err, "failed to insert telemetry_enabled:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestV14Migration_UpdateWorkspace_Failure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	sentinel := errors.New("duplicate column")
	mock.ExpectExec("ADD COLUMN IF NOT EXISTS channel_options").WillReturnError(sentinel)

	err = (&V14Migration{}).UpdateWorkspace(context.Background(), &config.Config{},
		&domain.Workspace{ID: "ws-1"}, db)
	require.ErrorContains(t, err, "failed to add channel_options column:")
	assert.ErrorIs(t, err, sentinel)
}
