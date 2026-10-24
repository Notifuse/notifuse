package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
)

// InitDB against an unreachable database must fail fast with the step
// named. Booting on with a dead pool — or panicking on the masked-password
// logging — would turn a config typo into a hung or crashed process.

func initDBWithDatabase(t *testing.T, dbConfig config.DatabaseConfig) error {
	t.Helper()
	cfg := &config.Config{Database: dbConfig}
	appInterface := NewApp(cfg)
	app, ok := appInterface.(*App)
	require.True(t, ok)
	return app.InitDB()
}

func TestAppInitDB_UnreachableDatabase(t *testing.T) {
	err := initDBWithDatabase(t, config.DatabaseConfig{
		Host: "127.0.0.1", Port: 1, User: "postgres", Password: "pw", DBName: "notifuse_system",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ensure system database exists:")
}

func TestAppInitDB_UnreachableDatabaseWithoutPassword(t *testing.T) {
	// The password masker must not index an empty password.
	err := initDBWithDatabase(t, config.DatabaseConfig{
		Host: "127.0.0.1", Port: 1, User: "postgres", DBName: "notifuse_system",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ensure system database exists:")
}
