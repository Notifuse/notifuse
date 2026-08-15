//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestMessageHistoryRedactionFiresNoTriggerFanout pins the blast radius of the
// erasure UPDATE on message_history.
//
// Three AFTER UPDATE triggers sit on that table — track_message_history_changes
// (writes contact_timeline), update_contact_lists_on_status_change (rewrites
// subscription status) and webhook_message_history_trigger (fans out to customer
// webhooks). TestContactDeletionErasure cannot see what they do here: it probes
// for the victim's address, and the redaction renames the row to DELETED_EMAIL
// first, so anything a trigger writes afterwards is filed under a name no probe
// looks for.
func TestMessageHistoryRedactionFiresNoTriggerFanout(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	const victim = "redact-me@example.com"
	const bystander = "spare-me@example.com"

	client := suite.APIClient
	workspace, err := suite.DataFactory.CreateWorkspace()
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

	// The subscriber goes in BEFORE the sends, so the email.sent deliveries the
	// seeding produces are the control: they prove this subscription really does
	// match email.* kinds, which is what makes "no new email.* delivery" mean
	// something rather than "nothing was ever subscribed".
	_, err = wsDB.Exec(`
		INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ('sub-1', 'crm', 'https://crm.example.com/hook', 'whsec_x', $1::jsonb, true)`,
		`{"event_types": ["email.sent","email.delivered","email.opened","email.clicked",
		  "email.bounced","email.complained","email.unsubscribed","contact.deleted","segment.left"]}`)
	require.NoError(t, err)

	_, err = wsDB.Exec(`INSERT INTO lists (id, name, is_double_optin, is_public)
		VALUES ('list1', 'Newsletter', false, true)`)
	require.NoError(t, err)

	// Every engagement timestamp is already set. That is the worst case for the
	// two triggers that gate on "OLD IS NULL AND NEW IS NOT NULL": a redaction
	// that touched the timestamps, or a trigger that gated on NEW alone, fires
	// every branch at once.
	seed := func(email string) string {
		t.Helper()
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("msg:"+email)).String()
		_, err := wsDB.Exec(`
			INSERT INTO message_history
				(id, contact_email, broadcast_id, list_id, template_id, template_version, channel,
				 status_info, message_data, clicked_links, sent_at, delivered_at, opened_at,
				 clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at)
			VALUES ($1, $2, 'b-1', 'list1', 'tpl-1', 1, 'email', 'ok', $3::jsonb, $4::jsonb,
			        NOW(), NOW(), NOW(), NOW(), NOW(), NOW(), NOW(), NOW(), NOW())`,
			id, email,
			fmt.Sprintf(`{"data":{"contact":{"email":%q}},"unsubscribe_url":"https://x/u?email=%s"}`, email, email),
			fmt.Sprintf(`[{"url":"https://x/o?email=%s","count":1}]`, email))
		require.NoError(t, err)

		_, err = wsDB.Exec(`INSERT INTO contact_lists (email, list_id, status) VALUES ($1, 'list1', 'active')`, email)
		require.NoError(t, err)
		return id
	}
	victimMsgID := seed(victim)
	seed(bystander)

	count := func(t *testing.T, query string, args ...interface{}) int {
		t.Helper()
		var n int
		require.NoError(t, wsDB.QueryRow(query, args...).Scan(&n))
		return n
	}

	// --- Before ---------------------------------------------------------------

	require.Equal(t, 2, count(t, `SELECT count(*) FROM webhook_deliveries WHERE event_type = 'email.sent'`),
		"the control failed: the seeded sends produced no delivery, so the subscription does not match email.* at all")

	type stamps struct {
		sent, delivered, opened, clicked, bounced, complained, unsubscribed string
		statusInfo                                                          string
	}
	readStamps := func(id string) stamps {
		var s stamps
		require.NoError(t, wsDB.QueryRow(`
			SELECT sent_at::text, delivered_at::text, opened_at::text, clicked_at::text,
			       bounced_at::text, complained_at::text, unsubscribed_at::text, status_info
			FROM message_history WHERE id = $1`, id).Scan(
			&s.sent, &s.delivered, &s.opened, &s.clicked, &s.bounced, &s.complained, &s.unsubscribed, &s.statusInfo))
		return s
	}
	before := readStamps(victimMsgID)

	timelineBefore := count(t, `SELECT count(*) FROM contact_timeline`)
	deliveriesBefore := count(t, `SELECT count(*) FROM webhook_deliveries`)
	bystanderTimelineBefore := count(t, `SELECT count(*) FROM contact_timeline WHERE email = $1`, bystander)

	// --- Delete ---------------------------------------------------------------

	resp, err := client.Post("/api/contacts.delete", map[string]interface{}{
		"workspace_id": workspace.ID,
		"email":        victim,
	})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "delete failed: %s", string(body))

	// --- After ----------------------------------------------------------------

	t.Run("the row is redacted in place", func(t *testing.T) {
		assert.Equal(t, 1, count(t,
			`SELECT count(*) FROM message_history WHERE id = $1 AND contact_email = 'DELETED_EMAIL'
			   AND message_data = '{}'::jsonb AND clicked_links IS NULL`, victimMsgID),
			"the erasure did not land as an in-place redaction")
	})

	t.Run("no engagement timestamp moved", func(t *testing.T) {
		assert.Equal(t, before, readStamps(victimMsgID),
			"broadcast stats sum these columns — moving one rewrites the broadcast's history")
	})

	t.Run("broadcast stats are unchanged", func(t *testing.T) {
		assert.Equal(t, 2, count(t, `SELECT count(*) FROM message_history WHERE broadcast_id = 'b-1'`),
			"the redacted send left the broadcast")
		assert.Equal(t, 2, count(t, `
			SELECT SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END)::int
			FROM message_history WHERE broadcast_id = 'b-1'`),
			"the broadcast's open count changed")
	})

	t.Run("the timeline trigger wrote nothing", func(t *testing.T) {
		assert.Zero(t, count(t, `SELECT count(*) FROM contact_timeline WHERE email = 'DELETED_EMAIL'`),
			"the redaction UPDATE fired track_message_history_changes, filing rows under the redacted name")
		assert.Zero(t, count(t, `SELECT count(*) FROM contact_timeline WHERE kind LIKE '%.updated'`),
			"the redaction UPDATE looked like a status change to the timeline trigger")
		assert.Equal(t, bystanderTimelineBefore, count(t, `SELECT count(*) FROM contact_timeline WHERE email = $1`, bystander),
			"the erasure moved another contact's timeline")
		assert.LessOrEqual(t, count(t, `SELECT count(*) FROM contact_timeline`), timelineBefore,
			"the erasure grew the timeline instead of shrinking it")
	})

	t.Run("the webhook trigger fanned out nothing", func(t *testing.T) {
		assert.Equal(t, 2, count(t, `SELECT count(*) FROM webhook_deliveries WHERE event_type = 'email.sent'`),
			"the redaction UPDATE re-announced the send")
		assert.Zero(t, count(t, `SELECT count(*) FROM webhook_deliveries WHERE event_type LIKE 'email.%' AND event_type != 'email.sent'`),
			"the redaction UPDATE announced an engagement event that never happened — and its payload carries the address")
		assert.Equal(t, deliveriesBefore+1, count(t, `SELECT count(*) FROM webhook_deliveries`),
			"contact.deleted is the only event the erasure should add")
	})

	t.Run("the subscription-status trigger stayed put", func(t *testing.T) {
		var status string
		require.NoError(t, wsDB.QueryRow(
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = 'list1'`, bystander).Scan(&status))
		assert.Equal(t, "active", status,
			"the redaction UPDATE fired update_contact_lists_on_status_change and re-graded a subscription")
	})

	t.Run("messages.list still reads the redacted row", func(t *testing.T) {
		// '{}' has no "_encrypted" key, so decryptMessageData has to treat it as
		// the legacy plaintext shape and pass it through. Erroring there would
		// take the whole page down, not just the redacted row.
		resp, err := client.Get("/api/messages.list", map[string]string{"workspace_id": workspace.ID, "limit": "50"})
		require.NoError(t, err)
		listBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode, "messages.list failed on a redacted row: %s", string(listBody))

		var payload struct {
			Messages []struct {
				ID           string `json:"id"`
				ContactEmail string `json:"contact_email"`
				MessageData  struct {
					Data     map[string]interface{} `json:"data"`
					Metadata map[string]interface{} `json:"metadata"`
				} `json:"message_data"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(listBody, &payload))

		var found bool
		for _, m := range payload.Messages {
			if m.ID != victimMsgID {
				continue
			}
			found = true
			assert.Equal(t, "DELETED_EMAIL", m.ContactEmail)
			assert.Empty(t, m.MessageData.Data, "the blob came back with content")
			assert.NotContains(t, string(listBody), victim, "messages.list still serves the erased address")
		}
		assert.True(t, found, "the redacted row vanished from messages.list")
	})

	t.Run("another contact's blob is untouched", func(t *testing.T) {
		assert.Equal(t, 1, count(t,
			`SELECT count(*) FROM message_history WHERE contact_email = $1 AND message_data::text LIKE '%' || $1 || '%'`,
			bystander), "the redaction was not scoped to the deleted address")
	})
}
