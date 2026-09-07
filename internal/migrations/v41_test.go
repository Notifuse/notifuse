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

func TestV41Migration_Metadata(t *testing.T) {
	m := &V41Migration{}
	assert.Equal(t, 41.0, m.GetMajorVersion())
	// System only: the audit log lives in the system database so that it
	// outlives the workspaces it describes. The dispatcher must not open a single
	// workspace connection for this migration.
	assert.True(t, m.HasSystemUpdate())
	assert.False(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV41Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(41.0)
	require.True(t, ok, "v41 must be registered, or the version stamp never reaches 41")
	assert.IsType(t, &V41Migration{}, migration)
}

func TestV41Migration_IsWhatLetsTheStampReachFortyOne(t *testing.T) {
	var selected []float64
	for _, m := range GetRegisteredMigrations() {
		v := m.GetMajorVersion()
		if v > 40.0 && v <= 41.0 {
			selected = append(selected, v)
		}
	}
	assert.Equal(t, []float64{41.0}, selected,
		"a database stamped 40 upgrading to a 41.0 build must select exactly one migration")
}

// The code version and the registered migrations are two halves of one fact.
func TestV41Migration_MatchesTheCodeVersion(t *testing.T) {
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

// The "two places" rule: what an upgrade runs must be byte-identical to what a
// fresh install runs. QueryMatcherEqual makes sqlmock compare the whole
// statement, so a migration that drifted from schema.AuditLogsTableDefinitions
// by a single character fails here.
func TestV41Migration_UpdateSystem_RunsTheSharedDDLVerbatimAndInOrder(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	defs := schema.AuditLogsTableDefinitions()
	require.NotEmpty(t, defs)
	for _, stmt := range defs {
		mock.ExpectExec(stmt).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	m := &V41Migration{}
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))

	// sqlmock fails on any statement it was not told to expect, so this also
	// pins that v41 backfills no permissions: there is no UPDATE user_workspaces
	// here, and there must not be — audit_logs is an opt-in resource.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV41Migration_UpdateSystem_StopsAtTheFirstFailureWithAPrefix(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	defs := schema.AuditLogsTableDefinitions()
	mock.ExpectExec(defs[0]).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(defs[1]).WillReturnError(errors.New("boom"))

	m := &V41Migration{}
	err = m.UpdateSystem(context.Background(), &config.Config{}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "v41: failed to create audit_logs in system database")
	assert.Contains(t, err.Error(), "boom")
	assert.NoError(t, mock.ExpectationsWereMet(), "the third statement must never run after the second failed")
}

func TestV41Migration_UpdateWorkspace_TouchesNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	m := &V41Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	assert.NoError(t, mock.ExpectationsWereMet())
}
