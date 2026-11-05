package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetOpened_RecordsOnlyTrackedAndConsentedOpens(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)

	now := time.Now()
	mock.ExpectExec(`WHERE id = \$2 AND opened_at IS NULL\s+AND `+regexp.QuoteMeta(opensRecordableCondition)).
		WithArgs(now, "msg-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, repo.SetOpened(context.Background(), "ws", "msg-1", now))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetClicked_RecordsOnlyTrackedAndConsentedClicks(t *testing.T) {
	now := time.Now()

	t.Run("without URL", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)

		mock.ExpectExec(`WHERE id = \$2 AND clicked_at IS NULL\s+AND `+regexp.QuoteMeta(clicksRecordableCondition)).
			WithArgs(now, "msg-1").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`WHERE id = \$2 AND opened_at IS NULL\s+AND `+regexp.QuoteMeta(opensRecordableCondition)).
			WithArgs(now, "msg-1").WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.SetClicked(context.Background(), "ws", "msg-1", now, ""))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("with URL", func(t *testing.T) {
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)

		mock.ExpectExec(`clicked_links = CASE[\s\S]*WHERE id = \$2\s+AND `+regexp.QuoteMeta(clicksRecordableCondition)).
			WithArgs(now, "msg-1", "https://example.com").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`WHERE id = \$2 AND opened_at IS NULL\s+AND `+regexp.QuoteMeta(opensRecordableCondition)).
			WithArgs(now, "msg-1").WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.SetClicked(context.Background(), "ws", "msg-1", now, "https://example.com"))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRecordableConditions_ReadTheFlagAndTheContactRefusal(t *testing.T) {
	assert.Contains(t, opensRecordableCondition, "opens_tracked IS NOT FALSE", "a message sent before the flag existed is NULL and stays recordable")
	assert.Contains(t, opensRecordableCondition, "c.tracking_opens_consent = 'denied'")
	assert.Contains(t, clicksRecordableCondition, "clicks_tracked IS NOT FALSE")
	assert.Contains(t, clicksRecordableCondition, "c.tracking_clicks_consent = 'denied'")
}

func TestCreate_WritesTheTrackedFlags(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)

	message := createSampleMessageHistory()
	opens, clicks := true, false
	message.OpensTracked = &opens
	message.ClicksTracked = &clicks

	args := insertArgsCapturing(&capturedArg{})
	args[25] = &opens
	args[26] = &clicks
	mock.ExpectExec(`INSERT INTO message_history`).WithArgs(args...).WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.Create(context.Background(), "ws", testSecretKey, message))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsert_KeepsStoredFlagsWhenTheRetryHasNone(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)

	mock.ExpectExec(regexp.QuoteMeta(`opens_tracked = COALESCE(EXCLUDED.opens_tracked, message_history.opens_tracked)`) + `[\s\S]*` +
		regexp.QuoteMeta(`clicks_tracked = COALESCE(EXCLUDED.clicks_tracked, message_history.clicks_tracked)`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.Upsert(context.Background(), "ws", testSecretKey, createSampleMessageHistory()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBroadcastStats_ReturnsTrackedTotals(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SUM(CASE WHEN sent_at IS NOT NULL AND failed_at IS NULL AND opens_tracked IS NOT FALSE THEN 1 ELSE 0 END) as total_opens_tracked`)).
		WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{
			"total_sent", "total_delivered", "total_failed", "total_opened", "total_clicked", "total_bounced", "total_complained", "total_unsubscribed",
			"total_opens_tracked", "total_clicks_tracked", "total_delivered_opens_tracked", "total_delivered_clicks_tracked",
		}).AddRow(10, 9, 0, 3, 2, 0, 0, 0, 6, 7, 5, 4))

	stats, err := repo.GetBroadcastStats(context.Background(), "ws", "b1")
	require.NoError(t, err)
	assert.Equal(t, 6, stats.TotalOpensTracked)
	assert.Equal(t, 7, stats.TotalClicksTracked)
	assert.Equal(t, 5, stats.TotalDeliveredOpensTracked)
	assert.Equal(t, 4, stats.TotalDeliveredClicksTracked)
}
