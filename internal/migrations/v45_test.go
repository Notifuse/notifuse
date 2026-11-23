package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
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

// The "two places" rule: what an upgrade runs must be byte-identical to what a fresh
// install runs. QueryMatcherEqual makes sqlmock compare the whole statement.
func TestV45Migration_UpdateSystem_RunsTheTwoFactorDDLVerbatimAndInOrder(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	for _, stmt := range schema.TwoFactorTableDefinitions() {
		mock.ExpectExec(stmt).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	m := &V45Migration{}
	require.True(t, m.HasSystemUpdate())
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV45Migration_UpdateSystem_StopsAtTheFirstFailureWithAPrefix(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	defs := schema.TwoFactorTableDefinitions()
	mock.ExpectExec(defs[0]).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(defs[1]).WillReturnError(errors.New("boom"))

	err = (&V45Migration{}).UpdateSystem(context.Background(), &config.Config{}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "v45: failed to create the two-factor authentication tables")
	assert.Contains(t, err.Error(), "boom")
	assert.NoError(t, mock.ExpectationsWereMet(), "the third statement must never run after the second failed")
}

// No workspace part yet: the dispatcher must not open a workspace connection for v45.
func TestV45Migration_UpdateWorkspace_TouchesNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	m := &V45Migration{}
	assert.False(t, m.HasWorkspaceUpdate())
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	assert.NoError(t, mock.ExpectationsWereMet())
}
