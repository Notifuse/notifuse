package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlogScheduledPost checks that a post published with a future date stays
// off the public blog until that date, while the console still lists it as
// published.
func TestBlogScheduledPost(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient
	const host = "scheduled.blog"

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(
		testutil.WithCustomDomain(host),
		testutil.WithBlogEnabled(true),
	)
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, ""))
	client.SetWorkspaceID(workspace.ID)

	listTitles := "<html><body>{% for post in posts %}[{{ post.title }}]{% endfor %}</body></html>"
	_, err = factory.CreateBlogTheme(workspace.ID,
		testutil.WithThemePublished(true),
		testutil.WithThemeFiles(domain.BlogThemeFiles{
			HomeLiquid:     listTitles,
			CategoryLiquid: listTitles,
			PostLiquid:     "<html><body>{{ post.title }}</body></html>",
		}),
	)
	require.NoError(t, err)

	category, err := factory.CreateBlogCategory(workspace.ID,
		testutil.WithCategoryName("News"),
		testutil.WithCategorySlug("news"),
	)
	require.NoError(t, err)

	live, err := factory.CreateBlogPost(workspace.ID, category.ID,
		testutil.WithPostTitle("Live Post"),
		testutil.WithPostSlug("live-post"),
		testutil.WithPostPublishedAt(time.Now().Add(-time.Hour)),
	)
	require.NoError(t, err)
	scheduled, err := factory.CreateBlogPost(workspace.ID, category.ID,
		testutil.WithPostTitle("Scheduled Post"),
		testutil.WithPostSlug("scheduled-post"),
		testutil.WithPostPublishedAt(time.Now().Add(24*time.Hour)),
	)
	require.NoError(t, err)

	get := func(t *testing.T, path string) (int, string) {
		t.Helper()
		resp, err := client.MakeRequestWithHost("GET", path, host, nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(body)
	}

	for _, path := range []string{"/", "/news"} {
		t.Run("page "+path+" lists only the live post", func(t *testing.T) {
			status, body := get(t, path)
			require.Equal(t, http.StatusOK, status)
			assert.Contains(t, body, "[Live Post]")
			assert.NotContains(t, body, "Scheduled Post")
		})
	}

	t.Run("live post page is served", func(t *testing.T) {
		status, body := get(t, "/news/live-post")
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, body, "Live Post")
	})

	t.Run("scheduled post page answers 404", func(t *testing.T) {
		status, body := get(t, "/news/scheduled-post")
		assert.Equal(t, http.StatusNotFound, status)
		assert.NotContains(t, body, "Scheduled Post")
	})

	for _, path := range []string{"/feed.xml", "/feed.json"} {
		t.Run("feed "+path+" lists only the live post", func(t *testing.T) {
			status, body := get(t, path)
			require.Equal(t, http.StatusOK, status)
			assert.Contains(t, body, "Live Post")
			assert.NotContains(t, body, "Scheduled Post")
		})
	}

	t.Run("console still lists both as published", func(t *testing.T) {
		resp, err := client.ListBlogPosts(map[string]string{"status": "published", "limit": "10"})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result struct {
			Posts []struct {
				ID string `json:"id"`
			} `json:"posts"`
			TotalCount int `json:"total_count"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		ids := []string{}
		for _, p := range result.Posts {
			ids = append(ids, p.ID)
		}
		assert.ElementsMatch(t, []string{live.ID, scheduled.ID}, ids)
		assert.Equal(t, 2, result.TotalCount)
	})
}
