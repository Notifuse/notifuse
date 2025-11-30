package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// webhookDeliveryRepository implements domain.WebhookDeliveryRepository for PostgreSQL
type webhookDeliveryRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewWebhookDeliveryRepository creates a new PostgreSQL webhook delivery repository
func NewWebhookDeliveryRepository(workspaceRepo domain.WorkspaceRepository) domain.WebhookDeliveryRepository {
	return &webhookDeliveryRepository{
		workspaceRepo: workspaceRepo,
	}
}

// WebhookDelivery alias for domain type
type WebhookDelivery = domain.WebhookDelivery

// GetPendingForWorkspace retrieves pending deliveries for a specific workspace
func (r *webhookDeliveryRepository) GetPendingForWorkspace(ctx context.Context, workspaceID string, limit int) ([]*WebhookDelivery, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT
			id, subscription_id, event_type, payload, status,
			attempts, max_attempts, next_attempt_at, last_attempt_at,
			delivered_at, last_response_status, last_response_body, last_error, created_at
		FROM webhook_deliveries
		WHERE status IN ('pending', 'failed')
			AND attempts < max_attempts
			AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at ASC
		LIMIT $1
	`

	rows, err := workspaceDB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*WebhookDelivery
	for rows.Next() {
		delivery, err := scanWebhookDeliveryFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan delivery: %w", err)
		}
		deliveries = append(deliveries, delivery)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deliveries: %w", err)
	}

	return deliveries, nil
}

// ListBySubscription retrieves deliveries for a specific subscription with pagination
func (r *webhookDeliveryRepository) ListBySubscription(ctx context.Context, workspaceID, subscriptionID string, limit, offset int) ([]*WebhookDelivery, int, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM webhook_deliveries WHERE subscription_id = $1`
	var total int
	err = workspaceDB.QueryRowContext(ctx, countQuery, subscriptionID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count deliveries: %w", err)
	}

	// Get deliveries
	query := `
		SELECT
			id, subscription_id, event_type, payload, status,
			attempts, max_attempts, next_attempt_at, last_attempt_at,
			delivered_at, last_response_status, last_response_body, last_error, created_at
		FROM webhook_deliveries
		WHERE subscription_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := workspaceDB.QueryContext(ctx, query, subscriptionID, limit, offset)
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

	query := `
		UPDATE webhook_deliveries
		SET status = $2, attempts = $3, last_attempt_at = $4,
			last_response_status = $5, last_response_body = $6, last_error = $7
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
			attempts = attempts + 1, last_response_status = $3, last_response_body = $4
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

	query := `
		UPDATE webhook_deliveries
		SET status = 'failed', attempts = $2, next_attempt_at = $3, last_attempt_at = $4,
			last_response_status = $5, last_response_body = $6, last_error = $7
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
			last_response_status = $4, last_response_body = $5, last_error = $6
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

// scanWebhookDeliveryFromRows scans a row from sql.Rows into a WebhookDelivery
func scanWebhookDeliveryFromRows(rows *sql.Rows) (*WebhookDelivery, error) {
	var delivery WebhookDelivery
	var payloadJSON []byte
	var lastAttemptAt sql.NullTime
	var deliveredAt sql.NullTime
	var lastResponseStatus sql.NullInt32
	var lastResponseBody sql.NullString
	var lastError sql.NullString

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

	return &delivery, nil
}
