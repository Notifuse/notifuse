package domain

//go:generate mockgen -destination mocks/mock_webhook_subscription_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WebhookSubscriptionRepository
//go:generate mockgen -destination mocks/mock_webhook_delivery_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WebhookDeliveryRepository

import (
	"context"
	"time"
)

// WebhookSubscription represents an outgoing webhook subscription configuration
type WebhookSubscription struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	URL                string              `json:"url"`
	Secret             string              `json:"secret"`
	EventTypes         []string            `json:"event_types"`
	CustomEventFilters *CustomEventFilters `json:"custom_event_filters,omitempty"`
	Enabled            bool                `json:"enabled"`
	Description        string              `json:"description,omitempty"`
	LastDeliveryAt     *time.Time          `json:"last_delivery_at,omitempty"`
	SuccessCount       int64               `json:"success_count"`
	FailureCount       int64               `json:"failure_count"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

// CustomEventFilters defines filters for custom event subscriptions
type CustomEventFilters struct {
	GoalTypes  []string `json:"goal_types,omitempty"`  // Filter by goal_type enum
	EventNames []string `json:"event_names,omitempty"` // Filter by event_name
}

// WebhookDelivery represents a pending or completed webhook delivery
type WebhookDelivery struct {
	ID                 string                 `json:"id"`
	SubscriptionID     string                 `json:"subscription_id"`
	EventType          string                 `json:"event_type"`
	Payload            map[string]interface{} `json:"payload"`
	Status             string                 `json:"status"` // pending, delivering, delivered, failed
	Attempts           int                    `json:"attempts"`
	MaxAttempts        int                    `json:"max_attempts"`
	NextAttemptAt      time.Time              `json:"next_attempt_at"`
	LastAttemptAt      *time.Time             `json:"last_attempt_at,omitempty"`
	DeliveredAt        *time.Time             `json:"delivered_at,omitempty"`
	LastResponseStatus *int                   `json:"last_response_status,omitempty"`
	LastResponseBody   *string                `json:"last_response_body,omitempty"`
	LastError          *string                `json:"last_error,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
}

// WebhookDeliveryStatus constants
const (
	WebhookDeliveryStatusPending    = "pending"
	WebhookDeliveryStatusDelivering = "delivering"
	WebhookDeliveryStatusDelivered  = "delivered"
	WebhookDeliveryStatusFailed     = "failed"
)

// Available webhook event types
var WebhookEventTypes = []string{
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
	// Custom events (with optional filtering)
	"custom_event.created",
	"custom_event.updated",
	"custom_event.deleted",
}

// WebhookSubscriptionRepository defines the interface for webhook subscription data access
type WebhookSubscriptionRepository interface {
	Create(ctx context.Context, workspaceID string, sub *WebhookSubscription) error
	GetByID(ctx context.Context, workspaceID, id string) (*WebhookSubscription, error)
	List(ctx context.Context, workspaceID string) ([]*WebhookSubscription, error)
	Update(ctx context.Context, workspaceID string, sub *WebhookSubscription) error
	Delete(ctx context.Context, workspaceID, id string) error
	IncrementStats(ctx context.Context, workspaceID, id string, success bool) error
	UpdateLastDeliveryAt(ctx context.Context, workspaceID, id string, deliveredAt time.Time) error
}

// WebhookDeliveryRepository defines the interface for webhook delivery data access
type WebhookDeliveryRepository interface {
	GetPendingForWorkspace(ctx context.Context, workspaceID string, limit int) ([]*WebhookDelivery, error)
	ListBySubscription(ctx context.Context, workspaceID, subscriptionID string, limit, offset int) ([]*WebhookDelivery, int, error)
	UpdateStatus(ctx context.Context, workspaceID, id string, status string, attempts int, responseStatus *int, responseBody, lastError *string) error
	MarkDelivered(ctx context.Context, workspaceID, id string, responseStatus int, responseBody string) error
	ScheduleRetry(ctx context.Context, workspaceID, id string, nextAttempt time.Time, attempts int, responseStatus *int, responseBody, lastError *string) error
	MarkFailed(ctx context.Context, workspaceID, id string, attempts int, lastError string, responseStatus *int, responseBody *string) error
	Create(ctx context.Context, workspaceID string, delivery *WebhookDelivery) error
}

// WebhookDeliveryWithSubscription contains a delivery with its associated subscription
type WebhookDeliveryWithSubscription struct {
	Delivery     *WebhookDelivery
	Subscription *WebhookSubscription
	WorkspaceID  string
}
