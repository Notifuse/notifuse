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

// TestLogsFilters covers two filters of the Logs page that the server ignored or refused:
// the Status filter of outgoing webhook deliveries, and the Auth email and User created
// types of incoming webhooks.
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
}
