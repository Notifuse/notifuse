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

func TestV27Migration_GetMajorVersion(t *testing.T) {
	m := &V27Migration{}
	assert.Equal(t, 27.0, m.GetMajorVersion())
}

func TestV27Migration_HasSystemUpdate(t *testing.T) {
	m := &V27Migration{}
	assert.True(t, m.HasSystemUpdate())
}

func TestV27Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V27Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV27Migration_ShouldRestartServer(t *testing.T) {
	m := &V27Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV27Migration_UpdateSystem_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}

	// Expect ALTER TABLE for recurring_interval
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurring_interval`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect ALTER TABLE for integration_id
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS integration_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect CREATE INDEX for integration_id
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_tasks_integration_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect CREATE UNIQUE INDEX for workspace_integration_active
	mock.ExpectExec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_integration_active`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV27Migration_UpdateSystem_RecurringIntervalError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}

	// First query fails
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurring_interval`).
		WillReturnError(assert.AnError)

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add recurring_interval column")
}

func TestV27Migration_UpdateSystem_IntegrationIDError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}

	// First query succeeds
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurring_interval`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Second query fails
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS integration_id`).
		WillReturnError(assert.AnError)

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add integration_id column")
}

func TestV27Migration_UpdateSystem_IndexError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}

	// Column additions succeed
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurring_interval`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS integration_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Index creation fails
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_tasks_integration_id`).
		WillReturnError(assert.AnError)

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create integration_id index")
}

func TestV27Migration_UpdateSystem_UniqueIndexError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}

	// Column additions and first index succeed
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurring_interval`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS integration_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_tasks_integration_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Unique index creation fails
	mock.ExpectExec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_integration_active`).
		WillReturnError(assert.AnError)

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create workspace_integration unique index")
}

func TestV27Migration_UpdateWorkspace_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Expect ADD COLUMN for data_feed (consolidated column)
	mock.ExpectExec(`ALTER TABLE broadcasts`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect DROP INDEX for redundant idx_contacts_email
	mock.ExpectExec(`DROP INDEX IF EXISTS idx_contacts_email`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect CREATE INDEX for contact_lists.list_id
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_contact_lists_list_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV27Migration_UpdateWorkspace_DataFeedError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Query fails
	mock.ExpectExec(`ALTER TABLE broadcasts`).
		WillReturnError(assert.AnError)

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add data_feed column")
}

func TestV27Migration_UpdateWorkspace_ColumnAlreadyExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Query succeeds (column already exists due to IF NOT EXISTS)
	mock.ExpectExec(`ALTER TABLE broadcasts`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect DROP INDEX for redundant idx_contacts_email
	mock.ExpectExec(`DROP INDEX IF EXISTS idx_contacts_email`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect CREATE INDEX for contact_lists.list_id
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_contact_lists_list_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
