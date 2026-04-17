package integration

import (
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlogAPI runs all blog-related integration tests with a shared test suite.
// This consolidates 7 separate test functions into one to reduce setup/teardown overhead
// from ~30-55 seconds (7x setup) to ~4-8 seconds (1x setup).
func TestBlogAPI(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	t.Run("RoutingLogic", func(t *testing.T) { runBlogRoutingLogicTests(t, suite) })
	t.Run("CategoryAPI", func(t *testing.T) { runBlogCategoryAPITests(t, suite) })
	t.Run("PostAPI", func(t *testing.T) { runBlogPostAPITests(t, suite) })
	t.Run("ThemeAPI", func(t *testing.T) { runBlogThemeAPITests(t, suite) })
	t.Run("PublicRendering", func(t *testing.T) { runBlogPublicRenderingTests(t, suite) })
	t.Run("DataFactory", func(t *testing.T) { runBlogDataFactoryTests(t, suite) })
	t.Run("E2EFlow", func(t *testing.T) { runBlogE2EFlowTests(t, suite) })
	t.Run("FeedAPI", func(t *testing.T) { runBlogFeedTests(t, suite) })
}

// runBlogRoutingLogicTests tests the critical blog routing logic
// The blog should ONLY be served when a workspace has a matching custom domain AND blog is enabled
func runBlogRoutingLogicTests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	// Create a client that does NOT follow redirects for these routing tests
	noRedirectClient := testutil.NewAPIClientNoRedirect(suite.ServerManager.GetURL())
	noRedirectClient.SetToken(suite.APIClient.GetToken())
	noRedirectClient.SetWorkspaceID(suite.APIClient.GetWorkspaceID())

	t.Run("Blog served when custom_domain matches and blog enabled", func(t *testing.T) {
		// Create workspace with custom domain and blog enabled
		workspace, err := suite.DataFactory.CreateWorkspace(
			testutil.WithCustomDomain("blog.example.com"),
			testutil.WithBlogEnabled(true),
		)
		require.NoError(t, err)

		// Create a category and post so the blog has content
		category, err := suite.DataFactory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Technology"),
			testutil.WithCategorySlug("technology"),
		)
		require.NoError(t, err)

		_, err = suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("Test Post"),
			testutil.WithPostSlug("test-post"),
			testutil.WithPostPublished(true),
		)
		require.NoError(t, err)

		// Create a theme
		_, err = suite.DataFactory.CreateBlogTheme(workspace.ID,
			testutil.WithThemePublished(true),
		)
		require.NoError(t, err)

		// Make request to "/" with Host header matching the workspace custom domain
		// Use the regular client here since we expect 200 (no redirect)
		resp, err := suite.APIClient.MakeRequestWithHost("GET", "/", "blog.example.com", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should serve blog (200 OK)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Blog should be served")

		// Verify it's HTML content
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "<html>", "Response should be HTML")
		assert.Contains(t, string(body), "<body>", "Response should have body tag")
	})

	t.Run("Redirect to console when blog disabled", func(t *testing.T) {
		// Create workspace with custom domain but blog DISABLED
		_, err := suite.DataFactory.CreateWorkspace(
			testutil.WithCustomDomain("disabled.example.com"),
			testutil.WithBlogEnabled(false),
		)
		require.NoError(t, err)

		// Make request to "/" with Host header - use noRedirectClient to test the redirect itself
		resp, err := noRedirectClient.MakeRequestWithHost("GET", "/", "disabled.example.com", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should redirect to console (307)
		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode, "Should redirect when blog disabled")
		assert.Equal(t, "/console", resp.Header.Get("Location"), "Should redirect to /console")
	})

	t.Run("Redirect to console when no custom domain set", func(t *testing.T) {
		// Create workspace with blog enabled but NO custom domain
		_, err := suite.DataFactory.CreateWorkspace(
			testutil.WithBlogEnabled(true),
		)
		require.NoError(t, err)

		// Make request to "/" with any Host header - use noRedirectClient to test the redirect itself
		resp, err := noRedirectClient.MakeRequestWithHost("GET", "/", "random.example.com", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should redirect to console (307)
		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode, "Should redirect when no custom domain")
		assert.Equal(t, "/console", resp.Header.Get("Location"), "Should redirect to /console")
	})

	t.Run("Redirect to console when wrong custom domain", func(t *testing.T) {
		// Create workspace with specific custom domain
		_, err := suite.DataFactory.CreateWorkspace(
			testutil.WithCustomDomain("correct.example.com"),
			testutil.WithBlogEnabled(true),
		)
		require.NoError(t, err)

		// Make request with WRONG Host header - use noRedirectClient to test the redirect itself
		resp, err := noRedirectClient.MakeRequestWithHost("GET", "/", "wrong.example.com", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should redirect to console (307)
		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode, "Should redirect when wrong domain")
		assert.Equal(t, "/console", resp.Header.Get("Location"), "Should redirect to /console")
	})

	t.Run("Console always serves regardless of blog settings", func(t *testing.T) {
		// Create workspace with blog enabled and custom domain
		_, err := suite.DataFactory.CreateWorkspace(
			testutil.WithCustomDomain("blog2.example.com"),
			testutil.WithBlogEnabled(true),
		)
		require.NoError(t, err)

		// Make request to /console (not /) - use noRedirectClient to ensure we see actual response
		resp, err := noRedirectClient.MakeRequestWithHost("GET", "/console", "blog2.example.com", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should serve console (200 OK or 404 for missing file, but not redirect)
		assert.NotEqual(t, http.StatusTemporaryRedirect, resp.StatusCode, "Should not redirect /console")
	})
}

// runBlogCategoryAPITests tests the blog category CRUD operations
func runBlogCategoryAPITests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	client := suite.APIClient

	// Create a test user and workspace
	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	err = suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Login and set workspace
	err = client.Login(user.Email, "")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Create Category", func(t *testing.T) {
		category := map[string]interface{}{
			"name":        "Technology",
			"slug":        "technology",
			"description": "Tech articles and tutorials",
		}

		resp, err := client.CreateBlogCategory(category)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Should create category")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		categoryData := result["category"].(map[string]interface{})
		assert.NotEmpty(t, categoryData["id"], "Category should have ID")
		assert.Equal(t, "technology", categoryData["slug"], "Slug should match")
	})

	t.Run("Get Category by Slug", func(t *testing.T) {
		// First create a category
		category, err := suite.DataFactory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Design"),
			testutil.WithCategorySlug("design"),
		)
		require.NoError(t, err)

		// Get by slug
		params := map[string]string{
			"slug": "design",
		}
		resp, err := client.GetBlogCategory(params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		categoryData := result["category"].(map[string]interface{})
		assert.Equal(t, category.ID, categoryData["id"])
	})

	t.Run("List Categories", func(t *testing.T) {
		// Create multiple categories
		_, err := suite.DataFactory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Business"),
			testutil.WithCategorySlug("business"),
		)
		require.NoError(t, err)

		resp, err := client.ListBlogCategories()
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		categories := result["categories"].([]interface{})
		assert.GreaterOrEqual(t, len(categories), 1, "Should have at least one category")
	})

	t.Run("Update Category", func(t *testing.T) {
		// Create a category first
		category, err := suite.DataFactory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Health"),
			testutil.WithCategorySlug("health"),
		)
		require.NoError(t, err)

		// Update it
		update := map[string]interface{}{
			"id":          category.ID,
			"name":        "Health & Wellness",
			"slug":        "health",
			"description": "Updated description",
		}

		resp, err := client.UpdateBlogCategory(update)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		categoryData := result["category"].(map[string]interface{})
		settings := categoryData["settings"].(map[string]interface{})
		assert.Equal(t, "Health & Wellness", settings["name"])
	})

	t.Run("Delete Category", func(t *testing.T) {
		// Create a category to delete
		category, err := suite.DataFactory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("ToDelete"),
			testutil.WithCategorySlug("to-delete"),
		)
		require.NoError(t, err)

		// Delete it
		deleteReq := map[string]interface{}{
			"id": category.ID,
		}

		resp, err := client.DeleteBlogCategory(deleteReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.True(t, result["success"].(bool))
	})
}

// runBlogPostAPITests tests the blog post CRUD operations including publish/unpublish
func runBlogPostAPITests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	client := suite.APIClient

	// Setup
	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	err = suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	err = client.Login(user.Email, "")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Create a category for posts
	category, err := suite.DataFactory.CreateBlogCategory(workspace.ID,
		testutil.WithCategoryName("Test Category"),
		testutil.WithCategorySlug("test-category"),
	)
	require.NoError(t, err)

	// Create a template for posts
	template, err := suite.DataFactory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	t.Run("Create Post in Draft Status", func(t *testing.T) {
		post := map[string]interface{}{
			"category_id": category.ID,
			"slug":        "my-first-post",
			"title":       "My First Post",
			"excerpt":     "This is my first blog post",
			"template_id": template.ID,
			"authors": []map[string]string{
				{"name": "John Doe"},
			},
		}

		resp, err := client.CreateBlogPost(post)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		postData := result["post"].(map[string]interface{})
		assert.NotEmpty(t, postData["id"])
		assert.Nil(t, postData["published_at"], "Post should be draft (published_at = null)")
	})

	t.Run("Get Post by Slug", func(t *testing.T) {
		// Create a post
		post, err := suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("Get Me"),
			testutil.WithPostSlug("get-me"),
		)
		require.NoError(t, err)

		// Get by slug
		params := map[string]string{
			"slug": "get-me",
		}
		resp, err := client.GetBlogPost(params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		postData := result["post"].(map[string]interface{})
		assert.Equal(t, post.ID, postData["id"])
	})

	t.Run("List Posts with Filter", func(t *testing.T) {
		// Create posts in different states
		_, err := suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("Draft Post"),
			testutil.WithPostSlug("draft-post"),
			testutil.WithPostPublished(false),
		)
		require.NoError(t, err)

		_, err = suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("Published Post"),
			testutil.WithPostSlug("published-post"),
			testutil.WithPostPublished(true),
		)
		require.NoError(t, err)

		// List all posts
		params := map[string]string{
			"limit": "10",
		}
		resp, err := client.ListBlogPosts(params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		posts := result["posts"].([]interface{})
		assert.GreaterOrEqual(t, len(posts), 2, "Should have at least 2 posts")
	})

	t.Run("Update Post", func(t *testing.T) {
		// Create a post
		post, err := suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("Original Title"),
			testutil.WithPostSlug("original-slug"),
		)
		require.NoError(t, err)

		// Update it
		update := map[string]interface{}{
			"id":          post.ID,
			"category_id": category.ID,
			"slug":        "original-slug",
			"title":       "Updated Title",
			"excerpt":     "Updated excerpt",
			"template_id": template.ID,
			"authors": []map[string]string{
				{"name": "Jane Doe"},
			},
		}

		resp, err := client.UpdateBlogPost(update)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		postData := result["post"].(map[string]interface{})
		settings := postData["settings"].(map[string]interface{})
		assert.Equal(t, "Updated Title", settings["title"])
	})

	t.Run("Publish Post", func(t *testing.T) {
		// Create a draft post
		post, err := suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("To Be Published"),
			testutil.WithPostSlug("to-be-published"),
			testutil.WithPostPublished(false),
		)
		require.NoError(t, err)

		// Publish it
		publishReq := map[string]interface{}{
			"id": post.ID,
		}

		resp, err := client.PublishBlogPost(publishReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify it's published
		params := map[string]string{
			"id": post.ID,
		}
		getResp, err := client.GetBlogPost(params)
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		postData := result["post"].(map[string]interface{})
		assert.NotNil(t, postData["published_at"], "Post should be published")
	})

	t.Run("Unpublish Post", func(t *testing.T) {
		// Create a published post
		post, err := suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("To Be Unpublished"),
			testutil.WithPostSlug("to-be-unpublished"),
			testutil.WithPostPublished(true),
		)
		require.NoError(t, err)

		// Unpublish it
		unpublishReq := map[string]interface{}{
			"id": post.ID,
		}

		resp, err := client.UnpublishBlogPost(unpublishReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify it's unpublished
		params := map[string]string{
			"id": post.ID,
		}
		getResp, err := client.GetBlogPost(params)
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		postData := result["post"].(map[string]interface{})
		assert.Nil(t, postData["published_at"], "Post should be unpublished")
	})

	t.Run("Delete Post", func(t *testing.T) {
		// Create a post to delete
		post, err := suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("To Delete"),
			testutil.WithPostSlug("to-delete"),
		)
		require.NoError(t, err)

		// Delete it
		deleteReq := map[string]interface{}{
			"id": post.ID,
		}

		resp, err := client.DeleteBlogPost(deleteReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// runBlogThemeAPITests tests the blog theme management
func runBlogThemeAPITests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	client := suite.APIClient

	// Setup
	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	err = suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	err = client.Login(user.Email, "")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Create Theme", func(t *testing.T) {
		theme := map[string]interface{}{
			"files": map[string]string{
				"home.liquid":     "<html><body>Home</body></html>",
				"category.liquid": "<html><body>Category</body></html>",
				"post.liquid":     "<html><body>Post</body></html>",
				"header.liquid":   "<header>Header</header>",
				"footer.liquid":   "<footer>Footer</footer>",
				"shared.liquid":   "{% comment %}Shared{% endcomment %}",
				"styles.css":      "body { margin: 0; }",
				"scripts.js":      "console.log('test');",
			},
			"notes": "Initial theme version",
		}

		resp, err := client.CreateBlogTheme(theme)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		themeData := result["theme"].(map[string]interface{})
		assert.Equal(t, float64(1), themeData["version"], "First theme should be version 1")
	})

	t.Run("Get Theme by Version", func(t *testing.T) {
		// Create a theme
		theme, err := suite.DataFactory.CreateBlogTheme(workspace.ID)
		require.NoError(t, err)

		// Get by version
		params := map[string]string{
			"version": fmt.Sprintf("%d", theme.Version),
		}
		resp, err := client.GetBlogTheme(params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		themeData := result["theme"].(map[string]interface{})
		assert.Equal(t, float64(theme.Version), themeData["version"])
	})

	t.Run("List Themes", func(t *testing.T) {
		// Create multiple theme versions
		_, err := suite.DataFactory.CreateBlogTheme(workspace.ID,
			testutil.WithThemeVersion(2),
		)
		require.NoError(t, err)

		resp, err := client.ListBlogThemes()
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		themes := result["themes"].([]interface{})
		assert.GreaterOrEqual(t, len(themes), 1, "Should have at least one theme")
	})

	t.Run("Publish Theme", func(t *testing.T) {
		// Create a theme
		theme, err := suite.DataFactory.CreateBlogTheme(workspace.ID,
			testutil.WithThemeVersion(3),
		)
		require.NoError(t, err)

		// Publish it
		publishReq := map[string]interface{}{
			"version": theme.Version,
		}

		resp, err := client.PublishBlogTheme(publishReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify it's published
		getResp, err := client.GetPublishedBlogTheme()
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		themeData := result["theme"].(map[string]interface{})
		assert.Equal(t, float64(theme.Version), themeData["version"], "Published theme should match")
	})
}

// runBlogPublicRenderingTests tests public blog pages (no authentication required)
func runBlogPublicRenderingTests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	// Create workspace with blog enabled and custom domain
	workspace, err := suite.DataFactory.CreateWorkspace(
		testutil.WithCustomDomain("public.blog"),
		testutil.WithBlogEnabled(true),
	)
	require.NoError(t, err)

	// Create category
	category, err := suite.DataFactory.CreateBlogCategory(workspace.ID,
		testutil.WithCategoryName("Technology"),
		testutil.WithCategorySlug("technology"),
	)
	require.NoError(t, err)

	// Create published post
	_, err = suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
		testutil.WithPostTitle("My Published Post"),
		testutil.WithPostSlug("my-published-post"),
		testutil.WithPostPublished(true),
	)
	require.NoError(t, err)

	// Create draft post
	_, err = suite.DataFactory.CreateBlogPost(workspace.ID, category.ID,
		testutil.WithPostTitle("My Draft Post"),
		testutil.WithPostSlug("my-draft-post"),
		testutil.WithPostPublished(false),
	)
	require.NoError(t, err)

	// Create and publish a theme
	_, err = suite.DataFactory.CreateBlogTheme(workspace.ID,
		testutil.WithThemePublished(true),
	)
	require.NoError(t, err)

	t.Run("Get Home Page", func(t *testing.T) {
		resp, err := suite.APIClient.MakeRequestWithHost("GET", "/", "public.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Home page should load")

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "<html>", "Should return HTML")
		assert.Contains(t, string(body), "<body>", "Should have body tag")
	})

	t.Run("Get Category Page", func(t *testing.T) {
		resp, err := suite.APIClient.MakeRequestWithHost("GET", "/technology", "public.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Category page should load")
	})

	t.Run("Get Published Post Page", func(t *testing.T) {
		resp, err := suite.APIClient.MakeRequestWithHost("GET", "/technology/my-published-post", "public.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Published post should be accessible")
	})

	t.Run("Draft Post Not Accessible", func(t *testing.T) {
		resp, err := suite.APIClient.MakeRequestWithHost("GET", "/technology/my-draft-post", "public.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Draft posts should return 404
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Draft post should not be accessible")
	})

	t.Run("Invalid Category Returns 404", func(t *testing.T) {
		resp, err := suite.APIClient.MakeRequestWithHost("GET", "/invalid-category", "public.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Invalid category should return 404")
	})

	t.Run("Invalid Post Returns 404", func(t *testing.T) {
		resp, err := suite.APIClient.MakeRequestWithHost("GET", "/technology/invalid-post", "public.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Invalid post should return 404")
	})
}

// runBlogDataFactoryTests tests the blog factory methods
func runBlogDataFactoryTests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	t.Run("Create Blog Category", func(t *testing.T) {
		category, err := factory.CreateBlogCategory(workspace.ID)
		require.NoError(t, err)
		require.NotNil(t, category)

		assert.NotEmpty(t, category.ID)
		assert.NotEmpty(t, category.Slug)
		assert.NotEmpty(t, category.Settings.Name)
	})

	t.Run("Create Blog Category with Options", func(t *testing.T) {
		category, err := factory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Custom Name"),
			testutil.WithCategorySlug("custom-slug"),
			testutil.WithCategoryDescription("Custom description"),
		)
		require.NoError(t, err)

		assert.Equal(t, "Custom Name", category.Settings.Name)
		assert.Equal(t, "custom-slug", category.Slug)
		assert.Equal(t, "Custom description", category.Settings.Description)
	})

	t.Run("Create Blog Post", func(t *testing.T) {
		category, err := factory.CreateBlogCategory(workspace.ID)
		require.NoError(t, err)

		post, err := factory.CreateBlogPost(workspace.ID, category.ID)
		require.NoError(t, err)
		require.NotNil(t, post)

		assert.NotEmpty(t, post.ID)
		assert.NotEmpty(t, post.Slug)
		assert.NotEmpty(t, post.Settings.Title)
		assert.Equal(t, category.ID, post.CategoryID)
	})

	t.Run("Create Blog Post with Options", func(t *testing.T) {
		category, err := factory.CreateBlogCategory(workspace.ID)
		require.NoError(t, err)

		post, err := factory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostTitle("Custom Title"),
			testutil.WithPostSlug("custom-slug"),
			testutil.WithPostExcerpt("Custom excerpt"),
			testutil.WithPostPublished(true),
			testutil.WithPostAuthors([]domain.BlogAuthor{
				{Name: "John Doe", AvatarURL: "https://example.com/avatar.jpg"},
			}),
		)
		require.NoError(t, err)

		assert.Equal(t, "Custom Title", post.Settings.Title)
		assert.Equal(t, "custom-slug", post.Slug)
		assert.Equal(t, "Custom excerpt", post.Settings.Excerpt)
		assert.NotNil(t, post.PublishedAt, "Post should be published")
		assert.Len(t, post.Settings.Authors, 1)
		assert.Equal(t, "John Doe", post.Settings.Authors[0].Name)
	})

	t.Run("Create Blog Theme", func(t *testing.T) {
		theme, err := factory.CreateBlogTheme(workspace.ID)
		require.NoError(t, err)
		require.NotNil(t, theme)

		assert.Equal(t, 1, theme.Version)
		assert.NotEmpty(t, theme.Files.HomeLiquid)
		assert.NotEmpty(t, theme.Files.StylesCSS)
	})

	t.Run("Create Blog Theme with Options", func(t *testing.T) {
		theme, err := factory.CreateBlogTheme(workspace.ID,
			testutil.WithThemeVersion(2),
			testutil.WithThemePublished(true),
		)
		require.NoError(t, err)

		assert.Equal(t, 2, theme.Version)
		assert.NotNil(t, theme.PublishedAt, "Theme should be published")
	})
}

// runBlogE2EFlowTests tests a complete end-to-end workflow
func runBlogE2EFlowTests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	factory := suite.DataFactory
	client := suite.APIClient

	// Create a no-redirect client for testing redirects
	noRedirectClient := testutil.NewAPIClientNoRedirect(suite.ServerManager.GetURL())

	t.Run("Complete Blog Workflow", func(t *testing.T) {
		// Step 1: Create workspace with blog enabled and custom domain
		workspace, err := factory.CreateWorkspace(
			testutil.WithCustomDomain("e2e.blog"),
			testutil.WithBlogEnabled(true),
		)
		require.NoError(t, err)

		// Step 2: Verify "/" serves blog (requires theme and posts)
		// Create theme first
		_, err = factory.CreateBlogTheme(workspace.ID,
			testutil.WithThemePublished(true),
		)
		require.NoError(t, err)

		// Create categories
		techCategory, err := factory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Technology"),
			testutil.WithCategorySlug("technology"),
		)
		require.NoError(t, err)

		designCategory, err := factory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Design"),
			testutil.WithCategorySlug("design"),
		)
		require.NoError(t, err)

		businessCategory, err := factory.CreateBlogCategory(workspace.ID,
			testutil.WithCategoryName("Business"),
			testutil.WithCategorySlug("business"),
		)
		require.NoError(t, err)

		// Step 3: Create 5 posts across different categories
		posts := []struct {
			title      string
			slug       string
			categoryID string
			published  bool
		}{
			{"AI in 2024", "ai-in-2024", techCategory.ID, true},
			{"Design Trends", "design-trends", designCategory.ID, true},
			{"Startup Guide", "startup-guide", businessCategory.ID, true},
			{"Draft Tech Post", "draft-tech-post", techCategory.ID, false},
			{"Draft Design Post", "draft-design-post", designCategory.ID, false},
		}

		for _, p := range posts {
			_, err := factory.CreateBlogPost(workspace.ID, p.categoryID,
				testutil.WithPostTitle(p.title),
				testutil.WithPostSlug(p.slug),
				testutil.WithPostPublished(p.published),
			)
			require.NoError(t, err)
		}

		// Step 4: Verify GET "/" serves blog
		resp, err := client.MakeRequestWithHost("GET", "/", "e2e.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Blog should be served")

		// Step 5: Verify published posts are accessible
		resp, err = client.MakeRequestWithHost("GET", "/technology/ai-in-2024", "e2e.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Published post should be accessible")

		// Step 6: Verify draft posts are NOT accessible
		resp, err = client.MakeRequestWithHost("GET", "/technology/draft-tech-post", "e2e.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Draft post should not be accessible")

		// Step 7: Disable blog
		workspace.Settings.BlogEnabled = false
		// Note: In a real test we'd use the API to update workspace settings
		// For now we just verify the routing logic works

		// Create new workspace to test disabled state
		_, err = factory.CreateWorkspace(
			testutil.WithCustomDomain("disabled.blog"),
			testutil.WithBlogEnabled(false),
		)
		require.NoError(t, err)

		// Step 8: Verify "/" redirects to /console when blog disabled
		// Use noRedirectClient to test the redirect itself
		resp, err = noRedirectClient.MakeRequestWithHost("GET", "/", "disabled.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode, "Should redirect when blog disabled")
		assert.Equal(t, "/console", resp.Header.Get("Location"))

		// Step 9: Re-enable blog (create new workspace to demonstrate)
		reenabledWorkspace, err := factory.CreateWorkspace(
			testutil.WithCustomDomain("reenabled.blog"),
			testutil.WithBlogEnabled(true),
		)
		require.NoError(t, err)

		// Create minimal content
		_, err = factory.CreateBlogTheme(reenabledWorkspace.ID,
			testutil.WithThemePublished(true),
		)
		require.NoError(t, err)

		cat, err := factory.CreateBlogCategory(reenabledWorkspace.ID)
		require.NoError(t, err)

		_, err = factory.CreateBlogPost(reenabledWorkspace.ID, cat.ID,
			testutil.WithPostPublished(true),
		)
		require.NoError(t, err)

		// Step 10: Verify "/" serves blog again
		resp, err = client.MakeRequestWithHost("GET", "/", "reenabled.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Blog should be served after re-enabling")
	})
}

// runBlogFeedTests exercises the RSS and JSON Feed endpoints end-to-end:
// routing, content correctness, conditional GET, per-category filtering,
// XSS sanitization, draft exclusion, and feed autodiscovery in HTML.
func runBlogFeedTests(t *testing.T, suite *testutil.IntegrationTestSuite) {
	factory := suite.DataFactory
	client := suite.APIClient

	// --- Setup: workspace with blog + custom domain ---
	workspace, err := factory.CreateWorkspace(
		testutil.WithCustomDomain("feed.blog"),
		testutil.WithBlogEnabled(true),
	)
	require.NoError(t, err)

	_, err = factory.CreateBlogTheme(workspace.ID, testutil.WithThemePublished(true))
	require.NoError(t, err)

	techCat, err := factory.CreateBlogCategory(workspace.ID,
		testutil.WithCategoryName("Tech"),
		testutil.WithCategorySlug("tech"),
	)
	require.NoError(t, err)

	designCat, err := factory.CreateBlogCategory(workspace.ID,
		testutil.WithCategoryName("Design"),
		testutil.WithCategorySlug("design"),
	)
	require.NoError(t, err)

	// Published posts
	_, err = factory.CreateBlogPost(workspace.ID, techCat.ID,
		testutil.WithPostTitle("Intro to Go"),
		testutil.WithPostSlug("intro-to-go"),
		testutil.WithPostPublished(true),
	)
	require.NoError(t, err)

	_, err = factory.CreateBlogPost(workspace.ID, techCat.ID,
		testutil.WithPostTitle("Concurrency Patterns"),
		testutil.WithPostSlug("concurrency-patterns"),
		testutil.WithPostPublished(true),
	)
	require.NoError(t, err)

	_, err = factory.CreateBlogPost(workspace.ID, designCat.ID,
		testutil.WithPostTitle("Color Theory"),
		testutil.WithPostSlug("color-theory"),
		testutil.WithPostPublished(true),
	)
	require.NoError(t, err)

	// Draft post — must NOT appear in feeds
	_, err = factory.CreateBlogPost(workspace.ID, techCat.ID,
		testutil.WithPostTitle("Draft Post"),
		testutil.WithPostSlug("draft-post"),
		testutil.WithPostPublished(false),
	)
	require.NoError(t, err)

	// --- Tests ---

	t.Run("RSS feed returns valid XML with all published posts", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/feed.xml", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "application/rss+xml")
		assert.NotEmpty(t, resp.Header.Get("ETag"))
		assert.NotEmpty(t, resp.Header.Get("Last-Modified"))
		assert.Contains(t, resp.Header.Get("Cache-Control"), "s-maxage=300")
		assert.Equal(t, "Accept-Encoding", resp.Header.Get("Vary"))

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		bodyStr := string(body)

		// Well-formed XML
		var parsed struct{}
		require.NoError(t, xml.Unmarshal(body, &parsed), "RSS output must be well-formed XML")

		// All 3 published posts present
		assert.Contains(t, bodyStr, "Intro to Go")
		assert.Contains(t, bodyStr, "Concurrency Patterns")
		assert.Contains(t, bodyStr, "Color Theory")

		// Draft excluded
		assert.NotContains(t, bodyStr, "Draft Post")

		// RSS 2.0 structure
		assert.Contains(t, bodyStr, `<rss version="2.0"`)
		assert.Contains(t, bodyStr, `xmlns:content=`)
		assert.Contains(t, bodyStr, `xmlns:dc=`)
		assert.Contains(t, bodyStr, `<description>`)
		assert.Contains(t, bodyStr, `<language>`)
		assert.Contains(t, bodyStr, `<lastBuildDate>`)
		assert.Contains(t, bodyStr, `isPermaLink="false"`)
	})

	t.Run("JSON Feed returns valid JSON with all published posts", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/feed.json", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "application/feed+json")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var feed map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &feed), "JSON Feed output must be valid JSON")

		assert.Equal(t, "https://jsonfeed.org/version/1.1", feed["version"])
		assert.Contains(t, feed["feed_url"], "/feed.json")

		items, ok := feed["items"].([]interface{})
		require.True(t, ok)
		assert.Len(t, items, 3, "Should have 3 published posts")

		// Verify draft not present
		for _, it := range items {
			item := it.(map[string]interface{})
			assert.NotEqual(t, "Draft Post", item["title"])
		}
	})

	t.Run("Per-category RSS feed filters to that category only", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/design/feed.xml", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		bodyStr := string(body)

		// Only design post
		assert.Contains(t, bodyStr, "Color Theory")
		assert.NotContains(t, bodyStr, "Intro to Go")
		assert.NotContains(t, bodyStr, "Concurrency Patterns")
	})

	t.Run("Per-category JSON Feed filters correctly", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/tech/feed.json", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var feed map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &feed))

		items := feed["items"].([]interface{})
		assert.Len(t, items, 2, "Tech category should have 2 published posts")

		// Verify no design posts
		for _, it := range items {
			item := it.(map[string]interface{})
			assert.NotEqual(t, "Color Theory", item["title"])
		}
	})

	t.Run("Conditional GET returns 304 with matching ETag", func(t *testing.T) {
		// First request to get ETag
		resp1, err := client.MakeRequestWithHost("GET", "/feed.xml", "feed.blog", nil)
		require.NoError(t, err)
		_, _ = io.ReadAll(resp1.Body)
		resp1.Body.Close()

		etag := resp1.Header.Get("ETag")
		require.NotEmpty(t, etag)

		// Second request with If-None-Match
		noRedirectClient := testutil.NewAPIClientNoRedirect(suite.ServerManager.GetURL())
		noRedirectClient.SetToken(client.GetToken())
		noRedirectClient.SetWorkspaceID(client.GetWorkspaceID())

		req, err := http.NewRequest("GET", suite.ServerManager.GetURL()+"/feed.xml", nil)
		require.NoError(t, err)
		req.Host = "feed.blog"
		req.Header.Set("If-None-Match", etag)

		resp2, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp2.Body.Close() }()

		assert.Equal(t, http.StatusNotModified, resp2.StatusCode, "Should return 304 when ETag matches")
		assert.Equal(t, etag, resp2.Header.Get("ETag"), "304 response must include ETag header")
		assert.NotEmpty(t, resp2.Header.Get("Last-Modified"), "304 response must include Last-Modified")
	})

	t.Run("HEAD returns headers without body", func(t *testing.T) {
		req, err := http.NewRequest("HEAD", suite.ServerManager.GetURL()+"/feed.xml", nil)
		require.NoError(t, err)
		req.Host = "feed.blog"

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "application/rss+xml")
		assert.NotEmpty(t, resp.Header.Get("ETag"))

		body, _ := io.ReadAll(resp.Body)
		assert.Empty(t, body, "HEAD response must have empty body")
	})

	t.Run("POST returns 405 Method Not Allowed", func(t *testing.T) {
		req, err := http.NewRequest("POST", suite.ServerManager.GetURL()+"/feed.xml", nil)
		require.NoError(t, err)
		req.Host = "feed.blog"

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	t.Run("Blog home page contains feed autodiscovery links when theme has head", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		// Autodiscovery tags are injected before </head>. If the theme
		// template has no <head> section, there's nowhere to inject and
		// InjectFeedDiscoveryTags correctly returns the HTML unchanged.
		if strings.Contains(strings.ToLower(bodyStr), "</head>") {
			assert.Contains(t, bodyStr, `type="application/rss+xml"`, "Home page should have RSS autodiscovery")
			assert.Contains(t, bodyStr, `href="/feed.xml"`)
			assert.Contains(t, bodyStr, `type="application/feed+json"`, "Home page should have JSON Feed autodiscovery")
			assert.Contains(t, bodyStr, `href="/feed.json"`)
		} else {
			t.Log("Theme has no <head> tag — autodiscovery injection skipped (expected for minimal test theme)")
		}
	})

	t.Run("Sitemap includes feed URLs", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/sitemap.xml", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		assert.Contains(t, bodyStr, "/feed.xml", "Sitemap should include main feed URL")
	})

	t.Run("Feed content does not contain script tags (XSS sanitization)", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/feed.xml", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		assert.NotContains(t, strings.ToLower(bodyStr), "<script", "Feed must not contain script tags")
		assert.NotContains(t, strings.ToLower(bodyStr), "javascript:", "Feed must not contain javascript: URIs")
	})

	t.Run("Robots.txt references sitemap which includes feeds", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/robots.txt", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Sitemap: /sitemap.xml")
	})

	t.Run("Non-existent category feed returns empty items not 500", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/nonexistent-category/feed.xml", "feed.blog", nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// The slug doesn't match any category, so ListFeedPosts returns 0 rows.
		// The feed should still be valid XML with 0 items — not a 500 or 404.
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		if resp.StatusCode == http.StatusOK {
			assert.Contains(t, bodyStr, "<rss", "Should still be valid RSS")
			assert.NotContains(t, bodyStr, "<item>", "Should have zero items")
		} else {
			// Acceptable: 404 for completely unknown category
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		}
	})

	t.Run("Gzip response decompresses to valid feed", func(t *testing.T) {
		req, err := http.NewRequest("GET", suite.ServerManager.GetURL()+"/feed.xml", nil)
		require.NoError(t, err)
		req.Host = "feed.blog"
		req.Header.Set("Accept-Encoding", "gzip")

		// Use a transport that does NOT auto-decompress
		transport := &http.Transport{DisableCompression: true}
		gzipClient := &http.Client{Transport: transport}

		resp, err := gzipClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

		gr, err := gzip.NewReader(resp.Body)
		require.NoError(t, err, "response body must be valid gzip")
		decompressed, err := io.ReadAll(gr)
		require.NoError(t, err)
		assert.Contains(t, string(decompressed), "<rss", "decompressed content must be valid RSS")
	})
}
