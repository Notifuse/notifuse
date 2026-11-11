package integration

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestAutomationLinkClickTrigger is issue #498 end to end: an automation started by a click
// on a given link of a given broadcast adds the contact to a list, even when that link is
// not the first one the contact clicked in the email. It ignores other links and other
// broadcasts, never restarts on a click on an email it sent itself, and keeps its filters
// through an update.
func TestAutomationLinkClickTrigger(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	factory := suite.DataFactory
	client := suite.APIClient
	ctx := context.Background()
	// The automation scheduler runs the add_to_list node.
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID
	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspaceID)

	key, err := trackingtoken.DeriveKey(workspace.Settings.SecretKey)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)
	otherBroadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)
	dripList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)

	// The link carries a quote, a '%' and an '_': the WHEN clause must match them literally.
	const pricing = "https://shop.example.com/what's-new/pricing_50%25off?utm_source=news"

	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	addNodeID := shortuuid.New()
	automation := map[string]interface{}{
		"id":           automationID,
		"workspace_id": workspaceID,
		"name":         "Pricing clickers drip",
		"status":       "draft",
		"trigger": map[string]interface{}{
			"event_kind":   "email.link_clicked",
			"link_url":     "What's-New/Pricing_50%25",
			"broadcast_id": broadcast.ID,
			"frequency":    "every_time",
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
				"config":   map[string]interface{}{"list_id": dripList.ID, "status": "active"},
				"position": map[string]interface{}{"x": 0, "y": 100},
			},
		},
		"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
	}
	expectOK := func(t *testing.T, resp *http.Response, err error, want int) {
		t.Helper()
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != want {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected %d, got %d: %s", want, resp.StatusCode, body)
		}
	}
	resp, err := client.CreateAutomation(map[string]interface{}{"workspace_id": workspaceID, "automation": automation})
	expectOK(t, resp, err, http.StatusCreated)
	resp, err = client.ActivateAutomation(map[string]interface{}{"workspace_id": workspaceID, "automation_id": automationID})
	expectOK(t, resp, err, http.StatusOK)

	redirectClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	click := func(t *testing.T, messageID, destination string) {
		t.Helper()
		token, err := trackingtoken.MintClick(key, messageID, workspaceID, time.Now().Add(-30*time.Second).Unix(), destination)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/r/%s", baseURL, token), nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
		resp, err := redirectClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Less(t, resp.StatusCode, 400)
	}
	message := func(t *testing.T, email, broadcastID, fromAutomation string) string {
		t.Helper()
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		opts := []testutil.MessageHistoryOption{
			testutil.WithMessageContact(email), testutil.WithMessageTemplate(template.ID),
			testutil.WithMessageBroadcast(broadcastID),
		}
		if fromAutomation != "" {
			opts = append(opts, testutil.WithMessageHistoryAutomationID(fromAutomation))
		}
		msg, err := factory.CreateMessageHistory(workspaceID, opts...)
		require.NoError(t, err)
		return msg.ID
	}
	inDripList := func(email string) bool {
		var status sql.NullString
		err := workspaceDB.QueryRowContext(ctx,
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2 AND deleted_at IS NULL`,
			email, dripList.ID).Scan(&status)
		return err == nil && status.String == "active"
	}
	enrollments := func(t *testing.T, email string) int {
		t.Helper()
		var n int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*) FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`,
			automationID, email).Scan(&n))
		return n
	}

	t.Run("the pricing link, clicked second, adds the contact to the drip list", func(t *testing.T) {
		email := "pricing-clicker@link.test"
		msg := message(t, email, broadcast.ID, "")
		click(t, msg, "https://shop.example.com/home")
		click(t, msg, pricing)

		require.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, email, 5*time.Second))
		testutil.WaitForCondition(t, func() bool { return inDripList(email) }, 15*time.Second,
			"waiting for the add_to_list node")
		assert.Equal(t, 1, enrollments(t, email))
	})

	t.Run("another link or another broadcast starts nothing", func(t *testing.T) {
		otherLink := "home-only@link.test"
		click(t, message(t, otherLink, broadcast.ID, ""), "https://shop.example.com/home")
		otherCampaign := "other-broadcast@link.test"
		click(t, message(t, otherCampaign, otherBroadcast.ID, ""), pricing)

		// A positive control in the same window: the trigger is live and fast.
		control := "control@link.test"
		click(t, message(t, control, broadcast.ID, ""), pricing)
		require.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, control, 5*time.Second))

		assert.Zero(t, enrollments(t, otherLink))
		assert.Zero(t, enrollments(t, otherCampaign))
	})

	t.Run("a click on an email the automation sent does not restart it", func(t *testing.T) {
		email := "own-email@link.test"
		click(t, message(t, email, broadcast.ID, automationID), pricing)
		control := "own-email-control@link.test"
		click(t, message(t, control, broadcast.ID, ""), pricing)
		require.NotNil(t, waitForEnrollment(t, factory, workspaceID, automationID, control, 5*time.Second))

		assert.Zero(t, enrollments(t, email), "every_time would loop on the automation's own emails")
	})

	t.Run("an update keeps the link filters on the installed trigger", func(t *testing.T) {
		automation["name"] = "Pricing clickers drip (renamed)"
		automation["status"] = "live"
		resp, err := client.UpdateAutomation(map[string]interface{}{"workspace_id": workspaceID, "automation": automation})
		expectOK(t, resp, err, http.StatusOK)

		var def string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal`,
			strings.ToLower("automation_trigger_"+strings.ReplaceAll(automationID, "-", ""))).Scan(&def))
		assert.Contains(t, def, "email.link_clicked")
		assert.Contains(t, def, "clicked_url")
		assert.Contains(t, def, broadcast.ID)
	})
}
