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
	status := domain.BroadcastStatusSending

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
			sqlmock.AnyArg(), // total_sent
			sqlmock.AnyArg(), // total_delivered
			sqlmock.AnyArg(), // total_bounced
			sqlmock.AnyArg(), // total_complained
			sqlmock.AnyArg(), // total_failed
			sqlmock.AnyArg(), // total_opens
			sqlmock.AnyArg(), // total_clicks
			sqlmock.AnyArg(), // winning_variation
			sqlmock.AnyArg(), // test_sent_at
			sqlmock.AnyArg(), // winner_sent_at
			sqlmock.AnyArg(), // created_at - timestamp will be added
			sqlmock.AnyArg(), // updated_at - timestamp will be added
			sqlmock.AnyArg(), // started_at
			sqlmock.AnyArg(), // completed_at
			sqlmock.AnyArg(), // cancelled_at
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
	defer db.Close()

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
	defer db.Close()

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
		"total_sent", "total_delivered", "total_bounced", "total_complained",
		"total_failed", "total_opens", "total_clicks", "winning_variation",
		"test_sent_at", "winner_sent_at", "created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at",
	}).
		AddRow(
			broadcastID, workspaceID, "Test Broadcast", domain.BroadcastStatusDraft,
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			0, 0, 0, 0,
			0, 0, 0, "", // Use empty string instead of nil for winning_variation
			nil, nil, time.Now(), time.Now(),
			nil, nil, nil,
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

// TestBroadcastRepository_GetBroadcast_ScanError tests that the repository
// handles scanning errors correctly when retrieving a broadcast.
func TestBroadcastRepository_GetBroadcast_ScanError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

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
		"total_sent", "total_delivered", "total_bounced", "total_complained",
		"total_failed", "total_opens", "total_clicks", "winning_variation",
		"test_sent_at", "winner_sent_at", "created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at",
	}).
		// Add a row with an invalid value for status (should be a string but using int)
		AddRow(
			broadcastID, workspaceID, "Test Broadcast", 123, // Invalid type for status
			nil, nil, nil, nil, nil,
			0, 0, 0, 0,
			0, 0, 0, nil,
			nil, nil, time.Now(), time.Now(),
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
	defer db.Close()

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
	defer db.Close()

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
			sqlmock.AnyArg(), // total_sent
			sqlmock.AnyArg(), // total_delivered
			sqlmock.AnyArg(), // total_bounced
			sqlmock.AnyArg(), // total_complained
			sqlmock.AnyArg(), // total_failed
			sqlmock.AnyArg(), // total_opens
			sqlmock.AnyArg(), // total_clicks
			sqlmock.AnyArg(), // winning_variation
			sqlmock.AnyArg(), // test_sent_at
			sqlmock.AnyArg(), // winner_sent_at
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // started_at
			sqlmock.AnyArg(), // completed_at
			sqlmock.AnyArg(), // cancelled_at
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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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

// TestBroadcastRepository_ListBroadcasts_DataError tests handling data fetch errors
func TestBroadcastRepository_ListBroadcasts_DataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBroadcastRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

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
	defer db.Close()

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
		"total_sent", "total_delivered", "total_bounced", "total_complained",
		"total_failed", "total_opens", "total_clicks", "winning_variation",
		"test_sent_at", "winner_sent_at", "created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at",
	}).
		AddRow(
			"bc123", workspaceID, "Broadcast 1", "draft", "{}", "{}", "{}", "{}", "{}",
			0, 0, 0, 0, 0, 0, 0, "", nil, nil, time.Now(), time.Now(), nil, nil, nil,
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
	defer db.Close()

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
	defer db.Close()

	ctx := context.Background()
	workspaceID := "ws123"
	status := domain.BroadcastStatusSending

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
		"total_sent", "total_delivered", "total_bounced", "total_complained",
		"total_failed", "total_opens", "total_clicks", "winning_variation",
		"test_sent_at", "winner_sent_at", "created_at", "updated_at",
		"started_at", "completed_at", "cancelled_at",
	}).
		AddRow(
			"bc123", workspaceID, "Broadcast 1", status, []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			0, 0, 0, 0, 0, 0, 0, "", nil, nil, time.Now(), time.Now(), nil, nil, nil,
		).
		AddRow(
			"bc456", workspaceID, "Broadcast 2", status, []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			0, 0, 0, 0, 0, 0, 0, "", nil, nil, time.Now(), time.Now(), nil, nil, nil,
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

// mockScanner is a mock implementation of the scanner interface used by scanBroadcast
type mockScanner struct {
	values []interface{}
	err    error
}

// Scan implements the scanner interface
func (m *mockScanner) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}

	for i, dest := range dest {
		if i < len(m.values) {
			switch v := dest.(type) {
			case *string:
				if str, ok := m.values[i].(string); ok {
					*v = str
				}
			case *bool:
				if b, ok := m.values[i].(bool); ok {
					*v = b
				}
			case *int:
				if num, ok := m.values[i].(int); ok {
					*v = num
				}
			case *time.Time:
				if t, ok := m.values[i].(time.Time); ok {
					*v = t
				}
			case **time.Time:
				if m.values[i] == nil {
					*v = nil
				} else if t, ok := m.values[i].(time.Time); ok {
					*v = &t
				}
			default:
				// For other types (like JSON fields), just continue
				// This is simplified for the test
			}
		}
	}
	return nil
}
