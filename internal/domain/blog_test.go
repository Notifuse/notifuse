package domain

import (
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSEOSettings_Value(t *testing.T) {
	t.Run("successful serialization", func(t *testing.T) {
		seo := SEOSettings{
			MetaTitle:       "Test Title",
			MetaDescription: "Test Description",
			OGTitle:         "OG Title",
			OGDescription:   "OG Description",
			OGImage:         "https://example.com/image.png",
			CanonicalURL:    "https://example.com/page",
			Keywords:        []string{"test", "seo"},
		}

		value, err := seo.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)

		// Verify it's valid JSON
		var decoded SEOSettings
		err = json.Unmarshal(value.([]byte), &decoded)
		require.NoError(t, err)
		assert.Equal(t, seo.MetaTitle, decoded.MetaTitle)
	})
}

func TestSEOSettings_Scan(t *testing.T) {
	t.Run("successful deserialization", func(t *testing.T) {
		jsonData := []byte(`{"meta_title":"Test","keywords":["test"]}`)
		var seo SEOSettings
		err := seo.Scan(jsonData)
		require.NoError(t, err)
		assert.Equal(t, "Test", seo.MetaTitle)
		assert.Equal(t, []string{"test"}, seo.Keywords)
	})

	t.Run("nil value", func(t *testing.T) {
		var seo SEOSettings
		err := seo.Scan(nil)
		require.NoError(t, err)
	})

	t.Run("invalid type", func(t *testing.T) {
		var seo SEOSettings
		err := seo.Scan("invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type assertion")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		var seo SEOSettings
		err := seo.Scan([]byte("invalid json"))
		require.Error(t, err)
	})
}

func TestSEOSettings_MergeWithDefaults(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		var seo *SEOSettings
		result := seo.MergeWithDefaults(nil)
		assert.NotNil(t, result)
	})

	t.Run("current nil, defaults provided", func(t *testing.T) {
		defaults := &SEOSettings{MetaTitle: "Default"}
		var seo *SEOSettings
		result := seo.MergeWithDefaults(defaults)
		assert.Equal(t, "Default", result.MetaTitle)
	})

	t.Run("current provided, defaults nil", func(t *testing.T) {
		seo := &SEOSettings{MetaTitle: "Current"}
		result := seo.MergeWithDefaults(nil)
		assert.Equal(t, "Current", result.MetaTitle)
	})

	t.Run("merge with empty fields", func(t *testing.T) {
		seo := &SEOSettings{
			MetaTitle: "Current Title",
			OGTitle:   "Current OG",
		}
		defaults := &SEOSettings{
			MetaTitle:       "Default Title",
			MetaDescription: "Default Description",
			OGDescription:   "Default OG Desc",
			Keywords:        []string{"default"},
		}
		result := seo.MergeWithDefaults(defaults)
		assert.Equal(t, "Current Title", result.MetaTitle)
		assert.Equal(t, "Default Description", result.MetaDescription)
		assert.Equal(t, "Current OG", result.OGTitle)
		assert.Equal(t, "Default OG Desc", result.OGDescription)
		assert.Equal(t, []string{"default"}, result.Keywords)
	})

	t.Run("current values preferred", func(t *testing.T) {
		seo := &SEOSettings{
			MetaTitle:       "Current",
			MetaDescription: "Current Desc",
			OGTitle:         "Current OG",
			OGDescription:   "Current OG Desc",
			OGImage:         "current.png",
			CanonicalURL:    "https://current.com",
			Keywords:        []string{"current"},
		}
		defaults := &SEOSettings{
			MetaTitle:       "Default",
			MetaDescription: "Default Desc",
			Keywords:        []string{"default"},
		}
		result := seo.MergeWithDefaults(defaults)
		assert.Equal(t, "Current", result.MetaTitle)
		assert.Equal(t, "Current Desc", result.MetaDescription)
		assert.Equal(t, []string{"current"}, result.Keywords)
	})
}

func TestBlogCategorySettings_Value(t *testing.T) {
	t.Run("successful serialization", func(t *testing.T) {
		settings := BlogCategorySettings{
			Name:        "Tech Blog",
			Description: "Technology posts",
			SEO:         &SEOSettings{MetaTitle: "Tech"},
		}

		value, err := settings.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)
	})
}

func TestBlogCategorySettings_Scan(t *testing.T) {
	t.Run("successful deserialization", func(t *testing.T) {
		jsonData := []byte(`{"name":"Tech","description":"Tech posts"}`)
		var settings BlogCategorySettings
		err := settings.Scan(jsonData)
		require.NoError(t, err)
		assert.Equal(t, "Tech", settings.Name)
	})

	t.Run("nil value", func(t *testing.T) {
		var settings BlogCategorySettings
		err := settings.Scan(nil)
		require.NoError(t, err)
	})

	t.Run("invalid type", func(t *testing.T) {
		var settings BlogCategorySettings
		err := settings.Scan(123)
		require.Error(t, err)
	})
}

func TestBlogCategory_Validate(t *testing.T) {
	t.Run("valid category", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat123",
			Slug: "tech-blog",
			Settings: BlogCategorySettings{
				Name: "Tech Blog",
			},
		}
		err := category.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		category := &BlogCategory{
			Slug: "tech-blog",
			Settings: BlogCategorySettings{
				Name: "Tech Blog",
			},
		}
		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("missing slug", func(t *testing.T) {
		category := &BlogCategory{
			ID: "cat123",
			Settings: BlogCategorySettings{
				Name: "Tech Blog",
			},
		}
		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug format", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat123",
			Slug: "Tech_Blog!",
			Settings: BlogCategorySettings{
				Name: "Tech Blog",
			},
		}
		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase")
	})

	t.Run("slug too long", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat123",
			Slug: "this-is-a-very-long-slug-that-exceeds-the-maximum-allowed-length-for-slugs-in-the-system-and-should-fail-validation",
			Settings: BlogCategorySettings{
				Name: "Tech Blog",
			},
		}
		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})

	t.Run("missing name", func(t *testing.T) {
		category := &BlogCategory{
			ID:   "cat123",
			Slug: "tech-blog",
			Settings: BlogCategorySettings{
				Name: "",
			},
		}
		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("name too long", func(t *testing.T) {
		longName := ""
		for i := 0; i < 256; i++ {
			longName += "a"
		}
		category := &BlogCategory{
			ID:   "cat123",
			Slug: "tech-blog",
			Settings: BlogCategorySettings{
				Name: longName,
			},
		}
		err := category.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must be less than 255 characters")
	})
}

func TestBlogPostSettings_Value(t *testing.T) {
	t.Run("successful serialization", func(t *testing.T) {
		settings := BlogPostSettings{
			Title: "Test Post",
			Template: BlogPostTemplateReference{
				TemplateID:      "tpl123",
				TemplateVersion: 1,
				TemplateData:    MapOfAny{"key": "value"},
			},
			Authors: []BlogAuthor{{Name: "John"}},
		}

		value, err := settings.Value()
		require.NoError(t, err)
		assert.NotNil(t, value)
	})
}

func TestBlogPostSettings_Scan(t *testing.T) {
	t.Run("successful deserialization", func(t *testing.T) {
		jsonData := []byte(`{"title":"Test","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`)
		var settings BlogPostSettings
		err := settings.Scan(jsonData)
		require.NoError(t, err)
		assert.Equal(t, "Test", settings.Title)
	})

	t.Run("nil value", func(t *testing.T) {
		var settings BlogPostSettings
		err := settings.Scan(nil)
		require.NoError(t, err)
	})
}

func TestBlogPost_Validate(t *testing.T) {
	t.Run("valid post", func(t *testing.T) {
		post := &BlogPost{
			ID:   "post123",
			Slug: "my-first-post",
			Settings: BlogPostSettings{
				Title: "My First Post",
				Template: BlogPostTemplateReference{
					TemplateID: "tpl123",
				},
			},
		}
		err := post.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		post := &BlogPost{
			Slug: "my-first-post",
			Settings: BlogPostSettings{
				Title: "My First Post",
				Template: BlogPostTemplateReference{
					TemplateID: "tpl123",
				},
			},
		}
		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("missing slug", func(t *testing.T) {
		post := &BlogPost{
			ID: "post123",
			Settings: BlogPostSettings{
				Title: "My First Post",
				Template: BlogPostTemplateReference{
					TemplateID: "tpl123",
				},
			},
		}
		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug", func(t *testing.T) {
		post := &BlogPost{
			ID:   "post123",
			Slug: "Invalid Slug!",
			Settings: BlogPostSettings{
				Title: "My First Post",
				Template: BlogPostTemplateReference{
					TemplateID: "tpl123",
				},
			},
		}
		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase")
	})

	t.Run("slug too long", func(t *testing.T) {
		post := &BlogPost{
			ID:   "post123",
			Slug: "this-is-a-very-long-slug-that-exceeds-the-maximum-allowed-length-for-slugs-in-the-system-and-should-fail",
			Settings: BlogPostSettings{
				Title: "My First Post",
				Template: BlogPostTemplateReference{
					TemplateID: "tpl123",
				},
			},
		}
		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must be less than 100 characters")
	})

	t.Run("missing title", func(t *testing.T) {
		post := &BlogPost{
			ID:   "post123",
			Slug: "my-first-post",
			Settings: BlogPostSettings{
				Template: BlogPostTemplateReference{
					TemplateID: "tpl123",
				},
			},
		}
		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title is required")
	})

	t.Run("title too long", func(t *testing.T) {
		longTitle := ""
		for i := 0; i < 501; i++ {
			longTitle += "a"
		}
		post := &BlogPost{
			ID:   "post123",
			Slug: "my-first-post",
			Settings: BlogPostSettings{
				Title: longTitle,
				Template: BlogPostTemplateReference{
					TemplateID: "tpl123",
				},
			},
		}
		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title must be less than 500 characters")
	})

	t.Run("missing template_id", func(t *testing.T) {
		post := &BlogPost{
			ID:   "post123",
			Slug: "my-first-post",
			Settings: BlogPostSettings{
				Title: "My First Post",
			},
		}
		err := post.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template_id is required")
	})
}

func TestBlogPost_IsDraft(t *testing.T) {
	t.Run("draft when published_at is nil", func(t *testing.T) {
		post := &BlogPost{PublishedAt: nil}
		assert.True(t, post.IsDraft())
	})

	t.Run("not draft when published_at is set", func(t *testing.T) {
		now := time.Now()
		post := &BlogPost{PublishedAt: &now}
		assert.False(t, post.IsDraft())
	})
}

func TestBlogPost_IsPublished(t *testing.T) {
	t.Run("not published when published_at is nil", func(t *testing.T) {
		post := &BlogPost{PublishedAt: nil}
		assert.False(t, post.IsPublished())
	})

	t.Run("published when published_at is set", func(t *testing.T) {
		now := time.Now()
		post := &BlogPost{PublishedAt: &now}
		assert.True(t, post.IsPublished())
	})
}

func TestBlogPost_GetEffectiveSEOSettings(t *testing.T) {
	t.Run("no category", func(t *testing.T) {
		seo := &SEOSettings{MetaTitle: "Post Title"}
		post := &BlogPost{
			Settings: BlogPostSettings{SEO: seo},
		}
		result := post.GetEffectiveSEOSettings(nil)
		assert.Equal(t, seo, result)
	})

	t.Run("no post SEO, use category", func(t *testing.T) {
		categorySEO := &SEOSettings{MetaTitle: "Category Title"}
		category := &BlogCategory{
			Settings: BlogCategorySettings{SEO: categorySEO},
		}
		post := &BlogPost{
			Settings: BlogPostSettings{SEO: nil},
		}
		result := post.GetEffectiveSEOSettings(category)
		assert.Equal(t, categorySEO, result)
	})

	t.Run("merge post and category SEO", func(t *testing.T) {
		categorySEO := &SEOSettings{
			MetaTitle:       "Category Title",
			MetaDescription: "Category Desc",
		}
		postSEO := &SEOSettings{
			MetaTitle: "Post Title",
		}
		category := &BlogCategory{
			Settings: BlogCategorySettings{SEO: categorySEO},
		}
		post := &BlogPost{
			Settings: BlogPostSettings{SEO: postSEO},
		}
		result := post.GetEffectiveSEOSettings(category)
		assert.Equal(t, "Post Title", result.MetaTitle)
		assert.Equal(t, "Category Desc", result.MetaDescription)
	})
}

func TestCreateBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: "Tech",
			Slug: "tech",
		}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing name", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Slug: "tech",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("name too long", func(t *testing.T) {
		longName := ""
		for i := 0; i < 256; i++ {
			longName += "a"
		}
		req := &CreateBlogCategoryRequest{
			Name: longName,
			Slug: "tech",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must be less than 255")
	})

	t.Run("missing slug", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: "Tech",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("invalid slug", func(t *testing.T) {
		req := &CreateBlogCategoryRequest{
			Name: "Tech",
			Slug: "Tech Blog!",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug must contain only lowercase")
	})
}

func TestUpdateBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			ID:   "cat123",
			Name: "Tech",
			Slug: "tech",
		}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		req := &UpdateBlogCategoryRequest{
			Name: "Tech",
			Slug: "tech",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestDeleteBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &DeleteBlogCategoryRequest{ID: "cat123"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		req := &DeleteBlogCategoryRequest{}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestGetBlogCategoryRequest_Validate(t *testing.T) {
	t.Run("valid with id", func(t *testing.T) {
		req := &GetBlogCategoryRequest{ID: "cat123"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid with slug", func(t *testing.T) {
		req := &GetBlogCategoryRequest{Slug: "tech"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing both", func(t *testing.T) {
		req := &GetBlogCategoryRequest{}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either id or slug is required")
	})
}

func TestCreateBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			Slug:       "my-post",
			Title:      "My Post",
			TemplateID: "tpl123",
		}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing slug", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			Title:      "My Post",
			TemplateID: "tpl123",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "slug is required")
	})

	t.Run("missing title", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			Slug:       "my-post",
			TemplateID: "tpl123",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title is required")
	})

	t.Run("missing template_id", func(t *testing.T) {
		req := &CreateBlogPostRequest{
			Slug:  "my-post",
			Title: "My Post",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template_id is required")
	})
}

func TestUpdateBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			ID:         "post123",
			Slug:       "my-post",
			Title:      "My Post",
			TemplateID: "tpl123",
		}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		req := &UpdateBlogPostRequest{
			Slug:       "my-post",
			Title:      "My Post",
			TemplateID: "tpl123",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestPublishBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &PublishBlogPostRequest{ID: "post123"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		req := &PublishBlogPostRequest{}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestUnpublishBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &UnpublishBlogPostRequest{ID: "post123"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		req := &UnpublishBlogPostRequest{}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestGetBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid with id", func(t *testing.T) {
		req := &GetBlogPostRequest{ID: "post123"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("valid with slug", func(t *testing.T) {
		req := &GetBlogPostRequest{Slug: "my-post"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing both", func(t *testing.T) {
		req := &GetBlogPostRequest{}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either id or slug is required")
	})
}

func TestListBlogPostsRequest_Validate(t *testing.T) {
	t.Run("valid request with defaults", func(t *testing.T) {
		req := &ListBlogPostsRequest{}
		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, BlogPostStatusAll, req.Status)
		assert.Equal(t, 50, req.Limit)
	})

	t.Run("valid with custom params", func(t *testing.T) {
		req := &ListBlogPostsRequest{
			Status: BlogPostStatusPublished,
			Limit:  20,
			Offset: 10,
		}
		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 20, req.Limit)
	})

	t.Run("invalid status", func(t *testing.T) {
		req := &ListBlogPostsRequest{
			Status: "invalid",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status")
	})

	t.Run("limit exceeds maximum", func(t *testing.T) {
		req := &ListBlogPostsRequest{
			Limit: 200,
		}
		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 100, req.Limit) // Should be capped at 100
	})

	t.Run("zero limit sets default", func(t *testing.T) {
		req := &ListBlogPostsRequest{
			Limit: 0,
		}
		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 50, req.Limit)
	})
}

func TestNormalizeSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Tech_Blog", "tech-blog"},
		{"My First Post!", "my-first-post"},
		{"  trimmed  ", "trimmed"},
		{"multiple---hyphens", "multiple---hyphens"},
		{"CamelCase", "camelcase"},
		{"123-numbers", "123-numbers"},
		{"special@#$chars", "specialchars"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeSlug(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeleteBlogPostRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &DeleteBlogPostRequest{ID: "post123"}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing id", func(t *testing.T) {
		req := &DeleteBlogPostRequest{}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

// TestDriverValueScannerInterfaces ensures the types implement database interfaces correctly
func TestDriverValueScannerInterfaces(t *testing.T) {
	t.Run("SEOSettings implements driver.Valuer", func(t *testing.T) {
		var _ driver.Valuer = SEOSettings{}
	})

	t.Run("BlogCategorySettings implements driver.Valuer", func(t *testing.T) {
		var _ driver.Valuer = BlogCategorySettings{}
	})

	t.Run("BlogPostSettings implements driver.Valuer", func(t *testing.T) {
		var _ driver.Valuer = BlogPostSettings{}
	})
}

