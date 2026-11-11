# Integration-managed refusal errors plan

**Status**: plan reviewed, implemented
**Date**: 2026-11-11
**Target**: 44.0 (`## [44.0] - unreleased` on `dev`). No migration, no environment variable, no console change.
**Base**: `origin/dev` at `5d922d5`.

## Goal

When the API refuses a change to a notification or template managed by an integration, it answers 400 with the
reason, instead of 500 "Failed to …". `templates.delete` also answers permission errors as the other endpoints do.

## Current behaviour

- `TransactionalNotificationService.UpdateNotification` (`internal/service/transactional_service.go:220-233`) refuses,
  for a notification whose `IntegrationID` is non-empty, a changed `name` and any `channels`, with a plain
  `fmt.Errorf` ("cannot update name of integration-managed notification: …", "cannot update channels of …").
- `DeleteNotification` (`:490-495`) refuses a managed notification the same way.
- `TemplateService.DeleteTemplate` (`internal/service/template_service.go:483-485`) refuses a managed template the same
  way.
- `handleUpdate` and `handleDelete` of `transactional_handler.go` (`:179-193`, `:220-231`) map permission errors and
  "not found" (and "invalid template" on update) and answer 500 for the rest. `TemplateHandler.handleDelete`
  (`template_handler.go:217-224`) maps only `ErrTemplateNotFound`, so its permission and API key errors answer
  500 too, unlike `handleUpdate` (`:169`).
- All of this shipped in v43.0 (and earlier). The console hides Delete for managed notifications and templates, omits
  `channels` and disables the name for a managed notification, so only API callers hit it.
- No caller depends on the refusal text: only the handlers call these service methods; the Supabase integration writes
  through the repositories.

## Design

1. Service: the four refusals return `domain.NewValidationError(...)` with the same message (the error then reads
   "validation error: cannot …"). `tracing.MarkSpanError` stays where it is.
2. Handlers (`transactional.update`, `transactional.delete`, `templates.delete`): directly after the permission check,
   before the substring and not-found checks, a new `writeValidationError` helper in `internal/http/utils.go`
   (`errors.As` against a value `domain.ValidationError`, 400 with `validationErr.Error()`, no logging, as
   `annotation_handler.go:196`). `templates.delete` also
   calls `writePermissionError` first, as its `handleUpdate` does.
3. `CHANGELOG.md`: a `Fix` line under 44.0: "`transactional.update`, `transactional.delete` and `templates.delete` now
   answer 400 with the reason, instead of 500, when they refuse to change the name or channels of a notification
   managed by an integration, or to delete such a notification or template. `templates.delete` also answers a missing
   permission with 403 and a revoked API key with 401, as the other endpoints do, instead of 500."
4. Docs: none; the API reference does not list these errors.

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `internal/service/transactional_service.go` | `internal/service/transactional_service_managed_test.go` (new, a focused test rather than a `checkErr` field in the two large tables) | Managed notification: update with a changed name → `ValidationError` (`errors.As`) naming the name; update with the stored name and channels → `ValidationError` naming channels (so the name check cannot stop it); update with the stored name and a description → success, `Update` called; delete → `ValidationError` (extends `Error_IntegrationManagedNotification`). Strict mocks pin that no template read, `Update` or `Delete` happens |
| `internal/service/template_service.go` | `internal/service/template_service_test.go` | the managed-template delete case also asserts `errors.As` a `ValidationError` |
| `internal/http/transactional_handler.go` | `internal/http/transactional_handler_test.go` | update and delete: the service returns a `%w`-wrapped `ValidationError` → 400 with `validationErr.Error()` as `error` (decoded in a dedicated test, since the table runners check the body only on 200); unrelated errors still → 500 (existing) |
| `internal/http/template_handler.go` | `internal/http/template_handler_test.go` | delete: wrapped `ValidationError` → 400 with its text; `PermissionError` → 403 with `resource`/`permission`; other error → 500 (existing) |
| full stack | `tests/integration/transactional_handler_test.go`, "Create and Edit Permissions" group, Transactional write only | managed notification: update with `channels` → 400 (refused before the template read, else 403); update with a changed `name` → 400; delete → 400; afterwards a get answers 200 with the name, template and `integration_id` unchanged |
| full stack | `tests/integration/template_api_test.go`, new test | managed template (factory with `IntegrationID`): delete → 400 and the template still exists; a member without Templates write → 403 |

## Known, not handled

- The transactional handlers still map "not found" by substring before "invalid template", so an update naming an
  unknown template answers 404 "Notification not found".

## Test commands

The `tests/compose.test.yaml` stack (Postgres, Mailpit) must be up.

```
go test ./internal/service/ -run 'TestTransactionalNotificationService|TestTemplateService' && go test ./internal/http/ -run 'TestTransactional|TestTemplate'
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run 'TestTransactionalHandler|TestTemplateIntegrationManaged' -v
make test-unit
```
