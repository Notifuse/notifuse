# Webhook Events on Template CRUD

## Status: Feasibility Investigation (complete)

## Summary

Adding webhook events for template CRUD operations is **highly feasible** and follows an established pattern already used by 5 other entity types (contacts, lists, segments, emails, custom_events).

## Current Webhook Architecture

All outgoing webhook events are generated via **PostgreSQL AFTER triggers** on workspace tables. When a row is inserted/updated/deleted, the trigger function:

1. Determines the event type (e.g. `contact.created`)
2. Builds a JSONB payload
3. Queries `webhook_subscriptions` for matching subscriptions
4. Inserts rows into `webhook_deliveries` with `status='pending'`

The `WebhookDeliveryWorker` polls every 10s, signs the payload with HMAC-SHA256 (Standard Webhooks spec), and delivers via HTTP POST with exponential backoff retries (up to 10 attempts over ~48h).

### Key Files

| Layer | File |
|---|---|
| Event types | `internal/domain/webhook_subscription.go` (line 79: `WebhookEventTypes`) |
| Delivery worker | `internal/service/webhook_delivery_worker.go` |
| Trigger examples | `internal/migrations/v19.go` (contacts, lists, segments, emails, custom_events) |
| Subscription service | `internal/service/webhook_subscription_service.go` |
| Template service | `internal/service/template_service.go` |
| Template repo | `internal/repository/template_postgres.go` |
| Template domain | `internal/domain/template.go` |

## New Event Types

```
template.created
template.updated
template.deleted
```

## Design Consideration: Template Versioning

Templates use an **INSERT-based versioning model** — updates don't `UPDATE` the row, they `INSERT` a new row with an incremented `version`. Deletes are **soft deletes** (set `deleted_at`). This affects trigger logic:

| Operation | DB Operation | Trigger Detection |
|---|---|---|
| Create | `INSERT` | `AFTER INSERT` where `version = 1` and `deleted_at IS NULL` |
| Update | `INSERT` (new version) | `AFTER INSERT` where `version > 1` and `deleted_at IS NULL` |
| Delete | `UPDATE` (set deleted_at) | `AFTER UPDATE` where `deleted_at` transitions from `NULL` to non-`NULL` |

This is different from contacts (which use standard INSERT/UPDATE/DELETE) but straightforward in PL/pgSQL:

```sql
IF TG_OP = 'INSERT' THEN
    IF NEW.version = 1 THEN
        event_kind := 'template.created';
    ELSE
        event_kind := 'template.updated';
    END IF;
ELSIF TG_OP = 'UPDATE' THEN
    IF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL THEN
        event_kind := 'template.deleted';
    ELSE
        RETURN NEW; -- skip non-delete updates
    END IF;
END IF;
```

## Design Decision: Payload Content

Templates can carry large MJML trees and compiled HTML in their `email` JSONB column. Two options:

### Option A: Lightweight payload (recommended)

Send only metadata — consumer calls back for full details if needed:

```json
{
  "template": {
    "id": "welcome-email",
    "name": "Welcome Email",
    "version": 3,
    "channel": "email",
    "category": "transactional",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

**Pros**: Small payload, fast delivery, no risk of exceeding webhook size limits.
**Cons**: Consumer needs an extra API call for full template content.

### Option B: Full payload

Send the entire template object including MJML tree and compiled HTML.

**Pros**: Consumer has everything in one delivery.
**Cons**: Payloads could be very large (MJML trees + compiled HTML), may cause delivery timeouts or exceed consumer limits.

## What Needs to Change

### 1. Domain layer (~3 lines)

Add 3 event types to `WebhookEventTypes` in `internal/domain/webhook_subscription.go`.

### 2. New database migration (~80 lines SQL)

New migration file (e.g. `internal/migrations/v28.go`) with:
- `webhook_templates_trigger()` function
- `AFTER INSERT OR UPDATE` trigger on `templates` table
- Following the exact same pattern as `webhook_contacts_trigger()` in v19

### 3. Tests (~50 lines)

- Update `WebhookEventTypes` test expectations in `internal/domain/webhook_subscription_test.go`
- Add migration tests in `internal/migrations/v28_test.go`

### 4. Frontend: zero changes

The subscription UI dynamically reads available event types from the `GET /api/webhookSubscriptions.eventTypes` endpoint, which returns `WebhookEventTypes`. New types will appear automatically.

### 5. No changes needed to

- Delivery worker (generic, processes any event type)
- Signing/retry logic
- Subscription service or HTTP handlers
- Template service or repository (triggers are at DB level)

## Out of Scope: Template Blocks

Template blocks are stored inside `workspace.Settings` (not a separate table), so adding webhook triggers for block CRUD would require an **application-level approach** — inserting into `webhook_deliveries` from Go code in the service layer. This is a different pattern from the rest and should be a separate initiative if desired.

## Risk Assessment

- **Low risk**: The pattern is battle-tested with 5 other entity types
- **Only nuance**: The version-based INSERT pattern requires careful trigger logic (see above)
- **No breaking changes**: Existing subscriptions are unaffected; new events only fire for subscriptions that opt in
