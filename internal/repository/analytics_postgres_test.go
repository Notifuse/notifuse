package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAnalyticsRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	assert.NotNil(t, repo)
	assert.IsType(t, &analyticsRepository{}, repo)
}

func TestAnalyticsRepository_Query_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	// Setup workspace repository mock
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "test-workspace").Return(db, nil)

	// Setup database expectations - the SQL builder wraps the count in parentheses
	mock.ExpectQuery("SELECT \\(COUNT\\(\\*\\)\\) AS count FROM message_history").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

	// Create repository
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	// Execute query
	ctx := context.Background()
	query := analytics.Query{
		Schema:   "message_history",
		Measures: []string{"count"},
	}
	response, err := repo.Query(ctx, "test-workspace", query)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Data, 1)
	assert.Equal(t, int64(42), response.Data[0]["count"])
	assert.Contains(t, response.Meta.Query, "SELECT (COUNT(*)) AS count FROM message_history")

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsRepository_Query_WithDimensions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	// Setup workspace repository mock
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "test-workspace").Return(db, nil)

	// Setup database expectations - the SQL builder puts measures first, then dimensions
	mock.ExpectQuery("SELECT \\(COUNT\\(\\*\\) FILTER \\(WHERE sent_at IS NOT NULL\\)\\) AS count_sent, channel AS channel FROM message_history GROUP BY channel").
		WillReturnRows(sqlmock.NewRows([]string{"count_sent", "channel"}).
			AddRow(int64(30), "email").
			AddRow(int64(12), "sms"))

	// Create repository
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	// Execute query
	ctx := context.Background()
	query := analytics.Query{
		Schema:     "message_history",
		Measures:   []string{"count_sent"},
		Dimensions: []string{"channel"},
	}
	response, err := repo.Query(ctx, "test-workspace", query)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Data, 2)

	expectedData := []map[string]interface{}{
		{"count_sent": int64(30), "channel": "email"},
		{"count_sent": int64(12), "channel": "sms"},
	}
	assert.Equal(t, expectedData, response.Data)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsRepository_Query_InvalidSchema(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	// Create repository
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	// Execute query with invalid schema
	ctx := context.Background()
	query := analytics.Query{
		Schema:   "nonexistent_schema",
		Measures: []string{"count"},
	}
	response, err := repo.Query(ctx, "test-workspace", query)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown schema: nonexistent_schema")
	assert.Nil(t, response)
}

func TestAnalyticsRepository_Query_DatabaseConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	// Setup workspace repository mock to return error
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "test-workspace").
		Return((*sql.DB)(nil), assert.AnError)

	// Create repository
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	// Execute query
	ctx := context.Background()
	query := analytics.Query{
		Schema:   "message_history",
		Measures: []string{"count"},
	}
	response, err := repo.Query(ctx, "test-workspace", query)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection:")
	assert.Nil(t, response)
}

func TestAnalyticsRepository_Query_SQLExecutionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	// Setup workspace repository mock
	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "test-workspace").Return(db, nil)

	// Setup database expectations to return error
	mock.ExpectQuery("SELECT \\(COUNT\\(\\*\\)\\) AS count FROM message_history").
		WillReturnError(sql.ErrConnDone)

	// Create repository
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	// Execute query
	ctx := context.Background()
	query := analytics.Query{
		Schema:   "message_history",
		Measures: []string{"count"},
	}
	response, err := repo.Query(ctx, "test-workspace", query)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute query:")
	assert.Nil(t, response)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsRepository_Query_ValidationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()

	// Create repository
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	// Execute query with invalid measure
	ctx := context.Background()
	query := analytics.Query{
		Schema:   "message_history",
		Measures: []string{"invalid_measure"},
	}
	response, err := repo.Query(ctx, "test-workspace", query)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "query validation failed:")
	assert.Nil(t, response)
}

func TestAnalyticsRepository_GetSchemas(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := logger.NewLogger()
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	schemas, err := repo.GetSchemas(ctx, "test-workspace")

	assert.NoError(t, err)
	assert.NotNil(t, schemas)

	// Verify we get the predefined schemas
	assert.Contains(t, schemas, "message_history")
	assert.Contains(t, schemas, "contacts")
	assert.Contains(t, schemas, "broadcasts")

	// Verify schema structure
	messageHistorySchema := schemas["message_history"]
	assert.Equal(t, "message_history", messageHistorySchema.Name)
	assert.Contains(t, messageHistorySchema.Measures, "count")
	assert.Contains(t, messageHistorySchema.Measures, "count_sent")
	assert.Contains(t, messageHistorySchema.Dimensions, "created_at")
	assert.Contains(t, messageHistorySchema.Dimensions, "channel")
}
