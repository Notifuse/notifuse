package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
)

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

	var customFiltersJSON []byte
	if sub.CustomEventFilters != nil {
		customFiltersJSON, err = json.Marshal(sub.CustomEventFilters)
		if err != nil {
			return fmt.Errorf("failed to marshal custom event filters: %w", err)
		}
	}

	query := `
		INSERT INTO webhook_subscriptions (
			id, name, url, secret, event_types, custom_event_filters,
			enabled, description, created_at, updated_at,
			success_count, failure_count
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`

	_, err = workspaceDB.ExecContext(ctx, query,
		sub.ID,
		sub.Name,
		sub.URL,
		sub.Secret,
		pq.Array(sub.EventTypes),
		customFiltersJSON,
		sub.Enabled,
		sub.Description,
		sub.CreatedAt,
		sub.UpdatedAt,
		sub.SuccessCount,
		sub.FailureCount,
	)

	if err != nil {
		return fmt.Errorf("failed to create webhook subscription: %w", err)
	}

	return nil
}

// GetByID retrieves a webhook subscription by ID
func (r *webhookSubscriptionRepository) GetByID(ctx context.Context, workspaceID, id string) (*WebhookSubscription, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT
			id, name, url, secret, event_types, custom_event_filters,
			enabled, description, created_at, updated_at,
			last_delivery_at, success_count, failure_count
		FROM webhook_subscriptions
		WHERE id = $1
	`

	row := workspaceDB.QueryRowContext(ctx, query, id)
	return scanWebhookSubscription(row)
}

// List retrieves all webhook subscriptions for a workspace
func (r *webhookSubscriptionRepository) List(ctx context.Context, workspaceID string) ([]*WebhookSubscription, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT
			id, name, url, secret, event_types, custom_event_filters,
			enabled, description, created_at, updated_at,
			last_delivery_at, success_count, failure_count
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

	var customFiltersJSON []byte
	if sub.CustomEventFilters != nil {
		customFiltersJSON, err = json.Marshal(sub.CustomEventFilters)
		if err != nil {
			return fmt.Errorf("failed to marshal custom event filters: %w", err)
		}
	}

	query := `
		UPDATE webhook_subscriptions
		SET name = $2, url = $3, secret = $4, event_types = $5,
			custom_event_filters = $6, enabled = $7, description = $8, updated_at = $9
		WHERE id = $1
	`

	result, err := workspaceDB.ExecContext(ctx, query,
		sub.ID,
		sub.Name,
		sub.URL,
		sub.Secret,
		pq.Array(sub.EventTypes),
		customFiltersJSON,
		sub.Enabled,
		sub.Description,
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
		return fmt.Errorf("webhook subscription not found: %s", sub.ID)
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
		return fmt.Errorf("webhook subscription not found: %s", id)
	}

	return nil
}

// IncrementStats increments success or failure count for a subscription
func (r *webhookSubscriptionRepository) IncrementStats(ctx context.Context, workspaceID, id string, success bool) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	var query string
	if success {
		query = `UPDATE webhook_subscriptions SET success_count = success_count + 1 WHERE id = $1`
	} else {
		query = `UPDATE webhook_subscriptions SET failure_count = failure_count + 1 WHERE id = $1`
	}

	_, err = workspaceDB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment webhook stats: %w", err)
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

// WebhookSubscription alias for domain type
type WebhookSubscription = domain.WebhookSubscription

// scanWebhookSubscription scans a single row into a WebhookSubscription
func scanWebhookSubscription(row *sql.Row) (*WebhookSubscription, error) {
	var sub WebhookSubscription
	var customFiltersJSON []byte
	var lastDeliveryAt sql.NullTime

	err := row.Scan(
		&sub.ID,
		&sub.Name,
		&sub.URL,
		&sub.Secret,
		pq.Array(&sub.EventTypes),
		&customFiltersJSON,
		&sub.Enabled,
		&sub.Description,
		&sub.CreatedAt,
		&sub.UpdatedAt,
		&lastDeliveryAt,
		&sub.SuccessCount,
		&sub.FailureCount,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("webhook subscription not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan webhook subscription: %w", err)
	}

	if lastDeliveryAt.Valid {
		sub.LastDeliveryAt = &lastDeliveryAt.Time
	}

	if len(customFiltersJSON) > 0 {
		var filters domain.CustomEventFilters
		if err := json.Unmarshal(customFiltersJSON, &filters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal custom event filters: %w", err)
		}
		sub.CustomEventFilters = &filters
	}

	return &sub, nil
}

// scanWebhookSubscriptionFromRows scans a row from sql.Rows into a WebhookSubscription
func scanWebhookSubscriptionFromRows(rows *sql.Rows) (*WebhookSubscription, error) {
	var sub WebhookSubscription
	var customFiltersJSON []byte
	var lastDeliveryAt sql.NullTime

	err := rows.Scan(
		&sub.ID,
		&sub.Name,
		&sub.URL,
		&sub.Secret,
		pq.Array(&sub.EventTypes),
		&customFiltersJSON,
		&sub.Enabled,
		&sub.Description,
		&sub.CreatedAt,
		&sub.UpdatedAt,
		&lastDeliveryAt,
		&sub.SuccessCount,
		&sub.FailureCount,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan webhook subscription: %w", err)
	}

	if lastDeliveryAt.Valid {
		sub.LastDeliveryAt = &lastDeliveryAt.Time
	}

	if len(customFiltersJSON) > 0 {
		var filters domain.CustomEventFilters
		if err := json.Unmarshal(customFiltersJSON, &filters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal custom event filters: %w", err)
		}
		sub.CustomEventFilters = &filters
	}

	return &sub, nil
}
