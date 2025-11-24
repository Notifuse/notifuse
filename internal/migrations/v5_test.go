package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV5Migration_GetMajorVersion(t *testing.T) {
	migration := &V5Migration{}
	assert.Equal(t, 5.0, migration.GetMajorVersion())
}

func TestV5Migration_HasSystemUpdate(t *testing.T) {
	migration := &V5Migration{}
	assert.False(t, migration.HasSystemUpdate())
}

func TestV5Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V5Migration{}
	assert.True(t, migration.HasWorkspaceUpdate())
}

func TestV5Migration_UpdateSystem(t *testing.T) {
	migration := &V5Migration{}
	ctx := context.Background()
	config := &config.Config{}

	// Should return nil since no system updates
	err := migration.UpdateSystem(ctx, config, nil)
	assert.NoError(t, err)
}

func TestV5Migration_UpdateWorkspace_Success(t *testing.T) {
	migration := &V5Migration{}
	ctx := context.Background()
	config := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Mock the ALTER TABLE query
	mock.ExpectExec("ALTER TABLE broadcasts ADD COLUMN IF NOT EXISTS pause_reason TEXT").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Execute migration
	err = migration.UpdateWorkspace(ctx, config, workspace, db)
	assert.NoError(t, err)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV5Migration_UpdateWorkspace_AlterTableFails(t *testing.T) {
	migration := &V5Migration{}
	ctx := context.Background()
	config := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace"}

	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Mock the ALTER TABLE query to fail
	mock.ExpectExec("ALTER TABLE broadcasts ADD COLUMN IF NOT EXISTS pause_reason TEXT").
		WillReturnError(sql.ErrConnDone)

	// Execute migration
	err = migration.UpdateWorkspace(ctx, config, workspace, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add pause_reason column to broadcasts table")

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV5Migration_Registration(t *testing.T) {
	// Test that V5Migration is registered in the default registry
	migration, exists := GetRegisteredMigration(5.0)
	assert.True(t, exists, "V5Migration should be registered")
	assert.NotNil(t, migration, "V5Migration should not be nil")
	assert.IsType(t, &V5Migration{}, migration, "Should be V5Migration type")
}
