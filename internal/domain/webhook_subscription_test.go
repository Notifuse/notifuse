package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWebhookEventTypes_ContainsExpectedEvents(t *testing.T) {
	// Verify all expected event types are present
	expectedEvents := []string{
		// Contact events
		"contact.created",
		"contact.updated",
		"contact.deleted",
		// List events
		"list.subscribed",
		"list.unsubscribed",
		"list.confirmed",
		"list.resubscribed",
		"list.bounced",
		"list.complained",
		"list.pending",
		"list.removed",
		// Segment events
		"segment.joined",
		"segment.left",
		// Email events
		"email.sent",
		"email.delivered",
		"email.opened",
		"email.clicked",
		"email.bounced",
		"email.complained",
		"email.unsubscribed",
		// Custom events
		"custom_event.created",
		"custom_event.updated",
		"custom_event.deleted",
	}

	for _, expected := range expectedEvents {
		found := false
		for _, actual := range WebhookEventTypes {
			if actual == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected event type %q to be in WebhookEventTypes", expected)
	}

	// Verify count matches
	assert.Equal(t, len(expectedEvents), len(WebhookEventTypes), "WebhookEventTypes should have exactly %d event types", len(expectedEvents))
}

func TestWebhookDeliveryStatus_Constants(t *testing.T) {
	// Verify status constants are correctly defined
	assert.Equal(t, "pending", WebhookDeliveryStatusPending)
	assert.Equal(t, "delivering", WebhookDeliveryStatusDelivering)
	assert.Equal(t, "delivered", WebhookDeliveryStatusDelivered)
	assert.Equal(t, "failed", WebhookDeliveryStatusFailed)
}

func TestWebhookSubscription_Fields(t *testing.T) {
	now := time.Now()
	lastDelivery := now.Add(-1 * time.Hour)

	sub := WebhookSubscription{
		ID:          "sub_123",
		Name:        "My Webhook",
		URL:         "https://example.com/webhook",
		Secret:      "whsec_abc123def456",
		EventTypes:  []string{"contact.created", "contact.updated"},
		CustomEventFilters: &CustomEventFilters{
			GoalTypes:  []string{"purchase", "subscription"},
			EventNames: []string{"orders/fulfilled"},
		},
		Enabled:        true,
		Description:    "Test webhook subscription",
		LastDeliveryAt: &lastDelivery,
		SuccessCount:   100,
		FailureCount:   5,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	assert.Equal(t, "sub_123", sub.ID)
	assert.Equal(t, "My Webhook", sub.Name)
	assert.Equal(t, "https://example.com/webhook", sub.URL)
	assert.Equal(t, "whsec_abc123def456", sub.Secret)
	assert.Equal(t, []string{"contact.created", "contact.updated"}, sub.EventTypes)
	assert.NotNil(t, sub.CustomEventFilters)
	assert.Equal(t, []string{"purchase", "subscription"}, sub.CustomEventFilters.GoalTypes)
	assert.Equal(t, []string{"orders/fulfilled"}, sub.CustomEventFilters.EventNames)
	assert.True(t, sub.Enabled)
	assert.Equal(t, "Test webhook subscription", sub.Description)
	assert.NotNil(t, sub.LastDeliveryAt)
	assert.Equal(t, int64(100), sub.SuccessCount)
	assert.Equal(t, int64(5), sub.FailureCount)
}

func TestWebhookSubscription_NilCustomEventFilters(t *testing.T) {
	sub := WebhookSubscription{
		ID:                 "sub_123",
		Name:               "Simple Webhook",
		URL:                "https://example.com/webhook",
		Secret:             "whsec_abc123",
		EventTypes:         []string{"contact.created"},
		CustomEventFilters: nil,
		Enabled:            true,
	}

	assert.Nil(t, sub.CustomEventFilters)
}

func TestCustomEventFilters_Fields(t *testing.T) {
	t.Run("with both filters", func(t *testing.T) {
		filters := CustomEventFilters{
			GoalTypes:  []string{"purchase", "subscription", "lead"},
			EventNames: []string{"orders/fulfilled", "payment.succeeded"},
		}

		assert.Equal(t, 3, len(filters.GoalTypes))
		assert.Equal(t, 2, len(filters.EventNames))
		assert.Contains(t, filters.GoalTypes, "purchase")
		assert.Contains(t, filters.EventNames, "orders/fulfilled")
	})

	t.Run("with only goal_types", func(t *testing.T) {
		filters := CustomEventFilters{
			GoalTypes: []string{"purchase"},
		}

		assert.Equal(t, 1, len(filters.GoalTypes))
		assert.Nil(t, filters.EventNames)
	})

	t.Run("with only event_names", func(t *testing.T) {
		filters := CustomEventFilters{
			EventNames: []string{"custom.event"},
		}

		assert.Nil(t, filters.GoalTypes)
		assert.Equal(t, 1, len(filters.EventNames))
	})

	t.Run("empty filters", func(t *testing.T) {
		filters := CustomEventFilters{}

		assert.Nil(t, filters.GoalTypes)
		assert.Nil(t, filters.EventNames)
	})
}

func TestWebhookDelivery_Fields(t *testing.T) {
	now := time.Now()
	lastAttempt := now.Add(-5 * time.Minute)
	delivered := now.Add(-1 * time.Minute)
	responseStatus := 200
	responseBody := `{"ok": true}`

	delivery := WebhookDelivery{
		ID:                 "del_123",
		SubscriptionID:     "sub_456",
		EventType:          "contact.created",
		Payload:            map[string]interface{}{"contact": map[string]interface{}{"email": "test@example.com"}},
		Status:             WebhookDeliveryStatusDelivered,
		Attempts:           1,
		MaxAttempts:        10,
		NextAttemptAt:      now,
		LastAttemptAt:      &lastAttempt,
		DeliveredAt:        &delivered,
		LastResponseStatus: &responseStatus,
		LastResponseBody:   &responseBody,
		LastError:          nil,
		CreatedAt:          now,
	}

	assert.Equal(t, "del_123", delivery.ID)
	assert.Equal(t, "sub_456", delivery.SubscriptionID)
	assert.Equal(t, "contact.created", delivery.EventType)
	assert.NotNil(t, delivery.Payload)
	assert.Equal(t, WebhookDeliveryStatusDelivered, delivery.Status)
	assert.Equal(t, 1, delivery.Attempts)
	assert.Equal(t, 10, delivery.MaxAttempts)
	assert.NotNil(t, delivery.LastAttemptAt)
	assert.NotNil(t, delivery.DeliveredAt)
	assert.Equal(t, 200, *delivery.LastResponseStatus)
	assert.Equal(t, `{"ok": true}`, *delivery.LastResponseBody)
	assert.Nil(t, delivery.LastError)
}

func TestWebhookDelivery_FailedDelivery(t *testing.T) {
	now := time.Now()
	lastAttempt := now.Add(-5 * time.Minute)
	nextAttempt := now.Add(30 * time.Second)
	responseStatus := 500
	responseBody := `{"error": "internal server error"}`
	lastError := "HTTP 500: Internal Server Error"

	delivery := WebhookDelivery{
		ID:                 "del_789",
		SubscriptionID:     "sub_456",
		EventType:          "contact.updated",
		Payload:            map[string]interface{}{"contact": map[string]interface{}{"email": "test@example.com"}},
		Status:             WebhookDeliveryStatusFailed,
		Attempts:           3,
		MaxAttempts:        10,
		NextAttemptAt:      nextAttempt,
		LastAttemptAt:      &lastAttempt,
		DeliveredAt:        nil,
		LastResponseStatus: &responseStatus,
		LastResponseBody:   &responseBody,
		LastError:          &lastError,
		CreatedAt:          now,
	}

	assert.Equal(t, WebhookDeliveryStatusFailed, delivery.Status)
	assert.Equal(t, 3, delivery.Attempts)
	assert.Nil(t, delivery.DeliveredAt)
	assert.NotNil(t, delivery.LastError)
	assert.Equal(t, "HTTP 500: Internal Server Error", *delivery.LastError)
	assert.Equal(t, 500, *delivery.LastResponseStatus)
}

func TestWebhookDelivery_PendingDelivery(t *testing.T) {
	now := time.Now()

	delivery := WebhookDelivery{
		ID:             "del_001",
		SubscriptionID: "sub_456",
		EventType:      "custom_event.created",
		Payload: map[string]interface{}{
			"custom_event": map[string]interface{}{
				"external_id": "ext_123",
				"email":       "user@example.com",
				"event_name":  "orders/fulfilled",
			},
		},
		Status:        WebhookDeliveryStatusPending,
		Attempts:      0,
		MaxAttempts:   10,
		NextAttemptAt: now,
		CreatedAt:     now,
	}

	assert.Equal(t, WebhookDeliveryStatusPending, delivery.Status)
	assert.Equal(t, 0, delivery.Attempts)
	assert.Nil(t, delivery.LastAttemptAt)
	assert.Nil(t, delivery.DeliveredAt)
	assert.Nil(t, delivery.LastResponseStatus)
	assert.Nil(t, delivery.LastResponseBody)
	assert.Nil(t, delivery.LastError)
}

func TestWebhookDeliveryWithSubscription_Fields(t *testing.T) {
	now := time.Now()

	delivery := &WebhookDelivery{
		ID:             "del_123",
		SubscriptionID: "sub_456",
		EventType:      "contact.deleted",
		Status:         WebhookDeliveryStatusPending,
		CreatedAt:      now,
	}

	subscription := &WebhookSubscription{
		ID:         "sub_456",
		Name:       "Test Webhook",
		URL:        "https://example.com/webhook",
		Secret:     "whsec_test",
		EventTypes: []string{"contact.deleted"},
		Enabled:    true,
	}

	withSub := WebhookDeliveryWithSubscription{
		Delivery:     delivery,
		Subscription: subscription,
		WorkspaceID:  "workspace_789",
	}

	assert.Equal(t, delivery, withSub.Delivery)
	assert.Equal(t, subscription, withSub.Subscription)
	assert.Equal(t, "workspace_789", withSub.WorkspaceID)
	assert.Equal(t, "contact.deleted", withSub.Delivery.EventType)
	assert.Equal(t, "Test Webhook", withSub.Subscription.Name)
}

func TestWebhookEventTypes_DeletedEventsPresent(t *testing.T) {
	// Specifically test that .deleted events are present
	deletedEvents := []string{
		"contact.deleted",
		"custom_event.deleted",
	}

	for _, expected := range deletedEvents {
		found := false
		for _, actual := range WebhookEventTypes {
			if actual == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected deleted event type %q to be in WebhookEventTypes", expected)
	}
}

func TestWebhookEventTypes_GroupedByCategory(t *testing.T) {
	// Count events by category prefix
	categories := make(map[string]int)
	for _, eventType := range WebhookEventTypes {
		// Extract prefix before the dot
		for i, c := range eventType {
			if c == '.' {
				prefix := eventType[:i]
				categories[prefix]++
				break
			}
		}
	}

	// Verify expected categories
	assert.Equal(t, 3, categories["contact"], "Should have 3 contact events")
	assert.Equal(t, 8, categories["list"], "Should have 8 list events")
	assert.Equal(t, 2, categories["segment"], "Should have 2 segment events")
	assert.Equal(t, 7, categories["email"], "Should have 7 email events")
	assert.Equal(t, 3, categories["custom_event"], "Should have 3 custom_event events")
}
