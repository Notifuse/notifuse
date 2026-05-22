package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V32Migration adds the per-user language preference.
//
// A new `language` column on the system `users` table stores each user's
// preferred locale. It is the single source of truth for both the console UI
// language and the language of the system emails (magic code, workspace
// invitation, circuit-breaker alert) sent to that user. Existing rows default
// to 'en'.
type V32Migration struct{}

func (m *V32Migration) GetMajorVersion() float64 {
	return 32.0
}

func (m *V32Migration) HasSystemUpdate() bool {
	return true
}

func (m *V32Migration) HasWorkspaceUpdate() bool {
	return false
}

func (m *V32Migration) ShouldRestartServer() bool {
	return false
}

func (m *V32Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	_, err := db.ExecContext(ctx, `
		ALTER TABLE users
		ADD COLUMN IF NOT EXISTS language VARCHAR(10) NOT NULL DEFAULT 'en'
	`)
	if err != nil {
		return fmt.Errorf("failed to add language column to users table: %w", err)
	}
	return nil
}

func (m *V32Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

func init() {
	Register(&V32Migration{})
}
