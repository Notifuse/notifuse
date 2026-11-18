package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	intdb "github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The startup steps of InitDB, on a real server, with and without DB_EXPECT_EXISTING:
// pointed at a server that does not hold the installation, the flag stops Notifuse
// before it writes anything; without it, the same server becomes a new installation.
func TestPGExpectExisting(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := *testDatabaseConfig()
	admin := openAdmin(t, &cfg)

	t.Run("a missing system database is refused under the flag, created without it", func(t *testing.T) {
		name := "notifuse_expect_" + uniqueID("sys")
		dropDatabaseAtCleanup(t, admin, name)

		err := intdb.EnsureSystemDatabaseExists(intdb.GetPostgresDSN(&cfg), name, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DB_EXPECT_EXISTING is set")
		assert.False(t, databaseExists(t, admin, name), "nothing created under the flag")

		require.NoError(t, intdb.EnsureSystemDatabaseExists(intdb.GetPostgresDSN(&cfg), name, false))
		assert.True(t, databaseExists(t, admin, name))
	})

	t.Run("a mixed-case system database name is found again at the next start", func(t *testing.T) {
		name := "Notifuse_Expect_" + uniqueID("case")
		dropDatabaseAtCleanup(t, admin, name)

		require.NoError(t, intdb.EnsureSystemDatabaseExists(intdb.GetPostgresDSN(&cfg), name, false))
		assert.True(t, databaseExists(t, admin, name), "created under its exact name")
		require.NoError(t, intdb.EnsureSystemDatabaseExists(intdb.GetPostgresDSN(&cfg), name, true), "found again, even under the flag")
	})

	t.Run("an empty system database is refused under the flag, initialised without it", func(t *testing.T) {
		name := "notifuse_expect_" + uniqueID("empty")
		createEmptyDatabase(t, admin, name)
		db := openDatabase(t, &cfg, name)
		ctx := context.Background()

		err := intdb.RequireExistingInstallation(db, name)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "holds no Notifuse installation")
		assert.Equal(t, 0, relationCount(t, db), "the check writes nothing")

		// The migrations refuse the first run too, for a caller that skipped the check.
		require.NoError(t, intdb.InitializeDatabase(db, nil))
		dbCfg := cfg
		dbCfg.DBName = name
		dbCfg.ExpectExisting = true
		err = migrations.NewManager(logger.NewLoggerWithLevel("disabled")).RunMigrations(ctx, &config.Config{Database: dbCfg}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "records no database version")
		require.Error(t, intdb.RequireExistingInstallation(db, name), "no version recorded by the refused run")

		dbCfg.ExpectExisting = false
		require.NoError(t, migrations.NewManager(logger.NewLoggerWithLevel("disabled")).RunMigrations(ctx, &config.Config{Database: dbCfg}, db))
		assert.NoError(t, intdb.RequireExistingInstallation(db, name), "a new installation now holds its version")
	})
}
