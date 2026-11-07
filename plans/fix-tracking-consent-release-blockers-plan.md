# Fix the tracking consent release blockers (43.0)

Two defects that #494 (per-contact tracking consent) put on `dev`, found by the high review run before the
43.0 release (`/mnt/project-files/reviews/pr-494-revue-high-post-merge.md`). Neither has shipped: 43.0 is not
released, so the CHANGELOG gets **no Fix bullet** (CLAUDE.md), and the 43.0 entries already describe the
intended behaviour.

A third finding of that review, "an empty `unknown_consent` is refused", is left out on purpose:
`TestEmailTrackingSettings_Validate` pins that `EmailTrackingSettings{}` is invalid, the console always sends the
field, and refusing an `email_tracking` object without it is safer than silently choosing "track" for a privacy
setting. It goes to the PR's "known, not addressed" list.

## 1. Automatic A/B winner never chosen when no variation has a tracked send

### Problem

`ABTestEvaluator.selectBestVariation` (`internal/service/broadcast/ab_test_evaluator.go`) skips a variation whose
tracked count for the chosen metric is 0, and returns `no winner could be determined` when every variation is
skipped. `BroadcastOrchestrator.evaluateWinner` turns that into an error on every evaluation, so the broadcast
stays in `test_completed` and the rest of the audience is never sent.

Reachable whenever no test send recorded the metric:
- the workspace tracks opens but not clicks and the metric is `click_rate` (or the reverse);
- a workspace still on the single switch with `email_tracking_enabled = false`;
- `unknown_consent = no_track` and no test recipient granted the consent.

Before #494, every variation scored 0 and the first one read won (strict `>` against `-1`), so the broadcast went on.

### Change

In `selectBestVariation`, remember the first variation whose stats were read. When no variation has a tracked
send, that variation wins, with a `Warn` log naming the metric ("no variation has a tracked send for the
metric; the first variation wins"). A variation with tracked sends still beats any untracked one, and when the
stats of every variation fail to load the error stays as it is.

The console drawer keeps letting such a test be saved (a warning exists for `no_track`); that stays in the
"known, not addressed" list.

### Tests (`internal/service/broadcast/ab_test_evaluator_test.go`)

- `TestABTestEvaluator_SelectBestVariation_NoTrackedSendFirstVariationWins`: two variations, both with
  `TotalClicksTracked = 0` and metric `click_rate` → `tplA` is the winner and the broadcast is updated with it.
- `TestABTestEvaluator_SelectBestVariation_FirstReadableVariationWinsWhenNoneTracked`: `tplA` stats fail, `tplB`
  read with 0 tracked → `tplB` wins.
- `TestABTestEvaluator_SelectBestVariation_TrackedVariationBeatsUntracked`: `tplA` tracked 0, `tplB` tracked 10
  with 0 clicks → `tplB` wins (a tracked score of 0 still beats having no data).
- Existing `NoWinnerWhenAllStatsFail` keeps passing unchanged.

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

- `internal/database/schema/webhook_triggers.go`: the contacts function is generated from its list of compared
  columns. `WebhookContactsTriggerFunction()` keeps today's list (unchanged output for v43 and new workspaces).
  A new `WebhookContactsTriggerFunctionBeforeTrackingConsent()` generates the same function without the three
  consent columns, which is the body every release from v39.0 to 42.2 installed. A new
  `WebhookTriggerFunctionsBeforeTrackingConsent()` returns the five functions with that contacts body.
- `internal/migrations/v39.go`: installs `schema.WebhookTriggerFunctionsBeforeTrackingConsent()`. v43 already
  reinstalls `schema.WebhookContactsTriggerFunction()` after adding the columns, so a database ends on today's body.
- Only databases below v39 run the changed v39; databases at v39 or above are unaffected (the dispatcher selects
  `migrationVersion > currentDBVersion`), which is exactly the set that has the problem. No new major, no new
  column, no data change.

### Tests

- `internal/database/schema/webhook_triggers_test.go`:
  - `TestWebhookContactsTriggerFunctionBeforeTrackingConsent_MatchesTheShippedBody`: the generated text equals
    the 42.2 body, frozen verbatim in the test (taken from `git show v42.2:internal/database/schema/webhook_triggers.go`).
  - `TestWebhookContactsTriggerFunction_ComparesTheConsentColumns`: today's body contains the three consent
    comparisons, the pre-consent body none of `tracking_`.
  - `TestWebhookTriggerFunctionsBeforeTrackingConsent_DiffersOnlyInTheContactsFunction`: same length and the same
    four other bodies as `WebhookTriggerFunctions()`.
- `internal/migrations/v39_test.go`: `TestV39Migration_ReinstallsEveryTriggerFunctionFromTheSharedGenerator`
  asserts against `WebhookTriggerFunctionsBeforeTrackingConsent()`, and a new assertion that no statement v39
  issues mentions `tracking_`.
- `tests/integration/v39_webhook_contacts_before_v43_test.go` (real PostgreSQL), `TestV39WebhookContactsFunctionBeforeV43`:
  1. Fresh workspace, one contact, one enabled webhook subscription for `contact.updated`.
  2. Bring `contacts` back to its pre-v43 shape: drop the four consent columns; disable `contact_changes_trigger`,
     whose current body also reads them (its pre-v43 body is not what is under test).
  3. Run v39's `UpdateWorkspace` in a transaction and commit, as the migration manager does.
  4. `UPDATE contacts SET first_name = ...` must succeed and queue one `contact.updated` delivery. Before the fix
     it fails with `record "new" has no field`.
  5. Run v43's `UpdateWorkspace` in a transaction, re-enable the trigger, update `tracking_opens_consent` → one
     more `contact.updated` delivery (today's body compares the consent columns again).

## CHANGELOG

No line: neither defect shipped. The 43.0 entries already describe the corrected behaviour.

## Test commands

- `go build ./... && go vet ./...`
- `go test -race ./internal/service/broadcast/ ./internal/database/schema/ ./internal/migrations/`
- `make test-unit`
- `INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestV39|TestV43|TestABTest|TestBroadcast.*AB'`,
  then the full integration suite in CI.
