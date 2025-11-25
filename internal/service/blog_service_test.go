package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func setupBlogServiceTest(t *testing.T) (
	*BlogService,
	*mocks.MockBlogCategoryRepository,
	*mocks.MockBlogPostRepository,
	*mocks.MockBlogThemeRepository,
	*mocks.MockWorkspaceRepository,
	*mocks.MockListRepository,
	*mocks.MockTemplateRepository,
	*mocks.MockAuthService,
) {
	ctrl := gomock.NewController(t)

	mockCategoryRepo := mocks.NewMockBlogCategoryRepository(ctrl)
	mockPostRepo := mocks.NewMockBlogPostRepository(ctrl)
	mockThemeRepo := mocks.NewMockBlogThemeRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockListRepo := mocks.NewMockListRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := logger.NewLoggerWithLevel("disabled")
	testCache := cache.NewInMemoryCache(30 * time.Second)

	service := NewBlogService(
		mockLogger,
		mockCategoryRepo,
		mockPostRepo,
		mockThemeRepo,
		mockWorkspaceRepo,
		mockListRepo,
		mockTemplateRepo,
		mockAuthService,
		testCache,
	)

	return service, mockCategoryRepo, mockPostRepo, mockThemeRepo, mockWorkspaceRepo, mockListRepo, mockTemplateRepo, mockAuthService
}

// setupBlogContextWithAuth creates a context with workspace_id and mocks authentication with permissions
func setupBlogContextWithAuth(mockAuthService *mocks.MockAuthService, workspaceID string, readPerm, writePerm bool) context.Context {
	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, workspaceID)

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBlog: domain.ResourcePermissions{
				Read:  readPerm,
				Write: writePerm,
			},
		},
	}

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil).
		Times(1)

	return ctx
}

func TestBlogService_CreateCategory(t *testing.T) {
	service, mockCategoryRepo, _, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful creation", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogCategoryRequest{
			Name:        "Tech Blog",
			Slug:        "tech-blog",
			Description: "Technology articles",
		}

		// Mock slug check - not found
		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, req.Slug).
			Return(nil, errors.New("not found"))

		// Mock create
		mockCategoryRepo.EXPECT().
			CreateCategory(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, cat *domain.BlogCategory) error {
				assert.Equal(t, req.Name, cat.Settings.Name)
				assert.Equal(t, req.Slug, cat.Slug)
				assert.Equal(t, req.Description, cat.Settings.Description)
				assert.NotEmpty(t, cat.ID)
				return nil
			})

		category, err := service.CreateCategory(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, category)
		assert.Equal(t, req.Name, category.Settings.Name)
		assert.Equal(t, req.Slug, category.Slug)
	})

	t.Run("validation error - missing name", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogCategoryRequest{
			Slug: "tech-blog",
		}

		category, err := service.CreateCategory(ctx, req)
		require.Error(t, err)
		assert.Nil(t, category)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("slug already exists", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogCategoryRequest{
			Name: "Tech Blog",
			Slug: "tech-blog",
		}

		existingCategory := &domain.BlogCategory{
			ID:   "existing123",
			Slug: req.Slug,
		}

		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, req.Slug).
			Return(existingCategory, nil)

		category, err := service.CreateCategory(ctx, req)
		require.Error(t, err)
		assert.Nil(t, category)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("repository error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogCategoryRequest{
			Name: "Tech Blog",
			Slug: "tech-blog",
		}

		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, req.Slug).
			Return(nil, errors.New("not found"))

		mockCategoryRepo.EXPECT().
			CreateCategory(ctx, gomock.Any()).
			Return(errors.New("database error"))

		category, err := service.CreateCategory(ctx, req)
		require.Error(t, err)
		assert.Nil(t, category)
		assert.Contains(t, err.Error(), "failed to create category")
	})
}

func TestBlogService_GetCategory(t *testing.T) {
	service, mockCategoryRepo, _, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful retrieval", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedCategory := &domain.BlogCategory{
			ID:   "cat123",
			Slug: "tech-blog",
			Settings: domain.BlogCategorySettings{
				Name: "Tech Blog",
			},
		}

		mockCategoryRepo.EXPECT().
			GetCategory(ctx, "cat123").
			Return(expectedCategory, nil)

		category, err := service.GetCategory(ctx, "cat123")
		require.NoError(t, err)
		assert.Equal(t, expectedCategory, category)
	})

	t.Run("category not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, "nonexistent").
			Return(nil, errors.New("not found"))

		category, err := service.GetCategory(ctx, "nonexistent")
		require.Error(t, err)
		assert.Nil(t, category)
	})
}

func TestBlogService_GetCategoryBySlug(t *testing.T) {
	service, mockCategoryRepo, _, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful retrieval", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedCategory := &domain.BlogCategory{
			ID:   "cat123",
			Slug: "tech-blog",
		}

		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, "tech-blog").
			Return(expectedCategory, nil)

		category, err := service.GetCategoryBySlug(ctx, "tech-blog")
		require.NoError(t, err)
		assert.Equal(t, expectedCategory, category)
	})
}

func TestBlogService_GetPublicCategoryBySlug(t *testing.T) {
	service, mockCategoryRepo, _, _, _, _, _, _ := setupBlogServiceTest(t)

	t.Run("successful retrieval without authentication", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		expectedCategory := &domain.BlogCategory{
			ID:   "cat123",
			Slug: "tech-blog",
			Settings: domain.BlogCategorySettings{
				Name: "Tech Blog",
			},
		}

		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, "tech-blog").
			Return(expectedCategory, nil)

		category, err := service.GetPublicCategoryBySlug(ctx, "tech-blog")
		require.NoError(t, err)
		assert.Equal(t, expectedCategory, category)
	})

	t.Run("returns error when category not found", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, "nonexistent").
			Return(nil, errors.New("category not found"))

		category, err := service.GetPublicCategoryBySlug(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, category)
	})

	t.Run("returns error when workspace_id missing from context", func(t *testing.T) {
		ctx := context.Background()

		// GetPublicCategoryBySlug doesn't check for workspace_id, it just calls GetCategoryBySlug
		// So we need to mock GetCategoryBySlug even without workspace_id
		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, "tech-blog").
			Return(nil, errors.New("category not found"))

		category, err := service.GetPublicCategoryBySlug(ctx, "tech-blog")
		assert.Error(t, err)
		assert.Nil(t, category)
	})
}

func TestBlogService_UpdateCategory(t *testing.T) {
	service, mockCategoryRepo, _, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful update", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogCategoryRequest{
			ID:          "cat123",
			Name:        "Updated Name",
			Slug:        "updated-slug",
			Description: "Updated description",
		}

		existingCategory := &domain.BlogCategory{
			ID:   "cat123",
			Slug: "old-slug",
			Settings: domain.BlogCategorySettings{
				Name: "Old Name",
			},
		}

		mockCategoryRepo.EXPECT().
			GetCategory(ctx, req.ID).
			Return(existingCategory, nil)

		// Mock slug check for new slug
		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, req.Slug).
			Return(nil, errors.New("not found"))

		mockCategoryRepo.EXPECT().
			UpdateCategory(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, cat *domain.BlogCategory) error {
				assert.Equal(t, req.Name, cat.Settings.Name)
				assert.Equal(t, req.Slug, cat.Slug)
				return nil
			})

		category, err := service.UpdateCategory(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, category)
		assert.Equal(t, req.Name, category.Settings.Name)
	})

	t.Run("validation error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogCategoryRequest{
			ID:   "cat123",
			Name: "Updated Name",
			// Missing slug
		}

		category, err := service.UpdateCategory(ctx, req)
		require.Error(t, err)
		assert.Nil(t, category)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("category not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogCategoryRequest{
			ID:   "nonexistent",
			Name: "Updated Name",
			Slug: "updated-slug",
		}

		mockCategoryRepo.EXPECT().
			GetCategory(ctx, req.ID).
			Return(nil, errors.New("not found"))

		category, err := service.UpdateCategory(ctx, req)
		require.Error(t, err)
		assert.Nil(t, category)
		assert.Contains(t, err.Error(), "category not found")
	})

	t.Run("new slug already exists", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogCategoryRequest{
			ID:   "cat123",
			Name: "Updated Name",
			Slug: "existing-slug",
		}

		existingCategory := &domain.BlogCategory{
			ID:   "cat123",
			Slug: "old-slug",
		}

		anotherCategory := &domain.BlogCategory{
			ID:   "cat456",
			Slug: "existing-slug",
		}

		mockCategoryRepo.EXPECT().
			GetCategory(ctx, req.ID).
			Return(existingCategory, nil)

		mockCategoryRepo.EXPECT().
			GetCategoryBySlug(ctx, req.Slug).
			Return(anotherCategory, nil)

		category, err := service.UpdateCategory(ctx, req)
		require.Error(t, err)
		assert.Nil(t, category)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestBlogService_DeleteCategory(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful deletion with cascade", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogCategoryRequest{
			ID: "cat123",
		}

		// Mock the transaction execution
		mockCategoryRepo.EXPECT().
			WithTransaction(ctx, "workspace123", gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				// Call the function with a nil tx (we don't actually need it in the test)
				return fn(nil)
			})

		// Mock the cascade delete of posts
		mockPostRepo.EXPECT().
			DeletePostsByCategoryIDTx(ctx, nil, req.ID).
			Return(int64(3), nil) // 3 posts deleted

		// Mock the category deletion
		mockCategoryRepo.EXPECT().
			DeleteCategoryTx(ctx, nil, req.ID).
			Return(nil)

		err := service.DeleteCategory(ctx, req)
		require.NoError(t, err)
	})

	t.Run("successful deletion with no posts", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogCategoryRequest{
			ID: "cat123",
		}

		mockCategoryRepo.EXPECT().
			WithTransaction(ctx, "workspace123", gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// No posts to delete
		mockPostRepo.EXPECT().
			DeletePostsByCategoryIDTx(ctx, nil, req.ID).
			Return(int64(0), nil)

		mockCategoryRepo.EXPECT().
			DeleteCategoryTx(ctx, nil, req.ID).
			Return(nil)

		err := service.DeleteCategory(ctx, req)
		require.NoError(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogCategoryRequest{}

		err := service.DeleteCategory(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("error deleting posts", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogCategoryRequest{
			ID: "cat123",
		}

		mockCategoryRepo.EXPECT().
			WithTransaction(ctx, "workspace123", gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Error when deleting posts
		mockPostRepo.EXPECT().
			DeletePostsByCategoryIDTx(ctx, nil, req.ID).
			Return(int64(0), errors.New("database error"))

		err := service.DeleteCategory(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete posts")
	})

	t.Run("error deleting category", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogCategoryRequest{
			ID: "cat123",
		}

		mockCategoryRepo.EXPECT().
			WithTransaction(ctx, "workspace123", gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		mockPostRepo.EXPECT().
			DeletePostsByCategoryIDTx(ctx, nil, req.ID).
			Return(int64(2), nil)

		// Error when deleting category
		mockCategoryRepo.EXPECT().
			DeleteCategoryTx(ctx, nil, req.ID).
			Return(errors.New("database error"))

		err := service.DeleteCategory(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete category")
	})

	t.Run("transaction error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogCategoryRequest{
			ID: "cat123",
		}

		// Transaction itself fails
		mockCategoryRepo.EXPECT().
			WithTransaction(ctx, "workspace123", gomock.Any()).
			Return(errors.New("transaction error"))

		err := service.DeleteCategory(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction error")
	})
}

func TestBlogService_ListCategories(t *testing.T) {
	service, mockCategoryRepo, _, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful listing", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedCategories := []*domain.BlogCategory{
			{ID: "cat1", Slug: "tech"},
			{ID: "cat2", Slug: "news"},
		}

		mockCategoryRepo.EXPECT().
			ListCategories(ctx).
			Return(expectedCategories, nil)

		result, err := service.ListCategories(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount)
		assert.Len(t, result.Categories, 2)
	})

	t.Run("empty list", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		mockCategoryRepo.EXPECT().
			ListCategories(ctx).
			Return([]*domain.BlogCategory{}, nil)

		result, err := service.ListCategories(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, result.TotalCount)
	})
}

func TestBlogService_CreatePost(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	categoryID := "cat123"

	t.Run("successful creation", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogPostRequest{
			CategoryID: categoryID,
			Slug:       "my-first-post",
			Title:      "My First Post",
			TemplateID: "tpl123",
			Authors:    []domain.BlogAuthor{{Name: "John"}},
		}

		// Mock slug check
		mockPostRepo.EXPECT().
			GetPostBySlug(ctx, req.Slug).
			Return(nil, errors.New("not found"))

		// Mock category check
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, categoryID).
			Return(&domain.BlogCategory{ID: categoryID}, nil)

		// Mock create
		mockPostRepo.EXPECT().
			CreatePost(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, post *domain.BlogPost) error {
				assert.Equal(t, req.Title, post.Settings.Title)
				assert.Equal(t, req.Slug, post.Slug)
				assert.NotEmpty(t, post.ID)
				assert.Nil(t, post.PublishedAt) // Draft by default
				return nil
			})

		post, err := service.CreatePost(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, post)
		assert.Equal(t, req.Title, post.Settings.Title)
		assert.True(t, post.IsDraft())
	})

	t.Run("validation error - missing category", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogPostRequest{
			Slug:       "my-post",
			Title:      "My Post",
			TemplateID: "tpl123",
			// Missing category_id
		}

		post, err := service.CreatePost(ctx, req)
		require.Error(t, err)
		assert.Nil(t, post)
		assert.Contains(t, err.Error(), "category_id is required")
	})

	t.Run("slug already exists", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogPostRequest{
			CategoryID: categoryID,
			Slug:       "existing-post",
			Title:      "My Post",
			TemplateID: "tpl123",
		}

		existingPost := &domain.BlogPost{
			ID:   "post123",
			Slug: req.Slug,
		}

		mockPostRepo.EXPECT().
			GetPostBySlug(ctx, req.Slug).
			Return(existingPost, nil)

		post, err := service.CreatePost(ctx, req)
		require.Error(t, err)
		assert.Nil(t, post)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("category not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogPostRequest{
			CategoryID: categoryID,
			Slug:       "my-post",
			Title:      "My Post",
			TemplateID: "tpl123",
		}

		mockPostRepo.EXPECT().
			GetPostBySlug(ctx, req.Slug).
			Return(nil, errors.New("not found"))

		mockCategoryRepo.EXPECT().
			GetCategory(ctx, categoryID).
			Return(nil, errors.New("not found"))

		post, err := service.CreatePost(ctx, req)
		require.Error(t, err)
		assert.Nil(t, post)
		assert.Contains(t, err.Error(), "category not found")
	})
}

func TestBlogService_GetPost(t *testing.T) {
	service, _, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful retrieval", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedPost := &domain.BlogPost{
			ID:   "post123",
			Slug: "my-post",
		}

		mockPostRepo.EXPECT().
			GetPost(ctx, "post123").
			Return(expectedPost, nil)

		post, err := service.GetPost(ctx, "post123")
		require.NoError(t, err)
		assert.Equal(t, expectedPost, post)
	})
}

func TestBlogService_UpdatePost(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, _, mockWorkspaceRepo, _, _, mockAuthService := setupBlogServiceTest(t)

	categoryID := "cat123"

	t.Run("successful update", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogPostRequest{
			ID:         "post123",
			CategoryID: categoryID,
			Slug:       "updated-post",
			Title:      "Updated Title",
			TemplateID: "tpl123",
		}

		existingPost := &domain.BlogPost{
			ID:         "post123",
			Slug:       "old-post",
			CategoryID: categoryID,
		}

		oldCategory := &domain.BlogCategory{
			ID:   categoryID,
			Slug: "old-category",
		}

		newCategory := &domain.BlogCategory{
			ID:   categoryID,
			Slug: "new-category",
		}

		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(existingPost, nil)

		// Mock old category fetch for cache invalidation
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, categoryID).
			Return(oldCategory, nil)

		// Mock slug check
		mockPostRepo.EXPECT().
			GetPostBySlug(ctx, req.Slug).
			Return(nil, errors.New("not found"))

		// Mock new category check
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, categoryID).
			Return(newCategory, nil)

		mockPostRepo.EXPECT().
			UpdatePost(ctx, gomock.Any()).
			Return(nil)

		// GetByID is called by invalidateBlogCaches (uses context.Background())
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), "workspace123").
			Return(workspace, nil).
			Times(2) // Called twice: once for old post invalidation, once for new post invalidation

		// ListCategories is called when invalidateAllCategories is true
		mockCategoryRepo.EXPECT().
			ListCategories(gomock.Any()).
			Return([]*domain.BlogCategory{newCategory}, nil)

		post, err := service.UpdatePost(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, post)
		assert.Equal(t, req.Title, post.Settings.Title)
	})

	t.Run("post not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogPostRequest{
			ID:         "nonexistent",
			CategoryID: categoryID,
			Slug:       "my-post",
			Title:      "My Post",
			TemplateID: "tpl123",
		}

		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(nil, errors.New("not found"))

		post, err := service.UpdatePost(ctx, req)
		require.Error(t, err)
		assert.Nil(t, post)
		assert.Contains(t, err.Error(), "post not found")
	})

	t.Run("new slug already exists", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogPostRequest{
			ID:         "post123",
			CategoryID: categoryID,
			Slug:       "existing-slug",
			Title:      "My Post",
			TemplateID: "tpl123",
		}

		existingPost := &domain.BlogPost{
			ID:   "post123",
			Slug: "old-slug",
		}

		anotherPost := &domain.BlogPost{
			ID:   "post456",
			Slug: "existing-slug",
		}

		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(existingPost, nil)

		mockPostRepo.EXPECT().
			GetPostBySlug(ctx, req.Slug).
			Return(anotherPost, nil)

		post, err := service.UpdatePost(ctx, req)
		require.Error(t, err)
		assert.Nil(t, post)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestBlogService_DeletePost(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, _, mockWorkspaceRepo, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful deletion", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogPostRequest{
			ID: "post123",
		}

		post := &domain.BlogPost{
			ID:         "post123",
			CategoryID: "cat-1",
			Slug:       "my-post",
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
		}

		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		// GetPost is called before DeletePost to get category ID for cache invalidation
		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(post, nil)

		// GetCategory is called to get category slug for cache invalidation
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, post.CategoryID).
			Return(category, nil)

		mockPostRepo.EXPECT().
			DeletePost(ctx, req.ID).
			Return(nil)

		// GetByID is called by invalidateBlogCaches (uses context.Background())
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), "workspace123").
			Return(workspace, nil)

		err := service.DeletePost(ctx, req)
		require.NoError(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogPostRequest{}

		err := service.DeletePost(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("post not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogPostRequest{
			ID: "nonexistent",
		}

		// GetPost is called before DeletePost to get category ID for cache invalidation
		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(nil, errors.New("not found"))

		err := service.DeletePost(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "post not found")
	})
}

func TestBlogService_ListPosts(t *testing.T) {
	service, _, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful listing", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		params := &domain.ListBlogPostsRequest{
			Status: domain.BlogPostStatusAll,
			Limit:  50,
		}

		expectedResponse := &domain.BlogPostListResponse{
			Posts: []*domain.BlogPost{
				{ID: "post1", Slug: "first"},
				{ID: "post2", Slug: "second"},
			},
			TotalCount: 2,
		}

		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, p domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				// Verify the request was validated (Page should be 1, Offset should be 0)
				assert.Equal(t, 1, p.Page)
				assert.Equal(t, 50, p.Limit)
				assert.Equal(t, 0, p.Offset)
				return expectedResponse, nil
			})

		result, err := service.ListPosts(ctx, params)
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount)
		assert.Len(t, result.Posts, 2)
	})

	t.Run("validation error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		params := &domain.ListBlogPostsRequest{
			Status: "invalid",
		}

		result, err := service.ListPosts(ctx, params)
		require.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestBlogService_PublishPost(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, _, mockWorkspaceRepo, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful publish", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.PublishBlogPostRequest{
			ID: "post123",
		}

		post := &domain.BlogPost{
			ID:         "post123",
			CategoryID: "cat-1",
			Slug:       "my-post",
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
		}

		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		// GetPost is called before PublishPost to get category ID for cache invalidation
		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(post, nil)

		// GetCategory is called to get category slug for cache invalidation
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, post.CategoryID).
			Return(category, nil)

		mockPostRepo.EXPECT().
			PublishPost(ctx, req.ID).
			Return(nil)

		// GetByID is called by invalidateBlogCaches (uses context.Background())
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), "workspace123").
			Return(workspace, nil)

		err := service.PublishPost(ctx, req)
		require.NoError(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.PublishBlogPostRequest{}

		err := service.PublishPost(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("repository error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.PublishBlogPostRequest{
			ID: "post123",
		}

		post := &domain.BlogPost{
			ID:         "post123",
			CategoryID: "cat-1",
			Slug:       "my-post",
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
		}

		// GetPost is called before PublishPost to get category ID for cache invalidation
		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(post, nil)

		// GetCategory is called to get category slug for cache invalidation
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, post.CategoryID).
			Return(category, nil)

		mockPostRepo.EXPECT().
			PublishPost(ctx, req.ID).
			Return(errors.New("already published"))

		err := service.PublishPost(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to publish post")
	})
}

func TestBlogService_UnpublishPost(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, _, mockWorkspaceRepo, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful unpublish", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UnpublishBlogPostRequest{
			ID: "post123",
		}

		post := &domain.BlogPost{
			ID:         "post123",
			CategoryID: "cat-1",
			Slug:       "my-post",
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
		}

		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		// GetPost is called before UnpublishPost to get category ID for cache invalidation
		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(post, nil)

		// GetCategory is called to get category slug for cache invalidation
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, post.CategoryID).
			Return(category, nil)

		mockPostRepo.EXPECT().
			UnpublishPost(ctx, req.ID).
			Return(nil)

		// GetByID is called by invalidateBlogCaches (uses context.Background())
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), "workspace123").
			Return(workspace, nil)

		err := service.UnpublishPost(ctx, req)
		require.NoError(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UnpublishBlogPostRequest{}

		err := service.UnpublishPost(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestBlogService_GetPublicPostByCategoryAndSlug(t *testing.T) {
	service, _, mockPostRepo, _, _, _, _, _ := setupBlogServiceTest(t)
	ctx := context.Background()

	t.Run("published post", func(t *testing.T) {
		now := time.Now()
		expectedPost := &domain.BlogPost{
			ID:          "post123",
			Slug:        "my-post",
			PublishedAt: &now,
		}

		mockPostRepo.EXPECT().
			GetPostByCategoryAndSlug(ctx, "tech", "my-post").
			Return(expectedPost, nil)

		post, err := service.GetPublicPostByCategoryAndSlug(ctx, "tech", "my-post")
		require.NoError(t, err)
		assert.Equal(t, expectedPost, post)
	})

	t.Run("draft post - should not be accessible", func(t *testing.T) {
		draftPost := &domain.BlogPost{
			ID:          "post123",
			Slug:        "my-draft",
			PublishedAt: nil,
		}

		mockPostRepo.EXPECT().
			GetPostByCategoryAndSlug(ctx, "tech", "my-draft").
			Return(draftPost, nil)

		post, err := service.GetPublicPostByCategoryAndSlug(ctx, "tech", "my-draft")
		require.Error(t, err)
		assert.Nil(t, post)
		assert.Contains(t, err.Error(), "post not found")
	})

	t.Run("post not found", func(t *testing.T) {
		mockPostRepo.EXPECT().
			GetPostByCategoryAndSlug(ctx, "tech", "nonexistent").
			Return(nil, errors.New("not found"))

		post, err := service.GetPublicPostByCategoryAndSlug(ctx, "tech", "nonexistent")
		require.Error(t, err)
		assert.Nil(t, post)
	})
}

func TestBlogService_ListPublicPosts(t *testing.T) {
	service, _, mockPostRepo, _, _, _, _, _ := setupBlogServiceTest(t)
	ctx := context.Background()

	t.Run("successful listing - only published", func(t *testing.T) {
		params := &domain.ListBlogPostsRequest{
			Status: domain.BlogPostStatusAll, // Will be forced to published
			Limit:  50,
		}

		now := time.Now()
		expectedResponse := &domain.BlogPostListResponse{
			Posts: []*domain.BlogPost{
				{ID: "post1", PublishedAt: &now},
				{ID: "post2", PublishedAt: &now},
			},
			TotalCount: 2,
		}

		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, p domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				// Verify status was forced to published
				assert.Equal(t, domain.BlogPostStatusPublished, p.Status)
				return expectedResponse, nil
			})

		result, err := service.ListPublicPosts(ctx, params)
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount)
	})

	t.Run("repository error", func(t *testing.T) {
		params := &domain.ListBlogPostsRequest{
			Limit: 50,
		}

		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			Return(nil, errors.New("database error"))

		result, err := service.ListPublicPosts(ctx, params)
		require.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestBlogService_GetPostBySlug(t *testing.T) {
	service, _, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful retrieval", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedPost := &domain.BlogPost{
			ID:   "post123",
			Slug: "my-post",
		}

		mockPostRepo.EXPECT().
			GetPostBySlug(ctx, "my-post").
			Return(expectedPost, nil)

		post, err := service.GetPostBySlug(ctx, "my-post")
		require.NoError(t, err)
		assert.Equal(t, expectedPost, post)
	})
}

func TestBlogService_GetPostByCategoryAndSlug(t *testing.T) {
	service, _, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful retrieval", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedPost := &domain.BlogPost{
			ID:   "post123",
			Slug: "my-post",
		}

		mockPostRepo.EXPECT().
			GetPostByCategoryAndSlug(ctx, "tech", "my-post").
			Return(expectedPost, nil)

		post, err := service.GetPostByCategoryAndSlug(ctx, "tech", "my-post")
		require.NoError(t, err)
		assert.Equal(t, expectedPost, post)
	})
}

// Blog Theme Service Tests

func TestBlogService_CreateTheme(t *testing.T) {
	service, _, _, mockThemeRepo, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful creation", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogThemeRequest{
			Files: domain.BlogThemeFiles{
				HomeLiquid:     "home template",
				CategoryLiquid: "category template",
				PostLiquid:     "post template",
			},
		}

		mockThemeRepo.EXPECT().
			CreateTheme(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, theme *domain.BlogTheme) error {
				theme.Version = 1
				return nil
			})

		theme, err := service.CreateTheme(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, theme)
		assert.Equal(t, 1, theme.Version)
		assert.Equal(t, req.Files.HomeLiquid, theme.Files.HomeLiquid)
	})

	t.Run("validation error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		// CreateBlogThemeRequest validation always passes, so this is just for consistency
		req := &domain.CreateBlogThemeRequest{
			Files: domain.BlogThemeFiles{},
		}

		mockThemeRepo.EXPECT().
			CreateTheme(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, theme *domain.BlogTheme) error {
				theme.Version = 1
				return nil
			})

		theme, err := service.CreateTheme(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, theme)
	})

	t.Run("repository error", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.CreateBlogThemeRequest{
			Files: domain.BlogThemeFiles{HomeLiquid: "template"},
		}

		mockThemeRepo.EXPECT().
			CreateTheme(ctx, gomock.Any()).
			Return(errors.New("database error"))

		theme, err := service.CreateTheme(ctx, req)
		require.Error(t, err)
		assert.Nil(t, theme)
	})

	t.Run("permission denied - no write permission", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		req := &domain.CreateBlogThemeRequest{
			Files: domain.BlogThemeFiles{HomeLiquid: "template"},
		}

		theme, err := service.CreateTheme(ctx, req)
		require.Error(t, err)
		assert.Nil(t, theme)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})
}

func TestBlogService_GetTheme(t *testing.T) {
	service, _, _, mockThemeRepo, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful retrieval", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedTheme := &domain.BlogTheme{
			Version: 1,
			Files:   domain.BlogThemeFiles{HomeLiquid: "home"},
		}

		mockThemeRepo.EXPECT().
			GetTheme(ctx, 1).
			Return(expectedTheme, nil)

		theme, err := service.GetTheme(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, expectedTheme, theme)
	})

	t.Run("theme not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)

		mockThemeRepo.EXPECT().
			GetTheme(ctx, 999).
			Return(nil, errors.New("blog theme not found"))

		theme, err := service.GetTheme(ctx, 999)
		require.Error(t, err)
		assert.Nil(t, theme)
	})

	t.Run("permission denied - no read permission", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", false, false)

		theme, err := service.GetTheme(ctx, 1)
		require.Error(t, err)
		assert.Nil(t, theme)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})
}

func TestBlogService_GetPublishedTheme(t *testing.T) {
	service, _, _, mockThemeRepo, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful retrieval", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		publishedTime := time.Now()
		expectedTheme := &domain.BlogTheme{
			Version:     2,
			PublishedAt: &publishedTime,
			Files:       domain.BlogThemeFiles{HomeLiquid: "published"},
		}

		mockThemeRepo.EXPECT().
			GetPublishedTheme(ctx).
			Return(expectedTheme, nil)

		theme, err := service.GetPublishedTheme(ctx)
		require.NoError(t, err)
		assert.Equal(t, expectedTheme, theme)
		assert.True(t, theme.IsPublished())
	})

	t.Run("no published theme", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)

		mockThemeRepo.EXPECT().
			GetPublishedTheme(ctx).
			Return(nil, errors.New("no published blog theme found"))

		theme, err := service.GetPublishedTheme(ctx)
		require.Error(t, err)
		assert.Nil(t, theme)
	})
}

func TestBlogService_UpdateTheme(t *testing.T) {
	service, _, _, mockThemeRepo, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful update", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogThemeRequest{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid: "updated home",
			},
		}

		// GetTheme to check if it's not published
		mockThemeRepo.EXPECT().
			GetTheme(ctx, 1).
			Return(&domain.BlogTheme{Version: 1, PublishedAt: nil}, nil)

		mockThemeRepo.EXPECT().
			UpdateTheme(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, theme *domain.BlogTheme) error {
				assert.Equal(t, 1, theme.Version)
				assert.Equal(t, "updated home", theme.Files.HomeLiquid)
				return nil
			})

		theme, err := service.UpdateTheme(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, theme)
		assert.Equal(t, "updated home", theme.Files.HomeLiquid)
	})

	t.Run("validation error - zero version", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogThemeRequest{Version: 0}

		theme, err := service.UpdateTheme(ctx, req)
		require.Error(t, err)
		assert.Nil(t, theme)
		assert.Contains(t, err.Error(), "version must be positive")
	})

	t.Run("cannot update published theme", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		publishedTime := time.Now()
		req := &domain.UpdateBlogThemeRequest{
			Version: 1,
			Files:   domain.BlogThemeFiles{HomeLiquid: "updated"},
		}

		mockThemeRepo.EXPECT().
			GetTheme(ctx, 1).
			Return(&domain.BlogTheme{Version: 1, PublishedAt: &publishedTime}, nil)

		theme, err := service.UpdateTheme(ctx, req)
		require.Error(t, err)
		assert.Nil(t, theme)
		assert.Contains(t, err.Error(), "cannot update published theme")
	})

	t.Run("theme not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UpdateBlogThemeRequest{
			Version: 999,
			Files:   domain.BlogThemeFiles{},
		}

		mockThemeRepo.EXPECT().
			GetTheme(ctx, 999).
			Return(nil, errors.New("blog theme not found"))

		theme, err := service.UpdateTheme(ctx, req)
		require.Error(t, err)
		assert.Nil(t, theme)
	})

	t.Run("permission denied - no write permission", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		req := &domain.UpdateBlogThemeRequest{
			Version: 1,
			Files:   domain.BlogThemeFiles{},
		}

		theme, err := service.UpdateTheme(ctx, req)
		require.Error(t, err)
		assert.Nil(t, theme)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})
}

func TestBlogService_PublishTheme(t *testing.T) {
	service, mockCategoryRepo, _, mockThemeRepo, mockWorkspaceRepo, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful publish", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.PublishBlogThemeRequest{Version: 1}

		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		// GetByID is called by invalidateBlogCaches (uses context.Background())
		mockWorkspaceRepo.EXPECT().
			GetByID(gomock.Any(), "workspace123").
			Return(workspace, nil)

		// ListCategories is called when invalidateAllCategories is true
		mockCategoryRepo.EXPECT().
			ListCategories(gomock.Any()).
			Return([]*domain.BlogCategory{}, nil)

		mockThemeRepo.EXPECT().
			PublishTheme(ctx, 1, "user123").
			Return(nil)

		err := service.PublishTheme(ctx, req)
		require.NoError(t, err)
	})

	t.Run("validation error - zero version", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.PublishBlogThemeRequest{Version: 0}

		err := service.PublishTheme(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})

	t.Run("theme not found", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.PublishBlogThemeRequest{Version: 999}

		mockThemeRepo.EXPECT().
			PublishTheme(ctx, 999, "user123").
			Return(errors.New("blog theme not found"))

		err := service.PublishTheme(ctx, req)
		require.Error(t, err)
	})

	t.Run("permission denied - no write permission", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		req := &domain.PublishBlogThemeRequest{Version: 1}

		err := service.PublishTheme(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})
}

func TestBlogService_ListThemes(t *testing.T) {
	service, _, _, mockThemeRepo, _, _, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful listing", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedResponse := &domain.BlogThemeListResponse{
			Themes: []*domain.BlogTheme{
				{Version: 2, PublishedAt: timePtr(time.Now())},
				{Version: 1, PublishedAt: nil},
			},
			TotalCount: 2,
		}

		mockThemeRepo.EXPECT().
			ListThemes(ctx, domain.ListBlogThemesRequest{Limit: 50, Offset: 0}).
			Return(expectedResponse, nil)

		result, err := service.ListThemes(ctx, &domain.ListBlogThemesRequest{Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount)
		assert.Len(t, result.Themes, 2)
	})

	t.Run("with pagination", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedResponse := &domain.BlogThemeListResponse{
			Themes:     []*domain.BlogTheme{{Version: 1}},
			TotalCount: 10,
		}

		mockThemeRepo.EXPECT().
			ListThemes(ctx, domain.ListBlogThemesRequest{Limit: 10, Offset: 5}).
			Return(expectedResponse, nil)

		result, err := service.ListThemes(ctx, &domain.ListBlogThemesRequest{Limit: 10, Offset: 5})
		require.NoError(t, err)
		assert.Equal(t, 10, result.TotalCount)
		assert.Len(t, result.Themes, 1)
	})

	t.Run("empty list", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		expectedResponse := &domain.BlogThemeListResponse{
			Themes:     []*domain.BlogTheme{},
			TotalCount: 0,
		}

		mockThemeRepo.EXPECT().
			ListThemes(ctx, domain.ListBlogThemesRequest{Limit: 50, Offset: 0}).
			Return(expectedResponse, nil)

		result, err := service.ListThemes(ctx, &domain.ListBlogThemesRequest{Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 0, result.TotalCount)
	})

	t.Run("permission denied - no read permission", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", false, false)

		result, err := service.ListThemes(ctx, &domain.ListBlogThemesRequest{Limit: 50})
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})
}

// Helper function for tests
func timePtr(t time.Time) *time.Time {
	return &t
}

func TestBlogService_RenderHomePage(t *testing.T) {
	service, _, mockPostRepo, mockThemeRepo, mockWorkspaceRepo, mockListRepo, _, _ := setupBlogServiceTest(t)

	t.Run("successful render with public lists", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid:   "<h1>{{ workspace.name }}</h1>{% for list in public_lists %}<div>{{ list.name }}</div>{% endfor %}",
				HeaderLiquid: "<header></header>",
				FooterLiquid: "<footer></footer>",
				SharedLiquid: "",
			},
		}

		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		posts := []*domain.BlogPost{
			{
				ID:   "post-1",
				Slug: "test-post",
				Settings: domain.BlogPostSettings{
					Title: "Test Post",
				},
			},
		}

		categories := []*domain.BlogCategory{
			{
				ID:   "cat-1",
				Slug: "tech",
				Settings: domain.BlogCategorySettings{
					Name: "Technology",
				},
			},
		}

		publicLists := []*domain.List{
			{
				ID:          "list-1",
				Name:        "Newsletter",
				Description: "Weekly updates",
				IsPublic:    true,
			},
		}

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return(publicLists, nil)
		posts[0].CategoryID = "cat-1" // Add category ID to post
		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			Return(&domain.BlogPostListResponse{Posts: posts, TotalCount: 1}, nil)
		mockCategoryRepo := mocks.NewMockBlogCategoryRepository(gomock.NewController(t))
		service.categoryRepo = mockCategoryRepo
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return(categories, nil)
		// Mock GetCategoriesByIDs for posts (including deleted categories for slug lookup)
		mockCategoryRepo.EXPECT().
			GetCategoriesByIDs(ctx, []string{"cat-1"}).
			Return(categories, nil)

		html, err := service.RenderHomePage(ctx, "workspace123", 1, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Test Workspace")
		assert.Contains(t, html, "Newsletter")
	})

	t.Run("filters deleted categories from navigation but uses them for slug lookup", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid:   "<h1>{{ workspace.name }}</h1>{% for cat in categories %}<div>{{ cat.name }}</div>{% endfor %}",
				HeaderLiquid: "<header></header>",
				FooterLiquid: "<footer></footer>",
				SharedLiquid: "",
			},
		}

		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		deletedAt := time.Now()
		posts := []*domain.BlogPost{
			{
				ID:         "post-1",
				Slug:       "test-post",
				CategoryID: "cat-deleted",
				Settings: domain.BlogPostSettings{
					Title: "Test Post",
				},
			},
		}

		// Non-deleted category for navigation
		activeCategory := &domain.BlogCategory{
			ID:   "cat-active",
			Slug: "active",
			Settings: domain.BlogCategorySettings{
				Name: "Active Category",
			},
		}

		// Deleted category (should be filtered from navigation but used for slug lookup)
		deletedCategory := &domain.BlogCategory{
			ID:        "cat-deleted",
			Slug:      "deleted",
			DeletedAt: &deletedAt,
			Settings: domain.BlogCategorySettings{
				Name: "Deleted Category",
			},
		}

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)
		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			Return(&domain.BlogPostListResponse{Posts: posts, TotalCount: 1}, nil)
		mockCategoryRepo := mocks.NewMockBlogCategoryRepository(gomock.NewController(t))
		service.categoryRepo = mockCategoryRepo
		// ListCategories returns only non-deleted categories
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{activeCategory}, nil)
		// GetCategoriesByIDs returns deleted category for slug lookup
		mockCategoryRepo.EXPECT().
			GetCategoriesByIDs(ctx, []string{"cat-deleted"}).
			Return([]*domain.BlogCategory{deletedCategory}, nil)

		html, err := service.RenderHomePage(ctx, "workspace123", 1, nil)
		require.NoError(t, err)
		// Should contain active category in navigation
		assert.Contains(t, html, "Active Category")
		// Should NOT contain deleted category in navigation
		assert.NotContains(t, html, "Deleted Category")
	})

	t.Run("handles no published theme", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(nil, errors.New("no published theme found"))

		html, err := service.RenderHomePage(ctx, "workspace123", 1, nil)
		assert.Error(t, err)
		assert.Empty(t, html)

		blogErr, ok := err.(*domain.BlogRenderError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrCodeThemeNotPublished, blogErr.Code)
	})

	t.Run("handles empty public lists gracefully", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid:   "<h1>Home</h1>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)
		mockPostRepo.EXPECT().ListPosts(ctx, gomock.Any()).Return(&domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}, nil)
		mockCategoryRepo := mocks.NewMockBlogCategoryRepository(gomock.NewController(t))
		service.categoryRepo = mockCategoryRepo
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{}, nil)

		html, err := service.RenderHomePage(ctx, "workspace123", 1, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Home")
	})

	t.Run("returns 404 when page > total_pages", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid:   "<h1>Home</h1>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)

		// Return pagination data showing only 2 pages, but request page 5
		mockPostRepo.EXPECT().ListPosts(ctx, gomock.Any()).Return(&domain.BlogPostListResponse{
			Posts:       []*domain.BlogPost{},
			TotalCount:  20,
			CurrentPage: 5,
			TotalPages:  2,
		}, nil)

		html, err := service.RenderHomePage(ctx, "workspace123", 5, nil)
		assert.Error(t, err)
		assert.Empty(t, html)

		blogErr, ok := err.(*domain.BlogRenderError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrCodePostNotFound, blogErr.Code)
		assert.Contains(t, err.Error(), "Page 5 does not exist")
	})
}

func TestBlogService_RenderPostPage(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, mockThemeRepo, mockWorkspaceRepo, mockListRepo, mockTemplateRepo, _ := setupBlogServiceTest(t)

	t.Run("successful render", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		publishedAt := time.Now()
		post := &domain.BlogPost{
			ID:          "post-1",
			Slug:        "test-post",
			CategoryID:  "cat-1",
			PublishedAt: &publishedAt,
			Settings: domain.BlogPostSettings{
				Title:   "Test Post",
				Excerpt: "Test excerpt",
				Template: domain.BlogPostTemplateReference{
					TemplateID:      "tpl-1",
					TemplateVersion: 1,
				},
			},
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
		}

		template := &domain.Template{
			ID:      "tpl-1",
			Version: 1,
			Web: &domain.WebTemplate{
				HTML: "<div>Blog post content</div>",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				PostLiquid:   "<h1>{{ post.title }}</h1><div>{{ post.content }}</div>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		theme.PublishedAt = &publishedAt

		publicLists := []*domain.List{
			{ID: "list-1", Name: "Newsletter", IsPublic: true},
		}

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockPostRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "test-post").Return(post, nil)
		mockTemplateRepo.EXPECT().GetTemplateByID(ctx, "workspace123", "tpl-1", int64(1)).Return(template, nil)
		mockCategoryRepo.EXPECT().GetCategory(ctx, "cat-1").Return(category, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return(publicLists, nil)
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil)

		html, err := service.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Test Post")
		assert.Contains(t, html, "Blog post content")
	})

	t.Run("handles unpublished post", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		post := &domain.BlogPost{
			ID:          "post-1",
			Slug:        "test-post",
			CategoryID:  "cat-1",
			PublishedAt: nil, // Not published
			Settings: domain.BlogPostSettings{
				Title: "Draft Post",
				Template: domain.BlogPostTemplateReference{
					TemplateID:      "tpl-1",
					TemplateVersion: 1,
				},
			},
		}

		theme := &domain.BlogTheme{Version: 1}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockPostRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "draft-post").Return(post, nil)

		html, err := service.RenderPostPage(ctx, "workspace123", "tech", "draft-post", nil)
		assert.Error(t, err)
		assert.Empty(t, html)

		blogErr, ok := err.(*domain.BlogRenderError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrCodePostNotFound, blogErr.Code)
	})

	t.Run("handles post not found", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}
		theme := &domain.BlogTheme{Version: 1}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockPostRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "nonexistent").Return(nil, errors.New("not found"))

		html, err := service.RenderPostPage(ctx, "workspace123", "tech", "nonexistent", nil)
		assert.Error(t, err)
		assert.Empty(t, html)

		blogErr, ok := err.(*domain.BlogRenderError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrCodePostNotFound, blogErr.Code)
	})

	t.Run("handles template not found", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		publishedAt := time.Now()
		post := &domain.BlogPost{
			ID:          "post-1",
			Slug:        "test-post",
			CategoryID:  "cat-1",
			PublishedAt: &publishedAt,
			Settings: domain.BlogPostSettings{
				Title:   "Test Post",
				Excerpt: "Test excerpt",
				Template: domain.BlogPostTemplateReference{
					TemplateID:      "tpl-1",
					TemplateVersion: 1,
				},
			},
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				PostLiquid:   "<h1>{{ post.title }}</h1><div>{{ post.content }}</div>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		theme.PublishedAt = &publishedAt

		publicLists := []*domain.List{
			{ID: "list-1", Name: "Newsletter", IsPublic: true},
		}

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockPostRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "test-post").Return(post, nil)
		mockTemplateRepo.EXPECT().GetTemplateByID(ctx, "workspace123", "tpl-1", int64(1)).Return(nil, errors.New("template not found"))
		mockCategoryRepo.EXPECT().GetCategory(ctx, "cat-1").Return(category, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return(publicLists, nil)
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil)

		html, err := service.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Test Post")
		// Content should be empty when template is not found
		assert.NotContains(t, html, "Blog post content")
	})

	t.Run("handles template with no web content", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		publishedAt := time.Now()
		post := &domain.BlogPost{
			ID:          "post-1",
			Slug:        "test-post",
			CategoryID:  "cat-1",
			PublishedAt: &publishedAt,
			Settings: domain.BlogPostSettings{
				Title:   "Test Post",
				Excerpt: "Test excerpt",
				Template: domain.BlogPostTemplateReference{
					TemplateID:      "tpl-1",
					TemplateVersion: 1,
				},
			},
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
		}

		// Template with no Web field or empty HTML
		template := &domain.Template{
			ID:      "tpl-1",
			Version: 1,
			Web:     nil, // No web template
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				PostLiquid:   "<h1>{{ post.title }}</h1><div>{{ post.content }}</div>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		theme.PublishedAt = &publishedAt

		publicLists := []*domain.List{
			{ID: "list-1", Name: "Newsletter", IsPublic: true},
		}

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockPostRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "test-post").Return(post, nil)
		mockTemplateRepo.EXPECT().GetTemplateByID(ctx, "workspace123", "tpl-1", int64(1)).Return(template, nil)
		mockCategoryRepo.EXPECT().GetCategory(ctx, "cat-1").Return(category, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return(publicLists, nil)
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil)

		html, err := service.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Test Post")
		// Content should be empty when template has no web content
	})
}

func TestBlogService_RenderCategoryPage(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, mockThemeRepo, mockWorkspaceRepo, mockListRepo, _, _ := setupBlogServiceTest(t)

	t.Run("successful render", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name:        "Technology",
				Description: "Tech posts",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				CategoryLiquid: "<h1>{{ category.name }}</h1>",
				HeaderLiquid:   "",
				FooterLiquid:   "",
				SharedLiquid:   "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		publicLists := []*domain.List{
			{ID: "list-1", Name: "Newsletter", IsPublic: true},
		}

		posts := []*domain.BlogPost{
			{ID: "post-1", Slug: "post-1", CategoryID: "cat-1", Settings: domain.BlogPostSettings{Title: "Post 1"}},
		}

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockCategoryRepo.EXPECT().GetCategoryBySlug(ctx, "tech").Return(category, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return(publicLists, nil)
		mockPostRepo.EXPECT().ListPosts(ctx, gomock.Any()).Return(&domain.BlogPostListResponse{Posts: posts, TotalCount: 1}, nil)
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil)

		html, err := service.RenderCategoryPage(ctx, "workspace123", "tech", 1, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Technology")
	})

	t.Run("handles category not found", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}
		theme := &domain.BlogTheme{Version: 1}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockCategoryRepo.EXPECT().GetCategoryBySlug(ctx, "nonexistent").Return(nil, errors.New("not found"))

		html, err := service.RenderCategoryPage(ctx, "workspace123", "nonexistent", 1, nil)
		assert.Error(t, err)
		assert.Empty(t, html)

		blogErr, ok := err.(*domain.BlogRenderError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrCodeCategoryNotFound, blogErr.Code)
	})

	t.Run("returns 404 when page > total_pages for category", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		workspace := &domain.Workspace{ID: "workspace123", Name: "Test"}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				CategoryLiquid: "<h1>{{ category.name }}</h1>",
				HeaderLiquid:   "",
				FooterLiquid:   "",
				SharedLiquid:   "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockCategoryRepo.EXPECT().GetCategoryBySlug(ctx, "tech").Return(category, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)

		// Return pagination showing only 3 pages, but request page 10
		mockPostRepo.EXPECT().ListPosts(ctx, gomock.Any()).Return(&domain.BlogPostListResponse{
			Posts:       []*domain.BlogPost{},
			TotalCount:  30,
			CurrentPage: 10,
			TotalPages:  3,
		}, nil)

		html, err := service.RenderCategoryPage(ctx, "workspace123", "tech", 10, nil)
		assert.Error(t, err)
		assert.Empty(t, html)

		blogErr, ok := err.(*domain.BlogRenderError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrCodePostNotFound, blogErr.Code)
		assert.Contains(t, err.Error(), "Page 10 does not exist")
	})
}

func TestBlogService_RenderHomePage_WithPaginationSettings(t *testing.T) {
	service, _, mockPostRepo, mockThemeRepo, mockWorkspaceRepo, mockListRepo, _, _ := setupBlogServiceTest(t)

	t.Run("uses custom home page size from settings", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

		// Workspace with custom pagination settings
		customPageSize := 50
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:    "UTC",
				BlogEnabled: true,
				BlogSettings: &domain.BlogSettings{
					Title:        "Test Blog",
					HomePageSize: customPageSize,
				},
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid:   "<h1>{{ workspace.name }}</h1>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)

		// Verify the ListPosts is called with the custom page size
		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, customPageSize, params.Limit, "Expected custom page size to be used")
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}, nil
			})

		mockCategoryRepo := mocks.NewMockBlogCategoryRepository(gomock.NewController(t))
		service.categoryRepo = mockCategoryRepo
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{}, nil)

		html, err := service.RenderHomePage(ctx, "workspace123", 1, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, html)
	})

	t.Run("uses default page size when not configured", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid:   "<h1>{{ workspace.name }}</h1>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)

		// Verify the ListPosts is called with the default page size (20)
		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, 20, params.Limit, "Expected default page size of 20")
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}, nil
			})

		mockCategoryRepo := mocks.NewMockBlogCategoryRepository(gomock.NewController(t))
		service.categoryRepo = mockCategoryRepo
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{}, nil)

		html, err := service.RenderHomePage(ctx, "workspace123", 1, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, html)
	})

	t.Run("uses default when page size is invalid", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				BlogSettings: &domain.BlogSettings{
					Title:        "Test Blog",
					HomePageSize: 0, // Invalid - should use default
				},
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				HomeLiquid:   "<h1>{{ workspace.name }}</h1>",
				HeaderLiquid: "",
				FooterLiquid: "",
				SharedLiquid: "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)

		// Verify the ListPosts is called with the default page size due to validation
		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, 20, params.Limit, "Expected default page size when invalid value provided")
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}, nil
			})

		mockCategoryRepo := mocks.NewMockBlogCategoryRepository(gomock.NewController(t))
		service.categoryRepo = mockCategoryRepo
		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{}, nil)

		html, err := service.RenderHomePage(ctx, "workspace123", 1, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, html)
	})
}

func TestBlogService_RenderCategoryPage_WithPaginationSettings(t *testing.T) {
	service, mockCategoryRepo, mockPostRepo, mockThemeRepo, mockWorkspaceRepo, mockListRepo, _, _ := setupBlogServiceTest(t)

	t.Run("uses custom category page size from settings", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

		customPageSize := 30
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:    "UTC",
				BlogEnabled: true,
				BlogSettings: &domain.BlogSettings{
					Title:            "Test Blog",
					CategoryPageSize: customPageSize,
				},
			},
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				CategoryLiquid: "<h1>{{ category.name }}</h1>",
				HeaderLiquid:   "",
				FooterLiquid:   "",
				SharedLiquid:   "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockCategoryRepo.EXPECT().GetCategoryBySlug(ctx, "tech").Return(category, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)

		// Verify the ListPosts is called with the custom category page size
		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, customPageSize, params.Limit, "Expected custom category page size to be used")
				assert.Equal(t, "cat-1", params.CategoryID, "Expected category ID filter")
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}, nil
			})

		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil)

		html, err := service.RenderCategoryPage(ctx, "workspace123", "tech", 1, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, html)
	})

	t.Run("uses default category page size when not configured", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

		customURL := "https://example.com"
		workspace := &domain.Workspace{
			ID:   "workspace123",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:          "UTC",
				CustomEndpointURL: &customURL,
			},
		}

		category := &domain.BlogCategory{
			ID:   "cat-1",
			Slug: "tech",
			Settings: domain.BlogCategorySettings{
				Name: "Technology",
			},
		}

		theme := &domain.BlogTheme{
			Version: 1,
			Files: domain.BlogThemeFiles{
				CategoryLiquid: "<h1>{{ category.name }}</h1>",
				HeaderLiquid:   "",
				FooterLiquid:   "",
				SharedLiquid:   "",
			},
		}
		publishedAt := time.Now()
		theme.PublishedAt = &publishedAt

		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		mockThemeRepo.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		mockCategoryRepo.EXPECT().GetCategoryBySlug(ctx, "tech").Return(category, nil)
		mockListRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)

		// Verify the ListPosts is called with the default page size (20)
		mockPostRepo.EXPECT().
			ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, 20, params.Limit, "Expected default category page size of 20")
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}, nil
			})

		mockCategoryRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil)

		html, err := service.RenderCategoryPage(ctx, "workspace123", "tech", 1, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, html)
	})
}

func TestBlogSettings_ValidationHelpers(t *testing.T) {
	t.Run("GetHomePageSize returns value when valid", func(t *testing.T) {
		settings := &domain.BlogSettings{
			HomePageSize: 50,
		}
		assert.Equal(t, 50, settings.GetHomePageSize())
	})

	t.Run("GetHomePageSize returns default when too small", func(t *testing.T) {
		settings := &domain.BlogSettings{
			HomePageSize: 0,
		}
		assert.Equal(t, 20, settings.GetHomePageSize())
	})

	t.Run("GetHomePageSize returns default when too large", func(t *testing.T) {
		settings := &domain.BlogSettings{
			HomePageSize: 101,
		}
		assert.Equal(t, 20, settings.GetHomePageSize())
	})

	t.Run("GetHomePageSize returns default when nil", func(t *testing.T) {
		var settings *domain.BlogSettings
		assert.Equal(t, 20, settings.GetHomePageSize())
	})

	t.Run("GetCategoryPageSize returns value when valid", func(t *testing.T) {
		settings := &domain.BlogSettings{
			CategoryPageSize: 25,
		}
		assert.Equal(t, 25, settings.GetCategoryPageSize())
	})

	t.Run("GetCategoryPageSize returns default when invalid", func(t *testing.T) {
		settings := &domain.BlogSettings{
			CategoryPageSize: -5,
		}
		assert.Equal(t, 20, settings.GetCategoryPageSize())
	})

	t.Run("GetCategoryPageSize returns default when nil", func(t *testing.T) {
		var settings *domain.BlogSettings
		assert.Equal(t, 20, settings.GetCategoryPageSize())
	})
}
