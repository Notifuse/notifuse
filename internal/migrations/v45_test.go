package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV45Migration_Metadata(t *testing.T) {
	m := &V45Migration{}
	assert.Equal(t, 45.0, m.GetMajorVersion())
	assert.False(t, m.ShouldRestartServer())
}

func TestV45Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(45.0)
	require.True(t, ok, "v45 must be registered, or the version stamp never reaches 45")
	assert.IsType(t, &V45Migration{}, migration)
}

func TestV45Migration_IsWhatLetsTheStampReachFortyFive(t *testing.T) {
	var selected []float64
	for _, m := range GetRegisteredMigrations() {
		v := m.GetMajorVersion()
		if v > 44.0 && v <= 45.0 {
			selected = append(selected, v)
		}
	}
	assert.Equal(t, []float64{45.0}, selected,
		"a database stamped 44 upgrading to a 45.0 build must select exactly one migration")
}

// The code version and the registered migrations are two halves of one fact.
func TestV45Migration_MatchesTheCodeVersion(t *testing.T) {
	code, err := GetCurrentCodeVersion()
	require.NoError(t, err)

	var highest float64
	for _, m := range GetRegisteredMigrations() {
		if v := m.GetMajorVersion(); v > highest {
			highest = v
		}
	}
	assert.Equal(t, highest, code,
		"config.VERSION (%v) and the highest registered migration (%v) must agree", code, highest)
}

// Each project adds its part to UpdateSystem or UpdateWorkspace and replaces this test with
// its own. Until then the migration touches neither database: sqlmock fails on any statement
// it was not told to expect.
func TestV45Migration_StartsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	m := &V45Migration{}
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	assert.NoError(t, mock.ExpectationsWereMet())
}
