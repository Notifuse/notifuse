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

// TestOptOutHashMatrix deletes a contact in every state they can have on a list, on three
// kinds of list, with and without forget_opt_outs, then brings the address back by every
// path that creates a contact, and checks the resulting state against one table.
//
// The rule under test: a deleted contact's unsubscribe, bounce or complaint, removed from the
// list or not, comes back with the address and holds exactly as it would have held had the
// contact never been deleted; with forget_opt_outs, nothing comes back. A subscribe that the
// remembered opt-out refuses writes nothing: the address does not come back yet, and the
// opt-out stays remembered for its next return.
//
// Paths back: an import with subscribe_to_lists, contacts.upsert, lists.subscribe with an API
// key, the public form (no signature), the contact's signed link, and a custom event.
func TestOptOutHashMatrix(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx := context.Background()
	factory := suite.DataFactory
	client := suite.APIClient
	serverURL := suite.ServerManager.GetURL()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	type listKind struct {
		name        string
		list        *domain.List
		public      bool
		doubleOptin bool
	}
	newKind := func(name string, public, doubleOptin bool) listKind {
		list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(public), testutil.WithListDoubleOptin(doubleOptin),
			func(l *domain.List) {
				if doubleOptin {
					l.DoubleOptInTemplate = &domain.TemplateReference{ID: template.ID, Version: 1}
				}
			})
		require.NoError(t, err)
		return listKind{name: name, list: list, public: public, doubleOptin: doubleOptin}
	}
	kinds := []listKind{
		newKind("public double opt-in", true, true),
		newKind("public single opt-in", true, false),
		newKind("private", false, false),
	}

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
	post := func(t *testing.T, path string, body interface{}) int {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Less(t, resp.StatusCode, 500, "%s: %s", path, raw)
		return resp.StatusCode
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	sent := 0
	// The public route allows 50 requests a minute per IP (read from X-Forwarded-For).
	postPublic := func(t *testing.T, payload map[string]interface{}) int {
		t.Helper()
		sent++
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, serverURL+"/subscribe", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.%d.%d", 113+sent/250, 1+sent%250))
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	states := []string{"none", "active", "pending", "unsubscribed", "bounced", "complained",
		"removed active", "removed pending", "removed unsubscribed", "removed bounced", "removed complained"}
	deletions := []string{"delete", "delete forgetting opt-outs"}
	paths := []string{"import", "upsert", "api", "form", "signed", "custom event"}

	isOptOut := func(state string) bool {
		return domain.IsOptOutContactListStatus(domain.ContactListStatus(strings.TrimPrefix(state, "removed ")))
	}
	type outcome struct {
		code  int // 0 where the path answers no single request worth checking
		after string
		// refused: a subscribe the remembered opt-out refuses writes nothing, so the address
		// does not come back and its hash stays for a later return.
		refused bool
	}
	// expect gives the state after the deletion and the return. The deletion leaves the
	// opt-out it remembered (removed or not) or nothing; the path then acts on that state as
	// it would on a contact never deleted (see TestResubscribeMatrix for those rules).
	expect := func(kind listKind, before, deletion, path string) outcome {
		state := "none"
		if deletion == "delete" && isOptOut(before) {
			state = before
		}
		switch path {
		case "upsert", "custom event":
			return outcome{after: state}
		case "import":
			if isOptOut(state) {
				return outcome{code: http.StatusOK, after: state}
			}
			return outcome{code: http.StatusOK, after: "active"}
		case "api":
			if isOptOut(state) {
				return outcome{code: http.StatusOK, after: state, refused: true}
			}
			return outcome{code: http.StatusOK, after: "active"}
		case "form":
			if !kind.public {
				return outcome{code: http.StatusBadRequest, after: state, refused: isOptOut(state)}
			}
			if state == "none" {
				if kind.doubleOptin {
					return outcome{code: http.StatusOK, after: "pending"}
				}
				return outcome{code: http.StatusOK, after: "active"}
			}
			// On a double opt-in list, the form sends an unsubscribed contact the list's
			// confirmation email, so the address comes back to receive it.
			confirms := kind.doubleOptin && strings.TrimPrefix(state, "removed ") == "unsubscribed"
			return outcome{code: http.StatusOK, after: state, refused: !confirms}
		case "signed":
			status := strings.TrimPrefix(state, "removed ")
			if status == "bounced" || status == "complained" {
				return outcome{code: http.StatusOK, after: state, refused: true}
			}
			if !kind.public {
				return outcome{code: http.StatusBadRequest, after: state, refused: isOptOut(state)}
			}
			return outcome{code: http.StatusOK, after: "active"}
		}
		t.Fatalf("no outcome for %s", path)
		return outcome{}
	}

	for _, kind := range kinds {
		for _, before := range states {
			for _, deletion := range deletions {
				for _, path := range paths {
					t.Run(fmt.Sprintf("%s, %s, %s, back by %s", kind.name, before, deletion, path), func(t *testing.T) {
						email := fmt.Sprintf("hash-matrix-%s@example.com", strings.ToLower(uuid.New().String()[:12]))
						_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
						require.NoError(t, err)
						if before != "none" {
							status, removed := strings.CutPrefix(before, "removed ")
							_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
								testutil.WithContactListListID(kind.list.ID), testutil.WithContactListStatus(domain.ContactListStatus(status)))
							require.NoError(t, err)
							if removed {
								require.Equal(t, http.StatusOK, post(t, "/api/contactLists.removeContact", map[string]interface{}{
									"workspace_id": workspace.ID, "email": email, "list_id": kind.list.ID,
								}))
							}
						}
						require.Equal(t, before, stateOf(t, email, kind.list.ID))

						body := map[string]interface{}{"workspace_id": workspace.ID, "email": email}
						if deletion != "delete" {
							body["forget_opt_outs"] = true
						}
						require.Equal(t, http.StatusOK, post(t, "/api/contacts.delete", body))
						require.Equal(t, "none", stateOf(t, email, kind.list.ID))

						want := expect(kind, before, deletion, path)
						// The address comes back in capitals: the hash and the contact both
						// normalise it.
						back := strings.ToUpper(email[:1]) + email[1:]
						code := 0
						switch path {
						case "import":
							code = post(t, "/api/contacts.import", map[string]interface{}{
								"workspace_id": workspace.ID, "contacts": []map[string]interface{}{{"email": back}},
								"subscribe_to_lists": []string{kind.list.ID},
							})
						case "upsert":
							require.Equal(t, http.StatusOK, post(t, "/api/contacts.upsert", map[string]interface{}{
								"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": back},
							}))
						case "api":
							code = post(t, "/api/lists.subscribe", map[string]interface{}{
								"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": back},
								"list_ids": []string{kind.list.ID},
							})
						case "form", "signed":
							contact := map[string]interface{}{"email": back}
							if path == "signed" {
								contact["email_hmac"] = domain.ComputeEmailHMAC(email, workspace.Settings.SecretKey)
							}
							code = postPublic(t, map[string]interface{}{
								"workspace_id": workspace.ID, "contact": contact, "list_ids": []string{kind.list.ID},
							})
						case "custom event":
							require.Equal(t, http.StatusOK, post(t, "/api/customEvents.upsert", map[string]interface{}{
								"workspace_id": workspace.ID, "email": back, "event_name": "came_back",
								"external_id": uuid.New().String(),
							}))
						}

						if want.code != 0 {
							assert.Equal(t, want.code, code, "HTTP status")
						}
						if want.refused {
							assert.Equal(t, "none", stateOf(t, email, kind.list.ID), "a refused subscribe brings nothing back")
							assert.Equal(t, 1, countHashesFor(t, db, email), "the opt-out stays remembered for a later return")
						} else {
							assert.Equal(t, want.after, stateOf(t, email, kind.list.ID), "state afterwards")
							assert.Zero(t, countHashesFor(t, db, email), "no hash is left once the address is back")
						}
					})
				}
			}
		}
	}
}
