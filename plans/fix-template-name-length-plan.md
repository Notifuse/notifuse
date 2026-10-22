# Fix: names and IDs over the length limit fail only when saved (#463)

Created 2026-10-22 on `dev` @ `5499619`. `VERSION` is `42.0`, unreleased.
Branch `fix/template-name-length`, one PR to `dev`; a second PR to `dev` on the docs repository for the API reference.
Amended after Gemini's counter-review (raw answer `reviews/fix-template-name-length-gemini-brut-plan.md` in the project files, each remark checked in `reviews/fix-template-name-length-gemini-verif-plan.md`).

## Problem

- **Template name.** `Template.Validate`, `CreateTemplateRequest.Validate` and `UpdateTemplateRequest.Validate` (`internal/domain/template.go:275`, `:623`, `:780`) refuse a name over 32 bytes. The column is `templates.name VARCHAR(255)` (`internal/database/init.go:155`), and lists, segments, broadcasts, automations and template blocks all accept 255. The console's name field has no limit, so the author learns it only from the error when saving.
- **Template ID.** The console fills the ID with `kebabCase(name)` (`console/src/components/templates/CreateTemplateDrawer.tsx:806`), never cut to the 32 characters `validateTemplateID` allows (`template.go:32`). A long name gives an ID the API refuses. Same for the ID of a duplicated template (`kebabCase(`${name}-copy`)`, `:485`).
- **Transactional notification API identifier.** The console fills it with `generateApiId(name)` (`UpsertTransactionalNotificationDrawer.tsx:17`), no length cap. `CreateTransactionalRequest.Validate` (`internal/domain/transactional.go:317`) checks no length, so the insert into `transactional_notifications.id VARCHAR(32)` fails and the handler answers **500 "Failed to create notification"** (`internal/http/transactional_handler.go:152`).
- **Transactional notification name.** Neither `CreateTransactionalRequest.Validate` nor `UpdateTransactionalRequest.Validate` (`transactional.go:375`) bounds it, and the console field has no maximum, so a name over the column's 255 (`init.go:234`) ends in the same 500 on create and on update (`transactional_handler.go:193`).
- **Workspace name.** `CreateWorkspaceRequest.Validate` and `UpdateWorkspaceRequest.Validate` (`internal/domain/workspace.go:1483`, `:1607`) refuse a name over 32 bytes; the console's create page and General settings have no maximum. Bytes, not characters: a 30-character name with three accented letters is refused.

## Decisions (Pierre, 2026-10-22)

1. Template name limit raised to 255. *(recommended, awaiting the answer; the alternative keeps 32 and changes only the console)*
2. IDs stay at 32, no migration: the console cuts generated IDs to 32 and limits the fields; the API answers a clear 400 for a transactional ID over 32 instead of the 500. Length only, no format check added to the API.
3. Workspace name: the console limits the field to 32 with a counter.
4. No comment on the issue; the PR closes it.

No table, column or environment variable changes.

## Counting characters, not bytes

PostgreSQL's `VARCHAR(n)` counts characters on a UTF-8 database. The API checks touched here count with `utf8.RuneCountInString`, so they agree with the column and with the console counter:

- the console's `maxLength` and `max` rule count UTF-16 code units, which is never fewer than the rune count (equal outside astral characters such as emoji), so whatever the console lets through, the API accepts;
- the template-name and transactional-ID checks are new or wider, so nothing that was accepted before is refused;
- the workspace-name check moves from bytes to runes at the same 32: only names that were refused before (accented, under 32 characters, over 32 bytes) become accepted. Without it, the console counter would accept names the API refuses. This is the one API change under decision 3, and it only relaxes.

`validateTemplateID` keeps `len`: its pattern only admits ASCII.

## Changes

### Backend

**B1. `internal/domain/template.go`.** New `validateTemplateName(name string) error`, next to `validateTemplateID`: `"name is required"` when empty, `"name length must be between 1 and 255"` when `utf8.RuneCountInString(name) > 255`. Used by `Template.Validate`, `CreateTemplateRequest.Validate` and `UpdateTemplateRequest.Validate`, each keeping its own prefix (`invalid template: `, `invalid create template request: `, `invalid update template request: `) so the messages stay as they are apart from the number.

Tests, `internal/domain/template_test.go`:
- `validateTemplateName`: empty → required; 33 ASCII characters (the issue's case) → ok; 255 ASCII → ok; 256 ASCII → length error; 255 × `é` (510 bytes) → ok; 256 × `é` → length error.
- `Template.Validate`, `CreateTemplateRequest.Validate`, `UpdateTemplateRequest.Validate`: 255 characters → ok, 256 → error carrying each method's prefix.

**B2. `internal/domain/transactional.go`.**
- `CreateTransactionalRequest.Validate`, after `notification.id is required`: `NewValidationError("notification.id length must be between 1 and 32")` when `utf8.RuneCountInString(req.Notification.ID) > 32`; after `notification.name is required`: `NewValidationError("notification.name length must be between 1 and 255")` above 255 runes.
- `UpdateTransactionalRequest.Validate`: `NewValidationError("updates.name length must be between 1 and 255")` when `Updates.Name` is over 255 runes (an empty name still means "not updated").
- The handler already answers a validation error with 400 and its message (`transactional_handler.go:134`, `:174`). Update and send requests look an existing ID up, so a long ID already ends as "not found"; their ID checks stay as they are.

Tests:
- `internal/domain/transactional_test.go`, `TestCreateTransactionalRequest_Validate`: ID of 32 characters → ok; 33 → the ID length error; 32 × `é` → ok; name of 255 × `é` → ok; 256 → the name length error. `TestUpdateTransactionalRequest_Validate`: name of 255 × `é` → ok; 256 → the name length error.
- `internal/http/transactional_handler_test.go`: create with a 33-character ID → 400 with the message, service not called; update with a 256-character name → 400, service not called.

**B3. `internal/domain/workspace.go`.** The name checks of `CreateWorkspaceRequest.Validate` and `UpdateWorkspaceRequest.Validate` count runes. Messages unchanged.

Tests, `internal/domain/workspace_test.go`: for both requests, 32 × `é` → ok, 33 ASCII → `name length must be between 1 and 32`.

**B4. `internal/service/demo_automation_templates.go` / `_test.go`.** The comment "Template.Name is capped at 32 characters (much tighter than a list's 255)" and the test asserting demo template names fit 32 are updated to the new rule (255, counted in characters), so the demo keeps guarding against a name the API would refuse.

### Console

**C1. `console/src/utils/identifiers.ts` (new).** `truncateIdentifier(id, maxLength)`: an id within the limit is returned unchanged (so short generated IDs stay exactly as today, a lone `_` included); a longer one is cut to `maxLength` and the `-` and `_` left at the end of the cut are removed, so it never ends as `welcome-email-for-`.

Tests, `console/src/utils/identifiers.test.ts`: id at or under the limit unchanged, trailing separator included; longer id cut to the limit; a cut landing on `-` or `_` drops them; empty string stays empty.

**C2. `CreateTemplateDrawer.tsx`.**
- Name: `maxLength={255}`, `showCount`, and a `max: 255` rule with a translated message (the field can still receive a longer value programmatically, from a duplicate).
- ID: generated as `truncateIdentifier(kebabCase(name), 32)`; `maxLength={32}`, `showCount`, `max: 32` rule with a translated message.
- Duplicate: ID `${base}-copy` with `base = truncateIdentifier(kebabCase(name), 27)`, so the `-copy` suffix survives the cut and the copy does not collide with the original's truncated ID; `copy` when `base` is empty (a name like `!!!`, which `kebabCase` empties), as today, instead of `-copy`.
- The `template.id || kebabCase(template.name)` fallbacks (`:393`, `:458`) are wrapped in `truncateIdentifier(…, 32)` too. The ID of an existing template stays read-only.

Tests, `CreateTemplateDrawer.test.tsx`:
- typing a 60-character name fills an ID of at most 32 characters, with no trailing hyphen;
- duplicating a template with a long name gives an ID ending in `-copy` of at most 32 characters;
- duplicating a template named `!!!` gives the ID `copy`;
- a duplicate whose name exceeds 255 (`name` of 252 characters + ` copy`) is not sent to the API and the name field shows the length message.

**C3. `UpsertTransactionalNotificationDrawer.tsx`.** `generateApiId` returns `truncateIdentifier(…, 32)`; the ID field gets `maxLength={32}`, `showCount`, `max: 32` rule; the name field `maxLength={255}`, `showCount`, `max: 255` rule. `utm_content` is filled from the same value as the ID, as today.

Tests, `UpsertTransactionalNotificationDrawer.test.tsx`: typing a long name fills an ID and `utm_content` of at most 32 characters with no trailing underscore; the create payload carries that ID; the name input's `maxlength` is 255.

**C4. Workspace name, `pages/CreateWorkspacePage.tsx` and `components/settings/GeneralSettings.tsx`.** `maxLength={32}`, `showCount`, `max: 32` rule with a translated message.

Tests: `GeneralSettings.test.tsx`: the name input refuses input beyond 32 characters (its `maxlength` attribute is 32) and a stored name over 32 (set as the form's initial value) blocks the save with the message. `CreateWorkspacePage`: a new `pages/CreateWorkspacePage.test.tsx` checking the name input's `maxlength` is 32, if the page renders in isolation with the existing test helpers; otherwise covered by the smoke test only, and said so in the PR.

**C5. i18n.** `npm run lingui:extract` for the new messages; French and other locale entries filled as the existing ones are.

### API reference

**D1.** `openapi/components/schemas/template.yaml`: `name` `maxLength` 32 → 255 in `Template`, `CreateTemplateRequest`, `UpdateTemplateRequest`; `openapi.json` rebundled (`make openapi-bundle`; by hand on the three entries if the bundler is unavailable offline, then checked by diffing against a bundle).
**D2.** Docs repository, branch `docs/template-name-length` from `dev`: the same three entries in `openapi.json`. PR to `dev`.

### CHANGELOG (42.0)

> - **Improvement**: Template names can now be up to 255 characters (#463). The console counts characters in the names and IDs of templates and transactional notifications and in workspace names, and an ID filled in from a long name now fits the 32-character limit. A transactional notification sent to the API with an ID over 32 characters or a name over 255 is now refused with the reason (400).

## Not in scope

- Longer IDs (columns `VARCHAR(32)` in `templates`, `transactional_notifications`, `message_history`, `broadcasts.winning_template` and others): decision 2.
- A format check on transactional IDs in the API (the console's `^[a-z0-9_]+$`): would refuse IDs integrations may already send.
- The template name is stamped into the email's `mj-title`, and into `mj-preview` when the subject preview is empty (`template_service.go:46`). Unchanged; a longer name only makes that fallback preheader longer, and inbox clients cut it.
- `generateApiId` drops accented letters (`\w` is ASCII in JS) where `kebabCase` transliterates them, and keeps a leading `_` from a leading space. Unchanged.
- API reference for transactional notification and workspace limits: `transactional.create/update` and `workspaces.create/update` are not in the OpenAPI spec (`openapi/openapi.yaml` documents `transactional.send` only).

## Test commands

```
make test-domain
make test-service
make test-http
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestTemplate|TestTransactional|TestWorkspace' -v
make test-integration
cd console && npm test -- --run && npm run lint && npx tsc --noEmit && npm run lingui:extract
```

Integration tests added:
- `tests/integration/template_api_test.go`: create a template whose name is 255 × `é` → created, read back unchanged; update it (same name, new version) → saved; update it to 256 characters → 400 with the length message.
- `tests/integration/transactional_handler_test.go`: create with a 33-character ID → 400 with the message (was 500); with 32 characters → created; create with a 256-character name → 400 (was 500); update to a 256-character name → 400 (was 500); 255 × `é` → saved.
- `tests/integration/workspace_test.go`: create a workspace named with 32 accented characters → created, read back unchanged.
