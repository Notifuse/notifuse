# Plan — console redesign, server part 6: settings and content (specs 9 and 10)

## Need

Before their console PRs, the redesigned Settings (spec 9) and Content screens (spec 10)
need:

1. `workspaces.members` returns each member's `name` (spec 9);
2. a new route `templates.usage` (spec 10);
3. a new route `blogPosts.counts` (spec 10);
4. `blogPosts.list` accepts `status=scheduled` and `search` (spec 10).

Specs (reviewed): "Spec 9 · Paramètres" and "Spec 10 · Contenu", "Côté serveur" and the server tables of
"Fichiers, tests et livraison".

Not here (separate "published server defects" thread, agreed on 12 Nov 2026): the public
blog showing scheduled posts before their date (`IsLive`, `PublishedBefore`, the RSS `NOW()`
as a parameter), the blog list limited to 100 posts, and the file manager fixes. That thread
adds `PublishedBefore` (`json:"-"`) to `ListBlogPostsRequest`; whichever PR merges second
keeps both changes when rebasing.

No table, column, index, migration or environment variable.

## Changes

### 1. `workspaces.members`: `name` (spec 9)

- `domain.UserWorkspaceWithEmail` gains `Name string` (`json:"name,omitempty" db:"name"`).
- `GetWorkspaceUsersWithEmail` (`workspace_postgres.go:959`) selects `COALESCE(u.name, '')`.
- Invitation rows built by the service have no name; the reduced response (caller only,
  without `workspace.read`) keeps the same shape, with the caller's name.
- Platform admin rows built for the owner from `rootUser` (`workspace_service.go:1441-1472`)
  copy `Name: rootUser.Name`.

### 2. `GET /api/templates.usage` (spec 10)

Parameters `workspace_id` (required), `template_id` (optional). Permission `templates.read`.

```json
{
  "usage": { "black-friday": { "broadcasts": 1, "transactional_notifications": 0, "automations": 0, "lists": 0 } },
  "references": [ { "type": "broadcast", "id": "bc_123", "name": "Black Friday", "status": "scheduled" } ],
  "hidden_types": ["lists"]
}
```

- `usage` holds only templates used at least once; with `template_id`, only that one, and
  `references` is filled (absent otherwise).
- A type the member cannot read (`broadcasts.read`, `transactional.read`, `automations.read`,
  `lists.read`) is not queried, is absent from `usage` and `references`, and is listed in
  `hidden_types`. The counter of a hidden type is omitted from each entry
  (`*int`, `omitempty`), so a member never sees a 0 for a type they cannot read.
- One grouped query per readable type, on the workspace database:
  - broadcasts: `jsonb_array_elements(test_settings->'variations')->>'template_id'`,
    `COUNT(DISTINCT b.id)`, statuses `draft`, `scheduled`, `processing`, `paused`, `testing`,
    `test_completed`, `winner_selected` only;
  - transactional notifications: `channels->'email'->>'template_id'`, `deleted_at IS NULL`;
  - automations: email nodes' `config->>'template_id'` and `trigger_config->>'template_id'`,
    `deleted_at IS NULL`, all statuses, counted once per automation;
  - lists: `double_optin_template->>'id'`, `deleted_at IS NULL AND is_double_optin`.
  Conditions on templates inside `segments.tree` and `trigger_config.conditions` are not
  counted.
- With `template_id`, each query filters that template and also returns ID, name and (for
  broadcasts and automations) status, ordered by name, 50 rows at most per type; the count
  in `usage` stays exact beyond.
- Types: `TemplateUsage`, `TemplateUsageReference` in `internal/domain/template.go`;
  `TemplateRepository.GetTemplateUsage(ctx, workspaceID, templateID string, types []TemplateUsageType)`;
  `TemplateService.GetTemplateUsage`; route in `template_handler.go`; route classified in
  `AuditExcludedRoutes`.

The column names above are checked against `init.go` before writing the SQL (in particular
the column holding a broadcast's variations and the automation trigger column); any
difference with the spec is corrected here and noted in the PR.

### 3. `GET /api/blogPosts.counts` (spec 10)

Parameters `workspace_id`, `category_id` (optional). Permission `blog.read`.
`{ "all_total", "total", "published", "scheduled", "draft", "by_category" }`, one query on
`blog_posts` with `deleted_at IS NULL` and the current instant passed as a parameter
(`time.Now().UTC()`, since `published_at` is a `TIMESTAMP` without time zone written in UTC):

- `all_total`: `COUNT(*)` whatever `category_id`;
- `total`: `COUNT(*) FILTER (WHERE category_id = $c)`, or `all_total` without category;
- `published`: `published_at IS NOT NULL`; `scheduled`: `published_at > $now`; `draft`:
  `published_at IS NULL`; each with the same category filter as `total`;
- `by_category`: a second grouped query (`GROUP BY category_id`, category not NULL) over all
  posts, whatever `category_id`.

Layers: `BlogPostCounts` in `domain/blog.go`, `BlogPostRepository.CountPosts`,
`BlogService.CountPosts`, route in `blog_handler.go`, `AuditExcludedRoutes`.

### 4. `blogPosts.list`: `scheduled` and `search` (spec 10)

- `BlogPostStatusScheduled`; `ListBlogPostsRequest.Validate` accepts it.
- `status=scheduled`: `published_at IS NOT NULL AND published_at > $now` (`$now` =
  `time.Now().UTC()` set by the service on the request, `json:"-"`), ordered
  `published_at ASC, id ASC`. `all`, `draft` and `published` unchanged.
- `search` (optional, at most 200 characters after trim): `settings->>'title' ILIKE $n`
  with `%`, `_` and `\` escaped (`escapeLikePattern`); `total_count` follows it.
- The handler reads `search` from the query.

### 5. OpenAPI

`openapi/paths/templates.yaml` (`templates.usage`, with `hidden_types`),
`openapi/paths/blog.yaml` (`blogPosts.counts`, `scheduled`, `search`), two `$ref` in
`openapi/openapi.yaml`; `make openapi-lint`, `make openapi-bundle`.

## Files and tests (TDD: tests first)

| File | Change | Test file | Cases |
| --- | --- | --- | --- |
| `internal/domain/workspace.go` | `Name` | `internal/domain/workspace_test.go` | JSON with `name`; without `name` when empty |
| `internal/repository/workspace_postgres.go` | `COALESCE(u.name, '')` | `internal/repository/workspace_users_email_test.go` | column read; NULL name returned empty |
| `internal/service/workspace_service.go` | `Name: rootUser.Name` | `internal/service/workspace_service_test.go` | reduced response keeps the caller's name; invitation without name; platform admin row carries its name |
| `internal/domain/template.go` | usage types, repository and service methods | `internal/domain/template_test.go` | JSON: `references` omitted without `template_id`; `hidden_types`; hidden counter omitted |
| `internal/repository/template_postgres.go` | `GetTemplateUsage` | `internal/repository/template_postgres_test.go` | sqlmock: SQL of each type; `template_id` filter; `deleted_at IS NULL` on three tables; kept broadcast statuses; `COUNT(DISTINCT` on broadcasts and automations; `is_double_optin` on lists; a type not requested: no query |
| `internal/service/template_service.go` | `GetTemplateUsage` | `internal/service/template_service_test.go` | refused without `templates.read`; an unreadable type absent from `usage` and `references`, present in `hidden_types`, not queried |
| `internal/http/template_handler.go` | route | `internal/http/template_handler_test.go` | non-GET → 405; missing `workspace_id` → 400; `template_id` optional; permission → 403; response |
| `internal/domain/blog.go` | `BlogPostStatusScheduled`, `Search` (200), `Now` (`json:"-"`), `BlogPostCounts`, repository and service methods | `internal/domain/blog_test.go` | `scheduled` accepted; unknown status refused; 201-character `search` refused; `Now` never read from JSON |
| `internal/repository/blog_postgres.go` | `scheduled` filter and order; `search`; `CountPosts` | `internal/repository/blog_postgres_test.go` | `scheduled` SQL with the instant as argument and `published_at ASC, id ASC`; escaped `ILIKE`; `total_count` filtered; counts with and without category; `by_category` |
| `internal/service/blog_service.go` | `CountPosts`; `Now` set on list requests | `internal/service/blog_service_test.go` | permission and repository relay; `Now` set to a UTC instant |
| `internal/http/blog_handler.go` | `blogPosts.counts`; `search` read | `internal/http/blog_handler_test.go` | route method and params; `search` passed to the service |
| `internal/domain/audit_actions.go` | two routes excluded | `internal/http/audit_routes_test.go` (existing) | passes |
| `openapi/…` | routes and parameters | `make openapi-lint` | — |
| `tests/integration/templates_usage_test.go` (new) | — | same | `TestTemplatesUsage`: one template used by a draft broadcast (two variations), a notification, an automation (email node and trigger) and a double opt-in list, each counter equal to 1 (`require.Equal` per counter); a `processed` broadcast, a deleted object and a list without double opt-in not counted; a member without `automations.read`: `automations` absent and in `hidden_types` |
| `tests/integration/blog_api_test.go` | new cases | same | a scheduled post counted in `published` and `scheduled`, listed by `status=scheduled`; `all_total` inside a category; search with `%` in a title |
| `tests/integration/workspace_test.go` | case added | same | a member with a name returns it; one without returns an empty string |
| `CHANGELOG.md` | Feature line | — | — |

CHANGELOG (short; the console PRs extend it with the screens):

- **Feature**: New API endpoints `templates.usage` and `blogPosts.counts`; `blogPosts.list`
  accepts `status=scheduled` and `search`; `workspaces.members` returns each member's `name`.

## Verification

```bash
make test-domain test-repo test-service test-http
make openapi-lint && make openapi-bundle
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestTemplatesUsage|TestBlog|TestWorkspace' -v
```

Then `make test-unit` once before review. T2 timing on a demo workspace grown to 5,000
broadcasts and 500 automations is measured in the console PR.

## Plan review and corrections

Review: `reviews/console-redesign-server/plan-6-review.md` in the project files. Every finding
was checked in the code; the corrections below take precedence over the sections above.

- **B1**: `automations.nodes` can hold JSON `null` (an automation without nodes,
  `automation_postgres.go:72, 330`), and `jsonb_array_elements` on a scalar fails the whole
  grouped query. Both arrays go through `CASE WHEN jsonb_typeof(x) = 'array' THEN x ELSE '[]'::jsonb END`;
  sqlmock assertion and an integration case with an automation without nodes.
- **S1**: empty template IDs (trigger `template_id: ""`, a non-A/B draft variation without
  template) are filtered out (`<> ''`) on every source.
- **S2**: with `template_id`, each object is selected once with `EXISTS` (automations: node
  `OR` trigger), ordered `name, id`, `LIMIT 50`; the exact count comes from
  `COUNT(*) OVER ()` computed before the limit. Integration case with `template_id`.
- **S3**: `blogPosts.list` validation errors (status, `search` over 200 runes) answer 400
  through a `domain.ValidationError` and `writeValidationError`; today an unknown status gives
  500. CHANGELOG: one Improvement line ("`blogPosts.list` answers 400 instead of 500 for an
  invalid status").
- **S4**: the five existing sqlmock expectations of `workspace_users_email_test.go` are
  rewritten with the new column and a regex including `COALESCE`; integration: a user with
  `name = NULL` inserted by SQL, and the key absent from the JSON (`omitempty`), a named member
  returns it.
- **S5**: mocks of `TemplateRepository`, `TemplateService`, `BlogPostRepository`,
  `BlogService` regenerated one by one with mockgen v1.6.0.
- **S6**: JSON contract: `usage` always an object (`{}`); `references` present (possibly `[]`)
  exactly when `template_id` is given; `hidden_types` always an array (`[]`); `references[].type`
  is one of `broadcast`, `transactional_notification`, `automation`, `list`; a hidden type's
  counter is absent from every entry (the example above showing `"lists": 0` with `lists`
  hidden is wrong).
- **S7**: `make test-integration` once before review.
- N1: `blogPosts.counts` validates `category_id` as a UUID (400 otherwise); one query grouped
  by category, totals computed in Go from it (one snapshot); `by_category` always an object.
  N2: `CountPosts(ctx, workspaceID, categoryID string, now time.Time)`; a zero `Now` on
  `status=scheduled` falls back to `time.Now().UTC()`; sqlmock matcher checks UTC; no cast to
  `timestamptz`. N3: members service cases in `internal/service/workspace_users_service_test.go`.
  N4: `blogPosts.counts` in `TestBlogHandler_PermissionErrors`; the new template handler maps
  permission errors to 403. N5: literals for 201 and 50 in tests, 200 accepted, runes counted.
  N6: column hedge removed (names match `init.go`); broadcast statuses from domain constants;
  tie-breaker test placeholders adjusted. N7: blog web templates are not counted, said in the
  OpenAPI description. N8: one deleted object per soft-deletable type. N9: response schemas in
  `components/schemas/template.yaml` and `blog.yaml`; regenerated `openapi.json` committed.

## Final verification

```bash
make test-domain test-repo test-service test-http
make openapi-lint && make openapi-bundle
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestTemplatesUsage|TestBlogAPI|TestWorkspaceMembershipSuite' -v
make test-unit
make test-integration
```

## Code review and corrections

Review: `reviews/console-redesign-server/pr-6-code-review.md` in the project files.

Corrected:

- `blogPosts.counts` passes `category_id` in canonical UUID form: an upper-case, braced or
  undashed ID returned zero counts (handler test added).
- The doc comment of `ListPosts` had moved onto `CountPosts`; restored.
- Files as built (the table above names the files planned): `GetTemplateUsage` is in
  `internal/repository/template_usage_postgres.go`, and the new unit tests are in
  `template_usage_postgres_test.go`, `template_usage_service_test.go`,
  `template_usage_handler_test.go`, `blog_counts_postgres_test.go`,
  `blog_counts_service_test.go` and `blog_counts_handler_test.go`, beside the domain tests in
  `template_test.go`, `blog_test.go` and `workspace_test.go`.

Known, not handled:

- A NUL byte or invalid UTF-8 in `search` or `template_id` answers 500, and
  `blogPosts.list?category_id=news` answers 500 as before this change.
- A `winner_selected` broadcast counts its losing variation's template, and a broadcast with
  A/B testing off counts every stored variation although only the first is sent.
- The `<> ''` guards, the broadcast-variations guard and `LIMIT 50` with `COUNT(*) OVER ()` are
  pinned by SQL-string tests only; no test compares instants across time zones; the sqlmock
  labels of `…_OnlyRequestedTypes` do not identify the query that ran.
- Nits: a stray comment in `blog_postgres.go`, an imprecise doc on `GetTemplateUsage`, the
  `uuid` import in the standard-library group of `blog_handler.go`, `template_id=` (empty)
  read as absent, and an error-level log for a request answered with 400.
