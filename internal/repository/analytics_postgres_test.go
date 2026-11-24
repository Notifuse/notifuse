package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func TestNewAnalyticsRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger, "")

	assert.NotNil(t, repo)
	assert.Implements(t, (*domain.AnalyticsRepository)(nil), repo)
}

func TestAnalyticsRepository_Query_Success(t *testing.T) {
	// Set up test schema
	originalSchemas := domain.PredefinedSchemas
	domain.PredefinedSchemas = map[string]analytics.SchemaDefinition{
		"test_schema": {
			Name: "test_schema",
			Measures: map[string]analytics.MeasureDefinition{
				"count": {Type: "count", SQL: "COUNT(*)"},
			},
		},
	}
	defer func() { domain.PredefinedSchemas = originalSchemas }()

	// Create mock database
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Set up mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace-123").
		Return(&domain.Workspace{ID: "workspace-123"}, nil).
		AnyTimes()
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace-123").
		Return(db, nil)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(42)
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("SET LOCAL statement_timeout = 60000").WillReturnResult(sqlmock.NewResult(0, 0))
	sqlMock.ExpectQuery("SELECT (.+) FROM test_schema").WillReturnRows(rows)
	sqlMock.ExpectCommit()

	// Execute test
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger, "")
	query := analytics.Query{
		Schema:   "test_schema",
		Measures: []string{"count"},
	}

	response, err := repo.Query(context.Background(), "workspace-123", query)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Data, 1)
	assert.Equal(t, int64(42), response.Data[0]["count"])
	assert.NotEmpty(t, response.Meta.Query)
}

func TestAnalyticsRepository_Query_UnknownSchema(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace-123").
		Return(&domain.Workspace{ID: "workspace-123"}, nil).
		AnyTimes()

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger, "")
	query := analytics.Query{
		Schema:   "unknown_schema",
		Measures: []string{"count"},
	}

	response, err := repo.Query(context.Background(), "workspace-123", query)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "unknown schema")
}

func TestAnalyticsRepository_Query_DatabaseError(t *testing.T) {
	// Set up test schema
	originalSchemas := domain.PredefinedSchemas
	domain.PredefinedSchemas = map[string]analytics.SchemaDefinition{
		"test_schema": {
			Name: "test_schema",
			Measures: map[string]analytics.MeasureDefinition{
				"count": {Type: "count", SQL: "COUNT(*)"},
			},
		},
	}
	defer func() { domain.PredefinedSchemas = originalSchemas }()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace-123").
		Return(&domain.Workspace{ID: "workspace-123"}, nil).
		AnyTimes()
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace-123").
		Return(nil, assert.AnError)

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger, "")
	query := analytics.Query{
		Schema:   "test_schema",
		Measures: []string{"count"},
	}

	response, err := repo.Query(context.Background(), "workspace-123", query)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to get database connection")
}

func TestAnalyticsRepository_GetSchemas(t *testing.T) {
	// Set up test schemas
	originalSchemas := domain.PredefinedSchemas
	testSchemas := map[string]analytics.SchemaDefinition{
		"schema1": {Name: "table1"},
		"schema2": {Name: "table2"},
	}
	domain.PredefinedSchemas = testSchemas
	defer func() { domain.PredefinedSchemas = originalSchemas }()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace-123").
		Return(&domain.Workspace{ID: "workspace-123"}, nil).
		AnyTimes()

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger, "")

	schemas, err := repo.GetSchemas(context.Background(), "workspace-123")

	assert.NoError(t, err)
	assert.Len(t, schemas, 2)
	assert.Contains(t, schemas, "schema1")
	assert.Contains(t, schemas, "schema2")
}

// runMessageHistoryQuery runs q against the real message_history schema on a mocked
// database and returns the SQL the repository sent.
func runMessageHistoryQuery(t *testing.T, q analytics.Query, columns []string) string {
	t.Helper()
	var sent string
	db, sqlMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, actual string) error {
		sent = actual
		return nil
	})))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").
		Return(&domain.Workspace{ID: "workspace-123"}, nil).AnyTimes()
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)

	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
	sqlMock.ExpectQuery("").WillReturnRows(sqlmock.NewRows(columns))
	sqlMock.ExpectCommit()

	_, err = NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "").Query(context.Background(), "workspace-123", q)
	require.NoError(t, err)
	require.NoError(t, sqlMock.ExpectationsWereMet())
	return sent
}

const originCaseSQL = "(CASE WHEN broadcast_id IS NOT NULL THEN 'broadcast' WHEN automation_id IS NOT NULL THEN 'automation' WHEN transactional_notification_id IS NOT NULL THEN 'transactional' ELSE 'other' END)"

func TestAnalyticsRepository_MessageHistoryOriginFilter(t *testing.T) {
	sent := runMessageHistoryQuery(t, analytics.Query{
		Schema:   "message_history",
		Measures: []string{"count_sent"},
		Filters:  []analytics.Filter{{Member: "origin", Operator: "equals", Values: []string{"automation"}}},
	}, []string{"count_sent"})

	// The query's own WHERE, after FROM: the measures' FILTER (WHERE ...) come before it.
	afterFrom := sent[strings.Index(sent, " FROM "):]
	require.Contains(t, afterFrom, "WHERE")
	where := afterFrom[strings.Index(afterFrom, "WHERE"):]
	assert.Contains(t, where, originCaseSQL)
}

func TestAnalyticsRepository_MessageHistoryOriginDimension(t *testing.T) {
	sent := runMessageHistoryQuery(t, analytics.Query{
		Schema:     "message_history",
		Measures:   []string{"count_sent"},
		Dimensions: []string{"origin"},
	}, []string{"origin", "count_sent"})

	selectPart := sent[:strings.Index(sent, "FROM")]
	groupBy := sent[strings.Index(sent, "GROUP BY"):]
	assert.Contains(t, selectPart, originCaseSQL)
	assert.Contains(t, groupBy, originCaseSQL)
}

func TestAnalyticsRepository_MessageHistoryLastSentAt(t *testing.T) {
	sent := runMessageHistoryQuery(t, analytics.Query{
		Schema:     "message_history",
		Measures:   []string{"last_sent_at"},
		Dimensions: []string{"transactional_notification_id"},
		Filters:    []analytics.Filter{{Member: "transactional_notification_id", Operator: "in", Values: []string{"welcome", "reset"}}},
	}, []string{"transactional_notification_id", "last_sent_at"})

	assert.Contains(t, sent, "MAX(sent_at) FILTER (WHERE failed_at IS NULL)")
}

// Opens of one broadcast by local hour: the console sends the range as a filter, not as the
// time dimension's dateRange, so the series has no filler row.
func TestAnalyticsRepository_MessageHistoryOpenedAtByHour(t *testing.T) {
	timezone := "Europe/Paris"
	sent := runMessageHistoryQuery(t, analytics.Query{
		Schema:         "message_history",
		Measures:       []string{"count"},
		Timezone:       &timezone,
		TimeDimensions: []analytics.TimeDimension{{Dimension: "opened_at", Granularity: "hour"}},
		Filters: []analytics.Filter{
			{Member: "broadcast_id", Operator: "equals", Values: []string{"b1"}},
			{Member: "opened_at", Operator: "inDateRange", Values: []string{"2026-11-01T00:00:00+01:00", "2026-11-03T23:59:59+01:00"}},
		},
	}, []string{"opened_at_hour", "count"})

	assert.Equal(t, "SELECT (COUNT(*)) AS count, (DATE_TRUNC('hour', opened_at AT TIME ZONE 'Europe/Paris')) AS opened_at_hour "+
		"FROM message_history WHERE broadcast_id = $1 AND (opened_at >= $2 AND opened_at <= $3) GROUP BY opened_at_hour LIMIT 50001", sent)
	assert.NotContains(t, sent, "generate_series", "no filler rows")
}

// Every query runs in a transaction bounded by the statement timeout, whatever the work_mem
// setting, and the errors its caller can fix come back as analytics.QueryError.
func TestAnalyticsRepository_StatementTimeoutAndCallerErrors(t *testing.T) {
	originalSchemas := domain.PredefinedSchemas
	domain.PredefinedSchemas = map[string]analytics.SchemaDefinition{
		"test_schema": {
			Name:     "test_schema",
			Measures: map[string]analytics.MeasureDefinition{"count": {Type: "count", SQL: "COUNT(*)"}},
		},
	}
	defer func() { domain.PredefinedSchemas = originalSchemas }()

	run := func(t *testing.T, queryErr error) error {
		t.Helper()
		db, sqlMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").
			Return(&domain.Workspace{ID: "workspace-123"}, nil).AnyTimes()
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)

		sqlMock.ExpectBegin()
		sqlMock.ExpectExec("SET LOCAL statement_timeout = 60000").WillReturnResult(sqlmock.NewResult(0, 0))
		sqlMock.ExpectQuery("SELECT").WillReturnError(queryErr)
		sqlMock.ExpectRollback()

		_, err = NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "").
			Query(context.Background(), "workspace-123", analytics.Query{Schema: "test_schema", Measures: []string{"count"}})
		require.NoError(t, sqlMock.ExpectationsWereMet())
		return err
	}

	t.Run("a statement past the timeout says to narrow the query", func(t *testing.T) {
		err := run(t, &pq.Error{Code: "57014", Message: "canceling statement due to statement timeout"})
		var queryErr *analytics.QueryError
		require.True(t, errors.As(err, &queryErr), "%v", err)
		assert.Equal(t, "the query ran past 1m0s: narrow its date range or filters", queryErr.Message)
	})

	t.Run("a value the column type cannot read is the caller's, with a fixed message", func(t *testing.T) {
		for _, cause := range []*pq.Error{
			{Code: "22P02", Message: `invalid input syntax for type integer: "abc"`},
			{Code: "22007", Message: `invalid input syntax for type timestamp with time zone: "soon"`},
			{Code: "22008", Message: `date/time field value out of range: "2026-13-45"`},
		} {
			err := run(t, cause)
			var queryErr *analytics.QueryError
			require.True(t, errors.As(err, &queryErr), "%s: %v", cause.Code, err)
			assert.Equal(t, "a value of the query's filters or having does not fit its member's type: check its numbers, dates and times", queryErr.Message)
			assert.NotContains(t, queryErr.Message, cause.Message)
		}
	})

	t.Run("a division by zero, a numeric overflow and other class 22 errors stay ours", func(t *testing.T) {
		for _, cause := range []*pq.Error{
			{Code: "22012", Message: "division by zero"},
			{Code: "22003", Message: "bigint out of range"},
			{Code: "22001", Message: "value too long for type character varying(10)"},
		} {
			err := run(t, cause)
			var queryErr *analytics.QueryError
			assert.False(t, errors.As(err, &queryErr), "%s: %v", cause.Code, err)
		}
	})

	t.Run("a cancelled request and any other error stay ours", func(t *testing.T) {
		for _, cause := range []error{
			&pq.Error{Code: "57014", Message: "canceling statement due to user request"},
			&pq.Error{Code: "42P01", Message: `relation "test_schema" does not exist`},
		} {
			err := run(t, cause)
			var queryErr *analytics.QueryError
			assert.False(t, errors.As(err, &queryErr), "%v", err)
		}
	})
}
