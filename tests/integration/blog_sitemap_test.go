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
	// A published theme, so that every listed page renders.
	page := "<html><body>{{ blog.title }}</body></html>"
	_, err = factory.CreateBlogTheme(workspace.ID,
		testutil.WithThemePublished(true),
		testutil.WithThemeFiles(domain.BlogThemeFiles{HomeLiquid: page, CategoryLiquid: page, PostLiquid: page}),
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
	// A category created before its slug was refused: another route answers its page, while
	// its posts and feed reach the blog (TestBlogReservedCategorySlugs covers the others).
	track, err := factory.CreateBlogCategory(workspace.ID, testutil.WithCategorySlug("track"))
	require.NoError(t, err)
	_, err = factory.CreateBlogPost(workspace.ID, track.ID,
		testutil.WithPostSlug("tracked-post"),
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
	// The oldest tech post, published 70 hours ago, edited 30 hours ago: two different days.
	edited := time.Now().Add(-30 * time.Hour).UTC()
	_, err = db.Exec(`UPDATE blog_posts SET updated_at = $1 WHERE slug = 'tech-post-69'`, edited)
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
			Loc     string `xml:"loc"`
			LastMod string `xml:"lastmod"`
		} `xml:"url"`
	}
	require.NoError(t, xml.Unmarshal(body, &sitemap), "the sitemap is well-formed XML")
	locs := map[string]int{}
	lastmods := map[string]string{}
	for _, u := range sitemap.URLs {
		locs[u.Loc]++
		lastmods[u.Loc] = u.LastMod
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

	t.Run("lists the home page, the main feed and the page and feed of each live category once", func(t *testing.T) {
		for _, path := range []string{"/", "/feed.xml", "/tech", "/tech/feed.xml", "/design", "/design/feed.xml"} {
			assert.Equal(t, 1, locs[url(path)], path)
		}
		assert.Zero(t, locs[url("/archived")])
		assert.Zero(t, locs[url("/archived/feed.xml")])
	})

	t.Run("leaves out the page another route answers, and lists the category's post and feed", func(t *testing.T) {
		assert.Zero(t, locs[url("/track")])
		assert.Equal(t, 1, locs[url("/track/feed.xml")])
		assert.Equal(t, 1, locs[url("/track/tracked-post")])
	})

	t.Run("lists nothing else", func(t *testing.T) {
		// Home page, main feed, the live posts, the two live categories' pages and feeds, and
		// the track category's post and feed.
		assert.Len(t, sitemap.URLs, 2+len(live)+4+2)
	})

	t.Run("an edited post's lastmod is its edit date", func(t *testing.T) {
		assert.Equal(t, edited.Format("2006-01-02"), lastmods[url("/tech/tech-post-69")])
	})

	t.Run("every listed URL answers 200", func(t *testing.T) {
		for _, u := range sitemap.URLs {
			path := u.Loc[len("https://"+host):]
			resp, err := client.MakeRequestWithHost("GET", path, host, nil)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode, path)
		}
	})

	t.Run("a second request within the cache's TTL is served from the cache", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/sitemap.xml", host, nil)
		require.NoError(t, err)
		again, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, "HIT", resp.Header.Get("X-Cache"))
		assert.Equal(t, string(body), string(again))
	})

	t.Run("robots.txt names the sitemap by its absolute URL", func(t *testing.T) {
		resp, err := client.MakeRequestWithHost("GET", "/robots.txt", host, nil)
		require.NoError(t, err)
		robots, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(robots), "Sitemap: https://"+host+"/sitemap.xml\n")
	})

	t.Run("robots.txt and the sitemap answer 405 to a POST", func(t *testing.T) {
		for _, path := range []string{"/robots.txt", "/sitemap.xml"} {
			resp, err := client.MakeRequestWithHost("POST", path, host, nil)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, path)
		}
	})
}
