package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V29Migration renames SMTP Relay settings keys to SMTP Bridge.
//
// This migration:
// - System: copies smtp_relay_* settings keys to smtp_bridge_* equivalents
type V29Migration struct{}

func (m *V29Migration) GetMajorVersion() float64 {
	return 29.0
}

func (m *V29Migration) HasSystemUpdate() bool {
	return true
}

func (m *V29Migration) HasWorkspaceUpdate() bool {
	return false
}

func (m *V29Migration) ShouldRestartServer() bool {
	return false
}

func (m *V29Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Migrate settings keys from smtp_relay_* to smtp_bridge_*
	keyMappings := [][2]string{
		{"smtp_relay_enabled", "smtp_bridge_enabled"},
		{"smtp_relay_domain", "smtp_bridge_domain"},
		{"smtp_relay_port", "smtp_bridge_port"},
		{"encrypted_smtp_relay_tls_cert_base64", "encrypted_smtp_bridge_tls_cert_base64"},
		{"encrypted_smtp_relay_tls_key_base64", "encrypted_smtp_bridge_tls_key_base64"},
	}

	for _, mapping := range keyMappings {
		oldKey, newKey := mapping[0], mapping[1]
		// Copy old key to new key only if old key exists and new key does not
		_, err := db.ExecContext(ctx, `
			INSERT INTO settings (key, value)
			SELECT $1::text, value FROM settings WHERE key = $2
			AND NOT EXISTS (SELECT 1 FROM settings WHERE key = $3)
		`, newKey, oldKey, newKey)
		if err != nil {
			return fmt.Errorf("failed to migrate setting %s -> %s: %w", oldKey, newKey, err)
		}
	}

	return nil
}

func (m *V29Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

func init() {
	Register(&V29Migration{})
}
