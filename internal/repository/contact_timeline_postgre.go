package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
)

// ContactTimelineRepository implements domain.ContactTimelineRepository
type ContactTimelineRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewContactTimelineRepository creates a new contact timeline repository
func NewContactTimelineRepository(workspaceRepo domain.WorkspaceRepository) *ContactTimelineRepository {
	return &ContactTimelineRepository{
		workspaceRepo: workspaceRepo,
	}
}

// List retrieves timeline entries for a contact with cursor-based pagination
func (r *ContactTimelineRepository) List(ctx context.Context, workspaceID string, email string, limit int, cursor *string) ([]*domain.ContactTimelineEntry, *string, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Default limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// Build query with JOINs to fetch entity data
	query := `
		SELECT 
			ct.id,
			ct.email,
			ct.operation,
			ct.entity_type,
			ct.kind,
			ct.changes,
			ct.entity_id,
			ct.created_at,
			ct.db_created_at,
			CASE 
				WHEN ct.entity_type = 'contact' THEN json_build_object(
					'email', c.email,
					'first_name', c.first_name,
					'last_name', c.last_name,
					'external_id', c.external_id
				)
				WHEN ct.entity_type = 'contact_list' THEN json_build_object(
					'id', l.id,
					'name', l.name,
					'status', cl.status
				)
				WHEN ct.entity_type = 'message_history' THEN json_build_object(
					'id', mh.id,
					'template_id', mh.template_id,
					'template_version', mh.template_version,
					'template_name', t_mh.name,
					'template_category', t_mh.category,
					'template_email', t_mh.email,
					'channel', mh.channel,
					'sent_at', mh.sent_at,
					'delivered_at', mh.delivered_at,
					'opened_at', mh.opened_at,
					'clicked_at', mh.clicked_at,
					'message_data', mh.message_data
				)
				WHEN ct.entity_type = 'webhook_event' THEN json_build_object(
					'id', we.id,
					'type', we.type,
					'message_id', we.message_id,
					'timestamp', we.timestamp,
					'bounce_type', we.bounce_type,
					'bounce_category', we.bounce_category,
					'bounce_diagnostic', we.bounce_diagnostic,
					'complaint_feedback_type', we.complaint_feedback_type,
					'template_id', mh_we.template_id,
					'template_version', mh_we.template_version,
					'template_name', t_we.name
				)
				ELSE NULL
			END as entity_data
		FROM contact_timeline ct
		LEFT JOIN contacts c ON ct.entity_type = 'contact' AND ct.email = c.email
		LEFT JOIN contact_lists cl ON ct.entity_type = 'contact_list' AND ct.entity_id = cl.list_id AND ct.email = cl.email
		LEFT JOIN lists l ON cl.list_id = l.id
		LEFT JOIN message_history mh ON ct.entity_type = 'message_history' AND ct.entity_id = mh.id
		LEFT JOIN templates t_mh ON ct.entity_type = 'message_history' AND mh.template_id = t_mh.id AND mh.template_version = t_mh.version
		LEFT JOIN webhook_events we ON ct.entity_type = 'webhook_event' AND ct.entity_id = we.message_id
		LEFT JOIN message_history mh_we ON ct.entity_type = 'webhook_event' AND we.message_id = mh_we.id
		LEFT JOIN templates t_we ON ct.entity_type = 'webhook_event' AND mh_we.template_id = t_we.id AND mh_we.template_version = t_we.version
		WHERE ct.email = $1
	`

	args := []interface{}{email}
	argIndex := 2

	// Handle cursor-based pagination
	if cursor != nil && *cursor != "" {
		decodedCursor, err := decodeCursor(*cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}

		// Cursor format: "timestamp|id"
		parts := strings.Split(decodedCursor, "|")
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid cursor format")
		}

		cursorTime, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor timestamp: %w", err)
		}
		cursorID := parts[1]

		query += fmt.Sprintf(" AND (ct.created_at < $%d OR (ct.created_at = $%d AND ct.id < $%d))", argIndex, argIndex+1, argIndex+2)
		args = append(args, cursorTime, cursorTime, cursorID)
		argIndex += 3
	}

	query += fmt.Sprintf(" ORDER BY ct.created_at DESC, ct.id DESC LIMIT $%d", argIndex)
	args = append(args, limit+1) // Fetch one extra to determine if there's a next page

	// Execute query
	rows, err := workspaceDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query timeline: %w", err)
	}
	defer rows.Close()

	// Parse results
	var entries []*domain.ContactTimelineEntry
	for rows.Next() {
		entry := &domain.ContactTimelineEntry{}
		var changesJSON []byte
		var entityDataJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.Email,
			&entry.Operation,
			&entry.EntityType,
			&entry.Kind,
			&changesJSON,
			&entry.EntityID,
			&entry.CreatedAt,
			&entry.DBCreatedAt,
			&entityDataJSON,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan timeline entry: %w", err)
		}

		// Parse changes JSON
		if changesJSON != nil {
			changes := make(map[string]interface{})
			if err := parseJSON(changesJSON, &changes); err != nil {
				return nil, nil, fmt.Errorf("failed to parse changes JSON: %w", err)
			}
			entry.Changes = changes
		}

		// Parse entity data JSON
		if entityDataJSON != nil {
			entityData := make(map[string]interface{})
			if err := parseJSON(entityDataJSON, &entityData); err != nil {
				return nil, nil, fmt.Errorf("failed to parse entity data JSON: %w", err)
			}
			entry.EntityData = entityData
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating timeline rows: %w", err)
	}

	// Determine if there's a next page
	var nextCursor *string
	if len(entries) > limit {
		// There are more results, create cursor from the last item
		lastEntry := entries[limit-1]
		cursorStr := encodeCursor(lastEntry.CreatedAt, lastEntry.ID)
		nextCursor = &cursorStr
		// Return only the requested number of items
		entries = entries[:limit]
	}

	return entries, nextCursor, nil
}

// encodeCursor creates a cursor string from timestamp and ID
func encodeCursor(timestamp time.Time, id string) string {
	cursorData := fmt.Sprintf("%s|%s", timestamp.Format(time.RFC3339Nano), id)
	return base64.StdEncoding.EncodeToString([]byte(cursorData))
}

// decodeCursor decodes a cursor string
func decodeCursor(cursor string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// parseJSON is a helper function to parse JSONB data
func parseJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

// DeleteForEmail deletes all timeline entries for a contact email
func (r *ContactTimelineRepository) DeleteForEmail(ctx context.Context, workspaceID string, email string) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Build and execute delete query
	query, args, err := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Delete("contact_timeline").
		Where(sq.Eq{"email": email}).
		ToSql()

	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = workspaceDB.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete timeline entries: %w", err)
	}

	return nil
}
