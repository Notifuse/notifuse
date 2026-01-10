package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSchemaVerificationService implements SchemaVerificationServiceInterface for testing
type MockSchemaVerificationService struct {
	verifyResult *domain.SchemaVerificationResult
	verifyError  error
	repairResult *domain.SchemaRepairResult
	repairError  error
}

func (m *MockSchemaVerificationService) VerifyAllSchemas(ctx context.Context) (*domain.SchemaVerificationResult, error) {
	return m.verifyResult, m.verifyError
}

func (m *MockSchemaVerificationService) RepairSchemas(ctx context.Context, req *domain.SchemaRepairRequest) (*domain.SchemaRepairResult, error) {
	return m.repairResult, m.repairError
}

// MockAuthServiceForSchema implements a subset of AuthService for testing
type MockAuthServiceForSchema struct {
	user  *domain.User
	error error
}

func (m *MockAuthServiceForSchema) AuthenticateUserFromContext(ctx context.Context) (*domain.User, error) {
	if m.error != nil {
		return nil, m.error
	}
	return m.user, nil
}

func createSchemaTestToken(t *testing.T, jwtSecret []byte, userID string) string {
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

func setupSchemaHandlerTest(t *testing.T) (*MockSchemaVerificationService, *MockAuthServiceForSchema, *http.ServeMux, []byte, string) {
	jwtSecret := []byte("test-secret-key-32-bytes-long!!!")
	rootEmail := "root@example.com"

	mockService := &MockSchemaVerificationService{}
	mockAuth := &MockAuthServiceForSchema{}
	log := logger.NewLogger()

	handler := NewSchemaVerificationHandler(
		mockService,
		mockAuth,
		func() ([]byte, error) { return jwtSecret, nil },
		rootEmail,
		log,
	)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return mockService, mockAuth, mux, jwtSecret, rootEmail
}

func TestSchemaVerificationHandler_VerifySchema_RootUser_Success(t *testing.T) {
	mockService, mockAuth, mux, jwtSecret, rootEmail := setupSchemaHandlerTest(t)

	// Setup mock auth to return root user
	mockAuth.user = &domain.User{
		ID:    "root-user-id",
		Email: rootEmail,
	}

	// Setup mock verification result
	mockService.verifyResult = &domain.SchemaVerificationResult{
		VerifiedAt: time.Now().UTC(),
		WorkspaceDBs: []domain.WorkspaceVerification{
			{
				WorkspaceID:   "ws-123",
				WorkspaceName: "Test Workspace",
				DatabaseVerification: domain.DatabaseVerification{
					Status: "passed",
					TriggerFunctions: []domain.FunctionVerification{
						{Name: "track_contact_changes", Exists: true},
					},
					Triggers: []domain.TriggerVerification{
						{Name: "contact_changes_trigger", TableName: "contacts", Exists: true},
					},
				},
			},
		},
		Summary: domain.VerificationSummary{
			TotalDatabases:  1,
			PassedDatabases: 1,
			FailedDatabases: 0,
			TotalIssues:     0,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/debug.verifySchema", nil)
	req.Header.Set("Authorization", "Bearer "+createSchemaTestToken(t, jwtSecret, "root-user-id"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response domain.SchemaVerificationResult
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.Summary.TotalDatabases)
	assert.Equal(t, 1, response.Summary.PassedDatabases)
}

func TestSchemaVerificationHandler_VerifySchema_NonRootUser_Forbidden(t *testing.T) {
	_, mockAuth, mux, jwtSecret, _ := setupSchemaHandlerTest(t)

	// Setup mock auth to return non-root user
	mockAuth.user = &domain.User{
		ID:    "regular-user-id",
		Email: "user@example.com", // Not root email
	}

	req := httptest.NewRequest(http.MethodGet, "/api/debug.verifySchema", nil)
	req.Header.Set("Authorization", "Bearer "+createSchemaTestToken(t, jwtSecret, "regular-user-id"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "root user only")
}

func TestSchemaVerificationHandler_VerifySchema_Unauthenticated(t *testing.T) {
	_, _, mux, _, _ := setupSchemaHandlerTest(t)

	// No auth token
	req := httptest.NewRequest(http.MethodGet, "/api/debug.verifySchema", nil)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSchemaVerificationHandler_VerifySchema_InvalidMethod(t *testing.T) {
	_, mockAuth, mux, jwtSecret, rootEmail := setupSchemaHandlerTest(t)

	mockAuth.user = &domain.User{
		ID:    "root-user-id",
		Email: rootEmail,
	}

	// POST instead of GET
	req := httptest.NewRequest(http.MethodPost, "/api/debug.verifySchema", nil)
	req.Header.Set("Authorization", "Bearer "+createSchemaTestToken(t, jwtSecret, "root-user-id"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSchemaVerificationHandler_RepairSchema_RootUser_Success(t *testing.T) {
	mockService, mockAuth, mux, jwtSecret, rootEmail := setupSchemaHandlerTest(t)

	mockAuth.user = &domain.User{
		ID:    "root-user-id",
		Email: rootEmail,
	}

	mockService.repairResult = &domain.SchemaRepairResult{
		RepairedAt: time.Now().UTC(),
		WorkspaceDBs: []domain.WorkspaceRepairResult{
			{
				WorkspaceID:        "ws-123",
				WorkspaceName:      "Test Workspace",
				Status:             "success",
				FunctionsRecreated: []string{"track_contact_changes"},
				TriggersRecreated:  []string{"contact_changes_trigger"},
			},
		},
		Summary: domain.RepairSummary{
			TotalWorkspaces:    1,
			SuccessfulRepairs:  1,
			FailedRepairs:      0,
			FunctionsRecreated: 1,
			TriggersRecreated:  1,
		},
	}

	reqBody := domain.SchemaRepairRequest{
		WorkspaceIDs:    []string{"ws-123"},
		RepairTriggers:  true,
		RepairFunctions: true,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/debug.repairSchema", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createSchemaTestToken(t, jwtSecret, "root-user-id"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response domain.SchemaRepairResult
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.Summary.TotalWorkspaces)
	assert.Equal(t, 1, response.Summary.SuccessfulRepairs)
}

func TestSchemaVerificationHandler_RepairSchema_NonRootUser_Forbidden(t *testing.T) {
	_, mockAuth, mux, jwtSecret, _ := setupSchemaHandlerTest(t)

	mockAuth.user = &domain.User{
		ID:    "regular-user-id",
		Email: "user@example.com",
	}

	reqBody := domain.SchemaRepairRequest{
		RepairTriggers:  true,
		RepairFunctions: true,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/debug.repairSchema", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createSchemaTestToken(t, jwtSecret, "regular-user-id"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSchemaVerificationHandler_RepairSchema_InvalidMethod(t *testing.T) {
	_, mockAuth, mux, jwtSecret, rootEmail := setupSchemaHandlerTest(t)

	mockAuth.user = &domain.User{
		ID:    "root-user-id",
		Email: rootEmail,
	}

	// GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/debug.repairSchema", nil)
	req.Header.Set("Authorization", "Bearer "+createSchemaTestToken(t, jwtSecret, "root-user-id"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSchemaVerificationHandler_RegisterRoutes(t *testing.T) {
	jwtSecret := []byte("test-secret-key-32-bytes-long!!!")
	rootEmail := "root@example.com"

	mockService := &MockSchemaVerificationService{}
	mockAuth := &MockAuthServiceForSchema{}
	log := logger.NewLogger()

	handler := NewSchemaVerificationHandler(
		mockService,
		mockAuth,
		func() ([]byte, error) { return jwtSecret, nil },
		rootEmail,
		log,
	)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Check that routes are registered by making requests
	// The 401 response means routes are registered but auth is missing
	req := httptest.NewRequest(http.MethodGet, "/api/debug.verifySchema", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/debug.repairSchema", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Ensure middleware is imported for compilation
var _ = middleware.NewAuthMiddleware
