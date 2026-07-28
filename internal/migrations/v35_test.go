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

func TestV35Migration_GetMajorVersion(t *testing.T) {
	m := &V35Migration{}
	assert.Equal(t, 35.0, m.GetMajorVersion())
}

func TestV35Migration_HasSystemUpdate(t *testing.T) {
	m := &V35Migration{}
	assert.False(t, m.HasSystemUpdate())
}

func TestV35Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V35Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV35Migration_ShouldRestartServer(t *testing.T) {
	m := &V35Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV35Migration_UpdateSystem(t *testing.T) {
	m := &V35Migration{}
	cfg := &config.Config{}

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
}

func TestV35Migration_UpdateWorkspace_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Step 1: Expect contact_lists status update
	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnResult(sqlmock.NewResult(0, 5))

	// Step 2: Expect automation nodes update
	mock.ExpectExec(`UPDATE automations`).
		WillReturnResult(sqlmock.NewResult(0, 2))

	// Step 3: Expect contact_timeline update
	mock.ExpectExec(`UPDATE contact_timeline`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	// Step 4: Expect automation stats recomputation
	mock.ExpectExec(`UPDATE automations a`).
		WillReturnResult(sqlmock.NewResult(0, 10))

	// Step 5: Expect clicked_links column addition
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS clicked_links JSONB`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Step 6: Expect supabase notifications tracking opt-out
	mock.ExpectExec(`UPDATE transactional_notifications`).
		WillReturnResult(sqlmock.NewResult(0, 6))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV35Migration_UpdateWorkspace_HandlesEmptyTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// All updates affect 0 rows - should still succeed
	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contact_timeline`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations a`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS clicked_links JSONB`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE transactional_notifications`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV35Migration_UpdateWorkspace_ContactListsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// First query fails
	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update contact_lists status")
}

func TestV35Migration_UpdateWorkspace_AutomationNodesError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update automation nodes")
}

func TestV35Migration_UpdateWorkspace_TimelineError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contact_timeline`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update contact_timeline changes")
}

func TestV35Migration_UpdateWorkspace_StatsRecomputeError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contact_timeline`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations a`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to recompute automation stats")
}

func TestV35Migration_UpdateWorkspace_AddClickedLinksColumnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contact_timeline`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations a`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS clicked_links JSONB`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add clicked_links column")
}

func TestV35Migration_UpdateWorkspace_TrackingSettingsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V35Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	mock.ExpectExec(`UPDATE contact_lists`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contact_timeline`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE automations a`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS clicked_links JSONB`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE transactional_notifications`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update supabase notification tracking settings")
}
