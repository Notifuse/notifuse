package http

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
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupContactHandlerTest prepares test dependencies and creates a contact handler
func setupContactHandlerTest(t *testing.T) (*mocks.MockContactService, *pkgmocks.MockLogger, *ContactHandler) {
	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockService := mocks.NewMockContactService(ctrl)
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
	handler := NewContactHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	return mockService, mockLogger, handler
}

func TestContactHandler_RegisterRoutes(t *testing.T) {
	_, _, handler := setupContactHandlerTest(t)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Check if routes were registered - indirect test by ensuring no panic
	endpoints := []string{
		"/api/contacts.list",
		"/api/contacts.count",
		"/api/contacts.get",
		"/api/contacts.getByEmail",
		"/api/contacts.getByExternalID",
		"/api/contacts.delete",
		"/api/contacts.import",
		"/api/contacts.upsert",
	}

	for _, endpoint := range endpoints {
		// This is a basic check - just ensure the handler exists
		h, _ := mux.Handler(&http.Request{URL: &url.URL{Path: endpoint}})
		if h == nil {
			t.Errorf("Expected handler to be registered for %s, but got nil", endpoint)
		}
	}
}

func TestContactHandler_HandleList(t *testing.T) {
	testCases := []struct {
		name             string
		method           string
		queryParams      string
		setupMock        func(*mocks.MockContactService)
		expectedStatus   int
		expectedContacts bool
	}{
		{
			name:        "Get Contacts Success",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&limit=2",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), &domain.GetContactsRequest{
					WorkspaceID: "workspace123",
					Limit:       2,
				}).Return(&domain.GetContactsResponse{
					Contacts: []*domain.Contact{
						{
							Email:      "test1@example.com",
							ExternalID: &domain.NullableString{String: "ext1", IsNull: false},
							Timezone:   &domain.NullableString{String: "UTC", IsNull: false},
						},
					},
				}, nil)
			},
			expectedStatus:   http.StatusOK,
			expectedContacts: true,
		},
		{
			name:        "Get Contacts Service Error",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123&limit=2",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), &domain.GetContactsRequest{
					WorkspaceID: "workspace123",
					Limit:       2,
				}).Return(nil, errors.New("service error"))
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedContacts: false,
		},
		{
			name:        "Get Contacts Success Without Limit (default 20)",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), &domain.GetContactsRequest{
					WorkspaceID: "workspace123",
					Limit:       20,
				}).Return(&domain.GetContactsResponse{
					Contacts: []*domain.Contact{
						{
							Email:      "test1@example.com",
							ExternalID: &domain.NullableString{String: "ext1", IsNull: false},
							Timezone:   &domain.NullableString{String: "UTC", IsNull: false},
						},
					},
				}, nil)
			},
			expectedStatus:   http.StatusOK,
			expectedContacts: true,
		},
		{
			name:        "Method Not Allowed",
			method:      http.MethodPost,
			queryParams: "workspace_id=workspace123&limit=2",
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus:   http.StatusMethodNotAllowed,
			expectedContacts: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)

			// Setup mock expectations
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/contacts.list?"+tc.queryParams, nil)

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call handler
			handler.handleList(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// If we expect contacts, check the response body
			if tc.expectedContacts {
				var response domain.GetContactsResponse
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotEmpty(t, response.Contacts)
			}
		})
	}
}

func TestContactHandler_HandleCount(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		queryParams    string
		setupMock      func(*mocks.MockContactService)
		expectedStatus int
		expectedCount  int
	}{
		{
			name:        "Count Contacts Success",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().CountContacts(gomock.Any(), "workspace123").Return(42, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  42,
		},
		{
			name:        "Count Contacts Service Error",
			method:      http.MethodGet,
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().CountContacts(gomock.Any(), "workspace123").Return(0, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCount:  0,
		},
		{
			name:        "Missing Workspace ID",
			method:      http.MethodGet,
			queryParams: "",
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus: http.StatusBadRequest,
			expectedCount:  0,
		},
		{
			name:        "Method Not Allowed",
			method:      http.MethodPost,
			queryParams: "workspace_id=workspace123",
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedCount:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)

			// Setup mock expectations
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/contacts.count?"+tc.queryParams, nil)

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call handler
			handler.handleCount(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// If success, check the response body
			if tc.expectedStatus == http.StatusOK {
				var response map[string]int
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedCount, response["total_contacts"])
			}
		})
	}
}

func TestContactHandler_HandleGet(t *testing.T) {
	testCases := []struct {
		name            string
		method          string
		contactEmail    string
		contact         *domain.Contact
		err             error
		expectedStatus  int
		expectedContact bool
	}{
		{
			name:         "Get_Contact_Success",
			method:       "GET",
			contactEmail: "test1@example.com",
			contact: &domain.Contact{
				Email:     "test1@example.com",
				FirstName: &domain.NullableString{String: "Test", IsNull: false},
				LastName:  &domain.NullableString{String: "User", IsNull: false},
			},
			err:             nil,
			expectedStatus:  http.StatusOK,
			expectedContact: true,
		},
		{
			name:            "Get_Contact_Not_Found",
			method:          "GET",
			contactEmail:    "nonexistent@example.com",
			contact:         nil,
			err:             fmt.Errorf("contact not found"),
			expectedStatus:  http.StatusNotFound,
			expectedContact: false,
		},
		{
			name:            "Get_Contact_Service_Error",
			method:          "GET",
			contactEmail:    "test1@example.com",
			contact:         nil,
			err:             errors.New("service error"),
			expectedStatus:  http.StatusInternalServerError,
			expectedContact: false,
		},
		{
			name:            "Missing_Contact_Email",
			method:          "GET",
			contactEmail:    "",
			contact:         nil,
			err:             nil,
			expectedStatus:  http.StatusBadRequest,
			expectedContact: false,
		},
		{
			name:            "Method_Not_Allowed",
			method:          "POST",
			contactEmail:    "test1@example.com",
			contact:         nil,
			err:             nil,
			expectedStatus:  http.StatusMethodNotAllowed,
			expectedContact: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)

			// Set up mock expectations only for test cases that should call the service
			if tc.method == http.MethodGet && tc.contactEmail != "" {
				mockService.EXPECT().
					GetContactByEmail(gomock.Any(), "workspace123", tc.contactEmail).
					Return(tc.contact, tc.err)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/contacts.get?workspace_id=workspace123&email="+tc.contactEmail, nil)

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call handler
			handler.handleGetByEmail(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// If we expect a contact, check the response body
			if tc.expectedContact {
				var response struct {
					Contact *domain.Contact `json:"contact"`
				}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response.Contact)
				assert.Equal(t, tc.contactEmail, response.Contact.Email)
			}
		})
	}
}

func TestContactHandler_HandleGetByExternalID(t *testing.T) {
	testCases := []struct {
		name            string
		method          string
		externalID      string
		setupMock       func(*mocks.MockContactService)
		expectedStatus  int
		expectedContact bool
	}{
		{
			name:       "Get Contact By External ID Success",
			method:     http.MethodGet,
			externalID: "ext1",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					GetContactByExternalID(gomock.Any(), "workspace123", "ext1").
					Return(&domain.Contact{
						Email: "test@example.com",
						ExternalID: &domain.NullableString{
							String: "ext1",
							IsNull: false,
						},
						Timezone: &domain.NullableString{
							String: "UTC",
							IsNull: false,
						},
					}, nil)
			},
			expectedStatus:  http.StatusOK,
			expectedContact: true,
		},
		{
			name:       "Get Contact By External ID Not Found",
			method:     http.MethodGet,
			externalID: "nonexistent",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					GetContactByExternalID(gomock.Any(), "workspace123", "nonexistent").
					Return(nil, fmt.Errorf("contact not found"))
			},
			expectedStatus:  http.StatusNotFound,
			expectedContact: false,
		},
		{
			name:       "Get Contact By External ID Service Error",
			method:     http.MethodGet,
			externalID: "error",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					GetContactByExternalID(gomock.Any(), "workspace123", "error").
					Return(nil, errors.New("service error"))
			},
			expectedStatus:  http.StatusInternalServerError,
			expectedContact: false,
		},
		{
			name:       "Missing External ID",
			method:     http.MethodGet,
			externalID: "",
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus:  http.StatusBadRequest,
			expectedContact: false,
		},
		{
			name:       "Method Not Allowed",
			method:     http.MethodPost,
			externalID: "ext1",
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus:  http.StatusMethodNotAllowed,
			expectedContact: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)

			// Setup mock expectations
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/contacts.getByExternalID?workspace_id=workspace123&external_id="+tc.externalID, nil)

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call handler
			handler.handleGetByExternalID(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// If we expect a contact, check the response body
			if tc.expectedContact {
				var response struct {
					Contact *domain.Contact `json:"contact"`
				}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotNil(t, response.Contact)
				assert.Equal(t, tc.externalID, response.Contact.ExternalID.String)
			}
		})
	}
}

func TestContactHandler_HandleDelete(t *testing.T) {
	testCases := []struct {
		name            string
		method          string
		reqBody         interface{}
		setupMock       func(*mocks.MockContactService)
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:   "Delete Contact Success",
			method: http.MethodPost,
			reqBody: domain.DeleteContactRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().DeleteContact(gomock.Any(), "workspace123", "test@example.com").Return(nil)
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "Contact deleted successfully",
		},
		{
			name:   "Delete Contact Not Found",
			method: http.MethodPost,
			reqBody: domain.DeleteContactRequest{
				WorkspaceID: "workspace123",
				Email:       "nonexistent@example.com",
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().DeleteContact(gomock.Any(), "workspace123", "nonexistent@example.com").Return(fmt.Errorf("contact not found"))
			},
			expectedStatus:  http.StatusNotFound,
			expectedMessage: "Contact not found",
		},
		{
			name:   "Delete Contact Service Error",
			method: http.MethodPost,
			reqBody: domain.DeleteContactRequest{
				WorkspaceID: "workspace123",
				Email:       "error@example.com",
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().DeleteContact(gomock.Any(), "workspace123", "error@example.com").Return(errors.New("service error"))
			},
			expectedStatus:  http.StatusInternalServerError,
			expectedMessage: "Failed to delete contact",
		},
		{
			name:    "Invalid Request Body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "Invalid request body",
		},
		{
			name:   "Missing Email in Request",
			method: http.MethodPost,
			reqBody: domain.DeleteContactRequest{
				WorkspaceID: "workspace123",
				Email:       "",
			},
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "email is required",
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodGet,
			reqBody: domain.DeleteContactRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
			},
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed for this test
			},
			expectedStatus:  http.StatusMethodNotAllowed,
			expectedMessage: "Method not allowed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)

			// Setup mock expectations
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			var reqBody bytes.Buffer
			if tc.reqBody != nil {
				if err := json.NewEncoder(&reqBody).Encode(tc.reqBody); err != nil {
					t.Fatalf("Failed to encode request body: %v", err)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/contacts.delete", &reqBody)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.handleDelete(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// Check response body
			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.True(t, response["success"].(bool))
			} else {
				var response map[string]string
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedMessage, response["error"])
			}
		})
	}
}

func TestContactHandler_HandleImport(t *testing.T) {
	testCases := []struct {
		name            string
		method          string
		reqBody         interface{}
		setupMock       func(*mocks.MockContactService)
		expectedStatus  int
		expectedMessage string
		expectedCount   int
	}{
		{
			name:   "successful_batch_import",
			method: http.MethodPost,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contacts": []map[string]interface{}{
					{
						"email":       "contact1@example.com",
						"external_id": "ext1",
						"timezone":    "UTC",
					},
				},
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					BatchImportContacts(gomock.Any(), "workspace123", gomock.Any(), gomock.Any()).
					Return(&domain.BatchImportContactsResponse{
						Operations: []*domain.UpsertContactOperation{
							{
								Email:  "contact1@example.com",
								Action: domain.UpsertContactOperationCreate,
							},
						},
					})
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "contact1@example.com",
			expectedCount:   1,
		},
		{
			name:   "service error",
			method: http.MethodPost,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contacts": []map[string]interface{}{
					{
						"email":       "contact1@example.com",
						"external_id": "ext1",
						"timezone":    "UTC",
					},
				},
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					BatchImportContacts(gomock.Any(), "workspace123", gomock.Any(), gomock.Any()).
					Return(&domain.BatchImportContactsResponse{
						Error: "service error",
					})
			},
			expectedStatus:  http.StatusInternalServerError,
			expectedMessage: "service error",
			expectedCount:   0,
		},
		{
			name:   "invalid request - empty contacts",
			method: http.MethodPost,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contacts":     []map[string]interface{}{},
			},
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "contacts array is empty",
			expectedCount:   0,
		},
		{
			name:   "method not allowed",
			method: http.MethodGet,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contacts": []map[string]interface{}{
					{
						"email":       "contact1@example.com",
						"external_id": "ext1",
						"timezone":    "UTC",
					},
				},
			},
			setupMock: func(m *mocks.MockContactService) {
				// No setup needed
			},
			expectedStatus:  http.StatusMethodNotAllowed,
			expectedMessage: "Method not allowed",
			expectedCount:   0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)

			// Setup mock expectations
			if tc.setupMock != nil {
				tc.setupMock(mockService)
			}

			var reqBody bytes.Buffer
			if tc.reqBody != nil {
				if err := json.NewEncoder(&reqBody).Encode(tc.reqBody); err != nil {
					t.Fatalf("Failed to encode request body: %v", err)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/contacts.import", &reqBody)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.handleImport(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response domain.BatchImportContactsResponse
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotEmpty(t, response.Operations)
				assert.Equal(t, tc.expectedCount, len(response.Operations))
				assert.Equal(t, tc.expectedMessage, response.Operations[0].Email)
				assert.Equal(t, domain.UpsertContactOperationCreate, response.Operations[0].Action)
			}
		})
	}
}

func TestContactHandler_HandleUpsert(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockContactService)
		expectedStatus int
		expectedAction string
	}{
		{
			name:   "Create Contact Without UUID",
			method: http.MethodPost,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contact": map[string]interface{}{
					"external_id": "new-ext",
					"email":       "new@example.com",
					"first_name":  "John",
					"last_name":   "Doe",
					"timezone":    "UTC",
				},
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
					Return(domain.UpsertContactOperation{
						Email:  "new@example.com",
						Action: domain.UpsertContactOperationCreate,
					})
			},
			expectedStatus: http.StatusOK,
			expectedAction: domain.UpsertContactOperationCreate,
		},
		{
			name:   "Create Contact With Email",
			method: http.MethodPost,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contact": map[string]interface{}{
					"external_id": "new-ext",
					"email":       "new@example.com",
					"first_name":  "John",
					"last_name":   "Doe",
					"timezone":    "UTC",
				},
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
					Return(domain.UpsertContactOperation{
						Email:  "new@example.com",
						Action: domain.UpsertContactOperationCreate,
					})
			},
			expectedStatus: http.StatusOK,
			expectedAction: domain.UpsertContactOperationCreate,
		},
		{
			name:   "Update Existing Contact",
			method: http.MethodPost,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contact": map[string]interface{}{
					"external_id": "updated-ext",
					"email":       "old@example.com",
					"timezone":    "UTC",
				},
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
					Return(domain.UpsertContactOperation{
						Email:  "old@example.com",
						Action: domain.UpsertContactOperationUpdate,
					})
			},
			expectedStatus: http.StatusOK,
			expectedAction: domain.UpsertContactOperationUpdate,
		},
		{
			name:    "Invalid Request Body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().UpsertContact(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusBadRequest,
			expectedAction: "",
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodGet,
			reqBody: map[string]interface{}{
				"external_id": "updated-ext",
				"email":       "updated@example.com",
				"timezone":    "UTC",
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().UpsertContact(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedAction: "",
		},
		{
			name:   "Service Error on Upsert",
			method: http.MethodPost,
			reqBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"contact": map[string]interface{}{
					"external_id": "ext1",
					"email":       "test@example.com",
					"timezone":    "UTC",
				},
			},
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().
					UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
					Return(domain.UpsertContactOperation{
						Email:  "test@example.com",
						Action: domain.UpsertContactOperationError,
						Error:  "service error",
					})
			},
			expectedStatus: http.StatusBadRequest,
			expectedAction: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)
			tc.setupMock(mockService)

			var reqBody bytes.Buffer
			if tc.reqBody != nil {
				// If it's a string, just use it directly
				if str, ok := tc.reqBody.(string); ok {
					reqBody = *bytes.NewBufferString(str)
				} else {
					// Otherwise encode as JSON
					if err := json.NewEncoder(&reqBody).Encode(tc.reqBody); err != nil {
						t.Fatalf("Failed to encode request body: %v", err)
					}
				}
			}

			req := httptest.NewRequest(tc.method, "/api/contacts.upsert", &reqBody)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("Failed to parse form: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.handleUpsert(rr, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// Check response body for success cases
			if tc.expectedStatus == http.StatusOK {
				var response domain.UpsertContactOperation
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedAction, response.Action)
			}
		})
	}
}

// TestContactHandler_PermissionDenied covers every contacts route: a scoped key
// refused on the contacts resource must get a 403 naming the missing grant, not
// the opaque 500 (or, on upsert, the 400) each handler otherwise falls back to.
// The methods returning an error carry a wrapped denial, because the service
// wraps on its way up; import and upsert report through their result struct, so
// the typed denial travels on its Err field instead.
func TestContactHandler_PermissionDenied(t *testing.T) {
	denial := func(resource domain.PermissionResource, permission domain.PermissionType) *domain.PermissionError {
		return domain.NewPermissionError(resource, permission, "Insufficient permissions")
	}
	wrapped := func(resource domain.PermissionResource, permission domain.PermissionType) error {
		return fmt.Errorf("failed to authenticate user: %w", denial(resource, permission))
	}

	tests := []struct {
		name               string
		setupMock          func(*mocks.MockContactService)
		serve              func(*ContactHandler, http.ResponseWriter)
		expectedResource   domain.PermissionResource
		expectedPermission domain.PermissionType
	}{
		{
			name: "List",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
					Return(nil, wrapped(domain.PermissionResourceContacts, domain.PermissionTypeRead))
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				req := httptest.NewRequest(http.MethodGet, "/api/contacts.list?workspace_id=workspace123", nil)
				h.handleList(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name: "Count",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().CountContacts(gomock.Any(), "workspace123").
					Return(0, wrapped(domain.PermissionResourceContacts, domain.PermissionTypeRead))
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				req := httptest.NewRequest(http.MethodGet, "/api/contacts.count?workspace_id=workspace123", nil)
				h.handleCount(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name: "GetByEmail",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContactByEmail(gomock.Any(), "workspace123", "test@example.com").
					Return(nil, wrapped(domain.PermissionResourceContacts, domain.PermissionTypeRead))
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				req := httptest.NewRequest(http.MethodGet, "/api/contacts.getByEmail?workspace_id=workspace123&email=test@example.com", nil)
				h.handleGetByEmail(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name: "GetByExternalID",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContactByExternalID(gomock.Any(), "workspace123", "ext1").
					Return(nil, wrapped(domain.PermissionResourceContacts, domain.PermissionTypeRead))
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				req := httptest.NewRequest(http.MethodGet, "/api/contacts.getByExternalID?workspace_id=workspace123&external_id=ext1", nil)
				h.handleGetByExternalID(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeRead,
		},
		{
			name: "Delete",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().DeleteContact(gomock.Any(), "workspace123", "test@example.com").
					Return(wrapped(domain.PermissionResourceContacts, domain.PermissionTypeWrite))
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				body, _ := json.Marshal(domain.DeleteContactRequest{
					WorkspaceID: "workspace123",
					Email:       "test@example.com",
				})
				req := httptest.NewRequest(http.MethodPost, "/api/contacts.delete", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				h.handleDelete(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeWrite,
		},
		{
			name: "Import",
			setupMock: func(m *mocks.MockContactService) {
				permErr := denial(domain.PermissionResourceContacts, domain.PermissionTypeWrite)
				m.EXPECT().BatchImportContacts(gomock.Any(), "workspace123", gomock.Any(), gomock.Any()).
					Return(&domain.BatchImportContactsResponse{Error: permErr.Error(), Err: permErr})
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				body, _ := json.Marshal(map[string]interface{}{
					"workspace_id": "workspace123",
					"contacts": []map[string]interface{}{
						{"email": "contact1@example.com"},
					},
				})
				req := httptest.NewRequest(http.MethodPost, "/api/contacts.import", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				h.handleImport(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeWrite,
		},
		{
			name: "ImportToLists",
			setupMock: func(m *mocks.MockContactService) {
				permErr := denial(domain.PermissionResourceLists, domain.PermissionTypeWrite)
				m.EXPECT().BatchImportContacts(gomock.Any(), "workspace123", gomock.Any(), []string{"list123"}).
					Return(&domain.BatchImportContactsResponse{Error: permErr.Error(), Err: permErr})
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				body, _ := json.Marshal(map[string]interface{}{
					"workspace_id": "workspace123",
					"contacts": []map[string]interface{}{
						{"email": "contact1@example.com"},
					},
					"subscribe_to_lists": []string{"list123"},
				})
				req := httptest.NewRequest(http.MethodPost, "/api/contacts.import", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				h.handleImport(w, req)
			},
			expectedResource:   domain.PermissionResourceLists,
			expectedPermission: domain.PermissionTypeWrite,
		},
		{
			name: "Upsert",
			setupMock: func(m *mocks.MockContactService) {
				permErr := denial(domain.PermissionResourceContacts, domain.PermissionTypeWrite)
				m.EXPECT().UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
					Return(domain.UpsertContactOperation{
						Email:  "contact1@example.com",
						Action: domain.UpsertContactOperationError,
						Error:  permErr.Error(),
						Err:    permErr,
					})
			},
			serve: func(h *ContactHandler, w http.ResponseWriter) {
				body, _ := json.Marshal(map[string]interface{}{
					"workspace_id": "workspace123",
					"contact":      map[string]interface{}{"email": "contact1@example.com"},
				})
				req := httptest.NewRequest(http.MethodPost, "/api/contacts.upsert", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				h.handleUpsert(w, req)
			},
			expectedResource:   domain.PermissionResourceContacts,
			expectedPermission: domain.PermissionTypeWrite,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)
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

// TestContactHandler_NonPermissionErrorKeepsItsStatus pins the other half of the
// change: only a permission denial is rerouted. A genuine per-contact failure
// still reports through the response string with the status it had before.
func TestContactHandler_NonPermissionErrorKeepsItsStatus(t *testing.T) {
	mockService, _, handler := setupContactHandlerTest(t)
	mockService.EXPECT().UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
		Return(domain.UpsertContactOperation{
			Email:  "contact1@example.com",
			Action: domain.UpsertContactOperationError,
			Error:  "invalid contact: email is required",
		})

	body, _ := json.Marshal(map[string]interface{}{
		"workspace_id": "workspace123",
		"contact":      map[string]interface{}{"email": "contact1@example.com"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/contacts.upsert", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleUpsert(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var response map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, "invalid contact: email is required", response["error"])
}

// TestContactHandler_HandleUpsert_ReturnsStoredContact covers the field an
// integration maps its next step from. The contact in the response is the stored
// row, which is not the one the request described: the merge and the database
// between them decide the external_id, the custom fields and the timestamps.
func TestContactHandler_HandleUpsert_ReturnsStoredContact(t *testing.T) {
	storedAt := time.Date(2026, 8, 24, 10, 30, 0, 0, time.UTC)

	testCases := []struct {
		name   string
		action string
		stored *domain.Contact
	}{
		{
			name:   "create",
			action: domain.UpsertContactOperationCreate,
			stored: &domain.Contact{
				Email:       "new@example.com",
				ExternalID:  &domain.NullableString{String: "crm-42"},
				Timezone:    &domain.NullableString{String: "Europe/Paris"},
				DBCreatedAt: storedAt,
				DBUpdatedAt: storedAt,
			},
		},
		{
			name:   "update",
			action: domain.UpsertContactOperationUpdate,
			stored: &domain.Contact{
				Email: "existing@example.com",
				// Set by an earlier write and untouched by this request: only the
				// stored row can report it.
				FirstName:   &domain.NullableString{String: "Ada"},
				ExternalID:  &domain.NullableString{String: "crm-7"},
				DBCreatedAt: storedAt.Add(-24 * time.Hour),
				DBUpdatedAt: storedAt,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)
			mockService.EXPECT().UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
				Return(domain.UpsertContactOperation{
					Email:   tc.stored.Email,
					Action:  tc.action,
					Contact: tc.stored,
				})

			body := []byte(`{
				"workspace_id": "workspace123",
				"contact": {"email": "` + tc.stored.Email + `"}
			}`)
			req := httptest.NewRequest(http.MethodPost, "/api/contacts.upsert", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.handleUpsert(rr, req)
			require.Equal(t, http.StatusOK, rr.Code)

			var response domain.UpsertContactOperation
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
			assert.Equal(t, tc.action, response.Action)
			require.NotNil(t, response.Contact)
			assert.Equal(t, tc.stored.Email, response.Contact.Email)
			require.NotNil(t, response.Contact.ExternalID)
			assert.Equal(t, tc.stored.ExternalID.String, response.Contact.ExternalID.String)
		})
	}
}

// TestContactHandler_HandleUpsert_OldClientShape pins the additive half: a client
// written against the previous response reads the same fields, and a response
// without a read-back contact carries no contact key at all rather than a null.
func TestContactHandler_HandleUpsert_OldClientShape(t *testing.T) {
	t.Run("existing keys are untouched", func(t *testing.T) {
		mockService, _, handler := setupContactHandlerTest(t)
		mockService.EXPECT().UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
			Return(domain.UpsertContactOperation{
				Email:   "new@example.com",
				Action:  domain.UpsertContactOperationCreate,
				Contact: &domain.Contact{Email: "new@example.com"},
			})

		body := []byte(`{"workspace_id":"workspace123","contact":{"email":"new@example.com"}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/contacts.upsert", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.handleUpsert(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		// The shape a client that predates the contact field declares.
		var legacy struct {
			Email  string `json:"email"`
			Action string `json:"action"`
			Error  string `json:"error,omitempty"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &legacy))
		assert.Equal(t, "new@example.com", legacy.Email)
		assert.Equal(t, domain.UpsertContactOperationCreate, legacy.Action)
		assert.Empty(t, legacy.Error)
	})

	t.Run("no read-back leaves the key absent", func(t *testing.T) {
		mockService, _, handler := setupContactHandlerTest(t)
		mockService.EXPECT().UpsertContact(gomock.Any(), "workspace123", gomock.Any()).
			Return(domain.UpsertContactOperation{
				Email:  "new@example.com",
				Action: domain.UpsertContactOperationCreate,
			})

		body := []byte(`{"workspace_id":"workspace123","contact":{"email":"new@example.com"}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/contacts.upsert", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.handleUpsert(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
		_, present := response["contact"]
		assert.False(t, present)
	})
}

// TestContactHandler_HandleList_ErrorsAreJSON pins that every exit from
// contacts.list is JSON. It used to answer plain text through http.Error, which an
// API client parsing the body has no way to read: it gets a status it can act on
// and a body that fails to decode.
func TestContactHandler_HandleList_ErrorsAreJSON(t *testing.T) {
	testCases := []struct {
		name           string
		setupMock      func(*mocks.MockContactService)
		request        func() *http.Request
		expectedStatus int
	}{
		{
			name: "method not allowed",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), gomock.Any()).Times(0)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/contacts.list", nil)
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name: "invalid request parameters",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), gomock.Any()).Times(0)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/contacts.list?workspace_id=workspace123&limit=not-a-number", nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "validation failure",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), gomock.Any()).Times(0)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/contacts.list", nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "service failure",
			setupMock: func(m *mocks.MockContactService) {
				m.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database unreachable"))
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/contacts.list?workspace_id=workspace123", nil)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupContactHandlerTest(t)
			tc.setupMock(mockService)

			rr := httptest.NewRecorder()
			handler.handleList(rr, tc.request())

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			var response map[string]interface{}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
			assert.NotEmpty(t, response["error"])
		})
	}
}

// TestContactHandler_AuthenticationFailureStatus pins the status codes an
// integration reads to decide what to do next. contacts.upsert and contacts.import
// both report a refusal inside their response struct rather than through an error
// return, so the typed error rides on Err and only Err can be matched — the string
// in Error carries no type.
//
// The 401 is the one that matters most: an integration platform prompts its user to
// reconnect on 401 and gives up on anything else, so a revoked key answering with
// the handler's catch-all status stopped the automation with a generic failure and
// never surfaced the reconnect. The wrapped errors mirror how the service reports
// them, which also pins that the mapping still matches through the wrap.
func TestContactHandler_AuthenticationFailureStatus(t *testing.T) {
	const workspaceID = "workspace123"

	authErrors := []struct {
		name            string
		err             error
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "revoked api key",
			err:             fmt.Errorf("api key has been revoked: %w", domain.ErrAPIKeyRevoked),
			expectedStatus:  http.StatusUnauthorized,
			expectedMessage: "API key has been revoked",
		},
		{
			name:            "not a member",
			err:             fmt.Errorf("failed to get user workspace: %w", domain.ErrUserNotInWorkspace),
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "You do not have access to this workspace",
		},
		{
			name:            "unknown workspace",
			err:             fmt.Errorf("failed to get workspace: %w", &domain.ErrWorkspaceNotFound{WorkspaceID: workspaceID}),
			expectedStatus:  http.StatusNotFound,
			expectedMessage: "Workspace not found",
		},
	}

	endpoints := []struct {
		name      string
		body      string
		setupMock func(m *mocks.MockContactService, authErr error)
		serve     func(h *ContactHandler, w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "contacts.upsert",
			body: `{"workspace_id":"workspace123","contact":{"email":"contact1@example.com"}}`,
			setupMock: func(m *mocks.MockContactService, authErr error) {
				m.EXPECT().UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
					Return(domain.UpsertContactOperation{
						Email:  "contact1@example.com",
						Action: domain.UpsertContactOperationError,
						Error:  authErr.Error(),
						Err:    authErr,
					})
			},
			serve: func(h *ContactHandler, w http.ResponseWriter, r *http.Request) { h.handleUpsert(w, r) },
		},
		{
			name: "contacts.import",
			body: `{"workspace_id":"workspace123","contacts":[{"email":"contact1@example.com"}]}`,
			setupMock: func(m *mocks.MockContactService, authErr error) {
				m.EXPECT().BatchImportContacts(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
					Return(&domain.BatchImportContactsResponse{
						Error: fmt.Sprintf("failed to authenticate user: %v", authErr),
						Err:   authErr,
					})
			},
			serve: func(h *ContactHandler, w http.ResponseWriter, r *http.Request) { h.handleImport(w, r) },
		},
	}

	for _, endpoint := range endpoints {
		for _, authErr := range authErrors {
			t.Run(endpoint.name+"/"+authErr.name, func(t *testing.T) {
				mockService, _, handler := setupContactHandlerTest(t)
				endpoint.setupMock(mockService, authErr.err)

				req := httptest.NewRequest(http.MethodPost, "/api/"+endpoint.name, bytes.NewReader([]byte(endpoint.body)))
				req.Header.Set("Content-Type", "application/json")

				rr := httptest.NewRecorder()
				endpoint.serve(handler, rr, req)

				assert.Equal(t, authErr.expectedStatus, rr.Code)

				var response map[string]interface{}
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.Equal(t, authErr.expectedMessage, response["error"])
			})
		}
	}
}
