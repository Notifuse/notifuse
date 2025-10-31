package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V14Migration adds channel_options JSONB column to message_history table
// and sets telemetry/check_for_updates defaults for existing installations
type V14Migration struct{}

func (m *V14Migration) GetMajorVersion() float64 {
	return 14.0
}

func (m *V14Migration) HasSystemUpdate() bool {
	return true // System database changes for telemetry settings
}

func (m *V14Migration) HasWorkspaceUpdate() bool {
	return true // Workspace database changes
}

func (m *V14Migration) ShouldRestartServer() bool {
	return true // Restart required to reload telemetry config
}

func (m *V14Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// Check if system is installed
	var isInstalled string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'is_installed'").Scan(&isInstalled)
	if err != nil {
		if err == sql.ErrNoRows {
			// System not installed yet, skip
			return nil
		}
		return fmt.Errorf("failed to check is_installed: %w", err)
	}

	if isInstalled != "true" {
		// System not installed, skip
		return nil
	}

	// Set telemetry_enabled if not already set
	var telemetryExists string
	err = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'telemetry_enabled'").Scan(&telemetryExists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check telemetry_enabled: %w", err)
	}

	if err == sql.ErrNoRows {
		// Setting doesn't exist, check env var or default to true
		telemetryValue := "true"
		if envVal := os.Getenv("TELEMETRY"); envVal != "" {
			if envVal == "false" || envVal == "0" {
				telemetryValue = "false"
			}
		}

		_, err = db.ExecContext(ctx,
			"INSERT INTO settings (key, value, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())",
			"telemetry_enabled", telemetryValue)
		if err != nil {
			return fmt.Errorf("failed to insert telemetry_enabled: %w", err)
		}
	}

	// Set check_for_updates if not already set
	var checkUpdatesExists string
	err = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'check_for_updates'").Scan(&checkUpdatesExists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check check_for_updates: %w", err)
	}

	if err == sql.ErrNoRows {
		// Setting doesn't exist, check env var or default to true
		checkUpdatesValue := "true"
		if envVal := os.Getenv("CHECK_FOR_UPDATES"); envVal != "" {
			if envVal == "false" || envVal == "0" {
				checkUpdatesValue = "false"
			}
		}

		_, err = db.ExecContext(ctx,
			"INSERT INTO settings (key, value, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())",
			"check_for_updates", checkUpdatesValue)
		if err != nil {
			return fmt.Errorf("failed to insert check_for_updates: %w", err)
		}
	}

	return nil
}

func (m *V14Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add channel_options column to message_history table
	_, err := db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS channel_options JSONB
	`)
	if err != nil {
		return fmt.Errorf("failed to add channel_options column: %w", err)
	}

	return nil
}

func init() {
	Register(&V14Migration{})
}
