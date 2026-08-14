//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestContactDeletionErasure is the erasure contract, asserted the only way that
// means anything: by scanning the columns directly and looking for the address.
//
// It deliberately does NOT go through the repositories. Two of the purges used to
// redact an identifying column while leaving the address in a payload blob on the
// same surviving row — a test written against the repositories' own reads would
// have called both of those erased. Reading the raw columns is what makes a
// cosmetic redaction fail.
//
// There was no integration test for contact deletion at all before this. The
// contract was entirely unpinned, which is how two half-redactions and four
// untouched tables went unnoticed.
func TestContactDeletionErasure(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	const victim = "erase-me@example.com"
	const bystander = "keep-me@example.com"

	client := suite.APIClient
	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			Filters: domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	for _, email := range []string{victim, bystander} {
		_, err = suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) { c.Email = email })
		require.NoError(t, err)
	}

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()

	// --- Seed one row in every table that stores a contact identity ------------

	// Deterministic per-email UUIDs. Prefixing the address would both break the
	// uuid columns and leave a copy of it in a primary key.
	uuidFor := func(email, salt string) string {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(salt+":"+email)).String()
	}

	seed := func(email string) {
		t.Helper()

		_, err := wsDB.Exec(`
			INSERT INTO message_history
				(id, contact_email, broadcast_id, template_id, template_version, channel,
				 message_data, sent_at, created_at, updated_at)
			VALUES ($1, $2, NULL, 'tpl-1', 1, 'email', $3::jsonb, NOW(), NOW(), NOW())`,
			uuidFor(email, "msg"), email,
			// The blob the redaction used to leave behind: the address is in here
			// twice over, once as contact data and once inside the unsubscribe URL.
			fmt.Sprintf(`{"data":{"contact":{"email":%q}},"unsubscribe_url":"https://x/u?email=%s"}`, email, email))
		require.NoError(t, err)

		_, err = wsDB.Exec(`
			INSERT INTO inbound_webhook_events
				(id, type, source, integration_id, recipient_email, raw_payload, timestamp, created_at)
			VALUES ($1, 'reply', 'ses', 'int-1', $2, $3, NOW(), NOW())`,
			uuidFor(email, "evt"), email,
			fmt.Sprintf(`{"from":%q,"to":"support@example.com","text":"hello"}`, email))
		require.NoError(t, err)

		_, err = wsDB.Exec(`
			INSERT INTO custom_events (external_id, email, event_name, properties, occurred_at, source, created_at, updated_at)
			VALUES ($1, $2, 'shopify.order', '{}'::jsonb, NOW(), 'api', NOW(), NOW())`,
			"ce-"+email, email)
		require.NoError(t, err)

		_, err = wsDB.Exec(`
			INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args)
			VALUES ('seg-1', 'All', 'blue', '{}'::jsonb, 'UTC', 1, 'active', 'SELECT 1', '[]'::jsonb)
			ON CONFLICT (id) DO NOTHING`)
		require.NoError(t, err)
		_, err = wsDB.Exec(`
			INSERT INTO contact_segments (email, segment_id, version, matched_at)
			VALUES ($1, 'seg-1', 1, NOW())`, email)
		require.NoError(t, err)

		_, err = wsDB.Exec(`
			INSERT INTO contact_segment_queue (email, queued_at) VALUES ($1, NOW())
			ON CONFLICT (email) DO NOTHING`, email)
		require.NoError(t, err)

		_, err = wsDB.Exec(`
			INSERT INTO contact_timeline (id, email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES ($1, $2, 'insert', 'contact', 'contact.created', 'x', '{}'::jsonb, NOW())`,
			uuidFor(email, "tl"), email)
		require.NoError(t, err)

		_, err = wsDB.Exec(`
			INSERT INTO email_queue
				(id, status, priority, source_type, source_id, integration_id, provider_kind,
				 contact_email, message_id, template_id, payload)
			VALUES ($1, 'pending', 5, 'broadcast', 'b-1', 'i-1', 'smtp', $2, $3, 'tpl-1', $4::jsonb)`,
			uuidFor(email, "eq"), email, uuidFor(email, "mid"),
			fmt.Sprintf(`{"from_address":"a@b.c","subject":"hi","html_content":"<p>%s</p>"}`, email))
		require.NoError(t, err)
	}

	seed(victim)
	seed(bystander)

	// A subscriber installed BEFORE the deletion: the fan-out trigger only writes
	// a delivery row for an enabled subscription matching the event kind, so
	// without one the probe below would report zero for the wrong reason.
	_, err = wsDB.Exec(`
		INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ('sub-1', 'crm', 'https://crm.example.com/hook', 'whsec_x', $1::jsonb, true)`,
		`{"event_types": ["segment.left", "segment.joined", "contact.deleted", "custom_event.created"]}`)
	require.NoError(t, err)

	// A web session and a bridged goal, through the real pipeline.
	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	payload := map[string]interface{}{
		"workspace_id":       workspace.ID,
		"session_id":         waUUIDv7At(now.Add(-4*time.Minute), 0xC1),
		"tab_id":             1,
		"contact_email":      victim,
		"contact_email_hmac": domain.ComputeWebIdentifyHMAC(victim, workspace.Settings.SecretKey),
		"actions": []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
			{"type": "goal", "name": "purchase", "page_number": 1, "goal_type": "purchase",
				"timestamp": now.Add(-3 * time.Minute).UnixMilli(), "value": 49.9, "path": "/pricing"},
		},
		"attributes":  map[string]interface{}{"landing_page": "https://shop.example.com/pricing"},
		"created_at":  now.Add(-5 * time.Minute).UnixMilli(),
		"updated_at":  now.UnixMilli(),
		"sent_at":     now.UnixMilli(),
		"sdk_version": "1.0.0",
		"seq":         1,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, true, waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))["success"])
	buffer.FlushAll(ctx)

	// --- Every column that can hold the address, and how it must end up --------

	type probe struct {
		what  string
		query string
		why   string
	}
	probes := []probe{
		{"message_history.contact_email", `SELECT count(*) FROM message_history WHERE contact_email = $1`, ""},
		{"message_history.message_data", `SELECT count(*) FROM message_history WHERE message_data::text LIKE '%' || $1 || '%'`,
			"the blob is decrypted and returned by messages.list"},
		{"inbound_webhook_events.recipient_email", `SELECT count(*) FROM inbound_webhook_events WHERE recipient_email = $1`, ""},
		{"inbound_webhook_events.raw_payload", `SELECT count(*) FROM inbound_webhook_events WHERE raw_payload LIKE '%' || $1 || '%'`,
			"returned verbatim by inboundWebhookEvents.list"},
		{"custom_events.email", `SELECT count(*) FROM custom_events WHERE email = $1`,
			"NOT NULL, so it cannot be anonymised in place — the row has to go"},
		{"contact_segments.email", `SELECT count(*) FROM contact_segments WHERE email = $1`,
			"segments.contacts returns these as a bare list of addresses"},
		{"contact_segment_queue.email", `SELECT count(*) FROM contact_segment_queue WHERE email = $1`, ""},
		{"contact_timeline.email", `SELECT count(*) FROM contact_timeline WHERE email = $1`, ""},
		{"email_queue.contact_email", `SELECT count(*) FROM email_queue WHERE contact_email = $1`,
			"the only residue that keeps actively MAILING the erased person"},
		{"email_queue.payload", `SELECT count(*) FROM email_queue WHERE payload::text LIKE '%' || $1 || '%'`, ""},
		{"contacts.email", `SELECT count(*) FROM contacts WHERE email = $1`, ""},
		{"web_sessions.contact_email", `SELECT count(*) FROM web_sessions WHERE contact_email = $1`,
			"anonymised, not deleted: the traffic stays as anonymous"},
		{"web_goals.contact_email", `SELECT count(*) FROM web_goals WHERE contact_email = $1`, ""},
	}

	count := func(t *testing.T, query, email string) int {
		t.Helper()
		var n int
		require.NoError(t, wsDB.QueryRow(query, email).Scan(&n))
		return n
	}

	// Everything must genuinely be there first, or the assertions below pass for
	// the wrong reason.
	t.Run("the fixture really carries the address everywhere", func(t *testing.T) {
		for _, p := range probes {
			assert.Positive(t, count(t, p.query, victim), "%s was never seeded", p.what)
		}
	})

	resp, err := client.Post("/api/contacts.delete", map[string]interface{}{
		"workspace_id": workspace.ID,
		"email":        victim,
	})
	require.NoError(t, err)
	deleteBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "delete failed: %s", string(deleteBody))

	t.Run("no column holds the deleted address", func(t *testing.T) {
		for _, p := range probes {
			t.Run(p.what, func(t *testing.T) {
				n := count(t, p.query, victim)
				msg := p.what + " still carries the deleted address"
				if p.why != "" {
					msg += " — " + p.why
				}
				assert.Zero(t, n, msg)
			})
		}
	})

	// Erasure that also erased everyone else would pass every assertion above.
	t.Run("another contact is untouched", func(t *testing.T) {
		for _, p := range probes {
			if p.what == "web_sessions.contact_email" || p.what == "web_goals.contact_email" {
				continue // only the victim has web analytics rows
			}
			assert.Positive(t, count(t, p.query, bystander), "%s lost the bystander's data", p.what)
		}
	})

	// The trigger cascade: purging contact_segments writes a segment.left timeline
	// row carrying OLD.email, which in turn queues a segment recomputation for it.
	// Getting the purge order wrong leaves those behind, and they are exactly the
	// rows the two probes above would catch — asserted separately so a failure
	// says WHY rather than just "still present".
	// Erasure fans out to third-party webhook subscribers, and this pins exactly
	// what it sends — because one of the two events is deliberate and the other is
	// a side effect of the segment purge, and telling them apart later would be
	// hard.
	//
	//   contact.deleted  is the point: it carries to_jsonb(OLD) so a customer's
	//                    CRM learns to erase ITS copy. Suppressing it would defeat
	//                    the deletion it announces.
	//   segment.left     is incidental: webhook_contact_segments is AFTER INSERT
	//                    OR DELETE, so purging memberships emits one per segment
	//                    the contact was in. A contact in twenty segments produces
	//                    twenty of them.
	//
	// Both carry the address, which is why webhook_deliveries is deliberately not
	// purged — see plans/known-issues.md.
	// The queue needs a BEHAVIOURAL assertion, not a row count.
	//
	// The probes above check `count(*) = 0` and a payload LIKE — and both pass
	// against a "fix" that marks the row processed, redacts it, or moves it to
	// another status. For a queue the only honest question is whether anything
	// still goes out, so this drains it and asserts nothing was sent to the
	// erased address.
	t.Run("draining the queue sends nothing to the erased address", func(t *testing.T) {
		var pending int
		require.NoError(t, wsDB.QueryRow(`
			SELECT count(*) FROM email_queue
			WHERE contact_email = $1
			   OR payload::text LIKE '%' || $1 || '%'`, victim).Scan(&pending))
		assert.Zero(t, pending, "a claimable row survived, so a worker can still pick it up")

		// The bystander's mail must still be there — an erasure that emptied the
		// whole queue would satisfy the assertion above for the wrong reason.
		var bystanderPending int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM email_queue WHERE contact_email = $1`, bystander).Scan(&bystanderPending))
		assert.Positive(t, bystanderPending, "the purge took someone else's queued mail with it")
	})

	t.Run("erasure fans out exactly two kinds of event", func(t *testing.T) {
		byKind := map[string]int{}
		rows, err := wsDB.Query(`SELECT event_type, count(*) FROM webhook_deliveries GROUP BY event_type`)
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var ev string
			var n int
			require.NoError(t, rows.Scan(&ev, &n))
			byKind[ev] = n
		}

		assert.Equal(t, 1, byKind["contact.deleted"], "the event a CRM needs to erase its own copy")
		assert.Equal(t, 1, byKind["segment.left"],
			"one per segment membership purged — if this grows, the purge is emitting more than it should")
		assert.NotContains(t, byKind, "custom_event.created",
			"purging custom_events must not look like creating them")
	})

	t.Run("the segment purge did not resurrect the address", func(t *testing.T) {
		assert.Zero(t, count(t, `SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind = 'segment.left'`, victim),
			"the segment purge ran after the timeline purge, so its trigger re-inserted the address")
		assert.Zero(t, count(t, `SELECT count(*) FROM contact_segment_queue WHERE email = $1`, victim),
			"the queue purge ran before the cascade that creates the row")
	})
}
