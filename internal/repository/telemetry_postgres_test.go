package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

// setupTelemetryMockDB creates a mock database and sqlmock for testing
func setupTelemetryMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err, "Failed to create mock database")

	cleanup := func() {
		db.Close()
	}

	return db, mock, cleanup
}

func TestNewTelemetryRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	repo := NewTelemetryRepository(workspaceRepo)

	assert.NotNil(t, repo)
	assert.IsType(t, &telemetryRepository{}, repo)
}

func TestGetWorkspaceMetrics_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceDB, workspaceMock, workspaceCleanup := setupTelemetryMockDB(t)
	defer workspaceCleanup()

	systemDB, systemMock, systemCleanup := setupTelemetryMockDB(t)
	defer systemCleanup()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(workspaceDB, nil)
	workspaceRepo.EXPECT().GetSystemConnection(gomock.Any()).Return(systemDB, nil)

	repo := NewTelemetryRepository(workspaceRepo)

	// Mock all count queries
	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(100))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM broadcasts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(25))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(50))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1500))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM lists`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM segments`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))

	systemMock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE workspace_id = \$1 AND deleted_at IS NULL`).
		WithArgs("workspace123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	lastMessageTime := time.Now().UTC()
	workspaceMock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(lastMessageTime))

	ctx := context.Background()
	metrics, err := repo.GetWorkspaceMetrics(ctx, "workspace123")

	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, 100, metrics.ContactsCount)
	assert.Equal(t, 25, metrics.BroadcastsCount)
	assert.Equal(t, 50, metrics.TransactionalCount)
	assert.Equal(t, 1500, metrics.MessagesCount)
	assert.Equal(t, 10, metrics.ListsCount)
	assert.Equal(t, 8, metrics.SegmentsCount)
	assert.Equal(t, 3, metrics.UsersCount)
	assert.Equal(t, lastMessageTime.Format(time.RFC3339), metrics.LastMessageAt)

	// Verify all expectations were met
	require.NoError(t, workspaceMock.ExpectationsWereMet())
	require.NoError(t, systemMock.ExpectationsWereMet())
}

func TestGetWorkspaceMetrics_WorkspaceConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").
		Return(nil, errors.New("connection failed"))

	repo := NewTelemetryRepository(workspaceRepo)

	ctx := context.Background()
	metrics, err := repo.GetWorkspaceMetrics(ctx, "workspace123")

	assert.Error(t, err)
	assert.Nil(t, metrics)
	assert.Contains(t, err.Error(), "failed to get workspace database connection")
}

func TestGetWorkspaceMetrics_SystemConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceDB, _, workspaceCleanup := setupTelemetryMockDB(t)
	defer workspaceCleanup()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(workspaceDB, nil)
	workspaceRepo.EXPECT().GetSystemConnection(gomock.Any()).
		Return(nil, errors.New("system connection failed"))

	repo := NewTelemetryRepository(workspaceRepo)

	ctx := context.Background()
	metrics, err := repo.GetWorkspaceMetrics(ctx, "workspace123")

	assert.Error(t, err)
	assert.Nil(t, metrics)
	assert.Contains(t, err.Error(), "failed to get system database connection")
}

func TestGetWorkspaceMetrics_PartialFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceDB, workspaceMock, workspaceCleanup := setupTelemetryMockDB(t)
	defer workspaceCleanup()

	systemDB, systemMock, systemCleanup := setupTelemetryMockDB(t)
	defer systemCleanup()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(workspaceDB, nil)
	workspaceRepo.EXPECT().GetSystemConnection(gomock.Any()).Return(systemDB, nil)

	repo := NewTelemetryRepository(workspaceRepo)

	// Mock some successful queries and some failures
	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(100))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM broadcasts`).
		WillReturnError(errors.New("broadcasts query failed"))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(50))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history`).
		WillReturnError(errors.New("message history query failed"))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM lists`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM segments`).
		WillReturnError(errors.New("segments query failed"))

	systemMock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE workspace_id = \$1 AND deleted_at IS NULL`).
		WithArgs("workspace123").
		WillReturnError(errors.New("users query failed"))

	workspaceMock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(time.Now().UTC()))

	ctx := context.Background()
	metrics, err := repo.GetWorkspaceMetrics(ctx, "workspace123")

	// Should not return error even if individual queries fail
	require.NoError(t, err)
	assert.NotNil(t, metrics)

	// Only successful queries should have values
	assert.Equal(t, 100, metrics.ContactsCount)
	assert.Equal(t, 0, metrics.BroadcastsCount) // Failed query, should be 0
	assert.Equal(t, 50, metrics.TransactionalCount)
	assert.Equal(t, 0, metrics.MessagesCount) // Failed query, should be 0
	assert.Equal(t, 10, metrics.ListsCount)
	assert.Equal(t, 0, metrics.SegmentsCount) // Failed query, should be 0
	assert.Equal(t, 0, metrics.UsersCount) // Failed query, should be 0
}

func TestCountContacts_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedCount := 150
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

	ctx := context.Background()
	count, err := repo.CountContacts(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountContacts_Error(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	count, err := repo.CountContacts(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count contacts")
}

func TestCountBroadcasts_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedCount := 42
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM broadcasts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

	ctx := context.Background()
	count, err := repo.CountBroadcasts(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountBroadcasts_Error(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM broadcasts`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	count, err := repo.CountBroadcasts(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count broadcasts")
}

func TestCountTransactional_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedCount := 75
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

	ctx := context.Background()
	count, err := repo.CountTransactional(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountTransactional_Error(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	count, err := repo.CountTransactional(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count transactional notifications")
}

func TestCountMessages_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedCount := 2500
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

	ctx := context.Background()
	count, err := repo.CountMessages(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountMessages_Error(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	count, err := repo.CountMessages(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count messages")
}

func TestCountLists_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedCount := 15
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM lists`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

	ctx := context.Background()
	count, err := repo.CountLists(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountLists_Error(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM lists`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	count, err := repo.CountLists(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count lists")
}

func TestCountSegments_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedCount := 12
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM segments`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

	ctx := context.Background()
	count, err := repo.CountSegments(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountSegments_Error(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM segments`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	count, err := repo.CountSegments(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count segments")
}

func TestCountUsers_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedCount := 5
	workspaceID := "workspace123"
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE workspace_id = \$1 AND deleted_at IS NULL`).
		WithArgs(workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

	ctx := context.Background()
	count, err := repo.CountUsers(ctx, db, workspaceID)

	require.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountUsers_Error(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	workspaceID := "workspace123"
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE workspace_id = \$1 AND deleted_at IS NULL`).
		WithArgs(workspaceID).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	count, err := repo.CountUsers(ctx, db, workspaceID)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count users")
}

func TestGetLastMessageAt_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	expectedTime := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(expectedTime))

	ctx := context.Background()
	lastMessageAt, err := repo.GetLastMessageAt(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, expectedTime.Format(time.RFC3339), lastMessageAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLastMessageAt_NoRows(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnError(sql.ErrNoRows)

	ctx := context.Background()
	lastMessageAt, err := repo.GetLastMessageAt(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, "", lastMessageAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLastMessageAt_NullValue(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	// Return a null value
	mock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(nil))

	ctx := context.Background()
	lastMessageAt, err := repo.GetLastMessageAt(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, "", lastMessageAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLastMessageAt_DatabaseError(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo)

	mock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	lastMessageAt, err := repo.GetLastMessageAt(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, "", lastMessageAt)
	assert.Contains(t, err.Error(), "failed to get last message timestamp")
}

func TestGetSystemConnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, _, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetSystemConnection(gomock.Any()).Return(db, nil)

	repo := &telemetryRepository{workspaceRepo: workspaceRepo}

	ctx := context.Background()
	result, err := repo.getSystemConnection(ctx)

	require.NoError(t, err)
	assert.Equal(t, db, result)
}

func TestGetSystemConnection_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedError := errors.New("system connection failed")
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetSystemConnection(gomock.Any()).Return(nil, expectedError)

	repo := &telemetryRepository{workspaceRepo: workspaceRepo}

	ctx := context.Background()
	result, err := repo.getSystemConnection(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
}

// Test edge cases and integration scenarios
func TestGetWorkspaceMetrics_EmptyDatabase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceDB, workspaceMock, workspaceCleanup := setupTelemetryMockDB(t)
	defer workspaceCleanup()

	systemDB, systemMock, systemCleanup := setupTelemetryMockDB(t)
	defer systemCleanup()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(workspaceDB, nil)
	workspaceRepo.EXPECT().GetSystemConnection(gomock.Any()).Return(systemDB, nil)

	repo := NewTelemetryRepository(workspaceRepo)

	// Mock all queries returning 0 counts
	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM broadcasts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM lists`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM segments`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	systemMock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces WHERE workspace_id = \$1 AND deleted_at IS NULL`).
		WithArgs("workspace123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// No messages, so return no rows for last message query
	workspaceMock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnError(sql.ErrNoRows)

	ctx := context.Background()
	metrics, err := repo.GetWorkspaceMetrics(ctx, "workspace123")

	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, 0, metrics.ContactsCount)
	assert.Equal(t, 0, metrics.BroadcastsCount)
	assert.Equal(t, 0, metrics.TransactionalCount)
	assert.Equal(t, 0, metrics.MessagesCount)
	assert.Equal(t, 0, metrics.ListsCount)
	assert.Equal(t, 0, metrics.SegmentsCount)
	assert.Equal(t, 0, metrics.UsersCount)
	assert.Equal(t, "", metrics.LastMessageAt)

	// Verify all expectations were met
	require.NoError(t, workspaceMock.ExpectationsWereMet())
	require.NoError(t, systemMock.ExpectationsWereMet())
}
