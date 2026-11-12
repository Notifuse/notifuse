package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestBlogHandler_HandleListPosts_SearchAndValidation(t *testing.T) {
	t.Run("search and status=scheduled reach the service", func(t *testing.T) {
		handler, mockService, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		mockService.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, "50% off", params.Search)
				assert.Equal(t, domain.BlogPostStatusScheduled, params.Status)
				return &domain.BlogPostListResponse{}, nil
			})

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id=ws-123&status=scheduled&search=50%25+off", nil)
		w := httptest.NewRecorder()
		handler.HandleListPosts(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("a validation error is a 400 with its reason", func(t *testing.T) {
		handler, mockService, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		mockService.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
			Return(nil, domain.ValidationError{Message: "invalid status: archived"})

		req := httptest.NewRequest(http.MethodGet, "/api/blogPosts.list?workspace_id=ws-123&status=archived", nil)
		w := httptest.NewRecorder()
		handler.HandleListPosts(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid status: archived")
	})
}

func TestBlogHandler_HandleCountPosts(t *testing.T) {
	const categoryID = "6f1c2a8e-3d4b-4c5a-9e7f-1a2b3c4d5e6f"

	t.Run("non-GET is refused", func(t *testing.T) {
		handler, _, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		w := httptest.NewRecorder()
		handler.HandleCountPosts(w, httptest.NewRequest(http.MethodPost, "/api/blogPosts.counts?workspace_id=ws-123", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("workspace_id is required", func(t *testing.T) {
		handler, _, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		w := httptest.NewRecorder()
		handler.HandleCountPosts(w, httptest.NewRequest(http.MethodGet, "/api/blogPosts.counts", nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("a category_id that is not a UUID is a 400", func(t *testing.T) {
		handler, _, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		w := httptest.NewRecorder()
		handler.HandleCountPosts(w, httptest.NewRequest(http.MethodGet, "/api/blogPosts.counts?workspace_id=ws-123&category_id=news", nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("the counts of a category, with the workspace in the context", func(t *testing.T) {
		handler, mockService, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		counts := domain.NewBlogPostCounts()
		counts.AllTotal, counts.Total, counts.Published, counts.Scheduled, counts.Draft = 8, 5, 3, 1, 2
		counts.ByCategory[categoryID] = 5
		mockService.EXPECT().CountPosts(gomock.Any(), categoryID).
			DoAndReturn(func(ctx context.Context, _ string) (*domain.BlogPostCounts, error) {
				assert.Equal(t, "ws-123", ctx.Value(domain.WorkspaceIDKey))
				return &counts, nil
			})

		w := httptest.NewRecorder()
		handler.HandleCountPosts(w, httptest.NewRequest(http.MethodGet, "/api/blogPosts.counts?workspace_id=ws-123&category_id="+categoryID, nil))
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"all_total":8,"total":5,"published":3,"scheduled":1,"draft":2,"by_category":{"`+categoryID+`":5}}`, w.Body.String())
	})

	t.Run("a category_id in another UUID spelling is passed in canonical form", func(t *testing.T) {
		handler, mockService, _, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		counts := domain.NewBlogPostCounts()
		mockService.EXPECT().CountPosts(gomock.Any(), categoryID).Return(&counts, nil)

		w := httptest.NewRecorder()
		handler.HandleCountPosts(w, httptest.NewRequest(http.MethodGet, "/api/blogPosts.counts?workspace_id=ws-123&category_id={"+strings.ToUpper(categoryID)+"}", nil))
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("a service error is a 500", func(t *testing.T) {
		handler, mockService, mockLogger, ctrl := setupBlogHandler(t)
		defer ctrl.Finish()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())
		mockService.EXPECT().CountPosts(gomock.Any(), "").Return(nil, errors.New("db down"))

		w := httptest.NewRecorder()
		handler.HandleCountPosts(w, httptest.NewRequest(http.MethodGet, "/api/blogPosts.counts?workspace_id=ws-123", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
