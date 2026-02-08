package http_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	http_handler "github.com/Notifuse/notifuse/internal/http"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	notifusemjml "github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test setup helper
func setupTemplateBlockHandlerTest(t *testing.T) (*mocks.MockTemplateBlockService, *pkgmocks.MockLogger, string, []byte, func()) {
	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockService := mocks.NewMockTemplateBlockService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Create key pair for testing
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	handler := http_handler.NewTemplateBlockHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	cleanup := func() {
		server.Close()
	}

	return mockService, mockLogger, server.URL, jwtSecret, cleanup
}

func createTestTemplateBlock() *domain.TemplateBlock {
	blockJSON := []byte(`{"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}`)
	blk, _ := notifusemjml.UnmarshalEmailBlock(blockJSON)
	now := time.Now().UTC()
	return &domain.TemplateBlock{
		ID:      uuid.New().String(),
		Name:    "Test Block",
		Block:   blk,
		Created: now,
		Updated: now,
	}
}

// Create a test token for authentication, signed with the correct secret key
func createTestTokenForBlock(jwtSecret []byte) string {
	claims := &service.UserClaims{
		UserID:    "test-user",
		Type:      string(domain.UserTypeUser),
		SessionID: "test-session",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, _ := token.SignedString(jwtSecret)
	return signedToken
}

// Helper to create and send request
func sendBlockRequest(t *testing.T, method, urlStr, token string, body interface{}) *http.Response {
	var reqBodyReader *bytes.Reader

	if body != nil {
		if strBody, ok := body.(string); ok {
			// Handle raw string body (for bad JSON tests)
			reqBodyReader = bytes.NewReader([]byte(strBody))
		} else {
			// Marshal other body types to JSON
			reqBodyBytes, err := json.Marshal(body)
			require.NoError(t, err)
			reqBodyReader = bytes.NewReader(reqBodyBytes)
		}
	} else {
		reqBodyReader = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequest(method, urlStr, reqBodyReader)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Use a client that doesn't follow redirects for more predictable testing
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	require.NoError(t, err)

	return resp
}

func TestTemplateBlockHandler_HandleList(t *testing.T) {
	workspaceID := "workspace123"

	testCases := []struct {
		name           string
		queryParams    url.Values
		setupMock      func(*mocks.MockTemplateBlockService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			queryParams: url.Values{"workspace_id": {workspaceID}},
			setupMock: func(m *mocks.MockTemplateBlockService) {
				block1 := createTestTemplateBlock()
				block2 := createTestTemplateBlock()
				m.EXPECT().ListTemplateBlocks(gomock.Any(), workspaceID).Return([]*domain.TemplateBlock{block1, block2}, nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Success Empty List",
			queryParams: url.Values{"workspace_id": {workspaceID}},
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().ListTemplateBlocks(gomock.Any(), workspaceID).Return([]*domain.TemplateBlock{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			queryParams: url.Values{"workspace_id": {workspaceID}},
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().ListTemplateBlocks(gomock.Any(), workspaceID).Return(nil, errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Missing Workspace ID",
			queryParams:    url.Values{},
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			queryParams:    url.Values{"workspace_id": {workspaceID}},
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateBlockHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			listURL := fmt.Sprintf("%s/api/templateBlocks.list?%s", serverURL, tc.queryParams.Encode())
			token := ""
			if tc.authenticate {
				token = createTestTokenForBlock(secretKey)
			}

			resp := sendBlockRequest(t, http.MethodGet, listURL, token, nil)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				blocks, ok := responseMap["blocks"].([]interface{})
				assert.True(t, ok, "Response should contain a blocks array")
				if tc.name == "Success" {
					assert.NotEmpty(t, blocks)
				}
			}
		})
	}
}

func TestTemplateBlockHandler_HandleGet(t *testing.T) {
	workspaceID := "workspace123"
	blockID := uuid.New().String()

	testCases := []struct {
		name           string
		queryParams    url.Values
		setupMock      func(*mocks.MockTemplateBlockService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			queryParams: url.Values{"workspace_id": {workspaceID}, "id": {blockID}},
			setupMock: func(m *mocks.MockTemplateBlockService) {
				block := createTestTemplateBlock()
				block.ID = blockID
				m.EXPECT().GetTemplateBlock(gomock.Any(), workspaceID, blockID).Return(block, nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Not Found",
			queryParams: url.Values{"workspace_id": {workspaceID}, "id": {blockID}},
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().GetTemplateBlock(gomock.Any(), workspaceID, blockID).Return(nil, &domain.ErrTemplateBlockNotFound{Message: "not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			queryParams: url.Values{"workspace_id": {workspaceID}, "id": {blockID}},
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().GetTemplateBlock(gomock.Any(), workspaceID, blockID).Return(nil, errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Missing Block ID",
			queryParams:    url.Values{"workspace_id": {workspaceID}},
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			queryParams:    url.Values{"workspace_id": {workspaceID}, "id": {blockID}},
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateBlockHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			getURL := fmt.Sprintf("%s/api/templateBlocks.get?%s", serverURL, tc.queryParams.Encode())
			token := ""
			if tc.authenticate {
				token = createTestTokenForBlock(secretKey)
			}

			resp := sendBlockRequest(t, http.MethodGet, getURL, token, nil)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				assert.NotNil(t, responseMap["block"])
			}
		})
	}
}

func TestTemplateBlockHandler_HandleCreate(t *testing.T) {
	workspaceID := "workspace123"
	blockJSON := []byte(`{"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}`)
	blk, _ := notifusemjml.UnmarshalEmailBlock(blockJSON)

	validRequest := domain.CreateTemplateBlockRequest{
		WorkspaceID: workspaceID,
		Name:        "New Block",
		Block:       blk,
	}

	invalidRequestMissingName := validRequest
	invalidRequestMissingName.Name = ""

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockTemplateBlockService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().CreateTemplateBlock(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:           "Invalid JSON",
			requestBody:    `{"invalid": json}`,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Missing Required Fields",
			requestBody:    invalidRequestMissingName,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().CreateTemplateBlock(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Duplicate ID Error",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().CreateTemplateBlock(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("already exists"))
			},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
		{
			name:           "Method Not Allowed",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectBody:     false,
			authenticate:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateBlockHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			createURL := fmt.Sprintf("%s/api/templateBlocks.create", serverURL)
			token := ""
			if tc.authenticate {
				token = createTestTokenForBlock(secretKey)
			}

			method := http.MethodPost
			if tc.name == "Method Not Allowed" {
				method = http.MethodGet
			}

			resp := sendBlockRequest(t, method, createURL, token, tc.requestBody)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusCreated {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				assert.NotNil(t, responseMap["block"])
			}
		})
	}
}

func TestTemplateBlockHandler_HandleUpdate(t *testing.T) {
	workspaceID := "workspace123"
	blockID := uuid.New().String()
	blockJSON := []byte(`{"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}`)
	blk, _ := notifusemjml.UnmarshalEmailBlock(blockJSON)

	validRequest := domain.UpdateTemplateBlockRequest{
		WorkspaceID: workspaceID,
		ID:          blockID,
		Name:        "Updated Block",
		Block:       blk,
	}

	invalidRequestMissingName := validRequest
	invalidRequestMissingName.Name = ""

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockTemplateBlockService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				block := createTestTemplateBlock()
				block.ID = blockID
				m.EXPECT().UpdateTemplateBlock(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:           "Invalid JSON",
			requestBody:    `{"invalid": json}`,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Missing Required Fields",
			requestBody:    invalidRequestMissingName,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Block Not Found",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().UpdateTemplateBlock(gomock.Any(), workspaceID, gomock.Any()).Return(&domain.ErrTemplateBlockNotFound{Message: "not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().UpdateTemplateBlock(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
		{
			name:           "Method Not Allowed",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectBody:     false,
			authenticate:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateBlockHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			updateURL := fmt.Sprintf("%s/api/templateBlocks.update", serverURL)
			token := ""
			if tc.authenticate {
				token = createTestTokenForBlock(secretKey)
			}

			method := http.MethodPost
			if tc.name == "Method Not Allowed" {
				method = http.MethodGet
			}

			resp := sendBlockRequest(t, method, updateURL, token, tc.requestBody)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				assert.NotNil(t, responseMap["block"])
			}
		})
	}
}

func TestTemplateBlockHandler_HandleDelete(t *testing.T) {
	workspaceID := "workspace123"
	blockID := uuid.New().String()

	validRequest := domain.DeleteTemplateBlockRequest{
		WorkspaceID: workspaceID,
		ID:          blockID,
	}

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockTemplateBlockService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().DeleteTemplateBlock(gomock.Any(), workspaceID, blockID).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:           "Invalid JSON",
			requestBody:    `{"invalid": json}`,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Block Not Found",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().DeleteTemplateBlock(gomock.Any(), workspaceID, blockID).Return(&domain.ErrTemplateBlockNotFound{Message: "not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateBlockService) {
				m.EXPECT().DeleteTemplateBlock(gomock.Any(), workspaceID, blockID).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
		{
			name:           "Method Not Allowed",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateBlockService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectBody:     false,
			authenticate:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateBlockHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			deleteURL := fmt.Sprintf("%s/api/templateBlocks.delete", serverURL)
			token := ""
			if tc.authenticate {
				token = createTestTokenForBlock(secretKey)
			}

			method := http.MethodPost
			if tc.name == "Method Not Allowed" {
				method = http.MethodGet
			}

			resp := sendBlockRequest(t, method, deleteURL, token, tc.requestBody)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				assert.True(t, responseMap["success"].(bool))
			}
		})
	}
}
