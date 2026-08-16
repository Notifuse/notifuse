//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsSchemaParity guards the invariant that makes the web
// analytics DDL safe: a brand-new workspace (internal/database/init.go) and an
// upgraded one (the v38 migration) must end up with byte-identical tables,
// indexes and storage parameters. Both call the same shared DDL source, and
// this test fails loudly if that ever stops being true.
//
// The annotations table rides along: it is created from the same shared
// definitions by the same two paths, and its partial unique index is the one
// piece of this schema that a repository query has to name verbatim.
func TestWebAnalyticsSchemaParity(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	envOr := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return fallback
	}
	dsn := func(dbName string) string {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			envOr("TEST_DB_USER", "notifuse_test"),
			envOr("TEST_DB_PASSWORD", "test_password"),
			envOr("TEST_DB_HOST", "localhost"),
			envOr("TEST_DB_PORT", "5433"),
			dbName)
	}

	admin, err := sql.Open("postgres", dsn("postgres"))
	require.NoError(t, err)
	defer func() { _ = admin.Close() }()
	require.NoError(t, admin.Ping(), "integration Postgres must be running (docker compose -f tests/compose.test.yaml up -d)")

	suffix := time.Now().UnixNano()
	initDBName := fmt.Sprintf("wa_parity_init_%d", suffix)
	migrDBName := fmt.Sprintf("wa_parity_migr_%d", suffix)

	for _, name := range []string{initDBName, migrDBName} {
		_, err := admin.Exec("CREATE DATABASE " + name)
		require.NoError(t, err)
		defer func(n string) { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + n + " WITH (FORCE)") }(name)
	}

	// Path A: what InitializeWorkspaceDatabase runs for a fresh workspace.
	initDB, err := sql.Open("postgres", dsn(initDBName))
	require.NoError(t, err)
	defer func() { _ = initDB.Close() }()

	for _, query := range schema.WebAnalyticsTableDefinitions() {
		_, err := initDB.Exec(query)
		require.NoError(t, err, query)
	}
	for _, query := range schema.AnnotationsTableDefinitions() {
		_, err := initDB.Exec(query)
		require.NoError(t, err, query)
	}
	// The usage definitions index contact_timeline, so the table has to exist
	// first — exactly as it does in init.go, which creates it hundreds of lines
	// before the shared-DDL block, and in every workspace database that reaches
	// v38.
	createContactTimelineFixture(t, initDB)
	for _, query := range schema.UsageTableDefinitions() {
		_, err := initDB.Exec(query)
		require.NoError(t, err, query)
	}
	now := time.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 1, 0)} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := initDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	// Path B: the v38 migration on an existing workspace database.
	migrDB, err := sql.Open("postgres", dsn(migrDBName))
	require.NoError(t, err)
	defer func() { _ = migrDB.Close() }()

	// v38 does more than create the web analytics tables: it also regenerates the trigger
	// of every live automation. That step reads `automations`, which every workspace
	// database reaching v38 has carried since v20 — so give this one the same table the
	// migration will find in production. It stays empty: the heal step itself is covered
	// end-to-end in automation_email_click_trigger_test.go, and only the web_* relations
	// and annotations are compared below.
	_, err = migrDB.Exec(`
		CREATE TABLE automations (
			id VARCHAR(36) PRIMARY KEY,
			workspace_id VARCHAR(36) NOT NULL,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(20) DEFAULT 'draft',
			list_id VARCHAR(36),
			exit_on_reply BOOLEAN NOT NULL DEFAULT false,
			trigger_config JSONB NOT NULL,
			trigger_sql TEXT,
			root_node_id VARCHAR(36),
			nodes JSONB DEFAULT '[]',
			stats JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`)
	require.NoError(t, err)

	createContactTimelineFixture(t, migrDB)

	migration := &migrations.V38Migration{}
	require.NoError(t, migration.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws"}, migrDB))
	// Idempotency: re-running the migration must change nothing.
	require.NoError(t, migration.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws"}, migrDB))

	initSchema := dumpWebAnalyticsSchema(t, initDB)
	migrSchema := dumpWebAnalyticsSchema(t, migrDB)
	assert.Equal(t, initSchema, migrSchema, "fresh-install and upgrade paths must produce identical schemas")

	// Spot-check the properties the design depends on.
	assert.Contains(t, initSchema, "web_sessions.session_date date notnull=true")
	assert.Contains(t, initSchema, "web_sessions.beat_seq bigint notnull=true")
	assert.Contains(t, initSchema, "USING brin (created_at)")
	assert.Contains(t, initSchema, "WHERE (contact_email IS NOT NULL)")
	assert.Contains(t, initSchema, "fillfactor=85", "partitions must carry the HOT-update fillfactor")

	// The annotations unique index must be partial in both databases: it is the
	// arbiter CreateFromSource names, and it must leave manual rows (source_id
	// NULL) free to repeat.
	const partialUniqueIndex = "CREATE UNIQUE INDEX idx_annotations_source ON public.annotations " +
		"USING btree (source, source_id) WHERE (source_id IS NOT NULL)"
	assert.Contains(t, initSchema, partialUniqueIndex)
	assert.Contains(t, migrSchema, partialUniqueIndex)
	assert.Contains(t, initSchema, "annotations.source_id character varying(255) notnull=false")

	assertAnnotationConflictTargetResolves(t, initDB)
	assertAnnotationConflictTargetResolves(t, migrDB)

	// The usage meter's index must be partial in both databases, and its
	// predicate must match the meter's WHERE clause exactly or PostgreSQL will
	// not use it and the meter seq-scans contact_timeline on every pass.
	const billableTimelineIndex = "CREATE INDEX idx_contact_timeline_billable ON public.contact_timeline " +
		"USING btree (created_at) WHERE ((entity_type)::text <> ALL " +
		"((ARRAY['web_page'::character varying, 'web_session'::character varying])::text[]))"
	assert.Contains(t, initSchema, billableTimelineIndex)
	assert.Contains(t, migrSchema, billableTimelineIndex)
	assert.Contains(t, initSchema, "monthly_usage.period_month date notnull=true")
	// BIGINT, not the SMALLINT that web_sessions.pageview_count has to clamp.
	assert.Contains(t, initSchema, "monthly_usage.pageviews bigint notnull=true")
	assert.Contains(t, initSchema, "monthly_usage.timeline_entries bigint notnull=true")

	assertBillableTimelineIndexIsUsed(t, initDB)

	// Partitioned parents hold no rows themselves; the monthly children do.
	var partitionCount int
	require.NoError(t, initDB.QueryRow(`
		SELECT COUNT(*) FROM pg_inherits i
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname IN ('web_sessions','web_pages','web_goals')`).Scan(&partitionCount))
	assert.Equal(t, 6, partitionCount, "current + next month partitions for all three tables")
}

// createContactTimelineFixture mirrors the contact_timeline DDL that
// internal/database/init.go creates long before the shared-DDL block, and that
// every workspace database reaching v38 already carries. Both parity paths need
// it because the usage definitions put an index on it. Kept to the columns that
// index and the timeline meter touch; the rest of the table is not compared.
func createContactTimelineFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS contact_timeline (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) NOT NULL,
			operation VARCHAR(20) NOT NULL,
			entity_type VARCHAR(50) NOT NULL,
			kind VARCHAR(150) NOT NULL DEFAULT '',
			changes JSONB,
			entity_id VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			db_created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`)
	require.NoError(t, err)
}

// assertBillableTimelineIndexIsUsed proves the meter's WHERE clause still
// implies the predicate of idx_contact_timeline_billable.
//
// This is the failure the index has: a mismatch does not break the query, it
// makes the index unusable, and the meter quietly degrades to a full scan of
// contact_timeline on every maintenance pass. Nothing else would ever notice.
//
// enable_seqscan=off is a preference, not a force — PostgreSQL still seq-scans
// when no index can serve the query — so the plan choosing this index is real
// evidence, even on an empty table.
//
// Keep the statement byte-identical to monthlyUsageTimelineCount in
// internal/repository/web_analytics_postgres.go.
func assertBillableTimelineIndexIsUsed(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`SET enable_seqscan = off`)
	require.NoError(t, err)
	defer func() { _, _ = db.Exec(`RESET enable_seqscan`) }()

	rows, err := db.Query(`EXPLAIN
		SELECT COUNT(*) FROM contact_timeline
		WHERE created_at >= $1 AND created_at < $2
		AND entity_type NOT IN ('web_page', 'web_session')`,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var plan string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan += line + "\n"
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, plan, "idx_contact_timeline_billable",
		"the meter's WHERE clause must keep implying the index predicate; plan was:\n%s", plan)
}

// assertAnnotationConflictTargetResolves executes the insert that
// repository.CreateFromSource issues, twice, against a real database.
//
// PostgreSQL only accepts a partial unique index as an ON CONFLICT arbiter when
// the statement repeats the index predicate; without it the insert raises 42P10
// at runtime. sqlmock never parses SQL against a schema, so no unit test can see
// that — this is the only guard in the repository, and the bug it catches has
// already happened once here. Keep the statement byte-identical to the one in
// internal/repository/annotation_postgres.go.
func assertAnnotationConflictTargetResolves(t *testing.T, db *sql.DB) {
	t.Helper()

	const insertFromSource = `
		INSERT INTO annotations (
			id, annotated_at, timezone, title, description,
			color, source, source_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (source, source_id) WHERE source_id IS NOT NULL DO NOTHING
	`

	at := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	insert := func(id string, sourceID interface{}) int64 {
		res, err := db.Exec(insertFromSource, id, at, "UTC", "Broadcast sent", "",
			"#7763f1", "broadcast", sourceID, at, at)
		require.NoError(t, err, "ON CONFLICT must resolve against the partial unique index")
		affected, err := res.RowsAffected()
		require.NoError(t, err)
		return affected
	}

	assert.Equal(t, int64(1), insert("ann_first", "bcast1"))
	// Same (source, source_id): a task retry or a racing dispatcher collapses
	// onto the first row rather than duplicating the annotation.
	assert.Equal(t, int64(0), insert("ann_second", "bcast1"))

	// Manual rows carry no source_id and fall outside the index predicate, so
	// two of them at the same instant must both survive.
	assert.Equal(t, int64(1), insert("ann_manual_a", nil))
	assert.Equal(t, int64(1), insert("ann_manual_b", nil))

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM annotations`).Scan(&count))
	assert.Equal(t, 3, count)

	_, err := db.Exec(`DELETE FROM annotations`)
	require.NoError(t, err)
}

// dumpWebAnalyticsSchema renders columns, indexes and storage parameters of
// every web_* relation, plus annotations, as a stable, comparable string.
// annotations is matched by name because it deliberately carries no web_
// prefix — the rows annotate events that are not web analytics.
func dumpWebAnalyticsSchema(t *testing.T, db *sql.DB) string {
	t.Helper()
	var out string

	rows, err := db.Query(`
		SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid
		WHERE n.nspname = 'public' AND (c.relname LIKE 'web\_%' OR c.relname IN ('annotations', 'monthly_usage', 'contact_timeline'))
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY c.relname, a.attname`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table, column, dataType string
		var notNull bool
		require.NoError(t, rows.Scan(&table, &column, &dataType, &notNull))
		out += fmt.Sprintf("%s.%s %s notnull=%v\n", table, column, dataType, notNull)
	}
	require.NoError(t, rows.Err())

	indexes, err := db.Query(`
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'public' AND (tablename LIKE 'web\_%' OR tablename IN ('annotations', 'monthly_usage', 'contact_timeline'))
		ORDER BY indexdef`)
	require.NoError(t, err)
	defer func() { _ = indexes.Close() }()
	for indexes.Next() {
		var def string
		require.NoError(t, indexes.Scan(&def))
		out += "IDX " + def + "\n"
	}
	require.NoError(t, indexes.Err())

	options, err := db.Query(`
		SELECT c.relname, COALESCE(array_to_string(c.reloptions, ','), '')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND (c.relname LIKE 'web\_%' OR c.relname IN ('annotations', 'monthly_usage', 'contact_timeline'))
		  AND c.relkind = 'r'
		ORDER BY c.relname`)
	require.NoError(t, err)
	defer func() { _ = options.Close() }()
	for options.Next() {
		var name, reloptions string
		require.NoError(t, options.Scan(&name, &reloptions))
		out += fmt.Sprintf("OPTS %s [%s]\n", name, reloptions)
	}
	require.NoError(t, options.Err())

	return out
}
