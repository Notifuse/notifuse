# Blog RSS Feeds — Implementation Progress

Plan: `plans/blog-rss-feeds-plan.md`

## Phase A — Domain + service layer (steps 1–4)

- [x] **Step 1** — Extend `BlogSettings` with `FeedSummaryOnly` + `FeedMaxItems` + `GetFeedMaxItems()` helper. Tests in `internal/domain/workspace_test.go`. ✔ `TestBlogSettings_GetFeedMaxItems`, `TestBlogSettings_FeedFieldsRoundTrip` passing.
- [x] **Step 2** — New file `internal/domain/blog_feed.go` with `BlogFeedItem`, `BlogFeedMeta`, `BlogFeed` types. ✔ Builds clean.
- [x] **Step 3** — `RenderPostContent` service method + body-only renderer + goquery absolute-URL rewriter + bluemonday sanitizer + control-char strip. ✔ `pkg/liquid/feed_content.go` + `feed_content_test.go`; `BlogService.RenderPostContent` + 4 service tests. All passing.
- [x] **Step 4** — `BuildFeed` + `GetFeedFingerprint` service, `ListFeedPosts` + `GetFeedFingerprint` repo (single-query JOIN with blog_categories for `GREATEST(p.updated_at, c.updated_at)`). Fallback ladder (full → excerpt → drop). ETag includes maxUpdatedAt, idsHash, categorySlug, settings blob. Mocks regen'd with `github.com/golang/mock/mockgen@v1.6.0`. ✔ 5 `BuildFeed` + 4 `GetFeedFingerprint` tests passing. **Repo-level sqlmock tests not yet written** — pending for Phase A "tests green" step.

After Phase A: `make test-domain test-service test-repo test-pkg` all green, then stop for review. ✔ All green as of 2026-04-15.

**Phase A pending items** — resolved 2026-04-16 via post-review fixes:
- [x] Repo-layer sqlmock tests added (ListFeedPosts baseline, category filter, fingerprint happy path, empty-feed NULL handling).

## Post-Phase A review fixes (2026-04-16)

Gemini-driven review surfaced security, correctness, and perf issues. All fixed before Phase B:

- [x] **srcset XSS bypass** — `cleanSrcset` drops entries with unsafe schemes, runs unconditionally. Regression test in `feed_content_test.go`.
- [x] **Empty-origin guard** — `BuildFeed` returns error when workspace has no WebsiteURL/CustomEndpointURL. Prevents feeds with relative GUIDs.
- [x] **N+1 eliminated** — `BuildFeed` fetches workspace once, preloads templates keyed by `(id, version)`. Added `renderPostContentFromEntities` as the shared render path. Test `templates are batched across posts sharing the same template` verifies `GetTemplateByID` called only once for 3 shared-template posts.
- [x] **Weak ETag** — `W/"..."` prefix. Strong ETag overstated byte-identity across sanitizer version bumps.
- [x] **Epoch fallback** — repo returns zero-time for empty feeds (via `sql.NullTime`); service substitutes `workspace.UpdatedAt` so `<lastBuildDate>` isn't 1970.
- [x] **Mock lib state cleaned** — `go mod tidy` dropped accidental `go.uber.org/mock` indirect dep. Project stays on `github.com/golang/mock v1.6.0`. Whole-project migration deferred as separate cleanup.
- [x] **Test gaps closed** — added: empty-feed UpdatedAt fallback, Unicode titles preserved, weak-ETag prefix, template batching, srcset XSS regression (4 cases), repo sqlmock for both new methods.

## Phase B — HTTP + renderers + UI (steps 5–10) ✔ completed 2026-04-17

- [x] **Step 5** — `pkg/blogfeed/rss.go` + `jsonfeed.go`. RSS 2.0 with 4 namespaces, CDATA, RFC 822 dates, media:content, dc:creator, channel image. JSON Feed 1.1 with RFC 3339 dates, tags, authors. 13 tests.
- [x] **Step 6** — `serveBlogFeed` in `root_handler.go`: `/feed.xml`, `/feed.json`, `/{slug}/feed.xml|json`. Two-phase conditional GET (304 skips BuildFeed), HEAD support, gzip, Cache-Control: `public, max-age=0, s-maxage=300, must-revalidate`. 8 handler tests.
- [x] **Step 7** — `pkg/liquid/feed_discovery.go`: `InjectFeedDiscoveryTags` — injected before `</head>` in all three render methods. Category pages get category-scoped links too. 4 tests.
- [x] **Step 8** — Sitemap: `/feed.xml` + per-category `/slug/feed.xml` appended. Existing sitemap tests updated (Times(3), 2 URL count).
- [x] **Step 9** — Console `BlogSettings.tsx`: RSS/Feeds section with `feed_max_items` Select + `feed_summary_only` Select. TS types extended. `npm run lingui:extract` run.
- [ ] **Step 10** — Integration tests deferred (requires live DB; will be in a separate session).

## Final review fixes (2026-04-17)

Gemini deep review surfaced 3 blockers + 7 should-fixes. All blockers and critical should-fixes resolved:

### Blockers fixed
- [x] **`valuePropName="checked"` on `<Select>`** (`BlogSettings.tsx`) — removed; Select uses `value` by default
- [x] **`Vary: Accept-Encoding` only on gzip path** (`root_handler.go`) — moved to unconditional before response body
- [x] **RSS `<description>` empty** (`blog_service.go`) — populated from `BlogSettings.SEO.MetaDescription`, fallback to blog title

### Should-fixes resolved
- [x] **`pkg/blogfeed` imports `internal/domain`** — moved to `internal/blogfeed/`
- [x] **JSON Feed `feed_url` points to `/feed.xml`** — handler patches SelfURL/FeedURL for JSON format after BuildFeed
- [x] **Dead `_ = idsHash`** — already removed in earlier edit

### Accepted / deferred
- HEAD still calls BuildFeed — acceptable for now; optimize if latency shows up in monitoring
- RSS date format warning (RFC 1123Z vs strict RFC 822) — all major parsers accept RFC 1123Z
- `console.log` calls in BlogSettings.tsx — pre-existing, not introduced by this PR
- `Content-Length` on non-gzip — Go net/http sets it automatically for small responses
- `categorySlug` in discovery tags not URL-encoded — slugs are validated at creation time to `^[a-z0-9]+(?:-[a-z0-9]+)*$`, so special chars are impossible

## Notes / deviations

- **Step 3 (sanitizer URL scheme policy)** — plan said "`data:` URIs allowed only for `image/*`". bluemonday's `AllowURLSchemesMatching` takes a `*regexp.Regexp`, not a predicate, and distinguishing `data:image/png` from `data:text/html,...` with a scheme-only regex is impossible (the `image/png` part is the MIME type inside the URI, not part of the scheme). Simpler + safer policy: drop `data:` entirely, allow only `http`, `https`, `mailto`, `tel`. Trade-off: no inline base64 images survive into feeds — acceptable because most aggregators don't render them anyway.
