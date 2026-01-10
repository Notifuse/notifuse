package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V24Migration fixes automation stats field that may contain JSONB scalar values
// instead of proper JSONB objects.
//
// Root cause: When creating automations, if Stats was nil (not provided by frontend),
// json.Marshal(nil) produced "null" which was stored as JSONB null - a scalar value.
// Later, when automation_enroll_contact tries to use jsonb_set on the stats field,
// PostgreSQL throws "cannot set path in scalar" because jsonb_set cannot operate
// on scalar values (null, numbers, strings, booleans).
//
// This migration:
// 1. Fixes any existing automations with scalar stats to use empty object '{}'
// 2. The code fix in automation_postgres.go prevents new automations from having this issue
type V24Migration struct{}

func (m *V24Migration) GetMajorVersion() float64 {
	return 24.0
}

func (m *V24Migration) HasSystemUpdate() bool {
	return false
}

func (m *V24Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V24Migration) ShouldRestartServer() bool {
	return false
}

func (m *V24Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V24Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Fix any automations where stats is not a JSONB object
	// This handles: NULL, null (JSONB), numbers, strings, booleans, arrays
	_, err := db.ExecContext(ctx, `
		UPDATE automations
		SET stats = '{}'::jsonb,
		    updated_at = NOW()
		WHERE stats IS NULL
		   OR jsonb_typeof(stats) != 'object'
	`)
	if err != nil {
		return fmt.Errorf("failed to fix automation stats: %w", err)
	}

	return nil
}

func init() {
	Register(&V24Migration{})
}
