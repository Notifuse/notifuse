package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The conditions of each filter, as ListMessages and CountMessages both add them.
func TestApplyMessageFilters(t *testing.T) {
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	yes, no := true, false

	tests := []struct {
		name   string
		filter domain.MessageFilter
		where  string
		args   []interface{}
	}{
		{"no filter", domain.MessageFilter{}, "", nil},
		{"origin broadcast", domain.MessageFilter{Origin: domain.MessageOriginBroadcast}, "broadcast_id IS NOT NULL", nil},
		{"origin automation", domain.MessageFilter{Origin: domain.MessageOriginAutomation}, "automation_id IS NOT NULL", nil},
		{"origin transactional", domain.MessageFilter{Origin: domain.MessageOriginTransactional}, "transactional_notification_id IS NOT NULL", nil},
		{"origin other", domain.MessageFilter{Origin: domain.MessageOriginOther},
			"automation_id IS NULL AND broadcast_id IS NULL AND transactional_notification_id IS NULL", nil},
		{"created range", domain.MessageFilter{CreatedAfter: &after, CreatedBefore: &before},
			"created_at >= $1 AND created_at <= $2", []interface{}{&after, &before}},
		{"identifiers", domain.MessageFilter{ID: "m1", ExternalID: "e1", ListID: "l1", Channel: "email", ContactEmail: "a@example.com",
			BroadcastID: "b1", TransactionalNotificationID: "t1", TemplateID: "tpl1"},
			"id = $1 AND external_id = $2 AND list_id = $3 AND channel = $4 AND contact_email = $5 AND broadcast_id = $6 AND transactional_notification_id = $7 AND template_id = $8",
			[]interface{}{"m1", "e1", "l1", "email", "a@example.com", "b1", "t1", "tpl1"}},
		{"clicked url", domain.MessageFilter{BroadcastID: "b1", ClickedURL: "https://example.com"},
			"broadcast_id = $1 AND jsonb_typeof(clicked_links) = 'object' AND clicked_links ? $2", []interface{}{"b1", "https://example.com"}},
		{"statuses", domain.MessageFilter{IsSent: &yes, IsDelivered: &no, IsFailed: &yes, IsOpened: &no, IsClicked: &yes,
			IsBounced: &no, IsComplained: &yes, IsUnsubscribed: &no},
			"sent_at IS NOT NULL AND delivered_at IS NULL AND failed_at IS NOT NULL AND opened_at IS NULL AND clicked_at IS NOT NULL AND bounced_at IS NULL AND complained_at IS NOT NULL AND unsubscribed_at IS NULL", nil},
		{"sent and updated ranges", domain.MessageFilter{SentAfter: &after, SentBefore: &before, UpdatedAfter: &after, UpdatedBefore: &before},
			"sent_at >= $1 AND sent_at <= $2 AND updated_at >= $3 AND updated_at <= $4", []interface{}{&after, &before, &after, &before}},
		{"origin with a period and a status", domain.MessageFilter{Origin: domain.MessageOriginAutomation, IsOpened: &yes, CreatedAfter: &after},
			"opened_at IS NOT NULL AND automation_id IS NOT NULL AND created_at >= $1", []interface{}{&after}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query, args, err := applyMessageFilters(sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Select("id").From("message_history"), tc.filter).ToSql()
			require.NoError(t, err)
			want := "SELECT id FROM message_history"
			if tc.where != "" {
				want += " WHERE " + tc.where
			}
			assert.Equal(t, want, query)
			if tc.args == nil {
				assert.Empty(t, args)
			} else {
				assert.Equal(t, tc.args, args)
			}
		})
	}
}

const countMessagesSelect = `SELECT COUNT(*), COUNT(*) FILTER (WHERE delivered_at IS NOT NULL), COUNT(*) FILTER (WHERE opened_at IS NOT NULL), ` +
	`COUNT(*) FILTER (WHERE clicked_at IS NOT NULL), COUNT(*) FILTER (WHERE bounced_at IS NOT NULL), ` +
	`COUNT(*) FILTER (WHERE failed_at IS NOT NULL), COUNT(*) FILTER (WHERE complained_at IS NOT NULL) FROM message_history`

func TestMessageHistoryRepository_CountMessages(t *testing.T) {
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	t.Run("every count, with the filter's conditions", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)

		mock.ExpectQuery(regexp.QuoteMeta(countMessagesSelect + ` WHERE automation_id IS NOT NULL AND created_at >= $1`)).
			WithArgs(driver.Value(after)).
			WillReturnRows(sqlmock.NewRows([]string{"total", "delivered", "opened", "clicked", "bounced", "failed", "complained"}).
				AddRow(70, 60, 50, 40, 3, 2, 1))

		counts, err := repo.CountMessages(context.Background(), "workspace-123", domain.MessageFilter{
			Origin:       domain.MessageOriginAutomation,
			CreatedAfter: &after,
		})
		require.NoError(t, err)
		assert.Equal(t, &domain.MessageCounts{Total: 70, Delivered: 60, Opened: 50, Clicked: 40, Bounced: 3, Failed: 2, Complained: 1}, counts)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the same conditions as ListMessages for one filter", func(t *testing.T) {
		yes := true
		filter := domain.MessageFilter{BroadcastID: "b1", IsOpened: &yes, Origin: domain.MessageOriginBroadcast, CreatedAfter: &after}
		list, listArgs, err := applyMessageFilters(sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Select("id").From("message_history"), filter).ToSql()
		require.NoError(t, err)

		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)
		where := list[len("SELECT id FROM message_history"):]
		require.Len(t, listArgs, 2)
		mock.ExpectQuery(regexp.QuoteMeta(countMessagesSelect + where)).
			WithArgs("b1", driver.Value(after)).
			WillReturnRows(sqlmock.NewRows([]string{"total", "delivered", "opened", "clicked", "bounced", "failed", "complained"}).
				AddRow(0, 0, 0, 0, 0, 0, 0))

		_, err = repo.CountMessages(context.Background(), "workspace-123", filter)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())

		// ListMessages applies the same filter through the list parameters.
		mockWorkspaceRepo2, repo2, mock2, db2, cleanup2 := setupMessageHistoryTest(t)
		defer cleanup2()
		mockWorkspaceRepo2.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db2, nil)
		mock2.ExpectQuery(regexp.QuoteMeta(where + ` ORDER BY created_at DESC, id DESC LIMIT 21`)).
			WithArgs("b1", driver.Value(after)).
			WillReturnError(errors.New("stop"))
		params := domain.MessageListParams{Limit: 20, BroadcastID: "b1", IsOpened: &yes, Origin: domain.MessageOriginBroadcast, CreatedAfter: &after}
		_, _, err = repo2.ListMessages(context.Background(), "workspace-123", testSecretKey, params)
		require.Error(t, err)
		require.NoError(t, mock2.ExpectationsWereMet())
	})

	t.Run("connection and query errors returned", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(nil, errors.New("no db"))
		_, err := repo.CountMessages(context.Background(), "workspace-123", domain.MessageFilter{CreatedAfter: &after})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no db")

		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)
		mock.ExpectQuery(regexp.QuoteMeta(countMessagesSelect)).WillReturnError(errors.New("db down"))
		_, err = repo.CountMessages(context.Background(), "workspace-123", domain.MessageFilter{CreatedAfter: &after})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}
