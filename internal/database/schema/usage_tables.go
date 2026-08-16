package schema

// Monthly usage metering: one row per UTC month holding the counts a plan quota
// is measured against — pageviews and billable timeline entries.
//
// The row is a recomputed snapshot, not an incremental ledger. The maintenance
// worker recounts the open month and the one before it and stores the result,
// which keeps the ingest path completely untouched: nothing on the hot flush
// transaction has to decide whether a web_pages row was inserted or updated by
// this beat, and there is no counter row to lock while a flush holds a
// transaction open. It also makes the metered number the same COUNT(*) over
// web_pages that the pricing page describes, rather than a parallel tally that
// can drift away from it.
//
// Deliberately not partitioned and deliberately absent from
// WebAnalyticsTableNames, which drives monthly partition creation in the
// maintenance worker: this table gains twelve rows a year.

// UsageTableDefinitions returns the DDL for the monthly usage snapshot table and
// the index the timeline meter counts through. Shared verbatim by
// internal/database/init.go and the v38 migration, so a fresh install and an
// upgraded one cannot drift.
func UsageTableDefinitions() []string {
	return []string{
		// period_month is the first day of the UTC month. Counts are BIGINT
		// because web_sessions.pageview_count already showed what a narrow type
		// costs here: it is SMALLINT and has to be clamped.
		`CREATE TABLE IF NOT EXISTS monthly_usage (
	period_month DATE PRIMARY KEY,
	pageviews BIGINT NOT NULL DEFAULT 0,
	timeline_entries BIGINT NOT NULL DEFAULT 0,
	computed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
		// The timeline meter counts a calendar month of contact_timeline while
		// excluding the rows the web analytics projection writes, so a pageview
		// already counted in monthly_usage.pageviews is never billed twice.
		//
		// contact_timeline has no index a bare created_at range can use — its
		// only dated index leads with email — so without this the meter seq-scans
		// the whole table on every pass.
		//
		// The predicate repeats the entity_type literals written by
		// internal/repository/web_analytics_timeline_projection.go verbatim, and
		// has to stay identical to the meter's WHERE clause or PostgreSQL will
		// not match the index. A rename on either side costs a seq scan, not
		// correctness.
		`CREATE INDEX IF NOT EXISTS idx_contact_timeline_billable
	ON contact_timeline (created_at)
	WHERE entity_type NOT IN ('web_page', 'web_session')`,
	}
}
