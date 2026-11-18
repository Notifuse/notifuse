//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestDemoSeedOnAProductionInstance drives /api/demo.seed the way the preview's
// scheduler does: an instance running as production, its background workers on,
// with a workspace of its own that the seed must leave alone.
//
// What only a real stack shows: the route is served on production at all, the
// seed's workspace database is dropped and created again under the same name
// while workers run (the second seed), and a "demo" workspace the seed did not
// create is refused with its data intact.
func TestDemoSeedOnAProductionInstance(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.Environment = "production"
		return appFactory(cfg)
	})
	defer func() { suite.Cleanup() }()

	// The instance's own workspace, with a contact. Created before the
	// restart below, which leaves the data factory's repositories behind.
	own, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	_, err = suite.DataFactory.CreateContact(own.ID, func(c *domain.Contact) { c.Email = "keep-me@example.com" })
	require.NoError(t, err)
	ownDB, err := suite.DBManager.GetWorkspaceDB(own.ID)
	require.NoError(t, err)

	// The application's server has no write timeout; a seed runs for tens of seconds.
	suite.ServerManager.SetWriteTimeout(10 * time.Minute)
	require.NoError(t, suite.ServerManager.Restart())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	systemDB := suite.DBManager.GetDB()
	hmac := domain.ComputeEmailHMAC("test@example.com", testutil.TestSecretKey)

	seed := func(t *testing.T, signature string) (int, map[string]string) {
		t.Helper()
		client := &http.Client{Timeout: 10 * time.Minute}
		resp, err := client.Get(suite.ServerManager.GetURL() + "/api/demo.seed?hmac=" + signature)
		require.NoError(t, err)
		defer resp.Body.Close()
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
	count := func(t *testing.T, db *sql.DB, query string, args ...interface{}) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&n))
		return n
	}

	t.Run("an unsigned request is refused", func(t *testing.T) {
		code, _ := seed(t, "not-the-hmac")
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	// Unknown paths fall through to the root handler, so the status code alone says
	// nothing: what matters is that a correctly signed reset deletes nothing.
	t.Run("demo.reset is not served on production", func(t *testing.T) {
		resp, err := http.Get(suite.ServerManager.GetURL() + "/api/demo.reset?hmac=" + hmac)
		require.NoError(t, err)
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		assert.NotEqual(t, "reset", body["status"])
		assert.Equal(t, 1, count(t, systemDB, `SELECT COUNT(*) FROM workspaces WHERE id = $1`, own.ID),
			"a signed reset on production must not delete the instance's workspaces")
	})

	assertSeeded := func(t *testing.T) {
		t.Helper()
		demoDB, err := suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM contacts`), "contacts")
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM templates`), "templates")
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM broadcasts`), "broadcasts")
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM automations`), "automations")
		assert.Zero(t, count(t, demoDB, `SELECT COUNT(*) FROM automations WHERE status = 'live'`),
			"off ENVIRONMENT=demo the seeded automations must stay inert: this instance's scheduler runs")
		var marker string
		require.NoError(t, systemDB.QueryRowContext(ctx,
			`SELECT value FROM settings WHERE key = 'demo_fixtures_workspace_created_at'`).Scan(&marker),
			"the seed marks the workspace it created")
		var createdAt time.Time
		require.NoError(t, systemDB.QueryRowContext(ctx,
			`SELECT created_at FROM workspaces WHERE id = 'demo'`).Scan(&createdAt))
		markedAt, err := time.Parse(time.RFC3339Nano, marker)
		require.NoError(t, err)
		assert.True(t, markedAt.Equal(createdAt), "the marker %s is the workspace's created_at %s", marker, createdAt)

		assertDemoTemplates(t, demoDB)
		assertDemoMessageOutcomes(t, demoDB)
		assertDemoBroadcastHistory(t, demoDB)

		// The instance's own workspace is untouched.
		assert.Equal(t, 1, count(t, systemDB, `SELECT COUNT(*) FROM workspaces WHERE id = $1`, own.ID))
		assert.Equal(t, 1, count(t, ownDB, `SELECT COUNT(*) FROM contacts WHERE email = 'keep-me@example.com'`))
	}

	t.Run("a signed seed creates the fixtures workspace and nothing else changes", func(t *testing.T) {
		code, body := seed(t, hmac)
		require.Equal(t, http.StatusOK, code, "body: %v", body)
		assert.Equal(t, "seeded", body["status"])
		assertSeeded(t)
	})

	t.Run("a second seed replaces the fixtures workspace", func(t *testing.T) {
		demoDB, err := suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		_, err = demoDB.ExecContext(ctx,
			`INSERT INTO contacts (email, created_at, updated_at) VALUES ('stale@example.com', NOW(), NOW())`)
		require.NoError(t, err)

		// A fresh handler: the five-minute debounce lives in the handler.
		require.NoError(t, suite.ServerManager.Restart())
		require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

		code, body := seed(t, hmac)
		require.Equal(t, http.StatusOK, code, "body: %v", body)
		assertSeeded(t)

		demoDB, err = suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		assert.Zero(t, count(t, demoDB, `SELECT COUNT(*) FROM contacts WHERE email = 'stale@example.com'`),
			"the previous fixtures were dropped, not added to")
	})

	t.Run("a demo workspace the seed did not create is refused", func(t *testing.T) {
		_, err := systemDB.ExecContext(ctx, `DELETE FROM settings WHERE key = 'demo_fixtures_workspace_created_at'`)
		require.NoError(t, err)
		require.NoError(t, suite.ServerManager.Restart())

		code, body := seed(t, hmac)
		assert.Equal(t, http.StatusConflict, code, "body: %v", body)
		assert.Contains(t, body["error"], "nothing was deleted")

		demoDB, err := suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM contacts`), "its data is intact")
	})
}

// The demo's templates built from the template gallery, as the seed stored them: their images
// Unsplash photos, every language there, and every template a broadcast or an automation sends
// present.
func assertDemoTemplates(t *testing.T, demoDB *sql.DB) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{
		"newsletter-weekly", "newsletter-weekly-v2", "welcome-email", "password-reset",
		"cart-recovery-a", "cart-recovery-b", "winback-offer", "order-thank-you",
	} {
		var email, translations, testData string
		require.NoError(t, demoDB.QueryRowContext(ctx,
			`SELECT email::text, translations::text, test_data::text FROM templates WHERE id = $1 AND deleted_at IS NULL`, id).
			Scan(&email, &translations, &testData), "template %s", id)
		all := email + translations + testData
		assert.NotContains(t, all, "notifuse_gallery_base_url", "template %s keeps the image placeholder", id)
		assert.Contains(t, email, demoWorkspaceLogoURL, "template %s shows the workspace's logo", id)
		assert.Contains(t, email, `"subject_preview"`, "template %s has a preview text", id)
		for _, language := range []string{`"fr"`, `"es"`} {
			assert.Contains(t, translations, language, "template %s is not translated in %s", id, language)
		}
		// Photos, not the gallery's images on the instance: the emails do not depend on the console.
		assert.NotContains(t, all, "/console/gallery/", "template %s takes its images from the instance", id)
		if id != "password-reset" && id != "winback-offer" {
			assert.Contains(t, all, "https://images.unsplash.com/photo-", "template %s shows no photo", id)
		}
	}

	// Every variation of a broadcast and every email node of an automation names a template.
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM broadcasts b, jsonb_array_elements(b.test_settings->'variations') v
		WHERE NOT EXISTS (SELECT 1 FROM templates t WHERE t.id = v->>'template_id' AND t.deleted_at IS NULL)`))
	assert.Positive(t, scanCount(t, demoDB, `SELECT COUNT(*) FROM automations a, jsonb_array_elements(a.nodes) n WHERE n->>'type' = 'email'`))
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM automations a, jsonb_array_elements(a.nodes) n
		WHERE n->>'type' = 'email'
		AND NOT EXISTS (SELECT 1 FROM templates t WHERE t.id = n->'config'->>'template_id' AND t.deleted_at IS NULL)`))
}

// The seeded history reads as SES reported it: nothing dated after the seed, a hard bounce or a
// complaint sets the contact's status as ProcessWebhook does, and no newsletter reaches it after.
func assertDemoMessageOutcomes(t *testing.T, demoDB *sql.DB) {
	t.Helper()
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history
		WHERE GREATEST(sent_at, delivered_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at) > NOW()`),
		"a seeded event is dated after the seed")
	assert.Positive(t, scanCount(t, demoDB, `SELECT COUNT(*) FROM message_history WHERE bounced_at IS NOT NULL`), "no bounce seeded")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history m
		JOIN contact_lists cl ON cl.email = m.contact_email AND cl.list_id = 'newsletter'
		WHERE m.bounced_at IS NOT NULL AND cl.status NOT IN ('bounced', 'complained')`),
		"a hard-bounced contact is still subscribed to the newsletter")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history m
		JOIN contact_lists cl ON cl.email = m.contact_email AND cl.list_id = m.list_id
		WHERE m.complained_at IS NOT NULL AND cl.status <> 'complained'`),
		"a contact who complained about a newsletter is still subscribed to it")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history b
		JOIN message_history later ON later.contact_email = b.contact_email AND later.sent_at > b.sent_at
		WHERE b.bounced_at IS NOT NULL AND later.automation_id IS NULL`),
		"an email was seeded to an address after it hard-bounced")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history c
		JOIN message_history later ON later.contact_email = c.contact_email AND later.sent_at > c.sent_at
		WHERE c.complained_at IS NOT NULL AND later.broadcast_id IS NOT NULL`),
		"a newsletter was seeded to a contact after their spam complaint")
}

// The seeded contacts and broadcasts read as a real send left them: addresses only at the
// domains reserved for examples, each broadcast's emails dated from its own started_at over
// its send window, the A/B test sent both variations in its sample and the winner to everyone
// else, with counters that are its emails, and every event on the contact timeline dated at
// the event rather than at the seed.
func assertDemoBroadcastHistory(t *testing.T, demoDB *sql.DB) {
	t.Helper()
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM contacts
		WHERE email !~ '@example\.(com|org|net)$'`),
		"a contact's address is not at a reserved example domain")

	assert.Positive(t, scanCount(t, demoDB, `SELECT COUNT(*) FROM message_history WHERE broadcast_id IS NOT NULL`))
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history m JOIN broadcasts b ON b.id = m.broadcast_id
		WHERE m.sent_at < b.started_at OR m.sent_at > b.completed_at
			OR m.created_at < b.started_at OR m.created_at > b.completed_at`),
		"a broadcast's email is dated outside its send window")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history
		WHERE LEAST(delivered_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at) < sent_at`),
		"an event of an email is dated before the email was sent")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM broadcasts b
		WHERE b.enqueued_count <> (SELECT COUNT(*) FROM message_history m WHERE m.broadcast_id = b.id)`),
		"a broadcast's enqueued_count is not its emails")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history m JOIN broadcasts b ON b.id = m.broadcast_id
		WHERE NOT EXISTS (SELECT 1 FROM jsonb_array_elements(b.test_settings->'variations') v WHERE v->>'template_id' = m.template_id)`),
		"a broadcast's email was sent a template that is none of its variations")

	// The A/B test.
	assert.Equal(t, 1, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM broadcasts
		WHERE (test_settings->>'enabled')::boolean AND winning_template IS NOT NULL
			AND test_sent_at IS NOT NULL AND winner_sent_at > test_sent_at`))
	assert.Equal(t, 2, scanCount(t, demoDB, `
		SELECT COUNT(DISTINCT m.template_id) FROM message_history m JOIN broadcasts b ON b.id = m.broadcast_id
		WHERE (b.test_settings->>'enabled')::boolean AND m.sent_at >= b.test_sent_at AND m.sent_at < b.winner_sent_at`),
		"the A/B test's sample was not sent both variations")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM message_history m JOIN broadcasts b ON b.id = m.broadcast_id
		WHERE (b.test_settings->>'enabled')::boolean AND m.sent_at >= b.winner_sent_at AND m.template_id <> b.winning_template`),
		"an email of the A/B test's winner phase was sent another template")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM broadcasts b
		WHERE (b.test_settings->>'enabled')::boolean AND (
			b.test_phase_recipient_count <> (SELECT COUNT(*) FROM message_history m WHERE m.broadcast_id = b.id AND m.sent_at < b.winner_sent_at)
			OR b.winner_phase_recipient_count <> (SELECT COUNT(*) FROM message_history m WHERE m.broadcast_id = b.id AND m.sent_at >= b.winner_sent_at)
			OR b.test_phase_recipient_count = 0 OR b.winner_phase_recipient_count = 0)`),
		"the A/B test's phase counts are not its emails")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM broadcasts
		WHERE NOT (test_settings->>'enabled')::boolean AND (test_phase_recipient_count <> 0 OR winner_phase_recipient_count <> 0)`),
		"a broadcast with one template has phase counts")

	// The contact timeline, at the moment of each event. db_created_at is the moment the seed
	// wrote the entry, and every event of the history happened before the seed began, so an
	// entry dated at its event lies between the email's send and, well before, its own writing;
	// one left at the moment the seed recorded the event has the two equal.
	assert.Positive(t, scanCount(t, demoDB, `SELECT COUNT(*) FROM contact_timeline WHERE kind = 'email.opened'`))
	assert.Positive(t, scanCount(t, demoDB, `SELECT COUNT(*) FROM contact_timeline WHERE kind = 'email.updated'`))
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM contact_timeline t JOIN message_history m ON m.id = t.entity_id
		WHERE t.entity_type = 'message_history'
			AND (t.created_at < m.sent_at OR t.created_at > t.db_created_at - INTERVAL '1 second')`),
		"an email's timeline entry is dated before the email or at the moment of the seed")

	// The broadcast rows, written the day before they went out and last updated as they
	// completed, rather than at the seed (the repository stamps updated_at as it writes).
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM broadcasts
		WHERE NOT (created_at < started_at) OR updated_at IS DISTINCT FROM completed_at
			OR completed_at > NOW() - INTERVAL '1 day'`),
		"a broadcast row is dated at the seed rather than at its send")
	assert.Zero(t, scanCount(t, demoDB, `
		SELECT COUNT(*) FROM inbound_webhook_events e JOIN message_history m ON m.id = e.message_id
		WHERE e.created_at < m.sent_at OR e.created_at > NOW()`),
		"a provider event is dated before its email or after the seed")
}

func scanCount(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(), query).Scan(&n), query)
	return n
}

// The demo workspace's logo (internal/service/demo_service.go).
const demoWorkspaceLogoURL = "https://www.apple.com/ac/structured-data/images/knowledge_graph_logo.png"
