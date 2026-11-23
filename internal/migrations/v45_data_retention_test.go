package migrations

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The data retention part of v45 installs the bodies the schema package builds, the
// ones init.go installs on a new workspace, so the two cannot differ.
func TestDataRetentionWorkspaceV45Statements(t *testing.T) {
	statements := dataRetentionWorkspaceV45Statements()
	all := strings.Join(statements, "\n")
	assert.Contains(t, statements, schema.TrackMessageHistoryChangesFunction())
	assert.Contains(t, statements, schema.TrackContactListChangesFunction())
	assert.Contains(t, statements, schema.WebhookContactListsTriggerFunction())
	for _, statement := range schema.ContactOptOutStatements() {
		assert.Contains(t, statements, statement)
	}
	// No PostgreSQL 18-only DDL and no extension.
	assert.NotContains(t, all, "pgcrypto")
	assert.NotContains(t, strings.ToUpper(all), "VIRTUAL")
}

func TestApplyDataRetentionWorkspaceV45(t *testing.T) {
	t.Run("runs every statement in order", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		for _, statement := range dataRetentionWorkspaceV45Statements() {
			mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		require.NoError(t, applyDataRetentionWorkspaceV45(context.Background(), db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at the first error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		statements := dataRetentionWorkspaceV45Statements()
		mock.ExpectExec(regexp.QuoteMeta(statements[0])).WillReturnError(errors.New("boom"))
		err = applyDataRetentionWorkspaceV45(context.Background(), db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data retention (v45)")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
