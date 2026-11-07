//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestV39WebhookContactsFunctionBeforeV43 upgrades a workspace that has none of
// the tracking consent columns, the way a database below 39 meets the 43.0
// binary: v39 to v42 each commit on their own before v43 adds the columns.
// Contacts must stay writable in between. A webhook_contacts_trigger() body that
// names NEW.tracking_opens_consent installs without complaint, since PL/pgSQL
// resolves record fields when a statement runs, and then fails every UPDATE on
// contacts with `record "new" has no field`.
func TestV39WebhookContactsFunctionBeforeV43(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	bg := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	email := fmt.Sprintf("v39-contacts-%s@example.com", uuid.New().String()[:8])
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	_, err = db.ExecContext(bg, `
		INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ('sub-v39-contacts', 'Contacts', 'https://example.com/hook', 'whsec_x', $1::jsonb, true)`,
		`{"event_types":["contact.updated"]}`)
	require.NoError(t, err)

	countUpdated := func(t *testing.T) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg, `
			SELECT COUNT(*) FROM webhook_deliveries
			WHERE subscription_id = 'sub-v39-contacts' AND event_type = 'contact.updated'`).Scan(&n))
		return n
	}

	// --- pre-43 state, as in TestV43TrackingConsentMigration -----------------------
	for _, statement := range []string{
		pre43TrackContactChanges(),
		pre43WebhookContactsTrigger(t),
		`ALTER TABLE contacts
			DROP COLUMN tracking_opens_consent,
			DROP COLUMN tracking_clicks_consent,
			DROP COLUMN tracking_consent_updated_at,
			DROP COLUMN tracking_consent_source`,
		`ALTER TABLE message_history DROP COLUMN opens_tracked, DROP COLUMN clicks_tracked`,
		`ALTER TABLE broadcasts DROP COLUMN tracking_mode`,
	} {
		_, err := db.ExecContext(bg, statement)
		require.NoError(t, err)
	}

	// --- v39 to v42, each committed, the path a database below 39 takes -------------
	var applied []float64
	for _, migration := range migrations.GetRegisteredMigrations() {
		version := migration.GetMajorVersion()
		if version < 39 || version >= 43 || !migration.HasWorkspaceUpdate() {
			continue
		}
		tx, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		if mErr := migration.UpdateWorkspace(bg, &config.Config{}, workspace, tx); mErr != nil {
			_ = tx.Rollback()
			t.Fatalf("v%.0f UpdateWorkspace failed: %v", version, mErr)
		}
		require.NoError(t, tx.Commit())
		applied = append(applied, version)
	}
	require.Contains(t, applied, 39.0, "v39 must be registered, or this test migrates nothing")

	before := countUpdated(t)
	_, err = db.ExecContext(bg, `UPDATE contacts SET first_name = 'Between' WHERE email = $1`, email)
	require.NoError(t, err, "contacts must stay writable between v39 and v43")
	require.Equal(t, before+1, countUpdated(t), "the change still reaches contact.updated subscribers")

	// --- v43 brings the columns and today's body -------------------------------------
	tx, err := db.BeginTx(bg, nil)
	require.NoError(t, err)
	if mErr := (&migrations.V43Migration{}).UpdateWorkspace(bg, &config.Config{}, workspace, tx); mErr != nil {
		_ = tx.Rollback()
		t.Fatalf("v43 UpdateWorkspace failed: %v", mErr)
	}
	require.NoError(t, tx.Commit())

	before = countUpdated(t)
	_, err = db.ExecContext(bg, `UPDATE contacts SET tracking_opens_consent = 'granted' WHERE email = $1`, email)
	require.NoError(t, err)
	require.Equal(t, before+1, countUpdated(t), "after v43, a consent change is a contact.updated event")
}
