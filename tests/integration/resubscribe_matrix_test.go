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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResubscribeMatrix runs every way a contact can be subscribed to a list against every
// state they can have on it, on four kinds of list, and checks the resulting state, the
// HTTP answer and the confirmation email against one table. The rule under test: nothing
// brings back a contact who opted out unless the request says so (the resubscribe flag) or
// the contact does it through their own signed link.
//
// Actions:
//   - form: the public /subscribe with no signature, as a website form posts it;
//   - signed: the public /subscribe with the contact's email_hmac, as the notification
//     center and the confirmation link post it;
//   - api, api resubscribe: lists.subscribe with an API key, without and with the flag;
//   - status, status resubscribe: contactLists.updateStatus to active with an API key,
//     without and with the flag;
//   - console resubscribe: the same with the flag, from a console session;
//   - automation: an automation's add-to-list step;
//   - import: contacts.import with subscribe_to_lists;
//   - mcp subscribe, mcp subscribe resubscribe: the MCP tool subscribe_contact_to_list,
//     without and with resubscribe;
//   - mcp status, mcp status resubscribe: the MCP tool set_contact_list_status to active,
//     without and with resubscribe.
//
// An MCP call has no HTTP status of its own: the table holds the status its audit row
// records for the outcome (200 success, 404 not_found, 409 invalid_state).
//
// A removed row is a membership soft-deleted through contactLists.removeContact; it keeps
// the status it had. Every case has its own contact. The confirmation emails are read from
// Mailpit, and the last subtest follows the link of one as delivered.
func TestResubscribeMatrix(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.MCP.Enabled = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()
	// /mcp answers on the API endpoint's host, which the harness leaves empty.
	suite.Config.APIEndpoint = suite.ServerManager.GetURL()

	ctx := context.Background()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	factory := suite.DataFactory
	console := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, console.Login(user.Email, "password"))
	console.SetWorkspaceID(workspace.ID)
	serverURL := suite.ServerManager.GetURL()
	// The harness runs with an empty API endpoint, which puts relative URLs in the email.
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
	key, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	apiKey := testutil.NewAPIClient(serverURL)
	apiKey.SetToken(authService.GenerateAPIAuthToken(key))
	apiKey.SetWorkspaceID(workspace.ID)

	tag := uuid.New().String()[:8]

	// --- the lists ------------------------------------------------------------------------

	type listKind struct {
		name        string
		list        *domain.List
		public      bool
		doubleOptin bool
		template    bool
		subject     string // of its confirmation email
		eventName   string // the custom event that runs its add-to-list automation
	}
	newKind := func(name string, public, doubleOptin, template bool) *listKind {
		kind := &listKind{name: name, public: public, doubleOptin: doubleOptin, template: template,
			subject:   fmt.Sprintf("resub-confirm-%s-%s", strings.ReplaceAll(name, " ", "-"), tag),
			eventName: fmt.Sprintf("resub_matrix_%s_%s", strings.ReplaceAll(name, " ", "_"), tag)}
		var reference *domain.TemplateReference
		if template {
			tpl, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateSubject(kind.subject),
				testutil.WithTemplateEmailContent(`Confirm: <a href="{{ confirm_subscription_url }}">confirm</a>`))
			require.NoError(t, err)
			reference = &domain.TemplateReference{ID: tpl.ID, Version: 1}
		}
		kind.list, err = factory.CreateList(workspace.ID, testutil.WithListPublic(public), testutil.WithListDoubleOptin(doubleOptin),
			func(l *domain.List) { l.DoubleOptInTemplate = reference })
		require.NoError(t, err)
		return kind
	}
	kinds := []*listKind{
		newKind("public double opt-in", true, true, true),
		newKind("public single opt-in with template", true, false, true),
		newKind("public without template", true, false, false),
		newKind("private double opt-in", false, true, true),
	}

	// One automation per list: a custom event, then an add-to-list step setting active.
	for _, kind := range kinds {
		automationID, triggerID, addID := shortuuid.New(), shortuuid.New(), shortuuid.New()
		resp, err := console.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id": automationID, "workspace_id": workspace.ID, "name": "Add to " + kind.name, "status": "draft",
				"trigger":      map[string]interface{}{"event_kind": "custom_event", "custom_event_name": kind.eventName, "frequency": "once"},
				"root_node_id": triggerID,
				"nodes": []map[string]interface{}{
					{"id": triggerID, "automation_id": automationID, "type": "trigger", "config": map[string]interface{}{},
						"next_node_id": addID, "position": map[string]interface{}{"x": 0, "y": 0}},
					{"id": addID, "automation_id": automationID, "type": "add_to_list",
						"config":   map[string]interface{}{"list_id": kind.list.ID, "status": "active"},
						"position": map[string]interface{}{"x": 0, "y": 100}},
				},
				"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
			},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
		resp, err = console.ActivateAutomation(map[string]interface{}{"workspace_id": workspace.ID, "automation_id": automationID})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
		kind.eventName += "|" + automationID // split below
	}
	automationOf := func(kind *listKind) (eventName, automationID string) {
		parts := strings.SplitN(kind.eventName, "|", 2)
		return parts[0], parts[1]
	}

	// --- helpers --------------------------------------------------------------------------

	stateOf := func(t *testing.T, email, listID string) string {
		t.Helper()
		var status string
		var removed bool
		err := db.QueryRowContext(ctx,
			`SELECT status, deleted_at IS NOT NULL FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, listID).Scan(&status, &removed)
		if err == sql.ErrNoRows {
			return "none"
		}
		require.NoError(t, err)
		if removed {
			return "removed " + status
		}
		return status
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	// postPublic posts to the public /subscribe from its own address: the route allows 50
	// requests a minute per IP (read from X-Forwarded-For), and the matrix sends more.
	sent := 0
	postPublic := func(t *testing.T, payload []byte) int {
		t.Helper()
		sent++
		req, err := http.NewRequest(http.MethodPost, serverURL+"/subscribe", bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.%d.%d", 100+sent/250, 1+sent%250))
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	publicSubscribe := func(t *testing.T, email, listID string, signed bool) int {
		t.Helper()
		contact := map[string]interface{}{"email": email}
		if signed {
			contact["email_hmac"] = domain.ComputeEmailHMAC(email, workspace.Settings.SecretKey)
		}
		// resubscribe is sent on purpose: the public route must ignore it.
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id": workspace.ID, "contact": contact, "list_ids": []string{listID}, "resubscribe": true,
		})
		require.NoError(t, err)
		return postPublic(t, body)
	}
	post := func(t *testing.T, client *testutil.APIClient, path string, body map[string]interface{}) int {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	// --- the outcome table ----------------------------------------------------------------

	states := []string{"none", "active", "pending", "unsubscribed", "bounced", "complained",
		"removed active", "removed pending", "removed unsubscribed", "removed bounced", "removed complained"}
	actions := []string{"form", "signed", "api", "api resubscribe", "status", "status resubscribe",
		"console resubscribe", "automation", "import",
		"mcp subscribe", "mcp subscribe resubscribe", "mcp status", "mcp status resubscribe"}

	type outcome struct {
		code  int    // HTTP status, 0 where no request answers (automation)
		after string // state on the list afterwards
		email bool   // a confirmation email was sent
	}
	same := func(before string, code int) outcome { return outcome{code: code, after: before} }

	// writeActive is what an ordinary write of "active" leaves (the API without the flag,
	// an automation, an import): an opt-out stays as it is, removed or not; anything else,
	// a removed active or pending row included, becomes active.
	writeActive := func(before string, code int) outcome {
		status := strings.TrimPrefix(before, "removed ")
		if domain.IsOptOutContactListStatus(domain.ContactListStatus(status)) {
			return same(before, code)
		}
		return outcome{code: code, after: "active"}
	}

	expect := func(kind *listKind, before, action string) outcome {
		switch action {
		case "form":
			// An anonymous post never reaches a private list, and never writes over an
			// opt-out. A pending or unsubscribed contact is sent the confirmation again
			// when the list has one; a new subscriber goes through double opt-in when the
			// list asks for it.
			if !kind.public {
				return same(before, http.StatusBadRequest)
			}
			switch before {
			case "none", "removed active", "removed pending":
				if kind.doubleOptin {
					return outcome{code: http.StatusOK, after: "pending", email: kind.template}
				}
				return outcome{code: http.StatusOK, after: "active"}
			case "active":
				return same(before, http.StatusOK)
			case "pending", "unsubscribed", "removed unsubscribed":
				return outcome{code: http.StatusOK, after: before, email: kind.template}
			default: // bounced and complained, removed or not
				return same(before, http.StatusOK)
			}

		case "signed":
			// The contact's own request. On a private list it only confirms a pending row
			// and never brings back someone who left; on a public one it subscribes them,
			// an unsubscribed contact included. A bounce or complaint is kept.
			switch before {
			case "bounced", "complained", "removed bounced", "removed complained":
				return same(before, http.StatusOK)
			case "active", "pending":
				return outcome{code: http.StatusOK, after: "active"}
			}
			if !kind.public {
				return same(before, http.StatusBadRequest)
			}
			return outcome{code: http.StatusOK, after: "active"}

		case "api", "automation", "import":
			code := http.StatusOK
			if action == "automation" {
				code = 0
			}
			return writeActive(before, code)

		case "api resubscribe":
			if before == "unsubscribed" || before == "removed unsubscribed" {
				return outcome{code: http.StatusOK, after: "active"}
			}
			return writeActive(before, http.StatusOK)

		case "mcp subscribe", "mcp subscribe resubscribe":
			// lists.subscribe's rules, with no confirmation email. A contact left opted out
			// is an invalid_state answer naming their status, where the API answers 200.
			want := writeActive(before, http.StatusOK)
			if action == "mcp subscribe resubscribe" && (before == "unsubscribed" || before == "removed unsubscribed") {
				want = outcome{code: http.StatusOK, after: "active"}
			}
			if want.after == before && domain.IsOptOutContactListStatus(domain.ContactListStatus(strings.TrimPrefix(before, "removed "))) {
				want.code = http.StatusConflict
			}
			return want

		case "mcp status", "mcp status resubscribe":
			// contactLists.updateStatus's rules with an API key; a contact not on the list
			// is not_found, where the API answers 200 with "contact not in list".
			if before == "none" || strings.HasPrefix(before, "removed ") {
				return same(before, http.StatusNotFound)
			}
			switch before {
			case "active", "pending":
				return outcome{code: http.StatusOK, after: "active"}
			case "unsubscribed":
				if action == "mcp status resubscribe" {
					return outcome{code: http.StatusOK, after: "active"}
				}
				return same(before, http.StatusConflict)
			default: // bounced and complained
				return same(before, http.StatusConflict)
			}

		case "status", "status resubscribe", "console resubscribe":
			// Only a row on the list changes; a missing or removed one answers 200 with
			// "contact not in list" and stays as it is.
			if before == "none" || strings.HasPrefix(before, "removed ") {
				return same(before, http.StatusOK)
			}
			switch before {
			case "active", "pending":
				return outcome{code: http.StatusOK, after: "active"}
			case "unsubscribed":
				if action == "status" {
					return same(before, http.StatusConflict)
				}
				return outcome{code: http.StatusOK, after: "active"}
			case "bounced":
				if action == "console resubscribe" {
					return outcome{code: http.StatusOK, after: "active"}
				}
				return same(before, http.StatusConflict)
			default: // complained
				return same(before, http.StatusConflict)
			}
		}
		t.Fatalf("no outcome for %s", action)
		return outcome{}
	}

	// The MCP calls use one key per list kind and action: the mcp_send limit is per key,
	// and the matrix makes more subscription changes than one key may.
	mcpKeys := map[string]*mcpClient{}
	mcpKey := func(t *testing.T, kind *listKind, action string) *mcpClient {
		t.Helper()
		name := kind.name + "/" + strings.TrimSuffix(action, " resubscribe")
		if client, ok := mcpKeys[name]; ok {
			return client
		}
		key, err := factory.CreateAPIKey(workspace.ID)
		require.NoError(t, err)
		mcpKeys[name] = &mcpClient{t: t, url: serverURL, token: authService.GenerateAPIAuthToken(key)}
		return mcpKeys[name]
	}
	// mcpStatus is the status the audit row of an MCP call records for its outcome.
	mcpStatus := func(t *testing.T, result mcpToolResult) int {
		t.Helper()
		if !result.IsError {
			return http.StatusOK
		}
		return map[string]int{"validation": http.StatusBadRequest, "not_found": http.StatusNotFound,
			"invalid_state": http.StatusConflict, "permission_denied": http.StatusForbidden}[result.errorCode(t)]
	}

	// --- the cases ------------------------------------------------------------------------

	// Recipients of a confirmation email, by subject, as the table expects them.
	wantEmail := map[string]map[string]bool{}
	for _, kind := range kinds {
		wantEmail[kind.subject] = map[string]bool{}
	}

	for _, kind := range kinds {
		for _, before := range states {
			for _, action := range actions {
				t.Run(fmt.Sprintf("%s, %s, %s", kind.name, before, action), func(t *testing.T) {
					email := fmt.Sprintf("resub-matrix-%s@example.com", strings.ToLower(uuid.New().String()[:12]))
					_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
					require.NoError(t, err)
					if before != "none" {
						status, removed := strings.CutPrefix(before, "removed ")
						_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
							testutil.WithContactListListID(kind.list.ID), testutil.WithContactListStatus(domain.ContactListStatus(status)))
						require.NoError(t, err)
						if removed {
							require.Equal(t, http.StatusOK, post(t, console, "/api/contactLists.removeContact", map[string]interface{}{
								"workspace_id": workspace.ID, "email": email, "list_id": kind.list.ID,
							}))
						}
					}
					require.Equal(t, before, stateOf(t, email, kind.list.ID))

					want := expect(kind, before, action)
					var code int
					switch action {
					case "form", "signed":
						code = publicSubscribe(t, email, kind.list.ID, action == "signed")
					case "api", "api resubscribe":
						body := map[string]interface{}{
							"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": email},
							"list_ids": []string{kind.list.ID},
						}
						if action == "api resubscribe" {
							body["resubscribe"] = true
						}
						code = post(t, apiKey, "/api/lists.subscribe", body)
					case "status", "status resubscribe", "console resubscribe":
						body := map[string]interface{}{
							"workspace_id": workspace.ID, "email": email, "list_id": kind.list.ID, "status": "active",
						}
						client := apiKey
						if action != "status" {
							body["resubscribe"] = true
						}
						if action == "console resubscribe" {
							client = console
						}
						code = post(t, client, "/api/contactLists.updateStatus", body)
					case "automation":
						eventName, automationID := automationOf(kind)
						require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, nil))
						completed := waitForAutomationComplete(t, factory, workspace.ID, automationID, email, 15*time.Second)
						require.NotNil(t, completed)
						assert.Equal(t, domain.ContactAutomationStatusCompleted, completed.Status, "the step never fails the journey")
					case "import":
						code = post(t, apiKey, "/api/contacts.import", map[string]interface{}{
							"workspace_id": workspace.ID, "contacts": []map[string]interface{}{{"email": email}},
							"subscribe_to_lists": []string{kind.list.ID},
						})
					case "mcp subscribe", "mcp subscribe resubscribe":
						client := mcpKey(t, kind, action)
						client.t = t
						code = mcpStatus(t, client.call("subscribe_contact_to_list", fmt.Sprintf(`{"workspace_id":%q,"email":%q,"list_id":%q,"resubscribe":%t}`,
							workspace.ID, email, kind.list.ID, action == "mcp subscribe resubscribe")))
					case "mcp status", "mcp status resubscribe":
						client := mcpKey(t, kind, action)
						client.t = t
						code = mcpStatus(t, client.call("set_contact_list_status", fmt.Sprintf(`{"workspace_id":%q,"email":%q,"list_id":%q,"status":"active","resubscribe":%t}`,
							workspace.ID, email, kind.list.ID, action == "mcp status resubscribe")))
					}

					if want.code != 0 {
						assert.Equal(t, want.code, code, "HTTP status")
					}
					assert.Equal(t, want.after, stateOf(t, email, kind.list.ID), "state afterwards")
					if want.email {
						wantEmail[kind.subject][email] = true
					}
				})
			}
		}
	}

	// --- the confirmation emails ----------------------------------------------------------

	t.Run("the confirmation emails went to exactly the contacts the table expects", func(t *testing.T) {
		for _, kind := range kinds {
			if !kind.template {
				continue
			}
			var got map[string][]*testutil.MailpitMessage
			// Sent during the request, so they are in Mailpit by now; the wait covers
			// Mailpit's own indexing.
			testutil.WaitForCondition(t, func() bool {
				got, err = testutil.GetMailpitMessagesByRecipient(t, kind.subject)
				if err != nil {
					return false
				}
				for email := range wantEmail[kind.subject] {
					if len(got[email]) == 0 {
						return false
					}
				}
				return true
			}, 20*time.Second, "waiting for the confirmation emails of "+kind.name)
			for email, messages := range got {
				assert.True(t, wantEmail[kind.subject][email], "%s: unexpected confirmation email to %s", kind.name, email)
				assert.Len(t, messages, 1, "%s: one confirmation email to %s", kind.name, email)
			}
			assert.Len(t, got, len(wantEmail[kind.subject]), kind.name)
		}
	})

	// The whole way back for a contact who unsubscribed: the form post changes nothing and
	// sends the confirmation; the link in that email, as delivered, brings them back.
	t.Run("an unsubscribed contact comes back through the link of the confirmation email", func(t *testing.T) {
		kind := kinds[1] // public, single opt-in, with a template
		email := fmt.Sprintf("resub-link-%s@example.com", strings.ToLower(uuid.New().String()[:12]))
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
			testutil.WithContactListListID(kind.list.ID), testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, publicSubscribe(t, email, kind.list.ID, false))
		require.Equal(t, "unsubscribed", stateOf(t, email, kind.list.ID))

		var message *testutil.MailpitMessage
		testutil.WaitForCondition(t, func() bool {
			byRecipient, err := testutil.GetMailpitMessagesByRecipient(t, kind.subject)
			if err != nil || len(byRecipient[email]) == 0 {
				return false
			}
			message, err = testutil.GetMailpitMessage(t, byRecipient[email][0].ID)
			return err == nil
		}, 20*time.Second, "waiting for the confirmation email")

		confirm := confirmLinkOf(t, httpClient, message.HTML)
		query := confirm.Query()
		require.Equal(t, "confirm", query.Get("action"))
		require.Equal(t, kind.list.ID, query.Get("lid"))

		// What the notification center sends when the visitor clicks Confirm on that link's
		// page (notification_center/src/App.tsx); loading the page writes nothing.
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id": query.Get("wid"),
			"contact":      map[string]interface{}{"email": query.Get("email"), "email_hmac": query.Get("email_hmac")},
			"list_ids":     []string{query.Get("lid")},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, postPublic(t, body))
		assert.Equal(t, "active", stateOf(t, email, kind.list.ID))
	})
}

var hrefPattern = regexp.MustCompile(`href="([^"]+)"`)

// confirmLinkOf returns the notification-center confirmation URL of an email. It leads to
// the instance's own page, so it is sent as written: a click redirect leading to it fails
// the test.
func confirmLinkOf(t *testing.T, client *http.Client, html string) *url.URL {
	t.Helper()
	noRedirect := &http.Client{Timeout: client.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	for _, match := range hrefPattern.FindAllStringSubmatch(html, -1) {
		raw := strings.ReplaceAll(match[1], "&amp;", "&")
		if strings.Contains(raw, "action=confirm") && strings.Contains(raw, "notification-center") {
			parsed, err := url.Parse(raw)
			require.NoError(t, err)
			return parsed
		}
		if !strings.HasPrefix(raw, "http") || strings.Contains(raw, "notification-center") {
			continue
		}
		resp, err := noRedirect.Get(raw)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
		location := resp.Header.Get("Location")
		require.NotContains(t, location, "action=confirm", "the confirmation link went through the click redirect")
	}
	t.Fatalf("no confirmation link in the email")
	return nil
}
