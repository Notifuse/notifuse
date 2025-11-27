package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

type customEventRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

func NewCustomEventRepository(workspaceRepo domain.WorkspaceRepository) domain.CustomEventRepository {
	return &customEventRepository{
		workspaceRepo: workspaceRepo,
	}
}

// Upsert creates or updates a custom event with goal tracking and soft-delete support
func (r *customEventRepository) Upsert(ctx context.Context, workspaceID string, event *domain.CustomEvent) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	propertiesJSON, err := json.Marshal(event.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	// UPSERT: Insert new event or update if (event_name, external_id) exists
	// Updates when: new occurred_at is more recent OR deleted_at changed
	query := `
		INSERT INTO custom_events (
			event_name, external_id, email, properties, occurred_at,
			source, integration_id, goal_name, goal_type, goal_value,
			deleted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (event_name, external_id) DO UPDATE SET
			email = EXCLUDED.email,
			properties = EXCLUDED.properties,
			occurred_at = CASE
				WHEN EXCLUDED.occurred_at > custom_events.occurred_at
				THEN EXCLUDED.occurred_at
				ELSE custom_events.occurred_at
			END,
			source = EXCLUDED.source,
			integration_id = EXCLUDED.integration_id,
			goal_name = COALESCE(EXCLUDED.goal_name, custom_events.goal_name),
			goal_type = COALESCE(EXCLUDED.goal_type, custom_events.goal_type),
			goal_value = COALESCE(EXCLUDED.goal_value, custom_events.goal_value),
			deleted_at = EXCLUDED.deleted_at,
			updated_at = NOW()
		WHERE EXCLUDED.occurred_at > custom_events.occurred_at
		   OR EXCLUDED.deleted_at IS DISTINCT FROM custom_events.deleted_at
	`

	_, err = db.ExecContext(ctx, query,
		event.EventName,
		event.ExternalID,
		event.Email,
		propertiesJSON,
		event.OccurredAt,
		event.Source,
		event.IntegrationID,
		event.GoalName,
		event.GoalType,
		event.GoalValue,
		event.DeletedAt,
		event.CreatedAt,
		event.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert custom event: %w", err)
	}

	return nil
}

// BatchUpsert creates or updates multiple custom events with goal tracking and soft-delete support
func (r *customEventRepository) BatchUpsert(ctx context.Context, workspaceID string, events []*domain.CustomEvent) error {
	if len(events) == 0 {
		return nil
	}

	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Use transaction for batch upsert
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO custom_events (
			event_name, external_id, email, properties, occurred_at,
			source, integration_id, goal_name, goal_type, goal_value,
			deleted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (event_name, external_id) DO UPDATE SET
			email = EXCLUDED.email,
			properties = EXCLUDED.properties,
			occurred_at = CASE
				WHEN EXCLUDED.occurred_at > custom_events.occurred_at
				THEN EXCLUDED.occurred_at
				ELSE custom_events.occurred_at
			END,
			source = EXCLUDED.source,
			integration_id = EXCLUDED.integration_id,
			goal_name = COALESCE(EXCLUDED.goal_name, custom_events.goal_name),
			goal_type = COALESCE(EXCLUDED.goal_type, custom_events.goal_type),
			goal_value = COALESCE(EXCLUDED.goal_value, custom_events.goal_value),
			deleted_at = EXCLUDED.deleted_at,
			updated_at = NOW()
		WHERE EXCLUDED.occurred_at > custom_events.occurred_at
		   OR EXCLUDED.deleted_at IS DISTINCT FROM custom_events.deleted_at
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		propertiesJSON, err := json.Marshal(event.Properties)
		if err != nil {
			return fmt.Errorf("failed to marshal properties for event %s: %w", event.ExternalID, err)
		}

		_, err = stmt.ExecContext(ctx,
			event.EventName,
			event.ExternalID,
			event.Email,
			propertiesJSON,
			event.OccurredAt,
			event.Source,
			event.IntegrationID,
			event.GoalName,
			event.GoalType,
			event.GoalValue,
			event.DeletedAt,
			event.CreatedAt,
			event.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert event %s: %w", event.ExternalID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *customEventRepository) GetByID(ctx context.Context, workspaceID, eventName, externalID string) (*domain.CustomEvent, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT event_name, external_id, email, properties, occurred_at,
		       source, integration_id, goal_name, goal_type, goal_value,
		       deleted_at, created_at, updated_at
		FROM custom_events
		WHERE event_name = $1 AND external_id = $2 AND deleted_at IS NULL
	`

	var event domain.CustomEvent
	var propertiesJSON []byte
	var integrationID sql.NullString
	var goalName sql.NullString
	var goalType sql.NullString
	var goalValue sql.NullFloat64
	var deletedAt sql.NullTime

	err = db.QueryRowContext(ctx, query, eventName, externalID).Scan(
		&event.EventName,
		&event.ExternalID,
		&event.Email,
		&propertiesJSON,
		&event.OccurredAt,
		&event.Source,
		&integrationID,
		&goalName,
		&goalType,
		&goalValue,
		&deletedAt,
		&event.CreatedAt,
		&event.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("custom event not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get custom event: %w", err)
	}

	if integrationID.Valid {
		event.IntegrationID = &integrationID.String
	}
	if goalName.Valid {
		event.GoalName = &goalName.String
	}
	if goalType.Valid {
		event.GoalType = &goalType.String
	}
	if goalValue.Valid {
		event.GoalValue = &goalValue.Float64
	}
	if deletedAt.Valid {
		event.DeletedAt = &deletedAt.Time
	}

	if err := json.Unmarshal(propertiesJSON, &event.Properties); err != nil {
		return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
	}

	return &event, nil
}

func (r *customEventRepository) ListByEmail(ctx context.Context, workspaceID, email string, limit int, offset int) ([]*domain.CustomEvent, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT event_name, external_id, email, properties, occurred_at,
		       source, integration_id, goal_name, goal_type, goal_value,
		       deleted_at, created_at, updated_at
		FROM custom_events
		WHERE email = $1 AND deleted_at IS NULL
		ORDER BY occurred_at DESC
		LIMIT $2 OFFSET $3
	`

	return r.scanEvents(ctx, db, query, email, limit, offset)
}

func (r *customEventRepository) ListByEventName(ctx context.Context, workspaceID, eventName string, limit int, offset int) ([]*domain.CustomEvent, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT event_name, external_id, email, properties, occurred_at,
		       source, integration_id, goal_name, goal_type, goal_value,
		       deleted_at, created_at, updated_at
		FROM custom_events
		WHERE event_name = $1 AND deleted_at IS NULL
		ORDER BY occurred_at DESC
		LIMIT $2 OFFSET $3
	`

	return r.scanEvents(ctx, db, query, eventName, limit, offset)
}

func (r *customEventRepository) DeleteForEmail(ctx context.Context, workspaceID, email string) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM custom_events WHERE email = $1`

	_, err = db.ExecContext(ctx, query, email)
	if err != nil {
		return fmt.Errorf("failed to delete custom events: %w", err)
	}

	return nil
}

// Helper function to scan events from query results
func (r *customEventRepository) scanEvents(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]*domain.CustomEvent, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom events: %w", err)
	}
	defer rows.Close()

	var events []*domain.CustomEvent
	for rows.Next() {
		var event domain.CustomEvent
		var propertiesJSON []byte
		var integrationID sql.NullString
		var goalName sql.NullString
		var goalType sql.NullString
		var goalValue sql.NullFloat64
		var deletedAt sql.NullTime

		err := rows.Scan(
			&event.EventName,
			&event.ExternalID,
			&event.Email,
			&propertiesJSON,
			&event.OccurredAt,
			&event.Source,
			&integrationID,
			&goalName,
			&goalType,
			&goalValue,
			&deletedAt,
			&event.CreatedAt,
			&event.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan custom event: %w", err)
		}

		if integrationID.Valid {
			event.IntegrationID = &integrationID.String
		}
		if goalName.Valid {
			event.GoalName = &goalName.String
		}
		if goalType.Valid {
			event.GoalType = &goalType.String
		}
		if goalValue.Valid {
			event.GoalValue = &goalValue.Float64
		}
		if deletedAt.Valid {
			event.DeletedAt = &deletedAt.Time
		}

		if err := json.Unmarshal(propertiesJSON, &event.Properties); err != nil {
			return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
		}

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating custom events: %w", err)
	}

	return events, nil
}
