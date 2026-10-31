package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/lib/pq"
)

// workspaceIDInDatabaseName matches the characters allowed in the workspace ID part of a
// database name, capitals included (refused next, with their own message). The ID is
// written into DSNs and SQL as is. Workspace IDs have at most 20 characters; the bound only
// keeps an arbitrary input from going further.
var workspaceIDInDatabaseName = regexp.MustCompile(`^[A-Za-z0-9_]{1,63}$`)

// WorkspaceDatabaseName returns the name of a workspace's database, or an error when the
// workspace ID cannot be part of one. Every place that names a workspace database goes
// through it, so the name a database is created under is the one it is opened and
// dropped under.
func WorkspaceDatabaseName(prefix, workspaceID string) (string, error) {
	// Replace hyphens with underscores for PostgreSQL compatibility
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	// Characters first, case aside, so that an ID refused for its characters is reported
	// quoted whatever its case.
	if !workspaceIDInDatabaseName.MatchString(safeID) {
		return "", fmt.Errorf("invalid workspace ID for a database name: %q", workspaceID)
	}
	dbName := fmt.Sprintf("%s_ws_%s", prefix, safeID)

	// PostgreSQL folds the ASCII capitals of an unquoted name to lowercase, so a name with
	// one would be created and opened under different names by tools that do not quote it.
	if strings.ContainsAny(dbName, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return "", fmt.Errorf("workspace database name must be lowercase: %s", dbName)
	}
	return dbName, nil
}

// GetConnectionPoolSettings returns connection pool settings based on environment
func GetConnectionPoolSettings() (maxOpen, maxIdle int, maxLifetime time.Duration) {
	environment := os.Getenv("ENVIRONMENT")

	// Use smaller pools for test environment to conserve connections
	if environment == "test" || os.Getenv("INTEGRATION_TESTS") == "true" {
		return 10, 5, 2 * time.Minute
	}

	// Production settings
	return 25, 25, 20 * time.Minute
}

// GetSystemDSN returns the DSN for the system database
func GetSystemDSN(cfg *config.DatabaseConfig) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)
}

// GetPostgresDSN returns the DSN for connecting to PostgreSQL server without specifying a database
func GetPostgresDSN(cfg *config.DatabaseConfig) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.SSLMode,
	)
}

// GetWorkspaceDSN returns the DSN for a workspace database
func GetWorkspaceDSN(cfg *config.DatabaseConfig, workspaceID string) (string, error) {
	dbName, err := WorkspaceDatabaseName(cfg.Prefix, workspaceID)
	if err != nil {
		return "", err
	}
	return workspaceDSNForName(cfg, dbName), nil
}

// workspaceDSNForName returns the DSN for a database name WorkspaceDatabaseName returned.
func workspaceDSNForName(cfg *config.DatabaseConfig, dbName string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		dbName,
		cfg.SSLMode,
	)
}

// ConnectToWorkspace creates a new database connection for a workspace
func ConnectToWorkspace(cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	// Ensure the workspace database exists
	if err := EnsureWorkspaceDatabaseExists(context.Background(), cfg, workspaceID, nil); err != nil {
		return nil, fmt.Errorf("failed to ensure workspace database exists: %w", err)
	}

	dsn, err := GetWorkspaceDSN(cfg, workspaceID)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping workspace database: %w", err)
	}

	// Set connection pool settings based on environment
	maxOpen, maxIdle, maxLifetime := GetConnectionPoolSettings()
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)
	db.SetConnMaxIdleTime(maxLifetime / 2)

	return db, nil
}

// workspaceDatabaseProvisioner holds what creating and initialising a workspace
// database depends on, so tests can inject failures.
type workspaceDatabaseProvisioner struct {
	lock          func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error
	openWorkspace func(ctx context.Context, cfg *config.DatabaseConfig, dbName string) (*sql.DB, error)
	initialize    func(ctx context.Context, db *sql.DB) error
	holdsTables   func(ctx context.Context, db *sql.DB) (bool, error)
}

var defaultWorkspaceDatabaseProvisioner = workspaceDatabaseProvisioner{
	lock:          WithWorkspaceDatabaseLock,
	openWorkspace: openWorkspaceDatabase,
	initialize:    initializeWorkspaceDatabase,
	holdsTables:   WorkspaceDatabaseHoldsTables,
}

// lockWorkspaceDSN is the workspace DSN of the paths that take the name lock, with a
// bound on the whole connection establishment.
func lockWorkspaceDSN(cfg *config.DatabaseConfig, dbName string) string {
	return WithConnectTimeout(workspaceDSNForName(cfg, dbName))
}

// openWorkspaceDatabase opens and pings a workspace database for its creation or
// initialisation.
func openWorkspaceDatabase(ctx context.Context, cfg *config.DatabaseConfig, dbName string) (*sql.DB, error) {
	db, err := sql.Open("postgres", lockWorkspaceDSN(cfg, dbName))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace database %s: %w", dbName, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping workspace database %s: %w", dbName, err)
	}
	return db, nil
}

// EnsureWorkspaceDatabaseExists makes sure the database of a registered workspace exists
// and holds the workspace schema, under the workspace's name lock: it creates and
// initialises a missing database, initialises an existing one that holds no tables,
// and leaves one that holds tables as it is.
//
// stillRegistered, when not nil, runs first inside the lock, and its error stops
// everything before any query on the database: a request that waited while the
// workspace was deleted then creates nothing. A database this call created is dropped
// again when its setup fails; no other database is ever dropped.
func EnsureWorkspaceDatabaseExists(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, stillRegistered func(ctx context.Context) error) error {
	return defaultWorkspaceDatabaseProvisioner.ensure(ctx, cfg, workspaceID, stillRegistered)
}

func (p workspaceDatabaseProvisioner) ensure(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, stillRegistered func(ctx context.Context) error) error {
	dbName, err := WorkspaceDatabaseName(cfg.Prefix, workspaceID)
	if err != nil {
		return err
	}

	return p.lock(ctx, cfg, dbName, func(ctx context.Context, admin *sql.DB) error {
		if stillRegistered != nil {
			if err := stillRegistered(ctx); err != nil {
				return err
			}
		}

		// Only callers for registered workspaces reach this, so giving an empty
		// database its schema is a repair. A caller that waited while another
		// initialised it finds tables and does nothing.
		return p.ensureSchema(ctx, cfg, admin, dbName)
	})
}

// ensureSchema makes sure the database of a registered workspace exists with its
// schema: a missing one is created and initialised, an empty one initialised, one that
// holds tables left as it is.
func (p workspaceDatabaseProvisioner) ensureSchema(ctx context.Context, cfg *config.DatabaseConfig, admin *sql.DB, dbName string) error {
	exists, err := WorkspaceDatabaseExists(ctx, admin, dbName)
	if err != nil {
		return err
	}
	if !exists {
		return p.createAndInitialize(ctx, cfg, admin, dbName)
	}
	_, err = p.initializeUnlessHoldsTables(ctx, cfg, dbName)
	return err
}

// WorkspaceRow is what a workspace creation finds registered under its workspace ID.
type WorkspaceRow int

const (
	// WorkspaceRowAbsent: no workspace has the ID.
	WorkspaceRowAbsent WorkspaceRow = iota
	// WorkspaceRowOurs: the row is the one this creation inserted.
	WorkspaceRowOurs
	// WorkspaceRowOther: another creation's row has the ID.
	WorkspaceRowOther
)

// errInsertNotSent marks an insert error raised before the statement reached the
// server, such as failing to get a connection: that attempt certainly changed nothing.
var errInsertNotSent = errors.New("insert not sent")

// WorkspaceInsert registers a workspace on the system database: Values go into the
// workspaces columns of the same names, and id is set to the workspace ID. It always
// runs as INSERT ... ON CONFLICT (id) DO NOTHING, which waits for an insert of the same
// ID still in progress and never fails on the ID: how CreateWorkspaceDatabase reads a
// refusal depends on it.
type WorkspaceInsert struct {
	DB      *sql.DB
	Columns []string
	Values  []any
}

var workspaceColumnName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// prepare returns what runs the insert of workspaceID's row once.
func (w WorkspaceInsert) prepare(workspaceID string) (func(ctx context.Context) (bool, error), error) {
	if w.DB == nil {
		return nil, errors.New("invalid workspace insert: no database")
	}
	if len(w.Columns) != len(w.Values) {
		return nil, fmt.Errorf("invalid workspace insert: %d columns, %d values", len(w.Columns), len(w.Values))
	}
	columns := []string{pq.QuoteIdentifier("id")}
	params := []string{"$1"}
	seen := map[string]bool{}
	for i, c := range w.Columns {
		if strings.ToLower(c) == "id" {
			return nil, fmt.Errorf("invalid workspace insert: column %s, set from the workspace ID", c)
		}
		// The workspaces columns are plain lower-case names; a quoted name in another
		// case would be another column.
		if !workspaceColumnName.MatchString(c) {
			return nil, fmt.Errorf("invalid workspace insert: column %q is not a lower-case name", c)
		}
		if seen[c] {
			return nil, fmt.Errorf("invalid workspace insert: column %s set twice", c)
		}
		seen[c] = true
		columns = append(columns, pq.QuoteIdentifier(c))
		params = append(params, fmt.Sprintf("$%d", i+2))
	}
	query := fmt.Sprintf("INSERT INTO workspaces (%s) VALUES (%s) ON CONFLICT (id) DO NOTHING",
		strings.Join(columns, ", "), strings.Join(params, ", "))
	args := append([]any{workspaceID}, w.Values...)
	return func(ctx context.Context) (bool, error) { return w.run(ctx, query, args) }, nil
}

// run runs query once, on a live connection of its own: on *sql.DB, database/sql runs a
// statement again on a fresh connection after a lost one, which would hide whether it
// was sent. It reports whether it inserted the row.
func (w WorkspaceInsert) run(ctx context.Context, query string, args []any) (bool, error) {
	conn, err := liveConnection(ctx, w.DB)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errInsertNotSent, err)
	}
	defer func() { _ = conn.Close() }()
	result, err := conn.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	return inserted == 1, err
}

// CreateWorkspaceDatabase creates the database of a new workspace and registers the
// workspace, under the workspace's name lock. row reports the workspace row under the
// ID; insert registers it, and is run again after a failure until its outcome is known.
//
// A registered workspace is refused with domain.ErrWorkspaceAlreadyExists, nothing
// touched. A missing database is created and initialised; an existing one that holds
// no tables (left by a failed creation) is initialised; one that holds tables is
// refused with *domain.ErrWorkspaceDatabaseNotEmpty, unchanged. When insert finds a
// row, this creation's own (an earlier attempt that committed) is a success, and
// another's is refused with domain.ErrWorkspaceAlreadyExists, the database left to that
// workspace. The database, which holds nothing yet, is dropped when no attempt can have
// committed; when one may have, it is kept and named in the error. After a retried
// insert registered the workspace, its row is looked up again: another creation's is
// refused with domain.ErrWorkspaceAlreadyExists, and one removed since with an error,
// its database dropped; this creation's has its database checked again and recreated
// or initialised as needed, a failure there being logged and nil returned.
func CreateWorkspaceDatabase(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, row func(ctx context.Context) (WorkspaceRow, error), insert WorkspaceInsert) error {
	run, err := insert.prepare(workspaceID)
	if err != nil {
		return err
	}
	return defaultWorkspaceDatabaseProvisioner.create(ctx, cfg, workspaceID, row, run)
}

func (p workspaceDatabaseProvisioner) create(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string, row func(ctx context.Context) (WorkspaceRow, error), insert func(ctx context.Context) (bool, error)) error {
	dbName, err := WorkspaceDatabaseName(cfg.Prefix, workspaceID)
	if err != nil {
		return err
	}

	return p.lock(ctx, cfg, dbName, func(ctx context.Context, admin *sql.DB) error {
		found, err := row(ctx)
		if err != nil {
			return fmt.Errorf("failed to check workspace %s: %w", workspaceID, err)
		}
		if found != WorkspaceRowAbsent {
			return domain.ErrWorkspaceAlreadyExists
		}

		exists, err := WorkspaceDatabaseExists(ctx, admin, dbName)
		if err != nil {
			return err
		}
		if exists {
			holdsTables, err := p.initializeUnlessHoldsTables(ctx, cfg, dbName)
			if err != nil {
				return err
			}
			if holdsTables {
				return &domain.ErrWorkspaceDatabaseNotEmpty{WorkspaceID: workspaceID, DatabaseName: dbName}
			}
		} else if err := p.createAndInitialize(ctx, cfg, admin, dbName); err != nil {
			return err
		}

		// What follows the insert gets bounds of its own: the insert may have ended on
		// ctx's deadline, and its retries on theirs.
		settled, insertErr := settleInsert(ctx, workspaceID, row, insert)
		switch settled.outcome {
		case insertOurs:
			if settled.retried {
				// Ending after the retries and before a drop after a failed setup, so
				// each keeps time of its own.
				checkCtx, cancelCheck := detachedBefore(ctx, dropDatabaseTimeout, checkHoldMargin)
				defer cancelCheck()
				return p.ensureAfterRetry(checkCtx, cfg, admin, workspaceID, dbName, row)
			}
			return nil
		case insertOther:
			// Another writer's row: one that does not take the name lock (an instance of
			// an earlier release during a rolling deploy) registered the ID meanwhile.
			// The database is now that workspace's: nothing is dropped.
			return domain.ErrWorkspaceAlreadyExists
		case insertNotRegistered:
			// No attempt committed. Created or adopted empty, the database holds nothing
			// yet.
			cleanupCtx, cancel := cleanupContext(ctx)
			defer cancel()
			if dropErr := dropWorkspaceDatabase(cleanupCtx, admin, dbName); dropErr != nil {
				return fmt.Errorf("%w; and workspace database %s could not be dropped: %v", insertErr, dbName, dropErr)
			}
			return insertErr
		default:
			// An attempt may have committed: dropping could leave a registered workspace
			// without its database, so it is not dropped.
			return fmt.Errorf("could not tell whether workspace %s was registered, so its database %s was not dropped: %w", workspaceID, dbName, insertErr)
		}
	})
}

// ensureAfterRetry makes sure a workspace registered by a retried insert keeps a
// database with its schema: during the pauses, an instance that takes no name lock may
// have registered and removed the ID, dropping the database first, and one may have
// recreated it meanwhile. It looks the row up first. A row that is now another
// creation's is refused with domain.ErrWorkspaceAlreadyExists, its database left to it;
// a row removed since fails the creation, and the database this creation made, which no
// workspace has now, is dropped. Once the row is this creation's, or cannot be looked up
// (most likely still its own, and a failure would leave it without an owner), a failure
// is only logged: the workspace is registered.
func (p workspaceDatabaseProvisioner) ensureAfterRetry(ctx context.Context, cfg *config.DatabaseConfig, admin *sql.DB, workspaceID, dbName string, row func(ctx context.Context) (WorkspaceRow, error)) error {
	found, err := row(ctx)
	if err != nil {
		logRetryCheckError(workspaceID, dbName, "Workspace registration retried, but its row could not be looked up again: check it is this creation's and its database holds the workspace tables", err)
		return nil
	}
	switch found {
	case WorkspaceRowOurs:
		if err := p.ensureSchema(ctx, cfg, admin, dbName); err != nil {
			logRetryCheckError(workspaceID, dbName, "Workspace registered, but its database could not be checked after its registration was retried: make sure it exists and holds the workspace tables", err)
		}
		return nil
	case WorkspaceRowOther:
		return domain.ErrWorkspaceAlreadyExists
	case WorkspaceRowAbsent:
		removed := fmt.Errorf("workspace %s was removed while it was being created", workspaceID)
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		if dropErr := dropWorkspaceDatabase(cleanupCtx, admin, dbName); dropErr != nil {
			return fmt.Errorf("%w; and workspace database %s could not be dropped: %v", removed, dbName, dropErr)
		}
		return removed
	default:
		return fmt.Errorf("unexpected answer %d looking up workspace %s after its insert", found, workspaceID)
	}
}

// logRetryCheckError reports what went wrong checking a workspace after its
// registration was retried. A variable for tests.
var logRetryCheckError = func(workspaceID, dbName, message string, err error) {
	logger.NewLogger().WithField("workspace", workspaceID).WithField("database", dbName).WithField("error", err.Error()).Error(message)
}

type insertOutcome int

const (
	insertUnknown insertOutcome = iota
	insertOurs
	insertOther
	insertNotRegistered
)

type settledInsert struct {
	outcome insertOutcome
	// retried: the attempt that settled it was not the first.
	retried bool
}

// insertRetryTimeout bounds the retries of an insert, which run detached from the
// caller: the first attempt may have failed on its deadline. They end with the work
// under the lock at the latest, which leaves what follows them time of its own. A
// variable for tests.
var insertRetryTimeout = 2 * time.Minute

// checkHoldMargin: the check after a retried insert ends that long before the lock's
// hold bound, between the end of the work and its retries (lockWorkMargin) and the end
// of a drop after a failed setup (cleanupHoldMargin).
const checkHoldMargin = (lockWorkMargin + cleanupHoldMargin) / 2

// insertRetryDelays are the pauses before each insert attempt after the first: long
// enough together to ride out a failover of the system database. A variable for tests.
var insertRetryDelays = []time.Duration{2 * time.Second, 8 * time.Second}

// settleInsert inserts a workspace's row and tells how it ended: this creation's row
// (inserted now, or found from an earlier attempt that committed), another writer's,
// not registered (no attempt can have committed), or unknown. After a failure the
// insert runs again, which waits for an attempt still in progress. Everything after the
// first insert, its row lookup included, runs on a bound of its own,
// insertRetryTimeout, since the first may have ended on the caller's deadline. The
// error holds every failure met.
func settleInsert(ctx context.Context, workspaceID string, row func(ctx context.Context) (WorkspaceRow, error), insert func(ctx context.Context) (bool, error)) (settledInsert, error) {
	var failures error
	fail := func(err error) { failures = thenErr(failures, err) }
	var settled settledInsert
	// mayHaveCommitted: an attempt reached the server and its outcome is unknown.
	// sawRow: an attempt found a row under the ID.
	mayHaveCommitted, sawRow := false, false
	end := func() (settledInsert, error) {
		if mayHaveCommitted || sawRow {
			settled.outcome = insertUnknown
		} else {
			settled.outcome = insertNotRegistered
		}
		return settled, failures
	}

	attemptCtx := ctx
	for attempt := 0; ; attempt++ {
		settled.retried = attempt > 0
		inserted, err := insert(attemptCtx)
		if err == nil && inserted {
			settled.outcome = insertOurs
			return settled, nil
		}
		if attempt == 0 {
			retryCtx, cancel := detachedBefore(ctx, insertRetryTimeout, lockWorkMargin)
			defer cancel()
			attemptCtx = retryCtx
		}
		switch {
		case err == nil:
			sawRow = true
			found, lookupErr := row(attemptCtx)
			if lookupErr != nil {
				fail(fmt.Errorf("failed to check workspace %s after its insert found a row: %w", workspaceID, lookupErr))
				break
			}
			switch found {
			case WorkspaceRowOurs:
				settled.outcome = insertOurs
				return settled, nil
			case WorkspaceRowOther:
				settled.outcome = insertOther
				return settled, nil
			case WorkspaceRowAbsent:
				// The row went between the insert and its lookup: inserting again tells.
				fail(fmt.Errorf("the row of workspace %s went between its insert and its lookup", workspaceID))
			default:
				fail(fmt.Errorf("unexpected answer %d looking up workspace %s", found, workspaceID))
				return settled, failures
			}
		case errors.Is(err, errInsertNotSent):
			fail(err)
		case refusedByServer(err):
			fail(err)
			if insertCanNeverSucceed(err) {
				// Every attempt fails the same way: none committed, unless one found a
				// row, which contradicts it.
				mayHaveCommitted = false
				return end()
			}
		default:
			fail(err)
			mayHaveCommitted = true
		}

		if attempt == len(insertRetryDelays) {
			return end()
		}
		select {
		case <-time.After(insertRetryDelays[attempt]):
		case <-attemptCtx.Done():
			fail(fmt.Errorf("no time left to insert again: %w", attemptCtx.Err()))
			return end()
		}
	}
}

// thenErr chains err after earlier, on one line for the logs; earlier may be nil.
func thenErr(earlier, err error) error {
	if earlier == nil {
		return err
	}
	return fmt.Errorf("%w; then %w", earlier, err)
}

// refusedByServer reports whether err is the server refusing a statement. On a
// connection of its own, lib/pq returns a *pq.Error only for that (a FATAL one ends the
// connection and comes back as a lost connection), so the attempt changed nothing.
func refusedByServer(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr)
}

// insertCanNeverSucceed reports whether err, a refused insert, refuses it for its values
// (a data exception, or an integrity constraint): every attempt of the same insert with
// the same values fails the same way, so an earlier one that may have committed did
// not. A refusal for the statement itself (class 42: a missing privilege or column) can
// change between attempts, so it is not one.
func insertCanNeverSucceed(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || len(pqErr.Code) < 2 {
		return false
	}
	switch pqErr.Code.Class() {
	case "22", "23":
		return true
	}
	return false
}

// IsDatabaseMissingError reports whether err is PostgreSQL's invalid_catalog_name
// (3D000): the database a connection names does not exist.
func IsDatabaseMissingError(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "3D000"
}

// WorkspaceDatabaseExists reports whether a database named dbName exists, through a
// connection to any database of the server.
func WorkspaceDatabaseExists(ctx context.Context, db *sql.DB, dbName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check whether database %s exists: %w", dbName, err)
	}
	return exists, nil
}

// createAndInitialize creates dbName and gives it the workspace schema, without the
// emptiness check: a database copied from a template1 that holds a user table must
// still get its schema, and re-running over a complete schema is harmless. When the
// setup fails, the database it created is dropped again.
func (p workspaceDatabaseProvisioner) createAndInitialize(ctx context.Context, cfg *config.DatabaseConfig, admin *sql.DB, dbName string) error {
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(dbName)); err != nil {
		return fmt.Errorf("failed to create workspace database: %w", err)
	}

	setupErr := p.setUpNewDatabase(ctx, cfg, dbName)
	if setupErr == nil {
		return nil
	}

	// The workspace connection is closed by now. The drop runs on its own bound: the
	// setup may have failed on the caller's deadline.
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	if dropErr := dropWorkspaceDatabase(cleanupCtx, admin, dbName); dropErr != nil {
		return fmt.Errorf("%w; and the workspace database %s it created could not be dropped, left empty: %v", setupErr, dbName, dropErr)
	}
	return setupErr
}

func (p workspaceDatabaseProvisioner) setUpNewDatabase(ctx context.Context, cfg *config.DatabaseConfig, dbName string) error {
	wsDB, err := p.openWorkspace(ctx, cfg, dbName)
	if err != nil {
		return fmt.Errorf("failed to connect to new workspace database: %w", err)
	}
	defer func() { _ = wsDB.Close() }()

	if err := p.initialize(ctx, wsDB); err != nil {
		return fmt.Errorf("failed to initialize workspace database schema: %w", err)
	}
	return nil
}

// initializeUnlessHoldsTables gives an existing database that holds no tables the
// workspace schema, and reports whether it already held tables.
func (p workspaceDatabaseProvisioner) initializeUnlessHoldsTables(ctx context.Context, cfg *config.DatabaseConfig, dbName string) (bool, error) {
	wsDB, err := p.openWorkspace(ctx, cfg, dbName)
	if err != nil {
		return false, fmt.Errorf("failed to connect to workspace database: %w", err)
	}
	defer func() { _ = wsDB.Close() }()

	holdsTables, err := p.holdsTables(ctx, wsDB)
	if err != nil {
		return false, err
	}
	if holdsTables {
		return true, nil
	}
	if err := p.initialize(ctx, wsDB); err != nil {
		return false, fmt.Errorf("failed to initialize workspace database schema: %w", err)
	}
	return false, nil
}

// dropWorkspaceDatabase drops dbName, terminating its remaining backends. Only for a
// database the caller created or adopted empty under the name lock it holds.
func dropWorkspaceDatabase(ctx context.Context, admin *sql.DB, dbName string) error {
	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pq.QuoteIdentifier(dbName)+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("failed to drop workspace database %s: %w", dbName, err)
	}
	return nil
}

// EnsureSystemDatabaseExists creates the system database if it doesn't exist
func EnsureSystemDatabaseExists(dsn string, dbName string) error {
	// Connect to PostgreSQL server without specifying a database
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL server: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Test the connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL server: %w", err)
	}

	// Check if database exists
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err = db.QueryRow(query, dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if database exists: %w", err)
	}

	// Create database if it doesn't exist
	if !exists {
		// Use fmt.Sprintf for proper quoting of identifiers in SQL
		createDBQuery := fmt.Sprintf("CREATE DATABASE %s",
			// Proper quoting to prevent SQL injection
			strings.ReplaceAll(dbName, `"`, `""`))

		_, err = db.Exec(createDBQuery)
		if err != nil {
			return fmt.Errorf("failed to create system database: %w", err)
		}
	}

	return nil
}
