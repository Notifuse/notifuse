# Broadcasts list without Templates read plan

**Status**: plan reviewed, implemented
**Date**: 2026-11-12
**Target**: 44.0 (`## [44.0] - unreleased` on `dev`). No migration, no environment variable, no console change, no
docs change.
**Base**: `origin/dev` at `6688f19`.

## Goal

`broadcasts.list` with `with_templates` no longer tries, and logs a refusal for, each variation's template when the
caller has no Templates read. Follow-up of #536, which listed this under "Known, not handled".

## Current behaviour

- `BroadcastsPage` lists broadcasts with `with_templates: true` (`console/src/pages/BroadcastsPage.tsx:1378-1393`),
  whatever the member's permissions.
- `BroadcastService.ListBroadcasts` (`internal/service/broadcast_service.go:275-327`) authenticates the caller
  (`userWorkspace`, `:278`), then, with `WithTemplates`, calls `templateSvc.GetTemplateByID` per variation. Without
  Templates read each call answers a permission error, which the loop logs at error level ("Failed to fetch template
  for broadcast variation") and skips: the response carries the broadcasts without templates.
- So the answer is already right; the cost is one refused call and one error log per variation, per page load, for
  the console and for any API key without Templates read.

## Design

The fix belongs in the service, which already holds the caller's permissions, rather than in the console (reviewed
alternative: making the page wait for `workspaces.members` before listing, which would add a serial request for
everyone, a new loading state and a query key that eight prefix invalidations depend on).

1. `ListBroadcasts`: fetch the variations' templates only when
   `params.WithTemplates && userWorkspace.HasPermission(domain.PermissionResourceTemplates, domain.PermissionTypeRead)`.
   Without the permission the broadcasts come back without templates, as they already did, without the refused calls
   or the logs. A comment says why.
2. `CHANGELOG.md`: no new line. #536's `Fix` line (44.0, unreleased) already says "these screens no longer request
   templates such a member cannot read", which this makes true for the broadcasts list too.

## Tests

| Implementation | Test file | Cases |
|---|---|---|
| `internal/service/broadcast_service.go` | `internal/service/broadcast_service_test.go` | a helper authenticates a member with given permissions; without Templates read, `WithTemplates`: `GetTemplateByID` is not called (strict gomock) and the broadcast comes back with its variation, template nil; the two existing `WithTemplates` cases authenticate a member with Templates read (with it, `GetTemplateByID` is called, as now); an owner without explicit permissions gets the templates |

## Known, not handled

- `broadcasts.get` with `with_templates` (`internal/http/broadcast_handler.go:187-205`) does the same in the
  handler, which has no `userWorkspace`; the console never asks it for templates.
- `useWorkspacePermissions` keeps the previous workspace's permissions until the next answer when the workspace
  changes while a page stays mounted.

## Test commands

```
go test ./internal/service/ -run 'TestBroadcastService_ListBroadcasts' -v
make test-service
```
