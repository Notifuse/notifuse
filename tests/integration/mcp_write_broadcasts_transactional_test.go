package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The write and send tools of broadcasts and transactional notifications
// through /mcp: each one changes real state or sends through Mailpit, a
// replay answers as spec §5.8 says, an update keeps what it was not given, a
// key lacking a pair neither sees nor runs the tool, and every call leaves an
// audit row with channel mcp (the harness licence carries audit_logs).
func TestMCPWriteToolsBroadcastsAndTransactional(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, mcpAppFactory)
	defer func() { suite.Cleanup() }()
	suite.Config.APIEndpoint = suite.ServerManager.GetURL()

	factory := suite.DataFactory
	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
	url := suite.ServerManager.GetURL()

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	ownerClient := testutil.NewAPIClient(url)
	require.NoError(t, ownerClient.Login(owner.Email, "password"))

	run := uuid.New().String()[:8]
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateName("MCP send"), testutil.WithTemplateSubject("mcp-"+run))
	require.NoError(t, err)
	other, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateName("MCP other"), testutil.WithTemplateSubject("mcp-other-"+run))
	require.NoError(t, err)
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	// The send tools count in mcp_send, and the mcp_send limit is per key:
	// each case gets its own key.
	newKey := func(permissions domain.UserPermissions) *mcpClient {
		var options []testutil.APIKeyOption
		if permissions != nil {
			options = append(options, testutil.WithAPIKeyPermissions(permissions))
		}
		key, err := factory.CreateAPIKey(workspace.ID, options...)
		require.NoError(t, err)
		return &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(key)}
	}

	args := func(fields string) string {
		return fmt.Sprintf(`{"workspace_id":%q,%s}`, workspace.ID, fields)
	}
	structured := func(t *testing.T, result mcpToolResult) map[string]any {
		t.Helper()
		require.False(t, result.IsError, string(result.StructuredContent))
		var out map[string]any
		require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
		return out
	}
	toolError := func(t *testing.T, result mcpToolResult) map[string]any {
		t.Helper()
		require.True(t, result.IsError, string(result.StructuredContent))
		var out struct {
			Error map[string]any `json:"error"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
		return out.Error
	}
	// auditStatuses reads the status of the MCP rows of an action on a
	// target, oldest first.
	auditStatuses := func(t *testing.T, action, targetID string) []int {
		t.Helper()
		page := listAuditLogs(t, ownerClient, map[string]string{
			"workspace_id": workspace.ID, "actions": action, "target_id": targetID, "limit": "100",
		})
		out := []int{}
		for i := len(page.Logs) - 1; i >= 0; i-- {
			if page.Logs[i].Metadata["channel"] == "mcp" {
				out = append(out, page.Logs[i].StatusCode)
			}
		}
		return out
	}
	broadcastStatus := func(t *testing.T, id string) string {
		t.Helper()
		var status string
		require.NoError(t, db.QueryRow(`SELECT status FROM broadcasts WHERE id = $1`, id).Scan(&status))
		return status
	}
	draft := func(t *testing.T, options ...testutil.BroadcastOption) *domain.Broadcast {
		t.Helper()
		options = append([]testutil.BroadcastOption{
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}),
		}, options...)
		broadcast, err := factory.CreateBroadcast(workspace.ID, options...)
		require.NoError(t, err)
		return broadcast
	}

	t.Run("create_broadcast and update_broadcast keep what they are not given", func(t *testing.T) {
		full := newKey(nil)
		created := args(fmt.Sprintf(`"name":"Launch","list_id":%q,"template_id":%q`, list.ID, template.ID))
		out := structured(t, full.call("create_broadcast", created))
		broadcast := out["broadcast"].(map[string]any)
		id := broadcast["id"].(string)
		assert.Equal(t, "draft", broadcast["status"])
		var exclude bool
		var variations int
		require.NoError(t, db.QueryRow(`SELECT (audience->>'exclude_unsubscribed')::bool, jsonb_array_length(test_settings->'variations') FROM broadcasts WHERE id = $1`, id).
			Scan(&exclude, &variations))
		assert.True(t, exclude, "an omitted exclude_unsubscribed keeps the unsubscribed contacts out")
		assert.Equal(t, 1, variations)

		// A replay creates another draft: the ID is the server's.
		again := structured(t, full.call("create_broadcast", created))
		assert.NotEqual(t, id, again["broadcast"].(map[string]any)["id"])

		// A schedule stored by the console, then an update that names one field.
		_, err := db.Exec(`UPDATE broadcasts SET schedule = '{"is_scheduled":true,"scheduled_date":"2030-01-02","scheduled_time":"09:00","timezone":"UTC","use_recipient_timezone":false}',
			audience = jsonb_set(audience, '{segments}', '["seg1"]') WHERE id = $1`, id)
		require.NoError(t, err)
		structured(t, full.call("update_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"utm_parameters":{"campaign":"launch"}`, id))))
		var name, schedule, audience, templateID, campaign string
		require.NoError(t, db.QueryRow(`SELECT name, schedule::text, audience::text, test_settings->'variations'->0->>'template_id', utm_parameters->>'campaign' FROM broadcasts WHERE id = $1`, id).
			Scan(&name, &schedule, &audience, &templateID, &campaign))
		assert.Equal(t, "Launch", name, "the name left out is kept")
		assert.JSONEq(t, `{"is_scheduled":true,"scheduled_date":"2030-01-02","scheduled_time":"09:00","timezone":"UTC","use_recipient_timezone":false}`, schedule)
		assert.JSONEq(t, fmt.Sprintf(`{"list":%q,"segments":["seg1"],"exclude_unsubscribed":true}`, list.ID), audience)
		assert.Equal(t, template.ID, templateID)
		assert.Equal(t, "launch", campaign)

		structured(t, full.call("update_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"exclude_unsubscribed":false,"template_id":%q`, id, other.ID))))
		require.NoError(t, db.QueryRow(`SELECT audience::text, test_settings->'variations'->0->>'template_id' FROM broadcasts WHERE id = $1`, id).Scan(&audience, &templateID))
		assert.JSONEq(t, fmt.Sprintf(`{"list":%q,"segments":["seg1"],"exclude_unsubscribed":false}`, list.ID), audience)
		assert.Equal(t, other.ID, templateID)

		assert.Equal(t, "not_found", toolError(t, full.call("update_broadcast", args(`"broadcast_id":"nope","name":"X"`)))["code"])
		assert.Equal(t, []int{200}, auditStatuses(t, "broadcasts.create", id))
		assert.Equal(t, []int{200, 200}, auditStatuses(t, "broadcasts.update", id))
	})

	t.Run("schedule_broadcast, cancel_broadcast and delete_broadcast, with their replays", func(t *testing.T) {
		full := newKey(nil)
		broadcast := draft(t)
		when := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
		out := structured(t, full.call("schedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"when":%q`, broadcast.ID, when.Format(time.RFC3339)))))
		assert.Equal(t, "scheduled", out["status"])
		assert.Equal(t, when.Format(time.RFC3339), out["scheduled_at"])
		var date, clock, timezone string
		require.NoError(t, db.QueryRow(`SELECT schedule->>'scheduled_date', schedule->>'scheduled_time', schedule->>'timezone' FROM broadcasts WHERE id = $1`, broadcast.ID).
			Scan(&date, &clock, &timezone))
		assert.Equal(t, when.Format("2006-01-02"), date)
		assert.Equal(t, when.Format("15:04"), clock)
		assert.Equal(t, "UTC", timezone)
		assert.Equal(t, "scheduled", broadcastStatus(t, broadcast.ID))

		replay := toolError(t, full.call("schedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"when":"now"`, broadcast.ID))))
		assert.Equal(t, "invalid_state", replay["code"])
		assert.Equal(t, "scheduled", replay["status"])
		past := toolError(t, full.call("schedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"when":"2020-01-01T00:00:00Z"`, broadcast.ID))))
		assert.Equal(t, "validation", past["code"])

		out = structured(t, full.call("cancel_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, false, out["already"])
		assert.Equal(t, "cancelled", broadcastStatus(t, broadcast.ID))
		out = structured(t, full.call("cancel_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, true, out["already"])

		out = structured(t, full.call("delete_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, true, out["deleted"])
		var rows int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM broadcasts WHERE id = $1`, broadcast.ID).Scan(&rows))
		assert.Equal(t, 0, rows)
		assert.Equal(t, "not_found", toolError(t, full.call("delete_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))["code"])

		assert.Equal(t, []int{200, 409, 400}, auditStatuses(t, "broadcasts.schedule", broadcast.ID))
		assert.Equal(t, []int{200, 200}, auditStatuses(t, "broadcasts.cancel", broadcast.ID))
		assert.Equal(t, []int{200, 404}, auditStatuses(t, "broadcasts.delete", broadcast.ID))
	})

	t.Run("unschedule_broadcast sends a scheduled broadcast back to draft, and a replay answers already", func(t *testing.T) {
		full := newKey(nil)
		broadcast := draft(t)
		when := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
		structured(t, full.call("schedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"when":%q`, broadcast.ID, when.Format(time.RFC3339)))))
		tasks := func() int {
			var count int
			require.NoError(t, suite.DBManager.GetDB().QueryRow(
				`SELECT COUNT(*) FROM tasks WHERE workspace_id = $1 AND broadcast_id = $2 AND type = 'send_broadcast'`,
				workspace.ID, broadcast.ID).Scan(&count))
			return count
		}
		require.Equal(t, 1, tasks())

		// delete_broadcast refuses it and names the tool that unschedules it.
		refused := toolError(t, full.call("delete_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, "invalid_state", refused["code"])
		assert.Equal(t, "scheduled", refused["status"])
		assert.Contains(t, refused["message"], "unschedule_broadcast")
		assert.NotContains(t, refused["message"], "console")

		out := structured(t, full.call("unschedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, "draft", out["status"])
		assert.Equal(t, false, out["already"])
		assert.Equal(t, "draft", broadcastStatus(t, broadcast.ID))
		assert.Equal(t, 0, tasks(), "the send task is removed")
		var date string
		require.NoError(t, db.QueryRow(`SELECT schedule->>'scheduled_date' FROM broadcasts WHERE id = $1`, broadcast.ID).Scan(&date))
		assert.Equal(t, when.Format("2006-01-02"), date, "the date is kept")

		out = structured(t, full.call("unschedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, true, out["already"])
		assert.Equal(t, "draft", out["status"])

		// A cancelled broadcast is refused, and an unknown one is not found.
		cancelled := draft(t)
		structured(t, full.call("schedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"when":%q`, cancelled.ID, when.Format(time.RFC3339)))))
		structured(t, full.call("cancel_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, cancelled.ID))))
		assert.Equal(t, "validation", toolError(t, full.call("unschedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, cancelled.ID))))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("unschedule_broadcast", args(`"broadcast_id":"nope"`)))["code"])

		// The draft can be deleted now.
		structured(t, full.call("delete_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, []int{200, 200}, auditStatuses(t, "broadcasts.unschedule", broadcast.ID))
		assert.Equal(t, []int{409, 200}, auditStatuses(t, "broadcasts.delete", broadcast.ID))
	})

	t.Run("pause_broadcast and resume_broadcast, and a paused broadcast is not deleted", func(t *testing.T) {
		full := newKey(nil)
		later := time.Now().UTC().Add(72 * time.Hour)
		broadcast := draft(t, testutil.WithBroadcastStatus(domain.BroadcastStatusProcessing), func(b *domain.Broadcast) {
			b.Schedule = domain.ScheduleSettings{IsScheduled: true, ScheduledDate: later.Format("2006-01-02"), ScheduledTime: later.Format("15:04"), Timezone: "UTC"}
		})

		out := structured(t, full.call("pause_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, false, out["already"])
		assert.Equal(t, "paused", broadcastStatus(t, broadcast.ID))
		out = structured(t, full.call("pause_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, true, out["already"])

		refused := toolError(t, full.call("delete_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, "invalid_state", refused["code"])
		assert.Equal(t, "paused", refused["status"])
		assert.Equal(t, "paused", broadcastStatus(t, broadcast.ID), "nothing is deleted")

		// Its date is still ahead, so it goes back to waiting for it.
		out = structured(t, full.call("resume_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, "scheduled", out["status"])
		assert.Equal(t, "scheduled", broadcastStatus(t, broadcast.ID))
		replay := toolError(t, full.call("resume_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID))))
		assert.Equal(t, "invalid_state", replay["code"])
		assert.Equal(t, "scheduled", replay["status"])

		assert.Equal(t, []int{200, 200}, auditStatuses(t, "broadcasts.pause", broadcast.ID))
		assert.Equal(t, []int{200, 409}, auditStatuses(t, "broadcasts.resume", broadcast.ID))
		assert.Equal(t, []int{409}, auditStatuses(t, "broadcasts.delete", broadcast.ID))
	})

	t.Run("select_broadcast_winner, replayed with the same template", func(t *testing.T) {
		full := newKey(nil)
		broadcast := draft(t, testutil.WithBroadcastABTesting([]string{template.ID, other.ID}), testutil.WithBroadcastStatus(domain.BroadcastStatusTestCompleted))

		for range 2 {
			out := structured(t, full.call("select_broadcast_winner", args(fmt.Sprintf(`"broadcast_id":%q,"template_id":%q`, broadcast.ID, other.ID))))
			assert.Equal(t, "winner_selected", out["status"])
		}
		var winner string
		require.NoError(t, db.QueryRow(`SELECT winning_template FROM broadcasts WHERE id = $1`, broadcast.ID).Scan(&winner))
		assert.Equal(t, other.ID, winner)
		assert.Equal(t, "winner_selected", broadcastStatus(t, broadcast.ID))

		refused := toolError(t, full.call("select_broadcast_winner", args(fmt.Sprintf(`"broadcast_id":%q,"template_id":%q`, broadcast.ID, template.ID))))
		assert.Equal(t, "invalid_state", refused["code"])
		assert.Equal(t, []int{200, 200, 409}, auditStatuses(t, "broadcasts.selectWinner", broadcast.ID))
	})

	countHistory := func(t *testing.T, query string, values ...any) int {
		t.Helper()
		var count int
		require.NoError(t, db.QueryRow(query, values...).Scan(&count))
		return count
	}

	t.Run("send_broadcast_to_contact sends again on replay", func(t *testing.T) {
		full := newKey(nil)
		broadcast := draft(t)
		recipient := fmt.Sprintf("mcp-individual-%s@example.com", run)
		for range 2 {
			out := structured(t, full.call("send_broadcast_to_contact", args(fmt.Sprintf(`"broadcast_id":%q,"recipient_email":%q`, broadcast.ID, recipient))))
			assert.Equal(t, template.ID, out["template_id"])
		}
		assert.Equal(t, 2, countHistory(t, `SELECT count(*) FROM message_history WHERE contact_email = $1 AND template_id = $2`, recipient, template.ID))
		assert.Equal(t, []int{200, 200}, auditStatuses(t, "broadcasts.sendToIndividual", broadcast.ID))
	})

	t.Run("create, update and delete a transactional notification, with their replays", func(t *testing.T) {
		full := newKey(nil)
		id := "mcp" + run
		created := args(fmt.Sprintf(`"notification_id":%q,"name":"Order","description":"After checkout","email_template_id":%q,"tracking_mode":"no_tracking","utm_parameters":{"source":"shop"}`, id, template.ID))
		out := structured(t, full.call("create_transactional_notification", created))
		assert.Equal(t, id, out["notification"].(map[string]any)["id"])
		assert.Equal(t, "conflict", toolError(t, full.call("create_transactional_notification", created))["code"])

		structured(t, full.call("update_transactional_notification", args(fmt.Sprintf(`"notification_id":%q,"tracking_mode":"disabled"`, id))))
		var name, description, templateID, mode, source string
		read := func() {
			require.NoError(t, db.QueryRow(`SELECT name, description, channels->'email'->>'template_id', tracking_settings->>'tracking_mode', tracking_settings->>'utm_source'
				FROM transactional_notifications WHERE id = $1`, id).Scan(&name, &description, &templateID, &mode, &source))
		}
		read()
		assert.Equal(t, "Order", name)
		assert.Equal(t, "After checkout", description)
		assert.Equal(t, template.ID, templateID)
		assert.Equal(t, "disabled", mode)
		assert.Equal(t, "shop", source, "the UTM parameters left out are kept")

		structured(t, full.call("update_transactional_notification", args(fmt.Sprintf(`"notification_id":%q,"description":"","email_template_id":%q`, id, other.ID))))
		read()
		assert.Equal(t, "Order", name)
		assert.Equal(t, "", description)
		assert.Equal(t, other.ID, templateID)
		assert.Equal(t, "disabled", mode, "the tracking left out is kept")

		structured(t, full.call("delete_transactional_notification", args(fmt.Sprintf(`"notification_id":%q`, id))))
		var deleted bool
		require.NoError(t, db.QueryRow(`SELECT deleted_at IS NOT NULL FROM transactional_notifications WHERE id = $1`, id).Scan(&deleted))
		assert.True(t, deleted)
		assert.Equal(t, "not_found", toolError(t, full.call("delete_transactional_notification", args(fmt.Sprintf(`"notification_id":%q`, id))))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("update_transactional_notification", args(fmt.Sprintf(`"notification_id":%q,"name":"Back"`, id))))["code"])

		assert.Equal(t, []int{200, 409}, auditStatuses(t, "transactional.create", id))
		assert.Equal(t, []int{200, 200, 404}, auditStatuses(t, "transactional.update", id))
		assert.Equal(t, []int{200, 404}, auditStatuses(t, "transactional.delete", id))
	})

	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationID("mcpsend"+run),
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: template.ID}}))
	require.NoError(t, err)

	t.Run("send_transactional sends once per external_id", func(t *testing.T) {
		full := newKey(nil)
		recipient := fmt.Sprintf("mcp-transactional-%s@example.com", run)
		// already says whether the key was used before: false on a first send,
		// true on its replay, which sends nothing.
		send := func(externalID string, already bool) string {
			out := structured(t, full.call("send_transactional", args(fmt.Sprintf(`"notification_id":%q,"external_id":%q,"email":%q,"data":{"order":"981"}`,
				notification.ID, externalID, recipient))))
			assert.Equal(t, already, out["already"], externalID)
			return out["message_id"].(string)
		}
		first := send("order-981-"+run, false)
		assert.Equal(t, first, send("order-981-"+run, true), "a replay answers the first message")
		assert.Equal(t, 1, countHistory(t, `SELECT count(*) FROM message_history WHERE contact_email = $1`, recipient))
		assert.NotEqual(t, first, send("order-982-"+run, false))
		assert.Equal(t, 2, countHistory(t, `SELECT count(*) FROM message_history WHERE contact_email = $1`, recipient))
		assert.Equal(t, 1, countHistory(t, `SELECT count(*) FROM message_history WHERE id = $1 AND external_id = $2 AND transactional_notification_id = $3`,
			first, "order-981-"+run, notification.ID))

		page := listAuditLogs(t, ownerClient, map[string]string{"workspace_id": workspace.ID, "actions": "transactional.send", "target_id": notification.ID, "limit": "100"})
		require.NotEmpty(t, page.Logs)
		assert.Equal(t, recipient, page.Logs[0].Metadata["recipient_email"])
		assert.Equal(t, []int{200, 200, 200}, auditStatuses(t, "transactional.send", notification.ID))
	})

	t.Run("send_template_test sends again on replay", func(t *testing.T) {
		full := newKey(nil)
		recipient := fmt.Sprintf("mcp-test-%s@example.com", run)
		for range 2 {
			out := structured(t, full.call("send_template_test", args(fmt.Sprintf(`"template_id":%q,"recipient_email":%q,"list_id":%q`, template.ID, recipient, list.ID))))
			assert.Equal(t, "noreply@notifuse.test", out["from_email"])
		}
		assert.Equal(t, 2, countHistory(t, `SELECT count(*) FROM message_history WHERE contact_email = $1 AND template_id = $2`, recipient, template.ID))
		assert.Equal(t, []int{200, 200}, auditStatuses(t, "transactional.testTemplate", template.ID))
	})

	t.Run("a key lacking a pair neither sees nor runs the tool", func(t *testing.T) {
		broadcast := draft(t)
		readOnly := newKey(domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true}, domain.PermissionResourceTransactional: {Read: true}, domain.PermissionResourceTemplates: {Read: true},
		})
		sendOnly := newKey(domain.UserPermissions{domain.PermissionResourceBroadcasts: {Write: true}, domain.PermissionResourceTransactional: {Write: true}})
		withTemplates := newKey(domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Write: true}, domain.PermissionResourceTransactional: {Write: true}, domain.PermissionResourceTemplates: {Read: true},
		})
		recipient := fmt.Sprintf("mcp-refused-%s@example.com", run)
		for _, refusal := range []struct {
			client     *mcpClient
			tool       string
			args       string
			listed     bool
			resource   string
			permission string
			action     string
		}{
			{readOnly, "create_broadcast", args(fmt.Sprintf(`"name":"Refused","list_id":%q,"template_id":%q`, list.ID, template.ID)), false, "broadcasts", "write", "broadcasts.create"},
			{readOnly, "update_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"name":"Refused"`, broadcast.ID)), false, "broadcasts", "write", "broadcasts.update"},
			{readOnly, "schedule_broadcast", args(fmt.Sprintf(`"broadcast_id":%q,"when":"now"`, broadcast.ID)), false, "broadcasts", "write", "broadcasts.schedule"},
			{readOnly, "select_broadcast_winner", args(fmt.Sprintf(`"broadcast_id":%q,"template_id":%q`, broadcast.ID, template.ID)), false, "broadcasts", "write", "broadcasts.selectWinner"},
			{readOnly, "pause_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID)), false, "broadcasts", "write", "broadcasts.pause"},
			{readOnly, "resume_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID)), false, "broadcasts", "write", "broadcasts.resume"},
			{readOnly, "cancel_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID)), false, "broadcasts", "write", "broadcasts.cancel"},
			{readOnly, "delete_broadcast", args(fmt.Sprintf(`"broadcast_id":%q`, broadcast.ID)), false, "broadcasts", "write", "broadcasts.delete"},
			{sendOnly, "send_broadcast_to_contact", args(fmt.Sprintf(`"broadcast_id":%q,"recipient_email":%q`, broadcast.ID, recipient)), false, "templates", "read", "broadcasts.sendToIndividual"},
			{sendOnly, "create_transactional_notification", args(fmt.Sprintf(`"notification_id":"refused%s","name":"Refused","email_template_id":%q`, run, template.ID)), false, "templates", "read", "transactional.create"},
			{readOnly, "delete_transactional_notification", args(fmt.Sprintf(`"notification_id":%q`, notification.ID)), false, "transactional", "write", "transactional.delete"},
			{readOnly, "send_transactional", args(fmt.Sprintf(`"notification_id":%q,"external_id":"refused-%s","email":%q`, notification.ID, run, recipient)), false, "transactional", "write", "transactional.send"},
			{sendOnly, "send_template_test", args(fmt.Sprintf(`"template_id":%q,"recipient_email":%q`, template.ID, recipient)), false, "templates", "read", "transactional.testTemplate"},
			// Argument-dependent pairs: the tool is listed, and the call refused.
			{sendOnly, "update_transactional_notification", args(fmt.Sprintf(`"notification_id":%q,"email_template_id":%q`, notification.ID, other.ID)), true, "templates", "read", "transactional.update"},
			{sendOnly, "send_transactional", args(fmt.Sprintf(`"notification_id":%q,"external_id":"refused-cc-%s","email":%q,"cc":["boss@example.com"]`, notification.ID, run, recipient)), true, "contacts", "read", "transactional.send"},
			{withTemplates, "send_template_test", args(fmt.Sprintf(`"template_id":%q,"recipient_email":%q,"list_id":%q`, template.ID, recipient, list.ID)), true, "lists", "read", "transactional.testTemplate"},
		} {
			if refusal.listed {
				assert.Contains(t, refusal.client.toolNames(), refusal.tool)
			} else {
				assert.NotContains(t, refusal.client.toolNames(), refusal.tool)
			}
			result := refusal.client.call(refusal.tool, refusal.args)
			assert.JSONEq(t, fmt.Sprintf(`{"error":{"code":"permission_denied","message":"this API key lacks the %s %s permission","resource":%q,"permission":%q}}`,
				refusal.resource, refusal.permission, refusal.resource, refusal.permission), string(result.StructuredContent), refusal.tool)

			page := listAuditLogs(t, ownerClient, map[string]string{"workspace_id": workspace.ID, "actions": refusal.action, "outcome": domain.AuditOutcomeDenied, "limit": "100"})
			found := false
			for _, row := range page.Logs {
				if row.Metadata["mcp_tool"] == refusal.tool && row.StatusCode == http.StatusForbidden && row.Metadata["resource"] == refusal.resource {
					found = true
				}
			}
			assert.True(t, found, "%s: no audit row with status 403", refusal.tool)
		}

		var name string
		require.NoError(t, db.QueryRow(`SELECT name, status FROM broadcasts WHERE id = $1`, broadcast.ID).Scan(&name, new(string)))
		assert.Equal(t, broadcast.Name, name)
		assert.Equal(t, "draft", broadcastStatus(t, broadcast.ID))
		assert.Equal(t, 0, countHistory(t, `SELECT count(*) FROM message_history WHERE contact_email = $1`, recipient), "nothing was sent")
		var channel sql.NullString
		require.NoError(t, db.QueryRow(`SELECT channels->'email'->>'template_id' FROM transactional_notifications WHERE id = $1`, notification.ID).Scan(&channel))
		assert.Equal(t, template.ID, channel.String)
	})
}
