package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestToken(t *testing.T, jwtSecret []byte, userID string) string {
	claims := &service.UserClaims{
		UserID:    userID,
		Type:      string(domain.UserTypeUser),
		SessionID: "test-session",
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{"test"},
			Issuer:    "test",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	require.NoError(t, err)
	return signed
}

func TestWorkspaceHandler_Create(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful workspace creation
	expectedWorkspace := &domain.Workspace{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	workspaceSvc.EXPECT().
		CreateWorkspace(gomock.Any(), "testworkspace1", "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", gomock.Any()).
		DoAndReturn(func(ctx context.Context, id, name, websiteURL, logoURL, coverURL, timezone string, fileManager domain.FileManagerSettings) (*domain.Workspace, error) {
			// Verify file manager settings
			assert.Equal(t, "https://s3.amazonaws.com", fileManager.Endpoint)
			assert.Equal(t, "my-bucket", fileManager.Bucket)
			assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", fileManager.AccessKey)
			return expectedWorkspace, nil
		})

	// Create request
	reqBody := domain.CreateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusCreated, w.Code)

	var response domain.Workspace
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, expectedWorkspace.ID, response.ID)
	assert.Equal(t, expectedWorkspace.Name, response.Name)
	assert.Equal(t, expectedWorkspace.Settings, response.Settings)
}

func TestWorkspaceHandler_Get(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful workspace retrieval
	expectedWorkspace := &domain.Workspace{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	workspaceSvc.EXPECT().
		GetWorkspace(gomock.Any(), "testworkspace1").
		Return(expectedWorkspace, nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.get?id=testworkspace1", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Workspace domain.Workspace `json:"workspace"`
	}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, expectedWorkspace.ID, response.Workspace.ID)
	assert.Equal(t, expectedWorkspace.Name, response.Workspace.Name)
	assert.Equal(t, expectedWorkspace.Settings, response.Workspace.Settings)
}

func TestWorkspaceHandler_List(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful workspace list retrieval
	expectedWorkspaces := []*domain.Workspace{
		{
			ID:   "testworkspace1",
			Name: "Test Workspace 1",
			Settings: domain.WorkspaceSettings{
				WebsiteURL: "https://example1.com",
				LogoURL:    "https://example1.com/logo.png",
				Timezone:   "UTC",
				FileManager: domain.FileManagerSettings{
					Endpoint:  "https://s3.amazonaws.com",
					Bucket:    "my-bucket",
					AccessKey: "AKIAIOSFODNN7EXAMPLE",
				},
			},
		},
		{
			ID:   "testworkspace2",
			Name: "Test Workspace 2",
			Settings: domain.WorkspaceSettings{
				WebsiteURL: "https://example2.com",
				LogoURL:    "https://example2.com/logo.png",
				Timezone:   "UTC",
				FileManager: domain.FileManagerSettings{
					Endpoint:  "https://s3.amazonaws.com",
					Bucket:    "my-bucket",
					AccessKey: "AKIAIOSFODNN7EXAMPLE",
				},
			},
		},
	}
	workspaceSvc.EXPECT().
		ListWorkspaces(gomock.Any()).
		Return(expectedWorkspaces, nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.list", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response []*domain.Workspace
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, expectedWorkspaces, response)
}

func TestWorkspaceHandler_Update(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful workspace update
	expectedWorkspace := &domain.Workspace{
		ID:   "testworkspace1",
		Name: "Updated Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://updated.com",
			LogoURL:    "https://updated.com/logo.png",
			CoverURL:   "https://updated.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	workspaceSvc.EXPECT().
		UpdateWorkspace(gomock.Any(), "testworkspace1", "Updated Workspace", gomock.Any()).
		DoAndReturn(func(ctx context.Context, id, name string, settings domain.WorkspaceSettings) (*domain.Workspace, error) {
			// Verify settings
			assert.Equal(t, "https://updated.com", settings.WebsiteURL)
			assert.Equal(t, "https://updated.com/logo.png", settings.LogoURL)
			assert.Equal(t, "https://updated.com/cover.png", settings.CoverURL)
			assert.Equal(t, "UTC", settings.Timezone)

			// Verify file manager settings
			assert.Equal(t, "https://s3.amazonaws.com", settings.FileManager.Endpoint)
			assert.Equal(t, "my-bucket", settings.FileManager.Bucket)
			assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", settings.FileManager.AccessKey)
			return expectedWorkspace, nil
		})

	// Create request
	reqBody := domain.UpdateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Updated Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://updated.com",
			LogoURL:    "https://updated.com/logo.png",
			CoverURL:   "https://updated.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.update", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response domain.Workspace
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, expectedWorkspace.ID, response.ID)
	assert.Equal(t, expectedWorkspace.Name, response.Name)
	assert.Equal(t, expectedWorkspace.Settings, response.Settings)
}

func TestWorkspaceHandler_Delete(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful workspace deletion
	workspaceSvc.EXPECT().
		DeleteWorkspace(gomock.Any(), "testid123").
		Return(nil)

	// Create request
	reqBody := map[string]string{
		"id": "testid123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.delete", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWorkspaceHandler_List_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Try with POST instead of GET
	reqBody := bytes.NewBuffer([]byte("{}"))
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.list", reqBody)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleList(w, req)

	// Verify response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWorkspaceHandler_List_ServiceError(t *testing.T) {
	handler, workspaceService, _, _, _ := setupTest(t)

	// Mock error from service
	workspaceService.EXPECT().
		ListWorkspaces(gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.list", nil)

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleList(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to list workspaces", response["error"])
}

func TestWorkspaceHandler_Get_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Try with POST instead of GET
	reqBody := bytes.NewBuffer([]byte(`{"id": "workspace123"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.get", reqBody)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleGet(w, req)

	// Verify response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWorkspaceHandler_Get_MissingID(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request without ID
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.get", nil)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleGet(w, req)

	// Verify response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error message
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Missing workspace ID", response["error"])
}

func TestWorkspaceHandler_Get_ServiceError(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	// Mock error from service
	workspaceService.EXPECT().
		GetWorkspace(gomock.Any(), "workspace123").
		Return(nil, fmt.Errorf("database error"))

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.get?id=workspace123", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleGet(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to get workspace", response["error"])
}

func TestWorkspaceHandler_Create_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Try with GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.create", nil)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleCreate(w, req)

	// Verify response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWorkspaceHandler_Create_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create invalid JSON request
	reqBody := bytes.NewBuffer([]byte(`{invalid json`))
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", reqBody)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleCreate(w, req)

	// Verify response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error message
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_Create_MissingID(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing ID
	reqBody := domain.CreateWorkspaceRequest{
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid create workspace request: id is required")
}

func TestWorkspaceHandler_Create_MissingName(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing name
	reqBody := domain.CreateWorkspaceRequest{
		ID: "testworkspace1",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid create workspace request: name is required")
}

func TestWorkspaceHandler_Create_MissingTimezone(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing timezone
	reqBody := domain.CreateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid create workspace request: timezone is required")
}

func TestWorkspaceHandler_Create_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		CreateWorkspace(gomock.Any(), "testworkspace1", "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Create request with valid data
	reqBody := domain.CreateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://example.com",
			LogoURL:    "https://example.com/logo.png",
			CoverURL:   "https://example.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to create workspace")
}

func TestWorkspaceHandler_Update_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Try with GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.update", nil)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleUpdate(w, req)

	// Verify response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWorkspaceHandler_Update_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create invalid JSON request
	reqBody := bytes.NewBuffer([]byte(`{invalid json`))
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.update", reqBody)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleUpdate(w, req)

	// Verify response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error message
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_Update_MissingID(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing ID
	reqBody := domain.UpdateWorkspaceRequest{
		Name: "Updated Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://updated.com",
			LogoURL:    "https://updated.com/logo.png",
			CoverURL:   "https://updated.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.update", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid update workspace request: id is required")
}

func TestWorkspaceHandler_Update_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		UpdateWorkspace(gomock.Any(), "testworkspace1", "Updated Workspace", gomock.Any()).
		Return(nil, fmt.Errorf("service error"))

	// Create request
	reqBody := domain.UpdateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Updated Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://updated.com",
			LogoURL:    "https://updated.com/logo.png",
			CoverURL:   "https://updated.com/cover.png",
			Timezone:   "UTC",
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.update", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to update workspace", response["error"])
}

func TestWorkspaceHandler_Delete_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Try with GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.delete", nil)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleDelete(w, req)

	// Verify response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWorkspaceHandler_Delete_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create invalid JSON request
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.delete", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "user123"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleDelete(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_Delete_MissingID(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with missing ID
	reqBody := bytes.NewBuffer([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.delete", reqBody)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleDelete(w, req)

	// Verify response - the handler validates the request body and returns a bad request error
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error message
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "invalid delete workspace request: id is required", response["error"])
}

func TestWorkspaceHandler_Delete_ServiceError(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	// Create valid request
	reqBody := bytes.NewBuffer([]byte(`{"id": "workspace123"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.delete", reqBody)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Mock workspace service to return error
	workspaceService.EXPECT().
		DeleteWorkspace(gomock.Any(), "workspace123").
		Return(fmt.Errorf("database error"))

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleDelete(w, req)

	// Verify response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify error message
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to delete workspace", response["error"])
}

func TestWorkspaceHandler_HandleMembers(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful members retrieval
	expectedMembers := []*domain.UserWorkspaceWithEmail{
		{
			UserWorkspace: domain.UserWorkspace{
				UserID:      "user1",
				WorkspaceID: "workspace1",
				Role:        "owner",
			},
			Email: "user1@example.com",
		},
	}
	workspaceSvc.EXPECT().
		GetWorkspaceMembersWithEmail(gomock.Any(), "workspace1").
		Return(expectedMembers, nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.members?id=workspace1", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response, "members")
}

func TestWorkspaceHandler_HandleMembers_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Try with POST instead of GET
	reqBody := bytes.NewBuffer([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.members", reqBody)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleMembers(w, req)

	// Verify response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWorkspaceHandler_HandleMembers_MissingID(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request without ID
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.members", nil)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleMembers(w, req)

	// Verify response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error message
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Missing workspace ID", response["error"])
}

func TestWorkspaceHandler_HandleMembers_ServiceError(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.members?id=workspace123", nil)
	w := httptest.NewRecorder()

	// Add auth token
	token := createTestToken(t, secretKey, "user123")
	req.Header.Set("Authorization", "Bearer "+token)

	// Mock workspace service to return error
	workspaceService.EXPECT().
		GetWorkspaceMembersWithEmail(gomock.Any(), "workspace123").
		Return(nil, fmt.Errorf("database error"))

	// Setup context with authenticated user
	ctx := req.Context()
	ctx = context.WithValue(ctx, domain.UserIDKey, "user123")
	req = req.WithContext(ctx)

	// Call handler directly
	handler.handleMembers(w, req)

	// Verify response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify error message
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to get workspace members", response["error"])
}

func TestWorkspaceHandler_HandleInviteMember(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful member invitation
	mockInvitation := &domain.WorkspaceInvitation{
		ID:          "inv-123",
		WorkspaceID: "testworkspace123",
		Email:       "test@example.com",
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}
	mockToken := "invitation-token-123"

	workspaceSvc.EXPECT().
		InviteMember(gomock.Any(), "testworkspace123", "test@example.com", gomock.Any()).
		Return(mockInvitation, mockToken, nil)

	// Create request
	reqBody := domain.InviteMemberRequest{
		WorkspaceID: "testworkspace123",
		Email:       "test@example.com",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.inviteMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Invitation sent", response["message"])
	assert.Equal(t, mockToken, response["token"])

	// Check invitation details
	invitationMap, ok := response["invitation"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, mockInvitation.ID, invitationMap["id"])
	assert.Equal(t, mockInvitation.WorkspaceID, invitationMap["workspace_id"])
	assert.Equal(t, mockInvitation.Email, invitationMap["email"])
}

func TestWorkspaceHandler_HandleInviteMember_DirectAdd(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock case where user already exists (direct add)
	workspaceSvc.EXPECT().
		InviteMember(gomock.Any(), "testworkspace123", "existing@example.com", gomock.Any()).
		Return(nil, "", nil) // nil invitation means user was directly added

	// Create request
	reqBody := domain.InviteMemberRequest{
		WorkspaceID: "testworkspace123",
		Email:       "existing@example.com",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.inviteMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "User added to workspace", response["message"])
}

func TestWorkspaceHandler_HandleInviteMember_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create GET request (method not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.inviteMember", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler to test method check
	w := httptest.NewRecorder()
	handler.handleInviteMember(w, req)

	// Assert response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestWorkspaceHandler_HandleInviteMember_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.inviteMember", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleInviteMember(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_HandleInviteMember_ValidationError(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing required fields
	reqBody := domain.InviteMemberRequest{
		// Missing WorkspaceID
		Email: "test@example.com",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.inviteMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "invalid invite member request: workspace_id is required")
}

func TestWorkspaceHandler_HandleInviteMember_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		InviteMember(gomock.Any(), "testworkspace123", "test@example.com", gomock.Any()).
		Return(nil, "", fmt.Errorf("service error"))

	// Create request
	reqBody := domain.InviteMemberRequest{
		WorkspaceID: "testworkspace123",
		Email:       "test@example.com",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.inviteMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to invite member", response["error"])
}

func TestWorkspaceHandler_HandleCreateAPIKey(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful API key creation
	mockToken := "api-key-token-123"
	mockEmail := "api-123@example.com"

	workspaceSvc.EXPECT().
		CreateAPIKey(gomock.Any(), "workspace-123", "api").
		Return(mockToken, mockEmail, nil)

	// Create request
	reqBody := domain.CreateAPIKeyRequest{
		WorkspaceID: "workspace-123",
		EmailPrefix: "api",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, mockToken, response["token"])
	assert.Equal(t, mockEmail, response["email"])
}

func TestWorkspaceHandler_HandleCreateAPIKey_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create GET request (method not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.createAPIKey", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleCreateAPIKey(w, req)

	// Assert response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestWorkspaceHandler_HandleCreateAPIKey_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleCreateAPIKey(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_HandleCreateAPIKey_ValidationError(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing required fields
	reqBody := domain.CreateAPIKeyRequest{
		// Missing WorkspaceID
		EmailPrefix: "api",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "workspace ID is required")
}

func TestWorkspaceHandler_HandleCreateAPIKey_UnauthorizedError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock unauthorized error
	unauthorizedErr := &domain.ErrUnauthorized{Message: "Unauthorized to create API key"}
	workspaceSvc.EXPECT().
		CreateAPIKey(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", "", unauthorizedErr)

	// Create request
	reqBody := domain.CreateAPIKeyRequest{
		WorkspaceID: "workspace-123",
		EmailPrefix: "api",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Only workspace owners can create API keys", response["error"])
}

func TestWorkspaceHandler_HandleCreateAPIKey_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		CreateAPIKey(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", "", fmt.Errorf("service error"))

	// Create request
	reqBody := domain.CreateAPIKeyRequest{
		WorkspaceID: "workspace-123",
		EmailPrefix: "api",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "service error", response["error"])
}

func TestWorkspaceHandler_HandleRemoveMember(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful member removal
	workspaceSvc.EXPECT().
		RemoveMember(gomock.Any(), "workspace-123", "user-123").
		Return(nil)

	// Create request
	reqBody := struct {
		WorkspaceID string `json:"workspace_id"`
		UserID      string `json:"user_id"`
	}{
		WorkspaceID: "workspace-123",
		UserID:      "user-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.removeMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Member removed successfully", response["message"])
}

func TestWorkspaceHandler_HandleRemoveMember_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create GET request (method not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.removeMember", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler to test method check
	w := httptest.NewRecorder()
	handler.handleRemoveMember(w, req)

	// Assert response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestWorkspaceHandler_HandleRemoveMember_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.removeMember", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleRemoveMember(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_HandleRemoveMember_MissingWorkspaceID(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with missing workspace ID
	reqBody := struct {
		UserID string `json:"user_id"`
	}{
		UserID: "user-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.removeMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleRemoveMember(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Missing workspace_id", response["error"])
}

func TestWorkspaceHandler_HandleRemoveMember_MissingUserID(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with missing user ID
	reqBody := struct {
		WorkspaceID string `json:"workspace_id"`
	}{
		WorkspaceID: "workspace-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.removeMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleRemoveMember(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Missing user_id", response["error"])
}

func TestWorkspaceHandler_HandleRemoveMember_UnauthorizedError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock unauthorized error
	unauthorizedErr := &domain.ErrUnauthorized{Message: "Only workspace owners can remove members"}
	workspaceSvc.EXPECT().
		RemoveMember(gomock.Any(), "workspace-123", "user-123").
		Return(unauthorizedErr)

	// Create request
	reqBody := struct {
		WorkspaceID string `json:"workspace_id"`
		UserID      string `json:"user_id"`
	}{
		WorkspaceID: "workspace-123",
		UserID:      "user-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.removeMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Only workspace owners can remove members", response["error"])
}

func TestWorkspaceHandler_HandleRemoveMember_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		RemoveMember(gomock.Any(), "workspace-123", "user-123").
		Return(fmt.Errorf("service error"))

	// Create request
	reqBody := struct {
		WorkspaceID string `json:"workspace_id"`
		UserID      string `json:"user_id"`
	}{
		WorkspaceID: "workspace-123",
		UserID:      "user-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.removeMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to remove member from workspace", response["error"])
}

func TestWorkspaceHandler_HandleCreateIntegration(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	integrationID := "integration-123"

	// Mock successful integration creation
	workspaceSvc.EXPECT().
		CreateIntegration(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.CreateIntegrationRequest) (string, error) {
			// Verify request fields
			assert.Equal(t, "workspace-123", req.WorkspaceID)
			assert.Equal(t, "Test Integration", req.Name)
			assert.Equal(t, domain.IntegrationTypeEmail, req.Type)
			assert.Equal(t, domain.EmailProviderKindSES, req.Provider.Kind)
			return integrationID, nil
		})

	// Create request
	reqBody := domain.CreateIntegrationRequest{
		WorkspaceID: "workspace-123",
		Name:        "Test Integration",
		Type:        domain.IntegrationTypeEmail,
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "AKIAEXAMPLE",
				SecretKey: "secret-key-example",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, integrationID, response["integration_id"])
}

func TestWorkspaceHandler_HandleCreateIntegration_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create GET request (method not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.createIntegration", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleCreateIntegration(w, req)

	// Assert response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestWorkspaceHandler_HandleCreateIntegration_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createIntegration", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleCreateIntegration(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_HandleCreateIntegration_ValidationError(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing required fields
	reqBody := domain.CreateIntegrationRequest{
		// Missing WorkspaceID
		Name:     "Test Integration",
		Type:     "email",
		Provider: domain.EmailProvider{Kind: domain.EmailProviderKindSES},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "workspace ID is required")
}

func TestWorkspaceHandler_HandleCreateIntegration_UnauthorizedError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock unauthorized error
	unauthorizedErr := &domain.ErrUnauthorized{Message: "Unauthorized to create integration"}
	workspaceSvc.EXPECT().
		CreateIntegration(gomock.Any(), gomock.Any()).
		Return("", unauthorizedErr)

	// Create request with valid provider data
	reqBody := domain.CreateIntegrationRequest{
		WorkspaceID: "workspace-123",
		Name:        "Test Integration",
		Type:        domain.IntegrationTypeEmail,
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "AKIAEXAMPLE",
				SecretKey: "secret-key-example",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Unauthorized to create integration", response["error"])
}

func TestWorkspaceHandler_HandleCreateIntegration_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		CreateIntegration(gomock.Any(), gomock.Any()).
		Return("", fmt.Errorf("service error"))

	// Create request with valid provider data
	reqBody := domain.CreateIntegrationRequest{
		WorkspaceID: "workspace-123",
		Name:        "Test Integration",
		Type:        domain.IntegrationTypeEmail,
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindSES,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "AKIAEXAMPLE",
				SecretKey: "secret-key-example",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to create integration", response["error"])
}

func TestWorkspaceHandler_HandleUpdateIntegration(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful integration update
	workspaceSvc.EXPECT().
		UpdateIntegration(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.UpdateIntegrationRequest) error {
			// Verify request fields
			assert.Equal(t, "workspace-123", req.WorkspaceID)
			assert.Equal(t, "integration-123", req.IntegrationID)
			assert.Equal(t, "Updated Integration", req.Name)
			assert.Equal(t, domain.EmailProviderKindMailgun, req.Provider.Kind)
			return nil
		})

	// Create request
	reqBody := domain.UpdateIntegrationRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		Name:          "Updated Integration",
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailgun,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			Mailgun: &domain.MailgunSettings{
				Domain: "test.com",
				APIKey: "api-key-example",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.updateIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Integration updated successfully", response["message"])
}

func TestWorkspaceHandler_HandleUpdateIntegration_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create GET request (method not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.updateIntegration", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleUpdateIntegration(w, req)

	// Assert response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestWorkspaceHandler_HandleUpdateIntegration_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.updateIntegration", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleUpdateIntegration(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_HandleUpdateIntegration_ValidationError(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing required fields
	reqBody := domain.UpdateIntegrationRequest{
		// Missing WorkspaceID
		IntegrationID: "integration-123",
		Name:          "Updated Integration",
		Provider: domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.updateIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "workspace ID is required")
}

func TestWorkspaceHandler_HandleUpdateIntegration_UnauthorizedError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock unauthorized error
	unauthorizedErr := &domain.ErrUnauthorized{Message: "Unauthorized to update integration"}
	workspaceSvc.EXPECT().
		UpdateIntegration(gomock.Any(), gomock.Any()).
		Return(unauthorizedErr)

	// Create request
	reqBody := domain.UpdateIntegrationRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		Name:          "Updated Integration",
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailgun,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			Mailgun: &domain.MailgunSettings{
				Domain: "test.com",
				APIKey: "api-key-example",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.updateIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Unauthorized to update integration", response["error"])
}

func TestWorkspaceHandler_HandleUpdateIntegration_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		UpdateIntegration(gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("service error"))

	// Create request
	reqBody := domain.UpdateIntegrationRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		Name:          "Updated Integration",
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailgun,
			RateLimitPerMinute: 25,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			Mailgun: &domain.MailgunSettings{
				Domain: "test.com",
				APIKey: "api-key-example",
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.updateIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to update integration", response["error"])
}

func TestWorkspaceHandler_HandleDeleteIntegration(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful integration deletion
	workspaceSvc.EXPECT().
		DeleteIntegration(gomock.Any(), "workspace-123", "integration-123").
		Return(nil)

	// Create request
	reqBody := domain.DeleteIntegrationRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Integration deleted successfully", response["message"])
}

func TestWorkspaceHandler_HandleDeleteIntegration_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create GET request (method not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.deleteIntegration", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleDeleteIntegration(w, req)

	// Assert response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestWorkspaceHandler_HandleDeleteIntegration_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// Create request with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteIntegration", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request directly against handler
	w := httptest.NewRecorder()
	handler.handleDeleteIntegration(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_HandleDeleteIntegration_ValidationError(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// Create request with missing required fields
	reqBody := domain.DeleteIntegrationRequest{
		// Missing WorkspaceID
		IntegrationID: "integration-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "workspace ID is required")
}

func TestWorkspaceHandler_HandleDeleteIntegration_UnauthorizedError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock unauthorized error
	unauthorizedErr := &domain.ErrUnauthorized{Message: "Unauthorized to delete integration"}
	workspaceSvc.EXPECT().
		DeleteIntegration(gomock.Any(), "workspace-123", "integration-123").
		Return(unauthorizedErr)

	// Create request
	reqBody := domain.DeleteIntegrationRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Unauthorized to delete integration", response["error"])
}

func TestWorkspaceHandler_HandleDeleteIntegration_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service error
	workspaceSvc.EXPECT().
		DeleteIntegration(gomock.Any(), "workspace-123", "integration-123").
		Return(fmt.Errorf("service error"))

	// Create request
	reqBody := domain.DeleteIntegrationRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteIntegration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	// Execute request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to delete integration", response["error"])
}

func TestWriteJSON(t *testing.T) {
	// Create a response recorder
	w := httptest.NewRecorder()

	// Call the function with a test struct
	testData := map[string]string{"key": "value"}
	writeJSON(w, http.StatusOK, testData)

	// Check status code
	assert.Equal(t, http.StatusOK, w.Code)

	// Check content type
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Parse the response body
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Check data
	assert.Equal(t, "value", response["key"])
}

func TestGetBytesFromBody(t *testing.T) {
	// Prepare a body
	content := "hello world"
	rc := io.NopCloser(strings.NewReader(content))
	// Call helper
	got := getBytesFromBody(rc)
	assert.Equal(t, []byte(content), got)
}

func TestWorkspaceHandler_HandleVerifyInvitationToken(t *testing.T) {
	handler, workspaceSvc, _, _, authSvc := setupTest(t)

	invitationID := "invitation-123"
	workspaceID := "workspace-123"
	email := "test@example.com"

	invitation := &domain.WorkspaceInvitation{
		ID:          invitationID,
		WorkspaceID: workspaceID,
		InviterID:   "inviter-123",
		Email:       email,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	workspace := &domain.Workspace{
		ID:   workspaceID,
		Name: "Test Workspace",
	}

	t.Run("successful verification", func(t *testing.T) {
		// Mock token validation
		authSvc.EXPECT().
			ValidateInvitationToken("valid-token").
			Return(invitationID, workspaceID, email, nil)

		// Mock invitation retrieval
		workspaceSvc.EXPECT().
			GetInvitationByID(gomock.Any(), invitationID).
			Return(invitation, nil)

		// Mock workspace retrieval
		workspaceSvc.EXPECT().
			GetWorkspace(gomock.Any(), workspaceID).
			Return(workspace, nil)

		// Create request
		reqBody := VerifyInvitationTokenRequest{
			Token: "valid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.verifyInvitationToken", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleVerifyInvitationToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Equal(t, true, response["valid"])
		assert.NotNil(t, response["invitation"])
		assert.NotNil(t, response["workspace"])
	})

	t.Run("invalid token", func(t *testing.T) {
		// Mock token validation failure
		authSvc.EXPECT().
			ValidateInvitationToken("invalid-token").
			Return("", "", "", errors.New("invalid token"))

		// Create request
		reqBody := VerifyInvitationTokenRequest{
			Token: "invalid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.verifyInvitationToken", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleVerifyInvitationToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid or expired invitation token", response["error"])
	})

	t.Run("invitation not found", func(t *testing.T) {
		// Mock token validation
		authSvc.EXPECT().
			ValidateInvitationToken("valid-token").
			Return(invitationID, workspaceID, email, nil)

		// Mock invitation retrieval failure
		workspaceSvc.EXPECT().
			GetInvitationByID(gomock.Any(), invitationID).
			Return(nil, errors.New("invitation not found"))

		// Create request
		reqBody := VerifyInvitationTokenRequest{
			Token: "valid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.verifyInvitationToken", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleVerifyInvitationToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invitation not found", response["error"])
	})

	t.Run("invitation details mismatch", func(t *testing.T) {
		mismatchInvitation := &domain.WorkspaceInvitation{
			ID:          invitationID,
			WorkspaceID: "different-workspace",
			InviterID:   "inviter-123",
			Email:       "different@example.com",
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Mock token validation
		authSvc.EXPECT().
			ValidateInvitationToken("valid-token").
			Return(invitationID, workspaceID, email, nil)

		// Mock invitation retrieval with mismatched data
		workspaceSvc.EXPECT().
			GetInvitationByID(gomock.Any(), invitationID).
			Return(mismatchInvitation, nil)

		// Create request
		reqBody := VerifyInvitationTokenRequest{
			Token: "valid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.verifyInvitationToken", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleVerifyInvitationToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid invitation token", response["error"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.verifyInvitationToken", nil)

		w := httptest.NewRecorder()
		handler.handleVerifyInvitationToken(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Method not allowed", response["error"])
	})

	t.Run("invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.verifyInvitationToken", strings.NewReader("invalid json"))

		w := httptest.NewRecorder()
		handler.handleVerifyInvitationToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid request body", response["error"])
	})

	t.Run("missing token", func(t *testing.T) {
		reqBody := VerifyInvitationTokenRequest{
			Token: "",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.verifyInvitationToken", bytes.NewReader(body))

		w := httptest.NewRecorder()
		handler.handleVerifyInvitationToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Token is required", response["error"])
	})
}

func TestWorkspaceHandler_HandleAcceptInvitation(t *testing.T) {
	handler, workspaceSvc, _, _, authSvc := setupTest(t)

	invitationID := "invitation-123"
	workspaceID := "workspace-123"
	email := "test@example.com"

	invitation := &domain.WorkspaceInvitation{
		ID:          invitationID,
		WorkspaceID: workspaceID,
		InviterID:   "inviter-123",
		Email:       email,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	t.Run("successful acceptance", func(t *testing.T) {
		// Mock token validation
		authSvc.EXPECT().
			ValidateInvitationToken("valid-token").
			Return(invitationID, workspaceID, email, nil)

		// Mock invitation retrieval
		workspaceSvc.EXPECT().
			GetInvitationByID(gomock.Any(), invitationID).
			Return(invitation, nil)

		// Mock invitation acceptance
		authResponse := &domain.AuthResponse{
			Token: "auth-token-123",
			User: domain.User{
				ID:    "user-123",
				Email: email,
				Type:  domain.UserTypeUser,
			},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		workspaceSvc.EXPECT().
			AcceptInvitation(gomock.Any(), invitationID, workspaceID, email).
			Return(authResponse, nil)

		// Create request
		reqBody := AcceptInvitationRequest{
			Token: "valid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.acceptInvitation", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleAcceptInvitation(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Equal(t, "Invitation accepted successfully", response["message"])
		assert.Equal(t, workspaceID, response["workspace_id"])
		assert.Equal(t, email, response["email"])
		assert.Equal(t, "auth-token-123", response["token"])
		assert.NotNil(t, response["user"])
		assert.NotNil(t, response["expires_at"])
	})

	t.Run("invalid token", func(t *testing.T) {
		// Mock token validation failure
		authSvc.EXPECT().
			ValidateInvitationToken("invalid-token").
			Return("", "", "", errors.New("invalid token"))

		// Create request
		reqBody := AcceptInvitationRequest{
			Token: "invalid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.acceptInvitation", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleAcceptInvitation(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid or expired invitation token", response["error"])
	})

	t.Run("invitation not found", func(t *testing.T) {
		// Mock token validation
		authSvc.EXPECT().
			ValidateInvitationToken("valid-token").
			Return(invitationID, workspaceID, email, nil)

		// Mock invitation acceptance failure (invitation not found handled in service)
		workspaceSvc.EXPECT().
			AcceptInvitation(gomock.Any(), invitationID, workspaceID, email).
			Return(nil, errors.New("invitation not found"))

		// Create request
		reqBody := AcceptInvitationRequest{
			Token: "valid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.acceptInvitation", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleAcceptInvitation(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to accept invitation", response["error"])
	})

	t.Run("invitation acceptance failure", func(t *testing.T) {
		// Mock token validation
		authSvc.EXPECT().
			ValidateInvitationToken("valid-token").
			Return(invitationID, workspaceID, email, nil)

		// Mock invitation acceptance failure
		workspaceSvc.EXPECT().
			AcceptInvitation(gomock.Any(), invitationID, workspaceID, email).
			Return(nil, errors.New("acceptance failed"))

		// Create request
		reqBody := AcceptInvitationRequest{
			Token: "valid-token",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.acceptInvitation", bytes.NewReader(body))

		// Execute request
		w := httptest.NewRecorder()
		handler.handleAcceptInvitation(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to accept invitation", response["error"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.acceptInvitation", nil)

		w := httptest.NewRecorder()
		handler.handleAcceptInvitation(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Method not allowed", response["error"])
	})

	t.Run("invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.acceptInvitation", strings.NewReader("invalid json"))

		w := httptest.NewRecorder()
		handler.handleAcceptInvitation(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid request body", response["error"])
	})

	t.Run("missing token", func(t *testing.T) {
		reqBody := AcceptInvitationRequest{
			Token: "",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.acceptInvitation", bytes.NewReader(body))

		w := httptest.NewRecorder()
		handler.handleAcceptInvitation(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Token is required", response["error"])
	})
}

func TestWorkspaceHandler_DeleteInvitation(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	t.Run("successful deletion", func(t *testing.T) {
		invitationID := "inv-123"

		// Create test token
		token := createTestToken(t, secretKey, "user1")

		reqBody := map[string]string{
			"invitation_id": invitationID,
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteInvitation", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		// Mock the workspace service
		workspaceSvc.EXPECT().DeleteInvitation(gomock.Any(), invitationID).Return(nil)

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "success", response["status"])
		assert.Equal(t, "Invitation deleted successfully", response["message"])
	})

	t.Run("missing invitation_id", func(t *testing.T) {
		token := createTestToken(t, secretKey, "user1")

		reqBody := map[string]string{}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteInvitation", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "invitation_id is required", response["error"])
	})

	t.Run("invalid JSON", func(t *testing.T) {
		token := createTestToken(t, secretKey, "user1")

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteInvitation", strings.NewReader("invalid json"))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid request body", response["error"])
	})

	t.Run("service error", func(t *testing.T) {
		invitationID := "inv-123"
		token := createTestToken(t, secretKey, "user1")

		reqBody := map[string]string{
			"invitation_id": invitationID,
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteInvitation", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		workspaceSvc.EXPECT().DeleteInvitation(gomock.Any(), invitationID).Return(fmt.Errorf("service error"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to delete invitation", response["error"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		token := createTestToken(t, secretKey, "user1")

		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.deleteInvitation", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Method not allowed", response["error"])
	})
}

func TestWorkspaceHandler_HandleSetUserPermissions(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	t.Run("successful permission update", func(t *testing.T) {
		// Mock successful permission update
		workspaceSvc.EXPECT().
			SetUserPermissions(gomock.Any(), "workspace-123", "user-123", gomock.Any()).
			Return(nil)

		// Create request
		reqBody := domain.SetUserPermissionsRequest{
			WorkspaceID: "workspace-123",
			UserID:      "user-123",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: domain.ResourcePermissions{
					Read:  true,
					Write: false,
				},
				domain.PermissionResourceLists: domain.ResourcePermissions{
					Read:  true,
					Write: true,
				},
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPermissions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		// Execute request
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Equal(t, "User permissions updated successfully", response["message"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.setUserPermissions", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Method not allowed", response["error"])
	})

	t.Run("invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPermissions", strings.NewReader("invalid json"))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid request body", response["error"])
	})

	t.Run("validation error", func(t *testing.T) {
		// Create request with missing required fields
		reqBody := domain.SetUserPermissionsRequest{
			// Missing WorkspaceID
			UserID: "user-123",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: domain.ResourcePermissions{
					Read:  true,
					Write: false,
				},
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPermissions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "Missing workspace_id")
	})

	t.Run("unauthorized error", func(t *testing.T) {
		// Mock unauthorized error
		unauthorizedErr := &domain.ErrUnauthorized{Message: "Only workspace owners can manage user permissions"}
		workspaceSvc.EXPECT().
			SetUserPermissions(gomock.Any(), "workspace-123", "user-123", gomock.Any()).
			Return(unauthorizedErr)

		// Create request
		reqBody := domain.SetUserPermissionsRequest{
			WorkspaceID: "workspace-123",
			UserID:      "user-123",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: domain.ResourcePermissions{
					Read:  true,
					Write: false,
				},
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPermissions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Only workspace owners can manage user permissions", response["error"])
	})

	t.Run("service error", func(t *testing.T) {
		// Mock service error
		workspaceSvc.EXPECT().
			SetUserPermissions(gomock.Any(), "workspace-123", "user-123", gomock.Any()).
			Return(fmt.Errorf("service error"))

		// Create request
		reqBody := domain.SetUserPermissionsRequest{
			WorkspaceID: "workspace-123",
			UserID:      "user-123",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: domain.ResourcePermissions{
					Read:  true,
					Write: false,
				},
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPermissions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to set user permissions", response["error"])
	})
}
