# Test coverage above 90 % — plan

## Goal

Codecov reports 89.04 % on `dev` @ bcffc25 (74 082 lines, 65 966 hits). PR #566
(`test/coverage-critical-paths`) brings it to 89.47 % (66 287 hits). Reaching 90 % once
both PRs are merged needs 387 more covered lines, or 361 once the stray mockgen program
below leaves the denominator. This PR aims at about 450 lines, so that the target holds
with a margin, on code that is cheap to test: `SECRET_KEY` rotation (asked for by name),
then refusal branches in HTTP handlers and pure validation code in `internal/domain`.

Codecov counts a partial line (a condition with one branch run) as not covered, so
covering the error branch of an `if err != nil` gains the `if` line too.

**No production code change.** A defect found is reported in the PR, not fixed.

### Stray file removed

`internal/domain/gomock_reflect_1620494672/prog.go` is the temporary `package main` that
`mockgen` writes in reflect mode, committed by accident in 8ce64ae. Nothing imports it,
no `go:generate` line, Makefile, CI file or `codecov.yml` names it, it is not a mock (so
the Codecov `ignore` list does not catch it) and it counts 29 uncovered lines. It is
deleted; `go.mod` does not change (`golang/mock` is still used by the tests).

### Not reachable (reported, not tested)

- `RotateSecretKey`: the non-`ErrUndecryptable` error from `crypto.ReencryptValue` only
  comes from `EncryptString` failing to read random bytes.
- `SupabaseWebhookHandler.handleUserCreatedWebhook`, the "error, but returning success"
  branch: `ProcessUserCreatedHook` returns an error only for a disposable address, which
  takes the 400 branch; every other failure is logged and returns nil.
- `tree.go`: the second `field_type` check inside the `json_path` block (an invalid type
  is refused earlier) and the `json.Unmarshal` failure in `TreeNode.ToMapOfAny`; likewise
  the `json.Unmarshal` failure in `Contact.ToMapOfAny`.
- `NullableTime.Scan` of `[]byte`, success branch: see the defect below.

### Possible defects found (reported, not fixed here)

- `NullableTime.Scan([]byte)` can never succeed: it delegates to `sql.NullTime.Scan`,
  which refuses `[]byte` ("unsupported Scan, storing driver.Value type []uint8 into type
  *time.Time"). A driver that hands a timestamp as text would fail the read.
- `tasks.reset` / `tasks.trigger` answer a database failure with 400 and the raw error
  text (`failed to reset task: …`), where a 500 with a generic message is expected.
- `TaskService.ResetTask` and `TaskService.TriggerTask` turn any error from the task lookup
  into `ErrTaskNotFound`, so a database failure between the handler's authorization read
  and the service's own read answers 404.

## Files and test cases

### 1. `internal/repository/secret_key_rotation.go`

Test file: `internal/repository/secret_key_rotation_failures_test.go` (sqlmock, reusing
the constants and helpers of `secret_key_rotation_test.go`).

`RotateSecretKey`, each failure returns an error and commits nothing (no `ExpectCommit`,
`ExpectationsWereMet`):
- `BeginTx` fails → "failed to begin transaction" (no transaction, so no rollback).
- Reading the workspaces fails → "failed to read workspaces", rolled back.
- A workspace row with the wrong column count → "failed to scan workspace".
- A row error ending the read (`RowError`, surfaced by `rows.Err()`) → "failed to read
  workspaces".
- Settings that are not JSON → "workspace <id>, settings"; integrations that are not
  JSON → "workspace <id>, integrations".
- Writing a re-encrypted workspace fails → "failed to update workspace <id>".
- Reading the settings fails, a setting row with the wrong column count, a row error →
  "failed to read settings" / "failed to scan setting".
- Writing a re-encrypted setting fails → "failed to update setting <key>", and the
  workspace written before it is rolled back with it.
- `Commit` fails (`ExpectCommit().WillReturnError`, no `ExpectRollback`: `Tx.Commit` marks
  the transaction done, so the deferred rollback never reaches the driver) → "failed to
  commit the rotation", and the result is zero.

`SecretKeyOpensAWorkspace` / `SecretKeyOpensASetting` (both through
`keyOpensAStoredValue`): a row that does not scan and a row error → error, opens false.

### 2. `internal/service/webhook_registration_service.go`

Test file: `internal/service/webhook_registration_service_resources_test.go`, with a
fake SES provider implementing `WebhookProvider`, `TenantAssociator`,
`InboundRouteRegistrar` and `SendingResourceTeardown` and recording its calls.

- `NewWebhookRegistrationService` registers each provider service that implements
  `domain.WebhookProvider` under its kind, and a call reaches that kind's service.
- `RegisterWebhooks` on SES:
  - status names a managed configuration set → persisted through
    `PatchIntegrationSESSettings` with `managed_configuration_set`;
  - the same name already stored → no patch; a set that is not managed → no second
    workspace read, no patch;
  - persisting fails (workspace read error, integration gone, patch error) → registration
    still succeeds;
  - integration with a tenant → `AssociateExistingTenant` called after registration with
    the SES settings and senders; its failure does not fail the registration; no tenant →
    not called;
  - inbound route registered with an `InboundTopicARN` → ARN persisted (each workspace
    read returns a fresh copy, otherwise the ARN already written through the provider
    config would read as persisted); persistence failure does not fail the registration.
- `persistSESManagedConfigurationSet` with an empty name → nothing read.
- `persistSESInboundTopicARN`: empty ARN → nothing read; workspace read error and an
  integration that is not SES → error.
- `DeleteIntegrationResources` (integration deletion):
  - webhooks unregistered, then `DeleteSendingResources`, in that order;
  - unregister failure → teardown still runs, nil returned;
  - teardown failure → nil returned (deletion is never blocked);
  - a provider without teardown → unregistered once, nothing else;
  - integration gone → error, no provider call; provider kind without webhooks → error.

### 3. `internal/http/supabase_webhook_handler.go`

Test file: `internal/http/supabase_webhook_handler_processing_test.go` (a real
`service.SupabaseService` over strict mocks, as the handler takes the concrete type;
payloads signed with the standard-webhooks signer).

- Auth email hook: a signed `signup` payload → the matching `supabase_signup_*`
  notification is sent to the user, 200 `{"success": true}`.
- Auth email hook: the URL names an integration the workspace does not have → 400
  "Failed to process webhook", internal detail not in the body.
- Before-user-created hook: disposable address with `RejectDisposableEmail` → 400 in the
  hook's `{"error": {"http_code": 400, "message": …}}` shape.
- Before-user-created hook: workspace lookup fails → 204, a Notifuse failure never blocks
  a sign-up.

### 4. `internal/http/task_handler.go`

Test file: `internal/http/task_handler_refusals_test.go`.

- Authentication fails with an error `writeServiceError` does not map → 500 "Failed to
  get task", task service not called.
- Loading the task for authorization fails (not a not-found) → 500 for reset and trigger,
  neither reaches the service.
- Delete: the service answers "not found" after the check → 404.
- Execute: a body that cannot be read → 400; `*domain.ErrNotFound` from `ExecuteTask` →
  404; `*domain.ErrTaskAlreadyRunning` → 409; `*domain.ErrTaskExecution` wrapping
  `ErrTaskAlreadyRunning` → 409; reason "failed to mark task as running" → 409; any other
  error → 500.
- Reset / trigger: `domain.ErrTaskNotFound` → 404; a state refusal from the service ("task
  is not a recurring task", "task cannot be triggered in current status: completed") → 400
  with its message (fixtures are the service's own state errors, not database errors: see
  the defects).

### 5. `internal/http/blog_theme_handler.go`

Test file: `internal/http/blog_theme_handler_refusals_test.go`.

For the six endpoints (create, get, getPublished, update, publish, list): wrong method →
405; missing `workspace_id` → 400; for the three that read a body (create, update,
publish) invalid JSON → 400; `*domain.PermissionError` from the service → 403. List:
non-numeric `limit` / `offset` → 400. A refused request never reaches the service
(strict gomock). Create's refusals and the missing `workspace_id` of get, getPublished
and list are already covered; they run again in the table at no cost.

### 6. `internal/domain/tree.go`

Test file: `internal/domain/tree_validation_test.go`.

Refusals of `TreeNode.Validate`: nil branch leaf; `contact_lists`, `contact_timeline`,
`custom_events_goals` leaves without their condition; an invalid timeline condition;
contact condition without filters / with a nil filter; timeline with an unknown
`timeframe_operator`, a nil filter, an invalid filter; goal with empty / unknown
`goal_type`, unknown aggregate, unknown operator, `between` without `value_2`, unknown
timeframe, timeframe without values, nil filter, invalid filter; `in_array` without
`string_values`. Accepted: goal `between` over the last days with a filter, goal type
`*`, timeline over the last days. `HasRelativeDates` for a goal over the last days, a goal
over any time and an unknown node kind. `TreeNodeFromMapOfAny` with a map of the wrong
shape and `TreeNode.ToMapOfAny` with a NaN value → error.

### 7. `internal/domain/contact.go`

Test file: `internal/domain/contact_parsing_test.go`.

- `FromJSON`: each nullable string, number and datetime field with a value of the wrong
  JSON type is refused, naming the field (table over the field names); an unknown
  `tracking_clicks_consent` is refused.
- `Contact.Validate`: an unknown `tracking_opens_consent` is refused.
- `UpsertContactRequest.Validate` without `contact` → "contact field is required".
- `GetContactsRequest.FromQueryParams`: `segments[]` read; an unknown
  `tracking_clicks_consent` refused. `GetContactsRequest.Validate`: both consent filters
  refused when unknown.
- `Contact.Merge`: each optional field set on the other contact (external ID, timezone,
  language, last name, full name, phone, address line 2, postcode, state, job title,
  consent source, consent updated at) is copied; an empty email keeps this one.
- `Contact.ToMapOfAny` with a NaN custom number → error.

### 8. `internal/domain/nullables.go` and `internal/domain/data_feed.go`

Test files: `internal/domain/nullables_json_test.go`, `internal/domain/data_feed_scan_test.go`.

- `NullableString`, `NullableFloat64`, `NullableTime` decode their struct form
  (`{"String"|"Float64"|"Time": …, "IsNull": …}`), and a JSON value of neither form
  (`[1]`) is refused.
- `NullableFloat64.Scan` of bytes that are not a number is refused.
- `GlobalFeedSettings`, `RecipientFeedSettings`, `DataFeedSettings` `Scan` of stored JSON
  that does not decode is refused; feeds stored without headers get an empty list.
- `BuildRecipientFeedContact` copies custom numbers, datetimes (RFC 3339) and JSON fields
  2 to 5, and leaves null ones out.

## Spec review

Reviewed by an independent sub-agent on 2026-11-13. Confirmed remarks are folded in
above: function and type names (`BuildRecipientFeedContact`, `GetContactsRequest`,
`TreeNodeFromMapOfAny`), cases already covered replaced by the uncovered ones (delete
not-found, unreadable execute body), sqlmock expectations for begin and commit failures,
the unreachable `NullableTime` case turned into a reported defect, fixtures that would
have pinned a database error as a 400, and the extra reachable lines (NaN marshal
failures, valid timeline timeframe, empty managed set name).

## Rules for the tests

- Each test asserts the behaviour (status code, error, no side effect through strict
  gomock or sqlmock expectations), not only that a line ran.
- No count in a test name or comment unless an assertion pins it.
- No production code change; defects found are reported in the PR.

## Verification

```bash
go test -race ./internal/repository/ -run 'RotateSecretKey|SecretKeyOpens|SecretKeyChecks'
go test -race ./internal/service/ -run 'RegisterWebhooks_SES|PersistSES|DeleteIntegrationResources|NewWebhookRegistrationService'
go test -race ./internal/http/ -run 'Supabase|TaskHandler|BlogTheme'
go test -race ./internal/domain/
make test-unit
go test -coverprofile=coverage.out ./internal/... && go tool cover -func=coverage.out
```

Then compare the per-file coverage of the files above with Codecov on `dev` @ bcffc25,
and the project total on the PR with the 90 % target once #566 is merged.
