# Goal Tracking Plan

## Overview

Enhance the `custom_event` system to support goal tracking with monetary values, enabling:
1. Categorized goal types (purchase, subscription, lead, signup, booking, trial, other)
2. Segment contacts based on goal metrics (LTV, transaction count, AOV, etc.)
3. Soft-delete support for handling deleted external records (e.g., cancelled Shopify orders)

---

## Schema Changes

### 1. Update `custom_events` Table (v18 migration)

Update the existing v18 migration to include goal fields and soft-delete:

```sql
CREATE TABLE IF NOT EXISTS custom_events (
    event_name VARCHAR(100) NOT NULL,
    external_id VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL,
    source VARCHAR(50) NOT NULL DEFAULT 'api',
    integration_id VARCHAR(32),
    -- Goal tracking fields
    goal_name VARCHAR(100) DEFAULT NULL,
    goal_type VARCHAR(20) DEFAULT NULL,
    goal_value DECIMAL(15,2) DEFAULT NULL,
    -- Soft delete
    deleted_at TIMESTAMPTZ DEFAULT NULL,
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (event_name, external_id)
);

-- Indexes for goal queries (exclude deleted)
CREATE INDEX IF NOT EXISTS idx_custom_events_goal_type
ON custom_events (email, goal_type, occurred_at DESC)
WHERE goal_type IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_custom_events_transactions
ON custom_events (email, goal_value, occurred_at)
WHERE goal_type = 'purchase' AND deleted_at IS NULL;

-- Index for soft-deleted records cleanup
CREATE INDEX IF NOT EXISTS idx_custom_events_deleted
ON custom_events (deleted_at)
WHERE deleted_at IS NOT NULL;
```

### 2. Currency in Workspace Settings (JSON)

Use the existing workspace `settings` JSONB field:

```json
{
    "currency": "USD"
}
```

---

## Goal Types

### Supported Goal Types

```go
const (
    GoalTypePurchase     = "purchase"      // Transaction with revenue (goal_value REQUIRED)
    GoalTypeSubscription = "subscription"  // Recurring revenue started (goal_value REQUIRED)
    GoalTypeLead         = "lead"          // Form/inquiry submission (goal_value optional)
    GoalTypeSignup       = "signup"        // Registration/account creation (goal_value optional)
    GoalTypeBooking      = "booking"       // Appointment/demo scheduled (goal_value optional)
    GoalTypeTrial        = "trial"         // Trial started (goal_value optional)
    GoalTypeOther        = "other"         // Custom goal (goal_value optional)
)

var ValidGoalTypes = []string{
    GoalTypePurchase,
    GoalTypeSubscription,
    GoalTypeLead,
    GoalTypeSignup,
    GoalTypeBooking,
    GoalTypeTrial,
    GoalTypeOther,
}

// Goal types that require goal_value
var GoalTypesRequiringValue = []string{
    GoalTypePurchase,
    GoalTypeSubscription,
}
```

### Goal Value Requirements

| Goal Type | `goal_value` | Notes |
|-----------|--------------|-------|
| `purchase` | **Required** | Positive for orders, negative for refunds |
| `subscription` | **Required** | Positive for new/renewal, negative for refunds |
| `lead` | Optional | Can be used for lead scoring |
| `signup` | Optional | Usually not needed |
| `booking` | Optional | Value of consultation/service |
| `trial` | Optional | Usually not needed |
| `other` | Optional | Depends on use case |

### Negative Values for Refunds

Negative `goal_value` is allowed for refunds/chargebacks:

```bash
# Original purchase: +$149.99
POST /api/customEvent.upsert
{
    "event_name": "orders/completed",
    "external_id": "order_456",
    "goal_type": "purchase",
    "goal_value": 149.99
}

# Partial refund: -$25.00 (LTV now = $124.99)
POST /api/customEvent.upsert
{
    "event_name": "orders/refunded",
    "external_id": "refund_789",
    "goal_type": "purchase",
    "goal_value": -25.00,
    "properties": { "original_order_id": "order_456", "reason": "damaged_item" }
}

# Full refund via soft-delete (LTV now = $0)
POST /api/customEvent.upsert
{
    "event_name": "orders/completed",
    "external_id": "order_456",
    "deleted_at": "2025-01-28T10:00:00Z"
}
```

**When to use each approach:**

| Scenario | Approach | Result |
|----------|----------|--------|
| Partial refund | Negative value event | LTV reduced, history preserved |
| Full cancellation | Soft-delete (deleted_at) | Event excluded from all calculations |
| Chargeback | Negative value event | LTV reduced, audit trail |
| Duplicate/test data | Soft-delete | Event hidden |

### Platform Mapping

| Notifuse Goal | Meta/Facebook | Google Ads | LinkedIn | TikTok |
|---------------|---------------|------------|----------|--------|
| `purchase` | Purchase | Purchase | Purchase | CompletePayment |
| `subscription` | Subscribe | Subscribe | Sign up | Subscribe |
| `lead` | Lead | Submit lead form | Lead | SubmitForm |
| `signup` | CompleteRegistration | Sign-up | Sign up | CompleteRegistration |
| `booking` | Schedule | Book appointment | Book appointment | Schedule |
| `trial` | StartTrial | Start trial | - | - |
| `other` | Custom Event | Other | Other | Custom |

---

## Goal Type → Computed Fields Mapping

### `purchase` - Transaction Revenue

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `lifetime_value` | SUM(goal_value) | "LTV >= $1000" - VIP customers |
| `total_purchases` | COUNT(*) | "3+ purchases" - Repeat buyers |
| `avg_order_value` | AVG(goal_value) | "AOV >= $100" - High-value shoppers |
| `max_order_value` | MAX(goal_value) | "Largest order >= $500" - Big spenders |
| `first_purchase_at` | MIN(occurred_at) | "Customer since 2024" - Cohort analysis |
| `last_purchase_at` | MAX(occurred_at) | "No purchase in 90 days" - Churn risk |

### `subscription` - Recurring Revenue

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `total_subscription_value` | SUM(goal_value) | "Total MRR contribution >= $500" |
| `subscription_count` | COUNT(*) | "Multiple subscriptions" - Upsell |
| `avg_subscription_value` | AVG(goal_value) | "High-tier subscribers" |
| `first_subscription_at` | MIN(occurred_at) | "Subscriber tenure > 1 year" |
| `last_subscription_at` | MAX(occurred_at) | "Recent upgrade/renewal" |

### `lead` - Form Submissions

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `total_leads` | COUNT(*) | "Submitted 2+ forms" - Engaged |
| `total_lead_value` | SUM(goal_value) | "Lead score >= 100" - Hot leads |
| `avg_lead_value` | AVG(goal_value) | "High-value leads" |
| `first_lead_at` | MIN(occurred_at) | "Lead age > 30 days" - Nurture |
| `last_lead_at` | MAX(occurred_at) | "Recent lead in 7 days" - Follow-up |

### `signup` - Account Registration

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `signup_count` | COUNT(*) | Usually 1 |
| `signed_up_at` | MIN(occurred_at) | "Signed up in last 7 days" - Onboarding |

### `booking` - Appointments / Demos

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `total_bookings` | COUNT(*) | "Multiple demos" - High intent |
| `total_booking_value` | SUM(goal_value) | "High-value consultations" |
| `first_booking_at` | MIN(occurred_at) | "First demo date" |
| `last_booking_at` | MAX(occurred_at) | "Recent demo in 14 days" |

### `trial` - Trial Started

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `trial_count` | COUNT(*) | "Multiple trials" |
| `first_trial_at` | MIN(occurred_at) | "Trial started > 7 days ago" |
| `last_trial_at` | MAX(occurred_at) | "Active trialer" |

### `other` - Custom Goals

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `total_goals` | COUNT(*) | "Completed X custom goals" |
| `total_goal_value` | SUM(goal_value) | "Total engagement value" |
| `first_goal_at` | MIN(occurred_at) | "First engagement" |
| `last_goal_at` | MAX(occurred_at) | "Recent activity" |

### Cross-Goal Fields

| Computed Field | Aggregation | Segment Use Case |
|----------------|-------------|------------------|
| `total_revenue` | SUM WHERE goal_type IN ('purchase', 'subscription') | "Total revenue >= $X" |
| `is_customer` | EXISTS purchase OR subscription | "Has paid" vs "Prospect" |
| `first_touch_at` | MIN(occurred_at) any goal | "First interaction date" |
| `last_activity_at` | MAX(occurred_at) any goal | "Last engagement" |

---

## Soft Delete Support

### How It Works

1. **Soft delete via upsert** - Send `deleted_at` timestamp in upsert/import request
2. **Restore via upsert** - Send `deleted_at: null` to restore
3. **All queries exclude deleted** - `WHERE deleted_at IS NULL`
4. **Aggregations ignore deleted** - LTV, counts, etc. skip soft-deleted rows

### API for Soft Delete (via Upsert)

Soft-delete is handled through the `upsert` and `import` endpoints by including `deleted_at`:

```bash
# Soft delete an event (e.g., cancelled Shopify order)
curl -X POST https://api.notifuse.com/api/customEvent.upsert \
  -d '{
    "workspace_id": "ws_abc123",
    "event_name": "orders/completed",
    "external_id": "order_456",
    "email": "customer@example.com",
    "occurred_at": "2025-01-27T14:30:00Z",
    "deleted_at": "2025-01-28T10:00:00Z"
}'
```

```bash
# Restore a soft-deleted event
curl -X POST https://api.notifuse.com/api/customEvent.upsert \
  -d '{
    "workspace_id": "ws_abc123",
    "event_name": "orders/completed",
    "external_id": "order_456",
    "email": "customer@example.com",
    "occurred_at": "2025-01-27T14:30:00Z",
    "deleted_at": null
}'
```

```bash
# Bulk soft-delete via import
curl -X POST https://api.notifuse.com/api/customEvent.import \
  -d '{
    "workspace_id": "ws_abc123",
    "events": [
        {
            "event_name": "orders/completed",
            "external_id": "order_456",
            "email": "customer1@example.com",
            "occurred_at": "2025-01-27T14:30:00Z",
            "deleted_at": "2025-01-28T10:00:00Z"
        },
        {
            "event_name": "orders/completed",
            "external_id": "order_789",
            "email": "customer2@example.com",
            "occurred_at": "2025-01-27T15:00:00Z",
            "deleted_at": "2025-01-28T10:00:00Z"
        }
    ]
}'
```

### SQL Queries (Always Exclude Deleted)

```sql
-- All goal aggregations MUST include: AND deleted_at IS NULL

-- LTV calculation
SELECT SUM(goal_value) as lifetime_value
FROM custom_events
WHERE email = $1
AND goal_type = 'purchase'
AND deleted_at IS NULL;

-- Segment query
SELECT email FROM contacts c
WHERE EXISTS (
    SELECT 1 FROM custom_events ce
    WHERE ce.email = c.email
    AND ce.goal_type = 'purchase'
    AND ce.deleted_at IS NULL  -- Always exclude deleted
    GROUP BY ce.email
    HAVING SUM(ce.goal_value) >= 1000
)
```

### Use Cases for Soft Delete

| Scenario | Action | API |
|----------|--------|-----|
| Shopify order cancelled | Soft delete | `deleted_at: "timestamp"` |
| Order un-cancelled | Restore | `deleted_at: null` |
| Duplicate event sent | Soft delete | `deleted_at: "timestamp"` |
| Test data cleanup | Bulk soft delete | Import with `deleted_at` |
| GDPR deletion request | Soft delete + anonymize | Update email + set deleted_at |
| Partial refund | Negative value event | `goal_value: -25.00` (don't delete) |

---

## Segmentation Enhancements

### Segment Condition Schema

```go
type TreeLeafGoal struct {
    GoalType          string   `json:"goal_type"`           // purchase, lead, signup, etc. or "*" for all
    GoalName          *string  `json:"goal_name,omitempty"` // Optional filter by goal name
    AggregateOperator string   `json:"aggregate_operator"`  // sum, count, avg, min, max
    Operator          string   `json:"operator"`            // gte, lte, eq, between
    Value             float64  `json:"value"`
    Value2            *float64 `json:"value_2,omitempty"`   // For between operator
    TimeframeOperator string   `json:"timeframe_operator"`  // anytime, in_the_last_days, in_date_range
    TimeframeValues   []string `json:"timeframe_values,omitempty"`
}
```

### Aggregate Operators

| Operator | SQL | Use Case |
|----------|-----|----------|
| `sum` | SUM(goal_value) | Lifetime Value, Total Revenue |
| `count` | COUNT(*) | Number of purchases, leads |
| `avg` | AVG(goal_value) | Average Order Value |
| `min` | MIN(goal_value) or MIN(occurred_at) | Smallest order, First date |
| `max` | MAX(goal_value) or MAX(occurred_at) | Largest order, Last date |

### SQL Generation

```go
func (qb *QueryBuilder) buildGoalCondition(leaf *TreeLeaf) (string, []interface{}, error) {
    goal := leaf.Goal

    var args []interface{}
    argIndex := qb.argCount + 1

    // Build subquery - ALWAYS exclude deleted
    sql := "EXISTS (SELECT 1 FROM custom_events ce WHERE ce.email = contacts.email AND ce.deleted_at IS NULL"

    // Goal type filter
    if goal.GoalType != "" && goal.GoalType != "*" {
        sql += fmt.Sprintf(" AND ce.goal_type = $%d", argIndex)
        args = append(args, goal.GoalType)
        argIndex++
    }

    // Goal name filter (optional)
    if goal.GoalName != nil && *goal.GoalName != "" {
        sql += fmt.Sprintf(" AND ce.goal_name = $%d", argIndex)
        args = append(args, *goal.GoalName)
        argIndex++
    }

    // Timeframe filter
    timeframeSql, timeframeArgs := qb.buildTimeframeCondition(goal.TimeframeOperator, goal.TimeframeValues, argIndex)
    if timeframeSql != "" {
        sql += " AND " + timeframeSql
        args = append(args, timeframeArgs...)
        argIndex += len(timeframeArgs)
    }

    // GROUP BY and HAVING
    sql += " GROUP BY ce.email HAVING "

    // Aggregate expression
    aggExpr := fmt.Sprintf("%s(ce.goal_value)", GoalAggregateOperators[goal.AggregateOperator])
    if goal.AggregateOperator == "count" {
        aggExpr = "COUNT(*)"
    }

    // Comparison
    switch goal.Operator {
    case "gte":
        sql += fmt.Sprintf("%s >= $%d", aggExpr, argIndex)
        args = append(args, goal.Value)
    case "lte":
        sql += fmt.Sprintf("%s <= $%d", aggExpr, argIndex)
        args = append(args, goal.Value)
    case "eq":
        sql += fmt.Sprintf("%s = $%d", aggExpr, argIndex)
        args = append(args, goal.Value)
    case "between":
        sql += fmt.Sprintf("%s BETWEEN $%d AND $%d", aggExpr, argIndex, argIndex+1)
        args = append(args, goal.Value, *goal.Value2)
    }

    sql += ")"
    return sql, args, nil
}
```

### Segment Condition Examples

**VIP Customers (LTV >= $1000)**:
```json
{
    "table": "custom_events_goals",
    "goal_type": "purchase",
    "aggregate_operator": "sum",
    "operator": "gte",
    "value": 1000,
    "timeframe_operator": "anytime"
}
```

**Repeat Buyers (3+ purchases)**:
```json
{
    "table": "custom_events_goals",
    "goal_type": "purchase",
    "aggregate_operator": "count",
    "operator": "gte",
    "value": 3,
    "timeframe_operator": "anytime"
}
```

**High AOV Customers (avg >= $100)**:
```json
{
    "table": "custom_events_goals",
    "goal_type": "purchase",
    "aggregate_operator": "avg",
    "operator": "gte",
    "value": 100,
    "timeframe_operator": "anytime"
}
```

**Hot Leads (lead in last 7 days)**:
```json
{
    "table": "custom_events_goals",
    "goal_type": "lead",
    "aggregate_operator": "count",
    "operator": "gte",
    "value": 1,
    "timeframe_operator": "in_the_last_days",
    "timeframe_values": ["7"]
}
```

**Churning Customers (purchased before, no purchase in 90 days)**:
```json
{
    "operator": "and",
    "children": [
        {
            "table": "custom_events_goals",
            "goal_type": "purchase",
            "aggregate_operator": "count",
            "operator": "gte",
            "value": 1,
            "timeframe_operator": "anytime"
        },
        {
            "table": "custom_events_goals",
            "goal_type": "purchase",
            "aggregate_operator": "count",
            "operator": "eq",
            "value": 0,
            "timeframe_operator": "in_the_last_days",
            "timeframe_values": ["90"]
        }
    ]
}
```

**Trial → No Purchase (conversion opportunity)**:
```json
{
    "operator": "and",
    "children": [
        {
            "table": "custom_events_goals",
            "goal_type": "trial",
            "aggregate_operator": "count",
            "operator": "gte",
            "value": 1,
            "timeframe_operator": "anytime"
        },
        {
            "table": "custom_events_goals",
            "goal_type": "purchase",
            "aggregate_operator": "count",
            "operator": "eq",
            "value": 0,
            "timeframe_operator": "anytime"
        }
    ]
}
```

**Subscribers with High LTV**:
```json
{
    "operator": "and",
    "children": [
        {
            "table": "custom_events_goals",
            "goal_type": "subscription",
            "aggregate_operator": "count",
            "operator": "gte",
            "value": 1,
            "timeframe_operator": "anytime"
        },
        {
            "table": "custom_events_goals",
            "goal_type": "purchase",
            "aggregate_operator": "sum",
            "operator": "gte",
            "value": 500,
            "timeframe_operator": "anytime"
        }
    ]
}
```

---

## Implementation Plan

### Phase 1: Update v18 Migration

**File**: `internal/migrations/v18.go`

```go
// Update the CREATE TABLE statement
_, err := db.ExecContext(ctx, `
    CREATE TABLE IF NOT EXISTS custom_events (
        event_name VARCHAR(100) NOT NULL,
        external_id VARCHAR(255) NOT NULL,
        email VARCHAR(255) NOT NULL,
        properties JSONB NOT NULL DEFAULT '{}'::jsonb,
        occurred_at TIMESTAMPTZ NOT NULL,
        source VARCHAR(50) NOT NULL DEFAULT 'api',
        integration_id VARCHAR(32),
        goal_name VARCHAR(100) DEFAULT NULL,
        goal_type VARCHAR(20) DEFAULT NULL,
        goal_value DECIMAL(15,2) DEFAULT NULL,
        deleted_at TIMESTAMPTZ DEFAULT NULL,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        PRIMARY KEY (event_name, external_id)
    )
`)

// Update indexes to exclude deleted rows
_, err = db.ExecContext(ctx, `
    CREATE INDEX IF NOT EXISTS idx_custom_events_email
        ON custom_events(email, occurred_at DESC)
        WHERE deleted_at IS NULL;

    CREATE INDEX IF NOT EXISTS idx_custom_events_goal_type
        ON custom_events(email, goal_type, occurred_at DESC)
        WHERE goal_type IS NOT NULL AND deleted_at IS NULL;

    CREATE INDEX IF NOT EXISTS idx_custom_events_transactions
        ON custom_events(email, goal_value, occurred_at)
        WHERE goal_type = 'purchase' AND deleted_at IS NULL;

    CREATE INDEX IF NOT EXISTS idx_custom_events_deleted
        ON custom_events(deleted_at)
        WHERE deleted_at IS NOT NULL;
`)
```

### Phase 2: Domain Model Updates

**File**: `internal/domain/custom_event.go`

```go
type CustomEvent struct {
    EventName     string           `json:"event_name"`
    ExternalID    string           `json:"external_id"`
    Email         string           `json:"email"`
    Properties    json.RawMessage  `json:"properties"`
    OccurredAt    time.Time        `json:"occurred_at"`
    Source        string           `json:"source"`
    IntegrationID *NullableString  `json:"integration_id,omitempty"`

    // Goal tracking fields
    GoalName      *NullableString  `json:"goal_name,omitempty"`
    GoalType      *NullableString  `json:"goal_type,omitempty"`
    GoalValue     *NullableFloat64 `json:"goal_value,omitempty"`

    // Soft delete
    DeletedAt     *NullableTime    `json:"deleted_at,omitempty"`

    // Timestamps
    CreatedAt     time.Time        `json:"created_at"`
    UpdatedAt     time.Time        `json:"updated_at"`
}

// Goal type constants
const (
    GoalTypePurchase     = "purchase"
    GoalTypeSubscription = "subscription"
    GoalTypeLead         = "lead"
    GoalTypeSignup       = "signup"
    GoalTypeBooking      = "booking"
    GoalTypeTrial        = "trial"
    GoalTypeOther        = "other"
)

var ValidGoalTypes = []string{
    GoalTypePurchase,
    GoalTypeSubscription,
    GoalTypeLead,
    GoalTypeSignup,
    GoalTypeBooking,
    GoalTypeTrial,
    GoalTypeOther,
}

func (e *CustomEvent) Validate() error {
    // ... existing validation ...

    if e.GoalType != nil && !e.GoalType.IsNull {
        // Validate goal_type is in allowed list
        valid := false
        for _, t := range ValidGoalTypes {
            if e.GoalType.String == t {
                valid = true
                break
            }
        }
        if !valid {
            return fmt.Errorf("goal_type must be one of: %v", ValidGoalTypes)
        }

        // Require goal_value for purchase and subscription
        requiresValue := false
        for _, t := range GoalTypesRequiringValue {
            if e.GoalType.String == t {
                requiresValue = true
                break
            }
        }

        hasValue := e.GoalValue != nil && !e.GoalValue.IsNull
        if requiresValue && !hasValue {
            return fmt.Errorf("goal_value is required for goal_type '%s'", e.GoalType.String)
        }
    }

    // Note: Negative goal_value is allowed for refunds/chargebacks
    // No validation against negative values

    return nil
}

// IsDeleted returns true if the event has been soft-deleted
func (e *CustomEvent) IsDeleted() bool {
    return e.DeletedAt != nil && !e.DeletedAt.IsNull
}
```

**File**: `internal/domain/segment.go`

```go
// TreeLeafGoal for goal-based segmentation
type TreeLeafGoal struct {
    GoalType          string   `json:"goal_type"`
    GoalName          *string  `json:"goal_name,omitempty"`
    AggregateOperator string   `json:"aggregate_operator"` // sum, count, avg, min, max
    Operator          string   `json:"operator"`           // gte, lte, eq, between
    Value             float64  `json:"value"`
    Value2            *float64 `json:"value_2,omitempty"`
    TimeframeOperator string   `json:"timeframe_operator"`
    TimeframeValues   []string `json:"timeframe_values,omitempty"`
}

// Add to TreeLeaf
type TreeLeaf struct {
    // ... existing fields ...
    Goal *TreeLeafGoal `json:"goal,omitempty"`
}
```

### Phase 3: Repository Updates

**File**: `internal/repository/custom_event_postgres.go`

```go
// All queries must include: WHERE deleted_at IS NULL

func (r *CustomEventRepository) GetByID(ctx context.Context, eventName, externalID string) (*domain.CustomEvent, error) {
    query := `
        SELECT event_name, external_id, email, properties, occurred_at, source,
               integration_id, goal_name, goal_type, goal_value, deleted_at,
               created_at, updated_at
        FROM custom_events
        WHERE event_name = $1 AND external_id = $2 AND deleted_at IS NULL
    `
    // ...
}

func (r *CustomEventRepository) ListByEmail(ctx context.Context, email string) ([]*domain.CustomEvent, error) {
    query := `
        SELECT ...
        FROM custom_events
        WHERE email = $1 AND deleted_at IS NULL
        ORDER BY occurred_at DESC
    `
    // ...
}

func (r *CustomEventRepository) SoftDelete(ctx context.Context, eventName, externalID string) error {
    query := `
        UPDATE custom_events
        SET deleted_at = NOW(), updated_at = NOW()
        WHERE event_name = $1 AND external_id = $2 AND deleted_at IS NULL
    `
    // ...
}
```

### Phase 4: API Updates

**File**: `internal/http/custom_event_handler.go`

```go
// UpsertCustomEventRequest - used for both creating and updating events
// Soft-delete by setting deleted_at, restore by setting deleted_at to null
type UpsertCustomEventRequest struct {
    WorkspaceID   string   `json:"workspace_id"`
    Email         string   `json:"email"`
    EventName     string   `json:"event_name"`
    ExternalID    string   `json:"external_id"`
    OccurredAt    string   `json:"occurred_at"`
    Properties    any      `json:"properties,omitempty"`
    Source        string   `json:"source,omitempty"`
    IntegrationID *string  `json:"integration_id,omitempty"`
    GoalName      *string  `json:"goal_name,omitempty"`
    GoalType      *string  `json:"goal_type,omitempty"`
    GoalValue     *float64 `json:"goal_value,omitempty"`      // Required for purchase/subscription, can be negative for refunds
    DeletedAt     *string  `json:"deleted_at,omitempty"`      // ISO 8601 timestamp to soft-delete, null to restore
}

// ImportCustomEventsRequest - bulk upsert with same semantics
// Each event is upserted (insert or update on conflict)
type ImportCustomEventsRequest struct {
    WorkspaceID string                      `json:"workspace_id"`
    Events      []UpsertCustomEventRequest  `json:"events"`  // Each event is upserted, can include deleted_at
}
```

**Upsert Logic with Soft-Delete:**

```go
func (r *CustomEventRepository) Upsert(ctx context.Context, event *domain.CustomEvent) error {
    query := `
        INSERT INTO custom_events (
            event_name, external_id, email, properties, occurred_at, source,
            integration_id, goal_name, goal_type, goal_value, deleted_at,
            created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
        ON CONFLICT (event_name, external_id) DO UPDATE SET
            email = EXCLUDED.email,
            properties = EXCLUDED.properties,
            occurred_at = CASE
                WHEN EXCLUDED.occurred_at > custom_events.occurred_at
                THEN EXCLUDED.occurred_at
                ELSE custom_events.occurred_at
            END,
            goal_name = COALESCE(EXCLUDED.goal_name, custom_events.goal_name),
            goal_type = COALESCE(EXCLUDED.goal_type, custom_events.goal_type),
            goal_value = COALESCE(EXCLUDED.goal_value, custom_events.goal_value),
            deleted_at = EXCLUDED.deleted_at,  -- Can set or clear soft-delete
            updated_at = NOW()
        WHERE EXCLUDED.occurred_at > custom_events.occurred_at
           OR EXCLUDED.deleted_at IS DISTINCT FROM custom_events.deleted_at
    `
    // ...
}
```

### Phase 5: Frontend Updates

| File | Changes |
|------|---------|
| `console/src/services/api/custom_event.ts` | Add goal fields + delete method |
| `console/src/services/api/workspace.ts` | Add currency to settings type |
| `console/src/components/segment/form_leaf.tsx` | Add goal condition UI |
| `console/src/components/segment/table_schemas.ts` | Add goal condition schema |
| `console/src/components/workspace/workspace_settings.tsx` | Currency selection |

---

## API Examples

### Upsert a Purchase Goal

```bash
curl -X POST https://api.notifuse.com/api/customEvent.upsert \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "workspace_id": "ws_abc123",
    "email": "customer@example.com",
    "event_name": "orders/completed",
    "external_id": "order_456",
    "occurred_at": "2025-01-27T14:30:00Z",
    "goal_name": "shopify_order",
    "goal_type": "purchase",
    "goal_value": 149.99,
    "properties": {
        "order_id": "456",
        "product_ids": ["prod_001", "prod_002"]
    }
}'
```

### Upsert a Lead Goal

```bash
curl -X POST https://api.notifuse.com/api/customEvent.upsert \
  -d '{
    "workspace_id": "ws_abc123",
    "email": "prospect@example.com",
    "event_name": "forms/demo_request",
    "external_id": "form_789",
    "occurred_at": "2025-01-27T10:00:00Z",
    "goal_name": "demo_request",
    "goal_type": "lead",
    "goal_value": 500
}'
```

### Upsert a Subscription Goal

```bash
curl -X POST https://api.notifuse.com/api/customEvent.upsert \
  -d '{
    "workspace_id": "ws_abc123",
    "email": "subscriber@example.com",
    "event_name": "subscriptions/started",
    "external_id": "sub_123",
    "occurred_at": "2025-01-27T09:00:00Z",
    "goal_name": "pro_plan",
    "goal_type": "subscription",
    "goal_value": 29.99
}'
```

### Soft Deleting an Event (Cancelled Order)

```bash
# Soft-delete by upserting the same event with deleted_at
curl -X POST https://api.notifuse.com/api/customEvent.upsert \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "workspace_id": "ws_abc123",
    "event_name": "orders/completed",
    "external_id": "order_456",
    "email": "customer@example.com",
    "occurred_at": "2025-01-27T14:30:00Z",
    "deleted_at": "2025-01-28T10:00:00Z"
}'
```

### Restoring a Soft-Deleted Event

```bash
# Restore by upserting with deleted_at: null
curl -X POST https://api.notifuse.com/api/customEvent.upsert \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "workspace_id": "ws_abc123",
    "event_name": "orders/completed",
    "external_id": "order_456",
    "email": "customer@example.com",
    "occurred_at": "2025-01-27T14:30:00Z",
    "deleted_at": null
}'
```

---

## File Changes Summary

### Backend (Go)

| File | Changes |
|------|---------|
| `internal/migrations/v18.go` | Add goal columns + deleted_at + indexes |
| `internal/domain/custom_event.go` | Add goal fields, DeletedAt, IsDeleted(), validation |
| `internal/domain/custom_event_test.go` | Add goal validation tests |
| `internal/domain/workspace.go` | Add GetCurrency() helper |
| `internal/domain/segment.go` | Add TreeLeafGoal struct |
| `internal/repository/custom_event_postgres.go` | Update queries with deleted_at filter, upsert handles soft-delete |
| `internal/repository/custom_event_postgres_test.go` | Test goal fields + soft delete via upsert |
| `internal/service/custom_event_service.go` | Handle goal fields, validate goal_value requirements |
| `internal/http/custom_event_handler.go` | Rename to UpsertCustomEventRequest, add deleted_at field |
| `internal/domain/query_builder.go` | Add goal condition SQL generation |
| `internal/domain/query_builder_test.go` | Test goal condition SQL |

### Frontend (React/TypeScript)

| File | Changes |
|------|---------|
| `console/src/services/api/custom_event.ts` | Add goal fields + delete method |
| `console/src/services/api/workspace.ts` | Add currency to settings type |
| `console/src/components/segment/form_leaf.tsx` | Add goal condition UI |
| `console/src/components/segment/table_schemas.ts` | Add goal condition schema |
| `console/src/components/workspace/workspace_settings.tsx` | Currency selection |

---

## Testing Requirements

### Backend Tests

```bash
make test-domain      # Goal validation tests
make test-repo        # Goal field persistence + soft delete tests
make test-service     # Service tests
make test-integration # Integration tests
```

### Test Cases

1. **Goal field validation**:
   - Valid/invalid goal_type values
   - goal_value required for purchase/subscription
   - goal_value optional for lead/signup/booking/trial/other
   - Negative goal_value allowed (for refunds)

2. **Goal persistence**: Insert/update/select with goal fields

3. **Soft delete via upsert**:
   - Create with deleted_at sets soft-delete
   - Upsert with deleted_at: null restores event
   - Import with deleted_at for bulk soft-delete

4. **Queries exclude deleted**:
   - GetByID returns null for deleted events
   - ListByEmail excludes deleted events
   - Aggregations (SUM, COUNT, AVG) exclude deleted rows

5. **Segmentation SQL**:
   - Goal conditions generate correct SQL with deleted_at IS NULL
   - Negative values correctly reduce LTV

6. **Timeframe filters**: in_the_last_days, in_date_range work with goals

---

## Success Metrics

- Goal events tracked with proper categorization
- Soft-deleted events excluded from all aggregations
- Segments can filter by goal aggregates (SUM >= $X, COUNT >= N)
- Workspace currency correctly read from settings JSON
- Query performance acceptable for goal-based segments
