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
	assert.False(t, m.HasSystemUpdate())
}

func TestV27Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V27Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV27Migration_ShouldRestartServer(t *testing.T) {
	m := &V27Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV27Migration_UpdateSystem(t *testing.T) {
	m := &V27Migration{}
	cfg := &config.Config{}

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	err = m.UpdateSystem(context.Background(), cfg, db)
	assert.NoError(t, err)
}

func TestV27Migration_UpdateWorkspace_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V27Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Expect single ADD COLUMN for data_feed (consolidated column)
	mock.ExpectExec(`ALTER TABLE broadcasts`).
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

	err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
