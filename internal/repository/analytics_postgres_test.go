package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
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
	sqlMock.ExpectQuery("SELECT (.+) FROM test_schema").WillReturnRows(rows)

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

	sqlMock.ExpectQuery("").WillReturnRows(sqlmock.NewRows(columns))

	_, err = NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "").Query(context.Background(), "workspace-123", q)
	require.NoError(t, err)
	require.NoError(t, sqlMock.ExpectationsWereMet())
	return sent
}

const originCaseSQL = "(CASE WHEN broadcast_id IS NOT NULL THEN 'broadcast' WHEN automation_id IS NOT NULL THEN 'automation' ELSE 'transactional' END)"

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
		"FROM message_history WHERE broadcast_id = $1 AND (opened_at >= $2 AND opened_at <= $3) GROUP BY opened_at_hour", sent)
	assert.NotContains(t, sent, "generate_series", "no filler rows")
}
