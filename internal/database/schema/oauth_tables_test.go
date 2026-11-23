package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthTableDefinitions_Idempotent(t *testing.T) {
	defs := OAuthTableDefinitions()
	require.NotEmpty(t, defs)
	for _, stmt := range defs {
		assert.Contains(t, stmt, "IF NOT EXISTS", "every statement must be safe to run twice: %s", stmt)
		if strings.HasPrefix(stmt, "CREATE TABLE") {
			// House rule from system_tables.go: constraints never live in the CREATE TABLE.
			assert.NotContains(t, stmt, "REFERENCES", stmt)
			assert.NotContains(t, stmt, " CHECK ", stmt)
		}
	}
}

func TestOAuthTableDefinitions_Tables(t *testing.T) {
	assert.Equal(t, []string{"oauth_clients", "oauth_authorization_requests", "oauth_grants", "oauth_authorization_codes", "oauth_tokens"}, OAuthTableNames)
	joined := strings.Join(OAuthTableDefinitions(), "\n")
	for _, table := range OAuthTableNames {
		assert.Contains(t, joined, "CREATE TABLE IF NOT EXISTS "+table+" (")
	}
	// The access and refresh tokens issued together share a pair.
	assert.Contains(t, joined, "pair_id UUID NOT NULL")
	// Codes and tokens are stored as hashes only.
	assert.Contains(t, joined, "code_hash VARCHAR(64) PRIMARY KEY")
	assert.Contains(t, joined, "token_hash VARCHAR(64) PRIMARY KEY")
	assert.NotContains(t, joined, "api_key TEXT", "the API key itself is never stored")
}

// Deleting a key, or its workspace, ends its connections; the owner who made
// one may leave without ending it.
func TestOAuthTableDefinitions_ForeignKeys(t *testing.T) {
	joined := strings.Join(OAuthTableDefinitions(), "\n")
	for _, fk := range []string{
		"FOREIGN KEY (api_key_user_id) REFERENCES users(id) ON DELETE CASCADE",
		"FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE",
		"FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL",
		"FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE",
	} {
		assert.Contains(t, joined, fk)
	}
	assert.Equal(t, 2, strings.Count(joined, "FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE"), "codes and tokens both follow their grant")
}

// ::regclass is resolved when the statement is planned; before the table
// exists it raises instead of skipping.
func TestOAuthTableDefinitions_ForeignKeysAfterTheirTables(t *testing.T) {
	defs := OAuthTableDefinitions()
	lastCreate, firstFK := -1, -1
	for i, stmt := range defs {
		if strings.HasPrefix(stmt, "CREATE TABLE") {
			lastCreate = i
		}
		if strings.Contains(stmt, "::regclass") && firstFK == -1 {
			firstFK = i
		}
	}
	require.NotEqual(t, -1, firstFK)
	assert.Greater(t, firstFK, lastCreate)
}
