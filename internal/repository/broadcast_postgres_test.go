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
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastRepository_CreateBroadcast_ConnectionError tests that the repository
// handles connection errors correctly when creating a broadcast.
func TestBroadcastRepository_CreateBroadcast_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws123"

	testBroadcast := &domain.Broadcast{
		ID:          "bc123",
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		Status:      domain.BroadcastStatusDraft,
	}

	expectedErr := errors.New("connection error")
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, expectedErr)

	err := repo.CreateBroadcast(ctx, testBroadcast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

// TestBroadcastRepository_GetBroadcast_ConnectionError tests that the repository
// handles connection errors correctly when retrieving a broadcast.
func TestBroadcastRepository_GetBroadcast_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	expectedErr := errors.New("connection error")
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, expectedErr)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)
	assert.Nil(t, broadcast)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

// TestBroadcastRepository_UpdateBroadcast_ConnectionError tests that the repository
// handles connection errors correctly when updating a broadcast.
func TestBroadcastRepository_UpdateBroadcast_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws123"

	testBroadcast := &domain.Broadcast{
		ID:          "bc123",
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		Status:      domain.BroadcastStatusDraft,
	}

	expectedErr := errors.New("connection error")
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, expectedErr)

	err := repo.UpdateBroadcast(ctx, testBroadcast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

// TestBroadcastRepository_ListBroadcasts_ConnectionError tests that the repository
// handles connection errors correctly when listing broadcasts.
func TestBroadcastRepository_ListBroadcasts_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws123"
	status := domain.BroadcastStatusProcessing

	params := domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Status:      status,
		Limit:       10,
		Offset:      0,
	}

	expectedErr := errors.New("connection error")
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, expectedErr)

	response, err := repo.ListBroadcasts(ctx, params)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

// TestBroadcastRepository_GetBroadcast_NotFound tests that the repository
// handles not found errors correctly when retrieving a broadcast.
func TestBroadcastRepository_GetBroadcast_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	mock.ExpectQuery("SELECT").
		WithArgs(broadcastID, workspaceID).
		WillReturnError(sql.ErrNoRows)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)
	assert.Nil(t, broadcast)

	var notFoundErr *domain.ErrBroadcastNotFound
	assert.ErrorAs(t, err, &notFoundErr)
}

// TestBroadcastRepository_ListBroadcasts_CountQueryError tests handling of count query errors
func TestBroadcastRepository_ListBroadcasts_CountQueryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Make the count query return an error
	expectedErr := errors.New("database error")
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(workspaceID).
		WillReturnError(expectedErr)

	// Expect rollback
	mock.ExpectRollback()

	_, err = repo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Limit:       10,
		Offset:      0,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count broadcasts")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_DeleteBroadcast_ConnectionError tests that the repository
// handles connection errors correctly when deleting a broadcast.
func TestBroadcastRepository_DeleteBroadcast_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	expectedErr := errors.New("connection error")
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, expectedErr)

	err := repo.DeleteBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

// TestBroadcastRepository_DeleteBroadcast_Success tests successful deletion of a broadcast.
func TestBroadcastRepository_DeleteBroadcast_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect DELETE query with the correct parameters and returning 1 row affected
	mock.ExpectExec("DELETE FROM broadcasts").
		WithArgs(broadcastID, workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect transaction commit
	mock.ExpectCommit()

	err = repo.DeleteBroadcast(ctx, workspaceID, broadcastID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_DeleteBroadcast_NotFound tests that the repository
// handles not found errors correctly when deleting a broadcast.
func TestBroadcastRepository_DeleteBroadcast_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "nonexistent"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect DELETE query with the correct parameters but no rows affected
	mock.ExpectExec("DELETE FROM broadcasts").
		WithArgs(broadcastID, workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect rollback since there was an error (broadcast not found)
	mock.ExpectRollback()

	err = repo.DeleteBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)

	var notFoundErr *domain.ErrBroadcastNotFound
	assert.ErrorAs(t, err, &notFoundErr)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_DeleteBroadcast_ExecError tests that the repository
// handles execution errors correctly when deleting a broadcast.
func TestBroadcastRepository_DeleteBroadcast_ExecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect DELETE query but return an error
	expectedErr := errors.New("database error")
	mock.ExpectExec("DELETE FROM broadcasts").
		WithArgs(broadcastID, workspaceID).
		WillReturnError(expectedErr)

	// Expect rollback since there was an error
	mock.ExpectRollback()

	err = repo.DeleteBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete broadcast")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_DeleteBroadcast_RowsAffectedError tests that the repository
// handles errors when getting rows affected.
func TestBroadcastRepository_DeleteBroadcast_RowsAffectedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Create a custom result that returns an error for RowsAffected
	expectedErr := errors.New("rows affected error")
	mock.ExpectExec("DELETE FROM broadcasts").
		WithArgs(broadcastID, workspaceID).
		WillReturnResult(sqlmock.NewErrorResult(expectedErr))

	// Expect rollback since there was an error
	mock.ExpectRollback()

	err = repo.DeleteBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get rows affected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_CreateBroadcast_Success tests successful creation of a broadcast
func TestBroadcastRepository_CreateBroadcast_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	testBroadcast := &domain.Broadcast{
		ID:          "bc123",
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		Status:      domain.BroadcastStatusDraft,
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Use AnyArg() matcher since the broadcast will have timestamps added
	mock.ExpectExec("INSERT INTO broadcasts").
		WithArgs(
			testBroadcast.ID,
			testBroadcast.WorkspaceID,
			testBroadcast.Name,
			testBroadcast.Status,
			sqlmock.AnyArg(), // audience
			sqlmock.AnyArg(), // schedule
			sqlmock.AnyArg(), // test_settings
			sqlmock.AnyArg(), // utm_parameters
			sqlmock.AnyArg(), // metadata
			sqlmock.AnyArg(), // winning_template
			sqlmock.AnyArg(), // test_sent_at
			sqlmock.AnyArg(), // winner_sent_at
			sqlmock.AnyArg(), // enqueued_count
			sqlmock.AnyArg(), // created_at - timestamp will be added
			sqlmock.AnyArg(), // updated_at - timestamp will be added
			sqlmock.AnyArg(), // started_at
			sqlmock.AnyArg(), // completed_at
			sqlmock.AnyArg(), // cancelled_at
			sqlmock.AnyArg(), // paused_at
			sqlmock.AnyArg(), // pause_reason
			sqlmock.AnyArg(), // data_feed (consolidated)
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect transaction commit
	mock.ExpectCommit()

	err = repo.CreateBroadcast(ctx, testBroadcast)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Verify the timestamps were added
	assert.False(t, testBroadcast.CreatedAt.IsZero())
	assert.False(t, testBroadcast.UpdatedAt.IsZero())
}

// TestBroadcastRepository_CreateBroadcast_ExecError tests that the repository
// handles execution errors correctly when creating a broadcast.
func TestBroadcastRepository_CreateBroadcast_ExecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	testBroadcast := &domain.Broadcast{
		ID:          "bc123",
		WorkspaceID: workspaceID,
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect INSERT query but return an error
	expectedErr := errors.New("database error")
	mock.ExpectExec("INSERT INTO broadcasts").
		WillReturnError(expectedErr)

	// Expect rollback since there was an error
	mock.ExpectRollback()

	err = repo.CreateBroadcast(ctx, testBroadcast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create broadcast")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_GetBroadcast_Success tests successful retrieval of a broadcast.
func TestBroadcastRepository_GetBroadcast_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Create mock rows for the broadcast
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "audience", "schedule",
		"test_settings", "utm_parameters", "metadata",
		"winning_template",
		"test_sent_at", "winner_sent_at", "enqueued_count",
		"created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
		"data_feed",
	}).
		AddRow(
			broadcastID, workspaceID, "Test Broadcast", domain.BroadcastStatusDraft,
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"",          // Use empty string instead of nil for winning_template
			nil, nil, 0, // enqueued_count
			time.Now(), time.Now(),
			nil, nil, nil, nil, nil,
			nil, // data_feed
		)

	mock.ExpectQuery("SELECT").
		WithArgs(broadcastID, workspaceID).
		WillReturnRows(rows)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.NoError(t, err)
	assert.NotNil(t, broadcast)
	assert.Equal(t, broadcastID, broadcast.ID)
	assert.Equal(t, workspaceID, broadcast.WorkspaceID)
	assert.Equal(t, "Test Broadcast", broadcast.Name)
	assert.Equal(t, domain.BroadcastStatusDraft, broadcast.Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_GetBroadcast_NullPauseReason tests that the repository
// can correctly handle NULL pause_reason values (simulating migrated old broadcasts).
// This regression test ensures we don't reintroduce the bug where NULL values
// couldn't be scanned into non-nullable string fields.
func TestBroadcastRepository_GetBroadcast_NullPauseReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Create mock rows with NULL pause_reason (simulating migrated data)
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "audience", "schedule",
		"test_settings", "utm_parameters", "metadata",
		"winning_template",
		"test_sent_at", "winner_sent_at", "enqueued_count",
		"created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
		"data_feed",
	}).
		AddRow(
			broadcastID, workspaceID, "Test Broadcast", domain.BroadcastStatusDraft,
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"",          // Use empty string instead of nil for winning_template
			nil, nil, 0, // enqueued_count
			time.Now(), time.Now(),
			nil, nil, nil, nil, nil, // NULL pause_reason
			nil, // data_feed
		)

	mock.ExpectQuery("SELECT").
		WithArgs(broadcastID, workspaceID).
		WillReturnRows(rows)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.NoError(t, err, "Should successfully scan broadcast with NULL pause_reason")
	assert.NotNil(t, broadcast)
	assert.Equal(t, broadcastID, broadcast.ID)
	assert.Equal(t, workspaceID, broadcast.WorkspaceID)
	assert.Equal(t, "Test Broadcast", broadcast.Name)
	assert.Nil(t, broadcast.PauseReason, "NULL pause_reason should be scanned as nil pointer")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_GetBroadcast_WithPauseReason tests that the repository
// correctly handles non-NULL pause_reason values.
func TestBroadcastRepository_GetBroadcast_WithPauseReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"
	expectedReason := "Circuit breaker triggered: Email provider error"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Create mock rows with non-NULL pause_reason
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "audience", "schedule",
		"test_settings", "utm_parameters", "metadata",
		"winning_template",
		"test_sent_at", "winner_sent_at", "enqueued_count",
		"created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
		"data_feed",
	}).
		AddRow(
			broadcastID, workspaceID, "Test Broadcast", domain.BroadcastStatusPaused,
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"",
			nil, nil, 0, // enqueued_count
			time.Now(), time.Now(),
			nil, nil, nil, time.Now(), expectedReason, // Non-NULL pause_reason
			nil, // data_feed
		)

	mock.ExpectQuery("SELECT").
		WithArgs(broadcastID, workspaceID).
		WillReturnRows(rows)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.NoError(t, err, "Should successfully scan broadcast with pause_reason")
	assert.NotNil(t, broadcast)
	assert.Equal(t, broadcastID, broadcast.ID)
	assert.NotNil(t, broadcast.PauseReason, "Non-NULL pause_reason should be scanned as pointer")
	assert.Equal(t, expectedReason, *broadcast.PauseReason, "Pause reason should match expected value")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_GetBroadcast_ScanError tests that the repository
// handles scanning errors correctly when retrieving a broadcast.
func TestBroadcastRepository_GetBroadcast_ScanError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Create mock rows with incorrect types to cause a scan error
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "audience", "schedule",
		"test_settings", "utm_parameters", "metadata",
		"winning_template",
		"test_sent_at", "winner_sent_at", "created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at",
	}).
		// Add a row with an invalid value for status (should be a string but using int)
		AddRow(
			broadcastID, workspaceID, "Test Broadcast", 123, // Invalid type for status
			nil, nil, nil, nil, nil,
			"", nil, nil,
			time.Now(), time.Now(),
			nil, nil, nil,
		)

	mock.ExpectQuery("SELECT").
		WithArgs(broadcastID, workspaceID).
		WillReturnRows(rows)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)
	assert.Nil(t, broadcast)
	assert.Contains(t, err.Error(), "failed to get broadcast")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_GetBroadcast_QueryError tests that the repository
// handles query errors correctly when retrieving a broadcast.
func TestBroadcastRepository_GetBroadcast_QueryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	expectedErr := errors.New("database error")
	mock.ExpectQuery("SELECT").
		WithArgs(broadcastID, workspaceID).
		WillReturnError(expectedErr)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.Error(t, err)
	assert.Nil(t, broadcast)
	assert.Contains(t, err.Error(), "failed to get broadcast")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_UpdateBroadcast_Success tests successful update of a broadcast
func TestBroadcastRepository_UpdateBroadcast_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	// Create a test broadcast with updated values
	testBroadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Updated Broadcast",
		Status:      domain.BroadcastStatusDraft,
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect UPDATE query with the correct parameters
	mock.ExpectExec("UPDATE broadcasts SET").
		WithArgs(
			broadcastID,
			workspaceID,
			testBroadcast.Name,
			testBroadcast.Status,
			sqlmock.AnyArg(), // audience
			sqlmock.AnyArg(), // schedule
			sqlmock.AnyArg(), // test_settings
			sqlmock.AnyArg(), // utm_parameters
			sqlmock.AnyArg(), // metadata
			sqlmock.AnyArg(), // winning_template
			sqlmock.AnyArg(), // test_sent_at
			sqlmock.AnyArg(), // winner_sent_at
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // started_at
			sqlmock.AnyArg(), // completed_at
			sqlmock.AnyArg(), // cancelled_at
			sqlmock.AnyArg(), // paused_at
			sqlmock.AnyArg(), // pause_reason
			sqlmock.AnyArg(), // enqueued_count
			sqlmock.AnyArg(), // data_feed (consolidated)
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect transaction commit
	mock.ExpectCommit()

	err = repo.UpdateBroadcast(ctx, testBroadcast)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Verify the updated_at timestamp was updated
	assert.False(t, testBroadcast.UpdatedAt.IsZero())
}

// TestBroadcastRepository_UpdateBroadcast_NotFound tests that the repository
// handles not found errors correctly when updating a broadcast.
func TestBroadcastRepository_UpdateBroadcast_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	// Create a test broadcast with a non-existent ID
	testBroadcast := &domain.Broadcast{
		ID:          "nonexistent",
		WorkspaceID: workspaceID,
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect UPDATE query with correct parameters but return that no rows were affected
	mock.ExpectExec("UPDATE broadcasts SET").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect rollback since there was an error (broadcast not found)
	mock.ExpectRollback()

	err = repo.UpdateBroadcast(ctx, testBroadcast)
	assert.Error(t, err)

	var notFoundErr *domain.ErrBroadcastNotFound
	assert.ErrorAs(t, err, &notFoundErr)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_UpdateBroadcast_ExecError tests that the repository
// handles execution errors correctly when updating a broadcast.
func TestBroadcastRepository_UpdateBroadcast_ExecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	testBroadcast := &domain.Broadcast{
		ID:          "bc123",
		WorkspaceID: workspaceID,
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect UPDATE query but return an error
	expectedErr := errors.New("database error")
	mock.ExpectExec("UPDATE broadcasts SET").
		WillReturnError(expectedErr)

	// Expect rollback since there was an error
	mock.ExpectRollback()

	err = repo.UpdateBroadcast(ctx, testBroadcast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update broadcast")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_UpdateBroadcast_RowsAffectedError tests that the repository
// handles errors when getting rows affected during update.
func TestBroadcastRepository_UpdateBroadcast_RowsAffectedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	testBroadcast := &domain.Broadcast{
		ID:          "bc123",
		WorkspaceID: workspaceID,
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Return a result with an error for RowsAffected
	expectedErr := errors.New("rows affected error")
	mock.ExpectExec("UPDATE broadcasts SET").
		WillReturnResult(sqlmock.NewErrorResult(expectedErr))

	// Expect rollback since there was an error
	mock.ExpectRollback()

	err = repo.UpdateBroadcast(ctx, testBroadcast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get rows affected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_UpdateBroadcast_CancelledBroadcast tests that the repository
// prevents updating a broadcast that is already cancelled due to the WHERE clause restriction.
func TestBroadcastRepository_UpdateBroadcast_CancelledBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	// Create a test broadcast that is cancelled
	testBroadcast := &domain.Broadcast{
		ID:          "cancelled-broadcast",
		WorkspaceID: workspaceID,
		Status:      domain.BroadcastStatusCancelled,
		Name:        "Test Cancelled Broadcast",
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect UPDATE query with WHERE clause restrictions - should return 0 rows affected
	// because the broadcast is cancelled and the WHERE clause prevents updating cancelled broadcasts
	mock.ExpectExec("UPDATE broadcasts SET").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect rollback since no rows were affected (treated as not found)
	mock.ExpectRollback()

	err = repo.UpdateBroadcast(ctx, testBroadcast)
	assert.Error(t, err)

	// Should return ErrBroadcastNotFound because no rows were affected due to WHERE clause restriction
	var notFoundErr *domain.ErrBroadcastNotFound
	assert.ErrorAs(t, err, &notFoundErr)
	assert.Equal(t, "cancelled-broadcast", notFoundErr.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_ListBroadcasts_DataError tests handling data fetch errors
func TestBroadcastRepository_ListBroadcasts_DataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Count query succeeds
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	// Data query fails
	expectedErr := errors.New("database error")
	mock.ExpectQuery("SELECT(.+)FROM broadcasts").
		WithArgs(workspaceID, 10, 0).
		WillReturnError(expectedErr)

	// Expect rollback
	mock.ExpectRollback()

	// Execute the method
	_, err = repo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Limit:       10,
		Offset:      0,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list broadcasts")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_ListBroadcasts_RowsIterationError tests errors during rows iteration
func TestBroadcastRepository_ListBroadcasts_RowsIterationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Count query succeeds
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	// Setup a rows object that will return an error when we iterate
	iterationErr := errors.New("iteration error")
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "audience", "schedule",
		"test_settings", "utm_parameters", "metadata",
		"winning_template",
		"test_sent_at", "winner_sent_at", "enqueued_count",
		"created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
		"data_feed",
	}).
		AddRow(
			"bc123", workspaceID, "Broadcast 1", "draft", []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"", nil, nil, 0, time.Now(), time.Now(), nil, nil, nil, nil, nil,
			nil, // data_feed
		).
		RowError(0, iterationErr) // Set error on the first row

	// Expect data query
	mock.ExpectQuery("SELECT(.+)FROM broadcasts").
		WithArgs(workspaceID, 10, 0).
		WillReturnRows(rows)

	// Expect rollback
	mock.ExpectRollback()

	// Execute the method
	_, err = repo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Limit:       10,
		Offset:      0,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error iterating broadcast rows")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_ListBroadcasts_ScanError tests handling scan errors
func TestBroadcastRepository_ListBroadcasts_ScanError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Count query succeeds
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	// Create invalid rows that will cause a scan error
	// Using wrong number of columns will force a scan error
	rows := sqlmock.NewRows([]string{"id", "workspace_id"}).
		AddRow("bc123", workspaceID)

	// Expect data query
	mock.ExpectQuery("SELECT(.+)FROM broadcasts").
		WithArgs(workspaceID, 10, 0).
		WillReturnRows(rows)

	// Expect rollback
	mock.ExpectRollback()

	// Execute the method
	_, err = repo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Limit:       10,
		Offset:      0,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan broadcast")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_ListBroadcasts_WithStatus tests listing broadcasts with status filter
func TestBroadcastRepository_ListBroadcasts_WithStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	status := domain.BroadcastStatusProcessing

	// Setup mock expectations for workspace DB connection
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect count query with status filter
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(workspaceID, status).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// Setup mock rows
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "audience", "schedule",
		"test_settings", "utm_parameters", "metadata",
		"winning_template",
		"test_sent_at", "winner_sent_at", "enqueued_count",
		"created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
		"data_feed",
	}).
		AddRow(
			"bc123", workspaceID, "Broadcast 1", status, []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"", nil, nil, 0, time.Now(), time.Now(), nil, nil, nil, nil, nil,
			nil, // data_feed
		).
		AddRow(
			"bc456", workspaceID, "Broadcast 2", status, []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"", nil, nil, 0, time.Now(), time.Now(), nil, nil, nil, nil, nil,
			nil, // data_feed
		)

	// Expect query with limit/offset
	mock.ExpectQuery("SELECT(.+)FROM broadcasts").
		WithArgs(workspaceID, status, 10, 0).
		WillReturnRows(rows)

	// Expect commit
	mock.ExpectCommit()

	// Execute the method
	result, err := repo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Status:      status,
		Limit:       10,
		Offset:      0,
	})

	// Assert expectations
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.TotalCount)
	assert.Equal(t, 2, len(result.Broadcasts))
	assert.Equal(t, "bc123", result.Broadcasts[0].ID)
	assert.Equal(t, "bc456", result.Broadcasts[1].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// broadcastListColumnNames mirrors the column order selected by ListBroadcastsTx.
var broadcastListColumnNames = []string{
	"id", "workspace_id", "name", "status", "audience", "schedule",
	"test_settings", "utm_parameters", "metadata",
	"winning_template",
	"test_sent_at", "winner_sent_at", "enqueued_count",
	"created_at", "updated_at",
	"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
	"data_feed",
}

// TestBroadcastRepository_ListBroadcasts_WithStatusesAndSearch tests listing
// broadcasts filtered by multiple statuses combined with a name search.
func TestBroadcastRepository_ListBroadcasts_WithStatusesAndSearch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	statuses := []domain.BroadcastStatus{
		domain.BroadcastStatusProcessing,
		domain.BroadcastStatusPaused,
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	mock.ExpectBegin()

	// Count query pins the full WHERE clause and placeholder numbering so a
	// future reorder / off-by-one in the dynamic builder is caught.
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM broadcasts WHERE workspace_id = \$1 AND status IN \(\$2, \$3\) AND name ILIKE \$4`).
		WithArgs(workspaceID, statuses[0], statuses[1], "%promo%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows(broadcastListColumnNames).
		AddRow(
			"bc1", workspaceID, "Promo Blast", domain.BroadcastStatusProcessing,
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"", nil, nil, 0, time.Now(), time.Now(), nil, nil, nil, nil, nil,
			nil, // data_feed
		)

	// Data query pins the same WHERE clause plus pagination placeholders.
	mock.ExpectQuery(`FROM broadcasts WHERE workspace_id = \$1 AND status IN \(\$2, \$3\) AND name ILIKE \$4 ORDER BY created_at DESC LIMIT \$5 OFFSET \$6`).
		WithArgs(workspaceID, statuses[0], statuses[1], "%promo%", 10, 0).
		WillReturnRows(rows)

	mock.ExpectCommit()

	result, err := repo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Statuses:    statuses,
		Search:      "promo",
		Limit:       10,
		Offset:      0,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Broadcasts, 1)
	assert.Equal(t, "bc1", result.Broadcasts[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_ListBroadcasts_SearchOnly tests listing broadcasts
// filtered by a name search with no status filter (verifies placeholder math).
func TestBroadcastRepository_ListBroadcasts_SearchOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	mock.ExpectBegin()

	// Special LIKE characters in the term must be escaped before binding, and
	// the search-only path must use $2 (no status placeholders before it).
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM broadcasts WHERE workspace_id = \$1 AND name ILIKE \$2`).
		WithArgs(workspaceID, `%50\% off%`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows(broadcastListColumnNames).
		AddRow(
			"bc1", workspaceID, "50% off sale", domain.BroadcastStatusDraft,
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"", nil, nil, 0, time.Now(), time.Now(), nil, nil, nil, nil, nil,
			nil,
		)

	mock.ExpectQuery(`FROM broadcasts WHERE workspace_id = \$1 AND name ILIKE \$2 ORDER BY created_at DESC LIMIT \$3 OFFSET \$4`).
		WithArgs(workspaceID, `%50\% off%`, 25, 0).
		WillReturnRows(rows)

	mock.ExpectCommit()

	result, err := repo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Search:      "50% off",
		Limit:       25,
		Offset:      0,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Broadcasts, 1)
	assert.Equal(t, "bc1", result.Broadcasts[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestEscapeLikePattern verifies the LIKE/ILIKE special-character escaping.
func TestEscapeLikePattern(t *testing.T) {
	assert.Equal(t, "plain", escapeLikePattern("plain"))
	assert.Equal(t, `50\% off`, escapeLikePattern("50% off"))
	assert.Equal(t, `a\_b`, escapeLikePattern("a_b"))
	assert.Equal(t, `back\\slash`, escapeLikePattern(`back\slash`))
	assert.Equal(t, `\\\%\_`, escapeLikePattern(`\%_`))
}

func TestBroadcastRepository_GetBroadcastTx(t *testing.T) {
	// Test broadcastRepository.GetBroadcastTx - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	workspaceID := "ws123"
	broadcastID := "bc123"

	t.Run("Success - Broadcast found", func(t *testing.T) {
		sqlMock.ExpectBegin()
		sqlMock.ExpectQuery("SELECT").
			WithArgs(broadcastID, workspaceID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "workspace_id", "name", "status", "audience", "schedule",
				"test_settings", "utm_parameters", "metadata",
				"winning_template",
				"test_sent_at", "winner_sent_at", "enqueued_count",
				"created_at", "updated_at",
				"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
				"data_feed",
			}).
				AddRow(
					broadcastID, workspaceID, "Test Broadcast", "draft",
					[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
					"", nil, nil, 0, time.Now(), time.Now(), nil, nil, nil, nil, nil,
					nil, // data_feed
				))
		sqlMock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		broadcast, err := repo.GetBroadcastTx(ctx, tx, workspaceID, broadcastID)
		_ = tx.Commit()
		assert.NoError(t, err)
		assert.NotNil(t, broadcast)
		assert.Equal(t, broadcastID, broadcast.ID)
		assert.Equal(t, workspaceID, broadcast.WorkspaceID)
	})

	t.Run("Error - Broadcast not found", func(t *testing.T) {
		sqlMock.ExpectBegin()
		sqlMock.ExpectQuery("SELECT").
			WithArgs("nonexistent", workspaceID).
			WillReturnError(sql.ErrNoRows)
		sqlMock.ExpectRollback()

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		broadcast, err := repo.GetBroadcastTx(ctx, tx, workspaceID, "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, broadcast)
		assert.Contains(t, err.Error(), "Broadcast not found")
	})
}

// TestBroadcastRepository_GetBroadcast_WithDataFeed tests that the repository
// correctly handles broadcasts with DataFeed settings.
func TestBroadcastRepository_GetBroadcast_WithDataFeed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Create mock rows with consolidated DataFeed data
	dataFeedJSON := []byte(`{"global_feed":{"enabled":true,"url":"https://api.example.com/feed","headers":[]},"global_feed_data":{"products":[{"id":"1","name":"Product A"}]},"global_feed_fetched_at":"2024-01-15T10:00:00Z"}`)

	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "audience", "schedule",
		"test_settings", "utm_parameters", "metadata",
		"winning_template",
		"test_sent_at", "winner_sent_at", "enqueued_count",
		"created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at", "paused_at", "pause_reason",
		"data_feed",
	}).
		AddRow(
			broadcastID, workspaceID, "Test Broadcast", domain.BroadcastStatusDraft,
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			"", nil, nil, 0, time.Now(), time.Now(), nil, nil, nil, nil, nil,
			dataFeedJSON,
		)

	mock.ExpectQuery("SELECT").
		WithArgs(broadcastID, workspaceID).
		WillReturnRows(rows)

	broadcast, err := repo.GetBroadcast(ctx, workspaceID, broadcastID)
	assert.NoError(t, err)
	assert.NotNil(t, broadcast)
	assert.Equal(t, broadcastID, broadcast.ID)

	// Verify DataFeed settings
	assert.NotNil(t, broadcast.DataFeed, "DataFeed should not be nil when data exists")
	assert.NotNil(t, broadcast.DataFeed.GlobalFeed, "GlobalFeed should not be nil when data exists")
	assert.True(t, broadcast.DataFeed.GlobalFeed.Enabled)
	assert.Equal(t, "https://api.example.com/feed", broadcast.DataFeed.GlobalFeed.URL)

	// Verify GlobalFeedData
	assert.NotNil(t, broadcast.DataFeed.GlobalFeedData, "GlobalFeedData should not be nil when data exists")
	products, ok := broadcast.DataFeed.GlobalFeedData["products"]
	assert.True(t, ok, "Should have products key in GlobalFeedData")
	assert.NotNil(t, products)

	// Verify GlobalFeedFetchedAt
	assert.NotNil(t, broadcast.DataFeed.GlobalFeedFetchedAt, "GlobalFeedFetchedAt should not be nil when set")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_CreateBroadcast_WithDataFeed tests successful creation of a broadcast with DataFeed settings
func TestBroadcastRepository_CreateBroadcast_WithDataFeed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"

	fetchedAt := time.Now()
	testBroadcast := &domain.Broadcast{
		ID:          "bc123",
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast with DataFeed",
		Status:      domain.BroadcastStatusDraft,
		DataFeed: &domain.DataFeedSettings{
			GlobalFeed: &domain.GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/feed",
				Headers: []domain.DataFeedHeader{},
			},
			GlobalFeedData: domain.MapOfAny{
				"products": []interface{}{
					map[string]interface{}{"id": "1", "name": "Product A"},
				},
			},
			GlobalFeedFetchedAt: &fetchedAt,
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Use AnyArg() matcher since the broadcast will have timestamps added
	mock.ExpectExec("INSERT INTO broadcasts").
		WithArgs(
			testBroadcast.ID,
			testBroadcast.WorkspaceID,
			testBroadcast.Name,
			testBroadcast.Status,
			sqlmock.AnyArg(), // audience
			sqlmock.AnyArg(), // schedule
			sqlmock.AnyArg(), // test_settings
			sqlmock.AnyArg(), // utm_parameters
			sqlmock.AnyArg(), // metadata
			sqlmock.AnyArg(), // winning_template
			sqlmock.AnyArg(), // test_sent_at
			sqlmock.AnyArg(), // winner_sent_at
			sqlmock.AnyArg(), // enqueued_count
			sqlmock.AnyArg(), // created_at - timestamp will be added
			sqlmock.AnyArg(), // updated_at - timestamp will be added
			sqlmock.AnyArg(), // started_at
			sqlmock.AnyArg(), // completed_at
			sqlmock.AnyArg(), // cancelled_at
			sqlmock.AnyArg(), // paused_at
			sqlmock.AnyArg(), // pause_reason
			sqlmock.AnyArg(), // data_feed (consolidated)
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect transaction commit
	mock.ExpectCommit()

	err = repo.CreateBroadcast(ctx, testBroadcast)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_UpdateBroadcast_WithDataFeed tests updating broadcast with DataFeed settings
func TestBroadcastRepository_UpdateBroadcast_WithDataFeed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "bc123"

	fetchedAt := time.Now()
	testBroadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Updated Broadcast",
		Status:      domain.BroadcastStatusScheduled,
		DataFeed: &domain.DataFeedSettings{
			GlobalFeed: &domain.GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/updated-feed",
				Headers: []domain.DataFeedHeader{},
			},
			GlobalFeedData: domain.MapOfAny{
				"updated": true,
			},
			GlobalFeedFetchedAt: &fetchedAt,
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect UPDATE query with the correct parameters
	mock.ExpectExec("UPDATE broadcasts SET").
		WithArgs(
			broadcastID,
			workspaceID,
			testBroadcast.Name,
			testBroadcast.Status,
			sqlmock.AnyArg(), // audience
			sqlmock.AnyArg(), // schedule
			sqlmock.AnyArg(), // test_settings
			sqlmock.AnyArg(), // utm_parameters
			sqlmock.AnyArg(), // metadata
			sqlmock.AnyArg(), // winning_template
			sqlmock.AnyArg(), // test_sent_at
			sqlmock.AnyArg(), // winner_sent_at
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // started_at
			sqlmock.AnyArg(), // completed_at
			sqlmock.AnyArg(), // cancelled_at
			sqlmock.AnyArg(), // paused_at
			sqlmock.AnyArg(), // pause_reason
			sqlmock.AnyArg(), // enqueued_count
			sqlmock.AnyArg(), // data_feed (consolidated)
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect transaction commit
	mock.ExpectCommit()

	err = repo.UpdateBroadcast(ctx, testBroadcast)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBroadcastRepository_UpdateBroadcastStatusUnless covers the status write a
// failed broadcast run ends with: it decides on the row it writes, so a status
// another transaction committed after the caller's read is kept.
func TestBroadcastRepository_UpdateBroadcastStatusUnless(t *testing.T) {
	workspaceID := "ws123"
	pausedAt := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	reason := "Sending was interrupted"
	unless := []domain.BroadcastStatus{
		domain.BroadcastStatusCancelled,
		domain.BroadcastStatusPaused,
		domain.BroadcastStatusProcessed,
	}
	newBroadcast := func() *domain.Broadcast {
		return &domain.Broadcast{
			ID:          "bc123",
			WorkspaceID: workspaceID,
			Status:      domain.BroadcastStatusPaused,
			PausedAt:    &pausedAt,
			PauseReason: &reason,
		}
	}
	setup := func(t *testing.T) (*broadcastRepository, *mocks.MockWorkspaceRepository, sqlmock.Sqlmock) {
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil).AnyTimes()
		return NewBroadcastRepository(mockWorkspaceRepo).(*broadcastRepository), mockWorkspaceRepo, mock
	}
	const statusOnlyUpdate = `UPDATE broadcasts SET status = \$3, paused_at = \$4, pause_reason = \$5, updated_at = \$6\s+WHERE id = \$1 AND workspace_id = \$2 AND status <> ALL\(\$7\)`

	t.Run("writes the status fields in a transaction when the stored status is not excluded", func(t *testing.T) {
		repo, _, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectExec(statusOnlyUpdate).
			WithArgs("bc123", workspaceID, "paused", &pausedAt, &reason, sqlmock.AnyArg(),
				pq.Array([]string{"cancelled", "paused", "processed"})).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), unless)
		require.NoError(t, err)
		assert.True(t, written)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports no write when the stored status is excluded or the broadcast is gone", func(t *testing.T) {
		repo, _, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectExec(statusOnlyUpdate).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), unless)
		require.NoError(t, err)
		assert.False(t, written)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("binds no excluded status as an empty list, not NULL", func(t *testing.T) {
		// status <> ALL(NULL) matches no row, so a nil list would never write.
		repo, _, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectExec(statusOnlyUpdate).
			WithArgs("bc123", workspaceID, "paused", &pausedAt, &reason, sqlmock.AnyArg(),
				pq.Array([]string{})).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), nil)
		require.NoError(t, err)
		assert.True(t, written)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the error of a failed write and rolls back", func(t *testing.T) {
		repo, _, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectExec(statusOnlyUpdate).
			WillReturnError(errors.New("deadlock detected"))
		mock.ExpectRollback()

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), unless)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deadlock detected")
		assert.False(t, written)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the error of a failed row count and rolls back", func(t *testing.T) {
		repo, _, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectExec(statusOnlyUpdate).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("driver lost the count")))
		mock.ExpectRollback()

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), unless)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get rows affected")
		assert.False(t, written)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the error of a failed begin", func(t *testing.T) {
		repo, _, mock := setup(t)
		mock.ExpectBegin().WillReturnError(errors.New("too many connections"))

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), unless)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
		assert.False(t, written)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports no write when the commit fails", func(t *testing.T) {
		repo, _, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectExec(statusOnlyUpdate).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errors.New("connection reset"))

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), unless)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to commit transaction")
		assert.False(t, written)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the error of a failed connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(nil, errors.New("connection error"))
		repo := NewBroadcastRepository(mockWorkspaceRepo)

		written, err := repo.UpdateBroadcastStatusUnless(context.Background(), newBroadcast(), unless)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		assert.False(t, written)
	})
}
