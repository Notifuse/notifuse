package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBlogTemplateData(t *testing.T) {
	now := time.Now().UTC()

	t.Run("builds data with all fields", func(t *testing.T) {
		workspace := &Workspace{
			ID:   "ws-123",
			Name: "Test Workspace",
		}

		post := &BlogPost{
			ID:          "post-1",
			Slug:        "test-post",
			CategoryID:  "cat-1",
			PublishedAt: &now,
			CreatedAt:   now,
			UpdatedAt:   now,
			Settings: BlogPostSettings{
				Title:              "Test Post",
				Excerpt:            "Test excerpt",
				FeaturedImageURL:   "https://example.com/image.jpg",
				Authors:            []BlogAuthor{{Name: "John Doe"}},
				ReadingTimeMinutes: 5,
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
		publicListsData := data["public_lists"].([]map[string]interface{})
		assert.Len(t, publicListsData, 2)
		assert.Equal(t, "list-1", publicListsData[0]["id"])
		assert.Equal(t, "Weekly Newsletter", publicListsData[0]["name"])
		assert.Equal(t, "Get updates weekly", publicListsData[0]["description"])
		assert.Equal(t, "list-2", publicListsData[1]["id"])
		assert.Equal(t, "Product Updates", publicListsData[1]["name"])
		_, hasDesc := publicListsData[1]["description"]
		assert.False(t, hasDesc, "list 2 should not have description")

		// Check posts array
		postsData := data["posts"].([]map[string]interface{})
		assert.Len(t, postsData, 1)
		assert.Equal(t, "post-1", postsData[0]["id"])

		// Check categories array
		categoriesData := data["categories"].([]map[string]interface{})
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

		publicListsData := data["public_lists"].([]map[string]interface{})
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

		publicListsData := data["public_lists"].([]map[string]interface{})
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
		pagination := data["pagination"].(map[string]interface{})
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

// TestBuildBlogTemplateData_WorkspaceId tests that workspace.id is always present in template data
func TestBuildBlogTemplateData_WorkspaceId(t *testing.T) {
	t.Run("workspace.id is always present when workspace is provided", func(t *testing.T) {
		workspace := &Workspace{
			ID:   "ws-test-123",
			Name: "Test Workspace",
		}

		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			PublicLists: []*List{},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		// Verify workspace data exists
		workspaceData, hasWorkspace := data["workspace"]
		assert.True(t, hasWorkspace, "workspace should be present in template data")

		workspaceMap := workspaceData.(MapOfAny)
		workspaceID, hasID := workspaceMap["id"]
		assert.True(t, hasID, "workspace.id should be present")
		assert.Equal(t, "ws-test-123", workspaceID)
	})

	t.Run("workspace.id is set correctly from workspace.ID", func(t *testing.T) {
		workspace := &Workspace{
			ID:   "ws-abc-xyz",
			Name: "Another Workspace",
		}

		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			PublicLists: []*List{},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		workspaceData := data["workspace"].(MapOfAny)
		assert.Equal(t, "ws-abc-xyz", workspaceData["id"])
		assert.Equal(t, "Another Workspace", workspaceData["name"])
	})

	t.Run("workspace.id is present even with minimal workspace data", func(t *testing.T) {
		workspace := &Workspace{
			ID:   "minimal-ws",
			Name: "Minimal",
		}

		req := BlogTemplateDataRequest{
			Workspace:   workspace,
			PublicLists: []*List{},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		workspaceData := data["workspace"].(MapOfAny)
		// Verify id is always present
		assert.Equal(t, "minimal-ws", workspaceData["id"])
		assert.Equal(t, "Minimal", workspaceData["name"])
	})

	t.Run("workspace is nil when not provided", func(t *testing.T) {
		req := BlogTemplateDataRequest{
			Workspace:   nil,
			PublicLists: []*List{},
		}

		data, err := BuildBlogTemplateData(req)
		assert.NoError(t, err)

		_, hasWorkspace := data["workspace"]
		assert.False(t, hasWorkspace, "workspace should not be present when nil")
	})
}

func TestSEOSettings_Value(t *testing.T) {
	t.Run("serializes all fields", func(t *testing.T) {
		seo := SEOSettings{
			MetaTitle:       "Test Title",
			MetaDescription: "Test Description",
			OGTitle:         "OG Title",
			OGDescription:   "OG Description",
			OGImage:         "https://example.com/image.jpg",
			CanonicalURL:    "https://example.com/page",
			Keywords:        []string{"test", "seo"},
		}

		value, err := seo.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)

		// Verify it's valid JSON
		var result SEOSettings
		err = json.Unmarshal(value.([]byte), &result)
		require.NoError(t, err)
		assert.Equal(t, seo.MetaTitle, result.MetaTitle)
		assert.Equal(t, seo.Keywords, result.Keywords)
	})

	t.Run("serializes empty fields", func(t *testing.T) {
		seo := SEOSettings{}

		value, err := seo.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)
	})

	t.Run("serializes nil Keywords slice", func(t *testing.T) {
		seo := SEOSettings{
			MetaTitle: "Test",
			Keywords:  nil,
		}

		value, err := seo.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)
	})
}

func TestSEOSettings_Scan(t *testing.T) {
	t.Run("deserializes valid JSON bytes", func(t *testing.T) {
		original := SEOSettings{
			MetaTitle:       "Test Title",
			MetaDescription: "Test Description",
			OGTitle:         "OG Title",
			OGDescription:   "OG Description",
			OGImage:         "https://example.com/image.jpg",
			CanonicalURL:    "https://example.com/page",
			Keywords:        []string{"test", "seo"},
		}

		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		var scanned SEOSettings
		err = scanned.Scan(jsonData)
		require.NoError(t, err)
		assert.Equal(t, original.MetaTitle, scanned.MetaTitle)
		assert.Equal(t, original.MetaDescription, scanned.MetaDescription)
		assert.Equal(t, original.Keywords, scanned.Keywords)
	})

	t.Run("handles nil value", func(t *testing.T) {
		var seo SEOSettings
		err := seo.Scan(nil)
		require.NoError(t, err)
	})

	t.Run("handles invalid type", func(t *testing.T) {
		var seo SEOSettings
		err := seo.Scan("not-a-byte-array")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type assertion to []byte failed")
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		var seo SEOSettings
		err := seo.Scan([]byte("{invalid json}"))
		require.Error(t, err)
	})
}

func TestSEOSettings_MergeWithDefaults(t *testing.T) {
	t.Run("both nil returns empty", func(t *testing.T) {
		var current *SEOSettings
		var defaults *SEOSettings

		result := current.MergeWithDefaults(defaults)
		require.NotNil(t, result)
		assert.Equal(t, &SEOSettings{}, result)
	})

	t.Run("current nil returns defaults", func(t *testing.T) {
		var current *SEOSettings
		defaults := &SEOSettings{
			MetaTitle: "Default Title",
		}

		result := current.MergeWithDefaults(defaults)
		require.NotNil(t, result)
		assert.Equal(t, defaults.MetaTitle, result.MetaTitle)
	})

	t.Run("defaults nil returns current", func(t *testing.T) {
		current := &SEOSettings{
			MetaTitle: "Current Title",
		}
		var defaults *SEOSettings

		result := current.MergeWithDefaults(defaults)
		require.NotNil(t, result)
		assert.Equal(t, current.MetaTitle, result.MetaTitle)
	})

	t.Run("current has value uses current", func(t *testing.T) {
		current := &SEOSettings{
			MetaTitle: "Post Title",
		}
		defaults := &SEOSettings{
			MetaTitle: "Default Title",
		}

		result := current.MergeWithDefaults(defaults)
		assert.Equal(t, "Post Title", result.MetaTitle)
	})

	t.Run("current empty uses defaults", func(t *testing.T) {
		current := &SEOSettings{
			MetaTitle: "",
		}
		defaults := &SEOSettings{
			MetaTitle: "Default Title",
		}

		result := current.MergeWithDefaults(defaults)
		assert.Equal(t, "Default Title", result.MetaTitle)
	})

	t.Run("keywords current empty uses defaults", func(t *testing.T) {
		current := &SEOSettings{
			Keywords: []string{},
		}
		defaults := &SEOSettings{
			Keywords: []string{"tag1", "tag2"},
		}

		result := current.MergeWithDefaults(defaults)
		assert.Equal(t, defaults.Keywords, result.Keywords)
	})

	t.Run("keywords current has values uses current", func(t *testing.T) {
		current := &SEOSettings{
			Keywords: []string{"tag1"},
		}
		defaults := &SEOSettings{
			Keywords: []string{},
		}

		result := current.MergeWithDefaults(defaults)
		assert.Equal(t, current.Keywords, result.Keywords)
	})

	t.Run("mixed empty and non-empty fields", func(t *testing.T) {
		current := &SEOSettings{
			MetaTitle:       "Post Title",
			MetaDescription: "", // Empty, should use default
			OGTitle:         "OG Post Title",
			OGDescription:   "", // Empty, should use default
		}
		defaults := &SEOSettings{
			MetaTitle:       "Default Title",
			MetaDescription: "Default Description",
			OGTitle:         "Default OG Title",
			OGDescription:   "Default OG Description",
		}

		result := current.MergeWithDefaults(defaults)
		assert.Equal(t, "Post Title", result.MetaTitle)                 // Current used
		assert.Equal(t, "Default Description", result.MetaDescription)  // Default used
		assert.Equal(t, "OG Post Title", result.OGTitle)                // Current used
		assert.Equal(t, "Default OG Description", result.OGDescription) // Default used
	})
}

func TestBlogCategorySettings_Value(t *testing.T) {
	t.Run("serializes all fields", func(t *testing.T) {
		settings := BlogCategorySettings{
			Name:        "Technology",
			Description: "Tech category",
			SEO: &SEOSettings{
				MetaTitle: "Tech SEO",
			},
		}

		value, err := settings.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)

		// Verify it's valid JSON
		var result BlogCategorySettings
		err = json.Unmarshal(value.([]byte), &result)
		require.NoError(t, err)
		assert.Equal(t, settings.Name, result.Name)
		assert.Equal(t, settings.Description, result.Description)
	})

	t.Run("serializes without SEO", func(t *testing.T) {
		settings := BlogCategorySettings{
			Name:        "Category",
			Description: "Description",
			SEO:         nil,
		}

		value, err := settings.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)
	})
}

func TestBlogCategorySettings_Scan(t *testing.T) {
	t.Run("deserializes valid JSON bytes", func(t *testing.T) {
		original := BlogCategorySettings{
			Name:        "Technology",
			Description: "Tech category",
			SEO: &SEOSettings{
				MetaTitle: "Tech SEO",
			},
		}

		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		var scanned BlogCategorySettings
		err = scanned.Scan(jsonData)
		require.NoError(t, err)
		assert.Equal(t, original.Name, scanned.Name)
		assert.Equal(t, original.Description, scanned.Description)
	})

	t.Run("handles nil value", func(t *testing.T) {
		var settings BlogCategorySettings
		err := settings.Scan(nil)
		require.NoError(t, err)
	})

	t.Run("handles invalid type", func(t *testing.T) {
		var settings BlogCategorySettings
		err := settings.Scan("not-a-byte-array")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type assertion to []byte failed")
	})
}

func TestBlogCategory_Validate(t *testing.T) {
	t.Run("valid category", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: "technology",
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}

		err := category.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		category := &BlogCategory{
			Slug: "technology",
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("missing slug", func(t *testing.T) {
		category := &BlogCategory{
			ID: "cat-123",
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug format - uppercase", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: "My-Slug",
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("invalid slug format - special chars", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: "my@slug",
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("invalid slug format - spaces", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: "my slug",
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("slug exactly 100 chars", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: string(make([]byte, 100)),
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}
		// Fill with valid slug chars
		for i := range category.Slug {
			category.Slug = category.Slug[:i] + "a" + category.Slug[i+1:]
		}

		err := category.Validate()
		assert.NoError(t, err)
	})

	t.Run("slug >100 chars", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: string(make([]byte, 101)),
			Settings: BlogCategorySettings{
				Name: "Technology",
			},
		}
		// Fill with valid slug chars
		for i := range category.Slug {
			category.Slug = category.Slug[:i] + "a" + category.Slug[i+1:]
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})

	t.Run("missing name", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: "technology",
			Settings: BlogCategorySettings{
				Name: "",
			},
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("name exactly 255 chars", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: "technology",
			Settings: BlogCategorySettings{
				Name: string(make([]byte, 255)),
			},
		}

		err := category.Validate()
		assert.NoError(t, err)
	})

	t.Run("name >255 chars", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat-123",
			Slug: "technology",
			Settings: BlogCategorySettings{
				Name: string(make([]byte, 256)),
			},
		}

		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must be less than 255 characters")
	})
}

func TestBlogPostSettings_Value(t *testing.T) {
	t.Run("serializes all fields", func(t *testing.T) {
		settings := BlogPostSettings{
			Title:              "Test Post",
			Template:           BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			Excerpt:            "Excerpt",
			FeaturedImageURL:   "https://example.com/image.jpg",
			Authors:            []BlogAuthor{{Name: "Author"}},
			ReadingTimeMinutes: 5,
			SEO:                &SEOSettings{MetaTitle: "SEO Title"},
		}

		value, err := settings.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)

		// Verify it's valid JSON
		var result BlogPostSettings
		err = json.Unmarshal(value.([]byte), &result)
		require.NoError(t, err)
		assert.Equal(t, settings.Title, result.Title)
		assert.Equal(t, settings.Template.TemplateID, result.Template.TemplateID)
	})

	t.Run("serializes without SEO", func(t *testing.T) {
		settings := BlogPostSettings{
			Title:    "Post",
			Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			Authors:  []BlogAuthor{},
		}

		value, err := settings.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)
	})
}

func TestBlogPostSettings_Scan(t *testing.T) {
	t.Run("deserializes valid JSON bytes", func(t *testing.T) {
		original := BlogPostSettings{
			Title:              "Test Post",
			Template:           BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			Authors:            []BlogAuthor{{Name: "Author"}},
			ReadingTimeMinutes: 5,
		}

		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		var scanned BlogPostSettings
		err = scanned.Scan(jsonData)
		require.NoError(t, err)
		assert.Equal(t, original.Title, scanned.Title)
		assert.Equal(t, original.Template.TemplateID, scanned.Template.TemplateID)
	})

	t.Run("handles nil value", func(t *testing.T) {
		var settings BlogPostSettings
		err := settings.Scan(nil)
		require.NoError(t, err)
	})

	t.Run("handles invalid type", func(t *testing.T) {
		var settings BlogPostSettings
		err := settings.Scan("not-a-byte-array")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type assertion to []byte failed")
	})
}

func TestBlogPost_Validate(t *testing.T) {
	t.Run("valid post", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			Settings: BlogPostSettings{
				Title:    "Test Post",
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		post := &BlogPost{
			CategoryID: "cat-123",
			Slug:       "test-post",
			Settings: BlogPostSettings{
				Title:    "Test Post",
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("missing CategoryID", func(t *testing.T) {
		post := &BlogPost{
			ID:   "post-123",
			Slug: "test-post",
			Settings: BlogPostSettings{
				Title:    "Test Post",
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "category_id is required")
	})

	t.Run("missing slug", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Settings: BlogPostSettings{
				Title:    "Test Post",
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug format", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "Invalid-Slug",
			Settings: BlogPostSettings{
				Title:    "Test Post",
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("slug >100 chars", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       string(make([]byte, 101)),
			Settings: BlogPostSettings{
				Title:    "Test Post",
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}
		// Fill with valid slug chars
		for i := range post.Slug {
			post.Slug = post.Slug[:i] + "a" + post.Slug[i+1:]
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})

	t.Run("missing title", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			Settings: BlogPostSettings{
				Title:    "",
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title is required")
	})

	t.Run("title exactly 500 chars", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			Settings: BlogPostSettings{
				Title:    string(make([]byte, 500)),
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		assert.NoError(t, err)
	})

	t.Run("title >500 chars", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			Settings: BlogPostSettings{
				Title:    string(make([]byte, 501)),
				Template: BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title must be less than 500 characters")
	})

	t.Run("missing template_id", func(t *testing.T) {
		post := &BlogPost{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			Settings: BlogPostSettings{
				Title:    "Test Post",
				Template: BlogPostTemplateReference{TemplateID: "", TemplateVersion: 1},
			},
		}

		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template_id is required")
	})
}

func TestBlogPost_IsDraft(t *testing.T) {
	t.Run("returns true when PublishedAt is nil", func(t *testing.T) {
		post := &BlogPost{
			PublishedAt: nil,
		}

		assert.True(t, post.IsDraft())
	})

	t.Run("returns false when PublishedAt is set", func(t *testing.T) {
		now := time.Now()
		post := &BlogPost{
			PublishedAt: &now,
		}

		assert.False(t, post.IsDraft())
	})
}

func TestBlogPost_IsPublished(t *testing.T) {
	t.Run("returns true when PublishedAt is not nil", func(t *testing.T) {
		now := time.Now()
		post := &BlogPost{
			PublishedAt: &now,
		}

		assert.True(t, post.IsPublished())
	})

	t.Run("returns false when PublishedAt is nil", func(t *testing.T) {
		post := &BlogPost{
			PublishedAt: nil,
		}

		assert.False(t, post.IsPublished())
	})
}

func TestBlogPost_GetEffectiveSEOSettings(t *testing.T) {
	t.Run("post has SEO, category nil", func(t *testing.T) {
		post := &BlogPost{
			Settings: BlogPostSettings{
				SEO: &SEOSettings{MetaTitle: "Post Title"},
			},
		}

		result := post.GetEffectiveSEOSettings(nil)
		assert.Equal(t, post.Settings.SEO, result)
	})

	t.Run("post SEO nil, category has SEO", func(t *testing.T) {
		post := &BlogPost{
			Settings: BlogPostSettings{
				SEO: nil,
			},
		}
		category := &BlogCategory{
			Settings: BlogCategorySettings{
				SEO: &SEOSettings{MetaTitle: "Category Title"},
			},
		}

		result := post.GetEffectiveSEOSettings(category)
		assert.Equal(t, category.Settings.SEO, result)
	})

	t.Run("both have SEO merges", func(t *testing.T) {
		post := &BlogPost{
			Settings: BlogPostSettings{
				SEO: &SEOSettings{
					MetaTitle:       "Post Title",
					MetaDescription: "", // Empty, should use default
				},
			},
		}
		category := &BlogCategory{
			Settings: BlogCategorySettings{
				SEO: &SEOSettings{
					MetaTitle:       "Category Title",
					MetaDescription: "Category Description",
				},
			},
		}

		result := post.GetEffectiveSEOSettings(category)
		require.NotNil(t, result)
		assert.Equal(t, "Post Title", result.MetaTitle)                 // Post value used
		assert.Equal(t, "Category Description", result.MetaDescription) // Default used
	})

	t.Run("both nil returns nil", func(t *testing.T) {
		post := &BlogPost{
			Settings: BlogPostSettings{
				SEO: nil,
			},
		}
		category := &BlogCategory{
			Settings: BlogCategorySettings{
				SEO: nil,
			},
		}

		result := post.GetEffectiveSEOSettings(category)
		assert.Nil(t, result)
	})
}

func TestCreateBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: "Technology",
			Slug: "technology",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing name", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Slug: "technology",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("name >255 chars", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: string(make([]byte, 256)),
			Slug: "technology",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must be less than 255 characters")
	})

	t.Run("missing slug", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: "Technology",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: "Technology",
			Slug: "Invalid-Slug",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("slug >100 chars", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: "Technology",
			Slug: string(make([]byte, 101)),
		}
		for i := range req.Slug {
			req.Slug = req.Slug[:i] + "a" + req.Slug[i+1:]
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})
}

func TestUpdateBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			ID:   "cat-123",
			Name: "Technology",
			Slug: "technology",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			Name: "Technology",
			Slug: "technology",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("missing name", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			ID:   "cat-123",
			Slug: "technology",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("name >255 chars", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			ID:   "cat-123",
			Name: string(make([]byte, 256)),
			Slug: "technology",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must be less than 255 characters")
	})

	t.Run("missing slug", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			ID:   "cat-123",
			Name: "Technology",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			ID:   "cat-123",
			Name: "Technology",
			Slug: "Invalid-Slug",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("slug >100 chars", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			ID:   "cat-123",
			Name: "Technology",
			Slug: string(make([]byte, 101)),
		}
		for i := range req.Slug {
			req.Slug = req.Slug[:i] + "a" + req.Slug[i+1:]
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})
}

func TestDeleteBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &DeleteBlogCategoryRequest{
			ID: "cat-123",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		req := &DeleteBlogCategoryRequest{}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestGetBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid with ID", func(t *testing.T) {
		req := &GetBlogCategoryRequest{
			ID: "cat-123",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid with slug", func(t *testing.T) {
		req := &GetBlogCategoryRequest{
			Slug: "technology",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("both empty", func(t *testing.T) {
		req := &GetBlogCategoryRequest{}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either id or slug is required")
	})

	t.Run("both provided", func(t *testing.T) {
		req := &GetBlogCategoryRequest{
			ID:   "cat-123",
			Slug: "technology",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})
}

func TestCreateBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			CategoryID:      "cat-123",
			Slug:            "test-post",
			Title:           "Test Post",
			TemplateID:      "tpl-1",
			TemplateVersion: 1,
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing category_id", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			Slug:       "test-post",
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "category_id is required")
	})

	t.Run("missing slug", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			CategoryID: "cat-123",
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			CategoryID: "cat-123",
			Slug:       "Invalid-Slug",
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("slug >100 chars", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			CategoryID: "cat-123",
			Slug:       string(make([]byte, 101)),
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}
		for i := range req.Slug {
			req.Slug = req.Slug[:i] + "a" + req.Slug[i+1:]
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})

	t.Run("missing title", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			CategoryID: "cat-123",
			Slug:       "test-post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title is required")
	})

	t.Run("title >500 chars", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			CategoryID: "cat-123",
			Slug:       "test-post",
			Title:      string(make([]byte, 501)),
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title must be less than 500 characters")
	})

	t.Run("missing template_id", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			CategoryID: "cat-123",
			Slug:       "test-post",
			Title:      "Test Post",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template_id is required")
	})
}

func TestUpdateBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:              "post-123",
			CategoryID:      "cat-123",
			Slug:            "test-post",
			Title:           "Test Post",
			TemplateID:      "tpl-1",
			TemplateVersion: 1,
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			CategoryID: "cat-123",
			Slug:       "test-post",
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("missing category_id", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post-123",
			Slug:       "test-post",
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "category_id is required")
	})

	t.Run("missing slug", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post-123",
			CategoryID: "cat-123",
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "Invalid-Slug",
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase letters")
	})

	t.Run("slug >100 chars", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       string(make([]byte, 101)),
			Title:      "Test Post",
			TemplateID: "tpl-1",
		}
		for i := range req.Slug {
			req.Slug = req.Slug[:i] + "a" + req.Slug[i+1:]
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})

	t.Run("missing title", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title is required")
	})

	t.Run("title >500 chars", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			Title:      string(make([]byte, 501)),
			TemplateID: "tpl-1",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title must be less than 500 characters")
	})

	t.Run("missing template_id", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post-123",
			CategoryID: "cat-123",
			Slug:       "test-post",
			Title:      "Test Post",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template_id is required")
	})
}

func TestDeleteBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &DeleteBlogPostRequest{
			ID: "post-123",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		req := &DeleteBlogPostRequest{}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestPublishBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &PublishBlogPostRequest{
			ID: "post-123",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		req := &PublishBlogPostRequest{}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestUnpublishBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &UnpublishBlogPostRequest{
			ID: "post-123",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		req := &UnpublishBlogPostRequest{}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestGetBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid with ID", func(t *testing.T) {
		req := &GetBlogPostRequest{
			ID: "post-123",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid with slug", func(t *testing.T) {
		req := &GetBlogPostRequest{
			Slug: "test-post",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("both empty", func(t *testing.T) {
		req := &GetBlogPostRequest{}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either id or slug is required")
	})

	t.Run("both provided", func(t *testing.T) {
		req := &GetBlogPostRequest{
			ID:   "post-123",
			Slug: "test-post",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})
}

func TestNormalizeSlug(t *testing.T) {
	t.Run("normal string", func(t *testing.T) {
		result := NormalizeSlug("My Blog Post")
		assert.Equal(t, "my-blog-post", result)
	})

	t.Run("uppercase", func(t *testing.T) {
		result := NormalizeSlug("MY_POST")
		assert.Equal(t, "my-post", result)
	})

	t.Run("spaces", func(t *testing.T) {
		result := NormalizeSlug("Post with  Spaces")
		assert.Equal(t, "post-with-spaces", result)
	})

	t.Run("special characters", func(t *testing.T) {
		result := NormalizeSlug("Post@#$%Special")
		assert.Equal(t, "postspecial", result)
	})

	t.Run("empty string", func(t *testing.T) {
		result := NormalizeSlug("")
		assert.Equal(t, "", result)
	})

	t.Run("leading and trailing hyphens", func(t *testing.T) {
		result := NormalizeSlug("---post---")
		assert.Equal(t, "post", result)
	})

	t.Run("consecutive hyphens", func(t *testing.T) {
		result := NormalizeSlug("post--with---hyphens")
		assert.Equal(t, "post-with-hyphens", result)
	})

	t.Run("underscores converted to hyphens", func(t *testing.T) {
		result := NormalizeSlug("post_with_underscores")
		assert.Equal(t, "post-with-underscores", result)
	})
}

func TestBlogThemeFiles_Value(t *testing.T) {
	t.Run("serializes all fields", func(t *testing.T) {
		files := BlogThemeFiles{
			HomeLiquid:     "<div>Home</div>",
			CategoryLiquid: "<div>Category</div>",
			PostLiquid:     "<div>Post</div>",
			HeaderLiquid:   "<div>Header</div>",
			FooterLiquid:   "<div>Footer</div>",
			SharedLiquid:   "<div>Shared</div>",
			StylesCSS:      "body { margin: 0; }",
			ScriptsJS:      "console.log('test');",
		}

		value, err := files.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)

		// Verify it's valid JSON
		var result BlogThemeFiles
		err = json.Unmarshal(value.([]byte), &result)
		require.NoError(t, err)
		assert.Equal(t, files.HomeLiquid, result.HomeLiquid)
		assert.Equal(t, files.StylesCSS, result.StylesCSS)
	})

	t.Run("serializes empty fields", func(t *testing.T) {
		files := BlogThemeFiles{}

		value, err := files.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)
	})
}

func TestBlogThemeFiles_Scan(t *testing.T) {
	t.Run("deserializes valid JSON bytes", func(t *testing.T) {
		original := BlogThemeFiles{
			HomeLiquid: "<div>Home</div>",
			StylesCSS:  "body { margin: 0; }",
		}

		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		var scanned BlogThemeFiles
		err = scanned.Scan(jsonData)
		require.NoError(t, err)
		assert.Equal(t, original.HomeLiquid, scanned.HomeLiquid)
		assert.Equal(t, original.StylesCSS, scanned.StylesCSS)
	})

	t.Run("handles nil value", func(t *testing.T) {
		var files BlogThemeFiles
		err := files.Scan(nil)
		require.NoError(t, err)
	})

	t.Run("handles invalid type", func(t *testing.T) {
		var files BlogThemeFiles
		err := files.Scan("not-a-byte-array")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type assertion to []byte failed")
	})
}

func TestBlogTheme_Validate(t *testing.T) {
	t.Run("valid theme", func(t *testing.T) {
		theme := &BlogTheme{
			Version: 1,
		}

		err := theme.Validate()
		assert.NoError(t, err)
	})

	t.Run("version <=0", func(t *testing.T) {
		theme := &BlogTheme{
			Version: 0,
		}

		err := theme.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})

	t.Run("negative version", func(t *testing.T) {
		theme := &BlogTheme{
			Version: -1,
		}

		err := theme.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})
}

func TestBlogTheme_IsPublished(t *testing.T) {
	t.Run("returns true when PublishedAt is not nil", func(t *testing.T) {
		now := time.Now()
		theme := &BlogTheme{
			PublishedAt: &now,
		}

		assert.True(t, theme.IsPublished())
	})

	t.Run("returns false when PublishedAt is nil", func(t *testing.T) {
		theme := &BlogTheme{
			PublishedAt: nil,
		}

		assert.False(t, theme.IsPublished())
	})
}

func TestCreateBlogThemeRequest_Validate(t *testing.T) {
	t.Run("always returns nil", func(t *testing.T) {
		req := &CreateBlogThemeRequest{}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("with files returns nil", func(t *testing.T) {
		req := &CreateBlogThemeRequest{
			Files: BlogThemeFiles{
				HomeLiquid: "<div>Home</div>",
			},
		}

		err := req.Validate()
		assert.NoError(t, err)
	})
}

func TestUpdateBlogThemeRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &UpdateBlogThemeRequest{
			Version: 1,
			Files:   BlogThemeFiles{},
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("version <=0", func(t *testing.T) {
		req := &UpdateBlogThemeRequest{
			Version: 0,
			Files:   BlogThemeFiles{},
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})

	t.Run("negative version", func(t *testing.T) {
		req := &UpdateBlogThemeRequest{
			Version: -1,
			Files:   BlogThemeFiles{},
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})
}

func TestPublishBlogThemeRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &PublishBlogThemeRequest{
			Version: 1,
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("version <=0", func(t *testing.T) {
		req := &PublishBlogThemeRequest{
			Version: 0,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})

	t.Run("negative version", func(t *testing.T) {
		req := &PublishBlogThemeRequest{
			Version: -1,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})
}

func TestGetBlogThemeRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &GetBlogThemeRequest{
			Version: 1,
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("version <=0", func(t *testing.T) {
		req := &GetBlogThemeRequest{
			Version: 0,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})

	t.Run("negative version", func(t *testing.T) {
		req := &GetBlogThemeRequest{
			Version: -1,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version must be positive")
	})
}

func TestListBlogThemesRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &ListBlogThemesRequest{
			Limit:  10,
			Offset: 0,
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 10, req.Limit)
		assert.Equal(t, 0, req.Offset)
	})

	t.Run("limit defaults to 50 if <=0", func(t *testing.T) {
		req := &ListBlogThemesRequest{
			Limit: 0,
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 50, req.Limit)
	})

	t.Run("negative limit defaults to 50", func(t *testing.T) {
		req := &ListBlogThemesRequest{
			Limit: -5,
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 50, req.Limit)
	})

	t.Run("limit >100 capped to 100", func(t *testing.T) {
		req := &ListBlogThemesRequest{
			Limit: 200,
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 100, req.Limit)
	})

	t.Run("negative offset set to 0", func(t *testing.T) {
		req := &ListBlogThemesRequest{
			Limit:  10,
			Offset: -5,
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 0, req.Offset)
	})

	t.Run("valid offset unchanged", func(t *testing.T) {
		req := &ListBlogThemesRequest{
			Limit:  10,
			Offset: 20,
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 20, req.Offset)
	})
}
