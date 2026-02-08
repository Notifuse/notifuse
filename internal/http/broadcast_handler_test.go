package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	http_handler "github.com/Notifuse/notifuse/internal/http"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	notifusemjml "github.com/Notifuse/notifuse/pkg/notifuse_mjml"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

// Helper function to create a test broadcast
func createTestBroadcast() *domain.Broadcast {
	now := time.Now()
	return &domain.Broadcast{
		ID:          "broadcast123",
		WorkspaceID: "workspace123",
		Name:        "Test Broadcast",
		Status:      domain.BroadcastStatusDraft,
		Audience: domain.AudienceSettings{
			List:     "list123",
			Segments: []string{"segment123"},
		},
		Schedule: domain.ScheduleSettings{
			IsScheduled: false,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// setupBroadcastHandler sets up a broadcast handler with mocks for testing
func setupBroadcastHandler(t *testing.T) (
	*http_handler.BroadcastHandler,
	*mocks.MockBroadcastService,
	*mocks.MockTemplateService,
	*pkgmocks.MockLogger,
	*gomock.Controller,
) {
	ctrl := gomock.NewController(t)

	// Create mocks
	mockBroadcastService := mocks.NewMockBroadcastService(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Create a public key for authentication
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	// Create the handler with mocks
	handler := http_handler.NewBroadcastHandler(
		mockBroadcastService,
		mockTemplateService,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		false,
	)

	return handler, mockBroadcastService, mockTemplateService, mockLogger, ctrl
}

// TestHandleList tests the handleList function
func TestHandleList(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()
	broadcasts := []*domain.Broadcast{createTestBroadcast()}

	// Create a response with broadcasts and a total count
	responseWithTotal := &domain.BroadcastListResponse{
		Broadcasts: broadcasts,
		TotalCount: 1,
	}

	// Test successful list
	t.Run("Success", func(t *testing.T) {
		mockService.EXPECT().
			ListBroadcasts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
				assert.Equal(t, "workspace123", params.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatus(""), params.Status)
				return responseWithTotal, nil
			})

		// Create a test request with query parameters
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.list?workspace_id=workspace123", nil)
		w := httptest.NewRecorder()

		// Call the exported handler method directly
		handler.HandleList(w, req)

		// Verify the response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "broadcasts")
		assert.Contains(t, response, "total_count")
		assert.Equal(t, float64(1), response["total_count"]) // JSON unmarshals numbers as float64
	})

	// Test with pagination parameters
	t.Run("WithPagination", func(t *testing.T) {
		mockService.EXPECT().
			ListBroadcasts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
				assert.Equal(t, "workspace123", params.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatus("draft"), params.Status)
				assert.Equal(t, 10, params.Limit)
				assert.Equal(t, 20, params.Offset)
				return responseWithTotal, nil
			})

		// Create a test request with query parameters including pagination
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.list?workspace_id=workspace123&status=draft&limit=10&offset=20", nil)
		w := httptest.NewRecorder()

		// Call the exported handler method directly
		handler.HandleList(w, req)

		// Verify the response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "broadcasts")
		assert.Contains(t, response, "total_count")
	})

	// Test invalid pagination parameters
	t.Run("InvalidPaginationParams", func(t *testing.T) {
		// Create a test request with invalid pagination parameters
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.list?workspace_id=workspace123&limit=invalid", nil)
		w := httptest.NewRecorder()

		// Call the exported handler method directly
		handler.HandleList(w, req)

		// Verify the response
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test missing workspace_id
	t.Run("MissingWorkspaceID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.list", nil)
		w := httptest.NewRecorder()

		// Call the exported handler method directly
		handler.HandleList(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test service error
	t.Run("ServiceError", func(t *testing.T) {
		// Set up expectations for logger
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to list broadcasts")

		mockService.EXPECT().
			ListBroadcasts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
				assert.Equal(t, "workspace123", params.WorkspaceID)
				assert.Equal(t, domain.BroadcastStatus(""), params.Status)
				return nil, errors.New("service error")
			})

		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.list?workspace_id=workspace123", nil)
		w := httptest.NewRecorder()

		// Call the exported handler method directly
		handler.HandleList(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.list?workspace_id=workspace123", nil)
		w := httptest.NewRecorder()

		// Call the exported handler method directly
		handler.HandleList(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

// TestHandleGet tests the handleGet function
func TestHandleGet(t *testing.T) {
	handler, mockService, mockTemplateService, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()
	broadcast := createTestBroadcast()

	// Extend test broadcast to include test settings with variations
	broadcast.TestSettings = domain.BroadcastTestSettings{
		Enabled: true,
		Variations: []domain.BroadcastVariation{
			{
				VariationName: "variation1",
				TemplateID:    "template123",
			},
		},
	}

	// Test successful get
	t.Run("Success", func(t *testing.T) {
		mockService.EXPECT().
			GetBroadcast(gomock.Any(), "workspace123", "broadcast123").
			Return(broadcast, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.get?workspace_id=workspace123&id=broadcast123", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "broadcast")
	})

	// Test successful get with template fetching
	t.Run("SuccessWithTemplates", func(t *testing.T) {
		mockService.EXPECT().
			GetBroadcast(gomock.Any(), "workspace123", "broadcast123").
			Return(broadcast, nil)

		// Setup mock template service to return a template when requested
		template := &domain.Template{
			ID:      "template123",
			Name:    "Test Template",
			Channel: "email",
			Email: &domain.EmailTemplate{
				SenderID:        "sender123",
				Subject:         "Test Subject",
				CompiledPreview: "<p>Test HTML content</p>",
				VisualEditorTree: func() notifusemjml.EmailBlock {
					base := notifusemjml.NewBaseBlock("root", notifusemjml.MJMLComponentMjml)
					base.Attributes["version"] = "4.0.0"
					return &notifusemjml.MJMLBlock{BaseBlock: base}
				}(),
			},
			Category:  "marketing",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), "workspace123", "template123", gomock.Any()).
			Return(template, nil)

		// Create request with WithTemplates=true
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.get?workspace_id=workspace123&id=broadcast123&with_templates=true", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "broadcast")

		// Access the broadcast data to verify the template was attached
		broadcastData, ok := response["broadcast"].(map[string]interface{})
		assert.True(t, ok)

		testSettings, ok := broadcastData["test_settings"].(map[string]interface{})
		assert.True(t, ok)

		variations, ok := testSettings["variations"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, variations, 1)

		// The template should be attached to the variation
		variation, ok := variations[0].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, variation, "template")
	})

	// Test template fetch error (should continue without failing)
	t.Run("TemplateError", func(t *testing.T) {
		mockService.EXPECT().
			GetBroadcast(gomock.Any(), "workspace123", "broadcast123").
			Return(broadcast, nil)

		// Setup mock template service to return an error
		templateError := errors.New("template error")
		mockTemplateService.EXPECT().
			GetTemplateByID(gomock.Any(), "workspace123", "template123", gomock.Any()).
			Return(nil, templateError)

		// Setup mock for logger.WithFields and the warning
		mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
		expectedFields := map[string]interface{}{
			"error":        templateError,
			"workspace_id": "workspace123",
			"broadcast_id": "broadcast123",
			"template_id":  "template123",
		}
		mockLogger.EXPECT().
			WithFields(expectedFields).
			Return(mockLoggerWithFields)
		mockLoggerWithFields.EXPECT().
			Warn("Failed to fetch template for broadcast variation")

		// Create request with WithTemplates=true
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.get?workspace_id=workspace123&id=broadcast123&with_templates=true", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "broadcast")
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		mockService.EXPECT().
			GetBroadcast(gomock.Any(), "workspace123", "nonexistent").
			Return(nil, &domain.ErrBroadcastNotFound{ID: "nonexistent"})

		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.get?workspace_id=workspace123&id=nonexistent", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.get?workspace_id=workspace123&id=broadcast123", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	// Test missing required parameters
	t.Run("MissingParams", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.get?workspace_id=workspace123", nil) // missing id
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandleCreate tests the handleCreate function
func TestHandleCreate(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()
	broadcast := createTestBroadcast()

	// Test successful create
	t.Run("Success", func(t *testing.T) {
		createRequest := &domain.CreateBroadcastRequest{
			WorkspaceID: "workspace123",
			Name:        "Test Broadcast",
			Audience: domain.AudienceSettings{
				List:     "list123",
				Segments: []string{"segment123"},
			},
		}

		mockService.EXPECT().
			CreateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.CreateBroadcastRequest) (*domain.Broadcast, error) {
				assert.Equal(t, createRequest.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, createRequest.Name, req.Name)
				return broadcast, nil
			})

		requestBody, _ := json.Marshal(createRequest)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.create", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "broadcast")
	})

	// Test service error
	t.Run("CreateError", func(t *testing.T) {
		createRequest := &domain.CreateBroadcastRequest{
			WorkspaceID: "workspace123",
			Name:        "Test Broadcast",
			Audience: domain.AudienceSettings{
				List:     "list123",
				Segments: []string{"segment123"},
			},
		}

		// Set up expectations for the logger using gomock
		errorLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to create broadcast")

		mockService.EXPECT().
			CreateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.CreateBroadcastRequest) (*domain.Broadcast, error) {
				assert.Equal(t, createRequest.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, createRequest.Name, req.Name)
				return nil, errors.New("service error")
			})

		requestBody, _ := json.Marshal(createRequest)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.create", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		// Set up logger expectations using gomock
		errorLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "invalid character 'i' looking for beginning of object key string").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to decode request body")

		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.create", bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandleSchedule tests the handleSchedule function
func TestHandleSchedule(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	// Test successful scheduling for later
	t.Run("ScheduleForLater", func(t *testing.T) {
		scheduledTime := time.Now().Add(24 * time.Hour).UTC()
		scheduledDate := scheduledTime.Format("2006-01-02")
		scheduledTimeStr := scheduledTime.Format("15:04")

		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID:          "workspace123",
			ID:                   "broadcast123",
			SendNow:              false,
			ScheduledDate:        scheduledDate,
			ScheduledTime:        scheduledTimeStr,
			Timezone:             "UTC",
			UseRecipientTimezone: false,
		}

		mockService.EXPECT().
			ScheduleBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.ScheduleBroadcastRequest) error {
				assert.Equal(t, request.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, request.ID, req.ID)
				assert.Equal(t, request.SendNow, req.SendNow)
				assert.Equal(t, scheduledDate, req.ScheduledDate)
				assert.Equal(t, scheduledTimeStr, req.ScheduledTime)
				assert.Equal(t, "UTC", req.Timezone)
				return nil
			})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.schedule", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	// Test successful send now
	t.Run("SendNow", func(t *testing.T) {
		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
			SendNow:     true,
		}

		mockService.EXPECT().
			ScheduleBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.ScheduleBroadcastRequest) error {
				assert.Equal(t, request.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, request.ID, req.ID)
				assert.Equal(t, request.SendNow, req.SendNow)
				return nil
			})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.schedule", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	// Test validation error
	t.Run("ValidationError", func(t *testing.T) {
		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
			SendNow:     false,
			// Missing scheduled date and time
		}

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.schedule", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistent",
			SendNow:     true,
		}

		// Create a custom handler just for this test
		customController := gomock.NewController(t)
		defer customController.Finish()
		customMock := mocks.NewMockBroadcastService(customController)
		customTemplateService := mocks.NewMockTemplateService(customController)
		customLogger := pkgmocks.NewMockLogger(customController)

		// Create a public key for authentication
		jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

		// Create the handler with mocks
		customHandler := http_handler.NewBroadcastHandler(
			customMock,
			customTemplateService,
			func() ([]byte, error) { return jwtSecret, nil },
			customLogger,
			false,
		)

		// Setup mock to return a broadcast not found error
		customMock.EXPECT().
			ScheduleBroadcast(gomock.Any(), gomock.Any()).
			Return(&domain.ErrBroadcastNotFound{ID: "nonexistent"})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.schedule", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test invalid status (not draft)
	t.Run("InvalidStatus", func(t *testing.T) {
		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
			SendNow:     true,
		}

		// Create a custom handler just for this test
		customController := gomock.NewController(t)
		defer customController.Finish()
		customMock := mocks.NewMockBroadcastService(customController)
		customTemplateService := mocks.NewMockTemplateService(customController)
		customLogger := pkgmocks.NewMockLogger(customController)

		// Create a public key for authentication
		jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

		// Create the handler with mocks
		customHandler := http_handler.NewBroadcastHandler(
			customMock,
			customTemplateService,
			func() ([]byte, error) { return jwtSecret, nil },
			customLogger,
			false,
		)

		// Set up expectations differently - this is more direct and explicit
		errorLogger := pkgmocks.NewMockLogger(customController)
		customLogger.EXPECT().WithField("error", "only broadcasts with draft status can be scheduled, current status: sending").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to schedule broadcast")

		// Setup mock to return a status error
		customMock.EXPECT().
			ScheduleBroadcast(gomock.Any(), gomock.Any()).
			Return(fmt.Errorf("only broadcasts with draft status can be scheduled, current status: sending"))

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.schedule", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test service error
	t.Run("ServiceError", func(t *testing.T) {
		request := &domain.ScheduleBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
			SendNow:     true,
		}

		// Create a custom handler just for this test
		customController := gomock.NewController(t)
		defer customController.Finish()
		customMock := mocks.NewMockBroadcastService(customController)
		customTemplateService := mocks.NewMockTemplateService(customController)
		customLogger := pkgmocks.NewMockLogger(customController)

		// Create a public key for authentication
		jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

		// Create the handler with mocks
		customHandler := http_handler.NewBroadcastHandler(
			customMock,
			customTemplateService,
			func() ([]byte, error) { return jwtSecret, nil },
			customLogger,
			false,
		)

		// Set up expectations differently - this is more direct and explicit
		errorLogger := pkgmocks.NewMockLogger(customController)
		customLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to schedule broadcast")

		// Setup mock to return a generic error
		customMock.EXPECT().
			ScheduleBroadcast(gomock.Any(), gomock.Any()).
			Return(errors.New("service error"))

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.schedule", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.schedule", nil)
		w := httptest.NewRecorder()

		handler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	// Test invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		// Set up logger expectations
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "invalid character 'i' looking for beginning of object key string").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.schedule", bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSchedule(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandleCancel tests the handleCancel function
func TestHandleCancel(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	// Test successful cancel
	t.Run("Success", func(t *testing.T) {
		request := &domain.CancelBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		mockService.EXPECT().
			CancelBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.CancelBroadcastRequest) error {
				assert.Equal(t, request.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, request.ID, req.ID)
				return nil
			})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.cancel", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Direct method call for testing
		handler.HandleCancel(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	// Test validation error
	t.Run("ValidationError", func(t *testing.T) {
		request := &domain.CancelBroadcastRequest{
			// Missing required fields
		}

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.cancel", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleCancel(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		request := &domain.CancelBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistent",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, _, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Setup mock to return a broadcast not found error
		customMock.EXPECT().
			CancelBroadcast(gomock.Any(), gomock.Any()).
			Return(&domain.ErrBroadcastNotFound{ID: "nonexistent"})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.cancel", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleCancel(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test invalid status
	t.Run("InvalidStatus", func(t *testing.T) {
		request := &domain.CancelBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, customLogger, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Set up expectations differently - this is more direct and explicit
		errorLogger := pkgmocks.NewMockLogger(customCtrl)
		customLogger.EXPECT().WithField("error", "only broadcasts with scheduled or paused status can be cancelled, current status: draft").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to cancel broadcast")

		// Setup mock to return a status error
		customMock.EXPECT().
			CancelBroadcast(gomock.Any(), gomock.Any()).
			Return(fmt.Errorf("only broadcasts with scheduled or paused status can be cancelled, current status: draft"))

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.cancel", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleCancel(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test service error
	t.Run("ServiceError", func(t *testing.T) {
		request := &domain.CancelBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, customLogger, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Set up expectations differently - this is more direct and explicit
		errorLogger := pkgmocks.NewMockLogger(customCtrl)
		customLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to cancel broadcast")

		// Setup mock to return a generic error
		customMock.EXPECT().
			CancelBroadcast(gomock.Any(), gomock.Any()).
			Return(errors.New("service error"))

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.cancel", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleCancel(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.cancel", nil)
		w := httptest.NewRecorder()

		handler.HandleCancel(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	// Test invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		// Set up logger expectations
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "invalid character 'i' looking for beginning of object key string").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.cancel", bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleCancel(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandlePause tests the handlePause function
func TestHandlePause(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	// Test successful pause
	t.Run("Success", func(t *testing.T) {
		request := &domain.PauseBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		mockService.EXPECT().
			PauseBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.PauseBroadcastRequest) error {
				assert.Equal(t, request.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, request.ID, req.ID)
				return nil
			})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.pause", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Direct method call for testing
		handler.HandlePause(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	// Test validation error
	t.Run("ValidationError", func(t *testing.T) {
		request := &domain.PauseBroadcastRequest{
			// Missing required fields
		}

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.pause", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandlePause(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		request := &domain.PauseBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistent",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, _, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Setup mock to return a broadcast not found error
		customMock.EXPECT().
			PauseBroadcast(gomock.Any(), gomock.Any()).
			Return(&domain.ErrBroadcastNotFound{ID: "nonexistent"})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.pause", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandlePause(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test invalid status
	t.Run("InvalidStatus", func(t *testing.T) {
		request := &domain.PauseBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, customLogger, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Set up expectations differently - this is more direct and explicit
		errorLogger := pkgmocks.NewMockLogger(customCtrl)
		customLogger.EXPECT().WithField("error", "only broadcasts with sending status can be paused, current status: draft").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to pause broadcast")

		// Setup mock to return a status error
		customMock.EXPECT().
			PauseBroadcast(gomock.Any(), gomock.Any()).
			Return(fmt.Errorf("only broadcasts with sending status can be paused, current status: draft"))

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.pause", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandlePause(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test service error
	t.Run("ServiceError", func(t *testing.T) {
		request := &domain.PauseBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, customLogger, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Set up expectations differently - this is more direct and explicit
		errorLogger := pkgmocks.NewMockLogger(customCtrl)
		customLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to pause broadcast")

		// Setup mock to return a generic error
		customMock.EXPECT().
			PauseBroadcast(gomock.Any(), gomock.Any()).
			Return(errors.New("service error"))

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.pause", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandlePause(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.pause", nil)
		w := httptest.NewRecorder()

		handler.HandlePause(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	// Test invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		// Set up logger expectations
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "invalid character 'i' looking for beginning of object key string").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.pause", bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandlePause(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandleDelete tests the handleDelete function
func TestHandleDelete(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	// Test successful delete
	t.Run("Success", func(t *testing.T) {
		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		mockService.EXPECT().
			DeleteBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.DeleteBroadcastRequest) error {
				assert.Equal(t, request.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, request.ID, req.ID)
				return nil
			})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.delete", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Direct method call for testing
		handler.HandleDelete(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	// Test validation error
	t.Run("ValidationError", func(t *testing.T) {
		request := &domain.DeleteBroadcastRequest{
			// Missing required fields
		}

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.delete", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleDelete(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistent",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, _, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Setup mock to return a broadcast not found error
		customMock.EXPECT().
			DeleteBroadcast(gomock.Any(), gomock.Any()).
			Return(&domain.ErrBroadcastNotFound{ID: "nonexistent"})

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.delete", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleDelete(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test service error
	t.Run("ServiceError", func(t *testing.T) {
		request := &domain.DeleteBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		// Create a custom handler just for this test
		customHandler, customMock, _, customLogger, customCtrl := setupBroadcastHandler(t)
		defer customCtrl.Finish()

		// Set up expectations differently - this is more direct and explicit
		errorLogger := pkgmocks.NewMockLogger(customCtrl)
		customLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to delete broadcast")

		// Setup mock to return a generic error
		customMock.EXPECT().
			DeleteBroadcast(gomock.Any(), gomock.Any()).
			Return(errors.New("service error"))

		requestBody, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.delete", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		customHandler.HandleDelete(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.delete", nil)
		w := httptest.NewRecorder()

		handler.HandleDelete(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	// Test invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		// Set up logger expectations
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "invalid character 'i' looking for beginning of object key string").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.delete", bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleDelete(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandleUpdate tests the handleUpdate function
func TestHandleUpdate(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()
	broadcast := createTestBroadcast()
	updatedBroadcast := &domain.Broadcast{
		ID:          broadcast.ID,
		WorkspaceID: broadcast.WorkspaceID,
		Name:        "Updated Broadcast",
		Status:      broadcast.Status,
		Audience:    broadcast.Audience,
		Schedule:    broadcast.Schedule,
		CreatedAt:   broadcast.CreatedAt,
		UpdatedAt:   time.Now(),
	}

	// Test successful update
	t.Run("Success", func(t *testing.T) {
		updateRequest := &domain.UpdateBroadcastRequest{
			ID:          broadcast.ID,
			WorkspaceID: broadcast.WorkspaceID,
			Name:        "Updated Broadcast",
			Audience: domain.AudienceSettings{
				List:     "list123",
				Segments: []string{"segment123"},
			},
		}

		mockService.EXPECT().
			GetBroadcast(gomock.Any(), broadcast.WorkspaceID, broadcast.ID).
			Return(broadcast, nil)

		mockService.EXPECT().
			UpdateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.UpdateBroadcastRequest) (*domain.Broadcast, error) {
				assert.Equal(t, updateRequest.ID, req.ID)
				assert.Equal(t, updateRequest.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, updateRequest.Name, req.Name)
				return updatedBroadcast, nil
			})

		requestBody, _ := json.Marshal(updateRequest)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.update", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Use the exported handler method
		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "broadcast")
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		updateRequest := &domain.UpdateBroadcastRequest{
			ID:          "nonexistent",
			WorkspaceID: broadcast.WorkspaceID,
			Name:        "Updated Broadcast",
			Audience: domain.AudienceSettings{
				List:     "list123",
				Segments: []string{"segment123"},
			},
		}

		mockService.EXPECT().
			GetBroadcast(gomock.Any(), broadcast.WorkspaceID, "nonexistent").
			Return(nil, &domain.ErrBroadcastNotFound{ID: "nonexistent"})

		requestBody, _ := json.Marshal(updateRequest)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.update", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Use the exported handler method
		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test validation error
	t.Run("ValidationError", func(t *testing.T) {
		updateRequest := &domain.UpdateBroadcastRequest{
			ID:          broadcast.ID,
			WorkspaceID: broadcast.WorkspaceID,
			// Missing required fields like Name and no audience
		}

		mockService.EXPECT().
			GetBroadcast(gomock.Any(), broadcast.WorkspaceID, broadcast.ID).
			Return(broadcast, nil)

		requestBody, _ := json.Marshal(updateRequest)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.update", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Use the exported handler method
		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test service GetBroadcast error
	t.Run("GetBroadcastError", func(t *testing.T) {
		updateRequest := &domain.UpdateBroadcastRequest{
			ID:          broadcast.ID,
			WorkspaceID: broadcast.WorkspaceID,
			Name:        "Updated Broadcast",
			Audience: domain.AudienceSettings{
				List:     "list123",
				Segments: []string{"segment123"},
			},
		}

		// Set up expectations for logger using gomock
		errorLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to get existing broadcast")

		mockService.EXPECT().
			GetBroadcast(gomock.Any(), broadcast.WorkspaceID, broadcast.ID).
			Return(nil, errors.New("service error"))

		requestBody, _ := json.Marshal(updateRequest)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.update", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Use the exported handler method
		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test service UpdateBroadcast error
	t.Run("UpdateBroadcastError", func(t *testing.T) {
		updateRequest := &domain.UpdateBroadcastRequest{
			ID:          broadcast.ID,
			WorkspaceID: broadcast.WorkspaceID,
			Name:        "Updated Broadcast",
			Audience: domain.AudienceSettings{
				List:     "list123",
				Segments: []string{"segment123"},
			},
		}

		mockService.EXPECT().
			GetBroadcast(gomock.Any(), broadcast.WorkspaceID, broadcast.ID).
			Return(broadcast, nil)

		// Set up expectations for logger using gomock
		errorLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to update broadcast")

		mockService.EXPECT().
			UpdateBroadcast(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("service error"))

		requestBody, _ := json.Marshal(updateRequest)
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.update", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Use the exported handler method
		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// Test invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		// Set up expectations for logger using gomock
		errorLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "invalid character 'i' looking for beginning of object key string").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to decode request body")

		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.update", bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Use the exported handler method
		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.update", nil)
		w := httptest.NewRecorder()

		// Use the exported handler method
		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

// TestHandleResume tests the HandleResume function
func TestHandleResume(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	// Test successful resume
	t.Run("Success", func(t *testing.T) {
		// Prepare request
		req := domain.ResumeBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		// Set expectations
		mockService.EXPECT().
			ResumeBroadcast(gomock.Any(), &req).
			Return(nil)

		// Create HTTP request
		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.resume", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the handler
		handler.HandleResume(w, httpReq)

		// Verify response
		assert.Equal(t, http.StatusOK, w.Code)
		var response map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &response)
		assert.True(t, response["success"].(bool))
	})

	// Test invalid request (method not allowed)
	t.Run("MethodNotAllowed", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/broadcasts.resume", nil)
		w := httptest.NewRecorder()

		handler.HandleResume(w, httpReq)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	// Test invalid request body
	t.Run("InvalidRequestBody", func(t *testing.T) {
		// Set up logger expectations for error logging
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", gomock.Any()).Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.resume", bytes.NewBuffer([]byte("invalid json")))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleResume(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test missing required fields
	t.Run("MissingRequiredFields", func(t *testing.T) {
		// Prepare request with missing WorkspaceID
		req := map[string]string{
			"id": "broadcast123", // Missing workspace_id
		}

		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.resume", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleResume(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		// Prepare request
		req := domain.ResumeBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistentbroadcast",
		}

		// Set expectations - service returns not found error
		mockService.EXPECT().
			ResumeBroadcast(gomock.Any(), &req).
			Return(&domain.ErrBroadcastNotFound{ID: req.ID})

		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.resume", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleResume(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test service error
	t.Run("ServiceError", func(t *testing.T) {
		// Prepare request
		req := domain.ResumeBroadcastRequest{
			WorkspaceID: "workspace123",
			ID:          "broadcast123",
		}

		// Set logger expectations
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to resume broadcast")

		// Set service expectations - returns error
		mockService.EXPECT().
			ResumeBroadcast(gomock.Any(), &req).
			Return(errors.New("service error"))

		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.resume", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleResume(w, httpReq)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// TestHandleSendToIndividual tests the HandleSendToIndividual function
func TestHandleSendToIndividual(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	// Test successful send to individual
	t.Run("Success", func(t *testing.T) {
		// Prepare request
		req := domain.SendToIndividualRequest{
			WorkspaceID:    "workspace123",
			BroadcastID:    "broadcast123",
			RecipientEmail: "user@example.com",
		}

		// Set expectations
		mockService.EXPECT().
			SendToIndividual(gomock.Any(), &req).
			Return(nil)

		// Create HTTP request
		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.sendToIndividual", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the handler
		handler.HandleSendToIndividual(w, httpReq)

		// Verify response
		assert.Equal(t, http.StatusOK, w.Code)
		var response map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &response)
		assert.True(t, response["success"].(bool))
	})

	// Test invalid request (method not allowed)
	t.Run("MethodNotAllowed", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/broadcasts.sendToIndividual", nil)
		w := httptest.NewRecorder()

		handler.HandleSendToIndividual(w, httpReq)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	// Test invalid request body
	t.Run("InvalidRequestBody", func(t *testing.T) {
		// Set up logger expectations for error logging
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", gomock.Any()).Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.sendToIndividual", bytes.NewBuffer([]byte("invalid json")))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSendToIndividual(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test missing required fields
	t.Run("MissingRequiredFields", func(t *testing.T) {
		// Prepare request with missing fields
		req := map[string]string{
			"workspace_id": "workspace123",
			// Missing broadcast_id and recipient_email
		}

		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.sendToIndividual", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSendToIndividual(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test broadcast not found
	t.Run("BroadcastNotFound", func(t *testing.T) {
		// Prepare request
		req := domain.SendToIndividualRequest{
			WorkspaceID:    "workspace123",
			BroadcastID:    "nonexistentbroadcast",
			RecipientEmail: "user@example.com",
		}

		// Set expectations - service returns not found error
		mockService.EXPECT().
			SendToIndividual(gomock.Any(), &req).
			Return(&domain.ErrBroadcastNotFound{ID: req.BroadcastID})

		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.sendToIndividual", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSendToIndividual(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test service error
	t.Run("ServiceError", func(t *testing.T) {
		// Prepare request
		req := domain.SendToIndividualRequest{
			WorkspaceID:    "workspace123",
			BroadcastID:    "broadcast123",
			RecipientEmail: "user@example.com",
		}

		// Set logger expectations
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to send broadcast to individual")

		// Set service expectations - returns error
		mockService.EXPECT().
			SendToIndividual(gomock.Any(), &req).
			Return(errors.New("service error"))

		jsonData, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.sendToIndividual", bytes.NewBuffer(jsonData))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleSendToIndividual(w, httpReq)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// TestRegisterRoutes tests the RegisterRoutes function
func TestRegisterRoutes(t *testing.T) {
	handler, _, _, _, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	// Create a new mux
	mux := http.NewServeMux()

	// Register routes
	handler.RegisterRoutes(mux)

	// Define all expected routes
	routes := []string{
		"/api/broadcasts.list",
		"/api/broadcasts.get",
		"/api/broadcasts.create",
		"/api/broadcasts.update",
		"/api/broadcasts.schedule",
		"/api/broadcasts.pause",
		"/api/broadcasts.resume",
		"/api/broadcasts.cancel",
		"/api/broadcasts.sendToIndividual",
		"/api/broadcasts.delete",
	}

	// Verify all routes are registered
	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		match, _ := mux.Handler(req)
		assert.NotNil(t, match, "Route should be registered: "+route)
	}
}

// Tests for A/B testing endpoints: HandleGetTestResults and HandleSelectWinner
func TestHandleGetTestResults(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		now := time.Now()
		resp := &domain.TestResultsResponse{
			BroadcastID:     "broadcast123",
			Status:          "completed",
			TestStartedAt:   &now,
			TestCompletedAt: &now,
			VariationResults: map[string]*domain.VariationResult{
				"templateA": {TemplateID: "templateA", TemplateName: "A", Recipients: 100, Delivered: 100, Opens: 50, Clicks: 10, OpenRate: 0.5, ClickRate: 0.1},
			},
		}

		mockService.EXPECT().GetTestResults(gomock.Any(), "workspace123", "broadcast123").Return(resp, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.getTestResults?workspace_id=workspace123&id=broadcast123", nil)
		w := httptest.NewRecorder()

		handler.HandleGetTestResults(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &body)
		assert.NoError(t, err)
		assert.Equal(t, "broadcast123", body["broadcast_id"])
	})

	t.Run("ValidationError", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.getTestResults?workspace_id=workspace123", nil) // missing id
		w := httptest.NewRecorder()
		handler.HandleGetTestResults(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ServiceError", func(t *testing.T) {
		withFields := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(withFields)
		withFields.EXPECT().Error("Failed to get test results")

		mockService.EXPECT().GetTestResults(gomock.Any(), "workspace123", "broadcast123").Return(nil, errors.New("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/broadcasts.getTestResults?workspace_id=workspace123&id=broadcast123", nil)
		w := httptest.NewRecorder()
		handler.HandleGetTestResults(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.getTestResults?workspace_id=workspace123&id=broadcast123", nil)
		w := httptest.NewRecorder()
		handler.HandleGetTestResults(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestHandleSelectWinner(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		reqBody := domain.SelectWinnerRequest{WorkspaceID: "workspace123", ID: "broadcast123", TemplateID: "templateA"}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.selectWinner", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		mockService.EXPECT().SelectWinner(gomock.Any(), "workspace123", "broadcast123", "templateA").Return(nil)

		w := httptest.NewRecorder()
		handler.HandleSelectWinner(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		assert.True(t, body["success"].(bool))
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		lf := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", gomock.Any()).Return(lf)
		lf.EXPECT().Error("Failed to decode request body")

		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.selectWinner", bytes.NewBuffer([]byte("{invalid")))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.HandleSelectWinner(w, httpReq)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ValidationError", func(t *testing.T) {
		// Missing required fields
		reqBody := map[string]string{"workspace_id": "workspace123"}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.selectWinner", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.HandleSelectWinner(w, httpReq)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ServiceError", func(t *testing.T) {
		withFields := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(withFields)
		withFields.EXPECT().Error("Failed to select winner")

		reqBody := domain.SelectWinnerRequest{WorkspaceID: "workspace123", ID: "broadcast123", TemplateID: "templateA"}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.selectWinner", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		mockService.EXPECT().SelectWinner(gomock.Any(), "workspace123", "broadcast123", "templateA").Return(errors.New("svc error"))

		w := httptest.NewRecorder()
		handler.HandleSelectWinner(w, httpReq)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/broadcasts.selectWinner", nil)
		w := httptest.NewRecorder()
		handler.HandleSelectWinner(w, httpReq)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

// TestHandleRefreshGlobalFeed tests the HandleRefreshGlobalFeed handler
func TestHandleRefreshGlobalFeed(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		now := time.Now().UTC()
		feedData := map[string]interface{}{
			"products": []interface{}{
				map[string]interface{}{"id": "1", "name": "Product 1"},
			},
			"_success":    true,
			"_fetched_at": now.Format(time.RFC3339),
		}

		mockService.EXPECT().
			RefreshGlobalFeed(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.RefreshGlobalFeedRequest) (*domain.RefreshGlobalFeedResponse, error) {
				assert.Equal(t, "workspace123", req.WorkspaceID)
				assert.Equal(t, "broadcast123", req.BroadcastID)
				assert.Equal(t, "https://example.com/feed", req.URL)
				return &domain.RefreshGlobalFeedResponse{
					Success:   true,
					Data:      feedData,
					FetchedAt: &now,
				}, nil
			})

		reqBody := domain.RefreshGlobalFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "broadcast123",
			URL:         "https://example.com/feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.refreshGlobalFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleRefreshGlobalFeed(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response domain.RefreshGlobalFeedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response.Success)
		assert.NotNil(t, response.Data)
		assert.NotNil(t, response.FetchedAt)
	})

	t.Run("FetchError", func(t *testing.T) {
		mockService.EXPECT().
			RefreshGlobalFeed(gomock.Any(), gomock.Any()).
			Return(&domain.RefreshGlobalFeedResponse{
				Success: false,
				Error:   "failed to fetch global feed: connection timeout",
			}, nil)

		reqBody := domain.RefreshGlobalFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "broadcast123",
			URL:         "https://example.com/feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.refreshGlobalFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleRefreshGlobalFeed(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response domain.RefreshGlobalFeedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response.Success)
		assert.Contains(t, response.Error, "connection timeout")
	})

	t.Run("MissingParams", func(t *testing.T) {
		// Missing broadcast_id
		reqBody := map[string]string{
			"workspace_id": "workspace123",
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.refreshGlobalFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleRefreshGlobalFeed(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", gomock.Any()).Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.refreshGlobalFeed", bytes.NewBuffer([]byte("{invalid")))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleRefreshGlobalFeed(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/broadcasts.refreshGlobalFeed", nil)
		w := httptest.NewRecorder()

		handler.HandleRefreshGlobalFeed(w, httpReq)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("BroadcastNotFound", func(t *testing.T) {
		mockService.EXPECT().
			RefreshGlobalFeed(gomock.Any(), gomock.Any()).
			Return(nil, &domain.ErrBroadcastNotFound{ID: "nonexistent"})

		reqBody := domain.RefreshGlobalFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "nonexistent",
			URL:         "https://example.com/feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.refreshGlobalFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleRefreshGlobalFeed(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("ServiceError", func(t *testing.T) {
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to refresh global feed")

		mockService.EXPECT().
			RefreshGlobalFeed(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("service error"))

		reqBody := domain.RefreshGlobalFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "broadcast123",
			URL:         "https://example.com/feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.refreshGlobalFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleRefreshGlobalFeed(w, httpReq)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// TestHandleTestRecipientFeed tests the HandleTestRecipientFeed handler
func TestHandleTestRecipientFeed(t *testing.T) {
	handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		now := time.Now().UTC()
		feedData := map[string]interface{}{
			"recommendations": []interface{}{
				map[string]interface{}{"id": "rec1", "title": "Product 1"},
			},
			"_success":    true,
			"_fetched_at": now.Format(time.RFC3339),
		}

		mockService.EXPECT().
			TestRecipientFeed(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.TestRecipientFeedRequest) (*domain.TestRecipientFeedResponse, error) {
				assert.Equal(t, "workspace123", req.WorkspaceID)
				assert.Equal(t, "broadcast123", req.BroadcastID)
				assert.Equal(t, "https://example.com/recipient-feed", req.URL)
				return &domain.TestRecipientFeedResponse{
					Success:      true,
					Data:         feedData,
					FetchedAt:    &now,
					ContactEmail: "sample@example.com",
				}, nil
			})

		reqBody := domain.TestRecipientFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "broadcast123",
			URL:         "https://example.com/recipient-feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response domain.TestRecipientFeedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response.Success)
		assert.NotNil(t, response.Data)
		assert.NotNil(t, response.FetchedAt)
		assert.Equal(t, "sample@example.com", response.ContactEmail)
	})

	t.Run("SuccessWithSpecificContact", func(t *testing.T) {
		now := time.Now().UTC()
		feedData := map[string]interface{}{
			"user_data": map[string]interface{}{"preferences": "premium"},
		}

		mockService.EXPECT().
			TestRecipientFeed(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *domain.TestRecipientFeedRequest) (*domain.TestRecipientFeedResponse, error) {
				assert.Equal(t, "workspace123", req.WorkspaceID)
				assert.Equal(t, "broadcast123", req.BroadcastID)
				assert.Equal(t, "test@example.com", req.ContactEmail)
				return &domain.TestRecipientFeedResponse{
					Success:      true,
					Data:         feedData,
					FetchedAt:    &now,
					ContactEmail: "test@example.com",
				}, nil
			})

		reqBody := domain.TestRecipientFeedRequest{
			WorkspaceID:  "workspace123",
			BroadcastID:  "broadcast123",
			ContactEmail: "test@example.com",
			URL:          "https://example.com/recipient-feed",
			Headers:      []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response domain.TestRecipientFeedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response.Success)
		assert.Equal(t, "test@example.com", response.ContactEmail)
	})

	t.Run("FetchError", func(t *testing.T) {
		mockService.EXPECT().
			TestRecipientFeed(gomock.Any(), gomock.Any()).
			Return(&domain.TestRecipientFeedResponse{
				Success:      false,
				Error:        "failed to fetch recipient feed: connection timeout",
				ContactEmail: "sample@example.com",
			}, nil)

		reqBody := domain.TestRecipientFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "broadcast123",
			URL:         "https://example.com/recipient-feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response domain.TestRecipientFeedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response.Success)
		assert.Contains(t, response.Error, "connection timeout")
	})

	t.Run("MissingParams", func(t *testing.T) {
		// Missing broadcast_id
		reqBody := map[string]string{
			"workspace_id": "workspace123",
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", gomock.Any()).Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to decode request body")

		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer([]byte("{invalid")))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/broadcasts.testRecipientFeed", nil)
		w := httptest.NewRecorder()

		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("BroadcastNotFound", func(t *testing.T) {
		mockService.EXPECT().
			TestRecipientFeed(gomock.Any(), gomock.Any()).
			Return(nil, &domain.ErrBroadcastNotFound{ID: "nonexistent"})

		reqBody := domain.TestRecipientFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "nonexistent",
			URL:         "https://example.com/recipient-feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("ContactNotFound", func(t *testing.T) {
		mockService.EXPECT().
			TestRecipientFeed(gomock.Any(), gomock.Any()).
			Return(nil, &domain.ErrContactNotFoundForFeed{Email: "notfound@example.com"})

		reqBody := domain.TestRecipientFeedRequest{
			WorkspaceID:  "workspace123",
			BroadcastID:  "broadcast123",
			ContactEmail: "notfound@example.com",
			URL:          "https://example.com/recipient-feed",
			Headers:      []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"], "Contact not found")
	})

	t.Run("ServiceError", func(t *testing.T) {
		mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(mockLoggerWithField)
		mockLoggerWithField.EXPECT().Error("Failed to test recipient feed")

		mockService.EXPECT().
			TestRecipientFeed(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("service error"))

		reqBody := domain.TestRecipientFeedRequest{
			WorkspaceID: "workspace123",
			BroadcastID: "broadcast123",
			URL:         "https://example.com/recipient-feed",
			Headers:     []domain.DataFeedHeader{},
		}
		b, _ := json.Marshal(reqBody)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/broadcasts.testRecipientFeed", bytes.NewBuffer(b))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.HandleTestRecipientFeed(w, httpReq)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestMissingParameterError_Error(t *testing.T) {
	// Test MissingParameterError.Error - this was at 0% coverage
	t.Run("returns formatted error message", func(t *testing.T) {
		err := &http_handler.MissingParameterError{
			Param: "workspace_id",
		}
		expected := "Missing parameter: workspace_id"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("returns formatted error message with different param", func(t *testing.T) {
		err := &http_handler.MissingParameterError{
			Param: "broadcast_id",
		}
		expected := "Missing parameter: broadcast_id"
		assert.Equal(t, expected, err.Error())
	})
}
