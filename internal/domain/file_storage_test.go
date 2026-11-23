package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateStorageKey(t *testing.T) {
	valid := []string{"logo.png", "images/logo@2x.png", "a/b/c(1).png", "my photo.png", "café.png", "folder/"}
	for _, key := range valid {
		assert.NoError(t, ValidateStorageKey(key), key)
	}

	invalid := map[string]string{
		"empty":           "",
		"too long":        strings.Repeat("a", 1025),
		"control":         "a\nb.png",
		"leading slash":   "/logo.png",
		"double slash":    "a//b.png",
		"dot segment":     "a/./b.png",
		"dot dot segment": "a/../b.png",
		"only dot dot":    "..",
	}
	for name, key := range invalid {
		assert.Error(t, ValidateStorageKey(key), name)
	}
}

func TestValidateStorageKeyForDelete_AcceptsKeysOtherToolsCreated(t *testing.T) {
	for _, key := range []string{"/leading.png", "a//b.png", "a/../b.png", strings.Repeat("a", 1024)} {
		assert.NoError(t, ValidateStorageKeyForDelete(key), key)
	}
	for _, key := range []string{"", "a\x00b", strings.Repeat("a", 1025)} {
		assert.Error(t, ValidateStorageKeyForDelete(key), key)
	}
}

func TestValidateStorageFolderPrefix(t *testing.T) {
	assert.NoError(t, ValidateStorageFolderPrefix("images/"))
	assert.NoError(t, ValidateStorageFolderPrefix("a/b/"))
	assert.Error(t, ValidateStorageFolderPrefix(""), "an empty prefix would name the whole bucket")
	assert.Error(t, ValidateStorageFolderPrefix("images"), "a/ prefix also matches a/b-sibling without the slash")
	assert.Error(t, ValidateStorageFolderPrefix("/images/"))
	assert.Error(t, ValidateStorageFolderPrefix("a/../"))
}

func TestNormalizeStorageContentType(t *testing.T) {
	got, err := NormalizeStorageContentType("")
	assert.NoError(t, err)
	assert.Equal(t, "application/octet-stream", got, "a file the browser has no type for")

	got, err = NormalizeStorageContentType(" image/png ")
	assert.NoError(t, err)
	assert.Equal(t, "image/png", got)

	got, err = NormalizeStorageContentType("text/plain; charset=utf-8")
	assert.NoError(t, err)
	assert.Equal(t, "text/plain; charset=utf-8", got)

	for _, contentType := range []string{"image/png\r\nX-Evil: 1", "not a type", "image/" + strings.Repeat("x", 300)} {
		_, err := NormalizeStorageContentType(contentType)
		assert.Error(t, err, contentType)
	}
}

func TestValidateStorageEndpoint(t *testing.T) {
	for _, endpoint := range []string{"https://s3.eu-west-1.amazonaws.com", "http://localhost:9000", "https://acc.r2.cloudflarestorage.com/"} {
		assert.NoError(t, ValidateStorageEndpoint(endpoint), endpoint)
	}
	for _, endpoint := range []string{
		"", "s3.amazonaws.com", "ftp://files.example.com", "file:///etc", "https://",
		"https://user:pass@s3.example.com", "https://s3.example.com?x=1", "https://s3.example.com#frag",
	} {
		assert.Error(t, ValidateStorageEndpoint(endpoint), endpoint)
	}
}

func TestSameStorageAccount(t *testing.T) {
	stored := FileManagerSettings{Endpoint: "https://s3.example.com", AccessKey: "AK"}

	assert.True(t, SameStorageAccount(FileManagerSettings{Endpoint: "https://S3.example.com/", AccessKey: "AK"}, stored),
		"case and a trailing slash do not change the host")
	assert.True(t, SameStorageAccount(FileManagerSettings{Endpoint: "https://s3.example.com/other-path", AccessKey: "AK", Bucket: "other"}, stored),
		"another bucket on the same host and key is the same account")
	assert.False(t, SameStorageAccount(FileManagerSettings{Endpoint: "https://evil.example.com", AccessKey: "AK"}, stored))
	assert.False(t, SameStorageAccount(FileManagerSettings{Endpoint: "http://s3.example.com", AccessKey: "AK"}, stored),
		"a downgrade to http is another destination")
	assert.False(t, SameStorageAccount(FileManagerSettings{Endpoint: "https://s3.example.com", AccessKey: "OTHER"}, stored))
}

func TestUserWorkspaceFileRights(t *testing.T) {
	member := func(perms UserPermissions) *UserWorkspace {
		return &UserWorkspace{Role: "member", Permissions: perms}
	}
	tests := []struct {
		name          string
		uw            *UserWorkspace
		read, canEdit bool
	}{
		{"owner", &UserWorkspace{Role: "owner"}, true, true},
		{"no rights", member(UserPermissions{}), false, false},
		{"contacts only", member(UserPermissions{PermissionResourceContacts: {Read: true, Write: true}}), false, false},
		{"templates read", member(UserPermissions{PermissionResourceTemplates: {Read: true}}), true, false},
		{"blog read", member(UserPermissions{PermissionResourceBlog: {Read: true}}), true, false},
		{"templates write only", member(UserPermissions{PermissionResourceTemplates: {Write: true}}), true, true},
		{"blog write only", member(UserPermissions{PermissionResourceBlog: {Write: true}}), true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.read, tt.uw.CanReadFiles(), "read")
			assert.Equal(t, tt.canEdit, tt.uw.CanWriteFiles(), "write")
		})
	}
}
