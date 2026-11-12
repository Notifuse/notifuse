package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestBlogService_ListPosts_SetsNow(t *testing.T) {
	service, _, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)
	ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)

	before := time.Now().UTC()
	mockPostRepo.EXPECT().ListPosts(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, p domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
			assert.Equal(t, time.UTC, p.Now.Location(), "published_at is a UTC timestamp without time zone")
			assert.False(t, p.Now.Before(before))
			assert.False(t, p.Now.After(time.Now().UTC()))
			return &domain.BlogPostListResponse{}, nil
		})

	_, err := service.ListPosts(ctx, &domain.ListBlogPostsRequest{Status: domain.BlogPostStatusScheduled, Now: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
}

func TestBlogService_CountPosts(t *testing.T) {
	t.Run("relays the repository with the category and a UTC instant", func(t *testing.T) {
		service, _, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)

		counts := domain.NewBlogPostCounts()
		counts.AllTotal = 3
		before := time.Now().UTC()
		mockPostRepo.EXPECT().CountPosts(ctx, "workspace123", "cat-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ string, now time.Time) (*domain.BlogPostCounts, error) {
				assert.Equal(t, time.UTC, now.Location())
				assert.False(t, now.Before(before))
				return &counts, nil
			})

		got, err := service.CountPosts(ctx, "cat-1")
		require.NoError(t, err)
		assert.Equal(t, &counts, got)
	})

	t.Run("refused without blog read", func(t *testing.T) {
		service, _, _, _, _, _, _, mockAuthService := setupBlogServiceTest(t)
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", false, false)

		_, err := service.CountPosts(ctx, "")
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
	})

	t.Run("authentication failure is returned", func(t *testing.T) {
		service, _, _, _, _, _, _, mockAuthService := setupBlogServiceTest(t)
		ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "workspace123").Return(ctx, nil, nil, errors.New("bad token"))

		_, err := service.CountPosts(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad token")
	})

	t.Run("no workspace in the context", func(t *testing.T) {
		service, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := service.CountPosts(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		service, _, mockPostRepo, _, _, _, _, mockAuthService := setupBlogServiceTest(t)
		ctx := setupBlogContextWithAuth(mockAuthService, "workspace123", true, false)
		mockPostRepo.EXPECT().CountPosts(ctx, "workspace123", "", gomock.Any()).Return(nil, errors.New("db down"))

		_, err := service.CountPosts(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}
