package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/tracing"
	"github.com/lib/pq"
)

type inboundWebhookEventRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewInboundWebhookEventRepository creates a new PostgreSQL repository for inbound webhook events
func NewInboundWebhookEventRepository(workspaceRepo domain.WorkspaceRepository) domain.InboundWebhookEventRepository {
	return &inboundWebhookEventRepository{
		workspaceRepo: workspaceRepo,
	}
}

// StoreEvents stores multiple inbound webhook events in the database as a batch
func (r *inboundWebhookEventRepository) StoreEvents(ctx context.Context, workspaceID string, events []*domain.InboundWebhookEvent) error {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "InboundWebhookEventRepository", "StoreEvents")
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
		INSERT INTO inbound_webhook_events (
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
		// Target-less ON CONFLICT so any unique violation is a no-op: the PK (id)
		// AND the reply dedup index (integration_id, message_id) for replayed replies.
		batchSQL := baseSQL + strings.Join(placeholders[:len(currentBatch)], ",") + " ON CONFLICT DO NOTHING"
		_, err = workspaceDB.ExecContext(ctx, batchSQL, args...)

		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return fmt.Errorf("failed to store inbound webhook events batch: %w", err)
		}
	}

	return nil
}

// StoreEvent stores a single inbound webhook event in the database
func (r *inboundWebhookEventRepository) StoreEvent(ctx context.Context, workspaceID string, event *domain.InboundWebhookEvent) error {
	return r.StoreEvents(ctx, workspaceID, []*domain.InboundWebhookEvent{event})
}

// StoreReplyEvent inserts a single inbound reply event and reports whether it was newly
// stored. The reply dedup index (integration_id, message_id WHERE type IN reply/auto_reply)
// makes a provider retry of the same reply conflict; ON CONFLICT DO NOTHING ... RETURNING
// then yields no row, which we surface as inserted=false so callers don't re-fire side
// effects (e.g. exiting a journey twice).
func (r *inboundWebhookEventRepository) StoreReplyEvent(ctx context.Context, workspaceID string, event *domain.InboundWebhookEvent) (bool, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	const query = `
		INSERT INTO inbound_webhook_events (
			id, type, source, integration_id, recipient_email, message_id, timestamp, raw_payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING
		RETURNING id`

	var id string
	err = workspaceDB.QueryRowContext(ctx, query,
		event.ID, event.Type, event.Source, event.IntegrationID, event.RecipientEmail,
		event.MessageID, event.Timestamp, event.RawPayload, time.Now().UTC(),
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // deduplicated — an event with this Message-ID already exists
	}
	if err != nil {
		return false, fmt.Errorf("failed to store reply event: %w", err)
	}
	return true, nil
}

// ListEvents retrieves all inbound webhook events for a workspace
func (r *inboundWebhookEventRepository) ListEvents(ctx context.Context, workspaceID string, params domain.InboundWebhookEventListParams) (*domain.InboundWebhookEventListResult, error) {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "InboundWebhookEventRepository", "ListEvents")
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
	).From("inbound_webhook_events")

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

		cursorTime, err := time.Parse(time.RFC3339Nano, cursorParts[0])
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
		return nil, fmt.Errorf("failed to query inbound webhook events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := []*domain.InboundWebhookEvent{}
	for rows.Next() {
		event := &domain.InboundWebhookEvent{}
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
			return nil, fmt.Errorf("failed to scan inbound webhook event row: %w", err)
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
		// Use RFC3339Nano to preserve sub-second precision and avoid skipping events
		// recorded within the same second
		cursorStr := fmt.Sprintf("%s~%s", lastEvent.Timestamp.Format(time.RFC3339Nano), lastEvent.ID)
		nextCursor = base64.StdEncoding.EncodeToString([]byte(cursorStr))
	}

	return &domain.InboundWebhookEventListResult{
		Events:     events,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// countConsecutiveSoftBouncesSQL counts bounce events that count toward the
// soft-bounce threshold for each recipient email since that email's last
// successful delivery.
//
// We do not filter on bounce_type because the per-provider string varies
// (`Transient` for SES, `Failed` for Mailgun, `Bounce` for SparkPost/SMTP,
// `bounce`/`blocked`/`dropped` for SendGrid, etc.) — a strict allow-list would
// silently exclude most providers' soft bounces. Instead we count every
// `type='bounce'` event in the window minus message-level rejections, and rely
// on the per-event `domain.ClassifyBounce` call in the service to have already
// gated which events become a count attempt.
//
// Edge case: a hard bounce since last delivery would also be counted, but a
// hard bounce already flipped contact_lists.status='bounced', so the eventual
// MarkEmailsAsBounced UPDATE is a no-op via its status guard.
//
// The ignored-subtype filter must stay in sync with the SoftIgnore branch of
// ClassifyBounce.
const countConsecutiveSoftBouncesSQL = `
WITH last_delivery AS (
  SELECT contact_email AS email, MAX(delivered_at) AS ts
    FROM message_history
   WHERE contact_email = ANY($1)
     AND delivered_at IS NOT NULL
   GROUP BY contact_email
)
SELECT e.recipient_email, COUNT(*)::int
  FROM inbound_webhook_events e
  LEFT JOIN last_delivery d ON d.email = e.recipient_email
 WHERE e.recipient_email = ANY($1)
   AND e.type = 'bounce'
   AND lower(coalesce(e.bounce_category, '')) NOT IN
        ('messagetoolarge','contentrejected','attachmentrejected')
   AND e.timestamp > COALESCE(d.ts, 'epoch'::timestamptz)
 GROUP BY e.recipient_email`

// CountConsecutiveSoftBounces returns, per email, the number of countable soft
// bounces recorded since that email's last successful delivery.
func (r *inboundWebhookEventRepository) CountConsecutiveSoftBounces(ctx context.Context, workspaceID string, emails []string) (map[string]int, error) {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "InboundWebhookEventRepository", "CountConsecutiveSoftBounces")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	tracing.AddAttribute(ctx, "emailCount", len(emails))
	// codecov:ignore:end

	result := map[string]int{}
	if len(emails) == 0 {
		return result, nil
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	rows, err := workspaceDB.QueryContext(ctx, countConsecutiveSoftBouncesSQL, pq.Array(emails))
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to count consecutive soft bounces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var email string
		var count int
		if err := rows.Scan(&email, &count); err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return nil, fmt.Errorf("failed to scan soft-bounce count row: %w", err)
		}
		result[email] = count
	}

	if err := rows.Err(); err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("error iterating soft-bounce count rows: %w", err)
	}

	return result, nil
}

// DeleteForEmail redacts the email address in all inbound webhook events for a specific email
func (r *inboundWebhookEventRepository) DeleteForEmail(ctx context.Context, workspaceID, email string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Redact the email address by replacing it with a generic redacted identifier.
	//
	// raw_payload is cleared with it. It holds the verbatim provider body the
	// address was parsed out of — and, for replies, the from/to pair as top-level
	// JSON keys — and inboundWebhookEvents.list returns it. Redacting
	// recipient_email alone left the address in the same row.
	//
	// '{}' rather than '': the column is TEXT, but the console renders the tab
	// with JSON.parse(raw_payload), and an empty string throws — so a redacted
	// row would crash the inbound events tab for everyone.
	redactedEmail := "DELETED_EMAIL"
	query := `UPDATE inbound_webhook_events SET recipient_email = $1, raw_payload = '{}' WHERE recipient_email = $2`

	result, err := workspaceDB.ExecContext(ctx, query, redactedEmail, email)
	if err != nil {
		return fmt.Errorf("failed to redact email in inbound webhook events: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	// Note: We don't return an error if no rows were affected since the contact might not have any webhook events
	_ = rows

	return nil
}
