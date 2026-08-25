package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		ID:     "sub_123",
		Name:   "My Webhook",
		URL:    "https://example.com/webhook",
		Secret: "whsec_abc123def456",
		Settings: WebhookSubscriptionSettings{
			EventTypes: []string{"contact.created", "contact.updated"},
			CustomEventFilters: &CustomEventFilters{
				GoalTypes:  []string{"purchase", "subscription"},
				EventNames: []string{"orders/fulfilled"},
			},
		},
		Enabled:        true,
		LastDeliveryAt: &lastDelivery,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	assert.Equal(t, "sub_123", sub.ID)
	assert.Equal(t, "My Webhook", sub.Name)
	assert.Equal(t, "https://example.com/webhook", sub.URL)
	assert.Equal(t, "whsec_abc123def456", sub.Secret)
	assert.Equal(t, []string{"contact.created", "contact.updated"}, sub.Settings.EventTypes)
	assert.NotNil(t, sub.Settings.CustomEventFilters)
	assert.Equal(t, []string{"purchase", "subscription"}, sub.Settings.CustomEventFilters.GoalTypes)
	assert.Equal(t, []string{"orders/fulfilled"}, sub.Settings.CustomEventFilters.EventNames)
	assert.True(t, sub.Enabled)
	assert.NotNil(t, sub.LastDeliveryAt)
}

func TestWebhookSubscription_NilCustomEventFilters(t *testing.T) {
	sub := WebhookSubscription{
		ID:     "sub_123",
		Name:   "Simple Webhook",
		URL:    "https://example.com/webhook",
		Secret: "whsec_abc123",
		Settings: WebhookSubscriptionSettings{
			EventTypes:         []string{"contact.created"},
			CustomEventFilters: nil,
		},
		Enabled: true,
	}

	assert.Nil(t, sub.Settings.CustomEventFilters)
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
		ID:     "sub_456",
		Name:   "Test Webhook",
		URL:    "https://example.com/webhook",
		Secret: "whsec_test",
		Settings: WebhookSubscriptionSettings{
			EventTypes: []string{"contact.deleted"},
		},
		Enabled: true,
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

func TestValidateWebhookSubscriptionSource(t *testing.T) {
	// The column drives behaviour — the console badge, the deletion guard, the
	// delete-versus-disable branch on a dead endpoint — so it has to stay an
	// enum. An unrecognised value would read as "not user-created" everywhere
	// while matching none of the integration branches.
	accepted := []string{WebhookSubscriptionSourceUser, WebhookSubscriptionSourceZapier}
	for _, source := range accepted {
		t.Run("accepts "+strconv.Quote(source), func(t *testing.T) {
			assert.NoError(t, ValidateWebhookSubscriptionSource(source))
		})
	}

	rejected := []string{
		"evil",
		"Zapier",  // case matters: the delivery worker compares against the constant
		"zapier ", // whitespace is not trimmed for us anywhere upstream
		" ",
		"make",
		"user",
	}
	for _, source := range rejected {
		t.Run("rejects "+strconv.Quote(source), func(t *testing.T) {
			err := ValidateWebhookSubscriptionSource(source)
			require.Error(t, err)
			assert.Contains(t, err.Error(), source, "the error should name the offending value")
		})
	}
}

func TestErrWebhookSubscriptionNotFound_MatchesThroughWrapping(t *testing.T) {
	// The delivery worker moves a row to a terminal state on a genuine miss and
	// leaves it pending on anything else, so the sentinel has to survive being
	// wrapped by the repository.
	wrapped := fmt.Errorf("failed to get webhook subscription %q: %w", "sub_1", ErrWebhookSubscriptionNotFound)
	assert.True(t, errors.Is(wrapped, ErrWebhookSubscriptionNotFound))

	// The bug this sentinel replaces: a bare error carrying the same text is not
	// the sentinel, and treating it as one is how a transient database failure
	// would be mistaken for a deleted subscription.
	lookalike := errors.New("webhook subscription not found")
	assert.False(t, errors.Is(lookalike, ErrWebhookSubscriptionNotFound))

	// Neither is an unrelated failure, which must leave the delivery retryable.
	assert.False(t, errors.Is(fmt.Errorf("dial tcp: connection refused"), ErrWebhookSubscriptionNotFound))
}

func TestWebhookSubscriptionSettings_JSONRoundTrip(t *testing.T) {
	t.Run("with list and segment filters", func(t *testing.T) {
		original := WebhookSubscriptionSettings{
			EventTypes: []string{"list.subscribed", "segment.joined"},
			ListIDs:    []string{"list_a", "list_b"},
			SegmentIDs: []string{"seg_a"},
		}

		encoded, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded WebhookSubscriptionSettings
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.Equal(t, original, decoded)
	})

	t.Run("absent filters decode as nil, not as an empty filter", func(t *testing.T) {
		// This distinction is the whole contract: nil means "match every list",
		// while an empty non-nil slice would have to mean "match no list". A
		// subscription written before these fields existed decodes through this
		// path, so getting it wrong silences every one of them.
		var decoded WebhookSubscriptionSettings
		require.NoError(t, json.Unmarshal([]byte(`{"event_types":["contact.created"]}`), &decoded))

		assert.Equal(t, []string{"contact.created"}, decoded.EventTypes)
		assert.Nil(t, decoded.ListIDs)
		assert.Nil(t, decoded.SegmentIDs)
	})

	t.Run("empty filters are omitted on the way out", func(t *testing.T) {
		encoded, err := json.Marshal(WebhookSubscriptionSettings{EventTypes: []string{"contact.created"}})
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &raw))
		assert.NotContains(t, raw, "list_ids")
		assert.NotContains(t, raw, "segment_ids")
	})

	t.Run("filters coexist with custom event filters", func(t *testing.T) {
		original := WebhookSubscriptionSettings{
			EventTypes:         []string{"custom_event.created", "list.subscribed"},
			CustomEventFilters: &CustomEventFilters{GoalTypes: []string{"purchase"}},
			ListIDs:            []string{"list_a"},
		}

		encoded, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded WebhookSubscriptionSettings
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.Equal(t, original, decoded)
	})
}

func TestWebhookSubscription_MarshalJSON(t *testing.T) {
	// Settings are stored nested and served flattened. The flattening is the
	// published API shape, so every settings field the API exposes has to be
	// listed in MarshalJSON as well as in the struct.
	decode := func(t *testing.T, sub WebhookSubscription) map[string]json.RawMessage {
		t.Helper()
		encoded, err := json.Marshal(sub)
		require.NoError(t, err)
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &raw))
		return raw
	}

	t.Run("flattens every settings field alongside the nested settings object", func(t *testing.T) {
		raw := decode(t, WebhookSubscription{
			ID: "sub_1",
			Settings: WebhookSubscriptionSettings{
				EventTypes:         []string{"list.subscribed"},
				CustomEventFilters: &CustomEventFilters{EventNames: []string{"orders/fulfilled"}},
				ListIDs:            []string{"list_a"},
				SegmentIDs:         []string{"seg_a"},
			},
		})

		assert.JSONEq(t, `["list.subscribed"]`, string(raw["event_types"]))
		assert.JSONEq(t, `{"event_names":["orders/fulfilled"]}`, string(raw["custom_event_filters"]))
		assert.JSONEq(t, `["list_a"]`, string(raw["list_ids"]))
		assert.JSONEq(t, `["seg_a"]`, string(raw["segment_ids"]))

		// The nested object stays, since existing clients read it.
		require.Contains(t, raw, "settings")
		var nested WebhookSubscriptionSettings
		require.NoError(t, json.Unmarshal(raw["settings"], &nested))
		assert.Equal(t, []string{"list_a"}, nested.ListIDs)
		assert.Equal(t, []string{"seg_a"}, nested.SegmentIDs)
	})

	t.Run("omits the filters a subscription does not set", func(t *testing.T) {
		raw := decode(t, WebhookSubscription{
			ID:       "sub_1",
			Settings: WebhookSubscriptionSettings{EventTypes: []string{"contact.created"}},
		})

		assert.NotContains(t, raw, "list_ids")
		assert.NotContains(t, raw, "segment_ids")
		assert.NotContains(t, raw, "custom_event_filters")
		assert.Contains(t, raw, "event_types")
	})

	t.Run("omits source for a user-created subscription", func(t *testing.T) {
		raw := decode(t, WebhookSubscription{ID: "sub_1", Source: WebhookSubscriptionSourceUser})
		assert.NotContains(t, raw, "source")
	})

	t.Run("surfaces source for an integration-created subscription", func(t *testing.T) {
		raw := decode(t, WebhookSubscription{ID: "sub_1", Source: WebhookSubscriptionSourceZapier})
		assert.JSONEq(t, `"zapier"`, string(raw["source"]))
	})

	t.Run("always reports the failure counter, including zero", func(t *testing.T) {
		// Without this a healthy subscription would serve no counter at all and
		// a console rendering it could not tell zero failures from an older
		// server that does not know the field.
		raw := decode(t, WebhookSubscription{ID: "sub_1", ConsecutiveFailures: 0})
		require.Contains(t, raw, "consecutive_failures")
		assert.JSONEq(t, `0`, string(raw["consecutive_failures"]))

		raw = decode(t, WebhookSubscription{ID: "sub_1", ConsecutiveFailures: 7})
		assert.JSONEq(t, `7`, string(raw["consecutive_failures"]))
	})

	t.Run("reports disabled_reason only when one was recorded", func(t *testing.T) {
		raw := decode(t, WebhookSubscription{ID: "sub_1"})
		assert.NotContains(t, raw, "disabled_reason")

		reason := "endpoint returned 410 Gone"
		raw = decode(t, WebhookSubscription{ID: "sub_1", DisabledReason: &reason})
		assert.JSONEq(t, `"endpoint returned 410 Gone"`, string(raw["disabled_reason"]))
	})
}

func TestWebhookDelivery_ClaimedAt(t *testing.T) {
	t.Run("omitted while the row is unclaimed", func(t *testing.T) {
		encoded, err := json.Marshal(WebhookDelivery{ID: "del_1", Status: WebhookDeliveryStatusPending})
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &raw))
		assert.NotContains(t, raw, "claimed_at")
	})

	t.Run("round-trips once a worker has claimed the row", func(t *testing.T) {
		claimed := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
		encoded, err := json.Marshal(WebhookDelivery{
			ID:        "del_1",
			Status:    WebhookDeliveryStatusDelivering,
			ClaimedAt: &claimed,
		})
		require.NoError(t, err)

		var decoded WebhookDelivery
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.NotNil(t, decoded.ClaimedAt)
		assert.True(t, claimed.Equal(*decoded.ClaimedAt))
	})
}
