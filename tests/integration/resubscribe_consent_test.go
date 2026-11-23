package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// TestResubscribeConsent covers the rule that nothing re-subscribes a contact who opted
// out unless the caller says so, through the real HTTP routes and the real automation
// scheduler, as an integration sends them: raw JSON bodies, an API key and a console
// session.
func TestResubscribeConsent(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx := context.Background()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	factory := suite.DataFactory
	console := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, console.Login(user.Email, "password"))
	console.SetWorkspaceID(workspace.ID)
	db, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
	key, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	apiKey := testutil.NewAPIClient(suite.ServerManager.GetURL())
	apiKey.SetToken(authService.GenerateAPIAuthToken(key))
	apiKey.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	// newContactWithStatus creates a contact whose row on the list holds status, written
	// directly: how they got there is not what these cases are about.
	newContactWithStatus := func(t *testing.T, status domain.ContactListStatus) string {
		t.Helper()
		email := fmt.Sprintf("resub-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx,
			`INSERT INTO contact_lists (email, list_id, status, created_at, updated_at) VALUES ($1, $2, $3, NOW(), NOW())`,
			email, list.ID, status)
		require.NoError(t, err)
		return email
	}
	statusOf := func(t *testing.T, email string) domain.ContactListStatus {
		t.Helper()
		var status string
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, list.ID).Scan(&status))
		return domain.ContactListStatus(status)
	}
	post := func(t *testing.T, client *testutil.APIClient, path string, body map[string]interface{}) (int, map[string]interface{}) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &decoded), string(raw))
		return resp.StatusCode, decoded
	}
	updateStatus := func(t *testing.T, client *testutil.APIClient, email string, status domain.ContactListStatus, resubscribe bool) (int, map[string]interface{}) {
		t.Helper()
		body := map[string]interface{}{
			"workspace_id": workspace.ID, "email": email, "list_id": list.ID, "status": string(status),
		}
		if resubscribe {
			body["resubscribe"] = true
		}
		return post(t, client, "/api/contactLists.updateStatus", body)
	}
	auditEntries := func(t *testing.T, action, email string) []*domain.AuditLog {
		t.Helper()
		page := listAuditLogs(t, console, map[string]string{"workspace_id": workspace.ID, "limit": "100"})
		var found []*domain.AuditLog
		for _, entry := range page.Logs {
			if entry.Action == action && entry.TargetID == email {
				found = append(found, entry)
			}
		}
		return found
	}

	t.Run("lists.subscribe leaves an unsubscribed contact alone without the flag, and brings them back with it", func(t *testing.T) {
		email := newContactWithStatus(t, domain.ContactListStatusUnsubscribed)
		subscribe := func(resubscribe bool) map[string]interface{} {
			body := map[string]interface{}{
				"workspace_id": workspace.ID,
				"contact":      map[string]interface{}{"email": email},
				"list_ids":     []string{list.ID},
			}
			if resubscribe {
				body["resubscribe"] = true
			}
			code, decoded := post(t, apiKey, "/api/lists.subscribe", body)
			require.Equal(t, http.StatusOK, code, decoded)
			return decoded
		}

		// Twice: a second plain call must not finish what a first one started.
		for i := 0; i < 2; i++ {
			decoded := subscribe(false)
			memberships := decoded["contact_lists"].([]interface{})
			assert.Equal(t, "unsubscribed", memberships[0].(map[string]interface{})["status"])
			assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email))
		}
		assert.Empty(t, auditEntries(t, "lists.subscribe", email), "a plain subscribe is not audited")

		subscribe(true)
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
		entries := auditEntries(t, "lists.subscribe", email)
		require.Len(t, entries, 1, "the re-subscription is audited once")
		assert.Equal(t, []interface{}{list.ID}, entries[0].Metadata[domain.AuditResubscribedListIDs])
	})

	t.Run("contactLists.updateStatus refuses to leave an opt-out without the flag", func(t *testing.T) {
		for _, tc := range []struct {
			stored domain.ContactListStatus
			target domain.ContactListStatus
		}{
			{domain.ContactListStatusUnsubscribed, domain.ContactListStatusActive},
			{domain.ContactListStatusUnsubscribed, domain.ContactListStatusPending},
			{domain.ContactListStatusBounced, domain.ContactListStatusUnsubscribed},
			{domain.ContactListStatusComplained, domain.ContactListStatusBounced},
		} {
			email := newContactWithStatus(t, tc.stored)
			code, body := updateStatus(t, apiKey, email, tc.target, false)
			assert.Equal(t, http.StatusConflict, code, "%s -> %s", tc.stored, tc.target)
			assert.Equal(t, string(tc.stored), body["status"])
			assert.Equal(t, tc.stored, statusOf(t, email))
		}

		// A stronger opt-out is fine.
		email := newContactWithStatus(t, domain.ContactListStatusUnsubscribed)
		code, _ := updateStatus(t, apiKey, email, domain.ContactListStatusComplained, false)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, domain.ContactListStatusComplained, statusOf(t, email))
	})

	t.Run("the flag clears an unsubscribe for anyone, a bounce for a console user only, never a complaint", func(t *testing.T) {
		unsubscribed := newContactWithStatus(t, domain.ContactListStatusUnsubscribed)
		code, _ := updateStatus(t, apiKey, unsubscribed, domain.ContactListStatusActive, true)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, unsubscribed))
		assert.Len(t, auditEntries(t, "contactLists.updateStatus", unsubscribed), 1)

		bounced := newContactWithStatus(t, domain.ContactListStatusBounced)
		code, _ = updateStatus(t, apiKey, bounced, domain.ContactListStatusActive, true)
		assert.Equal(t, http.StatusConflict, code, "an API key cannot clear a bounce")
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, bounced))
		assert.Empty(t, auditEntries(t, "contactLists.updateStatus", bounced), "a refusal is not audited")

		code, _ = updateStatus(t, console, bounced, domain.ContactListStatusActive, true)
		assert.Equal(t, http.StatusOK, code, "a console user can, after a mailbox was fixed")
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, bounced))

		complained := newContactWithStatus(t, domain.ContactListStatusComplained)
		code, _ = updateStatus(t, console, complained, domain.ContactListStatusActive, true)
		assert.Equal(t, http.StatusConflict, code)
		assert.Equal(t, domain.ContactListStatusComplained, statusOf(t, complained))

		code, body := updateStatus(t, console, complained, domain.ContactListStatusPending, true)
		assert.Equal(t, http.StatusBadRequest, code, "resubscribe goes with status active: %v", body)
	})

	// Someone posts the public form with an unsubscribed contact's address, then the
	// workspace's own sync subscribes them without the flag. Neither step may bring them back.
	t.Run("an anonymous form post followed by a plain API call leaves the contact unsubscribed", func(t *testing.T) {
		publicList, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), func(l *domain.List) {
			l.DoubleOptInTemplate = &domain.TemplateReference{ID: "doi-template", Version: 1}
		})
		require.NoError(t, err)
		email := fmt.Sprintf("resub-form-%s@example.com", uuid.New().String()[:8])
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx,
			`INSERT INTO contact_lists (email, list_id, status, created_at, updated_at) VALUES ($1, $2, 'unsubscribed', NOW(), NOW())`,
			email, publicList.ID)
		require.NoError(t, err)
		statusOnPublicList := func() string {
			var status string
			require.NoError(t, db.QueryRowContext(ctx,
				`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, publicList.ID).Scan(&status))
			return status
		}

		anonymous := testutil.NewAPIClient(suite.ServerManager.GetURL())
		resp, err := anonymous.Post("/subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{publicList.ID},
			"resubscribe":  true,
		})
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "unsubscribed", statusOnPublicList(), "the form post writes nothing, not even pending")

		code, decoded := post(t, apiKey, "/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{publicList.ID},
		})
		require.Equal(t, http.StatusOK, code, decoded)
		assert.Equal(t, "unsubscribed", statusOnPublicList())
	})

	t.Run("an automation's add-to-list step leaves an unsubscribed contact unsubscribed", func(t *testing.T) {
		automationID := shortuuid.New()
		triggerNodeID := shortuuid.New()
		addNodeID := shortuuid.New()
		eventName := "resubscribe_consent_" + uuid.New().String()[:8]

		resp, err := console.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspace.ID,
				"name":         "Add to list after an unsubscribe",
				"status":       "draft",
				"trigger": map[string]interface{}{
					"event_kind": "custom_event", "custom_event_name": eventName,
					"frequency": "once",
				},
				"root_node_id": triggerNodeID,
				"nodes": []map[string]interface{}{
					{
						"id": triggerNodeID, "automation_id": automationID, "type": "trigger",
						"config": map[string]interface{}{}, "next_node_id": addNodeID,
						"position": map[string]interface{}{"x": 0, "y": 0},
					},
					{
						"id": addNodeID, "automation_id": automationID, "type": "add_to_list",
						"config":   map[string]interface{}{"list_id": list.ID, "status": "active"},
						"position": map[string]interface{}{"x": 0, "y": 100},
					},
				},
				"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
			},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
		resp, err = console.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"automation_id": automationID,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		email := newContactWithStatus(t, domain.ContactListStatusUnsubscribed)
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, nil))

		completed := waitForAutomationComplete(t, factory, workspace.ID, automationID, email, 15*time.Second)
		require.NotNil(t, completed)
		assert.Equal(t, domain.ContactAutomationStatusCompleted, completed.Status, "the step does not fail the journey")
		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email))

		var output []byte
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT output FROM automation_node_executions WHERE contact_automation_id = $1 AND node_id = $2 AND output ? 'list_id'`,
			completed.ID, addNodeID).Scan(&output))
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(output, &decoded))
		assert.Equal(t, "unsubscribed", decoded["status"])
		assert.Equal(t, "opted_out", decoded["skipped"])
	})
}
