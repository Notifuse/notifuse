package repository

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var webVersionQuery = regexp.QuoteMeta(`SELECT id, broadcast_id, automation_id, template_id, template_version, message_data FROM message_history WHERE id = $1`)

func webVersionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "broadcast_id", "automation_id", "template_id", "template_version", "message_data"})
}

func TestMessageHistoryRepository_GetForWebVersion(t *testing.T) {
	ctx := context.Background()
	workspaceID := "workspace-123"

	t.Run("a message with encrypted data", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		encrypted, err := encryptMessageData(domain.MessageData{Data: map[string]interface{}{"contact": map[string]interface{}{"first_name": "Camille"}}}, testSecretKey)
		require.NoError(t, err)
		stored, err := json.Marshal(encrypted)
		require.NoError(t, err)

		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)
		mock.ExpectQuery(webVersionQuery).WithArgs("msg-1").
			WillReturnRows(webVersionRows().AddRow("msg-1", "broadcast-1", nil, "tpl-1", int64(4), stored))

		message, err := repo.GetForWebVersion(ctx, workspaceID, testSecretKey, "msg-1")
		require.NoError(t, err)
		require.NotNil(t, message)
		assert.Equal(t, "msg-1", message.ID)
		require.NotNil(t, message.BroadcastID)
		assert.Equal(t, "broadcast-1", *message.BroadcastID)
		assert.Nil(t, message.AutomationID)
		assert.Equal(t, "tpl-1", message.TemplateID)
		assert.Equal(t, int64(4), message.TemplateVersion)
		assert.Equal(t, map[string]interface{}{"first_name": "Camille"}, message.MessageData.Data["contact"])
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a message whose contact was erased", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)
		mock.ExpectQuery(webVersionQuery).WithArgs("msg-2").
			WillReturnRows(webVersionRows().AddRow("msg-2", nil, "auto-1", "tpl-1", int64(2), []byte(`{}`)))

		message, err := repo.GetForWebVersion(ctx, workspaceID, testSecretKey, "msg-2")
		require.NoError(t, err)
		require.NotNil(t, message)
		require.NotNil(t, message.AutomationID)
		assert.Equal(t, "auto-1", *message.AutomationID)
		assert.Empty(t, message.MessageData.Data)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no such message", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)
		mock.ExpectQuery(webVersionQuery).WithArgs("msg-3").WillReturnRows(webVersionRows())

		message, err := repo.GetForWebVersion(ctx, workspaceID, testSecretKey, "msg-3")
		require.NoError(t, err)
		assert.Nil(t, message)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)
		mock.ExpectQuery(webVersionQuery).WithArgs("msg-4").WillReturnError(errors.New("boom"))

		_, err := repo.GetForWebVersion(ctx, workspaceID, testSecretKey, "msg-4")
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("data that does not decrypt", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)
		mock.ExpectQuery(webVersionQuery).WithArgs("msg-5").
			WillReturnRows(webVersionRows().AddRow("msg-5", "broadcast-1", nil, "tpl-1", int64(1), []byte(`{"data":{"_encrypted":"00"}}`)))

		_, err := repo.GetForWebVersion(ctx, workspaceID, testSecretKey, "msg-5")
		assert.Error(t, err)
	})

	t.Run("connection error", func(t *testing.T) {
		mockWorkspaceRepo, repo, _, _, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(nil, errors.New("no db"))
		_, err := repo.GetForWebVersion(ctx, workspaceID, testSecretKey, "msg-6")
		assert.ErrorContains(t, err, "no db")
	})
}
