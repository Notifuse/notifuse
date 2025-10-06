package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// AttachmentService handles attachment storage and retrieval
type AttachmentService struct {
	attachmentRepo domain.AttachmentRepository
}

// NewAttachmentService creates a new attachment service
func NewAttachmentService(attachmentRepo domain.AttachmentRepository) *AttachmentService {
	return &AttachmentService{
		attachmentRepo: attachmentRepo,
	}
}

// ProcessAttachments processes and stores attachments, returning metadata
func (s *AttachmentService) ProcessAttachments(ctx context.Context, workspaceID string, attachments []domain.Attachment) ([]domain.AttachmentMetadata, error) {
	if len(attachments) == 0 {
		return nil, nil
	}

	// Validate all attachments first
	if err := domain.ValidateAttachments(attachments); err != nil {
		return nil, err
	}

	metadata := make([]domain.AttachmentMetadata, 0, len(attachments))

	for i, att := range attachments {
		// Detect content type if not provided
		if err := att.DetectContentType(); err != nil {
			return nil, fmt.Errorf("attachment %d: failed to detect content type: %w", i, err)
		}

		// Calculate checksum
		checksum, err := att.CalculateChecksum()
		if err != nil {
			return nil, fmt.Errorf("attachment %d: failed to calculate checksum: %w", i, err)
		}

		// Check if attachment already exists (deduplication)
		exists, err := s.attachmentRepo.Exists(ctx, workspaceID, checksum)
		if err != nil {
			return nil, fmt.Errorf("attachment %d: failed to check existence: %w", i, err)
		}

		// Store attachment if it doesn't exist
		if !exists {
			content, err := att.DecodeContent()
			if err != nil {
				return nil, fmt.Errorf("attachment %d: failed to decode content: %w", i, err)
			}

			record := &domain.AttachmentRecord{
				Checksum:    checksum,
				Content:     content,
				ContentType: att.ContentType,
				SizeBytes:   int64(len(content)),
			}

			if err := s.attachmentRepo.Store(ctx, workspaceID, record); err != nil {
				return nil, fmt.Errorf("attachment %d: failed to store: %w", i, err)
			}
		}

		// Add metadata
		metadata = append(metadata, *att.ToMetadata(checksum))
	}

	return metadata, nil
}

// GetAttachment retrieves an attachment by checksum
func (s *AttachmentService) GetAttachment(ctx context.Context, workspaceID string, checksum string) (*domain.AttachmentRecord, error) {
	return s.attachmentRepo.Get(ctx, workspaceID, checksum)
}

// GetAttachmentsForMessage retrieves all attachments for a message
func (s *AttachmentService) GetAttachmentsForMessage(ctx context.Context, workspaceID string, metadata []domain.AttachmentMetadata) ([]domain.Attachment, error) {
	if len(metadata) == 0 {
		return nil, nil
	}

	attachments := make([]domain.Attachment, 0, len(metadata))

	for _, meta := range metadata {
		_, err := s.attachmentRepo.Get(ctx, workspaceID, meta.Checksum)
		if err != nil {
			return nil, fmt.Errorf("failed to get attachment %s: %w", meta.Checksum, err)
		}

		attachments = append(attachments, domain.Attachment{
			Filename:    meta.Filename,
			ContentType: meta.ContentType,
			Disposition: meta.Disposition,
			// Content is stored as binary, not returned in this method
		})
	}

	return attachments, nil
}

// RetrieveAttachmentContent retrieves attachment content for sending emails
func (s *AttachmentService) RetrieveAttachmentContent(ctx context.Context, workspaceID string, metadata []domain.AttachmentMetadata) ([]AttachmentWithContent, error) {
	if len(metadata) == 0 {
		return nil, nil
	}

	result := make([]AttachmentWithContent, 0, len(metadata))

	for _, meta := range metadata {
		record, err := s.attachmentRepo.Get(ctx, workspaceID, meta.Checksum)
		if err != nil {
			return nil, fmt.Errorf("failed to get attachment %s: %w", meta.Checksum, err)
		}

		result = append(result, AttachmentWithContent{
			Filename:    meta.Filename,
			Content:     record.Content,
			ContentType: meta.ContentType,
			Disposition: meta.Disposition,
		})
	}

	return result, nil
}

// AttachmentWithContent represents an attachment with binary content
type AttachmentWithContent struct {
	Filename    string
	Content     []byte
	ContentType string
	Disposition string
}
