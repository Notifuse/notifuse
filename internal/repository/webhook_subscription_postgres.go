package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
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

	// Marshal settings to JSON
	settingsJSON, err := json.Marshal(sub.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		INSERT INTO webhook_subscriptions (
			id, name, url, secret, settings,
			enabled, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`

	_, err = workspaceDB.ExecContext(ctx, query,
		sub.ID,
		sub.Name,
		sub.URL,
		sub.Secret,
		settingsJSON,
		sub.Enabled,
		sub.CreatedAt,
		sub.UpdatedAt,
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
			id, name, url, secret, settings,
			enabled, created_at, updated_at,
			last_delivery_at
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
			id, name, url, secret, settings,
			enabled, created_at, updated_at,
			last_delivery_at
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

	query := `
		UPDATE webhook_subscriptions
		SET name = $2, url = $3, secret = $4, settings = $5,
			enabled = $6, updated_at = $7
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
	var settingsJSON []byte
	var lastDeliveryAt sql.NullTime

	err := row.Scan(
		&sub.ID,
		&sub.Name,
		&sub.URL,
		&sub.Secret,
		&settingsJSON,
		&sub.Enabled,
		&sub.CreatedAt,
		&sub.UpdatedAt,
		&lastDeliveryAt,
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

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &sub.Settings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
		}
	}

	return &sub, nil
}

// scanWebhookSubscriptionFromRows scans a row from sql.Rows into a WebhookSubscription
func scanWebhookSubscriptionFromRows(rows *sql.Rows) (*WebhookSubscription, error) {
	var sub WebhookSubscription
	var settingsJSON []byte
	var lastDeliveryAt sql.NullTime

	err := rows.Scan(
		&sub.ID,
		&sub.Name,
		&sub.URL,
		&sub.Secret,
		&settingsJSON,
		&sub.Enabled,
		&sub.CreatedAt,
		&sub.UpdatedAt,
		&lastDeliveryAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan webhook subscription: %w", err)
	}

	if lastDeliveryAt.Valid {
		sub.LastDeliveryAt = &lastDeliveryAt.Time
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &sub.Settings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
		}
	}

	return &sub, nil
}
