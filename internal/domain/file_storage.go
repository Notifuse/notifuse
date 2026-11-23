package domain

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"time"
	"unicode"
)

//go:generate mockgen -destination mocks/mock_file_storage_service.go -package mocks github.com/Notifuse/notifuse/internal/domain FileStorageService

const (
	// MaxListedStorageObjects is the most objects a listing returns, as the console's file
	// manager has always read at most.
	MaxListedStorageObjects = 10_000

	// MaxDeletedStorageKeys is the most keys one files.delete call accepts.
	MaxDeletedStorageKeys = 1000

	// StorageUploadURLExpiry is how long a presigned upload URL stays valid.
	StorageUploadURLExpiry = 5 * time.Minute

	maxStorageKeyBytes          = 1024
	maxStorageContentTypeLength = 255
	defaultStorageContentType   = "application/octet-stream"
)

// StoredObject is one object of the workspace's bucket.
type StoredObject struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// StoredObjectList is a listing of the bucket. Capped is set when the listing stopped at
// MaxListedStorageObjects, CutShort when the storage stopped answering before the end.
type StoredObjectList struct {
	Objects  []StoredObject `json:"objects"`
	Capped   bool           `json:"capped"`
	CutShort bool           `json:"cut_short"`
}

// PresignUploadRequest asks for a URL that uploads one file.
type PresignUploadRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	ContentType string `json:"content_type"`
	// Size is the size the client announces. It is not signed: no size limit applies.
	Size int64 `json:"size"`
}

// PresignedUpload is a URL that accepts one PUT of the file, with the headers the PUT must
// carry exactly as given, since the signature covers them.
type PresignedUpload struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// Folder creation outcomes.
const (
	FolderCreated = "created"
	FolderExists  = "exists"
)

// CreateFolderResult says whether the folder marker was written or something already lived
// under that prefix.
type CreateFolderResult struct {
	Status string `json:"status"`
}

// DeleteFilesResult counts the keys deleted and those the storage refused.
type DeleteFilesResult struct {
	Deleted int `json:"deleted"`
	Failed  int `json:"failed"`
}

// DeleteFolderResult counts every object deleted or refused, and the same counts without
// the folder markers, for a message that speaks of files.
type DeleteFolderResult struct {
	Deleted      int `json:"deleted"`
	Failed       int `json:"failed"`
	DeletedFiles int `json:"deleted_files"`
	FailedFiles  int `json:"failed_files"`
}

// FileStorageError is the storage provider refusing or failing a call. Its message is the
// provider's, so an owner can tell a wrong endpoint from a wrong key.
type FileStorageError struct {
	Message string
}

func (e *FileStorageError) Error() string {
	return "file storage: " + e.Message
}

// FileStorageService lists, uploads to and deletes from a workspace's bucket. The bucket's
// secret never leaves the server: uploads go through a presigned URL.
type FileStorageService interface {
	List(ctx context.Context, workspaceID, prefix string) (*StoredObjectList, error)
	PresignUpload(ctx context.Context, req PresignUploadRequest) (*PresignedUpload, error)
	CreateFolder(ctx context.Context, workspaceID, key string) (*CreateFolderResult, error)
	Delete(ctx context.Context, workspaceID string, keys []string) (*DeleteFilesResult, error)
	DeleteFolder(ctx context.Context, workspaceID, prefix string) (*DeleteFolderResult, error)
	// TestSettings checks that settings reach their bucket, for the owner's settings form.
	// An empty secret means the stored one, for the same access key and host only.
	TestSettings(ctx context.Context, workspaceID string, settings FileManagerSettings) error
}

// CanReadFiles reports whether the member may list the workspace's files: any right, read
// or write, on templates or the blog, the two screens that pick images.
func (uw *UserWorkspace) CanReadFiles() bool {
	return uw.CanWriteFiles() ||
		uw.HasPermission(PermissionResourceTemplates, PermissionTypeRead) ||
		uw.HasPermission(PermissionResourceBlog, PermissionTypeRead)
}

// CanWriteFiles reports whether the member may upload and delete files: write access to
// templates or to the blog.
func (uw *UserWorkspace) CanWriteFiles() bool {
	return uw.HasPermission(PermissionResourceTemplates, PermissionTypeWrite) ||
		uw.HasPermission(PermissionResourceBlog, PermissionTypeWrite)
}

func hasControlCharacter(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// ValidateStorageKeyForDelete checks the loose rule for a key that already exists: the
// file manager shows objects other tools created, with a leading slash or "//", and they
// must stay deletable.
func ValidateStorageKeyForDelete(key string) error {
	if key == "" {
		return NewValidationError("a key is required")
	}
	if len(key) > maxStorageKeyBytes {
		return NewValidationError(fmt.Sprintf("a key is at most %d bytes", maxStorageKeyBytes))
	}
	if hasControlCharacter(key) {
		return NewValidationError("a key cannot contain control characters")
	}
	return nil
}

// ValidateStorageKey checks a key the server writes or signs. On top of the delete rule it
// refuses a leading slash, "//" and "." or ".." segments: a browser resolves those in a
// URL, which would then no longer match the signed path.
func ValidateStorageKey(key string) error {
	if err := ValidateStorageKeyForDelete(key); err != nil {
		return err
	}
	if strings.HasPrefix(key, "/") {
		return NewValidationError("a key cannot start with /")
	}
	if strings.Contains(key, "//") {
		return NewValidationError("a key cannot contain //")
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "." || segment == ".." {
			return NewValidationError(`a key cannot contain "." or ".." segments`)
		}
	}
	return nil
}

// ValidateStorageFolderPrefix checks a folder prefix: a valid key ending in "/". An empty
// prefix is refused, since it would name the whole bucket.
func ValidateStorageFolderPrefix(prefix string) error {
	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		return NewValidationError(fmt.Sprintf("not a folder prefix: %q", prefix))
	}
	return ValidateStorageKey(prefix)
}

// NormalizeStorageContentType returns the Content-Type an upload is signed with:
// application/octet-stream when the browser knows no type, otherwise the given type once
// checked, since it goes into a signed header.
func NormalizeStorageContentType(contentType string) (string, error) {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return defaultStorageContentType, nil
	}
	if len(contentType) > maxStorageContentTypeLength || hasControlCharacter(contentType) {
		return "", NewValidationError("invalid content type")
	}
	if _, _, err := mime.ParseMediaType(contentType); err != nil {
		return "", NewValidationError("invalid content type")
	}
	return contentType, nil
}

// ValidateStorageEndpoint checks that a file storage endpoint is an http or https URL with
// a host, and nothing that would change where a signed request goes: no user info, query
// or fragment.
func ValidateStorageEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return NewValidationError("the endpoint must be an http or https URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return NewValidationError("the endpoint cannot carry credentials, a query or a fragment")
	}
	return nil
}

// SameStorageAccount reports whether two settings point the same access key at the same
// host. A stored secret is only reused for settings that do: signing a request for
// another host would hand that host a signature it could test guesses of the secret
// against.
func SameStorageAccount(a, b FileManagerSettings) bool {
	return a.AccessKey == b.AccessKey && storageHost(a.Endpoint) == storageHost(b.Endpoint)
}

func storageHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimRight(endpoint, "/"))
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}
