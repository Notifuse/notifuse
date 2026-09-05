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

func TestV40Migration_Metadata(t *testing.T) {
	m := &V40Migration{}
	assert.Equal(t, 40.0, m.GetMajorVersion())
	// Both false on purpose: v40 is a licence change, not a schema change. The
	// dispatcher connects only to the databases a migration declares, so this
	// opens one empty transaction against the system database and no workspace
	// connections at all.
	assert.False(t, m.HasSystemUpdate())
	assert.False(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV40Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(40.0)
	require.True(t, ok, "v40 must be registered, or the version stamp never reaches 40")
	assert.IsType(t, &V40Migration{}, migration)
}

// The reason this migration exists at all. Manager.RunMigrations writes the new
// version only after the migration loop, behind an early return taken when the
// selection is empty — so a code version with no migration at or below it that
// is above the database version leaves the stamp where it was, on every boot,
// for the life of the release.
func TestV40Migration_IsWhatLetsTheStampReachForty(t *testing.T) {
	var selected []float64
	for _, m := range GetRegisteredMigrations() {
		v := m.GetMajorVersion()
		if v > 39.0 && v <= 40.0 {
			selected = append(selected, v)
		}
	}
	assert.Equal(t, []float64{40.0}, selected,
		"a database stamped 39 upgrading to a 40.0 build must select exactly one migration; "+
			"select none and Manager.RunMigrations returns before SetCurrentDBVersion")
}

// The code version and the registered migrations are two halves of one fact.
func TestV40Migration_MatchesTheCodeVersion(t *testing.T) {
	code, err := GetCurrentCodeVersion()
	require.NoError(t, err)

	var highest float64
	for _, m := range GetRegisteredMigrations() {
		if v := m.GetMajorVersion(); v > highest {
			highest = v
		}
	}
	assert.Equal(t, highest, code,
		"config.VERSION (%v) and the highest registered migration (%v) must agree: a VERSION "+
			"above every migration never stamps, and a migration above VERSION never runs",
		code, highest)
}

func TestV40Migration_TouchesNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	m := &V40Migration{}
	cfg := &config.Config{}

	require.NoError(t, m.UpdateSystem(context.Background(), cfg, db))
	require.NoError(t, m.UpdateWorkspace(context.Background(), cfg, &domain.Workspace{ID: "ws1"}, db))

	// sqlmock fails on any statement it was not told to expect, and it was told
	// to expect none: a v40 that quietly grew a statement would fail here.
	assert.NoError(t, mock.ExpectationsWereMet())
}
