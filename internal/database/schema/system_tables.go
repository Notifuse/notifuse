package schema

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
		magic_code VARCHAR(255),
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
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (user_id, workspace_id)
	)`,
	`CREATE TABLE IF NOT EXISTS workspace_invitations (
		id UUID PRIMARY KEY,
		workspace_id VARCHAR(20) NOT NULL,
		inviter_id UUID NOT NULL,
		email VARCHAR(255) NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
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
		broadcast_id VARCHAR(36)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks (workspace_id)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks (type)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_next_run_after ON tasks (next_run_after)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks (created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_broadcast_id ON tasks (broadcast_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_broadcast_id ON tasks (workspace_id, broadcast_id) WHERE broadcast_id IS NOT NULL`,
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
}
