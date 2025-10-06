package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewContactSegmentQueueProcessor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	assert.NotNil(t, processor)
	assert.NotNil(t, processor.queueRepo)
	assert.NotNil(t, processor.segmentRepo)
	assert.NotNil(t, processor.contactRepo)
	assert.NotNil(t, processor.workspaceRepo)
	assert.NotNil(t, processor.queryBuilder)
	assert.NotNil(t, processor.logger)
	assert.Equal(t, 100, processor.batchSize)
}

func TestContactSegmentQueueProcessor_ProcessQueue_GetConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(nil, errors.New("connection failed"))

	count, err := processor.ProcessQueue(ctx, "workspace1")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

func TestContactSegmentQueueProcessor_ProcessQueue_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	// Create a mock DB that fails on BeginTx
	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin().WillReturnError(errors.New("begin tx failed"))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to begin transaction")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_NoPendingContacts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	// Create a mock DB
	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"})
	mock.ExpectQuery("SELECT email").WillReturnRows(rows)
	mock.ExpectRollback()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_GetSegmentsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	// Create a mock DB
	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email").WillReturnRows(rows)
	mock.ExpectRollback()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return(nil, errors.New("db error"))

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to get segments")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_NoActiveSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	// Create a mock DB
	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email").WillReturnRows(rows)
	mock.ExpectExec("DELETE FROM contact_segment_queue").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{}, nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	// Create a mock DB
	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer db.Close()

	// Setup segment
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	mock.ExpectBegin()
	// Mock getting pending emails
	emailRows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email").WillReturnRows(emailRows)

	// Mock segment evaluation query
	segmentRows := sqlmock.NewRows([]string{"segment_id"}).AddRow("segment1")
	mock.ExpectQuery("SELECT 'segment1' as segment_id").WillReturnRows(segmentRows)

	// Mock delete from queue
	mock.ExpectExec("DELETE FROM contact_segment_queue").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{segment}, nil)

	mockSegmentRepo.EXPECT().
		AddContactToSegment(ctx, "workspace1", "test@test.com", "segment1", int64(1)).
		Return(nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_RemoveFromSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	// Create a mock DB
	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer db.Close()

	// Setup segment
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	mock.ExpectBegin()
	// Mock getting pending emails
	emailRows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email").WillReturnRows(emailRows)

	// Mock segment evaluation query - no rows returned means contact doesn't match
	segmentRows := sqlmock.NewRows([]string{"segment_id"})
	mock.ExpectQuery("SELECT 'segment1' as segment_id").WillReturnRows(segmentRows)

	// Mock delete from queue
	mock.ExpectExec("DELETE FROM contact_segment_queue").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{segment}, nil)

	mockSegmentRepo.EXPECT().
		RemoveContactFromSegment(ctx, "workspace1", "test@test.com", "segment1").
		Return(nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_RemoveBatchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	// Create a mock DB
	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer db.Close()

	// Setup segment
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	mock.ExpectBegin()
	emailRows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email").WillReturnRows(emailRows)

	segmentRows := sqlmock.NewRows([]string{"segment_id"}).AddRow("segment1")
	mock.ExpectQuery("SELECT 'segment1' as segment_id").WillReturnRows(segmentRows)

	mock.ExpectExec("DELETE FROM contact_segment_queue").WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{segment}, nil)

	mockSegmentRepo.EXPECT().
		AddContactToSegment(ctx, "workspace1", "test@test.com", "segment1", int64(1)).
		Return(nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessContact_NoSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()
	db, _, _ := sqlmock.New()
	defer db.Close()

	err := processor.processContact(ctx, "workspace1", db, "test@test.com", []*domain.Segment{})
	assert.NoError(t, err)
}

func TestContactSegmentQueueProcessor_ProcessContact_NoGeneratedSQL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()
	db, _, _ := sqlmock.New()
	defer db.Close()

	segment := &domain.Segment{
		ID:           "segment1",
		Name:         "Test Segment",
		Status:       string(domain.SegmentStatusActive),
		Version:      1,
		GeneratedSQL: nil, // No SQL
	}

	err := processor.processContact(ctx, "workspace1", db, "test@test.com", []*domain.Segment{segment})
	assert.NoError(t, err)
}

func TestContactSegmentQueueProcessor_RebindPlaceholders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	tests := []struct {
		name     string
		input    string
		offset   int
		expected string
	}{
		{
			name:     "rebind from 1",
			input:    "SELECT * FROM contacts WHERE field = $1 AND other = $2",
			offset:   1,
			expected: "SELECT * FROM contacts WHERE field = $1 AND other = $2",
		},
		{
			name:     "rebind from 5",
			input:    "SELECT * FROM contacts WHERE field = $1 AND other = $2",
			offset:   5,
			expected: "SELECT * FROM contacts WHERE field = $5 AND other = $6",
		},
		{
			name:     "no placeholders",
			input:    "SELECT * FROM contacts",
			offset:   10,
			expected: "SELECT * FROM contacts",
		},
		{
			name:     "multiple digit placeholder",
			input:    "WHERE field = $10",
			offset:   1,
			expected: "WHERE field = $1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.rebindPlaceholders(tt.input, tt.offset)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContactSegmentQueueProcessor_GetQueueSize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mockQueueRepo.EXPECT().
			GetQueueSize(ctx, "workspace1").
			Return(42, nil)

		size, err := processor.GetQueueSize(ctx, "workspace1")
		assert.NoError(t, err)
		assert.Equal(t, 42, size)
	})

	t.Run("error", func(t *testing.T) {
		mockQueueRepo.EXPECT().
			GetQueueSize(ctx, "workspace1").
			Return(0, errors.New("db error"))

		size, err := processor.GetQueueSize(ctx, "workspace1")
		assert.Error(t, err)
		assert.Equal(t, 0, size)
	})
}

func TestContactSegmentQueueProcessor_GetPendingEmailsInTx_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT email").WillReturnError(errors.New("query error"))
	mock.ExpectRollback()

	tx, err := db.Begin()
	assert.NoError(t, err)

	emails, err := processor.getPendingEmailsInTx(ctx, tx, 100)
	assert.Error(t, err)
	assert.Nil(t, emails)
	assert.Contains(t, err.Error(), "failed to query pending emails")

	tx.Rollback()
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_RemoveBatchFromQueueInTx_EmptyList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	tx, err := db.Begin()
	assert.NoError(t, err)

	err = processor.removeBatchFromQueueInTx(ctx, tx, []string{})
	assert.NoError(t, err)

	tx.Rollback()
	assert.NoError(t, mock.ExpectationsWereMet())
}
