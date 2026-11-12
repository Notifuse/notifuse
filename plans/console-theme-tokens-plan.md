# Console theme tokens (spec 1 of the "Nuit claire" redesign)

Source spec: "Spec 1 · Thème antd et guide de style Tailwind" (validated by Pierre on 11 November 2026).
Base: `origin/dev` at `de243ef` (VERSION 44.0, unreleased). Branch `feat/console-theme-tokens`, PR to `dev`.

## Goal

Define every value of the Nuit claire identity once, in `console/src/theme/tokens.ts`. The antd theme
and the Tailwind `@theme` block derive from it, and a contract test keeps them in sync. After this PR the
console keeps its current layout but takes the colours, typeface and density of Nuit claire. No backend,
database or environment change. No new translatable string.

## Files

### 1. `console/src/theme/tokens.ts` (new)

Exports (all colours 6-digit lowercase hex, as later code appends opacity suffixes):

- `colors`: `canvas #f3f4f7`, `surface #ffffff`, `subtle #f8f9fb`, `fill #f1f2f6`, `line #e1e3ea`,
  `lineSoft #eceef3`, `ink #171923`, `ink2 #545a6e`, `ink3 #666c81`, `ink4 #9095a7`;
  `gray` scale 50..900 (`50 #f8f9fb`, `100 #f1f2f6`, `200 #e1e3ea`, `300 #cdd0da`, `400 #9095a7`,
  `500 #666c81`, `600 #545a6e`, `700 #3d4255`, `800 #262a38`, `900 #171923`);
  `primary` 50/100/300/400/500/600/700 (`#f5f4ff #efedff #c9c4ff #8c82ff #6a5af9 #5b4beb #4b3fd6`);
  `brandPink #e91e63`; `status` (`info #2563eb`, `success #16a34a`, `warning #d97706`, `danger #dc2626`);
  `tones` for Pill: `neutral {bg #f1f2f6, fg #545a6e}`, `primary {bg #efedff, fg #4b3fd6}`,
  `info {#eaf2fe, #1d4ed8}`, `success {#e9f7ef, #15803d}`, `warning {#fff4e5, #b45309}`,
  `danger {#fdecec, #b91c1c}`; `chart` (`primary = primary[500]`, `comparison #b4b8c7`,
  `series2 #0ea5e9`, `series3 #14b8a6`).
- `typography` (`t`): `sans`, `mono` stacks; sizes `overline 10.5`, `xs2 11.5`, `xs 12`, `sm 12.5`,
  `base 13`, `subtitle 15`, `kpi 17`, `title 18` with their line heights.
- `radii`: `xs 3`, `sm 4`, `md 6`, `lg 8`.
- `shadows` (`s`): `card`, `popup` strings, `focus = alpha(primary[500], 0.18)`.
- `motion`: `fast 120`, `slow 180` (ms).
- `layout`: `sidebarHead 48`, `sidebar 228`, `sidebarCollapsed 56`, `rail 220`, `aside 320`, `sidepanel 520`.
- `alpha(hex, opacity)` → `rgba(r, g, b, opacity)`; throws on a non 6-digit hex.
- `contrast(fg, bg)` helper (WCAG relative luminance), exported for the test only… kept in the test file
  instead, so `tokens.ts` holds values only (plus `alpha`).

Test `console/src/theme/tokens.test.ts`:
- every text/background pair reaches 4.5:1: `ink`, `ink2`, `ink3` on `surface`, `subtle`, `canvas`;
  each tone fg on its bg; white on `primary[500]`; `primary[600]` on white; `primary[700]` on `primary[100]`.
- every base colour (walk the object, skip derived `shadows.focus`) matches `/^#[0-9a-f]{6}$/`.
- `alpha('#6a5af9', 0.5)` returns `rgba(106, 90, 249, 0.5)`; `alpha('#fff', 1)` throws.
- `shadows.focus === alpha(colors.primary[500], 0.18)`.

### 2. `console/src/theme/antdTheme.ts` (new)

`export const antdTheme: ThemeConfig` built only from `tokens.ts` — the global tokens and per-component
tokens exactly as listed in the spec (Layout, Menu, Card, Table, Tabs, Modal, Button, Segmented, Tag,
Timeline, Statistic). No hex nor `rgb(` literal in the file (`'transparent'` and `'none'` are allowed).
`motionDurationMid: '0.12s'`, `motionDurationSlow: '0.18s'` derived from `motion` (ms → s string).

Test `console/src/theme/antdTheme.test.ts`:
- global colours equal the tokens (`colorPrimary`, `colorLink`, `colorText…`, `colorBorder…`,
  `colorBgLayout`, `colorBgContainer`, `colorBgElevated`, statuses); sizes (`fontSize 13`, `controlHeight 28`,
  radii, `fontSizeHeading4 18`, `lineHeightHeading4 1.3`, …).
- component tokens spot-checked (`Table.cellPaddingBlock 12`, `Menu.itemSelectedBg primary[100]`,
  `Card.colorBorderSecondary line`, `Layout.bodyBg canvas`).
- the source file (read with `fs`) contains no `#[0-9a-f]{3,8}\b` and no `rgb(`.
- token names: TypeScript (`npx tsc -b`) rejects unknown ones; nothing to assert at runtime.

### 3. `console/src/App.tsx`

Remove the inline `theme` object; pass `antdTheme`. Existing `__tests__/App.test.tsx` must pass unchanged.

### 4. `console/src/index.css`

- `@theme` block replaced with the spec's full block (fonts, gray scale, roles, primary scale, brand pink,
  tones, text sizes with line heights, radii, shadows, structural spacings).
- `:root`: drop `--background`, `--foreground`, `--primary`, `font-family`; `background-color:
  var(--color-canvas)`; `color: var(--color-ink)`; `color-scheme: light`; `line-height: 1.45`; keep
  `font-weight`, `font-synthesis`, `text-rendering`, smoothing.
- `a` / `a:hover`: become `@layer base { a { color: var(--color-primary-600); font-weight: 500;
  text-decoration: inherit } a:hover { color: var(--color-primary-700) } }`. Weight and decoration are kept
  as today (the spec only names the colour; `WorkspaceLayout` relies on the 500 weight of its `Link`s).
  Moving into `@layer base` also stops the rule from beating Tailwind text utilities on anchors.
- `h1 { font-size: 3.2em; line-height: 1.1 }` removed.
- Table hover blue border (`!important`) removed.
- Contacts fixed columns: `#F9F9F9` → `var(--ant-color-bg-container)` (both rules).
- `.ant-tag`, `.ant-alert*`, required-mark and `best-timescore-row` rules moved to `antd-overrides.css`.
- Kept here (scoped by console classes): `.contacts-fixed-col`, `.drawer-body-no-padding`, `.tabs-in-header`,
  `.vertical-tabs`, `.workspace-sider*` (replaced in spec 2).

Test `console/src/theme/tailwindTheme.contract.test.ts`: parse the `@theme { … }` block of `index.css`
(read with `fs`), and assert (a) the expected map built from `tokens.ts` (every colour, text size and
line height, radius, shadow, spacing, font stack) is present with equal values (hex compared lowercase),
(b) the block declares no variable outside that map. Also asserts `:root` no longer declares
`--background`/`--primary`, and that `a {` sits in `@layer base`.

### 5. `console/src/theme/antd-overrides.css` (new), imported in `main.tsx` after `index.css`

Each rule with a comment saying why no token fits; colours via `--ant-*` variables:
- table headers: `font-size: 11.5px; font-weight: 500; line-height: 1.3; padding-block: 7px` on
  `.ant-table-wrapper .ant-table .ant-table-thead > tr > th` and the `.ant-table-middle` / `.ant-table-small`
  variants (antd specificity, no `!important`). Must be checked in the browser: antd's selectors are
  `:where(.css-…)` + the same classes, so equal specificity and later source order (our file loads after
  `reset.css`, antd injects styles at runtime in `<head>`)… if antd's rule wins by order, raise specificity
  with `th.ant-table-cell` rather than `!important`.
- sidebar active item: `.ant-menu-light .ant-menu-item-selected { box-shadow: inset 2px 0 0
  var(--ant-color-primary) }`, and its `::after` hidden. Scoped to inline menus
  (`.ant-menu-inline`) so dropdown menus and the horizontal menus are untouched.
- `.ant-tag { border-color: transparent !important }` and `.ant-alert` left border rules: moved as they are
  (they already use `!important` and are existing rules; the spec's "no new !important" applies to new
  rules), colours now `var(--ant-color-info|success|warning|error)`.
- required-mark asterisk and `.best-timescore-row`: moved unchanged.

### 6. `console/src/main.tsx`

Import `@fontsource-variable/inter/wght.css` (latin subset only if the package exposes a latin-only CSS;
otherwise import the variable CSS, whose `@font-face` rules carry `unicode-range`, so only the latin file is
downloaded by browsers — the build still copies every subset; weight and file names noted in the PR) and
`@fontsource-variable/jetbrains-mono/wght.css` likewise, then `antd/dist/reset.css`, `index.css`,
`theme/antd-overrides.css`. The exact file names are checked after install and the choice noted in the PR.

### 7. `console/src/theme/antdOverrides.test.ts` (new)

- walk every `.css` under `src/` except the allow-list (`theme/antd-overrides.css`,
  `components/email_builder/editor.css`, `components/blog_editor/**`, `components/segment/input.module.css`,
  `identity/**`): none contains `.ant-`.
- in `src/index.css`, every selector containing `.ant-` also contains one of `.contacts-fixed-col`,
  `.drawer-body-no-padding`, `.workspace-sider`, `.tabs-in-header`, `.vertical-tabs`.
- `floating-toolbar.css` has no `.ant-popover-inner`; every `.ant-dropdown` selector in
  `block-actions.css` is preceded by `.block-actions-dropdown`.
- `main.tsx` imports both font packages and `./theme/antd-overrides.css`.

### 8. Blog editor CSS

- `blog_editor/toolbars/floating-toolbar.css`: delete `.ant-popover-inner { border-radius: 8px }`.
- `blog_editor/menus/block-actions/block-actions.css`: prefix the three `.ant-dropdown …` rules with
  `.block-actions-dropdown`; `BlockActionsMenu.tsx` sets `rootClassName="block-actions-dropdown"` on its
  `Dropdown`. Existing `BlockActionsMenu` tests (if any) keep passing.

### 9. Email canvas

Today the global `h1 { font-size: 3.2em; line-height: 1.1 }` and `a { color: #646cff; font-weight: 500;
text-decoration: inherit } a:hover { color: #535bf2 }` reach customer HTML shown in the canvas
(`MjTextBlock` `dangerouslySetInnerHTML`). They move to `email_builder/editor.css` under `.email-canvas`
(`.email-canvas h1 {…}`, `.email-canvas a {…}`, `.email-canvas a:hover {…}`), and `.email-canvas` is put on
the canvas root element (the scroll container of the edit panel that holds the rendered blocks). The colours
stay the old literals on purpose (the canvas must render as before). These literals are in `editor.css`,
which is not scanned by `noHardcodedColors.test.ts`.
Check: `.email-canvas a` (0,1,1) must beat Tailwind preflight (`@layer base`) — it does, being unlayered.
Test: an `EmailBuilder`/`EditPanel` test asserts the canvas root has the `email-canvas` class.

### 10. Hard-coded colours

| File | Change |
|---|---|
| `App.tsx`, `layouts/WorkspaceLayout.tsx` | every `#F9F9F9` removed (Layout bg from `colorBgLayout`, Sider from `surface` through `Layout.lightSiderBg`); the Menu inline `backgroundColor` removed |
| `components/settings/SettingsSidebar.tsx`, `components/blog/BlogSidebar.tsx` | `#F9F9F9` → `colors.surface` |
| `components/automations/nodes/BaseNode.tsx` | `#7763F1` → `colors.primary[500]`, shadow → `alpha(colors.primary[500], 0.3)` |
| `components/broadcasts/BroadcastLinkStats.tsx` | `BAR_FILL`/`BAR_TRACK` → `alpha(primary[500], .5/.12)` |
| `components/web_analytics/CountryMapView.tsx`, `TrafficHeatmapWidget.tsx` | ramps → `[0.2,0.4,0.6,0.8,1].map(o => alpha(primary[500], o))` |
| `components/web_analytics/lib/types.ts` | `PRIMARY_COLOR = colors.chart.primary`, `COMPARISON_COLOR = colors.chart.comparison` |

Untouched on purpose (customer values): `components/blog/themePresets.ts`, `components/templates/email-template.json`,
`AnnotationFormModal.tsx` colour choices.

Test `console/src/theme/noHardcodedColors.test.ts`: none of the files of the table matches
`/#7763f1|rgba\(\s*119\s*,\s*99\s*,\s*241|#f9f9f9/i`; `PRIMARY_COLOR === colors.primary[500]`;
`COMPARISON_COLOR === colors.chart.comparison`. Existing `__tests__/App.test.tsx`,
`__tests__/WorkspaceLayout.test.tsx`, `MetricChart.test.tsx` pass with unchanged assertions.

Known, not treated (listed in the PR): the email builder chrome added by the saved-blocks PRs uses
`#7763F1` in Tailwind brackets (`LibraryDnd`, `SavedBlockCard`, `EmailBuilder`, `TreePanel`, `Preview`,
`CanvasDropIndicator`, `editor.css`); the spec's list does not name them and their tests assert those
class names. They stay for the email editor spec.

### 11. `src/App.css`

Deleted (imported nowhere; checked with grep).

### 12. `console/package.json`

Add `@fontsource-variable/inter` and `@fontsource-variable/jetbrains-mono` (5.x, OFL), `package-lock.json`
updated by `npm install`.

### 13. `CHANGELOG.md` (last, after rebase on dev)

Under `[44.0] - unreleased`: `- **Feature**: The console has a new, denser look: the Inter typeface, new
colors and tighter spacing.` (Merged with the spec 2 line if both land in the same version.)

## Verification

Visual check, 1440 px, before/after screenshots with Playwright on the dev build with mocked API data:
Contacts, Broadcasts, Automations (list + flow editor), Web analytics, Settings, Logs, a drawer, a modal, the
email editor canvas and the blog editor. Nothing overflows, no truncated text, links visible, both editors
render as before.

## Commands (in `console/`)

1. `npx vitest run src/theme` during development.
2. `npx eslint .` (the CI gate; `npm run lint` already fails on `dev` with 12 pre-existing warnings) and `npx tsc -b`.
3. `npx vitest run` once at the end.
4. `npm run build`, then check the font files in `dist/`.
5. `npm run test:e2e` if the environment allows (CI otherwise).

## Spec review amendments (Opus, xhigh — findings verified in the code)

These override the sections above where they differ.

1. **`--primary` / `--background` consumers.** 17 uses of `var(--primary)`/`var(--background)` in web
   analytics components (`LiveButton`, `GoalCard`, `DimensionTableWidget`, `MetricSummary`, `FilterBuilder`,
   `WidgetTabs`, `toolbar`, `BreakdownModal`, `DimensionSelector`, `GoalDashboardDrawer`) and
   `styles/reactflow.css` (`.add-node-button`, plus `#6354d1` hover → `var(--color-primary-600)`) are
   migrated to the Tailwind tokens (`text-primary`, `bg-primary`, `border-primary`, `var(--color-canvas)`…).
   Test (in `noHardcodedColors.test.ts`): no `.ts/.tsx/.css` under `src/` contains
   `var(--primary)`, `var(--background)` or `var(--foreground)`.
2. **Email canvas rules use `:where()`.** `:where(.email-canvas) h1`, `:where(.email-canvas) a`,
   `:where(.email-canvas) a:hover`, keeping today's (0,0,1)/(0,1,1) specificity so
   `.email-text-preview a` (`editor.css:99`) still wins as today. A test pins the `:where(` form.
3. **Canvas class placement.** `email-canvas` goes on `div[data-canvas-root]` (`EditPanel.tsx:325`) and on
   the inline block renderer of `BlockThumb.tsx` (thumbnails in SavedBlockCard, AddBlockPopover,
   SaveBlockModal). Both tested. The Preview iframe is unaffected.
4. **Contacts fixed columns.** The four background rules are deleted instead of rebound: with an opaque
   table, antd's fixed cells paint `tableBg`, and its hover/selected rules then apply to the fixed cells like
   the rest of the row. Noted as a deviation from the spec in the PR.
5. **Table header sizes.** The medium selector is `.ant-table.ant-table-medium` (antd 6.6 maps `middle` to
   `medium`), not `.ant-table-middle`. Specificities checked: base (0,3,2) beats antd's (0,2,2); small/medium
   (0,4,2) tie and win on order (antd prepends its styles).
6. **Fonts: latin-only by hand.** No latin-only CSS ships with fontsource. New `src/theme/fonts.css` declares
   `@font-face` for `Inter Variable` (normal and italic, `files/inter-latin-wght-*.woff2`, weights 100-900)
   and `JetBrains Mono Variable` (normal, `files/jetbrains-mono-latin-wght-normal.woff2`), with the latin
   `unicode-range` and `font-display: swap`, imported in `main.tsx`. `opsz` axis exists but is not loaded
   (+73 KB for a hardly visible gain at 13 px). Build check: only those three `.woff2` files in `dist/`.
   Test: `main.tsx` imports `./theme/fonts.css`; `fonts.css` references only `-latin-` files.
7. **Italic face** declared (see 6), so the "not set" italics keep rendering italic with
   `font-synthesis: none`.
8. **`:root` keeps `font-family: var(--font-sans)`**: antd's `reset.css` sets `html { font-family:
   sans-serif }` unlayered, which beats Tailwind's layered preflight; body portals outside `.ant-app`
   would otherwise fall back to `sans-serif`.
9. **Block actions selector is compound**: `.ant-dropdown.block-actions-dropdown …` (antd puts
   `rootClassName` on the `.ant-dropdown` element). The test requires the compound form and checks that
   `BlockActionsMenu.tsx` passes `rootClassName="block-actions-dropdown"`.
10. **`antdOverrides.test` scope**: `src/index.css` is excluded from the "no `.ant-`" walk (it has its own
    check); comments are stripped before parsing; the dead commented `.drawer-no-transition` rule is deleted.
11. **Contract test mapping, explicit**: gray scale, roles (`canvas`…`ink-4`), primary scale plus the
    `--color-primary` alias, `brand-pink`, tones except `primary` as `--color-<tone>-bg/fg`, text sizes and
    line heights, radii, `card`/`popup` shadows, layout spacings, font stacks. Declarations split on `;`.
    `status`, `chart`, `tones.primary`, `motion`, `shadows.focus` are not Tailwind tokens and are not
    expected in the block.
12. **No tautological tests**: `PRIMARY_COLOR` and `COMPARISON_COLOR` are pinned to `'#6a5af9'` and
    `'#b4b8c7'`, `shadows.focus` to `'rgba(106, 90, 249, 0.18)'`, and a source check asserts
    `web_analytics/lib/types.ts` imports from `theme/tokens` with no hex on those lines.
13. **Literal-colour regex** in `antdTheme.test`: `/#[0-9a-f]{3,8}\b/i` and `/rgba?\(|hsla?\(/i`. The theme
    is one object literal typed `ThemeConfig` (no intermediate per-component objects), so TypeScript catches
    misspelled token names.
14. **Why `a` moves into `@layer base`** (corrected): inside `.ant-app`, antd already colours anchors with
    `colorLink` (primary-600); the base rule covers anchors outside it (portals) and stops beating Tailwind
    utilities. `text-decoration`/`font-weight` kept as today.
15. **Known deviation**: `Card.colorBorderSecondary: line` also draws the card header divider (`line`,
    not `line-soft`); no Card token separates them. Listed in the PR.
16. **Current top bar**: `Layout.headerHeight: 48` would set `line-height: 48px` on the existing 64 px
    `Header` until spec 2 removes it; `WorkspaceLayout`'s header gets an inline `lineHeight: '64px'` for this PR.
17. **Known, not treated** list extended: `LibraryPanel.tsx:62` (`hover:border-[#7763F1]`, asserted by its
    test), `WebAnalyticsAIAssistant.tsx:228` (`#4f46e5` gradient end). `index.css` is added to the
    `noHardcodedColors` file list.
18. **Usage rules and typography details**: the Tailwind usage rules are written into `console/CLAUDE.md`
    (new "Styling" section). `tabular-nums` and the -0.015 em heading tracking are applied by the spec 2
    components (`PageHeader`, `KpiStrip`, counts). Visual check adds the AI assistant markdown `<h1>`.
