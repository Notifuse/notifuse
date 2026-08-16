package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Rows per INSERT statement. Sessions and goals carry ~60 bound parameters per
// row and lib/pq caps a statement at 65535 parameters; 200 rows stays far
// below that while keeping round trips low.
const webAnalyticsUpsertChunkSize = 200

type webAnalyticsRepository struct {
	workspaceRepo domain.WorkspaceRepository
	logger        logger.Logger
}

// NewWebAnalyticsRepository creates the PostgreSQL web analytics repository.
func NewWebAnalyticsRepository(workspaceRepo domain.WorkspaceRepository, logger logger.Logger) domain.WebAnalyticsRepository {
	return &webAnalyticsRepository{workspaceRepo: workspaceRepo, logger: logger}
}

// webSessionColumns: insert order. The first two are the primary key;
// created_at is set on first insert and never updated afterwards.
var webSessionColumns = []string{
	"session_date", "id", "created_at",
	"beat_seq", "updated_at",
	"duration_ms", "pageview_count", "median_page_duration_ms", "max_scroll", "goal_count", "goal_value",
	"exit_path",
	"referrer", "referrer_domain", "referrer_path", "is_direct",
	"landing_page", "landing_domain", "landing_path",
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id", "utm_id_from",
	"channel", "channel_group",
	"custom_1", "custom_2", "custom_3", "custom_4", "custom_5", "custom_6", "custom_7", "custom_8", "custom_9", "custom_10",
	"screen_width", "screen_height", "viewport_width", "viewport_height",
	"device", "browser", "browser_type", "os", "user_agent", "connection_type",
	"language", "timezone", "country", "region", "city", "latitude", "longitude",
	"contact_email", "sdk_version",
}

var webPageColumns = []string{
	"session_date", "session_id", "tab_id", "page_number",
	"beat_seq", "path", "entered_at", "exited_at", "duration_ms", "max_scroll",
	"is_landing", "is_exit", "entry_type", "contact_email",
}

var webGoalColumns = []string{
	"session_date", "session_id", "tab_id", "goal_name", "client_ts_ms",
	"beat_seq", "goal_at", "goal_value", "goal_type", "path", "page_number", "properties",
	"referrer", "referrer_domain", "referrer_path", "is_direct",
	"landing_page", "landing_domain", "landing_path",
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id", "utm_id_from",
	"channel", "channel_group",
	"custom_1", "custom_2", "custom_3", "custom_4", "custom_5", "custom_6", "custom_7", "custom_8", "custom_9", "custom_10",
	"screen_width", "screen_height", "viewport_width", "viewport_height",
	"device", "browser", "browser_type", "os", "user_agent", "connection_type",
	"language", "timezone", "country", "region", "city", "latitude", "longitude",
	"contact_email",
}

// webSessionStickyColumns are the attribution facts that describe the SESSION,
// not whichever tab happened to beat last.
//
// A second tab opened from a link inside the site carries its own landing page
// and a referrer pointing at the first tab's page. Letting it overwrite the
// session row would silently rewrite the visit's acquisition source, so the
// first writer that supplies a non-empty value keeps it. Only TEXT columns
// belong here; the rest of the attribution block (device, browser, geo) is
// identical across a session's tabs, and screen/viewport genuinely differ per
// window, where last-writer-wins is fine.
var webSessionStickyColumns = func() map[string]bool {
	cols := []string{
		"referrer", "referrer_domain", "referrer_path",
		"landing_page", "landing_domain", "landing_path",
		"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id", "utm_id_from",
		"channel", "channel_group",
	}
	for i := 1; i <= 10; i++ {
		cols = append(cols, fmt.Sprintf("custom_%d", i))
	}
	set := make(map[string]bool, len(cols))
	for _, c := range cols {
		set[c] = true
	}
	return set
}()

// upsertAssignment picks the merge rule for one column.
//
// web_sessions is the interesting case: it is a single row written by every tab
// of the session, so no rule may depend on arrival order. Aggregates are simply
// taken from EXCLUDED here and then recomputed from the child rows by the
// rollup that runs after the page insert; timestamps take the later value;
// attribution sticks to the first writer.
func upsertAssignment(table, c string) string {
	if c == "contact_email" {
		// Server-managed linkage: sticky once set, beats never clear it.
		return "contact_email = COALESCE(EXCLUDED.contact_email, " + table + ".contact_email)"
	}
	if table == "web_sessions" {
		switch {
		case c == "updated_at" || c == "beat_seq":
			return fmt.Sprintf("%s = GREATEST(EXCLUDED.%s, %s.%s)", c, c, table, c)
		case webSessionStickyColumns[c]:
			return fmt.Sprintf("%s = COALESCE(NULLIF(%s.%s, ''), EXCLUDED.%s)", c, table, c, c)
		}
	}
	return c + " = EXCLUDED." + c
}

// upsertSuffix builds the ON CONFLICT clause. skip lists columns that must never
// be updated after first insert.
//
// The beat_seq guard applies to the child tables only. There, tab_id is part of
// the conflict target, so the comparison is between beats of the SAME writer and
// genuinely means "ignore a stale replay". On web_sessions there is no single
// writer, and a guard would let the tab with the highest counter block every
// other tab's beats for the life of the session.
func upsertSuffix(table string, columns, conflictCols, skip []string) string {
	skipSet := make(map[string]bool, len(conflictCols)+len(skip))
	for _, c := range conflictCols {
		skipSet[c] = true
	}
	for _, c := range skip {
		skipSet[c] = true
	}
	assignments := make([]string, 0, len(columns))
	for _, c := range columns {
		if skipSet[c] {
			continue
		}
		assignments = append(assignments, upsertAssignment(table, c))
	}
	guard := ""
	if table != "web_sessions" {
		guard = fmt.Sprintf(" WHERE EXCLUDED.beat_seq > %s.beat_seq", table)
	}
	return fmt.Sprintf("ON CONFLICT (%s) DO UPDATE SET %s%s",
		strings.Join(conflictCols, ", "), strings.Join(assignments, ", "), guard)
}

// Upsert suffixes are built once and shared with the tests, so a change at
// the call site (for example dropping created_at from the skip list) cannot
// pass a test that hand-builds its own copy.
var (
	webSessionUpsertSuffix = upsertSuffix("web_sessions", webSessionColumns,
		[]string{"session_date", "id"}, []string{"created_at"})
	webPageUpsertSuffix = upsertSuffix("web_pages", webPageColumns,
		[]string{"session_date", "session_id", "tab_id", "page_number"}, nil)
	webGoalUpsertSuffix = upsertSuffix("web_goals", webGoalColumns,
		[]string{"session_date", "session_id", "tab_id", "goal_name", "client_ts_ms"}, nil)
)

func clampSmallint(v int) int {
	if v < 0 {
		return 0
	}
	if v > 32767 {
		return 32767
	}
	return v
}

// clampInt32 bounds a value to the INTEGER columns (duration_ms and friends).
// Without it a single hostile or buggy beat carrying a huge duration aborts
// the whole workspace transaction with a numeric-overflow error, taking every
// other visitor batched alongside it down too.
func clampInt32(v int64) int64 {
	if v < 0 {
		return 0
	}
	if v > 2147483647 {
		return 2147483647
	}
	return v
}

// dedupeByKey keeps the last row per primary key. A single INSERT ... ON
// CONFLICT DO UPDATE cannot touch the same row twice ("command cannot affect
// row a second time"), so two actions sharing a key — two goals fired in the
// same millisecond, a repeated page_number — would abort the entire batch.
func dedupeByKey[T any](rows []T, key func(T) string) []T {
	if len(rows) < 2 {
		return rows
	}
	positions := make(map[string]int, len(rows))
	out := make([]T, 0, len(rows))
	for _, row := range rows {
		k := key(row)
		if i, seen := positions[k]; seen {
			out[i] = row // later action wins
			continue
		}
		positions[k] = len(out)
		out = append(out, row)
	}
	return out
}

// webPageDedupeKey and webGoalDedupeKey mirror the primary keys exactly. They
// must include tab_id, or two tabs' page 1 collapse into one row inside a single
// batch — before the database ever sees them.
func webPageDedupeKey(p *domain.WebPage) string {
	return fmt.Sprintf("%s|%s|%d|%d", p.SessionDate.Format("2006-01-02"), p.SessionID, p.TabID, p.PageNumber)
}

func webGoalDedupeKey(g *domain.WebGoal) string {
	return fmt.Sprintf("%s|%s|%d|%s|%d", g.SessionDate.Format("2006-01-02"), g.SessionID, g.TabID, g.GoalName, g.ClientTsMs)
}

// Rollup statements. Every tab of a session writes the same web_sessions row, so
// no aggregate can be trusted from a single payload — whichever tab beat last
// would otherwise decide the session's pageview count and duration. Recomputing
// from the child rows makes those columns order-free, and that is precisely what
// allows the session upsert to drop its beat_seq guard.
//
// They run as their own statements after the child inserts, deliberately. A
// subquery inside the session INSERT would read web_pages before this beat's
// pages existed; and even reordered, READ COMMITTED lets an updating command see
// the new version of the row it blocked on but not other rows, so a concurrent
// tab's pages could still be invisible. A new statement takes a fresh snapshot.
const webSessionPageRollup = `
UPDATE web_sessions s SET
	pageview_count = LEAST(agg.pageviews, 32767)::smallint,
	duration_ms = LEAST(agg.total_duration, 2147483647)::int,
	max_scroll = agg.max_scroll,
	median_page_duration_ms = LEAST(agg.median_duration, 2147483647)::int,
	exit_path = agg.exit_path
FROM (
	SELECT session_id,
		COUNT(*) AS pageviews,
		COALESCE(SUM(duration_ms), 0) AS total_duration,
		COALESCE(MAX(max_scroll), 0) AS max_scroll,
		COALESCE(ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms)), 0) AS median_duration,
		(ARRAY_AGG(path ORDER BY exited_at DESC, page_number DESC))[1] AS exit_path
	FROM web_pages
	WHERE session_date = $1 AND session_id = ANY($2)
	GROUP BY session_id
) agg
WHERE s.session_date = $1 AND s.id = agg.session_id`

const webSessionGoalRollup = `
UPDATE web_sessions s SET
	goal_count = LEAST(agg.goals, 32767)::smallint,
	goal_value = agg.total_value
FROM (
	SELECT session_id, COUNT(*) AS goals, COALESCE(SUM(goal_value), 0) AS total_value
	FROM web_goals
	WHERE session_date = $1 AND session_id = ANY($2)
	GROUP BY session_id
) agg
WHERE s.session_date = $1 AND s.id = agg.session_id`

// webPageEntryExitRollup recomputes is_landing/is_exit across ALL of a session's
// tabs. Both are written per-payload from a tab's own ordinals, so a visitor with
// three tabs would otherwise register three entries and three exits, inflating
// the Entries, Exits and Exit Rate measures for exactly the multi-tab users this
// work exists to serve. Neither column is indexed, so these updates stay HOT.
//
// It picks exactly ONE row for each flag rather than comparing every row against
// MIN/MAX: pages within a beat routinely share an exit timestamp (only the
// current page's is refreshed on each build), so a timestamp equality test would
// mark several rows as the exit and inflate the Exits and Exit Rate measures.
// page_number breaks the tie, which is the right answer within a tab; tab_id
// breaks it across tabs, arbitrarily but deterministically.
const webPageEntryExitRollup = `
UPDATE web_pages p SET
	is_landing = (p.tab_id = agg.first_tab AND p.page_number = agg.first_page),
	is_exit = (p.tab_id = agg.last_tab AND p.page_number = agg.last_page)
FROM (
	SELECT session_id,
		(ARRAY_AGG(tab_id ORDER BY entered_at ASC, tab_id ASC, page_number ASC))[1] AS first_tab,
		(ARRAY_AGG(page_number ORDER BY entered_at ASC, tab_id ASC, page_number ASC))[1] AS first_page,
		(ARRAY_AGG(tab_id ORDER BY exited_at DESC, tab_id DESC, page_number DESC))[1] AS last_tab,
		(ARRAY_AGG(page_number ORDER BY exited_at DESC, tab_id DESC, page_number DESC))[1] AS last_page
	FROM web_pages
	WHERE session_date = $1 AND session_id = ANY($2)
	GROUP BY session_id
) agg
WHERE p.session_date = $1 AND p.session_id = agg.session_id`

// rollupSessions re-derives the session row and the entry/exit flags for every
// session touched by this batch, one statement per partition date so the
// planner can still prune.
func rollupSessions(ctx context.Context, tx *sql.Tx, sessions []*domain.WebSession) error {
	byDate := make(map[time.Time][]string, 4)
	for _, s := range sessions {
		byDate[s.SessionDate] = append(byDate[s.SessionDate], s.ID)
	}
	for date, ids := range byDate {
		for _, stmt := range []string{webSessionPageRollup, webSessionGoalRollup, webPageEntryExitRollup} {
			if _, err := tx.ExecContext(ctx, stmt, date, pq.Array(ids)); err != nil {
				return fmt.Errorf("failed to roll up web analytics session aggregates: %w", err)
			}
		}
	}
	return nil
}

func webSessionValues(s *domain.WebSession) []interface{} {
	return []interface{}{
		s.SessionDate, s.ID, s.CreatedAt,
		s.BeatSeq, s.UpdatedAt,
		clampInt32(s.DurationMs), clampSmallint(s.PageviewCount), clampInt32(s.MedianPageDurationMs), clampSmallint(s.MaxScroll), clampSmallint(s.GoalCount), s.GoalValue,
		s.ExitPath,
		s.Referrer, s.ReferrerDomain, s.ReferrerPath, s.IsDirect,
		s.LandingPage, s.LandingDomain, s.LandingPath,
		s.UTMSource, s.UTMMedium, s.UTMCampaign, s.UTMTerm, s.UTMContent, s.UTMID, s.UTMIDFrom,
		s.Channel, s.ChannelGroup,
		s.Custom1, s.Custom2, s.Custom3, s.Custom4, s.Custom5, s.Custom6, s.Custom7, s.Custom8, s.Custom9, s.Custom10,
		clampSmallint(s.ScreenWidth), clampSmallint(s.ScreenHeight), clampSmallint(s.ViewportWidth), clampSmallint(s.ViewportHeight),
		s.Device, s.Browser, s.BrowserType, s.OS, s.UserAgent, s.ConnectionType,
		s.Language, s.Timezone, s.Country, s.Region, s.City, s.Latitude, s.Longitude,
		s.ContactEmail, s.SDKVersion,
	}
}

func webPageValues(p *domain.WebPage) []interface{} {
	return []interface{}{
		p.SessionDate, p.SessionID, p.TabID, clampSmallint(p.PageNumber),
		p.BeatSeq, p.Path, p.EnteredAt, p.ExitedAt, clampInt32(p.DurationMs), clampSmallint(p.MaxScroll),
		p.IsLanding, p.IsExit, p.EntryType, p.ContactEmail,
	}
}

func webGoalValues(g *domain.WebGoal) ([]interface{}, error) {
	var properties interface{}
	if len(g.Properties) > 0 {
		raw, err := json.Marshal(g.Properties)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal goal properties: %w", err)
		}
		properties = raw
	}
	return []interface{}{
		g.SessionDate, g.SessionID, g.TabID, g.GoalName, g.ClientTsMs,
		g.BeatSeq, g.GoalAt, g.GoalValue, g.GoalType, g.Path, clampSmallint(g.PageNumber), properties,
		g.Referrer, g.ReferrerDomain, g.ReferrerPath, g.IsDirect,
		g.LandingPage, g.LandingDomain, g.LandingPath,
		g.UTMSource, g.UTMMedium, g.UTMCampaign, g.UTMTerm, g.UTMContent, g.UTMID, g.UTMIDFrom,
		g.Channel, g.ChannelGroup,
		g.Custom1, g.Custom2, g.Custom3, g.Custom4, g.Custom5, g.Custom6, g.Custom7, g.Custom8, g.Custom9, g.Custom10,
		clampSmallint(g.ScreenWidth), clampSmallint(g.ScreenHeight), clampSmallint(g.ViewportWidth), clampSmallint(g.ViewportHeight),
		g.Device, g.Browser, g.BrowserType, g.OS, g.UserAgent, g.ConnectionType,
		g.Language, g.Timezone, g.Country, g.Region, g.City, g.Latitude, g.Longitude,
		g.ContactEmail,
	}, nil
}

// FlushBatch upserts the rows in one transaction. Rows are sorted by primary
// key first so two replicas flushing overlapping sessions lock rows in the
// same order and cannot deadlock. A flush that hits a missing monthly
// partition creates the needed partitions and retries once.
// AnonymizeContact clears contact_email for one address across the three web
// analytics tables.
//
// Without it, deleting a contact leaves their email stamped on every session,
// page and goal they ever produced — and the sticky COALESCE in the upsert means
// no later beat can ever clear it. The ingest-side contact gate stops NEW beats
// re-stamping a deleted contact; this erases what was already written.
//
// Partition pruning cannot help here (the address is not the partition key), so
// this is a full scan of the workspace's web tables. It runs once per contact
// deletion, which is rare, and the partial contact_email indexes keep it to the
// identified rows only.
func (r *webAnalyticsRepository) AnonymizeContact(ctx context.Context, workspaceID string, email string) error {
	if strings.TrimSpace(email) == "" {
		return nil
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	for _, table := range schema.WebAnalyticsTableNames {
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET contact_email = NULL WHERE contact_email = $1`, table), email,
		); err != nil {
			return fmt.Errorf("failed to anonymize %s: %w", table, err)
		}
	}
	return nil
}

func (r *webAnalyticsRepository) FlushBatch(ctx context.Context, workspaceID string, sessions []*domain.WebSession, pages []*domain.WebPage, goals []*domain.WebGoal) error {
	if len(sessions) == 0 && len(pages) == 0 && len(goals) == 0 {
		return nil
	}

	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].SessionDate.Equal(sessions[j].SessionDate) {
			return sessions[i].SessionDate.Before(sessions[j].SessionDate)
		}
		return sessions[i].ID < sessions[j].ID
	})
	sort.Slice(pages, func(i, j int) bool {
		if !pages[i].SessionDate.Equal(pages[j].SessionDate) {
			return pages[i].SessionDate.Before(pages[j].SessionDate)
		}
		if pages[i].SessionID != pages[j].SessionID {
			return pages[i].SessionID < pages[j].SessionID
		}
		if pages[i].TabID != pages[j].TabID {
			return pages[i].TabID < pages[j].TabID
		}
		return pages[i].PageNumber < pages[j].PageNumber
	})
	sort.Slice(goals, func(i, j int) bool {
		if !goals[i].SessionDate.Equal(goals[j].SessionDate) {
			return goals[i].SessionDate.Before(goals[j].SessionDate)
		}
		if goals[i].SessionID != goals[j].SessionID {
			return goals[i].SessionID < goals[j].SessionID
		}
		if goals[i].TabID != goals[j].TabID {
			return goals[i].TabID < goals[j].TabID
		}
		if goals[i].GoalName != goals[j].GoalName {
			return goals[i].GoalName < goals[j].GoalName
		}
		return goals[i].ClientTsMs < goals[j].ClientTsMs
	})

	// Two tabs of one session each contribute a session row to the batch, and a
	// single INSERT ... ON CONFLICT cannot affect the same row twice. Collapsing
	// them is safe because the aggregates are recomputed from the child tables
	// by the rollup, and the child rows carry every tab.
	sessions = dedupeByKey(sessions, func(s *domain.WebSession) string {
		return s.SessionDate.Format("2006-01-02") + "|" + s.ID
	})
	pages = dedupeByKey(pages, webPageDedupeKey)
	goals = dedupeByKey(goals, webGoalDedupeKey)

	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	err = r.flushOnce(ctx, db, sessions, pages, goals)
	if isMissingPartitionError(err) {
		months := collectMonths(sessions, pages, goals)
		if ensureErr := r.EnsureMonthlyPartitions(ctx, workspaceID, months); ensureErr != nil {
			return fmt.Errorf("failed to create missing partitions: %w (after %v)", ensureErr, err)
		}
		err = r.flushOnce(ctx, db, sessions, pages, goals)
	}
	return err
}

func (r *webAnalyticsRepository) flushOnce(ctx context.Context, db *sql.DB, sessions []*domain.WebSession, pages []*domain.WebPage, goals []*domain.WebGoal) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for chunk := 0; chunk < len(sessions); chunk += webAnalyticsUpsertChunkSize {
		end := min(chunk+webAnalyticsUpsertChunkSize, len(sessions))
		builder := sq.Insert("web_sessions").Columns(webSessionColumns...).
			PlaceholderFormat(sq.Dollar).Suffix(webSessionUpsertSuffix)
		for _, s := range sessions[chunk:end] {
			builder = builder.Values(webSessionValues(s)...)
		}
		if err := execBuilder(ctx, tx, builder, "web_sessions"); err != nil {
			return err
		}
	}

	for chunk := 0; chunk < len(pages); chunk += webAnalyticsUpsertChunkSize {
		end := min(chunk+webAnalyticsUpsertChunkSize, len(pages))
		builder := sq.Insert("web_pages").Columns(webPageColumns...).
			PlaceholderFormat(sq.Dollar).Suffix(webPageUpsertSuffix)
		for _, p := range pages[chunk:end] {
			builder = builder.Values(webPageValues(p)...)
		}
		if err := execBuilder(ctx, tx, builder, "web_pages"); err != nil {
			return err
		}
	}

	for chunk := 0; chunk < len(goals); chunk += webAnalyticsUpsertChunkSize {
		end := min(chunk+webAnalyticsUpsertChunkSize, len(goals))
		builder := sq.Insert("web_goals").Columns(webGoalColumns...).
			PlaceholderFormat(sq.Dollar).Suffix(webGoalUpsertSuffix)
		for _, g := range goals[chunk:end] {
			values, err := webGoalValues(g)
			if err != nil {
				return err
			}
			builder = builder.Values(values...)
		}
		if err := execBuilder(ctx, tx, builder, "web_goals"); err != nil {
			return err
		}
	}

	if len(sessions) > 0 {
		if err := rollupSessions(ctx, tx, sessions); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit web analytics flush: %w", err)
	}
	return nil
}

func execBuilder(ctx context.Context, tx *sql.Tx, builder sq.InsertBuilder, table string) error {
	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build %s upsert: %w", table, err)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to upsert %s: %w", table, err)
	}
	return nil
}

// isMissingPartitionError detects an insert that found no partition for its
// session_date. SQLSTATE 23514 is the generic check_violation, so the message
// is matched too.
func isMissingPartitionError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "23514" && strings.Contains(pqErr.Message, "no partition of relation")
}

func collectMonths(sessions []*domain.WebSession, pages []*domain.WebPage, goals []*domain.WebGoal) []time.Time {
	seen := map[string]time.Time{}
	add := func(d time.Time) {
		m := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
		seen[m.Format("2006-01")] = m
	}
	for _, s := range sessions {
		add(s.SessionDate)
	}
	for _, p := range pages {
		add(p.SessionDate)
	}
	for _, g := range goals {
		add(g.SessionDate)
	}
	months := make([]time.Time, 0, len(seen))
	for _, m := range seen {
		months = append(months, m)
	}
	sort.Slice(months, func(i, j int) bool { return months[i].Before(months[j]) })
	return months
}

// EnsureMonthlyPartitions creates the monthly partitions of every web
// analytics table for the given months (idempotent). Current and future
// months also get the aggressive autovacuum profile — the maintenance worker
// resets it once the month rolls over and the partition goes cold.
func (r *webAnalyticsRepository) EnsureMonthlyPartitions(ctx context.Context, workspaceID string, months []time.Time) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	currentMonth := time.Now().UTC().Format("2006-01")
	for _, month := range months {
		for _, table := range schema.WebAnalyticsTableNames {
			if _, err := db.ExecContext(ctx, schema.WebAnalyticsPartitionDDL(table, month)); err != nil {
				return fmt.Errorf("failed to create partition of %s for %s: %w", table, month.Format("2006-01"), err)
			}
			if month.Format("2006-01") >= currentMonth {
				partition := schema.WebAnalyticsPartitionName(table, month)
				if err := r.SetPartitionAutovacuum(ctx, workspaceID, partition, true); err != nil {
					r.logger.WithField("workspace_id", workspaceID).WithField("partition", partition).
						WithField("error", err.Error()).Error("Failed to apply autovacuum settings to new partition")
				}
			}
		}
	}
	return nil
}

// ListPartitions returns the partition names of a web analytics parent table.
func (r *webAnalyticsRepository) ListPartitions(ctx context.Context, workspaceID string, table string) ([]string, error) {
	if !isWebAnalyticsTable(table) {
		return nil, fmt.Errorf("unknown web analytics table: %s", table)
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	return r.listPartitions(ctx, db, table)
}

func (r *webAnalyticsRepository) listPartitions(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = $1
		ORDER BY c.relname`, table)
	if err != nil {
		return nil, fmt.Errorf("failed to list partitions of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// AnalyzePartitions runs ANALYZE on the given partitions (names validated
// against the partition naming scheme before being interpolated).
func (r *webAnalyticsRepository) AnalyzePartitions(ctx context.Context, workspaceID string, partitions []string) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	for _, name := range partitions {
		if _, _, ok := schema.ParseWebAnalyticsPartitionName(name); !ok {
			return fmt.Errorf("invalid partition name: %s", name)
		}
		if _, err := db.ExecContext(ctx, "ANALYZE "+pq.QuoteIdentifier(name)); err != nil {
			return fmt.Errorf("failed to analyze %s: %w", name, err)
		}
	}
	return nil
}

// SetPartitionAutovacuum applies (aggressive) or resets the autovacuum storage
// parameters of one partition. The aggressive profile keeps up with the
// upsert-per-beat churn of the current month.
func (r *webAnalyticsRepository) SetPartitionAutovacuum(ctx context.Context, workspaceID string, partition string, aggressive bool) error {
	if _, _, ok := schema.ParseWebAnalyticsPartitionName(partition); !ok {
		return fmt.Errorf("invalid partition name: %s", partition)
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	var query string
	if aggressive {
		query = fmt.Sprintf(`ALTER TABLE %s SET (
			autovacuum_vacuum_scale_factor = 0.05,
			autovacuum_vacuum_insert_scale_factor = 0.05,
			autovacuum_vacuum_cost_delay = 2,
			autovacuum_vacuum_cost_limit = 1000
		)`, pq.QuoteIdentifier(partition))
	} else {
		query = fmt.Sprintf(`ALTER TABLE %s RESET (
			autovacuum_vacuum_scale_factor,
			autovacuum_vacuum_insert_scale_factor,
			autovacuum_vacuum_cost_delay,
			autovacuum_vacuum_cost_limit
		)`, pq.QuoteIdentifier(partition))
	}
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to alter autovacuum settings of %s: %w", partition, err)
	}
	return nil
}

// Usage metering. The counts are recomputed from the data rather than
// incremented as it lands, which is what keeps the flush path unchanged: a
// running total would have to tell an INSERT apart from the fortieth heartbeat
// UPDATE of the same page, and `RETURNING (xmax = 0)` is unavailable on a
// partitioned parent while MERGE gives up ON CONFLICT's speculative-insertion
// protection and raises 23505 when two flushes race the same new key.
//
// Recounting also costs nothing to get right: COUNT(*) over web_pages *is* the
// billable definition, so the meter cannot drift from it.
const (
	// Bounded to the requested month so range partitioning prunes to the single
	// web_pages partition. Both bounds are cast to date: session_date is a DATE
	// column, and comparing it against a bound lib/pq sends as a timestamptz
	// would resolve the date through the session's TimeZone.
	monthlyUsagePageviewCount = `
		SELECT COUNT(*) FROM web_pages
		WHERE session_date >= $1::date AND session_date < $2::date`

	// The exclusion is on entity_type, matching the literals
	// web_analytics_timeline_projection.go writes and the predicate of
	// idx_contact_timeline_billable. Without it every pageview would be metered
	// twice, once as a pageview and once as an event.
	monthlyUsageTimelineCount = `
		SELECT COUNT(*) FROM contact_timeline
		WHERE created_at >= $1 AND created_at < $2
		AND entity_type NOT IN ('web_page', 'web_session')`

	// The open month, where a lower count is legitimate: deleting a contact
	// removes its timeline rows.
	monthlyUsageUpsertLive = `
		INSERT INTO monthly_usage (period_month, pageviews, timeline_entries, computed_at, updated_at)
		VALUES ($1::date, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (period_month) DO UPDATE SET
			pageviews = EXCLUDED.pageviews,
			timeline_entries = EXCLUDED.timeline_entries,
			computed_at = EXCLUDED.computed_at,
			updated_at = EXCLUDED.updated_at`

	// A closed month. GREATEST is the whole retention guard: once history is
	// dropped the counts come back as 0 and the stored, already-reported value
	// has to win. It subsumes checking pg_inherits for the partition, and also
	// covers partial loss, which that check would not.
	monthlyUsageUpsertClosed = `
		INSERT INTO monthly_usage (period_month, pageviews, timeline_entries, computed_at, updated_at)
		VALUES ($1::date, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (period_month) DO UPDATE SET
			pageviews = GREATEST(monthly_usage.pageviews, EXCLUDED.pageviews),
			timeline_entries = GREATEST(monthly_usage.timeline_entries, EXCLUDED.timeline_entries),
			computed_at = EXCLUDED.computed_at,
			updated_at = EXCLUDED.updated_at`

	// period_month is read back through to_char rather than scanned as a date:
	// lib/pq resolves a DATE into a time.Time at the connection's location, and
	// a non-UTC location would shift the first of the month into the previous
	// one. DateStyle cannot affect to_char.
	monthlyUsageSelect = `
		SELECT to_char(period_month, 'YYYY-MM-DD'), pageviews, timeline_entries, computed_at
		FROM monthly_usage
		WHERE period_month = ANY($1::date[])
		ORDER BY period_month`
)

// utcMonthStart returns the first instant of the UTC month containing t.
func utcMonthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// RecomputeUsage recounts one UTC month of metered usage and stores the
// snapshot. See the domain interface for the meaning of live.
func (r *webAnalyticsRepository) RecomputeUsage(ctx context.Context, workspaceID string, month time.Time, live bool) error {
	start := utcMonthStart(month)
	end := start.AddDate(0, 1, 0)
	label := start.Format("2006-01")

	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	var pageviews int64
	if err := db.QueryRowContext(ctx, monthlyUsagePageviewCount, start, end).Scan(&pageviews); err != nil {
		return fmt.Errorf("failed to count pageviews for %s: %w", label, err)
	}

	var timelineEntries int64
	if err := db.QueryRowContext(ctx, monthlyUsageTimelineCount, start, end).Scan(&timelineEntries); err != nil {
		return fmt.Errorf("failed to count timeline entries for %s: %w", label, err)
	}

	stmt := monthlyUsageUpsertClosed
	if live {
		stmt = monthlyUsageUpsertLive
	}
	if _, err := db.ExecContext(ctx, stmt, start, pageviews, timelineEntries); err != nil {
		return fmt.Errorf("failed to store usage snapshot for %s: %w", label, err)
	}
	return nil
}

// GetUsage returns the stored snapshots for the given UTC months.
func (r *webAnalyticsRepository) GetUsage(ctx context.Context, workspaceID string, months []time.Time) ([]*domain.MonthlyUsage, error) {
	if len(months) == 0 {
		return nil, nil
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	starts := make([]string, 0, len(months))
	for _, m := range months {
		starts = append(starts, utcMonthStart(m).Format("2006-01-02"))
	}

	rows, err := db.QueryContext(ctx, monthlyUsageSelect, pq.Array(starts))
	if err != nil {
		return nil, fmt.Errorf("failed to query usage snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var usage []*domain.MonthlyUsage
	for rows.Next() {
		var periodMonth string
		u := &domain.MonthlyUsage{}
		if err := rows.Scan(&periodMonth, &u.Pageviews, &u.TimelineEntries, &u.ComputedAt); err != nil {
			return nil, fmt.Errorf("failed to scan usage snapshot: %w", err)
		}
		parsed, err := time.Parse("2006-01-02", periodMonth)
		if err != nil {
			return nil, fmt.Errorf("failed to parse usage period month %q: %w", periodMonth, err)
		}
		u.PeriodMonth = parsed.UTC()
		u.ComputedAt = u.ComputedAt.UTC()
		usage = append(usage, u)
	}
	return usage, rows.Err()
}

func isWebAnalyticsTable(table string) bool {
	for _, t := range schema.WebAnalyticsTableNames {
		if t == table {
			return true
		}
	}
	return false
}
