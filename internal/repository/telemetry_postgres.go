package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

type telemetryRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewTelemetryRepository creates a new PostgreSQL telemetry repository
func NewTelemetryRepository(workspaceRepo domain.WorkspaceRepository) domain.TelemetryRepository {
	return &telemetryRepository{
		workspaceRepo: workspaceRepo,
	}
}

// GetWorkspaceMetrics retrieves aggregated metrics for a specific workspace
func (r *telemetryRepository) GetWorkspaceMetrics(ctx context.Context, workspaceID string) (*domain.TelemetryMetrics, error) {
	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace database connection: %w", err)
	}

	// Get system database connection for user count
	systemDB, err := r.getSystemConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system database connection: %w", err)
	}

	metrics := &domain.TelemetryMetrics{}

	// Count contacts
	if contactsCount, err := r.CountContacts(ctx, db); err == nil {
		metrics.ContactsCount = contactsCount
	}

	// Count broadcasts
	if broadcastsCount, err := r.CountBroadcasts(ctx, db); err == nil {
		metrics.BroadcastsCount = broadcastsCount
	}

	// Count transactional notifications
	if transactionalCount, err := r.CountTransactional(ctx, db); err == nil {
		metrics.TransactionalCount = transactionalCount
	}

	// Count messages
	if messagesCount, err := r.CountMessages(ctx, db); err == nil {
		metrics.MessagesCount = messagesCount
	}

	// Count lists
	if listsCount, err := r.CountLists(ctx, db); err == nil {
		metrics.ListsCount = listsCount
	}

	// Count segments
	if segmentsCount, err := r.CountSegments(ctx, db); err == nil {
		metrics.SegmentsCount = segmentsCount
	}

	// Count users (from system database)
	if usersCount, err := r.CountUsers(ctx, systemDB, workspaceID); err == nil {
		metrics.UsersCount = usersCount
	}

	// Count blog posts
	if blogPostsCount, err := r.CountBlogPosts(ctx, db); err == nil {
		metrics.BlogPostsCount = blogPostsCount
	}

	// Get last message timestamp
	if lastMessageAt, err := r.GetLastMessageAt(ctx, db); err == nil {
		metrics.LastMessageAt = lastMessageAt
	}

	return metrics, nil
}

// CountContacts counts the total number of contacts in a workspace
func (r *telemetryRepository) CountContacts(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM contacts`
	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count contacts: %w", err)
	}
	return count, nil
}

// CountBroadcasts counts the total number of broadcasts in a workspace
func (r *telemetryRepository) CountBroadcasts(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM broadcasts`
	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count broadcasts: %w", err)
	}
	return count, nil
}

// CountTransactional counts the total number of transactional notifications in a workspace
func (r *telemetryRepository) CountTransactional(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM transactional_notifications WHERE deleted_at IS NULL`
	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactional notifications: %w", err)
	}
	return count, nil
}

// CountMessages counts the total number of messages in a workspace
func (r *telemetryRepository) CountMessages(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM message_history`
	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}
	return count, nil
}

// CountLists counts the total number of lists in a workspace
func (r *telemetryRepository) CountLists(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM lists`
	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count lists: %w", err)
	}
	return count, nil
}

// CountSegments counts the total number of segments in a workspace
func (r *telemetryRepository) CountSegments(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM segments`
	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count segments: %w", err)
	}
	return count, nil
}

// CountUsers counts the total number of users in a workspace from the system database
func (r *telemetryRepository) CountUsers(ctx context.Context, systemDB *sql.DB, workspaceID string) (int, error) {
	query := `SELECT COUNT(*) FROM user_workspaces WHERE workspace_id = $1 AND deleted_at IS NULL`
	var count int
	err := systemDB.QueryRowContext(ctx, query, workspaceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return count, nil
}

// CountBlogPosts counts the total number of blog posts in a workspace
func (r *telemetryRepository) CountBlogPosts(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM blog_posts`
	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count blog posts: %w", err)
	}
	return count, nil
}

// GetLastMessageAt gets the timestamp of the last message sent from the workspace
func (r *telemetryRepository) GetLastMessageAt(ctx context.Context, db *sql.DB) (string, error) {
	// Use ORDER BY with LIMIT 1 to leverage the existing index (created_at DESC, id DESC)
	// This is much faster than MAX() on large tables as it can use the index directly
	query := `SELECT created_at FROM message_history 
			  WHERE created_at IS NOT NULL 
			  ORDER BY created_at DESC, id DESC 
			  LIMIT 1`

	var lastMessageAt sql.NullTime
	err := db.QueryRowContext(ctx, query).Scan(&lastMessageAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // No messages found, return empty string
		}
		return "", fmt.Errorf("failed to get last message timestamp: %w", err)
	}

	if !lastMessageAt.Valid {
		return "", nil // No messages found, return empty string
	}

	return lastMessageAt.Time.Format(time.RFC3339), nil
}

// getSystemConnection is a helper method to get the system database connection
func (r *telemetryRepository) getSystemConnection(ctx context.Context) (*sql.DB, error) {
	return r.workspaceRepo.GetSystemConnection(ctx)
}
