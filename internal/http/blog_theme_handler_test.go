package http_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	http_handler "github.com/Notifuse/notifuse/internal/http"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupBlogThemeHandler sets up a blog theme handler with mocks for testing
func setupBlogThemeHandler(t *testing.T) (
	*http_handler.BlogThemeHandler,
	*mocks.MockBlogService,
	*pkgmocks.MockLogger,
	*gomock.Controller,
) {
	ctrl := gomock.NewController(t)

	// Create mocks
	mockBlogService := mocks.NewMockBlogService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Create a JWT secret for authentication
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	// Create the handler with mocks
	handler := http_handler.NewBlogThemeHandler(
		mockBlogService,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
	)

	return handler, mockBlogService, mockLogger, ctrl
}

func TestBlogThemeHandler_HandleCreate(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogThemeHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := map[string]interface{}{
			"files": map[string]interface{}{
				"home":     "home template",
				"category": "category template",
				"post":     "post template",
				"header":   "header template",
				"footer":   "footer template",
				"shared":   "shared template",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				Home:     "home template",
				Category: "category template",
				Post:     "post template",
				Header:   "header template",
				Footer:   "footer template",
				Shared:   "shared template",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			CreateTheme(gomock.Any(), gomock.Any()).
			Return(theme, nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.create?workspace_id="+workspaceID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["theme"])
		themeData := response["theme"].(map[string]interface{})
		assert.Equal(t, float64(1), themeData["version"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		reqBody := map[string]interface{}{
			"files": map[string]interface{}{},
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		// Service will be called and should return an error for missing workspace_id
		mockService.EXPECT().
			CreateTheme(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("workspace_id not found in context"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.create", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.create?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		handler, _, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.create?workspace_id=ws-123", bytes.NewBufferString("invalid json"))
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := map[string]interface{}{
			"files": map[string]interface{}{},
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			CreateTheme(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("service error"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.create?workspace_id="+workspaceID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.HandleCreate(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestBlogThemeHandler_HandleGet(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogThemeHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		version := 1

		theme := &domain.BlogTheme{
			Version: version,
			Files: domain.BlogThemeFiles{
				Home: "home template",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			GetTheme(gomock.Any(), 1).
			Return(theme, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.get?workspace_id="+workspaceID+"&version=1", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["theme"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		// Service will be called and should return an error for missing workspace_id
		mockService.EXPECT().
			GetTheme(gomock.Any(), 1).
			Return(nil, errors.New("workspace_id not found in context"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.get?version=1", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Missing version", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.get?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid version format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.get?workspace_id=ws-123&version=invalid", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Theme not found", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			GetTheme(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("blog theme not found"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.get?workspace_id="+workspaceID+"&version=999", nil)
		w := httptest.NewRecorder()

		handler.HandleGet(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestBlogThemeHandler_HandleGetPublished(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogThemeHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		publishedTime := time.Now()

		theme := &domain.BlogTheme{
			Version:     2,
			PublishedAt: &publishedTime,
			Files: domain.BlogThemeFiles{
				Home: "published home",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			GetPublishedTheme(gomock.Any()).
			Return(theme, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.getPublished?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleGetPublished(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["theme"])
		themeData := response["theme"].(map[string]interface{})
		assert.Equal(t, float64(2), themeData["version"])
		assert.NotNil(t, themeData["published_at"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		// Service will be called and should return an error for missing workspace_id
		mockService.EXPECT().
			GetPublishedTheme(gomock.Any()).
			Return(nil, errors.New("workspace_id not found in context"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.getPublished", nil)
		w := httptest.NewRecorder()

		handler.HandleGetPublished(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("No published theme", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			GetPublishedTheme(gomock.Any()).
			Return(nil, errors.New("no published blog theme found"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.getPublished?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleGetPublished(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestBlogThemeHandler_HandleUpdate(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogThemeHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := map[string]interface{}{
			"version": 1,
			"files": map[string]interface{}{
				"home": "updated home",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				Home: "updated home",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			UpdateTheme(gomock.Any(), gomock.Any()).
			Return(theme, nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.update?workspace_id="+workspaceID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["theme"])
	})

	t.Run("Cannot update published theme", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := map[string]interface{}{
			"version": 1,
			"files": map[string]interface{}{
				"home": "updated",
			},
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			UpdateTheme(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("cannot update a published theme"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.update?workspace_id="+workspaceID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.HandleUpdate(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestBlogThemeHandler_HandlePublish(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogThemeHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := map[string]interface{}{
			"version": 1,
		}

		mockService.EXPECT().
			PublishTheme(gomock.Any(), gomock.Any()).
			Return(nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.publish?workspace_id="+workspaceID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.HandlePublish(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, true, response["success"])
		assert.NotNil(t, response["message"])
	})

	t.Run("Theme not found", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := map[string]interface{}{
			"version": 999,
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			PublishTheme(gomock.Any(), gomock.Any()).
			Return(errors.New("blog theme not found"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogThemes.publish?workspace_id="+workspaceID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.HandlePublish(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestBlogThemeHandler_HandleList(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogThemeHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		publishedTime := time.Now()

		themes := []*domain.BlogTheme{
			{
				Version:     2,
				PublishedAt: &publishedTime,
				Files:       domain.BlogThemeFiles{Home: "v2"},
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			{
				Version:     1,
				PublishedAt: nil,
				Files:       domain.BlogThemeFiles{Home: "v1"},
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		mockService.EXPECT().
			ListThemes(gomock.Any(), gomock.Any()).
			Return(&domain.BlogThemeListResponse{
				Themes:     themes,
				TotalCount: 2,
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.list?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleList(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["themes"])
		assert.Equal(t, float64(2), response["total_count"])
	})

	t.Run("With pagination", func(t *testing.T) {
		workspaceID := "ws-123"

		themes := []*domain.BlogTheme{
			{Version: 1, Files: domain.BlogThemeFiles{}},
		}

		mockService.EXPECT().
			ListThemes(gomock.Any(), gomock.Any()).
			Return(&domain.BlogThemeListResponse{
				Themes:     themes,
				TotalCount: 100,
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.list?workspace_id="+workspaceID+"&limit=10&offset=5", nil)
		w := httptest.NewRecorder()

		handler.HandleList(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		// Service will be called and should return an error for missing workspace_id
		mockService.EXPECT().
			ListThemes(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("workspace_id not found in context"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.list", nil)
		w := httptest.NewRecorder()

		handler.HandleList(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogThemeHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			ListThemes(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("service error"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogThemes.list?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleList(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

