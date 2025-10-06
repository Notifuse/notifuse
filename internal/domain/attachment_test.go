package domain

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachment_Validate(t *testing.T) {
	validBase64 := base64.StdEncoding.EncodeToString([]byte("test content"))

	tests := []struct {
		name       string
		attachment Attachment
		wantErr    bool
		errMsg     string
	}{
		{
			name: "valid attachment with all fields",
			attachment: Attachment{
				Filename:    "test.pdf",
				Content:     validBase64,
				ContentType: "application/pdf",
				Disposition: "attachment",
			},
			wantErr: false,
		},
		{
			name: "valid attachment with minimal fields",
			attachment: Attachment{
				Filename: "document.txt",
				Content:  validBase64,
			},
			wantErr: false,
		},
		{
			name: "valid attachment with inline disposition",
			attachment: Attachment{
				Filename:    "image.png",
				Content:     validBase64,
				ContentType: "image/png",
				Disposition: "inline",
			},
			wantErr: false,
		},
		{
			name: "missing filename",
			attachment: Attachment{
				Content:     validBase64,
				ContentType: "application/pdf",
			},
			wantErr: true,
			errMsg:  "filename is required",
		},
		{
			name: "missing content",
			attachment: Attachment{
				Filename:    "test.pdf",
				ContentType: "application/pdf",
			},
			wantErr: true,
			errMsg:  "content is required",
		},
		{
			name: "invalid base64 content",
			attachment: Attachment{
				Filename: "test.pdf",
				Content:  "not-valid-base64!!!",
			},
			wantErr: true,
			errMsg:  "content must be valid base64",
		},
		{
			name: "filename too long",
			attachment: Attachment{
				Filename: strings.Repeat("a", 256) + ".pdf",
				Content:  validBase64,
			},
			wantErr: true,
			errMsg:  "filename must be less than 255 characters",
		},
		{
			name: "filename with path separator /",
			attachment: Attachment{
				Filename: "../etc/passwd",
				Content:  validBase64,
			},
			wantErr: true,
			errMsg:  "filename must not contain path separators",
		},
		{
			name: "filename with path separator \\",
			attachment: Attachment{
				Filename: "..\\windows\\system32",
				Content:  validBase64,
			},
			wantErr: true,
			errMsg:  "filename must not contain path separators",
		},
		{
			name: "invalid disposition",
			attachment: Attachment{
				Filename:    "test.pdf",
				Content:     validBase64,
				Disposition: "invalid",
			},
			wantErr: true,
			errMsg:  "disposition must be 'attachment' or 'inline'",
		},
		{
			name: "unsupported .exe extension",
			attachment: Attachment{
				Filename: "malware.exe",
				Content:  validBase64,
			},
			wantErr: true,
			errMsg:  "file extension .exe is not supported by email service providers",
		},
		{
			name: "unsupported .bat extension",
			attachment: Attachment{
				Filename: "script.bat",
				Content:  validBase64,
			},
			wantErr: true,
			errMsg:  "file extension .bat is not supported by email service providers",
		},
		{
			name: "unsupported .js extension",
			attachment: Attachment{
				Filename: "code.js",
				Content:  validBase64,
			},
			wantErr: true,
			errMsg:  "file extension .js is not supported by email service providers",
		},
		{
			name: "case insensitive extension check - .EXE",
			attachment: Attachment{
				Filename: "PROGRAM.EXE",
				Content:  validBase64,
			},
			wantErr: true,
			errMsg:  "file extension .exe is not supported by email service providers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.attachment.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				// Verify default disposition is set
				if tt.attachment.Disposition == "" {
					assert.Equal(t, "attachment", tt.attachment.Disposition)
				}
			}
		})
	}
}

func TestAttachment_DecodeContent(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantResult []byte
		wantErr    bool
	}{
		{
			name:       "valid base64 content",
			content:    base64.StdEncoding.EncodeToString([]byte("hello world")),
			wantResult: []byte("hello world"),
			wantErr:    false,
		},
		{
			name:       "empty content",
			content:    "",
			wantResult: []byte{},
			wantErr:    false,
		},
		{
			name:    "invalid base64",
			content: "not-valid-base64!!!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			att := &Attachment{Content: tt.content}
			result, err := att.DecodeContent()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantResult, result)
			}
		})
	}
}

func TestAttachment_CalculateChecksum(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantErr  bool
		checkLen int
	}{
		{
			name:     "calculates SHA256 checksum",
			content:  base64.StdEncoding.EncodeToString([]byte("test content")),
			wantErr:  false,
			checkLen: 64, // SHA256 hex string length
		},
		{
			name:    "error on invalid base64",
			content: "invalid-base64!!!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			att := &Attachment{Content: tt.content}
			checksum, err := att.CalculateChecksum()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, checksum, tt.checkLen)
				// Verify it's a valid hex string (only contains 0-9, a-f)
				for _, c := range checksum {
					assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
						"checksum should only contain hex characters")
				}
			}
		})
	}
}

func TestAttachment_CalculateChecksum_Consistency(t *testing.T) {
	// Same content should produce same checksum
	content := base64.StdEncoding.EncodeToString([]byte("consistent content"))
	att1 := &Attachment{Content: content}
	att2 := &Attachment{Content: content}

	checksum1, err1 := att1.CalculateChecksum()
	checksum2, err2 := att2.CalculateChecksum()

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, checksum1, checksum2)

	// Different content should produce different checksum
	differentContent := base64.StdEncoding.EncodeToString([]byte("different content"))
	att3 := &Attachment{Content: differentContent}
	checksum3, err3 := att3.CalculateChecksum()

	require.NoError(t, err3)
	assert.NotEqual(t, checksum1, checksum3)
}

func TestAttachment_DetectContentType(t *testing.T) {
	tests := []struct {
		name               string
		filename           string
		content            []byte
		initialContentType string
		expectedType       string
	}{
		{
			name:               "skip detection when content type already set",
			filename:           "test.pdf",
			content:            []byte("fake pdf content"),
			initialContentType: "application/pdf",
			expectedType:       "application/pdf",
		},
		{
			name:         "detect PDF by extension",
			filename:     "document.pdf",
			content:      []byte("random content"),
			expectedType: "application/pdf",
		},
		{
			name:         "detect DOCX by extension",
			filename:     "report.docx",
			content:      []byte("random content"),
			expectedType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:         "detect XLSX by extension",
			filename:     "spreadsheet.xlsx",
			content:      []byte("random content"),
			expectedType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:         "detect ZIP by extension",
			filename:     "archive.zip",
			content:      []byte("random content"),
			expectedType: "application/zip",
		},
		{
			name:         "detect CSV by extension",
			filename:     "data.csv",
			content:      []byte("name,email\nJohn,john@example.com"),
			expectedType: "text/csv",
		},
		{
			name:         "detect JSON by extension",
			filename:     "config.json",
			content:      []byte(`{"key": "value"}`),
			expectedType: "application/json",
		},
		{
			name:         "detect PNG by extension",
			filename:     "image.png",
			content:      []byte("fake png"),
			expectedType: "image/png",
		},
		{
			name:         "detect JPEG by .jpg extension",
			filename:     "photo.jpg",
			content:      []byte("fake jpeg"),
			expectedType: "image/jpeg",
		},
		{
			name:         "detect JPEG by .jpeg extension",
			filename:     "photo.jpeg",
			content:      []byte("fake jpeg"),
			expectedType: "image/jpeg",
		},
		{
			name:         "detect GIF by extension",
			filename:     "animation.gif",
			content:      []byte("fake gif"),
			expectedType: "image/gif",
		},
		{
			name:         "detect WebP by extension",
			filename:     "modern.webp",
			content:      []byte("fake webp"),
			expectedType: "image/webp",
		},
		{
			name:         "detect SVG by extension",
			filename:     "icon.svg",
			content:      []byte("<svg></svg>"),
			expectedType: "image/svg+xml",
		},
		{
			name:         "case insensitive extension - .PDF",
			filename:     "DOCUMENT.PDF",
			content:      []byte("content"),
			expectedType: "application/pdf",
		},
		{
			name:         "case insensitive extension - .PNG",
			filename:     "IMAGE.PNG",
			content:      []byte("content"),
			expectedType: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := base64.StdEncoding.EncodeToString(tt.content)
			att := &Attachment{
				Filename:    tt.filename,
				Content:     content,
				ContentType: tt.initialContentType,
			}

			err := att.DetectContentType()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedType, att.ContentType)
		})
	}
}

func TestAttachment_ToMetadata(t *testing.T) {
	att := &Attachment{
		Filename:    "test.pdf",
		Content:     base64.StdEncoding.EncodeToString([]byte("content")),
		ContentType: "application/pdf",
		Disposition: "attachment",
	}

	checksum := "abc123def456"
	metadata := att.ToMetadata(checksum)

	assert.NotNil(t, metadata)
	assert.Equal(t, checksum, metadata.Checksum)
	assert.Equal(t, att.Filename, metadata.Filename)
	assert.Equal(t, att.ContentType, metadata.ContentType)
	assert.Equal(t, att.Disposition, metadata.Disposition)
}

func TestValidateAttachments(t *testing.T) {
	validContent := base64.StdEncoding.EncodeToString([]byte("test content"))
	largeContent := base64.StdEncoding.EncodeToString(make([]byte, 4*1024*1024)) // 4MB

	tests := []struct {
		name        string
		attachments []Attachment
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "nil attachments",
			attachments: nil,
			wantErr:     false,
		},
		{
			name:        "empty attachments",
			attachments: []Attachment{},
			wantErr:     false,
		},
		{
			name: "single valid attachment",
			attachments: []Attachment{
				{
					Filename: "test.pdf",
					Content:  validContent,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple valid attachments",
			attachments: []Attachment{
				{Filename: "doc1.pdf", Content: validContent},
				{Filename: "doc2.pdf", Content: validContent},
				{Filename: "doc3.pdf", Content: validContent},
			},
			wantErr: false,
		},
		{
			name: "too many attachments (21)",
			attachments: func() []Attachment {
				atts := make([]Attachment, 21)
				for i := 0; i < 21; i++ {
					atts[i] = Attachment{
						Filename: "file.pdf",
						Content:  validContent,
					}
				}
				return atts
			}(),
			wantErr: true,
			errMsg:  "maximum 20 attachments allowed, got 21",
		},
		{
			name: "attachment too large (4MB)",
			attachments: []Attachment{
				{
					Filename: "large.pdf",
					Content:  largeContent,
				},
			},
			wantErr: true,
			errMsg:  "size",
		},
		{
			name: "total size exceeds limit",
			attachments: func() []Attachment {
				// Create 4 attachments of ~3MB each (total > 10MB)
				content := base64.StdEncoding.EncodeToString(make([]byte, 3*1024*1024))
				return []Attachment{
					{Filename: "file1.pdf", Content: content},
					{Filename: "file2.pdf", Content: content},
					{Filename: "file3.pdf", Content: content},
					{Filename: "file4.pdf", Content: content},
				}
			}(),
			wantErr: true,
			errMsg:  "total attachment size",
		},
		{
			name: "invalid attachment in list",
			attachments: []Attachment{
				{Filename: "valid.pdf", Content: validContent},
				{Filename: "invalid.exe", Content: validContent}, // Unsupported extension
			},
			wantErr: true,
			errMsg:  "attachment 1",
		},
		{
			name: "attachment with invalid base64",
			attachments: []Attachment{
				{Filename: "test.pdf", Content: "invalid-base64!!!"},
			},
			wantErr: true,
			errMsg:  "attachment 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAttachments(tt.attachments)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAttachments_SizeLimits(t *testing.T) {
	t.Run("exactly 3MB should pass", func(t *testing.T) {
		content := base64.StdEncoding.EncodeToString(make([]byte, 3*1024*1024))
		attachments := []Attachment{
			{Filename: "exactly3mb.pdf", Content: content},
		}
		err := ValidateAttachments(attachments)
		assert.NoError(t, err)
	})

	t.Run("exactly 10MB total should pass", func(t *testing.T) {
		// Create attachments that sum to exactly 10MB
		size := 2*1024*1024 + 500*1024 // 2.5MB each
		content := base64.StdEncoding.EncodeToString(make([]byte, size))
		attachments := []Attachment{
			{Filename: "file1.pdf", Content: content},
			{Filename: "file2.pdf", Content: content},
			{Filename: "file3.pdf", Content: content},
			{Filename: "file4.pdf", Content: content},
		}
		err := ValidateAttachments(attachments)
		assert.NoError(t, err)
	})

	t.Run("20 attachments should pass", func(t *testing.T) {
		smallContent := base64.StdEncoding.EncodeToString(make([]byte, 100*1024)) // 100KB
		attachments := make([]Attachment, 20)
		for i := 0; i < 20; i++ {
			attachments[i] = Attachment{
				Filename: "small.pdf",
				Content:  smallContent,
			}
		}
		err := ValidateAttachments(attachments)
		assert.NoError(t, err)
	})
}
