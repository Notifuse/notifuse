package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/database/schema"
)

// oauthSystemV45Statements is the MCP access part of the v45 system migration:
// the tables of the OAuth authorization server, from the schema package, which
// internal/database/init.go also uses, so a system database created at 45 and
// one migrated to it cannot differ.
func oauthSystemV45Statements() []string {
	return schema.OAuthTableDefinitions()
}

// applyOAuthSystemV45 runs the OAuth part of the v45 system migration in the
// migration's transaction. It is idempotent.
func applyOAuthSystemV45(ctx context.Context, db DBExecutor) error {
	for _, statement := range oauthSystemV45Statements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("oauth tables (v45 system): %w", err)
		}
	}
	return nil
}
