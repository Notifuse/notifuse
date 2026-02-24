package migrations

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestV28Migration_GetMajorVersion(t *testing.T) {
	m := &V28Migration{}
	assert.Equal(t, 28.0, m.GetMajorVersion())
}

func TestV28Migration_HasSystemUpdate(t *testing.T) {
	m := &V28Migration{}
	assert.False(t, m.HasSystemUpdate())
}

func TestV28Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V28Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV28Migration_ShouldRestartServer(t *testing.T) {
	m := &V28Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV28Migration_UpdateWorkspace(t *testing.T) {
	m := &V28Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test"}

	t.Run("success", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE templates").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE templates").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS workspace_translations").WillReturnResult(sqlmock.NewResult(0, 0))

		err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("translations column error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE templates").WillReturnError(fmt.Errorf("db error"))

		err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add translations column")
	})

	t.Run("default_language column error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE templates").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE templates").WillReturnError(fmt.Errorf("db error"))

		err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add default_language column")
	})

	t.Run("workspace_translations table error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE templates").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE templates").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS workspace_translations").WillReturnError(fmt.Errorf("db error"))

		err = m.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create workspace_translations table")
	})
}
