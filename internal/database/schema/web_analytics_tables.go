package schema

import (
	"fmt"
	"regexp"
	"time"
)

// Web analytics workspace tables. All three are declaratively partitioned by
// session_date (monthly ranges); the parent DDL below is shared by the
// new-workspace initializer (internal/database/init.go) and the v38 migration
// so the two paths cannot drift. Monthly partitions are created by
// WebAnalyticsPartitionDDL — at workspace init, by the maintenance worker, and
// on demand by the repository when an insert misses its partition.

// Each browser tab is a disjoint writer under one shared session_id, identified
// by tab_id in the child tables' primary keys. Tabs share a session id (it lives
// in localStorage) but keep their own cumulative action list and their own beat
// counter (both live in sessionStorage), so without tab_id in the key two tabs
// both number their pages from 1 and silently overwrite each other's rows —
// arbitrated only by a race between two unrelated counters. A cross-domain
// adoption is the same case: sessionStorage is origin-scoped, so the adopting
// origin naturally mints its own tab_id and becomes another disjoint writer.
//
// tab_id is BIGINT rather than a UUID because it only has to be unique among one
// session's tabs, and web_pages is the highest-volume partitioned table. DEFAULT 0
// is what keeps a payload from an SDK that does not send one behaving exactly as
// it does today.

// WebAnalyticsTableNames lists the partitioned parent tables.
var WebAnalyticsTableNames = []string{"web_sessions", "web_pages", "web_goals"}

// WebAnalyticsPartitionFillfactor leaves headroom for HOT updates on the
// upsert-heavy current month; none of the frequently-updated columns are
// indexed, so updates stay on-page. Cold partitions keep 15% padding, which is
// the accepted cost of not rewriting them.
const WebAnalyticsPartitionFillfactor = 85

// webAnalyticsAttributionColumns is the session-attribution column block shared
// verbatim by web_sessions and web_goals (goals carry a denormalized snapshot
// so goal reports never join the sessions table).
//
// contact_email lives here rather than on web_sessions alone so goal reports can
// answer "which contact converted" without a join. It replaces the opaque
// user_id: identity is now a verified contact address, never a customer-supplied
// string, so there is nothing left for a second column to hold.
const webAnalyticsAttributionColumns = `
	referrer TEXT NOT NULL DEFAULT '',
	referrer_domain TEXT NOT NULL DEFAULT '',
	referrer_path TEXT NOT NULL DEFAULT '',
	is_direct BOOLEAN NOT NULL DEFAULT FALSE,
	landing_page TEXT NOT NULL DEFAULT '',
	landing_domain TEXT NOT NULL DEFAULT '',
	landing_path TEXT NOT NULL DEFAULT '',
	utm_source TEXT NOT NULL DEFAULT '',
	utm_medium TEXT NOT NULL DEFAULT '',
	utm_campaign TEXT NOT NULL DEFAULT '',
	utm_term TEXT NOT NULL DEFAULT '',
	utm_content TEXT NOT NULL DEFAULT '',
	utm_id TEXT NOT NULL DEFAULT '',
	utm_id_from TEXT NOT NULL DEFAULT '',
	channel TEXT NOT NULL DEFAULT '',
	channel_group TEXT NOT NULL DEFAULT '',
	custom_1 TEXT NOT NULL DEFAULT '',
	custom_2 TEXT NOT NULL DEFAULT '',
	custom_3 TEXT NOT NULL DEFAULT '',
	custom_4 TEXT NOT NULL DEFAULT '',
	custom_5 TEXT NOT NULL DEFAULT '',
	custom_6 TEXT NOT NULL DEFAULT '',
	custom_7 TEXT NOT NULL DEFAULT '',
	custom_8 TEXT NOT NULL DEFAULT '',
	custom_9 TEXT NOT NULL DEFAULT '',
	custom_10 TEXT NOT NULL DEFAULT '',
	screen_width SMALLINT NOT NULL DEFAULT 0,
	screen_height SMALLINT NOT NULL DEFAULT 0,
	viewport_width SMALLINT NOT NULL DEFAULT 0,
	viewport_height SMALLINT NOT NULL DEFAULT 0,
	device TEXT NOT NULL DEFAULT '',
	browser TEXT NOT NULL DEFAULT '',
	browser_type TEXT NOT NULL DEFAULT '',
	os TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT '',
	connection_type TEXT NOT NULL DEFAULT '',
	language TEXT NOT NULL DEFAULT '',
	timezone TEXT NOT NULL DEFAULT '',
	country TEXT NOT NULL DEFAULT '',
	region TEXT NOT NULL DEFAULT '',
	city TEXT NOT NULL DEFAULT '',
	latitude REAL,
	longitude REAL,
	contact_email TEXT,`

// WebAnalyticsTableDefinitions returns the DDL creating the three partitioned
// parents and their (cascading) indexes. Idempotent; PostgreSQL 16+ syntax
// only, because AlloyDB Omni is the supported floor.
func WebAnalyticsTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS web_sessions (
	session_date DATE NOT NULL,
	id UUID NOT NULL,
	beat_seq BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	pageview_count SMALLINT NOT NULL DEFAULT 1,
	median_page_duration_ms INTEGER NOT NULL DEFAULT 0,
	max_scroll SMALLINT NOT NULL DEFAULT 0,
	goal_count SMALLINT NOT NULL DEFAULT 0,
	goal_value REAL NOT NULL DEFAULT 0,
	exit_path TEXT NOT NULL DEFAULT '',` +
			webAnalyticsAttributionColumns + `
	sdk_version TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (session_date, id)
) PARTITION BY RANGE (session_date)`,

		`CREATE INDEX IF NOT EXISTS idx_web_sessions_created_at_brin ON web_sessions USING BRIN (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_web_sessions_contact_email ON web_sessions (contact_email) WHERE contact_email IS NOT NULL`,

		`CREATE TABLE IF NOT EXISTS web_pages (
	session_date DATE NOT NULL,
	session_id UUID NOT NULL,
	tab_id BIGINT NOT NULL DEFAULT 0,
	page_number SMALLINT NOT NULL,
	beat_seq BIGINT NOT NULL DEFAULT 0,
	path TEXT NOT NULL DEFAULT '',
	entered_at TIMESTAMPTZ NOT NULL,
	exited_at TIMESTAMPTZ NOT NULL,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	max_scroll SMALLINT NOT NULL DEFAULT 0,
	is_landing BOOLEAN NOT NULL DEFAULT FALSE,
	is_exit BOOLEAN NOT NULL DEFAULT FALSE,
	entry_type TEXT NOT NULL DEFAULT 'navigation',
	contact_email TEXT,
	PRIMARY KEY (session_date, session_id, tab_id, page_number)
) PARTITION BY RANGE (session_date)`,

		`CREATE INDEX IF NOT EXISTS idx_web_pages_entered_at_brin ON web_pages USING BRIN (entered_at)`,
		`CREATE INDEX IF NOT EXISTS idx_web_pages_contact_email ON web_pages (contact_email) WHERE contact_email IS NOT NULL`,

		`CREATE TABLE IF NOT EXISTS web_goals (
	session_date DATE NOT NULL,
	session_id UUID NOT NULL,
	tab_id BIGINT NOT NULL DEFAULT 0,
	goal_name TEXT NOT NULL,
	client_ts_ms BIGINT NOT NULL,
	beat_seq BIGINT NOT NULL DEFAULT 0,
	goal_at TIMESTAMPTZ NOT NULL,
	goal_value REAL NOT NULL DEFAULT 0,
	goal_type TEXT NOT NULL DEFAULT '',
	path TEXT NOT NULL DEFAULT '',
	page_number SMALLINT NOT NULL DEFAULT 1,
	properties JSONB,` +
			webAnalyticsAttributionColumns + `
	PRIMARY KEY (session_date, session_id, tab_id, goal_name, client_ts_ms)
) PARTITION BY RANGE (session_date)`,

		`CREATE INDEX IF NOT EXISTS idx_web_goals_goal_at_brin ON web_goals USING BRIN (goal_at)`,
		`CREATE INDEX IF NOT EXISTS idx_web_goals_contact_email ON web_goals (contact_email) WHERE contact_email IS NOT NULL`,
	}
}

var webAnalyticsPartitionNameRe = regexp.MustCompile(`^(web_sessions|web_pages|web_goals)_y(\d{4})m(\d{2})$`)

// WebAnalyticsPartitionName returns the monthly partition name for a parent
// table, e.g. web_sessions_y2026m08.
func WebAnalyticsPartitionName(table string, month time.Time) string {
	return fmt.Sprintf("%s_y%dm%02d", table, month.Year(), int(month.Month()))
}

// ParseWebAnalyticsPartitionName validates a partition name and returns its
// parent table and the first day of its month. Doubles as the identifier
// whitelist before a name from the catalog is interpolated into DDL.
func ParseWebAnalyticsPartitionName(name string) (table string, month time.Time, ok bool) {
	m := webAnalyticsPartitionNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", time.Time{}, false
	}
	var year, mon int
	_, _ = fmt.Sscanf(m[2], "%d", &year)
	_, _ = fmt.Sscanf(m[3], "%d", &mon)
	if mon < 1 || mon > 12 {
		return "", time.Time{}, false
	}
	return m[1], time.Date(year, time.Month(mon), 1, 0, 0, 0, 0, time.UTC), true
}

// WebAnalyticsPartitionDDL returns the idempotent CREATE statement for one
// monthly partition, with the HOT-friendly fillfactor applied per partition
// (the partitioned parent has no storage, so WITH on the parent would be
// ineffective).
//
// The fillfactor is deliberately mild. A far lower one (50) was considered for
// the heartbeat update pattern and rejected: a partition is updated only while
// its month is current, but it is scanned by dashboards forever, and halving
// page density doubles that scan cost permanently. Reserving a little headroom
// buys most of the HOT benefit without paying for it on every later read.
func WebAnalyticsPartitionDDL(table string, month time.Time) string {
	from := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	return fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s') WITH (fillfactor = %d)`,
		WebAnalyticsPartitionName(table, month), table,
		from.Format("2006-01-02"), to.Format("2006-01-02"),
		WebAnalyticsPartitionFillfactor,
	)
}
