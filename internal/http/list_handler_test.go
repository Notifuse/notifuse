package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

// Test setup helper
func setupListHandlerTest(t *testing.T) (*mocks.MockListService, *pkgmocks.MockLogger, *ListHandler) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockListService(ctrl)
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
	handler := NewListHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	return mockService, mockLogger, handler
}

func TestListHandler_RegisterRoutes(t *testing.T) {
	_, _, handler := setupListHandlerTest(t)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Check if routes were registered - indirect test by ensuring no panic
	endpoints := []string{
		"/api/lists.list",
		"/api/lists.get",
		"/api/lists.create",
		"/api/lists.update",
		"/api/lists.delete",
	}

	for _, endpoint := range endpoints {
		// This is a basic check - just ensure the handler exists
		h, _ := mux.Handler(&http.Request{URL: &url.URL{Path: endpoint}})
		if h == nil {
			t.Errorf("Expected handler to be registered for %s, but got nil", endpoint)
		}
	}
}

func TestListHandler_HandleList(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		queryParams    url.Values
		setupMock      func(*mocks.MockListService)
		expectedStatus int
		expectedLists  bool
	}{
		{
			name:   "Get Lists Success",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().GetLists(gomock.Any(), "workspace123").Return([]*domain.List{
					{
						ID:          "list1",
						Name:        "Test List 1",
						Description: "Test Description 1",
					},
					{
						ID:          "list2",
						Name:        "Test List 2",
						Description: "Test Description 2",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedLists:  true,
		},
		{
			name:   "Get Lists Service Error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().GetLists(gomock.Any(), "workspace123").Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedLists:  false,
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodPost,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedLists:  false,
		},
		{
			name:        "Missing Workspace ID",
			method:      http.MethodGet,
			queryParams: url.Values{},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusBadRequest,
			expectedLists:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupListHandlerTest(t)
			tc.setupMock(mockService)

			req := httptest.NewRequest(tc.method, "/api/lists.list?"+tc.queryParams.Encode(), nil)
			rr := httptest.NewRecorder()

			handler.handleList(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Contains(t, response, "lists")
				if tc.expectedLists {
					lists, ok := response["lists"].([]interface{})
					assert.True(t, ok)
					assert.NotEmpty(t, lists)
				}
			}
		})
	}
}

func TestListHandler_HandleGet(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		queryParams    url.Values
		setupMock      func(*mocks.MockListService)
		expectedStatus int
		expectedList   bool
	}{
		{
			name:   "Get List Success",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"list1"},
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().GetListByID(gomock.Any(), "workspace123", "list1").Return(&domain.List{
					ID:          "list1",
					Name:        "Test List",
					Description: "Test Description",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedList:   true,
		},
		{
			name:   "Get List Not Found",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"nonexistent"},
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().GetListByID(gomock.Any(), "workspace123", "nonexistent").Return(nil, &domain.ErrListNotFound{Message: "list not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectedList:   false,
		},
		{
			name:   "Get List Service Error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"list1"},
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().GetListByID(gomock.Any(), "workspace123", "list1").Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedList:   false,
		},
		{
			name:   "Missing List ID",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusBadRequest,
			expectedList:   false,
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodPost,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"list1"},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedList:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupListHandlerTest(t)
			tc.setupMock(mockService)

			req := httptest.NewRequest(tc.method, "/api/lists.get?"+tc.queryParams.Encode(), nil)
			rr := httptest.NewRecorder()

			handler.handleGet(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Contains(t, response, "list")
			}
		})
	}
}

func TestListHandler_HandleCreate(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockListService)
		expectedStatus int
		checkCreated   func(*testing.T, *mocks.MockListService)
	}{
		{
			name:   "Create List Success",
			method: http.MethodPost,
			reqBody: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1",
				Name:          "New List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "New Description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			setupMock: func(m *mocks.MockListService) {
				list := &domain.List{
					ID:            "list1",
					Name:          "New List",
					IsDoubleOptin: true,
					IsPublic:      true,
					Description:   "New Description",
					DoubleOptInTemplate: &domain.TemplateReference{
						ID:      "template123",
						Version: 1,
					},
				}
				m.EXPECT().CreateList(gomock.Any(), "workspace123", list).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			checkCreated: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:   "Create List Service Error",
			method: http.MethodPost,
			reqBody: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1",
				Name:          "New List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "New Description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			setupMock: func(m *mocks.MockListService) {
				list := &domain.List{
					ID:            "list1",
					Name:          "New List",
					IsDoubleOptin: true,
					IsPublic:      true,
					Description:   "New Description",
					DoubleOptInTemplate: &domain.TemplateReference{
						ID:      "template123",
						Version: 1,
					},
				}
				m.EXPECT().CreateList(gomock.Any(), "workspace123", list).Return(errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkCreated: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:    "Invalid Request Body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusBadRequest,
			checkCreated: func(t *testing.T, m *mocks.MockListService) {
				// No expectations needed
			},
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodGet,
			reqBody: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1",
				Name:          "New List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "New Description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusMethodNotAllowed,
			checkCreated: func(t *testing.T, m *mocks.MockListService) {
				// No expectations needed
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupListHandlerTest(t)
			tc.setupMock(mockService)

			var reqBody bytes.Buffer
			if tc.reqBody != nil {
				if err := json.NewEncoder(&reqBody).Encode(tc.reqBody); err != nil {
					t.Fatalf("Failed to encode request body: %v", err)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/lists.create", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleCreate(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusCreated {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Contains(t, response, "list")
			}

			tc.checkCreated(t, mockService)
		})
	}
}

func TestListHandler_HandleUpdate(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockListService)
		expectedStatus int
		checkUpdated   func(*testing.T, *mocks.MockListService)
	}{
		{
			name:   "Update List Success",
			method: http.MethodPost,
			reqBody: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1",
				Name:          "Updated List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Updated Description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			setupMock: func(m *mocks.MockListService) {
				list := &domain.List{
					ID:            "list1",
					Name:          "Updated List",
					IsDoubleOptin: true,
					IsPublic:      true,
					Description:   "Updated Description",
					DoubleOptInTemplate: &domain.TemplateReference{
						ID:      "template123",
						Version: 1,
					},
				}
				m.EXPECT().UpdateList(gomock.Any(), "workspace123", list).Return(nil)
			},
			expectedStatus: http.StatusOK,
			checkUpdated: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:   "Update List Not Found",
			method: http.MethodPost,
			reqBody: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "nonexistent",
				Name:          "Updated List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Updated Description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			setupMock: func(m *mocks.MockListService) {
				list := &domain.List{
					ID:            "nonexistent",
					Name:          "Updated List",
					IsDoubleOptin: true,
					IsPublic:      true,
					Description:   "Updated Description",
					DoubleOptInTemplate: &domain.TemplateReference{
						ID:      "template123",
						Version: 1,
					},
				}
				m.EXPECT().UpdateList(gomock.Any(), "workspace123", list).Return(&domain.ErrListNotFound{Message: "list not found"})
			},
			expectedStatus: http.StatusNotFound,
			checkUpdated: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:   "Update List Service Error",
			method: http.MethodPost,
			reqBody: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1",
				Name:          "Updated List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Updated Description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			setupMock: func(m *mocks.MockListService) {
				list := &domain.List{
					ID:            "list1",
					Name:          "Updated List",
					IsDoubleOptin: true,
					IsPublic:      true,
					Description:   "Updated Description",
					DoubleOptInTemplate: &domain.TemplateReference{
						ID:      "template123",
						Version: 1,
					},
				}
				m.EXPECT().UpdateList(gomock.Any(), "workspace123", list).Return(errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkUpdated: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:    "Invalid Request Body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusBadRequest,
			checkUpdated: func(t *testing.T, m *mocks.MockListService) {
				// No expectations needed
			},
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodGet,
			reqBody: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1",
				Name:          "Updated List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Updated Description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusMethodNotAllowed,
			checkUpdated: func(t *testing.T, m *mocks.MockListService) {
				// No expectations needed
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupListHandlerTest(t)
			tc.setupMock(mockService)

			var reqBody bytes.Buffer
			if tc.reqBody != nil {
				if err := json.NewEncoder(&reqBody).Encode(tc.reqBody); err != nil {
					t.Fatalf("Failed to encode request body: %v", err)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/lists.update", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleUpdate(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Contains(t, response, "list")
			}

			tc.checkUpdated(t, mockService)
		})
	}
}

func TestListHandler_HandleDelete(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockListService)
		expectedStatus int
		checkDeleted   func(*testing.T, *mocks.MockListService)
	}{
		{
			name:   "Delete List Success",
			method: http.MethodPost,
			reqBody: domain.DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "list1",
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().DeleteList(gomock.Any(), "workspace123", "list1").Return(nil)
			},
			expectedStatus: http.StatusOK,
			checkDeleted: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:   "Delete List Not Found",
			method: http.MethodPost,
			reqBody: domain.DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "nonexistent",
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().DeleteList(gomock.Any(), "workspace123", "nonexistent").Return(&domain.ErrListNotFound{Message: "list not found"})
			},
			expectedStatus: http.StatusNotFound,
			checkDeleted: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:   "Delete List Service Error",
			method: http.MethodPost,
			reqBody: domain.DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "list1",
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().DeleteList(gomock.Any(), "workspace123", "list1").Return(errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkDeleted: func(t *testing.T, m *mocks.MockListService) {
				// Expectations are handled by gomock
			},
		},
		{
			name:    "Invalid Request Body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusBadRequest,
			checkDeleted: func(t *testing.T, m *mocks.MockListService) {
				// No expectations needed
			},
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodGet,
			reqBody: domain.DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "list1",
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusMethodNotAllowed,
			checkDeleted: func(t *testing.T, m *mocks.MockListService) {
				// No expectations needed
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupListHandlerTest(t)
			tc.setupMock(mockService)

			var reqBody bytes.Buffer
			if tc.reqBody != nil {
				if err := json.NewEncoder(&reqBody).Encode(tc.reqBody); err != nil {
					t.Fatalf("Failed to encode request body: %v", err)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/lists.delete", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleDelete(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.True(t, response["success"].(bool))
			}

			tc.checkDeleted(t, mockService)
		})
	}
}

func TestListHandler_HandleStats(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		queryParams    url.Values
		setupMock      func(*mocks.MockListService)
		expectedStatus int
		expectedStats  bool
	}{
		{
			name:   "Get List Stats Success",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"list_id":      []string{"list1"},
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().GetListStats(gomock.Any(), "workspace123", "list1").Return(&domain.ListStats{
					TotalActive:       10,
					TotalPending:      5,
					TotalUnsubscribed: 3,
					TotalBounced:      1,
					TotalComplained:   0,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedStats:  true,
		},
		{
			name:   "Get List Stats Service Error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"list_id":      []string{"list1"},
			},
			setupMock: func(m *mocks.MockListService) {
				m.EXPECT().GetListStats(gomock.Any(), "workspace123", "list1").Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedStats:  false,
		},
		{
			name:   "Missing List ID",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusBadRequest,
			expectedStats:  false,
		},
		{
			name:   "Missing Workspace ID",
			method: http.MethodGet,
			queryParams: url.Values{
				"list_id": []string{"list1"},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusBadRequest,
			expectedStats:  false,
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodPost,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"list_id":      []string{"list1"},
			},
			setupMock: func(m *mocks.MockListService) {
				// No setup needed
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedStats:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupListHandlerTest(t)
			tc.setupMock(mockService)

			req := httptest.NewRequest(tc.method, "/api/lists.stats?"+tc.queryParams.Encode(), nil)
			rr := httptest.NewRecorder()

			handler.handleStats(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Contains(t, response, "list_id")
				assert.Contains(t, response, "stats")

				if tc.expectedStats {
					stats, ok := response["stats"].(map[string]interface{})
					assert.True(t, ok)
					assert.Contains(t, stats, "total_active")
					assert.Contains(t, stats, "total_pending")
					assert.Contains(t, stats, "total_unsubscribed")
					assert.Contains(t, stats, "total_bounced")
					assert.Contains(t, stats, "total_complained")
				}
			}
		})
	}
}

func TestListHandler_HandleSubscribe(t *testing.T) {
	mockService, _, handler := setupListHandlerTest(t)

	t.Run("Success", func(t *testing.T) {
		req := domain.SubscribeToListsRequest{
			WorkspaceID: "workspace123",
			Contact:     domain.Contact{Email: "user@example.com"},
			ListIDs:     []string{"list1"},
		}
		mockService.EXPECT().SubscribeToLists(gomock.Any(), &req, true).Return(nil)

		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/lists.subscribe", &buf)
		rr := httptest.NewRecorder()
		handler.handleSubscribe(rr, httpReq)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		assert.True(t, resp["success"].(bool))
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodPost, "/api/lists.subscribe", bytes.NewBufferString("{invalid"))
		rr := httptest.NewRecorder()
		handler.handleSubscribe(rr, httpReq)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("ValidationError", func(t *testing.T) {
		req := map[string]interface{}{
			"workspace_id": "workspace123",
			// missing email/list_ids
		}
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/lists.subscribe", &buf)
		rr := httptest.NewRecorder()
		handler.handleSubscribe(rr, httpReq)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("ServiceError", func(t *testing.T) {
		req := domain.SubscribeToListsRequest{
			WorkspaceID: "workspace123",
			Contact:     domain.Contact{Email: "user@example.com"},
			ListIDs:     []string{"list1"},
		}
		mockService.EXPECT().SubscribeToLists(gomock.Any(), &req, true).Return(errors.New("svc error"))

		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/lists.subscribe", &buf)
		rr := httptest.NewRecorder()
		handler.handleSubscribe(rr, httpReq)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/lists.subscribe", nil)
		rr := httptest.NewRecorder()
		handler.handleSubscribe(rr, httpReq)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}
