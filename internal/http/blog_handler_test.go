package http_test

import (
	"bytes"
	"context"
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

// setupBlogHandler sets up a blog handler with mocks for testing
func setupBlogHandler(t *testing.T) (
	*http_handler.BlogHandler,
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
	handler := http_handler.NewBlogHandler(
		mockBlogService,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		false,
	)

	return handler, mockBlogService, mockLogger, ctrl
}

func TestBlogHandler_HandleListCategories(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		categories := []*domain.BlogCategory{
			{
				ID:   "cat-1",
				Slug: "test-category",
				Settings: domain.BlogCategorySettings{
					Name:        "Test Category",
					Description: "Test Description",
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		mockService.EXPECT().
			ListCategories(gomock.Any()).
			Return(&domain.BlogCategoryListResponse{
				Categories: categories,
				TotalCount: 1,
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.list?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleListCategories(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["categories"])
		assert.Equal(t, float64(1), response["total_count"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.list", nil)
		w := httptest.NewRecorder()

		handler.HandleListCategories(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.list?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleListCategories(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			ListCategories(gomock.Any()).
			Return(nil, errors.New("service error"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.list?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleListCategories(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestBlogHandler_HandleGetCategory(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success by ID", func(t *testing.T) {
		workspaceID := "ws-123"
		categoryID := "cat-1"
		category := &domain.BlogCategory{
			ID:   categoryID,
			Slug: "test-category",
			Settings: domain.BlogCategorySettings{
				Name:        "Test Category",
				Description: "Test Description",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			GetCategory(gomock.Any(), categoryID).
			Return(category, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.get?workspace_id="+workspaceID+"&id="+categoryID, nil)
		w := httptest.NewRecorder()

		handler.HandleGetCategory(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["category"])
	})

	t.Run("Success by slug", func(t *testing.T) {
		workspaceID := "ws-123"
		slug := "test-category"
		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: slug,
			Settings: domain.BlogCategorySettings{
				Name:        "Test Category",
				Description: "Test Description",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			GetCategoryBySlug(gomock.Any(), slug).
			Return(category, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.get?workspace_id="+workspaceID+"&slug="+slug, nil)
		w := httptest.NewRecorder()

		handler.HandleGetCategory(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.get?id=cat-1", nil)
		w := httptest.NewRecorder()

		handler.HandleGetCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing id and slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.get?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleGetCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.get?workspace_id=ws-123&id=cat-1", nil)
		w := httptest.NewRecorder()

		handler.HandleGetCategory(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleCreateCategory(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := domain.CreateBlogCategoryRequest{
			Name:        "New Category",
			Slug:        "new-category",
			Description: "New Description",
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: reqBody.Slug,
			Settings: domain.BlogCategorySettings{
				Name:        reqBody.Name,
				Description: reqBody.Description,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			CreateCategory(gomock.Any(), &reqBody).
			Return(category, nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.create?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleCreateCategory(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["category"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.create", nil)
		w := httptest.NewRecorder()

		handler.HandleCreateCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.create?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandleCreateCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := domain.CreateBlogCategoryRequest{
			Name: "New Category",
			Slug: "new-category",
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			CreateCategory(gomock.Any(), &reqBody).
			Return(nil, errors.New("validation error"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.create?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleCreateCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.create?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleCreateCategory(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleUpdateCategory(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := domain.UpdateBlogCategoryRequest{
			ID:          "cat-1",
			Name:        "Updated Category",
			Slug:        "updated-category",
			Description: "Updated Description",
		}

		category := &domain.BlogCategory{
			ID:   reqBody.ID,
			Slug: reqBody.Slug,
			Settings: domain.BlogCategorySettings{
				Name:        reqBody.Name,
				Description: reqBody.Description,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			UpdateCategory(gomock.Any(), &reqBody).
			Return(category, nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.update?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleUpdateCategory(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["category"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.update", nil)
		w := httptest.NewRecorder()

		handler.HandleUpdateCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.update?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandleUpdateCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.update?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleUpdateCategory(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleDeleteCategory(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := domain.DeleteBlogCategoryRequest{
			ID: "cat-1",
		}

		mockService.EXPECT().
			DeleteCategory(gomock.Any(), &reqBody).
			Return(nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.delete?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleDeleteCategory(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.delete", nil)
		w := httptest.NewRecorder()

		handler.HandleDeleteCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.delete?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandleDeleteCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := domain.DeleteBlogCategoryRequest{
			ID: "cat-1",
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			DeleteCategory(gomock.Any(), &reqBody).
			Return(errors.New("category not found"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogCategories.delete?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleDeleteCategory(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogCategories.delete?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleDeleteCategory(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleListPosts(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		catID := "cat-1"
		posts := []*domain.BlogPost{
			{
				ID:         "post-1",
				CategoryID: catID,
				Slug:       "test-post",
				Settings: domain.BlogPostSettings{
					Title: "Test Post",
					Template: domain.BlogPostTemplateReference{
						TemplateID:      "tpl-1",
						TemplateVersion: 1,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		mockService.EXPECT().
			ListPosts(gomock.Any(), gomock.Any()).
			Return(&domain.BlogPostListResponse{
				Posts:      posts,
				TotalCount: 1,
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["posts"])
		assert.Equal(t, float64(1), response["total_count"])
	})

	t.Run("Success with filters", func(t *testing.T) {
		workspaceID := "ws-123"
		posts := []*domain.BlogPost{}

		mockService.EXPECT().
			ListPosts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, "cat-1", params.CategoryID)
				assert.Equal(t, domain.BlogPostStatusPublished, params.Status)
				assert.Equal(t, 10, params.Limit)
				assert.Equal(t, 20, params.Offset)
				return &domain.BlogPostListResponse{
					Posts:      posts,
					TotalCount: 0,
				}, nil
			})

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id="+workspaceID+"&category_id=cat-1&status=published&limit=10&offset=20", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid limit parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id=ws-123&limit=invalid", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid offset parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id=ws-123&offset=invalid", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			ListPosts(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("service error"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id="+workspaceID, nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.list?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleGetPost(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success by ID", func(t *testing.T) {
		workspaceID := "ws-123"
		postID := "post-1"
		catID := "cat-1"
		post := &domain.BlogPost{
			ID:         postID,
			CategoryID: catID,
			Slug:       "test-post",
			Settings: domain.BlogPostSettings{
				Title: "Test Post",
				Template: domain.BlogPostTemplateReference{
					TemplateID:      "tpl-1",
					TemplateVersion: 1,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			GetPost(gomock.Any(), postID).
			Return(post, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.get?workspace_id="+workspaceID+"&id="+postID, nil)
		w := httptest.NewRecorder()

		handler.HandleGetPost(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["post"])
	})

	t.Run("Success by slug", func(t *testing.T) {
		workspaceID := "ws-123"
		slug := "test-post"
		catID := "cat-1"
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: catID,
			Slug:       slug,
			Settings: domain.BlogPostSettings{
				Title: "Test Post",
				Template: domain.BlogPostTemplateReference{
					TemplateID:      "tpl-1",
					TemplateVersion: 1,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			GetPostBySlug(gomock.Any(), slug).
			Return(post, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.get?workspace_id="+workspaceID+"&slug="+slug, nil)
		w := httptest.NewRecorder()

		handler.HandleGetPost(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Success by category slug and post slug", func(t *testing.T) {
		workspaceID := "ws-123"
		categorySlug := "category-slug"
		postSlug := "post-slug"
		catID := "cat-1"
		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: catID,
			Slug:       postSlug,
			Settings: domain.BlogPostSettings{
				Title: "Test Post",
				Template: domain.BlogPostTemplateReference{
					TemplateID:      "tpl-1",
					TemplateVersion: 1,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			GetPostByCategoryAndSlug(gomock.Any(), categorySlug, postSlug).
			Return(post, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.get?workspace_id="+workspaceID+"&category_slug="+categorySlug+"&slug="+postSlug, nil)
		w := httptest.NewRecorder()

		handler.HandleGetPost(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.get?id=post-1", nil)
		w := httptest.NewRecorder()

		handler.HandleGetPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing id and slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.get?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleGetPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		postID := "post-1"

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			GetPost(gomock.Any(), postID).
			Return(nil, errors.New("post not found"))

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.get?workspace_id="+workspaceID+"&id="+postID, nil)
		w := httptest.NewRecorder()

		handler.HandleGetPost(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.get?workspace_id=ws-123&id=post-1", nil)
		w := httptest.NewRecorder()

		handler.HandleGetPost(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleCreatePost(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		catID := "cat-1"
		reqBody := domain.CreateBlogPostRequest{
			CategoryID:      catID,
			Slug:            "new-post",
			Title:           "New Post",
			TemplateID:      "tpl-1",
			TemplateVersion: 1,
		}

		post := &domain.BlogPost{
			ID:         "post-1",
			CategoryID: reqBody.CategoryID,
			Slug:       reqBody.Slug,
			Settings: domain.BlogPostSettings{
				Title: reqBody.Title,
				Template: domain.BlogPostTemplateReference{
					TemplateID:      reqBody.TemplateID,
					TemplateVersion: reqBody.TemplateVersion,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			CreatePost(gomock.Any(), &reqBody).
			Return(post, nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.create?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleCreatePost(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["post"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.create", nil)
		w := httptest.NewRecorder()

		handler.HandleCreatePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.create?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandleCreatePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		catID := "cat-1"
		reqBody := domain.CreateBlogPostRequest{
			CategoryID: catID,
			Slug:       "new-post",
			Title:      "New Post",
			TemplateID: "tpl-1",
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			CreatePost(gomock.Any(), &reqBody).
			Return(nil, errors.New("validation error"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.create?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleCreatePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.create?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleCreatePost(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleUpdatePost(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		catID := "cat-1"
		reqBody := domain.UpdateBlogPostRequest{
			ID:              "post-1",
			CategoryID:      catID,
			Slug:            "updated-post",
			Title:           "Updated Post",
			TemplateID:      "tpl-1",
			TemplateVersion: 2,
		}

		post := &domain.BlogPost{
			ID:         reqBody.ID,
			CategoryID: reqBody.CategoryID,
			Slug:       reqBody.Slug,
			Settings: domain.BlogPostSettings{
				Title: reqBody.Title,
				Template: domain.BlogPostTemplateReference{
					TemplateID:      reqBody.TemplateID,
					TemplateVersion: reqBody.TemplateVersion,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockService.EXPECT().
			UpdatePost(gomock.Any(), &reqBody).
			Return(post, nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.update?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleUpdatePost(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response["post"])
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.update", nil)
		w := httptest.NewRecorder()

		handler.HandleUpdatePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.update?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandleUpdatePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.update?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleUpdatePost(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleDeletePost(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := domain.DeleteBlogPostRequest{
			ID: "post-1",
		}

		mockService.EXPECT().
			DeletePost(gomock.Any(), &reqBody).
			Return(nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.delete?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleDeletePost(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.delete", nil)
		w := httptest.NewRecorder()

		handler.HandleDeletePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.delete?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandleDeletePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := domain.DeleteBlogPostRequest{
			ID: "post-1",
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			DeletePost(gomock.Any(), &reqBody).
			Return(errors.New("post not found"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.delete?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleDeletePost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.delete?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleDeletePost(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandlePublishPost(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := domain.PublishBlogPostRequest{
			ID: "post-1",
		}

		mockService.EXPECT().
			PublishPost(gomock.Any(), &reqBody).
			Return(nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.publish?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandlePublishPost(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.publish", nil)
		w := httptest.NewRecorder()

		handler.HandlePublishPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.publish?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandlePublishPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := domain.PublishBlogPostRequest{
			ID: "post-1",
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			PublishPost(gomock.Any(), &reqBody).
			Return(errors.New("cannot publish post"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.publish?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandlePublishPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.publish?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandlePublishPost(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestBlogHandler_HandleUnpublishPost(t *testing.T) {
	handler, mockService, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	t.Run("Success", func(t *testing.T) {
		workspaceID := "ws-123"
		reqBody := domain.UnpublishBlogPostRequest{
			ID: "post-1",
		}

		mockService.EXPECT().
			UnpublishPost(gomock.Any(), &reqBody).
			Return(nil)

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.unpublish?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleUnpublishPost(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.unpublish", nil)
		w := httptest.NewRecorder()

		handler.HandleUnpublishPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.unpublish?workspace_id=ws-123", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.HandleUnpublishPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		workspaceID := "ws-123"
		reqBody := domain.UnpublishBlogPostRequest{
			ID: "post-1",
		}

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		mockService.EXPECT().
			UnpublishPost(gomock.Any(), &reqBody).
			Return(errors.New("cannot unpublish post"))

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/blogPosts.unpublish?workspace_id="+workspaceID, bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleUnpublishPost(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.unpublish?workspace_id=ws-123", nil)
		w := httptest.NewRecorder()

		handler.HandleUnpublishPost(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
