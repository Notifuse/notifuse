package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	_ "github.com/lib/pq" // PostgreSQL driver
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
	// workspaceLock is the name lock a workspace whose database is empty or missing is
	// looked at again under; nil means database.WithWorkspaceDatabaseLock.
	workspaceLock workspaceDatabaseLock
}

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

// RunMigrations executes all necessary migrations based on version comparison
func (m *Manager) RunMigrations(ctx context.Context, cfg *config.Config, db *sql.DB) error {
	m.logger.Info("Starting migration process")

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
		return fmt.Errorf("%w: database is at version %.0f, this build is version %.0f - refusing to start; deploy a build at version %.0f or newer, or restore a database stamped %.0f or lower",
			ErrDatabaseAheadOfCode, currentDBVersion, currentCodeVersion, currentDBVersion, currentCodeVersion)
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
// bring the case back.
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
			return fmt.Errorf("workspace %s is registered but its database %s does not exist: if DB_PREFIX changed, restore it; otherwise create an empty database named %s owned by %s (startup skips empty databases) or delete the workspace's rows",
				workspaceID, dbName, dbName, cfg.Database.User)
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
