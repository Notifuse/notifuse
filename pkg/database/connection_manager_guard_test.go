package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

const registeredQuery = `SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`

type guardClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *guardClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *guardClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// guardManager builds a manager over a sqlmock system database and an injected pool
// opener that counts its calls and opens sqlmock pools, so no test dials anything.
func guardManager(t *testing.T, maxConns, perDB int) (*connectionManager, sqlmock.Sqlmock, *atomic.Int32, *guardClock) {
	t.Helper()
	cfg := createTestConfig()
	cfg.Database.MaxConnections = maxConns
	cfg.Database.MaxConnectionsPerDB = perDB
	sysDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sysDB.Close() })

	var dials atomic.Int32
	clock := &guardClock{t: time.Unix(1_730_000_000, 0)}
	cm := &connectionManager{
		config:              cfg,
		systemDB:            sysDB,
		workspacePools:      make(map[string]*sql.DB),
		poolAccessTimes:     make(map[string]time.Time),
		maxConnections:      maxConns,
		maxConnectionsPerDB: perDB,
		now:                 clock.now,
		openPool: func(ctx context.Context, dsn, workspaceID string) (*sql.DB, error) {
			dials.Add(1)
			pool, _, err := sqlmock.New()
			if err != nil {
				return nil, err
			}
			t.Cleanup(func() { _ = pool.Close() })
			return pool, nil
		},
	}
	return cm, mock, &dials, clock
}

func expectRegistered(mock sqlmock.Sqlmock, id string, exists bool) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(registeredQuery).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

// idlePool returns a sqlmock pool holding one open, idle connection, which the LRU
// eviction would close to make room.
func idlePool(t *testing.T) *sql.DB {
	t.Helper()
	pool, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, pool.Ping())
	require.Equal(t, 1, pool.Stats().OpenConnections)
	return pool
}

func requireNotFound(t *testing.T, err error, id string) {
	t.Helper()
	var notFound *domain.ErrWorkspaceNotFound
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, id, notFound.WorkspaceID)
}

func TestGuard_UnregisteredIDTouchesNothing(t *testing.T) {
	// One idle pool of a real workspace, and no room for another pool: a creation
	// would evict it.
	cm, mock, dials, _ := guardManager(t, 4, 3)
	real := idlePool(t)
	cm.workspacePools["realws"] = real
	cm.poolAccessTimes["realws"] = time.Now().Add(-time.Hour)
	expectRegistered(mock, "ghost", false)

	pool, err := cm.GetWorkspaceConnection(context.Background(), "ghost")
	assert.Nil(t, pool)
	requireNotFound(t, err, "ghost")

	assert.Equal(t, int32(0), dials.Load(), "no dial for an unregistered ID")
	assert.Equal(t, 0, cm.inflightPools, "no capacity reserved")
	assert.True(t, cm.workspacePools["realws"] == real, "the real workspace's idle pool is not evicted")
	assert.Equal(t, 1, real.Stats().OpenConnections)
	assert.NotContains(t, cm.workspacePools, "ghost")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGuard_NotFoundIsCachedBriefly(t *testing.T) {
	cm, mock, _, clock := guardManager(t, 100, 3)
	expectRegistered(mock, "ghost", false)

	_, err := cm.GetWorkspaceConnection(context.Background(), "ghost")
	requireNotFound(t, err, "ghost")

	// Within the TTL: answered from the cache. sqlmock fails any unexpected query.
	clock.advance(unknownWorkspaceTTL - time.Second)
	_, err = cm.GetWorkspaceConnection(context.Background(), "ghost")
	requireNotFound(t, err, "ghost")
	require.NoError(t, mock.ExpectationsWereMet())

	// After it: one query again.
	clock.advance(2 * time.Second)
	expectRegistered(mock, "ghost", false)
	_, err = cm.GetWorkspaceConnection(context.Background(), "ghost")
	requireNotFound(t, err, "ghost")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGuard_NotFoundCacheIsCapped(t *testing.T) {
	cm, _, _, _ := guardManager(t, 100, 3)
	for i := range maxUnknownWorkspaces + 1 {
		cm.rememberUnknownWorkspace(fmt.Sprintf("ghost%d", i), cm.currentForgetEpoch())
	}
	cm.unknownMu.Lock()
	size := len(cm.unknownWorkspaces)
	cm.unknownMu.Unlock()
	assert.LessOrEqual(t, size, maxUnknownWorkspaces)
}

func TestGuard_ForgetUnknownWorkspace(t *testing.T) {
	cm, mock, dials, _ := guardManager(t, 100, 3)
	expectRegistered(mock, "newws", false)
	_, err := cm.GetWorkspaceConnection(context.Background(), "newws")
	requireNotFound(t, err, "newws")

	// The workspace is created: its row exists, and the repository forgets the miss.
	cm.ForgetUnknownWorkspace("newws")
	expectRegistered(mock, "newws", true)
	pool, err := cm.GetWorkspaceConnection(context.Background(), "newws")
	require.NoError(t, err)
	assert.NotNil(t, pool)
	assert.Equal(t, int32(1), dials.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGuard_NoLostForget: a check whose SELECT read "no row" just before the row was
// inserted must not cache not-found after the forget that followed the insert.
func TestGuard_NoLostForget(t *testing.T) {
	cm, mock, _, _ := guardManager(t, 100, 3)
	expectRegistered(mock, "demows", false).WillDelayFor(300 * time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := cm.GetWorkspaceConnection(context.Background(), "demows")
		done <- err
	}()
	// The SELECT is in flight; the workspace is created and forgotten meanwhile.
	time.Sleep(100 * time.Millisecond)
	cm.ForgetUnknownWorkspace("demows")
	requireNotFound(t, <-done, "demows")

	assert.False(t, cm.isUnknownWorkspace("demows"), "a stale check must not cache not-found after a forget")
	expectRegistered(mock, "demows", true)
	_, err := cm.GetWorkspaceConnection(context.Background(), "demows")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGuard_RegisteredIDCreatesOnePool(t *testing.T) {
	cm, mock, dials, _ := guardManager(t, 100, 3)
	expectRegistered(mock, "realws", true)

	first, err := cm.GetWorkspaceConnection(context.Background(), "realws")
	require.NoError(t, err)
	for range 3 {
		again, err := cm.GetWorkspaceConnection(context.Background(), "realws")
		require.NoError(t, err)
		assert.True(t, again == first, "later calls take the cached pool")
	}
	assert.Equal(t, int32(1), dials.Load())
	require.NoError(t, mock.ExpectationsWereMet(), "one registration query, none on the fast path")
}

func TestGuard_ConcurrentUnregisteredCoalesce(t *testing.T) {
	cm, mock, dials, _ := guardManager(t, 100, 3)
	expectRegistered(mock, "ghost", false).WillDelayFor(100 * time.Millisecond)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cm.GetWorkspaceConnection(context.Background(), "ghost")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		requireNotFound(t, err, "ghost")
	}
	assert.Equal(t, int32(0), dials.Load())
	require.NoError(t, mock.ExpectationsWereMet(), "one query for all concurrent callers")
}

func TestGuard_ConcurrentMixedAtCapacityNeverEvictsForUnknownIDs(t *testing.T) {
	cm, mock, _, _ := guardManager(t, 4, 3)
	mock.MatchExpectationsInOrder(false)
	real := idlePool(t)
	cm.workspacePools["realws"] = real
	cm.poolAccessTimes["realws"] = time.Now().Add(-time.Hour)

	const ghosts = 20
	for i := range ghosts {
		expectRegistered(mock, fmt.Sprintf("ghost%d", i), false)
	}
	var wg sync.WaitGroup
	for i := range ghosts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("ghost%d", i)
			_, err := cm.GetWorkspaceConnection(context.Background(), id)
			requireNotFound(t, err, id)
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool, err := cm.GetWorkspaceConnection(context.Background(), "realws")
			assert.NoError(t, err)
			assert.True(t, pool == real)
		}()
	}
	wg.Wait()
	assert.True(t, cm.workspacePools["realws"] == real)
	assert.Equal(t, 1, real.Stats().OpenConnections, "the idle pool was never closed")
	assert.Equal(t, 0, cm.inflightPools)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGuard_CancelledFirstCallerDoesNotFailWaiters(t *testing.T) {
	cm, mock, _, _ := guardManager(t, 100, 3)
	expectRegistered(mock, "realws", true).WillDelayFor(200 * time.Millisecond)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	results := make(chan error, 11)
	call := func(ctx context.Context) {
		defer wg.Done()
		_, err := cm.GetWorkspaceConnection(ctx, "realws")
		results <- err
	}
	wg.Add(1)
	go call(firstCtx)
	time.Sleep(50 * time.Millisecond)
	for range 10 {
		wg.Add(1)
		go call(context.Background())
	}
	time.Sleep(50 * time.Millisecond)
	cancelFirst()
	wg.Wait()
	close(results)
	for err := range results {
		assert.NoError(t, err, "the creation is detached from the first caller's context")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
