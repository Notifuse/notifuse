package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBuildBlogTemplateData(t *testing.T) {
	now := time.Now().UTC()

	t.Run("builds data with all fields", func(t *testing.T) {
		workspace := &Workspace{
			ID:   "ws-123",
			Name: "Test Workspace",
		}

		post := &BlogPost{
			ID:         "post-1",
			Slug:       "test-post",
			CategoryID: "cat-1",
			PublishedAt: &now,
			CreatedAt:   now,
			UpdatedAt:   now,
			Settings: BlogPostSettings{
				Title:               "Test Post",
				Excerpt:             "Test excerpt",
				FeaturedImageURL:    "https://example.com/image.jpg",
				Authors:             []BlogAuthor{{Name: "John Doe"}},
				ReadingTimeMinutes:  5,
				SEO: &SEOSettings{
					MetaTitle:       "SEO Title",
					MetaDescription: "SEO Description",
					OGTitle:         "OG Title",
					OGDescription:   "OG Description",
					OGImage:         "https://example.com/og.jpg",
					CanonicalURL:    "https://example.com/post",
					Keywords:        []string{"test", "blog"},
				},
			},
		}

		category := &BlogCategory{
			ID:   "cat-1",
			Slug: "technology",
			Settings: BlogCategorySettings{
				Name:        "Technology",
				Description: "Tech posts",
				SEO: &SEOSettings{
					MetaTitle:       "Technology Category",
					MetaDescription: "Tech category description",
				},
			},
		}

		list1 := &List{
			ID:          "list-1",
			Name:        "Weekly Newsletter",
			Description: "Get updates weekly",
			IsPublic:    true,
		}

		list2 := &List{
			ID:       "list-2",
			Name:     "Product Updates",
			IsPublic: true,
		}

		posts := []*BlogPost{post}
		categories := []*BlogCategory{category}
		publicLists := []*List{list1, list2}

		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			Post:        post,
			Category:    category,
			PublicLists: publicLists,
			Posts:       posts,
			Categories:  categories,
			CustomData: MapOfAny{
				"custom_field": "custom_value",
			},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		// Check workspace
		workspaceData := data["workspace"].(MapOfAny)
		assert.Equal(t, "ws-123", workspaceData["id"])
		assert.Equal(t, "Test Workspace", workspaceData["name"])

		// Check post
		postData := data["post"].(MapOfAny)
		assert.Equal(t, "post-1", postData["id"])
		assert.Equal(t, "test-post", postData["slug"])
		assert.Equal(t, "Test Post", postData["title"])
		assert.Equal(t, "Test excerpt", postData["excerpt"])
		assert.Equal(t, 5, postData["reading_time_minutes"])

		// Check post SEO
		postSEO := postData["seo"].(MapOfAny)
		assert.Equal(t, "SEO Title", postSEO["meta_title"])
		assert.Equal(t, "SEO Description", postSEO["meta_description"])

		// Check category
		categoryData := data["category"].(MapOfAny)
		assert.Equal(t, "cat-1", categoryData["id"])
		assert.Equal(t, "technology", categoryData["slug"])
		assert.Equal(t, "Technology", categoryData["name"])

		// Check category SEO
		categorySEO := categoryData["seo"].(MapOfAny)
		assert.Equal(t, "Technology Category", categorySEO["meta_title"])

		// Check public lists
		publicListsData := data["public_lists"].([]MapOfAny)
		assert.Len(t, publicListsData, 2)
		assert.Equal(t, "list-1", publicListsData[0]["id"])
		assert.Equal(t, "Weekly Newsletter", publicListsData[0]["name"])
		assert.Equal(t, "Get updates weekly", publicListsData[0]["description"])
		assert.Equal(t, "list-2", publicListsData[1]["id"])
		assert.Equal(t, "Product Updates", publicListsData[1]["name"])
		_, hasDesc := publicListsData[1]["description"]
		assert.False(t, hasDesc, "list 2 should not have description")

		// Check posts array
		postsData := data["posts"].([]MapOfAny)
		assert.Len(t, postsData, 1)
		assert.Equal(t, "post-1", postsData[0]["id"])

		// Check categories array
		categoriesData := data["categories"].([]MapOfAny)
		assert.Len(t, categoriesData, 1)
		assert.Equal(t, "cat-1", categoriesData[0]["id"])

		// Check custom data
		assert.Equal(t, "custom_value", data["custom_field"])

		// Check current year
		assert.Equal(t, time.Now().Year(), data["current_year"])
	})

	t.Run("handles empty public lists", func(t *testing.T) {
		workspace := &Workspace{ID: "ws-123", Name: "Test"}
		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			PublicLists: []*List{},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		publicListsData := data["public_lists"].([]MapOfAny)
		assert.Len(t, publicListsData, 0)
		assert.NotNil(t, publicListsData)
	})

	t.Run("handles nil optional fields", func(t *testing.T) {
		workspace := &Workspace{ID: "ws-123", Name: "Test"}
		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			PublicLists: []*List{},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		_, hasPost := data["post"]
		assert.False(t, hasPost)

		_, hasCategory := data["category"]
		assert.False(t, hasCategory)

		_, hasPosts := data["posts"]
		assert.False(t, hasPosts)

		_, hasCategories := data["categories"]
		assert.False(t, hasCategories)
	})

	t.Run("handles list with empty description", func(t *testing.T) {
		workspace := &Workspace{ID: "ws-123", Name: "Test"}
		list := &List{
			ID:          "list-1",
			Name:        "Newsletter",
			Description: "", // Empty description
			IsPublic:    true,
		}

		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			PublicLists: []*List{list},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		publicListsData := data["public_lists"].([]MapOfAny)
		assert.Len(t, publicListsData, 1)
		_, hasDesc := publicListsData[0]["description"]
		assert.False(t, hasDesc)
	})
}

func TestBlogRenderError(t *testing.T) {
	t.Run("error message without details", func(t *testing.T) {
		err := &BlogRenderError{
			Code:    ErrCodeThemeNotFound,
			Message: "Theme not found",
			Details: nil,
		}

		assert.Equal(t, "theme_not_found: Theme not found", err.Error())
	})

	t.Run("error message with details", func(t *testing.T) {
		details := assert.AnError
		err := &BlogRenderError{
			Code:    ErrCodeRenderFailed,
			Message: "Render failed",
			Details: details,
		}

		assert.Contains(t, err.Error(), "render_failed: Render failed")
		assert.Contains(t, err.Error(), details.Error())
	})
}

func TestListBlogPostsRequest_Validate(t *testing.T) {
	t.Run("sets defaults for empty request", func(t *testing.T) {
		req := ListBlogPostsRequest{}
		err := req.Validate()
		
		assert.NoError(t, err)
		assert.Equal(t, BlogPostStatusAll, req.Status)
		assert.Equal(t, 1, req.Page)
		assert.Equal(t, 50, req.Limit)
		assert.Equal(t, 0, req.Offset)
	})

	t.Run("calculates offset from page number", func(t *testing.T) {
		req := ListBlogPostsRequest{
			Page:  3,
			Limit: 10,
		}
		err := req.Validate()
		
		assert.NoError(t, err)
		assert.Equal(t, 20, req.Offset) // (3-1) * 10
	})

	t.Run("defaults page to 1 if zero", func(t *testing.T) {
		req := ListBlogPostsRequest{
			Page: 0,
		}
		err := req.Validate()
		
		assert.NoError(t, err)
		assert.Equal(t, 1, req.Page)
		assert.Equal(t, 0, req.Offset)
	})

	t.Run("defaults page to 1 if negative", func(t *testing.T) {
		req := ListBlogPostsRequest{
			Page: -5,
		}
		err := req.Validate()
		
		assert.NoError(t, err)
		assert.Equal(t, 1, req.Page)
	})

	t.Run("enforces max limit", func(t *testing.T) {
		req := ListBlogPostsRequest{
			Limit: 200,
		}
		err := req.Validate()
		
		assert.NoError(t, err)
		assert.Equal(t, 100, req.Limit)
	})

	t.Run("validates status", func(t *testing.T) {
		req := ListBlogPostsRequest{
			Status: "invalid",
		}
		err := req.Validate()
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status")
	})
}

func TestBuildBlogTemplateData_WithPagination(t *testing.T) {
	workspace := &Workspace{ID: "ws-123", Name: "Test"}
	
	t.Run("includes pagination data when provided", func(t *testing.T) {
		paginationData := &BlogPostListResponse{
			Posts:           []*BlogPost{},
			TotalCount:      100,
			CurrentPage:     3,
			TotalPages:      10,
			HasNextPage:     true,
			HasPreviousPage: true,
		}

		req := BlogTemplateDataRequest{
			Workspace:      workspace,
			PublicLists:    []*List{},
			PaginationData: paginationData,
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		// Check pagination data
		pagination := data["pagination"].(MapOfAny)
		assert.Equal(t, 3, pagination["current_page"])
		assert.Equal(t, 10, pagination["total_pages"])
		assert.Equal(t, true, pagination["has_next"])
		assert.Equal(t, true, pagination["has_previous"])
		assert.Equal(t, 100, pagination["total_count"])
		assert.Equal(t, 0, pagination["per_page"]) // Default value, updated by caller
	})

	t.Run("omits pagination when not provided", func(t *testing.T) {
		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			PublicLists: []*List{},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		_, hasPagination := data["pagination"]
		assert.False(t, hasPagination)
	})
}
