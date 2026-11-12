# Plan — console redesign, server part 5: logs (spec 8)

## Need

The redesigned Logs page (24-hour default period, clickable counts by status, filter by
origin, incoming webhooks by provider) needs, before its console PR:

1. `messages.list` gains `origin`, `created_after` and `created_before`;
2. a new route `messages.count`, counting by status for the same filters;
3. `inboundWebhookEvents.list` gains `source`.

Spec (reviewed): "Spec 8 · Logs", "Côté serveur" and the server table of
"Fichiers, tests et livraison".

Not here (separate "published server defects" thread, agreed on 12 Nov 2026): the Fix of
`inboundWebhookEvents.list` refusing `auth_email` and `before_user_created`, and the Fix of
`webhookSubscriptions.deliveries` ignoring `status`. Both touch the same functions as this
PR; the second to merge rebases.

No table, column, index, migration or environment variable.

## Changes

### 1. `MessageFilter` and `applyMessageFilters`

- New `domain.MessageFilter` holding every filter field of `MessageListParams` (all fields but
  `Cursor` and `Limit`), with `FromQuery(url.Values) error` (parsing only), `Validate() error`
  (the filter rules of today's `MessageListParams.Validate`, plus the new ones) and
  `RequireBound() error` (for `messages.count`).
- `MessageListParams` stays flat (99 struct literals in tests would break with an embedded
  type). It gains `Filter() MessageFilter` (copies its filter fields) and `setFilter(f)`
  (the reverse, used by `FromQuery`). `MessageListParams.FromQuery` parses cursor and limit,
  calls `MessageFilter.FromQuery`, copies the result back, then `Validate`;
  `MessageListParams.Validate` keeps the limit rules and calls `p.Filter().Validate()`.
- A reflection test checks that every field of `MessageFilter` exists in `MessageListParams`
  with the same name, type and JSON tag, and that `MessageListParams` has no other field than
  those plus `Cursor` and `Limit`; a second test fills **every** filter field of
  `MessageListParams` through `reflect` (string, `*bool`, `*time.Time`; `t.Fatalf` on any
  other kind, so a new field type cannot be skipped), then checks field by field that
  `Filter()` and `setFilter` carry each one (review S1).
- `MessageFilter.Validate` has a value receiver and never normalises (so
  `p.Filter().Validate()` loses nothing); normalisation (limit defaults) stays in
  `MessageListParams.Validate` (review N2).
- The handler of `messages.count` uses one helper, `ParseMessageCountQuery(url.Values) (MessageFilter, error)`,
  which parses, validates and checks the bound, so `FromQuery` stays parse-only only as an
  internal step (review N3).
- Accepted `WebhookSource` values live in one exported slice next to the constants, used by
  `Validate` (review N6).

Review of this plan: `reviews/console-redesign-server/plan-5-review.md` in the
project files. Findings S1, S2, N2 to N9 are integrated; N1 noted (62 of the 99 literals name
a filter field: the flat struct is kept anyway). N8: the deliveries `status` integration case
lands with the other thread.
- Repository: the conditions of `ListMessages` (`message_history_postgre.go:755-875`) move to
  `applyMessageFilters(sq.SelectBuilder, domain.MessageFilter) sq.SelectBuilder`, called by
  `ListMessages` and by the new `CountMessages`. Existing `ListMessages` tests keep passing
  unchanged (same SQL).

### 2. `messages.list`: `origin`, `created_after`, `created_before`

| Parameter | Condition |
| --- | --- |
| `origin=broadcast` | `broadcast_id IS NOT NULL` |
| `origin=automation` | `automation_id IS NOT NULL` |
| `origin=transactional` | `transactional_notification_id IS NOT NULL` |
| `origin=other` | the three `IS NULL` |
| `created_after` | `created_at >= …` (RFC 3339) |
| `created_before` | `created_at <= …` (RFC 3339) |

`Validate` errors (400) for an unknown origin ("origin must be broadcast, automation,
transactional or other") and for `created_after` after `created_before`. `sent_after` and
`sent_before` stay as they are. `automation_id` and `transactional_notification_id` filters
(server parts 1 and 4) are part of `MessageFilter`; if either is not merged when this one
starts, this PR rebases on it.

### 3. `GET /api/messages.count` (new)

- Parameters: `workspace_id` and the filters of `messages.list`, read by
  `MessageFilter.FromQuery` (`cursor` and `limit` ignored), validated by `Validate` then
  `RequireBound`: 400 "messages.count needs created_after or an indexed filter" without
  `created_after` and without any of `id`, `contact_email`, `broadcast_id`, `automation_id`,
  `transactional_notification_id`, `template_id`. `list_id` and `external_id` alone need
  `created_after`.
- Response `{ "total", "delivered", "opened", "clicked", "bounced", "failed", "complained" }`,
  each following the condition of the `is_*=true` filter of the same name.
- One query: `SELECT COUNT(*), COUNT(*) FILTER (WHERE delivered_at IS NOT NULL), …` with
  `applyMessageFilters`, on the workspace connection.
- Layers: `MessageHistoryRepository.CountMessages(ctx, workspaceID, filter) (*domain.MessageCounts, error)`;
  `MessageHistoryService.CountMessages` with the `ListMessages` permission check
  (`message_history.read`); `handleCount` in `message_history_handler.go` modelled on
  `handleList`; interfaces and mocks regenerated.
- `messages.count` in `AuditExcludedRoutes` next to `messages.list`.
- Not in `openapi/` (same level as `messages.list`).

### 4. `inboundWebhookEvents.list`: `source`

`InboundWebhookEventListParams` gains `Source`, read by `FromQuery`, validated against the
`WebhookSource` values (400 otherwise); `ListEvents` adds `source = $n`.

## Files and tests (TDD: tests first)

| File | Change | Test file | Cases |
| --- | --- | --- | --- |
| `internal/domain/message_history.go` | `Origin`, `CreatedAfter`, `CreatedBefore`; `MessageFilter` (`FromQuery`, `Validate`, `RequireBound`); `Filter()`, `setFilter`; `MessageCounts`; `CountMessages` on both interfaces | `internal/domain/message_history_test.go` | `FromQuery` reads `origin`, `created_after`, `created_before`; unknown origin refused; `created_after` after `created_before` refused; reflection test of the field sets; round-trip test; bound refused without `created_after` nor indexed filter, refused with `list_id` or `external_id` alone, accepted with `created_after` or each indexed filter |
| `internal/repository/message_history_postgre.go` | `applyMessageFilters`; `CountMessages` | `internal/repository/message_history_postgre_test.go` | table test of `applyMessageFilters(…).ToSql()` for each origin, `created_after`/`created_before` and the existing filters (review N9); `CountMessages` builds every count with the same conditions as `ListMessages` for one filter; existing tests unchanged |
| `internal/service/message_history_service.go` | `CountMessages` | `internal/service/message_history_service_test.go` | authentication failure returned; permission error without `message_history.read`; repository called with the filter received |
| `internal/http/message_history_handler.go` | `messages.count` route | `internal/http/message_history_handler_test.go` | non-GET → 405; unauthenticated → 401; no `workspace_id` → 400; no bound → 400; `external_id` alone → 400; unknown origin → 400; `cursor` and `limit` ignored; permission → 403; service error → 500; JSON with every count field (count-agnostic name, or `require.Len`) |
| `internal/domain/audit_actions.go` | `messages.count` excluded | `internal/http/audit_routes_test.go` (existing) | passes |
| `internal/domain/inbound_webhook_event.go` | `Source`; one slice of accepted sources | `internal/domain/inbound_webhook_event_test.go` | `source` read and validated; each accepted source accepted (table over the slice, plus a check that the slice has every `WebhookSource` constant); unknown source refused |
| `internal/repository/inbound_webhook_event_postgres.go` | `source` condition | `internal/repository/inbound_webhook_event_postgres_test.go` | condition present with the filter, absent without |
| Interfaces and mocks | `CountMessages` | regenerated per interface: `go run github.com/golang/mock/mockgen@v1.6.0 -destination internal/domain/mocks/mock_message_history_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain MessageHistoryRepository`, same for `MessageHistoryService` (review N7) | — |
| `tests/integration/logs_filters_test.go` (new) | — | same | `TestLogsFilters` (review S2): messages built with inline `MessageHistoryOption` closures so each status subset differs (every count non-zero and all different) plus one message outside the `created_after` window; each `origin` returns only its own rows; `created_after`/`created_before` bound; `messages.count` with `created_after` and `origin` returns exact counts, each equal to the list length for its `is_*=true`; `broadcast_id` alone → 200; no bound → 400; `inboundWebhookEvents.list?source=` returns that provider's events only |
| `CHANGELOG.md` | Feature line | — | — |

CHANGELOG (short; the console PR extends it with the screen):

- **Feature**: The new `messages.count` API endpoint counts messages by status for the same
  filters as `messages.list`, which also takes `origin`, `created_after` and `created_before`;
  `inboundWebhookEvents.list` filters by `source`.

## Out of scope

Console (including the audit phrases test, which ships with the console PR), docs PR,
`EXPLAIN (ANALYZE, BUFFERS)` of `messages.count` on 30 days (console PR description).

## Verification

```bash
make test-domain test-repo test-service test-http
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration -run 'TestLogsFilters' -v
```

Then `make test-unit` once before review.
