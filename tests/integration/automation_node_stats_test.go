//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// automations.nodeStats on journeys run by the real executor and emails sent through the
// real queue: what each step counts, the journeys in place, the email counters against the
// message history, a period, and the permissions.
func TestAutomationNodeStats(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		// The retry case posts to an httptest server on 127.0.0.1.
		cfg.Webhook.AllowPrivateDeliveryHosts = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()
	ctx := context.Background()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	factory := suite.DataFactory
	client := suite.APIClient

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	// An automation with email steps sends to the members of its list.
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	readBody := func(resp *http.Response) string {
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	node := func(automationID, id, nodeType string, config map[string]interface{}, next string) map[string]interface{} {
		n := map[string]interface{}{
			"id": id, "automation_id": automationID, "type": nodeType, "config": config,
			"position": map[string]interface{}{"x": 0, "y": 0},
		}
		if next != "" {
			n["next_node_id"] = next
		}
		return n
	}
	createAndActivate := func(t *testing.T, automationID, eventName, rootID string, nodes []map[string]interface{}) {
		t.Helper()
		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id": automationID, "workspace_id": workspace.ID, "name": "Node stats " + automationID, "status": "draft",
				"list_id":      list.ID,
				"trigger":      map[string]interface{}{"event_kind": "custom_event", "custom_event_name": eventName, "frequency": "once"},
				"root_node_id": rootID,
				"nodes":        nodes,
			},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode, readBody(resp))
		resp, err = client.ActivateAutomation(map[string]interface{}{"workspace_id": workspace.ID, "automation_id": automationID})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, readBody(resp))
	}
	nodeStats := func(t *testing.T, automationID string, params map[string]string) (int, domain.NodeStatsResponse) {
		t.Helper()
		query := map[string]string{"workspace_id": workspace.ID, "automation_id": automationID}
		for k, v := range params {
			query[k] = v
		}
		resp, err := client.Get("/api/automations.nodeStats", query)
		require.NoError(t, err)
		body := readBody(resp)
		var stats domain.NodeStatsResponse
		if resp.StatusCode == http.StatusOK {
			require.NoError(t, json.Unmarshal([]byte(body), &stats), body)
		}
		return resp.StatusCode, stats
	}
	waitForStats := func(t *testing.T, automationID, what string, ok func(domain.NodeStatsResponse) bool) domain.NodeStatsResponse {
		t.Helper()
		var last domain.NodeStatsResponse
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			status, stats := nodeStats(t, automationID, nil)
			require.Equal(t, http.StatusOK, status)
			last = stats
			if ok(stats) {
				return stats
			}
			time.Sleep(250 * time.Millisecond)
		}
		raw, _ := json.Marshal(last)
		t.Fatalf("timed out waiting for %s; last stats: %s", what, raw)
		return last
	}

	// A: trigger → email-1 → filter on country FR → yes: a one-day delay, the last step;
	// no: email-2.
	automationA := shortuuid.New()
	triggerA, email1, filter, delay, email2 := shortuuid.New(), shortuuid.New(), shortuuid.New(), shortuuid.New(), shortuuid.New()
	createAndActivate(t, automationA, "node_stats_main", triggerA, []map[string]interface{}{
		node(automationA, triggerA, "trigger", map[string]interface{}{}, email1),
		node(automationA, email1, "email", map[string]interface{}{"template_id": template.ID}, filter),
		node(automationA, filter, "filter", map[string]interface{}{
			"conditions": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{"field_name": "country", "field_type": "string", "operator": "equals", "string_values": []string{"FR"}},
						},
					},
				},
			},
			"continue_node_id": delay,
			"exit_node_id":     email2,
		}, ""),
		node(automationA, delay, "delay", map[string]interface{}{"duration": 1, "unit": "days"}, ""),
		node(automationA, email2, "email", map[string]interface{}{"template_id": template.ID}, ""),
	})

	// B: trigger → email-b → a webhook answering 500 once, then 200.
	var calls int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer receiver.Close()
	automationB := shortuuid.New()
	triggerB, emailB, webhook := shortuuid.New(), shortuuid.New(), shortuuid.New()
	createAndActivate(t, automationB, "node_stats_retry", triggerB, []map[string]interface{}{
		node(automationB, triggerB, "trigger", map[string]interface{}{}, emailB),
		node(automationB, emailB, "email", map[string]interface{}{"template_id": template.ID}, webhook),
		node(automationB, webhook, "webhook", map[string]interface{}{"url": receiver.URL}, ""),
	})

	suffix := shortuuid.New()
	fr1, fr2, de1, retry := "fr1-"+suffix+"@example.com", "fr2-"+suffix+"@example.com", "de1-"+suffix+"@example.com", "retry-"+suffix+"@example.com"
	for email, country := range map[string]string{fr1: "FR", fr2: "FR", de1: "DE", retry: "DE"} {
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), testutil.WithContactCountry(country))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}
	for _, email := range []string{fr1, fr2, de1} {
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, "node_stats_main", nil))
	}
	require.NoError(t, factory.CreateCustomEvent(workspace.ID, retry, "node_stats_retry", nil))

	statsA := waitForStats(t, automationA, "two journeys on the delay and one past email-2", func(s domain.NodeStatsResponse) bool {
		return s.Nodes[delay] != nil && s.Nodes[delay].Waiting == 2 && s.Nodes[email2] != nil && s.Nodes[email2].Completed == 1
	})

	t.Run("each step: visits, the filter's split, the journeys waiting", func(t *testing.T) {
		require.Len(t, statsA.Nodes, 5, "every step of the graph")
		for id, want := range map[string]int64{triggerA: 3, email1: 3, filter: 3, delay: 2, email2: 1} {
			assert.Equal(t, want, statsA.Nodes[id].Visits, id)
		}
		assert.Equal(t, map[string]int64{"yes": 2, "no": 1}, statsA.Nodes[filter].Branches)
		assert.Equal(t, domain.NodeTypeFilter, statsA.Nodes[filter].NodeType)
		assert.Equal(t, int64(2), statsA.Nodes[delay].Waiting, "a delay as last step still holds its journeys")
		assert.Equal(t, int64(0), statsA.Nodes[email2].Waiting)
		assert.NotNil(t, statsA.Nodes[triggerA].SkipReasons, "maps are {}, never null")
		assert.Nil(t, statsA.Nodes[delay].Email, "only an email step has email counters")
	})

	// The retry: the first webhook call fails, the journey waits for its retry. The executor
	// marks the attempt failed before handleError writes the failed-attempt row, so a read in
	// between sees the journey retrying with no failed attempt yet: wait for both.
	statsB := waitForStats(t, automationB, "a webhook retry", func(s domain.NodeStatsResponse) bool {
		return s.Nodes[webhook] != nil && s.Nodes[webhook].Retrying == 1 && s.Nodes[webhook].FailedAttempts == 1
	})

	t.Run("a failed attempt: retrying while it waits, counted once", func(t *testing.T) {
		w := statsB.Nodes[webhook]
		assert.Equal(t, int64(1), w.FailedAttempts)
		assert.Equal(t, int64(1), w.Visits)
		assert.Equal(t, int64(0), w.Completed)
		assert.Equal(t, domain.NodeTypeWebhook, w.NodeType, "the type comes from the graph, not the failure row")
	})

	t.Run("after the retry succeeds: completed, no longer retrying", func(t *testing.T) {
		var caID string
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT id FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`, automationB, retry).Scan(&caID))
		require.NoError(t, factory.UpdateContactAutomationScheduledAt(workspace.ID, caID, time.Now().UTC().Add(-time.Second)))

		stats := waitForStats(t, automationB, "the retried webhook to complete", func(s domain.NodeStatsResponse) bool {
			return s.Nodes[webhook].Completed == 1
		})
		w := stats.Nodes[webhook]
		assert.Equal(t, int64(0), w.Retrying)
		assert.Equal(t, int64(1), w.FailedAttempts, "the failed attempt stays counted")
		assert.Equal(t, int64(1), w.Visits, "a retry is the same visit")
	})

	// The emails: four from A, one from B, sent through the queue to the SMTP server.
	listMessages := func(t *testing.T, automationID string) []map[string]interface{} {
		t.Helper()
		resp, err := client.Get("/api/messages.list", map[string]string{"workspace_id": workspace.ID, "automation_id": automationID, "limit": "100"})
		require.NoError(t, err)
		body := readBody(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)
		var result struct {
			Messages []map[string]interface{} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &result))
		return result.Messages
	}
	var messagesA []map[string]interface{}
	require.Eventually(t, func() bool {
		messagesA = listMessages(t, automationA)
		return len(messagesA) == 4
	}, 30*time.Second, 250*time.Millisecond, "the emails of A in the message history")

	t.Run("messages.list keeps the messages of one automation", func(t *testing.T) {
		for _, m := range messagesA {
			assert.Equal(t, automationA, m["automation_id"])
		}
		require.Eventually(t, func() bool { return len(listMessages(t, automationB)) == 1 }, 30*time.Second, 250*time.Millisecond)
	})

	t.Run("email counters joined to the message history agree with analytics", func(t *testing.T) {
		// One message of email-1 opened.
		var openedID string
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT e.output->>'message_id' FROM automation_node_executions e
			WHERE e.automation_id = $1 AND e.node_id = $2 AND e.action = 'completed' LIMIT 1`, automationA, email1).Scan(&openedID))
		// Written straight to the row: the workspace's tracking settings decide whether the
		// open path records it, and that is not what is under test here.
		_, err := db.ExecContext(ctx, `UPDATE message_history SET opened_at = NOW() WHERE id = $1`, openedID)
		require.NoError(t, err)

		stats := waitForStats(t, automationA, "the sends of A", func(s domain.NodeStatsResponse) bool {
			return s.Nodes[email1].Email != nil && s.Nodes[email1].Email.Sent == 3 && s.Nodes[email2].Email.Sent == 1
		})
		e1, e2 := stats.Nodes[email1].Email, stats.Nodes[email2].Email
		assert.Equal(t, int64(3), e1.Executions)
		assert.Equal(t, int64(1), e1.Opened)
		assert.Equal(t, int64(0), e1.Queued)
		assert.Equal(t, int64(0), e1.NotSent)
		assert.Equal(t, int64(0), e2.Opened)

		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema":   "message_history",
			"measures": []string{"count_sent", "count_opened"},
			"filters":  []map[string]interface{}{{"member": "automation_id", "operator": "equals", "values": []string{automationA}}},
		})
		require.Len(t, result.Data, 1)
		assert.Equal(t, float64(e1.Sent+e2.Sent), waNumber(t, result.Data[0]["count_sent"]))
		assert.Equal(t, float64(e1.Opened+e2.Opened), waNumber(t, result.Data[0]["count_opened"]))
	})

	t.Run("an email still in the queue is queued; a permanent failure and a cancelled send are not sent", func(t *testing.T) {
		var caID string
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT id FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`, automationA, fr1).Scan(&caID))
		// Three more email-1 executions of the same journey, each with a message ID the
		// message history does not have.
		for _, messageID := range []string{"queued-" + suffix, "permanent-" + suffix, "cancelled-" + suffix} {
			_, err := db.ExecContext(ctx, `
				INSERT INTO automation_node_executions (id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, completed_at, output)
				VALUES ($1, $2, $3, $4, 'email', 'completed', NOW(), NOW(), jsonb_build_object('message_id', $5::text))`,
				shortuuid.New(), caID, automationA, email1, messageID)
			require.NoError(t, err)
		}
		// A retry due tomorrow (out of the worker's reach today), entered twice, and a permanent
		// failure. The cancelled send's message ID is still queued, but by automation B: it
		// is not this automation's queue entry.
		insertQueue := func(sourceID, messageID string, nextRetry interface{}) {
			_, err := db.ExecContext(ctx, `
				INSERT INTO email_queue (id, status, source_type, source_id, integration_id, provider_kind, contact_email,
				                         message_id, template_id, payload, attempts, max_attempts, next_retry_at)
				VALUES ($1, 'failed', 'automation', $2, 'integration', 'smtp', $3, $4, $5, '{}'::jsonb, 1, 3, $6)`,
				shortuuid.New(), sourceID, fr1, messageID, template.ID, nextRetry)
			require.NoError(t, err)
		}
		tomorrow := time.Now().UTC().Add(24 * time.Hour)
		insertQueue(automationA, "queued-"+suffix, tomorrow)
		insertQueue(automationA, "queued-"+suffix, tomorrow)
		insertQueue(automationA, "permanent-"+suffix, nil)
		insertQueue(automationB, "cancelled-"+suffix, tomorrow)

		status, stats := nodeStats(t, automationA, nil)
		require.Equal(t, http.StatusOK, status)
		e1 := stats.Nodes[email1].Email
		assert.Equal(t, int64(6), e1.Executions, "a message queued twice is one execution")
		assert.Equal(t, int64(1), e1.Queued, "another automation's queue entry does not count")
		assert.Equal(t, int64(2), e1.NotSent)
		assert.Equal(t, int64(3), e1.Sent)
		assert.Equal(t, int64(6), stats.Nodes[email1].Visits, "each pass counts: three more email-1 runs")
	})

	t.Run("a period leaves out the executions entered before it", func(t *testing.T) {
		_, err := db.ExecContext(ctx,
			`UPDATE automation_node_executions SET entered_at = NOW() - INTERVAL '40 days' WHERE automation_id = $1 AND node_id = $2`,
			automationA, email2)
		require.NoError(t, err)

		now := time.Now().UTC()
		status, stats := nodeStats(t, automationA, map[string]string{
			"since": now.AddDate(0, 0, -30).Format(time.RFC3339),
			"until": now.Add(time.Hour).Format(time.RFC3339),
		})
		require.Equal(t, http.StatusOK, status)
		require.NotNil(t, stats.Since)
		assert.Equal(t, int64(0), stats.Nodes[email2].Visits)
		require.NotNil(t, stats.Nodes[email2].Email, "an email step keeps its counters, at zero")
		assert.Equal(t, domain.NodeEmailStats{}, *stats.Nodes[email2].Email)
		assert.Equal(t, int64(6), stats.Nodes[email1].Visits)

		status, stats = nodeStats(t, automationA, nil)
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, int64(1), stats.Nodes[email2].Visits, "all time still counts it")
	})

	t.Run("a member without message history read gets no email counters", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceMessageHistory] = domain.ResourcePermissions{}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		status, stats := nodeStats(t, automationA, nil)
		require.Equal(t, http.StatusOK, status)
		require.Len(t, stats.Nodes, 5)
		for id, n := range stats.Nodes {
			assert.Nil(t, n.Email, id)
		}
		assert.Equal(t, int64(6), stats.Nodes[email1].Visits)
	})

	t.Run("a member without automations read is refused", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceAutomations] = domain.ResourcePermissions{}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		status, _ := nodeStats(t, automationA, nil)
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("an unknown automation is a 404", func(t *testing.T) {
		status, _ := nodeStats(t, "missing-"+suffix, nil)
		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("an invalid period is a 400", func(t *testing.T) {
		status, _ := nodeStats(t, automationA, map[string]string{"since": "2026-11-01T00:00:00Z"})
		assert.Equal(t, http.StatusBadRequest, status)
		status, _ = nodeStats(t, automationA, map[string]string{"since": "2026-11-02T00:00:00Z", "until": "2026-11-01T00:00:00Z"})
		assert.Equal(t, http.StatusBadRequest, status)
	})

	// The passes, on automations of their own. statsAt reads a period, or all time when nil.
	statsAt := func(t *testing.T, automationID string, since, until *time.Time) domain.NodeStatsResponse {
		t.Helper()
		params := map[string]string{}
		if since != nil {
			params["since"], params["until"] = since.Format(time.RFC3339), until.Format(time.RFC3339)
		}
		status, stats := nodeStats(t, automationID, params)
		require.Equal(t, http.StatusOK, status)
		return stats
	}
	enrol := func(t *testing.T, eventName string) string {
		t.Helper()
		email := eventName + "-" + shortuuid.New() + "@example.com"
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, nil))
		return email
	}
	journeyOf := func(t *testing.T, automationID, email string) string {
		t.Helper()
		var id string
		require.Eventually(t, func() bool {
			return db.QueryRowContext(ctx, `SELECT id FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`,
				automationID, email).Scan(&id) == nil
		}, 30*time.Second, 100*time.Millisecond, "the journey of %s", email)
		return id
	}
	// Waits for the journey's k-th failure to be persisted (its retry count), then makes the
	// retry due now.
	retryNow := func(t *testing.T, caID string, k int) {
		t.Helper()
		require.Eventually(t, func() bool {
			var n int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT retry_count FROM contact_automations WHERE id = $1`, caID).Scan(&n))
			return n == k
		}, 30*time.Second, 100*time.Millisecond, "failure %d persisted", k)
		require.NoError(t, factory.UpdateContactAutomationScheduledAt(workspace.ID, caID, time.Now().UTC().Add(-time.Second)))
	}
	countRows := func(t *testing.T, query string, args ...interface{}) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&n))
		return n
	}

	t.Run("a loop counts each pass, and a pass not passed yet once", func(t *testing.T) {
		// L: trigger → W1 → W2 → back to W1. W2 answers 500 from its third call, and the retry
		// backoff (2 minutes) stops the loop there.
		var w1Calls, w2Calls int32
		loopReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/w2" && atomic.AddInt32(&w2Calls, 1) >= 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if r.URL.Path == "/w1" {
				atomic.AddInt32(&w1Calls, 1)
			}
			_, _ = w.Write([]byte(`{}`))
		}))
		defer loopReceiver.Close()
		automationL := shortuuid.New()
		triggerL, w1, w2 := shortuuid.New(), shortuuid.New(), shortuuid.New()
		createAndActivate(t, automationL, "node_stats_loop", triggerL, []map[string]interface{}{
			node(automationL, triggerL, "trigger", map[string]interface{}{}, w1),
			node(automationL, w1, "webhook", map[string]interface{}{"url": loopReceiver.URL + "/w1"}, w2),
			node(automationL, w2, "webhook", map[string]interface{}{"url": loopReceiver.URL + "/w2"}, w1),
		})
		enrol(t, "node_stats_loop")

		stats := waitForStats(t, automationL, "W2 retrying on its third pass", func(s domain.NodeStatsResponse) bool {
			return s.Nodes[w2] != nil && s.Nodes[w2].Retrying == 1 && s.Nodes[w2].FailedAttempts == 1
		})
		completedW2 := countRows(t, `SELECT COUNT(*) FROM automation_node_executions WHERE automation_id = $1 AND node_id = $2 AND action = 'completed'`, automationL, w2)
		assert.Equal(t, int64(2), completedW2)
		assert.Equal(t, completedW2+1, stats.Nodes[w2].Visits, "two passes done, the third one waiting for its retry")
		assert.Equal(t, int64(atomic.LoadInt32(&w1Calls)), stats.Nodes[w1].Visits, "one W1 pass per W1 call")
		assert.Equal(t, int64(3), stats.Nodes[w1].Visits)
		assert.Equal(t, int64(1), stats.Nodes[triggerL].Visits, "the trigger, once")
	})

	// C: trigger → a webhook that always fails.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	automationC := shortuuid.New()
	triggerC, webhookC := shortuuid.New(), shortuuid.New()
	var c1 string

	t.Run("retries exhausted: one pass, the journey failed there", func(t *testing.T) {
		createAndActivate(t, automationC, "node_stats_failing", triggerC, []map[string]interface{}{
			node(automationC, triggerC, "trigger", map[string]interface{}{}, webhookC),
			node(automationC, webhookC, "webhook", map[string]interface{}{"url": failing.URL}, ""),
		})
		c1 = journeyOf(t, automationC, enrol(t, "node_stats_failing"))
		retryNow(t, c1, 1)
		retryNow(t, c1, 2)
		stats := waitForStats(t, automationC, "the journey failed", func(s domain.NodeStatsResponse) bool {
			return s.Nodes[webhookC] != nil && s.Nodes[webhookC].FailedJourneys == 1
		})
		w := stats.Nodes[webhookC]
		assert.Equal(t, int64(1), w.Visits)
		assert.Equal(t, int64(3), w.FailedAttempts)
		assert.Equal(t, int64(0), w.Retrying)
		assert.Equal(t, int64(0), w.Completed)
	})

	t.Run("an exit while waiting for a retry keeps its pass, once", func(t *testing.T) {
		c2 := journeyOf(t, automationC, enrol(t, "node_stats_failing"))
		require.Eventually(t, func() bool {
			return countRows(t, `SELECT retry_count FROM contact_automations WHERE id = $1`, c2) == 1
		}, 30*time.Second, 100*time.Millisecond, "the first failure persisted")
		// As ExitContactJourneysOnReply writes it.
		_, err := db.ExecContext(ctx, `UPDATE contact_automations SET status = 'exited', scheduled_at = NULL, exit_reason = 'replied'
			WHERE id = $1 AND status = 'active'`, c2)
		require.NoError(t, err)

		w := statsAt(t, automationC, nil, nil).Nodes[webhookC]
		assert.Equal(t, int64(2), w.Visits, "the failed journey and the exited one")
		assert.Equal(t, int64(0), w.Retrying)
		assert.Equal(t, int64(4), w.FailedAttempts)
	})

	t.Run("a period dates a pass by its successful attempt, else by its last one", func(t *testing.T) {
		now := time.Now().UTC()
		last30, untilNow := now.AddDate(0, 0, -30), now.Add(time.Hour)
		before, beforeEnd := now.AddDate(0, 0, -50), now.AddDate(0, 0, -31)

		// B's webhook succeeded on its second attempt: that row 40 days back moves the pass out.
		_, err := db.ExecContext(ctx, `UPDATE automation_node_executions SET entered_at = entered_at - INTERVAL '40 days'
			WHERE automation_id = $1 AND node_id = $2 AND action = 'completed'`, automationB, webhook)
		require.NoError(t, err)
		w := statsAt(t, automationB, &last30, &untilNow).Nodes[webhook]
		assert.Equal(t, int64(0), w.Visits, "the pass is dated by its successful attempt")
		assert.Equal(t, int64(1), w.FailedAttempts, "its failed attempt stays in the period")
		assert.Equal(t, int64(1), statsAt(t, automationB, &before, &beforeEnd).Nodes[webhook].Visits)

		// c1 failed on C's webhook after three attempts: its first two, 40 days back, leave the
		// pass in the period of its last one.
		_, err = db.ExecContext(ctx, `UPDATE automation_node_executions SET entered_at = entered_at - INTERVAL '40 days'
			WHERE contact_automation_id = $1 AND node_id = $2 AND entered_at < (
				SELECT MAX(entered_at) FROM automation_node_executions WHERE contact_automation_id = $1 AND node_id = $2 AND completed_at IS NOT NULL)`,
			c1, webhookC)
		require.NoError(t, err)
		w = statsAt(t, automationC, &last30, &untilNow).Nodes[webhookC]
		assert.Equal(t, int64(2), w.Visits, "c1 by its last attempt, and the exited journey")
		assert.Equal(t, int64(2), w.FailedAttempts, "c1's last failure and the exited journey's")
		w = statsAt(t, automationC, &before, &beforeEnd).Nodes[webhookC]
		assert.Equal(t, int64(0), w.Visits)
		assert.Equal(t, int64(2), w.FailedAttempts, "c1's first two failures")
	})

	t.Run("a journey enrolled and not processed yet counts on the root", func(t *testing.T) {
		root := statsAt(t, automationA, nil, nil).Nodes[triggerA].Visits
		caID := shortuuid.New()
		// As the enrollment function writes it, due tomorrow so the scheduler leaves it alone.
		_, err := db.ExecContext(ctx, `INSERT INTO contact_automations (id, automation_id, contact_email, current_node_id, status, entered_at, scheduled_at)
			VALUES ($1, $2, $3, $4, 'active', NOW(), NOW() + INTERVAL '1 day')`, caID, automationA, "pending-"+suffix+"@example.com", triggerA)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO automation_node_executions (id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, output)
			VALUES ($1, $2, $3, $4, 'trigger', 'entered', NOW(), '{}'::jsonb)`, shortuuid.New(), caID, automationA, triggerA)
		require.NoError(t, err)
		assert.Equal(t, root+1, statsAt(t, automationA, nil, nil).Nodes[triggerA].Visits)
	})
}
