package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupCustomEventHandlerTest prepares test dependencies and creates a custom event handler
func setupCustomEventHandlerTest(t *testing.T) (*mocks.MockCustomEventService, *pkgmocks.MockLogger, *CustomEventHandler) {
	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockService := mocks.NewMockCustomEventService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewCustomEventHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	return mockService, mockLogger, handler
}

func TestNewCustomEventHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockCustomEventService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	jwtSecret := []byte("test-secret")

	handler := NewCustomEventHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockService, handler.service)
	assert.Equal(t, mockLogger, handler.logger)
	assert.NotNil(t, handler.getJWTSecret)
}

func TestCustomEventHandler_RegisterRoutes(t *testing.T) {
	_, _, handler := setupCustomEventHandlerTest(t)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Check if routes were registered
	endpoints := []string{
		"/api/customEvents.upsert",
		"/api/customEvents.import",
		"/api/customEvents.get",
		"/api/customEvents.list",
	}

	for _, endpoint := range endpoints {
		h, _ := mux.Handler(&http.Request{URL: &url.URL{Path: endpoint}})
		if h == nil {
			t.Errorf("Expected handler to be registered for %s, but got nil", endpoint)
		}
	}
}

func TestCustomEventHandler_UpsertCustomEvent(t *testing.T) {
	now := time.Now()
	goalType := "purchase"
	goalValue := 99.99

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockCustomEventService)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "Success",
			requestBody: domain.UpsertCustomEventRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				EventName:   "purchase",
				ExternalID:  "order-123",
				Properties:  map[string]interface{}{"product": "widget"},
				GoalType:    &goalType,
				GoalValue:   &goalValue,
			},
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).Return(&domain.CustomEvent{
					ExternalID: "order-123",
					Email:      "test@example.com",
					EventName:  "purchase",
					Properties: map[string]interface{}{"product": "widget"},
					OccurredAt: now,
					GoalType:   &goalType,
					GoalValue:  &goalValue,
					CreatedAt:  now,
					UpdatedAt:  now,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["event"])
			},
		},
		{
			name:        "Invalid JSON",
			requestBody: "invalid json",
			setupMock: func(m *mocks.MockCustomEventService) {
				// No mock call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Service Error",
			requestBody: domain.UpsertCustomEventRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				EventName:   "purchase",
				ExternalID:  "order-123",
			},
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "Permission Error",
			requestBody: domain.UpsertCustomEventRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				EventName:   "purchase",
				ExternalID:  "order-123",
			},
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).Return(nil, &domain.PermissionError{Message: "access denied"})
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupCustomEventHandlerTest(t)

			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			var body []byte
			if str, ok := tc.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, _ = json.Marshal(tc.requestBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/customEvents.upsert", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.UpsertCustomEvent(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			if tc.checkResponse != nil {
				tc.checkResponse(t, rr)
			}
		})
	}
}

func TestCustomEventHandler_ImportCustomEvents(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mocks.MockCustomEventService)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "Success",
			requestBody: domain.ImportCustomEventsRequest{
				WorkspaceID: "workspace123",
				Events: []*domain.CustomEvent{
					{
						ExternalID: "event-1",
						Email:      "test1@example.com",
						EventName:  "signup",
						OccurredAt: now,
					},
					{
						ExternalID: "event-2",
						Email:      "test2@example.com",
						EventName:  "purchase",
						OccurredAt: now,
					},
				},
			},
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ImportEvents(gomock.Any(), gomock.Any()).Return([]string{"event-1", "event-2"}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["event_ids"])
				assert.Equal(t, float64(2), response["count"])
			},
		},
		{
			name:        "Invalid JSON",
			requestBody: "invalid json",
			setupMock: func(m *mocks.MockCustomEventService) {
				// No mock call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Service Error",
			requestBody: domain.ImportCustomEventsRequest{
				WorkspaceID: "workspace123",
				Events: []*domain.CustomEvent{
					{
						ExternalID: "event-1",
						Email:      "test1@example.com",
						EventName:  "signup",
						OccurredAt: now,
					},
				},
			},
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ImportEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "Permission Error",
			requestBody: domain.ImportCustomEventsRequest{
				WorkspaceID: "workspace123",
				Events: []*domain.CustomEvent{
					{
						ExternalID: "event-1",
						Email:      "test1@example.com",
						EventName:  "signup",
						OccurredAt: now,
					},
				},
			},
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ImportEvents(gomock.Any(), gomock.Any()).Return(nil, &domain.PermissionError{Message: "access denied"})
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupCustomEventHandlerTest(t)

			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			var body []byte
			if str, ok := tc.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, _ = json.Marshal(tc.requestBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/customEvents.import", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.ImportCustomEvents(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			if tc.checkResponse != nil {
				tc.checkResponse(t, rr)
			}
		})
	}
}

func TestCustomEventHandler_GetCustomEvent(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name           string
		queryParams    string
		setupMock      func(*mocks.MockCustomEventService)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:        "Success",
			queryParams: "workspace_id=workspace123&event_name=purchase&external_id=order-123",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().GetEvent(gomock.Any(), "workspace123", "purchase", "order-123").Return(&domain.CustomEvent{
					ExternalID: "order-123",
					Email:      "test@example.com",
					EventName:  "purchase",
					OccurredAt: now,
					CreatedAt:  now,
					UpdatedAt:  now,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["event"])
			},
		},
		{
			name:        "Missing Workspace ID",
			queryParams: "event_name=purchase&external_id=order-123",
			setupMock: func(m *mocks.MockCustomEventService) {
				// No mock call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Missing Event Name",
			queryParams: "workspace_id=workspace123&external_id=order-123",
			setupMock: func(m *mocks.MockCustomEventService) {
				// No mock call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Missing External ID",
			queryParams: "workspace_id=workspace123&event_name=purchase",
			setupMock: func(m *mocks.MockCustomEventService) {
				// No mock call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Not Found",
			queryParams: "workspace_id=workspace123&event_name=purchase&external_id=nonexistent",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().GetEvent(gomock.Any(), "workspace123", "purchase", "nonexistent").Return(nil, errors.New("not found"))
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Permission Error",
			queryParams: "workspace_id=workspace123&event_name=purchase&external_id=order-123",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().GetEvent(gomock.Any(), "workspace123", "purchase", "order-123").Return(nil, &domain.PermissionError{Message: "access denied"})
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupCustomEventHandlerTest(t)

			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/customEvents.get?"+tc.queryParams, nil)
			rr := httptest.NewRecorder()

			handler.GetCustomEvent(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			if tc.checkResponse != nil {
				tc.checkResponse(t, rr)
			}
		})
	}
}

func TestCustomEventHandler_ListCustomEvents(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name           string
		queryParams    string
		setupMock      func(*mocks.MockCustomEventService)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:        "Success with Email",
			queryParams: "workspace_id=workspace123&email=test@example.com",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ListEvents(gomock.Any(), &domain.ListCustomEventsRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
					Limit:       50,
					Offset:      0,
				}).Return([]*domain.CustomEvent{
					{
						ExternalID: "event-1",
						Email:      "test@example.com",
						EventName:  "purchase",
						OccurredAt: now,
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["events"])
				assert.Equal(t, float64(1), response["count"])
			},
		},
		{
			name:        "Success with Event Name",
			queryParams: "workspace_id=workspace123&event_name=purchase",
			setupMock: func(m *mocks.MockCustomEventService) {
				eventName := "purchase"
				m.EXPECT().ListEvents(gomock.Any(), &domain.ListCustomEventsRequest{
					WorkspaceID: "workspace123",
					Email:       "",
					EventName:   &eventName,
					Limit:       50,
					Offset:      0,
				}).Return([]*domain.CustomEvent{
					{
						ExternalID: "event-1",
						Email:      "test@example.com",
						EventName:  "purchase",
						OccurredAt: now,
					},
					{
						ExternalID: "event-2",
						Email:      "test2@example.com",
						EventName:  "purchase",
						OccurredAt: now,
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["events"])
				assert.Equal(t, float64(2), response["count"])
			},
		},
		{
			name:        "Success with Custom Limit and Offset",
			queryParams: "workspace_id=workspace123&email=test@example.com&limit=10&offset=5",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ListEvents(gomock.Any(), &domain.ListCustomEventsRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
					Limit:       10,
					Offset:      5,
				}).Return([]*domain.CustomEvent{}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, float64(0), response["count"])
			},
		},
		{
			name:        "Missing Workspace ID",
			queryParams: "email=test@example.com",
			setupMock: func(m *mocks.MockCustomEventService) {
				// No mock call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Missing Email and Event Name",
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockCustomEventService) {
				// No mock call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Service Error",
			queryParams: "workspace_id=workspace123&email=test@example.com",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ListEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Permission Error",
			queryParams: "workspace_id=workspace123&email=test@example.com",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ListEvents(gomock.Any(), gomock.Any()).Return(nil, &domain.PermissionError{Message: "access denied"})
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:        "Invalid Limit Parsing",
			queryParams: "workspace_id=workspace123&email=test@example.com&limit=invalid",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ListEvents(gomock.Any(), &domain.ListCustomEventsRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
					Limit:       50, // Default when parsing fails
					Offset:      0,
				}).Return([]*domain.CustomEvent{}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Invalid Offset Parsing",
			queryParams: "workspace_id=workspace123&email=test@example.com&offset=invalid",
			setupMock: func(m *mocks.MockCustomEventService) {
				m.EXPECT().ListEvents(gomock.Any(), &domain.ListCustomEventsRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
					Limit:       50,
					Offset:      0, // Default when parsing fails
				}).Return([]*domain.CustomEvent{}, nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupCustomEventHandlerTest(t)

			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/customEvents.list?"+tc.queryParams, nil)
			rr := httptest.NewRecorder()

			handler.ListCustomEvents(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			if tc.checkResponse != nil {
				tc.checkResponse(t, rr)
			}
		})
	}
}

// A GET carrying the ordinary JSON body used to perform the write.
//
// The mux matches on path alone, and neither handler checked the verb, so `GET
// /api/customEvents.upsert` decoded the body and upserted — including the soft-delete path —
// while `GET /api/customEvents.import` bulk-imported. Every other write in this package
// guards; these two were a plain omission that nothing could see.
//
// internal/http/verb_guard_test.go closes the class by walking the parse tree. This is the
// half that proves the guard actually answers rather than merely existing.
func TestCustomEventHandler_WritesRefuseEveryMethodButPOST(t *testing.T) {
	routes := []struct {
		name    string
		handler func(*CustomEventHandler) func(http.ResponseWriter, *http.Request)
	}{
		{"customEvents.upsert", func(h *CustomEventHandler) func(http.ResponseWriter, *http.Request) {
			return h.UpsertCustomEvent
		}},
		{"customEvents.import", func(h *CustomEventHandler) func(http.ResponseWriter, *http.Request) {
			return h.ImportCustomEvents
		}},
	}

	for _, route := range routes {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			t.Run(route.name+" refuses "+method, func(t *testing.T) {
				// No service expectation at all: the guard must run before anything is
				// decoded, so a write that reached the service would fail the mock too.
				service, _, handler := setupCustomEventHandlerTest(t)
				service.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).Times(0)
				service.EXPECT().ImportEvents(gomock.Any(), gomock.Any()).Times(0)

				body, err := json.Marshal(domain.UpsertCustomEventRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
					EventName:   "pwned",
					ExternalID:  "order-123",
				})
				require.NoError(t, err)

				req := httptest.NewRequest(method, "/api/"+route.name, bytes.NewReader(body))
				rec := httptest.NewRecorder()
				route.handler(handler)(rec, req)

				assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
			})
		}
	}
}
