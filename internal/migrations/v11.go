package migrations

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V11Migration implements the migration from version 10.x to 11.0
// Marks existing deployments as installed without migrating env vars (env vars always win)
type V11Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V11Migration) GetMajorVersion() float64 {
	return 11.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V11Migration) HasSystemUpdate() bool {
	return true // Migrates settings to database
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V11Migration) HasWorkspaceUpdate() bool {
	return false // No workspace-level changes needed
}

// ShouldRestartServer indicates if the server should restart after this migration
func (m *V11Migration) ShouldRestartServer() bool {
	return false
}

// UpdateSystem executes system-level migration changes
// Marks existing installations as installed without migrating env vars to database
// With v11+, env vars always take precedence and should NOT be stored in database
func (m *V11Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Check if is_installed is already set
	var installedValue string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'is_installed'").Scan(&installedValue)
	if err == nil && installedValue == "true" {
		// Already installed and migrated
		return nil
	}

	// Check if this is an existing installation by looking for users
	var userCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return fmt.Errorf("failed to check for existing users: %w", err)
	}

	// If no users exist, this is a fresh installation - skip migration
	if userCount == 0 {
		return nil
	}

	// This is an existing installation with users
	// Check if required settings are configured via environment variables
	hasRequiredSettings := true

	// Check API endpoint
	if cfg.APIEndpoint == "" {
		hasRequiredSettings = false
	}

	// Check JWT secret (required for token signing)
	if len(cfg.Security.JWTSecret) == 0 || cfg.Security.SecretKey == "" {
		hasRequiredSettings = false
	}

	// Check SMTP settings (at minimum host and from email)
	if cfg.SMTP.Host == "" || cfg.SMTP.FromEmail == "" {
		hasRequiredSettings = false
	}

	// Only mark as installed if all required settings are present in env vars
	// NOTE: We do NOT migrate env var values to database - env vars always win
	// The database should only contain settings provided via setup wizard
	if hasRequiredSettings {
		now := time.Now().UTC()
		_, err := db.ExecContext(ctx, `
			INSERT INTO settings (key, value, created_at, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				updated_at = EXCLUDED.updated_at
		`, "is_installed", "true", now, now)
		if err != nil {
			return fmt.Errorf("failed to set is_installed: %w", err)
		}
	}

	return nil
}

// UpdateWorkspace executes workspace-level migration changes (none for v11)
func (m *V11Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V11Migration{})
}
