package domain

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

//go:generate mockgen -destination mocks/mock_attachment_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AttachmentRepository

// Attachment represents an email attachment
type Attachment struct {
	Filename    string `json:"filename" validate:"required"`
	Content     string `json:"content" validate:"required"` // base64 encoded
	ContentType string `json:"content_type,omitempty"`
	Disposition string `json:"disposition,omitempty"` // "attachment" (default) or "inline"
}

// AttachmentMetadata represents metadata stored in message_history
type AttachmentMetadata struct {
	Checksum    string `json:"checksum"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Disposition string `json:"disposition"`
}

// AttachmentRecord represents a stored attachment in the database
type AttachmentRecord struct {
	Checksum    string
	Content     []byte
	ContentType string
	SizeBytes   int64
}

// AttachmentRepository defines methods for attachment persistence
type AttachmentRepository interface {
	// Store saves an attachment and returns its checksum
	Store(ctx context.Context, workspaceID string, record *AttachmentRecord) error

	// Get retrieves an attachment by checksum
	Get(ctx context.Context, workspaceID string, checksum string) (*AttachmentRecord, error)

	// Exists checks if an attachment exists by checksum
	Exists(ctx context.Context, workspaceID string, checksum string) (bool, error)
}

// unsupportedFileExtensions lists file extensions that are blocked by major ESPs
// including AWS SES as documented at https://docs.aws.amazon.com/ses/latest/dg/attachments.html
var unsupportedFileExtensions = []string{
	".ade", ".adp", ".app", ".asp", ".bas", ".bat", ".cer", ".chm", ".cmd", ".com", ".cpl", ".crt", ".csh", ".der",
	".exe", ".fxp", ".gadget", ".hlp", ".hta", ".inf", ".ins", ".isp", ".its", ".js", ".jse", ".ksh", ".lib", ".lnk",
	".mad", ".maf", ".mag", ".mam", ".maq", ".mar", ".mas", ".mat", ".mau", ".mav", ".maw", ".mda", ".mdb", ".mde",
	".mdt", ".mdw", ".mdz", ".msc", ".msh", ".msh1", ".msh2", ".mshxml", ".msh1xml", ".msh2xml", ".msi", ".msp", ".mst",
	".ops", ".pcd", ".pif", ".plg", ".prf", ".prg", ".reg", ".scf", ".scr", ".sct", ".shb", ".shs", ".sys", ".ps1",
	".ps1xml", ".ps2", ".ps2xml", ".psc1", ".psc2", ".tmp", ".url", ".vb", ".vbe", ".vbs", ".vps", ".vsmacros", ".vss",
	".vst", ".vsw", ".vxd", ".ws", ".wsc", ".wsf", ".wsh", ".xnk",
}

// Validate validates an attachment
func (a *Attachment) Validate() error {
	if a.Filename == "" {
		return fmt.Errorf("filename is required")
	}

	// Check filename length
	if len(a.Filename) > 255 {
		return fmt.Errorf("filename must be less than 255 characters")
	}

	// Check for path separators
	if strings.ContainsAny(a.Filename, "/\\") {
		return fmt.Errorf("filename must not contain path separators")
	}

	// Check for unsupported file extensions (AWS SES and other ESPs)
	ext := strings.ToLower(filepath.Ext(a.Filename))
	for _, unsupportedExt := range unsupportedFileExtensions {
		if ext == unsupportedExt {
			return fmt.Errorf("file extension %s is not supported by email service providers", ext)
		}
	}

	if a.Content == "" {
		return fmt.Errorf("content is required")
	}

	// Validate base64 encoding
	if _, err := base64.StdEncoding.DecodeString(a.Content); err != nil {
		return fmt.Errorf("content must be valid base64: %w", err)
	}

	// Validate disposition
	if a.Disposition != "" && a.Disposition != "attachment" && a.Disposition != "inline" {
		return fmt.Errorf("disposition must be 'attachment' or 'inline'")
	}

	// Set default disposition
	if a.Disposition == "" {
		a.Disposition = "attachment"
	}

	return nil
}

// DecodeContent decodes the base64 content
func (a *Attachment) DecodeContent() ([]byte, error) {
	return base64.StdEncoding.DecodeString(a.Content)
}

// CalculateChecksum calculates SHA256 checksum of the content
func (a *Attachment) CalculateChecksum() (string, error) {
	content, err := a.DecodeContent()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

// DetectContentType detects content type if not provided
func (a *Attachment) DetectContentType() error {
	if a.ContentType != "" {
		return nil
	}

	content, err := a.DecodeContent()
	if err != nil {
		return err
	}

	// Detect content type
	contentType := http.DetectContentType(content)

	// Use file extension as fallback for better MIME type detection
	if contentType == "application/octet-stream" || strings.HasPrefix(contentType, "text/plain") {
		ext := strings.ToLower(filepath.Ext(a.Filename))
		switch ext {
		// Document types
		case ".pdf":
			contentType = "application/pdf"
		case ".doc":
			contentType = "application/msword"
		case ".docx":
			contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		case ".xls":
			contentType = "application/vnd.ms-excel"
		case ".xlsx":
			contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		case ".ppt":
			contentType = "application/vnd.ms-powerpoint"
		case ".pptx":
			contentType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
		case ".zip":
			contentType = "application/zip"
		case ".csv":
			contentType = "text/csv"
		case ".txt":
			contentType = "text/plain"
		case ".json":
			contentType = "application/json"
		case ".xml":
			contentType = "application/xml"
		// Image types
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		case ".svg":
			contentType = "image/svg+xml"
		case ".bmp":
			contentType = "image/bmp"
		case ".ico":
			contentType = "image/x-icon"
		case ".tiff", ".tif":
			contentType = "image/tiff"
		}
	}

	a.ContentType = contentType
	return nil
}

// ToMetadata converts an Attachment to AttachmentMetadata
func (a *Attachment) ToMetadata(checksum string) *AttachmentMetadata {
	return &AttachmentMetadata{
		Checksum:    checksum,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		Disposition: a.Disposition,
	}
}

// ValidateAttachments validates a slice of attachments
func ValidateAttachments(attachments []Attachment) error {
	if len(attachments) == 0 {
		return nil
	}

	// Check maximum number of attachments
	if len(attachments) > 20 {
		return fmt.Errorf("maximum 20 attachments allowed, got %d", len(attachments))
	}

	totalSize := int64(0)
	for i, att := range attachments {
		if err := att.Validate(); err != nil {
			return fmt.Errorf("attachment %d: %w", i, err)
		}

		content, err := att.DecodeContent()
		if err != nil {
			return fmt.Errorf("attachment %d: failed to decode content: %w", i, err)
		}

		size := int64(len(content))

		// Check individual file size (3MB per file recommended)
		const maxFileSize = 3 * 1024 * 1024
		if size > maxFileSize {
			return fmt.Errorf("attachment %d (%s): size %d bytes exceeds maximum of %d bytes (3MB)",
				i, att.Filename, size, maxFileSize)
		}

		totalSize += size
	}

	// Check total size (10MB total, well under AWS SES 40MB limit)
	// AWS SES allows up to 40MB total message size as documented at:
	// https://docs.aws.amazon.com/ses/latest/dg/attachments.html
	// We use 10MB as a conservative limit to account for email body and headers
	const maxTotalSize = 10 * 1024 * 1024
	if totalSize > maxTotalSize {
		return fmt.Errorf("total attachment size %d bytes exceeds maximum of %d bytes (10MB)",
			totalSize, maxTotalSize)
	}

	return nil
}
