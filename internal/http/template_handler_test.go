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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test setup helper
func setupTemplateHandlerTest(t *testing.T) (*mocks.MockTemplateService, *pkgmocks.MockLogger, string, []byte, func()) {
	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockService := mocks.NewMockTemplateService(ctrl)
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

	handler := http_handler.NewTemplateHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	cleanup := func() {
		server.Close()
	}

	return mockService, mockLogger, server.URL, jwtSecret, cleanup
}

func createTestEmailTemplate() *domain.EmailTemplate {
	base := notifusemjml.NewBaseBlock("root", notifusemjml.MJMLComponentMjml)
	base.Attributes["version"] = "4.0.0"
	return &domain.EmailTemplate{
		SenderID:         "sender123",
		Subject:          "Test Email",
		CompiledPreview:  "<html><body>Test</body></html>",
		VisualEditorTree: &notifusemjml.MJMLBlock{BaseBlock: base},
	}
}

// Create a test token for authentication, signed with the correct secret key
func createTestToken(jwtSecret []byte) string {
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
func sendRequest(t *testing.T, method, urlStr, token string, body interface{}) *http.Response {
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

func TestTemplateHandler_HandleList(t *testing.T) {
	workspaceID := "workspace123"

	testCases := []struct {
		name           string
		queryParams    url.Values
		setupMock      func(*mocks.MockTemplateService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			queryParams: url.Values{"workspace_id": {workspaceID}},
			setupMock: func(m *mocks.MockTemplateService) {
				now := time.Now().UTC()
				m.EXPECT().GetTemplates(gomock.Any(), workspaceID, "", "").Return([]*domain.Template{
					{ID: "template1", Name: "T1", Version: 1, Channel: "email", Category: "c1", Email: createTestEmailTemplate(), CreatedAt: now, UpdatedAt: now},
					{ID: "template2", Name: "T2", Version: 1, Channel: "email", Category: "c2", Email: createTestEmailTemplate(), CreatedAt: now, UpdatedAt: now},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			queryParams: url.Values{"workspace_id": {workspaceID}},
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().GetTemplates(gomock.Any(), workspaceID, "", "").Return(nil, errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Missing Workspace ID",
			queryParams:    url.Values{},
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest, // Validation happens before service call
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			queryParams:    url.Values{"workspace_id": {workspaceID}},
			setupMock:      func(m *mocks.MockTemplateService) {}, // No service call expected
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false, // Send request without token
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			listURL := fmt.Sprintf("%s/api/templates.list?%s", serverURL, tc.queryParams.Encode())
			token := ""
			if tc.authenticate {
				token = createTestToken(secretKey)
			}

			resp := sendRequest(t, http.MethodGet, listURL, token, nil)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				templates, ok := responseMap["templates"].([]interface{})
				assert.True(t, ok, "Response should contain a templates array")
				assert.NotEmpty(t, templates)
			} else if resp.StatusCode != http.StatusOK {
				// Optionally check error message structure for non-OK responses
				var errResp map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&errResp) // Ignore decode error if body is empty/not JSON
				// You could assert structure of errResp here if needed
				// fmt.Printf("DEBUG: Error response body for %s (%d): %+v\n", tc.name, resp.StatusCode, errResp) // Debugging
			}
		})
	}
}

func TestTemplateHandler_HandleGet(t *testing.T) {
	workspaceID := "workspace123"
	templateID := "template1"

	testCases := []struct {
		name           string
		queryParams    url.Values
		setupMock      func(*mocks.MockTemplateService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			queryParams: url.Values{"workspace_id": {workspaceID}, "id": {templateID}},
			setupMock: func(m *mocks.MockTemplateService) {
				now := time.Now().UTC()
				m.EXPECT().GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).Return(&domain.Template{
					ID: templateID, Name: "T1", Version: 1, Channel: "email", Category: "c1", Email: createTestEmailTemplate(), CreatedAt: now, UpdatedAt: now}, nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Success With Version",
			queryParams: url.Values{"workspace_id": {workspaceID}, "id": {templateID}, "version": {"2"}},
			setupMock: func(m *mocks.MockTemplateService) {
				now := time.Now().UTC()
				m.EXPECT().GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(2)).Return(&domain.Template{
					ID: templateID, Name: "T1", Version: 2, Channel: "email", Category: "c1", Email: createTestEmailTemplate(), CreatedAt: now, UpdatedAt: now}, nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Not Found",
			queryParams: url.Values{"workspace_id": {workspaceID}, "id": {templateID}},
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).Return(nil, &domain.ErrTemplateNotFound{Message: "not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			queryParams: url.Values{"workspace_id": {workspaceID}, "id": {templateID}},
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().GetTemplateByID(gomock.Any(), workspaceID, templateID, int64(0)).Return(nil, errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Missing Template ID",
			queryParams:    url.Values{"workspace_id": {workspaceID}},
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			queryParams:    url.Values{"workspace_id": {workspaceID}, "id": {templateID}},
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			getURL := fmt.Sprintf("%s/api/templates.get?%s", serverURL, tc.queryParams.Encode())
			token := ""
			if tc.authenticate {
				token = createTestToken(secretKey)
			}

			resp := sendRequest(t, http.MethodGet, getURL, token, nil)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				assert.NotNil(t, responseMap["template"])
			}
		})
	}
}

func TestTemplateHandler_HandleCreate(t *testing.T) {
	workspaceID := "workspace123"
	templateID := "newTemplate"
	validRequest := domain.CreateTemplateRequest{
		WorkspaceID: workspaceID,
		ID:          templateID,
		Name:        "New Template",
		Channel:     "email",
		Category:    "transactional",
		Email:       createTestEmailTemplate(),
	}

	invalidRequestMissingName := validRequest
	invalidRequestMissingName.Name = "" // Missing required field

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockTemplateService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().CreateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().CreateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Invalid Request Body (Bad JSON)",
			requestBody:    "this is not json", // Send raw string
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Validation Error (Missing Name)",
			requestBody:    invalidRequestMissingName,
			setupMock:      func(m *mocks.MockTemplateService) {}, // Validation happens before service call
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
		{
			name:           "Method Not Allowed (GET)",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectBody:     false,
			authenticate:   true,
			// We test method allowance by sending GET in the loop below
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			createURL := fmt.Sprintf("%s/api/templates.create", serverURL)
			token := ""
			if tc.authenticate {
				token = createTestToken(secretKey)
			}

			method := http.MethodPost
			if tc.name == "Method Not Allowed (GET)" {
				method = http.MethodGet
			}

			resp := sendRequest(t, method, createURL, token, tc.requestBody)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusCreated {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				assert.NotNil(t, responseMap["template"])
			}
		})
	}
}

func TestTemplateHandler_HandleUpdate(t *testing.T) {
	workspaceID := "workspace123"
	templateID := "template1"
	validRequest := domain.UpdateTemplateRequest{
		WorkspaceID: workspaceID,
		ID:          templateID,
		Name:        "Updated Template",
		Channel:     "email",
		Category:    "transactional",
		Email:       createTestEmailTemplate(),
	}

	invalidRequestMissingName := validRequest
	invalidRequestMissingName.Name = "" // Missing required field

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockTemplateService)
		expectedStatus int
		expectBody     bool
		authenticate   bool
	}{
		{
			name:        "Success",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().UpdateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Not Found",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().UpdateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(&domain.ErrTemplateNotFound{Message: "not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().UpdateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Invalid Request Body (Bad JSON)",
			requestBody:    "this is not json",
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Validation Error (Missing Name)",
			requestBody:    invalidRequestMissingName,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
		{
			name:           "Method Not Allowed (GET)",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectBody:     false,
			authenticate:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			updateURL := fmt.Sprintf("%s/api/templates.update", serverURL)
			token := ""
			if tc.authenticate {
				token = createTestToken(secretKey)
			}

			method := http.MethodPost
			if tc.name == "Method Not Allowed (GET)" {
				method = http.MethodGet
			}

			resp := sendRequest(t, method, updateURL, token, tc.requestBody)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				assert.NotNil(t, responseMap["template"])
			}
		})
	}
}

func TestTemplateHandler_HandleDelete(t *testing.T) {
	workspaceID := "workspace123"
	templateID := "template1"
	validRequest := domain.DeleteTemplateRequest{
		WorkspaceID: workspaceID,
		ID:          templateID,
	}

	invalidRequestMissingID := validRequest
	invalidRequestMissingID.ID = "" // Missing required field

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockTemplateService)
		expectedStatus int
		expectBody     bool // Expect a specific {success: true} body
		authenticate   bool
	}{
		{
			name:        "Success",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().DeleteTemplate(gomock.Any(), workspaceID, templateID).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectBody:     true,
			authenticate:   true,
		},
		{
			name:        "Not Found",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().DeleteTemplate(gomock.Any(), workspaceID, templateID).Return(&domain.ErrTemplateNotFound{Message: "not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:        "Service Error",
			requestBody: validRequest,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().DeleteTemplate(gomock.Any(), workspaceID, templateID).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Invalid Request Body (Bad JSON)",
			requestBody:    "this is not json",
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Validation Error (Missing ID)",
			requestBody:    invalidRequestMissingID,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest,
			expectBody:     false,
			authenticate:   true,
		},
		{
			name:           "Unauthorized",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusUnauthorized,
			expectBody:     false,
			authenticate:   false,
		},
		{
			name:           "Method Not Allowed (GET)",
			requestBody:    validRequest,
			setupMock:      func(m *mocks.MockTemplateService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectBody:     false,
			authenticate:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
			defer cleanup()

			tc.setupMock(mockService)

			deleteURL := fmt.Sprintf("%s/api/templates.delete", serverURL)
			token := ""
			if tc.authenticate {
				token = createTestToken(secretKey)
			}

			method := http.MethodPost
			if tc.name == "Method Not Allowed (GET)" {
				method = http.MethodGet
			}

			resp := sendRequest(t, method, deleteURL, token, tc.requestBody)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectBody && resp.StatusCode == http.StatusOK {
				var responseMap map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&responseMap)
				require.NoError(t, err, "Failed to decode response body")
				success, ok := responseMap["success"].(bool)
				assert.True(t, ok && success, "Expected 'success' field to be true")
			}
		})
	}
}

// Helper function from email_blocks_test.go (or define similarly here)
func createTestRootBlockHandler(children ...notifusemjml.EmailBlock) notifusemjml.EmailBlock {
	base := notifusemjml.NewBaseBlock("root", notifusemjml.MJMLComponentMjml)
	base.Children = children
	base.Attributes["version"] = "4.0.0"
	return &notifusemjml.MJMLBlock{BaseBlock: base}
}

func createTestTextBlockHandler(id, textContent string) notifusemjml.EmailBlock {
	content := textContent
	base := notifusemjml.NewBaseBlock(id, notifusemjml.MJMLComponentMjText)
	base.Content = &content
	return &notifusemjml.MJTextBlock{BaseBlock: base}
}

func TestHandleCompile_ServiceError(t *testing.T) {
	mockService, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
	defer cleanup()

	// Prepare request that passes validation but service returns error
	payload := domain.CompileTemplateRequest{
		WorkspaceID:      "workspace123",
		MessageID:        "msg-1",
		VisualEditorTree: createTestRootBlockHandler(createTestTextBlockHandler("t1", "hi")),
		TemplateData:     map[string]interface{}{"contact": map[string]interface{}{"first_name": "John"}},
	}

	// We don't assert logger calls here to avoid tight coupling with log behavior
	mockService.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("compile error"))

	url := fmt.Sprintf("%s/api/templates.compile", serverURL)
	token := createTestToken(secretKey)
	resp := sendRequest(t, http.MethodPost, url, token, payload)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleCompile_MethodNotAllowed(t *testing.T) {
	_, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
	defer cleanup()

	url := fmt.Sprintf("%s/api/templates.compile", serverURL)
	token := createTestToken(secretKey)
	resp := sendRequest(t, http.MethodGet, url, token, nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// Note: Testing the auth middleware itself requires a different setup,
// these tests focus on the handler logic assuming auth succeeds (by adding context value manually)
// or testing scenarios where the handler rejects before auth (like wrong method).

// --- Commented out tests (can be restored/fixed later if auth handling changes) ---
// func TestHandleCompile_Success(t *testing.T) {
// 	// ... (Original test code)
// }
// func TestHandleCompile_BadRequest_InvalidJSON(t *testing.T) {
// 	// ... (Original test code)
// }
// func TestHandleCompile_BadRequest_ValidationError(t *testing.T) {
// 	// ... (Original test code)
// }
