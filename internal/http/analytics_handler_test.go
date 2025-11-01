package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"



	"github.com/Notifuse/notifuse/internal/domain/mocks"


	"github.com/Notifuse/notifuse/pkg/analytics"


	"github.com/Notifuse/notifuse/pkg/logger"


	"github.com/golang/mock/gomock"


	"github.com/stretchr/testify/assert"


	"github.com/stretchr/testify/require"
)

func TestNewAnalyticsHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockAnalyticsService(ctrl)
	mockLogger := logger.NewLogger()

	// Create a test public key
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	handler := NewAnalyticsHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	assert.NotNil(t, handler)
	assert.IsType(t, &AnalyticsHandler{}, handler)
}

func TestAnalyticsHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockAnalyticsService(ctrl)
	mockLogger := logger.NewLogger()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	handler := NewAnalyticsHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered by making test requests
	// Note: These will fail authentication but should reach the handler

	// Test analytics.query route
	req := httptest.NewRequest(http.MethodPost, "/api/analytics.query", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code) // Should fail auth, not 404

	// Test analytics.schemas route
	req = httptest.NewRequest(http.MethodPost, "/api/analytics.schemas", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code) // Should fail auth, not 404
}

func TestAnalyticsHandler_handleQuery(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		requestBody    interface{}
		setupMocks     func(*mocks.MockAnalyticsService)
		expectedStatus int
		expectedError  string
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name:   "successful query",
			method: http.MethodPost,
			requestBody: AnalyticsQueryRequest{
				WorkspaceID: "test-workspace",
				Query: analytics.Query{
					Schema:   "message_history",
					Measures: []string{"count"},
				},
			},
			setupMocks: func(mockService *mocks.MockAnalyticsService) {
				response := &analytics.Response{
					Data: []map[string]interface{}{
						{"count": 42},
					},
					Meta: analytics.Meta{
						Query:  "SELECT COUNT(*) AS count FROM message_history",
						Params: []interface{}{},
					},
				}
				mockService.EXPECT().Query(gomock.Any(), "test-workspace", gomock.Any()).
					Return(response, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Contains(t, response, "data")
				assert.Contains(t, response, "meta")

				data := response["data"].([]interface{})
				assert.Len(t, data, 1)

				firstRow := data[0].(map[string]interface{})
				assert.Equal(t, float64(42), firstRow["count"]) // JSON numbers are float64
			},
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMocks:     func(mockService *mocks.MockAnalyticsService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:   "invalid request body",
			method: http.MethodPost,
			requestBody: map[string]interface{}{
				"invalid": "data",
			},
			setupMocks:     func(mockService *mocks.MockAnalyticsService) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required", // The JSON parses but workspace_id is missing
		},
		{
			name:   "missing workspace_id",
			method: http.MethodPost,
			requestBody: AnalyticsQueryRequest{
				Query: analytics.Query{
					Schema:   "message_history",
					Measures: []string{"count"},
				},
			},
			setupMocks:     func(mockService *mocks.MockAnalyticsService) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:   "service error",
			method: http.MethodPost,
			requestBody: AnalyticsQueryRequest{
				WorkspaceID: "test-workspace",
				Query: analytics.Query{
					Schema:   "message_history",
					Measures: []string{"count"},
				},
			},
			setupMocks: func(mockService *mocks.MockAnalyticsService) {
				mockService.EXPECT().Query(gomock.Any(), "test-workspace", gomock.Any()).
					Return((*analytics.Response)(nil), assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Query failed:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := mocks.NewMockAnalyticsService(ctrl)
			mockLogger := logger.NewLogger()

			jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	var err error

			handler := NewAnalyticsHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

			// Setup mocks
			tt.setupMocks(mockService)

			// Create request
			var body []byte
			if tt.requestBody != nil {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(tt.method, "/api/analytics.query", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Execute request
			handler.handleQuery(w, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectedError != "" {
				assert.Contains(t, response, "error")
				assert.Contains(t, response, "message")
				assert.True(t, response["error"].(bool))
				assert.Contains(t, response["message"].(string), tt.expectedError)
			} else if tt.checkResponse != nil {
				tt.checkResponse(t, response)
			}

			// Expectations are automatically verified by gomock
		})
	}
}

func TestAnalyticsHandler_handleGetSchemas(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		requestBody    interface{}
		setupMocks     func(*mocks.MockAnalyticsService)
		expectedStatus int
		expectedError  string
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name:   "successful schema retrieval",
			method: http.MethodPost,
			requestBody: AnalyticsSchemasRequest{
				WorkspaceID: "test-workspace",
			},
			setupMocks: func(mockService *mocks.MockAnalyticsService) {
				schemas := map[string]analytics.SchemaDefinition{
					"message_history": {
						Name: "message_history",
						Measures: map[string]analytics.MeasureDefinition{
							"count": {
								Type:        "count",
								SQL:         "COUNT(*)",
								Description: "Total count",
							},
						},
						Dimensions: map[string]analytics.DimensionDefinition{
							"created_at": {
								Type:        "time",
								SQL:         "created_at",
								Description: "Creation time",
							},
						},
					},
				}
				mockService.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return(schemas, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Contains(t, response, "schemas")

				schemas := response["schemas"].(map[string]interface{})
				assert.Contains(t, schemas, "message_history")

				messageHistorySchema := schemas["message_history"].(map[string]interface{})
				assert.Equal(t, "message_history", messageHistorySchema["name"])
				assert.Contains(t, messageHistorySchema, "measures")
				assert.Contains(t, messageHistorySchema, "dimensions")
			},
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMocks:     func(mockService *mocks.MockAnalyticsService) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "missing workspace_id",
			method:         http.MethodPost,
			requestBody:    AnalyticsSchemasRequest{},
			setupMocks:     func(mockService *mocks.MockAnalyticsService) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:   "service error",
			method: http.MethodPost,
			requestBody: AnalyticsSchemasRequest{
				WorkspaceID: "test-workspace",
			},
			setupMocks: func(mockService *mocks.MockAnalyticsService) {
				mockService.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return((map[string]analytics.SchemaDefinition)(nil), assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Failed to get schemas:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := mocks.NewMockAnalyticsService(ctrl)
			mockLogger := logger.NewLogger()

			jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	var err error

			handler := NewAnalyticsHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

			// Setup mocks
			tt.setupMocks(mockService)

			// Create request
			var body []byte
			if tt.requestBody != nil {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(tt.method, "/api/analytics.schemas", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Execute request
			handler.handleGetSchemas(w, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectedError != "" {
				assert.Contains(t, response, "error")
				assert.Contains(t, response, "message")
				assert.True(t, response["error"].(bool))
				assert.Contains(t, response["message"].(string), tt.expectedError)
			} else if tt.checkResponse != nil {
				tt.checkResponse(t, response)
			}

			// Expectations are automatically verified by gomock
		})
	}
}

func TestAnalyticsHandler_writeJSONResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockAnalyticsService(ctrl)
	mockLogger := logger.NewLogger()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewAnalyticsHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	tests := []struct {
		name       string
		statusCode int
		data       interface{}
		expected   string
	}{
		{
			name:       "success response",
			statusCode: http.StatusOK,
			data:       map[string]interface{}{"success": true, "count": 42},
			expected:   `{"count":42,"success":true}`,
		},
		{
			name:       "error response",
			statusCode: http.StatusBadRequest,
			data:       map[string]interface{}{"error": "Bad request"},
			expected:   `{"error":"Bad request"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			handler.writeJSONResponse(w, tt.statusCode, tt.data)

			assert.Equal(t, tt.statusCode, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.expected, w.Body.String())
		})
	}
}

func TestAnalyticsHandler_writeErrorResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockAnalyticsService(ctrl)
	mockLogger := logger.NewLogger()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	var err error

	handler := NewAnalyticsHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	w := httptest.NewRecorder()
	handler.writeErrorResponse(w, http.StatusBadRequest, "Test error message")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response["error"].(bool))
	assert.Equal(t, "Test error message", response["message"].(string))
}

func TestAnalyticsHandler_Integration(t *testing.T) {
	// Test the full request flow with a complete request/response cycle
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockAnalyticsService(ctrl)
	mockLogger := logger.NewLogger()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	var err error

	handler := NewAnalyticsHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	// Setup service mock
	response := &analytics.Response{
		Data: []map[string]interface{}{
			{"channel": "email", "count": 100},
			{"channel": "sms", "count": 50},
		},
		Meta: analytics.Meta{
			Query:  "SELECT channel, COUNT(*) FROM message_history GROUP BY channel",
			Params: []interface{}{},
		},
	}
	mockService.EXPECT().Query(gomock.Any(), "test-workspace", gomock.Any()).
		Return(response, nil)

	// Create request
	requestBody := AnalyticsQueryRequest{
		WorkspaceID: "test-workspace",
		Query: analytics.Query{
			Schema:     "message_history",
			Measures:   []string{"count"},
			Dimensions: []string{"channel"},
		},
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/analytics.query", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute request
	handler.handleQuery(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var responseData map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &responseData)
	require.NoError(t, err)

	// Verify response structure
	assert.Contains(t, responseData, "data")
	assert.Contains(t, responseData, "meta")

	data := responseData["data"].([]interface{})
	assert.Len(t, data, 2)

	// Verify first row
	firstRow := data[0].(map[string]interface{})
	assert.Equal(t, "email", firstRow["channel"])
	assert.Equal(t, float64(100), firstRow["count"])

	// Verify second row
	secondRow := data[1].(map[string]interface{})
	assert.Equal(t, "sms", secondRow["channel"])
	assert.Equal(t, float64(50), secondRow["count"])

	// Verify meta
	meta := responseData["meta"].(map[string]interface{})
	assert.Contains(t, meta["query"].(string), "SELECT")
	assert.Contains(t, meta["query"].(string), "message_history")

	// Expectations are automatically verified by gomock
}
