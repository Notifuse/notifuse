package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupContactEngagementTest(t *testing.T) (domain.ContactEngagementRepository, sqlmock.Sqlmock) {
	t.Helper()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil).AnyTimes()
	return NewContactEngagementRepository(workspaceRepo), mock
}

// Every write keeps the later date, skips a move under an hour, caps a future date at now,
// and only touches a contact that exists.
const engagementUpsertPattern = `.*ON CONFLICT \(email\) DO UPDATE SET last_engaged_at = EXCLUDED.last_engaged_at\s+` +
	`WHERE EXCLUDED.last_engaged_at > contact_engagement.last_engaged_at \+ INTERVAL '1 hour'`

func TestContactEngagementRepository_Touch(t *testing.T) {
	repo, mock := setupContactEngagementTest(t)
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(`(?s)INSERT INTO contact_engagement \(email, last_engaged_at\)\s+SELECT c.email, LEAST\(\$2::timestamptz, NOW\(\)\) FROM contacts c WHERE c.email = \$1 FOR KEY SHARE`+engagementUpsertPattern).
		WithArgs("a@example.com", at).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Touch(context.Background(), "ws", "a@example.com", at))

	mock.ExpectExec(`INSERT INTO contact_engagement`).WillReturnError(errors.New("boom"))
	assert.Error(t, repo.Touch(context.Background(), "ws", "a@example.com", at))
}

func TestContactEngagementRepository_TouchForMessage(t *testing.T) {
	repo, mock := setupContactEngagementTest(t)
	at := time.Now()
	mock.ExpectExec(`(?s)INSERT INTO contact_engagement .*FROM message_history\s+WHERE message_history.id = \$1`+
		`.*AND EXISTS \(SELECT 1 FROM contacts c WHERE c.email = message_history.contact_email FOR KEY SHARE\)`+
		`.*clicks_tracked IS NOT FALSE.*tracking_clicks_consent = 'denied'`+engagementUpsertPattern).
		WithArgs("msg-1", at).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.TouchForMessage(context.Background(), "ws", "msg-1", at))
}

func TestContactEngagementRepository_TouchForWebSessions(t *testing.T) {
	t.Run("one statement per partition date, in date then id order", func(t *testing.T) {
		repo, mock := setupContactEngagementTest(t)
		day1 := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
		day2 := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
		statement := `(?s)INSERT INTO contact_engagement .*FROM web_sessions s\s+WHERE s.session_date = \$1\s+AND s.id = ANY\(\$2::uuid\[\]\)` +
			`.*AND EXISTS \(SELECT 1 FROM contacts c WHERE c.email = s.contact_email FOR KEY SHARE\).*ORDER BY s.contact_email` + engagementUpsertPattern
		mock.ExpectExec(statement).WithArgs(day1, `{"a"}`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(statement).WithArgs(day2, `{"b","c"}`).WillReturnResult(sqlmock.NewResult(0, 2))

		require.NoError(t, repo.TouchForWebSessions(context.Background(), "ws", []*domain.WebSession{
			{SessionDate: day2, ID: "c"}, {SessionDate: day1, ID: "a"}, {SessionDate: day2, ID: "b"}, {SessionDate: day2, ID: "c"}, nil,
		}))
	})

	t.Run("no session writes nothing", func(t *testing.T) {
		repo, _ := setupContactEngagementTest(t)
		require.NoError(t, repo.TouchForWebSessions(context.Background(), "ws", nil))
	})
}

func TestContactEngagementRepository_BackfillBatch(t *testing.T) {
	statement := `(?s)WITH batch AS \(\s*SELECT email FROM contacts WHERE email > \$1 ORDER BY email LIMIT \$2` +
		`.*clicks AS .*jsonb_each\(mh.clicked_links\).*NOT LIKE '%/notification-center%'.*ELSE mh.clicked_at END` +
		`.*WHERE mh.clicked_at IS NOT NULL\s+AND NOT EXISTS .*tracking_clicks_consent = 'denied'` +
		`.*actions AS .*WHERE t.kind IN \('email.replied', 'list.confirmed'\) OR t.entity_type = 'web_session'` +
		`.*SELECT email, at FROM merged ORDER BY email.*SET last_engaged_at = GREATEST\(contact_engagement.last_engaged_at, EXCLUDED.last_engaged_at\)`

	t.Run("returns the cursor and the counts", func(t *testing.T) {
		repo, mock := setupContactEngagementTest(t)
		mock.ExpectQuery(statement).WithArgs("m@example.com", 500).
			WillReturnRows(sqlmock.NewRows([]string{"max", "contacts", "engaged"}).AddRow("z@example.com", 500, 120))
		last, contacts, engaged, err := repo.BackfillBatch(context.Background(), "ws", "m@example.com", 500)
		require.NoError(t, err)
		assert.Equal(t, "z@example.com", last)
		assert.Equal(t, int64(500), contacts)
		assert.Equal(t, int64(120), engaged)
	})

	t.Run("an empty batch is the end", func(t *testing.T) {
		repo, mock := setupContactEngagementTest(t)
		mock.ExpectQuery(statement).WithArgs("z@example.com", 500).
			WillReturnRows(sqlmock.NewRows([]string{"max", "contacts", "engaged"}).AddRow(nil, 0, 0))
		last, contacts, _, err := repo.BackfillBatch(context.Background(), "ws", "z@example.com", 500)
		require.NoError(t, err)
		assert.Empty(t, last)
		assert.Zero(t, contacts)
	})
}

func TestContactEngagementRepository_BackfillMarker(t *testing.T) {
	t.Run("complete keeps an earlier date", func(t *testing.T) {
		repo, mock := setupContactEngagementTest(t)
		mock.ExpectExec(`(?s)INSERT INTO data_retention_state \(id, backfill_completed_at\) VALUES \(1, NOW\(\)\)` +
			`.*SET backfill_completed_at = COALESCE\(data_retention_state.backfill_completed_at, EXCLUDED.backfill_completed_at\)`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.CompleteBackfill(context.Background(), "ws"))
	})

	t.Run("no marker reads as not ready", func(t *testing.T) {
		repo, mock := setupContactEngagementTest(t)
		mock.ExpectQuery(`SELECT backfill_completed_at IS NOT NULL FROM data_retention_state WHERE id = 1`).
			WillReturnError(sql.ErrNoRows)
		done, err := repo.BackfillCompleted(context.Background(), "ws")
		require.NoError(t, err)
		assert.False(t, done)
	})

	t.Run("a set marker reads as ready", func(t *testing.T) {
		repo, mock := setupContactEngagementTest(t)
		mock.ExpectQuery(`SELECT backfill_completed_at IS NOT NULL`).WillReturnRows(sqlmock.NewRows([]string{"done"}).AddRow(true))
		done, err := repo.BackfillCompleted(context.Background(), "ws")
		require.NoError(t, err)
		assert.True(t, done)
	})
}
