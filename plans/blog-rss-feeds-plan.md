# Blog RSS & JSON Feeds

## Context

The Notifuse blog feature (served per-workspace via custom domain in `internal/http/root_handler.go:220`) currently exposes HTML pages, `robots.txt`, and `sitemap.xml`, but no syndication feed. A search for `rss`, `atom`, or `feed` across the blog code (`internal/service/blog_service.go`, `internal/http/blog_handler.go`, `pkg/liquid/blog_renderer.go`) returns nothing.

This plan adds RSS 2.0 + JSON Feed 1.1 feeds so readers (Feedly, Inoreader, NetNewsWire) and AI ingestors can subscribe to public blog content. Scope was confirmed with the user: RSS 2.0 + JSON Feed 1.1, main feed + per-category (no per-author), full-content HTML with a per-workspace summary-only toggle, conditional GET. WebSub, ActivityPub, and RFC 5005 pagination are explicitly deferred (cap=20 always returns the newest 20).

## Architecture fit

- Blog routing is handled via `RootHandler.serveBlog` (`internal/http/root_handler.go:220`), a `switch` over `r.URL.Path` with custom-domain workspace resolution upstream at line 102.
- Public post data access already exists: `BlogService.ListPublicPosts`, `GetPublicCategoryBySlug`, `GetPublicPostByCategoryAndSlug` (`internal/domain/blog.go:702-704`).
- Rendering pipeline lives in `pkg/liquid/blog_renderer.go`; `BlogService.RenderPostPage` returns a full themed HTML page, so it is not reusable as-is for feed items. A new body-only renderer is required.
- `BlogSettings` is a JSONB column on the workspace row (`internal/domain/workspace.go:275`), so adding feed-related fields is additive — no DB migration needed.
- Posts live at `/{category-slug}/{post-slug}`. Feeds slot in as additional special paths.

## URL conventions

- `/feed.xml` — main RSS 2.0
- `/feed.json` — main JSON Feed 1.1
- `/{category-slug}/feed.xml` and `/{category-slug}/feed.json` — per-category
- No pagination — feed always returns newest `FeedMaxItems` (default/cap 20). Aggregators poll; that's the traversal mechanism.
- **Slug collision is impossible by construction**: `slugRegex` at `internal/domain/blog.go:21` is `^[a-z0-9]+(?:-[a-z0-9]+)*$` — it rejects dots, so `feed.xml`, `feed.json`, `robots.txt`, `sitemap.xml` can never be valid category or post slugs. No additional validation needed. The HTTP layer still short-circuits the feed path before post lookup as defense-in-depth.

## Implementation steps

### 1. Extend `BlogSettings` with feed fields
File: `internal/domain/workspace.go:275`

Add two fields to `BlogSettings`:
```go
FeedSummaryOnly bool `json:"feed_summary_only,omitempty"`
FeedMaxItems    int  `json:"feed_max_items,omitempty"` // default 20, cap 20
```
Add `GetFeedMaxItems()` helper mirroring the existing `GetHomePageSize()` / `GetCategoryPageSize()` pattern (lines 286, 294). Bounds: [1, 20], default 20. No migration — JSONB column.

Tests: `internal/domain/workspace_test.go` — default value, bounds (1 and 20, over-cap clamps to 20), Value/Scan round-trip preserving new fields.

### 2. Feed domain types
New file: `internal/domain/blog_feed.go`

```go
type BlogFeedItem struct {
    GUID             string       // post.ID (immutable — never reuse slug)
    Title            string
    URL              string       // absolute
    CategorySlug     string
    CategoryName     string
    ContentHTML      string       // full rendered body with absolute URLs
    Excerpt          string
    Authors          []BlogAuthor
    FeaturedImageURL string
    PublishedAt      time.Time
    UpdatedAt        time.Time
}

type BlogFeedMeta struct {
    Title, Description, SiteURL, FeedURL, SelfURL string
    Language, IconURL, LogoURL                    string
    UpdatedAt                                     time.Time
    ETag                                          string
}

type BlogFeed struct {
    Meta  BlogFeedMeta
    Items []BlogFeedItem
}
```

Tests: `internal/domain/blog_feed_test.go` — basic validation helpers if added.

### 3. Service: `RenderPostContent` (body-only HTML with absolute URLs)
Files:
- Interface: `internal/domain/blog.go:715` — add `RenderPostContent(ctx, workspaceID, categorySlug, postSlug string) (string, error)` to `BlogService`.
- Impl: `internal/service/blog_service.go`.
- Rewriter: extend `pkg/liquid/blog_renderer.go` (or add `pkg/liquid/absolute_urls.go`).

Reuse the existing liquid/template rendering pipeline, but skip theme chrome — return the article body only. Post-processing pipeline:
1. **goquery**: rewrite relative `href`, `src`, and `srcset` attributes to absolute URLs based on the workspace's public blog origin.
2. **bluemonday** (new dependency: `github.com/microcosm-cc/bluemonday`): run output through `UGCPolicy()` customized to allow image embedding but strip `<script>`, all `on*` event attributes, and `javascript:` / `vbscript:` URI schemes on `href`/`src`. `data:` URIs allowed only for `image/*`.
3. **Control-character strip**: remove U+0000–U+001F except TAB/LF/CR (XML 1.0 compliance — parsers choke otherwise).

Tests:
- `pkg/liquid/blog_renderer_test.go` — golden tests for absolute URL rewriting across href/src/srcset; control-char stripping; unchanged already-absolute URLs.
- XSS sanitization table: `<script>`, `onerror=`, `javascript:`, `vbscript:`, `data:text/html` are all stripped; `data:image/png` preserved; benign HTML preserved.
- `internal/service/blog_service_test.go` — `RenderPostContent` happy path and missing-post error.

### 4. Service: `BuildFeed`
File: `internal/service/blog_service.go` (+ interface entry in `internal/domain/blog.go:715`).

```go
BuildFeed(ctx context.Context, workspaceID string, categorySlug *string) (*domain.BlogFeed, error)
```

Behavior:
- Page size from `BlogSettings.GetFeedMaxItems()` (default 20, cap 20). No pagination — always returns the newest N.
- Data source: a new repo method `ListFeedPosts(ctx, workspaceID, categorySlug *string, limit int)` (or an option added to `ListPublicPosts`) that enforces:
  - `deleted_at IS NULL`
  - `published_at IS NOT NULL AND published_at <= NOW()` — `ListPublicPosts` today (see `internal/repository/blog_postgres.go:740`) only enforces `IS NOT NULL`; scheduled posts would leak otherwise.
  - `ORDER BY published_at DESC LIMIT :limit`
  This mirrors the filter used by `GetFeedFingerprint` so the two always agree.
- All `updated_at` / `published_at` comparisons use `TIMESTAMPTZ` in UTC to avoid ETag drift across zones.
- Resolves category + authors for each post.
- **Content fallback ladder** (replaces naive skip-broken):
  1. Try `RenderPostContent` (full HTML) when `FeedSummaryOnly == false`.
  2. On render error → fall back to `BlogPostSettings.Excerpt` + title + link (still useful item in the feed). Log at WARN level with post ID.
  3. Only if excerpt is also empty → drop the item, log at ERROR level with post ID, and increment a metric `blog_feed_items_dropped_total{workspace_id}` so admins get surfaced signal, not a stealth outage.
  When `FeedSummaryOnly == true`, step 1 is skipped and excerpt is used directly.
- **Two-phase ETag** (cheap path for conditional GET):
  - New repository method `GetFeedFingerprint(ctx, workspaceID string, categorySlug *string) (maxUpdatedAt time.Time, idsHash string, err error)`. Single SELECT joining `blog_posts p` with `blog_categories c`:
    ```sql
    SELECT
      max(GREATEST(p.updated_at, c.updated_at)) AT TIME ZONE 'UTC' AS max_updated_at,
      md5(coalesce(string_agg(p.id::text, ',' ORDER BY p.id), '')) AS ids_hash
    FROM blog_posts p
    JOIN blog_categories c ON c.id = p.category_id
    WHERE p.deleted_at IS NULL
      AND p.published_at IS NOT NULL
      AND p.published_at <= NOW()
      AND (<category slug filter when provided>)
    ORDER BY p.published_at DESC
    LIMIT <FeedMaxItems>
    ```
    - `max(GREATEST(p.updated_at, c.updated_at))` captures both post edits and **category renames** in one comparable timestamp (no separate hash needed). Category `updated_at` bumps on rename because `blog_categories` carries `updated_at`.
    - Author renames are captured automatically: authors are embedded JSON in `BlogPostSettings.Authors` (see `internal/domain/blog.go:114, 197`), so renaming an author updates the post and bumps `p.updated_at`.
    - `idsHash` detects same-second delete-and-publish (where `max_updated_at` alone would collide).
  - **ETag inputs** (hashed via SHA-256, hex-truncated to 16 chars):
    - `maxUpdatedAt` (UTC, RFC 3339 nanosecond precision)
    - `idsHash`
    - `categorySlug` (empty string for main feed — critical: without this, main and category feeds share ETags)
    - `settingsFingerprint` = SHA-256 of the marshalled subset: `BlogSettings.Title`, `LogoURL`, `IconURL`, `FeedSummaryOnly`, `FeedMaxItems`, plus `WorkspaceSettings.DefaultLanguage` (`internal/domain/workspace.go:337`). Any of these changing invalidates cached 304s.
  - HTTP layer calls `GetFeedFingerprint` first, checks `If-None-Match` / `If-Modified-Since`, returns 304 **without invoking `BuildFeed` or rendering anything**.
  - Only on cache miss does it call `BuildFeed`, which recomputes the same fingerprint for response headers.

Regenerate `internal/domain/mocks/mock_blog_service.go` with `go generate`.

Tests: `internal/service/blog_service_test.go`:
- Main feed with N published posts
- Per-category filter
- Summary-only toggle chooses excerpt
- Empty/unknown category → error
- Items capped at `FeedMaxItems` with newest-first ordering preserved
- Scheduled post (`published_at > NOW()`) is NOT included in the feed
- ETag stable across repeated calls when data unchanged
- ETag changes when a post is updated
- ETag changes when a post is **deleted** (idsHash regression test)
- ETag changes when a category is **renamed** (bumps `blog_categories.updated_at` → captured by `GREATEST(p.updated_at, c.updated_at)`)
- ETag changes when an author is renamed (triggers post settings write, bumping `p.updated_at`)
- ETag changes when `BlogSettings.FeedSummaryOnly` is toggled (settingsFingerprint regression test)
- Main feed and category feed produce DIFFERENT ETags for the same underlying posts (categorySlug in fingerprint)
- `GetFeedFingerprint` agrees with ETag produced by `BuildFeed` for the same inputs
- Fallback ladder: render error → excerpt used, logged WARN
- Fallback ladder: render error + empty excerpt → item dropped, logged ERROR, metric incremented

### 5. Feed renderer package
New package: `pkg/blogfeed/`

**Implementation requirements** (compliance + SOTA):
- Use `encoding/xml` (not string concatenation) so element text is always escaped safely.
- Declare all namespaces on `<rss>`:
  ```xml
  <rss version="2.0"
       xmlns:atom="http://www.w3.org/2005/Atom"
       xmlns:content="http://purl.org/rss/1.0/modules/content/"
       xmlns:dc="http://purl.org/dc/elements/1.1/"
       xmlns:media="http://search.yahoo.com/mrss/">
  ```
- Dates: RSS uses RFC 822 (`time.RFC1123Z`); JSON Feed uses RFC 3339.
- Deterministic element/attribute order so ETag stays byte-stable.

`rss.go` — RSS 2.0 with:
- `<channel>`: `title`, `description`, `link`, `language` (from `WorkspaceSettings.DefaultLanguage` at `internal/domain/workspace.go:337`, fallback `en`), `lastBuildDate` (= `meta.UpdatedAt` in RFC 822), `<atom:link rel="self">`, `<image>` (channel-level — `url`, `title`, `link` — when `LogoURL` or `IconURL` set).
- `<item>`: `title`, `link`, `guid isPermaLink="false"` (= post.ID), `pubDate` (RFC 822), `description` (excerpt), `content:encoded` inside CDATA (full HTML when not summary-only), one `<category>` per category the post belongs to, `<dc:creator>` per author, `<media:content url="…" medium="image">` and `<media:thumbnail url="…">` for featured image.

`jsonfeed.go` — JSON Feed 1.1 with:
- Top-level: `version`, `title`, `home_page_url`, `feed_url`, `language`, `icon`, `favicon`, `description`.
- Items: `id` (= post.ID), `url`, `title`, `content_html` (full when not summary-only), `summary` (excerpt), `date_published`, `date_modified` (both RFC 3339), `authors[]` with `name` + `url`, `image` (featured image), `banner_image` (when available), `tags` (category names).

**Transport**: handler negotiates `Accept-Encoding` and emits gzip when the client accepts it. Feed XML compresses ~8×; free infra win.

Tests: `pkg/blogfeed/rss_test.go`, `pkg/blogfeed/jsonfeed_test.go`:
- Golden-file output
- XML well-formedness (parse round-trip with `encoding/xml`)
- All four namespaces declared on `<rss>`
- JSON schema matches JSON Feed 1.1 fields
- Absolute URL invariant on all emitted links
- Dates in RFC 822 / RFC 3339 respectively
- `<media:thumbnail>` + `<media:content>` present when post has featured image
- Per-item `<category>` / JSON `tags` present
- Output survives a title containing `<`, `&`, `"`, and control chars without producing invalid XML

### 6. HTTP: wire feed routes + conditional GET
File: `internal/http/root_handler.go`

Extend the special-path switch in `serveBlog` (line 225) and the 2-part branch (line 252):

```go
case "/feed.xml":   h.serveBlogFeed(w, r, workspace, nil, feedFormatRSS);  return
case "/feed.json":  h.serveBlogFeed(w, r, workspace, nil, feedFormatJSON); return
```

In the `len(parts) == 2` block, intercept `parts[1] == "feed.xml" | "feed.json"` **before the post lookup** → per-category feed. This short-circuit protects against any legacy bad slugs that slipped in before the reserved-slug validation.

New `serveBlogFeed(w, r, workspace, categorySlug *string, format feedFormat)`:
- Accept only `GET` and `HEAD`; others → 405. `HEAD` returns same headers, empty body (free 304 support).
- **Conditional GET first (cheap path)**: call `blogService.GetFeedFingerprint` and compute ETag. If `If-None-Match` matches or `If-Modified-Since` ≥ `maxUpdatedAt`, return `304` with `ETag`, `Last-Modified` (= `maxUpdatedAt.UTC().Format(http.TimeFormat)`), and `Cache-Control` headers only — no `BuildFeed` invocation, no rendering.
- On cache miss, call `blogService.BuildFeed`.
- Headers: `Content-Type: application/rss+xml; charset=utf-8` or `application/feed+json; charset=utf-8`, `ETag`, `Last-Modified`, `Cache-Control: public, max-age=0, s-maxage=300, must-revalidate` (browsers revalidate via ETag; shared caches/CDNs coalesce for 5 min).
- `Vary: Accept-Encoding` when gzip negotiated.
- Render via `pkg/blogfeed`, optionally gzip-wrapping the body.
- Honor existing in-memory blog cache keyed by `host:path` (no page dimension anymore).

Tests: `internal/http/root_handler_test.go` — table-driven:
- 200 RSS + valid XML
- 200 JSON + schema validity
- 200 per-category
- 405 on POST/PUT/DELETE
- HEAD returns same headers, empty body
- 404 unknown category
- 304 on matching `If-None-Match`
- 304 on `If-Modified-Since` newer than `meta.UpdatedAt`
- 304 path does NOT call `BuildFeed` (verified via mock expectations — only `GetFeedFingerprint` invoked)
- 304 response includes `ETag` and `Last-Modified` headers (not just the status)
- `<language>` and `<lastBuildDate>` present at channel level
- gzip served when `Accept-Encoding: gzip` + `Vary: Accept-Encoding` emitted
- `Content-Type` and `Cache-Control` correctness

### 7. Autodiscovery `<link>` tags
File: `pkg/liquid/blog_renderer.go` (and any default theme `<head>` template it references).

Inject:
```html
<link rel="alternate" type="application/rss+xml" title="…" href="/feed.xml">
<link rel="alternate" type="application/feed+json" title="…" href="/feed.json">
```
On category pages, also include the category-scoped variants `/{category-slug}/feed.xml|.json`.

**Ship ordering**: this step must merge **with or after** Step 6 (HTTP routes). Shipping autodiscovery before the routes exist would advertise 404 URLs.

Tests: `pkg/liquid/blog_renderer_test.go` — presence assertions on rendered home/category pages.

### 8. Sitemap: include feed URLs
File: `internal/http/root_handler.go:502` (`serveBlogSitemap`)

Append `/feed.xml` and each per-category feed URL as `<url>` entries.

Tests: extend existing sitemap test in `internal/http/root_handler_test.go`.

### 9. Console: workspace blog settings UI
File: `console/src/components/settings/BlogSettings.tsx`

Add a "RSS / Feeds" section:
- `feed_summary_only` — Ant Design `Switch`
- `feed_max_items` — `InputNumber` with bounds [1, 20], default 20

All user-facing strings via `useLingui()` macro (`t\`RSS / Feeds\``, etc.). Run `npm run lingui:extract` afterwards.

Persist through the existing workspace settings save path (no new API endpoint — `BlogSettings` is updated as part of `workspace.update`).

Tests: existing `BlogSettings` Vitest component test — toggle persists, number bounds validated.

**Ship ordering**: this step must merge **after** Step 6 (HTTP routes). Shipping the settings UI before the routes exist would let admins toggle dead switches.

### 10. Integration tests
File: `tests/integration/blog_api_test.go`

Add feed E2E cases:
- Publish two posts in one category, one in another → GET `/feed.xml` returns both, per-category feed returns only matching.
- Update a post → ETag changes; follow-up GET with old ETag returns 304 on the pre-update feed, 200 on the post-update one.
- **Delete a post → ETag changes** (regression test for `idsHash`).
- Rename a category → ETag changes (regression for `categoryFingerprint`).
- `feed_summary_only = true` → `description` holds excerpt, not full body.
- Malicious post content containing `<script>alert(1)</script>` + `onerror=` → sanitized in both RSS `content:encoded` and JSON `content_html`.
- Scheduled post (`published_at` in the future) is not included.

## Critical files

- `internal/domain/workspace.go:275` — `BlogSettings` extension
- `internal/domain/blog.go:702,715` — public read methods + new service interface entries
- `internal/domain/blog_feed.go` (new) — feed types
- `internal/service/blog_service.go` — `BuildFeed`, `RenderPostContent`
- `internal/repository/blog_postgres.go:738` — new `ListFeedPosts` + `GetFeedFingerprint` (enforces `published_at <= NOW()`)
- `internal/http/root_handler.go:220,502` — feed routes + sitemap update
- `pkg/blogfeed/` (new package) — RSS + JSON renderers
- `pkg/liquid/blog_renderer.go` — body-only rendering + absolute URL rewriting + autodiscovery tags
- `console/src/components/settings/BlogSettings.tsx` — admin UI
- `tests/integration/blog_api_test.go` — E2E

## Reused utilities

- `ListPublicPosts`, `GetPublicCategoryBySlug` (`internal/domain/blog.go:702-704`) — data access
- `BlogSettings.GetHomePageSize` pattern (`internal/domain/workspace.go:286`) — default/cap helper style
- `PuerkitoBio/goquery` (already in `go.mod`) — HTML rewriting
- `github.com/microcosm-cc/bluemonday` (new dependency) — HTML sanitization
- Existing blog in-memory cache in `RootHandler` (used at `internal/http/root_handler.go:302`) — feed caching
- Liquid rendering pipeline in `pkg/liquid/blog_renderer.go` — body render

## Verification

Run from project root:

```bash
make test-domain
make test-service
make test-http
make test-pkg
make test-integration
make coverage
cd console && npm test -- BlogSettings
cd console && npm run lingui:extract
```

End-to-end manual verification:

1. `make dev` — start backend with hot reload.
2. Create a workspace with `blog_enabled = true`, add a category + 3 posts, publish them.
3. `curl -i https://<workspace-host>/feed.xml` — expect 200, `application/rss+xml`, valid XML with 3 items.
4. Repeat with returned ETag in `If-None-Match` — expect 304.
5. `curl -i https://<workspace-host>/feed.json` — expect 200, valid JSON Feed 1.1.
6. `curl -i https://<workspace-host>/<category-slug>/feed.xml` — expect filtered items.
7. View blog home in browser, inspect `<head>` — verify `<link rel="alternate">` tags.
8. Toggle `feed_summary_only` in the console, save, re-fetch feed — expect excerpts in `<description>` / `content_html`.
9. Set `feed_max_items = 2`, publish a 4th post — expect exactly 2 (newest) items.
10. Create a post containing `<script>alert(1)</script>` in its body → sanitized in the feed output.
11. Validate output in https://validator.w3.org/feed/ and https://validator.jsonfeed.org/.

## Out of scope (future work)

- Per-author feeds (`/author/{slug}/feed.xml`)
- WebSub hub integration (real-time push) — deliberately not stubbed; half-wired hub links break aggregator expectations
- ActivityPub / Fediverse bridging
- Atom 1.0 format
- Per-post `<enclosure>` for non-image media (podcasting)
- Persisting rendered post HTML for cheaper feed generation — revisit if feed latency becomes a problem
- RFC 5005 paged archive feeds — excluded because cap is 20; if the cap ever rises, revisit pagination then
- Caching serialized feed bytes keyed by ETag (weaker fallback to persisted rendered HTML) — revisit if render cost becomes a hotspot
