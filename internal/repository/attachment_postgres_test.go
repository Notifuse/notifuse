package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAttachmentRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewAttachmentRepository(workspaceRepo)

	assert.NotNil(t, repo)
	assert.Equal(t, workspaceRepo, repo.workspaceRepo)
}

func TestAttachmentRepository_Store(t *testing.T) {
	workspaceID := "ws-123"
	ctx := context.Background()

	tests := []struct {
		name      string
		record    *domain.AttachmentRecord
		setupMock func(*mocks.MockWorkspaceRepository, *sql.DB, sqlmock.Sqlmock)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful store - new attachment",
			record: &domain.AttachmentRecord{
				Checksum:    "abc123def456",
				Content:     []byte("test content"),
				ContentType: "application/pdf",
				SizeBytes:   12,
			},
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				mock.ExpectExec(`INSERT INTO message_attachments \(checksum, content, content_type, size_bytes, created_at\)`).
					WithArgs("abc123def456", []byte("test content"), "application/pdf", int64(12)).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name: "successful store - duplicate (ON CONFLICT DO NOTHING)",
			record: &domain.AttachmentRecord{
				Checksum:    "duplicate123",
				Content:     []byte("content"),
				ContentType: "text/plain",
				SizeBytes:   7,
			},
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				// ON CONFLICT DO NOTHING returns 0 rows affected
				mock.ExpectExec(`INSERT INTO message_attachments`).
					WithArgs("duplicate123", []byte("content"), "text/plain", int64(7)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: false,
		},
		{
			name: "workspace connection error",
			record: &domain.AttachmentRecord{
				Checksum: "test",
			},
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(nil, errors.New("connection failed"))
			},
			wantErr: true,
			errMsg:  "failed to get workspace connection",
		},
		{
			name: "database execution error",
			record: &domain.AttachmentRecord{
				Checksum:    "test",
				Content:     []byte("content"),
				ContentType: "text/plain",
				SizeBytes:   7,
			},
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				mock.ExpectExec(`INSERT INTO message_attachments`).
					WithArgs("test", []byte("content"), "text/plain", int64(7)).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to store attachment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			db, mock, cleanup := setupMockDB(t)
			defer cleanup()

			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			tt.setupMock(workspaceRepo, db, mock)

			repo := NewAttachmentRepository(workspaceRepo)
			err := repo.Store(ctx, workspaceID, tt.record)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAttachmentRepository_Get(t *testing.T) {
	workspaceID := "ws-123"
	checksum := "abc123def456"
	ctx := context.Background()

	tests := []struct {
		name        string
		setupMock   func(*mocks.MockWorkspaceRepository, *sql.DB, sqlmock.Sqlmock)
		wantErr     bool
		errMsg      string
		checkResult func(*testing.T, *domain.AttachmentRecord)
	}{
		{
			name: "successful retrieval",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				rows := sqlmock.NewRows([]string{"checksum", "content", "content_type", "size_bytes"}).
					AddRow(checksum, []byte("test content"), "application/pdf", int64(12))

				mock.ExpectQuery(`SELECT checksum, content, content_type, size_bytes FROM message_attachments WHERE checksum = \$1`).
					WithArgs(checksum).
					WillReturnRows(rows)
			},
			wantErr: false,
			checkResult: func(t *testing.T, record *domain.AttachmentRecord) {
				assert.NotNil(t, record)
				assert.Equal(t, checksum, record.Checksum)
				assert.Equal(t, []byte("test content"), record.Content)
				assert.Equal(t, "application/pdf", record.ContentType)
				assert.Equal(t, int64(12), record.SizeBytes)
			},
		},
		{
			name: "attachment not found",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				mock.ExpectQuery(`SELECT checksum, content, content_type, size_bytes FROM message_attachments WHERE checksum = \$1`).
					WithArgs(checksum).
					WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
			errMsg:  "attachment not found",
		},
		{
			name: "workspace connection error",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(nil, errors.New("connection failed"))
			},
			wantErr: true,
			errMsg:  "failed to get workspace connection",
		},
		{
			name: "database query error",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				mock.ExpectQuery(`SELECT checksum, content, content_type, size_bytes FROM message_attachments WHERE checksum = \$1`).
					WithArgs(checksum).
					WillReturnError(errors.New("query error"))
			},
			wantErr: true,
			errMsg:  "failed to get attachment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			db, mock, cleanup := setupMockDB(t)
			defer cleanup()

			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			tt.setupMock(workspaceRepo, db, mock)

			repo := NewAttachmentRepository(workspaceRepo)
			record, err := repo.Get(ctx, workspaceID, checksum)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, record)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAttachmentRepository_Exists(t *testing.T) {
	workspaceID := "ws-123"
	checksum := "abc123def456"
	ctx := context.Background()

	tests := []struct {
		name       string
		setupMock  func(*mocks.MockWorkspaceRepository, *sql.DB, sqlmock.Sqlmock)
		wantErr    bool
		wantExists bool
		errMsg     string
	}{
		{
			name: "attachment exists",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)

				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM message_attachments WHERE checksum = \$1\)`).
					WithArgs(checksum).
					WillReturnRows(rows)
			},
			wantErr:    false,
			wantExists: true,
		},
		{
			name: "attachment does not exist",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)

				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM message_attachments WHERE checksum = \$1\)`).
					WithArgs(checksum).
					WillReturnRows(rows)
			},
			wantErr:    false,
			wantExists: false,
		},
		{
			name: "workspace connection error",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(nil, errors.New("connection failed"))
			},
			wantErr:    true,
			wantExists: false,
			errMsg:     "failed to get workspace connection",
		},
		{
			name: "database query error",
			setupMock: func(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, mock sqlmock.Sqlmock) {
				workspaceRepo.EXPECT().
					GetConnection(ctx, workspaceID).
					Return(db, nil)

				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM message_attachments WHERE checksum = \$1\)`).
					WithArgs(checksum).
					WillReturnError(errors.New("query error"))
			},
			wantErr:    true,
			wantExists: false,
			errMsg:     "failed to check attachment existence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			db, mock, cleanup := setupMockDB(t)
			defer cleanup()

			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			tt.setupMock(workspaceRepo, db, mock)

			repo := NewAttachmentRepository(workspaceRepo)
			exists, err := repo.Exists(ctx, workspaceID, checksum)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantExists, exists)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAttachmentRepository_StoreAndRetrieve_Integration(t *testing.T) {
	// This test verifies the Store and Get methods work together correctly
	workspaceID := "ws-123"
	ctx := context.Background()
	checksum := "abc123def456"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Setup Store mock
	workspaceRepo.EXPECT().
		GetConnection(ctx, workspaceID).
		Return(db, nil)

	mock.ExpectExec(`INSERT INTO message_attachments`).
		WithArgs(checksum, []byte("test content"), "application/pdf", int64(12)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Setup Get mock
	workspaceRepo.EXPECT().
		GetConnection(ctx, workspaceID).
		Return(db, nil)

	rows := sqlmock.NewRows([]string{"checksum", "content", "content_type", "size_bytes"}).
		AddRow(checksum, []byte("test content"), "application/pdf", int64(12))

	mock.ExpectQuery(`SELECT checksum, content, content_type, size_bytes FROM message_attachments WHERE checksum = \$1`).
		WithArgs(checksum).
		WillReturnRows(rows)

	repo := NewAttachmentRepository(workspaceRepo)

	// Store
	record := &domain.AttachmentRecord{
		Checksum:    checksum,
		Content:     []byte("test content"),
		ContentType: "application/pdf",
		SizeBytes:   12,
	}
	err := repo.Store(ctx, workspaceID, record)
	require.NoError(t, err)

	// Get
	retrieved, err := repo.Get(ctx, workspaceID, checksum)
	require.NoError(t, err)
	assert.Equal(t, record.Checksum, retrieved.Checksum)
	assert.Equal(t, record.Content, retrieved.Content)
	assert.Equal(t, record.ContentType, retrieved.ContentType)
	assert.Equal(t, record.SizeBytes, retrieved.SizeBytes)

	assert.NoError(t, mock.ExpectationsWereMet())
}
