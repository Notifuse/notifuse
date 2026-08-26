package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// webhookSubscriptionColumns is the read projection shared by GetByID and List.
// Both scan positionally into the same helper, so keeping one column list is
// what stops a column added to a single query from turning into a runtime scan
// error that no compiler would catch.
const webhookSubscriptionColumns = `
		id, name, url, secret, settings,
		enabled, source, consecutive_failures, disabled_reason,
		created_at, updated_at, last_delivery_at, failing_since`

// webhookSubscriptionRepository implements domain.WebhookSubscriptionRepository for PostgreSQL
type webhookSubscriptionRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewWebhookSubscriptionRepository creates a new PostgreSQL webhook subscription repository
func NewWebhookSubscriptionRepository(workspaceRepo domain.WorkspaceRepository) domain.WebhookSubscriptionRepository {
	return &webhookSubscriptionRepository{
		workspaceRepo: workspaceRepo,
	}
}

// Create creates a new webhook subscription
func (r *webhookSubscriptionRepository) Create(ctx context.Context, workspaceID string, sub *WebhookSubscription) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()
	sub.CreatedAt = now
	sub.UpdatedAt = now

	// Marshal settings to JSON
	settingsJSON, err := json.Marshal(sub.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		INSERT INTO webhook_subscriptions (
			id, name, url, secret, settings,
			enabled, source, consecutive_failures, disabled_reason,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	_, err = workspaceDB.ExecContext(ctx, query,
		sub.ID,
		sub.Name,
		sub.URL,
		sub.Secret,
		settingsJSON,
		sub.Enabled,
		nullableSource(sub.Source),
		sub.ConsecutiveFailures,
		sub.DisabledReason,
		sub.CreatedAt,
		sub.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create webhook subscription: %w", err)
	}

	return nil
}

// GetByID retrieves a webhook subscription by ID. A row that does not exist is
// reported as an error wrapping domain.ErrWebhookSubscriptionNotFound; every
// other failure keeps its own cause, because the delivery worker decides
// whether to destroy a queued delivery on exactly that distinction.
func (r *webhookSubscriptionRepository) GetByID(ctx context.Context, workspaceID, id string) (*WebhookSubscription, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT ` + webhookSubscriptionColumns + `
		FROM webhook_subscriptions
		WHERE id = $1
	`

	row := workspaceDB.QueryRowContext(ctx, query, id)
	return scanWebhookSubscription(row, id)
}

// List retrieves all webhook subscriptions for a workspace
func (r *webhookSubscriptionRepository) List(ctx context.Context, workspaceID string) ([]*WebhookSubscription, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT ` + webhookSubscriptionColumns + `
		FROM webhook_subscriptions
		ORDER BY created_at DESC
	`

	rows, err := workspaceDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhook subscriptions: %w", err)
	}
	defer rows.Close()

	var subscriptions []*WebhookSubscription
	for rows.Next() {
		sub, err := scanWebhookSubscriptionFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan webhook subscription: %w", err)
		}
		subscriptions = append(subscriptions, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating webhook subscriptions: %w", err)
	}

	return subscriptions, nil
}

// Update updates an existing webhook subscription
func (r *webhookSubscriptionRepository) Update(ctx context.Context, workspaceID string, sub *WebhookSubscription) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	sub.UpdatedAt = time.Now().UTC()

	// Marshal settings to JSON
	settingsJSON, err := json.Marshal(sub.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// source is deliberately missing from this SET list. It records who created
	// the subscription and is written once, by Create: letting an edit change it
	// would let a Zapier-created row be relabelled as hand-made, which is what
	// the console badge and the delete-versus-disable branch on a dead endpoint
	// both key off.
	//
	// consecutive_failures, failing_since and disabled_reason are missing for a
	// sharper reason: they do not belong to the caller at all. The delivery
	// worker owns them and writes them one column at a time, in SQL, precisely so
	// concurrent failures cannot lose counts (see IncrementFailures). This
	// statement carries a snapshot read at the top of somebody's request, so
	// writing them here published a value that was already stale — an owner
	// rotating a secret while the worker was retiring the endpoint restored the
	// counter it had reached and erased the reason it had recorded, and any
	// client that touches its subscriptions on a timer re-armed failing_since
	// forever, so the endpoint could never be retired at all.
	//
	// The one legitimate user write to those three is clearing them when the
	// subscription is switched back ON, which is a statement that the endpoint
	// has been fixed. That is expressed here as a CASE against the row's own
	// current `enabled`, so it fires on the real off-to-on transition rather than
	// on whatever the caller happened to read a moment earlier.
	query := `
		UPDATE webhook_subscriptions
		SET name = $2, url = $3, secret = $4, settings = $5,
			enabled = $6,
			consecutive_failures = CASE WHEN $6 AND NOT enabled THEN 0 ELSE consecutive_failures END,
			failing_since = CASE WHEN $6 AND NOT enabled THEN NULL ELSE failing_since END,
			disabled_reason = CASE WHEN $6 AND NOT enabled THEN NULL ELSE disabled_reason END,
			updated_at = $7
		WHERE id = $1
	`

	result, err := workspaceDB.ExecContext(ctx, query,
		sub.ID,
		sub.Name,
		sub.URL,
		sub.Secret,
		settingsJSON,
		sub.Enabled,
		sub.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update webhook subscription: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("webhook subscription %s: %w", sub.ID, domain.ErrWebhookSubscriptionNotFound)
	}

	return nil
}

// Delete deletes a webhook subscription
func (r *webhookSubscriptionRepository) Delete(ctx context.Context, workspaceID, id string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM webhook_subscriptions WHERE id = $1`

	result, err := workspaceDB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete webhook subscription: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("webhook subscription %s: %w", id, domain.ErrWebhookSubscriptionNotFound)
	}

	return nil
}

// UpdateLastDeliveryAt updates the last delivery timestamp
func (r *webhookSubscriptionRepository) UpdateLastDeliveryAt(ctx context.Context, workspaceID, id string, deliveredAt time.Time) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `UPDATE webhook_subscriptions SET last_delivery_at = $2 WHERE id = $1`

	_, err = workspaceDB.ExecContext(ctx, query, id, deliveredAt)
	if err != nil {
		return fmt.Errorf("failed to update last delivery timestamp: %w", err)
	}

	return nil
}

// IncrementFailures bumps the consecutive-failure counter by one.
//
// The increment is computed by the database rather than in Go on purpose:
// several deliveries for one subscription can be in flight at once, and a
// counter written back from a value the worker read earlier would lose every
// increment but the last — which is the whole signal the auto-disable threshold
// rests on.
//
// Like the other bookkeeping writes here, a subscription that no longer exists
// is not an error: it means the row was deleted while a delivery was in flight,
// and the delivery it belonged to is already on its way out with it.
func (r *webhookSubscriptionRepository) IncrementFailures(ctx context.Context, workspaceID, id string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// COALESCE, not NOW(): failing_since marks the START of the current run of
	// failures, so only the first failure after a success may set it. Re-stamping
	// it on every failure would make the window it measures permanently zero and
	// give the threshold nothing to wait for.
	//
	// Ending a run that has grown too old to still be one is deliberately NOT
	// done here. It needs the run's maximum age, which only makes sense beside
	// the failure window and the retry ladder it is derived from — all of which
	// live in the delivery worker. The worker clears the run through
	// ResetFailures before this increment, so the COALESCE below opens the new
	// one. See expireStaleFailureRun.
	query := `
		UPDATE webhook_subscriptions
		SET consecutive_failures = consecutive_failures + 1,
			failing_since = COALESCE(failing_since, NOW())
		WHERE id = $1`

	_, err = workspaceDB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment webhook subscription failures: %w", err)
	}

	return nil
}

// ResetFailures returns the counter to zero after a successful delivery.
//
// The counter is almost always already zero — a healthy subscription succeeds
// on every delivery — so the write is guarded to skip those rows. Without the
// guard every successful delivery on a busy workspace writes a new row version
// of the same subscription, purely to store the value it already held.
func (r *webhookSubscriptionRepository) ResetFailures(ctx context.Context, workspaceID, id string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE webhook_subscriptions
		SET consecutive_failures = 0, failing_since = NULL
		WHERE id = $1 AND (consecutive_failures <> 0 OR failing_since IS NOT NULL)`

	_, err = workspaceDB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to reset webhook subscription failures: %w", err)
	}

	return nil
}

// DisableWithReason switches the subscription off and records why in the same
// statement, so a reader can never observe a subscription that has been
// disabled automatically without the explanation that goes with it — the only
// thing that tells a user this was us and not them.
func (r *webhookSubscriptionRepository) DisableWithReason(ctx context.Context, workspaceID, id, reason string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `UPDATE webhook_subscriptions SET enabled = false, disabled_reason = $2, updated_at = $3 WHERE id = $1`

	_, err = workspaceDB.ExecContext(ctx, query, id, reason, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to disable webhook subscription: %w", err)
	}

	return nil
}

// WebhookSubscription alias for domain type
type WebhookSubscription = domain.WebhookSubscription

// nullableSource maps the empty source — a subscription a user created by hand
// — onto SQL NULL. Storing ” instead would give the column two spellings of
// "nobody created this on the user's behalf", and every query that asks for
// hand-made subscriptions would have to know about both.
func nullableSource(source string) interface{} {
	if source == "" {
		return nil
	}
	return source
}

// webhookSubscriptionRowScanner is satisfied by both *sql.Row and *sql.Rows, so
// the single-row and multi-row reads share one scan body and cannot drift into
// disagreeing about the column order.
type webhookSubscriptionRowScanner interface {
	Scan(dest ...interface{}) error
}

// scanWebhookSubscription scans a single row into a WebhookSubscription,
// translating a missing row into domain.ErrWebhookSubscriptionNotFound. The
// sentinel is wrapped rather than returned bare so the id stays in the message,
// and a scan failure is wrapped by scanWebhookSubscriptionInto instead — the
// two must stay distinguishable by errors.Is, because a caller that cannot tell
// them apart has to treat a momentary database failure as a deletion.
func scanWebhookSubscription(row *sql.Row, id string) (*WebhookSubscription, error) {
	sub, err := scanWebhookSubscriptionInto(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("webhook subscription %s: %w", id, domain.ErrWebhookSubscriptionNotFound)
		}
		return nil, err
	}

	return sub, nil
}

// scanWebhookSubscriptionFromRows scans a row from sql.Rows into a WebhookSubscription
func scanWebhookSubscriptionFromRows(rows *sql.Rows) (*WebhookSubscription, error) {
	return scanWebhookSubscriptionInto(rows)
}

func scanWebhookSubscriptionInto(scanner webhookSubscriptionRowScanner) (*WebhookSubscription, error) {
	var sub WebhookSubscription
	var settingsJSON []byte
	var source sql.NullString
	var disabledReason sql.NullString
	var lastDeliveryAt sql.NullTime
	var failingSince sql.NullTime

	err := scanner.Scan(
		&sub.ID,
		&sub.Name,
		&sub.URL,
		&sub.Secret,
		&settingsJSON,
		&sub.Enabled,
		&source,
		&sub.ConsecutiveFailures,
		&disabledReason,
		&sub.CreatedAt,
		&sub.UpdatedAt,
		&lastDeliveryAt,
		&failingSince,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan webhook subscription: %w", err)
	}

	// NULL and '' are the same thing to a caller: nobody created this on the
	// user's behalf. See nullableSource for the write side.
	sub.Source = source.String

	if disabledReason.Valid {
		sub.DisabledReason = &disabledReason.String
	}

	if lastDeliveryAt.Valid {
		sub.LastDeliveryAt = &lastDeliveryAt.Time
	}

	if failingSince.Valid {
		sub.FailingSince = &failingSince.Time
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &sub.Settings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
		}
	}

	return &sub, nil
}
