package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type telemetryRepository struct {
	workspaceRepo domain.WorkspaceRepository
	logger        logger.Logger
}

// NewTelemetryRepository creates a new PostgreSQL telemetry repository.
//
// The logger is required, not optional. Every metric below is collected on a
// best-effort basis and a failure leaves a zero in the payload; without somewhere
// to say so, a query that has been broken since it was written reports a
// plausible-looking zero forever — which is exactly what CountUsers did. A
// nil-tolerant logger would restore that silence, so omitting it is a compile
// error.
func NewTelemetryRepository(workspaceRepo domain.WorkspaceRepository, logger logger.Logger) domain.TelemetryRepository {
	return &telemetryRepository{
		workspaceRepo: workspaceRepo,
		logger:        logger,
	}
}

// metricFailed records that one metric could not be collected.
//
// It is a warning rather than an error because the payload is still sent and the
// instance is unaffected: the cost is one missing number in an analytics table.
// It is not silence, because a metric that is structurally broken — a table that
// does not exist, a column that was never added — fails on every workspace of
// every installation, forever, and reports a zero that reads exactly like a real
// one.
func (r *telemetryRepository) metricFailed(workspaceID, metric string, err error) {
	r.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"metric":       metric,
		"error":        err.Error(),
	}).Warn("telemetry: failed to collect workspace metric")
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

	// Every metric below is best-effort: a workspace database that predates a
	// feature has no table for it, and one absent table must not cost the whole
	// payload. What changed is that a failure is now logged instead of collapsing
	// into an indistinguishable zero.
	if contactsCount, err := r.CountContacts(ctx, db); err != nil {
		r.metricFailed(workspaceID, "contacts_count", err)
	} else {
		metrics.ContactsCount = contactsCount
	}

	if broadcastsCount, err := r.CountBroadcasts(ctx, db); err != nil {
		r.metricFailed(workspaceID, "broadcasts_count", err)
	} else {
		metrics.BroadcastsCount = broadcastsCount
	}

	if transactionalCount, err := r.CountTransactional(ctx, db); err != nil {
		r.metricFailed(workspaceID, "transactional_count", err)
	} else {
		metrics.TransactionalCount = transactionalCount
	}

	if messagesCount, err := r.CountMessages(ctx, db); err != nil {
		r.metricFailed(workspaceID, "messages_count", err)
	} else {
		metrics.MessagesCount = messagesCount
	}

	if listsCount, err := r.CountLists(ctx, db); err != nil {
		r.metricFailed(workspaceID, "lists_count", err)
	} else {
		metrics.ListsCount = listsCount
	}

	if segmentsCount, err := r.CountSegments(ctx, db); err != nil {
		r.metricFailed(workspaceID, "segments_count", err)
	} else {
		metrics.SegmentsCount = segmentsCount
	}

	// Users live in the system database, not the workspace one.
	if usersCount, err := r.CountUsers(ctx, systemDB, workspaceID); err != nil {
		r.metricFailed(workspaceID, "users_count", err)
	} else {
		metrics.UsersCount = usersCount
	}

	if blogPostsCount, err := r.CountBlogPosts(ctx, db); err != nil {
		r.metricFailed(workspaceID, "blog_posts_count", err)
	} else {
		metrics.BlogPostsCount = blogPostsCount
	}

	if lastMessageAt, err := r.GetLastMessageAt(ctx, db); err != nil {
		r.metricFailed(workspaceID, "last_message_at", err)
	} else {
		metrics.LastMessageAt = lastMessageAt
	}

	if lastWebSessionAt, err := r.GetLastWebSessionAt(ctx, db); err != nil {
		r.metricFailed(workspaceID, "last_web_session_at", err)
	} else {
		metrics.LastWebSessionAt = lastWebSessionAt
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

// CountUsers counts the human members of a workspace, from the system database.
//
// The join onto users and the api_key exclusion are the same convention
// CountWorkspaceMembersAndInvitations uses for seat counting, and for the same
// reason: CreateAPIKey stores an ordinary user_workspaces row, so counting rows
// alone would report a workspace with two people and nine integrations as
// eleven users. This number exists to describe how many humans share an
// installation, which is one of the two distributions the open-core go/no-go
// rests on, so it has to mean people.
//
// Membership is hard-deleted here — RemoveUserFromWorkspace issues a DELETE and
// the table carries no deleted_at column — so there is nothing to filter for
// soft deletion. An earlier version of this query filtered on deleted_at anyway;
// Postgres rejected it as an undefined column on every run, GetWorkspaceMetrics
// swallowed the error, and users_count was zero for every workspace ever
// reported. Both halves of that are fixed: the predicate is gone and the caller
// now logs.
func (r *telemetryRepository) CountUsers(ctx context.Context, systemDB *sql.DB, workspaceID string) (int, error) {
	query := `SELECT COUNT(*) FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = $1 AND u.type != 'api_key'`
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

// GetLastWebSessionAt gets the date of the most recent web analytics session
// recorded in the workspace.
//
// MAX() on the partition key rather than a filtered EXISTS. web_sessions is
// RANGE partitioned by month on session_date with PRIMARY KEY (session_date,
// id), so MAX plans as an ordered Append of backward index-only scans that
// stops at the first non-empty partition — measured on 400k rows across 14
// monthly partitions: 14 shared buffer hits, and every partition below the
// newest non-empty one reported "never executed".
//
// A cutoff predicate would answer the same question, but its plan depends on
// the planner believing the range is selective, and it can only ever answer
// "recent". MAX answers both "ever" and "recent" from one round trip and leaves
// the window a decision the caller makes, without a second query when it moves.
//
// A workspace database that predates the web analytics tables has no
// web_sessions relation and errors here; GetWorkspaceMetrics swallows that the
// same way it swallows every other per-metric failure.
func (r *telemetryRepository) GetLastWebSessionAt(ctx context.Context, db *sql.DB) (string, error) {
	query := `SELECT MAX(session_date) FROM web_sessions`

	// An aggregate over no rows still returns one row, holding NULL — so the
	// empty case arrives as an invalid NullTime, never as sql.ErrNoRows.
	var lastWebSessionAt sql.NullTime
	err := db.QueryRowContext(ctx, query).Scan(&lastWebSessionAt)
	if err != nil {
		return "", fmt.Errorf("failed to get last web session date: %w", err)
	}

	if !lastWebSessionAt.Valid {
		return "", nil // No web analytics sessions recorded
	}

	return lastWebSessionAt.Time.Format(time.RFC3339), nil
}

// getSystemConnection is a helper method to get the system database connection
func (r *telemetryRepository) getSystemConnection(ctx context.Context) (*sql.DB, error) {
	return r.workspaceRepo.GetSystemConnection(ctx)
}
