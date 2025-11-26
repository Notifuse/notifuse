# Custom Events System - Implementation Plan

## Overview

Add support for custom events that can be imported via API for each contact. Custom events enable tracking of external system activities (CRM interactions, eCommerce orders, subscription changes, etc.) and automatically create timeline entries for use in automations and segments.

**v18 Migration also includes**: Unified semantic naming for ALL internal timeline events:

- **Contact Lists**: `list.subscribed`, `list.unsubscribed`, `list.confirmed`, etc.
- **Segments**: `segment.joined`, `segment.left`
- **Contacts**: `contact.created`, `contact.updated`
- **Historical Migration**: Automatically update all existing timeline entries to use new semantic names

## Goals

1. **Flexible Event Storage**: Accept arbitrary event data with JSONB properties
2. **Timeline Integration**: Automatically create `contact_timeline` entries for each custom event
3. **Idempotent**: Support duplicate event prevention via external IDs
4. **Performant**: Enable fast queries on events by email, event name, and JSON properties
5. **Automation-Ready**: Events immediately available as automation triggers
6. **Consistent Naming**: Unified semantic dotted format (`entity.action`) across ALL internal events and custom events

## Architecture Principles

- **Two-table approach**: `custom_events` (current state storage) + `contact_timeline` (event history)
- **Event-driven**: Database triggers auto-create timeline entries for INSERT and UPDATE operations
- **Single-row per event type**: Composite primary key `(event_name, external_id)` stores most recent version
- **Generic event tracking**: No hardcoded logic for any specific integration
- **Timestamp-based updates**: Only accept updates with `occurred_at > existing occurred_at`
- **Direct event name mapping**: Timeline `kind` uses exact `event_name`, operation field shows `insert` or `update`

---

## Database Schema

### Custom Events Table

**Single-Row Storage Design**: Each row represents the **current state** of an external resource for a specific event type (e.g., a Shopify order). The composite primary key `(event_name, external_id)` allows the same external resource to be tracked under different event contexts. Updates replace the row only if the new `occurred_at` timestamp is more recent, preventing out-of-order webhook processing.

```sql
CREATE TABLE custom_events (
    event_name VARCHAR(100) NOT NULL,                -- Generic: "shopify.order", "stripe.payment"
    external_id VARCHAR(255) NOT NULL,               -- External resource ID (e.g., "shopify_order_12345")
    email VARCHAR(255) NOT NULL,

    -- Core event data
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,   -- Current state of the resource
    occurred_at TIMESTAMPTZ NOT NULL,                -- When this version was created

    -- Tracking
    source VARCHAR(50) NOT NULL DEFAULT 'api',       -- "api", "integration", "import"
    integration_id VARCHAR(32),                      -- Optional integration ID for "integration" source

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),            -- When first inserted
    updated_at TIMESTAMPTZ DEFAULT NOW(),            -- When last updated

    -- Composite primary key: one row per event_name + external_id combination
    PRIMARY KEY (event_name, external_id)
);

-- Indexes for querying
CREATE INDEX idx_custom_events_email
    ON custom_events(email, occurred_at DESC);

CREATE INDEX idx_custom_events_external_id
    ON custom_events(external_id);

CREATE INDEX idx_custom_events_integration_id
    ON custom_events(integration_id)
    WHERE integration_id IS NOT NULL;

-- GIN index for querying JSONB properties
CREATE INDEX idx_custom_events_properties
    ON custom_events USING GIN (properties jsonb_path_ops);
```

### Database Trigger for Timeline Integration

**Generic Event Tracking**: The trigger is completely generic and works with ANY event type without hardcoded logic. It creates timeline entries with the exact event name as the `kind`, and uses `operation='insert'` or `operation='update'` to distinguish between new events and updates.

```sql
-- Function to create timeline entries for custom events (generic, no hardcoded logic)
CREATE OR REPLACE FUNCTION track_custom_event_timeline()
RETURNS TRIGGER AS $$
DECLARE
    timeline_operation TEXT;
    changes_json JSONB;
    property_key TEXT;
    property_diff JSONB;
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- On INSERT: Create timeline entry with operation='insert'
        timeline_operation := 'insert';

        changes_json := jsonb_build_object(
            'event_name', jsonb_build_object('new', NEW.event_name),
            'external_id', jsonb_build_object('new', NEW.external_id)
        );

    ELSIF TG_OP = 'UPDATE' THEN
        -- On UPDATE: Create timeline entry with operation='update'
        timeline_operation := 'update';

        -- Compute JSON diff between OLD.properties and NEW.properties
        property_diff := '{}'::jsonb;

        -- Find changed, added, or removed keys
        FOR property_key IN
            SELECT DISTINCT key
            FROM (
                SELECT key FROM jsonb_object_keys(OLD.properties) AS key
                UNION
                SELECT key FROM jsonb_object_keys(NEW.properties) AS key
            ) AS all_keys
        LOOP
            -- Compare old and new values for this key
            IF (OLD.properties->property_key) IS DISTINCT FROM (NEW.properties->property_key) THEN
                property_diff := property_diff || jsonb_build_object(
                    property_key,
                    jsonb_build_object(
                        'old', OLD.properties->property_key,
                        'new', NEW.properties->property_key
                    )
                );
            END IF;
        END LOOP;

        changes_json := jsonb_build_object(
            'properties', property_diff,
            'occurred_at', jsonb_build_object(
                'old', OLD.occurred_at,
                'new', NEW.occurred_at
            )
        );
    END IF;

    -- Insert timeline entry with exact event_name as kind
    INSERT INTO contact_timeline (
        email,
        operation,
        entity_type,
        kind,
        entity_id,
        changes,
        created_at
    ) VALUES (
        NEW.email,
        timeline_operation,
        'custom_event',
        NEW.event_name,  -- Use exact event_name as kind
        NEW.external_id,
        changes_json,
        NEW.occurred_at
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Attach trigger to custom_events table for both INSERT and UPDATE
CREATE TRIGGER custom_event_timeline_trigger
AFTER INSERT OR UPDATE ON custom_events
FOR EACH ROW EXECUTE FUNCTION track_custom_event_timeline();
```

**Key Design Decisions**:

1. **No hardcoded logic**: Works with ANY event type without integration-specific code
2. **Exact event name as kind**: Timeline `kind` = `custom_events.event_name`
3. **Operation field distinguishes INSERT vs UPDATE**: Use `operation='insert'` or `operation='update'`
4. **JSON diff computation**: On UPDATE, computes field-level diffs showing old vs new values for changed properties
5. **Automation-ready**: Timeline entries can be filtered by `kind` matching the exact event name

---

## Domain Layer

### Domain Models

**File**: `internal/domain/custom_event.go`

```go
package domain

import (
    "context"
    "fmt"
    "regexp"
    "strings"
    "time"
)

//go:generate mockgen -destination mocks/mock_custom_event_service.go -package mocks github.com/Notifuse/notifuse/internal/domain CustomEventService
//go:generate mockgen -destination mocks/mock_custom_event_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain CustomEventRepository

// CustomEvent represents the current state of an external resource
// Note: ExternalID is the primary key and represents the unique identifier
// from the external system (e.g., "shopify_order_12345", "stripe_pi_abc123")
type CustomEvent struct {
    ExternalID    string                 `json:"external_id"`   // Primary key: external system's unique ID
    Email         string                 `json:"email"`
    EventName     string                 `json:"event_name"`    // Generic: "shopify.order", "stripe.payment"
    Properties    map[string]interface{} `json:"properties"`    // Current state of the resource
    OccurredAt    time.Time              `json:"occurred_at"`   // When this version was created
    Source        string                 `json:"source"`        // "api", "integration", "import"
    IntegrationID *string                `json:"integration_id,omitempty"` // Optional integration ID
    CreatedAt     time.Time              `json:"created_at"`    // When first inserted
    UpdatedAt     time.Time              `json:"updated_at"`    // When last updated
}

// Validate validates the custom event
func (e *CustomEvent) Validate() error {
    if e.ExternalID == "" {
        return fmt.Errorf("external_id is required")
    }
    if e.Email == "" {
        return fmt.Errorf("email is required")
    }
    if e.EventName == "" {
        return fmt.Errorf("event_name is required")
    }
    if len(e.EventName) > 100 {
        return fmt.Errorf("event_name must be 100 characters or less")
    }
    // Validate event name format
    if !isValidEventName(e.EventName) {
        return fmt.Errorf("event_name must contain only lowercase letters, numbers, underscores, dots, and slashes")
    }
    if e.OccurredAt.IsZero() {
        return fmt.Errorf("occurred_at is required")
    }
    if e.Properties == nil {
        e.Properties = make(map[string]interface{})
    }
    return nil
}

// CreateCustomEventRequest represents the API request to create a custom event
type CreateCustomEventRequest struct {
    WorkspaceID   string                 `json:"workspace_id"`
    Email         string                 `json:"email"`
    EventName     string                 `json:"event_name"`
    ExternalID    string                 `json:"external_id"`               // Required: unique external resource ID
    Properties    map[string]interface{} `json:"properties"`
    OccurredAt    *time.Time             `json:"occurred_at,omitempty"`     // Optional, defaults to now
    IntegrationID *string                `json:"integration_id,omitempty"`  // Optional integration ID
}

func (r *CreateCustomEventRequest) Validate() error {
    if r.WorkspaceID == "" {
        return fmt.Errorf("workspace_id is required")
    }
    if r.Email == "" {
        return fmt.Errorf("email is required")
    }
    if r.EventName == "" {
        return fmt.Errorf("event_name is required")
    }
    if r.ExternalID == "" {
        return fmt.Errorf("external_id is required")
    }
    if r.Properties == nil {
        r.Properties = make(map[string]interface{})
    }
    return nil
}

// BatchCreateCustomEventsRequest for bulk import
type BatchCreateCustomEventsRequest struct {
    WorkspaceID string          `json:"workspace_id"`
    Events      []*CustomEvent  `json:"events"`
}

func (r *BatchCreateCustomEventsRequest) Validate() error {
    if r.WorkspaceID == "" {
        return fmt.Errorf("workspace_id is required")
    }
    if len(r.Events) == 0 {
        return fmt.Errorf("events array cannot be empty")
    }
    if len(r.Events) > 50 {
        return fmt.Errorf("cannot batch create more than 50 events at once")
    }
    return nil
}

// ListCustomEventsRequest represents query parameters for listing custom events
type ListCustomEventsRequest struct {
    WorkspaceID string
    Email       string
    EventName   *string  // Optional filter by event name
    Limit       int
    Offset      int
}

func (r *ListCustomEventsRequest) Validate() error {
    if r.WorkspaceID == "" {
        return fmt.Errorf("workspace_id is required")
    }
    if r.Email == "" && r.EventName == nil {
        return fmt.Errorf("either email or event_name is required")
    }
    if r.Limit <= 0 {
        r.Limit = 50 // Default
    }
    if r.Limit > 100 {
        r.Limit = 100 // Max
    }
    if r.Offset < 0 {
        r.Offset = 0
    }
    return nil
}

// CustomEventRepository defines persistence methods
type CustomEventRepository interface {
    Create(ctx context.Context, workspaceID string, event *CustomEvent) error
    BatchCreate(ctx context.Context, workspaceID string, events []*CustomEvent) error
    GetByID(ctx context.Context, workspaceID, eventName, externalID string) (*CustomEvent, error)
    ListByEmail(ctx context.Context, workspaceID, email string, limit int, offset int) ([]*CustomEvent, error)
    ListByEventName(ctx context.Context, workspaceID, eventName string, limit int, offset int) ([]*CustomEvent, error)
    DeleteForEmail(ctx context.Context, workspaceID, email string) error
}

// CustomEventService defines business logic
type CustomEventService interface {
    CreateEvent(ctx context.Context, req *CreateCustomEventRequest) (*CustomEvent, error)
    BatchCreateEvents(ctx context.Context, req *BatchCreateCustomEventsRequest) ([]string, error)
    GetEvent(ctx context.Context, workspaceID, eventName, externalID string) (*CustomEvent, error)
    ListEvents(ctx context.Context, req *ListCustomEventsRequest) ([]*CustomEvent, error)
}

// Helper function to validate event name format
func isValidEventName(name string) bool {
    // Event names can use various formats:
    // - Webhook topics: "orders/fulfilled", "customers/create"
    // - Dotted: "payment.succeeded", "subscription.created"
    // - Underscores: "trial_started", "feature_activated"
    // Allow lowercase letters, numbers, underscores, dots, and slashes
    pattern := regexp.MustCompile(`^[a-z0-9_./-]+$`)
    return pattern.MatchString(name)
}
```

---

## Repository Layer

**File**: `internal/repository/custom_event_postgres.go`

```go
package repository

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"

    "github.com/Notifuse/notifuse/internal/domain"
)

type customEventRepository struct {
    workspaceRepo domain.WorkspaceRepository
}

func NewCustomEventRepository(workspaceRepo domain.WorkspaceRepository) domain.CustomEventRepository {
    return &customEventRepository{
        workspaceRepo: workspaceRepo,
    }
}

func (r *customEventRepository) Create(ctx context.Context, workspaceID string, event *domain.CustomEvent) error {
    db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
    if err != nil {
        return fmt.Errorf("failed to get workspace connection: %w", err)
    }

    propertiesJSON, err := json.Marshal(event.Properties)
    if err != nil {
        return fmt.Errorf("failed to marshal properties: %w", err)
    }

    // UPSERT: Insert new event or update if (event_name, external_id) exists AND new occurred_at is more recent
    query := `
        INSERT INTO custom_events (
            event_name, external_id, email, properties, occurred_at,
            source, integration_id, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (event_name, external_id) DO UPDATE SET
            email = EXCLUDED.email,
            properties = EXCLUDED.properties,
            occurred_at = EXCLUDED.occurred_at,
            source = EXCLUDED.source,
            integration_id = EXCLUDED.integration_id,
            updated_at = NOW()
        WHERE EXCLUDED.occurred_at > custom_events.occurred_at
    `

    _, err = db.ExecContext(ctx, query,
        event.EventName,
        event.ExternalID,
        event.Email,
        propertiesJSON,
        event.OccurredAt,
        event.Source,
        event.IntegrationID,
        event.CreatedAt,
        event.UpdatedAt,
    )
    if err != nil {
        return fmt.Errorf("failed to create custom event: %w", err)
    }

    return nil
}

func (r *customEventRepository) BatchCreate(ctx context.Context, workspaceID string, events []*domain.CustomEvent) error {
    if len(events) == 0 {
        return nil
    }

    db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
    if err != nil {
        return fmt.Errorf("failed to get workspace connection: %w", err)
    }

    // Use transaction for batch upsert
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO custom_events (
            event_name, external_id, email, properties, occurred_at,
            source, integration_id, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (event_name, external_id) DO UPDATE SET
            email = EXCLUDED.email,
            properties = EXCLUDED.properties,
            occurred_at = EXCLUDED.occurred_at,
            source = EXCLUDED.source,
            integration_id = EXCLUDED.integration_id,
            updated_at = NOW()
        WHERE EXCLUDED.occurred_at > custom_events.occurred_at
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare statement: %w", err)
    }
    defer stmt.Close()

    for _, event := range events {
        propertiesJSON, err := json.Marshal(event.Properties)
        if err != nil {
            return fmt.Errorf("failed to marshal properties for event %s: %w", event.ExternalID, err)
        }

        _, err = stmt.ExecContext(ctx,
            event.EventName,
            event.ExternalID,
            event.Email,
            propertiesJSON,
            event.OccurredAt,
            event.Source,
            event.IntegrationID,
            event.CreatedAt,
            event.UpdatedAt,
        )
        if err != nil {
            return fmt.Errorf("failed to insert event %s: %w", event.ExternalID, err)
        }
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}

func (r *customEventRepository) GetByID(ctx context.Context, workspaceID, eventName, externalID string) (*domain.CustomEvent, error) {
    db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to get workspace connection: %w", err)
    }

    query := `
        SELECT event_name, external_id, email, properties, occurred_at,
               source, integration_id, created_at, updated_at
        FROM custom_events
        WHERE event_name = $1 AND external_id = $2
    `

    var event domain.CustomEvent
    var propertiesJSON []byte
    var integrationID sql.NullString

    err = db.QueryRowContext(ctx, query, eventName, externalID).Scan(
        &event.EventName,
        &event.ExternalID,
        &event.Email,
        &propertiesJSON,
        &event.OccurredAt,
        &event.Source,
        &integrationID,
        &event.CreatedAt,
        &event.UpdatedAt,
    )
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("custom event not found")
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get custom event: %w", err)
    }

    if integrationID.Valid {
        event.IntegrationID = &integrationID.String
    }

    if err := json.Unmarshal(propertiesJSON, &event.Properties); err != nil {
        return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
    }

    return &event, nil
}

func (r *customEventRepository) ListByEmail(ctx context.Context, workspaceID, email string, limit int, offset int) ([]*domain.CustomEvent, error) {
    db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to get workspace connection: %w", err)
    }

    query := `
        SELECT event_name, external_id, email, properties, occurred_at,
               source, integration_id, created_at, updated_at
        FROM custom_events
        WHERE email = $1
        ORDER BY occurred_at DESC
        LIMIT $2 OFFSET $3
    `

    return r.scanEvents(ctx, db, query, email, limit, offset)
}

func (r *customEventRepository) ListByEventName(ctx context.Context, workspaceID, eventName string, limit int, offset int) ([]*domain.CustomEvent, error) {
    db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to get workspace connection: %w", err)
    }

    query := `
        SELECT event_name, external_id, email, properties, occurred_at,
               source, integration_id, created_at, updated_at
        FROM custom_events
        WHERE event_name = $1
        ORDER BY occurred_at DESC
        LIMIT $2 OFFSET $3
    `

    return r.scanEvents(ctx, db, query, eventName, limit, offset)
}

func (r *customEventRepository) DeleteForEmail(ctx context.Context, workspaceID, email string) error {
    db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
    if err != nil {
        return fmt.Errorf("failed to get workspace connection: %w", err)
    }

    query := `DELETE FROM custom_events WHERE email = $1`

    _, err = db.ExecContext(ctx, query, email)
    if err != nil {
        return fmt.Errorf("failed to delete custom events: %w", err)
    }

    return nil
}

// Helper function to scan events from query results
func (r *customEventRepository) scanEvents(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]*domain.CustomEvent, error) {
    rows, err := db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("failed to query custom events: %w", err)
    }
    defer rows.Close()

    var events []*domain.CustomEvent
    for rows.Next() {
        var event domain.CustomEvent
        var propertiesJSON []byte
        var integrationID sql.NullString

        err := rows.Scan(
            &event.EventName,
            &event.ExternalID,
            &event.Email,
            &propertiesJSON,
            &event.OccurredAt,
            &event.Source,
            &integrationID,
            &event.CreatedAt,
            &event.UpdatedAt,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to scan custom event: %w", err)
        }

        if integrationID.Valid {
            event.IntegrationID = &integrationID.String
        }

        if err := json.Unmarshal(propertiesJSON, &event.Properties); err != nil {
            return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
        }

        events = append(events, &event)
    }

    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("error iterating custom events: %w", err)
    }

    return events, nil
}
```

---

## Service Layer

**File**: `internal/service/custom_event_service.go`

```go
package service

import (
    "context"
    "fmt"
    "time"

    "github.com/Notifuse/notifuse/internal/domain"
    "github.com/Notifuse/notifuse/pkg/logger"
    "github.com/google/uuid"
)

type CustomEventService struct {
    repo        domain.CustomEventRepository
    contactRepo domain.ContactRepository
    authService domain.AuthService
    logger      logger.Logger
}

func NewCustomEventService(
    repo domain.CustomEventRepository,
    contactRepo domain.ContactRepository,
    authService domain.AuthService,
    logger logger.Logger,
) *CustomEventService {
    return &CustomEventService{
        repo:        repo,
        contactRepo: contactRepo,
        authService: authService,
        logger:      logger,
    }
}

func (s *CustomEventService) CreateEvent(ctx context.Context, req *domain.CreateCustomEventRequest) (*domain.CustomEvent, error) {
    var err error
    ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to authenticate user: %w", err)
    }

    // Check permission for writing custom events
    if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
        return nil, domain.NewPermissionError(
            domain.PermissionResourceContacts,
            domain.PermissionTypeWrite,
            "Insufficient permissions: write access to contacts required for custom events",
        )
    }

    if err := req.Validate(); err != nil {
        return nil, fmt.Errorf("invalid request: %w", err)
    }

    // Verify contact exists (or create if it doesn't)
    contact, err := s.contactRepo.GetContactByEmail(ctx, req.WorkspaceID, req.Email)
    if err != nil {
        // Create contact if it doesn't exist
        contact = &domain.Contact{
            Email:     req.Email,
            CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
        }
        _, err = s.contactRepo.UpsertContact(ctx, req.WorkspaceID, contact)
        if err != nil {
            return nil, fmt.Errorf("failed to create contact for custom event: %w", err)
        }
    }

    // Create or update custom event
    now := time.Now()
    occurredAt := now
    if req.OccurredAt != nil {
        occurredAt = *req.OccurredAt
    }

    event := &domain.CustomEvent{
        ExternalID:    req.ExternalID,
        Email:         req.Email,
        EventName:     req.EventName,
        Properties:    req.Properties,
        OccurredAt:    occurredAt,
        Source:        "api",
        IntegrationID: req.IntegrationID,
        CreatedAt:     now,
        UpdatedAt:     now,
    }

    if err := event.Validate(); err != nil {
        return nil, fmt.Errorf("invalid custom event: %w", err)
    }

    if err := s.repo.Create(ctx, req.WorkspaceID, event); err != nil {
        s.logger.WithError(err).WithField("event_name", event.EventName).Error("Failed to create custom event")
        return nil, fmt.Errorf("failed to create custom event: %w", err)
    }

    s.logger.WithFields(map[string]interface{}{
        "workspace_id": req.WorkspaceID,
        "email":        req.Email,
        "event_name":   event.EventName,
        "external_id":  event.ExternalID,
    }).Info("Custom event created successfully")

    return event, nil
}

func (s *CustomEventService) BatchCreateEvents(ctx context.Context, req *domain.BatchCreateCustomEventsRequest) ([]string, error) {
    var err error
    ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to authenticate user: %w", err)
    }

    // Check permission
    if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
        return nil, domain.NewPermissionError(
            domain.PermissionResourceContacts,
            domain.PermissionTypeWrite,
            "Insufficient permissions: write access to contacts required for custom events",
        )
    }

    if err := req.Validate(); err != nil {
        return nil, fmt.Errorf("invalid request: %w", err)
    }

    // Validate and prepare all events
    now := time.Now()
    for i, event := range req.Events {
        if event.ExternalID == "" {
            return nil, fmt.Errorf("event at index %d: external_id is required", i)
        }
        if event.CreatedAt.IsZero() {
            event.CreatedAt = now
        }
        if event.UpdatedAt.IsZero() {
            event.UpdatedAt = now
        }
        if event.OccurredAt.IsZero() {
            event.OccurredAt = now
        }
        if event.Source == "" {
            event.Source = "api"
        }
        if event.Properties == nil {
            event.Properties = make(map[string]interface{})
        }

        if err := event.Validate(); err != nil {
            return nil, fmt.Errorf("invalid event at index %d: %w", i, err)
        }
    }

    // Batch create/update
    if err := s.repo.BatchCreate(ctx, req.WorkspaceID, req.Events); err != nil {
        s.logger.WithError(err).Error("Failed to batch create custom events")
        return nil, fmt.Errorf("failed to batch create custom events: %w", err)
    }

    // Extract external IDs
    externalIDs := make([]string, len(req.Events))
    for i, event := range req.Events {
        externalIDs[i] = event.ExternalID
    }

    s.logger.WithFields(map[string]interface{}{
        "workspace_id": req.WorkspaceID,
        "count":        len(externalIDs),
    }).Info("Custom events batch created successfully")

    return externalIDs, nil
}

func (s *CustomEventService) GetEvent(ctx context.Context, workspaceID, eventName, externalID string) (*domain.CustomEvent, error) {
    var err error
    ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to authenticate user: %w", err)
    }

    // Check permission for reading custom events
    if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
        return nil, domain.NewPermissionError(
            domain.PermissionResourceContacts,
            domain.PermissionTypeRead,
            "Insufficient permissions: read access to contacts required",
        )
    }

    return s.repo.GetByID(ctx, workspaceID, eventName, externalID)
}

func (s *CustomEventService) ListEvents(ctx context.Context, req *domain.ListCustomEventsRequest) ([]*domain.CustomEvent, error) {
    var err error
    ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to authenticate user: %w", err)
    }

    // Check permission for reading custom events
    if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
        return nil, domain.NewPermissionError(
            domain.PermissionResourceContacts,
            domain.PermissionTypeRead,
            "Insufficient permissions: read access to contacts required",
        )
    }

    if err := req.Validate(); err != nil {
        return nil, fmt.Errorf("invalid request: %w", err)
    }

    // Query by email or event name
    if req.Email != "" {
        return s.repo.ListByEmail(ctx, req.WorkspaceID, req.Email, req.Limit, req.Offset)
    }
    if req.EventName != nil {
        return s.repo.ListByEventName(ctx, req.WorkspaceID, *req.EventName, req.Limit, req.Offset)
    }

    return nil, fmt.Errorf("either email or event_name must be provided")
}
```

---

## HTTP Layer

**File**: `internal/http/custom_event_handler.go`

```go
package http

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/Notifuse/notifuse/internal/domain"
    "github.com/Notifuse/notifuse/pkg/logger"
)

type CustomEventHandler struct {
    service domain.CustomEventService
    logger  logger.Logger
}

func NewCustomEventHandler(service domain.CustomEventService, logger logger.Logger) *CustomEventHandler {
    return &CustomEventHandler{
        service: service,
        logger:  logger,
    }
}

// POST /api/customEvent.create
func (h *CustomEventHandler) CreateCustomEvent(w http.ResponseWriter, r *http.Request) {
    var req domain.CreateCustomEventRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.logger.WithError(err).Error("Failed to decode request")
        respondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }

    event, err := h.service.CreateEvent(r.Context(), &req)
    if err != nil {
        h.logger.WithError(err).Error("Failed to create custom event")
        if _, ok := err.(*domain.PermissionError); ok {
            respondWithError(w, http.StatusForbidden, err.Error())
            return
        }
        respondWithError(w, http.StatusInternalServerError, "Failed to create custom event")
        return
    }

    respondWithJSON(w, http.StatusCreated, map[string]interface{}{
        "event": event,
    })
}

// POST /api/customEvent.batchCreate
func (h *CustomEventHandler) BatchCreateCustomEvents(w http.ResponseWriter, r *http.Request) {
    var req domain.BatchCreateCustomEventsRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.logger.WithError(err).Error("Failed to decode request")
        respondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }

    eventIDs, err := h.service.BatchCreateEvents(r.Context(), &req)
    if err != nil {
        h.logger.WithError(err).Error("Failed to batch create custom events")
        if _, ok := err.(*domain.PermissionError); ok {
            respondWithError(w, http.StatusForbidden, err.Error())
            return
        }
        respondWithError(w, http.StatusInternalServerError, "Failed to create custom events")
        return
    }

    respondWithJSON(w, http.StatusCreated, map[string]interface{}{
        "event_ids": eventIDs,
        "count":     len(eventIDs),
    })
}

// GET /api/customEvent.get
func (h *CustomEventHandler) GetCustomEvent(w http.ResponseWriter, r *http.Request) {
    workspaceID := r.URL.Query().Get("workspace_id")
    eventID := r.URL.Query().Get("event_id")

    if workspaceID == "" || eventID == "" {
        respondWithError(w, http.StatusBadRequest, "workspace_id and event_id are required")
        return
    }

    event, err := h.service.GetEvent(r.Context(), workspaceID, eventID)
    if err != nil {
        h.logger.WithError(err).Error("Failed to get custom event")
        if _, ok := err.(*domain.PermissionError); ok {
            respondWithError(w, http.StatusForbidden, err.Error())
            return
        }
        respondWithError(w, http.StatusNotFound, "Custom event not found")
        return
    }

    respondWithJSON(w, http.StatusOK, map[string]interface{}{
        "event": event,
    })
}

// GET /api/customEvent.list
func (h *CustomEventHandler) ListCustomEvents(w http.ResponseWriter, r *http.Request) {
    workspaceID := r.URL.Query().Get("workspace_id")
    email := r.URL.Query().Get("email")
    eventName := r.URL.Query().Get("event_name")

    if workspaceID == "" {
        respondWithError(w, http.StatusBadRequest, "workspace_id is required")
        return
    }

    if email == "" && eventName == "" {
        respondWithError(w, http.StatusBadRequest, "either email or event_name is required")
        return
    }

    // Parse limit and offset
    limit := 50
    if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
        if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
            limit = parsedLimit
        }
    }

    offset := 0
    if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
        if parsedOffset, err := strconv.Atoi(offsetStr); err == nil {
            offset = parsedOffset
        }
    }

    req := &domain.ListCustomEventsRequest{
        WorkspaceID: workspaceID,
        Email:       email,
        Limit:       limit,
        Offset:      offset,
    }

    if eventName != "" {
        req.EventName = &eventName
    }

    events, err := h.service.ListEvents(r.Context(), req)
    if err != nil {
        h.logger.WithError(err).Error("Failed to list custom events")
        if _, ok := err.(*domain.PermissionError); ok {
            respondWithError(w, http.StatusForbidden, err.Error())
            return
        }
        respondWithError(w, http.StatusInternalServerError, "Failed to list custom events")
        return
    }

    respondWithJSON(w, http.StatusOK, map[string]interface{}{
        "events": events,
        "count":  len(events),
    })
}
```

---

## API Endpoints

### RPC-Style Endpoints

```
POST   /api/customEvent.create       - Create a single custom event
POST   /api/customEvent.batchCreate  - Create multiple custom events in bulk (max 50 events)
GET    /api/customEvent.get          - Get a custom event by ID
GET    /api/customEvent.list         - List custom events by email or event name
```

### Request/Response Examples

#### Create Custom Event

**Request**: `POST /api/customEvent.create`

```json
{
  "workspace_id": "ws_abc123",
  "email": "customer@example.com",
  "event_name": "orders/fulfilled",
  "external_id": "order_12345",
  "properties": {
    "order_id": 12345,
    "order_number": 1001,
    "total_price": "299.99",
    "currency": "USD",
    "financial_status": "paid",
    "fulfillment_status": "fulfilled",
    "products": [
      { "id": 1, "name": "Product A", "price": "149.99" },
      { "id": 2, "name": "Product B", "price": "150.00" }
    ]
  },
  "occurred_at": "2025-01-15T10:30:00Z",
  "integration_id": "int_shopify_123"
}
```

**Response**: `201 Created`

```json
{
  "event": {
    "event_name": "orders/fulfilled",
    "external_id": "order_12345",
    "email": "customer@example.com",
    "properties": {
      "order_id": 12345,
      "order_number": 1001,
      "total_price": "299.99",
      "currency": "USD",
      "financial_status": "paid",
      "fulfillment_status": "fulfilled",
      "products": [...]
    },
    "occurred_at": "2025-01-15T10:30:00Z",
    "source": "api",
    "integration_id": "int_shopify_123",
    "created_at": "2025-01-15T10:35:00Z",
    "updated_at": "2025-01-15T10:35:00Z"
  }
}
```

**Note**: This creates a timeline entry with `entity_type='custom_event'`, `kind='orders/fulfilled'`, and `operation='insert'`.

#### Batch Create Custom Events

**Request**: `POST /api/customEvent.batchCreate`

```json
{
  "workspace_id": "ws_abc123",
  "events": [
    {
      "email": "user1@example.com",
      "event_name": "payment.succeeded",
      "external_id": "pi_123",
      "properties": {
        "payment_id": "pi_123",
        "amount": 5000,
        "currency": "usd",
        "status": "succeeded"
      },
      "occurred_at": "2025-01-15T10:00:00Z"
    },
    {
      "email": "user2@example.com",
      "event_name": "payment.succeeded",
      "external_id": "pi_456",
      "properties": {
        "payment_id": "pi_456",
        "amount": 10000,
        "currency": "usd",
        "status": "succeeded"
      },
      "occurred_at": "2025-01-15T11:00:00Z"
    }
  ]
}
```

**Response**: `201 Created`

```json
{
  "event_ids": ["pi_123", "pi_456"],
  "count": 2
}
```

**Note**: Returns the external_ids, not generated IDs.

#### List Custom Events

**Request**: `GET /api/customEvent.list?workspace_id=ws_abc123&email=customer@example.com&limit=50&offset=0`

**Response**: `200 OK`

```json
{
  "events": [
    {
      "event_name": "orders/fulfilled",
      "external_id": "order_12345",
      "email": "customer@example.com",
      "properties": {
        "order_id": 12345,
        "total_price": "299.99",
        "financial_status": "paid",
        "fulfillment_status": "fulfilled"
      },
      "occurred_at": "2025-01-15T10:30:00Z",
      "source": "api",
      "integration_id": "int_shopify_123",
      "created_at": "2025-01-15T10:35:00Z",
      "updated_at": "2025-01-15T10:35:00Z"
    }
  ],
  "count": 1
}
```

---

## Database Migration

**File**: `internal/migrations/v18.go`

```go
package migrations

import (
    "context"
    "fmt"

    "github.com/Notifuse/notifuse/config"
    "github.com/Notifuse/notifuse/internal/domain"
)

// V18Migration adds custom events system
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
    // No system updates needed
    return nil
}

func (m *V18Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
    // Create custom_events table with composite PRIMARY KEY (event_name, external_id)
    _, err := db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS custom_events (
            event_name VARCHAR(100) NOT NULL,
            external_id VARCHAR(255) NOT NULL,
            email VARCHAR(255) NOT NULL,
            properties JSONB NOT NULL DEFAULT '{}'::jsonb,
            occurred_at TIMESTAMPTZ NOT NULL,
            source VARCHAR(50) NOT NULL DEFAULT 'api',
            integration_id VARCHAR(32),
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW(),
            PRIMARY KEY (event_name, external_id)
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create custom_events table: %w", err)
    }

    // Create indexes
    _, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_custom_events_email
            ON custom_events(email, occurred_at DESC);

        CREATE INDEX IF NOT EXISTS idx_custom_events_external_id
            ON custom_events(external_id);

        CREATE INDEX IF NOT EXISTS idx_custom_events_integration_id
            ON custom_events(integration_id)
            WHERE integration_id IS NOT NULL;

        CREATE INDEX IF NOT EXISTS idx_custom_events_properties
            ON custom_events USING GIN (properties jsonb_path_ops);
    `)
    if err != nil {
        return fmt.Errorf("failed to create custom_events indexes: %w", err)
    }

    // Create trigger function (generic, no hardcoded logic)
    _, err = db.ExecContext(ctx, `
        CREATE OR REPLACE FUNCTION track_custom_event_timeline()
        RETURNS TRIGGER AS $$
        DECLARE
            timeline_operation TEXT;
            changes_json JSONB;
            property_key TEXT;
            property_diff JSONB;
        BEGIN
            IF TG_OP = 'INSERT' THEN
                -- On INSERT: Create timeline entry with operation='insert'
                timeline_operation := 'insert';

                changes_json := jsonb_build_object(
                    'event_name', jsonb_build_object('new', NEW.event_name),
                    'external_id', jsonb_build_object('new', NEW.external_id)
                );

            ELSIF TG_OP = 'UPDATE' THEN
                -- On UPDATE: Create timeline entry with operation='update'
                timeline_operation := 'update';

                -- Compute JSON diff between OLD.properties and NEW.properties
                property_diff := '{}'::jsonb;

                -- Find changed, added, or removed keys
                FOR property_key IN
                    SELECT DISTINCT key
                    FROM (
                        SELECT key FROM jsonb_object_keys(OLD.properties) AS key
                        UNION
                        SELECT key FROM jsonb_object_keys(NEW.properties) AS key
                    ) AS all_keys
                LOOP
                    -- Compare old and new values for this key
                    IF (OLD.properties->property_key) IS DISTINCT FROM (NEW.properties->property_key) THEN
                        property_diff := property_diff || jsonb_build_object(
                            property_key,
                            jsonb_build_object(
                                'old', OLD.properties->property_key,
                                'new', NEW.properties->property_key
                            )
                        );
                    END IF;
                END LOOP;

                changes_json := jsonb_build_object(
                    'properties', property_diff,
                    'occurred_at', jsonb_build_object(
                        'old', OLD.occurred_at,
                        'new', NEW.occurred_at
                    )
                );
            END IF;

            -- Insert timeline entry with exact event_name as kind
            INSERT INTO contact_timeline (
                email, operation, entity_type, kind, entity_id, changes, created_at
            ) VALUES (
                NEW.email, timeline_operation, 'custom_event', NEW.event_name,
                NEW.external_id, changes_json, NEW.occurred_at
            );

            RETURN NEW;
        END;
        $$ LANGUAGE plpgsql;
    `)
    if err != nil {
        return fmt.Errorf("failed to create track_custom_event_timeline function: %w", err)
    }

    // Create trigger for INSERT and UPDATE
    _, err = db.ExecContext(ctx, `
        DROP TRIGGER IF EXISTS custom_event_timeline_trigger ON custom_events;

        CREATE TRIGGER custom_event_timeline_trigger
        AFTER INSERT OR UPDATE ON custom_events
        FOR EACH ROW EXECUTE FUNCTION track_custom_event_timeline();
    `)
    if err != nil {
        return fmt.Errorf("failed to create custom_event_timeline_trigger: %w", err)
    }

    // Update contact_list trigger to use semantic event names (list.subscribed, list.unsubscribed, etc.)
    // This aligns contact_list events with the custom_events naming pattern
    _, err = db.ExecContext(ctx, `
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
        return fmt.Errorf("failed to update track_contact_list_changes function: %w", err)
    }

    // Migrate existing contact_list timeline entries to use semantic event names
    // This ensures historical data aligns with the new naming convention
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
                    -- Confirmed double opt-in
                    WHEN changes->'status'->>'old' = 'pending' AND changes->'status'->>'new' = 'active'
                        THEN 'list.confirmed'

                    -- Resubscription from unsubscribed/bounced/complained
                    WHEN changes->'status'->>'old' IN ('unsubscribed', 'bounced', 'complained')
                        AND changes->'status'->>'new' = 'active'
                        THEN 'list.resubscribed'

                    -- Unsubscribe action
                    WHEN changes->'status'->>'new' = 'unsubscribed' THEN 'list.unsubscribed'

                    -- Bounce event
                    WHEN changes->'status'->>'new' = 'bounced' THEN 'list.bounced'

                    -- Complaint event
                    WHEN changes->'status'->>'new' = 'complained' THEN 'list.complained'

                    -- Moved to pending
                    WHEN changes->'status'->>'new' = 'pending' THEN 'list.pending'

                    -- Default fallback for any other transition to active
                    WHEN changes->'status'->>'new' = 'active' THEN 'list.subscribed'

                    -- Catch-all
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

### Contact List Event Migration Explanation

The v18 migration includes two updates for contact_list events:

1. **Trigger Update**: Updates the `track_contact_list_changes()` function to use semantic event names:

   - `insert_contact_list` → Status-based mapping (`list.subscribed`, `list.pending`, etc.)
   - `update_contact_list` → Transition-based mapping (`list.confirmed`, `list.unsubscribed`, etc.)

2. **Historical Data Migration**: Updates ALL existing contact_list timeline entries per workspace:
   - Maps old `insert_contact_list` entries based on the `changes.status.new` field
   - Maps old `update_contact_list` entries based on status transitions in `changes.status.old/new`
   - Handles soft deletes with `list.removed`

**Why in v18?** This creates a unified event naming convention across the entire platform:

- Custom events: `orders/fulfilled`, `payment.succeeded` (semantic names)
- Contact list events: `list.subscribed`, `list.unsubscribed` (semantic names)
- Both use the same dotted namespace pattern for consistency

**Breaking Changes**: Automations and segments filtering on old contact_list event kinds will need to be updated or will automatically work with historical entries after migration.

---

## Integration with Automation System

### Automation Trigger Example

Custom events can be used in automation triggers immediately after creation. Timeline entries use the exact event name as the `kind`:

```json
{
  "name": "Send fulfillment notification",
  "list_id": "customers",
  "trigger": {
    "event_kinds": ["orders/fulfilled"],
    "conditions": {
      "kind": "leaf",
      "leaf": {
        "table": "contact_timeline",
        "contact_timeline": {
          "kind": "orders/fulfilled",
          "operation": "insert",
          "count_operator": "at_least",
          "count_value": 1,
          "timeframe_operator": "anytime"
        }
      }
    },
    "frequency": "once"
  }
}
```

**Note**: To filter on event properties, you would need to query the `custom_events` table directly, as the timeline `changes` field only stores metadata about the event creation/update, not the full properties.

### Timeline Query Support

The segments `QueryBuilder` can filter on custom event timeline entries:

```go
// Example: Find contacts who have had a specific event in last 30 days
conditions := &domain.TreeNode{
    Kind: "leaf",
    Leaf: &domain.LeafNode{
        Table: "contact_timeline",
        ContactTimeline: &domain.ContactTimelineCondition{
            Kind:              "orders/fulfilled",
            Operation:         "insert",  // or "update"
            CountOperator:     "at_least",
            CountValue:        1,
            TimeframeOperator: "in_the_last_days",
            TimeframeValues:   []string{"30"},
        },
    },
}
```

For filtering on event properties, query `custom_events` directly:

```sql
-- Find contacts with custom events matching property criteria
SELECT DISTINCT email
FROM custom_events
WHERE event_name = 'orders/fulfilled'
  AND (properties->>'total_price')::numeric > 500;
```

---

## Implementation Phases

### Phase 1: Core Custom Events (This Plan)

1. **Database Migration (v18.0)**

   - Create `custom_events` table
   - Add indexes for performance
   - Create custom events timeline trigger function
   - Update `contact_list` trigger to use semantic event names
   - Migrate existing contact_list timeline entries to new naming
   - Test migration on development database

2. **Domain Layer**

   - Implement `CustomEvent` entity with validation
   - Create request/response types
   - Define repository and service interfaces
   - Generate mocks for testing

3. **Repository Layer**

   - Implement PostgreSQL repository
   - Support single and batch create
   - Implement query methods (by email, by event name)
   - Add comprehensive error handling

4. **Service Layer**

   - Implement business logic
   - Add authentication and authorization checks
   - Auto-create contacts if they don't exist
   - Validate event name format

5. **HTTP Layer**

   - Create RPC-style endpoints
   - Add request validation
   - Implement proper error responses
   - Wire up handlers to routes

6. **Testing**

   - Unit tests for domain validation
   - Repository tests with sqlmock
   - Service tests with mocked dependencies
   - Integration tests for end-to-end flows
   - Run: `make test-domain test-repo test-service test-http`

7. **Documentation**
   - Update API documentation with examples
   - Document event name conventions
   - Add sample payloads for common use cases

---

## Testing Requirements

### Unit Tests

**Domain Tests** (`internal/domain/custom_event_test.go`):

- Test event validation (valid/invalid event names, required fields)
- Test request validation
- Test helper functions (`isValidEventName`)

**Repository Tests** (`internal/repository/custom_event_postgres_test.go`):

- Test Create with valid event
- Test Create with duplicate external_id (should be idempotent)
- Test BatchCreate with multiple events
- Test GetByID (found and not found)
- Test ListByEmail with pagination
- Test ListByEventName
- Test DeleteForEmail
- Test error handling (connection failures, invalid JSON)

**Service Tests** (`internal/service/custom_event_service_test.go`):

- Test CreateEvent with authentication
- Test CreateEvent creates contact if missing
- Test CreateEvent validates permissions
- Test BatchCreateEvents validates all events
- Test GetEvent with permissions
- Test ListEvents by email and event name

**Handler Tests** (`internal/http/custom_event_handler_test.go`):

- Test CreateCustomEvent endpoint
- Test BatchCreateCustomEvents endpoint
- Test GetCustomEvent endpoint
- Test ListCustomEvents endpoint
- Test error responses (validation, permissions, not found)

### Integration Tests

**End-to-End Tests** (`tests/integration/custom_event_e2e_test.go`):

1. Create custom event via API
2. Verify event stored in `custom_events` table
3. Verify timeline entry created in `contact_timeline` table
4. Verify event is queryable by email
5. Test idempotency with duplicate external_id
6. Test batch create with 50 events (max limit)
7. Test timeline trigger fires correctly

### Test Commands

```bash
# Run all tests by layer
make test-domain      # Domain validation tests
make test-repo        # Repository tests with sqlmock
make test-service     # Service tests with mocks
make test-http        # HTTP handler tests

# Run integration tests
make test-integration

# Generate coverage report
make coverage
```

---

## Event Name Conventions

Custom events can use any naming format that makes sense for your integration. The event name is stored as-is in both `custom_events.event_name` and `contact_timeline.kind`.

### Format Guidelines

Use whatever naming convention your source system uses:

- Dotted format: `payment.succeeded`, `subscription.cancelled`

### Examples

**Event-based Format**:

- `payment.succeeded`
- `payment.failed`
- `subscription.created`
- `invoice.paid`

### Rules

- Event names are **stored exactly as provided** in the API request
- Timeline `kind` = exact `event_name` (no modifications)
- Use consistent naming within each integration/source
- Maximum 100 characters
- Use lowercase letters, numbers, underscores, slashes, and dots

---

## Success Criteria

### Functional Requirements

✅ Custom events can be created via API
✅ Events automatically create timeline entries
✅ Timeline entries use event_name as `kind`
✅ Events are idempotent via external_id
✅ Batch create supports up to 50 events
✅ Events queryable by email and event_name
✅ Full properties stored in JSONB
✅ Automations can trigger on custom events

### Non-Functional Requirements

✅ Timeline trigger fires within same transaction
✅ GIN index enables fast JSONB queries
✅ Batch operations use prepared statements
✅ Proper error handling and logging
✅ Permission checks on all endpoints
✅ Comprehensive test coverage (>80%)

### Integration Requirements

✅ Works with existing segments QueryBuilder
✅ Compatible with automation trigger system
✅ Visible in contact timeline UI
✅ No breaking changes to existing systems

---

## Notes

### Custom Events

- **Timeline Entry Format**: Custom events create timeline entries with `entity_type='custom_event'`, `kind={event_name}`, and `operation='insert'` or `operation='update'`
- **Exact Event Name Mapping**: Timeline `kind` = exact `event_name` from API (e.g., `orders/fulfilled`, `payment.succeeded`)
- **Properties Storage**: Full properties JSON stored in `custom_events` table, NOT in timeline `changes` field
- **Timeline Changes Field**: Only stores minimal metadata (event created/updated indicator), and old/new json diff for updates
- **Composite Primary Key**: `(event_name, external_id)` allows same external resource across different event contexts
- **External ID**: Required field for tracking external resources and enabling upserts
- **Timestamp-based Updates**: Updates only applied if `occurred_at > existing occurred_at`
- **Integration ID**: Optional field to link events to specific integrations
- **Source Types**: `"api"` (default for direct API calls), `"integration"` (for integration-sourced events), `"import"` (for bulk imports)
- **Occurred At**: Allows backdating events for historical imports
- **Contact Auto-creation**: Service automatically creates contact if email doesn't exist
- **Permissions**: Custom events require `contacts:write` permission
- **Batch Limit**: Maximum 50 events per batch create request
- **No Hardcoded Logic**: Trigger is generic and works with ANY event type without integration-specific code

### Internal Event Semantic Naming (Updated in v18)

**Contact List Events**:

- **Event Names**: `list.subscribed`, `list.unsubscribed`, `list.confirmed`, `list.bounced`, `list.complained`, `list.resubscribed`, `list.removed`, `list.pending`
- **Status-Based Mapping**: Event names derived from contact_list status transitions (e.g., pending→active becomes `list.confirmed`)
- **Historical Migration**: Converts `insert_contact_list` and `update_contact_list` to semantic names

**Segment Events**:

- **Event Names**: `segment.joined` (replaces `join_segment`), `segment.left` (replaces `leave_segment`)
- **Historical Migration**: Converts `join_segment` → `segment.joined`, `leave_segment` → `segment.left`

**Contact Events**:

- **Event Names**: `contact.created` (replaces `insert_contact`), `contact.updated` (replaces `update_contact`)
- **Historical Migration**: Converts `insert_contact` → `contact.created`, `update_contact` → `contact.updated`

**Unified Pattern**:

- All internal events use semantic dotted namespace format (`entity.action`)
- v18 migration automatically updates ALL historical timeline entries per workspace
- Consistent with custom events pattern for unified event taxonomy
- Operation field (`insert`/`update`/`delete`) still available for filtering

---

## Dependencies

### Existing Systems

- ✅ `contact_timeline` table and trigger infrastructure (v7, v8)
- ✅ Segments `QueryBuilder` for filtering timeline events (v8, v10)
- ✅ `WorkspaceRepository.GetConnection()` for database access
- ✅ `ContactRepository.UpsertContact()` for contact creation
- ✅ Permission system for authorization checks

### New Dependencies

- None - uses existing infrastructure

---

## Estimated Complexity

**Overall**: Medium

- **Database Migration**: Low (simple table + trigger)
- **Repository Layer**: Medium (JSONB handling, batch operations)
- **Service Layer**: Low (straightforward CRUD with validation)
- **HTTP Layer**: Low (standard RPC endpoints)
- **Testing**: Medium (comprehensive coverage required)

**Estimated Time**: 2-3 days for complete implementation and testing
