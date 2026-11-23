package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

func setupDataRetentionTest(t *testing.T) (domain.DataRetentionRepository, sqlmock.Sqlmock) {
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
	return NewDataRetentionRepository(workspaceRepo), mock
}

const purgeGUC = `SELECT set_config\('notifuse.retention_purge', 'on', true\)`

func TestDataRetentionRepository_PurgeCategory(t *testing.T) {
	before := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("the purge GUC is set in the purge's own transaction", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		mock.ExpectBegin()
		mock.ExpectExec(purgeGUC).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`(?s)WITH doomed AS \(\s*SELECT id FROM inbound_webhook_events WHERE timestamp < \$1 ORDER BY timestamp LIMIT \$2`).
			WithArgs(before, 5000).WillReturnResult(sqlmock.NewResult(0, 42))
		mock.ExpectCommit()
		n, err := repo.PurgeCategory(context.Background(), "ws", domain.RetentionInboundWebhookEvents, before, 5000, nil)
		require.NoError(t, err)
		assert.EqualValues(t, 42, n)
	})

	t.Run("custom events fold the designated ones into the engagement date before deleting", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		mock.ExpectBegin()
		mock.ExpectExec(purgeGUC).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`(?s)WITH doomed AS .*FROM custom_events.*folded AS \(\s*INSERT INTO contact_engagement.*event_name = ANY\(\$3::text\[\]\) AND d.deleted_at IS NULL`+
			`.*GREATEST\(contact_engagement.last_engaged_at, EXCLUDED.last_engaged_at\).*DELETE FROM custom_events ce USING doomed d`).
			WithArgs(before, 100, `{"purchase"}`).WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectCommit()
		_, err := repo.PurgeCategory(context.Background(), "ws", domain.RetentionCustomEvents, before, 100, []string{"purchase"})
		require.NoError(t, err)
	})

	t.Run("finished journeys only; never the trigger log", func(t *testing.T) {
		statement := purgeStatements[domain.RetentionAutomationHistory]
		assert.Contains(t, statement, "status IN ('completed', 'exited', 'failed')")
		assert.NotContains(t, statement, "automation_trigger_log")
	})

	t.Run("a failed statement rolls back", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		mock.ExpectBegin()
		mock.ExpectExec(purgeGUC).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DELETE FROM webhook_deliveries`).WillReturnError(assert.AnError)
		mock.ExpectRollback()
		_, err := repo.PurgeCategory(context.Background(), "ws", domain.RetentionWebhookDeliveries, before, 10, nil)
		assert.Error(t, err)
	})

	t.Run("message history is redacted from its cursor, which moves", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		cursorAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
		lastAt := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		mock.ExpectExec(purgeGUC).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT message_history_cursor_at, message_history_cursor_id FROM data_retention_state WHERE id = 1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"at", "id"}).AddRow(cursorAt, "m-1"))
		mock.ExpectQuery(`(?s)WITH batch AS .*created_at < \$1 AND \(created_at, id\) > \(\$2::timestamptz, \$3::varchar\)`+
			`.*AND contact_email <> 'DELETED_EMAIL'.*SET contact_email = 'DELETED_EMAIL', clicked_links = NULL, message_data = '\{\}'::jsonb,\s+channel_options = NULL, status_info = NULL, attachments = NULL`+
			`.*DELETE FROM message_attachments`).
			WithArgs(before, cursorAt, "m-1", 500).
			WillReturnRows(sqlmock.NewRows([]string{"n", "at", "id"}).AddRow(500, lastAt, "m-9"))
		mock.ExpectExec(`(?s)INSERT INTO data_retention_state .*message_history_cursor_at`).WithArgs(lastAt, "m-9").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		n, err := repo.PurgeCategory(context.Background(), "ws", domain.RetentionMessageHistory, before, 500, nil)
		require.NoError(t, err)
		assert.EqualValues(t, 500, n)
	})
}

func TestDataRetentionRepository_DeleteExpiredContacts(t *testing.T) {
	t.Run("one transaction: recheck, queue, hash and delete list rows, queued mail, memberships, contacts", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		q := domain.ExpiredContactsQuery{EngagedBefore: time.Now(), AfterEmail: "cursor@x", ExemptListIDs: []string{"vip"}}
		mock.ExpectBegin()
		mock.ExpectExec(purgeGUC).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(SELECT id FROM lists WHERE id = ANY\(\$1::text\[\]\) AND deleted_at IS NULL FOR SHARE\) l`).
			WithArgs(`{"vip"}`).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
		mock.ExpectQuery(`(?s)SELECT c.email FROM contacts c.*c.email = ANY\(\$6::text\[\]\).*db_created_at <= NOW\(\) - INTERVAL '7 days'.*FOR UPDATE OF c`).
			WithArgs("", q.EngagedBefore, "{}", `{"vip"}`, "{}", `{"a@x","b@x"}`).
			WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("a@x"))
		mock.ExpectExec(`INSERT INTO data_retention_erasure_queue`).WithArgs(`{"a@x"}`, "run-1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`(?s)WITH gone AS \(\s*DELETE FROM contact_lists WHERE email = ANY\(\$1::text\[\]\).*contact_opt_out_hash\(gone.email\)`).
			WithArgs(`{"a@x"}`, "retention").WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(`DELETE FROM email_queue WHERE contact_email = ANY`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM contact_segments WHERE email = ANY`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DELETE FROM contacts WHERE email = ANY`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		deleted, optOuts, err := repo.DeleteExpiredContacts(context.Background(), "ws", "run-1", q, []string{"a@x", "b@x"})
		require.NoError(t, err)
		assert.Equal(t, []string{"a@x"}, deleted)
		assert.EqualValues(t, 2, optOuts)
	})

	t.Run("an exempt segment gone or rebuilding since the run started stops the batch", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		mock.ExpectBegin()
		mock.ExpectExec(purgeGUC).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(SELECT id FROM segments WHERE id = ANY\(\$1::text\[\]\) AND status = 'active' FOR SHARE\) s`).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
		mock.ExpectRollback()
		_, _, err := repo.DeleteExpiredContacts(context.Background(), "ws", "run-1",
			domain.ExpiredContactsQuery{ExemptSegmentIDs: []string{"customers"}}, []string{"a@x"})
		assert.ErrorIs(t, err, domain.ErrRetentionExemptionUnavailable)
	})

	t.Run("nobody still expired: nothing written", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		mock.ExpectBegin()
		mock.ExpectExec(purgeGUC).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT c.email FROM contacts c`).WillReturnRows(sqlmock.NewRows([]string{"email"}))
		mock.ExpectCommit()
		deleted, _, err := repo.DeleteExpiredContacts(context.Background(), "ws", "run-1", domain.ExpiredContactsQuery{}, []string{"a@x"})
		require.NoError(t, err)
		assert.Empty(t, deleted)
	})
}

func TestDataRetentionRepository_ListExpiredContacts(t *testing.T) {
	repo, mock := setupDataRetentionTest(t)
	at := time.Now()
	mock.ExpectQuery(`(?s)SELECT c.email FROM contacts c\s+LEFT JOIN contact_engagement e ON e.email = c.email\s+WHERE c.email > \$1`+
		`.*COALESCE\(e.last_engaged_at, '-infinity'::timestamptz\),\s+LEAST\(c.created_at, c.db_created_at\)`+
		`.*ce.event_name = ANY\(\$3::text\[\]\) AND ce.deleted_at IS NULL`+
		`.*NOT EXISTS \(\s+SELECT 1 FROM contact_lists cl.*NOT EXISTS \(\s+SELECT 1 FROM contact_segments cs.*ORDER BY c.email LIMIT \$6`).
		WithArgs("m@x", at, `{"purchase"}`, `{"vip"}`, `{"customers"}`, 200).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("n@x"))
	emails, err := repo.ListExpiredContacts(context.Background(), "ws", domain.ExpiredContactsQuery{
		EngagedBefore: at, AfterEmail: "m@x", EngagementEventNames: []string{"purchase"},
		ExemptListIDs: []string{"vip"}, ExemptSegmentIDs: []string{"customers"}, Limit: 200,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"n@x"}, emails)
}

func TestDataRetentionRepository_DropWebAnalyticsMonth(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)

	t.Run("refuses the current and the next month", func(t *testing.T) {
		repo, _ := setupDataRetentionTest(t)
		for _, month := range []time.Time{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)} {
			err := repo.DropWebAnalyticsMonth(context.Background(), "ws", month, now)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not a past month", "refused before touching the database")
		}
	})

	t.Run("drops exactly that month's three partitions, with a short lock timeout", func(t *testing.T) {
		repo, mock := setupDataRetentionTest(t)
		mock.ExpectBegin()
		mock.ExpectExec(`SET LOCAL lock_timeout = '5s'`).WillReturnResult(sqlmock.NewResult(0, 0))
		for _, table := range []string{"web_goals", "web_pages", "web_sessions"} {
			mock.ExpectExec(`DROP TABLE IF EXISTS "` + table + `_y2026m07"`).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		mock.ExpectCommit()
		require.NoError(t, repo.DropWebAnalyticsMonth(context.Background(), "ws", time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), now))
	})
}

func TestDataRetentionRepository_MissingExemptions(t *testing.T) {
	repo, mock := setupDataRetentionTest(t)
	mock.ExpectQuery(`SELECT id FROM lists WHERE id = ANY\(\$1::text\[\]\) AND deleted_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("vip"))
	mock.ExpectQuery(`SELECT id FROM segments WHERE id = ANY\(\$1::text\[\]\) AND status = 'active'`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	missing, err := repo.MissingExemptions(context.Background(), "ws", []string{"vip", "gone"}, []string{"rebuilding"})
	require.NoError(t, err)
	assert.Equal(t, []string{"list:gone", "segment:rebuilding"}, missing)
}

func TestWorkspaceRepository_SetDataRetentionSettings(t *testing.T) {
	newRepo := func(t *testing.T) (*workspaceRepository, sqlmock.Sqlmock) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, mock.ExpectationsWereMet())
			_ = db.Close()
		})
		return &workspaceRepository{systemDB: db}, mock
	}
	write := `(?s)UPDATE workspaces\s+SET settings = jsonb_set\(COALESCE\(settings, '\{\}'::jsonb\), '\{data_retention\}', \$2::jsonb\), updated_at = \$3\s+` +
		`WHERE id = \$1 AND COALESCE\(\(settings->'data_retention'->>'revision'\)::int, 0\) = \$4`

	t.Run("writes the next revision when the stored one is still the one read", func(t *testing.T) {
		repo, mock := newRepo(t)
		mock.ExpectExec(write).WithArgs("ws", sqlmock.AnyArg(), sqlmock.AnyArg(), 3).WillReturnResult(sqlmock.NewResult(0, 1))
		settings := &domain.DataRetentionSettings{Policy: &domain.DataRetentionPolicy{}}
		ok, err := repo.SetDataRetentionSettings(context.Background(), "ws", 3, settings)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, 4, settings.Revision)
	})

	t.Run("another write came first", func(t *testing.T) {
		repo, mock := newRepo(t)
		mock.ExpectExec(write).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"e"}).AddRow(true))
		ok, err := repo.SetDataRetentionSettings(context.Background(), "ws", 3, &domain.DataRetentionSettings{})
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("no such workspace", func(t *testing.T) {
		repo, mock := newRepo(t)
		mock.ExpectExec(write).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"e"}).AddRow(false))
		_, err := repo.SetDataRetentionSettings(context.Background(), "ws", 0, &domain.DataRetentionSettings{})
		var notFound *domain.ErrWorkspaceNotFound
		assert.ErrorAs(t, err, &notFound)
	})
}

func TestDataRetentionRepository_CustomEventNames(t *testing.T) {
	repo, mock := setupDataRetentionTest(t)
	mock.ExpectQuery(`(?s)WITH RECURSIVE names AS .*WHERE c.event_name > n.event_name ORDER BY c.event_name LIMIT 1.*LIMIT \$1`).
		WithArgs(200).WillReturnRows(sqlmock.NewRows([]string{"event_name"}).AddRow("login").AddRow("purchase"))
	names, err := repo.CustomEventNames(context.Background(), "ws", 200)
	require.NoError(t, err)
	assert.Equal(t, []string{"login", "purchase"}, names)
}

func TestDataRetentionRepository_WriteContactPolicyMirror(t *testing.T) {
	repo, mock := setupDataRetentionTest(t)
	at := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO data_retention_state \(id, contacts_days, exempt_list_ids,.*pending_engagement_event_names\).*ON CONFLICT \(id\) DO UPDATE`).
		WithArgs(1095, `{"vip"}`, "{}", `{"purchase"}`, 365, &at, "{}", "{}", `{"purchase"}`).WillReturnResult(sqlmock.NewResult(0, 1))
	// The segments that use the condition are rebuilt: their members change with the policy.
	mock.ExpectExec(`UPDATE segments SET recompute_after = NOW\(\)\s+WHERE status <> 'deleted' AND jsonb_path_exists\(tree, '\$\.\*\* \? \(@\.source == "retention"\)'\)`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	require.NoError(t, repo.WriteContactPolicyMirror(context.Background(), "ws", domain.ContactPolicyMirror{
		Days: 1095, ExemptListIDs: []string{"vip"}, EngagementEventNames: []string{"purchase"},
		PendingDays: 365, PendingAt: &at, PendingEngagementEventNames: []string{"purchase"},
	}))
}
