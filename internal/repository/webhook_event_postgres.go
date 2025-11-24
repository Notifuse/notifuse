package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

type webhookEventRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewWebhookEventRepository creates a new PostgreSQL repository for webhook events
func NewWebhookEventRepository(workspaceRepo domain.WorkspaceRepository) domain.WebhookEventRepository {
	return &webhookEventRepository{
		workspaceRepo: workspaceRepo,
	}
}

// StoreEvents stores multiple webhook events in the database as a batch
func (r *webhookEventRepository) StoreEvents(ctx context.Context, workspaceID string, events []*domain.WebhookEvent) error {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "WebhookEventRepository", "StoreEvents")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	tracing.AddAttribute(ctx, "eventCount", len(events))
	// codecov:ignore:end

	if len(events) == 0 {
		return nil
	}

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Use multi-value INSERT for maximum batch efficiency
	baseSQL := `
		INSERT INTO webhook_events (
			id, type, source, integration_id, recipient_email, 
			message_id, timestamp, raw_payload,
			bounce_type, bounce_category, bounce_diagnostic, complaint_feedback_type,
			created_at
		) VALUES `

	// Generate placeholders for all events
	placeholders := make([]string, len(events))
	now := time.Now().UTC()

	// Batch size limit to avoid hitting Postgres parameter limits (max 65535 parameters)
	const batchSize = 1000 // Each event uses 13 parameters, so ~5000 events would hit the limit

	// Process in batches
	for i := 0; i < len(events); i += batchSize {
		end := i + batchSize
		if end > len(events) {
			end = len(events)
		}

		currentBatch := events[i:end]
		args := make([]interface{}, 0, len(currentBatch)*13)

		// Generate placeholders and collect args for this batch
		for j, event := range currentBatch {
			paramOffset := j * 13
			placeholders[j] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				paramOffset+1, paramOffset+2, paramOffset+3, paramOffset+4, paramOffset+5,
				paramOffset+6, paramOffset+7, paramOffset+8, paramOffset+9, paramOffset+10,
				paramOffset+11, paramOffset+12, paramOffset+13)

			args = append(args,
				event.ID,
				event.Type,
				event.Source,
				event.IntegrationID,
				event.RecipientEmail,
				event.MessageID,
				event.Timestamp,
				event.RawPayload,
				event.BounceType,
				event.BounceCategory,
				event.BounceDiagnostic,
				event.ComplaintFeedbackType,
				now,
			)
		}

		// Build and execute the SQL for this batch
		batchSQL := baseSQL + strings.Join(placeholders[:len(currentBatch)], ",") + " ON CONFLICT (id) DO NOTHING"
		_, err = workspaceDB.ExecContext(ctx, batchSQL, args...)

		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return fmt.Errorf("failed to store webhook events batch: %w", err)
		}
	}

	return nil
}

// StoreEvent stores a single webhook event in the database
func (r *webhookEventRepository) StoreEvent(ctx context.Context, workspaceID string, event *domain.WebhookEvent) error {
	return r.StoreEvents(ctx, workspaceID, []*domain.WebhookEvent{event})
}

// ListEvents retrieves all webhook events for a workspace
func (r *webhookEventRepository) ListEvents(ctx context.Context, workspaceID string, params domain.WebhookEventListParams) (*domain.WebhookEventListResult, error) {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "WebhookEventRepository", "ListEvents")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	// codecov:ignore:end

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Use squirrel to build the query with placeholders
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	queryBuilder := psql.Select(
		"id", "type", "source", "integration_id", "recipient_email",
		"message_id", "timestamp", "raw_payload",
		"bounce_type", "bounce_category", "bounce_diagnostic", "complaint_feedback_type",
		"created_at",
	).From("webhook_events")

	// Apply filters using squirrel
	if params.EventType != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"type": params.EventType})
	}

	if params.RecipientEmail != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"recipient_email": params.RecipientEmail})
	}

	if params.MessageID != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"message_id": params.MessageID})
	}

	// Time range filters
	if params.TimestampAfter != nil {
		queryBuilder = queryBuilder.Where(sq.GtOrEq{"timestamp": params.TimestampAfter})
	}

	if params.TimestampBefore != nil {
		queryBuilder = queryBuilder.Where(sq.LtOrEq{"timestamp": params.TimestampBefore})
	}

	// Handle cursor-based pagination
	if params.Cursor != "" {
		// Decode the base64 cursor
		decodedCursor, err := base64.StdEncoding.DecodeString(params.Cursor)
		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return nil, fmt.Errorf("invalid cursor encoding: %w", err)
		}

		// Parse the compound cursor (timestamp~id)
		cursorStr := string(decodedCursor)
		cursorParts := strings.Split(cursorStr, "~")
		if len(cursorParts) != 2 {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, fmt.Errorf("invalid cursor format"))
			// codecov:ignore:end
			return nil, fmt.Errorf("invalid cursor format: expected timestamp~id")
		}

		cursorTime, err := time.Parse(time.RFC3339, cursorParts[0])
		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return nil, fmt.Errorf("invalid cursor timestamp format: %w", err)
		}

		cursorID := cursorParts[1]

		// Query for events before the cursor (newer events first)
		// Either timestamp is less than cursor time
		// OR timestamp equals cursor time AND id is less than cursor id
		queryBuilder = queryBuilder.Where(
			sq.Or{
				sq.Lt{"timestamp": cursorTime},
				sq.And{
					sq.Eq{"timestamp": cursorTime},
					sq.Lt{"id": cursorID},
				},
			},
		)
	}

	// Default ordering - most recent first
	queryBuilder = queryBuilder.OrderBy("timestamp DESC", "id DESC")

	// Add limit (fetch one extra to determine if there are more results)
	limit := params.Limit
	if limit <= 0 {
		limit = 20 // Default limit
	}
	queryBuilder = queryBuilder.Limit(uint64(limit + 1))

	// Execute the query
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := workspaceDB.QueryContext(ctx, query, args...)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to query webhook events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := []*domain.WebhookEvent{}
	for rows.Next() {
		event := &domain.WebhookEvent{}
		var messageID, bounceType, bounceCategory, bounceDiagnostic, complaintFeedbackType sql.NullString

		err := rows.Scan(
			&event.ID,
			&event.Type,
			&event.Source,
			&event.IntegrationID,
			&event.RecipientEmail,
			&messageID,
			&event.Timestamp,
			&event.RawPayload,
			&bounceType,
			&bounceCategory,
			&bounceDiagnostic,
			&complaintFeedbackType,
			&event.CreatedAt,
		)

		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return nil, fmt.Errorf("failed to scan webhook event row: %w", err)
		}

		if messageID.Valid {
			event.MessageID = &messageID.String
		}

		if bounceType.Valid {
			event.BounceType = bounceType.String
		}

		if bounceCategory.Valid {
			event.BounceCategory = bounceCategory.String
		}

		if bounceDiagnostic.Valid {
			event.BounceDiagnostic = bounceDiagnostic.String
		}

		if complaintFeedbackType.Valid {
			event.ComplaintFeedbackType = complaintFeedbackType.String
		}

		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("error iterating webhook event rows: %w", err)
	}

	// Determine if we have more results and generate cursor
	var nextCursor string
	hasMore := false

	// Check if we got an extra result, which indicates there are more results
	if len(events) > limit {
		hasMore = true
		events = events[:limit] // Remove the extra item
	}

	// Generate the next cursor based on the last item if we have results
	if len(events) > 0 && hasMore {
		lastEvent := events[len(events)-1]
		cursorStr := fmt.Sprintf("%s~%s", lastEvent.Timestamp.Format(time.RFC3339), lastEvent.ID)
		nextCursor = base64.StdEncoding.EncodeToString([]byte(cursorStr))
	}

	return &domain.WebhookEventListResult{
		Events:     events,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// DeleteForEmail redacts the email address in all webhook events for a specific email
func (r *webhookEventRepository) DeleteForEmail(ctx context.Context, workspaceID, email string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Redact the email address by replacing it with a generic redacted identifier
	redactedEmail := "DELETED_EMAIL"
	query := `UPDATE webhook_events SET recipient_email = $1 WHERE recipient_email = $2`

	result, err := workspaceDB.ExecContext(ctx, query, redactedEmail, email)
	if err != nil {
		return fmt.Errorf("failed to redact email in webhook events: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	// Note: We don't return an error if no rows were affected since the contact might not have any webhook events
	_ = rows

	return nil
}
