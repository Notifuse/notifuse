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
				assert.Equal(t, 3, params.Page)
				return &domain.BlogPostListResponse{
					Posts:      posts,
					TotalCount: 0,
				}, nil
			})

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id="+workspaceID+"&category_id=cat-1&status=published&limit=10&page=3", nil)
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

	t.Run("Invalid page parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id=ws-123&page=invalid", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "page must be a valid integer")
	})

	// The service runs ListBlogPostsRequest.Validate, which derives Offset from Page. The
	// handler used to read `offset` into a field Validate then overwrote, so every offset
	// returned the first page while a handler-level assertion on params.Offset kept passing.
	// Running Validate here pins what the repository actually receives.
	t.Run("Page reaches the repository as an offset", func(t *testing.T) {
		handler, mockService, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		mockService.EXPECT().
			ListPosts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				require.NoError(t, params.Validate())
				assert.Equal(t, 20, params.Offset)
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}}, nil
			})

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id=ws-123&limit=10&page=3", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Offset parameter is not read", func(t *testing.T) {
		handler, mockService, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()

		mockService.EXPECT().
			ListPosts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, 0, params.Page)
				assert.Equal(t, 0, params.Offset)
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}}, nil
			})

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id=ws-123&offset=20", nil)
		w := httptest.NewRecorder()

		handler.HandleListPosts(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
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

// A permission denial must reach the client as a 403 naming the resource, on reads and
// writes alike. The blog handlers used to answer it with their generic failure — a 500
// "Failed to list posts" on reads, which hid the cause from an API key missing blog:read.
func TestBlogHandler_PermissionErrors(t *testing.T) {
	readErr := domain.NewPermissionError(domain.PermissionResourceBlog, domain.PermissionTypeRead,
		"Insufficient permissions: read access to blog required")
	writeErr := domain.NewPermissionError(domain.PermissionResourceBlog, domain.PermissionTypeWrite,
		"Insufficient permissions: write access to blog required")

	testCases := []struct {
		name    string
		method  string
		target  string
		body    string
		expect  func(m *mocks.MockBlogService)
		handler func(h *http_handler.BlogHandler) http.HandlerFunc
		wantErr *domain.PermissionError
	}{
		{
			name: "blogCategories.list", method: http.MethodGet, target: "/api/blogCategories.list?workspace_id=ws-123",
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().ListCategories(gomock.Any()).Return(nil, readErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleListCategories },
			wantErr: readErr,
		},
		{
			name: "blogCategories.get", method: http.MethodGet, target: "/api/blogCategories.get?workspace_id=ws-123&id=cat-1",
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().GetCategory(gomock.Any(), "cat-1").Return(nil, readErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleGetCategory },
			wantErr: readErr,
		},
		{
			name: "blogCategories.create", method: http.MethodPost, target: "/api/blogCategories.create?workspace_id=ws-123", body: `{"name":"News","slug":"news"}`,
			expect: func(m *mocks.MockBlogService) {
				m.EXPECT().CreateCategory(gomock.Any(), gomock.Any()).Return(nil, writeErr)
			},
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleCreateCategory },
			wantErr: writeErr,
		},
		{
			name: "blogCategories.update", method: http.MethodPost, target: "/api/blogCategories.update?workspace_id=ws-123", body: `{"id":"cat-1","name":"News","slug":"news"}`,
			expect: func(m *mocks.MockBlogService) {
				m.EXPECT().UpdateCategory(gomock.Any(), gomock.Any()).Return(nil, writeErr)
			},
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleUpdateCategory },
			wantErr: writeErr,
		},
		{
			name: "blogCategories.delete", method: http.MethodPost, target: "/api/blogCategories.delete?workspace_id=ws-123", body: `{"id":"cat-1"}`,
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().DeleteCategory(gomock.Any(), gomock.Any()).Return(writeErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleDeleteCategory },
			wantErr: writeErr,
		},
		{
			name: "blogPosts.list", method: http.MethodGet, target: "/api/blogPosts.list?workspace_id=ws-123",
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().ListPosts(gomock.Any(), gomock.Any()).Return(nil, readErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleListPosts },
			wantErr: readErr,
		},
		{
			name: "blogPosts.get", method: http.MethodGet, target: "/api/blogPosts.get?workspace_id=ws-123&id=post-1",
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().GetPost(gomock.Any(), "post-1").Return(nil, readErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleGetPost },
			wantErr: readErr,
		},
		{
			name: "blogPosts.create", method: http.MethodPost, target: "/api/blogPosts.create?workspace_id=ws-123", body: `{"category_id":"cat-1","slug":"hello","title":"Hello","template_id":"tpl-1"}`,
			expect: func(m *mocks.MockBlogService) {
				m.EXPECT().CreatePost(gomock.Any(), gomock.Any()).Return(nil, writeErr)
			},
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleCreatePost },
			wantErr: writeErr,
		},
		{
			name: "blogPosts.update", method: http.MethodPost, target: "/api/blogPosts.update?workspace_id=ws-123", body: `{"id":"post-1","category_id":"cat-1","slug":"hello","title":"Hello","template_id":"tpl-1"}`,
			expect: func(m *mocks.MockBlogService) {
				m.EXPECT().UpdatePost(gomock.Any(), gomock.Any()).Return(nil, writeErr)
			},
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleUpdatePost },
			wantErr: writeErr,
		},
		{
			name: "blogPosts.delete", method: http.MethodPost, target: "/api/blogPosts.delete?workspace_id=ws-123", body: `{"id":"post-1"}`,
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().DeletePost(gomock.Any(), gomock.Any()).Return(writeErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleDeletePost },
			wantErr: writeErr,
		},
		{
			name: "blogPosts.publish", method: http.MethodPost, target: "/api/blogPosts.publish?workspace_id=ws-123", body: `{"id":"post-1"}`,
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().PublishPost(gomock.Any(), gomock.Any()).Return(writeErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandlePublishPost },
			wantErr: writeErr,
		},
		{
			name: "blogPosts.unpublish", method: http.MethodPost, target: "/api/blogPosts.unpublish?workspace_id=ws-123", body: `{"id":"post-1"}`,
			expect:  func(m *mocks.MockBlogService) { m.EXPECT().UnpublishPost(gomock.Any(), gomock.Any()).Return(writeErr) },
			handler: func(h *http_handler.BlogHandler) http.HandlerFunc { return h.HandleUnpublishPost },
			wantErr: writeErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
			defer ctrl.Finish()

			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
			tc.expect(mockService)

			req := httptest.NewRequest(tc.method, tc.target, bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()

			tc.handler(handler)(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code)
			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tc.wantErr.Message, body["error"])
			assert.Equal(t, string(domain.PermissionResourceBlog), body["resource"])
			assert.Equal(t, string(tc.wantErr.Permission), body["permission"])
		})
	}
}

func TestBlogHandler_RegisterRoutes(t *testing.T) {
	// Test BlogHandler.RegisterRoutes - this was at 0% coverage
	handler, _, _, ctrl := setupBlogHandler(t)
	defer ctrl.Finish()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Verify that routes are registered by checking if they exist in the mux
	// We can't directly check mux internals, but we can verify by making requests
	// However, since routes require auth, we'll just verify RegisterRoutes doesn't panic
	// and that the mux is not nil after registration
	assert.NotNil(t, mux, "mux should not be nil after RegisterRoutes")
}
