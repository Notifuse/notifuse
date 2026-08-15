package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAutomationTest(t *testing.T) (*AutomationHandler, *mocks.MockAutomationService, *http.ServeMux, []byte) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	automationSvc := mocks.NewMockAutomationService(ctrl)
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	handler := NewAutomationHandler(automationSvc, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return handler, automationSvc, mux, jwtSecret
}

func createTestAutomation(id, workspaceID string) *domain.Automation {
	now := time.Now().UTC()
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Test Automation",
		Status:      domain.AutomationStatusDraft,
		ListID:      "list-123",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "email.opened",
			Frequency: domain.TriggerFrequencyOnce,
		},
		RootNodeID: "node-root",
		Stats:      &domain.AutomationStats{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func TestAutomationHandler_Create(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful create", func(t *testing.T) {
		automation := createTestAutomation("auto-123", "workspace-123")

		automationSvc.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).Return(nil)

		reqBody := domain.CreateAutomationRequest{
			WorkspaceID: "workspace-123",
			Automation:  automation,
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.create", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("validation error", func(t *testing.T) {
		reqBody := domain.CreateAutomationRequest{
			WorkspaceID: "",
			Automation:  nil,
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.create", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		automation := createTestAutomation("auto-123", "workspace-123")

		automationSvc.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).Return(errors.New("service error"))

		reqBody := domain.CreateAutomationRequest{
			WorkspaceID: "workspace-123",
			Automation:  automation,
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.create", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/automations.create", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestAutomationHandler_Get(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful get", func(t *testing.T) {
		expectedAutomation := createTestAutomation("auto-123", "workspace-123")

		automationSvc.EXPECT().Get(gomock.Any(), "workspace-123", "auto-123").Return(expectedAutomation, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/automations.get?workspace_id=workspace-123&automation_id=auto-123", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Automation *domain.Automation `json:"automation"`
		}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, expectedAutomation.ID, response.Automation.ID)
	})

	t.Run("validation error - missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/automations.get?automation_id=auto-123", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		automationSvc.EXPECT().Get(gomock.Any(), "workspace-123", "nonexistent").Return(nil, errors.New("not found"))

		req := httptest.NewRequest(http.MethodGet, "/api/automations.get?workspace_id=workspace-123&automation_id=nonexistent", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAutomationHandler_List(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful list", func(t *testing.T) {
		expectedAutomations := []*domain.Automation{
			createTestAutomation("auto-1", "workspace-123"),
			createTestAutomation("auto-2", "workspace-123"),
		}

		automationSvc.EXPECT().List(gomock.Any(), "workspace-123", gomock.Any()).Return(expectedAutomations, 2, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/automations.list?workspace_id=workspace-123", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Automations []*domain.Automation `json:"automations"`
			Total       int                  `json:"total"`
		}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Len(t, response.Automations, 2)
		assert.Equal(t, 2, response.Total)
	})

	t.Run("validation error - missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/automations.list", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAutomationHandler_Update(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful update", func(t *testing.T) {
		automation := createTestAutomation("auto-123", "workspace-123")
		automation.Name = "Updated Automation"

		automationSvc.EXPECT().Update(gomock.Any(), "workspace-123", gomock.Any()).Return(nil)

		reqBody := domain.UpdateAutomationRequest{
			WorkspaceID: "workspace-123",
			Automation:  automation,
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.update", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("validation error", func(t *testing.T) {
		reqBody := domain.UpdateAutomationRequest{
			WorkspaceID: "",
			Automation:  nil,
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.update", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAutomationHandler_Delete(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful delete", func(t *testing.T) {
		automationSvc.EXPECT().Delete(gomock.Any(), "workspace-123", "auto-123").Return(nil)

		reqBody := domain.DeleteAutomationRequest{
			WorkspaceID:  "workspace-123",
			AutomationID: "auto-123",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.delete", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("cannot delete live automation", func(t *testing.T) {
		automationSvc.EXPECT().Delete(gomock.Any(), "workspace-123", "auto-123").Return(errors.New("cannot delete live automation"))

		reqBody := domain.DeleteAutomationRequest{
			WorkspaceID:  "workspace-123",
			AutomationID: "auto-123",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.delete", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAutomationHandler_Activate(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful activate", func(t *testing.T) {
		automationSvc.EXPECT().Activate(gomock.Any(), "workspace-123", "auto-123").Return(nil)

		reqBody := domain.ActivateAutomationRequest{
			WorkspaceID:  "workspace-123",
			AutomationID: "auto-123",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.activate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("already active error", func(t *testing.T) {
		automationSvc.EXPECT().Activate(gomock.Any(), "workspace-123", "auto-123").Return(errors.New("already live"))

		reqBody := domain.ActivateAutomationRequest{
			WorkspaceID:  "workspace-123",
			AutomationID: "auto-123",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.activate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// decodeErrorMessage reads the {"error": "..."} body written by WriteJSONError.
func decodeErrorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	return response.Error
}

func TestAutomationHandler_Activate_ErrorMapping(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	const conditionMessage = "invalid trigger conditions: pq: column \"c.countryy\" does not exist"

	testCases := []struct {
		name            string
		serviceErr      error
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "trigger condition error is surfaced verbatim as 400",
			serviceErr:      domain.NewTriggerConditionError(conditionMessage),
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: conditionMessage,
		},
		{
			// The service wraps this error on its way up, which is why the handler
			// matches with errors.As rather than a type assertion.
			name:            "wrapped trigger condition error is surfaced verbatim as 400",
			serviceErr:      fmt.Errorf("failed to create automation trigger: %w", domain.NewTriggerConditionError(conditionMessage)),
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: conditionMessage,
		},
		{
			name:            "unrelated error stays a generic 500",
			serviceErr:      errors.New("database connection lost"),
			expectedStatus:  http.StatusInternalServerError,
			expectedMessage: "Failed to activate automation",
		},
		{
			name:            "permission error stays 403",
			serviceErr:      domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeWrite, "user lacks automations:write"),
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "user lacks automations:write",
		},
		{
			// The permission check sits next to an authenticate step that already wraps,
			// so the handler matches with errors.As rather than a type assertion: a
			// wrapped denial must stay a 403 instead of degrading into a generic 500.
			name:            "wrapped permission error stays 403",
			serviceErr:      fmt.Errorf("failed to authorize automation: %w", domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeWrite, "user lacks automations:write")),
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "user lacks automations:write",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			automationSvc.EXPECT().Activate(gomock.Any(), "workspace-123", "auto-123").Return(tc.serviceErr)

			body, err := json.Marshal(domain.ActivateAutomationRequest{
				WorkspaceID:  "workspace-123",
				AutomationID: "auto-123",
			})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/automations.activate", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, tc.expectedMessage, decodeErrorMessage(t, w))
		})
	}
}

func TestAutomationHandler_Update_ErrorMapping(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	const conditionMessage = "invalid trigger conditions: cannot use subquery in trigger WHEN condition"

	testCases := []struct {
		name            string
		serviceErr      error
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "trigger condition error is surfaced verbatim as 400",
			serviceErr:      domain.NewTriggerConditionError(conditionMessage),
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: conditionMessage,
		},
		{
			// Updating a live automation regenerates its trigger, and the service wraps
			// the failure before returning it.
			name:            "wrapped trigger condition error is surfaced verbatim as 400",
			serviceErr:      fmt.Errorf("failed to create automation trigger: %w", domain.NewTriggerConditionError(conditionMessage)),
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: conditionMessage,
		},
		{
			name:            "unrelated error stays a generic 500",
			serviceErr:      errors.New("database connection lost"),
			expectedStatus:  http.StatusInternalServerError,
			expectedMessage: "Failed to update automation",
		},
		{
			name:            "permission error stays 403",
			serviceErr:      domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeWrite, "user lacks automations:write"),
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "user lacks automations:write",
		},
		{
			// The permission check sits next to an authenticate step that already wraps,
			// so the handler matches with errors.As rather than a type assertion: a
			// wrapped denial must stay a 403 instead of degrading into a generic 500.
			name:            "wrapped permission error stays 403",
			serviceErr:      fmt.Errorf("failed to authorize automation: %w", domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeWrite, "user lacks automations:write")),
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "user lacks automations:write",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			automationSvc.EXPECT().Update(gomock.Any(), "workspace-123", gomock.Any()).Return(tc.serviceErr)

			body, err := json.Marshal(domain.UpdateAutomationRequest{
				WorkspaceID: "workspace-123",
				Automation:  createTestAutomation("auto-123", "workspace-123"),
			})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/automations.update", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, tc.expectedMessage, decodeErrorMessage(t, w))
		})
	}
}

func TestAutomationHandler_Pause(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful pause", func(t *testing.T) {
		automationSvc.EXPECT().Pause(gomock.Any(), "workspace-123", "auto-123").Return(nil)

		reqBody := domain.PauseAutomationRequest{
			WorkspaceID:  "workspace-123",
			AutomationID: "auto-123",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.pause", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("not active error", func(t *testing.T) {
		automationSvc.EXPECT().Pause(gomock.Any(), "workspace-123", "auto-123").Return(errors.New("not live"))

		reqBody := domain.PauseAutomationRequest{
			WorkspaceID:  "workspace-123",
			AutomationID: "auto-123",
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/automations.pause", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAutomationHandler_GetContactNodeExecutions(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	t.Run("successful get contact node executions", func(t *testing.T) {
		contactAutomation := &domain.ContactAutomation{
			ID:           "ca-123",
			AutomationID: "auto-123",
			ContactEmail: "test@example.com",
			Status:       domain.ContactAutomationStatusActive,
		}
		nodeExecutions := []*domain.NodeExecution{
			{
				ID:                  "entry-1",
				ContactAutomationID: "ca-123",
				NodeID:              "node-1",
				NodeType:            domain.NodeTypeTrigger,
				Action:              domain.NodeActionEntered,
			},
		}

		automationSvc.EXPECT().GetContactNodeExecutions(gomock.Any(), "workspace-123", "auto-123", "test@example.com").Return(contactAutomation, nodeExecutions, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/automations.nodeExecutions?workspace_id=workspace-123&automation_id=auto-123&email=test@example.com", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			ContactAutomation *domain.ContactAutomation `json:"contact_automation"`
			NodeExecutions    []*domain.NodeExecution   `json:"node_executions"`
		}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.NotNil(t, response.ContactAutomation)
		assert.Len(t, response.NodeExecutions, 1)
	})

	t.Run("validation error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/automations.nodeExecutions?workspace_id=workspace-123&automation_id=auto-123", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		automationSvc.EXPECT().GetContactNodeExecutions(gomock.Any(), "workspace-123", "auto-123", "notfound@example.com").Return(nil, nil, errors.New("not found"))

		req := httptest.NewRequest(http.MethodGet, "/api/automations.nodeExecutions?workspace_id=workspace-123&automation_id=auto-123&email=notfound@example.com", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// A transition that lost a race is the one automation failure the caller can actually do
// something about: reload and retry. A 500 tells them the server is broken, and a 400 tells
// them their request was — neither is true, and neither invites the retry that would work.
func TestAutomationHandler_ConflictMapping(t *testing.T) {
	_, automationSvc, mux, secretKey := setupAutomationTest(t)

	conflict := domain.NewAutomationConflictError("auto-123", domain.AutomationStatusLive)
	wrapped := fmt.Errorf("failed to update automation status: %w", conflict)

	testCases := []struct {
		name     string
		endpoint string
		expect   func()
		body     func(t *testing.T) []byte
	}{
		{
			name:     "update",
			endpoint: "/api/automations.update",
			expect: func() {
				automationSvc.EXPECT().Update(gomock.Any(), "workspace-123", gomock.Any()).Return(wrapped)
			},
			body: func(t *testing.T) []byte {
				body, err := json.Marshal(domain.UpdateAutomationRequest{
					WorkspaceID: "workspace-123",
					Automation:  createTestAutomation("auto-123", "workspace-123"),
				})
				require.NoError(t, err)
				return body
			},
		},
		{
			name:     "activate",
			endpoint: "/api/automations.activate",
			expect: func() {
				automationSvc.EXPECT().Activate(gomock.Any(), "workspace-123", "auto-123").Return(wrapped)
			},
			body: func(t *testing.T) []byte {
				body, err := json.Marshal(domain.ActivateAutomationRequest{
					WorkspaceID:  "workspace-123",
					AutomationID: "auto-123",
				})
				require.NoError(t, err)
				return body
			},
		},
		{
			name:     "pause",
			endpoint: "/api/automations.pause",
			expect: func() {
				automationSvc.EXPECT().Pause(gomock.Any(), "workspace-123", "auto-123").Return(wrapped)
			},
			body: func(t *testing.T) []byte {
				body, err := json.Marshal(domain.PauseAutomationRequest{
					WorkspaceID:  "workspace-123",
					AutomationID: "auto-123",
				})
				require.NoError(t, err)
				return body
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.expect()

			req := httptest.NewRequest(http.MethodPost, tc.endpoint, bytes.NewReader(tc.body(t)))
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusConflict, w.Code)
			assert.Equal(t, conflict.Error(), decodeErrorMessage(t, w))
		})
	}
}
