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
	"github.com/Notifuse/notifuse/internal/domain/mocks"
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

// createTestAPIKeyToken mints the token an integration holds: type api_key, and
// no session id, because a key has no session to expire.
//
// RequireAuth accepts it on every route a console session can reach — same
// middleware, same context — which is why what a response may contain has to be
// decided from the caller and not from the endpoint.
func createTestAPIKeyToken(t *testing.T, jwtSecret []byte, userID string) string {
	claims := &service.UserClaims{
		UserID: userID,
		Type:   string(domain.UserTypeAPIKey),
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

// demoRestrictedWorkspaceRoutes are the workspace routes closed on a demo
// instance: everything that mutates, since the demo is publicly writable and its
// membership, API-key and integration routes hand out durable credentials.
var demoRestrictedWorkspaceRoutes = []string{
	"/api/workspaces.create",
	"/api/workspaces.update",
	"/api/workspaces.delete",
	"/api/workspaces.inviteMember",
	"/api/workspaces.createAPIKey",
	"/api/workspaces.removeMember",
	"/api/workspaces.deleteInvitation",
	"/api/workspaces.setUserPermissions",
	"/api/workspaces.setCustomFieldLabels",
	"/api/workspaces.setBlogSettings",
	"/api/workspaces.setWebAnalyticsSettings",
	"/api/workspaces.createIntegration",
	"/api/workspaces.updateIntegration",
	"/api/workspaces.deleteIntegration",
	"/api/workspaces.connectZapier",
}

const demoRestrictedError = "This operation is not allowed in demo mode"

func TestWorkspaceHandler_DemoModeRestrictsMutatingRoutes(t *testing.T) {
	_, _, demoMux, secretKey, _ := setupDemoTest(t)
	_, _, defaultMux, _, _ := setupTest(t)

	for _, path := range demoRestrictedWorkspaceRoutes {
		t.Run(path, func(t *testing.T) {
			// Authenticated and well-formed: the refusal comes from demo mode, not
			// from auth or validation. The workspace service mock has no
			// expectations, so a route that slipped through would fail here.
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
			w := httptest.NewRecorder()
			demoMux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var response map[string]string
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			assert.Equal(t, demoRestrictedError, response["error"])

			// Off a demo instance the same route reaches the auth middleware
			// instead — the routes' success paths are covered by the per-route
			// tests below, which all run through this same default mux.
			req = httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			w = httptest.NewRecorder()
			defaultMux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			response = map[string]string{}
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			assert.NotEqual(t, demoRestrictedError, response["error"])
		})
	}
}

func TestWorkspaceHandler_DemoModeKeepsReadsOpen(t *testing.T) {
	_, workspaceSvc, demoMux, secretKey, _ := setupDemoTest(t)

	workspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Return([]*domain.Workspace{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.list", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	demoMux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWorkspaceHandler_Create(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock successful workspace creation
	expectedWorkspace := &domain.Workspace{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	workspaceSvc.EXPECT().
		CreateWorkspace(gomock.Any(), "testworkspace1", "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id, name, websiteURL, logoURL, coverURL, timezone string, fileManager domain.FileManagerSettings, defaultLanguage string, languages []string) (*domain.Workspace, error) {
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
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
				WebsiteURL:      "https://example1.com",
				LogoURL:         "https://example1.com/logo.png",
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
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
				WebsiteURL:      "https://example2.com",
				LogoURL:         "https://example2.com/logo.png",
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
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
			WebsiteURL:      "https://updated.com",
			LogoURL:         "https://updated.com/logo.png",
			CoverURL:        "https://updated.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
			WebsiteURL:      "https://updated.com",
			LogoURL:         "https://updated.com/logo.png",
			CoverURL:        "https://updated.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
		CreateWorkspace(gomock.Any(), "testworkspace1", "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Create request with valid data
	reqBody := domain.CreateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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

func TestWorkspaceHandler_Create_WorkspaceLimitReached(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// Mock service returning workspace limit error
	workspaceSvc.EXPECT().
		CreateWorkspace(gomock.Any(), "testworkspace1", "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, &domain.ErrWorkspaceLimitReached{Limit: 3, Current: 3})

	reqBody := domain.CreateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "workspace limit reached")
}

func TestWorkspaceHandler_Create_WithMultipleLanguages(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	expectedWorkspace := &domain.Workspace{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "fr",
			Languages:       []string{"fr", "en", "es"},
			FileManager: domain.FileManagerSettings{
				Endpoint:  "https://s3.amazonaws.com",
				Bucket:    "my-bucket",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
			},
		},
	}
	workspaceSvc.EXPECT().
		CreateWorkspace(gomock.Any(), "testworkspace1", "Test Workspace", "https://example.com", "https://example.com/logo.png", "https://example.com/cover.png", "UTC", gomock.Any(), "fr", []string{"fr", "en", "es"}).
		Return(expectedWorkspace, nil)

	reqBody := domain.CreateWorkspaceRequest{
		ID:   "testworkspace1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			WebsiteURL:      "https://example.com",
			LogoURL:         "https://example.com/logo.png",
			CoverURL:        "https://example.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "fr",
			Languages:       []string{"fr", "en", "es"},
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

	assert.Equal(t, http.StatusCreated, w.Code)

	var response domain.Workspace
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "fr", response.Settings.DefaultLanguage)
	assert.Equal(t, []string{"fr", "en", "es"}, response.Settings.Languages)
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
			WebsiteURL:      "https://updated.com",
			LogoURL:         "https://updated.com/logo.png",
			CoverURL:        "https://updated.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
			WebsiteURL:      "https://updated.com",
			LogoURL:         "https://updated.com/logo.png",
			CoverURL:        "https://updated.com/cover.png",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
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
		// InviteMemberRequest.Validate rejects an empty permissions map.
		Permissions: domain.NewFullPermissions(),
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
		// InviteMemberRequest.Validate rejects an empty permissions map.
		Permissions: domain.NewFullPermissions(),
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
		// InviteMemberRequest.Validate rejects an empty permissions map.
		Permissions: domain.NewFullPermissions(),
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
		CreateAPIKey(gomock.Any(), "workspace-123", "api", gomock.Any()).
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
		CreateAPIKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
		CreateAPIKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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

func TestWorkspaceHandler_HandleCreateAPIKey_ForwardsPermissions(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	scoped := domain.UserPermissions{
		domain.PermissionResourceTransactional: {Read: true, Write: true},
		domain.PermissionResourceContacts:      {Read: true},
	}

	// setupTest finishes the controller before the test body runs, so the mock's own
	// verification cannot carry this assertion. Capture the argument instead.
	var captured domain.UserPermissions
	workspaceSvc.EXPECT().
		CreateAPIKey(gomock.Any(), "workspace-123", "ci-bot", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ string, permissions domain.UserPermissions) (string, string, error) {
			captured = permissions
			return "token-123", "ci-bot@example.com", nil
		})

	reqBody := domain.CreateAPIKeyRequest{
		WorkspaceID: "workspace-123",
		EmailPrefix: "ci-bot",
		Permissions: scoped,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, scoped, captured)
}

func TestWorkspaceHandler_HandleCreateAPIKey_ForwardsNilPermissions(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// An omitted map must stay nil down to the service, which reads nil as full
	// access — the contract the endpoint had before it took permissions at all.
	captured := domain.UserPermissions{}
	workspaceSvc.EXPECT().
		CreateAPIKey(gomock.Any(), "workspace-123", "api", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ string, permissions domain.UserPermissions) (string, string, error) {
			captured = permissions
			return "token-123", "api@example.com", nil
		})

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey",
		strings.NewReader(`{"workspace_id":"workspace-123","email_prefix":"api"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, captured)
}

func TestWorkspaceHandler_HandleCreateAPIKey_WrappedUnauthorizedError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// The service wraps the denial on its way up ("failed to authenticate user: %w").
	// A bare type assertion misses that and answers 500 to what is really a 403.
	wrapped := fmt.Errorf("failed to create api key: %w",
		&domain.ErrUnauthorized{Message: "user is not a member of this workspace"})
	workspaceSvc.EXPECT().
		CreateAPIKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", "", wrapped)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey",
		strings.NewReader(`{"workspace_id":"workspace-123","email_prefix":"api"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "Only workspace owners can create API keys", response["error"])
}

func TestWorkspaceHandler_HandleCreateAPIKey_DuplicateEmailPrefix(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// users.email is unique across the deployment: a prefix already taken is a
	// conflict the caller can act on, not a server failure.
	wrapped := fmt.Errorf("api key email already in use: %w",
		&domain.ErrUserExists{Message: "user with this email already exists"})
	workspaceSvc.EXPECT().
		CreateAPIKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", "", wrapped)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey",
		strings.NewReader(`{"workspace_id":"workspace-123","email_prefix":"api"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Contains(t, response["error"], "already in use")
}

func TestWorkspaceHandler_HandleCreateAPIKey_UnknownPermissionResource(t *testing.T) {
	_, _, mux, secretKey, _ := setupTest(t)

	// No service call at all: an unknown resource is rejected by request validation.
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey",
		strings.NewReader(`{"workspace_id":"workspace-123","email_prefix":"api","permissions":{"nope":{"read":true,"write":true}}}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Contains(t, response["error"], "nope")
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

func TestWorkspaceHandler_HandleConnectZapier(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// The address is derived server-side from the label, so the handler forwards the
	// label and hands back whatever was minted.
	workspaceSvc.EXPECT().
		ConnectZapier(gomock.Any(), "workspace-123", "Marketing").
		Return("zapier-token-123", "zapier-marketing-3f9a1c02@example.com", "integration-123", nil)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.connectZapier",
		strings.NewReader(`{"workspace_id":"workspace-123","label":"Marketing"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "zapier-token-123", response["token"])
	assert.Equal(t, "zapier-marketing-3f9a1c02@example.com", response["email"])
	assert.Equal(t, "integration-123", response["integration_id"])
}

func TestWorkspaceHandler_HandleConnectZapier_MethodNotAllowed(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.connectZapier", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	handler.handleConnectZapier(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestWorkspaceHandler_HandleConnectZapier_InvalidBody(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.connectZapier", strings.NewReader("invalid json"))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	handler.handleConnectZapier(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "Invalid request body", response["error"])
}

func TestWorkspaceHandler_HandleConnectZapier_ValidationError(t *testing.T) {
	// Raw bodies, because a field the client omitted is the case under test and a
	// typed literal cannot express one. The service mock carries no expectation, so
	// a body that reached it would fail here.
	testCases := []struct {
		name     string
		body     string
		contains string
	}{
		{
			name:     "missing_workspace_id",
			body:     `{"label":"Marketing"}`,
			contains: "workspace_id",
		},
		{
			// The service mints the key before it names the integration and accepts an
			// empty label without complaint, so nothing downstream would catch this.
			name:     "missing_label",
			body:     `{"workspace_id":"workspace-123"}`,
			contains: "label",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, mux, secretKey, _ := setupTest(t)

			req := httptest.NewRequest(http.MethodPost, "/api/workspaces.connectZapier", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response map[string]string
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			assert.Contains(t, response["error"], tc.contains)
		})
	}
}

func TestWorkspaceHandler_HandleConnectZapier_UnauthorizedError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	workspaceSvc.EXPECT().
		ConnectZapier(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", "", "", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"})

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.connectZapier",
		strings.NewReader(`{"workspace_id":"workspace-123","label":"Marketing"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "user is not an owner of the workspace", response["error"])
}

func TestWorkspaceHandler_HandleConnectZapier_DuplicateAddress(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	// What an exhausted retry looks like: still wrapping *ErrUserExists, two layers
	// deep, so the mapping has to reach for it with errors.As rather than compare.
	wrapped := fmt.Errorf("failed to mint a unique zapier api key address after 5 attempts: %w",
		fmt.Errorf("api key email already in use: %w",
			&domain.ErrUserExists{Message: "user with this email already exists"}))
	workspaceSvc.EXPECT().
		ConnectZapier(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", "", "", wrapped)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.connectZapier",
		strings.NewReader(`{"workspace_id":"workspace-123","label":"Marketing"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Contains(t, response["error"], "already in use")
}

func TestWorkspaceHandler_HandleConnectZapier_ServiceError(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	workspaceSvc.EXPECT().
		ConnectZapier(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", "", "", fmt.Errorf("service error"))

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.connectZapier",
		strings.NewReader(`{"workspace_id":"workspace-123","label":"Marketing"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "Failed to connect Zapier", response["error"])
}

func TestWorkspaceHandler_HandleConnectZapier_RestrictedInDemo(t *testing.T) {
	_, _, demoMux, secretKey, _ := setupDemoTest(t)

	// Authenticated, and the body is the one the success test uses: the refusal has to
	// come from the middleware, not from auth or validation. No expectation on the
	// service mock, so a route that lost its wrapper would mint a key here and fail.
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.connectZapier",
		strings.NewReader(`{"workspace_id":"workspace-123","label":"Marketing"}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	demoMux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, demoRestrictedError, response["error"])
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

func TestWorkspaceHandler_HandleSetCustomFieldLabels(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	validBody := domain.SetCustomFieldLabelsRequest{
		WorkspaceID: "workspace123",
		CustomFieldLabels: map[string]string{
			"custom_string_1": "Company Name",
		},
	}

	t.Run("successful update", func(t *testing.T) {
		workspaceSvc.EXPECT().
			SetCustomFieldLabels(gomock.Any(), "workspace123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, labels map[string]string) error {
				assert.Equal(t, "Company Name", labels["custom_string_1"])
				return nil
			})

		body, err := json.Marshal(validBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setCustomFieldLabels", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "success", response["status"])
		assert.Equal(t, "Custom field labels updated successfully", response["message"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.setCustomFieldLabels", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setCustomFieldLabels", strings.NewReader("invalid json"))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "Invalid request body", response["error"])
	})

	t.Run("validation error - missing workspace_id", func(t *testing.T) {
		body, err := json.Marshal(domain.SetCustomFieldLabelsRequest{
			CustomFieldLabels: map[string]string{"custom_string_1": "Company Name"},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setCustomFieldLabels", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Contains(t, response["error"], "workspace_id is required")
	})

	t.Run("validation error - invalid label key", func(t *testing.T) {
		body, err := json.Marshal(domain.SetCustomFieldLabelsRequest{
			WorkspaceID:       "workspace123",
			CustomFieldLabels: map[string]string{"custom_string_99": "Bad"},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setCustomFieldLabels", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Contains(t, response["error"], "invalid custom field key")
	})

	t.Run("permission denied returns 403", func(t *testing.T) {
		permErr := domain.NewPermissionError(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite, "Insufficient permissions: write access to workspace required")
		workspaceSvc.EXPECT().
			SetCustomFieldLabels(gomock.Any(), "workspace123", gomock.Any()).
			Return(permErr)

		body, err := json.Marshal(validBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setCustomFieldLabels", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		workspaceSvc.EXPECT().
			SetCustomFieldLabels(gomock.Any(), "workspace123", gomock.Any()).
			Return(assert.AnError)

		body, err := json.Marshal(validBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setCustomFieldLabels", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestWorkspaceHandler_HandleSetBlogSettings(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	validBody := domain.SetBlogSettingsRequest{
		WorkspaceID:  "workspace123",
		BlogEnabled:  true,
		BlogSettings: &domain.BlogSettings{Title: "My Blog"},
	}

	t.Run("successful update", func(t *testing.T) {
		workspaceSvc.EXPECT().
			SetBlogSettings(gomock.Any(), "workspace123", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, enabled *bool, settings *domain.BlogSettings, _ bool) error {
				require.NotNil(t, enabled, "a body that carries blog_enabled must reach the service as a definite value")
				assert.True(t, *enabled)
				assert.Equal(t, "My Blog", settings.Title)
				return nil
			})

		body, err := json.Marshal(validBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "success", response["status"])
		assert.Equal(t, "Blog settings updated successfully", response["message"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.setBlogSettings", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", strings.NewReader("invalid json"))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "Invalid request body", response["error"])
	})

	t.Run("validation error - missing workspace_id", func(t *testing.T) {
		body, err := json.Marshal(domain.SetBlogSettingsRequest{
			BlogEnabled: true,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Contains(t, response["error"], "workspace_id is required")
	})

	t.Run("validation error - invalid blog settings", func(t *testing.T) {
		body, err := json.Marshal(domain.SetBlogSettingsRequest{
			WorkspaceID:  "workspace123",
			BlogEnabled:  true,
			BlogSettings: &domain.BlogSettings{HomePageSize: 999},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Contains(t, response["error"], "home_page_size must be between 1 and 100")
	})

	t.Run("permission denied returns 403", func(t *testing.T) {
		permErr := domain.NewPermissionError(domain.PermissionResourceBlog, domain.PermissionTypeWrite, "Insufficient permissions: write access to blog required")
		workspaceSvc.EXPECT().
			SetBlogSettings(gomock.Any(), "workspace123", gomock.Any(), gomock.Any(), gomock.Any()).
			Return(permErr)

		body, err := json.Marshal(validBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		workspaceSvc.EXPECT().
			SetBlogSettings(gomock.Any(), "workspace123", gomock.Any(), gomock.Any(), gomock.Any()).
			Return(assert.AnError)

		body, err := json.Marshal(validBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestWorkspaceHandler_HandleSetUserPermissions(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	t.Run("successful permission update", func(t *testing.T) {
		// Mock successful permission update
		workspaceSvc.EXPECT().
			SetUserPermissions(gomock.Any(), "workspace123", "user-123", gomock.Any()).
			Return(nil)

		// Create request
		reqBody := domain.SetUserPermissionsRequest{
			WorkspaceID: "workspace123",
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
		assert.Contains(t, response["error"], "workspace_id is required")
	})

	t.Run("unknown permission resource is a bad request", func(t *testing.T) {
		// The workspace service mock has no expectation here: an unknown resource
		// key must be refused by the handler, not carried into the service.
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id": "workspace123",
			"user_id":      "user-123",
			"permissions":  map[string]interface{}{"not_a_resource": map[string]bool{"read": true}},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPermissions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Contains(t, response["error"], "not_a_resource")
	})

	t.Run("unauthorized error", func(t *testing.T) {
		// Mock unauthorized error
		unauthorizedErr := &domain.ErrUnauthorized{Message: "Only workspace owners can manage user permissions"}
		workspaceSvc.EXPECT().
			SetUserPermissions(gomock.Any(), "workspace123", "user-123", gomock.Any()).
			Return(unauthorizedErr)

		// Create request
		reqBody := domain.SetUserPermissionsRequest{
			WorkspaceID: "workspace123",
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
			SetUserPermissions(gomock.Any(), "workspace123", "user-123", gomock.Any()).
			Return(fmt.Errorf("service error"))

		// Create request
		reqBody := domain.SetUserPermissionsRequest{
			WorkspaceID: "workspace123",
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

// --- Authorization denials map to 403 Forbidden (not a generic 500) ---

func TestWorkspaceHandler_Get_Forbidden(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	// A non-member (non-root) is denied; the service surfaces the typed not-in-workspace
	// error (wrapped, as GetWorkspace does in production).
	workspaceService.EXPECT().
		GetWorkspace(gomock.Any(), "workspace123").
		Return(nil, fmt.Errorf("failed to authenticate user: %w", domain.ErrUserNotInWorkspace))

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.get?id=workspace123", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	handler.handleGet(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestWorkspaceHandler_Update_Forbidden(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	workspaceService.EXPECT().
		UpdateWorkspace(gomock.Any(), "workspace123", "New Name", gomock.Any()).
		Return(nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"})

	reqBody := domain.UpdateWorkspaceRequest{
		ID:       "workspace123",
		Name:     "New Name",
		Settings: domain.WorkspaceSettings{Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"}},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.update", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestWorkspaceHandler_Delete_Forbidden(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	workspaceService.EXPECT().
		DeleteWorkspace(gomock.Any(), "workspace123").
		Return(&domain.ErrUnauthorized{Message: "user is not an owner of the workspace"})

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.delete", bytes.NewBuffer([]byte(`{"id": "workspace123"}`)))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	handler.handleDelete(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestWorkspaceHandler_HandleMembers_Forbidden(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	workspaceService.EXPECT().
		GetWorkspaceMembersWithEmail(gomock.Any(), "workspace123").
		Return(nil, &domain.ErrUnauthorized{Message: "You do not have access to this workspace"})

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces.members?id=workspace123", nil)
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	handler.handleMembers(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestWorkspaceHandler_HandleInviteMember_Forbidden(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	// Inviting is owner-only; the denial arrives wrapped, as InviteMember wraps it
	// in production.
	workspaceService.EXPECT().
		InviteMember(gomock.Any(), "workspace123", "invitee@example.com", gomock.Any()).
		Return(nil, "", fmt.Errorf("failed to invite member: %w", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}))

	reqBody := domain.InviteMemberRequest{
		WorkspaceID: "workspace123",
		Email:       "invitee@example.com",
		Permissions: domain.NewFullPermissions(),
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.inviteMember", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	handler.handleInviteMember(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "user is not an owner of the workspace", response["error"])
}

func TestWorkspaceHandler_HandleInviteMember_UnknownPermissionResource(t *testing.T) {
	handler, _, _, secretKey, _ := setupTest(t)

	// No service expectation: an unknown resource key must be refused here rather
	// than carried into the service.
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.inviteMember",
		strings.NewReader(`{"workspace_id":"workspace123","email":"invitee@example.com","permissions":{"not_a_resource":{"read":true}}}`))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	handler.handleInviteMember(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Contains(t, response["error"], "not_a_resource")
}

func TestWorkspaceHandler_HandleSetUserPermissions_Forbidden(t *testing.T) {
	handler, workspaceService, _, secretKey, _ := setupTest(t)

	workspaceService.EXPECT().
		SetUserPermissions(gomock.Any(), "workspace123", "user-123", gomock.Any()).
		Return(&domain.ErrUnauthorized{Message: "only workspace owners can manage user permissions"})

	reqBody := domain.SetUserPermissionsRequest{
		WorkspaceID: "workspace123",
		UserID:      "user-123",
		Permissions: domain.NewFullPermissions(),
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPermissions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
	w := httptest.NewRecorder()
	handler.handleSetUserPermissions(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "only workspace owners can manage user permissions", response["error"])
}

// TestWorkspaceHandler_List_RevokedKeyIsAnAuthFailure pins the status code a
// revoked API key gets.
//
// Revoking a key deletes its user row, and that delete is the whole of the
// revocation: the token itself carries no jti, has no denylist, and stays signed
// and valid for ten years. So "the user this key names is gone" means "this key
// was revoked" — and answering 500 told every API client its server was broken
// rather than that its credential was dead. Zapier in particular raises its
// "reconnect this account" prompt on 401 and on nothing else, so a 500 left
// every Zap failing with an error its owner could not act on.
func TestWorkspaceHandler_List_RevokedKeyIsAnAuthFailure(t *testing.T) {
	handler, workspaceService, _, _, _ := setupTest(t)

	workspaceService.EXPECT().
		ListWorkspaces(gomock.Any()).
		Return(nil, fmt.Errorf("failed to authenticate user: %w", domain.ErrAPIKeyRevoked))

	w := httptest.NewRecorder()
	handler.handleList(w, httptest.NewRequest(http.MethodGet, "/api/workspaces.list", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "API key has been revoked", response["error"])
}

// TestWorkspaceHandler_FileManagerSecretDependsOnTheCaller covers a credential
// leak that lives in the transport rather than in any one caller.
//
// Workspace.Redact deliberately keeps the S3 secret: the console builds an S3
// client in the browser from that exact field and talks to the bucket directly,
// so blanking it for everyone breaks the file manager rather than hardening
// anything. An API key, though, authenticates every endpoint below exactly as a
// session does — workspaces.list needs no permission at all, which is how an
// integration discovers what it is attached to — and no integration has any use
// for a live bucket credential, in a body those platforms routinely log whole.
//
// Driven through the mux with a real signed token so the caller's identity comes
// from the JWT claim the way it does in production, and asserted against the raw
// bytes rather than a decoded struct, because the leak is what goes on the wire.
func TestWorkspaceHandler_FileManagerSecretDependsOnTheCaller(t *testing.T) {
	const fileManagerSecret = "SENTINEL-live-bucket-secret"
	const fileManagerCiphertext = "SENTINEL-bucket-ciphertext"

	// A body that passes validation without configuring a file manager: the
	// secret under test comes back from the service, not from the request.
	const writeBody = `{"id":"workspace123","name":"Test Workspace","settings":{"timezone":"UTC","default_language":"en","languages":["en"]}}`

	newWorkspace := func() *domain.Workspace {
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test Workspace"}
		workspace.Settings.FileManager = domain.FileManagerSettings{
			Provider:           "s3",
			Bucket:             "assets",
			AccessKey:          "AKIAEXAMPLE",
			SecretKey:          fileManagerSecret,
			EncryptedSecretKey: fileManagerCiphertext,
		}
		return workspace
	}

	endpoints := []struct {
		name           string
		expectedStatus int
		expect         func(svc *mocks.MockWorkspaceServiceInterface)
		request        func() *http.Request
	}{
		{
			name:           "workspaces.list",
			expectedStatus: http.StatusOK,
			expect: func(svc *mocks.MockWorkspaceServiceInterface) {
				svc.EXPECT().ListWorkspaces(gomock.Any()).Return([]*domain.Workspace{newWorkspace()}, nil)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/workspaces.list", nil)
			},
		},
		{
			name:           "workspaces.get",
			expectedStatus: http.StatusOK,
			expect: func(svc *mocks.MockWorkspaceServiceInterface) {
				svc.EXPECT().GetWorkspace(gomock.Any(), "workspace123").Return(newWorkspace(), nil)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/workspaces.get?id=workspace123", nil)
			},
		},
		{
			name:           "workspaces.create",
			expectedStatus: http.StatusCreated,
			expect: func(svc *mocks.MockWorkspaceServiceInterface) {
				svc.EXPECT().CreateWorkspace(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(newWorkspace(), nil)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/workspaces.create", strings.NewReader(writeBody))
			},
		},
		{
			name:           "workspaces.update",
			expectedStatus: http.StatusOK,
			expect: func(svc *mocks.MockWorkspaceServiceInterface) {
				svc.EXPECT().UpdateWorkspace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(newWorkspace(), nil)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/workspaces.update", strings.NewReader(writeBody))
			},
		},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name+" withholds it from an API key", func(t *testing.T) {
			_, workspaceService, mux, secretKey, _ := setupTest(t)
			endpoint.expect(workspaceService)

			req := endpoint.request()
			req.Header.Set("Authorization", "Bearer "+createTestAPIKeyToken(t, secretKey, "api-key-user"))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			require.Equal(t, endpoint.expectedStatus, w.Code)
			body := w.Body.String()
			assert.NotContains(t, body, fileManagerSecret)
			assert.NotContains(t, body, fileManagerCiphertext)
			// Everything an integration actually reads still goes out.
			assert.Contains(t, body, "workspace123")
			assert.Contains(t, body, "assets")
		})

		t.Run(endpoint.name+" still serves it to a console session", func(t *testing.T) {
			_, workspaceService, mux, secretKey, _ := setupTest(t)
			endpoint.expect(workspaceService)

			req := endpoint.request()
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			require.Equal(t, endpoint.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), fileManagerSecret,
				"withholding this from the console breaks the browser file manager")
		})
	}
}

// The handler is where the body stops being a body, so it is where an absent key
// has to survive: UpdateWorkspace is handed the settings alone, and a settings
// value that has forgotten what the caller left out cannot preserve anything.
func TestWorkspaceHandler_HandleUpdate_OmittedSettingsSurviveTheDecode(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	storedEndpoint := "https://track.example.com"
	stored := domain.WorkspaceSettings{
		FileManager: domain.FileManagerSettings{
			Endpoint:           "https://s3.example.com",
			Bucket:             "stored-bucket",
			AccessKey:          "AKIASTORED",
			EncryptedSecretKey: "STORED-CIPHERTEXT",
		},
		TransactionalEmailProviderID: "provider-transactional",
		MarketingEmailProviderID:     "provider-marketing",
		EmailTrackingEnabled:         true,
		CustomEndpointURL:            &storedEndpoint,
	}

	var got domain.WorkspaceSettings
	workspaceSvc.EXPECT().
		UpdateWorkspace(gomock.Any(), "testworkspace1", "Renamed", gomock.Any()).
		DoAndReturn(func(_ context.Context, id, name string, settings domain.WorkspaceSettings) (*domain.Workspace, error) {
			got = settings
			return &domain.Workspace{ID: id, Name: name, Settings: settings}, nil
		})

	// A raw body, because a struct literal cannot express a missing key — which is
	// how the wipe shipped. The console sends the whole settings object; every other
	// client sends what it means to change.
	body := `{"id":"testworkspace1","name":"Renamed","settings":{"timezone":"UTC","default_language":"en","languages":["en"]}}`

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	got.PreserveOmitted(stored)
	assert.True(t, got.EmailTrackingEnabled, "tracking must not be switched off by a body that never mentions it")
	assert.Equal(t, "STORED-CIPHERTEXT", got.FileManager.EncryptedSecretKey)
	assert.Equal(t, "provider-transactional", got.TransactionalEmailProviderID)
	assert.Equal(t, "provider-marketing", got.MarketingEmailProviderID)
	assert.Equal(t, stored.CustomEndpointURL, got.CustomEndpointURL)
}

// The same for an integration rename: the provider block a body never carried must
// still be recognisable as absent by the time the service sees the request.
func TestWorkspaceHandler_HandleUpdateIntegration_OmittedProviderSurvivesTheDecode(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	var got domain.UpdateIntegrationRequest
	workspaceSvc.EXPECT().
		UpdateIntegration(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req domain.UpdateIntegrationRequest) error {
			got = req
			return nil
		})

	body := `{"workspace_id":"workspace-123","integration_id":"integration-123","name":"Renamed"}`

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.updateIntegration", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	assert.False(t, got.ProviderSpecified(),
		"a rename must not reach the service looking like a request to clear the provider")
	assert.Equal(t, "Renamed", got.Name)
}

// A body that says nothing about blog_enabled must not reach the service as a
// definite "off". The console already works around this by recomputing the flag
// from the workspace it happens to hold, which only works because the console is
// the one caller that always holds one.
func TestWorkspaceHandler_HandleSetBlogSettings_OmittedEnabledFlagStaysAbsent(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	workspaceSvc.EXPECT().
		SetBlogSettings(gomock.Any(), "workspace123", gomock.Nil(), gomock.Any(), gomock.Any()).
		Return(nil)

	// Raw body: a struct literal cannot express the missing key.
	body := `{"workspace_id":"workspace123","blog_settings":{"title":"My Blog"}}`

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// A body that names blog_enabled and nothing else must not reach the write looking like a
// request to clear the blog's configuration. SetBlogSettings stores whatever it is told
// about, so an absent blog_settings erased the title, the SEO block, the pagination and the
// feed settings — and the console's own disable button sends exactly that body whenever the
// settings fields are not on screen.
//
// The handler answers only "did the body mention it"; the merge itself belongs to the write,
// which already holds the workspace it is about to save. Reading the stored settings here
// instead meant a second lookup, with its own answer to who may see a workspace, standing in
// front of every blog save.
func TestWorkspaceHandler_HandleSetBlogSettings_SettingsPresenceReachesTheWrite(t *testing.T) {
	_, workspaceSvc, mux, secretKey, _ := setupTest(t)

	post := func(t *testing.T, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setBlogSettings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	// Raw bodies throughout: a struct literal cannot express the missing key this is about.
	t.Run("a body with no blog_settings key says so, and costs no read", func(t *testing.T) {
		workspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Times(0)

		var specified bool
		var received *domain.BlogSettings
		workspaceSvc.EXPECT().
			SetBlogSettings(gomock.Any(), "workspace123", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, enabled *bool, settings *domain.BlogSettings, settingsSpecified bool) error {
				specified = settingsSpecified
				received = settings
				require.NotNil(t, enabled)
				assert.False(t, *enabled, "the switch the caller did flip must still land")
				return nil
			})

		w := post(t, `{"workspace_id":"workspace123","blog_enabled":false}`)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.False(t, specified, "switching the blog off must not read as a request to erase how it is configured")
		assert.Nil(t, received)
	})

	t.Run("an explicit null is a clear, not a silence", func(t *testing.T) {
		workspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Times(0)

		workspaceSvc.EXPECT().
			SetBlogSettings(gomock.Any(), "workspace123", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ *bool, settings *domain.BlogSettings, settingsSpecified bool) error {
				assert.True(t, settingsSpecified, "an object has a null that means something: wiping the configuration stays expressible")
				assert.Nil(t, settings)
				return nil
			})

		w := post(t, `{"workspace_id":"workspace123","blog_enabled":true,"blog_settings":null}`)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("a body that carries blog_settings hands them over as a replacement", func(t *testing.T) {
		workspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Times(0)

		workspaceSvc.EXPECT().
			SetBlogSettings(gomock.Any(), "workspace123", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ *bool, settings *domain.BlogSettings, settingsSpecified bool) error {
				assert.True(t, settingsSpecified)
				require.NotNil(t, settings)
				assert.Equal(t, "Replaced", settings.Title)
				return nil
			})

		w := post(t, `{"workspace_id":"workspace123","blog_enabled":true,"blog_settings":{"title":"Replaced"}}`)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// A workspace the caller is not a member of is not this handler's to rule on: the write
	// owns the authorization answer, and short-circuiting here would tell an authenticated
	// stranger which workspace ids exist.
	t.Run("a workspace the caller cannot see is left to the write to refuse", func(t *testing.T) {
		workspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Times(0)

		permErr := domain.NewPermissionError(domain.PermissionResourceBlog, domain.PermissionTypeWrite, "Insufficient permissions: write access to blog required")
		workspaceSvc.EXPECT().
			SetBlogSettings(gomock.Any(), "workspace123", gomock.Any(), gomock.Any(), gomock.Any()).
			Return(permErr)

		w := post(t, `{"workspace_id":"workspace123","blog_enabled":false}`)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
