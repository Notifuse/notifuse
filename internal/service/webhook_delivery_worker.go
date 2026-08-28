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
	"runtime/debug"
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
	//
	// The lease only has to cover ONE delivery because processWorkspaceDeliveries
	// renews the claim row by row. Covering a whole batch instead would mean a
	// lease of batchSize x httpTimeout — a quarter of an hour — and a crashed
	// worker's rows would sit stranded for all of it.
	webhookClaimLeaseBuffer = 5 * time.Second

	// webhookFailureThreshold is how many back-to-back failures retire an
	// endpoint. It is the only garbage collector that does not depend on the
	// endpoint telling us it is dead — Zapier's hook URLs answer success
	// unconditionally to keep their ingest available, so a 200 proves bytes were
	// accepted and nothing more. High enough that an afternoon of 500s from a
	// healthy receiver does not switch a customer's integration off.
	webhookFailureThreshold = 20

	// webhookFailureWindow is how long the endpoint has to have been failing
	// before the count above is allowed to retire it.
	//
	// Both conditions are needed because the count on its own measures volume,
	// not persistence. A batch is a hundred deliveries walked serially, so a
	// receiver that is restarting for thirty seconds during an import fails
	// twenty of them inside a single ten-second poll and clears the threshold
	// before it has finished rebooting — which is the opposite of what the
	// comment above promises. The REST Hooks specification asks for "a
	// consistent 404 over time"; this is the "over time" half.
	//
	// Two hours, and the ceiling is what fixes it rather than the floor. This was
	// twelve hours, which is LONGER than the retry ladder a queued delivery can
	// actually walk (reachableRetryWindow, about 9h53m at the max_attempts = 10
	// the triggers write). Every delivery queued when an endpoint died was
	// therefore permanently failed before the window could ever open, and the
	// auto-disable depended entirely on new events still arriving two hours after
	// the trouble started. When someone tears a receiver down the workspace
	// usually goes quiet at the same moment, so the one mechanism that retires a
	// dead endpoint without being told it is dead retired nothing at all.
	//
	// Under the ladder, the backlog alone can satisfy it: a delivery queued when
	// the endpoint died reaches its eighth attempt at 1h53m30s and its ninth at
	// 3h53m30s, so the window opens with two rungs still to run and the failures
	// that open it are the same ones that reach the threshold. Two hours is also
	// far longer than the restart or deploy this rule exists to survive — the
	// burst that clears twenty failures inside one ten-second poll is measured in
	// seconds, not hours.
	webhookFailureWindow = 2 * time.Hour

	// webhookFailureRunMaxAge is how long a run of failures may go on being the
	// same run. A failure arriving later than this after the run began opens a
	// new one instead of joining it.
	//
	// Without an expiry a run never ended, it only got older: failing_since was
	// cleared by a success or a manual re-enable and by nothing else. A ten-hour
	// outage on Monday that fell short of the threshold left the counter at 40
	// and failing_since at Monday; if the workspace then went quiet — which is
	// exactly what happens when someone tears a receiver down — a single
	// transient 500 on Thursday satisfied both gates at once and retired the
	// subscription, under a reason claiming twenty consecutive failures over a
	// sustained period while describing failures three days apart. The count and
	// the window have to describe the same episode or neither of them means
	// anything.
	//
	// Twelve hours, which is the number webhookFailureWindow used to be, and it
	// was always the right number for this job rather than for that one. It has
	// to EXCEED the reachable retry ladder (reachableRetryWindow, about 9h53m at
	// max_attempts = 10): past that point every delivery queued when the trouble
	// started has walked its whole ladder and been given up on, so a failure
	// arriving later cannot be one of theirs and has no claim to their run.
	// Shorter than the ladder, it would cut a genuine outage's backlog in half
	// mid-ladder and restart the count from one while the endpoint was still
	// dead.
	webhookFailureRunMaxAge = 12 * time.Hour

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
	requestTimeout   time.Duration
	failureThreshold int
	failureWindow    time.Duration
	failureRunMaxAge time.Duration
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

// reachableRetryWindow is how long a delivery with the given ceiling keeps being
// retried before it is given up on: the sum of the rungs handleDeliveryFailure
// can actually pick, which is one fewer than maxAttempts because the last
// attempt is followed by MarkFailed rather than by a delay.
//
// It is the bound both auto-disable numbers are placed against, and they sit on
// opposite sides of it. webhookFailureWindow must be SHORTER, or the window only
// opens once the whole backlog has already been abandoned and nothing is left to
// retire the endpoint with. webhookFailureRunMaxAge must be LONGER, or it cuts a
// genuine outage's backlog in half mid-ladder and restarts the count from one.
// Deriving it here is what lets both be asserted against the ladder itself
// rather than against a duration copied out of the table above.
func reachableRetryWindow(maxAttempts int) time.Duration {
	var total time.Duration
	for i := 0; i < maxAttempts-1 && i < len(retryDelays); i++ {
		total += retryDelays[i]
	}
	return total
}

// WebhookDeliveryWorkerOption overrides one of the worker's timings at
// construction.
//
// A functional option rather than an exported field or a setter, because both of
// these are read by a loop that is already running: the ticker reads the poll
// interval once, at the top of Start, and never looks at it again, so a setter
// would appear to work and change nothing — while the lease is read on every
// sweep, from the worker's own goroutine, which makes writing it from anywhere
// else a data race in production for the sake of a test. Fixing both at
// construction leaves nothing to write later.
type WebhookDeliveryWorkerOption func(*WebhookDeliveryWorker)

// WithWebhookPollInterval sets how often the worker looks for work.
//
// It exists so a test can drive the real loop — ticker, recover, claim, POST,
// release — at a speed a test can wait for. The alternative on offer was an
// exported entry point into the middle of the loop, which would have proven that
// the middle works when called directly and nothing about the loop that calls
// it in production.
//
// A non-positive interval is ignored rather than honoured: time.NewTicker panics
// on one, and Start runs on a bare goroutine, so that panic would land at boot
// with no caller to catch it.
func WithWebhookPollInterval(interval time.Duration) WebhookDeliveryWorkerOption {
	return func(w *WebhookDeliveryWorker) {
		if interval <= 0 {
			return
		}
		w.pollInterval = interval
	}
}

// WithWebhookClaimLease overrides how long a claimed delivery may sit in
// 'delivering' before the sweep decides the worker holding it died.
//
// Production keeps the derived default (see claimLeaseFor). This exists so a
// test can prove the reclaim path in seconds instead of waiting out a real
// lease.
//
// Shortening the lease is safe to expose only because normaliseTimings shortens
// the request to match it: the invariant is "the POST cannot outlive the claim",
// and it is enforced there rather than here. An option cannot see the client it
// is being applied next to, so this function is structurally incapable of
// checking the lease against anything — which is exactly why the check does not
// live in it.
//
// A non-positive lease is ignored, for the same reason the interval is: it would
// make every claimed row instantly stale, so every delivery in flight would be
// reclaimed and sent again.
func WithWebhookClaimLease(lease time.Duration) WebhookDeliveryWorkerOption {
	return func(w *WebhookDeliveryWorker) {
		if lease <= 0 {
			return
		}
		w.claimLease = lease
	}
}

// NewWebhookDeliveryWorker creates a new webhook delivery worker
func NewWebhookDeliveryWorker(
	subscriptionRepo domain.WebhookSubscriptionRepository,
	deliveryRepo domain.WebhookDeliveryRepository,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
	httpClient *http.Client,
	opts ...WebhookDeliveryWorkerOption,
) *WebhookDeliveryWorker {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	worker := &WebhookDeliveryWorker{
		subscriptionRepo: subscriptionRepo,
		deliveryRepo:     deliveryRepo,
		workspaceRepo:    workspaceRepo,
		logger:           logger,
		httpClient:       httpClient,
		pollInterval:     10 * time.Second,
		batchSize:        100,
		cleanupInterval:  1 * time.Hour,
		retentionDays:    7,
		failureThreshold: webhookFailureThreshold,
		failureWindow:    webhookFailureWindow,
		failureRunMaxAge: webhookFailureRunMaxAge,
	}

	// After the defaults, so an explicit lease wins over the one derived from the
	// client's timeout rather than racing it.
	for _, opt := range opts {
		opt(worker)
	}

	// And after the options, because it is the only step that sees the client and
	// every override at once. See normaliseTimings.
	worker.normaliseTimings(httpClient.Timeout)

	return worker
}

// normaliseTimings settles the two timings that have to agree with each other,
// once, in the only place that can see both the client and whatever the options
// did.
//
// This is the cross-field step the options cannot perform. An option receives
// the worker mid-construction and nothing else — not the http.Client, not its
// siblings — so WithWebhookClaimLease could accept any positive duration and did
// not, and could not, know it had just installed a lease shorter than the
// request it is supposed to outlast. Everything that needs two knobs at once
// belongs here.
//
// The invariant it enforces: THE POST CANNOT OUTLIVE THE CLAIM. Break it and the
// reclaim sweep returns a row to 'pending' while its request is still open, a
// second worker claims and delivers it, and the subscriber gets the webhook
// twice — invisibly, because MarkDelivered's UPDATE carries no claim predicate,
// so the row still ends 'delivered' with one attempt and nothing in the delivery
// log says it went out twice.
//
// It is enforced by shortening the request to fit the lease, not by lengthening
// the lease to clear the request, and the direction is a deliberate choice:
//
//   - It is the only direction that also covers a client with Timeout: 0. In
//     net/http that means no timeout at all, and no lease can be long enough to
//     cover a request that never ends. Reading the timeout and clamping the
//     lease up against it silently does nothing in exactly the case where an
//     unbounded request is running under a bounded lease.
//   - Lengthening the lease is not the cheap direction it looks like. A lease is
//     also the recovery time for a crashed worker's rows, and one long enough to
//     matter silently overrides the first rungs of retryDelays — a lease clamped
//     to minutes turns a 30-second retry into a five-minute one with nothing in
//     the ladder saying so. Shortening a request costs one attempt of one
//     delivery, which the ladder already handles.
//   - It keeps the seam usable. A lease clamped up to httpTimeout +
//     webhookClaimLeaseBuffer can never be shorter than that buffer, so no test
//     could drive the reclaim path in under five seconds and the option would
//     exist without being able to do its job.
func (w *WebhookDeliveryWorker) normaliseTimings(httpTimeout time.Duration) {
	if w.claimLease <= 0 {
		w.claimLease = claimLeaseFor(httpTimeout)
	}
	w.requestTimeout = requestBudgetFor(w.claimLease, httpTimeout)
}

// requestBudgetFor is how long one delivery's HTTP request may run, given the
// lease that authorises it and the client it will run on.
//
// Strictly shorter than the lease, always, and the slack is not decoration: the
// write that records the outcome — MarkDelivered, ScheduleRetry, MarkFailed —
// happens after the response and still has to land inside the claim. A request
// allowed to consume the whole lease would finish just as the sweep decided the
// worker was dead.
//
// It is a budget for the whole claim, not for the POST alone. processDelivery
// spends it from the renewal, so the round-trips between the two — the
// subscription lookup above all — come out of this number rather than out of
// the slack that pays for the outcome write.
//
// The slack is webhookClaimLeaseBuffer, or a third of the lease when the lease
// is shorter than three of them. A flat buffer cannot be subtracted from a
// one-second test lease, and scaling it is what lets the same rule serve a lease
// of seconds and one of minutes.
func requestBudgetFor(lease, httpTimeout time.Duration) time.Duration {
	slack := webhookClaimLeaseBuffer
	if lease/3 < slack {
		slack = lease / 3
	}

	budget := lease - slack

	// A client timeout that is already tighter wins: it is the operator's own
	// ceiling on how long a receiver may hold a connection, and this must not
	// raise it. A zero timeout is not a tighter ceiling, it is the absence of
	// one, which is the case the lease-derived budget exists to cover.
	if httpTimeout > 0 && httpTimeout < budget {
		budget = httpTimeout
	}

	return budget
}

// claimLeaseFor derives the reclaim lease from the client actually in use.
//
// The lease has to outlast the request or the sweep reclaims rows that are still
// in flight, and the caller chooses the client — production passes a 10s one,
// the nil fallback above is 30s. Reading the timeout instead of hard-coding
// against it means raising the timeout cannot quietly turn the reaper into a
// source of duplicate deliveries.
//
// A zero timeout falls back to the floor rather than being treated as a very
// long one, and that is deliberate rather than an oversight. In net/http a zero
// Timeout means no timeout at all, so there is no duration to derive a lease
// from — any number picked here would be a guess dressed as a derivation, and a
// large one would also become this worker's recovery time for a crashed peer's
// rows. The unbounded request is handled where it can actually be handled, by
// requestBudgetFor bounding the request itself; this function is left saying
// only what it can know.
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
			w.processDeliveriesGuarded(ctx)
		}
	}
}

// processDeliveriesGuarded is this goroutine's outermost recover.
//
// Start runs on a bare goroutine — internal/app/app.go launches it with a plain
// `go func()` and no recover of its own — so any panic that escapes here takes
// the whole server down with it: every in-flight HTTP request, every other
// worker. deliverOne guards the one call most likely to panic, but that call is
// a single statement of the poll. Listing workspaces, the batch loop itself, the
// drain and release paths, handleSubscriptionLookupFailure, the reclaim sweep
// and the retention cleanup all run naked on this same goroutine, and a nil map
// or a short slice in any of them is just as fatal. The task service guards its
// whole processor goroutine for exactly this reason.
//
// A poll is the right unit to lose. The next tick is ten seconds away, nothing
// here is transactional, and rows this poll had claimed are handed back by the
// reclaim sweep — whereas losing the process strands every claimed row in every
// workspace until a restarted worker's first sweep.
func (w *WebhookDeliveryWorker) processDeliveriesGuarded(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.WithFields(map[string]interface{}{
				"panic": fmt.Sprintf("%v", r),
				"stack": string(debug.Stack()),
			}).Error("Panic in webhook delivery poll")
		}
	}()

	w.processDeliveries(ctx)
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

		// Renew the claim before doing anything that writes to this row or puts
		// bytes on the wire for it.
		//
		// The claim was stamped once, for the whole batch. Walking a hundred rows
		// serially with a ten-second-per-row ceiling can outrun a fifteen-second
		// lease a hundred times over, so without this the sweep hands rows still
		// held here to a second worker and both POST them — the duplicate
		// delivery the claim exists to prevent, manufactured by its own reaper.
		// Renewing per row keeps the lease short, which is what makes recovery
		// from a crashed worker fast.
		//
		// The renewal also restarts the clock the request is budgeted against, and
		// claimStart is this process's own reading of it — taken before the call
		// rather than after, because the UPDATE stamps claimed_at at or after
		// this line, so a budget anchored here can only run out early. Late is
		// the direction that duplicates. What it is spent on is processDelivery's
		// business.
		claimStart := time.Now()
		owned, renewedAt, renewErr := w.deliveryRepo.RenewClaim(ctx, workspaceID, delivery.ID, delivery.ClaimedAt)
		if renewErr != nil {
			// We cannot prove we still own the row, so we must not write to it.
			// Leaving it claimed hands it to the sweep, which is the one path
			// that is safe when the database is the thing that is unwell.
			w.logger.WithFields(map[string]interface{}{
				"delivery_id": delivery.ID,
				"error":       renewErr.Error(),
			}).Error("Failed to renew webhook delivery claim")
			continue
		}
		if !owned {
			// The sweep returned this row and someone else has it. Anything we
			// wrote now would be written over its new owner's work.
			w.logger.WithFields(map[string]interface{}{
				"delivery_id": delivery.ID,
			}).Warn("Skipping webhook delivery whose claim was reclaimed")
			continue
		}
		delivery.ClaimedAt = renewedAt

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
		w.deliverOne(ctx, workspaceID, delivery, sub, claimStart)
	}

	return nil
}

// deliverOne wraps one delivery in a recover, so a panic costs that delivery
// rather than the rest of the batch.
//
// This is the inner of two guards and it is about scope, not survival:
// processDeliveriesGuarded is what keeps the process alive, and losing a whole
// poll there means abandoning up to batchSize rows still in 'delivering' per
// workspace, which nothing can touch until a sweep returns them. Recovering here
// as well confines the ordinary case — a bug in one delivery — to that one
// delivery, and the batch walks on. The segment queue processor guards each
// contact inside its loop for the same reason.
//
// The row is released rather than drained: a panic is a bug in us, not a verdict
// on the delivery, and draining would discard a webhook the next build delivers
// perfectly well.
//
// claimStart travels down from the renewal because the request's budget is
// anchored to it rather than to the moment the request is built; see
// processDelivery.
//
// The release below runs after recover() has returned, so it is outside this
// guard no matter how it is indented — a panic there is a new panic on a normal
// unwind and would escape with the rest of the batch. releaseDelivery carries
// its own recover for that reason; see the comment on it.
func (w *WebhookDeliveryWorker) deliverOne(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, claimStart time.Time) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.WithFields(map[string]interface{}{
				"delivery_id":     delivery.ID,
				"subscription_id": delivery.SubscriptionID,
				"panic":           fmt.Sprintf("%v", r),
				"stack":           string(debug.Stack()),
			}).Error("Panic while delivering webhook")
			w.releaseDelivery(ctx, workspaceID, delivery, fmt.Errorf("panic while delivering: %v", r))
		}
	}()

	w.processDelivery(ctx, workspaceID, delivery, sub, claimStart)
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
//
// It carries its own recover, and the reason is a Go rule rather than a taste in
// defensiveness: its most important caller is deliverOne's deferred function,
// which runs it AFTER recover() has already returned. At that point the original
// panic is finished, so a fresh one raised here is a new panic on a normal
// unwind — it does not reach the recover it appears to be standing inside. It
// would leave deliverOne, abandon every remaining row of the batch in
// 'delivering' with a live claim, leave the workspace loop, and be caught only
// by processDeliveriesGuarded, which drops the whole poll. That is precisely the
// outcome deliverOne's own comment promises it prevents. The other caller,
// handleSubscriptionLookupFailure, runs bare in the batch loop and needs the
// same protection for the same reason.
//
// Swallowing the panic loses nothing the failure path did not already accept:
// the error branch below leaves the row claimed for the sweep to return, and so
// does this.
func (w *WebhookDeliveryWorker) releaseDelivery(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, cause error) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.WithFields(map[string]interface{}{
				"delivery_id": delivery.ID,
				"panic":       fmt.Sprintf("%v", r),
				"stack":       string(debug.Stack()),
			}).Error("Panic while releasing a webhook delivery claim")
		}
	}()

	message := cause.Error()

	// ReleaseClaim rather than UpdateStatus, and the difference is the delivery
	// log. UpdateStatus writes last_attempt_at, last_response_status and
	// last_response_body on every call, so releasing through it stamped an
	// attempt that never left the process and erased the receiver's last real
	// status and body — leaving a user debugging their webhook with a Postgres
	// pool message where the endpoint's own 500 used to be.
	//
	// The claim token goes with it. Releasing on the id alone let this write
	// outlive the claim that justified it: deliverOne's recover releases
	// unconditionally, so a panic AFTER MarkDelivered committed pushed a
	// delivered row back to 'pending' with attempts below its ceiling and
	// next_attempt_at in the past — claimed again on the next poll and POSTed a
	// second time, a duplicate manufactured by the guard that exists to prevent
	// them. The cross-worker shape is the same write from the other side: this
	// worker stalls past its lease, the sweep hands the row to another, and the
	// release yanks it back out from under a POST already in flight.
	if err := w.deliveryRepo.ReleaseClaim(ctx, workspaceID, delivery.ID, delivery.ClaimedAt, message); err != nil {
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
func (w *WebhookDeliveryWorker) processDelivery(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, claimStart time.Time) {
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

	// The request runs under a deadline derived from the claim this row is held
	// by, so it cannot still be open when the sweep decides this worker died. See
	// normaliseTimings for why the request is bounded by the lease rather than
	// the other way round.
	//
	// Anchored at claimStart, not measured from this line, and that is the whole
	// invariant rather than a refinement of it. The claim's clock started at the
	// renewal, several database round-trips ago, and everything since has been
	// spent on it: the renewal itself, the subscription lookup above all — a real
	// query, on a pool sql.DB will block on for as long as it takes — then the
	// marshal and the signature. A budget measured from here would hand out the
	// full lease-minus-slack a second time on top of those, so one slow lookup
	// puts the end of the POST past the lease with the request still open, which
	// is the duplicate the budget exists to prevent. Nor is the slack a reserve
	// they may draw on: it is what pays for the outcome write below.
	//
	// Deliberately a separate context from ctx, not a shadow of it: everything
	// after the response — MarkDelivered, ScheduleRetry, MarkFailed — has to run
	// on the caller's context. A POST that used most of its budget would leave a
	// shadowed ctx with nothing on it, the outcome write would fail with a
	// deadline error, and the row would stay claimed and be delivered again by
	// the next sweep. Reusing the request's deadline for the write that records
	// the request is how you build the duplicate you were bounding the request to
	// avoid.
	if spent := time.Since(claimStart); spent >= w.requestTimeout {
		// There is no budget left to send under, so this is not a request that
		// gets cut short — it is one that must not start. Releasing rather than
		// attempting keeps the cost to what it actually is: nothing left the
		// process, so nothing was attempted, and spending one of ten attempts on
		// our own database having a bad minute is how a transient outage turns
		// into lost deliveries. The row goes back to 'pending' with its ladder
		// intact and the next poll sends it under a claim of its own. ReleaseClaim
		// carries the claim token, so if the sweep has already taken this row the
		// write lands on nothing rather than on its new owner.
		w.logger.WithFields(map[string]interface{}{
			"delivery_id":     delivery.ID,
			"subscription_id": sub.ID,
			"spent":           spent.String(),
			"request_budget":  w.requestTimeout.String(),
		}).Warn("Releasing webhook delivery whose claim budget was spent before the request")
		w.releaseDelivery(ctx, workspaceID, delivery, fmt.Errorf(
			"delivery claim ran out of time before the request could be sent: %s of a %s budget was already spent",
			spent.Round(time.Millisecond), w.requestTimeout))
		return
	}

	reqCtx, cancelRequest := context.WithDeadline(ctx, claimStart.Add(w.requestTimeout))
	defer cancelRequest()

	// Create HTTP request
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sub.URL, bytes.NewReader(payloadBytes))
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
		// The reason describes the run of failures, not the one response that
		// happened to arrive last. Naming a cause from the final status alone
		// labelled a subscription retired by nineteen 500s and one 404 as a URL
		// problem — and, the other way round, hid the 404 that is a Catch Hook's
		// death code behind whichever 500 landed on top of it.
		reason := fmt.Sprintf(
			"automatically disabled after %d consecutive delivery failures over more than %d hours (most recent response: %s)",
			w.failureThreshold, int(w.failureWindow.Hours()), errorMsg)
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
	// Before this failure joins the run in progress, decide whether there still
	// is one to join.
	w.expireStaleFailureRun(ctx, workspaceID, sub)

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

	// Mirror failing_since the same way, and only when it is unset: the SQL is a
	// COALESCE, so the row records the first failure of the run and no later one.
	if sub.FailingSince == nil {
		startedAt := time.Now().UTC()
		sub.FailingSince = &startedAt
	}

	if sub.ConsecutiveFailures < w.failureThreshold {
		return false
	}

	// Volume is not persistence. Twenty failures prove the endpoint refused
	// twenty deliveries; they say nothing about how long it has been refusing
	// them, and a hundred deliveries go out per poll — so an import against a
	// receiver that is restarting clears the threshold in ten seconds and would
	// retire an integration that is healthy again a minute later, discarding
	// every queued delivery with it. The window is what makes "sustained" mean
	// sustained.
	if time.Since(*sub.FailingSince) < w.failureWindow {
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

// expireStaleFailureRun ends a run of failures that has grown too old for this
// failure to belong to it, so the count and the window keep describing one
// episode. See webhookFailureRunMaxAge for what goes wrong when they do not.
//
// It reuses ResetFailures rather than folding the expiry into IncrementFailures'
// SQL, which would need the run's maximum age — and, through it, the failure
// window and the retry ladder it is derived from — duplicated in the repository
// package, where nothing else would ever keep the copies in step. Two statements
// instead of one is affordable here in a way it is not for the increment itself:
// this fires once at the head of a stale run rather than on every failure, and
// two workers racing it both clear and both increment, which lands the counter
// at one or two instead of one. The increment stays atomic, which is the part
// concurrent failures can actually corrupt.
func (w *WebhookDeliveryWorker) expireStaleFailureRun(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) {
	if sub.FailingSince == nil || time.Since(*sub.FailingSince) < w.failureRunMaxAge {
		return
	}

	if err := w.subscriptionRepo.ResetFailures(ctx, workspaceID, sub.ID); err != nil {
		// The old run stands. That is the safe direction to fail in — it delays a
		// retirement rather than causing one — and the next failure tries again.
		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to expire a stale webhook subscription failure run")
		return
	}

	// Cleared here, re-opened by the COALESCE in the IncrementFailures that
	// follows: this failure becomes the first of the new run rather than the
	// twenty-first of one that ended days ago.
	sub.ConsecutiveFailures = 0
	sub.FailingSince = nil

	w.logger.WithFields(map[string]interface{}{
		"subscription_id": sub.ID,
		"workspace_id":    workspaceID,
	}).Info("Started a new webhook failure run after the previous one expired")
}

// resetSubscriptionFailures clears the counter after a delivery gets through.
func (w *WebhookDeliveryWorker) resetSubscriptionFailures(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription) {
	// Nothing to clear, and skipping the write is worth the branch: every
	// successful delivery already writes to webhook_subscriptions through
	// UpdateLastDeliveryAt, so an unconditional reset would double the
	// per-delivery write on that table for the healthy case, which is nearly all
	// of them. The cached counter is at most one batch stale, because the top of
	// each batch re-reads the subscription.
	//
	// The condition mirrors the repository's own guard, which skips on
	// `consecutive_failures <> 0 OR failing_since IS NOT NULL`: a row can carry a
	// window with a zero count, and returning early on the count alone would
	// leave the window standing.
	if sub.ConsecutiveFailures == 0 && sub.FailingSince == nil {
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

	// Both columns, because the SQL clears both. Zeroing only the counter left
	// the cached copy — one *sub shared by a whole hundred-row batch — carrying a
	// FailingSince from hours ago, so recordSubscriptionFailure's `if
	// sub.FailingSince == nil` guard never re-dated the run. A receiver that had
	// just proved it was alive was then retired by twenty seconds of a transient
	// blip, and the disabled branch drained everything it still had queued.
	sub.FailingSince = nil
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
