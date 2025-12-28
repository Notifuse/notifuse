package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V22Migration adds LLM permissions to existing users
type V22Migration struct{}

func (m *V22Migration) GetMajorVersion() float64 {
	return 22.0
}

func (m *V22Migration) HasSystemUpdate() bool {
	return true
}

func (m *V22Migration) HasWorkspaceUpdate() bool {
	return false
}

func (m *V22Migration) ShouldRestartServer() bool {
	return false
}

func (m *V22Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Add llm permissions to all existing user workspaces
	_, err := db.ExecContext(ctx, `
		UPDATE user_workspaces
		SET permissions = permissions || '{"llm": {"read": true, "write": true}}'::jsonb
		WHERE permissions IS NOT NULL
		AND NOT permissions ? 'llm'
	`)
	if err != nil {
		return fmt.Errorf("failed to add llm permissions to user workspaces: %w", err)
	}

	// Add llm permissions to all existing workspace invitations
	_, err = db.ExecContext(ctx, `
		UPDATE workspace_invitations
		SET permissions = permissions || '{"llm": {"read": true, "write": true}}'::jsonb
		WHERE permissions IS NOT NULL
		AND NOT permissions ? 'llm'
	`)
	if err != nil {
		return fmt.Errorf("failed to add llm permissions to workspace invitations: %w", err)
	}

	return nil
}

func (m *V22Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

func init() {
	Register(&V22Migration{})
}
