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

func TestV31Migration_GetMajorVersion(t *testing.T) {
	m := &V31Migration{}
	assert.Equal(t, 31.0, m.GetMajorVersion())
}

func TestV31Migration_HasSystemUpdate(t *testing.T) {
	m := &V31Migration{}
	assert.False(t, m.HasSystemUpdate())
}

func TestV31Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V31Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV31Migration_ShouldRestartServer(t *testing.T) {
	m := &V31Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV31Migration_UpdateSystem_NoOp(t *testing.T) {
	m := &V31Migration{}
	// System-side has nothing to do — just verify it returns nil cleanly.
	assert.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, nil))
}

func TestV31Migration_UpdateWorkspace_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`CREATE OR REPLACE FUNCTION queue_contact_for_segment_recomputation`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V31Migration{}
	err = m.UpdateWorkspace(context.Background(), &config.Config{},
		&domain.Workspace{ID: "ws_test"}, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV31Migration_UpdateWorkspace_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`CREATE OR REPLACE FUNCTION queue_contact_for_segment_recomputation`).
		WillReturnError(assert.AnError)

	m := &V31Migration{}
	err = m.UpdateWorkspace(context.Background(), &config.Config{},
		&domain.Workspace{ID: "ws_test"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update queue_contact_for_segment_recomputation")
	assert.Contains(t, err.Error(), "ws_test")
}

func TestV31Migration_Registered(t *testing.T) {
	// Verify init() registered the migration so the runner picks it up.
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 31.0 {
			return
		}
	}
	t.Fatal("V31Migration not registered")
}
