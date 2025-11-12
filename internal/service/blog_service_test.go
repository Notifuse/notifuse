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
	"github.com/Notifuse/notifuse/pkg/logger"
)

func setupBlogServiceTest(t *testing.T) (
	*BlogService,
	*mocks.MockBlogCategoryRepository,
	*mocks.MockBlogPostRepository,
	*mocks.MockWorkspaceRepository,
	*mocks.MockAuthService,
) {
	ctrl := gomock.NewController(t)

	mockCategoryRepo := mocks.NewMockBlogCategoryRepository(ctrl)
	mockPostRepo := mocks.NewMockBlogPostRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := logger.NewLoggerWithLevel("disabled")

	service := NewBlogService(
		mockLogger,
		mockCategoryRepo,
		mockPostRepo,
		mockWorkspaceRepo,
		mockAuthService,
	)

	return service, mockCategoryRepo, mockPostRepo, mockWorkspaceRepo, mockAuthService
}

// setupBlogContextWithAuth creates a context with workspace_id and mocks authentication with permissions
func setupBlogContextWithAuth(mockAuthService *mocks.MockAuthService, workspaceID string, readPerm, writePerm bool) context.Context {
	ctx := context.WithValue(context.Background(), "workspace_id", workspaceID)

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
		AnyTimes()

	return ctx
}

func TestBlogService_CreateCategory(t *testing.T) {
	service, mockCategoryRepo, _, _, mockAuthService := setupBlogServiceTest(t)

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
	service, mockCategoryRepo, _, _, mockAuthService := setupBlogServiceTest(t)

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
	service, mockCategoryRepo, _, _, mockAuthService := setupBlogServiceTest(t)

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

func TestBlogService_UpdateCategory(t *testing.T) {
	service, mockCategoryRepo, _, _, mockAuthService := setupBlogServiceTest(t)

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
	service, mockCategoryRepo, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

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
	service, mockCategoryRepo, _, _, mockAuthService := setupBlogServiceTest(t)

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
	service, mockCategoryRepo, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

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
	service, _, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

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
	service, mockCategoryRepo, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

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
			ID:   "post123",
			Slug: "old-post",
		}

		mockPostRepo.EXPECT().
			GetPost(ctx, req.ID).
			Return(existingPost, nil)

		// Mock slug check
		mockPostRepo.EXPECT().
			GetPostBySlug(ctx, req.Slug).
			Return(nil, errors.New("not found"))

		// Mock category check
		mockCategoryRepo.EXPECT().
			GetCategory(ctx, categoryID).
			Return(&domain.BlogCategory{ID: categoryID}, nil)

		mockPostRepo.EXPECT().
			UpdatePost(ctx, gomock.Any()).
			Return(nil)

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
	service, _, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful deletion", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.DeleteBlogPostRequest{
			ID: "post123",
		}

		mockPostRepo.EXPECT().
			DeletePost(ctx, req.ID).
			Return(nil)

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
}

func TestBlogService_ListPosts(t *testing.T) {
	service, _, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

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
			ListPosts(ctx, *params).
			Return(expectedResponse, nil)

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
	service, _, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful publish", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.PublishBlogPostRequest{
			ID: "post123",
		}

		mockPostRepo.EXPECT().
			PublishPost(ctx, req.ID).
			Return(nil)

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

		mockPostRepo.EXPECT().
			PublishPost(ctx, req.ID).
			Return(errors.New("already published"))

		err := service.PublishPost(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to publish post")
	})
}

func TestBlogService_UnpublishPost(t *testing.T) {
	service, _, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

	t.Run("successful unpublish", func(t *testing.T) {
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, true)
		req := &domain.UnpublishBlogPostRequest{
			ID: "post123",
		}

		mockPostRepo.EXPECT().
			UnpublishPost(ctx, req.ID).
			Return(nil)

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
	service, _, mockPostRepo, _, _ := setupBlogServiceTest(t)
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
	service, _, mockPostRepo, _, _ := setupBlogServiceTest(t)
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
	service, _, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

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
	service, _, mockPostRepo, _, mockAuthService := setupBlogServiceTest(t)

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
