//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

func setupAnalyticsOwner(t *testing.T, factory func(cfg *config.Config) testutil.AppInterface, opts ...testutil.WorkspaceOption) (*testutil.IntegrationTestSuite, *domain.Workspace) {
	t.Helper()
	suite := testutil.NewIntegrationTestSuite(t, factory)
	workspace, err := suite.DataFactory.CreateWorkspace(opts...)
	require.NoError(t, err)

	// The seeded test user queries analytics as an owner.
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))
	suite.APIClient.SetWorkspaceID(workspace.ID)
	return suite, workspace
}

// analyticsQuery posts analytics.query with the query written as an API client writes it.
func analyticsQuery(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID, query string) analyticsQueryResponse {
	t.Helper()
	resp, err := suite.APIClient.PostRaw("/api/analytics.query", fmt.Sprintf(`{"workspace_id": %q, "query": %s}`, workspaceID, query))
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", query, body)
	var decoded analyticsQueryResponse
	require.NoError(t, json.Unmarshal(body, &decoded), string(body))
	return decoded
}

// postRawOK posts a raw JSON body and returns the answer, which must be one of the statuses.
func postRawOK(t *testing.T, client *testutil.APIClient, path, body string, statuses ...int) []byte {
	t.Helper()
	resp, err := client.PostRaw(path, body)
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Contains(t, statuses, resp.StatusCode, "%s: %s", path, raw)
	return raw
}

// The broadcasts measures. A status measure counts the broadcasts in that status: "sending"
// is a broadcast whose task queues its emails, "completed" one that has queued them all. A
// broadcast's recipients are the emails it queued, which its task writes when the broadcast
// is processed: the recipient measures read processed broadcasts only, and a draft's 0 does
// not pull the average and the minimum down.
func TestAnalyticsBroadcastMeasures(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite, workspace := setupAnalyticsOwner(t, appFactory)
	defer func() { suite.Cleanup() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))
	factory := suite.DataFactory
	client := suite.APIClient
	_, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// One broadcast sent for real to three contacts: its task writes enqueued_count when it
	// is processed.
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}
	raw := postRawOK(t, client, "/api/broadcasts.create", fmt.Sprintf(`{
		"workspace_id": %q,
		"name": "Sent for real",
		"audience": {"list": %q, "exclude_unsubscribed": true},
		"test_settings": {"enabled": false, "variations": [{"template_id": %q}]}
	}`, workspace.ID, list.ID, template.ID), http.StatusCreated)
	var created struct {
		Broadcast struct {
			ID string `json:"id"`
		} `json:"broadcast"`
	}
	require.NoError(t, json.Unmarshal(raw, &created))
	postRawOK(t, client, "/api/broadcasts.schedule", fmt.Sprintf(`{"workspace_id": %q, "id": %q, "send_now": true}`,
		workspace.ID, created.Broadcast.ID), http.StatusOK)
	_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, created.Broadcast.ID, []string{"processed"}, 60*time.Second)
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	var enqueued int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT enqueued_count FROM broadcasts WHERE id = $1`, created.Broadcast.ID).Scan(&enqueued))
	require.Equal(t, 3, enqueued, "the task writes the emails it queued")

	// The other statuses, each with the enqueued_count the task leaves it with: 0 until the
	// broadcast is processed.
	seed := func(status domain.BroadcastStatus, enqueued int, opts ...testutil.BroadcastOption) {
		t.Helper()
		opts = append([]testutil.BroadcastOption{
			testutil.WithBroadcastStatus(status),
			func(b *domain.Broadcast) { b.EnqueuedCount = enqueued },
		}, opts...)
		_, err := factory.CreateBroadcast(workspace.ID, opts...)
		require.NoError(t, err)
	}
	abTest := testutil.WithBroadcastABTesting([]string{template.ID, template.ID})
	seed(domain.BroadcastStatusDraft, 0)
	// An A/B test not started yet: enabled, no phase count written.
	seed(domain.BroadcastStatusDraft, 0, abTest)
	seed(domain.BroadcastStatusScheduled, 0)
	// Sending: queueing its emails, or an A/B test sending its test or its winner.
	seed(domain.BroadcastStatusProcessing, 0)
	seed(domain.BroadcastStatusTesting, 0)
	seed(domain.BroadcastStatusWinnerSelected, 0)
	// Not sending: an A/B test waiting for its winner, with the winner phase it plans, and a
	// paused broadcast.
	seed(domain.BroadcastStatusTestCompleted, 0, abTest, func(b *domain.Broadcast) {
		b.TestPhaseRecipientCount, b.WinnerPhaseRecipientCount = 20, 80
	})
	seed(domain.BroadcastStatusPaused, 0)
	// Processed: one plain, one A/B test whose winner was sent.
	seed(domain.BroadcastStatusProcessed, 1000)
	sentAt := time.Now().UTC()
	seed(domain.BroadcastStatusProcessed, 3000, abTest, func(b *domain.Broadcast) {
		b.TestPhaseRecipientCount, b.WinnerPhaseRecipientCount = 500, 2500
		b.WinnerSentAt = &sentAt
	})
	// Processed before 21.0, which added enqueued_count with a default of 0 and renamed sent
	// to processed: it recorded no count, and is not a broadcast sent to nobody.
	seed(domain.BroadcastStatusProcessed, 0)
	// Neither.
	seed(domain.BroadcastStatusCancelled, 0)
	seed(domain.BroadcastStatusFailed, 0)

	result := analyticsQuery(t, suite, workspace.ID, `{
		"schema": "broadcasts",
		"measures": ["count", "count_draft", "count_scheduled", "count_sending", "completed_broadcasts_count",
			"avg_recipients", "sum_recipients", "max_recipients", "min_recipients",
			"avg_recipients_completed", "sum_recipients_completed", "count_large_broadcasts",
			"count_with_ab_test", "winner_recipients", "test_recipients"]
	}`)
	require.Len(t, result.Data, 1)
	row := result.Data[0]

	assert.Equal(t, float64(14), waNumber(t, row["count"]))
	assert.Equal(t, float64(2), waNumber(t, row["count_draft"]))
	assert.Equal(t, float64(1), waNumber(t, row["count_scheduled"]))
	assert.Equal(t, float64(3), waNumber(t, row["count_sending"]))
	assert.Equal(t, float64(4), waNumber(t, row["completed_broadcasts_count"]), "the one sent before 21.0 included")
	// The processed broadcasts with a count: 3, 1000 and 3000 emails.
	for _, measure := range []string{"avg_recipients", "avg_recipients_completed"} {
		assert.InDelta(t, 4003.0/3, waNumber(t, row[measure]), 0.01, measure)
	}
	for _, measure := range []string{"sum_recipients", "sum_recipients_completed"} {
		assert.Equal(t, float64(4003), waNumber(t, row[measure]), measure)
	}
	assert.Equal(t, float64(3000), waNumber(t, row["max_recipients"]))
	assert.Equal(t, float64(3), waNumber(t, row["min_recipients"]), "neither a draft's 0 nor an old broadcast's is a broadcast sent to nobody")
	// More than 1000: the broadcast of exactly 1000 is not large.
	assert.Equal(t, float64(1), waNumber(t, row["count_large_broadcasts"]))
	// Every broadcast with A/B testing on, the draft included.
	assert.Equal(t, float64(3), waNumber(t, row["count_with_ab_test"]))
	// The winner phase sent, not the one still planned.
	assert.Equal(t, float64(2500), waNumber(t, row["winner_recipients"]))
	assert.Equal(t, float64(520), waNumber(t, row["test_recipients"]))
}

// The automation_node_executions measures on journeys run by the real executor: a skipped
// email, a failed attempt, and the enrollments.
func TestAnalyticsAutomationExecutionMeasures(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite, workspace := setupAnalyticsOwner(t, func(cfg *config.Config) testutil.AppInterface {
		// The webhook step posts to an httptest server on 127.0.0.1.
		cfg.Webhook.AllowPrivateDeliveryHosts = true
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))
	factory := suite.DataFactory
	client := suite.APIClient
	_, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	require.Equal(t, "marketing", template.Category, "only marketing emails check the list status")

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()

	// trigger → email → a webhook that always answers 500.
	automationID, triggerID, emailID, webhookID := shortuuid.New(), shortuuid.New(), shortuuid.New(), shortuuid.New()
	eventName := "analytics_executions_" + strings.ToLower(shortuuid.New()[:8])
	postRawOK(t, client, "/api/automations.create", fmt.Sprintf(`{
		"workspace_id": %[1]q,
		"automation": {
			"id": %[2]q, "workspace_id": %[1]q, "name": "Analytics executions", "status": "draft",
			"list_id": %[3]q,
			"trigger": {"event_kind": "custom_event", "custom_event_name": %[4]q, "frequency": "once"},
			"root_node_id": %[5]q,
			"nodes": [
				{"id": %[5]q, "automation_id": %[2]q, "type": "trigger", "config": {}, "position": {"x": 0, "y": 0}, "next_node_id": %[6]q},
				{"id": %[6]q, "automation_id": %[2]q, "type": "email", "config": {"template_id": %[7]q}, "position": {"x": 0, "y": 0}, "next_node_id": %[8]q},
				{"id": %[8]q, "automation_id": %[2]q, "type": "webhook", "config": {"url": %[9]q}, "position": {"x": 0, "y": 0}}
			]
		}
	}`, workspace.ID, automationID, list.ID, eventName, triggerID, emailID, template.ID, webhookID, receiver.URL), http.StatusCreated)
	postRawOK(t, client, "/api/automations.activate",
		fmt.Sprintf(`{"workspace_id": %q, "automation_id": %q}`, workspace.ID, automationID), http.StatusOK)

	// One member who gets the email and reaches the failing webhook; one who unsubscribed,
	// whose email is skipped.
	suffix := strings.ToLower(shortuuid.New()[:8])
	member, unsubscribed := "member-"+suffix+"@example.com", "unsubscribed-"+suffix+"@example.com"
	for email, status := range map[string]domain.ContactListStatus{
		member:       domain.ContactListStatusActive,
		unsubscribed: domain.ContactListStatusUnsubscribed,
	} {
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(status))
		require.NoError(t, err)
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, nil))
	}

	// Both journeys done for now: the skip exited its journey, and the failed attempt is
	// recorded (the retry is minutes away).
	ca := waitForAutomationStatus(t, factory, workspace.ID, automationID, unsubscribed, domain.ContactAutomationStatusExited, 30*time.Second)
	require.NotNil(t, ca.ExitReason)
	require.Equal(t, string(domain.ContactListStatusUnsubscribed), *ca.ExitReason)
	require.Eventually(t, func() bool {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT retry_count FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`,
			automationID, member).Scan(&n))
		return n == 1
	}, 30*time.Second, 100*time.Millisecond, "the failed attempt persisted")

	filter := fmt.Sprintf(`[{"member": "automation_id", "operator": "equals", "values": [%q]}]`, automationID)
	result := analyticsQuery(t, suite, workspace.ID, fmt.Sprintf(`{
		"schema": "automation_node_executions",
		"measures": ["count", "count_entered", "count_completed", "count_failed", "count_skipped"],
		"filters": %s
	}`, filter))
	require.Len(t, result.Data, 1)
	row := result.Data[0]
	// entered ×2, trigger ×2, email ×2, the webhook's own row and its failure record.
	assert.Equal(t, float64(8), waNumber(t, row["count"]))
	assert.Equal(t, float64(2), waNumber(t, row["count_entered"]), "one enrollment each")
	assert.Equal(t, float64(1), waNumber(t, row["count_skipped"]), "the email to the unsubscribed contact")
	assert.Equal(t, float64(3), waNumber(t, row["count_completed"]), "both triggers and the email sent, not the skipped one")
	assert.Equal(t, float64(1), waNumber(t, row["count_failed"]), "one failed attempt, counted once")

	t.Run("a failure is filed under the step that failed", func(t *testing.T) {
		byType := analyticsQuery(t, suite, workspace.ID, fmt.Sprintf(`{
			"schema": "automation_node_executions",
			"measures": ["count_failed"],
			"dimensions": ["node_type"],
			"filters": %s
		}`, filter))
		got := map[string]float64{}
		for _, row := range byType.Data {
			got[row["node_type"].(string)] = waNumber(t, row["count_failed"])
		}
		assert.Equal(t, map[string]float64{
			string(domain.NodeTypeTrigger): 0,
			string(domain.NodeTypeEmail):   0,
			string(domain.NodeTypeWebhook): 1,
		}, got)
	})

	t.Run("the completed rows split as the step statistics count them", func(t *testing.T) {
		// Before 44.0 an add-to-list or remove-from-list step whose list write failed
		// completed with the error in its output; no step writes one since, so the row is
		// inserted as those versions wrote it. It goes on one of the automation's steps,
		// because the step statistics report only those.
		var caID string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`,
			automationID, member).Scan(&caID))
		_, err := db.ExecContext(ctx, `INSERT INTO automation_node_executions
			(id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, completed_at, output)
			VALUES ($1, $2, $3, $4, 'webhook', 'completed', NOW(), NOW(), '{"error": "list not found"}')`,
			shortuuid.New(), caID, automationID, webhookID)
		require.NoError(t, err)

		result := analyticsQuery(t, suite, workspace.ID, fmt.Sprintf(`{
			"schema": "automation_node_executions",
			"measures": ["count_completed", "count_skipped", "count_errors"],
			"filters": %s
		}`, filter))
		require.Len(t, result.Data, 1)
		row := result.Data[0]
		assert.Equal(t, float64(1), waNumber(t, row["count_errors"]), "the step that recorded an error")
		assert.Equal(t, float64(3), waNumber(t, row["count_completed"]), "the errored step is not a success")
		assert.Equal(t, float64(1), waNumber(t, row["count_skipped"]))

		resp, err := client.Get("/api/automations.nodeStats", map[string]string{"workspace_id": workspace.ID, "automation_id": automationID})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var stats domain.NodeStatsResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&stats))
		var completed, errs int64
		for _, node := range stats.Nodes {
			completed += node.Completed
			errs += node.Errors
		}
		assert.Equal(t, int64(waNumber(t, row["count_completed"])+waNumber(t, row["count_skipped"])), completed,
			"the step statistics count successes and skipped emails together")
		assert.Equal(t, int64(waNumber(t, row["count_errors"])), errs)
	})
}

// The email_queue measures on entries written by the queue's own writers: a retry scheduled,
// a failure given up on at its first attempt, a paused broadcast's entries.
func TestAnalyticsEmailQueueMeasures(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	// No workers: the entries stay as written.
	suite, workspace := setupAnalyticsOwner(t, appFactory)
	defer func() { suite.Cleanup() }()
	ctx := context.Background()
	queue := suite.ServerManager.GetApp().GetEmailQueueRepository()

	automationID, broadcastID := "auto-"+uuid.New().String()[:8], "bcast-"+uuid.New().String()[:8]
	entry := func(email, sourceID string, source domain.EmailQueueSourceType) *domain.EmailQueueEntry {
		e := testutil.CreateTestEmailQueueEntry("integration", email, sourceID, source)
		e.ID = uuid.New().String()
		return e
	}
	pending := entry("pending@example.com", automationID, domain.EmailQueueSourceAutomation)
	retrying := entry("retrying@example.com", automationID, domain.EmailQueueSourceAutomation)
	givenUp := entry("given-up@example.com", automationID, domain.EmailQueueSourceAutomation)
	paused1 := entry("paused-1@example.com", broadcastID, domain.EmailQueueSourceBroadcast)
	paused2 := entry("paused-2@example.com", broadcastID, domain.EmailQueueSourceBroadcast)
	require.NoError(t, queue.Enqueue(ctx, workspace.ID, []*domain.EmailQueueEntry{pending, retrying, givenUp, paused1, paused2}))

	// A temporary failure: retried tomorrow.
	require.NoError(t, queue.MarkAsProcessing(ctx, workspace.ID, retrying.ID))
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	require.NoError(t, queue.MarkAsFailed(ctx, workspace.ID, retrying.ID, "421 try later", &tomorrow))
	// A mailbox that does not exist: given up on at its first attempt of three.
	require.NoError(t, queue.MarkAsProcessing(ctx, workspace.ID, givenUp.ID))
	require.NoError(t, queue.MarkAsPermanentlyFailed(ctx, workspace.ID, givenUp.ID, "550 no such user"))
	// The broadcast is paused.
	n, err := queue.PauseBySource(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)

	result := analyticsQuery(t, suite, workspace.ID, `{
		"schema": "email_queue",
		"measures": ["count", "count_pending", "count_processing", "count_failed", "count_retryable", "count_paused", "max_attempts"]
	}`)
	require.Len(t, result.Data, 1)
	row := result.Data[0]
	assert.Equal(t, float64(5), waNumber(t, row["count"]))
	assert.Equal(t, float64(1), waNumber(t, row["count_pending"]))
	assert.Equal(t, float64(0), waNumber(t, row["count_processing"]))
	assert.Equal(t, float64(2), waNumber(t, row["count_failed"]), "retrying or given up")
	assert.Equal(t, float64(1), waNumber(t, row["count_retryable"]), "the failure given up on has attempts left, and no retry")
	assert.Equal(t, float64(2), waNumber(t, row["count_paused"]))
	assert.Equal(t, float64(1), waNumber(t, row["max_attempts"]), "attempts made, not the limit of 3")
}

// Every measure and dimension of every schema runs against a workspace database, web
// analytics included: a column the table does not have fails the query, whoever asks for it.
func TestAnalyticsPredefinedSchemasRun(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	settings := &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	suite, workspace := setupAnalyticsOwner(t, appFactory, func(w *domain.Workspace) {
		w.Settings.WebAnalytics = settings
	})
	defer func() { suite.Cleanup() }()

	schemas := domain.ResolveAnalyticsSchemas(settings, "UTC")
	require.Contains(t, schemas, "web_sessions")
	// The measure each schema's dimension queries count with.
	counts := map[string]string{"web_sessions": "sessions", "web_pages": "page_count", "web_goals": "goals"}
	schemaNames := make([]string, 0, len(schemas))
	for name := range schemas {
		schemaNames = append(schemaNames, name)
	}
	sort.Strings(schemaNames)

	for _, schemaName := range schemaNames {
		schema := schemas[schemaName]
		count := "count"
		if c, ok := counts[schemaName]; ok {
			count = c
		}
		require.Contains(t, schema.Measures, count, schemaName)
		for measure := range schema.Measures {
			t.Run(schemaName+"/measure/"+measure, func(t *testing.T) {
				result := analyticsQuery(t, suite, workspace.ID, fmt.Sprintf(`{"schema": %q, "measures": [%q]}`, schemaName, measure))
				assert.Len(t, result.Data, 1)
			})
		}
		for dimension, def := range schema.Dimensions {
			t.Run(schemaName+"/dimension/"+dimension, func(t *testing.T) {
				query := fmt.Sprintf(`{"schema": %q, "measures": [%q], "dimensions": [%q]}`, schemaName, count, dimension)
				if def.Type == "time" {
					query = fmt.Sprintf(`{"schema": %q, "measures": [%q], "timeDimensions": [{"dimension": %q, "granularity": "day"}]}`,
						schemaName, count, dimension)
				}
				analyticsQuery(t, suite, workspace.ID, query)
			})
		}
	}
}

// One person browsing in two tabs and one automated session, through /track and the
// session rollup. The entry type follows the session's landing page, not each tab's first
// page, and pages per session leaves the automated session out.
func TestAnalyticsWebEntryTypeAndPagesPerSession(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite, workspace := setupAnalyticsOwner(t, appFactory, func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	})
	defer func() { suite.Cleanup() }()
	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	now := time.Now().UTC()

	page := func(path string, number int, durationMs int64, entered, exited time.Time) string {
		return fmt.Sprintf(`{"type": "pageview", "path": %q, "page_number": %d, "duration": %d, "scroll": 50, "entered_at": %d, "exited_at": %d}`,
			path, number, durationMs, entered.UnixMilli(), exited.UnixMilli())
	}
	beat := func(sessionID string, tabID, seq int64, actions ...string) {
		t.Helper()
		body := fmt.Sprintf(`{
			"workspace_id": %q, "session_id": %q, "tab_id": %d, "seq": %d,
			"actions": [%s],
			"attributes": {"landing_page": "https://shop.example.com/landing", "user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36"},
			"created_at": %d, "updated_at": %d, "sent_at": %d, "sdk_version": "1.0.0"
		}`, workspace.ID, sessionID, tabID, seq, strings.Join(actions, ", "),
			now.Add(-15*time.Minute).UnixMilli(), now.UnixMilli(), now.UnixMilli())
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, []byte(body), nil))
		require.Equal(t, true, decoded["success"], "beat rejected: %v", decoded)
		buffer.FlushAll(context.Background())
	}

	// The person: tab A opens /a-first then /a-second, tab B opens /b-first later.
	person := waUUIDv7At(now.Add(-12*time.Minute), 0x81)
	aFirst := page("/a-first", 1, 1000, now.Add(-12*time.Minute), now.Add(-10*time.Minute))
	beat(person, 111, 1, aFirst)
	beat(person, 111, 2, aFirst, page("/a-second", 2, 2000, now.Add(-10*time.Minute), now.Add(-7*time.Minute)))
	beat(person, 222, 1, page("/b-first", 1, 4000, now.Add(-9*time.Minute), now.Add(-2*time.Minute)))
	// The robot: one page, one beat, nothing after it.
	beat(waUUIDv7At(now.Add(-6*time.Minute), 0x82), 333, 1, page("/robot", 1, 40, now.Add(-6*time.Minute), now.Add(-6*time.Minute)))

	t.Run("the first page of a second tab is a navigation", func(t *testing.T) {
		result := analyticsQuery(t, suite, workspace.ID, `{
			"schema": "web_pages",
			"measures": ["page_count", "landing_page_count"],
			"dimensions": ["page_entry_type"],
			"order": {"page_entry_type": "asc"}
		}`)
		require.Len(t, result.Data, 2)
		got := map[string]float64{}
		for _, row := range result.Data {
			got[row["page_entry_type"].(string)] = waNumber(t, row["page_count"])
		}
		// The person's /a-first and the robot's /robot entered a session; /a-second and
		// /b-first did not.
		assert.Equal(t, map[string]float64{domain.WebEntryTypeLanding: 2, domain.WebEntryTypeNavigation: 2}, got)
		total := waNumber(t, result.Data[0]["landing_page_count"]) + waNumber(t, result.Data[1]["landing_page_count"])
		assert.Equal(t, got[domain.WebEntryTypeLanding], total, "landing pages and entries agree")
	})

	t.Run("pages per session counts the person, not the robot", func(t *testing.T) {
		result := analyticsQuery(t, suite, workspace.ID, `{
			"schema": "web_sessions",
			"measures": ["sessions", "pageviews", "pages_per_session", "automated_sessions"]
		}`)
		require.Len(t, result.Data, 1)
		row := result.Data[0]
		assert.Equal(t, float64(2), waNumber(t, row["sessions"]))
		assert.Equal(t, float64(1), waNumber(t, row["automated_sessions"]))
		assert.Equal(t, float64(4), waNumber(t, row["pageviews"]), "pageviews still count every session")
		assert.Equal(t, float64(3), waNumber(t, row["pages_per_session"]), "three pages over one human session, not four over two")
	})
}
