package migrations

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
)

// The OAuth part of v45 installs the DDL the schema package builds, the one
// init.go runs on a new system database, so the two cannot differ.
func TestOAuthSystemV45Statements(t *testing.T) {
	statements := oauthSystemV45Statements()
	assert.Equal(t, schema.OAuthTableDefinitions(), statements)

	all := strings.Join(statements, "\n")
	for _, table := range []string{"oauth_clients", "oauth_authorization_requests", "oauth_grants", "oauth_authorization_codes", "oauth_tokens"} {
		assert.Contains(t, all, "CREATE TABLE IF NOT EXISTS "+table+" (")
	}
	// Replayable: every statement is guarded.
	for _, statement := range statements {
		guarded := strings.Contains(statement, "IF NOT EXISTS")
		assert.True(t, guarded, "every statement must be safe to run again: %s", statement)
	}
	// No PostgreSQL 18-only DDL and no extension.
	assert.NotContains(t, all, "pgcrypto")
	assert.NotContains(t, strings.ToUpper(all), "VIRTUAL")
	assert.NotContains(t, strings.ToLower(all), "uuidv7")
}

func TestApplyOAuthSystemV45(t *testing.T) {
	t.Run("runs every statement in order", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		for _, statement := range oauthSystemV45Statements() {
			mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		require.NoError(t, applyOAuthSystemV45(context.Background(), db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at the first error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		statements := oauthSystemV45Statements()
		mock.ExpectExec(regexp.QuoteMeta(statements[0])).WillReturnError(errors.New("boom"))
		err = applyOAuthSystemV45(context.Background(), db)
		require.ErrorContains(t, err, "oauth tables (v45 system)")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// The OAuth tables live in the system database: the workspace part of v45 never
// names them.
func TestOAuthTablesAreSystemOnly(t *testing.T) {
	workspace := strings.Join(append(dataRetentionWorkspaceV45Statements(), externalIDWorkspaceV45Statements()...), "\n")
	assert.NotContains(t, workspace, "oauth_")
}
