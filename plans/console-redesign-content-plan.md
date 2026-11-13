# Console redesign — Content screens (spec 10)

Source spec: "Spec 10 · Contenu" (Nuit claire redesign, screens 121 templates, 122 blog, 123 file manager,
124 image picker). This plan implements its console part.

## What already shipped on `dev`

- #527: `templates.usage`, `blogPosts.counts`, `blogPosts.list` with `status=scheduled` and `search`
  (server, OpenAPI, CHANGELOG feature line).
- #525: scheduled posts offline until their date (server fix, CHANGELOG `Fix`), and the console post list
  asks the server page by page (antd pagination, 50 per page).
- #526: `storageObjects.ts` — `listAllObjects` (continuation tokens, `MAX_OBJECTS` 10 000, `capped`,
  `cutShort`), `toStorageItems` (implicit folders), `deleteFolder` (prefix listing, one `DeleteObject` per
  key); `fileManager.tsx` uses them; CHANGELOG `Fix` lines written.
- #528: sitemap lists posts and category feeds.
- #531: layout components (`RailLayout`, `PageHeader` with `count`, `Panel`, `Pill`, `Mono`,
  `KeyValueList`, `CountTabs`, `TableToolbar`, `SidePanel`, `useDrawerNavKeys`, `lib/copyToClipboard.ts`).

So this PR is console-only: no server file, no SQL, no environment variable. Its CHANGELOG entry is one
Feature line for the screens (the Fix lines of the spec are already written by #525 and #526).

## Decisions taken while reading the code

1. **Blog list: "Load more".** #525 put antd pagination on the post table; the spec asks for "Load more"
   with `useInfiniteQuery` (key `['blog-posts', workspaceId, status, categoryId, q]`, `limit` 50). The
   table switches to it; the footer shows "{shown} of {total} posts".
2. **File manager split.** Built on `storageObjects.ts` rather than rewritten: `useStorage.ts` wraps
   `listAllObjects` + `toStorageItems` in a `useQuery` (key `['storage-objects', workspaceId, bucket,
   endpoint]`, `staleTime` 60 s, cap injected for tests) and exposes the mutations (upload, new folder,
   delete file, delete selection, delete folder through `deleteFolder`, zip); `storageTree.ts` adds the
   pure tree functions (children of a folder, recursive file count, total size, search). The existing
   `fileManager.test.tsx` and `storageObjects.test.ts` keep passing.
3. **Relative dates** use `relativeDate`/`useRelativeDate` from `components/layout/format.ts`, added by the Settings PR
   (spec 9). If that PR is not merged first, this PR adds it with its test.
4. **`targetLabel`** of `SelectFileButton` is passed by the email builder's image block only.
5. **Templates loaded at once**: T1 is `templates.list` with `channel=email` and no category, filtered in
   the browser; the category tab and `q` stay in the URL (`replace`, 300 ms debounce for `q`).
6. **Controlled drawers**: `CreateTemplateDrawer` and `TemplatePreviewDrawer` gain `open`/`onClose`; their
   current uncontrolled use (newsletters, transactional, automations) is unchanged and tested as such.
   Every page call passes `canManageSavedBlocks={!!permissions?.templates?.write}`.
7. **Strings**: Lingui everywhere; `.po` extraction and the 7 translations last, after the rebase on
   `dev`.

## Files and tests (TDD: tests first)

| File | Change | Test file and cases |
|---|---|---|
| `services/api/template.ts` | `usage()` and its types (`TemplateUsage`, `TemplateUsageCounts`, `TemplateUsageReference`, `hidden_types`) | type check; covered by page tests |
| `services/api/blog.ts` | `counts()`, `scheduled` status, `search` | type check |
| `services/api/permissions.ts` | `/api/templates.usage` under `templates.read`, `/api/blogPosts.counts` under `blog.read` | `permissions.test.ts`: both endpoints listed under their right |
| `components/templates/templateCategories.ts` (new) | `msg` table of the server categories, in order | covered by `TemplatesPage.test.tsx` and `CreateTemplateDrawer.test.tsx` |
| `components/templates/templateUsage.ts` (new) | usage summary: one type → "{n} newsletter…", several → "{total} uses" with per-type lines, zero → not used, hidden types flag | `templateUsage.test.ts`: each type alone, several types, zero, `hidden_types` |
| `components/templates/importTemplateFile.ts` (new), `ImportErrorsModal.tsx` (new) | read and validate a `.json` export or `.mjml` file, out of `ImportExportButton.tsx` | `importTemplateFile.test.ts`: valid JSON, invalid JSON (current errors), display-condition warning, MJML converted to a visual tree; `ImportExportButton.test.tsx` passes unchanged |
| `components/templates/CreateTemplateDrawer.tsx` | controlled mode, `initialTree`, `initialTestData`, category labels from `templateCategories.ts` | `CreateTemplateDrawer.test.tsx`: controlled open/close without own button; imported tree shown, nothing saved before Save |
| `components/templates/TemplatePreviewDrawer.tsx` | controlled mode | `TemplatePreviewDrawer.test.tsx`: controlled mode; current mode unchanged |
| `pages/TemplatesPage.tsx` | header with Import and New template, category `CountTabs`, name-or-subject search, table, row menu, delete confirmation with usages, `preview` in the URL | `TemplatesPage.test.tsx` (new): tab counts; category from the URL at 0; search on the subject; integration icon and tooltip; usage summary (one type, several, none, hidden type); row click → Edit with `templates.write`, Preview without; menu disabled without right and Delete disabled for an integration template; confirmation with and without usages, hidden type, T3 error; unknown `preview` and deleted template; Import opens the create drawer named after the file; `canManageSavedBlocks` follows `templates.write` |
| `router.tsx` | `preview` in `TemplatesSearch`; exported normalizers for `category`, `q`, `preview`, `status`, `category_id`, `path`, `file` | `router.test.ts`: each normalizer with a number, a boolean, an empty value, a value outside the allow-list |
| `pages/BlogPage.tsx`, `components/blog/BlogSidebar.tsx` | `RailLayout`, categories with counts, category actions under `blog.write`, header with domain link, Theme and New post | `BlogPage.test.tsx` (new): counts; "All posts" keeps `all_total` inside a category; category menu and button follow `blog.write`; empty state without category; domain link (custom endpoint, else API endpoint; category path inside a category). `BlogSidebar.test.tsx` updated |
| `components/blog/PostsTable.tsx` | status `CountTabs` with counts, title search, columns (Post, Category, Status, Author, Published, Views 30d), row menu, Load more | `PostsTable.test.tsx`: scheduled status; SEO alert; slug and thumbnail in the title tooltip; Category hidden inside a category; author and "+n"; reading time; Views column hidden without web analytics or without `web_analytics.read`; "Open on web" absent for a scheduled post; without `workspace.write` no row click nor menu; Load more appends the next page |
| `components/file_manager/storageTree.ts` (new) | pure tree functions | `storageTree.test.ts`: implicit folder, recursive count, markers not counted, total size, objects of a folder, search by name |
| `components/file_manager/useStorage.ts` (new) | query and mutations | `useStorage.test.ts` (mocked S3 client): pages read until the last (`toHaveBeenCalledTimes`), stop at an injected cap with `isCapped`, invalidation after an upload, folder delete through `deleteFolder` with partial failure counted |
| `components/file_manager/FileExplorer.tsx`, `FileGrid.tsx`, `FilePreview.tsx` (new), `fileManager.tsx` (relay) | explorer in `page` and `picker` modes | `FileExplorer.test.tsx`: tree and counts, Refresh, view kept in `localStorage`, selection bar, read-only, preview from `file`, menus, back to the parent after a folder delete; picker: refused file greyed out, footer with and without `targetLabel`, "Insert image" disabled without a choice, double click inserts, Delete folder present |
| `components/file_manager/context.tsx` | picker modal, `targetLabel` on `SelectFileButton` | `context.test.tsx` (new): title by type, large-file warning, `onSelect` called on insert only |
| `components/email_builder/ui/FileSrc.tsx` | passes `targetLabel` | covered by `context.test.tsx` |
| `pages/FileManagerPage.tsx` | layout, `file` in the URL | `FileManagerPage.test.tsx` (new): not configured panel, read error with Retry |
| `e2e/protected-pages.spec.ts`, `e2e/features/templates.spec.ts`, `blog.spec.ts` | new titles, row menus | the tests themselves |
| `src/i18n/locales/*.po` | extraction, translations of new and renamed strings in 7 catalogues | `catalogues.test.ts` |
| `CHANGELOG.md` | one Feature line for the screens | — |

## Verification

1. `cd console && npx vitest run <touched files>` while working, then `npx vitest run` once.
2. `npm run lint`, `npx tsc -b`, `npm run build`.
3. `npm run lingui:extract`, translations, `npx vitest run src/i18n`.
4. `npm run test:e2e -- --trace off e2e/protected-pages.spec.ts e2e/features/templates.spec.ts e2e/features/blog.spec.ts`.
5. Screenshots of templates, blog, file manager and picker at 1440 and 1280 px, dev server on port 19173,
   stopped afterwards.

## Spec review (Opus, effort xhigh) and what the plan takes from it

All findings below were checked in the code and are confirmed; each changes the plan as stated.

1. `fileManager.test.tsx` renders `FileManager` without a QueryClient and drives the folder delete through
   the old toolbar Popconfirm: its four cases (second confirmation ignored, back to the parent, stay where
   the user went, no navigation after unmount) move to `FileExplorer.test.tsx` with a provider; the guards of
   `fileManager.tsx` (`deletingFolderRef`, `mountedRef`, the current-path check) are kept.
2. `useStorage` needs the workspace for its key: `workspaceId` is added to `FileManagerProviderProps` and
   `FileManagerProps` (`layouts/WorkspaceLayout.tsx` passes it). The query caches the raw `_Object[]` and maps
   them with `toStorageItems` in a `useMemo` (file URLs depend on `cdn_endpoint`); the key is invalidated
   after `onUpdateSettings`. `cutShort` is exposed next to `isCapped` (same warning).
3. The folder-delete count comes from a `listAllObjects(prefix)` read when the confirmation opens
   ("… and its files?" while it loads). Deletes stay sequential, as `deleteFolder` and the bulk delete are
   today (the spec's "5 at a time" does not match the code).
4. File manager layout: `/file-manager` gets `staticData: { layout: 'rail' }` and `__tests__/routeLayout.test.ts`
   allows it. The folder tree is a component of `FileExplorer` (`FolderTree`), not `RailItem`s: it needs
   unlimited depth, chevrons and works inside the picker modal, where `RailLayout`'s sticky 100vh rail
   cannot go. The page wraps it in `RailLayout`; the picker in a plain bordered column.
5. The blog already uses `RailLayout`/`RailItem` (#531): remaining work is counts (B1), the category menu and
   the `blog.write` gate passed to `BlogSidebar` as a prop; the `limit: 1` probe of `BlogPage` is replaced by
   B1 `all_total`.
6. `blogPosts.list` returns no `has_next_page`: "Load more" shows while loaded < `total_count`. The pagination
   tests of `PostsTable.test.tsx` are rewritten and `counts` added to its API mock. `blog.ts` encodes the
   query parameters (`encodeURIComponent`), with a test for "A&B" and "50%".
7. Invalidation: B1 lives under the `['blog-posts', ws, 'counts', …]` prefix so every existing invalidation of
   `['blog-posts', ws]` (PostDrawer, PublishModal, PostsTable) refreshes it; `CategoryDrawer` also invalidates
   `['blog-posts', ws]`. After a template delete, T1 and T2 are invalidated.
8. Controlled drawers: `CreateTemplateDrawer` already has an after-close `onClose` used by
   `TemplateSelectorInput`; the controlled mode uses a new `open` prop with the existing `onClose`, fills the
   form when `open` turns true (the `showDrawer` path, `freshenFromServer` included), and takes `initialName`
   (sets the name and the derived ID) for an import. `TemplatePreviewDrawer` makes `children` optional and
   gains `open`/`onClose`; its six other callers stay unchanged.
9. The server has a ninth category, `blog`: `templateCategories.ts` includes it (allow-list and tab), and the
   drawer keeps its current options so `e2e/features/templates.spec.ts` keeps its count.
10. `FileSrc` gets a `targetLabel` prop set only by `MjImageBlock` (not by the background picker).
11. e2e: `e2e/fixtures/template-editor.ts` (row edit icon) is updated and the specs using it run; the e2e
    API mock answers `templates.usage` and `blogPosts.counts`, and the console guards against missing
    `usage`, `hidden_types` and `by_category`.
12. Usage response: count keys are plural, reference types singular, an unreadable type is absent (never 0),
    references are capped at 50 per type: "and {n} more" is computed from the `usage` counts, with a test.
13. Coverage added to the tests: B3 "Views 30d" (`analytics.query` on `web_pages`, `page_count` by `page_path`,
    `inDateRange` rounded to the hour, live posts only, one query per loaded page, key
    `['blog-views-30d', ws, from, paths]`); states (T1 error with Retry, T2 error "—", B1 loading and error
    without zeros, B2 first and next page errors, B3 error, capped alert, empty folder, search without
    result); picker (upload auto-selected, reopens at the last folder, Enter inserts), J/K/Esc in the
    preview, dimension cache; templates table without pagination; "No scheduled posts", "No posts match
    your search", Copy ID, the Send test rule. The docs PR (Views 30d theme script, scheduled posts,
    OpenAPI copy) is a separate PR on `Notifuse/docs` from `dev`.
14. Line numbers of the spec moved (ImportExportButton, fileManager, PostsTable); the plan follows the code.
15. `__tests__/pages.smoke.test.tsx` mocks `../router`: normalizers live in `lib/searchParams.ts`, not in
    `router.tsx`, so pages never import them from the router.
