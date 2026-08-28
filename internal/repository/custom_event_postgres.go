package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

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

	propertiesJSON, propertiesProvided, err := encodeEventProperties(event)
	if err != nil {
		return err
	}

	// UPSERT: Insert new event or update if (event_name, external_id) exists
	// Updates when: new occurred_at is more recent OR deleted_at changed
	query := upsertCustomEventQuery

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
		propertiesProvided,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert custom event: %w", err)
	}

	return nil
}

// upsertCustomEventQuery is shared by Upsert and BatchUpsert so the two paths
// cannot drift on which columns a partial write is allowed to overwrite.
//
// properties holds the entire current state of the external resource, and $14 says
// whether the caller mentioned it at all. EXCLUDED.properties cannot answer that
// on its own: an absent key and an explicit {} both arrive as an empty object, and
// the column is NOT NULL so a proposed row cannot carry a NULL to COALESCE away.
// Overwriting on absence is not a lost edit but a wipe — it clears every property
// at once, and because it lands as an UPDATE the timeline trigger records each one
// going null, which feeds segment recomputation and automation enrolment.
//
// integration_id needs no such flag: it is already a nullable pointer, so it joins
// the goal_* columns on COALESCE. An event created by an integration keeps that
// link when a later API call says nothing about it.
//
// source is missing from the DO UPDATE SET list on purpose. It records where a row
// came from, and webhook_custom_events_trigger gates its whole web-analytics
// exclusion on it, so rewriting it turns a bridged analytics row into an API row
// and starts fanning pageview-scale conversions — client-supplied properties
// included — out to third-party subscribers that asked for commerce events. No
// caller is asking for that either: UpsertCustomEventRequest has no source field at
// all, the service stamps "api" only so a first write records an origin, and an
// import entry's source describes the entry rather than the resource it names. A
// row's origin is set by the write that created it and never moved afterwards.
const upsertCustomEventQuery = `
		INSERT INTO custom_events (
			event_name, external_id, email, properties, occurred_at,
			source, integration_id, goal_name, goal_type, goal_value,
			deleted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (event_name, external_id) DO UPDATE SET
			email = EXCLUDED.email,
			properties = CASE WHEN $14::boolean
				THEN EXCLUDED.properties
				ELSE custom_events.properties
			END,
			occurred_at = CASE
				WHEN EXCLUDED.occurred_at > custom_events.occurred_at
				THEN EXCLUDED.occurred_at
				ELSE custom_events.occurred_at
			END,
			-- source is not assigned here; see the comment above the constant.
			integration_id = COALESCE(EXCLUDED.integration_id, custom_events.integration_id),
			goal_name = COALESCE(EXCLUDED.goal_name, custom_events.goal_name),
			goal_type = COALESCE(EXCLUDED.goal_type, custom_events.goal_type),
			goal_value = COALESCE(EXCLUDED.goal_value, custom_events.goal_value),
			deleted_at = EXCLUDED.deleted_at,
			updated_at = NOW()
		WHERE EXCLUDED.occurred_at > custom_events.occurred_at
		   OR EXCLUDED.deleted_at IS DISTINCT FROM custom_events.deleted_at
	`

// encodeEventProperties reports whether the write claims the properties column,
// and hands back a payload that is safe to insert either way. A nil map means the
// caller said nothing, so the row proposed for insertion carries an empty object —
// right for a brand-new event, and ignored by the ON CONFLICT clause for an
// existing one.
func encodeEventProperties(event *domain.CustomEvent) (payload []byte, provided bool, err error) {
	if event.Properties == nil {
		return []byte("{}"), false, nil
	}

	payload, err = json.Marshal(event.Properties)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal properties: %w", err)
	}
	return payload, true, nil
}

// BatchInsertNew inserts events that do not already exist and leaves existing
// rows entirely alone.
//
// DO NOTHING rather than the mutable DO UPDATE that BatchUpsert uses. The
// custom_events timeline trigger fires on INSERT *or* UPDATE and does no
// diffing, so re-writing an unchanged event appends a second contact_timeline
// row, queues another segment recomputation and re-enrols the contact in any
// matching automation. For a source that re-sends its whole history on every
// beat, that turns one conversion into an endless stream.
//
// It also protects against two subtler cases the mutable clause allows: an
// occurred_at that drifts between beats (clock-skew correction is recomputed
// per beat) would satisfy the "newer timestamp" guard, and a NULL deleted_at
// would resurrect an event an admin had deliberately removed.
func (r *customEventRepository) BatchInsertNew(ctx context.Context, workspaceID string, events []*domain.CustomEvent) error {
	if len(events) == 0 {
		return nil
	}

	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO custom_events (
			event_name, external_id, email, properties, occurred_at,
			source, integration_id, goal_name, goal_type, goal_value,
			deleted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (event_name, external_id) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, event := range events {
		// The provided flag has nothing to decide here: DO NOTHING never touches
		// an existing row, so a nil map only ever inserts an empty object.
		properties, _, err := encodeEventProperties(event)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			event.EventName, event.ExternalID, event.Email, properties, event.OccurredAt,
			event.Source, event.IntegrationID, event.GoalName, event.GoalType, event.GoalValue,
			event.DeletedAt, now, now,
		); err != nil {
			return fmt.Errorf("failed to insert custom event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
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

	stmt, err := tx.PrepareContext(ctx, upsertCustomEventQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		propertiesJSON, propertiesProvided, err := encodeEventProperties(event)
		if err != nil {
			return fmt.Errorf("event %s: %w", event.ExternalID, err)
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
			propertiesProvided,
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
