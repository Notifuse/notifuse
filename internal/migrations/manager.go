package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/lib/pq"
)

// ErrRestartRequired is returned when a migration requires a server restart
var ErrRestartRequired = errors.New("migration completed successfully - server restart required")

// ErrDatabaseAheadOfCode is returned when the database carries a version stamp
// higher than the running build's, i.e. it was migrated by a newer build.
var ErrDatabaseAheadOfCode = errors.New("database was migrated by a newer build")

// workspaceConnector interface for connecting to workspace databases
type workspaceConnector interface {
	connectToWorkspace(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error)
}

// workspaceDatabaseLock takes a workspace database's name lock (see
// database.WithWorkspaceDatabaseLock).
type workspaceDatabaseLock func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error

// Manager implements MigrationManager
type Manager struct {
	logger    logger.Logger
	connector workspaceConnector
	// workspaceLock is the name lock a workspace whose database is empty or missing, or
	// whose probe a deletion may have cut, is looked at again under; nil means
	// database.WithWorkspaceDatabaseLock.
	workspaceLock workspaceDatabaseLock
	// migrationLock and sharedMigrationLock take the migration lock across instances;
	// nil means database.WithMigrationLock and database.WithSharedMigrationLock.
	migrationLock       func(ctx context.Context, db *sql.DB, waiting func(holders []database.MigrationLockHolder, err error), fn func(ctx context.Context) error) error
	sharedMigrationLock func(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error
	// initializeWorkspace gives an empty workspace database its schema; nil means
	// database.InitializeWorkspaceDatabaseContext.
	initializeWorkspace func(ctx context.Context, db *sql.DB) error
	// repairCheckTimeout bounds connecting to a workspace database and checking its
	// tables during the repair; repairInitTimeout bounds an initialisation. Zero means
	// the defaults below.
	repairCheckTimeout time.Duration
	repairInitTimeout  time.Duration
}

const (
	defaultRepairCheckTimeout = 10 * time.Second
	defaultRepairInitTimeout  = 5 * time.Minute
)

// workspaceMigrationDSN is the DSN migrations connect to a workspace database with,
// bounding the whole connection establishment (see database.WithConnectTimeout).
func workspaceMigrationDSN(cfg *config.DatabaseConfig, workspaceID string) (string, error) {
	dsn, err := database.GetWorkspaceDSN(cfg, workspaceID)
	if err != nil {
		return "", err
	}
	return database.WithConnectTimeout(dsn), nil
}

// defaultConnector implements workspaceConnector
type defaultConnector struct{}

func (c *defaultConnector) connectToWorkspace(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	dsn, err := workspaceMigrationDSN(cfg, workspaceID)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace database: %w", err)
	}

	// Test the connection. The pq error stays wrapped: a missing database (3D000) is
	// told apart from other failures.
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping workspace database: %w", err)
	}

	return db, nil
}

// NewManager creates a new migration manager
func NewManager(logger logger.Logger) *Manager {
	return &Manager{
		logger:    logger,
		connector: &defaultConnector{},
	}
}

func (m *Manager) lockWorkspaceDatabase() workspaceDatabaseLock {
	if m.workspaceLock != nil {
		return m.workspaceLock
	}
	return database.WithWorkspaceDatabaseLock
}

func (m *Manager) lockMigrations() func(ctx context.Context, db *sql.DB, waiting func(holders []database.MigrationLockHolder, err error), fn func(ctx context.Context) error) error {
	if m.migrationLock != nil {
		return m.migrationLock
	}
	return database.WithMigrationLock
}

func (m *Manager) shareMigrationLock() func(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
	if m.sharedMigrationLock != nil {
		return m.sharedMigrationLock
	}
	return database.WithSharedMigrationLock
}

func (m *Manager) initializer() func(ctx context.Context, db *sql.DB) error {
	if m.initializeWorkspace != nil {
		return m.initializeWorkspace
	}
	return database.InitializeWorkspaceDatabaseContext
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

// GetCurrentDBVersion retrieves the current database version from settings table
func (m *Manager) GetCurrentDBVersion(ctx context.Context, db *sql.DB) (float64, error, bool) {
	var versionStr string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'db_version'").Scan(&versionStr)
	if err != nil {
		if err == sql.ErrNoRows {
			// No version found
			return 0, nil, false
		}
		return 0, fmt.Errorf("failed to get current database version: %w", err), false
	}

	// Parse as integer since we only store major version
	version, err := strconv.ParseFloat(versionStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid database version format '%s': %w", versionStr, err), false
	}

	return version, nil, true
}

// SetCurrentDBVersion updates the current database version in settings table
func (m *Manager) SetCurrentDBVersion(ctx context.Context, db *sql.DB, version float64) error {
	// Store only the major version as an integer
	versionStr := fmt.Sprintf("%.0f", version)

	_, err := db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES ('db_version', $1)
		ON CONFLICT (key) DO UPDATE SET
			value = $1,
			updated_at = CURRENT_TIMESTAMP
	`, versionStr)

	if err != nil {
		return fmt.Errorf("failed to set database version to %s: %w", versionStr, err)
	}

	m.logger.WithField("version", versionStr).Info("Database version updated")
	return nil
}

// RunMigrations executes all necessary migrations based on version comparison, holding
// the migration lock across instances from before the version is read under it until it
// returns,
// ErrRestartRequired included: an instance starting while another migrates waits for it
// without a time limit (a migration is never interrupted, and an instance must not
// serve before its migrations are applied), then finds the version current. A holder
// lost with its node is released once the server notices the dead connection, within
// about a minute with the lock's keepalives. Instances of 42.2 and older take no lock.
//
// The version is also read before the lock is tried. A database already ahead of the
// code is refused there, without waiting. When the version read under the lock is
// higher, another instance migrated in between, after this instance loaded its
// configuration: RunMigrations returns ErrRestartRequired when one of those migrations
// requires a restart or may (see mayRestartServer).
func (m *Manager) RunMigrations(ctx context.Context, cfg *config.Config, db *sql.DB) error {
	m.logger.Info("Starting migration process")
	// Read before trying the lock: a holder finishing between a refused try and a read
	// made then would hide its migrations.
	before, err, beforeFound := m.GetCurrentDBVersion(ctx, db)
	if err != nil {
		return err
	}
	codeVersion, err := GetCurrentCodeVersion()
	if err != nil {
		return fmt.Errorf("failed to get current code version: %w", err)
	}
	if beforeFound && before > codeVersion {
		return databaseAheadOfCodeError(before, codeVersion)
	}
	return m.lockMigrations()(ctx, db, m.logMigrationLockWait, func(ctx context.Context) error {
		return m.runMigrations(ctx, cfg, db, before, beforeFound)
	})
}

// mayRestartServer is implemented by a migration whose ShouldRestartServer depends on
// what its own run found (V36): an instance that did not run it cannot tell, and
// restarts whenever it may be needed.
type mayRestartServer interface {
	MayRestartServer() bool
}

// restartMigrationBetween returns the first registered migration in (from, to] that
// requires a restart or may, or 0.
func restartMigrationBetween(from, to float64) float64 {
	for _, migration := range GetRegisteredMigrations() {
		version := migration.GetMajorVersion()
		if version <= from || version > to {
			continue
		}
		if may, ok := migration.(mayRestartServer); migration.ShouldRestartServer() || (ok && may.MayRestartServer()) {
			return version
		}
	}
	return 0
}

func databaseAheadOfCodeError(dbVersion, codeVersion float64) error {
	return fmt.Errorf("%w: database is at version %.0f, this build is version %.0f - refusing to start; deploy a build at version %.0f or newer, or restore a database stamped %.0f or lower",
		ErrDatabaseAheadOfCode, dbVersion, codeVersion, dbVersion, codeVersion)
}

// logMigrationLockWait reports who holds the migration lock this instance waits for.
func (m *Manager) logMigrationLockWait(holders []database.MigrationLockHolder, err error) {
	log := m.logger
	if err != nil {
		log = log.WithField("error", err.Error())
	}
	if len(holders) > 0 {
		described := make([]string, len(holders))
		for i, h := range holders {
			described[i] = fmt.Sprintf("pid %d", h.PID)
			if h.Shared {
				described[i] += " repairing an empty workspace database"
			} else {
				described[i] += " running migrations"
			}
			if h.ApplicationName != "" {
				described[i] += " (" + h.ApplicationName + ")"
			}
			if h.ClientAddr != "" {
				described[i] += " from " + h.ClientAddr
			}
		}
		log = log.WithField("holders", strings.Join(described, ", "))
	}
	log.Warn("Another instance holds the migration lock: waiting for it, without a time limit")
}

// runMigrations runs under the migration lock; before is the version read before it was
// tried, when beforeFound.
func (m *Manager) runMigrations(ctx context.Context, cfg *config.Config, db *sql.DB, before float64, beforeFound bool) error {
	// Get current versions
	currentDBVersion, err, versionExists := m.GetCurrentDBVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to get current database version: %w", err)
	}

	currentCodeVersion, err := GetCurrentCodeVersion()
	if err != nil {
		return fmt.Errorf("failed to get current code version: %w", err)
	}

	// If no version exists in database, this is the first run - initialize it
	if !versionExists {
		m.logger.WithField("code_version", fmt.Sprintf("%.0f", currentCodeVersion)).Info("First run detected, initializing database version")
		if err := m.SetCurrentDBVersion(ctx, db, currentCodeVersion); err != nil {
			return fmt.Errorf("failed to initialize database version: %w", err)
		}
		m.logger.Info("Database version initialized successfully")
		return nil
	}

	m.logger.WithField("db_version", fmt.Sprintf("%.0f", currentDBVersion)).
		WithField("code_version", fmt.Sprintf("%.0f", currentCodeVersion)).
		Info("Version comparison")

	// The database is stamped HIGHER than this build. Deploys go backwards more
	// often than anyone plans for: a rollback to the previous image, a standby
	// promoted while it still carries the newer schema, a dump restored into an
	// older install. This used to fall into the >= check below and be reported
	// "up to date", which is the one answer that is certainly wrong — the newer
	// build's migrations have run here and this build does not contain them, so
	// it would serve traffic against a schema it was never compiled against.
	//
	// And the damage outlives the mistake. The stamp is a single scalar and the
	// dispatcher below only ever selects migrationVersion > currentDBVersion, so
	// every migration between the two numbers is skipped forever — including
	// once the code catches up, at which point the gap is invisible and the
	// database reports itself fully migrated while missing schema it never ran.
	//
	// Fatal rather than a logged refusal, deliberately: this error reaches
	// app.InitDB -> app.Initialize -> runServer, which calls logger.Fatal, so
	// the process refuses to start. There is no down-migration to recover with,
	// which means the only fixes are human — redeploy the newer build, or
	// restore a database matching this one — and a server that booted anyway
	// would be writing to the mismatched schema for as long as the operator
	// takes to find them. Refusing to start costs an outage that is already
	// underway; booting costs data nobody can un-write.
	if currentDBVersion > currentCodeVersion {
		return databaseAheadOfCodeError(currentDBVersion, currentCodeVersion)
	}

	// Another instance migrated between the read before the lock and now.
	if beforeFound && currentDBVersion > before {
		if version := restartMigrationBetween(before, currentDBVersion); version != 0 {
			return fmt.Errorf("%w: another instance migrated the database from version %.0f to %.0f, across version %.0f, which can require one",
				ErrRestartRequired, before, currentDBVersion, version)
		}
	}

	// Check if migrations are needed
	if currentDBVersion == currentCodeVersion {
		m.logger.Info("Database is up to date, no migrations needed")
		return nil
	}

	// Get all registered migrations
	registeredMigrations := GetRegisteredMigrations()

	// Find migrations that need to be executed
	var migrationsToRun []MajorMigrationInterface
	for _, migration := range registeredMigrations {
		migrationVersion := migration.GetMajorVersion()
		if migrationVersion > currentDBVersion && migrationVersion <= currentCodeVersion {
			migrationsToRun = append(migrationsToRun, migration)
		}
	}

	if len(migrationsToRun) == 0 {
		m.logger.Info("No migrations to run")
		return nil
	}

	m.logger.WithField("count", len(migrationsToRun)).Info("Migrations to execute")

	// Track if any migration requires a restart
	requiresRestart := false

	// Execute migrations in order
	for _, migration := range migrationsToRun {
		if err := m.executeMigration(ctx, cfg, db, migration); err != nil {
			return fmt.Errorf("migration failed for version %.0f: %w", migration.GetMajorVersion(), err)
		}

		// Check if this migration requires a restart
		if migration.ShouldRestartServer() {
			requiresRestart = true
		}
	}

	// Update database version after successful migrations
	if err := m.SetCurrentDBVersion(ctx, db, currentCodeVersion); err != nil {
		return fmt.Errorf("failed to update database version after migrations: %w", err)
	}

	m.logger.WithField("version", fmt.Sprintf("%.0f", currentCodeVersion)).Info("Migration process completed successfully")

	// Return restart signal if needed
	if requiresRestart {
		m.logger.Info("Migrations completed - server restart required to reload configuration")
		return ErrRestartRequired
	}

	return nil
}

// executeMigration runs a single migration
func (m *Manager) executeMigration(ctx context.Context, cfg *config.Config, db *sql.DB, migration MajorMigrationInterface) error {
	version := migration.GetMajorVersion()
	m.logger.WithField("version", fmt.Sprintf("%.0f", version)).Info("Executing migration")

	// Start transaction for atomicity
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Execute system migrations if needed
	if migration.HasSystemUpdate() {
		m.logger.WithField("version", fmt.Sprintf("%.0f", version)).Debug("Executing system migrations")
		if err := migration.UpdateSystem(ctx, cfg, tx); err != nil {
			return fmt.Errorf("system migration failed: %w", err)
		}
	}

	// Get all workspaces for workspace migrations
	if migration.HasWorkspaceUpdate() {
		m.logger.WithField("version", fmt.Sprintf("%.0f", version)).Debug("Executing workspace migrations")

		workspaces, err := m.getAllWorkspaces(ctx, db)
		if err != nil {
			return fmt.Errorf("failed to get workspaces: %w", err)
		}

		for _, workspace := range workspaces {
			m.logger.WithField("workspace", workspace.ID).
				WithField("version", fmt.Sprintf("%.0f", version)).
				Debug("Executing workspace migration")

			workspaceDB, migrate, err := m.connectForMigration(ctx, cfg, tx, workspace.ID)
			if err != nil {
				return err
			}
			if !migrate {
				continue
			}

			// Start transaction for the workspace database
			workspaceTx, err := workspaceDB.BeginTx(ctx, nil)
			if err != nil {
				_ = workspaceDB.Close()
				return fmt.Errorf("failed to start workspace transaction for %s: %w", workspace.ID, err)
			}

			// Execute the workspace migration
			migrationErr := migration.UpdateWorkspace(ctx, cfg, &workspace, workspaceTx)

			if migrationErr != nil {
				// Rollback workspace transaction and close connection
				_ = workspaceTx.Rollback()
				_ = workspaceDB.Close()
				return fmt.Errorf("workspace migration failed for workspace %s: %w", workspace.ID, migrationErr)
			}

			// Commit workspace transaction
			if err := workspaceTx.Commit(); err != nil {
				_ = workspaceDB.Close()
				return fmt.Errorf("failed to commit workspace migration for %s: %w", workspace.ID, err)
			}

			// Close workspace database connection
			_ = workspaceDB.Close()

			m.logger.WithField("workspace", workspace.ID).
				WithField("version", fmt.Sprintf("%.0f", version)).
				Debug("Workspace migration completed successfully")
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	m.logger.WithField("version", fmt.Sprintf("%.0f", version)).Info("Migration completed successfully")
	return nil
}

// connectForMigration connects to a workspace database to migrate it. A database that
// holds no tables, or is missing, is looked at again under its name lock, which waits
// for an initialisation in progress on another instance (it commits its whole schema at
// once, so the database looks empty until then) and for a deletion in progress. It
// returns migrate false for a workspace to skip; the caller closes the database. A
// database that holds tables is migrated without the lock, as before: a deletion
// dropping it meanwhile ends the migration with an error, and the next start goes on
// without that workspace.
//
// The name lock is taken while the migration's system transaction is open. When that
// transaction holds row locks on rows a deletion removes (workspaces, user_workspaces,
// workspace_invitations, which several system migrations below 42 write), a workspace
// deletion on an instance that takes the name lock, under way at the same time,
// deadlocks with it, unseen by PostgreSQL (each side waits in Go for another connection
// of its own), until the deletion's 10-minute bound ends it. An instance
// takes the name lock only from 42.3, and serves only once the database is at its own
// version, past those migrations; a future system migration writing those rows would
// bring the case back. The migration lock adds no such case: it is always taken before
// a name lock, here (exclusively, by RunMigrations) as in InitializeEmptyWorkspaces
// (shared). A workspace deletion, and the repair's look at a failed probe, take the name
// lock alone, and neither holds row locks of the system database while waiting for it.
func (m *Manager) connectForMigration(ctx context.Context, cfg *config.Config, tx *sql.Tx, workspaceID string) (*sql.DB, bool, error) {
	workspaceDB, err := m.connector.connectToWorkspace(ctx, &cfg.Database, workspaceID)
	if err != nil {
		if !database.IsDatabaseMissingError(err) {
			return nil, false, fmt.Errorf("failed to connect to workspace database %s: %w", workspaceID, err)
		}
	} else {
		holdsTables, err := database.WorkspaceDatabaseHoldsTables(ctx, workspaceDB)
		if err != nil {
			_ = workspaceDB.Close()
			return nil, false, fmt.Errorf("failed to inspect workspace database %s: %w", workspaceID, err)
		}
		if holdsTables {
			return workspaceDB, true, nil
		}
		_ = workspaceDB.Close()
	}

	migrate, err := m.lookAgainUnderNameLock(ctx, cfg, tx, workspaceID)
	if err != nil || !migrate {
		return nil, false, err
	}

	workspaceDB, err = m.connector.connectToWorkspace(ctx, &cfg.Database, workspaceID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to connect to workspace database %s: %w", workspaceID, err)
	}
	return workspaceDB, true, nil
}

// lookAgainUnderNameLock decides, under the workspace's name lock, what to do with a
// workspace whose database looked empty or missing: skip it when its row is gone or
// its database holds no tables (nothing to migrate; the repair gives it the current
// schema, so pending migrations never run on a freshly built one), fail with what to
// do when its database is missing (it is never created here: a changed DB_PREFIX would
// otherwise turn into one silently empty database per workspace), and migrate it when
// an initialisation committed meanwhile.
func (m *Manager) lookAgainUnderNameLock(ctx context.Context, cfg *config.Config, tx *sql.Tx, workspaceID string) (bool, error) {
	dbName, err := database.WorkspaceDatabaseName(cfg.Database.Prefix, workspaceID)
	if err != nil {
		return false, err
	}

	migrate := false
	err = m.lockWorkspaceDatabase()(ctx, &cfg.Database, dbName, func(ctx context.Context, admin *sql.DB) error {
		var registered bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`, workspaceID).Scan(&registered); err != nil {
			return fmt.Errorf("failed to check workspace %s: %w", workspaceID, err)
		}
		if !registered {
			m.logger.WithField("workspace", workspaceID).Info("Workspace deleted during the migration, skipped")
			return nil
		}

		exists, err := database.WorkspaceDatabaseExists(ctx, admin, dbName)
		if err != nil {
			return err
		}
		if !exists {
			return missingDatabaseError(workspaceID, dbName, cfg.Database.User)
		}

		workspaceDB, err := m.connector.connectToWorkspace(ctx, &cfg.Database, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to connect to workspace database %s: %w", workspaceID, err)
		}
		defer func() { _ = workspaceDB.Close() }()
		holdsTables, err := database.WorkspaceDatabaseHoldsTables(ctx, workspaceDB)
		if err != nil {
			return fmt.Errorf("failed to inspect workspace database %s: %w", workspaceID, err)
		}
		if !holdsTables {
			m.logger.WithField("workspace", workspaceID).
				WithField("database", dbName).
				Warn("Workspace database holds no tables, skipped by the migration")
			return nil
		}
		migrate = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return migrate, nil
}

// getAllWorkspaces retrieves all workspaces from the database
func (m *Manager) getAllWorkspaces(ctx context.Context, db *sql.DB) ([]domain.Workspace, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, name, settings, integrations, created_at, updated_at FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var workspaces []domain.Workspace
	for rows.Next() {
		var workspace domain.Workspace
		err := rows.Scan(
			&workspace.ID,
			&workspace.Name,
			&workspace.Settings,
			&workspace.Integrations,
			&workspace.CreatedAt,
			&workspace.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}

	return workspaces, rows.Err()
}

// InitializeEmptyWorkspaces gives every registered workspace whose database exists but
// holds no tables its schema: a failed creation on an earlier version could leave one,
// which migrations skip. Meant to run in the background once RunMigrations returned nil.
//
// Each workspace's database is first probed, without any lock. One found empty is then
// handled under the shared migration lock, held until its initialisation has committed,
// after the version is read again: the loop stops when it is not the code's (another
// build took over, or a migration is due), so no migration can start between that read
// and the initialisation, and a repaired workspace never receives pending migrations.
// The initialisation itself runs under the workspace's name lock, after its
// registration and emptiness are checked again inside it; a missing database is never
// created here. Failures never stop anything: a workspace that cannot be reached,
// checked or initialised is logged and skipped. A cancelled ctx stops the loop.
func (m *Manager) InitializeEmptyWorkspaces(ctx context.Context, cfg *config.Config, db *sql.DB) {
	codeVersion, err := GetCurrentCodeVersion()
	if err != nil {
		m.logger.WithField("error", err.Error()).Error("Empty workspace repair not run: failed to get the code version")
		return
	}
	ids, err := m.workspaceIDs(ctx, db)
	if err != nil {
		if ctx.Err() == nil {
			m.logger.WithField("error", err.Error()).Error("Empty workspace repair not run: failed to list workspaces")
		}
		return
	}

	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		empty, err := m.probeWorkspace(ctx, cfg, db, id)
		if err != nil || !empty {
			continue
		}
		stop := false
		err = m.shareMigrationLock()(ctx, db, func(ctx context.Context) error {
			version, err, found := m.GetCurrentDBVersion(ctx, db)
			if err != nil {
				return err
			}
			if !found || version != codeVersion {
				stop = true
				return nil
			}
			return m.repairWorkspace(ctx, cfg, db, id)
		})
		if stop {
			m.logger.WithField("code_version", fmt.Sprintf("%.0f", codeVersion)).
				Info("Empty workspace repair stopped: the database version is not this build's")
			return
		}
		if err != nil && ctx.Err() == nil {
			m.logger.WithField("workspace", id).WithField("error", err.Error()).
				Error("Failed to check or initialise the workspace database; skipped")
		}
	}
}

// probeWorkspace reports whether workspaceID's database holds no tables, within the
// check timeout. A failure a deletion may cause (a missing database, a terminated
// connection) is looked at again under the workspace's name lock, which waits for a
// deletion in progress: nothing is said for a workspace deleted meanwhile or a database
// created meanwhile, and a missing database is reported with what to do. Any other
// failure is logged as an error.
func (m *Manager) probeWorkspace(ctx context.Context, cfg *config.Config, db *sql.DB, workspaceID string) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, orDefault(m.repairCheckTimeout, defaultRepairCheckTimeout))
	defer cancel()
	empty, err := m.workspaceDatabaseEmpty(checkCtx, cfg, workspaceID)
	if err == nil || ctx.Err() != nil {
		return empty, err
	}
	log := m.logger.WithField("workspace", workspaceID).WithField("error", err.Error())
	dbName, nameErr := database.WorkspaceDatabaseName(cfg.Database.Prefix, workspaceID)
	if nameErr != nil || !mayComeFromADeletion(err) {
		log.Error("Failed to check or initialise the workspace database; skipped")
		return false, err
	}

	// A deletion holds the name lock for as long as its drop takes.
	lookCtx, cancelLook := context.WithTimeout(ctx, orDefault(m.repairInitTimeout, defaultRepairInitTimeout))
	defer cancelLook()
	registered, exists := true, true
	lookErr := m.lockWorkspaceDatabase()(lookCtx, &cfg.Database, dbName, func(ctx context.Context, admin *sql.DB) error {
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`, workspaceID).Scan(&registered); err != nil || !registered {
			return err
		}
		var err error
		exists, err = database.WorkspaceDatabaseExists(ctx, admin, dbName)
		return err
	})
	switch {
	case ctx.Err() != nil, lookErr == nil && !registered:
	case lookErr == nil && !exists:
		m.logger.WithField("workspace", workspaceID).Warn(missingDatabaseError(workspaceID, dbName, cfg.Database.User).Error())
	case lookErr == nil && database.IsDatabaseMissingError(err):
		// Created meanwhile; the next start looks at it again.
	case lookErr != nil:
		log.WithField("recheck_error", lookErr.Error()).Error("Failed to check or initialise the workspace database; skipped")
	default:
		log.Error("Failed to check or initialise the workspace database; skipped")
	}
	return false, err
}

// mayComeFromADeletion reports whether err is what a probe gets from a deletion: its
// database gone, or its connection terminated by DROP DATABASE ... WITH (FORCE).
func mayComeFromADeletion(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && (pqErr.Code == "3D000" || pqErr.Code == "57P01")
}

// repairWorkspace initialises workspaceID's database, found empty, when it is still
// registered, exists and holds no tables under its name lock.
func (m *Manager) repairWorkspace(ctx context.Context, cfg *config.Config, db *sql.DB, workspaceID string) error {
	dbName, err := database.WorkspaceDatabaseName(cfg.Database.Prefix, workspaceID)
	if err != nil {
		return err
	}
	return m.lockWorkspaceDatabase()(ctx, &cfg.Database, dbName, func(ctx context.Context, admin *sql.DB) error {
		var registered bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`, workspaceID).Scan(&registered); err != nil {
			return fmt.Errorf("failed to check workspace %s: %w", workspaceID, err)
		}
		if !registered {
			return nil
		}
		exists, err := database.WorkspaceDatabaseExists(ctx, admin, dbName)
		if err != nil || !exists {
			return err
		}

		initCtx, cancelInit := context.WithTimeout(ctx, orDefault(m.repairInitTimeout, defaultRepairInitTimeout))
		defer cancelInit()
		workspaceDB, err := m.connector.connectToWorkspace(initCtx, &cfg.Database, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to connect to workspace database %s: %w", workspaceID, err)
		}
		defer func() { _ = workspaceDB.Close() }()
		holdsTables, err := database.WorkspaceDatabaseHoldsTables(initCtx, workspaceDB)
		if err != nil || holdsTables {
			return err
		}
		if err := m.initializer()(initCtx, workspaceDB); err != nil {
			return fmt.Errorf("failed to initialise workspace database %s: %w", workspaceID, err)
		}
		m.logger.WithField("workspace", workspaceID).WithField("database", dbName).
			Info("Empty workspace database given its schema")
		return nil
	})
}

// missingDatabaseError says what to do about a registered workspace whose database does
// not exist.
func missingDatabaseError(workspaceID, dbName, owner string) error {
	return fmt.Errorf("workspace %s is registered but its database %s does not exist: if DB_PREFIX changed, restore it; otherwise create an empty database named %s owned by %s (startup skips empty databases) or delete the workspace's rows",
		workspaceID, dbName, dbName, owner)
}

// workspaceDatabaseEmpty reports whether workspaceID's database holds no tables.
func (m *Manager) workspaceDatabaseEmpty(ctx context.Context, cfg *config.Config, workspaceID string) (bool, error) {
	workspaceDB, err := m.connector.connectToWorkspace(ctx, &cfg.Database, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to connect to workspace database %s: %w", workspaceID, err)
	}
	defer func() { _ = workspaceDB.Close() }()
	holdsTables, err := database.WorkspaceDatabaseHoldsTables(ctx, workspaceDB)
	if err != nil {
		return false, fmt.Errorf("failed to inspect workspace database %s: %w", workspaceID, err)
	}
	return !holdsTables, nil
}

// workspaceIDs lists the registered workspaces.
func (m *Manager) workspaceIDs(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM workspaces ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
