package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intdb "github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// A caller cancelled while its work runs stops nothing: the lock stays held until the
// work returns, then goes.
func TestWorkspaceLock_CallerCancelledWhileWorkRuns(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	dbName := cfg.Prefix + "_ws_" + uniqueID("lockcancel")

	ctx, cancel := context.WithCancel(context.Background())
	running := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	workErr := errors.New("work result")
	go func() {
		done <- intdb.WithWorkspaceDatabaseLock(ctx, cfg, dbName, func(ctx context.Context, admin *sql.DB) error {
			close(running)
			<-finish
			return workErr
		})
	}()
	select {
	case <-running:
	case <-time.After(barrierTimeout):
		require.FailNow(t, "the lock was never taken")
	}

	cancel()
	time.Sleep(200 * time.Millisecond) // database/sql would roll back here, were the lock on ctx
	assert.Equal(t, 1, nameLockCount(t, admin, dbName, true), "the lock must outlive the caller's cancellation while the work runs")

	close(finish)
	select {
	case err := <-done:
		assert.ErrorIs(t, err, workErr)
	case <-time.After(barrierTimeout):
		require.FailNow(t, "WithWorkspaceDatabaseLock never returned")
	}
	waitForNameLockReleased(t, admin, dbName)
}

// lib/pq can report a lock granted while closing the connection, and the lock with it,
// when the caller's context ends at that moment. Whenever the work runs, the lock is
// really held.
func TestWorkspaceLock_GrantedAsTheContextEnds(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	dbName := cfg.Prefix + "_ws_" + uniqueID("lockrace")

	ran, refused := 0, 0
	for round := 0; round < 100; round++ {
		release := holdNameLock(t, cfg, dbName)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		var heldDuringWork bool
		var countErr error
		go func() {
			done <- intdb.WithWorkspaceDatabaseLock(ctx, cfg, dbName, func(ctx context.Context, _ *sql.DB) error {
				// No require here: this is not the test's goroutine.
				var n int
				n, countErr = queryNameLockCount(admin, dbName, true)
				heldDuringWork = n == 1
				return nil
			})
		}()
		waitForNameLockWaiters(t, admin, dbName, 1)

		require.NoError(t, release())
		cancel()

		select {
		case err := <-done:
			if err == nil {
				ran++
				require.NoError(t, countErr)
				require.True(t, heldDuringWork, "round %d: the work ran without the lock", round)
			} else {
				refused++
				require.ErrorIs(t, err, context.Canceled, "round %d", round)
			}
		case <-time.After(barrierTimeout):
			require.FailNowf(t, "barrier timeout", "round %d: the waiter never returned", round)
		}
		waitForNameLockReleased(t, admin, dbName)
	}
	t.Logf("work ran %d times, refused %d times", ran, refused)
}

// A role whose settings would end the holder early or abort the wait changes nothing:
// the lock transaction resets them.
func TestWorkspaceLock_ServerSettingsCannotBreakTheLock(t *testing.T) {
	testutil.SkipIfShort(t)
	base := testDatabaseConfig()
	admin := openAdmin(t, base)

	var serverVersion int
	require.NoError(t, admin.QueryRow(`SELECT current_setting('server_version_num')::int`).Scan(&serverVersion))

	role := uniqueID("lockrole")
	_, err := admin.Exec(fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD 'lock_probe'`, pq.QuoteIdentifier(role)))
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(`DROP ROLE IF EXISTS ` + pq.QuoteIdentifier(role)) })
	settings := []string{`idle_in_transaction_session_timeout = '500ms'`, `lock_timeout = '100ms'`, `statement_timeout = '100ms'`}
	if serverVersion >= 170000 {
		settings = append(settings, `transaction_timeout = '1s'`)
	}
	for _, s := range settings {
		_, err := admin.Exec(fmt.Sprintf(`ALTER ROLE %s SET %s`, pq.QuoteIdentifier(role), s))
		require.NoError(t, err)
	}

	cfg := *base
	cfg.User = role
	cfg.Password = "lock_probe"
	dbName := base.Prefix + "_ws_" + uniqueID("locksettings")

	holding := make(chan struct{})
	var wg sync.WaitGroup
	var holderErr, waiterErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		holderErr = intdb.WithWorkspaceDatabaseLock(context.Background(), &cfg, dbName, func(ctx context.Context, _ *sql.DB) error {
			close(holding)
			time.Sleep(2 * time.Second)
			return nil
		})
	}()
	select {
	case <-holding:
	case <-time.After(barrierTimeout):
		require.FailNow(t, "the holder never took the lock")
	}

	start := time.Now()
	wg.Add(1)
	go func() {
		defer wg.Done()
		waiterErr = intdb.WithWorkspaceDatabaseLock(context.Background(), &cfg, dbName, func(ctx context.Context, _ *sql.DB) error {
			return nil
		})
	}()
	wg.Wait()

	require.NoError(t, holderErr)
	require.NoError(t, waiterErr)
	assert.GreaterOrEqual(t, time.Since(start), 1500*time.Millisecond, "the waiter must have waited for the holder")
}
