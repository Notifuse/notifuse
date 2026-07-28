package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewContactSegmentQueueRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	assert.NotNil(t, repo)
}

func TestContactSegmentQueueRepository_RemoveFromQueue_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	// Create mock DB
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("DELETE FROM contact_segment_queue").
		WithArgs("user@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	err = repo.RemoveFromQueue(ctx, "workspace1", "user@example.com")

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueRepository_RemoveFromQueue_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(nil, errors.New("connection failed"))

	err := repo.RemoveFromQueue(ctx, "workspace1", "user@example.com")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

func TestContactSegmentQueueRepository_RemoveFromQueue_ExecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	// Create mock DB
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("DELETE FROM contact_segment_queue").
		WithArgs("user@example.com").
		WillReturnError(errors.New("delete failed"))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	err = repo.RemoveFromQueue(ctx, "workspace1", "user@example.com")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove email from queue")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueRepository_GetQueueSize_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	// Create mock DB
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(42)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(rows)

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	count, err := repo.GetQueueSize(ctx, "workspace1")

	assert.NoError(t, err)
	assert.Equal(t, 42, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueRepository_GetQueueSize_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(nil, errors.New("connection failed"))

	count, err := repo.GetQueueSize(ctx, "workspace1")

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

func TestContactSegmentQueueRepository_GetQueueSize_QueryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	// Create mock DB
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT COUNT").
		WillReturnError(errors.New("query failed"))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	count, err := repo.GetQueueSize(ctx, "workspace1")

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to get queue size")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueRepository_ClearQueue_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	// Create mock DB
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("DELETE FROM contact_segment_queue").
		WillReturnResult(sqlmock.NewResult(0, 100))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	err = repo.ClearQueue(ctx, "workspace1")

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueRepository_ClearQueue_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(nil, errors.New("connection failed"))

	err := repo.ClearQueue(ctx, "workspace1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

func TestContactSegmentQueueRepository_ClearQueue_ExecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactSegmentQueueRepository(mockWorkspaceRepo)

	ctx := context.Background()

	// Create mock DB
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("DELETE FROM contact_segment_queue").
		WillReturnError(errors.New("delete failed"))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	err = repo.ClearQueue(ctx, "workspace1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clear queue")
	assert.NoError(t, mock.ExpectationsWereMet())
}
