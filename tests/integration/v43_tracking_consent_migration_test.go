//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pre43TrackContactChanges is track_contact_changes() as it was before 43:
// the same body without the consent columns.
func pre43TrackContactChanges() string {
	var kept []string
	for _, line := range strings.Split(schema.TrackContactChangesFunction(), "\n") {
		if !strings.Contains(line, "tracking_") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// pre43WebhookContactsTrigger is webhook_contacts_trigger() before 43: its
// "nothing changed" test ended at custom_json_5.
func pre43WebhookContactsTrigger(t *testing.T) string {
	t.Helper()
	consentTail := regexp.MustCompile(`custom_json_5 AND\s+NEW\.tracking_opens_consent[^\n]*\n\s+NEW\.tracking_clicks_consent[^\n]*\n\s+NEW\.tracking_consent_source IS NOT DISTINCT FROM OLD\.tracking_consent_source THEN`)
	current := schema.WebhookContactsTriggerFunction()
	require.Regexp(t, consentTail, current, "the consent comparison this test removes must exist")
	return consentTail.ReplaceAllString(current, "custom_json_5 THEN")
}

// TestV43TrackingConsentMigration puts a workspace back in its pre-43 state —
// no consent columns, no message flags, no broadcast tracking mode, and the old
// trigger functions — then runs v43 the way the manager does, twice, and checks
// the workspace behaves as a fresh one.
func TestV43TrackingConsentMigration(t *testing.T) {
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

	tag := uuid.New().String()[:8]
	email := fmt.Sprintf("v43-%s@example.com", tag)
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	message, err := factory.CreateMessageHistory(workspace.ID,
		testutil.WithMessageHistoryContactEmail(email),
		testutil.WithMessageTemplate(template.ID))
	require.NoError(t, err)

	// --- pre-43 state ----------------------------------------------------------------
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
	// A pre-43 contact write works, so the state is a consistent one.
	_, err = db.ExecContext(bg, `UPDATE contacts SET first_name = 'Before' WHERE email = $1`, email)
	require.NoError(t, err)

	runV43 := func(t *testing.T) {
		t.Helper()
		tx, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		if mErr := (&migrations.V43Migration{}).UpdateWorkspace(bg, &config.Config{}, workspace, tx); mErr != nil {
			_ = tx.Rollback()
			t.Fatalf("v43 UpdateWorkspace failed: %v", mErr)
		}
		require.NoError(t, tx.Commit())
	}
	runV43(t)
	runV43(t) // idempotent

	t.Run("the columns exist, nullable and without default", func(t *testing.T) {
		rows, err := db.QueryContext(bg, `
			SELECT table_name || '.' || column_name, is_nullable, column_default IS NULL
			FROM information_schema.columns
			WHERE (table_name = 'contacts' AND column_name LIKE 'tracking_%')
			   OR (table_name = 'message_history' AND column_name IN ('opens_tracked', 'clicks_tracked'))
			   OR (table_name = 'broadcasts' AND column_name = 'tracking_mode')`)
		require.NoError(t, err)
		defer rows.Close()
		got := map[string]bool{}
		for rows.Next() {
			var name, nullable string
			var noDefault bool
			require.NoError(t, rows.Scan(&name, &nullable, &noDefault))
			assert.Equal(t, "YES", nullable, name)
			assert.True(t, noDefault, name)
			got[name] = true
		}
		require.NoError(t, rows.Err())
		assert.ElementsMatch(t, []string{
			"contacts.tracking_opens_consent", "contacts.tracking_clicks_consent",
			"contacts.tracking_consent_updated_at", "contacts.tracking_consent_source",
			"message_history.opens_tracked", "message_history.clicks_tracked",
			"broadcasts.tracking_mode",
		}, keysOfBoolMap(got))
	})

	t.Run("both trigger functions are the shared ones", func(t *testing.T) {
		for _, fn := range []string{"track_contact_changes", "webhook_contacts_trigger"} {
			var body string
			require.NoError(t, db.QueryRowContext(bg, `SELECT prosrc FROM pg_proc WHERE proname = $1`, fn).Scan(&body))
			assert.Contains(t, body, "tracking_opens_consent", fn)
			assert.Contains(t, body, "tracking_clicks_consent", fn)
			assert.Contains(t, body, "tracking_consent_source", fn)
			assert.NotContains(t, body, "tracking_consent_updated_at", fn)
		}
	})

	t.Run("a message sent before 43 reads as tracked", func(t *testing.T) {
		var opens, clicks sql.NullBool
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT opens_tracked, clicks_tracked FROM message_history WHERE id = $1`, message.ID).Scan(&opens, &clicks))
		assert.False(t, opens.Valid)
		assert.False(t, clicks.Valid)

		var tracked int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM message_history WHERE id = $1 AND opens_tracked IS NOT FALSE AND clicks_tracked IS NOT FALSE`,
			message.ID).Scan(&tracked))
		assert.Equal(t, 1, tracked)
	})

	t.Run("a consent change after 43 is logged on the timeline", func(t *testing.T) {
		_, err := db.ExecContext(bg,
			`UPDATE contacts SET tracking_clicks_consent = 'denied', tracking_consent_source = 'api', tracking_consent_updated_at = NOW() WHERE email = $1`,
			email)
		require.NoError(t, err)

		var n int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_timeline WHERE email = $1 AND kind = 'contact.updated'
			 AND changes ? 'tracking_clicks_consent' AND changes ? 'tracking_consent_source'`, email).Scan(&n))
		assert.Equal(t, 1, n)
	})

	t.Run("the contact reads back through the repository", func(t *testing.T) {
		contact, err := suite.ServerManager.GetApp().GetContactRepository().GetContactByEmail(bg, workspace.ID, email)
		require.NoError(t, err)
		require.NotNil(t, contact.TrackingClicksConsent)
		assert.Equal(t, domain.TrackingConsentDenied, contact.TrackingClicksConsent.String)
		assert.Nil(t, contact.TrackingOpensConsent)
	})
}

func keysOfBoolMap(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
