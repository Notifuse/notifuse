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

	// Partitioned parents hold no rows themselves; the monthly children do.
	var partitionCount int
	require.NoError(t, initDB.QueryRow(`
		SELECT COUNT(*) FROM pg_inherits i
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname IN ('web_sessions','web_pages','web_goals')`).Scan(&partitionCount))
	assert.Equal(t, 6, partitionCount, "current + next month partitions for all three tables")
}

// dumpWebAnalyticsSchema renders columns, indexes and storage parameters of
// every web_* relation as a stable, comparable string.
func dumpWebAnalyticsSchema(t *testing.T, db *sql.DB) string {
	t.Helper()
	var out string

	rows, err := db.Query(`
		SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid
		WHERE n.nspname = 'public' AND c.relname LIKE 'web\_%'
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
		WHERE schemaname = 'public' AND tablename LIKE 'web\_%'
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
		WHERE n.nspname = 'public' AND c.relname LIKE 'web\_%' AND c.relkind = 'r'
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
