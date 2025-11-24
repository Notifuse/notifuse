package repository

import (
	"context"
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

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

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
		GetConnection(gomock.Any(), "workspace-123").
		Return(db, nil)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(42)
	sqlMock.ExpectQuery("SELECT (.+) FROM test_schema").WillReturnRows(rows)

	// Execute test
	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)
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

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)
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
		GetConnection(gomock.Any(), "workspace-123").
		Return(nil, assert.AnError)

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)
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

	repo := NewAnalyticsRepository(mockWorkspaceRepo, mockLogger)

	schemas, err := repo.GetSchemas(context.Background(), "workspace-123")

	assert.NoError(t, err)
	assert.Len(t, schemas, 2)
	assert.Contains(t, schemas, "schema1")
	assert.Contains(t, schemas, "schema2")
}
