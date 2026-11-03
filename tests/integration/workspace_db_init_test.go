package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	intdb "github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// plantReturnTypeConflict makes the workspace schema fail late and deterministically:
// track_contact_changes() is created after every table, and CREATE OR REPLACE cannot
// change a function's return type.
func plantReturnTypeConflict(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`CREATE FUNCTION track_contact_changes() RETURNS int AS 'SELECT 1' LANGUAGE sql`)
	require.NoError(t, err)
}

func TestWorkspaceDBInit_IsAtomic(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	name := cfg.Prefix + "_ws_" + uniqueID("atomic")
	createEmptyDatabase(t, admin, name)
	db := openDatabase(t, cfg, name)
	plantReturnTypeConflict(t, db)

	err := intdb.InitializeWorkspaceDatabase(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "return type")

	assert.Equal(t, 0, relationCount(t, db), "a failed initialisation must leave no relation behind")
	holds, err := intdb.WorkspaceDatabaseHoldsTables(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, holds)
}

func TestWorkspaceDBInit_RunsTwiceOverACompleteSchema(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	name := cfg.Prefix + "_ws_" + uniqueID("twice")
	createEmptyDatabase(t, admin, name)
	db := openDatabase(t, cfg, name)

	require.NoError(t, intdb.InitializeWorkspaceDatabase(db))
	first := relationCount(t, db)
	require.NoError(t, intdb.InitializeWorkspaceDatabase(db))
	assert.Equal(t, first, relationCount(t, db))
}

func TestWorkspaceDBInit_HoldsTables(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	name := cfg.Prefix + "_ws_" + uniqueID("holds")
	createEmptyDatabase(t, admin, name)
	db := openDatabase(t, cfg, name)
	ctx := context.Background()

	holds, err := intdb.WorkspaceDatabaseHoldsTables(ctx, db)
	require.NoError(t, err)
	assert.False(t, holds, "a new database holds no tables")

	_, err = db.Exec(`CREATE EXTENSION pg_stat_statements`)
	require.NoError(t, err)
	holds, err = intdb.WorkspaceDatabaseHoldsTables(ctx, db)
	require.NoError(t, err)
	assert.False(t, holds, "an extension's own views are not the database's tables")

	// Another session's temporary table is listed in pg_class, under pg_temp_N.
	other := openDatabase(t, cfg, name)
	conn, err := other.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.ExecContext(ctx, `CREATE TEMPORARY TABLE scratch (x int)`)
	require.NoError(t, err)
	holds, err = intdb.WorkspaceDatabaseHoldsTables(ctx, db)
	require.NoError(t, err)
	assert.False(t, holds, "another session's temporary table does not count")

	_, err = db.Exec(`CREATE TABLE keep_me (x int)`)
	require.NoError(t, err)
	holds, err = intdb.WorkspaceDatabaseHoldsTables(ctx, db)
	require.NoError(t, err)
	assert.True(t, holds)
}

// provisioningSystem is a throwaway system database holding only the workspaces table,
// with the registration check and the insert workspace creation passes.
type provisioningSystem struct {
	db *sql.DB
}

func newProvisioningSystem(t *testing.T, cfg *config.DatabaseConfig, admin *sql.DB) *provisioningSystem {
	t.Helper()
	name := cfg.Prefix + "_" + uniqueID("provsys")
	createEmptyDatabase(t, admin, name)
	db := openDatabase(t, cfg, name)
	// creation tells rows apart as created_at does for the real table.
	_, err := db.Exec(`CREATE TABLE workspaces (id VARCHAR(32) PRIMARY KEY, creation TEXT)`)
	require.NoError(t, err)
	return &provisioningSystem{db: db}
}

func (s *provisioningSystem) registered(id string) func(ctx context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		var exists bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`, id).Scan(&exists)
		return exists, err
	}
}

// creation returns the row lookup and the insert of one creation of id, which tells
// its own row by a token of its own. Rows inserted otherwise are other creations'.
func (s *provisioningSystem) creation(id string, columns []string, values []any) (func(ctx context.Context) (intdb.WorkspaceRow, error), intdb.WorkspaceInsert) {
	token := uuid.NewString()
	row := func(ctx context.Context) (intdb.WorkspaceRow, error) {
		var stored sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT creation FROM workspaces WHERE id = $1`, id).Scan(&stored)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return intdb.WorkspaceRowAbsent, nil
		case err != nil:
			return intdb.WorkspaceRowAbsent, err
		case stored.Valid && stored.String == token:
			return intdb.WorkspaceRowOurs, nil
		default:
			return intdb.WorkspaceRowOther, nil
		}
	}
	insert := intdb.WorkspaceInsert{DB: s.db, Columns: append([]string{"creation"}, columns...), Values: append([]any{token}, values...)}
	return row, insert
}

// register inserts id's row directly.
func (s *provisioningSystem) register(t *testing.T, id string) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO workspaces (id) VALUES ($1)`, id)
	require.NoError(t, err)
}

// create creates id as workspace creation does.
func (s *provisioningSystem) create(cfg *config.DatabaseConfig, id string) error {
	row, insert := s.creation(id, nil, nil)
	return intdb.CreateWorkspaceDatabase(context.Background(), cfg, id, row, insert)
}

func (s *provisioningSystem) stillRegistered(id string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		ok, err := s.registered(id)(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return &domain.ErrWorkspaceNotFound{WorkspaceID: id}
		}
		return nil
	}
}

// Two creations of the same new ID serialise on the name lock: one creates, the other
// then finds the row. Without the lock, both pass the row check.
func TestWorkspaceDBInit_ConcurrentCreatesOfOneID(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	sys := newProvisioningSystem(t, cfg, admin)
	reference := referenceSchemaRelationCount(t, cfg, admin)

	id := uniqueID("concurrent")
	dbName := cfg.Prefix + "_ws_" + id
	dropDatabaseAtCleanup(t, admin, dbName)

	release := holdNameLock(t, cfg, dbName)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = sys.create(cfg, id)
		}(i)
	}
	waitForNameLockWaiters(t, admin, dbName, 2)
	require.NoError(t, release())
	wg.Wait()

	succeeded, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrWorkspaceAlreadyExists):
			refused++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, refused)

	var rows int
	require.NoError(t, sys.db.QueryRow(`SELECT count(*) FROM workspaces WHERE id = $1`, id).Scan(&rows))
	assert.Equal(t, 1, rows)
	assert.Equal(t, reference, relationCount(t, openDatabase(t, cfg, dbName)), "the schema must be complete")
}

// Two requests for a registered workspace whose database exists and is empty: one
// initialises it, the other waits and finds the tables.
func TestWorkspaceDBInit_ConcurrentEnsuresOfAnEmptyDatabase(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	sys := newProvisioningSystem(t, cfg, admin)
	reference := referenceSchemaRelationCount(t, cfg, admin)

	id := uniqueID("ensure")
	dbName := cfg.Prefix + "_ws_" + id
	createEmptyDatabase(t, admin, dbName)
	sys.register(t, id)

	release := holdNameLock(t, cfg, dbName)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = intdb.EnsureWorkspaceDatabaseExists(context.Background(), cfg, id, sys.stillRegistered(id))
		}(i)
	}
	waitForNameLockWaiters(t, admin, dbName, 2)
	require.NoError(t, release())
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, reference, relationCount(t, openDatabase(t, cfg, dbName)))
}

// A failed initialisation under the lock leaves no lock behind, nor a database it did
// not create.
func TestWorkspaceDBInit_FailureInsideTheLockReleasesIt(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	sys := newProvisioningSystem(t, cfg, admin)

	id := uniqueID("initfail")
	dbName := cfg.Prefix + "_ws_" + id
	createEmptyDatabase(t, admin, dbName)
	plantReturnTypeConflict(t, openDatabase(t, cfg, dbName))
	sys.register(t, id)

	err := intdb.EnsureWorkspaceDatabaseExists(context.Background(), cfg, id, sys.stillRegistered(id))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "return type")

	assert.Equal(t, 0, nameLockCount(t, admin, dbName, true))
	assert.Equal(t, 0, nameLockCount(t, admin, dbName, false))
	assert.True(t, databaseExists(t, admin, dbName), "a database the call did not create is never dropped")
}

// A database created by this call whose initialisation fails is dropped again, so the
// same ID can be created afterwards.
func TestWorkspaceDBInit_CreatedDatabaseIsDroppedOnFailure(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)
	sys := newProvisioningSystem(t, cfg, admin)

	id := uniqueID("dropfail")
	dbName := cfg.Prefix + "_ws_" + id
	dropDatabaseAtCleanup(t, admin, dbName)

	// The insert fails after the database was created and initialised, refused by the
	// server for what it is (a not-null violation), so it can never have committed and
	// the database goes.
	_, err := sys.db.Exec(`ALTER TABLE workspaces ADD COLUMN name TEXT NOT NULL`)
	require.NoError(t, err)
	createNamed := func(name any) error {
		row, insert := sys.creation(id, []string{"name"}, []any{name})
		return intdb.CreateWorkspaceDatabase(context.Background(), cfg, id, row, insert)
	}
	err = createNamed(nil)
	require.ErrorContains(t, err, "null value")
	assert.False(t, databaseExists(t, admin, dbName))

	require.NoError(t, createNamed("Acme"))
	assert.True(t, databaseExists(t, admin, dbName))
}

// A workspace creation tells its own row by its creation time, which it stamps to the
// microsecond (pinned by the repository's unit tests): on the real workspaces column, a
// value stamped so must come back equal. PostgreSQL rounds a finer value instead.
func TestWorkspaceCreatedAtRoundTrip(t *testing.T) {
	testutil.SkipIfShort(t)
	sys := newMigrationSystem(t, openAdmin(t, testDatabaseConfig()))

	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("roundtrip%d", i)
		sent := time.Now().UTC().Truncate(time.Microsecond)
		_, err := sys.db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ($1, $1, $2, $2)`, id, sent)
		require.NoError(t, err)
		var stored time.Time
		require.NoError(t, sys.db.QueryRow(`SELECT created_at FROM workspaces WHERE id = $1`, id).Scan(&stored))
		require.True(t, stored.Equal(sent), "round %d: sent %v, stored %v", i, sent, stored)
		time.Sleep(time.Duration(i%7) * 13 * time.Microsecond) // spread the stamps
	}
}

// An insert of the same ID still in progress elsewhere (a writer that takes no name
// lock: an earlier release, during a rolling deploy) is waited for, and its outcome
// decides: committed, the ID is that writer's and its database stays; rolled back, the
// ID is this creation's.
func TestWorkspaceDBInit_InsertWaitsForAnInsertInProgress(t *testing.T) {
	testutil.SkipIfShort(t)
	cfg := testDatabaseConfig()
	admin := openAdmin(t, cfg)

	for _, tc := range []struct {
		commit, killWaiter bool
	}{{true, false}, {false, false}, {true, true}, {false, true}} {
		commit := tc.commit
		t.Run(fmt.Sprintf("other insert committed: %v, waiting connection killed: %v", tc.commit, tc.killWaiter), func(t *testing.T) {
			sys := newProvisioningSystem(t, cfg, admin)
			id := uniqueID("inflight")
			dbName := cfg.Prefix + "_ws_" + id
			dropDatabaseAtCleanup(t, admin, dbName)

			other, err := sys.db.Begin()
			require.NoError(t, err)
			defer func() { _ = other.Rollback() }()
			_, err = other.Exec(`INSERT INTO workspaces (id) VALUES ($1)`, id)
			require.NoError(t, err)

			done := make(chan error, 1)
			go func() {
				done <- sys.create(cfg, id)
			}()
			waitForRowLockWaiter(t, sys.db)
			if tc.killWaiter {
				// The connection of the waiting insert goes: its outcome is unknown to
				// the creation, which inserts again on a new one and waits again.
				pids := rowLockWaiters(t, sys.db)
				require.Len(t, pids, 1, "only the creation's insert waits")
				pid := pids[0]
				var signalled bool
				require.NoError(t, sys.db.QueryRow(`SELECT pg_terminate_backend($1)`, pid).Scan(&signalled))
				require.True(t, signalled)
				// The signal only asks: the dying backend's wait shows until it is gone.
				waitForBackendGone(t, sys.db, pid)
				waitForRowLockWaiter(t, sys.db)
			}

			if commit {
				require.NoError(t, other.Commit())
			} else {
				require.NoError(t, other.Rollback())
			}
			select {
			case err := <-done:
				if commit {
					assert.ErrorIs(t, err, domain.ErrWorkspaceAlreadyExists)
				} else {
					assert.NoError(t, err)
				}
			case <-time.After(barrierTimeout):
				require.FailNow(t, "the creation never returned")
			}
			assert.True(t, databaseExists(t, admin, dbName), "the database stays either way")
		})
	}
}

// waitForBackendGone waits until the backend pid has exited.
func waitForBackendGone(t *testing.T, db *sql.DB, pid int) {
	t.Helper()
	deadline := time.Now().Add(barrierTimeout)
	for {
		var n int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&n))
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			require.FailNowf(t, "barrier timeout", "backend %d never exited", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// rowLockWaiters lists the sessions of db waiting on another transaction.
func rowLockWaiters(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query(`SELECT pid FROM pg_locks WHERE locktype = 'transactionid' AND NOT granted
		AND pid IN (SELECT pid FROM pg_stat_activity WHERE datname = current_database())`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var pids []int
	for rows.Next() {
		var pid int
		require.NoError(t, rows.Scan(&pid))
		pids = append(pids, pid)
	}
	require.NoError(t, rows.Err())
	return pids
}

// waitForRowLockWaiter waits until a session of db waits on another transaction.
func waitForRowLockWaiter(t *testing.T, db *sql.DB) {
	t.Helper()
	deadline := time.Now().Add(barrierTimeout)
	for {
		if len(rowLockWaiters(t, db)) > 0 {
			return
		}
		if time.Now().After(deadline) {
			require.FailNow(t, "barrier timeout", "no insert ever waited on the one in progress")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
