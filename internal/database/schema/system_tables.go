package schema

// Schema definitions - no external imports needed

// TableDefinitions contains all the SQL statements to create the database tables
// Don't put REFERENCES and don't put CHECK constraints in the CREATE TABLE statements
var TableDefinitions = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY,
		type VARCHAR(20) NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		name VARCHAR(255),
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS user_sessions (
		id UUID PRIMARY KEY,
		user_id UUID NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL,
		magic_code VARCHAR(255),  -- HMAC-SHA256 hash of authentication code (not plain text)
		magic_code_expires_at TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS workspaces (
		id VARCHAR(20) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		settings JSONB NOT NULL DEFAULT '{"timezone": "UTC"}',
		integrations JSONB,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS user_workspaces (
		user_id UUID NOT NULL,
		workspace_id VARCHAR(20) NOT NULL,
		role VARCHAR(20) NOT NULL,
		permissions JSONB DEFAULT '{}'::jsonb,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (user_id, workspace_id)
	)`,
	`CREATE TABLE IF NOT EXISTS workspace_invitations (
		id UUID PRIMARY KEY,
		workspace_id VARCHAR(20) NOT NULL,
		inviter_id UUID NOT NULL,
		email VARCHAR(255) NOT NULL,
		permissions JSONB DEFAULT '{}'::jsonb,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE (workspace_id, email)
	)`,
	`CREATE TABLE IF NOT EXISTS tasks (
		id UUID PRIMARY KEY,
		workspace_id VARCHAR(20) NOT NULL,
		type VARCHAR(50) NOT NULL,
		status VARCHAR(20) NOT NULL,
		progress FLOAT NOT NULL DEFAULT 0,
		state JSONB,
		error_message TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		last_run_at TIMESTAMP,
		completed_at TIMESTAMP,
		next_run_after TIMESTAMP,
		timeout_after TIMESTAMP,
		max_runtime INTEGER NOT NULL DEFAULT 300,
		max_retries INTEGER NOT NULL DEFAULT 3,
		retry_count INTEGER NOT NULL DEFAULT 0,
		retry_interval INTEGER NOT NULL DEFAULT 300,
		broadcast_id VARCHAR(36),
		recurring_interval INTEGER,
		integration_id VARCHAR(36)
	)`,
	`CREATE TABLE IF NOT EXISTS settings (
		key VARCHAR(255) PRIMARY KEY,
		value TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (key)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks (workspace_id)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks (type)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_next_run_after ON tasks (next_run_after)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks (created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_broadcast_id ON tasks (broadcast_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_broadcast_id ON tasks (workspace_id, broadcast_id) WHERE broadcast_id IS NOT NULL`,
}

// MigrationStatements contains SQL statements to be run after table creation
// These are for schema changes that need to be applied to existing databases
var MigrationStatements = []string{
	`DO $$
	BEGIN
		-- Add unique constraint on workspace_invitations (workspace_id, email) if it doesn't exist
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'workspace_invitations_workspace_id_email_key'
			AND conrelid = 'workspace_invitations'::regclass
		) THEN
			ALTER TABLE workspace_invitations ADD CONSTRAINT workspace_invitations_workspace_id_email_key UNIQUE (workspace_id, email);
		END IF;
	EXCEPTION
		WHEN duplicate_object THEN
			-- Constraint already exists, ignore
			NULL;
	END $$`,
	`CREATE TABLE IF NOT EXISTS settings (
		key VARCHAR(255) PRIMARY KEY,
		value TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (key)
	)`,
	// V27: Add recurring_interval and integration_id columns to tasks
	`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurring_interval INTEGER`,
	`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS integration_id VARCHAR(36)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_integration_id ON tasks (integration_id) WHERE integration_id IS NOT NULL`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_integration_active ON tasks (workspace_id, integration_id) WHERE integration_id IS NOT NULL AND status NOT IN ('completed', 'failed')`,
}

// GetMigrationStatements returns migration statements for database schema setup
func GetMigrationStatements() []string {
	return MigrationStatements
}

// TableNames returns a list of all table names in creation order
var TableNames = []string{
	"users",
	"user_sessions",
	"workspaces",
	"user_workspaces",
	"workspace_invitations",
	"broadcasts",
	"tasks",
	"settings",
}
