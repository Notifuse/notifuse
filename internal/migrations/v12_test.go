package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV12Migration_GetMajorVersion(t *testing.T) {
	migration := &V12Migration{}
	assert.Equal(t, 12.0, migration.GetMajorVersion(), "V12Migration should return version 12.0")
}

func TestV12Migration_HasSystemUpdate(t *testing.T) {
	migration := &V12Migration{}
	assert.True(t, migration.HasSystemUpdate(), "V12Migration should have system updates")
}

func TestV12Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V12Migration{}
	assert.False(t, migration.HasWorkspaceUpdate(), "V12Migration should not have workspace updates")
}

func TestV12Migration_UpdateWorkspace(t *testing.T) {
	migration := &V12Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - No workspace updates", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err, "UpdateWorkspace should succeed with no operations")
	})
}

func TestV12Migration_UpdateSystem(t *testing.T) {
	migration := &V12Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("Success - No workspaces", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock query for workspaces - return no rows
		rows := sqlmock.NewRows([]string{"id", "integrations"})
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Workspaces with no integrations", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock query for workspaces - return workspace with empty integrations
		emptyIntegrations, _ := json.Marshal([]domain.Integration{})
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", emptyIntegrations)
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Workspace with email integration without rate limit", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Create integration without rate limit
		integrations := []domain.Integration{
			{
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:                domain.EmailProviderKindSMTP,
					RateLimitPerMinute:  0, // No rate limit set
				},
			},
		}
		integrationsJSON, _ := json.Marshal(integrations)

		// Mock query for workspaces
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", integrationsJSON)
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		// Mock UPDATE to set rate limit
		mock.ExpectExec("UPDATE workspaces").
			WithArgs(sqlmock.AnyArg(), "workspace1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Workspace with email integration with existing rate limit", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Create integration with existing rate limit
		integrations := []domain.Integration{
			{
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:                domain.EmailProviderKindSMTP,
					RateLimitPerMinute:  50, // Rate limit already set
				},
			},
		}
		integrationsJSON, _ := json.Marshal(integrations)

		// Mock query for workspaces
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", integrationsJSON)
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		// No UPDATE should be called since rate limit is already set

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Multiple workspaces with mixed integrations", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Create different integration scenarios
		integrations1 := []domain.Integration{
			{
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:                domain.EmailProviderKindSMTP,
					RateLimitPerMinute:  0, // Needs update
				},
			},
		}
		integrations2 := []domain.Integration{
			{
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:                domain.EmailProviderKindMailgun,
					RateLimitPerMinute:  100, // Already set
				},
			},
		}
		integrations3 := []domain.Integration{
			{
				Type: "webhook", // Non-email integration
				EmailProvider: domain.EmailProvider{
					RateLimitPerMinute: 0,
				},
			},
		}

		integrationsJSON1, _ := json.Marshal(integrations1)
		integrationsJSON2, _ := json.Marshal(integrations2)
		integrationsJSON3, _ := json.Marshal(integrations3)

		// Mock query for workspaces
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", integrationsJSON1).
			AddRow("workspace2", integrationsJSON2).
			AddRow("workspace3", integrationsJSON3)
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		// Only workspace1 should be updated (workspace2 has rate limit, workspace3 is not email)
		mock.ExpectExec("UPDATE workspaces").
			WithArgs(sqlmock.AnyArg(), "workspace1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Workspace with multiple email integrations", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Create multiple email integrations
		integrations := []domain.Integration{
			{
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:                domain.EmailProviderKindSMTP,
					RateLimitPerMinute:  0, // Needs update
				},
			},
			{
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:                domain.EmailProviderKindMailgun,
					RateLimitPerMinute:  0, // Needs update
				},
			},
		}
		integrationsJSON, _ := json.Marshal(integrations)

		// Mock query for workspaces
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", integrationsJSON)
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		// Mock UPDATE
		mock.ExpectExec("UPDATE workspaces").
			WithArgs(sqlmock.AnyArg(), "workspace1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Query workspaces fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock query error
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query workspaces")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Scan workspace row fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock query with invalid data that will fail scanning
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", "invalid-json-data")
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal integrations")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - UPDATE workspaces fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Create integration without rate limit
		integrations := []domain.Integration{
			{
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:                domain.EmailProviderKindSMTP,
					RateLimitPerMinute:  0,
				},
			},
		}
		integrationsJSON, _ := json.Marshal(integrations)

		// Mock query for workspaces
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", integrationsJSON)
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		// Mock UPDATE failure
		mock.ExpectExec("UPDATE workspaces").
			WithArgs(sqlmock.AnyArg(), "workspace1").
			WillReturnError(sql.ErrConnDone)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update workspace integrations")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Workspace with NULL integrations", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock query for workspaces with NULL integrations
		rows := sqlmock.NewRows([]string{"id", "integrations"}).
			AddRow("workspace1", nil)
		mock.ExpectQuery("SELECT id, integrations FROM workspaces").WillReturnRows(rows)

		err = migration.UpdateSystem(ctx, cfg, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestV12Migration_Registration(t *testing.T) {
	// Test that the migration is registered in the default registry
	migration, exists := GetRegisteredMigration(12.0)
	assert.True(t, exists, "V12Migration should be registered")
	assert.NotNil(t, migration, "V12Migration should not be nil")
	assert.Equal(t, 12.0, migration.GetMajorVersion(), "Registered migration should be version 12.0")
}

