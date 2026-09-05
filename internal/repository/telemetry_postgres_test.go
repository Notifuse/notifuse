package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	_ "github.com/lib/pq"
)

// setupTelemetryMockDB creates a mock database and sqlmock for testing
func setupTelemetryMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err, "Failed to create mock database")

	cleanup := func() {
		_ = db.Close()
	}

	return db, mock, cleanup
}

func TestNewTelemetryRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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

	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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

	systemMock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces uw JOIN users u ON uw\.user_id = u\.id WHERE uw\.workspace_id = \$1 AND u\.type != 'api_key'`).
		WithArgs("workspace123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	lastMessageTime := time.Now().UTC()
	workspaceMock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(lastMessageTime))

	lastWebSessionDate := time.Now().UTC().Truncate(24 * time.Hour)
	workspaceMock.ExpectQuery(`SELECT MAX\(session_date\) FROM web_sessions`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(lastWebSessionDate))

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
	assert.Equal(t, lastWebSessionDate.Format(time.RFC3339), metrics.LastWebSessionAt)

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

	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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

	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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

	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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

	systemMock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces uw JOIN users u ON uw\.user_id = u\.id WHERE uw\.workspace_id = \$1 AND u\.type != 'api_key'`).
		WithArgs("workspace123").
		WillReturnError(errors.New("users query failed"))

	workspaceMock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(time.Now().UTC()))

	// A workspace database that predates the web analytics tables answers with
	// "relation does not exist"; the payload must survive it.
	workspaceMock.ExpectQuery(`SELECT MAX\(session_date\) FROM web_sessions`).
		WillReturnError(errors.New(`pq: relation "web_sessions" does not exist`))

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
	assert.Equal(t, 0, metrics.SegmentsCount)     // Failed query, should be 0
	assert.Equal(t, 0, metrics.UsersCount)        // Failed query, should be 0
	assert.Equal(t, "", metrics.LastWebSessionAt) // Failed query, should be empty

	// Without this, the expectations above are decorative: the assertions on
	// failed queries all read back a zero value, which a query that was never
	// issued produces just as well.
	require.NoError(t, workspaceMock.ExpectationsWereMet())
	require.NoError(t, systemMock.ExpectationsWereMet())
}

func TestCountContacts_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

	expectedCount := 5
	workspaceID := "workspace123"
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces uw JOIN users u ON uw\.user_id = u\.id WHERE uw\.workspace_id = \$1 AND u\.type != 'api_key'`).
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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

	workspaceID := "workspace123"
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces uw JOIN users u ON uw\.user_id = u\.id WHERE uw\.workspace_id = \$1 AND u\.type != 'api_key'`).
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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

	mock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnError(errors.New("database error"))

	ctx := context.Background()
	lastMessageAt, err := repo.GetLastMessageAt(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, "", lastMessageAt)
	assert.Contains(t, err.Error(), "failed to get last message timestamp")
}

func TestGetLastWebSessionAt_Success(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

	// session_date is a DATE, so the driver hands back a midnight UTC instant.
	expectedDate := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT MAX\(session_date\) FROM web_sessions`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(expectedDate))

	ctx := context.Background()
	lastWebSessionAt, err := repo.GetLastWebSessionAt(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, "2026-08-14T00:00:00Z", lastWebSessionAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLastWebSessionAt_NoSessions(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

	// MAX() over an empty table returns one row holding NULL, never ErrNoRows.
	mock.ExpectQuery(`SELECT MAX\(session_date\) FROM web_sessions`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))

	ctx := context.Background()
	lastWebSessionAt, err := repo.GetLastWebSessionAt(ctx, db)

	require.NoError(t, err)
	assert.Equal(t, "", lastWebSessionAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLastWebSessionAt_MissingTable(t *testing.T) {
	db, mock, cleanup := setupTelemetryMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

	// A workspace database created before the web analytics tables existed.
	mock.ExpectQuery(`SELECT MAX\(session_date\) FROM web_sessions`).
		WillReturnError(errors.New(`pq: relation "web_sessions" does not exist`))

	ctx := context.Background()
	lastWebSessionAt, err := repo.GetLastWebSessionAt(ctx, db)

	assert.Error(t, err)
	assert.Equal(t, "", lastWebSessionAt)
	assert.Contains(t, err.Error(), "failed to get last web session date")
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

	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

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

	systemMock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces uw JOIN users u ON uw\.user_id = u\.id WHERE uw\.workspace_id = \$1 AND u\.type != 'api_key'`).
		WithArgs("workspace123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// No messages, so return no rows for last message query
	workspaceMock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnError(sql.ErrNoRows)

	// The tables exist but hold no sessions: MAX over no rows is one NULL row.
	workspaceMock.ExpectQuery(`SELECT MAX\(session_date\) FROM web_sessions`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))

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
	assert.Equal(t, "", metrics.LastWebSessionAt)

	// Verify all expectations were met
	require.NoError(t, workspaceMock.ExpectationsWereMet())
	require.NoError(t, systemMock.ExpectationsWereMet())
}

// telemetryTestDSN builds a connection string for the throwaway Postgres the
// schema-backed tests below need. It reads the same TEST_DB_* variables the
// integration harness uses, so a developer who already has `make
// test-integration` working has nothing extra to configure.
func telemetryTestDSN(database string) string {
	value := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		value("TEST_DB_HOST", "localhost"),
		value("TEST_DB_PORT", "5433"),
		value("TEST_DB_USER", "notifuse_test"),
		value("TEST_DB_PASSWORD", "test_password"),
		database,
	)
}

// setupTelemetrySchemaDB creates a throwaway database, applies the real system
// schema to it and hands back a connection.
//
// It uses schema.TableDefinitions rather than a hand-written CREATE TABLE so the
// test cannot drift from what installations actually run — which is the whole
// point of it existing. sqlmock validates nothing about column names, so a query
// naming a column that has never existed passes every mock-backed test in this
// file; only a real Postgres says no.
func setupTelemetrySchemaDB(t *testing.T) *sql.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("schema-backed telemetry test needs a Postgres; skipped in short mode")
	}

	admin, err := sql.Open("postgres", telemetryTestDSN("postgres"))
	if err != nil {
		t.Skipf("no Postgres available for the schema-backed telemetry test: %v", err)
	}
	defer func() { _ = admin.Close() }()

	if err := admin.Ping(); err != nil {
		t.Skipf("no Postgres available for the schema-backed telemetry test: %v", err)
	}

	name := fmt.Sprintf("notifuse_telemetry_schema_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE DATABASE " + name)
	require.NoError(t, err, "failed to create the throwaway database")

	db, err := sql.Open("postgres", telemetryTestDSN(name))
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = db.Close()
		dropper, err := sql.Open("postgres", telemetryTestDSN("postgres"))
		if err != nil {
			return
		}
		defer func() { _ = dropper.Close() }()
		_, _ = dropper.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
	})

	require.NoError(t, db.Ping())

	for _, statement := range schema.TableDefinitions {
		_, err := db.Exec(statement)
		require.NoErrorf(t, err, "failed to apply system table definition: %s", statement)
	}
	for _, statement := range schema.MigrationStatements {
		_, err := db.Exec(statement)
		require.NoErrorf(t, err, "failed to apply system migration statement: %s", statement)
	}

	return db
}

// TestCountUsers_AgainstRealSystemSchema runs CountUsers against the system
// schema installations actually run.
//
// This is the test that was missing. The previous query filtered on
// user_workspaces.deleted_at, a column no CREATE TABLE and no migration has ever
// added; Postgres rejected it on every single run, GetWorkspaceMetrics swallowed
// the error into `if err == nil`, and users_count was reported as zero for every
// workspace of every installation since the field was introduced. Every
// sqlmock-backed test above passed throughout, because sqlmock matches the query
// text against a regex the same test wrote and never asks a database whether the
// columns exist.
func TestCountUsers_AgainstRealSystemSchema(t *testing.T) {
	db := setupTelemetrySchemaDB(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewTelemetryRepository(workspaceRepo, permissiveRepoLogger(ctrl))

	now := time.Now().UTC()
	_, err := db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ($1, $2, $3, $3), ($4, $5, $3, $3)`,
		"ws_counted", "Counted", now, "ws_other", "Other")
	require.NoError(t, err)

	// Two humans and one API key on the workspace under test, plus one human on a
	// neighbouring workspace that must not be counted.
	seed := []struct {
		id          string
		userType    string
		email       string
		workspaceID string
		role        string
	}{
		{"11111111-1111-1111-1111-111111111111", "user", "owner@example.com", "ws_counted", "owner"},
		{"22222222-2222-2222-2222-222222222222", "user", "member@example.com", "ws_counted", "member"},
		{"33333333-3333-3333-3333-333333333333", "api_key", "key@example.com", "ws_counted", "member"},
		{"44444444-4444-4444-4444-444444444444", "user", "elsewhere@example.com", "ws_other", "owner"},
	}
	for _, row := range seed {
		_, err := db.Exec(
			`INSERT INTO users (id, type, email, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`,
			row.id, row.userType, row.email, now)
		require.NoError(t, err)
		_, err = db.Exec(
			`INSERT INTO user_workspaces (user_id, workspace_id, role, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`,
			row.id, row.workspaceID, row.role, now)
		require.NoError(t, err)
	}

	ctx := context.Background()

	t.Run("the query runs against the real schema", func(t *testing.T) {
		// The old query failed here with `pq: column "deleted_at" does not exist`.
		count, err := repo.CountUsers(ctx, db, "ws_counted")
		require.NoError(t, err)
		assert.Equal(t, 2, count, "two humans, and the api_key row is not a user")
	})

	t.Run("a workspace with no members counts zero", func(t *testing.T) {
		count, err := repo.CountUsers(ctx, db, "ws_empty")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("members of another workspace are not counted", func(t *testing.T) {
		count, err := repo.CountUsers(ctx, db, "ws_other")
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

// TestGetWorkspaceMetrics_LogsMetricFailure pins the other half of the users_count
// defect: the collection loop used to drop every per-metric error into `if err ==
// nil`, so a permanently broken query looked exactly like a workspace that
// genuinely has none of whatever was being counted.
func TestGetWorkspaceMetrics_LogsMetricFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceDB, workspaceMock, workspaceCleanup := setupTelemetryMockDB(t)
	defer workspaceCleanup()

	systemDB, systemMock, systemCleanup := setupTelemetryMockDB(t)
	defer systemCleanup()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(workspaceDB, nil)
	workspaceRepo.EXPECT().GetSystemConnection(gomock.Any()).Return(systemDB, nil)

	var logged []map[string]interface{}
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithFields(gomock.Any()).DoAndReturn(func(fields map[string]interface{}) logger.Logger {
		logged = append(logged, fields)
		return log
	}).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()

	repo := NewTelemetryRepository(workspaceRepo, log)

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
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

	// The failure under test: exactly the shape the deleted_at predicate produced.
	systemMock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_workspaces uw JOIN users u ON uw\.user_id = u\.id WHERE uw\.workspace_id = \$1 AND u\.type != 'api_key'`).
		WithArgs("workspace123").
		WillReturnError(errors.New(`pq: column "deleted_at" does not exist`))

	workspaceMock.ExpectQuery(`SELECT COUNT\(\*\) FROM blog_posts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	workspaceMock.ExpectQuery(`SELECT created_at FROM message_history\s+WHERE created_at IS NOT NULL\s+ORDER BY created_at DESC, id DESC\s+LIMIT 1`).
		WillReturnError(sql.ErrNoRows)
	workspaceMock.ExpectQuery(`SELECT MAX\(session_date\) FROM web_sessions`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))

	metrics, err := repo.GetWorkspaceMetrics(context.Background(), "workspace123")

	// The payload still goes out — one broken metric must never cost the report.
	require.NoError(t, err)
	require.NotNil(t, metrics)
	assert.Equal(t, 7, metrics.ContactsCount)
	assert.Equal(t, 0, metrics.UsersCount)

	require.Len(t, logged, 1, "exactly the failing metric should be reported")
	assert.Equal(t, "users_count", logged[0]["metric"])
	assert.Equal(t, "workspace123", logged[0]["workspace_id"])
	assert.Contains(t, logged[0]["error"], "failed to count users")

	require.NoError(t, workspaceMock.ExpectationsWereMet())
	require.NoError(t, systemMock.ExpectationsWereMet())
}
