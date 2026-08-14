package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V34Migration adds OIDC / SSO support: a new federated_identities system table
// keyed by the durable (idp_issuer, idp_sub) pair linking an external IdP subject
// to a Notifuse user (email is NOT used for authentication once a link exists). The
// SQL is kept identical to system_tables.go to avoid new-install / migrated drift.
type V34Migration struct{}

func (m *V34Migration) GetMajorVersion() float64 { return 34.0 }
func (m *V34Migration) HasSystemUpdate() bool     { return true }
func (m *V34Migration) HasWorkspaceUpdate() bool  { return false }
func (m *V34Migration) ShouldRestartServer() bool { return false }

func (m *V34Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS federated_identities (
			id          UUID PRIMARY KEY,
			user_id     UUID NOT NULL,
			idp_issuer  VARCHAR(255) NOT NULL,
			idp_sub     VARCHAR(255) NOT NULL,
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (idp_issuer, idp_sub),
			UNIQUE (user_id, idp_issuer)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federated_identities_user_id ON federated_identities (user_id)`,
		// Functional index backing GetUserByEmailInsensitive: the OIDC bridge matches an
		// invited account case-insensitively, and without this index that lookup cannot
		// use the plain email index.
		`CREATE INDEX IF NOT EXISTS idx_users_lower_email ON users (lower(email))`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'federated_identities_user_id_fkey'
				AND conrelid = 'federated_identities'::regclass
			) THEN
				ALTER TABLE federated_identities
					ADD CONSTRAINT federated_identities_user_id_fkey
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
			END IF;
		END $$`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("v34 system migration failed: %w", err)
		}
	}
	return nil
}

func (m *V34Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

func init() { Register(&V34Migration{}) }
