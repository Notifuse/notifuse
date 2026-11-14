# Console sidebar: open on hover, pin to keep it — plan

## Today

`layouts/app-shell/AppSidebar.tsx` (fixed `aside`, `z-10`) and `layouts/WorkspaceLayout.tsx` (#531, unreleased, in the `44.0` WIP):

- Two widths: `w-sidebar` (228px) and `w-sidebar-collapsed` (56px). The choice is stored by `useSidebarCollapsed` under `notifuse.sidebar.collapsed`, expanded by default.
- Expanded: a collapse button (`PanelLeft`, "Collapse sidebar") at the top right. Collapsed: an expand button (`PanelLeft`, "Expand sidebar") at the bottom, under Help and the account.
- Rail pages (`staticData: { layout: 'rail' }` in `router.tsx`: Lists, Contacts, File manager, Settings, Settings sections, Blog) always fold the sidebar; their expand button "peeks" it over the content (`shadow-popup`), closed by Escape, a click outside, the focus leaving, or a navigation.
- The width animates: `motion-safe:transition-[width] duration-180` on the `aside`, `transition-[margin]` on `main`, and antd's own `Menu` styles (antd 6.6.0, `antd/es/menu/style/index.js`) transition the menu's `width`, item `padding`, and the label's `opacity` and `margin` when `inlineCollapsed` changes.
- Collapsed, the workspace icon does not look centred (reported by Pierre, 2026-11-14) although its button already has `mx-auto`; the cause is not established yet (step 1).

## Wanted (Pierre, 2026-11-14)

- Unpinned, the sidebar is the 56px rail; **hovering it opens it over the content**, at once, and leaving it closes it at once.
- **No toggle button at the bottom** of the sidebar.
- Collapsed, the **workspace icon is centred** horizontally.
- **No collapse/expand animation**.
- Open over the content, a **pin icon at the top right** keeps it open: the sidebar then **joins the content flow** (the content moves right) and is no longer over it.
- Pinned, the **same pin button**, in a pressed state, unpins: the content goes back to the 56px margin and the sidebar stays open over it until the pointer leaves.
- **The pin applies on every page**, rail pages included: no more forced fold, no more peek special case. (`RailLayout` puts its rail inside `main`, sticky, so it follows `main`'s margin: nothing to change there.)
- **Pinned by default**, and the choice **survives a reload** through the **existing** `notifuse.sidebar.collapsed` key, which now means "unpinned": no new key, no migration, and nobody's sidebar changes on upgrade.
- Delays: **none** (instant open and close). Accepted consequence: a pointer crossing the left 56px opens it.

## Change after the first try (Pierre, 2026-11-14)

- **Contacts and Broadcasts open with the sidebar unpinned**, whatever the stored choice: they need the width. Only those two (not every rail page): route flag `staticData.unpinSidebar` on `/contacts` and `/broadcasts` (`/lists` redirects to Contacts).
- **The pin stays offered there, for the visit**: pinning on Contacts holds while you stay on that route (opening a contact only changes the search), is not stored, and ends when you leave; the stored choice applies again elsewhere, and a later visit to Contacts starts unpinned again.
- Tests (`__tests__/WorkspaceLayout.test.tsx`): Contacts and Broadcasts open unpinned and store nothing; a pin on Contacts moves the page, leaves the storage alone, gives way to the stored choice on Broadcasts and is gone on the next visit; a stored pin applies again on Settings after Contacts.


Two states replace `collapsed` / `peek` / `rail`:

- `pinned: boolean` — `!collapsed` from the existing `useSidebarCollapsed` hook (key and default unchanged: absent or unreadable storage → pinned).
- `overlay: boolean` — not stored: the sidebar is open over the content because the pointer is over it, or the keyboard focus is inside it, or it was tapped (touch).

Derived: `expanded = pinned || overlay` (width 228px, full menu, logo, pin button); `floating = !pinned && overlay` (adds `shadow-popup`); `main` margin = `pinned ? ml-sidebar : ml-sidebar-collapsed`. "Joins the content flow" is that margin: the `aside` stays `fixed`. The `rail` layout keeps its role for the page margins only (`data-page-layout`), not for the sidebar.

Opening and closing the overlay:

- `pointerenter` on the `aside` with `pointerType !== 'touch'` (mouse, pen; jsdom's `fireEvent.pointerEnter` carries no `pointerType`, which must count as a mouse) → open; `pointerleave` → close, unless a sidebar popup is open. The workspace switcher, Help and account menus render inside the `aside` through `getPopupContainer` (the language submenu too, through the account menu's `getPopupContainer`), so moving into them is no leave; the case left is a popup still open while the pointer goes elsewhere: the overlay closes when that popup closes, if the pointer is outside by then. Each of the three components already holds a `menuOpen` state (`WorkspaceSwitcher.tsx`, `HelpMenu.tsx`, `AccountMenu.tsx`); each reports it through a new `onMenuOpenChange` prop, which replaces the `.ant-dropdown:not(.ant-dropdown-hidden)` DOM query.
- `focusin` inside the `aside` while the pointer is not on it → open (keyboard: tabbing into the rail opens it, so the pin button is reachable). A focus arriving under the pointer (a click on the pin or an item) does not hold it: otherwise unpinning with the mouse would leave the sidebar open after the pointer left, until a click elsewhere; focus leaving the `aside` → close (current `onFocusOut` logic); Escape → close, after an open sidebar menu has taken it (current logic).
- Touch (`pointerType === 'touch'`): a tap on the rail outside an item opens it; a tap outside the `aside` closes it (current `mousedown`-outside logic, as `pointerdown`). A tap on a menu icon navigates as today.
- Navigation (`navigationKey` changing) folds the overlay, **whatever holds it, the pointer included** (Pierre, 2026-11-14: clicking Contacts left it open under the pointer). The pointer's hold is recorded with the page it began on (`hoveredOn`), so a navigation ends it without an effect; it opens again once the pointer has left and come back. Focus and tap holds are cleared while rendering (`react-hooks/set-state-in-effect`).
- A pointer entering with a button held (`event.buttons`) is a drag passing by (email editor, automation flow) and does not open it.
- Pinning: `pinned = true`, stored; the overlay state is cleared. Unpinning: `pinned = false`, stored; the overlay stays open while the pointer or focus is inside.

**The focus must survive the fold.** Opening on `focusin` re-renders the sidebar expanded while the focused element is in it. Today `WorkspaceSwitcher`, `HelpMenu` and `AccountMenu` wrap their button in a `Tooltip` only when collapsed (`collapsed ? <Tooltip>{button}</Tooltip> : button`), so expanding remounts the button and the focus falls to `<body>` — a keyboard user tabbing into the rail would lose their place. Those collapsed-only tooltips can no longer show anyway (any pointer or focus on them opens the sidebar first, and the expanded rows show the name), so they go, and the button keeps one DOM node in both states (its `aria-label` names it when folded). antd's `MenuItem` always renders its `Tooltip` wrapper (`antd/es/menu/MenuItem.js`), so menu items keep their node. The logo `Link` keeps its node (only its child changes). The pin button exists only when expanded: Escape pressed on it moves the focus to the logo link before folding.

## Decisions

- **Taken by Pierre (2026-11-14):** the behaviours under "Wanted"; storage through the existing key.
- **D2 (proposed) — the pin button** is a toggle: lucide `Pin` (15px), `aria-label` "Pin sidebar", `aria-pressed` `false`/`true`, tooltip "Pin sidebar" / "Unpin sidebar" (the visible hint changes, the accessible name stays, as for a toggle button). Pressed: `bg-primary-50 text-primary-600`; unpressed: `text-ink-3 hover:bg-fill hover:text-ink`. It replaces the collapse button at the same place (`h-sidebar-head`, right). It renders only when `expanded`.
- **D3 (proposed) — no animation anywhere in the change**: the two `motion-safe:transition-*` classes go, and a rule in `src/theme/antd-overrides.css` turns off antd's `Menu` transitions under `.app-sidebar-nav` (`transition: none` on `.ant-menu`, `.ant-menu-item`, `.ant-menu-submenu-title`, `.ant-menu-title-content`, `.ant-menu-item-icon`), with its reason, allow-listed in `antdOverrides.test.ts`. Submenu open/close (rc-menu's height motion) is not the fold and stays.
- **D4 (proposed) — folded menu flyouts**: the pointer or the focus opens the whole sidebar before an antd flyout could show, so `inlineCollapsed` stays for the 56px rendering and no flyout-specific code is added.

## Steps

### Step 1 — measure the workspace icon (browser, before any change)

In the collapsed state, read `getBoundingClientRect()` centres of: the `aside` (its 1px right border is a `span` drawn over it, so 55px are visible), the `NotifuseMark`, the workspace button and its badge, one menu icon, Help, the account avatar. Write the offsets into this plan. Fix the switcher so its centre matches the menu icons' column (candidates: the switcher wrapper `shrink-0 mb-2.5` against the footer's `items-center px-2.5`; the 1px border; antd's collapsed item padding). The Chrome extension could not open `https://notifusedev.com:5173` on 2026-11-14 (certificate warning, which Claude does not click through): Pierre accepts the certificate in Chrome first, or headless Chrome is used.

**Measured 2026-11-14 (folded, Contacts, 1470px):** aside centre 28px; Notifuse mark, menu icons, Help and avatar 28px; workspace button 18px (left 0, 36px wide). Cause: the unlayered reset `input, button, select, optgroup, textarea { margin: 0 }` beats Tailwind's layered `mx-auto`. Fix: the switcher wrapper centres it (`flex justify-center` when folded), `mx-auto` dropped. After the fix every centre is 28px. Asked by Pierre at the same time: 8px above the switcher and 16px below (`mt-2 mb-4`, was 0 and 10px), and the trigger badge down from 24px to 20px (`size-5 rounded-sm text-2xs`; the menu's small badge and the settings' large one unchanged).

### Step 2 — state

| File | Change | Test file and cases |
|---|---|---|
| `layouts/app-shell/useSidebarCollapsed.ts` | Unchanged behaviour; its doc comment says the value now means "unpinned". | Existing behaviour covered by `__tests__/WorkspaceLayout.test.tsx` (below). |
| `layouts/WorkspaceLayout.tsx` | `pinned = !storedCollapsed`; `overlay` state; no `rail`-driven fold, `peek` removed; `main` margin from `pinned` only; the margin transition removed; navigation clears a focus/touch overlay (see Model). | `__tests__/WorkspaceLayout.test.tsx`: replace the six collapse/peek cases ("folds the sidebar on a rail page…", "keeps the collapse chosen…", "reads a stored collapse", "opens expanded and still collapses when the storage throws", "peeks on the Blog…", "folds the peek back on navigation") with — `main` has `ml-sidebar` when pinned and `ml-sidebar-collapsed` when not, **on a page and on a rail page**; pinning from the overlay moves `main` to `ml-sidebar` and writes `notifuse.sidebar.collapsed=false`; unpinning writes `true`; a stored `true` opens unpinned, on a rail page too; storage throwing still pins and unpins; on the Blog, unpinned, hovering the sidebar shows Content open with Blog selected without moving `main`; a navigation closes a focus-opened overlay; no `transition` class on `main`. "pads a page and leaves a rail page to its own layout" and the banner case stay. |

### Step 3 — the sidebar

| File | Change | Test file and cases |
|---|---|---|
| `layouts/app-shell/AppSidebar.tsx` | Props `pinned`, `onPinnedChange`, `onExpandedChange` (the layout restores the active group's `openKeys` on unfolding, as it did on `menuCollapsed`) and `navigationKey` (replace `collapsed`, `peeking`, `rail`, `onToggle`, `onPeekChange`). The overlay is the sidebar's own state, from four sources: pointer, focus, tap, open menu. Pointer, focus, touch and Escape handling per the Model. Header: `NotifuseMark` centred when folded; logo + pin button (D2) when expanded. Bottom expand button removed, with `collapseRef` / `expandRef` / `pendingFocus`. `aside` width transition removed; `data-collapsed` → `data-expanded` and `data-pinned` (the test helper `isCollapsed` follows). | `layouts/app-shell/AppSidebar.test.tsx`: replace the collapse and "on a rail page" cases with — unpinned renders 56px with the mark and **no button named Pin sidebar, Expand sidebar or Collapse sidebar**; `pointerEnter` (no `pointerType`) expands with `shadow-popup` and shows Pin sidebar `aria-pressed=false`; `pointerLeave` folds it back; `pointerEnter` with `pointerType: 'touch'` does not open, a tap (`pointerDown` touch) on the rail does, a `pointerDown` outside folds; clicking Pin calls `onPinnedChange(true)`; pinned renders 228px without shadow, Pin `aria-pressed=true`; pointer leave while pinned changes nothing; leave while the workspace switcher menu is open keeps it open, and closing that menu with the pointer outside folds it; same with the language submenu open; `focusIn` on a menu item expands, focus moving out folds, focus moving inside does not; **folded, focusing the workspace switcher, Help or account button expands the sidebar and `document.activeElement` is still that button**; Escape folds unless a sidebar menu is open; Escape on the pin button folds and focuses the logo link; no `transition` class on the `aside`. Kept: logo link, banner offset, "keeps the workspace, help and account reachable by name", workspace switcher, account, help, main navigation cases. |
| `layouts/app-shell/WorkspaceSwitcher.tsx` | `onMenuOpenChange` prop fed from `menuOpen`; the collapsed-only `Tooltip` removed (one button node in both states); the collapsed button centred per step 1. | `AppSidebar.test.tsx` `workspace switcher` describe: the cases above; the visual centring is checked in step 6. |
| `layouts/app-shell/HelpMenu.tsx`, `AccountMenu.tsx` | `onMenuOpenChange` prop fed from `menuOpen`; the collapsed-only `Tooltip` removed. | `AppSidebar.test.tsx` `help` and `account` describes: opening the menu keeps the overlay open on pointer leave; existing cases stay. |
| `src/theme/antd-overrides.css` | D3 rule with its reason. | `theme/antdOverrides.test.ts`: the new selectors are allow-listed under `.app-sidebar-nav`. |

### Step 4 — CHANGELOG

The sidebar is unreleased (#531, `44.0` WIP), so per `CLAUDE.md` no new line: amend the existing **Feature** bullet ("The console has a new navigation: …"). Replace "a lighter sidebar, which folds to icons," with "a lighter sidebar, which folds to icons and opens over the page when you point at it, or stays open once pinned," and drop "On Settings and the blog, which keep their own side menu, the sidebar stays folded and opens over the page."

### Step 5 — translations

New strings "Pin sidebar", "Unpin sidebar"; "Collapse sidebar" and "Expand sidebar" no longer used. `npm run lingui:extract`, then translate the new strings in `ca`, `de`, `es`, `fr`, `it`, `ja`, `pt-BR` (`src/i18n/locales/catalogues.test.ts` fails on an untranslated entry outside its licence exemption). Check `../docs` for a page describing the collapse buttons (`grep -ri "collapse sidebar\|expand sidebar" ../docs`).

### Step 6 — verification

1. `cd console && npx vitest run src/layouts src/__tests__/WorkspaceLayout.test.tsx src/theme src/i18n`
2. `cd console && npx tsc --noEmit -p tsconfig.app.json && npm run lint`
3. `cd console && npm test` (full suite; a test above ~1.9s locally can time out on the CI runner).
4. In a browser, on the demo workspace, at 1440 and 1280px: folded rail with the workspace icon centred (re-run step 1's measurements: every centre within 0.5px of the menu icons'); hover opens over the content with no movement of the page and no animation, the menu labels included; pin moves the content; unpin then leave folds; the same on Lists, Contacts, File manager, Settings and Blog, pinned and unpinned; workspace switcher, Help, account and language submenu usable from the overlay; keyboard: Tab into the rail opens it without losing the focus, the pin is reachable, Escape folds; no storage → pinned; `notifuse.sidebar.collapsed=true` → unpinned.
5. `/code-review` on the final diff before any commit.

## Plan check (2026-11-14)

Checked against the code before implementation:
- Rail routes in `router.tsx`: `/lists`, `/file-manager`, `/contacts`, `/settings`, `/settings/$section`, `/blog` (the first draft omitted Lists).
- `WorkspaceLayout.test.tsx` has six collapse/peek cases, not four.
- antd 6.6.0 `Menu` does animate the fold (width, padding, label opacity/margin): D3's override is required, not conditional.
- `WorkspaceSwitcher`, `HelpMenu`, `AccountMenu` remount their button when the fold changes (collapsed-only `Tooltip`): added the focus-survival requirement and tests; antd `MenuItem` keeps a constant wrapper.
- The pin button disappears on fold: Escape on it now moves the focus to the logo.
- jsdom pointer events carry no `pointerType`: the handlers treat anything but `'touch'` as a mouse.
- `catalogues.test.ts` does fail on untranslated entries (except its licence exemption).
- D1 (a new storage key with a migration) dropped: Pierre chose to keep the existing key.

## Code review fixes (2026-11-14)

`/code-review high` on the first implementation found ten issues; all fixed, each with a test, and the four behavioural fixes mutation-checked:
1. **Focus from a click held the sidebar** when the pointer's hold was stale (after a navigation under the pointer). A press inside now marks the focus it gives as the pointer's (`pressingRef`, capture-phase pointerdown/up/cancel), and refreshes the pointer's hold, so unpinning by click still stays open until the pointer leaves.
2. **Groups, unpinned** (Pierre's choice): a click on a group only opens it; pinned, it still lands on the group's first entry.
3. **A tap on an entry widened the rail before the click**: only a tap outside links, buttons and menu items opens it.
4. **Keyboard focus while the mouse rests on the sidebar** was not held, so the mouse leaving unmounted the focused pin: any non-pointer focus now holds it.
5. **Blur without relatedTarget** (a click into an iframe, a removed element) never released the focus hold: the check now runs once the focus has landed (`document.activeElement`), and switching windows keeps the sidebar as it was.
6. and 7. **Two layout re-renders per hover, and a group closed by hand reopened on every hover**: `onExpandedChange` is gone; AppSidebar passes no `onOpenChange` to the folded menu, so rc-menu's clearing on fold is not reported, and the layout's effect only follows the active group.
8. Stale comments about folded flyouts rewritten.
9. The CSS test anchors on the sidebar selector, not the first `transition: none`.
10. The duplicated `hoveredOn` state and ref merged into the state alone.

## Second review fixes (2026-11-14)

A second `/code-review high` found six probable bugs, fixed with tests and mutation-checked (one guard found redundant by its mutant and removed):
1. A press inside released over the page left `pressingRef` set, so a later Tab never opened the sidebar: released on `window` (capture `pointerup`/`pointercancel`).
2. A focused pin unmounted when the pointer left: the focus goes to the logo first (`rescuePinFocus`), on leave and on Escape.
3. A focus hold taken while pinned outlived a mouse unpin: the pin decides the hold anew (`event.detail === 0`, a keyboard click, holds; a mouse click does not), and a navigation clears it.
4. The pin's name now says what a click does, like its tooltip ("Pin sidebar" / "Unpin sidebar"); `aria-pressed` dropped (WCAG 2.5.3).
5. A page changing on its own (a redirect, a save) no longer folds the sidebar under the pointer: only a navigation following a press inside does (`pressedOn`).
6. The folded workspace, Help and account icons are named again by a tooltip, now always mounted and titled only when folded, so the button keeps its node.
Cleanups: the document listeners are registered once per pin state (`liveRef`); `useSidebarPinned` (same key) replaces the twice-inverted `useSidebarCollapsed`, with its own test file.

Left as known, minor: a focus antd gives back to a menu trigger after Escape holds the sidebar until the focus leaves; on a touch screen a sidebar menu opened from the folded rail moves when the sidebar widens.
