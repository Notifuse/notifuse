# Fix the tracking consent release blockers (43.0)

Two defects that #494 (per-contact tracking consent) put on `dev`, found by the high review run before the
43.0 release (`/mnt/project-files/reviews/pr-494-revue-high-post-merge.md`). Neither has shipped: 43.0 is not
released, so the CHANGELOG gets **no Fix bullet** (CLAUDE.md), and the 43.0 entries already describe the
intended behaviour.

A third finding of that review, "an empty `unknown_consent` is refused", is left out on purpose:
`TestEmailTrackingSettings_Validate` pins that `EmailTrackingSettings{}` is invalid, the console always sends the
field, and refusing an `email_tracking` object without it is safer than silently choosing "track" for a privacy
setting. It goes to the PR's "known, not addressed" list.

## 1. An A/B test with an automatic winner and no tracked send cannot be finished from the console

### Problem

`ABTestEvaluator.selectBestVariation` (`internal/service/broadcast/ab_test_evaluator.go`) skips a variation whose
tracked count for the chosen metric is 0, and returns `no winner could be determined` when every variation is
skipped. That is the intended 43.0 behaviour: the docs on dev say "A variation with no tracked recipient cannot
win, and when none has any, you pick the winner yourself" (`docs/features/email-tracking.mdx`), and the
orchestrator already logs "Auto winner evaluation failed, continuing with manual selection"
(`orchestrator.go`, `processBroadcast`). `SelectWinner` (`broadcast_service.go`) accepts a broadcast in
`test_completed` whatever `auto_send_winner` says.

The console is what breaks it: `BroadcastsPage.tsx` shows the Select button only when
`status === 'test_completed' && !test_settings.auto_send_winner`. With an automatic winner that can never be
chosen, the broadcast stays in "Test completed" and the console offers no way to send to the rest of the
audience; only the API can.

Reachable whenever no test send recorded the metric:
- the workspace tracks opens but not clicks and the metric is `click_rate` (or the reverse);
- a workspace still on the single switch with `email_tracking_enabled = false`;
- `unknown_consent = no_track` and no test recipient granted the consent.
(`tracking_mode = no_tracking` with an automatic winner is already refused by `Broadcast.Validate`.)

### Change (console only)

- `console/src/lib/abTestWinner.ts`: `canSelectWinner(broadcast)` returns true for any broadcast in
  `test_completed`, automatic winner or not; the variations table of `console/src/pages/BroadcastsPage.tsx` uses it. The server already
  accepts this, and a broadcast in `test_completed` with an automatic winner is either before its evaluation time
  (choosing early is the owner's explicit call) or one the evaluator could not decide.
- No backend change: the evaluator keeps refusing to crown a variation without data, as documented.

### Tests

- `console/src/lib/abTestWinner.test.ts`: `canSelectWinner` is true for `test_completed` with
  `auto_send_winner` true and false, false for `testing`, `winner_selected`, `processing` and `draft`.
  The first case fails before the change.
- Backend: none changed. The existing evaluator subtest "no tracked emails anywhere: no winner" stays and pins
  the documented rule.

## 2. v39 installs a contacts webhook function that reads columns only v43 adds

### Problem

v39 reinstalls every outbound webhook trigger function from the shared generator
(`schema.WebhookTriggerFunctions()`). #494 added `tracking_opens_consent`, `tracking_clicks_consent` and
`tracking_consent_source` to the change comparison of `schema.WebhookContactsTriggerFunction()`, and only v43
adds those columns. On a workspace database below v39, the 43.0 binary runs v39 (its own transaction, committed),
then v40, v41, v42, then v43. Between the v39 commit and the v43 commit, every UPDATE on `contacts` fails with
`record "new" has no field "tracking_opens_consent"`: PL/pgSQL resolves `NEW.<column>` when the statement runs,
and a function body registers no dependency. Any write from an instance still running, and, if v40-v42 fails, every
contact update until that is fixed.

This is what CLAUDE.md warns about: a shipped migration (v39) reached through a shared generator that a later
change amended.

### Change

- `internal/migrations/v39.go`: v39 installs a frozen copy of the contacts webhook function as it shipped from
  v39.0 to 42.2 (byte-identical in every tag), kept as a literal in `internal/migrations/v39_webhook_contacts.go`,
  as v19 and v23 keep theirs. The other
  four functions still come from the shared generator, as today.
- `internal/database/schema/` is not changed: `WebhookContactsTriggerFunction()` stays what v43 and
  `init.go` install.
- v43 installs today's body (`v43.go`, in the same transaction as the columns it reads), so a database ends on
  today's body.
- Only databases below v39 run the changed v39; databases at v39 or above are unaffected (the dispatcher selects
  `migrationVersion > currentDBVersion`, fresh installs run no migration), which is exactly the set that has the
  problem. No new major, no new column, no data change.

### Tests

- `internal/migrations/v39_test.go`:
  - `TestV39Migration_ReinstallsEveryTriggerFunctionFromTheSharedGenerator` becomes: the four other functions are
    the generator's text, the contacts function is the frozen v39 body.
  - New `TestV39Migration_InstallsNoColumnAddedAfterIt`: no statement v39 issues mentions `tracking_`.
  - New `TestV39WebhookContactsTriggerFunction_IsTheGeneratorWithoutTheConsentColumns`: the frozen body equals
    today's generator output with its three consent comparisons removed (pins that the two differ only there).
- `tests/integration/v39_webhook_contacts_before_v43_test.go` (real PostgreSQL), `TestV39WebhookContactsFunctionBeforeV43`:
  1. Fresh workspace, one contact, one enabled webhook subscription for `contact.updated`.
  2. Bring the workspace back to its pre-v43 shape with the helpers of `TestV43TrackingConsentMigration`: the
     pre-43 `track_contact_changes()` and `webhook_contacts_trigger()` bodies, and the consent, message and
     broadcast columns dropped.
  3. Run every registered workspace migration from v39 up to v42, each in a transaction committed, as the
     migration manager does.
  4. `UPDATE contacts SET first_name = ...` must succeed and queue one `contact.updated` delivery. Before the fix
     it fails with `record "new" has no field`.
  5. Run v43's `UpdateWorkspace` in a transaction, update `tracking_opens_consent` → one
     more `contact.updated` delivery (today's body compares the consent columns again).

## Known, not addressed

- v39's other four functions and v43's two contacts functions still come from live generators: a column added
  later to one of their comparison lists would reach databases below that migration the same way. Nothing does
  so today (checked from v38 to v43).
- An `email_tracking` object without `unknown_consent` is refused with a 400 (see the top of this plan).
- An automatic-winner A/B test whose metric is not tracked can still be saved; the drawer warns only for
  `unknown_consent = no_track`.

## CHANGELOG

No line: neither defect shipped. The 43.0 entries already describe the corrected behaviour.

## Counter-review

The Gemini key was refused, so on Pierre's request the plan was counter-reviewed by an independent xhigh
subagent (`/mnt/project-files/reviews/fix-tracking-consent-release-blockers-contre-avis-xhigh-plan.md`). Changes
made from it: the A/B fix follows the documented rule (manual choice) instead of crowning the first variation; v39
keeps a frozen literal instead of a new generator variant; the integration test runs v39 to v42.

## Test commands

- `go build ./... && go vet ./...`
- `go test -race ./internal/migrations/ ./internal/database/schema/`
- `cd console && npx vitest run src/pages/ && npx tsc -b && npm run lint`
- `make test-unit`
- `INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestV39|TestV43'`,
  then the full integration suite in CI.
