# Semantic Timeline Events - Migration Plan

## Status: ✅ MERGED INTO V18 MIGRATION

**Note**: This functionality has been merged into the v18 migration along with custom events. See `plans/custom-events.md` for the complete implementation.

## Overview

Update ALL internal timeline events from operation-based naming to semantic event names aligned with the custom_events pattern:
- **Contact List**: `insert_contact_list` → `list.subscribed`, `update_contact_list` → `list.unsubscribed`, etc.
- **Segments**: `join_segment` → `segment.joined`, `leave_segment` → `segment.left`
- **Contacts**: `insert_contact` → `contact.created`, `update_contact` → `contact.updated`

## Goals

1. **Align with custom events naming**: Use dotted namespace format across all internal events
2. **Improve automation UX**: Clear, semantic event names for trigger rules
3. **Maintain consistency**: Unified naming pattern across contact_list, segments, and contacts
4. **Historical consistency**: Migrate all existing timeline entries to new naming

## Event Naming Strategy

### Pattern: `{entity}.{action}`

Using dotted format to align with custom events like `payment.succeeded`, `subscription.created`:

### Contact List Events

| Event Name | Trigger Condition | Description |
|------------|------------------|-------------|
| `list.subscribed` | INSERT with status='active' | Initial active subscription |
| `list.pending` | INSERT with status='pending' | Double opt-in initiated |
| `list.confirmed` | UPDATE pending→active | Double opt-in confirmed |
| `list.unsubscribed` | UPDATE →unsubscribed | User unsubscribed |
| `list.bounced` | UPDATE →bounced | Hard bounce occurred |
| `list.complained` | UPDATE →complained | Spam complaint |
| `list.resubscribed` | UPDATE (unsub/bounced/complained)→active | Reactivated subscription |
| `list.removed` | UPDATE deleted_at set | Soft deleted from list |

### Segment Events

| Event Name | Old Name | Trigger Condition | Description |
|------------|----------|------------------|-------------|
| `segment.joined` | `join_segment` | INSERT into contact_segments | Contact entered segment |
| `segment.left` | `leave_segment` | DELETE from contact_segments | Contact left segment |

### Contact Events

| Event Name | Old Name | Trigger Condition | Description |
|------------|----------|------------------|-------------|
| `contact.created` | `insert_contact` | INSERT into contacts | New contact created |
| `contact.updated` | `update_contact` | UPDATE contacts (with changes) | Contact properties changed |

### Architecture Principles

- **Semantic naming**: Event names describe business actions, not database operations
- **Namespace consistency**: `list.` prefix aligns with custom events pattern (`payment.`, `subscription.`)
- **Status-aware**: Different event names for different status transitions
- **Operation field**: Still use `operation='insert'` or `operation='update'` for filtering

---

## Database Trigger Implementation

### Updated Trigger Function

```sql
CREATE OR REPLACE FUNCTION track_contact_list_changes()
RETURNS TRIGGER AS $$
DECLARE
    changes_json JSONB := '{}'::jsonb;
    op VARCHAR(20);
    kind_value VARCHAR(50);
BEGIN
    IF TG_OP = 'INSERT' THEN
        op := 'insert';

        -- Map initial status to semantic event kind
        kind_value := CASE NEW.status
            WHEN 'active' THEN 'list.subscribed'
            WHEN 'pending' THEN 'list.pending'
            WHEN 'unsubscribed' THEN 'list.unsubscribed'  -- Imported as unsubscribed
            WHEN 'bounced' THEN 'list.bounced'            -- Imported as bounced
            WHEN 'complained' THEN 'list.complained'      -- Imported as complained
            ELSE 'list.subscribed'  -- Default fallback
        END;

        changes_json := jsonb_build_object(
            'list_id', jsonb_build_object('new', NEW.list_id),
            'status', jsonb_build_object('new', NEW.status)
        );

    ELSIF TG_OP = 'UPDATE' THEN
        op := 'update';

        -- Handle soft delete (removed from list)
        IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at AND NEW.deleted_at IS NOT NULL THEN
            kind_value := 'list.removed';
            changes_json := jsonb_build_object(
                'deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at)
            );

        -- Handle status transitions
        ELSIF OLD.status IS DISTINCT FROM NEW.status THEN
            -- Determine semantic event based on status transition
            kind_value := CASE
                -- Confirmed double opt-in
                WHEN OLD.status = 'pending' AND NEW.status = 'active' THEN 'list.confirmed'

                -- Resubscription from unsubscribed/bounced/complained
                WHEN OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active'
                    THEN 'list.resubscribed'

                -- Unsubscribe action
                WHEN NEW.status = 'unsubscribed' THEN 'list.unsubscribed'

                -- Bounce event
                WHEN NEW.status = 'bounced' THEN 'list.bounced'

                -- Complaint event
                WHEN NEW.status = 'complained' THEN 'list.complained'

                -- Moved to pending (rare edge case)
                WHEN NEW.status = 'pending' THEN 'list.pending'

                -- Default fallback for any other transition to active
                WHEN NEW.status = 'active' THEN 'list.subscribed'

                -- Catch-all for unexpected transitions
                ELSE 'list.status_changed'
            END;

            changes_json := jsonb_build_object(
                'status', jsonb_build_object('old', OLD.status, 'new', NEW.status)
            );
        ELSE
            -- No relevant changes, skip timeline entry
            RETURN NEW;
        END IF;
    END IF;

    -- Insert timeline entry with semantic kind
    INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
    VALUES (NEW.email, op, 'contact_list', kind_value, NEW.list_id, changes_json, CURRENT_TIMESTAMP);

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

### Updated Segment Trigger Function

```sql
CREATE OR REPLACE FUNCTION track_contact_segment_changes()
RETURNS TRIGGER AS $$
DECLARE
    changes_json JSONB := '{}'::jsonb;
    op VARCHAR(20);
    kind_value VARCHAR(50);
BEGIN
    IF TG_OP = 'INSERT' THEN
        op := 'insert';
        kind_value := 'segment.joined';
        changes_json := jsonb_build_object(
            'segment_id', jsonb_build_object('new', NEW.segment_id),
            'version', jsonb_build_object('new', NEW.version),
            'matched_at', jsonb_build_object('new', NEW.matched_at)
        );

    ELSIF TG_OP = 'DELETE' THEN
        op := 'delete';
        kind_value := 'segment.left';
        changes_json := jsonb_build_object(
            'segment_id', jsonb_build_object('old', OLD.segment_id),
            'version', jsonb_build_object('old', OLD.version)
        );
        INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
        VALUES (OLD.email, op, 'contact_segment', kind_value, OLD.segment_id, changes_json, CURRENT_TIMESTAMP);
        RETURN OLD;
    END IF;

    INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
    VALUES (NEW.email, op, 'contact_segment', kind_value, NEW.segment_id, changes_json, CURRENT_TIMESTAMP);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

### Updated Contact Trigger Function

```sql
CREATE OR REPLACE FUNCTION track_contact_changes()
RETURNS TRIGGER AS $$
DECLARE
    changes_json JSONB := '{}'::jsonb;
    op VARCHAR(20);
BEGIN
    IF TG_OP = 'INSERT' THEN
        op := 'insert';
        changes_json := NULL;

    ELSIF TG_OP = 'UPDATE' THEN
        op := 'update';
        -- Track all field changes (same logic as before, just with new kind name)
        IF OLD.external_id IS DISTINCT FROM NEW.external_id THEN changes_json := changes_json || jsonb_build_object('external_id', jsonb_build_object('old', OLD.external_id, 'new', NEW.external_id)); END IF;
        IF OLD.timezone IS DISTINCT FROM NEW.timezone THEN changes_json := changes_json || jsonb_build_object('timezone', jsonb_build_object('old', OLD.timezone, 'new', NEW.timezone)); END IF;
        IF OLD.language IS DISTINCT FROM NEW.language THEN changes_json := changes_json || jsonb_build_object('language', jsonb_build_object('old', OLD.language, 'new', NEW.language)); END IF;
        IF OLD.first_name IS DISTINCT FROM NEW.first_name THEN changes_json := changes_json || jsonb_build_object('first_name', jsonb_build_object('old', OLD.first_name, 'new', NEW.first_name)); END IF;
        IF OLD.last_name IS DISTINCT FROM NEW.last_name THEN changes_json := changes_json || jsonb_build_object('last_name', jsonb_build_object('old', OLD.last_name, 'new', NEW.last_name)); END IF;
        IF OLD.phone IS DISTINCT FROM NEW.phone THEN changes_json := changes_json || jsonb_build_object('phone', jsonb_build_object('old', OLD.phone, 'new', NEW.phone)); END IF;
        IF OLD.address_line_1 IS DISTINCT FROM NEW.address_line_1 THEN changes_json := changes_json || jsonb_build_object('address_line_1', jsonb_build_object('old', OLD.address_line_1, 'new', NEW.address_line_1)); END IF;
        IF OLD.address_line_2 IS DISTINCT FROM NEW.address_line_2 THEN changes_json := changes_json || jsonb_build_object('address_line_2', jsonb_build_object('old', OLD.address_line_2, 'new', NEW.address_line_2)); END IF;
        IF OLD.country IS DISTINCT FROM NEW.country THEN changes_json := changes_json || jsonb_build_object('country', jsonb_build_object('old', OLD.country, 'new', NEW.country)); END IF;
        IF OLD.postcode IS DISTINCT FROM NEW.postcode THEN changes_json := changes_json || jsonb_build_object('postcode', jsonb_build_object('old', OLD.postcode, 'new', NEW.postcode)); END IF;
        IF OLD.state IS DISTINCT FROM NEW.state THEN changes_json := changes_json || jsonb_build_object('state', jsonb_build_object('old', OLD.state, 'new', NEW.state)); END IF;
        IF OLD.job_title IS DISTINCT FROM NEW.job_title THEN changes_json := changes_json || jsonb_build_object('job_title', jsonb_build_object('old', OLD.job_title, 'new', NEW.job_title)); END IF;
        IF OLD.lifetime_value IS DISTINCT FROM NEW.lifetime_value THEN changes_json := changes_json || jsonb_build_object('lifetime_value', jsonb_build_object('old', OLD.lifetime_value, 'new', NEW.lifetime_value)); END IF;
        IF OLD.orders_count IS DISTINCT FROM NEW.orders_count THEN changes_json := changes_json || jsonb_build_object('orders_count', jsonb_build_object('old', OLD.orders_count, 'new', NEW.orders_count)); END IF;
        IF OLD.last_order_at IS DISTINCT FROM NEW.last_order_at THEN changes_json := changes_json || jsonb_build_object('last_order_at', jsonb_build_object('old', OLD.last_order_at, 'new', NEW.last_order_at)); END IF;
        IF OLD.custom_string_1 IS DISTINCT FROM NEW.custom_string_1 THEN changes_json := changes_json || jsonb_build_object('custom_string_1', jsonb_build_object('old', OLD.custom_string_1, 'new', NEW.custom_string_1)); END IF;
        IF OLD.custom_string_2 IS DISTINCT FROM NEW.custom_string_2 THEN changes_json := changes_json || jsonb_build_object('custom_string_2', jsonb_build_object('old', OLD.custom_string_2, 'new', NEW.custom_string_2)); END IF;
        IF OLD.custom_string_3 IS DISTINCT FROM NEW.custom_string_3 THEN changes_json := changes_json || jsonb_build_object('custom_string_3', jsonb_build_object('old', OLD.custom_string_3, 'new', NEW.custom_string_3)); END IF;
        IF OLD.custom_string_4 IS DISTINCT FROM NEW.custom_string_4 THEN changes_json := changes_json || jsonb_build_object('custom_string_4', jsonb_build_object('old', OLD.custom_string_4, 'new', NEW.custom_string_4)); END IF;
        IF OLD.custom_string_5 IS DISTINCT FROM NEW.custom_string_5 THEN changes_json := changes_json || jsonb_build_object('custom_string_5', jsonb_build_object('old', OLD.custom_string_5, 'new', NEW.custom_string_5)); END IF;
        IF OLD.custom_number_1 IS DISTINCT FROM NEW.custom_number_1 THEN changes_json := changes_json || jsonb_build_object('custom_number_1', jsonb_build_object('old', OLD.custom_number_1, 'new', NEW.custom_number_1)); END IF;
        IF OLD.custom_number_2 IS DISTINCT FROM NEW.custom_number_2 THEN changes_json := changes_json || jsonb_build_object('custom_number_2', jsonb_build_object('old', OLD.custom_number_2, 'new', NEW.custom_number_2)); END IF;
        IF OLD.custom_number_3 IS DISTINCT FROM NEW.custom_number_3 THEN changes_json := changes_json || jsonb_build_object('custom_number_3', jsonb_build_object('old', OLD.custom_number_3, 'new', NEW.custom_number_3)); END IF;
        IF OLD.custom_number_4 IS DISTINCT FROM NEW.custom_number_4 THEN changes_json := changes_json || jsonb_build_object('custom_number_4', jsonb_build_object('old', OLD.custom_number_4, 'new', NEW.custom_number_4)); END IF;
        IF OLD.custom_number_5 IS DISTINCT FROM NEW.custom_number_5 THEN changes_json := changes_json || jsonb_build_object('custom_number_5', jsonb_build_object('old', OLD.custom_number_5, 'new', NEW.custom_number_5)); END IF;
        IF OLD.custom_datetime_1 IS DISTINCT FROM NEW.custom_datetime_1 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_1', jsonb_build_object('old', OLD.custom_datetime_1, 'new', NEW.custom_datetime_1)); END IF;
        IF OLD.custom_datetime_2 IS DISTINCT FROM NEW.custom_datetime_2 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_2', jsonb_build_object('old', OLD.custom_datetime_2, 'new', NEW.custom_datetime_2)); END IF;
        IF OLD.custom_datetime_3 IS DISTINCT FROM NEW.custom_datetime_3 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_3', jsonb_build_object('old', OLD.custom_datetime_3, 'new', NEW.custom_datetime_3)); END IF;
        IF OLD.custom_datetime_4 IS DISTINCT FROM NEW.custom_datetime_4 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_4', jsonb_build_object('old', OLD.custom_datetime_4, 'new', NEW.custom_datetime_4)); END IF;
        IF OLD.custom_datetime_5 IS DISTINCT FROM NEW.custom_datetime_5 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_5', jsonb_build_object('old', OLD.custom_datetime_5, 'new', NEW.custom_datetime_5)); END IF;
        IF OLD.custom_json_1 IS DISTINCT FROM NEW.custom_json_1 THEN changes_json := changes_json || jsonb_build_object('custom_json_1', jsonb_build_object('old', OLD.custom_json_1, 'new', NEW.custom_json_1)); END IF;
        IF OLD.custom_json_2 IS DISTINCT FROM NEW.custom_json_2 THEN changes_json := changes_json || jsonb_build_object('custom_json_2', jsonb_build_object('old', OLD.custom_json_2, 'new', NEW.custom_json_2)); END IF;
        IF OLD.custom_json_3 IS DISTINCT FROM NEW.custom_json_3 THEN changes_json := changes_json || jsonb_build_object('custom_json_3', jsonb_build_object('old', OLD.custom_json_3, 'new', NEW.custom_json_3)); END IF;
        IF OLD.custom_json_4 IS DISTINCT FROM NEW.custom_json_4 THEN changes_json := changes_json || jsonb_build_object('custom_json_4', jsonb_build_object('old', OLD.custom_json_4, 'new', NEW.custom_json_4)); END IF;
        IF OLD.custom_json_5 IS DISTINCT FROM NEW.custom_json_5 THEN changes_json := changes_json || jsonb_build_object('custom_json_5', jsonb_build_object('old', OLD.custom_json_5, 'new', NEW.custom_json_5)); END IF;
        IF changes_json = '{}'::jsonb THEN RETURN NEW; END IF;
    END IF;

    -- Use semantic event names
    IF TG_OP = 'INSERT' THEN
        INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
        VALUES (NEW.email, op, 'contact', 'contact.created', changes_json, NEW.created_at);
    ELSE
        INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
        VALUES (NEW.email, op, 'contact', 'contact.updated', changes_json, NEW.updated_at);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

---

## Migration Implementation

**Implementation**: Merged into v18 migration (see `plans/custom-events.md`)

**Original Plan**: v19.0 (now superseded by v18)

The trigger update and historical data migration are included in the v18 migration alongside custom events to create a unified event naming convention across the platform.

```go
// NOTE: This code is now part of V18Migration in plans/custom-events.md
// This standalone migration is not needed as it was merged with custom events

package migrations

import (
    "context"
    "fmt"

    "github.com/Notifuse/notifuse/config"
    "github.com/Notifuse/notifuse/internal/domain"
)

// V18Migration includes contact_list semantic naming alongside custom events
// This was originally planned as V19Migration but merged into v18 for consistency
type V18Migration struct{}

func (m *V18Migration) GetMajorVersion() float64 {
    return 18.0
}

func (m *V18Migration) HasSystemUpdate() bool {
    return false
}

func (m *V18Migration) HasWorkspaceUpdate() bool {
    return true
}

func (m *V18Migration) ShouldRestartServer() bool {
    return false
}

func (m *V18Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
    // No system-level changes needed
    return nil
}

func (m *V18Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
    // Update contact_list trigger function with semantic naming
    _, err := db.ExecContext(ctx, `
        CREATE OR REPLACE FUNCTION track_contact_list_changes()
        RETURNS TRIGGER AS $$
        DECLARE
            changes_json JSONB := '{}'::jsonb;
            op VARCHAR(20);
            kind_value VARCHAR(50);
        BEGIN
            IF TG_OP = 'INSERT' THEN
                op := 'insert';

                -- Map initial status to semantic event kind (dotted format)
                kind_value := CASE NEW.status
                    WHEN 'active' THEN 'list.subscribed'
                    WHEN 'pending' THEN 'list.pending'
                    WHEN 'unsubscribed' THEN 'list.unsubscribed'
                    WHEN 'bounced' THEN 'list.bounced'
                    WHEN 'complained' THEN 'list.complained'
                    ELSE 'list.subscribed'
                END;

                changes_json := jsonb_build_object(
                    'list_id', jsonb_build_object('new', NEW.list_id),
                    'status', jsonb_build_object('new', NEW.status)
                );

            ELSIF TG_OP = 'UPDATE' THEN
                op := 'update';

                -- Handle soft delete
                IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at AND NEW.deleted_at IS NOT NULL THEN
                    kind_value := 'list.removed';
                    changes_json := jsonb_build_object(
                        'deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at)
                    );

                -- Handle status transitions
                ELSIF OLD.status IS DISTINCT FROM NEW.status THEN
                    kind_value := CASE
                        WHEN OLD.status = 'pending' AND NEW.status = 'active' THEN 'list.confirmed'
                        WHEN OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN 'list.resubscribed'
                        WHEN NEW.status = 'unsubscribed' THEN 'list.unsubscribed'
                        WHEN NEW.status = 'bounced' THEN 'list.bounced'
                        WHEN NEW.status = 'complained' THEN 'list.complained'
                        WHEN NEW.status = 'pending' THEN 'list.pending'
                        WHEN NEW.status = 'active' THEN 'list.subscribed'
                        ELSE 'list.status_changed'
                    END;

                    changes_json := jsonb_build_object(
                        'status', jsonb_build_object('old', OLD.status, 'new', NEW.status)
                    );
                ELSE
                    RETURN NEW;
                END IF;
            END IF;

            INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
            VALUES (NEW.email, op, 'contact_list', kind_value, NEW.list_id, changes_json, CURRENT_TIMESTAMP);

            RETURN NEW;
        END;
        $$ LANGUAGE plpgsql;
    `)
    if err != nil {
        return fmt.Errorf("failed to update track_contact_list_changes function for workspace %s: %w", workspace.ID, err)
    }

    // Update segment trigger with semantic naming
    _, err = db.ExecContext(ctx, `
        CREATE OR REPLACE FUNCTION track_contact_segment_changes()
        RETURNS TRIGGER AS $$
        DECLARE
            changes_json JSONB := '{}'::jsonb;
            op VARCHAR(20);
            kind_value VARCHAR(50);
        BEGIN
            IF TG_OP = 'INSERT' THEN
                op := 'insert';
                kind_value := 'segment.joined';
                changes_json := jsonb_build_object(
                    'segment_id', jsonb_build_object('new', NEW.segment_id),
                    'version', jsonb_build_object('new', NEW.version),
                    'matched_at', jsonb_build_object('new', NEW.matched_at)
                );

            ELSIF TG_OP = 'DELETE' THEN
                op := 'delete';
                kind_value := 'segment.left';
                changes_json := jsonb_build_object(
                    'segment_id', jsonb_build_object('old', OLD.segment_id),
                    'version', jsonb_build_object('old', OLD.version)
                );
                INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
                VALUES (OLD.email, op, 'contact_segment', kind_value, OLD.segment_id, changes_json, CURRENT_TIMESTAMP);
                RETURN OLD;
            END IF;

            INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
            VALUES (NEW.email, op, 'contact_segment', kind_value, NEW.segment_id, changes_json, CURRENT_TIMESTAMP);
            RETURN NEW;
        END;
        $$ LANGUAGE plpgsql;
    `)
    if err != nil {
        return fmt.Errorf("failed to update track_contact_segment_changes function: %w", err)
    }

    // Update contact trigger with semantic naming
    _, err = db.ExecContext(ctx, `
        CREATE OR REPLACE FUNCTION track_contact_changes()
        RETURNS TRIGGER AS $$
        DECLARE
            changes_json JSONB := '{}'::jsonb;
            op VARCHAR(20);
        BEGIN
            IF TG_OP = 'INSERT' THEN
                op := 'insert';
                changes_json := NULL;
            ELSIF TG_OP = 'UPDATE' THEN
                op := 'update';
                IF OLD.external_id IS DISTINCT FROM NEW.external_id THEN changes_json := changes_json || jsonb_build_object('external_id', jsonb_build_object('old', OLD.external_id, 'new', NEW.external_id)); END IF;
                IF OLD.timezone IS DISTINCT FROM NEW.timezone THEN changes_json := changes_json || jsonb_build_object('timezone', jsonb_build_object('old', OLD.timezone, 'new', NEW.timezone)); END IF;
                IF OLD.language IS DISTINCT FROM NEW.language THEN changes_json := changes_json || jsonb_build_object('language', jsonb_build_object('old', OLD.language, 'new', NEW.language)); END IF;
                IF OLD.first_name IS DISTINCT FROM NEW.first_name THEN changes_json := changes_json || jsonb_build_object('first_name', jsonb_build_object('old', OLD.first_name, 'new', NEW.first_name)); END IF;
                IF OLD.last_name IS DISTINCT FROM NEW.last_name THEN changes_json := changes_json || jsonb_build_object('last_name', jsonb_build_object('old', OLD.last_name, 'new', NEW.last_name)); END IF;
                IF OLD.phone IS DISTINCT FROM NEW.phone THEN changes_json := changes_json || jsonb_build_object('phone', jsonb_build_object('old', OLD.phone, 'new', NEW.phone)); END IF;
                IF OLD.address_line_1 IS DISTINCT FROM NEW.address_line_1 THEN changes_json := changes_json || jsonb_build_object('address_line_1', jsonb_build_object('old', OLD.address_line_1, 'new', NEW.address_line_1)); END IF;
                IF OLD.address_line_2 IS DISTINCT FROM NEW.address_line_2 THEN changes_json := changes_json || jsonb_build_object('address_line_2', jsonb_build_object('old', OLD.address_line_2, 'new', NEW.address_line_2)); END IF;
                IF OLD.country IS DISTINCT FROM NEW.country THEN changes_json := changes_json || jsonb_build_object('country', jsonb_build_object('old', OLD.country, 'new', NEW.country)); END IF;
                IF OLD.postcode IS DISTINCT FROM NEW.postcode THEN changes_json := changes_json || jsonb_build_object('postcode', jsonb_build_object('old', OLD.postcode, 'new', NEW.postcode)); END IF;
                IF OLD.state IS DISTINCT FROM NEW.state THEN changes_json := changes_json || jsonb_build_object('state', jsonb_build_object('old', OLD.state, 'new', NEW.state)); END IF;
                IF OLD.job_title IS DISTINCT FROM NEW.job_title THEN changes_json := changes_json || jsonb_build_object('job_title', jsonb_build_object('old', OLD.job_title, 'new', NEW.job_title)); END IF;
                IF OLD.lifetime_value IS DISTINCT FROM NEW.lifetime_value THEN changes_json := changes_json || jsonb_build_object('lifetime_value', jsonb_build_object('old', OLD.lifetime_value, 'new', NEW.lifetime_value)); END IF;
                IF OLD.orders_count IS DISTINCT FROM NEW.orders_count THEN changes_json := changes_json || jsonb_build_object('orders_count', jsonb_build_object('old', OLD.orders_count, 'new', NEW.orders_count)); END IF;
                IF OLD.last_order_at IS DISTINCT FROM NEW.last_order_at THEN changes_json := changes_json || jsonb_build_object('last_order_at', jsonb_build_object('old', OLD.last_order_at, 'new', NEW.last_order_at)); END IF;
                IF OLD.custom_string_1 IS DISTINCT FROM NEW.custom_string_1 THEN changes_json := changes_json || jsonb_build_object('custom_string_1', jsonb_build_object('old', OLD.custom_string_1, 'new', NEW.custom_string_1)); END IF;
                IF OLD.custom_string_2 IS DISTINCT FROM NEW.custom_string_2 THEN changes_json := changes_json || jsonb_build_object('custom_string_2', jsonb_build_object('old', OLD.custom_string_2, 'new', NEW.custom_string_2)); END IF;
                IF OLD.custom_string_3 IS DISTINCT FROM NEW.custom_string_3 THEN changes_json := changes_json || jsonb_build_object('custom_string_3', jsonb_build_object('old', OLD.custom_string_3, 'new', NEW.custom_string_3)); END IF;
                IF OLD.custom_string_4 IS DISTINCT FROM NEW.custom_string_4 THEN changes_json := changes_json || jsonb_build_object('custom_string_4', jsonb_build_object('old', OLD.custom_string_4, 'new', NEW.custom_string_4)); END IF;
                IF OLD.custom_string_5 IS DISTINCT FROM NEW.custom_string_5 THEN changes_json := changes_json || jsonb_build_object('custom_string_5', jsonb_build_object('old', OLD.custom_string_5, 'new', NEW.custom_string_5)); END IF;
                IF OLD.custom_number_1 IS DISTINCT FROM NEW.custom_number_1 THEN changes_json := changes_json || jsonb_build_object('custom_number_1', jsonb_build_object('old', OLD.custom_number_1, 'new', NEW.custom_number_1)); END IF;
                IF OLD.custom_number_2 IS DISTINCT FROM NEW.custom_number_2 THEN changes_json := changes_json || jsonb_build_object('custom_number_2', jsonb_build_object('old', OLD.custom_number_2, 'new', NEW.custom_number_2)); END IF;
                IF OLD.custom_number_3 IS DISTINCT FROM NEW.custom_number_3 THEN changes_json := changes_json || jsonb_build_object('custom_number_3', jsonb_build_object('old', OLD.custom_number_3, 'new', NEW.custom_number_3)); END IF;
                IF OLD.custom_number_4 IS DISTINCT FROM NEW.custom_number_4 THEN changes_json := changes_json || jsonb_build_object('custom_number_4', jsonb_build_object('old', OLD.custom_number_4, 'new', NEW.custom_number_4)); END IF;
                IF OLD.custom_number_5 IS DISTINCT FROM NEW.custom_number_5 THEN changes_json := changes_json || jsonb_build_object('custom_number_5', jsonb_build_object('old', OLD.custom_number_5, 'new', NEW.custom_number_5)); END IF;
                IF OLD.custom_datetime_1 IS DISTINCT FROM NEW.custom_datetime_1 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_1', jsonb_build_object('old', OLD.custom_datetime_1, 'new', NEW.custom_datetime_1)); END IF;
                IF OLD.custom_datetime_2 IS DISTINCT FROM NEW.custom_datetime_2 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_2', jsonb_build_object('old', OLD.custom_datetime_2, 'new', NEW.custom_datetime_2)); END IF;
                IF OLD.custom_datetime_3 IS DISTINCT FROM NEW.custom_datetime_3 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_3', jsonb_build_object('old', OLD.custom_datetime_3, 'new', NEW.custom_datetime_3)); END IF;
                IF OLD.custom_datetime_4 IS DISTINCT FROM NEW.custom_datetime_4 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_4', jsonb_build_object('old', OLD.custom_datetime_4, 'new', NEW.custom_datetime_4)); END IF;
                IF OLD.custom_datetime_5 IS DISTINCT FROM NEW.custom_datetime_5 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_5', jsonb_build_object('old', OLD.custom_datetime_5, 'new', NEW.custom_datetime_5)); END IF;
                IF OLD.custom_json_1 IS DISTINCT FROM NEW.custom_json_1 THEN changes_json := changes_json || jsonb_build_object('custom_json_1', jsonb_build_object('old', OLD.custom_json_1, 'new', NEW.custom_json_1)); END IF;
                IF OLD.custom_json_2 IS DISTINCT FROM NEW.custom_json_2 THEN changes_json := changes_json || jsonb_build_object('custom_json_2', jsonb_build_object('old', OLD.custom_json_2, 'new', NEW.custom_json_2)); END IF;
                IF OLD.custom_json_3 IS DISTINCT FROM NEW.custom_json_3 THEN changes_json := changes_json || jsonb_build_object('custom_json_3', jsonb_build_object('old', OLD.custom_json_3, 'new', NEW.custom_json_3)); END IF;
                IF OLD.custom_json_4 IS DISTINCT FROM NEW.custom_json_4 THEN changes_json := changes_json || jsonb_build_object('custom_json_4', jsonb_build_object('old', OLD.custom_json_4, 'new', NEW.custom_json_4)); END IF;
                IF OLD.custom_json_5 IS DISTINCT FROM NEW.custom_json_5 THEN changes_json := changes_json || jsonb_build_object('custom_json_5', jsonb_build_object('old', OLD.custom_json_5, 'new', NEW.custom_json_5)); END IF;
                IF changes_json = '{}'::jsonb THEN RETURN NEW; END IF;
            END IF;
            IF TG_OP = 'INSERT' THEN
                INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
                VALUES (NEW.email, op, 'contact', 'contact.created', changes_json, NEW.created_at);
            ELSE
                INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
                VALUES (NEW.email, op, 'contact', 'contact.updated', changes_json, NEW.updated_at);
            END IF;
            RETURN NEW;
        END;
        $$ LANGUAGE plpgsql;
    `)
    if err != nil {
        return fmt.Errorf("failed to update track_contact_changes function: %w", err)
    }

    // Migrate historical contact_list timeline entries
    _, err = db.ExecContext(ctx, `
        UPDATE contact_timeline
        SET kind = CASE
            -- INSERT events mapped by status
            WHEN kind = 'insert_contact_list' AND changes->'status'->'new' IS NOT NULL THEN
                CASE changes->'status'->>'new'
                    WHEN 'active' THEN 'list.subscribed'
                    WHEN 'pending' THEN 'list.pending'
                    WHEN 'unsubscribed' THEN 'list.unsubscribed'
                    WHEN 'bounced' THEN 'list.bounced'
                    WHEN 'complained' THEN 'list.complained'
                    ELSE 'list.subscribed'
                END
            -- UPDATE events mapped by status transition
            WHEN kind = 'update_contact_list' AND changes->'status' IS NOT NULL THEN
                CASE
                    WHEN changes->'status'->>'old' = 'pending' AND changes->'status'->>'new' = 'active'
                        THEN 'list.confirmed'
                    WHEN changes->'status'->>'old' IN ('unsubscribed', 'bounced', 'complained')
                        AND changes->'status'->>'new' = 'active'
                        THEN 'list.resubscribed'
                    WHEN changes->'status'->>'new' = 'unsubscribed' THEN 'list.unsubscribed'
                    WHEN changes->'status'->>'new' = 'bounced' THEN 'list.bounced'
                    WHEN changes->'status'->>'new' = 'complained' THEN 'list.complained'
                    WHEN changes->'status'->>'new' = 'pending' THEN 'list.pending'
                    WHEN changes->'status'->>'new' = 'active' THEN 'list.subscribed'
                    ELSE 'list.status_changed'
                END
            -- Soft delete
            WHEN kind = 'update_contact_list' AND changes->'deleted_at'->'new' IS NOT NULL
                THEN 'list.removed'
            ELSE kind
        END
        WHERE entity_type = 'contact_list'
          AND kind IN ('insert_contact_list', 'update_contact_list')
    `)
    if err != nil {
        return fmt.Errorf("failed to migrate contact_list timeline entries: %w", err)
    }

    // Migrate historical segment timeline entries
    _, err = db.ExecContext(ctx, `
        UPDATE contact_timeline
        SET kind = CASE
            WHEN kind = 'join_segment' THEN 'segment.joined'
            WHEN kind = 'leave_segment' THEN 'segment.left'
            ELSE kind
        END
        WHERE entity_type = 'contact_segment'
          AND kind IN ('join_segment', 'leave_segment')
    `)
    if err != nil {
        return fmt.Errorf("failed to migrate segment timeline entries: %w", err)
    }

    // Migrate historical contact timeline entries
    _, err = db.ExecContext(ctx, `
        UPDATE contact_timeline
        SET kind = CASE
            WHEN kind = 'insert_contact' THEN 'contact.created'
            WHEN kind = 'update_contact' THEN 'contact.updated'
            ELSE kind
        END
        WHERE entity_type = 'contact'
          AND kind IN ('insert_contact', 'update_contact')
    `)
    if err != nil {
        return fmt.Errorf("failed to migrate contact timeline entries: %w", err)
    }

    return nil
}

func init() {
    Register(&V18Migration{})
}
```

---

## Automation Examples

### Example 1: Welcome Email on Subscription

```json
{
  "name": "Welcome new subscribers",
  "trigger": {
    "event_kinds": ["list.subscribed", "list.confirmed"],
    "conditions": {
      "kind": "leaf",
      "leaf": {
        "table": "contact_timeline",
        "contact_timeline": {
          "kind": "list.subscribed",
          "entity_id": "welcome_list_123"
        }
      }
    }
  }
}
```

### Example 2: Re-engagement for Resubscribers

```json
{
  "name": "Welcome back resubscribers",
  "trigger": {
    "event_kinds": ["list.resubscribed"],
    "conditions": {
      "kind": "leaf",
      "leaf": {
        "table": "contact_timeline",
        "contact_timeline": {
          "kind": "list.resubscribed",
          "operation": "update"
        }
      }
    }
  }
}
```

### Example 3: Stop Sending on Negative Events

```json
{
  "name": "Pause automations on issues",
  "trigger": {
    "event_kinds": ["list.unsubscribed", "list.bounced", "list.complained"]
  }
}
```

---

## Frontend Updates

### TypeScript Interface Updates

**File**: `console/src/services/api/contact_timeline.ts`

Add new event kinds to documentation:

```typescript
export interface ContactTimelineEntry {
  id: string
  email: string
  operation: 'insert' | 'update' | 'delete'
  entity_type: 'contact' | 'contact_list' | 'message_history' | 'webhook_event' | 'contact_segment' | 'custom_event'
  kind: string // Examples:
               // Contact list: 'list.subscribed', 'list.unsubscribed', 'list.confirmed', 'list.bounced', 'list.complained'
               // Custom events: 'orders/fulfilled', 'payment.succeeded'
               // Segments: 'join_segment', 'leave_segment'
               // Messages: 'open_email', 'click_email'
  changes: Record<string, any>
  entity_id?: string
  entity_data?: EntityData
  created_at: string
  db_created_at: string
}
```

### UI Display Logic

**File**: `console/src/components/timeline/ContactTimeline.tsx`

Update rendering logic to handle new event kinds:

```typescript
// Add to getEntityIcon function
const getEntityIcon = (entry: ContactTimelineEntry) => {
  const { entity_type, kind } = entry

  if (entity_type === 'contact_list') {
    if (kind === 'list.subscribed' || kind === 'list.confirmed' || kind === 'list.resubscribed') {
      return faUserPlus  // Add subscription icon
    } else if (kind === 'list.unsubscribed') {
      return faUserMinus  // Unsubscribe icon
    } else if (kind === 'list.bounced') {
      return faCircleExclamation
    } else if (kind === 'list.complained') {
      return faTriangleExclamation
    } else if (kind === 'list.removed') {
      return faUserXmark
    }
  }
  // ... rest of logic
}

// Add display messages
const renderContactListMessage = (entry: ContactTimelineEntry) => {
  const { kind } = entry
  const listData = entry.entity_data as ContactListEntityData | undefined

  const eventLabels = {
    'list.subscribed': 'Subscribed',
    'list.unsubscribed': 'Unsubscribed',
    'list.confirmed': 'Confirmed Subscription',
    'list.bounced': 'Email Bounced',
    'list.complained': 'Marked as Spam',
    'list.resubscribed': 'Resubscribed',
    'list.removed': 'Removed from List',
    'list.pending': 'Pending Confirmation'
  }

  const label = eventLabels[kind] || 'List Status Changed'

  return (
    <div>
      {renderTitleWithDate(entry, <Text strong>{label}</Text>)}
      <div className="mb-2">
        <Text>
          {label} {listData?.name ? (
            <Text strong>{listData.name}</Text>
          ) : (
            <Text code>{entry.entity_id}</Text>
          )}
        </Text>
      </div>
    </div>
  )
}
```

---

## Breaking Changes & Migration

### ⚠️ Impact Analysis

1. **Existing Timeline Entries**: Old entries have `insert_contact_list`, `update_contact_list`
2. **Automations**: Any automations filtering on old kind values will break
3. **Segments**: Segment conditions using old kind names won't match new events

### Migration Options

#### Option A: Only Update Trigger (No Historical Migration)

- New events use semantic names
- Old events keep old names
- Automations need to support both formats

**Pros**: Simple, no data migration
**Cons**: Inconsistent historical data

#### Option B: Migrate Historical Data (Recommended)

Update existing timeline entries to use new semantic names:

```sql
-- Migration script to update historical contact_list timeline entries
UPDATE contact_timeline
SET kind = CASE
    -- INSERT events mapped by status
    WHEN kind = 'insert_contact_list' AND changes->>'status' ? 'new' THEN
        CASE changes->'status'->>'new'
            WHEN 'active' THEN 'list.subscribed'
            WHEN 'pending' THEN 'list.pending'
            WHEN 'unsubscribed' THEN 'list.unsubscribed'
            WHEN 'bounced' THEN 'list.bounced'
            WHEN 'complained' THEN 'list.complained'
            ELSE 'list.subscribed'
        END

    -- UPDATE events mapped by status transition
    WHEN kind = 'update_contact_list' AND changes->'status' IS NOT NULL THEN
        CASE
            WHEN changes->'status'->>'old' = 'pending' AND changes->'status'->>'new' = 'active'
                THEN 'list.confirmed'
            WHEN changes->'status'->>'old' IN ('unsubscribed', 'bounced', 'complained')
                AND changes->'status'->>'new' = 'active'
                THEN 'list.resubscribed'
            WHEN changes->'status'->>'new' = 'unsubscribed' THEN 'list.unsubscribed'
            WHEN changes->'status'->>'new' = 'bounced' THEN 'list.bounced'
            WHEN changes->'status'->>'new' = 'complained' THEN 'list.complained'
            WHEN changes->'status'->>'new' = 'pending' THEN 'list.pending'
            WHEN changes->'status'->>'new' = 'active' THEN 'list.subscribed'
            ELSE 'list.status_changed'
        END

    -- Soft delete
    WHEN kind = 'update_contact_list' AND changes->'deleted_at' IS NOT NULL
        THEN 'list.removed'

    ELSE kind
END
WHERE entity_type = 'contact_list'
  AND kind IN ('insert_contact_list', 'update_contact_list');
```

---

## Comparison: Current vs New Naming

### Contact Lists

| Old Kind | New Kind (v18) | Benefits |
|----------|----------------|----------|
| `insert_contact_list` | `list.subscribed` | Clear, semantic |
| `update_contact_list` (pending→active) | `list.confirmed` | Specific intent |
| `update_contact_list` (→unsubscribed) | `list.unsubscribed` | Distinct, actionable |
| `update_contact_list` (→bounced) | `list.bounced` | Specific, can trigger workflows |
| `update_contact_list` (→complained) | `list.complained` | Clear negative signal |
| `update_contact_list` (unsub→active) | `list.resubscribed` | Distinguishes from initial subscription |

### Segments

| Old Kind | New Kind (v18) | Benefits |
|----------|----------------|----------|
| `join_segment` | `segment.joined` | Consistent with dotted format |
| `leave_segment` | `segment.left` | Consistent with dotted format |

### Contacts

| Old Kind | New Kind (v18) | Benefits |
|----------|----------------|----------|
| `insert_contact` | `contact.created` | Semantic, clear intent |
| `update_contact` | `contact.updated` | Semantic, clear intent |

---

## Future Consistency Plan

With this pattern established, future message events could follow the same dotted namespace:

**Potential Future Updates (not in v18)**:
- `message.sent` instead of `insert_message_history`
- `message.opened` instead of `open_email`
- `message.clicked` instead of `click_email`
- `message.bounced` instead of `bounce_email`
- `message.complained` instead of `complain_email`
- `message.unsubscribed` instead of `unsubscribe_email`

This creates a **unified event taxonomy** aligned with modern event-driven systems.

---

## Testing Requirements

### Unit Tests

**File**: `internal/migrations/v18_test.go`

Test the updated trigger:

```go
func TestV18Migration_ContactListSemanticEvents(t *testing.T) {
    // Test INSERT with active status generates 'list.subscribed'
    // Test INSERT with pending status generates 'list.pending'
    // Test UPDATE pending→active generates 'list.confirmed'
    // Test UPDATE →unsubscribed generates 'list.unsubscribed'
    // Test UPDATE →bounced generates 'list.bounced'
    // Test UPDATE →complained generates 'list.complained'
    // Test UPDATE (unsub/bounced)→active generates 'list.resubscribed'
    // Test UPDATE with deleted_at generates 'list.removed'
}
```

### Integration Tests

Verify automation triggers work with new event kinds:

```go
func TestAutomationTriggersWithListEvents(t *testing.T) {
    // Create automation triggered by 'list.subscribed'
    // Subscribe contact to list
    // Verify automation triggered
}
```

---

## Documentation Updates

### CLAUDE.md Timeline Conventions

```markdown
### Timeline Event Naming Conventions

Contact timeline events follow semantic naming patterns aligned with modern event-driven systems using dotted namespace format.

#### Semantic Events (v18+)

**Contact Lists** (`entity_type='contact_list'`):
- `list.subscribed` - Initial subscription (active status)
- `list.pending` - Awaiting confirmation (double opt-in)
- `list.confirmed` - Double opt-in confirmed
- `list.unsubscribed` - User unsubscribed
- `list.bounced` - Hard bounce occurred
- `list.complained` - Spam complaint received
- `list.resubscribed` - Reactivated from unsubscribed/bounced
- `list.removed` - Removed from list (soft delete)

**Segments** (`entity_type='contact_segment'`):
- `segment.joined` - Contact entered segment (replaces `join_segment`)
- `segment.left` - Contact left segment (replaces `leave_segment`)

**Contacts** (`entity_type='contact'`):
- `contact.created` - New contact created (replaces `insert_contact`)
- `contact.updated` - Contact properties changed (replaces `update_contact`)

**Custom Events** (`entity_type='custom_event'`):
- Exact event names from external systems (e.g., `orders/fulfilled`, `payment.succeeded`)
- Uses `operation` field to distinguish insert vs update

#### Legacy Events (Messages - Pre-v18, not yet updated)

Message events still use original naming (future migration candidate):
- **Messages**: `open_email`, `click_email`, `bounce_email`, `complain_email`, `unsubscribe_email`
- **Webhook Events**: `insert_webhook_event`
- **Message History**: `insert_message_history`, `update_message_history`

**Note**: v18 migration automatically updates all historical contact_list, segment, and contact timeline entries to use new semantic names.
```

---

## Rollout Strategy

**Note**: This is merged into v18 migration, deployed together with custom events.

### Phase 1: Deploy v18 Migration
1. Deploy migration to:
   - Create custom_events table and trigger
   - Update contact_list trigger function with semantic naming
   - Update segment trigger function with semantic naming
   - Update contact trigger function with semantic naming
   - Migrate historical contact_list timeline entries
   - Migrate historical segment timeline entries
   - Migrate historical contact timeline entries
2. All internal events (lists, segments, contacts) use semantic dotted names
3. Historical data automatically updated across all entity types
4. Monitor for issues

### Phase 2: Frontend Updates
1. Update ContactTimeline component with new event rendering for all entity types
2. Update automation UI to suggest new event kinds
3. Add documentation/tooltips for new event names
4. Update segment and contact event displays

### Phase 3: Future Migrations
1. Plan migration of message events to dotted format (`message.opened`, `message.clicked`, etc.)
2. Finalize unified event naming guide

---

## Success Criteria

✅ All internal timeline events use semantic dotted namespace format
✅ Contact lists: `list.subscribed`, `list.unsubscribed`, etc.
✅ Segments: `segment.joined`, `segment.left`
✅ Contacts: `contact.created`, `contact.updated`
✅ Event names align with custom_events pattern across the board
✅ Automations can trigger on all new event kinds
✅ Frontend displays new events correctly
✅ Historical data automatically migrated for all three entity types
✅ Documentation updated with comprehensive naming conventions
✅ Tests verify all event transitions for lists, segments, and contacts

---

## Implementation Status

**Status**: ✅ Merged into v18 migration

The complete semantic event naming for contact_list, segments, and contacts is now part of the v18 migration alongside custom events. This ensures:
- **Unified event naming convention** across all internal events
- **Historical data migration** happens automatically per workspace for all three entity types
- **Single deployment cycle** with custom events
- **Consistent dotted namespace** pattern: `entity.action` format
- No separate v19 migration needed

See `plans/custom-events.md` for the complete v18 migration implementation.
