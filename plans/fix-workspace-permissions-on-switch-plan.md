# Workspace permissions on a workspace switch plan

**Status**: plan reviewed (to be corrected, corrections applied below), implemented, code review rounds 1 and 2 applied
**Date**: 2026-11-13
**Target**: 44.0 (`## [44.0] - unreleased` on `dev`). No migration, no environment variable, no server change, no
docs change.
**Base**: `origin/dev` at `18ca152`.

## Goal

When the workspace changes while the workspace layout or a page stays mounted, the console stops using the previous
workspace's permissions. Follow-up of #542, which listed this under "Known, not handled".

## Current behaviour

- `useWorkspacePermissions(workspaceId)` (`console/src/contexts/AuthContext.tsx:162-204`) keeps `permissions` and
  `loading` in state; its effect fetches `workspaces.members` again when `workspaceId` or `user` change, never sets
  `loading` back to `true`, and has no cleanup.
- `WorkspaceLayout` (`console/src/layouts/WorkspaceLayout.tsx:32-34,66-108`) holds a copy of that code, with the
  role on top, for the sidebar menu (`hasAccess`), the account line's role and the file manager's `readOnly`.
- No route sets `remountDeps` (`console/src/router.tsx`), so a route whose id stays the same keeps its component
  when `workspaceId` changes:
  - the layout route `/console/workspace/$workspaceId` on every switch from the workspace picker (which navigates to
    the workspace's index route, `WorkspaceLayout.tsx:132-134`);
  - a page (Broadcasts, Automations, Contacts, Blog, Transactional, Logs, Templates, File manager) when the switch
    comes from the browser history, e.g. A/broadcasts, picker to B (whose dashboard redirects to B/broadcasts), then
    Back to A/broadcasts. `AnalyticsPage` is not affected: its `Dashboard` is keyed by workspace.
- So on a switch from A to B:
  1. until B's members answer, the menu, the role and the page gates use A's permissions, with `loading: false`;
  2. if A's answer arrives after B's, A's permissions are set last and stay for B;
  3. with no `workspaceId` (or no user) after a load, the previous permissions stay.

## Design

1. `useWorkspacePermissions` reads the members list through TanStack Query (the console's rule for server state),
   under the key `['workspace-members', workspaceId, userId]`, which `useAuditLogAccess` and the audit log now use
   too, so the list is fetched once for all of them. The user is in the key because the answer depends on who asks
   (a member without Workspace read gets only their own entry), and signing out does not clear the cache. Data is stored per workspace, so a switch finds no data for B (loading) and a
   late answer for A only fills A's entry.
   - `enabled` only with a user, a workspace and a non-root user; default `staleTime`, so each mount refetches.
   - A caller uses the list only once its own mount (or its switch to the key) has had an answer
     (`isFetchedAfterMount`): a cached list may predate a change of permissions, and today each caller waits for its
     own fetch. Callers already mounted follow every new answer.
   - The answer is derived during render and memoised (`useMemo`), so its identity changes only with the answer:
     - no user or no workspace: `{ permissions: null, role: undefined, loading: false }`;
     - root user: full permissions, role `root`, `loading: false`, on the first render, with no request;
     - no answer since this mount: `{ permissions: null, loading: true }`; a failed lookup with no list: no
       permissions; a failed refetch keeps the list the query holds (a network blip or a reconnect refetch neither
       empties the menu nor sends a page away);
     - otherwise the user's entry: the empty base merged with its stored map, role `owner` or `member`; a
       non-member gets no permissions.
   - The hook now also returns `role` (`'root' | 'owner' | 'member' | undefined`). Its return type is exported as
     `WorkspacePermissions`; the callers read `permissions` and `loading` as before, so no page changes.
   - No setState in an effect or during render and no ref read during render (eslint-plugin-react-hooks 7).
   - Trade-off accepted: the app's `shouldRetryQuery` retries a 5xx or network failure once before the "no
     permissions" answer.
2. `WorkspaceLayout` drops its copy and uses the hook (`permissions`, `role`, `loading`). Its
   `loadingPermissions ? [] : menuItems` guard stays, though `permissions` is already null while loading.
3. `useAuditLogAccess` and `AuditLogTab` add the user ID to the same key.
4. `CHANGELOG.md`, under 44.0, after #536's Templates-read fix (the behaviour shipped in v43.0, so a `Fix` line is
   right):

   > **Fix**: After you switch workspace, the console's menu and pages no longer use the previous workspace's
   > permissions until the new ones arrive, nor keep them when the previous workspace answers last.

What a page sees on a switch is now what it sees on its first load, which every page already handles: Contacts,
Logs, Transactional and Analytics wait on `loading`; Templates, Blog, Broadcasts and File manager briefly show their
"no write" state; Contacts redraws its skeleton.

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `console/src/contexts/AuthContext.tsx` | `console/src/contexts/useWorkspacePermissions.test.tsx` (new; `renderHook` inside a `QueryClientProvider` and `AuthContext.Provider`, every rendered answer recorded, `getMembers` answered by promises the test releases) | after A loads, a switch to B renders "loading" first and then only "loading" or B's answer, never A's; A answering after B is ignored; A failing after B is ignored; A, B, then A again with B answering first stays loading, then A's answer; another user on the same workspace waits for their own list, then gets their entry; a second caller mounted over a cached list waits for its own answer and both callers then follow it; when a second caller's fetch fails over a cached list, both keep that list; a root user gets full permissions on the first render without a request; a non-member and a failed lookup get no permissions; a partial map denies the rest; the answer object stays the same across a rerender; after a load, no workspace then no user answer `null`, not loading, without a request; no user from the start answers the same |
| `console/src/hooks/useAuditLogAccess.ts`, `console/src/components/logs/AuditLogTab.tsx` | `useAuditLogAccess.test.tsx`, `AuditLogTab.test.tsx` | another user on the same workspace with a warm cache fetches the list again and gets their own answer; the existing cases pass unchanged |
| `console/src/layouts/WorkspaceLayout.tsx` | `console/src/__tests__/WorkspaceLayout.test.tsx` (the real hook behind a provider; `useParams` reads a settable workspace) | an owner of ws1 sees Broadcasts and "Owner"; after a switch to ws2, while ws2 loads, no menu entry and no "Owner"; once ws2 answers, its menu (Settings, no Broadcasts) and "Member"; the existing cases pass unchanged |

## Known, not handled

- `resetQueries` on `['workspace-members']` would leave a caller mounted over a cached list loading (its answer count
  goes back to 0); nothing resets this key today, and the hook's comment says so.
- The user ID in `AuditLogTab`'s key is pinned by no test of its own (the same change in `useAuditLogAccess` is).

## Test commands

```
cd console && npx vitest run src/contexts/useWorkspacePermissions.test.tsx src/__tests__/WorkspaceLayout.test.tsx src/hooks/useAuditLogAccess.test.tsx src/components/logs/AuditLogTab.test.tsx
cd console && npm test
cd console && npx tsc --noEmit && npx eslint src/contexts/AuthContext.tsx src/layouts/WorkspaceLayout.tsx src/hooks/useAuditLogAccess.ts src/components/logs/AuditLogTab.tsx src/contexts/useWorkspacePermissions.test.tsx src/__tests__/WorkspaceLayout.test.tsx src/hooks/useAuditLogAccess.test.tsx
```
