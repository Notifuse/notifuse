package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// ErrRestartRequired is returned when a migration requires a server restart
var ErrRestartRequired = errors.New("migration completed successfully - server restart required")

// workspaceConnector interface for connecting to workspace databases
type workspaceConnector interface {
	connectToWorkspace(cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error)
}

// Manager implements MigrationManager
type Manager struct {
	logger    logger.Logger
	connector workspaceConnector
}

// defaultConnector implements workspaceConnector
type defaultConnector struct{}

func (c *defaultConnector) connectToWorkspace(cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	// Replace hyphens with underscores for PostgreSQL compatibility
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	dbName := fmt.Sprintf("%s_ws_%s", cfg.Prefix, safeID)
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		dbName,
		cfg.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
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

	// Check if migrations are needed
	if currentDBVersion >= currentCodeVersion {
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

			// Connect to the specific workspace database
			workspaceDB, err := m.connector.connectToWorkspace(&cfg.Database, workspace.ID)
			if err != nil {
				return fmt.Errorf("failed to connect to workspace database %s: %w", workspace.ID, err)
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
