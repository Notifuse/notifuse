# Fix the blog sitemap

## Problem

`GET /sitemap.xml` on a blog lists the home page and the main feed, but none of the posts and
none of the category feeds. This is released behaviour (the blog shipped in 17.0, the category
feeds in the sitemap in 29.4), so the CHANGELOG gets a `Fix` line.

`RootHandler.serveBlogSitemap` (`internal/http/root_handler.go`) lists the posts with
`BlogService.ListPublicPosts`, then resolves each post's category with
`BlogService.GetCategory`. `GetCategory` authenticates the caller; a crawler has no session, so
every call fails, and the handler silently skips every post URL and every category feed URL.

Even with the categories resolved, two further defects remain:

- `ListPublicPosts` goes through `ListBlogPostsRequest.Validate`, which caps `Limit` at 100: the
  handler asks for 1,000 and gets the 100 newest posts.
- `ListPublicPosts` lists every post with a `published_at`, scheduled posts included. PR #525
  makes the public pages answer 404 for them until their date, but that change is not on `dev`,
  and this PR must not depend on it. Without a filter here, the sitemap would hand crawlers a
  post that is not out yet (on `dev` its page even answers 200; with #525, 404).

## Goal

The sitemap lists, for the blog's workspace:

- the home page and the main feed, as today;
- every live post (published, `published_at` not in the future, not deleted) whose category
  exists and is not deleted, at `https://<host>/<category-slug>/<post-slug>`, with
  `lastmod` = the publication date, as today;
- the feed of each category that has at least one listed post, once, as today.

It stays within the sitemap protocol's limit of 50,000 URLs: past it, the oldest posts are left
out (the sitemap index format is out of scope).

No table, index, migration or environment variable change.

## Design

### Repository: one query for the sitemap

`domain.BlogPostRepository` gains:

```go
// ListSitemapPosts returns, newest first, at most limit live posts with the slug of their
// category: published at or before now, not deleted, in a category that is not deleted.
ListSitemapPosts(ctx context.Context, now time.Time, limit int) ([]*BlogSitemapPost, error)
```

with, in `internal/domain/blog.go`:

```go
// BlogSitemapPost is what the blog sitemap needs of a live post.
type BlogSitemapPost struct {
	CategorySlug string
	PostSlug     string
	PublishedAt  time.Time
}
```

`blogPostRepository.ListSitemapPosts` (`internal/repository/blog_postgres.go`):

```sql
SELECT c.slug, p.slug, p.published_at
FROM blog_posts p
JOIN blog_categories c ON c.id = p.category_id AND c.deleted_at IS NULL
WHERE p.deleted_at IS NULL
  AND p.published_at IS NOT NULL
  AND p.published_at <= $1
ORDER BY p.published_at DESC, p.id DESC
LIMIT $2
```

- `$1` is `now.UTC()`: `published_at` is a `TIMESTAMP` without time zone holding UTC, so it is
  compared to a UTC parameter rather than to `NOW()`, whose conversion depends on the session
  time zone (same choice as PR #525 for the public pages).
- The `JOIN` drops posts with an empty `category_id`, an unknown one, or a deleted category,
  whose URL would answer 404 (`GetPostByCategoryAndSlug` requires `c.deleted_at IS NULL`).
- One query, no `COUNT(*)`, three small columns: no `settings` JSON is read.
- `limit <= 0` is refused with an error rather than sending `LIMIT 0` or a negative limit.

### Service: public, no authentication

`domain.BlogService` gains:

```go
// ListPublicSitemapPosts returns the live posts the blog sitemap lists, newest first, at most
// BlogSitemapMaxPosts of them (no authentication: the sitemap is public).
ListPublicSitemapPosts(ctx context.Context) ([]*BlogSitemapPost, error)
```

`BlogService.ListPublicSitemapPosts` calls `postRepo.ListSitemapPosts(ctx, time.Now().UTC(),
domain.BlogSitemapMaxPosts)` and wraps the error. It calls no auth method, like the other
`*Public*` methods.

`domain.BlogSitemapMaxURLs = 50000` (the protocol limit) and
`domain.BlogSitemapMaxPosts = BlogSitemapMaxURLs - 2` (home page and main feed): the query never
needs to return more, since each post costs at least one URL.

### Handler

`serveBlogSitemap` calls `ListPublicSitemapPosts` (500 on error, as today) and writes the XML
through a pure function, with the host stripped of its port (`hostWithoutPort`, shared with
`RootHandler.Handle`'s workspace lookup). The workspace is found by the part before the colon only; the
rest of the `Host` header is unchecked, and used raw it would be repeated, unescaped, in every
`<loc>`: `Host: blog.example:&x` would make the XML malformed, and a long header would multiply
the response size by the number of posts.

```go
func writeBlogSitemap(w io.Writer, host string, posts []*domain.BlogSitemapPost, maxURLs int)
```

- Home page first, then posts newest first, then the main feed, then the category feeds in the
  order their first post appears: the same order and the same `changefreq` / `priority` values
  as today.
- Budget: `maxURLs - 2` URLs for posts and category feeds. A post costs 1, plus 1 if its
  category has no feed URL yet. The loop stops at the first post that does not fit, so the
  sitemap keeps the newest posts and never lists a category feed without a post of that
  category. `serveBlogSitemap` passes `domain.BlogSitemapMaxURLs`.
- `GetCategory` and `ListPublicPosts` are no longer called by the sitemap.

`ListPublicPosts` stays in the interface: nothing else calls it today, but removing a public
service method is out of scope.

## Files and tests (TDD: each test written and seen failing first)

| Implementation file | Test file | Test cases |
|---|---|---|
| `internal/domain/blog.go` (type, constants, two interface methods) | `internal/domain/blog_test.go` | `BlogSitemapMaxPosts` leaves room for the home page and the main feed within `BlogSitemapMaxURLs` (`BlogSitemapMaxURLs == 50000`, `BlogSitemapMaxPosts == BlogSitemapMaxURLs - 2`) |
| `internal/domain/mocks/mock_blog_post_repository.go`, `mock_blog_service.go` | (generated with `mockgen`, via the `go:generate` lines) | none |
| `internal/repository/blog_postgres.go` (`ListSitemapPosts`) | `internal/repository/blog_postgres_test.go` | query joins non-deleted categories, filters deleted, unpublished and future posts, orders by `published_at DESC, id DESC`, passes `now` in UTC and the limit (sqlmock, `QueryMatcherRegexp`); rows are mapped to `BlogSitemapPost`; query error, scan error and `rows.Err` are returned; missing workspace in context and connection error are returned; `limit <= 0` is refused without a query |
| `internal/service/blog_service.go` (`ListPublicSitemapPosts`) | `internal/service/blog_service_test.go` | calls the repository with a UTC `now` within the call's window and `BlogSitemapMaxPosts`, returns its posts; calls no auth method (the auth mock has no expectation); wraps a repository error |
| `internal/http/root_handler.go` (`serveBlogSitemap`, `writeBlogSitemap`, `hostWithoutPort`) | `internal/http/root_handler_test.go` | rewrite of `TestRootHandler_serveBlogSitemap`: posts of two categories give post URLs with their category slug, `lastmod`, and one feed URL per category, in order; `GetCategory` and `ListPublicPosts` are never called (strict gomock); no post: home page and main feed only; service error: 500; a `Host` of `example.com:&x` gives `https://example.com/...` URLs and XML that `encoding/xml` decodes; `writeBlogSitemap` with a small `maxURLs`: keeps the newest posts, stops at the first post that does not fit, counts a new category's feed against the budget, never lists a feed without its post; `maxURLs` of exactly 2 lists the home page and main feed only; the "routing to sitemap.xml" case of `serveBlog` expects the new method |
| `tests/integration/blog_sitemap_test.go` (new) | itself | `TestBlogSitemap`, red on `dev`: a workspace with a custom domain and the blog on, two categories with more live posts between them than the API's page limit (`require.Greater(len(live), 100)` pins it), plus a scheduled post, a draft, a deleted post, and a post whose category is soft-deleted through SQL (`factory.GetWorkspaceDB`); the sitemap lists every live post at its URL, none of the four others, one feed URL per live category and none for the deleted one, and its `<loc>` count is exactly home + main feed + live posts + live categories; the response is well-formed XML (`encoding/xml` decode) |
| `CHANGELOG.md` | none | one line under `[44.0] - unreleased`: `- **Fix**: The blog's sitemap now lists the blog's posts and the feeds of its categories.` |

The integration test sets `published_at` (converted to UTC, since `CreatePostTx` stores it as
given) with a local `testutil.BlogPostOption` closure, so this PR does not add
`WithPostPublishedAt` to `tests/testutil/factory.go`, which PR #525 adds. The new tests are
placed next to the related ones rather than at the end of the files, where #525 also appends.
The CHANGELOG line will still conflict with #525's (both end the `[44.0]` section): whichever
merges second needs a one-line resolution.

## Known, not treated

- `lastmod` is the publication date, not the last update: unchanged.
- Category pages (`/<category-slug>`) are not in the sitemap, as before.
- Past 50,000 URLs, the oldest posts are left out rather than split into a sitemap index.
- The sitemap is not cached: each request runs the query above, now up to 49,998 rows instead
  of 100. `idx_blog_posts_published` (`published_at DESC`, non-deleted published rows, scheduled
  ones included) can serve it, at the planner's choice. Caching it with `BlogCacheTTL` is a
  follow-up.
- Some category slugs never reach the post page: `v`, `t`, `r` are taken by other routes, and
  slugs starting with `api`, `console` or `notification-center` by prefix checks. Their posts are
  listed but do not resolve (existing routing problem).
- `robots.txt` gives `Sitemap: /sitemap.xml`, a relative URL, where the protocol expects an
  absolute one.
- The sitemap answers any method, POST included, as before; the feeds answer 405 to anything
  but GET and HEAD.
- `ListPublicPosts` has no caller left; removing it is a separate cleanup.

## Run the tests

```bash
go test ./internal/domain/... ./internal/repository/... ./internal/service/... ./internal/http/...
INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TestBlog' ./tests/integration/
make test-unit
go vet ./...
```
