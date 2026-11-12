package integration

import (
	"encoding/xml"
	"fmt"
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

// TestBlogSitemap checks that the public sitemap lists every live post, past the page limit of
// the API, with the feed of each category, and nothing a visitor could not open.
func TestBlogSitemap(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient
	const host = "sitemap.blog"

	workspace, err := factory.CreateWorkspace(
		testutil.WithCustomDomain(host),
		testutil.WithBlogEnabled(true),
	)
	require.NoError(t, err)

	publishedAt := func(at time.Time) testutil.BlogPostOption {
		return func(p *domain.BlogPost) {
			utc := at.UTC()
			p.PublishedAt = &utc
		}
	}

	tech, err := factory.CreateBlogCategory(workspace.ID, testutil.WithCategorySlug("tech"))
	require.NoError(t, err)
	design, err := factory.CreateBlogCategory(workspace.ID, testutil.WithCategorySlug("design"))
	require.NoError(t, err)
	archived, err := factory.CreateBlogCategory(workspace.ID, testutil.WithCategorySlug("archived"))
	require.NoError(t, err)

	// More live posts than a page of the API can hold, across two categories.
	const techPosts, designPosts = 70, 40
	var live []string
	for i := 0; i < techPosts+designPosts; i++ {
		category, slug := tech, fmt.Sprintf("tech-post-%d", i)
		if i >= techPosts {
			category, slug = design, fmt.Sprintf("design-post-%d", i)
		}
		_, err := factory.CreateBlogPost(workspace.ID, category.ID,
			testutil.WithPostSlug(slug),
			publishedAt(time.Now().Add(-time.Duration(i+1)*time.Hour)),
		)
		require.NoError(t, err)
		live = append(live, "/"+category.Slug+"/"+slug)
	}
	require.Greater(t, len(live), 100, "more live posts than the API's page limit")

	_, err = factory.CreateBlogPost(workspace.ID, tech.ID,
		testutil.WithPostSlug("scheduled-post"),
		publishedAt(time.Now().Add(24*time.Hour)),
	)
	require.NoError(t, err)
	_, err = factory.CreateBlogPost(workspace.ID, tech.ID, testutil.WithPostSlug("draft-post"))
	require.NoError(t, err)
	deleted, err := factory.CreateBlogPost(workspace.ID, tech.ID,
		testutil.WithPostSlug("deleted-post"),
		publishedAt(time.Now().Add(-time.Hour)),
	)
	require.NoError(t, err)
	_, err = factory.CreateBlogPost(workspace.ID, archived.ID,
		testutil.WithPostSlug("archived-post"),
		publishedAt(time.Now().Add(-time.Hour)),
	)
	require.NoError(t, err)

	// A deleted post, and a live post left in a deleted category: both answer 404.
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE blog_posts SET deleted_at = NOW() WHERE id = $1`, deleted.ID)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE blog_categories SET deleted_at = NOW() WHERE id = $1`, archived.ID)
	require.NoError(t, err)

	resp, err := client.MakeRequestWithHost("GET", "/sitemap.xml", host, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/xml", resp.Header.Get("Content-Type"))

	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	require.NoError(t, xml.Unmarshal(body, &sitemap), "the sitemap is well-formed XML")
	locs := map[string]int{}
	for _, u := range sitemap.URLs {
		locs[u.Loc]++
	}
	url := func(path string) string { return "https://" + host + path }

	t.Run("lists every live post once", func(t *testing.T) {
		for _, path := range live {
			assert.Equal(t, 1, locs[url(path)], path)
		}
	})

	t.Run("lists no post that is not live", func(t *testing.T) {
		for _, path := range []string{
			"/tech/scheduled-post", "/tech/draft-post", "/tech/deleted-post", "/archived/archived-post",
		} {
			assert.Zero(t, locs[url(path)], path)
		}
	})

	t.Run("lists the home page, the main feed and the feed of each live category once", func(t *testing.T) {
		for _, path := range []string{"/", "/feed.xml", "/tech/feed.xml", "/design/feed.xml"} {
			assert.Equal(t, 1, locs[url(path)], path)
		}
		assert.Zero(t, locs[url("/archived/feed.xml")])
	})

	t.Run("lists nothing else", func(t *testing.T) {
		// Home page, main feed, the live posts and the two live categories' feeds.
		assert.Len(t, sitemap.URLs, 2+len(live)+2)
	})
}
