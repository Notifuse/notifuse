package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	// webhookClaimLease is how long a claimed delivery may sit in 'delivering'
	// before the reclaim sweep decides the worker holding it died.
	//
	// Seconds, not minutes, and that is the whole point. The lease is the
	// production HTTP timeout (10s, where the worker's client is built in
	// internal/app/app.go) plus a small buffer: past it the request context is
	// already cancelled, so a longer lease only delays recovery — and a lease of
	// minutes would silently override the first rungs of retryDelays, turning a
	// 30-second retry into a five-minute one without anything in the ladder
	// saying so.
	webhookClaimLease = 15 * time.Second

	// webhookClaimLeaseBuffer keeps the lease clear of the HTTP timeout. A lease
	// shorter than the timeout would let the sweep reclaim a row whose POST is
	// still in flight and deliver it a second time, which is exactly the
	// duplicate the claim exists to prevent.
	webhookClaimLeaseBuffer = 5 * time.Second

	// webhookFailureThreshold is how many back-to-back failures retire an
	// endpoint. It is the only garbage collector that does not depend on the
	// endpoint telling us it is dead — Zapier's hook URLs answer success
	// unconditionally to keep their ingest available, so a 200 proves bytes were
	// accepted and nothing more. High enough that an afternoon of 500s from a
	// healthy receiver does not switch a customer's integration off.
	webhookFailureThreshold = 20

	// webhookResponseBodyLimit is how much of the receiver's response is kept for
	// the delivery log. Enough for an error message, small enough that a receiver
	// answering with a megabyte of HTML cannot bloat the table.
	webhookResponseBodyLimit = 1024
)

// WebhookDeliveryWorker processes pending webhook deliveries
type WebhookDeliveryWorker struct {
	subscriptionRepo domain.WebhookSubscriptionRepository
	deliveryRepo     domain.WebhookDeliveryRepository
	workspaceRepo    domain.WorkspaceRepository
	logger           logger.Logger
	httpClient       *http.Client
	pollInterval     time.Duration
	batchSize        int
	lastCleanupTime  time.Time
	cleanupInterval  time.Duration
	retentionDays    int
	claimLease       time.Duration
	failureThreshold int
}

// retryDelays is the backoff ladder, aggressive early as per the Standard
// Webhooks spec.
//
// Only the first nine rungs are reachable for a row the webhook triggers wrote,
// because they insert max_attempts = 10: handleDeliveryFailure gives up at
// attempts >= max_attempts and indexes at attempts-1, so index 8 is the last one
// it can pick. The real retry window is therefore about 9h53m, not the ~34h the
// whole table implies. The 24h rung and the clamp under it are kept rather than
// deleted because max_attempts is a per-row column: a row written with a larger
// ceiling does walk further up, and truncating the table would leave it retrying
// every six hours instead.
var retryDelays = []time.Duration{
	30 * time.Second,
	1 * time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
	2 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

// NewWebhookDeliveryWorker creates a new webhook delivery worker
func NewWebhookDeliveryWorker(
	subscriptionRepo domain.WebhookSubscriptionRepository,
	deliveryRepo domain.WebhookDeliveryRepository,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
	httpClient *http.Client,
) *WebhookDeliveryWorker {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &WebhookDeliveryWorker{
		subscriptionRepo: subscriptionRepo,
		deliveryRepo:     deliveryRepo,
		workspaceRepo:    workspaceRepo,
		logger:           logger,
		httpClient:       httpClient,
		pollInterval:     10 * time.Second,
		batchSize:        100,
		cleanupInterval:  1 * time.Hour,
		retentionDays:    7,
		claimLease:       claimLeaseFor(httpClient.Timeout),
		failureThreshold: webhookFailureThreshold,
	}
}

// claimLeaseFor derives the reclaim lease from the client actually in use.
//
// The lease has to outlast the request or the sweep reclaims rows that are still
// in flight, and the caller chooses the client — production passes a 10s one,
// the nil fallback above is 30s. Reading the timeout instead of hard-coding
// against it means raising the timeout cannot quietly turn the reaper into a
// source of duplicate deliveries.
func claimLeaseFor(httpTimeout time.Duration) time.Duration {
	lease := webhookClaimLease
	if httpTimeout > 0 && httpTimeout+webhookClaimLeaseBuffer > lease {
		lease = httpTimeout + webhookClaimLeaseBuffer
	}
	return lease
}

// Start starts the webhook delivery worker
func (w *WebhookDeliveryWorker) Start(ctx context.Context) {
	w.logger.Info("Webhook delivery worker started")

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Webhook delivery worker stopping...")
			return
		case <-ticker.C:
			w.processDeliveries(ctx)
		}
	}
}

// processDeliveries processes pending deliveries across all workspaces
func (w *WebhookDeliveryWorker) processDeliveries(ctx context.Context) {
	// Run cleanup of old deliveries (method handles timing internally)
	w.cleanupOldDeliveries(ctx)

	// Get all workspaces
	workspaces, err := w.workspaceRepo.List(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Failed to list workspaces for webhook processing")
		return
	}

	for _, workspace := range workspaces {
		if err := w.processWorkspaceDeliveries(ctx, workspace.ID); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"error":        err.Error(),
			}).Error("Failed to process webhook deliveries for workspace")
		}
	}
}

// processWorkspaceDeliveries processes pending deliveries for a specific workspace.
//
// INVARIANT: GetPendingForWorkspace does not read rows, it claims them — every
// delivery it returns is already in 'delivering' with claimed_at stamped. So this
// loop owns each row until it writes a terminal state back, and every exit from
// it has to write. An exit that does not leaves a row claimed until the lease
// expires, or worse, back in 'pending' forever while it can never succeed:
// re-selected on every poll for the whole retention window, holding one of the
// batch's slots against deliveries that could have gone out.
func (w *WebhookDeliveryWorker) processWorkspaceDeliveries(ctx context.Context, workspaceID string) error {
	// Sweep first, so rows a crashed worker stranded rejoin this very batch.
	w.reclaimStaleDeliveries(ctx, workspaceID)

	// Get pending deliveries
	deliveries, err := w.deliveryRepo.GetPendingForWorkspace(ctx, workspaceID, w.batchSize)
	if err != nil {
		return fmt.Errorf("failed to get pending deliveries: %w", err)
	}

	if len(deliveries) == 0 {
		return nil
	}

	w.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"count":        len(deliveries),
	}).Debug("Processing webhook deliveries")

	// Cache subscriptions to avoid repeated lookups
	subscriptionCache := make(map[string]*domain.WebhookSubscription)

	for _, delivery := range deliveries {
		select {
		case <-ctx.Done():
			// The rows still claimed here are left claimed on purpose: the
			// database is not the thing that went wrong, we are shutting down,
			// and the reclaim sweep returns them on the next worker's first poll.
			return ctx.Err()
		default:
		}

		// Get or cache subscription
		sub, ok := subscriptionCache[delivery.SubscriptionID]
		if !ok {
			loaded, lookupErr := w.subscriptionRepo.GetByID(ctx, workspaceID, delivery.SubscriptionID)
			if lookupErr != nil {
				w.handleSubscriptionLookupFailure(ctx, workspaceID, delivery, lookupErr)
				continue
			}
			sub = loaded
			subscriptionCache[delivery.SubscriptionID] = sub
		}

		// A disabled subscription is not a reason to hold the row. It used to be
		// skipped untouched, which was survivable while only a human could
		// disable a subscription; now that a dead endpoint disables one
		// automatically, skipping would convert that subscription's whole queue
		// into permanent ballast the moment the endpoint died.
		if !sub.Enabled {
			w.drainDelivery(ctx, workspaceID, delivery, nil, nil, "subscription is disabled")
			continue
		}

		// Process the delivery
		w.processDelivery(ctx, workspaceID, delivery, sub)
	}

	return nil
}

// handleSubscriptionLookupFailure decides whether a failed subscription lookup
// is fatal to the delivery or merely a bad moment for the database.
//
// The distinction is the whole point. GetByID reports pool exhaustion, a network
// timeout and a restarting Postgres exactly as readily as a row that is not
// there, so treating every error as "the subscription is gone" would mark
// thousands of in-flight deliveries permanently failed across every workspace
// during a five-second blip. Only the typed sentinel may be terminal.
func (w *WebhookDeliveryWorker) handleSubscriptionLookupFailure(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, cause error) {
	if errors.Is(cause, domain.ErrWebhookSubscriptionNotFound) {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id":     delivery.ID,
			"subscription_id": delivery.SubscriptionID,
		}).Warn("Dropping webhook delivery whose subscription no longer exists")
		w.drainDelivery(ctx, workspaceID, delivery, nil, nil, "subscription no longer exists")
		return
	}

	w.logger.WithFields(map[string]interface{}{
		"delivery_id":     delivery.ID,
		"subscription_id": delivery.SubscriptionID,
		"error":           cause.Error(),
	}).Error("Failed to get subscription for delivery")
	w.releaseDelivery(ctx, workspaceID, delivery, cause)
}

// drainDelivery moves a delivery that can never succeed to a terminal state and
// releases the claim in the same write.
func (w *WebhookDeliveryWorker) drainDelivery(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, statusCode *int, responseBody *string, reason string) {
	// Pinning attempts to the row's own ceiling is what makes the state terminal.
	// The claim query selects on `status IN ('pending','failed') AND attempts <
	// max_attempts`, so a row marked failed below its ceiling is claimed again on
	// the next poll and nothing has been drained. The human-readable why lives in
	// last_error, which is what the delivery log shows.
	attempts := delivery.MaxAttempts
	if attempts < delivery.Attempts {
		attempts = delivery.Attempts
	}

	if err := w.deliveryRepo.MarkFailed(ctx, workspaceID, delivery.ID, attempts, reason, statusCode, responseBody); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"reason":      reason,
			"error":       err.Error(),
		}).Error("Failed to drain undeliverable webhook delivery")
	}
}

// releaseDelivery hands a claimed row back to 'pending' untouched, for the case
// where nothing is wrong with the delivery — only with us.
func (w *WebhookDeliveryWorker) releaseDelivery(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, cause error) {
	message := cause.Error()

	// The attempt count does not move: nothing was sent, so nothing was
	// attempted, and spending one of ten attempts on our own database having a
	// bad minute is how a transient outage turns into lost deliveries.
	// next_attempt_at is already in the past, so the row rejoins the next batch.
	if err := w.deliveryRepo.UpdateStatus(ctx, workspaceID, delivery.ID,
		domain.WebhookDeliveryStatusPending, delivery.Attempts, nil, nil, &message); err != nil {
		// The release failed for the same reason the lookup did. The row stays
		// claimed and the reclaim sweep returns it once the lease expires —
		// which is precisely what the sweep is for.
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to release webhook delivery claim")
	}
}

// reclaimStaleDeliveries returns rows whose claim has outlived the lease.
//
// A worker killed mid-delivery leaves its rows in 'delivering', where no
// predicate selects them again — stranded exactly like an orphan whose
// subscription was deleted, arrived at from the other side. The sweep is
// deliberately at-least-once: a delivery whose POST succeeded but whose release
// write did not comes back and is sent a second time. That trade is chosen on
// purpose, because the alternative is a row that is never sent at all and never
// stops occupying a batch slot.
func (w *WebhookDeliveryWorker) reclaimStaleDeliveries(ctx context.Context, workspaceID string) {
	reclaimed, err := w.deliveryRepo.ReclaimStale(ctx, workspaceID, w.claimLease)
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to reclaim stale webhook deliveries")
		return
	}
	if reclaimed > 0 {
		w.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
			"reclaimed":    reclaimed,
		}).Info("Reclaimed stale webhook deliveries")
	}
}

// cleanupOldDeliveries removes webhook deliveries older than the retention period
func (w *WebhookDeliveryWorker) cleanupOldDeliveries(ctx context.Context) {
	// Skip if not enough time has passed since last cleanup
	if time.Since(w.lastCleanupTime) < w.cleanupInterval {
		return
	}
	w.lastCleanupTime = time.Now()

	workspaces, err := w.workspaceRepo.List(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Failed to list workspaces for webhook cleanup")
		return
	}

	for _, workspace := range workspaces {
		deleted, err := w.deliveryRepo.CleanupOldDeliveries(ctx, workspace.ID, w.retentionDays)
		if err != nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"error":        err.Error(),
			}).Error("Failed to cleanup old webhook deliveries")
			continue
		}
		if deleted > 0 {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"deleted":      deleted,
			}).Info("Cleaned up old webhook deliveries")
		}
	}
}

// processDelivery sends a single webhook delivery
func (w *WebhookDeliveryWorker) processDelivery(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription) {
	// Build the full payload envelope
	envelope := map[string]interface{}{
		"id":           delivery.ID,
		"type":         delivery.EventType,
		"workspace_id": workspaceID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"data":         delivery.Payload,
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to marshal webhook payload")
		// A payload that will not encode now will not encode on the tenth retry
		// either — the bytes in the row do not change. Retrying it would only
		// keep the slot occupied until the row aged out.
		w.drainDelivery(ctx, workspaceID, delivery, nil, nil,
			fmt.Sprintf("payload cannot be encoded: %s", err.Error()))
		return
	}

	// Generate timestamp for signing
	timestamp := time.Now().Unix()

	// Sign the payload using Standard Webhooks spec
	key, err := decodeSecret(sub.Secret)
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id":     delivery.ID,
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to decode webhook secret")
		// Retryable, unlike the two drains around it: rotating the secret repairs
		// the subscription in place and the queued rows then go out.
		w.handleDeliveryFailure(ctx, workspaceID, delivery, sub, nil, "", fmt.Sprintf("invalid webhook secret: %s", err.Error()))
		return
	}
	signature := signPayload(delivery.ID, timestamp, payloadBytes, key)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to create webhook request")
		// Unreachable in practice — validateURL already demands a parseable
		// http/https URL with a host — but the row is claimed, and an exit that
		// does not write is the defect this guards against, not the odds.
		w.drainDelivery(ctx, workspaceID, delivery, nil, nil,
			fmt.Sprintf("request cannot be built for %q: %s", sub.URL, err.Error()))
		return
	}

	// Set Standard Webhooks headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", delivery.ID)
	req.Header.Set("webhook-timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("webhook-signature", signature)

	// Send the request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		w.failDelivery(ctx, workspaceID, delivery, sub, nil, "", err.Error(),
			fmt.Sprintf("automatically disabled after repeated delivery failures (last error: %s)", err.Error()))
		return
	}
	defer resp.Body.Close()

	responseBody := readLimitedResponseBody(resp)

	w.handleResponseStatus(ctx, workspaceID, delivery, sub, resp.StatusCode, responseBody)
}

// readLimitedResponseBody keeps the first kilobyte of the response and discards
// the rest.
//
// Both halves matter. The kilobyte is what the delivery log stores. Draining the
// remainder is what lets the connection go back into the keep-alive pool:
// closing a body that still has unread bytes makes Go's HTTP client throw the
// connection away, so every delivery would pay a fresh TCP connect and TLS
// handshake to the same host.
func readLimitedResponseBody(resp *http.Response) string {
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, webhookResponseBodyLimit))
	_, _ = io.Copy(io.Discard, resp.Body)
	return string(bodyBytes)
}

// handleResponseStatus applies the response policy for one delivery attempt.
//
// The table it implements, and why each row is what it is:
//
//   - 2xx — delivered, and the consecutive-failure counter goes back to zero.
//   - 410 Gone — terminal. See handleGoneEndpoint.
//   - 429 — retry, and deliberately WITHOUT counting a failure. Rate limiting is
//     the receiver asking for less, not an endpoint dying, and a workspace busy
//     enough to be throttled must not have its integration switched off for
//     being busy.
//   - 404 — retried like any other error, never acted on alone. Zapier authored
//     the REST Hooks spec and it says an endpoint may only be marked bad once a
//     consistent 404 has been proven over time; a Zap that is turned back on
//     resumes answering 200. Persistence is what the shared counter measures.
//   - everything else — retry, count the failure, retire the subscription once
//     the count crosses the threshold.
func (w *WebhookDeliveryWorker) handleResponseStatus(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, statusCode int, responseBody string) {
	errorMsg := fmt.Sprintf("HTTP %d", statusCode)

	switch {
	case statusCode >= 200 && statusCode < 300:
		w.resetSubscriptionFailures(ctx, workspaceID, sub)
		w.handleDeliverySuccess(ctx, workspaceID, delivery, sub, statusCode, responseBody)

	case statusCode == http.StatusGone:
		w.handleGoneEndpoint(ctx, workspaceID, delivery, sub, statusCode, responseBody)

	case statusCode == http.StatusTooManyRequests:
		w.handleDeliveryFailure(ctx, workspaceID, delivery, sub, &statusCode, responseBody, errorMsg)

	default:
		reason := fmt.Sprintf("automatically disabled after repeated delivery failures (last response: %s)", errorMsg)
		if statusCode == http.StatusNotFound {
			reason = "automatically disabled after a sustained HTTP 404 from the endpoint"
		}
		w.failDelivery(ctx, workspaceID, delivery, sub, &statusCode, responseBody, errorMsg, reason)
	}
}

// failDelivery counts one failure against the subscription and then either
// drains the row — because that count just retired the subscription — or
// schedules the next attempt.
//
// Draining rather than scheduling in the disabled case is not a detail: a
// subscription that has just been switched off is about to send every one of its
// queued rows down the disabled branch in processWorkspaceDeliveries, and
// leaving this one scheduled would have it claimed once more, for nothing.
func (w *WebhookDeliveryWorker) failDelivery(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, statusCode *int, responseBody, errorMsg, disableReason string) {
	if w.recordSubscriptionFailure(ctx, workspaceID, sub, disableReason) {
		w.drainDelivery(ctx, workspaceID, delivery, statusCode, &responseBody, disableReason)
		return
	}
	w.handleDeliveryFailure(ctx, workspaceID, delivery, sub, statusCode, responseBody, errorMsg)
}

// recordSubscriptionFailure bumps the consecutive-failure counter and reports
// whether that retired the subscription.
func (w *WebhookDeliveryWorker) recordSubscriptionFailure(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription, reason string) bool {
	if err := w.subscriptionRepo.IncrementFailures(ctx, workspaceID, sub.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to record webhook subscription failure")
		return false
	}

	// Mirror the increment onto the cached copy. The row is the authority — the
	// repository increments in SQL so concurrent workers cannot lose counts — but
	// this batch holds one copy of the subscription for up to a hundred
	// deliveries, and without keeping it in step an endpoint failing every
	// delivery would need one poll per failure to reach the threshold.
	sub.ConsecutiveFailures++

	if sub.ConsecutiveFailures < w.failureThreshold {
		return false
	}

	if err := w.subscriptionRepo.DisableWithReason(ctx, workspaceID, sub.ID, reason); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to disable failing webhook subscription")
		return false
	}
	sub.Enabled = false

	w.logger.WithFields(map[string]interface{}{
		"subscription_id":      sub.ID,
		"workspace_id":         workspaceID,
		"consecutive_failures": sub.ConsecutiveFailures,
		"reason":               reason,
	}).Warn("Disabled webhook subscription after consecutive delivery failures")
	return true
}

// resetSubscriptionFailures clears the counter after a delivery gets through.
func (w *WebhookDeliveryWorker) resetSubscriptionFailures(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) {
	// Nothing to clear, and skipping the write is worth the branch: every
	// successful delivery already writes to webhook_subscriptions through
	// UpdateLastDeliveryAt, so an unconditional reset would double the
	// per-delivery write on that table for the healthy case, which is nearly all
	// of them. The cached counter is at most one batch stale, because the top of
	// each batch re-reads the subscription.
	if sub.ConsecutiveFailures == 0 {
		return
	}

	if err := w.subscriptionRepo.ResetFailures(ctx, workspaceID, sub.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to reset webhook subscription failure counter")
		return
	}
	sub.ConsecutiveFailures = 0
}

// handleGoneEndpoint retires a subscription whose endpoint answered 410 Gone.
//
// Zapier's REST Hook contract makes 410 at a target URL mean "this subscription
// is dead, stop sending", and a Zap that comes back re-creates its subscription
// through performSubscribe — so deleting a Zapier-created row loses nothing and
// spares the user a subscription they never made and cannot explain sitting in
// Settings → Webhooks. One somebody typed in by hand is a different thing:
// disabling it is reversible and visible, and the reason says why.
//
// Act on 410 when it arrives; never rely on receiving it. hooks.zapier.com
// answers success unconditionally to keep its ingest highly available, and it
// serves two protocols with different terminal codes — REST Hook target URLs
// (/hooks/standard/) report death with 410, Catch Hook URLs (/hooks/catch/)
// report it with 404. That is why the consecutive-failure sweep, not this
// branch, is the garbage collector that has to stand on its own.
func (w *WebhookDeliveryWorker) handleGoneEndpoint(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, statusCode int, responseBody string) {
	const reason = "endpoint returned HTTP 410 Gone"

	if sub.Source == domain.WebhookSubscriptionSourceZapier {
		if err := w.subscriptionRepo.Delete(ctx, workspaceID, sub.ID); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"subscription_id": sub.ID,
				"error":           err.Error(),
			}).Error("Failed to delete Zapier webhook subscription reported gone")
			// The subscription survived, so at least take this row out of
			// circulation rather than let it be claimed again for an endpoint
			// that has said it is finished.
			w.drainDelivery(ctx, workspaceID, delivery, &statusCode, &responseBody, reason)
			return
		}

		// Nothing else in this batch should be POSTed at the dead endpoint.
		sub.Enabled = false

		// Redundant once the foreign key cascade is in place, and the only thing
		// standing between a deleted subscription and a permanently poisoned
		// batch before that migration runs.
		if err := w.deliveryRepo.DeleteBySubscriptionID(ctx, workspaceID, sub.ID); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"subscription_id": sub.ID,
				"error":           err.Error(),
			}).Error("Failed to delete deliveries of a removed Zapier subscription")
			w.drainDelivery(ctx, workspaceID, delivery, &statusCode, &responseBody, reason)
			return
		}

		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"workspace_id":    workspaceID,
		}).Info("Deleted Zapier webhook subscription after its endpoint reported gone")
		return
	}

	if err := w.subscriptionRepo.DisableWithReason(ctx, workspaceID, sub.ID, reason); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to disable webhook subscription reported gone")
	} else {
		sub.Enabled = false
	}

	w.drainDelivery(ctx, workspaceID, delivery, &statusCode, &responseBody, reason)
}

// handleDeliverySuccess marks a delivery as successful
func (w *WebhookDeliveryWorker) handleDeliverySuccess(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, statusCode int, responseBody string) {
	now := time.Now().UTC()

	// Mark delivery as delivered
	if err := w.deliveryRepo.MarkDelivered(ctx, workspaceID, delivery.ID, statusCode, responseBody); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to mark delivery as delivered")
		return
	}

	// Update last delivery timestamp
	if err := w.subscriptionRepo.UpdateLastDeliveryAt(ctx, workspaceID, sub.ID, now); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to update last delivery timestamp")
	}

	w.logger.WithFields(map[string]interface{}{
		"delivery_id":     delivery.ID,
		"subscription_id": sub.ID,
		"status_code":     statusCode,
	}).Debug("Webhook delivered successfully")
}

// handleDeliveryFailure handles a failed delivery attempt
func (w *WebhookDeliveryWorker) handleDeliveryFailure(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, statusCode *int, responseBody, errorMsg string) {
	attempts := delivery.Attempts + 1

	// Check if we've exceeded max attempts
	if attempts >= delivery.MaxAttempts {
		// Mark as permanently failed
		if err := w.deliveryRepo.MarkFailed(ctx, workspaceID, delivery.ID, attempts, errorMsg, statusCode, &responseBody); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"delivery_id": delivery.ID,
				"error":       err.Error(),
			}).Error("Failed to mark delivery as permanently failed")
			return
		}

		w.logger.WithFields(map[string]interface{}{
			"delivery_id":     delivery.ID,
			"subscription_id": sub.ID,
			"attempts":        attempts,
			"error":           errorMsg,
		}).Warn("Webhook delivery permanently failed after max retries")
		return
	}

	// Calculate next retry time
	delayIndex := attempts - 1
	if delayIndex >= len(retryDelays) {
		delayIndex = len(retryDelays) - 1
	}
	nextAttempt := time.Now().UTC().Add(retryDelays[delayIndex])

	// Schedule retry
	if err := w.deliveryRepo.ScheduleRetry(ctx, workspaceID, delivery.ID, nextAttempt, attempts, statusCode, &responseBody, &errorMsg); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to schedule delivery retry")
		return
	}

	w.logger.WithFields(map[string]interface{}{
		"delivery_id":     delivery.ID,
		"subscription_id": sub.ID,
		"attempts":        attempts,
		"next_attempt":    nextAttempt.Format(time.RFC3339),
		"error":           errorMsg,
	}).Debug("Webhook delivery failed, scheduled retry")
}

// signPayload signs the webhook payload using Standard Webhooks spec.
// Signed content is `{msgID}.{timestamp}.{payload}`; output is `v1,{base64(HMAC-SHA256)}`.
// The secret must already be the raw HMAC key (see decodeSecret).
func signPayload(msgID string, timestamp int64, payload []byte, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(msgID))
	h.Write([]byte("."))
	h.Write([]byte(strconv.FormatInt(timestamp, 10)))
	h.Write([]byte("."))
	h.Write(payload)
	return "v1," + base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// buildTestPayload returns the `data` object a real delivery of eventType would
// carry.
//
// Nothing here is invented: each shape is the one the PL/pgSQL webhook trigger
// for that table builds — webhook_contacts_trigger,
// webhook_contact_lists_trigger, webhook_contact_segments_trigger and
// webhook_message_history_trigger in internal/database/init.go, and
// WebhookCustomEventsTriggerFunction in internal/database/schema. Keys therefore
// match the real event down to their absence: a field invented here is a field
// the console's Test button teaches a user to map and that arrives empty on
// every genuine delivery, and a Zapier app whose sample records come from this
// function would ship those wrong output fields to its whole install base.
//
// The payload is built inside PostgreSQL, so no compiler can warn when a trigger
// changes shape. Whoever edits a trigger body edits this function too.
func buildTestPayload(eventType string) map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)

	// Parse the event category (e.g., "contact" from "contact.created")
	parts := strings.Split(eventType, ".")
	category := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch category {
	case "contact":
		// to_jsonb(contact_record): the whole contacts row, one key per column,
		// unset columns present and null.
		return map[string]interface{}{
			"contact": testContactRecord(now),
		}
	case "list":
		status, previousStatus := testListStatuses(action)
		return map[string]interface{}{
			"email":     "test@example.com",
			"list_id":   "test_list_456",
			"list_name": "Test Newsletter",
			"status":    status,
			// Always present, null when the membership row was inserted straight
			// at its status rather than transitioning into it.
			"previous_status": previousStatus,
		}
	case "segment":
		return map[string]interface{}{
			"email":        "test@example.com",
			"segment_id":   "test_segment_789",
			"segment_name": "Test Segment",
		}
	case "email":
		return map[string]interface{}{
			"email":        "test@example.com",
			"message_id":   "test_msg_789",
			"template_id":  "test_template_345",
			"broadcast_id": "test_broadcast_012",
			"list_id":      "test_list_456",
			"channel":      "email",
			// Whichever of sent_at/delivered_at/opened_at/... the transition set.
			"event_timestamp": now,
		}
	case "custom_event":
		// to_jsonb(NEW): the whole custom_events row, same rule as contacts.
		return map[string]interface{}{
			"custom_event": testCustomEventRecord(now),
		}
	default:
		// "test" and anything unrecognised. No trigger produces these, so there
		// is no real shape to be faithful to — say so in the payload rather than
		// dress it up as an event.
		return map[string]interface{}{
			"message":    "This is a test webhook from Notifuse",
			"event_type": eventType,
			"created_at": now,
		}
	}
}

// testListStatuses returns the status/previous_status pair the contact_lists
// trigger would build for a list.<action> event. The trigger derives the event
// kind from the transition, so the pair is not free: list.confirmed can only
// come from pending → active, list.resubscribed only from a suppressed status
// back to active.
func testListStatuses(action string) (string, interface{}) {
	switch action {
	case "confirmed":
		return "active", "pending"
	case "resubscribed":
		return "active", "unsubscribed"
	case "unsubscribed", "bounced", "complained":
		// Reachable both ways; the transition from active is the common one.
		return action, "active"
	case "removed":
		// A soft delete leaves the status alone and sets deleted_at.
		return "active", "active"
	default:
		// subscribed → active, pending → pending, both written by an INSERT, so
		// there is no previous status.
		if action == "subscribed" {
			return "active", nil
		}
		return action, nil
	}
}

// testContactRecord mirrors to_jsonb() over a contacts row: every column, with
// the ones a typical contact leaves unset present and null.
func testContactRecord(now string) map[string]interface{} {
	return map[string]interface{}{
		"email":             "test@example.com",
		"external_id":       "ext_456",
		"timezone":          "Europe/Paris",
		"language":          "en",
		"first_name":        "Test",
		"last_name":         "User",
		"full_name":         "Test User",
		"phone":             nil,
		"address_line_1":    nil,
		"address_line_2":    nil,
		"country":           nil,
		"postcode":          nil,
		"state":             nil,
		"job_title":         nil,
		"custom_string_1":   nil,
		"custom_string_2":   nil,
		"custom_string_3":   nil,
		"custom_string_4":   nil,
		"custom_string_5":   nil,
		"custom_number_1":   nil,
		"custom_number_2":   nil,
		"custom_number_3":   nil,
		"custom_number_4":   nil,
		"custom_number_5":   nil,
		"custom_datetime_1": nil,
		"custom_datetime_2": nil,
		"custom_datetime_3": nil,
		"custom_datetime_4": nil,
		"custom_datetime_5": nil,
		"custom_json_1":     nil,
		"custom_json_2":     nil,
		"custom_json_3":     nil,
		"custom_json_4":     nil,
		"custom_json_5":     nil,
		"created_at":        now,
		"updated_at":        now,
		"db_created_at":     now,
		"db_updated_at":     now,
	}
}

// testCustomEventRecord mirrors to_jsonb() over a custom_events row.
func testCustomEventRecord(now string) map[string]interface{} {
	return map[string]interface{}{
		"event_name":  "test_purchase",
		"external_id": "test_event_012",
		"email":       "test@example.com",
		"properties": map[string]interface{}{
			"product_id": "prod_123",
			"amount":     99.99,
			"currency":   "USD",
		},
		"occurred_at": now,
		// Web analytics rows never reach a subscription; the trigger returns
		// early for them, so a delivered custom event is always an API one.
		"source":         "api",
		"integration_id": nil,
		"goal_name":      "Purchase",
		"goal_type":      "purchase",
		"goal_value":     99.99,
		"deleted_at":     nil,
		"created_at":     now,
		"updated_at":     now,
	}
}

// SendTestWebhook sends a test webhook to verify the endpoint
func (w *WebhookDeliveryWorker) SendTestWebhook(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription, eventType string) (int, string, error) {
	// Build test payload
	testID := fmt.Sprintf("test_%d", time.Now().UnixNano())

	// Use provided event type or default to "test"
	if eventType == "" {
		eventType = "test"
	}

	envelope := map[string]interface{}{
		"id":           testID,
		"type":         eventType,
		"workspace_id": workspaceID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"data":         buildTestPayload(eventType),
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return 0, "", fmt.Errorf("failed to marshal test payload: %w", err)
	}

	// Generate timestamp for signing
	timestamp := time.Now().Unix()

	// Sign the payload
	key, err := decodeSecret(sub.Secret)
	if err != nil {
		return 0, "", fmt.Errorf("invalid webhook secret: %w", err)
	}
	signature := signPayload(testID, timestamp, payloadBytes, key)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return 0, "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set Standard Webhooks headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", testID)
	req.Header.Set("webhook-timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("webhook-signature", signature)

	// Send the request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, readLimitedResponseBody(resp), nil
}
