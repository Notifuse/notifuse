package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
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

// TestLogsFilters covers the filters of the Logs page: the Status filter of outgoing webhook
// deliveries, the Auth email and User created types of incoming webhooks, the provider of
// incoming webhooks, the origin and creation period of messages, and messages.count.
func TestLogsFilters(t *testing.T) {
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
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	ctx := context.Background()

	get := func(t *testing.T, endpoint string, params map[string]string) (int, []byte) {
		resp, err := client.Get(endpoint, params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, body
	}

	t.Run("deliveries filtered by status", func(t *testing.T) {
		newSubscription := func(t *testing.T) string {
			id := uuid.New().String()[:32]
			_, err := db.ExecContext(ctx,
				`INSERT INTO webhook_subscriptions (id, name, url, secret) VALUES ($1, $2, 'https://example.com/hook', 'secret')`,
				id, "sub "+id[:6])
			require.NoError(t, err)
			return id
		}
		first, second := newSubscription(t), newSubscription(t)

		// Deliveries in each status, on both subscriptions: first has 2 failed, second 1.
		seed := []struct {
			subscription string
			status       string
		}{
			{first, domain.WebhookDeliveryStatusFailed},
			{first, domain.WebhookDeliveryStatusFailed},
			{first, domain.WebhookDeliveryStatusDelivered},
			{first, domain.WebhookDeliveryStatusPending},
			{second, domain.WebhookDeliveryStatusFailed},
			{second, domain.WebhookDeliveryStatusDelivering},
		}
		for i, d := range seed {
			_, err := db.ExecContext(ctx,
				`INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, created_at)
				 VALUES ($1, $2, 'contact.created', '{}'::jsonb, $3, $4)`,
				uuid.New().String(), d.subscription, d.status, time.Now().Add(-time.Duration(i)*time.Minute))
			require.NoError(t, err)
		}

		type deliveriesResponse struct {
			Deliveries []domain.WebhookDelivery `json:"deliveries"`
			Total      int                      `json:"total"`
		}
		list := func(t *testing.T, params map[string]string) deliveriesResponse {
			params["workspace_id"] = workspace.ID
			status, body := get(t, "/api/webhookSubscriptions.deliveries", params)
			require.Equal(t, http.StatusOK, status, string(body))
			var r deliveriesResponse
			require.NoError(t, json.Unmarshal(body, &r))
			return r
		}

		failed := list(t, map[string]string{"status": "failed"})
		assert.Equal(t, 3, failed.Total)
		require.Len(t, failed.Deliveries, 3)
		for _, d := range failed.Deliveries {
			assert.Equal(t, domain.WebhookDeliveryStatusFailed, d.Status)
		}

		firstFailed := list(t, map[string]string{"status": "failed", "subscription_id": first})
		assert.Equal(t, 2, firstFailed.Total)
		require.Len(t, firstFailed.Deliveries, 2)
		for _, d := range firstFailed.Deliveries {
			assert.Equal(t, first, d.SubscriptionID)
		}

		// The total counts the filtered rows, not the page.
		paged := list(t, map[string]string{"status": "failed", "limit": "1"})
		assert.Equal(t, 3, paged.Total)
		assert.Len(t, paged.Deliveries, 1)

		all := list(t, map[string]string{})
		assert.Equal(t, len(seed), all.Total)

		status, body := get(t, "/api/webhookSubscriptions.deliveries", map[string]string{
			"workspace_id": workspace.ID, "status": "bogus",
		})
		assert.Equal(t, http.StatusBadRequest, status, string(body))
	})

	t.Run("incoming webhooks filtered by Auth email and User created", func(t *testing.T) {
		email := "logs-" + uuid.New().String()[:8] + "@example.com"
		for _, eventType := range []domain.EmailEventType{
			domain.EmailEventAuthEmail, domain.EmailEventBeforeUserCreated, domain.EmailEventDelivered,
		} {
			_, err := db.ExecContext(ctx,
				`INSERT INTO inbound_webhook_events (id, type, source, integration_id, recipient_email, timestamp, raw_payload, created_at)
				 VALUES ($1, $2, 'supabase', 'integration-1', $3, NOW(), '{}', NOW())`,
				uuid.New(), string(eventType), email)
			require.NoError(t, err)
		}

		for _, eventType := range []domain.EmailEventType{domain.EmailEventAuthEmail, domain.EmailEventBeforeUserCreated} {
			status, body := get(t, "/api/inboundWebhookEvents.list", map[string]string{
				"workspace_id":    workspace.ID,
				"event_type":      string(eventType),
				"recipient_email": email,
			})
			require.Equal(t, http.StatusOK, status, "list %s events: %s", eventType, body)
			var listed domain.InboundWebhookEventListResult
			require.NoError(t, json.Unmarshal(body, &listed))
			require.Len(t, listed.Events, 1, "only the %s event", eventType)
			assert.Equal(t, eventType, listed.Events[0].Type)
		}
	})

	t.Run("incoming webhooks filtered by provider", func(t *testing.T) {
		email := "provider-" + uuid.New().String()[:8] + "@example.com"
		for _, source := range []domain.WebhookSource{domain.WebhookSourcePostmark, domain.WebhookSourcePostmark, domain.WebhookSourceSES} {
			_, err := db.ExecContext(ctx,
				`INSERT INTO inbound_webhook_events (id, type, source, integration_id, recipient_email, timestamp, raw_payload, created_at)
				 VALUES ($1, 'delivered', $2, 'integration-1', $3, NOW(), '{}', NOW())`,
				uuid.New(), string(source), email)
			require.NoError(t, err)
		}

		status, body := get(t, "/api/inboundWebhookEvents.list", map[string]string{
			"workspace_id": workspace.ID, "source": "postmark", "recipient_email": email,
		})
		require.Equal(t, http.StatusOK, status, string(body))
		var listed domain.InboundWebhookEventListResult
		require.NoError(t, json.Unmarshal(body, &listed))
		require.Len(t, listed.Events, 2)
		for _, e := range listed.Events {
			assert.Equal(t, domain.WebhookSourcePostmark, e.Source)
		}

		status, body = get(t, "/api/inboundWebhookEvents.list", map[string]string{"workspace_id": workspace.ID, "source": "brevo"})
		assert.Equal(t, http.StatusBadRequest, status, string(body))
	})

	t.Run("messages by origin and creation period, and their counts", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		windowStart := now.Add(-time.Hour)
		broadcastID := "logs-" + uuid.New().String()[:8]
		automationID := uuid.New().String()
		notificationID := uuid.New().String()[:32]
		at := now.Add(-10 * time.Minute)

		create := func(t *testing.T, createdAt time.Time, opts ...testutil.MessageHistoryOption) string {
			opts = append(opts, func(m *domain.MessageHistory) {
				m.TemplateID = uuid.New().String()[:32]
				m.CreatedAt, m.SentAt, m.UpdatedAt = createdAt, createdAt, createdAt
			})
			m, err := factory.CreateMessageHistory(workspace.ID, opts...)
			require.NoError(t, err)
			return m.ID
		}
		fromBroadcast := func(m *domain.MessageHistory) { m.BroadcastID = &broadcastID }

		// Broadcast messages in the window, with a different number in each status:
		// message i is delivered when i < 6, opened when i < 5, and so on down to complained.
		statusAt := func(i, below int) *time.Time {
			if i < below {
				return &at
			}
			return nil
		}
		var broadcastIDs []string
		for i := 0; i < 7; i++ {
			i := i
			broadcastIDs = append(broadcastIDs, create(t, at, fromBroadcast, func(m *domain.MessageHistory) {
				m.DeliveredAt = statusAt(i, 6)
				m.OpenedAt = statusAt(i, 5)
				m.ClickedAt = statusAt(i, 4)
				m.BouncedAt = statusAt(i, 3)
				m.FailedAt = statusAt(i, 2)
				m.ComplainedAt = statusAt(i, 1)
			}))
		}
		// One broadcast message before the window.
		oldBroadcast := create(t, now.Add(-48*time.Hour), fromBroadcast)

		automationMessage := create(t, at, func(m *domain.MessageHistory) { m.AutomationID = &automationID })
		transactionalMessage := create(t, at, func(m *domain.MessageHistory) { m.TransactionalNotificationID = &notificationID })
		otherMessage := create(t, at)

		list := func(t *testing.T, params map[string]string) []string {
			t.Helper()
			params["workspace_id"] = workspace.ID
			params["limit"] = "100"
			status, body := get(t, "/api/messages.list", params)
			require.Equal(t, http.StatusOK, status, string(body))
			var result domain.MessageListResult
			require.NoError(t, json.Unmarshal(body, &result))
			ids := make([]string, 0, len(result.Messages))
			for _, m := range result.Messages {
				ids = append(ids, m.ID)
			}
			return ids
		}
		after := windowStart.Format(time.RFC3339)

		t.Run("each origin lists its own messages only", func(t *testing.T) {
			assert.ElementsMatch(t, broadcastIDs, list(t, map[string]string{"origin": "broadcast", "created_after": after}))
			assert.ElementsMatch(t, []string{automationMessage}, list(t, map[string]string{"origin": "automation", "created_after": after}))
			assert.ElementsMatch(t, []string{transactionalMessage}, list(t, map[string]string{"origin": "transactional", "created_after": after}))
			assert.ElementsMatch(t, []string{otherMessage}, list(t, map[string]string{"origin": "other", "created_after": after}))
		})

		t.Run("the creation period bounds the list", func(t *testing.T) {
			assert.ElementsMatch(t, append([]string{oldBroadcast}, broadcastIDs...), list(t, map[string]string{"broadcast_id": broadcastID}))
			assert.ElementsMatch(t, []string{oldBroadcast}, list(t, map[string]string{
				"broadcast_id": broadcastID, "created_before": windowStart.Format(time.RFC3339),
			}))
		})

		t.Run("messages.count gives each status, equal to the list it filters", func(t *testing.T) {
			status, body := get(t, "/api/messages.count", map[string]string{
				"workspace_id": workspace.ID, "origin": "broadcast", "created_after": after,
			})
			require.Equal(t, http.StatusOK, status, string(body))
			var counts domain.MessageCounts
			require.NoError(t, json.Unmarshal(body, &counts))
			assert.Equal(t, domain.MessageCounts{Total: 7, Delivered: 6, Opened: 5, Clicked: 4, Bounced: 3, Failed: 2, Complained: 1}, counts)

			for filter, count := range map[string]int64{
				"is_delivered": counts.Delivered, "is_opened": counts.Opened, "is_clicked": counts.Clicked,
				"is_bounced": counts.Bounced, "is_failed": counts.Failed, "is_complained": counts.Complained,
			} {
				listed := list(t, map[string]string{"origin": "broadcast", "created_after": after, filter: "true"})
				assert.Len(t, listed, int(count), filter)
			}
		})

		t.Run("messages.count with an indexed filter alone, refused without a bound", func(t *testing.T) {
			status, body := get(t, "/api/messages.count", map[string]string{"workspace_id": workspace.ID, "broadcast_id": broadcastID})
			require.Equal(t, http.StatusOK, status, string(body))
			var counts domain.MessageCounts
			require.NoError(t, json.Unmarshal(body, &counts))
			assert.Equal(t, int64(8), counts.Total, "the window's broadcast messages and the older one")

			status, body = get(t, "/api/messages.count", map[string]string{"workspace_id": workspace.ID, "automation_id": automationID})
			require.Equal(t, http.StatusOK, status, string(body))
			require.NoError(t, json.Unmarshal(body, &counts))
			assert.Equal(t, int64(1), counts.Total, "the automation's message")

			status, body = get(t, "/api/messages.count", map[string]string{"workspace_id": workspace.ID, "origin": "broadcast"})
			assert.Equal(t, http.StatusBadRequest, status, string(body))
			assert.Contains(t, string(body), "needs created_after")
		})
	})
}
