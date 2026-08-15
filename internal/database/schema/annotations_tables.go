package schema

// Workspace annotations: dated markers explaining what a chart shows — a launch,
// a campaign, an outage. Deliberately not partitioned and deliberately absent
// from WebAnalyticsTableNames, which drives monthly partition creation in the
// maintenance worker; the volume is a handful of rows per workspace.
//
// The table is named annotations rather than web_annotations because the rows
// already annotate events that are not web analytics — a broadcast send writes
// one automatically.

// AnnotationsTableDefinitions returns the DDL for the workspace annotations
// table. Shared verbatim by internal/database/init.go and the v38 migration, so
// a fresh install and an upgraded one cannot drift.
func AnnotationsTableDefinitions() []string {
	return []string{
		// source_id is VARCHAR(255) to match broadcasts.id.
		`CREATE TABLE IF NOT EXISTS annotations (
	id VARCHAR(32) PRIMARY KEY,
	annotated_at TIMESTAMPTZ NOT NULL,
	timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
	title VARCHAR(100) NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	color VARCHAR(7) NOT NULL DEFAULT '#3b82f6',
	source VARCHAR(20) NOT NULL DEFAULT 'manual',
	source_id VARCHAR(255),
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE INDEX IF NOT EXISTS idx_annotations_annotated_at ON annotations (annotated_at DESC)`,
		// Idempotency for automatic annotations: a task retry, a redeploy mid-count
		// or two dispatchers racing all collapse to one row. Partial, so manual rows
		// (source_id NULL) stay unconstrained — a workspace can mark the same instant
		// twice by hand.
		//
		// PostgreSQL will not infer a *partial* unique index as an ON CONFLICT
		// arbiter, so every insert targeting this index must repeat the predicate
		// verbatim — `ON CONFLICT (source, source_id) WHERE source_id IS NOT NULL` —
		// or it raises 42P10 at runtime. Dropping the target entirely is not the fix:
		// a bare `ON CONFLICT DO NOTHING` would also swallow a primary-key collision.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_annotations_source
	ON annotations (source, source_id) WHERE source_id IS NOT NULL`,
	}
}
