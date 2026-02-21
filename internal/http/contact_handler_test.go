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

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
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
