package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/domain"
)

// webNavigationMaxPagesPerSession bounds how many pageviews of one session one
// projection run writes. /track is public and a beat may carry
// WebTrackMaxActions pageviews, so without it a single beat becomes that many
// timeline rows for one contact.
//
// The cap is a window over the whole session rather than `page_number <= N`.
// Page numbers are per TAB — web_pages' key is
// (session_date, session_id, tab_id, page_number) and each tab renumbers from 1
// — so an ordinal test bounds each tab separately, and WebTrackPayload.TabID is
// an unvalidated int64, so a caller can mint a fresh tab per beat and multiply
// the cap by however many tabs it claims.
//
// It bounds a RUN, not the accumulated total, and that distinction is real: rows
// an earlier run admitted stay when a later run's window ranks them out, so a
// session can hold more than the cap. Bounding the stored total would mean
// counting existing rows per candidate — a correlated subquery over an
// unindexed prefix, and racy besides. The residual is accepted because reaching
// it needs a valid identify credential, which either belongs to the workspace
// (who can already use the API) or was lifted for one address and can therefore
// only pollute that one contact's timeline — and the identified-beat rate limits
// bound how fast either can try.
const webNavigationMaxPagesPerSession = 100

// webNavigationSessionChunkSize bounds how many sessions one projection
// transaction covers.
//
// Deliberately small, because the unit that matters is ROWS, not sessions: each
// session contributes up to webNavigationMaxPagesPerSession page rows plus its
// summary, and ON CONFLICT DO UPDATE locks every conflicting row before
// evaluating the WHERE that may decline it. At 200 sessions a chunk could hold
// locks on ~20,000 timeline rows and 200 contact_segment_queue rows until
// commit — the batch-spanning-many-contacts shape that froze that queue before,
// which is what the chunking exists to prevent. 20 keeps the worst case near
// 2,000 rows.
const webNavigationSessionChunkSize = 20

// The projection writes contact_timeline rows for an identified visitor's web
// navigation, one per pageview and one per session.
//
// It reads the ANALYTICS TABLES rather than the beat that triggered it, and that
// is the whole design. web_sessions and web_pages are already final-state — the
// upserts are guarded by beat_seq, identity and attribution are sticky, and the
// rollups recompute session aggregates and is_landing/is_exit across every tab
// after each flush. A beat, by contrast, carries one tab's pre-rollup view. So
// the timeline row is a projection of settled state, never an accumulation, and
// it converges on the truth without anyone having to detect that a session ended
// — which is fortunate, because the buffer cannot reliably tell: an entry is
// marked clean by any flush and only a new beat marks it dirty again, so a
// writer whose last beat lands just before a periodic flush is never visited by
// the idle-flush path at all.
//
// Idempotency rides on the primary key. contact_timeline.id is a UUID, so the id
// is DERIVED from the natural key with md5(...)::uuid and ON CONFLICT (id)
// infers the existing primary key — no new unique index on the workspace's
// busiest table, and no ON CONFLICT ... WHERE predicate to forget. The buffer's
// in-process cursor could not carry this: a restart, an eviction or a second
// replica would each duplicate every row.
//
// The guard on DO UPDATE is content equality, not a sequence number. Every beat
// advances beat_seq on every page row of the writer, so a beat_seq comparison
// would rewrite all of a session's timeline rows on every flush; comparing the
// projected payload instead means a page that has stopped changing costs nothing
// after its first write, and only the page the visitor is currently on is
// rewritten. Updating a timeline row cascades nothing — the segment-queue and
// automation triggers are both AFTER INSERT — so convergence is free where a
// second INSERT would not be.
//
// ORDER BY email gives the AFTER INSERT triggers a deterministic firing order,
// so concurrent replicas take contact_segment_queue's row locks in the same
// sequence. FlushBatch sorts its own inputs for the same reason.
const webNavigationPageProjection = `
INSERT INTO contact_timeline (id, email, operation, entity_type, kind, entity_id, changes, created_at)
SELECT
	md5('web_page:' || p.session_id::text || ':' || p.tab_id::text || ':' || p.page_number::text)::uuid,
	p.contact_email,
	'insert',
	'web_page',
	'web.pageview',
	p.session_id::text || ':' || p.tab_id::text || ':' || p.page_number::text,
	jsonb_build_object(
		'path', jsonb_build_object('new', p.path),
		'page_number', jsonb_build_object('new', p.page_number),
		'duration_ms', jsonb_build_object('new', p.duration_ms),
		'max_scroll', jsonb_build_object('new', p.max_scroll),
		'is_landing', jsonb_build_object('new', p.is_landing),
		'is_exit', jsonb_build_object('new', p.is_exit),
		'entry_type', jsonb_build_object('new', p.entry_type),
		'session_id', jsonb_build_object('new', p.session_id::text),
		-- The domain, so a path can be turned back into the address the visitor
		-- actually opened. web_pages stores paths only, and the console cannot
		-- guess the host: a workspace's configured website URL is a different
		-- field that need not be the tracked one.
		--
		-- The visit's landing domain, since a page has no domain of its own here.
		-- For a visit that crosses hosts this names where it began rather than
		-- where this page was, which is the honest limit of what the table knows.
		'landing_domain', jsonb_build_object('new', COALESCE(s.landing_domain, ''))
	),
	-- Clamped: entered_at is the visitor's own clock, bounded only by
	-- webMaxEpochMs (~year 5138), and it becomes the row's created_at — which
	-- contact_timeline is indexed and ordered on, and which every "in the last N
	-- days" segment condition compares against. A single far-future beat would
	-- otherwise pin that contact to the top of the drawer and match every
	-- recency condition, permanently.
	LEAST(p.entered_at, CURRENT_TIMESTAMP)
FROM (
	SELECT *, row_number() OVER (
		-- Newest first: the cap keeps the most RECENT pages of an over-long
		-- session. Keeping the earliest instead left the session row naming an
		-- exit_path with no pageview row for it, so the drawer showed a visit
		-- ending on a page it did not list and the two console conditions
		-- disagreed. The landing page stays reachable through the session row.
		PARTITION BY session_id ORDER BY entered_at DESC, tab_id DESC, page_number DESC
	) AS session_rank
	FROM web_pages
	WHERE session_date = $1
		AND session_id = ANY($2)
		AND contact_email IS NOT NULL
) p
-- LEFT, not INNER: a pageview whose session row is somehow absent still belongs
-- on the timeline. Losing the domain costs a button in the drawer; losing the
-- row costs the visit.
LEFT JOIN web_sessions s ON s.session_date = p.session_date AND s.id = p.session_id
WHERE p.session_rank <= $3
	AND EXISTS (SELECT 1 FROM contacts c WHERE c.email = p.contact_email)
ORDER BY p.contact_email
ON CONFLICT (id) DO UPDATE SET changes = EXCLUDED.changes, email = EXCLUDED.email
WHERE contact_timeline.changes IS DISTINCT FROM EXCLUDED.changes
	OR contact_timeline.email IS DISTINCT FROM EXCLUDED.email
RETURNING email`

// webNavigationSessionProjection writes the one row that summarises the visit.
// created_at is the session's start, not the projection time, so the drawer
// stays chronologically honest and the row does not jump to the top of the
// timeline every time the visitor loads another page.
//
// updated_at is deliberately absent from the payload: it advances on every beat
// and would defeat the content-equality guard, turning every flush into a write.
const webNavigationSessionProjection = `
INSERT INTO contact_timeline (id, email, operation, entity_type, kind, entity_id, changes, created_at)
SELECT
	md5('web_session:' || s.id::text)::uuid,
	s.contact_email,
	'insert',
	'web_session',
	'web.session',
	s.id::text,
	jsonb_build_object(
		'pageview_count', jsonb_build_object('new', s.pageview_count),
		'duration_ms', jsonb_build_object('new', s.duration_ms),
		'max_scroll', jsonb_build_object('new', s.max_scroll),
		'goal_count', jsonb_build_object('new', s.goal_count),
		'goal_value', jsonb_build_object('new', s.goal_value),
		'landing_path', jsonb_build_object('new', s.landing_path),
		'exit_path', jsonb_build_object('new', s.exit_path),
		'referrer_domain', jsonb_build_object('new', s.referrer_domain),
		'is_direct', jsonb_build_object('new', s.is_direct),
		'utm_source', jsonb_build_object('new', s.utm_source),
		'utm_medium', jsonb_build_object('new', s.utm_medium),
		'utm_campaign', jsonb_build_object('new', s.utm_campaign),
		-- The creative, which is what tells two variants of one campaign apart.
		-- The drawer names the whole chain, and a campaign with its content
		-- missing cannot be read back to the ad that produced the visit.
		'utm_content', jsonb_build_object('new', s.utm_content),
		'channel', jsonb_build_object('new', s.channel),
		'channel_group', jsonb_build_object('new', s.channel_group),
		'device', jsonb_build_object('new', s.device),
		'browser', jsonb_build_object('new', s.browser),
		'os', jsonb_build_object('new', s.os),
		'country', jsonb_build_object('new', s.country)
	),
	-- Clamped for the same reason the pageview projection is: web_sessions
	-- .created_at is NOT server-set. It is correctedMs(sessionStart, skew) where
	-- skew comes from the client's own sent_at, which Validate never bounds — so
	-- a beat claiming sent_at=1 yields a session dated decades ahead, frozen on
	-- first insert because created_at is in the upsert's skip list.
	LEAST(s.created_at, CURRENT_TIMESTAMP)
FROM web_sessions s
WHERE s.session_date = $1
	AND s.id = ANY($2)
	AND s.contact_email IS NOT NULL
	AND EXISTS (SELECT 1 FROM contacts c WHERE c.email = s.contact_email)
ORDER BY s.contact_email
ON CONFLICT (id) DO UPDATE SET changes = EXCLUDED.changes, email = EXCLUDED.email
WHERE contact_timeline.changes IS DISTINCT FROM EXCLUDED.changes
	OR contact_timeline.email IS DISTINCT FROM EXCLUDED.email
RETURNING email`

// webNavigationRequeue re-queues the contacts whose navigation rows just changed.
//
// contact_timeline_queue_trigger is AFTER INSERT, so it queues a contact when a
// row is first written and never again — but these rows converge, so a segment
// reading changes->'duration_ms'->>'new' would otherwise evaluate the visit as
// it stood at its first flush and only catch up when some unrelated event
// re-queued that contact. Since the console can now filter on those values, the
// lag would be visible as a segment that simply under-matches.
//
// Redundant for rows that were INSERTed — the trigger already queued those — but
// distinguishing them costs an xmax trick whose semantics vary, and the queue is
// an upsert on email, so a second touch only refreshes queued_at.
// The ORDER BY is the point, not tidiness. The projections above take their
// contact_segment_queue locks through an AFTER INSERT trigger in the order their
// own ORDER BY email produced — that is the DATABASE's collation. Ordering this
// array in Go instead would take the same locks in byte order, and the two
// disagree on ordinary addresses: under en_US.utf8, ORDER BY gives
// bobby@x.com before bob@x.com while Go's sort.Strings gives the reverse,
// because '@' sorts below 'b' by byte and above it by collation weight. Two
// concurrent transactions — one inserting, one only converging — would then
// take the same two rows in opposite orders and deadlock. Workspace databases
// are created without an explicit locale, so they inherit the cluster's.
const webNavigationRequeue = `
INSERT INTO contact_segment_queue (email, queued_at)
SELECT e, CURRENT_TIMESTAMP FROM unnest($1::text[]) e ORDER BY e
ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at`

// webNavigationPreviousOwners finds who these sessions' summary rows currently
// belong to, so an identity switch does not strand the old contact.
//
// contact_email is sticky against NULL but a different address overwrites it, so
// a shared browser or a second login re-points a session at somebody else. The
// upsert then moves the projected rows and RETURNING yields only the NEW
// address — leaving the previous contact holding segment memberships derived
// from rows they no longer own, until something unrelated re-queues them.
//
// Only the session row is consulted: it is an exact `entity_id = ANY(...)` match
// on an indexed column, where the page rows would need a prefix scan, and both
// are projected from the same contact_email so the session row is enough to
// notice the switch.
const webNavigationPreviousOwners = `
SELECT DISTINCT email FROM contact_timeline
WHERE entity_type = 'web_session' AND entity_id = ANY($1)`

// ProjectContactNavigation refreshes the contact timeline for the given
// sessions. Safe to call repeatedly with the same sessions; that is how the
// final state of a visit ends up persisted.
//
// The caller must run this AFTER the analytics flush has committed and outside
// the flush transaction. Inside it, the cascade from contact_timeline into
// contact_segment_queue would hold row locks for the length of a batch spanning
// many contacts — the shape that has frozen that queue before.
//
// The EXISTS against contacts is the erasure guard: a contact deleted while the
// beat was buffered gets no timeline row, even though the web rows still name
// the address until AnonymizeContact reaches them.
func (r *webAnalyticsRepository) ProjectContactNavigation(ctx context.Context, workspaceID string, sessions []*domain.WebSession) error {
	byDate := make(map[time.Time]map[string]struct{}, 2)
	for _, s := range sessions {
		if s == nil {
			continue
		}
		// Identity is decided by the STATEMENTS, against the persisted rows.
		// contact_email is sticky in the database, so a beat that has gone
		// anonymous — a visitor who logged out, or an identify token that lapsed
		// mid-visit — still belongs to a session that is identified, and testing
		// the beat's own copy here would stop refining that visit's timeline for
		// the rest of its life.

		// A set, not a slice: the buffer keys per (session, tab), so a multi-tab
		// visit hands us the same session id once per tab.
		ids := byDate[s.SessionDate]
		if ids == nil {
			ids = map[string]struct{}{}
			byDate[s.SessionDate] = ids
		}
		ids[s.ID] = struct{}{}
	}
	if len(byDate) == 0 {
		return nil
	}

	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Dates in order, ids in order. Map iteration is randomised, and a run that
	// spans two partition dates — a session still beating either side of UTC
	// midnight — would otherwise have two replicas take contact_segment_queue's
	// row locks in opposite date order and deadlock. The ORDER BY email inside
	// each statement only orders within one.
	dates := make([]time.Time, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	var failures []error
	for _, date := range dates {
		ids := make([]string, 0, len(byDate[date]))
		for id := range byDate[date] {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		// One transaction per chunk rather than one for the whole run. The upsert
		// is ON CONFLICT DO UPDATE, which takes a row lock on every conflicting
		// row even when its WHERE declines the update — so an unchunked run holds
		// a lock on every timeline row it projected, plus one contact_segment_queue
		// row per contact it inserted, until commit. At MaxSessionsPerWorkspace
		// that is a transaction spanning tens of thousands of rows and thousands of
		// contacts: the batch-spanning-many-contacts shape that has frozen this
		// queue before. Partial progress is safe because the projection is
		// idempotent, so a chunk that fails costs only its own sessions — and the
		// loop carries on to the rest rather than abandoning them.
		for start := 0; start < len(ids); start += webNavigationSessionChunkSize {
			end := start + webNavigationSessionChunkSize
			if end > len(ids) {
				end = len(ids)
			}
			if err := r.projectNavigationChunk(ctx, db, date, ids[start:end]); err != nil {
				// Carry on rather than abandon the run. Each chunk commits on its
				// own and the projection is idempotent, so one chunk failing on a
				// deadlock or a lock timeout says nothing about the next — and
				// there is no second chance for the ones skipped: the buffer marks
				// an entry clean before the flush and only a new beat re-dirties
				// it, so a visit whose final beat was in an abandoned chunk is
				// simply never projected.
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

// projectNavigationChunk runs both statements for one partition date and one
// bounded set of sessions, in its own transaction.
func (r *webAnalyticsRepository) projectNavigationChunk(ctx context.Context, db *sql.DB, date time.Time, ids []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin contact navigation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := projectNavigationForDate(ctx, tx, date, ids); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit contact navigation: %w", err)
	}
	return nil
}

// projectNavigationForDate runs both statements against one partition date, so
// session_date can prune. Pages first: the session row is the summary, and a
// reader who catches the transaction mid-flight should not see a visit claiming
// pages that are not there yet.
func projectNavigationForDate(ctx context.Context, tx *sql.Tx, date time.Time, ids []string) error {
	changed := map[string]struct{}{}

	if err := collectChangedEmails(ctx, tx, changed, webNavigationPageProjection,
		date, pq.Array(ids), webNavigationMaxPagesPerSession); err != nil {
		return fmt.Errorf("failed to project web pageviews onto the contact timeline: %w", err)
	}
	if err := collectChangedEmails(ctx, tx, changed, webNavigationSessionProjection,
		date, pq.Array(ids)); err != nil {
		return fmt.Errorf("failed to project web sessions onto the contact timeline: %w", err)
	}
	if len(changed) == 0 {
		return nil
	}

	// Deliberately AFTER the empty check, and reading the rows as they are now —
	// i.e. already rewritten. Any address still attached to one of these sessions
	// that is not in `changed` is a contact the projection just moved rows away
	// from. A quiet re-projection changes nothing and never reaches here, so this
	// costs one indexed lookup on the flushes that did change something.
	previous, err := collectPreviousOwners(ctx, tx, ids)
	if err != nil {
		return fmt.Errorf("failed to read the previous owners of a visit: %w", err)
	}
	for _, email := range previous {
		changed[email] = struct{}{}
	}

	// Deduped, because the same contact reaches here from both projections and a
	// repeated key would abort the statement with "ON CONFLICT DO UPDATE command
	// cannot affect row a second time".
	//
	// Sorted only so the argument is reproducible — in a log, in a test, between
	// two runs of the same flush. It is NOT the lock order: Go compares bytes and
	// the database compares by collation, and those disagree on ordinary
	// addresses, which is why the statement carries its own ORDER BY.
	emails := make([]string, 0, len(changed))
	for email := range changed {
		emails = append(emails, email)
	}
	sort.Strings(emails)

	if _, err := tx.ExecContext(ctx, webNavigationRequeue, pq.Array(emails)); err != nil {
		return fmt.Errorf("failed to queue contacts for segment recomputation: %w", err)
	}
	return nil
}

// collectPreviousOwners lists the addresses currently attached to these
// sessions' summary rows.
func collectPreviousOwners(ctx context.Context, tx *sql.Tx, ids []string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, webNavigationPreviousOwners, pq.Array(ids))
	if err != nil {
		return nil, err
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

// collectChangedEmails runs a projection and records the contacts it touched.
// A row whose projected payload is unchanged declines its DO UPDATE and is not
// returned at all, so this is exactly the set that needs re-evaluating.
func collectChangedEmails(ctx context.Context, tx *sql.Tx, into map[string]struct{}, query string, args ...interface{}) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return err
		}
		into[email] = struct{}{}
	}
	return rows.Err()
}
