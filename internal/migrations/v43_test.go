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

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV43Migration_Metadata(t *testing.T) {
	m := &V43Migration{}
	assert.Equal(t, 43.0, m.GetMajorVersion())
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
	assert.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, nil))
}

func TestV43Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(43.0)
	require.True(t, ok)
	assert.IsType(t, &V43Migration{}, migration)
}

func TestV43Migration_IsWhatLetsTheStampReachFortyThree(t *testing.T) {
	var selected []float64
	for _, m := range GetRegisteredMigrations() {
		if v := m.GetMajorVersion(); v > 42.0 && v <= 43.0 {
			selected = append(selected, v)
		}
	}
	assert.Equal(t, []float64{43.0}, selected)
}

func TestV43Migration_UpdateWorkspaceRunsEveryStatementInOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	statements := v43WorkspaceStatements()
	for _, statement := range statements {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	require.NoError(t, (&V43Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV43Migration_StatementOrder(t *testing.T) {
	statements := v43WorkspaceStatements()
	require.NotEmpty(t, statements)

	assert.Equal(t, `SET LOCAL lock_timeout = '5s'`, statements[0], "the lock timeout must come before any lock is taken")
	assert.Contains(t, statements, schema.TrackContactChangesFunction())
	assert.Contains(t, statements, schema.WebhookContactsTriggerFunction())

	index := func(table string) int {
		for i, s := range statements {
			if strings.HasPrefix(s, "ALTER TABLE "+table) {
				return i
			}
		}
		t.Fatalf("no ALTER TABLE %s", table)
		return -1
	}
	assert.Less(t, index("message_history"), index("contacts"),
		"message_history is locked before contacts, as its status trigger does")

	for _, s := range statements {
		if strings.HasPrefix(s, "ALTER TABLE") {
			assert.NotContains(t, s, "DEFAULT", "a default would rewrite the table")
			assert.NotContains(t, s, "NOT NULL")
			assert.Equal(t, strings.Count(s, "ADD COLUMN"), strings.Count(s, "ADD COLUMN IF NOT EXISTS"), "the migration must be idempotent")
		}
		assert.NotContains(t, s, "CREATE TRIGGER", "reattaching a trigger would lock its table")
	}
}

func TestV43Migration_UpdateWorkspaceStopsOnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	statements := v43WorkspaceStatements()
	mock.ExpectExec(regexp.QuoteMeta(statements[0])).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(statements[1])).WillReturnError(errors.New("boom"))

	err = (&V43Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ws1")
	require.NoError(t, mock.ExpectationsWereMet())
}
