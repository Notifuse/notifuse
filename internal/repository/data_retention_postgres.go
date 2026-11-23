package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

type dataRetentionRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewDataRetentionRepository creates the PostgreSQL repository of the retention engine.
func NewDataRetentionRepository(workspaceRepo domain.WorkspaceRepository) domain.DataRetentionRepository {
	return &dataRetentionRepository{workspaceRepo: workspaceRepo}
}

// inPurge runs fn in a transaction with notifuse.retention_purge on: the segment triggers
// write no segment.left for an erased contact, and the contacts webhook announces a deleted
// contact by its address only.
func inPurge(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('`+schema.RetentionPurgeGUC+`', 'on', true)`); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// expiredContactPredicate is the D5 clock against $2, on contacts c joined with their
// engagement row e: the later of their collection, their last engagement and their last
// designated custom event ($3; a future date counts as now, a soft-deleted event not at all)
// is before $2. Never within RetentionContactFloor of the contact being stored, and never a
// member of an exempt list ($4, any status, not removed) or segment ($5).
const expiredContactPredicate = `
	c.db_created_at <= NOW() - INTERVAL '7 days'
	AND GREATEST(
		COALESCE(e.last_engaged_at, '-infinity'::timestamptz),
		LEAST(c.created_at, c.db_created_at),
		COALESCE((
			SELECT MAX(LEAST(ce.occurred_at, NOW())) FROM custom_events ce
			WHERE ce.email = c.email AND ce.event_name = ANY($3::text[]) AND ce.deleted_at IS NULL
		), '-infinity'::timestamptz)
	) < $2
	AND NOT EXISTS (
		SELECT 1 FROM contact_lists cl
		WHERE cl.email = c.email AND cl.list_id = ANY($4::text[]) AND cl.deleted_at IS NULL
	)
	AND NOT EXISTS (
		SELECT 1 FROM contact_segments cs WHERE cs.email = c.email AND cs.segment_id = ANY($5::text[])
	)`

func expiredArgs(q domain.ExpiredContactsQuery) []interface{} {
	return []interface{}{q.AfterEmail, q.EngagedBefore,
		pq.Array(nonNil(q.EngagementEventNames)), pq.Array(nonNil(q.ExemptListIDs)), pq.Array(nonNil(q.ExemptSegmentIDs))}
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (r *dataRetentionRepository) db(ctx context.Context, workspaceID string) (*sql.DB, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	return db, nil
}

func (r *dataRetentionRepository) ListExpiredContacts(ctx context.Context, workspaceID string, q domain.ExpiredContactsQuery) ([]string, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT c.email FROM contacts c
		LEFT JOIN contact_engagement e ON e.email = c.email
		WHERE c.email > $1 AND `+expiredContactPredicate+`
		ORDER BY c.email LIMIT $6`, append(expiredArgs(q), q.Limit)...)
	if err != nil {
		return nil, fmt.Errorf("failed to list expired contacts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

func (r *dataRetentionRepository) CountExpiredContacts(ctx context.Context, workspaceID string, q domain.ExpiredContactsQuery) (int64, int64, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return 0, 0, err
	}
	q.AfterEmail = ""
	var contacts, optOuts int64
	err = db.QueryRowContext(ctx, `WITH expired AS (
			SELECT c.email FROM contacts c
			LEFT JOIN contact_engagement e ON e.email = c.email
			WHERE c.email > $1 AND `+expiredContactPredicate+`
		)
		SELECT (SELECT COUNT(*) FROM expired),
			(SELECT COUNT(*) FROM contact_lists cl JOIN expired x ON x.email = cl.email
			 WHERE cl.status IN ('unsubscribed', 'bounced', 'complained'))`, expiredArgs(q)...).Scan(&contacts, &optOuts)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count expired contacts: %w", err)
	}
	return contacts, optOuts, nil
}

// deleteExpiredListRowsSQL is Lot 0's delete-and-hash statement in its batch form: the rows
// are deleted and their opt-outs hashed in one statement.
const deleteExpiredListRowsSQL = `WITH gone AS (
		DELETE FROM contact_lists WHERE email = ANY($1::text[])
		RETURNING email, list_id, status, deleted_at, updated_at
	)
	INSERT INTO contact_opt_out_hashes (email_hash, list_id, status, removed, opted_out_at, recorded_at, source)
	SELECT contact_opt_out_hash(gone.email), gone.list_id, gone.status, gone.deleted_at IS NOT NULL, gone.updated_at, NOW(), $2
	FROM gone
	WHERE gone.status IN ('unsubscribed', 'bounced', 'complained')
	ON CONFLICT (email_hash, list_id) DO UPDATE
	SET status = EXCLUDED.status, removed = EXCLUDED.removed, opted_out_at = EXCLUDED.opted_out_at,
		recorded_at = EXCLUDED.recorded_at, source = EXCLUDED.source
	WHERE EXCLUDED.opted_out_at >= contact_opt_out_hashes.opted_out_at`

// DeleteExpiredContacts runs the short transaction of the batch erasure. The contacts are
// locked and checked again (one may have engaged since it was listed); those still expired
// are queued for the rest of their history, then lose their list rows (opt-outs hashed),
// their queued mail, their segment memberships and their contact row. The rest of their
// history is erased from the queue afterwards, statement by statement: a crash leaves the
// queue to finish from.
func (r *dataRetentionRepository) DeleteExpiredContacts(ctx context.Context, workspaceID, runID string, q domain.ExpiredContactsQuery, emails []string) ([]string, int64, error) {
	if len(emails) == 0 {
		return nil, 0, nil
	}
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	var deleted []string
	var optOuts int64
	q.AfterEmail = "" // the recheck covers the given addresses, wherever the listing stood
	err = inPurge(ctx, db, func(tx *sql.Tx) error {
		// The exemptions are locked for the transaction and checked again: an exempt list or
		// segment deleted since the run started, or a segment being rebuilt, would let its
		// members through the NOT EXISTS below. Deleting or rebuilding one waits for this
		// transaction; one already gone stops the category.
		if err := lockExemptions(ctx, tx, q.ExemptListIDs, q.ExemptSegmentIDs); err != nil {
			return err
		}
		args := append(expiredArgs(q), pq.Array(emails))
		rows, err := tx.QueryContext(ctx, `SELECT c.email FROM contacts c
			LEFT JOIN contact_engagement e ON e.email = c.email
			WHERE c.email > $1 AND c.email = ANY($6::text[]) AND `+expiredContactPredicate+`
			ORDER BY c.email
			FOR UPDATE OF c`, args...)
		if err != nil {
			return fmt.Errorf("failed to lock expired contacts: %w", err)
		}
		for rows.Next() {
			var email string
			if err := rows.Scan(&email); err != nil {
				_ = rows.Close()
				return err
			}
			deleted = append(deleted, email)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(deleted) == 0 {
			return nil
		}
		doomed := pq.Array(deleted)
		if _, err := tx.ExecContext(ctx, `INSERT INTO data_retention_erasure_queue (email, run_id)
			SELECT unnest($1::text[]), $2
			ON CONFLICT (email) DO UPDATE SET run_id = EXCLUDED.run_id`, doomed, runID); err != nil {
			return fmt.Errorf("failed to queue the erasure: %w", err)
		}
		result, err := tx.ExecContext(ctx, deleteExpiredListRowsSQL, doomed, string(domain.OptOutHashSourceRetention))
		if err != nil {
			return fmt.Errorf("failed to record opt-out hashes: %w", err)
		}
		if optOuts, err = result.RowsAffected(); err != nil {
			return err
		}
		for _, statement := range []string{
			`DELETE FROM email_queue WHERE contact_email = ANY($1::text[])`,
			`DELETE FROM contact_segments WHERE email = ANY($1::text[])`,
			`DELETE FROM contacts WHERE email = ANY($1::text[])`,
		} {
			if _, err := tx.ExecContext(ctx, statement, doomed); err != nil {
				return fmt.Errorf("failed to delete expired contacts: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return deleted, optOuts, nil
}

func lockExemptions(ctx context.Context, tx *sql.Tx, listIDs, segmentIDs []string) error {
	check := func(query string, ids []string) error {
		if len(ids) == 0 {
			return nil
		}
		want := map[string]struct{}{}
		for _, id := range ids {
			want[id] = struct{}{}
		}
		var found int
		if err := tx.QueryRowContext(ctx, query, pq.Array(ids)).Scan(&found); err != nil {
			return err
		}
		if found != len(want) {
			return domain.ErrRetentionExemptionUnavailable
		}
		return nil
	}
	if err := check(`SELECT COUNT(*) FROM (SELECT id FROM lists WHERE id = ANY($1::text[]) AND deleted_at IS NULL FOR SHARE) l`, listIDs); err != nil {
		return err
	}
	return check(`SELECT COUNT(*) FROM (SELECT id FROM segments WHERE id = ANY($1::text[]) AND status = 'active' FOR SHARE) s`, segmentIDs)
}

func (r *dataRetentionRepository) Requeue(ctx context.Context, workspaceID, email string) error {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE data_retention_erasure_queue SET enqueued_at = NOW() WHERE email = $1`, email); err != nil {
		return fmt.Errorf("failed to requeue: %w", err)
	}
	return nil
}

func (r *dataRetentionRepository) ErasureQueue(ctx context.Context, workspaceID string, limit int) ([]string, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT email FROM data_retention_erasure_queue ORDER BY enqueued_at, email LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to read the erasure queue: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

func (r *dataRetentionRepository) Dequeue(ctx context.Context, workspaceID, email string) error {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM data_retention_erasure_queue WHERE email = $1`, email); err != nil {
		return fmt.Errorf("failed to dequeue: %w", err)
	}
	return nil
}

func (r *dataRetentionRepository) ContactExists(ctx context.Context, workspaceID, email string) (bool, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM contacts WHERE email = $1)`, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to look the contact up: %w", err)
	}
	return exists, nil
}

// The purge statement of each category: $1 the date before which rows go, $2 the batch size.
// Each selects a bounded batch by its own date and deletes it by primary key.
var purgeStatements = map[domain.RetentionCategory]string{
	domain.RetentionContactTimeline: `WITH doomed AS (
			SELECT id FROM contact_timeline WHERE created_at < $1 ORDER BY created_at LIMIT $2
		)
		DELETE FROM contact_timeline t USING doomed WHERE t.id = doomed.id`,
	domain.RetentionInboundWebhookEvents: `WITH doomed AS (
			SELECT id FROM inbound_webhook_events WHERE timestamp < $1 ORDER BY timestamp LIMIT $2
		)
		DELETE FROM inbound_webhook_events t USING doomed WHERE t.id = doomed.id`,
	domain.RetentionWebhookDeliveries: `WITH doomed AS (
			SELECT id FROM webhook_deliveries WHERE created_at < $1 ORDER BY created_at LIMIT $2
		)
		DELETE FROM webhook_deliveries t USING doomed WHERE t.id = doomed.id`,
	// The rows the queue gave up on, as its own sweep selects them.
	domain.RetentionFailedEmails: `WITH doomed AS (
			SELECT id FROM email_queue
			WHERE status = 'failed' AND next_retry_at IS NULL AND processed_at IS NOT NULL AND processed_at < $1
			ORDER BY processed_at LIMIT $2
		)
		DELETE FROM email_queue t USING doomed WHERE t.id = doomed.id`,
	// Finished journeys only (their node executions cascade). Never the trigger log: it is
	// what keeps a "once" automation from enrolling a contact twice.
	domain.RetentionAutomationHistory: `WITH doomed AS (
			SELECT id FROM contact_automations
			WHERE status IN ('completed', 'exited', 'failed') AND entered_at < $1
			ORDER BY entered_at LIMIT $2
		)
		DELETE FROM contact_automations t USING doomed WHERE t.id = doomed.id`,
	// Designated events are folded into the contact's engagement date before they go (D10),
	// so the custom-events period cannot shorten a contact's clock. $3 holds the names.
	domain.RetentionCustomEvents: `WITH doomed AS (
			SELECT event_name, external_id, email, occurred_at, deleted_at FROM custom_events
			WHERE occurred_at < $1 OR (deleted_at IS NOT NULL AND deleted_at < $1)
			ORDER BY occurred_at LIMIT $2
			FOR UPDATE
		), folded AS (
			INSERT INTO contact_engagement (email, last_engaged_at)
			SELECT d.email, MAX(LEAST(d.occurred_at, NOW())) FROM doomed d
			WHERE d.event_name = ANY($3::text[]) AND d.deleted_at IS NULL
				AND EXISTS (SELECT 1 FROM contacts c WHERE c.email = d.email FOR KEY SHARE)
			GROUP BY d.email
			ORDER BY d.email
			ON CONFLICT (email) DO UPDATE
			SET last_engaged_at = GREATEST(contact_engagement.last_engaged_at, EXCLUDED.last_engaged_at)
		)
		DELETE FROM custom_events ce USING doomed d
		WHERE ce.event_name = d.event_name AND ce.external_id = d.external_id`,
}

var countStatements = map[domain.RetentionCategory]string{
	domain.RetentionMessageHistory:       `SELECT COUNT(*) FROM message_history WHERE created_at < $1 AND contact_email <> 'DELETED_EMAIL'`,
	domain.RetentionContactTimeline:      `SELECT COUNT(*) FROM contact_timeline WHERE created_at < $1`,
	domain.RetentionInboundWebhookEvents: `SELECT COUNT(*) FROM inbound_webhook_events WHERE timestamp < $1`,
	domain.RetentionWebhookDeliveries:    `SELECT COUNT(*) FROM webhook_deliveries WHERE created_at < $1`,
	domain.RetentionFailedEmails: `SELECT COUNT(*) FROM email_queue
		WHERE status = 'failed' AND next_retry_at IS NULL AND processed_at IS NOT NULL AND processed_at < $1`,
	domain.RetentionAutomationHistory: `SELECT COUNT(*) FROM contact_automations
		WHERE status IN ('completed', 'exited', 'failed') AND entered_at < $1`,
	domain.RetentionCustomEvents: `SELECT COUNT(*) FROM custom_events
		WHERE occurred_at < $1 OR (deleted_at IS NOT NULL AND deleted_at < $1)`,
}

// redactMessageHistorySQL redacts a batch of messages older than $1, walking a keyset cursor
// on (created_at, id) that starts after ($2, $3). It wipes what P5 lists and keeps the row,
// whose timestamps the broadcast statistics read. The attachment files only these messages
// held are deleted with them; one another message or a queued email still names is kept.
// It returns the rows redacted and the new cursor.
const redactMessageHistorySQL = `WITH batch AS (
		SELECT id, created_at FROM message_history
		WHERE created_at < $1 AND (created_at, id) > ($2::timestamptz, $3::varchar)
			AND contact_email <> 'DELETED_EMAIL'
		ORDER BY created_at, id
		LIMIT $4
	), files AS (
		SELECT DISTINCT elem->>'checksum' AS checksum
		FROM message_history mh
		JOIN batch b ON b.id = mh.id,
			jsonb_array_elements(CASE WHEN jsonb_typeof(mh.attachments) = 'array' THEN mh.attachments ELSE '[]'::jsonb END) elem
	), redacted AS (
		UPDATE message_history mh
		SET contact_email = 'DELETED_EMAIL', clicked_links = NULL, message_data = '{}'::jsonb,
			channel_options = NULL, status_info = NULL, attachments = NULL
		FROM batch b WHERE mh.id = b.id
		RETURNING mh.id
	), gone_files AS (
		DELETE FROM message_attachments a
		USING files f
		WHERE a.checksum = f.checksum
		AND NOT EXISTS (
			SELECT 1 FROM message_history other
			WHERE other.id NOT IN (SELECT id FROM batch)
			AND other.attachments @> jsonb_build_array(jsonb_build_object('checksum', a.checksum))
		)
		AND NOT EXISTS (SELECT 1 FROM email_queue q WHERE strpos(q.payload::text, a.checksum) > 0)
	), last AS (
		SELECT created_at, id FROM batch ORDER BY created_at DESC, id DESC LIMIT 1
	)
	SELECT (SELECT COUNT(*) FROM redacted), (SELECT created_at FROM last), (SELECT id FROM last)`

// timelineCreatedAtIndex is the plain index the timeline purge needs (the partial
// idx_contact_timeline_billable leaves out web rows). It is built CONCURRENTLY the first time
// the category runs on a workspace, not by the migration: a blocking build on the busiest
// table would stop writes, and every workspace would pay it for an opt-in feature.
const timelineCreatedAtIndex = "idx_contact_timeline_created_at"

func (r *dataRetentionRepository) ensureTimelineIndex(ctx context.Context, db *sql.DB) error {
	var valid sql.NullBool
	err := db.QueryRowContext(ctx, `SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = $1`, timelineCreatedAtIndex).Scan(&valid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && valid.Bool {
		return nil
	}
	if err == nil {
		// A build interrupted earlier leaves an invalid index behind.
		if _, err := db.ExecContext(ctx, `DROP INDEX CONCURRENTLY IF EXISTS `+timelineCreatedAtIndex); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx, `CREATE INDEX CONCURRENTLY IF NOT EXISTS `+timelineCreatedAtIndex+` ON contact_timeline(created_at)`)
	return err
}

func (r *dataRetentionRepository) PurgeCategory(ctx context.Context, workspaceID string, category domain.RetentionCategory, before time.Time, limit int, engagementEventNames []string) (int64, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	if category == domain.RetentionMessageHistory {
		return r.redactMessageHistory(ctx, db, before, limit)
	}
	statement, ok := purgeStatements[category]
	if !ok {
		return 0, fmt.Errorf("no purge statement for %s", category)
	}
	if category == domain.RetentionContactTimeline {
		if err := r.ensureTimelineIndex(ctx, db); err != nil {
			return 0, fmt.Errorf("failed to build the timeline index: %w", err)
		}
	}
	args := []interface{}{before, limit}
	if category == domain.RetentionCustomEvents {
		args = append(args, pq.Array(nonNil(engagementEventNames)))
	}
	var affected int64
	err = inPurge(ctx, db, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, statement, args...)
		if err != nil {
			return err
		}
		affected, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("failed to purge %s: %w", category, err)
	}
	return affected, nil
}

func (r *dataRetentionRepository) redactMessageHistory(ctx context.Context, db *sql.DB, before time.Time, limit int) (int64, error) {
	var redacted int64
	err := inPurge(ctx, db, func(tx *sql.Tx) error {
		var cursorAt sql.NullTime
		var cursorID sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT message_history_cursor_at, message_history_cursor_id FROM data_retention_state WHERE id = 1 FOR UPDATE`).
			Scan(&cursorAt, &cursorID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		from := time.Time{}
		if cursorAt.Valid {
			from = cursorAt.Time
		}
		var lastAt sql.NullTime
		var lastID sql.NullString
		if err := tx.QueryRowContext(ctx, redactMessageHistorySQL, before, from, cursorID.String, limit).Scan(&redacted, &lastAt, &lastID); err != nil {
			return err
		}
		if !lastAt.Valid {
			return nil
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO data_retention_state (id, message_history_cursor_at, message_history_cursor_id) VALUES (1, $1, $2)
			ON CONFLICT (id) DO UPDATE SET message_history_cursor_at = EXCLUDED.message_history_cursor_at,
				message_history_cursor_id = EXCLUDED.message_history_cursor_id`, lastAt.Time, lastID.String)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("failed to redact message history: %w", err)
	}
	return redacted, nil
}

func (r *dataRetentionRepository) CountCategory(ctx context.Context, workspaceID string, category domain.RetentionCategory, before time.Time) (int64, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	statement, ok := countStatements[category]
	if !ok {
		return 0, fmt.Errorf("no count statement for %s", category)
	}
	var n int64
	if err := db.QueryRowContext(ctx, statement, before).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count %s: %w", category, err)
	}
	return n, nil
}

func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (r *dataRetentionRepository) WebAnalyticsMonthsBefore(ctx context.Context, workspaceID string, month time.Time) ([]time.Time, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT c.relname FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname IN ('web_sessions', 'web_pages', 'web_goals')`)
	if err != nil {
		return nil, fmt.Errorf("failed to list web analytics partitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	limit := monthStart(month)
	seen := map[time.Time]struct{}{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if _, m, ok := schema.ParseWebAnalyticsPartitionName(name); ok && m.Before(limit) {
			seen[m] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	months := make([]time.Time, 0, len(seen))
	for m := range seen {
		months = append(months, m)
	}
	sort.Slice(months, func(i, j int) bool { return months[i].Before(months[j]) })
	return months, nil
}

// DropWebAnalyticsMonth drops one month's partitions with a short lock_timeout: a DROP takes
// an ACCESS EXCLUSIVE lock on the parent and would otherwise queue behind a dashboard read,
// stalling ingestion behind it. A timeout fails the month, which the next run retries.
func (r *dataRetentionRepository) DropWebAnalyticsMonth(ctx context.Context, workspaceID string, month time.Time, now time.Time) error {
	month = monthStart(month)
	if !month.Before(monthStart(now)) {
		return fmt.Errorf("refusing to drop the web analytics partitions of %s: not a past month", month.Format("2006-01"))
	}
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	for _, table := range []string{"web_goals", "web_pages", "web_sessions"} {
		name := schema.WebAnalyticsPartitionName(table, month)
		if _, _, ok := schema.ParseWebAnalyticsPartitionName(name); !ok {
			return fmt.Errorf("unexpected partition name %q", name)
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS `+pq.QuoteIdentifier(name)); err != nil {
			return fmt.Errorf("failed to drop %s: %w", name, err)
		}
	}
	return tx.Commit()
}

func (r *dataRetentionRepository) BackfillCompleted(ctx context.Context, workspaceID string) (bool, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	var done bool
	err = db.QueryRowContext(ctx, `SELECT backfill_completed_at IS NOT NULL FROM data_retention_state WHERE id = 1`).Scan(&done)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the engagement backfill state: %w", err)
	}
	return done, nil
}

func (r *dataRetentionRepository) MissingExemptions(ctx context.Context, workspaceID string, listIDs, segmentIDs []string) ([]string, error) {
	if len(listIDs) == 0 && len(segmentIDs) == 0 {
		return nil, nil
	}
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	collect := func(query string, ids []string, prefix string) error {
		if len(ids) == 0 {
			return nil
		}
		rows, err := db.QueryContext(ctx, query, pq.Array(ids))
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			found[prefix+id] = true
		}
		return rows.Err()
	}
	if err := collect(`SELECT id FROM lists WHERE id = ANY($1::text[]) AND deleted_at IS NULL`, listIDs, "list:"); err != nil {
		return nil, fmt.Errorf("failed to check the exempt lists: %w", err)
	}
	// A segment being rebuilt has an incomplete membership: not to be trusted as an exemption.
	if err := collect(`SELECT id FROM segments WHERE id = ANY($1::text[]) AND status = 'active'`, segmentIDs, "segment:"); err != nil {
		return nil, fmt.Errorf("failed to check the exempt segments: %w", err)
	}
	var missing []string
	for _, id := range listIDs {
		if !found["list:"+id] {
			missing = append(missing, "list:"+id)
		}
	}
	for _, id := range segmentIDs {
		if !found["segment:"+id] {
			missing = append(missing, "segment:"+id)
		}
	}
	return missing, nil
}

// retentionLockKey is the advisory lock of a workspace's retention run. Each workspace has its
// own database, so one key per database is enough.
const retentionLockKey = "notifuse.data_retention"

// TryLock takes a session-level advisory lock on a connection of its own, held for the whole
// run. Behind a transaction-mode pooler a session lock does not hold; the docs say so.
func (r *dataRetentionRepository) TryLock(ctx context.Context, workspaceID string) (func(), bool, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return nil, false, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to take a connection for the retention lock: %w", err)
	}
	var ok bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, retentionLockKey).Scan(&ok); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("failed to take the retention lock: %w", err)
	}
	if !ok {
		_ = conn.Close()
		return nil, false, nil
	}
	release := func() {
		// A fresh context: the run's may be cancelled by then, and the lock must go.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, retentionLockKey)
		_ = conn.Close()
	}
	return release, true, nil
}

func (r *dataRetentionRepository) InsertRun(ctx context.Context, workspaceID string, run *domain.RetentionRun) error {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return err
	}
	policy, err := json.Marshal(run.Policy)
	if err != nil {
		return err
	}
	// A run still "running" here was cut short (a restart, a crash): the caller holds the
	// workspace's lock, so no other run is in progress.
	if _, err := db.ExecContext(ctx, `UPDATE data_retention_runs SET status = 'failed', finished_at = NOW(),
		error = 'interrupted before it finished' WHERE status = 'running'`); err != nil {
		return fmt.Errorf("failed to close interrupted retention runs: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO data_retention_runs (id, started_at, status, policy) VALUES ($1, $2, $3, $4)`,
		run.ID, run.StartedAt, string(run.Status), policy); err != nil {
		return fmt.Errorf("failed to record the retention run: %w", err)
	}
	return nil
}

func (r *dataRetentionRepository) FinishRun(ctx context.Context, workspaceID string, run *domain.RetentionRun) error {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return err
	}
	results, err := json.Marshal(run.Results)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE data_retention_runs SET finished_at = $2, status = $3, results = $4, error = NULLIF($5, '') WHERE id = $1`,
		run.ID, run.FinishedAt, string(run.Status), results, run.Error); err != nil {
		return fmt.Errorf("failed to record the retention run: %w", err)
	}
	return nil
}

func (r *dataRetentionRepository) ListRuns(ctx context.Context, workspaceID string, limit int) ([]*domain.RetentionRun, error) {
	db, err := r.db(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, started_at, finished_at, status, policy, results, COALESCE(error, '')
		FROM data_retention_runs ORDER BY started_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list retention runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var runs []*domain.RetentionRun
	for rows.Next() {
		run := &domain.RetentionRun{}
		var finished sql.NullTime
		var status string
		var policy, results []byte
		if err := rows.Scan(&run.ID, &run.StartedAt, &finished, &status, &policy, &results, &run.Error); err != nil {
			return nil, err
		}
		run.Status = domain.RetentionRunStatus(status)
		if finished.Valid {
			t := finished.Time
			run.FinishedAt = &t
		}
		if err := json.Unmarshal(policy, &run.Policy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(results, &run.Results); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}
