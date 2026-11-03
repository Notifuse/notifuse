package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

// workspaceLockRecorder is a pass-through name lock recording what ran inside it.
type workspaceLockRecorder struct {
	names  []string
	inLock bool
}

func (l *workspaceLockRecorder) lock(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
	l.names = append(l.names, dbName)
	l.inLock = true
	defer func() { l.inLock = false }()
	return fn(ctx, nil)
}

func passThroughWorkspaceLock(rec *workspaceLockRecorder) func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
	if rec == nil {
		rec = &workspaceLockRecorder{}
	}
	return rec.lock
}

const workspaceSecretKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newCreateTestRepo(t *testing.T) (*workspaceRepository, sqlmock.Sqlmock, *mockConnectionManager) {
	t.Helper()
	db, mock, cleanup := testutil.SetupMockDB(t)
	t.Cleanup(cleanup)
	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, &config.DatabaseConfig{Prefix: "notifuse"}, "secret-key", connMgr).(*workspaceRepository)
	return repo, mock, connMgr
}

func newTestWorkspace(id string) *domain.Workspace {
	return &domain.Workspace{ID: id, Name: "Acme", Settings: domain.WorkspaceSettings{Timezone: "UTC", SecretKey: workspaceSecretKey}}
}

func TestWorkspaceRepository_CreateUnderNameLock(t *testing.T) {
	rowCheck := `SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`
	createdAtQuery := `SELECT created_at FROM workspaces WHERE id = \$1`

	t.Run("passes its row check and its insert, on a detached context", func(t *testing.T) {
		repo, mock, connMgr := newCreateTestRepo(t)
		mock.ExpectQuery(rowCheck).WithArgs("acme").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // fast path
		mock.ExpectQuery(createdAtQuery).WithArgs("acme").WillReturnRows(sqlmock.NewRows([]string{"created_at"}))     // row

		repo.createWorkspaceDatabase = func(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, row func(ctx context.Context) (database.WorkspaceRow, error), insert database.WorkspaceInsert) error {
			assert.NoError(t, ctx.Err(), "the creation must not inherit the caller's cancellation")
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			assert.WithinDuration(t, time.Now().Add(createWorkspaceTimeout), deadline, 5*time.Second)
			assert.Equal(t, "acme", workspaceID)
			assert.Empty(t, connMgr.forgotten, "the not-found cache is cleared after the insert only")
			found, err := row(ctx)
			require.NoError(t, err)
			assert.Equal(t, database.WorkspaceRowAbsent, found)
			assert.Same(t, repo.systemDB, insert.DB, "the insert runs on the system database")
			assert.Equal(t, []string{"name", "settings", "integrations", "created_at", "updated_at"}, insert.Columns)
			require.Len(t, insert.Values, 5)
			assert.Equal(t, "Acme", insert.Values[0])
			createdAt, ok := insert.Values[3].(time.Time)
			require.True(t, ok)
			assert.Equal(t, createdAt.Truncate(time.Microsecond), createdAt, "stamped to the microsecond")
			assert.Equal(t, createdAt, insert.Values[4])
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, repo.Create(ctx, newTestWorkspace("acme")))
		assert.Equal(t, []string{"acme"}, connMgr.forgotten)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	for name, refusal := range map[string]error{
		"already exists":                domain.ErrWorkspaceAlreadyExists,
		"leftover database with tables": &domain.ErrWorkspaceDatabaseNotEmpty{WorkspaceID: "acme", DatabaseName: "notifuse_ws_acme"},
	} {
		t.Run(name+" comes back unchanged, no row inserted", func(t *testing.T) {
			repo, mock, connMgr := newCreateTestRepo(t)
			mock.ExpectQuery(rowCheck).WithArgs("acme").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			repo.createWorkspaceDatabase = func(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, row func(ctx context.Context) (database.WorkspaceRow, error), insert database.WorkspaceInsert) error {
				return refusal
			}

			err := repo.Create(context.Background(), newTestWorkspace("acme"))
			assert.Same(t, refusal, err)
			assert.Empty(t, connMgr.forgotten)
			assert.NoError(t, mock.ExpectationsWereMet()) // no INSERT expected
		})
	}

	t.Run("the fast-path row check returns ErrWorkspaceAlreadyExists", func(t *testing.T) {
		repo, mock, _ := newCreateTestRepo(t)
		mock.ExpectQuery(rowCheck).WithArgs("acme").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		repo.createWorkspaceDatabase = func(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, row func(ctx context.Context) (database.WorkspaceRow, error), insert database.WorkspaceInsert) error {
			t.Fatal("must not be reached")
			return nil
		}
		assert.ErrorIs(t, repo.Create(context.Background(), newTestWorkspace("acme")), domain.ErrWorkspaceAlreadyExists)
	})

	// The row a creation looks up before its insert, and when its insert found one: its
	// own is told by the creation time Create stamped, to the microsecond PostgreSQL
	// keeps.
	t.Run("reports the row under the ID as absent, its own, or another's", func(t *testing.T) {
		for name, tc := range map[string]struct {
			stored func(ours time.Time) []driver.Value
			want   database.WorkspaceRow
		}{
			"absent":    {stored: func(time.Time) []driver.Value { return nil }, want: database.WorkspaceRowAbsent},
			"its own":   {stored: func(ours time.Time) []driver.Value { return []driver.Value{ours} }, want: database.WorkspaceRowOurs},
			"another's": {stored: func(ours time.Time) []driver.Value { return []driver.Value{ours.Add(-time.Microsecond)} }, want: database.WorkspaceRowOther},
		} {
			t.Run(name, func(t *testing.T) {
				repo, mock, _ := newCreateTestRepo(t)
				ws := newTestWorkspace("acme")
				mock.ExpectQuery(rowCheck).WithArgs("acme").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
				var found database.WorkspaceRow
				repo.createWorkspaceDatabase = func(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, row func(ctx context.Context) (database.WorkspaceRow, error), insert database.WorkspaceInsert) error {
					assert.Equal(t, ws.CreatedAt.Truncate(time.Microsecond), ws.CreatedAt, "stamped to the microsecond")
					rows := sqlmock.NewRows([]string{"created_at"})
					if v := tc.stored(ws.CreatedAt); v != nil {
						rows.AddRow(v...)
					}
					mock.ExpectQuery(createdAtQuery).WithArgs("acme").WillReturnRows(rows)
					var err error
					found, err = row(ctx)
					return err
				}
				require.NoError(t, repo.Create(context.Background(), ws))
				assert.Equal(t, tc.want, found)
				assert.NoError(t, mock.ExpectationsWereMet())
			})
		}
	})

	t.Run("a failed row lookup is an error, not an answer", func(t *testing.T) {
		repo, mock, _ := newCreateTestRepo(t)
		mock.ExpectQuery(rowCheck).WithArgs("acme").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(createdAtQuery).WithArgs("acme").WillReturnError(errors.New("connection reset"))
		repo.createWorkspaceDatabase = func(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, row func(ctx context.Context) (database.WorkspaceRow, error), insert database.WorkspaceInsert) error {
			_, err := row(ctx)
			return err
		}
		assert.ErrorContains(t, repo.Create(context.Background(), newTestWorkspace("acme")), "connection reset")
	})
}

func TestWorkspaceRepository_DeleteUnderNameLock(t *testing.T) {
	const workspaceID = "ws1"
	dropQuery := regexp.QuoteMeta(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pq.QuoteIdentifier("notifuse_ws_ws1")))

	newRepo := func(t *testing.T, rec *workspaceLockRecorder) (*workspaceRepository, sqlmock.Sqlmock, *mockConnectionManager) {
		repo, mock, connMgr := newCreateTestRepo(t)
		repo.workspaceDatabaseLock = func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
			return rec.lock(ctx, cfg, dbName, func(ctx context.Context, admin *sql.DB) error {
				return fn(ctx, admin)
			})
		}
		return repo, mock, connMgr
	}

	t.Run("closes the pool, drops and deletes the rows inside the lock", func(t *testing.T) {
		rec := &workspaceLockRecorder{}
		repo, mock, _ := newRepo(t, rec)
		inLockAt := func() sqlmock.Argument { return lockedArg{rec} }

		mock.ExpectExec(dropQuery).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM user_workspaces WHERE workspace_id = \$1`).WithArgs(inLockAt()).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1`).WithArgs(inLockAt()).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM workspaces WHERE id = \$1`).WithArgs(inLockAt()).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.Delete(context.Background(), workspaceID))
		assert.Equal(t, []string{"notifuse_ws_ws1"}, rec.names)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failed drop deletes no row", func(t *testing.T) {
		rec := &workspaceLockRecorder{}
		repo, mock, _ := newRepo(t, rec)
		mock.ExpectExec(dropQuery).WillReturnError(errors.New("database is being accessed by other users"))

		err := repo.Delete(context.Background(), workspaceID)
		assert.ErrorContains(t, err, "being accessed")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a lock error changes nothing", func(t *testing.T) {
		repo, mock, _ := newCreateTestRepo(t)
		repo.workspaceDatabaseLock = func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
			return errors.New("failed to ping PostgreSQL server")
		}
		err := repo.Delete(context.Background(), workspaceID)
		assert.ErrorContains(t, err, "failed to ping")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an ID that cannot name a database is refused before the lock", func(t *testing.T) {
		rec := &workspaceLockRecorder{}
		repo, _, _ := newRepo(t, rec)
		err := repo.Delete(context.Background(), "a?b")
		assert.ErrorContains(t, err, "invalid workspace ID")
		assert.Empty(t, rec.names)
	})

	// The bound of the deletion covers the wait for the lock: the drop and the row
	// deletion that must follow it run detached from it, even when the wait used up the
	// caller's. How long they get under the lock (until shortly before its bound, which
	// counts from the grant) is pinned by the database package's tests.
	t.Run("the work under the lock is not cut short by a long wait", func(t *testing.T) {
		repo, mock, _ := newCreateTestRepo(t)
		repo.workspaceDatabaseLock = func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
			expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer cancel()
			return fn(expired, nil)
		}
		mock.ExpectExec(dropQuery).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM user_workspaces WHERE workspace_id = \$1`).WithArgs(workspaceID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1`).WithArgs(workspaceID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM workspaces WHERE id = \$1`).WithArgs(workspaceID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.Delete(context.Background(), workspaceID))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("runs on a detached context", func(t *testing.T) {
		repo, mock, _ := newCreateTestRepo(t)
		repo.workspaceDatabaseLock = func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
			assert.NoError(t, ctx.Err())
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			assert.WithinDuration(t, time.Now().Add(deleteWorkspaceTimeout), deadline, 5*time.Second)
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, repo.Delete(ctx, workspaceID))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// lockedArg matches the workspace ID argument only while the name lock is held, so a
// row deletion moved out of the lock fails its expectation.
type lockedArg struct{ rec *workspaceLockRecorder }

func (a lockedArg) Match(v driver.Value) bool {
	return a.rec.inLock && v == "ws1"
}
