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

// TestSignedLinkMembershipStateMatrix runs every action an email's signed link allows
// against every state a contact can have on a list, public and private, and checks the
// outcome against one table. Targeted tests missed cases such as removed rows coming back;
// the table covers every combination. Each case has its own contact, and its signature
// and message IDs come from real emails Mailpit received.
//
// "Bound" means the request names a message that was sent to this contact for this list;
// "unbound" names one sent to them for another list. A removed row is a soft-deleted
// membership (contactLists.removeContact).
func TestSignedLinkMembershipStateMatrix(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

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

	// The harness runs with an empty API endpoint, which puts relative URLs in the email.
	// Point the workspace endpoint at the live server so every delivered link is absolute.
	serverURL := suite.ServerManager.GetURL()
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// One list per visibility, plus another list whose emails serve as unbound messages.
	// Each gets a broadcast whose emails carry that list's links.
	type target struct {
		list    *domain.List
		send    func(t *testing.T, email string) *testutil.MailpitMessage
		subject string
	}
	tag := uuid.New().String()[:8]
	newTarget := func(name string, public bool) target {
		list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(public))
		require.NoError(t, err)
		subject := fmt.Sprintf("matrix-%s-%s", name, tag)
		template1, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject(subject))
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject(subject))
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
		require.NoError(t, err)

		send := func(t *testing.T, email string) *testutil.MailpitMessage {
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
			require.Equal(t, http.StatusOK, resp.StatusCode, "send: %s", body)

			messages, err := testutil.WaitForMailpitMessagesFast(t, subject, 20*time.Second)
			require.NoError(t, err)
			for _, m := range messages.Messages {
				if m.Subject == subject && len(m.To) == 1 && strings.EqualFold(m.To[0].Address, email) {
					msg, err := testutil.GetMailpitMessage(t, m.ID)
					require.NoError(t, err)
					return msg
				}
			}
			t.Fatalf("no %q email to %s in Mailpit", subject, email)
			return nil
		}
		return target{list: list, send: send, subject: subject}
	}
	public := newTarget("public", true)
	private := newTarget("private", false)
	other := newTarget("other", true)

	// stateOf reads a contact's state on a list: "none", "removed" or the row's status
	stateOf := func(t *testing.T, email, listID string) string {
		t.Helper()
		var status string
		var removed bool
		err := workspaceDB.QueryRowContext(context.Background(),
			`SELECT status, deleted_at IS NOT NULL FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, listID).Scan(&status, &removed)
		if err == sql.ErrNoRows {
			return "none"
		}
		require.NoError(t, err)
		if removed {
			return "removed"
		}
		return status
	}
	marked := func(t *testing.T, messageID string) bool {
		t.Helper()
		var unsubscribed bool
		require.NoError(t, workspaceDB.QueryRowContext(context.Background(),
			`SELECT unsubscribed_at IS NOT NULL FROM message_history WHERE id = $1`, messageID).Scan(&unsubscribed))
		return unsubscribed
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	post := func(t *testing.T, path string, payload interface{}) (int, string) {
		t.Helper()
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		resp, err := httpClient.Post(serverURL+path, "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(respBody)
	}
	// shown reports whether the preferences page receives listID, failing unless the
	// contact object carries the same lists
	shown := func(t *testing.T, signed url.Values, listID string) bool {
		t.Helper()
		query := url.Values{"workspace_id": {signed.Get("wid")}, "email": {signed.Get("email")}, "email_hmac": {signed.Get("email_hmac")}}
		resp, err := httpClient.Get(serverURL + "/preferences?" + query.Encode())
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "preferences: %s", body)
		var preferences struct {
			ContactLists []struct {
				ListID string `json:"list_id"`
			} `json:"contact_lists"`
			Contact struct {
				ContactLists []struct {
					ListID string `json:"list_id"`
				} `json:"contact_lists"`
			} `json:"contact"`
		}
		require.NoError(t, json.Unmarshal(body, &preferences))
		var top, inContact []string
		for _, cl := range preferences.ContactLists {
			top = append(top, cl.ListID)
		}
		for _, cl := range preferences.Contact.ContactLists {
			inContact = append(inContact, cl.ListID)
		}
		require.Equal(t, top, inContact, "the contact object must carry the same lists")
		for _, id := range top {
			if id == listID {
				return true
			}
		}
		return false
	}

	states := []string{"none", "active", "pending", "unsubscribed", "bounced", "complained", "removed"}

	// The outcome table. An unsubscribe leaves the contact unsubscribed, except that a
	// bounced or complained row is kept: it says more than an opt-out, and is never mailed
	afterUnsubscribe := func(before string) string {
		if before == "bounced" || before == "complained" {
			return before
		}
		return "unsubscribed"
	}
	type subscribeOutcome struct {
		code  int
		after string
	}
	// Through a signature: a private list only confirms a pending row; a public one is
	// open, a left list rejoining through the double opt-in (pending)
	subscribeOutcomes := map[bool]map[string]subscribeOutcome{
		false: { // private
			"none":         {http.StatusBadRequest, "none"},
			"active":       {http.StatusOK, "active"},
			"pending":      {http.StatusOK, "active"},
			"unsubscribed": {http.StatusBadRequest, "unsubscribed"},
			"bounced":      {http.StatusOK, "bounced"},
			"complained":   {http.StatusOK, "complained"},
			"removed":      {http.StatusBadRequest, "removed"},
		},
		true: { // public
			"none":         {http.StatusOK, "active"},
			"active":       {http.StatusOK, "active"},
			"pending":      {http.StatusOK, "active"},
			"unsubscribed": {http.StatusOK, "pending"},
			"bounced":      {http.StatusOK, "bounced"},
			"complained":   {http.StatusOK, "complained"},
			"removed":      {http.StatusOK, "active"},
		},
	}
	// The preferences show public-list rows, and private-list rows the contact is on
	shownFor := func(public bool, before string) bool {
		if before == "none" || before == "removed" {
			return false
		}
		return public || before == "active" || before == "pending"
	}

	for _, tgt := range []target{public, private} {
		visibility := "private"
		if tgt.list.IsPublic {
			visibility = "public"
		}
		for _, before := range states {
			for _, action := range []string{"one-click unsubscribe", "bound unsubscribe", "unbound unsubscribe", "subscribe", "preferences"} {
				t.Run(fmt.Sprintf("%s list, %s, %s", visibility, before, action), func(t *testing.T) {
					email := fmt.Sprintf("matrix-%s@example.com", uuid.New().String()[:12])
					_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
					require.NoError(t, err)
					switch before {
					case "none":
					case "removed":
						_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
							testutil.WithContactListListID(tgt.list.ID), testutil.WithContactListStatus(domain.ContactListStatusActive))
						require.NoError(t, err)
						resp, err := client.Post("/api/contactLists.removeContact", map[string]interface{}{
							"workspace_id": workspace.ID, "email": email, "list_id": tgt.list.ID,
						})
						require.NoError(t, err)
						_ = resp.Body.Close()
						require.Equal(t, http.StatusOK, resp.StatusCode)
					default:
						_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
							testutil.WithContactListListID(tgt.list.ID), testutil.WithContactListStatus(domain.ContactListStatus(before)))
						require.NoError(t, err)
					}
					require.Equal(t, before, stateOf(t, email, tgt.list.ID))

					boundURL := listUnsubscribeURL(t, tgt.send(t, email))
					bound := boundURL.Query()
					unbound := listUnsubscribeURL(t, other.send(t, email)).Query()
					require.Equal(t, tgt.list.ID, bound.Get("lids"))

					switch action {
					case "one-click unsubscribe":
						// What a mail client sends from the delivered List-Unsubscribe header
						resp := requestListUnsubscribeURL(t, http.MethodPost, boundURL,
							"application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
						require.Equal(t, http.StatusOK, resp.StatusCode, "a valid signature never fails")
						assert.Equal(t, afterUnsubscribe(before), stateOf(t, email, tgt.list.ID))
						assert.True(t, marked(t, bound.Get("mid")), "the message records the unsubscribe")

					case "bound unsubscribe", "unbound unsubscribe":
						mid := bound.Get("mid")
						if action == "unbound unsubscribe" {
							mid = unbound.Get("mid")
						}
						code, body := post(t, "/unsubscribe", map[string]interface{}{
							"wid": bound.Get("wid"), "email": bound.Get("email"), "email_hmac": bound.Get("email_hmac"),
							"lids": []string{tgt.list.ID}, "mid": mid,
						})
						require.Equal(t, http.StatusOK, code, "a valid signature never fails: %s", body)

						after := stateOf(t, email, tgt.list.ID)
						if action == "unbound unsubscribe" && before == "none" {
							assert.Equal(t, "none", after, "a message sent for another list must not create a row")
							assert.False(t, marked(t, mid), "nothing was recorded, so the message isn't marked")
							return
						}
						assert.Equal(t, afterUnsubscribe(before), after)
						if action == "bound unsubscribe" {
							assert.True(t, marked(t, mid), "the message records the unsubscribe")
						}

					case "subscribe":
						want := subscribeOutcomes[tgt.list.IsPublic][before]
						code, body := post(t, "/subscribe", map[string]interface{}{
							"workspace_id": bound.Get("wid"),
							"contact":      map[string]interface{}{"id": "", "email": bound.Get("email"), "email_hmac": bound.Get("email_hmac")},
							"list_ids":     []string{tgt.list.ID},
						})
						assert.Equal(t, want.code, code, body)
						assert.Equal(t, want.after, stateOf(t, email, tgt.list.ID))

					case "preferences":
						assert.Equal(t, shownFor(tgt.list.IsPublic, before), shown(t, bound, tgt.list.ID))
						assert.Equal(t, before, stateOf(t, email, tgt.list.ID), "viewing changes nothing")
					}
				})
			}
		}
	}
}
