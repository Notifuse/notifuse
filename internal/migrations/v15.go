package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V15Migration migrates from PASETO to JWT authentication
// - Removes PASETO key settings from database
// - Invalidates all API keys (PASETO tokens incompatible with JWT)
type V15Migration struct{}

func (m *V15Migration) GetMajorVersion() float64 {
	return 15.0
}

func (m *V15Migration) HasSystemUpdate() bool {
	return true
}

func (m *V15Migration) HasWorkspaceUpdate() bool {
	return false
}

func (m *V15Migration) ShouldRestartServer() bool {
	return false // No restart needed - JWT config already loaded in memory
}

func (m *V15Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// CRITICAL: Check for SECRET_KEY before migrating
	// Use cfg.Security.SecretKey which has already been loaded from env/viper (supports .env files)
	secretKey := cfg.Security.SecretKey

	if secretKey == "" {
		return fmt.Errorf(`MIGRATION FAILED: SECRET_KEY is required for v15 migration

This version migrates from PASETO to JWT authentication (HS256 symmetric signing).
You must set the SECRET_KEY before upgrading.

Options:
1. Set SECRET_KEY in your .env file or environment (recommended):
   SECRET_KEY=$(openssl rand -base64 32)

2. Or reuse your existing PASETO_PRIVATE_KEY for backward compatibility:
   SECRET_KEY="$PASETO_PRIVATE_KEY"

After setting SECRET_KEY, restart the application to complete the migration.

BREAKING CHANGES:
- All users will need to log in again
- All API keys will be invalidated and must be regenerated`)
	}

	// Log migration warnings
	fmt.Println("================================================================================")
	fmt.Println("NOTIFUSE v15.0 MIGRATION - PASETO → JWT + Magic Code Security")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("⚠️  BREAKING CHANGES:")
	fmt.Println("   • All user sessions will be invalidated")
	fmt.Println("   • All API keys will be deleted (incompatible with JWT)")
	fmt.Println("   • All pending workspace invitations will be invalidated")
	fmt.Println("   • All active magic codes will be cleared (migrating to HMAC-SHA256)")
	fmt.Println()
	fmt.Println("🔒 SECURITY IMPROVEMENTS:")
	fmt.Println("   • Magic codes now stored as HMAC-SHA256 hashes (no plain text)")
	fmt.Println("   • Database compromise cannot reveal authentication codes")
	fmt.Println()
	fmt.Println("📋 POST-MIGRATION ACTIONS REQUIRED:")
	fmt.Println("   1. Users must log in again with their passwords")
	fmt.Println("   2. API key holders must regenerate keys in Settings → API Keys")
	fmt.Println("   3. Update all integrations with new API keys")
	fmt.Println("   4. Workspace admins must resend pending invitations")
	fmt.Println()
	fmt.Println("================================================================================")

	// Count items that will be deleted for reporting
	var apiKeyCount, invitationCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE type = 'api_key'").Scan(&apiKeyCount)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_invitations WHERE expires_at > NOW()").Scan(&invitationCount)

	// Perform database schema migration
	queries := []string{
		// Delete PASETO key settings (no longer used with JWT)
		`DELETE FROM settings WHERE key = 'encrypted_paseto_private_key';`,
		`DELETE FROM settings WHERE key = 'encrypted_paseto_public_key';`,

		// CRITICAL: Invalidate all existing API keys
		// API keys are stored as users with type='api_key'
		// PASETO-signed tokens cannot be verified with JWT system
		// Users MUST regenerate API keys after migration
		`DELETE FROM users WHERE type = 'api_key';`,

		// CRITICAL: Invalidate all pending workspace invitations
		// Invitation tokens contain PASETO-signed URLs that won't validate with JWT
		// Admins must resend invitations after migration
		// Only delete non-expired invitations (expired ones are already invalid)
		`DELETE FROM workspace_invitations WHERE expires_at > NOW();`,

		// SECURITY: Clear all existing plain-text magic codes
		// Magic codes are now stored as HMAC-SHA256 hashes for security
		// Plain-text codes from v14 are incompatible with v15 HMAC verification
		// Users with active codes will need to request a new code
		`UPDATE user_sessions SET magic_code = NULL, magic_code_expires_at = NULL WHERE magic_code IS NOT NULL;`,
	}

	for i, query := range queries {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("migration query %d failed: %w", i+1, err)
		}
	}

	fmt.Println("✅ Migration completed successfully")
	fmt.Println()
	fmt.Println("📊 SUMMARY:")
	fmt.Printf("   • Deleted %d API key(s)\n", apiKeyCount)
	fmt.Printf("   • Deleted %d pending invitation(s)\n", invitationCount)
	if invitationCount > 0 {
		fmt.Println()
		fmt.Println("💡 TIP: Workspace admins should resend invitations via:")
		fmt.Println("   Settings → Members → Invitations → Resend")
	}
	fmt.Println()

	return nil
}

func (m *V15Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// No workspace-specific changes needed
	return nil
}

func init() {
	Register(&V15Migration{})
}
