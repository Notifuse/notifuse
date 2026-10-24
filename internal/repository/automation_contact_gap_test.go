package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

// Gap tests pin error branches left uncovered in automation_postgres.go and
// contact_postgres.go. Names use the TestGap prefix to avoid collisions.

var gapCACols = []string{"id", "automation_id", "contact_email", "current_node_id", "status", "exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error", "last_retry_at", "max_retries"}

func gapCARow(id string, ctxJSON []byte, now time.Time) []driver.Value {
	return []driver.Value{id, "auto-1", "a@example.com", "n1", "active", nil, now, nil, ctxJSON, int64(0), nil, nil, int64(3)}
}

func TestGapGetContactAutomationTxErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	t.Run("generic query error is wrapped, not mistaken for not-found", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("conn dropped")
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, automation_id, contact_email, current_node_id, status, exit_reason, entered_at, scheduled_at, context, retry_count, last_error, last_retry_at, max_retries FROM contact_automations WHERE id = $1")).
			WithArgs("ca-1").WillReturnError(sentinel)
		ca, err := repo.GetContactAutomationTx(ctx, nil, "ws-1", "ca-1")
		assert.Nil(t, ca)
		require.ErrorContains(t, err, "failed to get contact automation:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("invalid context JSON is wrapped", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectQuery("FROM contact_automations").WithArgs("ca-1").
			WillReturnRows(sqlmock.NewRows(gapCACols).AddRow(gapCARow("ca-1", []byte(`{bad`), now)...))
		_, err := repo.GetContactAutomationTx(ctx, nil, "ws-1", "ca-1")
		require.ErrorContains(t, err, "failed to unmarshal context:")
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("empty context leaves Context nil and succeeds", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectQuery("FROM contact_automations").WithArgs("ca-1").
			WillReturnRows(sqlmock.NewRows(gapCACols).AddRow(gapCARow("ca-1", []byte{}, now)...))
		ca, err := repo.GetContactAutomationTx(ctx, nil, "ws-1", "ca-1")
		require.NoError(t, err)
		assert.Equal(t, "ca-1", ca.ID)
		assert.Nil(t, ca.Context)
	})
}

func TestGapGetContactAutomationByEmailErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	t.Run("not found uses the exact email-bearing message", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectQuery("FROM contact_automations").WithArgs("auto-1", "ghost@example.com").
			WillReturnRows(sqlmock.NewRows(gapCACols))
		_, err := repo.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "ghost@example.com")
		require.EqualError(t, err, "contact automation not found for email: ghost@example.com")
	})
	t.Run("generic error and unmarshal error are distinctly wrapped", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("timeout")
		mock.ExpectQuery("FROM contact_automations").WillReturnError(sentinel)
		_, err := repo.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "a@example.com")
		require.ErrorContains(t, err, "failed to get contact automation by email:")
		assert.ErrorIs(t, err, sentinel)

		_, mock2, repo2 := setupAutomationMock(t)
		mock2.ExpectQuery("FROM contact_automations").
			WillReturnRows(sqlmock.NewRows(gapCACols).AddRow(gapCARow("ca-1", []byte(`{bad`), now)...))
		_, err = repo2.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "a@example.com")
		require.ErrorContains(t, err, "failed to unmarshal context:")
		_ = mock
	})
}

func TestGapListContactAutomationsRowErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	setup := func(mock sqlmock.Sqlmock, dataRows *sqlmock.Rows) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contact_automations`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("FROM contact_automations").WillReturnRows(dataRows)
	}
	t.Run("scan error aborts listing", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		setup(mock, sqlmock.NewRows([]string{"id"}).AddRow("only-one-col"))
		_, _, err := repo.ListContactAutomations(ctx, "ws-1", domain.ContactAutomationFilter{})
		require.ErrorContains(t, err, "failed to scan contact automation row:")
	})
	t.Run("unmarshal error aborts listing", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		setup(mock, sqlmock.NewRows(gapCACols).AddRow(gapCARow("ca-1", []byte(`{bad`), now)...))
		_, _, err := repo.ListContactAutomations(ctx, "ws-1", domain.ContactAutomationFilter{})
		require.ErrorContains(t, err, "failed to unmarshal context:")
	})
	t.Run("row iteration error is wrapped", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("stream reset")
		setup(mock, sqlmock.NewRows(gapCACols).AddRow(gapCARow("ca-1", []byte(`{}`), now)...).RowError(0, sentinel))
		_, _, err := repo.ListContactAutomations(ctx, "ws-1", domain.ContactAutomationFilter{})
		require.ErrorContains(t, err, "error iterating contact automation rows:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestGapUpdateContactAutomationTxExecErrors(t *testing.T) {
	ctx := context.Background()
	ca := &domain.ContactAutomation{ID: "ca-1", Context: map[string]interface{}{"k": "v"}, MaxRetries: 3}
	t.Run("exec error is wrapped and no success is reported", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("deadlock")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE contact_automations SET current_node_id = $1")).WillReturnError(sentinel)
		err := repo.UpdateContactAutomationTx(ctx, nil, "ws-1", ca)
		require.ErrorContains(t, err, "failed to update contact automation:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("RowsAffected error is wrapped distinctly from not-found", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("driver has no count")
		mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.UpdateContactAutomationTx(ctx, nil, "ws-1", ca)
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestGapUpdateNodeExecutionAndStatsRowsAffected(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("no count available")
	t.Run("node execution RowsAffected error", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec("UPDATE automation_node_executions").WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.UpdateNodeExecutionTx(ctx, nil, "ws-1", &domain.NodeExecution{ID: "ne-1", Output: map[string]interface{}{}})
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})
	t.Run("node execution exec error", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec("UPDATE automation_node_executions").WillReturnError(sentinel)
		err := repo.UpdateNodeExecutionTx(ctx, nil, "ws-1", &domain.NodeExecution{ID: "ne-1", Output: map[string]interface{}{}})
		require.ErrorContains(t, err, "failed to update node execution:")
		assert.ErrorIs(t, err, sentinel)
	})
	t.Run("automation stats RowsAffected error", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec("UPDATE automations SET stats").WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.UpdateAutomationStatsTx(ctx, nil, "ws-1", "auto-1", &domain.AutomationStats{Enrolled: 1})
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})
	t.Run("automation stats exec error", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec("UPDATE automations SET stats").WillReturnError(sentinel)
		err := repo.UpdateAutomationStatsTx(ctx, nil, "ws-1", "auto-1", &domain.AutomationStats{})
		require.ErrorContains(t, err, "failed to update automation stats:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestGapCreateTxExecAndNodeExecUnmarshal(t *testing.T) {
	ctx := context.Background()
	t.Run("CreateTx exec error uses exact wrap and does not succeed silently", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("unique violation")
		mock.ExpectExec("INSERT INTO automations").WillReturnError(sentinel)
		err := repo.CreateTx(ctx, nil, "ws-1", createTestAutomation("auto-1", "ws-1"))
		require.ErrorContains(t, err, "failed to create automation:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("GetNodeExecutions invalid output JSON is wrapped", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		now := time.Now().UTC()
		cols := []string{"id", "contact_automation_id", "node_id", "node_type", "action", "entered_at", "completed_at", "duration_ms", "output", "error"}
		mock.ExpectQuery("FROM automation_node_executions").WithArgs("ca-1").
			WillReturnRows(sqlmock.NewRows(cols).AddRow("ne-1", "ca-1", "n1", "email", "entered", now, nil, nil, []byte(`{bad`), nil))
		_, err := repo.GetNodeExecutions(ctx, "ws-1", "ca-1")
		require.ErrorContains(t, err, "failed to unmarshal output:")
	})
	t.Run("GetNodeExecutions query error is wrapped", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("db gone")
		mock.ExpectQuery("FROM automation_node_executions").WillReturnError(sentinel)
		_, err := repo.GetNodeExecutions(ctx, "ws-1", "ca-1")
		require.ErrorContains(t, err, "failed to get node executions:")
		assert.ErrorIs(t, err, sentinel)
	})
}

// --- contact_postgres.go gaps ---

func gapContactRow(email string, now time.Time) *sqlmock.Rows {
	cols := []string{"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2", "country", "postcode", "state", "job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5", "custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at"}
	return sqlmock.NewRows(cols).AddRow(email, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now, now, now)
}

func TestGapCreateContactIfAbsentErrors(t *testing.T) {
	t.Run("workspace connection failure runs no INSERT", func(t *testing.T) {
		db, mock, cleanup := setupMockDB(t)
		defer cleanup()
		_ = db
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		sentinel := errors.New("no pool")
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel).Times(1)
		repo := NewContactRepository(wsRepo)
		created, err := repo.CreateContactIfAbsent(context.Background(), "ws-1", &domain.Contact{Email: "x@example.com"})
		assert.False(t, created)
		require.ErrorContains(t, err, "failed to get workspace connection:")
		assert.ErrorIs(t, err, sentinel)
		// No Exec expectation was set: any INSERT would fail here.
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("RowsAffected failure is reported, never guessed as created", func(t *testing.T) {
		db, mock, cleanup := setupMockDB(t)
		defer cleanup()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).Times(1)
		sentinel := errors.New("driver cannot count")
		mock.ExpectExec(`INSERT INTO contacts`).WillReturnResult(sqlmock.NewErrorResult(sentinel))
		repo := NewContactRepository(wsRepo)
		created, err := repo.CreateContactIfAbsent(context.Background(), "ws-1", &domain.Contact{Email: "x@example.com"})
		assert.False(t, created)
		require.ErrorContains(t, err, "failed to read contact insert result:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestGapGetBatchForSegmentRowErrors(t *testing.T) {
	newRepo := func(t *testing.T) (*contactRepository, sqlmock.Sqlmock, func()) {
		db, mock, cleanup := setupMockDB(t)
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).AnyTimes()
		return NewContactRepository(wsRepo).(*contactRepository), mock, cleanup
	}
	t.Run("scan error is wrapped with exact args pinned", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t)
		defer cleanup()
		mock.ExpectQuery(`SELECT email FROM contacts ORDER BY email ASC LIMIT \$1 OFFSET \$2`).WithArgs(5, int64(10)).
			WillReturnRows(sqlmock.NewRows([]string{"email", "extra"}).AddRow("a@example.com", "unexpected"))
		_, err := repo.GetBatchForSegment(context.Background(), "ws-1", 10, 5)
		require.ErrorContains(t, err, "failed to scan email:")
	})
	t.Run("iteration error after a good row is wrapped", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t)
		defer cleanup()
		sentinel := errors.New("cursor lost")
		mock.ExpectQuery(`SELECT email`).WithArgs(5, int64(0)).
			WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("a@example.com").RowError(0, sentinel))
		_, err := repo.GetBatchForSegment(context.Background(), "ws-1", 0, 5)
		require.ErrorContains(t, err, "error iterating rows:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestGapFetchContactSegmentAndNotFound(t *testing.T) {
	now := time.Now().UTC()
	newRepo := func(t *testing.T) (domain.ContactRepository, sqlmock.Sqlmock, func()) {
		db, mock, cleanup := setupMockDB(t)
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).AnyTimes()
		return NewContactRepository(wsRepo), mock, cleanup
	}
	primeContactAndLists := func(mock sqlmock.Sqlmock, email string) {
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c`).WithArgs(email).WillReturnRows(gapContactRow(email, now))
		mock.ExpectQuery(`FROM contact_lists`).WithArgs(email).
			WillReturnRows(sqlmock.NewRows([]string{"list_id", "status", "created_at", "updated_at", "deleted_at", "list_name"}))
	}
	t.Run("segments query failure runs after lists and is wrapped", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t)
		defer cleanup()
		primeContactAndLists(mock, "a@example.com")
		sentinel := errors.New("segments down")
		mock.ExpectQuery(`FROM contact_segments`).WithArgs("a@example.com").WillReturnError(sentinel)
		_, err := repo.GetContactByEmail(context.Background(), "ws-1", "a@example.com")
		require.ErrorContains(t, err, "failed to fetch contact segments:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("segment scan error is wrapped", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t)
		defer cleanup()
		primeContactAndLists(mock, "a@example.com")
		mock.ExpectQuery(`FROM contact_segments`).WithArgs("a@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"segment_id"}).AddRow("only-one"))
		_, err := repo.GetContactByEmail(context.Background(), "ws-1", "a@example.com")
		require.ErrorContains(t, err, "failed to scan contact segment:")
	})
	t.Run("missing contact maps to ErrContactNotFound and fetches nothing else", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t)
		defer cleanup()
		cols := []string{"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2", "country", "postcode", "state", "job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5", "custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at"}
		mock.ExpectQuery(`FROM contacts c`).WithArgs("ghost@example.com").WillReturnRows(sqlmock.NewRows(cols))
		_, err := repo.GetContactByEmail(context.Background(), "ws-1", "ghost@example.com")
		require.ErrorIs(t, err, domain.ErrContactNotFound)
		// No list/segment expectations: reaching them would fail the test,
		// proving the guard returns before downstream queries.
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
