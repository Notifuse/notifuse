package integration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	intdb "github.com/Notifuse/notifuse/internal/database"
	pgdb "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/tests/testutil"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests reproduce the connection-manager churn behind issue #380
// ("phantom password authentication failed" errors). They drive the REAL
// pkg/database.connectionManager (not the testutil pool) through a controllable
// latency proxy, and assert the invariants the churn violates. Each test is
// designed to be RED on the pre-fix code and GREEN once the fix lands, so they
// double as regression guards.
//
// The connection manager is a process-wide singleton, so these tests must not
// run in parallel with each other.

func dsnFor(cfg *config.DatabaseConfig, dbName string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, dbName, cfg.SSLMode)
}

// setupChurnManager initializes a real connection manager whose workspace
// connections are routed through the latency proxy, while returning a
// system/monitoring DB and a "direct" config that both bypass the proxy (for
// fast test setup and for observing PostgreSQL's own connection counters).
func setupChurnManager(t *testing.T, proxy *testutil.LatencyProxy) (pgdb.ConnectionManager, *sql.DB, *config.DatabaseConfig) {
	t.Helper()

	// Registered first so it runs LAST (t.Cleanup is LIFO): the manager and its
	// pools must be torn down before the proxy that carries their connections.
	t.Cleanup(func() { _ = proxy.Close() })

	base := testutil.GetTestDatabaseConfig()
	direct := *base // real host/port, no proxy

	dbCfg := *base
	dbCfg.Host = proxy.Host()
	dbCfg.Port = proxy.Port()
	dbCfg.MaxConnections = 100
	dbCfg.MaxConnectionsPerDB = 10
	dbCfg.ConnectionMaxLifetime = 10 * time.Minute
	dbCfg.ConnectionMaxIdleTime = 5 * time.Minute

	// Note: InitializeConnectionManager reconfigures this handle's pool size, so
	// we don't bother capping it here.
	systemDB, err := sql.Open("postgres", dsnFor(&direct, "postgres"))
	require.NoError(t, err)
	require.NoError(t, systemDB.Ping())

	cfg := &config.Config{Database: dbCfg}
	pgdb.ResetConnectionManager()
	require.NoError(t, pgdb.InitializeConnectionManager(cfg, systemDB))
	cm, err := pgdb.GetConnectionManager()
	require.NoError(t, err)

	t.Cleanup(func() {
		pgdb.ResetConnectionManager()
		_ = systemDB.Close()
	})

	return cm, systemDB, &direct
}

func ensureWSDB(t *testing.T, direct *config.DatabaseConfig, wsID string) {
	t.Helper()
	require.NoError(t, intdb.EnsureWorkspaceDatabaseExists(direct, wsID))
}

// wsDBName mirrors how the connection manager derives the physical database name
// for a workspace.
func wsDBName(prefix, wsID string) string {
	return fmt.Sprintf("%s_ws_%s", prefix, strings.ReplaceAll(wsID, "-", "_"))
}

// adminSessions reads PostgreSQL's cumulative session counter for the `postgres`
// admin database. EnsureWorkspaceDatabaseExists is the only Notifuse code path
// that connects to `postgres`, so a growing counter is a direct measurement of
// admin-DB churn during workspace pool (re)creation.
func adminSessions(t *testing.T, monDB *sql.DB) int64 {
	t.Helper()
	var n int64
	err := monDB.QueryRow(`SELECT sessions FROM pg_stat_database WHERE datname = 'postgres'`).Scan(&n)
	require.NoError(t, err)
	return n
}

// T1: a transient connection blip must not turn a healthy cached pool into a
// caller-visible "failed to get workspace connection" error, nor Close the pool
// out from under other goroutines. Today the per-call health-check ping fails
// during the blip, the pool is evicted+Closed, and recreation fails too — the
// exact phantom failure reported in issue #380 that "works on reload".
func TestConnectionManagerChurn_TransientBlipDoesNotDestroyHealthyPool(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	base := testutil.GetTestDatabaseConfig()
	proxy, err := testutil.NewLatencyProxy(fmt.Sprintf("%s:%d", base.Host, base.Port))
	require.NoError(t, err)

	cm, _, direct := setupChurnManager(t, proxy)
	ws := "churn_t1"
	ensureWSDB(t, direct, ws)

	ctx := context.Background()
	db1, err := cm.GetWorkspaceConnection(ctx, ws)
	require.NoError(t, err)
	require.NoError(t, db1.PingContext(ctx)) // warm one idle connection

	// A brief, self-healing connection outage: severs the idle connection and
	// makes the health-check ping genuinely error.
	proxy.SetBlackhole(true)

	_, err = cm.GetWorkspaceConnection(ctx, ws)
	assert.NoError(t, err,
		"acquiring an already-healthy workspace pool failed during a transient connection blip")

	// Recovery: once the blip clears, the same pool must self-heal. A freshly
	// severed idle connection may surface one transient driver.ErrBadConn first,
	// so allow a couple of retries. On the pre-fix code db1 was Closed during
	// the blip ("sql: database is closed"), so it never recovers.
	proxy.SetBlackhole(false)
	require.Eventually(t, func() bool {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return db1.PingContext(c) == nil
	}, 5*time.Second, 100*time.Millisecond,
		"the original pool was Closed out from under its holders and never recovered")
}

// T2: reconnecting to an existing workspace database must not open a fresh
// connection to the `postgres` admin database every time.
func TestConnectionManagerChurn_NoAdminDBStormOnReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	base := testutil.GetTestDatabaseConfig()
	proxy, err := testutil.NewLatencyProxy(fmt.Sprintf("%s:%d", base.Host, base.Port))
	require.NoError(t, err)

	cm, systemDB, direct := setupChurnManager(t, proxy)
	ws := "churn_t2"
	ensureWSDB(t, direct, ws)

	ctx := context.Background()

	// Warm the pool once (may legitimately touch the admin DB the first time),
	// then measure only steady-state reconnects.
	_, err = cm.GetWorkspaceConnection(ctx, ws)
	require.NoError(t, err)
	_ = adminSessions(t, systemDB) // warm the monitoring connection

	const recreations = 10
	s0 := adminSessions(t, systemDB)
	for i := 0; i < recreations; i++ {
		require.NoError(t, cm.CloseWorkspaceConnection(ws))
		_, err := cm.GetWorkspaceConnection(ctx, ws)
		require.NoError(t, err)
	}
	s1 := adminSessions(t, systemDB)

	// The bug re-connects to the admin DB once per recreation (delta ==
	// recreations); the fix touches it zero times. Assert against the midpoint so
	// there is margin in BOTH directions: unrelated `postgres`-DB connections or
	// a not-yet-flushed session can't false-GREEN the pre-fix run, and stray
	// noise can't false-RED the fixed run.
	assert.Less(t, s1-s0, int64(recreations/2),
		"each pool recreation opened a connection to the postgres admin DB "+
			"(EnsureWorkspaceDatabaseExists runs on the hot path); delta=%d over %d recreations",
		s1-s0, recreations)
}

// T3: creating one workspace's pool must not block access to a different,
// already-cached workspace (no global lock held across network I/O).
func TestConnectionManagerChurn_PoolCreationDoesNotBlockOtherWorkspaces(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	base := testutil.GetTestDatabaseConfig()
	proxy, err := testutil.NewLatencyProxy(fmt.Sprintf("%s:%d", base.Host, base.Port))
	require.NoError(t, err)

	cm, _, direct := setupChurnManager(t, proxy)
	wsA := "churn_t3_a"
	wsB := "churn_t3_b"
	ensureWSDB(t, direct, wsA)
	ensureWSDB(t, direct, wsB)

	ctx := context.Background()

	// Cache B (healthy). A stays uncached so the next Get(A) creates its pool.
	_, err = cm.GetWorkspaceConnection(ctx, wsB)
	require.NoError(t, err)

	// Only NEW connections are slow; B's cached-pool round-trips stay fast.
	proxy.SetDialLatency(700 * time.Millisecond)

	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		_, _ = cm.GetWorkspaceConnection(ctx, wsA)
	}()

	// Give A's goroutine time to enter pool creation.
	time.Sleep(150 * time.Millisecond)

	start := time.Now()
	_, err = cm.GetWorkspaceConnection(ctx, wsB)
	elapsed := time.Since(start)
	require.NoError(t, err)

	assert.Less(t, elapsed, 300*time.Millisecond,
		"access to cached workspace B was blocked by workspace A's pool creation "+
			"(global lock held across network I/O); waited %s", elapsed)

	proxy.SetDialLatency(0)
	<-aDone
}

// T4: concurrent first-touch of the SAME uncached workspace must be coalesced
// (via singleflight) into a single pool creation — every caller receives the
// same *sql.DB. Without coalescing, callers racing during the creation window
// each build (and leak) their own pool. This guards the fix's central mechanism.
func TestConnectionManagerChurn_ConcurrentCreationCoalesces(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	base := testutil.GetTestDatabaseConfig()
	proxy, err := testutil.NewLatencyProxy(fmt.Sprintf("%s:%d", base.Host, base.Port))
	require.NoError(t, err)

	cm, _, direct := setupChurnManager(t, proxy)
	ws := "churn_coalesce"
	ensureWSDB(t, direct, ws)

	// Slow the initial connect so all goroutines pile up during the single
	// in-flight creation window; this makes a missing-coalescing regression fail
	// deterministically rather than racily.
	proxy.SetDialLatency(300 * time.Millisecond)

	const n = 12
	ctx := context.Background()
	pools := make([]*sql.DB, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			pools[i], errs[i] = cm.GetWorkspaceConnection(ctx, ws)
		}(i)
	}
	wg.Wait()
	proxy.SetDialLatency(0)

	for i := 0; i < n; i++ {
		require.NoErrorf(t, errs[i], "caller %d failed", i)
		require.Samef(t, pools[0], pools[i],
			"caller %d received a different pool — concurrent creation was not coalesced", i)
	}
}

// T5: a pool whose server-side backends are killed (PG restart, admin
// pg_terminate_backend) must self-heal transparently through database/sql's
// ErrBadConn retry — the manager keeps serving the SAME *sql.DB and never
// recreates it. This validates the premise for removing the per-call ping.
func TestConnectionManagerChurn_PoolSelfHealsAfterBackendKill(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	base := testutil.GetTestDatabaseConfig()
	proxy, err := testutil.NewLatencyProxy(fmt.Sprintf("%s:%d", base.Host, base.Port))
	require.NoError(t, err)

	cm, systemDB, direct := setupChurnManager(t, proxy)
	ws := "churn_selfheal"
	ensureWSDB(t, direct, ws)
	dbName := wsDBName(direct.Prefix, ws)

	ctx := context.Background()
	db1, err := cm.GetWorkspaceConnection(ctx, ws)
	require.NoError(t, err)
	var one int
	require.NoError(t, db1.QueryRowContext(ctx, "SELECT 1").Scan(&one)) // warm a connection

	// Kill every backend for this workspace database, as a server restart would.
	_, err = systemDB.ExecContext(ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`,
		dbName)
	require.NoError(t, err)

	// The same pool must recover on next use (a freshly severed idle connection
	// may surface one transient driver.ErrBadConn first).
	require.Eventually(t, func() bool {
		return db1.PingContext(ctx) == nil
	}, 5*time.Second, 100*time.Millisecond, "pool did not self-heal after its backends were killed")

	db2, err := cm.GetWorkspaceConnection(ctx, ws)
	require.NoError(t, err)
	require.Same(t, db1, db2, "manager recreated the pool instead of reusing the self-healed one")
}
