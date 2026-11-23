package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/domain"
)

type contactEngagementRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewContactEngagementRepository creates the PostgreSQL contact engagement repository.
func NewContactEngagementRepository(workspaceRepo domain.WorkspaceRepository) domain.ContactEngagementRepository {
	return &contactEngagementRepository{workspaceRepo: workspaceRepo}
}

// engagementUpsert ends every write: the later date wins, and a move of less than an hour is
// not written, so a busy contact does not rewrite the row on every click or page.
const engagementUpsert = `
	ON CONFLICT (email) DO UPDATE SET last_engaged_at = EXCLUDED.last_engaged_at
	WHERE EXCLUDED.last_engaged_at > contact_engagement.last_engaged_at + INTERVAL '1 hour'`

// Each statement selects from contacts, so an engagement that arrives after the contact was
// deleted (a click on an old email, a late reply) writes nothing. The contact row is locked
// FOR KEY SHARE: a touch racing a deletion either waits for it and then finds no contact, or
// commits first and is removed by the deletion's engagement step. A date in the future,
// which a provider or a client clock can send, is capped at now.
const (
	touchEngagementSQL = `INSERT INTO contact_engagement (email, last_engaged_at)
	SELECT c.email, LEAST($2::timestamptz, NOW()) FROM contacts c WHERE c.email = $1 FOR KEY SHARE` + engagementUpsert

	// A click counts only where it is recorded at all: clicksRecordableCondition leaves out
	// a message sent with click tracking off and a contact who refused click tracking.
	// Deriving engagement from a click the contact refused to have tracked would be click
	// tracking by another name.
	touchEngagementForMessageSQL = `INSERT INTO contact_engagement (email, last_engaged_at)
	SELECT message_history.contact_email, LEAST($2::timestamptz, NOW())
	FROM message_history
	WHERE message_history.id = $1
		AND EXISTS (SELECT 1 FROM contacts c WHERE c.email = message_history.contact_email FOR KEY SHARE)
		AND ` + clicksRecordableCondition + engagementUpsert

	// One statement per partition date, as the timeline projection reads web_sessions: a
	// lookup by id alone would scan every monthly partition. Rows in email order, so two
	// replicas writing the same contacts take their locks in the same order.
	touchEngagementForWebSessionsSQL = `INSERT INTO contact_engagement (email, last_engaged_at)
	SELECT s.contact_email, MAX(LEAST(s.updated_at, NOW()))
	FROM web_sessions s
	WHERE s.session_date = $1
		AND s.id = ANY($2::uuid[])
		AND s.contact_email IS NOT NULL
		AND EXISTS (SELECT 1 FROM contacts c WHERE c.email = s.contact_email FOR KEY SHARE)
	GROUP BY s.contact_email
	ORDER BY s.contact_email` + engagementUpsert
)

func (r *contactEngagementRepository) Touch(ctx context.Context, workspaceID, email string, at time.Time) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	if _, err := db.ExecContext(ctx, touchEngagementSQL, email, at); err != nil {
		return fmt.Errorf("failed to record engagement: %w", err)
	}
	return nil
}

func (r *contactEngagementRepository) TouchForMessage(ctx context.Context, workspaceID, messageID string, at time.Time) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	if _, err := db.ExecContext(ctx, touchEngagementForMessageSQL, messageID, at); err != nil {
		return fmt.Errorf("failed to record engagement for message: %w", err)
	}
	return nil
}

func (r *contactEngagementRepository) TouchForWebSessions(ctx context.Context, workspaceID string, sessions []*domain.WebSession) error {
	byDate := map[time.Time]map[string]struct{}{}
	for _, s := range sessions {
		if s == nil {
			continue
		}
		if byDate[s.SessionDate] == nil {
			byDate[s.SessionDate] = map[string]struct{}{}
		}
		byDate[s.SessionDate][s.ID] = struct{}{}
	}
	if len(byDate) == 0 {
		return nil
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	dates := make([]time.Time, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	for _, date := range dates {
		ids := make([]string, 0, len(byDate[date]))
		for id := range byDate[date] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if _, err := db.ExecContext(ctx, touchEngagementForWebSessionsSQL, date, pq.Array(ids)); err != nil {
			return fmt.Errorf("failed to record engagement for web sessions: %w", err)
		}
	}
	return nil
}

// backfillEngagementSQL reads one batch of contacts' engagement back from history: the last
// click on any of their emails, their last reply and double opt-in confirmation, and their
// last identified visit (all three from the timeline). A past subscription is not read:
// history does not say whether the contact or the workspace made it (plan, P3).
//
// A click is read per message. Where clicked_links lists the links clicked, the latest
// last_at among real destinations counts: a link back to the notification center or the
// one-click unsubscribe is an opposition, not an interest, and is left out by its URL (it is
// not recorded there at all since links back to the instance record no destination). Where
// clicked_links lists nothing, the click came before per-link tracking, or only went to a
// link back to the instance, and clicked_at cannot tell which: it counts, which errs on
// keeping the contact. A contact who refused click tracking gets nothing from their clicks.
const backfillEngagementSQL = `WITH batch AS (
		SELECT email FROM contacts WHERE email > $1 ORDER BY email LIMIT $2
	), clicks AS (
		SELECT mh.contact_email AS email, MAX(
			CASE WHEN jsonb_typeof(mh.clicked_links) = 'object' AND mh.clicked_links <> '{}'::jsonb THEN (
				SELECT MAX((l.value ->> 'last_at')::timestamptz)
				FROM jsonb_each(mh.clicked_links) AS l(url, value)
				WHERE jsonb_typeof(l.value) = 'object'
					AND l.value ->> 'last_at' ~ '^\d{4}-\d{2}-\d{2}T'
					AND l.url NOT LIKE '%/notification-center%'
					AND l.url NOT LIKE '%/unsubscribe-oneclick%'
			) ELSE mh.clicked_at END) AS at
		FROM message_history mh JOIN batch b ON b.email = mh.contact_email
		WHERE mh.clicked_at IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.email = mh.contact_email AND c.tracking_clicks_consent = 'denied')
		GROUP BY mh.contact_email
	), actions AS (
		SELECT t.email, MAX(t.created_at) AS at
		FROM contact_timeline t JOIN batch b ON b.email = t.email
		WHERE t.kind IN ('email.replied', 'list.confirmed') OR t.entity_type = 'web_session'
		GROUP BY t.email
	), merged AS (
		SELECT email, MAX(LEAST(at, NOW())) AS at
		FROM (SELECT email, at FROM clicks UNION ALL SELECT email, at FROM actions) everything
		WHERE at IS NOT NULL
		GROUP BY email
	), written AS (
		-- In email order, as the live writes, so the two cannot deadlock.
		INSERT INTO contact_engagement (email, last_engaged_at)
		SELECT email, at FROM merged ORDER BY email
		ON CONFLICT (email) DO UPDATE
		SET last_engaged_at = GREATEST(contact_engagement.last_engaged_at, EXCLUDED.last_engaged_at)
		RETURNING 1
	)
	SELECT (SELECT MAX(email) FROM batch), (SELECT COUNT(*) FROM batch), (SELECT COUNT(*) FROM written)`

func (r *contactEngagementRepository) BackfillBatch(ctx context.Context, workspaceID, afterEmail string, limit int) (string, int64, int64, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	var last sql.NullString
	var contacts, engaged int64
	if err := db.QueryRowContext(ctx, backfillEngagementSQL, afterEmail, limit).Scan(&last, &contacts, &engaged); err != nil {
		return "", 0, 0, fmt.Errorf("failed to backfill engagement: %w", err)
	}
	return last.String, contacts, engaged, nil
}

func (r *contactEngagementRepository) CompleteBackfill(ctx context.Context, workspaceID string) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO data_retention_state (id, backfill_completed_at) VALUES (1, NOW())
		ON CONFLICT (id) DO UPDATE SET backfill_completed_at = COALESCE(data_retention_state.backfill_completed_at, EXCLUDED.backfill_completed_at)`); err != nil {
		return fmt.Errorf("failed to mark the engagement backfill complete: %w", err)
	}
	return nil
}

func (r *contactEngagementRepository) BackfillCompleted(ctx context.Context, workspaceID string) (bool, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	var done bool
	err = db.QueryRowContext(ctx, `SELECT backfill_completed_at IS NOT NULL FROM data_retention_state WHERE id = 1`).Scan(&done)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the engagement backfill state: %w", err)
	}
	return done, nil
}
