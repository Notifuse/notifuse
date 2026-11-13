# Fix the console bugs seen while refreshing the docs screenshots

Branch `fix/console-capture-bugs` from `dev`, PR to `dev`. No schema change, no migration, no environment
variable: the two broadcast columns used below already exist in every workspace database
(`internal/database/init.go:255-256`) and are already read by the broadcast analytics measures
(`internal/domain/analytics.go:460-510`).

Plan reviewed independently before coding; its findings (in-loop refresh, `getBroadcastQuery`, test
fixture returning a fresh copy, integration wait on `processed`, changelog placement) are integrated below.

## 1. A/B panel says "sent to the remaining 0"

**Cause.** `AbTestSummary` (`console/src/components/broadcasts/detail/AbTestPanel.tsx:262-265`) prints
`broadcast.winner_phase_recipient_count`. The orchestrator computes the test and winner counts
(`internal/service/broadcast/orchestrator.go:1030-1044`) but only into the task state
(`SendBroadcastState`); the broadcast repository neither writes nor reads `test_phase_recipient_count` /
`winner_phase_recipient_count` (`internal/repository/broadcast_postgres.go`: `broadcastListColumns`, the
INSERT, the UPDATE, `scanBroadcast`). So the API always returns 0 and every real A/B broadcast shows
"remaining 0". The broadcast analytics measures `test_recipients`, `winner_recipients`,
`avg_test_recipients` and `count_with_ab_test` also always read 0.

**Fix.**
- Orchestrator: the send loop re-reads the broadcast at every iteration (`broadcast = refreshed`,
  `orchestrator.go:1205-1207`), so counts set on it where they are computed would be lost. The state
  (`broadcastState`) is recomputed on every run, so the counts are copied from it onto the broadcast right
  before each write: inside the `handleTestPhaseCompletion` transaction (before `UpdateBroadcastTx`) and in
  the final block next to `broadcast.EnqueuedCount = …` (before `UpdateBroadcast`). The final save also
  repairs a copy `SelectWinner` wrote with 0. A single-template broadcast keeps 0.
- Repository: write both columns in `CreateBroadcastTx` and `UpdateBroadcastTx`, read them in
  `broadcastListColumns`, `getBroadcastQuery` (used by `GetBroadcast` / `getBroadcastTx`) and
  `scanBroadcast` (appended at the end, order kept in sync). The status-only UPDATEs are untouched.
- Console: broadcasts sent before this fix keep 0 in the column (backfilling from task state would need a
  migration, out of scope). `AbTestSummary` shows "sent to the remaining {n} at {time}" when the count is
  above 0, and "winner sent at {time}" otherwise, instead of a false 0. New string extracted and translated
  in every locale.

**Tests (written first, seen failing).**
- `internal/repository/broadcast_postgres_test.go`
  - `CreateBroadcast_Success`: expects both counts among the INSERT args.
  - `UpdateBroadcast_Success`: expects both counts among the UPDATE args.
  - `GetBroadcast_Success`: rows carry the two columns; the returned broadcast has them.
  - `broadcastListColumnNames` and every ad-hoc sqlmock row set gain the two columns (scan order).
- `internal/service/broadcast/orchestrator_test.go`
  - new `TestBroadcastOrchestrator_Process_ABTestStoresPhaseCountsOnBroadcast`: A/B broadcast, 50 % sample
    of 10 recipients, `GetBroadcast` returns a fresh copy of the stored broadcast on every call
    (`DoAndReturn`) so the in-loop refresh behaves as in production → the broadcast written by
    `UpdateBroadcastTx` at test completion carries test 5 / winner 5.
  - winner-phase case: the broadcast written by the final `UpdateBroadcast` carries 5 / 5.
  - an existing single-template test asserts the written broadcast keeps 0 / 0.
- `tests/integration/broadcast_ab_testing_e2e_test.go`, "should complete full A/B testing workflow": after
  waiting for status `processed` (not `winner_selected`, which can come before the final save),
  `broadcasts.get` returns `test_phase_recipient_count` 10 and `winner_phase_recipient_count` 10
  (20 contacts, 50 %).
- `console/src/components/broadcasts/detail/AbTestPanel.test.tsx`: winner sent with a count → "sent to the
  remaining 332 at …"; winner sent with 0 → "winner sent at …", no "remaining 0".

## 2. API key name: hyphens stripped by the console

**Cause.** `WorkspaceMembers.tsx` (input `onChange` and `handleCreateApiKey`) keeps `[a-z0-9_]` only, while
the backend accepts `^[a-z0-9_-]{1,64}$` (`internal/domain/workspace.go:1341`) and the docs say hyphens are
accepted.

**Fix.** One exported helper `toApiKeyPrefix(name)` in `console/src/components/settings/apiKeyPrefix.ts`:
lowercase, spaces → `_`, drop everything outside `[a-z0-9_-]`, cut at 64, no trim (it also runs while
the user types). Used by both places; `handleCreateApiKey` keeps its `.trim()` before it.

**Tests.** `console/src/components/settings/apiKeyPrefix.test.ts`: keeps hyphens (`website-signup`), spaces
to underscores, drops other characters, lowercases, caps at 64. `WorkspaceMembers.test.tsx`: typing
`Website-Signup` creates the key with `email_prefix: 'website-signup'`.

## 3. Broadcasts list: the Broadcast column is squashed

**Cause.** `BroadcastsTable.tsx` fixes every other column's width (1126 px with web analytics on) and sets
`scroll={{ x: 1150 }}`, which leaves the name column 24 px minimum, not the "about 200px" its comment
promises. At 1440 px with the sidebar open, the name column is about 40 px wide.

**Fix.** `scroll.x` = the sum of the fixed widths of the columns actually shown + 200
(`BROADCAST_NAME_MIN_WIDTH`), computed by an exported pure helper `minTableWidth(columns, nameMinWidth)` that counts numeric widths only.
Below that width the table scrolls, as the comment intends.

**Tests.** `console/src/components/broadcasts/BroadcastsTable.test.tsx` (new): `minTableWidth` adds the
name minimum to the fixed widths and ignores a column without width; the rendered table's scroll width
equals the visible columns' widths + 200 with and without the web analytics column.

## 4. Integration drawer titles in capitals and not translated

**Cause.** `Integrations.tsx` builds "Add New MAILGUN Integration", "Edit SUPABASE Integration",
"Add New OPENAI Integration" with `toUpperCase()` and outside `t`.

**Fix.** `t\`Add ${name} integration\`` / `t\`Edit ${name} integration\`` with the provider's display name
(`getProviderName`, `getLLMProviderName`) bound to a variable `name`, so the four drawers (email provider,
Supabase, LLM, Firecrawl) share one message `{name}`. Strings extracted and translated in every locale.
Out of scope, listed as known: the untranslated card subtitle `Type: ${integration.type}`.

**Tests.** `console/src/components/settings/Integrations.test.tsx` (existing): opening the Mailgun drawer
shows "Add Mailgun integration"; Supabase "Add Supabase integration".

## Docs


The Notifuse/docs PR #46 captures `supabase_notifuse.png` and the provider `*_form.png` show the old
titles; re-shot after merge if wanted. No docs text names these titles.

## CHANGELOG

44.0 is unreleased; 43.0 is on `main`. `AbTestPanel.tsx` and `BroadcastsTable.tsx` are not on `main`, so the
"winner sent at" fallback and fix 3 get no `Fix` bullet: they are folded into the 44.0 broadcasts feature
bullet (new list, detail page). Released behaviour gets `Fix` bullets:
- fix 1: `broadcasts.get` / `broadcasts.list` returned the A/B test and winner recipient counts as 0, and the
  analytics measures built on them read 0; true for A/B broadcasts sent from now on.
- fixes 2 and 4: one console `Fix` bullet (hyphens kept in an API key name; integration drawer titles
  translated, no longer in capitals).
No database version bump (no schema change).

## Checks

- `make test-repo`, `make test-service` (broadcast package), `go vet ./...`
- Integration: `INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration/ -run TestBroadcastABTestingE2E`
- `cd console && npx vitest run AbTestPanel apiKeyPrefix WorkspaceMembers BroadcastsTable Integrations`,
  then `npm run lint`, `npx tsc --noEmit`, `npm run lingui:extract` with no missing translation,
  `npm run build`.
- Full `make test-unit` and `cd console && npx vitest run` once before review.
