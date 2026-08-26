package domain

//go:generate mockgen -destination mocks/mock_webhook_subscription_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WebhookSubscriptionRepository
//go:generate mockgen -destination mocks/mock_webhook_delivery_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WebhookDeliveryRepository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrWebhookSubscriptionNotFound reports that a subscription genuinely does not
// exist, as opposed to the lookup having failed. The delivery worker branches on
// exactly that distinction: a missing subscription means the delivery can never
// succeed and its row must be moved to a terminal state, while a pool exhaustion,
// a network timeout or a Postgres restart must leave the row pending so it is
// retried. Repositories therefore have to wrap this sentinel rather than report
// the miss as a bare string, or errors.Is can never tell the two apart and a
// five-second database blip would permanently kill every in-flight delivery.
var ErrWebhookSubscriptionNotFound = errors.New("webhook subscription not found")

// Recognised values for WebhookSubscription.Source. The empty string is the
// user-created case and covers every row written before the column existed;
// anything else identifies the integration that created the subscription on the
// user's behalf, so the console can label it and the delivery worker can decide
// whether a dead endpoint should be deleted or merely disabled.
const (
	WebhookSubscriptionSourceUser   = ""
	WebhookSubscriptionSourceZapier = "zapier"
)

// ValidateWebhookSubscriptionSource rejects any source outside the known set.
// The column exists to drive behaviour — the console badge, the deletion guard,
// the delete-versus-disable branch on a dead endpoint — so an unrecognised value
// is worse than none at all: it silently reads as "not user-created" everywhere
// without matching any of the integration branches. Validating on the way in is
// what keeps it an enum instead of a free-text field.
func ValidateWebhookSubscriptionSource(source string) error {
	switch source {
	case WebhookSubscriptionSourceUser, WebhookSubscriptionSourceZapier:
		return nil
	default:
		return fmt.Errorf("invalid webhook subscription source: %q", source)
	}
}

// WebhookSubscriptionSettings contains event subscription configuration
type WebhookSubscriptionSettings struct {
	EventTypes         []string            `json:"event_types"`
	CustomEventFilters *CustomEventFilters `json:"custom_event_filters,omitempty"`
	// ListIDs and SegmentIDs narrow the fan-out for the list.* and segment.*
	// event types. Absent or empty means "no filter" — every list, every segment
	// — which is the behaviour every subscription written before these fields
	// existed already has, so upgrading changes nothing for them. A populated
	// array matches only those ids, which is what stops a subscription watching
	// one list from receiving a delivery row per contact for every other list in
	// the workspace.
	ListIDs    []string `json:"list_ids,omitempty"`
	SegmentIDs []string `json:"segment_ids,omitempty"`
}

// WebhookSubscription represents an outgoing webhook subscription configuration
type WebhookSubscription struct {
	ID       string                      `json:"id"`
	Name     string                      `json:"name"`
	URL      string                      `json:"url"`
	Secret   string                      `json:"secret"`
	Settings WebhookSubscriptionSettings `json:"settings"`
	Enabled  bool                        `json:"enabled"`
	// Source attributes the subscription to whatever created it. Empty means a
	// user created it by hand; see the WebhookSubscriptionSource constants.
	Source string `json:"source,omitempty"`
	// ConsecutiveFailures counts delivery attempts that have failed back to back,
	// reset to zero by the first success. It is the only garbage collector for a
	// dead endpoint that does not depend on that endpoint telling us it is dead.
	ConsecutiveFailures int `json:"consecutive_failures"`
	// FailingSince is when the current run of failures started, set by the first
	// failure after a success and cleared by the next success.
	//
	// It exists because a count of failures is not a measure of time. Deliveries
	// are processed in batches of a hundred, so an endpoint that is down for
	// thirty seconds during an import can fail twenty of them inside one ten-
	// second poll — which would retire a perfectly healthy integration on the
	// strength of one restart. The threshold is only allowed to act once this
	// says the endpoint has been failing for longer than a delivery's own retry
	// ladder can run.
	FailingSince *time.Time `json:"failing_since,omitempty"`
	// DisabledReason records why the subscription was disabled automatically, so
	// a user who finds a switched-off webhook can tell it from one they switched
	// off themselves. Nil for a subscription that was never auto-disabled.
	DisabledReason *string    `json:"disabled_reason,omitempty"`
	LastDeliveryAt *time.Time `json:"last_delivery_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// MarshalJSON implements custom JSON marshaling to flatten settings into top-level fields
// This maintains backward-compatible API responses while using nested internal structure
func (w WebhookSubscription) MarshalJSON() ([]byte, error) {
	type Alias WebhookSubscription
	return json.Marshal(&struct {
		Alias
		EventTypes         []string            `json:"event_types"`
		CustomEventFilters *CustomEventFilters `json:"custom_event_filters,omitempty"`
		ListIDs            []string            `json:"list_ids,omitempty"`
		SegmentIDs         []string            `json:"segment_ids,omitempty"`
	}{
		Alias:              Alias(w),
		EventTypes:         w.Settings.EventTypes,
		CustomEventFilters: w.Settings.CustomEventFilters,
		ListIDs:            w.Settings.ListIDs,
		SegmentIDs:         w.Settings.SegmentIDs,
	})
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
	// ClaimedAt is when a worker took the row, and doubles as the lease clock: a
	// row still in 'delivering' well past the HTTP timeout belongs to a worker
	// that died mid-flight, and the reclaim sweep uses this to return it to
	// 'pending' rather than leave it stranded forever.
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
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
	// GetByID returns an error wrapping ErrWebhookSubscriptionNotFound, and no
	// other error type, when the row does not exist. Callers branch on that
	// sentinel to separate a genuinely deleted subscription from a database
	// that is briefly unavailable, so reporting a miss any other way silently
	// turns transient failures into permanent ones.
	GetByID(ctx context.Context, workspaceID, id string) (*WebhookSubscription, error)
	List(ctx context.Context, workspaceID string) ([]*WebhookSubscription, error)
	Update(ctx context.Context, workspaceID string, sub *WebhookSubscription) error
	Delete(ctx context.Context, workspaceID, id string) error
	UpdateLastDeliveryAt(ctx context.Context, workspaceID, id string, deliveredAt time.Time) error
	// IncrementFailures bumps the consecutive-failure counter by one. It is
	// deliberately a read-modify-write done in SQL rather than in Go: several
	// deliveries for one subscription can fail concurrently, and a counter
	// computed from a value the worker read earlier would lose those increments.
	IncrementFailures(ctx context.Context, workspaceID, id string) error
	// ResetFailures returns the counter to zero after a successful delivery.
	ResetFailures(ctx context.Context, workspaceID, id string) error
	// DisableWithReason switches the subscription off and records why, so an
	// automatic disable is distinguishable from a user switching it off.
	DisableWithReason(ctx context.Context, workspaceID, id, reason string) error
}

// WebhookDeliveryRepository defines the interface for webhook delivery data access
type WebhookDeliveryRepository interface {
	// GetPendingForWorkspace atomically CLAIMS the rows it returns: every row in
	// the result has already been moved to WebhookDeliveryStatusDelivering with
	// claimed_at stamped, in the same statement that selected it. It is not a
	// read. Two workers polling the same workspace therefore receive disjoint
	// sets, which is the only thing standing between a multi-replica deployment
	// and delivering every event twice — and a duplicate delivery is a duplicate
	// side effect in whatever system consumes the webhook.
	//
	// Because the claim is a durable status change rather than a held lock, the
	// caller owns every returned row until it writes a terminal state back. A
	// caller that returns early without writing strands the row until the
	// reclaim sweep picks it up; see ReclaimStale.
	GetPendingForWorkspace(ctx context.Context, workspaceID string, limit int) ([]*WebhookDelivery, error)
	// RenewClaim re-stamps one claimed row's lease and reports whether the caller
	// still owns it. It takes the claimed_at the caller was handed and matches on
	// it, so "someone else has this row now" is answered false rather than
	// silently stolen back.
	//
	// It exists because the claim is stamped once for a whole batch. A batch of a
	// hundred rows walked serially can outlive the lease many times over — the
	// binding constraint is lease > batchSize x httpTimeout, not lease >
	// httpTimeout — so the reclaim sweep would hand later rows to a second worker
	// while the first is still holding them, manufacturing the duplicate delivery
	// the claim exists to prevent. Renewing per row keeps the lease short (fast
	// recovery from a crash) while making it mean what it says.
	//
	// The returned time is the new claimed_at, which the caller must carry into
	// its next renewal of the same row.
	RenewClaim(ctx context.Context, workspaceID, id string, claimedAt *time.Time) (bool, *time.Time, error)
	ListAll(ctx context.Context, workspaceID string, subscriptionID *string, limit, offset int) ([]*WebhookDelivery, int, error)
	UpdateStatus(ctx context.Context, workspaceID, id string, status string, attempts int, responseStatus *int, responseBody, lastError *string) error
	MarkDelivered(ctx context.Context, workspaceID, id string, responseStatus int, responseBody string) error
	ScheduleRetry(ctx context.Context, workspaceID, id string, nextAttempt time.Time, attempts int, responseStatus *int, responseBody, lastError *string) error
	MarkFailed(ctx context.Context, workspaceID, id string, attempts int, lastError string, responseStatus *int, responseBody *string) error
	Create(ctx context.Context, workspaceID string, delivery *WebhookDelivery) error
	CleanupOldDeliveries(ctx context.Context, workspaceID string, retentionDays int) (int64, error)
	// DeleteBySubscriptionID removes every delivery belonging to one
	// subscription. Deleting a subscription without this leaves its queued
	// deliveries matching the pending predicate forever, each one occupying a
	// slot in every batch for the whole retention window.
	DeleteBySubscriptionID(ctx context.Context, workspaceID, subscriptionID string) error
	// ReclaimStale returns rows claimed longer than lease ago to
	// WebhookDeliveryStatusPending and reports how many it moved. A worker that
	// crashes mid-delivery leaves its claims behind, and without this they are
	// stranded exactly like the orphans DeleteBySubscriptionID exists to
	// prevent. The lease belongs just past the HTTP timeout: the request context
	// is already cancelled by then, so a longer one only delays recovery, and
	// one measured in minutes would quietly override the first rungs of the
	// retry ladder.
	ReclaimStale(ctx context.Context, workspaceID string, lease time.Duration) (int64, error)
	// ReleaseClaim hands a claimed row back to WebhookDeliveryStatusPending
	// without recording an attempt, for the case where nothing was sent.
	//
	// It exists so the release does not have to go through UpdateStatus, which
	// writes last_attempt_at, last_response_status and last_response_body on
	// every call: releasing through it stamped an attempt that never happened and
	// erased the status and body of the last one that did, leaving the delivery
	// log describing our database instead of the user's endpoint.
	//
	// It takes the same claim token RenewClaim does, and for the same reason: a
	// release is only ever a release of OUR claim. Matching on the id alone let
	// it write to a row this caller no longer held — a panic after the delivery
	// had been marked delivered pushed it back to pending to be sent again, and a
	// caller that stalled past its lease could yank a row back from the worker
	// the sweep had legitimately given it to. A nil claimedAt is not a claim and
	// releases nothing; the row is left for ReclaimStale, which treats a
	// claimless 'delivering' row as infinitely stale.
	ReleaseClaim(ctx context.Context, workspaceID, id string, claimedAt *time.Time, lastError string) error
}

// WebhookDeliveryWithSubscription contains a delivery with its associated subscription
type WebhookDeliveryWithSubscription struct {
	Delivery     *WebhookDelivery
	Subscription *WebhookSubscription
	WorkspaceID  string
}
