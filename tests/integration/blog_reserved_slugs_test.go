//go:build integration
// +build integration

package integration

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlogReservedCategorySlugs ties domain.BlogCategorySlugRouteOf to the routes the running
// server registers: for a category per slug, each of its addresses reaches the blog exactly
// when the domain says so, and the sitemap lists exactly the addresses that do.
func TestBlogReservedCategorySlugs(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient
	const host = "reserved.blog"

	workspace, err := factory.CreateWorkspace(testutil.WithCustomDomain(host), testutil.WithBlogEnabled(true))
	require.NoError(t, err)
	page := "<html><body>{{ blog.title }}</body></html>"
	_, err = factory.CreateBlogTheme(workspace.ID,
		testutil.WithThemePublished(true),
		testutil.WithThemeFiles(domain.BlogThemeFiles{HomeLiquid: page, CategoryLiquid: page, PostLiquid: page}),
	)
	require.NoError(t, err)

	// The slugs are written out, as in the domain test: this pins what the server does for
	// each of them, not that the domain knows every route the server registers.
	slugs := []string{
		"v", "t", "r", "api", "console", "notification-center", "apis",
		"visit", "opens", "track", "preferences", "subscribe", "unsubscribe", "unsubscribe-oneclick", "health", "healthz",
		"webhooks", "tracking",
	}
	publishedAt := time.Now().Add(-time.Hour)
	categoryIDs := map[string]string{}
	for _, slug := range slugs {
		// Created below the API, as a category made before its slug was refused would be.
		category, err := factory.CreateBlogCategory(workspace.ID, testutil.WithCategorySlug(slug))
		require.NoError(t, err)
		categoryIDs[slug] = category.ID
		_, err = factory.CreateBlogPost(workspace.ID, category.ID, testutil.WithPostSlug("post-in-"+slug),
			testutil.WithPostPublishedAt(publishedAt))
		require.NoError(t, err)
	}

	// The blog's pages carry X-Cache and its feeds are RSS; no other route sends either.
	reachesBlog := func(t *testing.T, path string) bool {
		t.Helper()
		resp, err := client.MakeRequestWithHost("GET", path, host, nil)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK &&
			(resp.Header.Get("X-Cache") != "" || strings.HasPrefix(resp.Header.Get("Content-Type"), "application/rss+xml"))
	}

	resp, err := client.MakeRequestWithHost("GET", "/sitemap.xml", host, nil)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	require.NoError(t, xml.Unmarshal(body, &sitemap))
	listed := map[string]bool{}
	for _, u := range sitemap.URLs {
		listed[strings.TrimPrefix(u.Loc, "https://"+host)] = true
	}

	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			route := domain.BlogCategorySlugRouteOf(slug)
			pagePath, postPath, feedPath := "/"+slug, "/"+slug+"/post-in-"+slug, "/"+slug+"/feed.xml"

			assert.Equal(t, route == domain.BlogCategorySlugFree, reachesBlog(t, pagePath), "the category page")
			assert.Equal(t, route != domain.BlogCategorySlugSubtreeTaken, reachesBlog(t, postPath), "the post")
			assert.Equal(t, route != domain.BlogCategorySlugSubtreeTaken, reachesBlog(t, feedPath), "the category feed")

			assert.Equal(t, route == domain.BlogCategorySlugFree, listed[pagePath], "the sitemap lists the page")
			assert.Equal(t, route != domain.BlogCategorySlugSubtreeTaken, listed[postPath], "the sitemap lists the post")
			assert.Equal(t, route != domain.BlogCategorySlugSubtreeTaken, listed[feedPath], "the sitemap lists the feed")
		})
	}

	t.Run("a new category may not take a slug another route answers", func(t *testing.T) {
		owner, err := factory.CreateUser()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
		require.NoError(t, client.Login(owner.Email, "password"))
		client.SetWorkspaceID(workspace.ID)

		for slug, wantStatus := range map[string]int{"health": http.StatusBadRequest, "v": http.StatusBadRequest, "webhooks-news": http.StatusCreated} {
			resp, err := client.Post("/api/blogCategories.create", map[string]interface{}{
				"workspace_id": workspace.ID, "name": "Name " + slug, "slug": slug,
			})
			require.NoError(t, err)
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			assert.Equal(t, wantStatus, resp.StatusCode, "%s: %s", slug, raw)
			if wantStatus == http.StatusBadRequest {
				assert.Contains(t, string(raw), "slug is reserved by another route", slug)
			}
		}

		// A rename may not take one either, while a category that already has one keeps it.
		update := func(id, name, slug string) (int, string) {
			resp, err := client.Post("/api/blogCategories.update", map[string]interface{}{
				"workspace_id": workspace.ID, "id": id, "name": name, "slug": slug,
			})
			require.NoError(t, err)
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return resp.StatusCode, string(raw)
		}
		renamed, err := factory.CreateBlogCategory(workspace.ID, testutil.WithCategorySlug("renamed-later"))
		require.NoError(t, err)
		status, raw := update(renamed.ID, "Renamed", "healthz")
		assert.Equal(t, http.StatusBadRequest, status, raw)
		assert.Contains(t, raw, "slug is reserved by another route")

		status, raw = update(categoryIDs["opens"], "Opens, renamed", "opens")
		assert.Equal(t, http.StatusOK, status, raw)
	})
}
