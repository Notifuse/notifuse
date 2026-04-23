package migrations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V30Migration bundles two independent data rewrites:
//
//  1. Webhook subscription secret rotation.
//     Rotates webhook_subscriptions.secret into the Standard Webhooks format
//     `whsec_<base64(32 random bytes)>`. The old format (raw base64 without
//     prefix, used directly as HMAC-key bytes) does not match the Standard
//     Webhooks spec and the published docs, so every existing secret is
//     regenerated. URLs, names, settings, enabled state, and delivery history
//     are preserved. Consumers must copy the new secret from the console and
//     update their verification code to the spec-compliant form (see docs).
//
//  2. Europe/Kiev → Europe/Kyiv rename.
//     IANA tzdata 2022b renamed Europe/Kiev to Europe/Kyiv. We drop "Kiev"
//     from the UI dropdown and rewrite every stored occurrence to the
//     canonical name. Stored values with "Europe/Kiev" continued to work at
//     runtime (Go's tzdata still resolves the alias), but users would have
//     seen an empty selection in the UI. The webhook_contacts and
//     contact_changes_trigger on the contacts table are briefly disabled
//     around that UPDATE so the rename does not emit contact.updated webhook
//     events, pollute contact_timeline, or trigger cascading segment
//     recomputation for a no-op rename.
//
//     Caveat: ALTER TABLE ... DISABLE TRIGGER takes an AccessExclusiveLock on
//     contacts, held until the transaction commits. On workspaces with many
//     Kiev-timezoned contacts this can stall reads/writes against the
//     contacts table for a few seconds. Migrations run at startup so this is
//     an acceptable brief window in practice.
type V30Migration struct{}

func (m *V30Migration) GetMajorVersion() float64 {
	return 30.0
}

func (m *V30Migration) HasSystemUpdate() bool {
	return true
}

func (m *V30Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V30Migration) ShouldRestartServer() bool {
	return false
}

// UpdateSystem rewrites Europe/Kiev to Europe/Kyiv inside the workspaces.settings
// JSONB column on the system database.
func (m *V30Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	_, err := db.ExecContext(ctx, `
		UPDATE workspaces
		SET settings = jsonb_set(settings, '{timezone}', '"Europe/Kyiv"'),
		    updated_at = NOW()
		WHERE settings->>'timezone' = 'Europe/Kiev'
	`)
	if err != nil {
		return fmt.Errorf("failed to rewrite workspace timezone Europe/Kiev -> Europe/Kyiv: %w", err)
	}
	return nil
}

// UpdateWorkspace rotates webhook subscription secrets and rewrites the
// Europe/Kiev timezone alias across the workspace DB.
func (m *V30Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// 1. Webhook secret rotation.
	// Secrets are generated in Go using crypto/rand so the migration does not
	// depend on the pgcrypto extension. Format matches the Standard Webhooks
	// spec and the runtime generator in webhook_subscription_service.go:
	// `whsec_<base64(32 random bytes)>`.
	rows, err := db.QueryContext(ctx, `SELECT id FROM webhook_subscriptions`)
	if err != nil {
		return fmt.Errorf("failed to list webhook subscriptions for workspace %s: %w", workspace.ID, err)
	}
	var subscriptionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan webhook subscription id for workspace %s: %w", workspace.ID, err)
		}
		subscriptionIDs = append(subscriptionIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate webhook subscriptions for workspace %s: %w", workspace.ID, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close webhook subscriptions cursor for workspace %s: %w", workspace.ID, err)
	}
	for _, id := range subscriptionIDs {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return fmt.Errorf("failed to generate webhook secret for workspace %s: %w", workspace.ID, err)
		}
		secret := "whsec_" + base64.StdEncoding.EncodeToString(buf)
		if _, err := db.ExecContext(ctx,
			`UPDATE webhook_subscriptions SET secret = $1, updated_at = NOW() WHERE id = $2`,
			secret, id); err != nil {
			return fmt.Errorf("failed to rotate webhook secret for workspace %s subscription %s: %w", workspace.ID, id, err)
		}
	}

	// 2. Europe/Kiev -> Europe/Kyiv across workspace tables.
	// Suppress the contacts triggers around the UPDATE so an alias rewrite does
	// not emit contact.updated webhooks or fill contact_timeline with rename
	// entries.
	stmts := []struct {
		label string
		sql   string
	}{
		{"disable webhook_contacts trigger", `ALTER TABLE contacts DISABLE TRIGGER webhook_contacts`},
		{"disable contact_changes_trigger", `ALTER TABLE contacts DISABLE TRIGGER contact_changes_trigger`},
		{"rewrite contacts.timezone", `UPDATE contacts SET timezone = 'Europe/Kyiv' WHERE timezone = 'Europe/Kiev'`},
		{"enable contact_changes_trigger", `ALTER TABLE contacts ENABLE TRIGGER contact_changes_trigger`},
		{"enable webhook_contacts trigger", `ALTER TABLE contacts ENABLE TRIGGER webhook_contacts`},
		{"rewrite segments.timezone", `UPDATE segments SET timezone = 'Europe/Kyiv' WHERE timezone = 'Europe/Kiev'`},
		{"rewrite broadcasts.schedule.timezone",
			`UPDATE broadcasts
			 SET schedule = jsonb_set(schedule, '{timezone}', '"Europe/Kyiv"')
			 WHERE schedule->>'timezone' = 'Europe/Kiev'`},
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("workspace %s: %s: %w", workspace.ID, s.label, err)
		}
	}
	return nil
}

func init() {
	Register(&V30Migration{})
}
