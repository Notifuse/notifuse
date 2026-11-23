package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTwoFactorTableDefinitions_Idempotent(t *testing.T) {
	defs := TwoFactorTableDefinitions()
	require.NotEmpty(t, defs)

	for _, stmt := range defs {
		idempotent := strings.Contains(stmt, "IF NOT EXISTS")
		assert.True(t, idempotent, "every statement must be safe to run twice: %s", stmt)
		if strings.HasPrefix(stmt, "CREATE TABLE") {
			// House rule from system_tables.go: constraints never live in the CREATE TABLE.
			assert.NotContains(t, stmt, "REFERENCES")
			assert.NotContains(t, stmt, " CHECK ")
		}
	}
}

func TestTwoFactorTableDefinitions_EveryTimeHasATimeZone(t *testing.T) {
	for _, stmt := range TwoFactorTableDefinitions() {
		for _, line := range strings.Split(stmt, "\n") {
			if strings.Contains(line, "TIMESTAMP") {
				assert.Contains(t, line, "TIMESTAMPTZ", "deadlines are compared to now() in SQL: %s", strings.TrimSpace(line))
			}
		}
	}
}

func TestTwoFactorTableDefinitions_ForeignKeysComeAfterTheirTables(t *testing.T) {
	defs := TwoFactorTableDefinitions()
	index := func(prefix string) int {
		for i, stmt := range defs {
			if strings.Contains(stmt, prefix) {
				return i
			}
		}
		return -1
	}

	for _, table := range []string{"user_totp", "user_recovery_codes"} {
		create := index("CREATE TABLE IF NOT EXISTS " + table + " (")
		fk := index("'" + table + "'::regclass")
		require.NotEqual(t, -1, create, table)
		require.NotEqual(t, -1, fk, "%s needs its foreign key to users", table)
		// ::regclass is resolved when the statement is planned; before the table exists it
		// raises instead of skipping.
		assert.Greater(t, fk, create, table)
		assert.Contains(t, defs[fk], "REFERENCES users(id) ON DELETE CASCADE")
	}
}

func TestTwoFactorTableDefinitions_SessionColumnsAndChallengeIndex(t *testing.T) {
	joined := strings.Join(TwoFactorTableDefinitions(), "\n")
	for _, column := range []string{"auth_method", "second_factor", "second_factor_at", "challenge_hash",
		"challenge_expires_at", "challenge_attempts", "pending_invitation_id", "authenticated_at"} {
		assert.Contains(t, joined, "ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS "+column+" ", column)
	}
	assert.Contains(t, joined,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_user_sessions_challenge_hash ON user_sessions (challenge_hash) WHERE challenge_hash IS NOT NULL")
}
