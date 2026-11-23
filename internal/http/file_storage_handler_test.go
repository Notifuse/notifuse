package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

var fileStorageJWTSecret = []byte("test-jwt-secret-key-for-testing-32bytes")

func setupFileStorageHandlerTest(t *testing.T, isDemo bool) (*mocks.MockFileStorageService, *http.ServeMux) {
	ctrl := gomock.NewController(t)
	service := mocks.NewMockFileStorageService(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	mux := http.NewServeMux()
	NewFileStorageHandler(service, func() ([]byte, error) { return fileStorageJWTSecret, nil }, logger, isDemo).RegisterRoutes(mux)
	return service, mux
}

func fileStorageRequest(t *testing.T, mux *http.ServeMux, method, target string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		payload, err := json.Marshal(b)
		require.NoError(t, err)
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, fileStorageJWTSecret, "test-user"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestFileStorageHandler_List(t *testing.T) {
	service, mux := setupFileStorageHandlerTest(t, false)
	modified := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	service.EXPECT().List(gomock.Any(), "ws1", "images/").Return(&domain.StoredObjectList{
		Objects: []domain.StoredObject{{Key: "images/a.png", Size: 12, LastModified: modified}},
		Capped:  true,
	}, nil)

	rec := fileStorageRequest(t, mux, http.MethodGet, "/api/files.list?workspace_id=ws1&prefix=images/", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"objects":[{"key":"images/a.png","size":12,"last_modified":"2026-11-01T12:00:00Z"}],"capped":true,"cut_short":false}`, rec.Body.String())
}

func TestFileStorageHandler_PresignUpload(t *testing.T) {
	service, mux := setupFileStorageHandlerTest(t, false)
	expires := time.Date(2026, 11, 23, 10, 5, 0, 0, time.UTC)
	service.EXPECT().PresignUpload(gomock.Any(), domain.PresignUploadRequest{
		WorkspaceID: "ws1", Key: "a.png", ContentType: "image/png", Size: 42,
	}).Return(&domain.PresignedUpload{URL: "https://signed", Headers: map[string]string{"Content-Type": "image/png"}, ExpiresAt: expires}, nil)

	rec := fileStorageRequest(t, mux, http.MethodPost, "/api/files.presignUpload",
		`{"workspace_id":"ws1","key":"a.png","content_type":"image/png","size":42}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"url":"https://signed","headers":{"Content-Type":"image/png"},"expires_at":"2026-11-23T10:05:00Z"}`, rec.Body.String())
}

func TestFileStorageHandler_CreateFolderDeleteAndDeleteFolder(t *testing.T) {
	service, mux := setupFileStorageHandlerTest(t, false)
	service.EXPECT().CreateFolder(gomock.Any(), "ws1", "photos/").Return(&domain.CreateFolderResult{Status: domain.FolderCreated}, nil)
	service.EXPECT().Delete(gomock.Any(), "ws1", []string{"a.png", "b.png"}).Return(&domain.DeleteFilesResult{Deleted: 1, Failed: 1}, nil)
	service.EXPECT().DeleteFolder(gomock.Any(), "ws1", "photos/").Return(&domain.DeleteFolderResult{Deleted: 3, DeletedFiles: 2}, nil)

	rec := fileStorageRequest(t, mux, http.MethodPost, "/api/files.createFolder", `{"workspace_id":"ws1","key":"photos/"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"created"}`, rec.Body.String())

	rec = fileStorageRequest(t, mux, http.MethodPost, "/api/files.delete", `{"workspace_id":"ws1","keys":["a.png","b.png"]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"deleted":1,"failed":1}`, rec.Body.String())

	rec = fileStorageRequest(t, mux, http.MethodPost, "/api/files.deleteFolder", `{"workspace_id":"ws1","prefix":"photos/"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"deleted":3,"failed":0,"deleted_files":2,"failed_files":0}`, rec.Body.String())
}

func TestFileStorageHandler_TestSettings(t *testing.T) {
	service, mux := setupFileStorageHandlerTest(t, false)
	service.EXPECT().TestSettings(gomock.Any(), "ws1", gomock.Any()).DoAndReturn(
		func(_ interface{}, _ string, settings domain.FileManagerSettings) error {
			assert.Equal(t, "https://s3.example.com", settings.Endpoint)
			assert.Equal(t, "typed", settings.SecretKey)
			return nil
		})

	rec := fileStorageRequest(t, mux, http.MethodPost, "/api/workspaces.testFileManager",
		`{"workspace_id":"ws1","settings":{"endpoint":"https://s3.example.com","bucket":"b","access_key":"AK","secret_key":"typed"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"success":true}`, rec.Body.String())
}

func TestFileStorageHandler_TestSettingsIsRefusedInDemo(t *testing.T) {
	_, mux := setupFileStorageHandlerTest(t, true)

	rec := fileStorageRequest(t, mux, http.MethodPost, "/api/workspaces.testFileManager", `{"workspace_id":"ws1","settings":{}}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "demo mode")
}

func TestFileStorageHandler_ErrorStatuses(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"not allowed", &domain.ErrUnauthorized{Message: "listing files requires access to templates or the blog"}, http.StatusForbidden},
		{"permission", &domain.PermissionError{Resource: domain.PermissionResourceTemplates, Permission: domain.PermissionTypeRead, Message: "no"}, http.StatusForbidden},
		{"invalid input", domain.NewValidationError("not a folder prefix"), http.StatusBadRequest},
		{"provider", &domain.FileStorageError{Message: "NoSuchBucket: gone"}, http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mux := setupFileStorageHandlerTest(t, false)
			service.EXPECT().List(gomock.Any(), "ws1", "").Return(nil, tt.err)

			rec := fileStorageRequest(t, mux, http.MethodGet, "/api/files.list?workspace_id=ws1", nil)
			assert.Equal(t, tt.status, rec.Code, rec.Body.String())
		})
	}
}

func TestFileStorageHandler_ProviderMessageReachesTheOwner(t *testing.T) {
	service, mux := setupFileStorageHandlerTest(t, false)
	service.EXPECT().List(gomock.Any(), "ws1", "").Return(nil, &domain.FileStorageError{Message: "NoSuchBucket: gone"})

	rec := fileStorageRequest(t, mux, http.MethodGet, "/api/files.list?workspace_id=ws1", nil)
	assert.Contains(t, rec.Body.String(), "NoSuchBucket: gone")
}

func TestFileStorageHandler_RequestShape(t *testing.T) {
	_, mux := setupFileStorageHandlerTest(t, false)

	cases := []struct {
		name, method, target string
		body                 interface{}
		status               int
	}{
		{"list by POST", http.MethodPost, "/api/files.list?workspace_id=ws1", nil, http.StatusMethodNotAllowed},
		{"presign by GET", http.MethodGet, "/api/files.presignUpload", nil, http.StatusMethodNotAllowed},
		{"delete by GET", http.MethodGet, "/api/files.delete", nil, http.StatusMethodNotAllowed},
		{"list without workspace", http.MethodGet, "/api/files.list", nil, http.StatusBadRequest},
		{"presign without workspace", http.MethodPost, "/api/files.presignUpload", `{"key":"a.png"}`, http.StatusBadRequest},
		{"not JSON", http.MethodPost, "/api/files.createFolder", `{not json`, http.StatusBadRequest},
		{"too large", http.MethodPost, "/api/files.delete", `{"workspace_id":"ws1","keys":["` + strings.Repeat("a", maxFileStorageRequestBytes) + `"]}`, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := fileStorageRequest(t, mux, tc.method, tc.target, tc.body)
			assert.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}
}

func TestFileStorageHandler_RequiresAuthentication(t *testing.T) {
	_, mux := setupFileStorageHandlerTest(t, false)
	for _, route := range []string{"/api/files.list?workspace_id=ws1", "/api/files.presignUpload", "/api/files.createFolder", "/api/files.delete", "/api/files.deleteFolder", "/api/workspaces.testFileManager"} {
		req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, route)
	}
}
