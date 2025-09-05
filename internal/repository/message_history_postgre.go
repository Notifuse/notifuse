package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"encoding/base64"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// MessageHistoryRepository implements domain.MessageHistoryRepository
type MessageHistoryRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewMessageHistoryRepository creates a new message history repository
func NewMessageHistoryRepository(workspaceRepo domain.WorkspaceRepository) *MessageHistoryRepository {
	return &MessageHistoryRepository{
		workspaceRepo: workspaceRepo,
	}
}

// Create adds a new message history record
func (r *MessageHistoryRepository) Create(ctx context.Context, workspaceID string, message *domain.MessageHistory) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		INSERT INTO message_history (
			id, external_id, contact_email, broadcast_id, template_id, template_version, 
			channel, status_info, message_data, sent_at, delivered_at, 
			failed_at, opened_at, clicked_at, bounced_at, complained_at, 
			unsubscribed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, 
			$7, LEFT($8, 255), $9, $10, $11, 
			$12, $13, $14, $15, $16, 
			$17, $18, $19
		)
	`

	_, err = workspaceDB.ExecContext(
		ctx,
		query,
		message.ID,
		message.ExternalID,
		message.ContactEmail,
		message.BroadcastID,
		message.TemplateID,
		message.TemplateVersion,
		message.Channel,
		message.StatusInfo,
		message.MessageData,
		message.SentAt,
		message.DeliveredAt,
		message.FailedAt,
		message.OpenedAt,
		message.ClickedAt,
		message.BouncedAt,
		message.ComplainedAt,
		message.UnsubscribedAt,
		message.CreatedAt,
		message.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create message history: %w", err)
	}

	return nil
}

// Update updates an existing message history record
func (r *MessageHistoryRepository) Update(ctx context.Context, workspaceID string, message *domain.MessageHistory) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE message_history SET
			external_id = $2,
			contact_email = $3,
			broadcast_id = $4,
			template_id = $5,
			template_version = $6,
			channel = $7,
			status_info = LEFT($8, 255),
			message_data = $9,
			sent_at = $10,
			delivered_at = $11,
			failed_at = $12,
			opened_at = $13,	
			clicked_at = $14,
			bounced_at = $15,
			complained_at = $16,
			unsubscribed_at = $17,
			updated_at = $18
		WHERE id = $1
	`

	_, err = workspaceDB.ExecContext(
		ctx,
		query,
		message.ID,
		message.ExternalID,
		message.ContactEmail,
		message.BroadcastID,
		message.TemplateID,
		message.TemplateVersion,
		message.Channel,
		message.StatusInfo,
		message.MessageData,
		message.SentAt,
		message.DeliveredAt,
		message.FailedAt,
		message.OpenedAt,
		message.ClickedAt,
		message.BouncedAt,
		message.ComplainedAt,
		message.UnsubscribedAt,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to update message history: %w", err)
	}

	return nil
}

// Get retrieves a message history by ID
func (r *MessageHistoryRepository) Get(ctx context.Context, workspaceID, id string) (*domain.MessageHistory, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			id, external_id, contact_email, broadcast_id, template_id, template_version, 
			channel, status_info, message_data, sent_at, delivered_at, 
			failed_at, opened_at, clicked_at, bounced_at, complained_at, 
			unsubscribed_at, created_at, updated_at
		FROM message_history
		WHERE id = $1
	`

	var message domain.MessageHistory
	err = workspaceDB.QueryRowContext(ctx, query, id).Scan(
		&message.ID,
		&message.ExternalID,
		&message.ContactEmail,
		&message.BroadcastID,
		&message.TemplateID,
		&message.TemplateVersion,
		&message.Channel,
		&message.StatusInfo,
		&message.MessageData,
		&message.SentAt,
		&message.DeliveredAt,
		&message.FailedAt,
		&message.OpenedAt,
		&message.ClickedAt,
		&message.BouncedAt,
		&message.ComplainedAt,
		&message.UnsubscribedAt,
		&message.CreatedAt,
		&message.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("message history with id %s not found", id)
		}
		return nil, fmt.Errorf("failed to get message history: %w", err)
	}

	return &message, nil
}

// GetByExternalID retrieves a message history by external ID for idempotency checks
func (r *MessageHistoryRepository) GetByExternalID(ctx context.Context, workspaceID, externalID string) (*domain.MessageHistory, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			id, external_id, contact_email, broadcast_id, template_id, template_version, 
			channel, status_info, message_data, sent_at, delivered_at, 
			failed_at, opened_at, clicked_at, bounced_at, complained_at, 
			unsubscribed_at, created_at, updated_at
		FROM message_history
		WHERE external_id = $1
	`

	var message domain.MessageHistory
	err = workspaceDB.QueryRowContext(ctx, query, externalID).Scan(
		&message.ID,
		&message.ExternalID,
		&message.ContactEmail,
		&message.BroadcastID,
		&message.TemplateID,
		&message.TemplateVersion,
		&message.Channel,
		&message.StatusInfo,
		&message.MessageData,
		&message.SentAt,
		&message.DeliveredAt,
		&message.FailedAt,
		&message.OpenedAt,
		&message.ClickedAt,
		&message.BouncedAt,
		&message.ComplainedAt,
		&message.UnsubscribedAt,
		&message.CreatedAt,
		&message.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("message history with external_id %s not found", externalID)
		}
		return nil, fmt.Errorf("failed to get message history by external_id: %w", err)
	}

	return &message, nil
}

// GetByContact retrieves message history for a specific contact
func (r *MessageHistoryRepository) GetByContact(ctx context.Context, workspaceID, contactEmail string, limit, offset int) ([]*domain.MessageHistory, int, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// First get total count
	countQuery := `SELECT COUNT(*) FROM message_history WHERE contact_email = $1`
	var totalCount int
	err = workspaceDB.QueryRowContext(ctx, countQuery, contactEmail).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count message history: %w", err)
	}

	// Set default limit and offset if not provided
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT 
			id, external_id, contact_email, broadcast_id, template_id, template_version, 
			channel, status_info, message_data, sent_at, delivered_at, 
			failed_at, opened_at, clicked_at, bounced_at, complained_at, 
			unsubscribed_at, created_at, updated_at
		FROM message_history
		WHERE contact_email = $1
		ORDER BY sent_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := workspaceDB.QueryContext(ctx, query, contactEmail, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query message history: %w", err)
	}
	defer rows.Close()

	var messages []*domain.MessageHistory
	for rows.Next() {
		var message domain.MessageHistory
		err := rows.Scan(
			&message.ID,
			&message.ExternalID,
			&message.ContactEmail,
			&message.BroadcastID,
			&message.TemplateID,
			&message.TemplateVersion,
			&message.Channel,
			&message.StatusInfo,
			&message.MessageData,
			&message.SentAt,
			&message.DeliveredAt,
			&message.FailedAt,
			&message.OpenedAt,
			&message.ClickedAt,
			&message.BouncedAt,
			&message.ComplainedAt,
			&message.UnsubscribedAt,
			&message.CreatedAt,
			&message.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan message history: %w", err)
		}
		messages = append(messages, &message)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating message history rows: %w", err)
	}

	return messages, totalCount, nil
}

// GetByBroadcast retrieves message history for a specific broadcast
func (r *MessageHistoryRepository) GetByBroadcast(ctx context.Context, workspaceID, broadcastID string, limit, offset int) ([]*domain.MessageHistory, int, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// First get total count
	countQuery := `SELECT COUNT(*) FROM message_history WHERE broadcast_id = $1`
	var totalCount int
	err = workspaceDB.QueryRowContext(ctx, countQuery, broadcastID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count message history: %w", err)
	}

	// Set default limit and offset if not provided
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT 
			id, external_id, contact_email, broadcast_id, template_id, template_version, 
			channel, status_info, message_data, sent_at, delivered_at, 
			failed_at, opened_at, clicked_at, bounced_at, complained_at, 
			unsubscribed_at, created_at, updated_at
		FROM message_history
		WHERE broadcast_id = $1
		ORDER BY sent_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := workspaceDB.QueryContext(ctx, query, broadcastID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query message history: %w", err)
	}
	defer rows.Close()

	var messages []*domain.MessageHistory
	for rows.Next() {
		var message domain.MessageHistory
		err := rows.Scan(
			&message.ID,
			&message.ExternalID,
			&message.ContactEmail,
			&message.BroadcastID,
			&message.TemplateID,
			&message.TemplateVersion,
			&message.Channel,
			&message.StatusInfo,
			&message.MessageData,
			&message.SentAt,
			&message.DeliveredAt,
			&message.FailedAt,
			&message.OpenedAt,
			&message.ClickedAt,
			&message.BouncedAt,
			&message.ComplainedAt,
			&message.UnsubscribedAt,
			&message.CreatedAt,
			&message.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan message history: %w", err)
		}
		messages = append(messages, &message)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating message history rows: %w", err)
	}

	return messages, totalCount, nil
}

// SetStatusesIfNotSet updates multiple message statuses in a single batch operation
// but only if the corresponding status timestamp is not already set
func (r *MessageHistoryRepository) SetStatusesIfNotSet(ctx context.Context, workspaceID string, updates []domain.MessageEventUpdate) error {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "MessageHistoryRepository", "SetStatusesIfNotSet")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	tracing.AddAttribute(ctx, "updateCount", len(updates))
	// codecov:ignore:end

	if len(updates) == 0 {
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

	// Group updates by status type for more efficient processing
	messageEventGroups := make(map[domain.MessageEvent][]domain.MessageEventUpdate)
	for _, update := range updates {
		messageEventGroups[update.Event] = append(messageEventGroups[update.Event], update)
	}

	now := time.Now()

	// Process each status group with a single query
	for messageEvent, groupUpdates := range messageEventGroups {
		// Determine which field to check and update based on status
		var field string
		switch messageEvent {
		case domain.MessageEventDelivered:
			field = "delivered_at"
		case domain.MessageEventFailed:
			field = "failed_at"
		case domain.MessageEventOpened:
			field = "opened_at"
		case domain.MessageEventClicked:
			field = "clicked_at"
		case domain.MessageEventBounced:
			field = "bounced_at"
		case domain.MessageEventComplained:
			field = "complained_at"
		case domain.MessageEventUnsubscribed:
			field = "unsubscribed_at"
		default:
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, fmt.Errorf("invalid status: %s", messageEvent))
			// codecov:ignore:end
			return fmt.Errorf("invalid status: %s", messageEvent)
		}

		// Build VALUES clause for batch update with explicit timestamp casting and status_info
		valuesParts := make([]string, len(groupUpdates))
		args := []interface{}{now}

		for i, update := range groupUpdates {
			valuesParts[i] = fmt.Sprintf("($%d, $%d::TIMESTAMP WITH TIME ZONE, $%d)", len(args)+1, len(args)+2, len(args)+3)
			args = append(args, update.ID, update.Timestamp, update.StatusInfo)
		}

		valuesClause := strings.Join(valuesParts, ", ")

		query := fmt.Sprintf(`
			UPDATE message_history 
			SET %s = updates.timestamp, 
				status_info = COALESCE(LEFT(updates.status_info, 255), message_history.status_info), 
				updated_at = $1::TIMESTAMP WITH TIME ZONE
			FROM (VALUES %s) AS updates(id, timestamp, status_info)
			WHERE message_history.id = updates.id AND %s IS NULL
		`, field, valuesClause, field)
		_, err = workspaceDB.ExecContext(ctx, query, args...)
		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return fmt.Errorf("failed to batch update message statuses for status %s: %w", messageEvent, err)
		}
	}

	return nil
}

func (r *MessageHistoryRepository) SetClicked(ctx context.Context, workspaceID, id string, timestamp time.Time) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// First query: Update clicked_at if it's null
	clickQuery := `
		UPDATE message_history 
		SET 
			clicked_at = $1,
			updated_at = NOW()
		WHERE id = $2 AND clicked_at IS NULL
	`

	_, err = workspaceDB.ExecContext(ctx, clickQuery, timestamp, id)
	if err != nil {
		return fmt.Errorf("failed to set clicked: %w", err)
	}

	// Second query: Update opened_at if it's null as a click means the message was opened
	openQuery := `
		UPDATE message_history 
		SET 
			opened_at = $1,
			updated_at = NOW()
		WHERE id = $2 AND opened_at IS NULL
	`

	_, err = workspaceDB.ExecContext(ctx, openQuery, timestamp, id)
	if err != nil {
		return fmt.Errorf("failed to set opened: %w", err)
	}

	return nil
}

func (r *MessageHistoryRepository) SetOpened(ctx context.Context, workspaceID, id string, timestamp time.Time) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// First query: Update opened_at if it's null
	query := `
		UPDATE message_history 
		SET 
			opened_at = $1,
			updated_at = NOW()
		WHERE id = $2 AND opened_at IS NULL
	`

	_, err = workspaceDB.ExecContext(ctx, query, timestamp, id)
	if err != nil {
		return fmt.Errorf("failed to set opened: %w", err)
	}

	return nil
}

// ListMessages retrieves message history with cursor-based pagination and filtering
func (r *MessageHistoryRepository) ListMessages(ctx context.Context, workspaceID string, params domain.MessageListParams) ([]*domain.MessageHistory, string, error) {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "MessageHistoryRepository", "ListMessages")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	// codecov:ignore:end

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, "", fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Set a reasonable default limit if not provided
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}

	// Use squirrel to build the query with placeholders
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	queryBuilder := psql.Select(
		"id", "external_id", "contact_email", "broadcast_id", "template_id", "template_version",
		"channel", "status_info", "message_data", "sent_at", "delivered_at",
		"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
		"unsubscribed_at", "created_at", "updated_at",
	).From("message_history")

	// Apply filters using squirrel
	if params.Channel != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"channel": params.Channel})
	}

	if params.ContactEmail != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"contact_email": params.ContactEmail})
	}

	if params.BroadcastID != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"broadcast_id": params.BroadcastID})
	}

	if params.TemplateID != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"template_id": params.TemplateID})
	}

	if params.IsSent != nil {
		if *params.IsSent {
			queryBuilder = queryBuilder.Where(sq.NotEq{"sent_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"sent_at": nil})
		}
	}

	if params.IsDelivered != nil {
		if *params.IsDelivered {
			queryBuilder = queryBuilder.Where(sq.NotEq{"delivered_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"delivered_at": nil})
		}
	}

	if params.IsFailed != nil {
		if *params.IsFailed {
			queryBuilder = queryBuilder.Where(sq.NotEq{"failed_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"failed_at": nil})
		}
	}

	if params.IsOpened != nil {
		if *params.IsOpened {
			queryBuilder = queryBuilder.Where(sq.NotEq{"opened_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"opened_at": nil})
		}
	}

	if params.IsClicked != nil {
		if *params.IsClicked {
			queryBuilder = queryBuilder.Where(sq.NotEq{"clicked_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"clicked_at": nil})
		}
	}

	if params.IsBounced != nil {
		if *params.IsBounced {
			queryBuilder = queryBuilder.Where(sq.NotEq{"bounced_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"bounced_at": nil})
		}
	}

	if params.IsComplained != nil {
		if *params.IsComplained {
			queryBuilder = queryBuilder.Where(sq.NotEq{"complained_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"complained_at": nil})
		}
	}

	if params.IsUnsubscribed != nil {
		if *params.IsUnsubscribed {
			queryBuilder = queryBuilder.Where(sq.NotEq{"unsubscribed_at": nil})
		} else {
			queryBuilder = queryBuilder.Where(sq.Eq{"unsubscribed_at": nil})
		}
	}

	// Time range filters
	if params.SentAfter != nil {
		queryBuilder = queryBuilder.Where(sq.GtOrEq{"sent_at": params.SentAfter})
	}

	if params.SentBefore != nil {
		queryBuilder = queryBuilder.Where(sq.LtOrEq{"sent_at": params.SentBefore})
	}

	if params.UpdatedAfter != nil {
		queryBuilder = queryBuilder.Where(sq.GtOrEq{"updated_at": params.UpdatedAfter})
	}

	if params.UpdatedBefore != nil {
		queryBuilder = queryBuilder.Where(sq.LtOrEq{"updated_at": params.UpdatedBefore})
	}

	// Handle cursor-based pagination
	if params.Cursor != "" {
		// Decode the base64 cursor
		decodedCursor, err := base64.StdEncoding.DecodeString(params.Cursor)
		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return nil, "", fmt.Errorf("invalid cursor encoding: %w", err)
		}

		// Parse the compound cursor (timestamp~id)
		cursorStr := string(decodedCursor)
		cursorParts := strings.Split(cursorStr, "~")
		if len(cursorParts) != 2 {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, fmt.Errorf("invalid cursor format"))
			// codecov:ignore:end
			return nil, "", fmt.Errorf("invalid cursor format: expected timestamp~id")
		}

		cursorTime, err := time.Parse(time.RFC3339, cursorParts[0])
		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return nil, "", fmt.Errorf("invalid cursor timestamp format: %w", err)
		}

		cursorID := cursorParts[1]

		// Query for messages before the cursor (newer messages first)
		// Either created_at is less than cursor time
		// OR created_at equals cursor time AND id is less than cursor id
		queryBuilder = queryBuilder.Where(
			sq.Or{
				sq.Lt{"created_at": cursorTime},
				sq.And{
					sq.Eq{"created_at": cursorTime},
					sq.Lt{"id": cursorID},
				},
			},
		)
		queryBuilder = queryBuilder.OrderBy("created_at DESC", "id DESC")
	} else {
		// Default ordering when no cursor is provided - most recent first
		queryBuilder = queryBuilder.OrderBy("created_at DESC", "id DESC")
	}

	// Add limit
	queryBuilder = queryBuilder.Limit(uint64(limit + 1)) // Fetch one extra to determine if there are more results

	// Execute the query
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, "", fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := workspaceDB.QueryContext(ctx, query, args...)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, "", fmt.Errorf("failed to query message history: %w", err)
	}
	defer rows.Close()

	messages := []*domain.MessageHistory{}
	for rows.Next() {
		message := &domain.MessageHistory{}
		var externalID sql.NullString
		var broadcastID sql.NullString
		var statusInfo sql.NullString
		var deliveredAt, failedAt, openedAt, clickedAt, bouncedAt, complainedAt, unsubscribedAt sql.NullTime

		err := rows.Scan(
			&message.ID, &externalID, &message.ContactEmail, &broadcastID, &message.TemplateID, &message.TemplateVersion,
			&message.Channel, &statusInfo, &message.MessageData,
			&message.SentAt, &deliveredAt, &failedAt, &openedAt,
			&clickedAt, &bouncedAt, &complainedAt, &unsubscribedAt,
			&message.CreatedAt, &message.UpdatedAt,
		)

		if err != nil {
			// codecov:ignore:start
			tracing.MarkSpanError(ctx, err)
			// codecov:ignore:end
			return nil, "", fmt.Errorf("failed to scan message history row: %w", err)
		}

		// Convert nullable fields
		if externalID.Valid {
			message.ExternalID = &externalID.String
		}

		if broadcastID.Valid {
			message.BroadcastID = &broadcastID.String
		}

		if statusInfo.Valid {
			message.StatusInfo = &statusInfo.String
		}

		if deliveredAt.Valid {
			message.DeliveredAt = &deliveredAt.Time
		}

		if failedAt.Valid {
			message.FailedAt = &failedAt.Time
		}

		if openedAt.Valid {
			message.OpenedAt = &openedAt.Time
		}

		if clickedAt.Valid {
			message.ClickedAt = &clickedAt.Time
		}

		if bouncedAt.Valid {
			message.BouncedAt = &bouncedAt.Time
		}

		if complainedAt.Valid {
			message.ComplainedAt = &complainedAt.Time
		}

		if unsubscribedAt.Valid {
			message.UnsubscribedAt = &unsubscribedAt.Time
		}

		messages = append(messages, message)
	}

	if err = rows.Err(); err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, "", fmt.Errorf("error iterating message history rows: %w", err)
	}

	// Determine if we have more results and generate cursor
	var nextCursor string

	// Check if we got an extra result, which indicates there are more results
	hasMore := len(messages) > limit
	if hasMore {
		// Remove the extra item
		messages = messages[:limit]
	}

	// Generate the next cursor based on the last item if we have results
	if len(messages) > 0 && hasMore {
		lastMessage := messages[len(messages)-1]
		cursorStr := fmt.Sprintf("%s~%s", lastMessage.CreatedAt.Format(time.RFC3339), lastMessage.ID)
		nextCursor = base64.StdEncoding.EncodeToString([]byte(cursorStr))
	}

	return messages, nextCursor, nil
}

func (r *MessageHistoryRepository) GetBroadcastStats(ctx context.Context, workspaceID string, id string) (*domain.MessageHistoryStatusSum, error) {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "MessageHistoryRepository", "GetBroadcastStats")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	tracing.AddAttribute(ctx, "broadcastID", id)
	// codecov:ignore:end

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// MessageEventSent         MessageEvent = "sent"
	// MessageEventDelivered    MessageEvent = "delivered"
	// MessageEventFailed       MessageEvent = "failed"
	// MessageEventOpened       MessageEvent = "opened"
	// MessageEventClicked      MessageEvent = "clicked"
	// MessageEventBounced      MessageEvent = "bounced"
	// MessageEventComplained   MessageEvent = "complained"
	// MessageEventUnsubscribed MessageEvent = "unsubscribed"

	query := `
		SELECT 
			SUM(CASE WHEN sent_at IS NOT NULL THEN 1 ELSE 0 END) as total_sent,
			SUM(CASE WHEN delivered_at IS NOT NULL THEN 1 ELSE 0 END) as total_delivered,
			SUM(CASE WHEN failed_at IS NOT NULL THEN 1 ELSE 0 END) as total_failed,
			SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END) as total_opened,
			SUM(CASE WHEN clicked_at IS NOT NULL THEN 1 ELSE 0 END) as total_clicked,
			SUM(CASE WHEN bounced_at IS NOT NULL THEN 1 ELSE 0 END) as total_bounced,
			SUM(CASE WHEN complained_at IS NOT NULL THEN 1 ELSE 0 END) as total_complained,
			SUM(CASE WHEN unsubscribed_at IS NOT NULL THEN 1 ELSE 0 END) as total_unsubscribed
		FROM message_history
		WHERE broadcast_id = $1
	`

	row := workspaceDB.QueryRowContext(ctx, query, id)
	stats := &domain.MessageHistoryStatusSum{}

	// Use NullInt64 to handle NULL values from database
	var totalSent, totalDelivered, totalFailed, totalOpened sql.NullInt64
	var totalClicked, totalBounced, totalComplained, totalUnsubscribed sql.NullInt64

	err = row.Scan(
		&totalSent,
		&totalDelivered,
		&totalFailed,
		&totalOpened,
		&totalClicked,
		&totalBounced,
		&totalComplained,
		&totalUnsubscribed,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return stats, nil // Return empty stats (all zeros)
		}
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to get broadcast stats: %w", err)
	}

	// Convert nullable values to integers (use 0 for NULL values)
	if totalSent.Valid {
		stats.TotalSent = int(totalSent.Int64)
	}
	if totalDelivered.Valid {
		stats.TotalDelivered = int(totalDelivered.Int64)
	}
	if totalFailed.Valid {
		stats.TotalFailed = int(totalFailed.Int64)
	}
	if totalOpened.Valid {
		stats.TotalOpened = int(totalOpened.Int64)
	}
	if totalClicked.Valid {
		stats.TotalClicked = int(totalClicked.Int64)
	}
	if totalBounced.Valid {
		stats.TotalBounced = int(totalBounced.Int64)
	}
	if totalComplained.Valid {
		stats.TotalComplained = int(totalComplained.Int64)
	}
	if totalUnsubscribed.Valid {
		stats.TotalUnsubscribed = int(totalUnsubscribed.Int64)
	}

	return stats, nil
}

// GetBroadcastVariationStats retrieves statistics for a specific variation of a broadcast
func (r *MessageHistoryRepository) GetBroadcastVariationStats(ctx context.Context, workspaceID string, broadcastID, templateID string) (*domain.MessageHistoryStatusSum, error) {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "MessageHistoryRepository", "GetBroadcastVariationStats")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	tracing.AddAttribute(ctx, "broadcastID", broadcastID)
	tracing.AddAttribute(ctx, "templateID", templateID)
	// codecov:ignore:end

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT 
			SUM(CASE WHEN sent_at IS NOT NULL THEN 1 ELSE 0 END) as total_sent,
			SUM(CASE WHEN delivered_at IS NOT NULL THEN 1 ELSE 0 END) as total_delivered,
			SUM(CASE WHEN failed_at IS NOT NULL THEN 1 ELSE 0 END) as total_failed,
			SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END) as total_opened,
			SUM(CASE WHEN clicked_at IS NOT NULL THEN 1 ELSE 0 END) as total_clicked,
			SUM(CASE WHEN bounced_at IS NOT NULL THEN 1 ELSE 0 END) as total_bounced,
			SUM(CASE WHEN complained_at IS NOT NULL THEN 1 ELSE 0 END) as total_complained,
			SUM(CASE WHEN unsubscribed_at IS NOT NULL THEN 1 ELSE 0 END) as total_unsubscribed
		FROM message_history
		WHERE broadcast_id = $1 AND template_id = $2
	`

	row := workspaceDB.QueryRowContext(ctx, query, broadcastID, templateID)
	stats := &domain.MessageHistoryStatusSum{}

	// Use NullInt64 to handle NULL values from database
	var totalSent, totalDelivered, totalFailed, totalOpened sql.NullInt64
	var totalClicked, totalBounced, totalComplained, totalUnsubscribed sql.NullInt64

	err = row.Scan(
		&totalSent,
		&totalDelivered,
		&totalFailed,
		&totalOpened,
		&totalClicked,
		&totalBounced,
		&totalComplained,
		&totalUnsubscribed,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return stats, nil // Return empty stats (all zeros)
		}
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to get broadcast variation stats: %w", err)
	}

	// Convert nullable values to integers (use 0 for NULL values)
	if totalSent.Valid {
		stats.TotalSent = int(totalSent.Int64)
	}
	if totalDelivered.Valid {
		stats.TotalDelivered = int(totalDelivered.Int64)
	}
	if totalFailed.Valid {
		stats.TotalFailed = int(totalFailed.Int64)
	}
	if totalOpened.Valid {
		stats.TotalOpened = int(totalOpened.Int64)
	}
	if totalClicked.Valid {
		stats.TotalClicked = int(totalClicked.Int64)
	}
	if totalBounced.Valid {
		stats.TotalBounced = int(totalBounced.Int64)
	}
	if totalComplained.Valid {
		stats.TotalComplained = int(totalComplained.Int64)
	}
	if totalUnsubscribed.Valid {
		stats.TotalUnsubscribed = int(totalUnsubscribed.Int64)
	}

	return stats, nil
}
