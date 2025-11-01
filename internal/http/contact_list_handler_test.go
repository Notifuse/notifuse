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

func setupContactListHandlerTest(t *testing.T) (*mocks.MockContactListService, *pkgmocks.MockLogger, *ContactListHandler) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockContactListService(ctrl)
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
	handler := NewContactListHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	return mockService, mockLogger, handler
}

func TestContactListHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockContactListService(ctrl)
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
	handler := NewContactListHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Check if routes were registered
	endpoints := []string{
		"/api/contactLists.addContact",
		"/api/contactLists.getByIDs",
		"/api/contactLists.getContactsByList",
		"/api/contactLists.getListsByContact",
		"/api/contactLists.updateStatus",
		"/api/contactLists.removeContact",
	}

	for _, endpoint := range endpoints {
		h, _ := mux.Handler(&http.Request{URL: &url.URL{Path: endpoint}})
		if h == nil {
			t.Errorf("Expected handler to be registered for %s, but got nil", endpoint)
		}
	}
}

func TestContactListHandler_HandleGetByIDs(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		queryParams    string
		setupMock      func(*mocks.MockContactListService)
		expectedStatus int
	}{
		{
			name:        "Success",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&email=test@example.com&list_id=list123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactListByIDs(gomock.Any(), "workspace123", "test@example.com", "list123").Return(&domain.ContactList{
					Email:  "test@example.com",
					ListID: "list123",
					Status: domain.ContactListStatusActive,
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Missing Required Parameters",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Method Not Allowed",
			method:      http.MethodPost,
			queryParams: "workspace_id=workspace123&email=test@example.com&list_id=list123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:        "Service Error",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&email=test@example.com&list_id=list123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactListByIDs(gomock.Any(), "workspace123", "test@example.com", "list123").Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tt.setupMock(mockService)

			req := httptest.NewRequest(tt.method, "/api/contactLists.getByIDs?"+tt.queryParams, nil)
			rr := httptest.NewRecorder()
			handler.handleGetByIDs(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["contact_list"])
			}
		})
	}
}

func TestContactListHandler_HandleGetContactsByList(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		queryParams    string
		setupMock      func(*mocks.MockContactListService)
		expectedStatus int
	}{
		{
			name:        "Success",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&list_id=list123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactsByListID(gomock.Any(), "workspace123", "list123").Return([]*domain.ContactList{
					{
						Email:  "test1@example.com",
						ListID: "list123",
						Status: domain.ContactListStatusActive,
					},
					{
						Email:  "test2@example.com",
						ListID: "list123",
						Status: domain.ContactListStatusActive,
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Missing Required Parameters",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactsByListID(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Method Not Allowed",
			method:      http.MethodPost,
			queryParams: "workspace_id=workspace123&list_id=list123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactsByListID(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:        "Service Error",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&list_id=list123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactsByListID(gomock.Any(), "workspace123", "list123").Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tt.setupMock(mockService)

			req := httptest.NewRequest(tt.method, "/api/contactLists.getContactsByList?"+tt.queryParams, nil)
			rr := httptest.NewRecorder()
			handler.handleGetContactsByList(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["contact_lists"])
			}
		})
	}
}

func TestContactListHandler_HandleGetListsByContact(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		queryParams    string
		setupMock      func(*mocks.MockContactListService)
		expectedStatus int
	}{
		{
			name:        "Success",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&email=test@example.com",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetListsByEmail(gomock.Any(), "workspace123", "test@example.com").Return([]*domain.ContactList{
					{
						Email:  "test@example.com",
						ListID: "list1",
						Status: domain.ContactListStatusActive,
					},
					{
						Email:  "test@example.com",
						ListID: "list2",
						Status: domain.ContactListStatusActive,
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Missing Required Parameters",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetListsByEmail(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Method Not Allowed",
			method:      http.MethodPost,
			queryParams: "workspace_id=workspace123&email=test@example.com",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetListsByEmail(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:        "Service Error",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&email=test@example.com",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetListsByEmail(gomock.Any(), "workspace123", "test@example.com").Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tt.setupMock(mockService)

			req := httptest.NewRequest(tt.method, "/api/contactLists.getListsByContact?"+tt.queryParams, nil)
			rr := httptest.NewRecorder()
			handler.handleGetListsByContact(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response["contact_lists"])
			}
		})
	}
}

func TestContactListHandler_HandleUpdateStatus(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockContactListService)
		expectedStatus int
	}{
		{
			name:   "Success",
			method: http.MethodPost,
			reqBody: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
				Status:      "unsubscribed",
			},
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusUnsubscribed).Return(&domain.UpdateContactListStatusResult{
					Success: true,
					Message: "status updated successfully",
					Found:   true,
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "Invalid Request Body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodGet,
			reqBody: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
				Status:      "unsubscribed",
			},
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:   "Service Error",
			method: http.MethodPost,
			reqBody: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
				Status:      "unsubscribed",
			},
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusUnsubscribed).Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tt.setupMock(mockService)

			var reqBody bytes.Buffer
			if tt.reqBody != nil {
				if err := json.NewEncoder(&reqBody).Encode(tt.reqBody); err != nil {
					t.Fatalf("Failed to encode request body: %v", err)
				}
			}

			req := httptest.NewRequest(tt.method, "/api/contactLists.updateStatus", &reqBody)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.handleUpdateStatus(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response domain.UpdateContactListStatusResult
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
				assert.NotEmpty(t, response.Message)
			}
		})
	}
}

func TestContactListHandler_HandleRemoveContact(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockContactListService)
		expectedStatus int
	}{
		{
			name:   "Success",
			method: http.MethodPost,
			reqBody: domain.RemoveContactFromListRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
			},
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().RemoveContactFromList(gomock.Any(), "workspace123", "test@example.com", "list123").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "Invalid Request Body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().RemoveContactFromList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodGet,
			reqBody: domain.RemoveContactFromListRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
			},
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().RemoveContactFromList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:   "Service Error",
			method: http.MethodPost,
			reqBody: domain.RemoveContactFromListRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
			},
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().RemoveContactFromList(gomock.Any(), "workspace123", "test@example.com", "list123").Return(errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tt.setupMock(mockService)

			var reqBody bytes.Buffer
			if tt.reqBody != nil {
				if err := json.NewEncoder(&reqBody).Encode(tt.reqBody); err != nil {
					t.Fatalf("Failed to encode request body: %v", err)
				}
			}

			req := httptest.NewRequest(tt.method, "/api/contactLists.removeContact", &reqBody)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.handleRemoveContact(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.True(t, response["success"].(bool))
			}
		})
	}
}
