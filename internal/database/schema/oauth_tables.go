package schema

// The OAuth authorization server of the MCP endpoint, in the SYSTEM database:
// cached client metadata documents, authorization requests waiting for the
// authorization page, single-use codes, grants (one per connection of an API
// key) and tokens. Codes, secrets and tokens are stored as SHA-256 hashes.
//
// Shared verbatim by internal/database/init.go (fresh installs) and the v45
// migration (upgrades), so the two cannot drift. Every statement is safe to
// run again: v45 is replayed on databases that already ran a 45.0 build.
//
// House rule from system_tables.go: no REFERENCES and no CHECK inside a
// CREATE TABLE. The foreign keys are added by the guarded DO blocks at the end,
// which resolve the tables with ::regclass and so must stay after them.
//
// Deleting an API key deletes its users row, which deletes its grants, and
// through them their codes and tokens. Deleting a workspace deletes the grants
// of its keys the same way. The owner who made a key on the authorization
// page may leave: the grant keeps working and forgets who made it.

// OAuthTableNames are the tables, in creation order.
var OAuthTableNames = []string{
	"oauth_clients",
	"oauth_authorization_requests",
	"oauth_grants",
	"oauth_authorization_codes",
	"oauth_tokens",
}

// OAuthTableDefinitions returns the DDL of the OAuth tables, their indexes and
// their foreign keys.
func OAuthTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS oauth_clients (
	client_id TEXT PRIMARY KEY,
	client_name TEXT NOT NULL DEFAULT '',
	redirect_uris JSONB NOT NULL DEFAULT '[]'::jsonb,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL,
	last_used_at TIMESTAMPTZ,
	fetched_at TIMESTAMPTZ,
	cache_expires_at TIMESTAMPTZ,
	fetch_failed_until TIMESTAMPTZ
)`,
		`CREATE TABLE IF NOT EXISTS oauth_authorization_requests (
	id VARCHAR(64) PRIMARY KEY,
	client_id TEXT NOT NULL,
	client_snapshot JSONB NOT NULL,
	redirect_uri TEXT NOT NULL,
	state TEXT NOT NULL DEFAULT '',
	code_challenge VARCHAR(128) NOT NULL,
	resource TEXT NOT NULL,
	browser_binding_hash VARCHAR(64) NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	consumed_at TIMESTAMPTZ,
	expires_at TIMESTAMPTZ NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS oauth_grants (
	id UUID PRIMARY KEY,
	api_key_user_id UUID NOT NULL,
	workspace_id VARCHAR(20) NOT NULL,
	client_id TEXT NOT NULL,
	client_name TEXT NOT NULL,
	resource TEXT NOT NULL,
	created_via VARCHAR(20) NOT NULL,
	created_by_user_id UUID,
	created_at TIMESTAMPTZ NOT NULL,
	last_used_at TIMESTAMPTZ,
	revoked_at TIMESTAMPTZ,
	revoked_reason VARCHAR(32)
)`,
		`CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
	code_hash VARCHAR(64) PRIMARY KEY,
	grant_id UUID NOT NULL,
	client_id TEXT NOT NULL,
	redirect_uri TEXT NOT NULL,
	code_challenge VARCHAR(128) NOT NULL,
	resource TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ
)`,
		`CREATE TABLE IF NOT EXISTS oauth_tokens (
	token_hash VARCHAR(64) PRIMARY KEY,
	grant_id UUID NOT NULL,
	pair_id UUID NOT NULL,
	client_id TEXT NOT NULL,
	kind VARCHAR(10) NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	rotated_at TIMESTAMPTZ,
	replaced_by_hash VARCHAR(64),
	invalidated_at TIMESTAMPTZ
)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_requests_expires_at ON oauth_authorization_requests (expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_grants_workspace_id ON oauth_grants (workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_grants_api_key_user_id ON oauth_grants (api_key_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_grants_client_id ON oauth_grants (client_id)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_grants_created_by_user_id ON oauth_grants (created_by_user_id) WHERE created_by_user_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_codes_grant_id ON oauth_authorization_codes (grant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires_at ON oauth_authorization_codes (expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_tokens_grant_kind ON oauth_tokens (grant_id, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_tokens_expires_at ON oauth_tokens (expires_at)`,
		oauthForeignKey("oauth_grants", "oauth_grants_api_key_user_id_fkey", "api_key_user_id", "users(id)", "CASCADE"),
		oauthForeignKey("oauth_grants", "oauth_grants_workspace_id_fkey", "workspace_id", "workspaces(id)", "CASCADE"),
		oauthForeignKey("oauth_grants", "oauth_grants_created_by_user_id_fkey", "created_by_user_id", "users(id)", "SET NULL"),
		oauthForeignKey("oauth_authorization_codes", "oauth_authorization_codes_grant_id_fkey", "grant_id", "oauth_grants(id)", "CASCADE"),
		oauthForeignKey("oauth_tokens", "oauth_tokens_grant_id_fkey", "grant_id", "oauth_grants(id)", "CASCADE"),
	}
}

// oauthForeignKey adds a foreign key unless a constraint of that name exists.
func oauthForeignKey(table, name, column, references, onDelete string) string {
	return `DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = '` + name + `' AND conrelid = '` + table + `'::regclass
	) THEN
		ALTER TABLE ` + table + ` ADD CONSTRAINT ` + name + `
			FOREIGN KEY (` + column + `) REFERENCES ` + references + ` ON DELETE ` + onDelete + `;
	END IF;
END $$`
}
