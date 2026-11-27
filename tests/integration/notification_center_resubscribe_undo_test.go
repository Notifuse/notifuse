//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotificationCenterResubscribeUndo exercises the full undo/resubscribe contract
// against real Postgres (triggers, timeline, webhooks, engagement, message_history),
// real HTTP routes (including rate limiting), and a real email delivered to Mailpit.
func TestNotificationCenterResubscribeUndo(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	serverURL := suite.ServerManager.GetURL()
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// Private list that we will deliver an email for and test full undo semantics on.
	privateList, err := factory.CreateList(workspace.ID, testutil.WithListPublic(false))
	require.NoError(t, err)

	// Register a webhook subscription listening to list.unsubscribed and list.resubscribed.
	webhookSubID := createZapierSubscription(t, suite, workspace.ID, "Undo resubscribe webhook",
		"https://hooks.example.com/undo-resubscribe",
		[]string{"list.unsubscribed", "list.resubscribed"},
		[]string{privateList.ID}, nil)

	template1, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	template2, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}),
		testutil.WithBroadcastAudience(domain.AudienceSettings{List: privateList.ID, ExcludeUnsubscribed: true}))
	require.NoError(t, err)

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	sendBroadcastEmail := func(t *testing.T, email string) *url.URL {
		t.Helper()
		resp, err := client.SendBroadcastToIndividual(map[string]interface{}{
			"workspace_id":    workspace.ID,
			"broadcast_id":    broadcast.ID,
			"recipient_email": email,
			"template_id":     template1.ID,
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "send broadcast: %s", body)

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, email, 20*time.Second)
		require.NoError(t, err, "broadcast email should reach Mailpit")
		return listUnsubscribeURL(t, msg)
	}

	type unsubResp struct {
		Success           bool                            `json:"success"`
		UnsubscribedLists []domain.UnsubscribedListResult `json:"unsubscribed_lists"`
	}

	postUnsubscribe := func(t *testing.T, signed url.Values, listID string) (int, unsubResp) {
		t.Helper()
		payload, err := json.Marshal(map[string]interface{}{
			"wid":        signed.Get("wid"),
			"email":      signed.Get("email"),
			"email_hmac": signed.Get("email_hmac"),
			"lids":       []string{listID},
			"mid":        signed.Get("mid"),
		})
		require.NoError(t, err)
		resp, err := httpClient.Post(serverURL+"/unsubscribe", "application/json", bytes.NewReader(payload))
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var decoded unsubResp
		require.NoError(t, json.Unmarshal(raw, &decoded), string(raw))
		return resp.StatusCode, decoded
	}

	postResubscribe := func(t *testing.T, wid, email, emailHMAC, listID, token, xff string) (int, string, http.Header) {
		t.Helper()
		payload, err := json.Marshal(map[string]interface{}{
			"wid":        wid,
			"email":      email,
			"email_hmac": emailHMAC,
			"lid":        listID,
			"token":      token,
		})
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, serverURL+"/resubscribe", bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(raw), resp.Header
	}

	shownListIDs := func(t *testing.T, signed url.Values) []string {
		t.Helper()
		query := url.Values{
			"workspace_id": {signed.Get("wid")},
			"email":        {signed.Get("email")},
			"email_hmac":   {signed.Get("email_hmac")},
		}
		resp, err := httpClient.Get(serverURL + "/preferences?" + query.Encode())
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "preferences: %s", raw)
		var decoded struct {
			ContactLists []struct {
				ListID string `json:"list_id"`
			} `json:"contact_lists"`
		}
		require.NoError(t, json.Unmarshal(raw, &decoded))
		ids := make([]string, 0, len(decoded.ContactLists))
		for _, cl := range decoded.ContactLists {
			ids = append(ids, cl.ListID)
		}
		return ids
	}

	statusOf := func(t *testing.T, email, listID string) (string, bool) {
		t.Helper()
		var status string
		var deletedAt sql.NullTime
		err := db.QueryRowContext(ctx,
			`SELECT status, deleted_at FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, listID).Scan(&status, &deletedAt)
		require.NoError(t, err)
		return status, deletedAt.Valid
	}

	t.Run("private list unsubscribe -> undo resubscribe logs timeline, webhook, engagement, and keeps message_history.unsubscribed_at intact", func(t *testing.T) {
		startedAt := time.Now().UTC().Add(-time.Second)
		email := fmt.Sprintf("undo-e2e-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(privateList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		unsubURL := sendBroadcastEmail(t, email)
		signed := unsubURL.Query()
		mid := signed.Get("mid")
		require.NotEmpty(t, mid)

		// GET on List-Unsubscribe header redirects to confirm_unsubscribe without unsubscribing.
		getResp, err := httpClient.Get(unsubURL.String())
		require.NoError(t, err)
		_ = getResp.Body.Close()
		assert.Equal(t, http.StatusSeeOther, getResp.StatusCode)
		assert.Contains(t, getResp.Header.Get("Location"), "action=confirm_unsubscribe")
		st, removed := statusOf(t, email, privateList.ID)
		assert.Equal(t, "active", st)
		assert.False(t, removed)

		// POST /unsubscribe transitions active -> unsubscribed and mints the undo token.
		code, res := postUnsubscribe(t, signed, privateList.ID)
		require.Equal(t, http.StatusOK, code)
		require.True(t, res.Success)
		require.Len(t, res.UnsubscribedLists, 1)
		assert.Equal(t, privateList.ID, res.UnsubscribedLists[0].ID)
		assert.Equal(t, privateList.Name, res.UnsubscribedLists[0].Name)
		undoToken := res.UnsubscribedLists[0].ResubscribeToken
		require.NotEmpty(t, undoToken)

		// Private list is now unsubscribed, hidden in /preferences, and /subscribe still refuses it.
		st, removed = statusOf(t, email, privateList.ID)
		assert.Equal(t, "unsubscribed", st)
		assert.False(t, removed)
		assert.NotContains(t, shownListIDs(t, signed), privateList.ID)

		subBody, err := json.Marshal(map[string]interface{}{
			"workspace_id": signed.Get("wid"),
			"contact":      map[string]interface{}{"email": email, "email_hmac": signed.Get("email_hmac")},
			"list_ids":     []string{privateList.ID},
		})
		require.NoError(t, err)
		subResp, err := httpClient.Post(serverURL+"/subscribe", "application/json", bytes.NewReader(subBody))
		require.NoError(t, err)
		_ = subResp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, subResp.StatusCode, "plain signed /subscribe must still refuse private lists")

		// Record message_history.unsubscribed_at right after unsubscribe.
		var unsubAtBefore time.Time
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT unsubscribed_at FROM message_history WHERE id = $1`, mid).Scan(&unsubAtBefore))
		require.False(t, unsubAtBefore.IsZero())

		// POST /resubscribe restores the private list membership.
		rCode, rBody, _ := postResubscribe(t, signed.Get("wid"), email, signed.Get("email_hmac"), privateList.ID, undoToken, "198.51.100.10")
		require.Equal(t, http.StatusOK, rCode, rBody)
		assert.JSONEq(t, fmt.Sprintf(`{"success":true,"list":{"id":%q,"name":%q}}`, privateList.ID, privateList.Name), rBody)

		st, removed = statusOf(t, email, privateList.ID)
		assert.Equal(t, "active", st)
		assert.False(t, removed)
		assert.Contains(t, shownListIDs(t, signed), privateList.ID, "restored private list is visible again in /preferences")

		// Verify contact_timeline recorded the exact ordered sequence:
		// list.subscribed -> list.unsubscribed -> list.resubscribed.
		rows, err := db.QueryContext(ctx,
			`SELECT kind, changes FROM contact_timeline WHERE email = $1 AND entity_id = $2 ORDER BY created_at ASC, id ASC`,
			email, privateList.ID)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		var kinds []string
		var resubChanges string
		for rows.Next() {
			var kind, changes string
			require.NoError(t, rows.Scan(&kind, &changes))
			kinds = append(kinds, kind)
			if kind == "list.resubscribed" {
				resubChanges = changes
			}
		}
		require.NoError(t, rows.Err())
		assert.Equal(t, []string{"list.subscribed", "list.unsubscribed", "list.resubscribed"}, kinds)
		assert.JSONEq(t, `{"status":{"old":"unsubscribed","new":"active"}}`, resubChanges)

		// Verify webhook_deliveries queued exact cardinality & order:
		// 1x list.unsubscribed followed by 1x list.resubscribed.
		wRows, err := db.QueryContext(ctx,
			`SELECT event_type, payload::text FROM webhook_deliveries WHERE subscription_id = $1 AND payload->>'email' = $2 ORDER BY created_at ASC, id ASC`,
			webhookSubID, email)
		require.NoError(t, err)
		defer func() { _ = wRows.Close() }()
		var webhookTypes []string
		var webhookPayloads []string
		for wRows.Next() {
			var evType, payloadStr string
			require.NoError(t, wRows.Scan(&evType, &payloadStr))
			webhookTypes = append(webhookTypes, evType)
			webhookPayloads = append(webhookPayloads, payloadStr)
		}
		require.NoError(t, wRows.Err())
		require.Equal(t, []string{"list.unsubscribed", "list.resubscribed"}, webhookTypes)
		var decodedWebhook map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(webhookPayloads[1]), &decodedWebhook))
		assert.Equal(t, "active", decodedWebhook["status"])
		assert.Equal(t, "unsubscribed", decodedWebhook["previous_status"])
		assert.Equal(t, privateList.ID, decodedWebhook["list_id"])

		// Verify message_history.unsubscribed_at stayed untouched (append-only).
		var unsubAtAfter time.Time
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT unsubscribed_at FROM message_history WHERE id = $1`, mid).Scan(&unsubAtAfter))
		assert.True(t, unsubAtBefore.Equal(unsubAtAfter), "message_history.unsubscribed_at must remain append-only")

		// Verify EngagementRecorder.Touch recorded engagement for the contact.
		var lastEngagedAt time.Time
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT last_engaged_at FROM contact_engagement WHERE email = $1`, email).Scan(&lastEngagedAt))
		assert.False(t, lastEngagedAt.Before(startedAt), "engagement clock must be touched on undo resubscribe")

		// Cycle check: a second unsubscribe mints a fresh undo token and allows a second undo.
		code2, res2 := postUnsubscribe(t, signed, privateList.ID)
		require.Equal(t, http.StatusOK, code2)
		require.Len(t, res2.UnsubscribedLists, 1)
		require.NotEmpty(t, res2.UnsubscribedLists[0].ResubscribeToken)
		rCode2, rBody2, _ := postResubscribe(t, signed.Get("wid"), email, signed.Get("email_hmac"), privateList.ID, res2.UnsubscribedLists[0].ResubscribeToken, "198.51.100.11")
		require.Equal(t, http.StatusOK, rCode2, rBody2)
		st, _ = statusOf(t, email, privateList.ID)
		assert.Equal(t, "active", st)
	})

	t.Run("RFC 8058 POST /unsubscribe-oneclick never discloses unsubscribed_lists or undo token", func(t *testing.T) {
		email := fmt.Sprintf("undo-oneclick-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(privateList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		unsubURL := sendBroadcastEmail(t, email)
		resp := requestListUnsubscribeURL(t, http.MethodPost, unsubURL,
			"application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"success":true}`, string(body))
		assert.NotContains(t, string(body), "unsubscribed_lists")
		assert.NotContains(t, string(body), privateList.Name)
	})

	t.Run("security guards: invalid/expired tokens, workspace removal, bounce/complaint, non-member, deleted list", func(t *testing.T) {
		// 1. Non-member targeted by an email for a private list: unsubscribe emits NO token & NO list name.
		nonMember := fmt.Sprintf("undo-nonmember-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(nonMember))
		require.NoError(t, err)
		nonMemberSigned := sendBroadcastEmail(t, nonMember).Query()
		code, res := postUnsubscribe(t, nonMemberSigned, privateList.ID)
		require.Equal(t, http.StatusOK, code)
		assert.Empty(t, res.UnsubscribedLists, "non-member unsubscribe must not emit a private list name or undo token")

		// 2. Active member unsubscribes, then admin removes them: undo must be refused (409) and deleted_at stays set.
		removedEmail := fmt.Sprintf("undo-removed-%s@example.com", uuid.New().String()[:8])
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(removedEmail))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(removedEmail),
			testutil.WithContactListListID(privateList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		removedSigned := sendBroadcastEmail(t, removedEmail).Query()
		code, res = postUnsubscribe(t, removedSigned, privateList.ID)
		require.Equal(t, http.StatusOK, code)
		require.Len(t, res.UnsubscribedLists, 1)
		tok := res.UnsubscribedLists[0].ResubscribeToken

		// Re-unsubscribing an already unsubscribed private list emits nothing.
		codeAgain, resAgain := postUnsubscribe(t, removedSigned, privateList.ID)
		require.Equal(t, http.StatusOK, codeAgain)
		assert.Empty(t, resAgain.UnsubscribedLists)

		// Invalid email_hmac -> 401
		rCode, _, _ := postResubscribe(t, workspace.ID, removedEmail, "wrong-hmac", privateList.ID, tok, "198.51.100.20")
		assert.Equal(t, http.StatusUnauthorized, rCode)

		// Token used for another list -> 400
		otherList, err := factory.CreateList(workspace.ID, testutil.WithListPublic(false))
		require.NoError(t, err)
		rCode, _, _ = postResubscribe(t, workspace.ID, removedEmail, removedSigned.Get("email_hmac"), otherList.ID, tok, "198.51.100.21")
		assert.Equal(t, http.StatusBadRequest, rCode)

		// Expired token -> 400
		expiredTok, err := domain.MintResubscribeTokenAt(workspace.Settings.SecretKey, workspace.ID, removedEmail, privateList.ID, time.Now().UTC().Add(-2*time.Hour))
		require.NoError(t, err)
		rCode, _, _ = postResubscribe(t, workspace.ID, removedEmail, removedSigned.Get("email_hmac"), privateList.ID, expiredTok, "198.51.100.22")
		assert.Equal(t, http.StatusBadRequest, rCode)

		// Workspace removes the contact from the list -> undo returns 409 and deleted_at stays non-null.
		rmResp, err := client.Post("/api/contactLists.removeContact", map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        removedEmail,
			"list_id":      privateList.ID,
		})
		require.NoError(t, err)
		_ = rmResp.Body.Close()
		require.Equal(t, http.StatusOK, rmResp.StatusCode)

		rCode, _, _ = postResubscribe(t, workspace.ID, removedEmail, removedSigned.Get("email_hmac"), privateList.ID, tok, "198.51.100.23")
		assert.Equal(t, http.StatusConflict, rCode)
		st, removed := statusOf(t, removedEmail, privateList.ID)
		assert.Equal(t, "unsubscribed", st)
		assert.True(t, removed, "concurrent or prior removal must survive undo attempt")

		// 3. Contact transitions to complained after unsubscribe -> undo returns 409 and complaint stays.
		complainedEmail := fmt.Sprintf("undo-complained-%s@example.com", uuid.New().String()[:8])
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(complainedEmail))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(complainedEmail),
			testutil.WithContactListListID(privateList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		complainedSigned := sendBroadcastEmail(t, complainedEmail).Query()
		_, res = postUnsubscribe(t, complainedSigned, privateList.ID)
		require.Len(t, res.UnsubscribedLists, 1)
		compTok := res.UnsubscribedLists[0].ResubscribeToken

		_, err = db.ExecContext(ctx, `UPDATE contact_lists SET status = 'complained' WHERE email = $1 AND list_id = $2`, complainedEmail, privateList.ID)
		require.NoError(t, err)

		rCode, _, _ = postResubscribe(t, workspace.ID, complainedEmail, complainedSigned.Get("email_hmac"), privateList.ID, compTok, "198.51.100.24")
		assert.Equal(t, http.StatusConflict, rCode)
		st, _ = statusOf(t, complainedEmail, privateList.ID)
		assert.Equal(t, "complained", st)

		// 4. List deleted after unsubscribe -> undo returns 404.
		deletedList, err := factory.CreateList(workspace.ID, testutil.WithListPublic(false))
		require.NoError(t, err)
		delEmail := fmt.Sprintf("undo-dellist-%s@example.com", uuid.New().String()[:8])
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(delEmail))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(delEmail),
			testutil.WithContactListListID(deletedList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		delHMAC := domain.ComputeEmailHMAC(delEmail, workspace.Settings.SecretKey)
		delSigned := url.Values{"wid": {workspace.ID}, "email": {delEmail}, "email_hmac": {delHMAC}}
		_, res = postUnsubscribe(t, delSigned, deletedList.ID)
		require.Len(t, res.UnsubscribedLists, 1)
		delTok := res.UnsubscribedLists[0].ResubscribeToken

		delResp, err := client.Post("/api/lists.delete", map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           deletedList.ID,
		})
		require.NoError(t, err)
		_ = delResp.Body.Close()
		require.Equal(t, http.StatusOK, delResp.StatusCode)

		rCode, _, _ = postResubscribe(t, workspace.ID, delEmail, delHMAC, deletedList.ID, delTok, "198.51.100.25")
		assert.Equal(t, http.StatusNotFound, rCode)
	})

	t.Run("POST /resubscribe enforces subscribe:email rate limit", func(t *testing.T) {
		rlEmail := fmt.Sprintf("undo-ratelimit-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(rlEmail))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(rlEmail),
			testutil.WithContactListListID(privateList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
		require.NoError(t, err)

		hm := domain.ComputeEmailHMAC(rlEmail, workspace.Settings.SecretKey)
		tok, err := domain.MintResubscribeToken(workspace.Settings.SecretKey, workspace.ID, rlEmail, privateList.ID)
		require.NoError(t, err)

		// subscribe:email policy allows 10 requests per minute; the 11th must return 429 + Retry-After.
		hit429 := false
		for i := 0; i < 12; i++ {
			code, _, hdr := postResubscribe(t, workspace.ID, rlEmail, hm, privateList.ID, tok, fmt.Sprintf("198.51.101.%d", 10+i))
			if code == http.StatusTooManyRequests {
				assert.NotEmpty(t, hdr.Get("Retry-After"))
				hit429 = true
				break
			}
			assert.Equal(t, http.StatusOK, code)
		}
		assert.True(t, hit429, "POST /resubscribe must enforce subscribe:email rate limiting")
	})
}
