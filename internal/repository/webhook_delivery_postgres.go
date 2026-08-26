package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// webhookDeliveryColumns is the read projection shared by every delivery query,
// including the RETURNING clause of the claim. They all scan positionally into
// one helper, so a column added to a single query would be a runtime scan error
// no compiler would catch.
const webhookDeliveryColumns = `
		id, subscription_id, event_type, payload, status,
		attempts, max_attempts, next_attempt_at, last_attempt_at,
		delivered_at, last_response_status, last_response_body, last_error,
		claimed_at, created_at`

// webhookDeliveryRepository implements domain.WebhookDeliveryRepository for PostgreSQL
type webhookDeliveryRepository struct {
	workspaceRepo domain.WorkspaceRepository
	// logger exists for one case that has no other way out: a claimed row this
	// repository decides to skip. Skipping is right — see GetPendingForWorkspace
	// — but it is a decision taken here, about a row only this layer can name,
	// and returning it as an error would throw away the rest of the batch. The
	// analytics repositories take a logger for the same reason.
	logger logger.Logger
}

// NewWebhookDeliveryRepository creates a new PostgreSQL webhook delivery repository
func NewWebhookDeliveryRepository(workspaceRepo domain.WorkspaceRepository, log logger.Logger) domain.WebhookDeliveryRepository {
	return &webhookDeliveryRepository{
		workspaceRepo: workspaceRepo,
		logger:        log,
	}
}

// WebhookDelivery alias for domain type
type WebhookDelivery = domain.WebhookDelivery

// GetPendingForWorkspace claims the next batch of deliveries for this worker.
//
// It is a write, not a read: the same statement that selects the rows moves
// them to 'delivering' and stamps claimed_at, so two workers polling the same
// workspace get disjoint batches. Without that, every replica delivers every
// event, and a duplicate webhook is a duplicate side effect — a second order, a
// second message — in whatever system consumes it.
//
// The claim is a durable status change rather than a held row lock, which is
// the point: holding a transaction open across the outbound HTTP call would put
// an idle-in-transaction session on every workspace database for the length of
// every delivery, and that pattern has already frozen this codebase's broadcast
// queue once. The cost is that a crashed worker leaves its claims behind, which
// is what ReclaimStale exists to undo.
//
// Note that 'delivering' is absent from the inner predicate. That absence is
// the entire mechanism: were it listed, a claimed row would keep matching and
// the claim would buy nothing.
func (r *webhookDeliveryRepository) GetPendingForWorkspace(ctx context.Context, workspaceID string, limit int) ([]*WebhookDelivery, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// SKIP LOCKED rather than a plain FOR UPDATE: a second worker arriving
	// mid-claim should walk past the contended rows and take the next ones, not
	// queue behind them and then find they no longer match.
	query := `
		UPDATE webhook_deliveries
		SET status = 'delivering', claimed_at = NOW()
		WHERE id IN (
			SELECT id FROM webhook_deliveries
			WHERE status IN ('pending', 'failed')
				AND attempts < max_attempts
				AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + webhookDeliveryColumns

	rows, err := workspaceDB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending deliveries: %w", err)
	}
	defer rows.Close()

	// From here the claim is already durable: the UPDATE ran in autocommit, so
	// every row this statement matched is in 'delivering' whether or not it makes
	// it back through this loop. Returning an error therefore does not undo
	// anything — it only discards rows that are already claimed. That makes a
	// single unscannable row the worst kind of poison pill: it would fail the
	// whole batch, on every poll, forever, and take the workspace's entire
	// webhook queue with it. So a row that will not scan is skipped and the rest
	// of the batch goes out; the skipped row stays 'delivering' until the reclaim
	// sweep returns it, which bounds the damage to one row per lease instead of
	// the workspace.
	var deliveries []*WebhookDelivery
	var scanErr error
	var skipped int
	for rows.Next() {
		delivery, err := scanWebhookDeliveryFromRows(rows)
		if err != nil {
			scanErr = fmt.Errorf("failed to scan delivery: %w", err)
			skipped++
			continue
		}
		deliveries = append(deliveries, delivery)
	}

	if err := rows.Err(); err != nil {
		scanErr = fmt.Errorf("error iterating deliveries: %w", err)
	}

	// Log before deciding whether to return it, because on the common path it is
	// never returned at all. A row that cannot be scanned cannot be scanned on
	// the next poll either: it is claimed, skipped, reclaimed by the sweep and
	// skipped again, every ten seconds for the whole retention window, occupying
	// one of the batch's hundred slots the entire time. Silently, before this —
	// no log, no metric, no error — while an operator watching the console sees a
	// delivery wedged in Delivering with nothing anywhere to correlate it to.
	// Same silence swallowed a mid-iteration rows.Err(), which is not a poison
	// row at all but a batch cut short by a connection that died.
	if scanErr != nil {
		r.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
			"skipped":      skipped,
			"claimed":      len(deliveries),
			"error":        scanErr.Error(),
		}).Error("Skipped unreadable rows in a claimed webhook delivery batch")
	}

	// Only report the failure when it cost the caller everything. With rows in
	// hand, surfacing it would throw away deliveries that are claimed, valid and
	// ready to send, to describe a row the sweep will pick up anyway.
	if scanErr != nil && len(deliveries) == 0 {
		return nil, scanErr
	}

	return deliveries, nil
}

// RenewClaim re-stamps one row's lease, and reports whether we still hold it.
//
// The claimed_at we were handed is the claim token. Matching on it is what makes
// this a renewal rather than a theft: if the sweep returned the row and another
// worker took it, its claimed_at has moved and this updates nothing, so the
// caller learns to leave the row alone instead of POSTing it a second time
// alongside its new owner. Matching on status = 'delivering' covers the other
// half — a row the sweep returned to 'pending' that nobody has taken yet.
func (r *webhookDeliveryRepository) RenewClaim(ctx context.Context, workspaceID, id string, claimedAt *time.Time) (bool, *time.Time, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// A NULL claimed_at is not a claim anyone can renew: rows written before the
	// claim existed carry one, and ReclaimStale treats them as stale for exactly
	// that reason. Refusing here rather than in SQL keeps the predicate a plain
	// equality, which an index can use.
	if claimedAt == nil {
		return false, nil, nil
	}

	query := `
		UPDATE webhook_deliveries
		SET claimed_at = NOW()
		WHERE id = $1 AND status = 'delivering' AND claimed_at = $2
		RETURNING claimed_at
	`

	var renewed time.Time
	err = workspaceDB.QueryRowContext(ctx, query, id, *claimedAt).Scan(&renewed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("failed to renew delivery claim: %w", err)
	}

	return true, &renewed, nil
}

// ListAll retrieves all deliveries for a workspace with optional subscription filter and pagination
func (r *webhookDeliveryRepository) ListAll(ctx context.Context, workspaceID string, subscriptionID *string, limit, offset int) ([]*WebhookDelivery, int, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	var total int
	var rows *sql.Rows

	if subscriptionID != nil && *subscriptionID != "" {
		// With subscription filter
		countQuery := `SELECT COUNT(*) FROM webhook_deliveries WHERE subscription_id = $1`
		err = workspaceDB.QueryRowContext(ctx, countQuery, *subscriptionID).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to count deliveries: %w", err)
		}

		query := `
			SELECT ` + webhookDeliveryColumns + `
			FROM webhook_deliveries
			WHERE subscription_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		rows, err = workspaceDB.QueryContext(ctx, query, *subscriptionID, limit, offset)
	} else {
		// Without subscription filter - all deliveries
		countQuery := `SELECT COUNT(*) FROM webhook_deliveries`
		err = workspaceDB.QueryRowContext(ctx, countQuery).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to count deliveries: %w", err)
		}

		query := `
			SELECT ` + webhookDeliveryColumns + `
			FROM webhook_deliveries
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
		rows, err = workspaceDB.QueryContext(ctx, query, limit, offset)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("failed to query deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*WebhookDelivery
	for rows.Next() {
		delivery, err := scanWebhookDeliveryFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan delivery: %w", err)
		}
		deliveries = append(deliveries, delivery)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating deliveries: %w", err)
	}

	return deliveries, total, nil
}

// UpdateStatus updates the status of a delivery
func (r *webhookDeliveryRepository) UpdateStatus(ctx context.Context, workspaceID, id string, status string, attempts int, responseStatus *int, responseBody, lastError *string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()

	// The claim travels with the status. Any status but 'delivering' means this
	// worker is done with the row and its claim has to go, or a row handed back
	// to 'pending' still reads as in flight; a caller writing 'delivering' is
	// re-asserting its own claim and keeps the original timestamp. Holding that
	// invariant — in 'delivering' if and only if claimed — is what lets
	// ReclaimStale treat a claimless 'delivering' row as stranded.
	//
	// The ::varchar on the assignment is load-bearing, not decoration. Comparing
	// $2 to a string literal types it as text, while assigning it to status
	// types it as the column's varchar, and PostgreSQL rejects the whole
	// statement rather than choose ("inconsistent types deduced for parameter").
	// Casting on the assignment side lets the parameter settle on text
	// everywhere and be coerced once, here.
	query := `
		UPDATE webhook_deliveries
		SET status = $2::varchar, attempts = $3, last_attempt_at = $4,
			last_response_status = $5, last_response_body = $6, last_error = $7,
			claimed_at = CASE WHEN $2 = 'delivering' THEN claimed_at ELSE NULL END
		WHERE id = $1
	`

	_, err = workspaceDB.ExecContext(ctx, query, id, status, attempts, now, responseStatus, responseBody, lastError)
	if err != nil {
		return fmt.Errorf("failed to update delivery status: %w", err)
	}

	return nil
}

// MarkDelivered marks a delivery as successfully delivered
func (r *webhookDeliveryRepository) MarkDelivered(ctx context.Context, workspaceID, id string, responseStatus int, responseBody string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()

	// Truncate response body to 1KB
	if len(responseBody) > 1024 {
		responseBody = responseBody[:1024]
	}

	query := `
		UPDATE webhook_deliveries
		SET status = 'delivered', delivered_at = $2, last_attempt_at = $2,
			attempts = attempts + 1, last_response_status = $3, last_response_body = $4,
			claimed_at = NULL
		WHERE id = $1
	`

	_, err = workspaceDB.ExecContext(ctx, query, id, now, responseStatus, responseBody)
	if err != nil {
		return fmt.Errorf("failed to mark delivery as delivered: %w", err)
	}

	return nil
}

// ScheduleRetry schedules a retry for a failed delivery
func (r *webhookDeliveryRepository) ScheduleRetry(ctx context.Context, workspaceID, id string, nextAttempt time.Time, attempts int, responseStatus *int, responseBody, lastError *string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()

	// Truncate response body to 1KB
	if responseBody != nil && len(*responseBody) > 1024 {
		truncated := (*responseBody)[:1024]
		responseBody = &truncated
	}

	// Releasing the claim here is what puts the row back in reach of the next
	// claim: 'failed' matches the pending predicate again, and a leftover
	// claimed_at on a row that is no longer in flight would only mislead whoever
	// reads the table next.
	query := `
		UPDATE webhook_deliveries
		SET status = 'failed', attempts = $2, next_attempt_at = $3, last_attempt_at = $4,
			last_response_status = $5, last_response_body = $6, last_error = $7,
			claimed_at = NULL
		WHERE id = $1
	`

	_, err = workspaceDB.ExecContext(ctx, query, id, attempts, nextAttempt, now, responseStatus, responseBody, lastError)
	if err != nil {
		return fmt.Errorf("failed to schedule retry: %w", err)
	}

	return nil
}

// MarkFailed marks a delivery as permanently failed (max retries exceeded)
func (r *webhookDeliveryRepository) MarkFailed(ctx context.Context, workspaceID, id string, attempts int, lastError string, responseStatus *int, responseBody *string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()

	// Truncate response body to 1KB
	if responseBody != nil && len(*responseBody) > 1024 {
		truncated := (*responseBody)[:1024]
		responseBody = &truncated
	}

	query := `
		UPDATE webhook_deliveries
		SET status = 'failed', attempts = $2, last_attempt_at = $3,
			last_response_status = $4, last_response_body = $5, last_error = $6,
			claimed_at = NULL
		WHERE id = $1
	`

	_, err = workspaceDB.ExecContext(ctx, query, id, attempts, now, responseStatus, responseBody, lastError)
	if err != nil {
		return fmt.Errorf("failed to mark delivery as failed: %w", err)
	}

	return nil
}

// Create creates a new webhook delivery (used for test webhooks)
func (r *webhookDeliveryRepository) Create(ctx context.Context, workspaceID string, delivery *WebhookDelivery) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()
	delivery.CreatedAt = now
	delivery.NextAttemptAt = now

	payloadJSON, err := json.Marshal(delivery.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	query := `
		INSERT INTO webhook_deliveries (
			id, subscription_id, event_type, payload, status,
			attempts, max_attempts, next_attempt_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`

	_, err = workspaceDB.ExecContext(ctx, query,
		delivery.ID,
		delivery.SubscriptionID,
		delivery.EventType,
		payloadJSON,
		delivery.Status,
		delivery.Attempts,
		delivery.MaxAttempts,
		delivery.NextAttemptAt,
		delivery.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create webhook delivery: %w", err)
	}

	return nil
}

// CleanupOldDeliveries deletes deliveries older than the specified retention period
func (r *webhookDeliveryRepository) CleanupOldDeliveries(ctx context.Context, workspaceID string, retentionDays int) (int64, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM webhook_deliveries WHERE created_at < NOW() - INTERVAL '1 day' * $1`

	result, err := workspaceDB.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old deliveries: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// DeleteBySubscriptionID removes every delivery belonging to one subscription.
//
// Deleting the subscription without this leaves its queued deliveries matching
// the pending predicate for the whole retention window, and each of them takes
// a slot in every batch the worker claims — a workspace that turns integrations
// on and off eventually fills its batch with rows that can never succeed and
// stops delivering anything at all.
func (r *webhookDeliveryRepository) DeleteBySubscriptionID(ctx context.Context, workspaceID, subscriptionID string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM webhook_deliveries WHERE subscription_id = $1`

	_, err = workspaceDB.ExecContext(ctx, query, subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to delete deliveries for subscription: %w", err)
	}

	return nil
}

// ReclaimStale returns rows claimed longer than lease ago to 'pending' and
// reports how many it moved.
//
// A worker that dies mid-delivery leaves its rows in 'delivering', where
// nothing selects them again — the same stranding DeleteBySubscriptionID exists
// to prevent, arrived at from the other direction. The lease is the caller's to
// choose because it belongs just past the HTTP timeout, which the caller owns:
// the request context is already cancelled by then, so a longer lease only
// delays recovery, and a lease measured in minutes would quietly swallow the
// first rungs of the retry ladder.
//
// A row in 'delivering' with no claimed_at is treated as infinitely stale. That
// covers rows stranded by a build that predates the claim, which would
// otherwise sit outside every predicate forever.
//
// Reclaiming is at-least-once by construction: a delivery whose POST succeeded
// but whose release write did not comes back and is sent again. That is the
// deliberate trade — a rare duplicate instead of a permanently stuck row.
func (r *webhookDeliveryRepository) ReclaimStale(ctx context.Context, workspaceID string, lease time.Duration) (int64, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE webhook_deliveries
		SET status = 'pending', claimed_at = NULL
		WHERE status = 'delivering'
			AND (claimed_at IS NULL OR claimed_at < NOW() - INTERVAL '1 second' * $1)
	`

	result, err := workspaceDB.ExecContext(ctx, query, lease.Seconds())
	if err != nil {
		return 0, fmt.Errorf("failed to reclaim stale deliveries: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// ReleaseClaim returns a claimed row to 'pending' without touching the delivery log.
//
// Deliberately not UpdateStatus. That statement writes last_attempt_at,
// last_response_status and last_response_body unconditionally, so releasing a
// row through it recorded an attempt that never left the process and wiped the
// receiver's last real status and body — exactly the two fields a user needs to
// debug the endpoint. attempts is untouched for the same reason: nothing was
// sent, so nothing was attempted, and spending one of ten attempts on our own
// database having a bad minute is how a transient outage turns into lost
// deliveries. next_attempt_at is already in the past, so the row rejoins the
// next batch.
//
// The predicate is RenewClaim's, and it is not optional. On `id` alone this
// statement released whatever state the row happened to be in rather than this
// caller's claim, and both ways that goes wrong end in the same duplicate POST
// the claim exists to prevent:
//
//   - the worker's recover releases unconditionally, so a panic AFTER
//     MarkDelivered had committed moved a delivered row back to 'pending' —
//     attempts below max_attempts, next_attempt_at already past — and the next
//     poll claimed and re-sent an event that had been delivered;
//   - a worker that stalls past its lease has its row reclaimed and handed to
//     another, and its late release then yanks that row out from under a POST
//     already in flight.
//
// Matching `status = 'delivering' AND claimed_at = $3` makes both of those match
// zero rows and change nothing, which is the right outcome for a claim that is
// no longer ours.
func (r *webhookDeliveryRepository) ReleaseClaim(ctx context.Context, workspaceID, id string, claimedAt *time.Time, lastError string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// No token, no release — and deliberately never a predicate-free one as the
	// fallback. A caller that cannot name the claim it is handing back cannot
	// prove it holds one, so the row is better left in 'delivering' with a NULL
	// claimed_at: ReclaimStale reads exactly that as infinitely stale and returns
	// it on its very next sweep, which is strictly safer than an UPDATE that
	// could land on a row another worker is delivering right now. Refusing here
	// rather than in SQL also keeps the predicate a plain equality, as RenewClaim
	// does.
	if claimedAt == nil {
		return nil
	}

	query := `
		UPDATE webhook_deliveries
		SET status = 'pending', claimed_at = NULL, last_error = $2
		WHERE id = $1 AND status = 'delivering' AND claimed_at = $3
	`

	if _, err := workspaceDB.ExecContext(ctx, query, id, lastError, *claimedAt); err != nil {
		return fmt.Errorf("failed to release delivery claim: %w", err)
	}

	return nil
}

// scanWebhookDeliveryFromRows scans a row from sql.Rows into a WebhookDelivery
func scanWebhookDeliveryFromRows(rows *sql.Rows) (*WebhookDelivery, error) {
	var delivery WebhookDelivery
	var payloadJSON []byte
	var lastAttemptAt sql.NullTime
	var deliveredAt sql.NullTime
	var lastResponseStatus sql.NullInt32
	var lastResponseBody sql.NullString
	var lastError sql.NullString
	var claimedAt sql.NullTime

	err := rows.Scan(
		&delivery.ID,
		&delivery.SubscriptionID,
		&delivery.EventType,
		&payloadJSON,
		&delivery.Status,
		&delivery.Attempts,
		&delivery.MaxAttempts,
		&delivery.NextAttemptAt,
		&lastAttemptAt,
		&deliveredAt,
		&lastResponseStatus,
		&lastResponseBody,
		&lastError,
		&claimedAt,
		&delivery.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan webhook delivery: %w", err)
	}

	if err := json.Unmarshal(payloadJSON, &delivery.Payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	if lastAttemptAt.Valid {
		delivery.LastAttemptAt = &lastAttemptAt.Time
	}
	if deliveredAt.Valid {
		delivery.DeliveredAt = &deliveredAt.Time
	}
	if lastResponseStatus.Valid {
		status := int(lastResponseStatus.Int32)
		delivery.LastResponseStatus = &status
	}
	if lastResponseBody.Valid {
		delivery.LastResponseBody = &lastResponseBody.String
	}
	if lastError.Valid {
		delivery.LastError = &lastError.String
	}
	if claimedAt.Valid {
		delivery.ClaimedAt = &claimedAt.Time
	}

	return &delivery, nil
}
