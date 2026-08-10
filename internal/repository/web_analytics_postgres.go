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
	"user_id", "sdk_version", "contact_email",
}

var webPageColumns = []string{
	"session_date", "session_id", "page_number",
	"beat_seq", "path", "entered_at", "exited_at", "duration_ms", "max_scroll",
	"is_landing", "is_exit", "entry_type", "user_id",
}

var webGoalColumns = []string{
	"session_date", "session_id", "goal_name", "client_ts_ms",
	"beat_seq", "goal_at", "goal_value", "path", "page_number", "properties",
	"referrer", "referrer_domain", "referrer_path", "is_direct",
	"landing_page", "landing_domain", "landing_path",
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id", "utm_id_from",
	"channel", "channel_group",
	"custom_1", "custom_2", "custom_3", "custom_4", "custom_5", "custom_6", "custom_7", "custom_8", "custom_9", "custom_10",
	"screen_width", "screen_height", "viewport_width", "viewport_height",
	"device", "browser", "browser_type", "os", "user_agent", "connection_type",
	"language", "timezone", "country", "region", "city", "latitude", "longitude",
	"user_id",
}

// upsertSuffix builds the ON CONFLICT clause: overwrite every non-key column
// from EXCLUDED, guarded so only a strictly newer beat wins. skip lists
// columns that must never be updated after first insert.
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
		if c == "contact_email" {
			// Server-managed linkage: sticky once set, beats never clear it.
			assignments = append(assignments, "contact_email = COALESCE(EXCLUDED.contact_email, "+table+".contact_email)")
			continue
		}
		assignments = append(assignments, c+" = EXCLUDED."+c)
	}
	return fmt.Sprintf("ON CONFLICT (%s) DO UPDATE SET %s WHERE EXCLUDED.beat_seq > %s.beat_seq",
		strings.Join(conflictCols, ", "), strings.Join(assignments, ", "), table)
}

// Upsert suffixes are built once and shared with the tests, so a change at
// the call site (for example dropping created_at from the skip list) cannot
// pass a test that hand-builds its own copy.
var (
	webSessionUpsertSuffix = upsertSuffix("web_sessions", webSessionColumns,
		[]string{"session_date", "id"}, []string{"created_at"})
	webPageUpsertSuffix = upsertSuffix("web_pages", webPageColumns,
		[]string{"session_date", "session_id", "page_number"}, nil)
	webGoalUpsertSuffix = upsertSuffix("web_goals", webGoalColumns,
		[]string{"session_date", "session_id", "goal_name", "client_ts_ms"}, nil)
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
		s.UserID, s.SDKVersion, s.ContactEmail,
	}
}

func webPageValues(p *domain.WebPage) []interface{} {
	return []interface{}{
		p.SessionDate, p.SessionID, clampSmallint(p.PageNumber),
		p.BeatSeq, p.Path, p.EnteredAt, p.ExitedAt, clampInt32(p.DurationMs), clampSmallint(p.MaxScroll),
		p.IsLanding, p.IsExit, p.EntryType, p.UserID,
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
		g.SessionDate, g.SessionID, g.GoalName, g.ClientTsMs,
		g.BeatSeq, g.GoalAt, g.GoalValue, g.Path, clampSmallint(g.PageNumber), properties,
		g.Referrer, g.ReferrerDomain, g.ReferrerPath, g.IsDirect,
		g.LandingPage, g.LandingDomain, g.LandingPath,
		g.UTMSource, g.UTMMedium, g.UTMCampaign, g.UTMTerm, g.UTMContent, g.UTMID, g.UTMIDFrom,
		g.Channel, g.ChannelGroup,
		g.Custom1, g.Custom2, g.Custom3, g.Custom4, g.Custom5, g.Custom6, g.Custom7, g.Custom8, g.Custom9, g.Custom10,
		clampSmallint(g.ScreenWidth), clampSmallint(g.ScreenHeight), clampSmallint(g.ViewportWidth), clampSmallint(g.ViewportHeight),
		g.Device, g.Browser, g.BrowserType, g.OS, g.UserAgent, g.ConnectionType,
		g.Language, g.Timezone, g.Country, g.Region, g.City, g.Latitude, g.Longitude,
		g.UserID,
	}, nil
}

// FlushBatch upserts the rows in one transaction. Rows are sorted by primary
// key first so two replicas flushing overlapping sessions lock rows in the
// same order and cannot deadlock. A flush that hits a missing monthly
// partition creates the needed partitions and retries once.
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
		return pages[i].PageNumber < pages[j].PageNumber
	})
	sort.Slice(goals, func(i, j int) bool {
		if !goals[i].SessionDate.Equal(goals[j].SessionDate) {
			return goals[i].SessionDate.Before(goals[j].SessionDate)
		}
		if goals[i].SessionID != goals[j].SessionID {
			return goals[i].SessionID < goals[j].SessionID
		}
		if goals[i].GoalName != goals[j].GoalName {
			return goals[i].GoalName < goals[j].GoalName
		}
		return goals[i].ClientTsMs < goals[j].ClientTsMs
	})

	pages = dedupeByKey(pages, func(p *domain.WebPage) string {
		return fmt.Sprintf("%s|%s|%d", p.SessionDate.Format("2006-01-02"), p.SessionID, p.PageNumber)
	})
	goals = dedupeByKey(goals, func(g *domain.WebGoal) string {
		return fmt.Sprintf("%s|%s|%s|%d", g.SessionDate.Format("2006-01-02"), g.SessionID, g.GoalName, g.ClientTsMs)
	})

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

func isWebAnalyticsTable(table string) bool {
	for _, t := range schema.WebAnalyticsTableNames {
		if t == table {
			return true
		}
	}
	return false
}
