package service

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAttachmentService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAttachmentRepository(ctrl)
	service := NewAttachmentService(mockRepo)

	assert.NotNil(t, service)
	assert.Equal(t, mockRepo, service.attachmentRepo)
}

func TestAttachmentService_ProcessAttachments(t *testing.T) {
	validContent := base64.StdEncoding.EncodeToString([]byte("test content"))
	workspaceID := "ws-123"
	ctx := context.Background()

	tests := []struct {
		name        string
		attachments []domain.Attachment
		setupMock   func(*mocks.MockAttachmentRepository)
		wantErr     bool
		errMsg      string
		checkResult func(*testing.T, []domain.AttachmentMetadata)
	}{
		{
			name:        "nil attachments returns nil",
			attachments: nil,
			setupMock:   func(m *mocks.MockAttachmentRepository) {},
			wantErr:     false,
			checkResult: func(t *testing.T, metadata []domain.AttachmentMetadata) {
				assert.Nil(t, metadata)
			},
		},
		{
			name:        "empty attachments returns nil",
			attachments: []domain.Attachment{},
			setupMock:   func(m *mocks.MockAttachmentRepository) {},
			wantErr:     false,
			checkResult: func(t *testing.T, metadata []domain.AttachmentMetadata) {
				assert.Nil(t, metadata)
			},
		},
		{
			name: "single attachment - new attachment",
			attachments: []domain.Attachment{
				{
					Filename:    "test.pdf",
					Content:     validContent,
					ContentType: "application/pdf",
					Disposition: "attachment",
				},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Exists(ctx, workspaceID, gomock.Any()).
					Return(false, nil).
					Times(1)

				m.EXPECT().
					Store(ctx, workspaceID, gomock.Any()).
					DoAndReturn(func(ctx context.Context, wID string, record *domain.AttachmentRecord) error {
						assert.Equal(t, workspaceID, wID)
						assert.NotEmpty(t, record.Checksum)
						assert.Equal(t, []byte("test content"), record.Content)
						assert.Equal(t, "application/pdf", record.ContentType)
						assert.Equal(t, int64(12), record.SizeBytes)
						return nil
					}).
					Times(1)
			},
			wantErr: false,
			checkResult: func(t *testing.T, metadata []domain.AttachmentMetadata) {
				require.Len(t, metadata, 1)
				assert.Equal(t, "test.pdf", metadata[0].Filename)
				assert.Equal(t, "application/pdf", metadata[0].ContentType)
				assert.Equal(t, "attachment", metadata[0].Disposition)
				assert.NotEmpty(t, metadata[0].Checksum)
			},
		},
		{
			name: "single attachment - existing attachment (deduplication)",
			attachments: []domain.Attachment{
				{
					Filename:    "existing.pdf",
					Content:     validContent,
					ContentType: "application/pdf",
					Disposition: "attachment",
				},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Exists(ctx, workspaceID, gomock.Any()).
					Return(true, nil).
					Times(1)

				// Store should NOT be called for existing attachment
				m.EXPECT().
					Store(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			},
			wantErr: false,
			checkResult: func(t *testing.T, metadata []domain.AttachmentMetadata) {
				require.Len(t, metadata, 1)
				assert.Equal(t, "existing.pdf", metadata[0].Filename)
			},
		},
		{
			name: "multiple attachments - mixed new and existing",
			attachments: []domain.Attachment{
				{Filename: "new.pdf", Content: validContent},
				{Filename: "existing.pdf", Content: validContent},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				// First attachment - new
				m.EXPECT().
					Exists(ctx, workspaceID, gomock.Any()).
					Return(false, nil).
					Times(1)
				m.EXPECT().
					Store(ctx, workspaceID, gomock.Any()).
					Return(nil).
					Times(1)

				// Second attachment - existing
				m.EXPECT().
					Exists(ctx, workspaceID, gomock.Any()).
					Return(true, nil).
					Times(1)
				// No Store call for existing
			},
			wantErr: false,
			checkResult: func(t *testing.T, metadata []domain.AttachmentMetadata) {
				require.Len(t, metadata, 2)
				assert.Equal(t, "new.pdf", metadata[0].Filename)
				assert.Equal(t, "existing.pdf", metadata[1].Filename)
			},
		},
		{
			name: "attachment without content type - auto-detect",
			attachments: []domain.Attachment{
				{
					Filename: "document.pdf",
					Content:  validContent,
					// ContentType not set
				},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Exists(ctx, workspaceID, gomock.Any()).
					Return(false, nil)

				m.EXPECT().
					Store(ctx, workspaceID, gomock.Any()).
					DoAndReturn(func(ctx context.Context, wID string, record *domain.AttachmentRecord) error {
						// Content type should be auto-detected as PDF
						assert.Equal(t, "application/pdf", record.ContentType)
						return nil
					})
			},
			wantErr: false,
			checkResult: func(t *testing.T, metadata []domain.AttachmentMetadata) {
				require.Len(t, metadata, 1)
				assert.Equal(t, "application/pdf", metadata[0].ContentType)
			},
		},
		{
			name: "invalid attachment - validation error",
			attachments: []domain.Attachment{
				{
					Filename: "malware.exe", // Unsupported extension
					Content:  validContent,
				},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				// No mock calls expected
			},
			wantErr: true,
			errMsg:  "file extension .exe is not supported",
		},
		{
			name: "repository exists error",
			attachments: []domain.Attachment{
				{Filename: "test.pdf", Content: validContent},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Exists(ctx, workspaceID, gomock.Any()).
					Return(false, errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to check existence",
		},
		{
			name: "repository store error",
			attachments: []domain.Attachment{
				{Filename: "test.pdf", Content: validContent},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Exists(ctx, workspaceID, gomock.Any()).
					Return(false, nil)

				m.EXPECT().
					Store(ctx, workspaceID, gomock.Any()).
					Return(errors.New("storage error"))
			},
			wantErr: true,
			errMsg:  "failed to store",
		},
		{
			name: "too many attachments",
			attachments: func() []domain.Attachment {
				atts := make([]domain.Attachment, 21)
				for i := 0; i < 21; i++ {
					atts[i] = domain.Attachment{
						Filename: "file.pdf",
						Content:  validContent,
					}
				}
				return atts
			}(),
			setupMock: func(m *mocks.MockAttachmentRepository) {},
			wantErr:   true,
			errMsg:    "maximum 20 attachments allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockAttachmentRepository(ctrl)
			tt.setupMock(mockRepo)

			service := NewAttachmentService(mockRepo)
			metadata, err := service.ProcessAttachments(ctx, workspaceID, tt.attachments)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, metadata)
				}
			}
		})
	}
}

func TestAttachmentService_GetAttachment(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	checksum := "abc123"

	tests := []struct {
		name      string
		setupMock func(*mocks.MockAttachmentRepository)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful retrieval",
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, checksum).
					Return(&domain.AttachmentRecord{
						Checksum:    checksum,
						Content:     []byte("content"),
						ContentType: "application/pdf",
						SizeBytes:   7,
					}, nil)
			},
			wantErr: false,
		},
		{
			name: "repository error",
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, checksum).
					Return(nil, errors.New("not found"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockAttachmentRepository(ctrl)
			tt.setupMock(mockRepo)

			service := NewAttachmentService(mockRepo)
			record, err := service.GetAttachment(ctx, workspaceID, checksum)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, record)
				assert.Equal(t, checksum, record.Checksum)
			}
		})
	}
}

func TestAttachmentService_GetAttachmentsForMessage(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"

	tests := []struct {
		name        string
		metadata    []domain.AttachmentMetadata
		setupMock   func(*mocks.MockAttachmentRepository)
		wantErr     bool
		errMsg      string
		checkResult func(*testing.T, []domain.Attachment)
	}{
		{
			name:     "nil metadata returns nil",
			metadata: nil,
			setupMock: func(m *mocks.MockAttachmentRepository) {
				// No calls expected
			},
			wantErr: false,
			checkResult: func(t *testing.T, attachments []domain.Attachment) {
				assert.Nil(t, attachments)
			},
		},
		{
			name:     "empty metadata returns nil",
			metadata: []domain.AttachmentMetadata{},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				// No calls expected
			},
			wantErr: false,
			checkResult: func(t *testing.T, attachments []domain.Attachment) {
				assert.Nil(t, attachments)
			},
		},
		{
			name: "single metadata",
			metadata: []domain.AttachmentMetadata{
				{
					Checksum:    "abc123",
					Filename:    "test.pdf",
					ContentType: "application/pdf",
					Disposition: "attachment",
				},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, "abc123").
					Return(&domain.AttachmentRecord{
						Checksum:    "abc123",
						Content:     []byte("content"),
						ContentType: "application/pdf",
						SizeBytes:   7,
					}, nil)
			},
			wantErr: false,
			checkResult: func(t *testing.T, attachments []domain.Attachment) {
				require.Len(t, attachments, 1)
				assert.Equal(t, "test.pdf", attachments[0].Filename)
				assert.Equal(t, "application/pdf", attachments[0].ContentType)
				assert.Equal(t, "attachment", attachments[0].Disposition)
				// Content should not be included
				assert.Empty(t, attachments[0].Content)
			},
		},
		{
			name: "multiple metadata",
			metadata: []domain.AttachmentMetadata{
				{Checksum: "abc123", Filename: "file1.pdf", ContentType: "application/pdf"},
				{Checksum: "def456", Filename: "file2.png", ContentType: "image/png"},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, "abc123").
					Return(&domain.AttachmentRecord{Checksum: "abc123"}, nil)

				m.EXPECT().
					Get(ctx, workspaceID, "def456").
					Return(&domain.AttachmentRecord{Checksum: "def456"}, nil)
			},
			wantErr: false,
			checkResult: func(t *testing.T, attachments []domain.Attachment) {
				require.Len(t, attachments, 2)
				assert.Equal(t, "file1.pdf", attachments[0].Filename)
				assert.Equal(t, "file2.png", attachments[1].Filename)
			},
		},
		{
			name: "repository error",
			metadata: []domain.AttachmentMetadata{
				{Checksum: "abc123", Filename: "test.pdf"},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, "abc123").
					Return(nil, errors.New("not found"))
			},
			wantErr: true,
			errMsg:  "failed to get attachment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockAttachmentRepository(ctrl)
			tt.setupMock(mockRepo)

			service := NewAttachmentService(mockRepo)
			attachments, err := service.GetAttachmentsForMessage(ctx, workspaceID, tt.metadata)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, attachments)
				}
			}
		})
	}
}

func TestAttachmentService_RetrieveAttachmentContent(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"

	tests := []struct {
		name        string
		metadata    []domain.AttachmentMetadata
		setupMock   func(*mocks.MockAttachmentRepository)
		wantErr     bool
		errMsg      string
		checkResult func(*testing.T, []AttachmentWithContent)
	}{
		{
			name:     "nil metadata returns nil",
			metadata: nil,
			setupMock: func(m *mocks.MockAttachmentRepository) {
				// No calls expected
			},
			wantErr: false,
			checkResult: func(t *testing.T, result []AttachmentWithContent) {
				assert.Nil(t, result)
			},
		},
		{
			name: "single metadata with content",
			metadata: []domain.AttachmentMetadata{
				{
					Checksum:    "abc123",
					Filename:    "test.pdf",
					ContentType: "application/pdf",
					Disposition: "attachment",
				},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, "abc123").
					Return(&domain.AttachmentRecord{
						Checksum:    "abc123",
						Content:     []byte("binary content"),
						ContentType: "application/pdf",
						SizeBytes:   14,
					}, nil)
			},
			wantErr: false,
			checkResult: func(t *testing.T, result []AttachmentWithContent) {
				require.Len(t, result, 1)
				assert.Equal(t, "test.pdf", result[0].Filename)
				assert.Equal(t, []byte("binary content"), result[0].Content)
				assert.Equal(t, "application/pdf", result[0].ContentType)
				assert.Equal(t, "attachment", result[0].Disposition)
			},
		},
		{
			name: "multiple attachments",
			metadata: []domain.AttachmentMetadata{
				{Checksum: "abc123", Filename: "file1.pdf", ContentType: "application/pdf"},
				{Checksum: "def456", Filename: "file2.png", ContentType: "image/png"},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, "abc123").
					Return(&domain.AttachmentRecord{
						Checksum: "abc123",
						Content:  []byte("pdf content"),
					}, nil)

				m.EXPECT().
					Get(ctx, workspaceID, "def456").
					Return(&domain.AttachmentRecord{
						Checksum: "def456",
						Content:  []byte("png content"),
					}, nil)
			},
			wantErr: false,
			checkResult: func(t *testing.T, result []AttachmentWithContent) {
				require.Len(t, result, 2)
				assert.Equal(t, "file1.pdf", result[0].Filename)
				assert.Equal(t, []byte("pdf content"), result[0].Content)
				assert.Equal(t, "file2.png", result[1].Filename)
				assert.Equal(t, []byte("png content"), result[1].Content)
			},
		},
		{
			name: "repository error",
			metadata: []domain.AttachmentMetadata{
				{Checksum: "abc123", Filename: "test.pdf"},
			},
			setupMock: func(m *mocks.MockAttachmentRepository) {
				m.EXPECT().
					Get(ctx, workspaceID, "abc123").
					Return(nil, errors.New("storage error"))
			},
			wantErr: true,
			errMsg:  "failed to get attachment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockAttachmentRepository(ctrl)
			tt.setupMock(mockRepo)

			service := NewAttachmentService(mockRepo)
			result, err := service.RetrieveAttachmentContent(ctx, workspaceID, tt.metadata)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}
