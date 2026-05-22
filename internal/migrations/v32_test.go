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

func TestV32Migration_GetMajorVersion(t *testing.T) {
	m := &V32Migration{}
	assert.Equal(t, 32.0, m.GetMajorVersion())
}

func TestV32Migration_HasSystemUpdate(t *testing.T) {
	m := &V32Migration{}
	assert.True(t, m.HasSystemUpdate())
}

func TestV32Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V32Migration{}
	assert.False(t, m.HasWorkspaceUpdate())
}

func TestV32Migration_ShouldRestartServer(t *testing.T) {
	m := &V32Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV32Migration_UpdateSystem_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`ALTER TABLE users\s+ADD COLUMN IF NOT EXISTS language`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V32Migration{}
	err = m.UpdateSystem(context.Background(), &config.Config{}, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV32Migration_UpdateSystem_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`ALTER TABLE users`).WillReturnError(assert.AnError)

	m := &V32Migration{}
	err = m.UpdateSystem(context.Background(), &config.Config{}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add language column to users table")
}

func TestV32Migration_UpdateWorkspace_NoOp(t *testing.T) {
	m := &V32Migration{}
	// Workspace-side has nothing to do — just verify it returns nil cleanly.
	assert.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{},
		&domain.Workspace{ID: "ws_test"}, nil))
}

func TestV32Migration_Registered(t *testing.T) {
	// Verify init() registered the migration so the runner picks it up.
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 32.0 {
			return
		}
	}
	t.Fatal("V32Migration not registered")
}
