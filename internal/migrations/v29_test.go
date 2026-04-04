package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
)

func TestV29Migration_GetMajorVersion(t *testing.T) {
	m := &V29Migration{}
	assert.Equal(t, 29.0, m.GetMajorVersion())
}

func TestV29Migration_HasSystemUpdate(t *testing.T) {
	m := &V29Migration{}
	assert.True(t, m.HasSystemUpdate())
}

func TestV29Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V29Migration{}
	assert.False(t, m.HasWorkspaceUpdate())
}

func TestV29Migration_ShouldRestartServer(t *testing.T) {
	m := &V29Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV29Migration_UpdateSystem_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V29Migration{}
	cfg := &config.Config{}

	// Expect 5 INSERT queries (one per key mapping)
	for i := 0; i < 5; i++ {
		mock.ExpectExec(`INSERT INTO settings`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV29Migration_UpdateSystem_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V29Migration{}
	cfg := &config.Config{}

	// First query fails
	mock.ExpectExec(`INSERT INTO settings`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(assert.AnError)

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to migrate setting")
}

func TestV29Migration_UpdateWorkspace(t *testing.T) {
	m := &V29Migration{}
	cfg := &config.Config{}

	// UpdateWorkspace should be a no-op
	err := m.UpdateWorkspace(context.Background(), cfg, nil, nil)
	assert.NoError(t, err)
}

func TestV29Migration_Registration(t *testing.T) {
	// Verify the migration is registered
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 29.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V29Migration should be registered")
}
