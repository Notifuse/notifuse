package migrations

import (
	"context"
	"database/sql"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// DBExecutor represents a database connection that can execute queries
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// MajorMigrationInterface defines a major version migration
type MajorMigrationInterface interface {
	GetMajorVersion() float64
	HasSystemUpdate() bool
	HasWorkspaceUpdate() bool
	ShouldRestartServer() bool
	UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error
	UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error
}

// MigrationManager interface for managing migrations
type MigrationManager interface {
	GetCurrentDBVersion(ctx context.Context, db *sql.DB) (float64, error, bool)
	SetCurrentDBVersion(ctx context.Context, db *sql.DB, version float64) error
	RunMigrations(ctx context.Context, config *config.Config, db *sql.DB) error
}

// MigrationRegistry manages registered migrations
type MigrationRegistry interface {
	Register(migration MajorMigrationInterface)
	GetMigrations() []MajorMigrationInterface
	GetMigration(version float64) (MajorMigrationInterface, bool)
}
