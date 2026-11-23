package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusUnsubscribed, false).Return(&domain.UpdateContactListStatusResult{
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
				m.EXPECT().UpdateContactListStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
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
				m.EXPECT().UpdateContactListStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
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
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusUnsubscribed, false).Return(nil, errors.New("service error"))
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

// The resubscribe flag as an API client sends it, in a raw body: present, absent, and
// with a status it does not apply to; and the opt-out refusal as a 409 that names the
// stored status.
func TestContactListHandler_HandleUpdateStatus_Resubscribe(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		setupMock      func(*mocks.MockContactListService)
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name: "resubscribe true reaches the service",
			body: `{"workspace_id":"workspace123","email":"test@example.com","list_id":"list123","status":"active","resubscribe":true}`,
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusActive, true).
					Return(&domain.UpdateContactListStatusResult{Success: true, Message: "contact resubscribed", Found: true}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "no resubscribe key means false",
			body: `{"workspace_id":"workspace123","email":"test@example.com","list_id":"list123","status":"active"}`,
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusActive, false).
					Return(nil, &domain.ErrContactListOptedOut{Status: domain.ContactListStatusUnsubscribed})
			},
			expectedStatus: http.StatusConflict,
			expectedBody: map[string]interface{}{
				"status": "unsubscribed",
				"error":  "the contact unsubscribed from this list: set resubscribe to true to subscribe them again",
			},
		},
		{
			name: "a wrapped opt-out refusal is still a 409",
			body: `{"workspace_id":"workspace123","email":"test@example.com","list_id":"list123","status":"pending"}`,
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusPending, false).
					Return(nil, fmt.Errorf("update: %w", &domain.ErrContactListOptedOut{Status: domain.ContactListStatusComplained}))
			},
			expectedStatus: http.StatusConflict,
			expectedBody:   map[string]interface{}{"status": "complained"},
		},
		{
			name: "resubscribe with a status other than active is refused before the service",
			body: `{"workspace_id":"workspace123","email":"test@example.com","list_id":"list123","status":"pending","resubscribe":true}`,
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tt.setupMock(mockService)

			req := httptest.NewRequest(http.MethodPost, "/api/contactLists.updateStatus", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.handleUpdateStatus(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != nil {
				var body map[string]interface{}
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
				for key, want := range tt.expectedBody {
					assert.Equal(t, want, body[key], key)
				}
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

// TestContactListHandler_PermissionDenied covers every contactLists route: a
// permission denial from the service must answer 403 with the missing grant named
// in the body, not the generic 500 each handler falls back to. The service wraps
// its errors on the way up, so the denial is wrapped here too.
func TestContactListHandler_PermissionDenied(t *testing.T) {
	denial := func(resource domain.PermissionResource, permission domain.PermissionType) error {
		return fmt.Errorf("failed to authenticate user: %w",
			domain.NewPermissionError(resource, permission, "Insufficient permissions"))
	}

	tests := []struct {
		name               string
		setupMock          func(*mocks.MockContactListService)
		serve              func(*ContactListHandler, http.ResponseWriter)
		expectedResource   domain.PermissionResource
		expectedPermission domain.PermissionType
	}{
		{
			name: "GetByIDs",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactListByIDs(gomock.Any(), "workspace123", "test@example.com", "list123").
					Return(nil, denial(domain.PermissionResourceLists, domain.PermissionTypeRead))
			},
			serve: func(h *ContactListHandler, w http.ResponseWriter) {
				req := httptest.NewRequest(http.MethodGet, "/api/contactLists.getByIDs?workspace_id=workspace123&email=test@example.com&list_id=list123", nil)
				h.handleGetByIDs(w, req)
			},
			expectedResource:   domain.PermissionResourceLists,
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name: "GetContactsByList",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetContactsByListID(gomock.Any(), "workspace123", "list123").
					Return(nil, denial(domain.PermissionResourceContacts, domain.PermissionTypeRead))
			},
			serve: func(h *ContactListHandler, w http.ResponseWriter) {
				req := httptest.NewRequest(http.MethodGet, "/api/contactLists.getContactsByList?workspace_id=workspace123&list_id=list123", nil)
				h.handleGetContactsByList(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name: "GetListsByContact",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().GetListsByEmail(gomock.Any(), "workspace123", "test@example.com").
					Return(nil, denial(domain.PermissionResourceContacts, domain.PermissionTypeRead))
			},
			serve: func(h *ContactListHandler, w http.ResponseWriter) {
				req := httptest.NewRequest(http.MethodGet, "/api/contactLists.getListsByContact?workspace_id=workspace123&email=test@example.com", nil)
				h.handleGetListsByContact(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name: "UpdateStatus",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "workspace123", "test@example.com", "list123", domain.ContactListStatusUnsubscribed, false).
					Return(nil, denial(domain.PermissionResourceLists, domain.PermissionTypeWrite))
			},
			serve: func(h *ContactListHandler, w http.ResponseWriter) {
				body, _ := json.Marshal(domain.UpdateContactListStatusRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
					ListID:      "list123",
					Status:      "unsubscribed",
				})
				req := httptest.NewRequest(http.MethodPost, "/api/contactLists.updateStatus", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				h.handleUpdateStatus(w, req)
			},
			expectedResource:   domain.PermissionResourceLists,
			expectedPermission: domain.PermissionTypeWrite,
		},
		{
			name: "RemoveContact",
			setupMock: func(m *mocks.MockContactListService) {
				m.EXPECT().RemoveContactFromList(gomock.Any(), "workspace123", "test@example.com", "list123").
					Return(denial(domain.PermissionResourceContacts, domain.PermissionTypeWrite))
			},
			serve: func(h *ContactListHandler, w http.ResponseWriter) {
				body, _ := json.Marshal(domain.RemoveContactFromListRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
					ListID:      "list123",
				})
				req := httptest.NewRequest(http.MethodPost, "/api/contactLists.removeContact", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				h.handleRemoveContact(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeWrite,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tt.setupMock(mockService)

			rr := httptest.NewRecorder()
			tt.serve(handler, rr)

			assert.Equal(t, http.StatusForbidden, rr.Code)

			var response map[string]interface{}
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
			assert.Equal(t, string(tt.expectedResource), response["resource"])
			assert.Equal(t, string(tt.expectedPermission), response["permission"])
		})
	}
}

// contactLists.updateStatus is audited only when the service marks a re-subscription:
// a malformed request, a wrong method or a plain status change leaves no entry.
func TestContactListHandler_HandleUpdateStatus_AuditOnlyResubscriptions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		method   string
		body     string
		mock     func(*mocks.MockContactListService)
		audited  bool
		wantCode int
	}{
		{name: "malformed body", method: http.MethodPost, body: `{"workspace_id":"ws","email":"a@example.com","status":"active","resubscribe":true}`,
			mock: func(m *mocks.MockContactListService) {}, wantCode: http.StatusBadRequest},
		{name: "wrong method", method: http.MethodGet, body: `{}`,
			mock: func(m *mocks.MockContactListService) {}, wantCode: http.StatusMethodNotAllowed},
		{name: "plain status change", method: http.MethodPost, body: `{"workspace_id":"ws","email":"a@example.com","list_id":"l","status":"unsubscribed"}`,
			mock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "ws", "a@example.com", "l", domain.ContactListStatusUnsubscribed, false).
					Return(&domain.UpdateContactListStatusResult{Success: true, Found: true}, nil)
			}, wantCode: http.StatusOK},
		{name: "re-subscription", method: http.MethodPost, body: `{"workspace_id":"ws","email":"a@example.com","list_id":"l","status":"active","resubscribe":true}`,
			mock: func(m *mocks.MockContactListService) {
				m.EXPECT().UpdateContactListStatus(gomock.Any(), "ws", "a@example.com", "l", domain.ContactListStatusActive, true).
					DoAndReturn(func(ctx context.Context, _, _, _ string, _ domain.ContactListStatus, _ bool) (*domain.UpdateContactListStatusResult, error) {
						domain.AuditFromContext(ctx).AddMetadata(domain.AuditResubscribedListIDs, []string{"l"})
						return &domain.UpdateContactListStatusResult{Success: true, Found: true}, nil
					})
			}, audited: true, wantCode: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactListHandlerTest(t)
			tc.mock(mockService)

			record := domain.NewAuditRecord("", "", "")
			req := httptest.NewRequest(tc.method, "/api/contactLists.updateStatus", strings.NewReader(tc.body))
			req = req.WithContext(domain.WithAuditRecord(req.Context(), record))
			rr := httptest.NewRecorder()
			handler.handleUpdateStatus(rr, req)

			require.Equal(t, tc.wantCode, rr.Code)
			_, skipped := record.Finalize(rr.Code)
			assert.Equal(t, !tc.audited, skipped)
		})
	}
}
