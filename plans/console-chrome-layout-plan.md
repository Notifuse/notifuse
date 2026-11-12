# Console chrome and layout components (spec 2 of the "Nuit claire" redesign)

Source spec: "Spec 2 · Chrome et composants de mise en page" (rewritten on 12 November 2026: no top bar).
Depends on `plans/console-theme-tokens-plan.md` (spec 1). Branch `feat/console-chrome-layout`, created from
`feat/console-theme-tokens`, PR to `dev` (merged after the spec 1 PR).

Decisions taken by Pierre that change the spec text:
- **This PR ships every shared component** (`Panel`, `KpiStrip` with `connector` and `aside`, `Pill`,
  `FullDrawer`, `SidePanel`, `RateBar`, `KeyValueList`, `CountTabs`, `TableToolbar`) with its tests, not only
  the ones the migration uses. The four screen threads branch from this PR.
- The sign-in wallpaper commit `5e1c765` (branch `feat/signin-wallpaper`) is cherry-picked into this PR.
- Gravatar kept, initials as fallback. Sidebar over the rail on `rail` pages, without pushing the page.

No backend, database or environment change.

## A. Shell

### A1. `layouts/app-shell/menuItems.tsx` (new)

Pure functions extracted from `WorkspaceLayout.tsx`, same entries, order, rights and behaviour:
- `selectedMenuKey(pathname)`: the current `includes` cascade, order included (`/settings` before
  `/web-analytics`; unknown web-analytics section → `web-analytics-dashboard`; default `analytics`).
- `activeGroup(pathname)`: `null` under `/settings`, else `web-analytics` or `content` by fragment.
- `firstPageOfGroup(group, hasAccess)`: `web-analytics` → dashboard tab; `content` → templates if allowed,
  else blog (the first child the group renders), else `null`.
- `buildMenuItems({ hasAccess, workspaceId, t })` → antd `MenuProps['items']`: labels are `Link`s, icons are
  lucide (15 px, `strokeWidth 1.75`): `LayoutGrid`, `ChartLine`, `SquareUserRound`, `FolderOpen`, `Send`,
  `Workflow`, `SquareTerminal`, `Files`, `AlignJustify`, `Settings`. Content dropped when it has no child.
  Children without icons (as today). `t` is passed in so the function stays pure and testable.

Test `menuItems.test.ts`: each path of today's cascade gives the same key (table-driven, including
`/settings/web-analytics` → `settings`, `/web-analytics/unknown` → `web-analytics-dashboard`,
`/lists` → `lists`, `/blog` → `blog`); an entry without right is absent; Content disappears without any
child and keeps Blog + File Manager without templates; `activeGroup`; `firstPageOfGroup` with and without
templates access.

### A2. `layouts/app-shell/gravatar.ts` (new)

`getGravatarUrl(email, size)` moved from `WorkspaceLayout.tsx` (md5 of trimmed lower-cased email,
`d=identicon`), `initialsFor(user)`: first letters of the first two words of `name`, else the first two
characters of the email, upper-cased, `?` if neither.
Test `gravatar.test.ts`: hash of a known email, trimming/case, `''` without email; initials from name, from
one-word name, from email, empty.

### A3. `components/brand/NotifuseLogo.tsx` (new)

`NotifuseLogo` and `NotifuseMark` SVGs from the mock-up, `role="img"`, `aria-label="Notifuse"`, colours
from `tokens.ts` (`gray[700]` ink, `brandPink` dot). Covered by `AppSidebar.test.tsx`.

### A4. `layouts/app-shell/useSidebarCollapsed.ts` (new)

`localStorage` key `notifuse.sidebar.collapsed`, read and written inside `try`, default expanded.
Tested through `AppSidebar.test.tsx` (including a throwing `localStorage`).

### A5. `layouts/app-shell/WorkspaceSwitcher.tsx`, `HelpMenu.tsx`, `AccountMenu.tsx` (new)

- `WorkspaceSwitcher` (`workspaces`, `workspaceId`, `isRoot`, `collapsed`, `onSelect`): 36 px button,
  gradient badge (`bg-linear-135 from-primary-500 to-brand-pink`, initial in white) or the workspace logo
  (`settings.logo_url`, 24 px, `object-contain`, `bg-surface border-line`), name truncated, `ChevronsUpDown`.
  Collapsed: badge only, name in a `Tooltip`, menu placed `rightTop`. Dropdown (click) with one item per
  workspace (18 px badge + name), current one selected, then divider + "New workspace" (`Plus`) for root
  only, never disabled; `max-height: 360px` + scroll via `overlayStyle`/menu style.
- `HelpMenu` (`collapsed`): full-width 34 px text button `CircleHelp` + "Help"; dropdown opened `topLeft`:
  Documentation (`BookOpen`, https://docs.notifuse.com/), Report an issue (`Bug`,
  https://github.com/notifuse/notifuse/issues), external links `target=_blank rel=noopener noreferrer`.
  Collapsed: icon only, "Help" tooltip and `aria-label`.
- `AccountMenu` (`user`, `role`, `collapsed`, `onSignout`): avatar 26 px (`Avatar` with Gravatar `src`
  size 52 and initials as children: antd shows children while the image loads or fails), email (600) and
  "{role} · {LOCALE}"; dropdown opened `topLeft`: disabled email header, "Language" submenu built from the
  exported `LANGUAGES`/`changeLanguage` (selected = current locale), divider, "Logout" (`LogOut`) calling
  `signout`, divider, disabled `Notifuse v{window.VERSION}` in `font-mono text-overline text-ink-3`.
  Collapsed: avatar only, email tooltip and `aria-label`.

### A6. `layouts/app-shell/AppSidebar.tsx` (new)

Props: `layout: 'page' | 'rail'`, `menuItems`, `selectedKey`, `openKeys`, `onOpenChange`, workspace and
account props. `<aside>` `fixed left-0`, `top: withBannerOffset('0px')`, `height: minusBannerOffset('100vh')`,
`bg-surface border-r border-line flex flex-col`, `w-sidebar` / `w-sidebar-collapsed`,
`motion-safe:transition-[width] motion-safe:duration-180`, `z-20`.
- Collapsed state = `layout === 'rail' ? !peek : stored`. `peek` (temporary expansion over the rail) lives here.
- Header `h-sidebar-head pl-5 pr-3.5`: `Link` to the workspace dashboard wrapping `NotifuseLogo` (22 px);
  collapse button (`PanelLeft`, `aria-label` "Collapse sidebar", `aria-expanded`). Collapsed: `NotifuseMark`
  24 px centred (still the dashboard link).
- `WorkspaceSwitcher`, then `<nav aria-label="Main navigation" className="app-sidebar-nav">` with the antd
  `Menu` (`mode="inline"`, `inlineCollapsed={collapsed}`, `openKeys` only when expanded, items empty while
  rights load), then the footer (`border-t border-line-soft px-2.5 pt-2 pb-2.5`): `HelpMenu`, `AccountMenu`,
  and when collapsed an expand button (`PanelLeft`, "Expand sidebar", `aria-expanded=false`).
- Expand button on `page`: writes `stored=false`. On `rail`: sets `peek=true`, nothing written; the aside
  gets `shadow-popup`. Peek closes on Escape (focus back to the expand button), on `mousedown` outside the
  aside (ignoring clicks inside antd popups opened from the sidebar, detected by `.ant-dropdown`,
  `.ant-menu-submenu-popup`, `.ant-tooltip` ancestors), on navigation (pathname change from `useLocation`),
  and on the header collapse button.
- Exposes nothing else; the content offset is computed by `WorkspaceLayout` from `collapsed` *ignoring peek*.

Test `AppSidebar.test.tsx` (router hooks mocked): collapse writes `localStorage`; a throwing
`localStorage` still renders (expanded); `aria-expanded` follows; in `rail` it renders collapsed even if the
stored value is expanded; expand in `rail` shows labels without writing storage, closes on Escape (focus
returns to the button), outside click, pathname change, and header button; collapsed, workspace/help/account
are reachable by their accessible names; workspace selection calls `onSelect`; "New workspace" only for
root; logo replaces the badge; Gravatar `src` and initials fallback (name then email); role line
"Owner · EN", "Member", "Root"; language change from the account menu calls `changeLanguage`; version item
present; Logout calls `signout`; logo has `aria-label="Notifuse"`.

### A7. `layouts/WorkspaceLayout.tsx` (rewritten)

Keeps: `ContactsCsvUploadProvider` around everything; rights loading (root → full; member → its
`permissions` merged on empty; also keeps the member's `role` for the account line); empty menu while
loading; `FileManagerProvider` with the same `key`, `readOnly`, `settings`, `onUpdateSettings` →
`handleUpdateWorkspaceSettings`; `openKeys` effect and `handleOpenChange` (open group → navigate to
`firstPageOfGroup`, not when collapsed); workspace switch navigation and `create` route.
New: reads the layout of the deepest match declaring `staticData.layout` (`useMatches()`), default `page`;
renders `<div className="min-h-screen bg-canvas">`, `AppSidebar`, and
`<main className={ml-sidebar | ml-sidebar-collapsed, motion-safe transition}>` with
`padding-top: var(--license-banner-height, 0px)` (via `withBannerOffset('0px')`), inner wrapper with
`pt-4 px-5 pb-6` on `page`, none on `rail`. Removed: `Sider`, `Header`, `Select`, `#F9F9F9`, `isSettingsPage`,
`padding: 24px`, `marginTop: withBannerOffset('64px')`, the FontAwesome/antd icon imports.

Test `__tests__/WorkspaceLayout.test.tsx` (adapted: `useMatches` mock returns `staticData`, `useLocation`
mocked, `LanguageSwitcher` mock exports `LANGUAGES`/`changeLanguage`): existing submenu and web-analytics
cases pass; `main` has `pt-4 px-5 pb-6` on `page` and no padding class on `rail`; sidebar collapsed on
`rail` with stored expanded; banner variable used for sidebar `top` and `main` padding; no `#F9F9F9` in the
rendered HTML; menu empty while rights load.

### A8. `contexts/AuthContext.tsx`, `services/api/auth.ts`

`name?: string` on `User` (and on the API type if separate). Covered by `AppSidebar.test.tsx`.

### A9. `components/LanguageSwitcher.tsx`

Export `LANGUAGES` (the `locales`/`localeNames` pairs, from `i18n`) and `changeLanguage(setLocale, l)`
(set locale + fire-and-forget `authService.updateLanguage`). Component rendering unchanged and uses them.
Test: `src/__tests__/i18n.test.tsx` gets one case: `changeLanguage` calls `setLocale` and
`authService.updateLanguage` and survives a rejected save.

### A10. `router.tsx`

`declare module '@tanstack/react-router' { interface StaticDataRouteOption { layout?: 'page' | 'rail' } }`;
`staticData: { layout: 'rail' }` on `/settings`, `/settings/$section` and `/blog`.
Test `src/__tests__/routeLayout.test.ts` (new): walks `router.routeTree` children of the workspace route:
those three declare `rail`, all others declare nothing.

## B. Layout components (`components/layout/`, exported by `index.ts`)

All in Tailwind with spec 1 tokens, no literal colour or pixel size (bracket values only for widths specific
to a component, e.g. 300 px search, 140 px label column), texts through Lingui. One test file per component.

| Component | Test cases |
|---|---|
| `format.ts` — `useNumberFormat()` returns `int`, `percent(ratio, digits=1)`, `points(delta)`, `compact` built on `Intl.NumberFormat(locale)`; `points` uses Lingui `plural` for "pt"/"pts" and `+`/`−` (U+2212) signs | fr: 18342 → "18 342" (U+202F), 0.493 → "49,3 %", 2.1 → "+2,1 pts", −0.4 → "−0,4 pt", 2500 compact → "2,5 k"; en: "18,342", "49.3%" |
| `PageHeader` — `title`, `level` (`h1` `text-title` / `h2` `text-subtitle`), `count` capsule formatted by `int`, `status`, `leading`, `description`, `meta` with 3 px dot separators, `actions` (`shrink-0`, `flex-nowrap`), `className`, `min-h-8` | count "1 204" in fr and "1,204" in en; `h1` on page, `h2` on section; actions container is `flex-nowrap shrink-0`, title `truncate min-w-0`; meta separators |
| `RailLayout`, `RailSection`, `RailItem` | title and footer rendered; rail is `sticky` with banner offset; section `action` requires `aria-label` (typed) and renders it; item with `to` is a link with `aria-current="page"` when active; without `to` a `button`; `trailing` is a sibling of the main element (no nested interactive); `depth` indent; `danger` tone; count formatted |
| `SaveBar` (+ `components/settings/SettingsSaveBar.tsx` re-exports it as `SettingsSaveBar`) — behaviour unchanged; new rendering `sticky bottom-3`, `bg-ink text-white rounded-lg shadow-popup pl-4 pr-2 py-2`, "Unsaved changes", "Discard" (ghost), "Save" with tooltip "⌘S"/"Ctrl+S" | the cases of `SettingsSaveBar.test.tsx` moved to `SaveBar.test.tsx` with the new labels; platform tooltip |
| `Mono` — `<code>` `font-mono text-xs text-ink-2 bg-fill rounded-sm px-1.5 truncate`, `title`; `copy` adds a 12 px `Copy` button ("Copy", message "Copied") | copies with `navigator.clipboard`, then with the fallback when absent |
| `Panel` — `title`, `icon`, `extra`, `flush`, `footer`, `className` | header only with title; `flush` body without padding; footer rendered |
| `KpiStrip` — `items` (`key`, `label`, `value`, `sub`, `tone`, `trend`, `hint`, `onClick`, `active`), `connector='arrow'`, `aside`, `loading`, `embedded`; columns from a `ResizeObserver` (N if each cell ≥ 140 px, else `ceil(N/2)`), last cell spans the rest when wrapped, arrows hidden and aside full width when wrapped | N columns when wide; wraps under 140 px/cell with last cell span; clickable cell is a button with `aria-pressed`; arrows rendered between cells and hidden when wrapped; `loading` shows skeletons; `embedded` drops border/shadow; tone classes |
| `Pill` — `tone`, `dot` (default true), `pulse`, `icon` | tone bg/fg classes (`primary` → `primary-100`/`primary-700`); `pulse` uses `motion-safe:animate-pill-pulse`; `icon` replaces the dot |
| `RateBar` — `rate`, `scale`, `tone` (`default`/`partial`/`warning`), value through `percent` | width = rate/scale capped at 100 %; `null` → "—" without bar; tones |
| `KeyValueList` — `items`, `labelWidth=140` | empty value "—"; grid template uses the width |
| `CountTabs` — antd `Segmented`, label + tabular count | counts formatted; `onChange` called with the value |
| `TableToolbar` — `start`, `end` | both slots rendered |
| `FullDrawer` — antd `Drawer` `placement=right`, full width, no antd header/close, `rootClassName="app-full-drawer"`, `destroyOnHidden`; 40 px sticky nav bar with back, prev/next (`ChevronUp`/`ChevronDown`, tooltips "Previous (K)"/"Next (J)"), "4 of 12" + `context`, `topActions`, copy link (`Link2`, "Link copied"), close (`X`, "Close"); J/K handler | back and close call `onClose`; prev/next disabled without handler; position text only with `position`+`total`; J/K call next/prev, ignored in inputs/contenteditable, with modifiers, and when another modal/drawer is open on top; copy link uses `copyToClipboard(window.location.href)` |
| `SidePanel` — antd `Drawer` width `layout.sidepanel`, no header, `mask={{ blur: false }}`; header with title/status/meta/actions/nav/copy/close, optional `tabs`, scrolling body, optional footer; same J/K and copy rules (shared hook `useDrawerNavKeys`) | same nav/J-K cases as `FullDrawer`; tabs rendered |

`lib/copyToClipboard.ts` (new): extracted from `CodeSnippet.tsx`; `CodeSnippet` uses it.
Test `lib/copyToClipboard.test.ts`: with `navigator.clipboard`; without (fallback `execCommand`); fallback
failing returns `false`; existing `CodeSnippet.test.tsx` passes unchanged.

Theme additions: `Menu.collapsedWidth: 56` in `antdTheme.ts`; `Layout` tokens `headerHeight`, `headerPadding`,
`siderBg` removed; `--animate-pill-pulse` + `@keyframes pill-pulse` in the `@theme` block (tokens contract
test updated: animation listed in `tokens.ts` → `motion.pillPulse`).
Global rules added to `theme/antd-overrides.css`: active bar hidden when `.ant-menu-inline-collapsed`; icon
colour (`ink-3`, active `primary-500`) and label weight (500) of `.app-sidebar-nav .ant-menu`; collapsed
gutter; `.app-full-drawer .ant-drawer-section`/body background `var(--ant-color-bg-layout)`.
`src/index.css`: `.workspace-sider*` rules removed, `.workspace-sider-nav` and its keyframes renamed
`.app-sidebar-nav`. `antdOverrides.test.ts`: allow-list `.app-sidebar-nav`, `.app-full-drawer` instead of
`.workspace-sider`; no `.workspace-sider` left in `src/`. `antdTheme.test.ts`: `Menu.collapsedWidth` is 56,
removed Layout tokens absent.

## C. Page migration

Pages on `page` lose their own margin (`p-6`, `p-4 md:p-6`) and their root becomes `flex flex-col gap-3`; the
title row becomes a `PageHeader` with the existing controls in `actions`. Nothing else changes.

| Page | Change |
|---|---|
| `AnalyticsPage` | `PageHeader` "Dashboard", controls in `actions` |
| `WebAnalyticsPage` | title of the tab, `LiveButton` in `status`, pickers in `actions` |
| `WebAnalyticsLivePage` | title "Live", `leading` = back link to Dashboard (text button `ArrowLeft`), `status` = `Pill success pulse` "N live now" |
| `ContactsPage`, `ListsPage`, `BroadcastsPage`, `AutomationsPage`, `TransactionalNotificationsPage`, `LogsPage`, `TemplatesPage`, `FileManagerPage` | `PageHeader` with the same title and actions |
| `WorkspaceSettingsPage` + `SettingsSidebar` | `RailLayout title="Settings"`; `SettingsSidebar` renders `RailItem`s (`onClick`) with the same entries, order, antd icons and rights; its title and `Divider` removed; content keeps `max-w-[700px]` |
| `BlogPage` + `BlogSidebar` | `RailLayout` (no title); "All Posts" item, `RailSection title="Categories" action=New Category (aria-label)`, category items with `MissingMetaTagsWarning` in the label and Edit/Delete in `trailing` |
| `SettingsSectionHeader` | `level?: 'page' \| 'section'` (`page` → `PageHeader` title+description; `section` → `PageHeader level="section"` with `border-t border-line-soft pt-4`), `Divider` removed, `className` on the container; the five non-first calls (in `BlogSettings`, `GeneralSettings`, `LicenseSettings`, `WebAnalyticsSettings`) pass `level="section"` |

Tests: `SettingsSectionHeader.test.tsx` (new): `page` → `h1`, `section` → `h2` with border, `className`
passed. Existing tests (`SettingsSidebar.test.tsx`, `ContactsPage.test.tsx`, `LogsPage.test.tsx`,
`pages.smoke.test.tsx`, `BlogSettings.test.tsx`…) pass unchanged, except assertions targeting the
`SettingsSidebar` `Menu`, rewritten on `RailItem`s with the same expectations.

E2E (`e2e/`): `templates.spec.ts` (l. 42), `workspace-settings.spec.ts` (l. 292, 305, 308-332),
`blog.spec.ts` (l. 203-212), `protected-pages.spec.ts` (l. 170): `.ant-layout-content` → `main`,
`.ant-layout-sider-dark` → `nav[aria-label="Main navigation"]`, settings `.ant-menu` → rail links/buttons.

## D. Sign-in wallpaper

Cherry-pick `5e1c765` from `origin/feat/signin-wallpaper` (photo asset, credit in `MainLayout.tsx`, 8
catalogues, CHANGELOG line merged into this PR's bullet).

## E. Translations and CHANGELOG (last, after rebase on `dev`)

`npm run lingui:extract`, then translate every new string in the 7 non-English catalogues (Collapse
sidebar, Expand sidebar, Main navigation, Help, Report an issue, Language, Owner, Member, Root, New workspace,
Unsaved changes, Save, Discard, Copy, Copied, Link copied, Previous (K), Next (J), Close, `{0} of {1}`, Live,
`{0} live now`, Settings, Categories…). CHANGELOG `[44.0]`: `- **Feature**: The console has a new navigation:
a lighter sidebar that folds to icons, that also holds the workspace switcher, help and account menus; the
top bar is gone.` merged with the wallpaper line.

## F. Verification

Playwright screenshots at 1440 and 1280 px on the dev server with mocked API: each menu page, sidebar
expanded and collapsed, Settings and Blog with their rail, the sidebar peeking over the rail, licence banner
shown, an existing full-width drawer, workspace menu with more than 10 workspaces. Compared with
`sans-topbar/131-tableau-de-bord-sans-topbar.png`.

## Known, not treated

- `DebugSegmentPage` (debug route, not in the menu) keeps its own layout.
- The six existing full-width drawers keep their antd header until their screen spec.
- `LanguageSwitcher` component: its only caller was the old top bar; kept (its rendering unchanged, still tested by `i18n.test.tsx`) so this PR removes no function.

## Spec review amendments (Opus, xhigh — findings verified in the code)

These override the sections above where they differ.

1. **Menu labels and Lingui.** `t` cannot be passed into a module (the macro only rewrites `t` from
   `useLingui()` in the same scope). `menuItems.tsx` declares its labels with `msg` from
   `@lingui/core/macro` and `buildMenuItems` takes `i18n` (from `useLingui()`), calling `i18n._(LABELS.x)`.
   `i18nMacro.test.ts` must stay green.
2. **SettingsSectionHeader levels.** The first call of `BlogSettings`, `GeneralSettings`,
   `WebAnalyticsSettings`, `LicenseSettings` is the read-only branch and the second the editable branch of
   the same page title: both stay `page`. Only `BlogSettings.tsx:417` ("RSS / Feeds") becomes
   `level="section"`, and the `<Divider className="!my-8"/>` just above it is removed (the header draws
   its own `border-t`).
3. **E2E remapping.** `.ant-layout-sider-dark` is the Settings page's own Sider (dark default theme), not
   the main nav. `RailLayout` renders its rail as `<nav aria-label={title ?? ariaLabel}>` (Blog passes
   `ariaLabel="Blog"`) and its content as a labelled region. In `workspace-settings.spec.ts`: lines 18, 31,
   35, 39, 123, 157, 292, 305-332 remapped to `getByRole('navigation', { name: 'Settings' })`, its buttons
   by exact name, `aria-current="page"` instead of `ant-menu-item-selected`, and content assertions scoped to
   the content region; `protected-pages.spec.ts:170` and `templates.spec.ts:42`, `blog.spec.ts:203-212`
   checked one by one.
4. **Collapse state owned by `WorkspaceLayout`.** `stored` (localStorage) and `peek` live in
   `WorkspaceLayout`; it derives `menuCollapsed = rail ? !peek : stored` (drives `inlineCollapsed`, whether
   `openKeys` is passed, the openKeys effect deps `[activeGroup, menuCollapsed]`, and the
   `handleOpenChange` guard) and `offsetCollapsed = rail || stored` (main margin). `AppSidebar` receives
   `collapsed`, `peeking`, `onToggle`, `onPeekChange`. Tests in `WorkspaceLayout.test`: on `/blog` the peek
   shows Content open with Blog selected; the content keeps `ml-sidebar-collapsed` while peeking.
5. **Existing tests updated, not "unchanged"**: `GeneralSettings.test`, `WebAnalyticsSettings.test`,
   `BlogSettings.test` and `SettingsSaveBar.test` move to the new labels ("Save", "Unsaved changes").
   `WorkspaceLayout.test` mocks `LocaleContext`, honours `useMatches({ select })`, clears `localStorage` in
   `beforeEach`, and throws from `localStorage.getItem` with `vi.mocked(...).mockImplementationOnce`.
   `window.VERSION` keeps a fallback.
6. **Avatar.** antd shows children only after an image error. `AccountMenu` preloads the Gravatar with
   `new Image()` and passes `src` only after `load`; initials show until then and after an error; no `src`
   without email. Tests: initials before load, image after `load`, initials after `error`.
7. **Peek and popups.** The sidebar's Dropdowns and Tooltips render inside the aside
   (`getPopupContainer={() => asideRef.current}`), so "outside" is simply `!aside.contains(target)`.
   Escape while a sidebar dropdown is open closes only the dropdown. Test: during a peek, pick a language
   from the account menu: the peek stays open and `changeLanguage` is called.
8. **Lint**: `LANGUAGES`/`changeLanguage` move to `src/i18n/languages.ts` (a `.ts` module, no
   fast-refresh warning); `useDrawerNavKeys` in its own `.ts`; `format.ts` hook in `.ts`. No new lint warning.
9. **`format.ts`**: `points` formats with `Intl.NumberFormat(locale, { signDisplay: 'exceptZero',
   minimumFractionDigits: 1, maximumFractionDigits: 1 })`, replaces `-` by `−`, and picks "pt"/"pts" with
   `Intl.PluralRules(locale).select(Math.abs(rounded))` mapped onto two Lingui messages (`msg` `{value} pt`
   and `{value} pts`), so French 0.4 is singular. Expected strings written with explicit `\u202f` / `\u00a0`
   (fr integer U+202F; fr percent and compact U+00A0; en compact "2.5K").
10. **Collapsed nav width**: `.app-sidebar-nav` gets `overflow-x: hidden`; a `.is-collapsed` modifier sets
    `scrollbar-gutter: auto` and `.ant-menu-inline-collapsed { width: 100% }`; the aside's right border is
    drawn with an inset shadow so the 56 px stay usable. Verification adds classic scrollbars and a copy
    over http.
11. **Tone classes are literal**: `Record<Tone, string>` maps of complete class strings (Pill, KpiStrip
    `sub`, RateBar), never template-built names.
12. **WebAnalyticsPage**: `actions` holds exactly what the header row holds today
    (`showToolbar && settings && isExplore ? periodControls : null`); the filter bar inside the gate is
    untouched.
13. **Settings content**: the `max-w-[700px]` wrapper becomes `max-w-[700px] flex flex-col gap-3`.
14. **RailItem**: active `button` also gets `aria-current="page"`; `trailingMode?: 'actions' | 'indicator'`
    (`actions` → `opacity-0 group-hover:opacity-100 group-focus-within:opacity-100`, still tabbable);
    Danger Zone uses `tone="danger"`; Blog Edit/Delete buttons get `aria-label`s. New
    `BlogSidebar.test.tsx`: All Posts → `onCategoryChange(null)`, a category → its id, Edit/Delete call only
    their handler, New Category reachable by name, no button inside a button.
15. **Focus**: after each toggle the focus moves to the other toggle button (ref + effect); opening the peek
    focuses the header collapse button; Escape refocuses the expand button. Asserted on `document.activeElement`.
16. **antd APIs**: `SidePanel` `size={layout.sidepanel}`, `FullDrawer` `size="100%"`; workspace menu
    `menu.style = { maxHeight: 360, overflowY: 'auto' }` (no deprecated `overlayStyle`).
17. **Tests added**: `WorkspaceLayout.test` checks that picking a workspace navigates to it and "New
    workspace" to `/console/workspace/create`; page padding asserted on the inner wrapper.
18. **Other own paddings removed**: `AnalyticsPage` not-found branch, `AutomationsPage` error branch,
    `BlogPage` content `padding: 24px`.
19. **Wallpaper**: `git fetch origin feat/signin-wallpaper` before the cherry-pick.
20. **z-index**: the aside uses `z-10` (the licence banner is at 20 and expects the chrome at 10).
21. **Mock-up details**: `inlineIndent={14}`; PageHeader `tracking-[-0.015em]` and `tabular-nums` counts;
    switcher hover `fill` (no `surface-2` token); `Menu.collapsedWidth` from `layout.sidebarCollapsed`.
22. **Pill animation token**: `tokens.ts` gets `motion.pillPulse` (duration) and the contract test pins
    `--animate-pill-pulse` instead of exempting every `--animate-*`.
23. **Strings**: placeholders are named (`{position} of {total}`); `{liveSessions} live now` already exists.
24. **copyToClipboard tests** keep the "clipboard rejects → fallback" case.

## Commands (in `console/`)

1. `npx vitest run src/components/layout src/layouts src/lib src/__tests__/WorkspaceLayout.test.tsx
   src/__tests__/routeLayout.test.ts src/__tests__/i18n.test.tsx src/theme src/components/settings
   src/components/blog` during development.
2. `npx eslint .` (the CI gate; `npm run lint` already fails on `dev` with 12 pre-existing warnings), `npx tsc -b`.
3. `npx vitest run` once at the end.
4. `npm run test:e2e` (or CI), `npm run build`.
