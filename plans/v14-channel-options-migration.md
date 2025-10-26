# Migration Plan: Add Channel Options to Message History

**Version:** 14.0  
**Date:** 2025-10-26  
**Status:** Planning  
**Author:** AI Assistant  
**Scope:** Email options only (CC, BCC, FromName, ReplyTo)

---

## ⚠️ Critical Reminders

1. **Update `internal/database/init.go`** - Add `channel_options JSONB` column and index
2. **Create `internal/migrations/v14.go`** - Migration for existing workspaces
3. **Update `config/config.go`** - Change VERSION to "14.0"
4. **Email only** - No SMS/push fields in this version (JSONB allows future extension)  

---

## Table of Contents

1. [Overview](#overview)
2. [Objectives](#objectives)
3. [Database Schema Changes](#database-schema-changes)
4. [Backend Implementation](#backend-implementation)
5. [Frontend Implementation](#frontend-implementation)
6. [Testing Strategy](#testing-strategy)
7. [Deployment Plan](#deployment-plan)
8. [Rollback Plan](#rollback-plan)
9. [Risk Assessment](#risk-assessment)
10. [Success Criteria](#success-criteria)
11. [Timeline](#timeline)

---

## Overview

This migration adds support for storing email-specific delivery options (CC, BCC, FromName, ReplyTo) in the `message_history` table. The implementation uses a generic `channel_options` JSONB column to allow future extension for SMS/push channels without requiring schema changes.

### Current State
- **Database Version:** 13.7
- **Message History Schema:** Basic message tracking without delivery options
- **Channels Supported:** Email only

### Target State
- **Database Version:** 14.0
- **Message History Schema:** Includes `channel_options` JSONB column
- **Stored Fields:** Email options only (FromName, CC, BCC, ReplyTo)
- **Preview UI:** Displays email delivery options in message preview drawer

### Critical: Two Schema Updates Required

⚠️ **IMPORTANT:** This migration requires updates in TWO places:

1. **`internal/database/init.go`** - Base schema for NEW workspaces
2. **`internal/migrations/v14.go`** - Migration for EXISTING workspaces

Both must include the `channel_options JSONB` column and GIN index.

---

## Objectives

### Primary Goals
- ✅ Store email delivery options (CC, BCC, FromName, ReplyTo) with each message
- ✅ Enable message preview to show these options
- ✅ Update both `init.go` (new workspaces) and migration (existing workspaces)
- ✅ Use JSONB structure that allows future extension for SMS/push
- ✅ Maintain backward compatibility with existing messages

### Non-Goals
- ❌ Migrate/backfill existing message records (they'll remain NULL)
- ❌ Implement SMS/push channel options in this migration (email only)
- ❌ Change existing message sending logic (only storage)
- ❌ Add SMS/push fields to ChannelOptions struct (can be added later without schema changes)

---

## Database Schema Changes

### Migration v14.0

**File:** `/workspace/internal/migrations/v14.go`

#### Schema Changes

```sql
-- Add channel_options column
ALTER TABLE message_history
ADD COLUMN IF NOT EXISTS channel_options JSONB DEFAULT NULL;

-- Create GIN index for efficient querying
CREATE INDEX IF NOT EXISTS idx_message_history_channel_options 
ON message_history USING gin(channel_options);
```

#### Column Details

| Property | Value |
|----------|-------|
| Column Name | `channel_options` |
| Data Type | `JSONB` |
| Nullable | Yes (NULL for existing records) |
| Default | NULL |
| Index Type | GIN (Generalized Inverted Index) |
| Index Name | `idx_message_history_channel_options` |

#### Data Structure Examples

**Email Channel:**
```json
{
  "from_name": "Customer Support Team",
  "cc": ["manager@example.com", "team@example.com"],
  "bcc": ["archive@example.com"],
  "reply_to": "support@example.com"
}
```

**Null/Empty Cases:**
```json
null  // No options specified
{}    // Empty options object
```

**Future Extensibility:**
The JSONB column design allows adding SMS/push fields later without schema changes.

### Migration Implementation

**File:** `/workspace/internal/migrations/v14.go`

```go
package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V14Migration implements the migration from version 13.x to 14.0
// Adds channel_options column to message_history table to store delivery options
// like CC, BCC, FromName for email, and future options for SMS/push
type V14Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V14Migration) GetMajorVersion() float64 {
	return 14.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V14Migration) HasSystemUpdate() bool {
	return false // No system-level changes needed
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V14Migration) HasWorkspaceUpdate() bool {
	return true // Adds channel_options column to message_history table
}

// UpdateSystem executes system-level migration changes (none for v14)
func (m *V14Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V14Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add channel_options column to message_history table
	// This column stores channel-specific delivery options for email:
	// - CC, BCC: Additional recipients
	// - FromName: Override sender display name
	// - ReplyTo: Reply-to address override
	_, err := db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS channel_options JSONB DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add channel_options column to message_history for workspace %s: %w", workspace.ID, err)
	}

	// Create GIN index for channel_options JSONB column
	// Enables efficient querying like:
	// - Find all messages with CC recipients
	// - Find messages sent with specific from_name override
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_message_history_channel_options 
		ON message_history USING gin(channel_options)
	`)
	if err != nil {
		return fmt.Errorf("failed to create index on channel_options for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V14Migration{})
}
```

### Database Init Schema Update

**File:** `/workspace/internal/database/init.go`

**Update message_history table creation (around line 179):**

Add `channel_options JSONB` column after `message_data`:

```go
`CREATE TABLE IF NOT EXISTS message_history (
	id VARCHAR(255) NOT NULL PRIMARY KEY,
	contact_email VARCHAR(255) NOT NULL,
	external_id VARCHAR(255),
	broadcast_id VARCHAR(255),
	list_ids TEXT[],
	template_id VARCHAR(32) NOT NULL,
	template_version INTEGER NOT NULL,
	channel VARCHAR(20) NOT NULL,
	status_info VARCHAR(255),
	message_data JSONB NOT NULL,
	channel_options JSONB,  -- NEW: Add this line
	attachments JSONB,
	sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
	delivered_at TIMESTAMP WITH TIME ZONE,
	failed_at TIMESTAMP WITH TIME ZONE,
	opened_at TIMESTAMP WITH TIME ZONE,
	clicked_at TIMESTAMP WITH TIME ZONE,
	bounced_at TIMESTAMP WITH TIME ZONE,
	complained_at TIMESTAMP WITH TIME ZONE,
	unsubscribed_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
```

**Add index after existing message_history indexes (after line 205):**

```go
`CREATE INDEX IF NOT EXISTS idx_message_history_channel_options ON message_history USING gin(channel_options)`,
```

### Configuration Update

**File:** `/workspace/config/config.go`

**Change Line 18:**
```go
// Before
const VERSION = "13.7"

// After
const VERSION = "14.0"
```

### Changelog Update

**File:** `/workspace/CHANGELOG.md`

```markdown
## v14.0 - 2025-10-26

### Database Schema Changes
- Added `channel_options` JSONB column to `message_history` table
  - Updated `internal/database/init.go` for new workspaces
  - Created migration `v14.go` for existing workspaces
- Created GIN index on `channel_options` for efficient querying
- JSONB structure allows future SMS/push options without schema changes

### Features
- Message history now stores email delivery options:
  - CC (carbon copy recipients)
  - BCC (blind carbon copy recipients)
  - FromName (sender display name override)
  - ReplyTo (reply-to address override)
- Message preview drawer displays email delivery options
- Only stores email options in this version (SMS/push to be added later)

### Migration Notes
- Existing messages will have `channel_options = NULL` (no backfill)
- Migration is idempotent and safe to run multiple times
- Estimated migration time: < 1 second per workspace
- **Critical:** Both `init.go` and `v14.go` migration must be updated
```

---

## Backend Implementation

### Phase 1: Domain Layer

#### File: `/workspace/internal/domain/message_history.go`

**Changes:**

1. **Add ChannelOptions type definitions**

```go
// ChannelOptions represents channel-specific delivery configuration for email
// Stored as JSONB to allow future extension without schema changes
type ChannelOptions struct {
	// Email-specific options
	FromName *string  `json:"from_name,omitempty"` // Override sender display name
	CC       []string `json:"cc,omitempty"`        // Carbon copy recipients
	BCC      []string `json:"bcc,omitempty"`       // Blind carbon copy recipients
	ReplyTo  string   `json:"reply_to,omitempty"`  // Reply-to address
}

// Value implements the driver.Valuer interface for database storage
func (co ChannelOptions) Value() (driver.Value, error) {
	if co.IsEmpty() {
		return nil, nil
	}
	return json.Marshal(co)
}

// Scan implements the sql.Scanner interface for database retrieval
func (co *ChannelOptions) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return sql.ErrNoRows
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, co)
}

// IsEmpty returns true if no options are set
func (co ChannelOptions) IsEmpty() bool {
	return co.FromName == nil &&
		len(co.CC) == 0 &&
		len(co.BCC) == 0 &&
		co.ReplyTo == ""
}
```

2. **Update MessageHistory struct**

```go
// MessageHistory represents a record of a message sent to a contact
type MessageHistory struct {
	ID              string               `json:"id"`
	ExternalID      *string              `json:"external_id,omitempty"`
	ContactEmail    string               `json:"contact_email"`
	BroadcastID     *string              `json:"broadcast_id,omitempty"`
	ListIDs         ListIDs              `json:"list_ids,omitempty" db:"list_ids"`
	TemplateID      string               `json:"template_id"`
	TemplateVersion int64                `json:"template_version"`
	Channel         string               `json:"channel"`
	StatusInfo      *string              `json:"status_info,omitempty"`
	MessageData     MessageData          `json:"message_data"`
	ChannelOptions  *ChannelOptions      `json:"channel_options,omitempty"` // NEW
	Attachments     []AttachmentMetadata `json:"attachments,omitempty"`

	// Event timestamps
	SentAt         time.Time  `json:"sent_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	FailedAt       *time.Time `json:"failed_at,omitempty"`
	OpenedAt       *time.Time `json:"opened_at,omitempty"`
	ClickedAt      *time.Time `json:"clicked_at,omitempty"`
	BouncedAt      *time.Time `json:"bounced_at,omitempty"`
	ComplainedAt   *time.Time `json:"complained_at,omitempty"`
	UnsubscribedAt *time.Time `json:"unsubscribed_at,omitempty"`

	// System timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

3. **Update EmailOptions mapping** (if needed)

**File:** `/workspace/internal/domain/email_provider.go`

Add helper to convert EmailOptions to ChannelOptions:

```go
// ToChannelOptions converts EmailOptions to ChannelOptions for storage
func (eo EmailOptions) ToChannelOptions() *ChannelOptions {
	// Don't create ChannelOptions if all fields are empty
	if eo.FromName == nil && len(eo.CC) == 0 && len(eo.BCC) == 0 && eo.ReplyTo == "" {
		return nil
	}

	return &ChannelOptions{
		FromName: eo.FromName,
		CC:       eo.CC,
		BCC:      eo.BCC,
		ReplyTo:  eo.ReplyTo,
	}
}
```

#### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/database/init.go` | Modify | Add channel_options column and index to schema |
| `internal/domain/message_history.go` | Modify | Add ChannelOptions type and update MessageHistory struct |
| `internal/domain/email_provider.go` | Modify | Add ToChannelOptions helper method |
| `internal/domain/message_history_test.go` | Modify | Add tests for ChannelOptions serialization |

### Phase 2: Repository Layer

#### File: `/workspace/internal/repository/message_history_postgre.go`

**Changes:**

1. **Update scanMessageHistory helper**

```go
// scanMessageHistory is a helper to scan a row into a MessageHistory struct
func scanMessageHistory(row interface {
	Scan(dest ...interface{}) error
}, message *domain.MessageHistory) error {
	var externalID, broadcastID, statusInfo sql.NullString
	var deliveredAt, failedAt, openedAt, clickedAt, bouncedAt, complainedAt, unsubscribedAt sql.NullTime
	var attachmentsJSON, channelOptionsJSON []byte // NEW
	var listIDs pq.StringArray

	err := row.Scan(
		&message.ID,
		&externalID,
		&message.ContactEmail,
		&broadcastID,
		&listIDs,
		&message.TemplateID,
		&message.TemplateVersion,
		&message.Channel,
		&statusInfo,
		&message.MessageData,
		&channelOptionsJSON, // NEW - Add after message_data
		&attachmentsJSON,
		&message.SentAt,
		&deliveredAt,
		&failedAt,
		&openedAt,
		&clickedAt,
		&bouncedAt,
		&complainedAt,
		&unsubscribedAt,
		&message.CreatedAt,
		&message.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Handle nullable fields
	if externalID.Valid {
		message.ExternalID = &externalID.String
	}
	if broadcastID.Valid {
		message.BroadcastID = &broadcastID.String
	}
	if statusInfo.Valid {
		message.StatusInfo = &statusInfo.String
	}

	// Handle list_ids
	message.ListIDs = listIDs

	// Handle channel_options JSONB
	if len(channelOptionsJSON) > 0 {
		var options domain.ChannelOptions
		if err := json.Unmarshal(channelOptionsJSON, &options); err == nil {
			if !options.IsEmpty() {
				message.ChannelOptions = &options
			}
		}
	}

	// Handle attachments JSONB
	if len(attachmentsJSON) > 0 {
		if err := json.Unmarshal(attachmentsJSON, &message.Attachments); err != nil {
			return fmt.Errorf("failed to unmarshal attachments: %w", err)
		}
	}

	// Handle timestamp fields
	if deliveredAt.Valid {
		message.DeliveredAt = &deliveredAt.Time
	}
	// ... (rest of timestamp handling)

	return nil
}
```

2. **Update getMessageColumns helper**

```go
// getMessageColumns returns the column list for message history queries
func getMessageColumns() string {
	return `id, external_id, contact_email, broadcast_id, list_ids, template_id, template_version,
		channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, 
		failed_at, opened_at, clicked_at, bounced_at, complained_at, 
		unsubscribed_at, created_at, updated_at`
}
```

3. **Update Create method**

```go
// Create adds a new message history record
func (r *MessageHistoryRepository) Create(ctx context.Context, workspaceID string, message *domain.MessageHistory) error {
	// ... existing workspace connection code ...

	// Serialize attachments
	attachmentsJSON, err := json.Marshal(message.Attachments)
	if err != nil {
		return fmt.Errorf("failed to marshal attachments: %w", err)
	}

	// Serialize channel_options
	var channelOptionsJSON []byte
	if message.ChannelOptions != nil && !message.ChannelOptions.IsEmpty() {
		channelOptionsJSON, err = json.Marshal(message.ChannelOptions)
		if err != nil {
			return fmt.Errorf("failed to marshal channel_options: %w", err)
		}
	}

	query := `
		INSERT INTO message_history (
			id, contact_email, external_id, broadcast_id, list_ids, template_id, template_version,
			channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, 
			failed_at, opened_at, clicked_at, bounced_at, complained_at, 
			unsubscribed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, 
			$8, LEFT($9, 255), $10, $11, $12, $13, $14, 
			$15, $16, $17, $18, $19, 
			$20, $21, $22
		)
	`

	_, err = workspaceDB.ExecContext(
		ctx,
		query,
		message.ID,
		message.ContactEmail,
		message.ExternalID,
		message.BroadcastID,
		pq.Array(message.ListIDs),
		message.TemplateID,
		message.TemplateVersion,
		message.Channel,
		message.StatusInfo,
		message.MessageData,
		channelOptionsJSON, // NEW - parameter $11
		attachmentsJSON,    // Now parameter $12
		message.SentAt,
		message.DeliveredAt,
		message.FailedAt,
		message.OpenedAt,
		message.ClickedAt,
		message.BouncedAt,
		message.ComplainedAt,
		message.UnsubscribedAt,
		message.CreatedAt,
		message.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create message history: %w", err)
	}

	return nil
}
```

4. **Update Update method**

```go
// Update updates an existing message history record
func (r *MessageHistoryRepository) Update(ctx context.Context, workspaceID string, message *domain.MessageHistory) error {
	// ... existing workspace connection code ...

	// Serialize attachments and channel_options
	attachmentsJSON, err := json.Marshal(message.Attachments)
	if err != nil {
		return fmt.Errorf("failed to marshal attachments: %w", err)
	}

	var channelOptionsJSON []byte
	if message.ChannelOptions != nil && !message.ChannelOptions.IsEmpty() {
		channelOptionsJSON, err = json.Marshal(message.ChannelOptions)
		if err != nil {
			return fmt.Errorf("failed to marshal channel_options: %w", err)
		}
	}

	query := `
		UPDATE message_history SET
			contact_email = $2,
			external_id = $3,
			broadcast_id = $4,
			list_ids = $5,
			template_id = $6,
			template_version = $7,
			channel = $8,
			status_info = LEFT($9, 255),
			message_data = $10,
			channel_options = $11,
			attachments = $12,
			sent_at = $13,
			delivered_at = $14,
			failed_at = $15,
			opened_at = $16,	
			clicked_at = $17,
			bounced_at = $18,
			complained_at = $19,
			unsubscribed_at = $20,
			updated_at = $21
		WHERE id = $1
	`

	_, err = workspaceDB.ExecContext(
		ctx,
		query,
		message.ID,
		message.ContactEmail,
		message.ExternalID,
		message.BroadcastID,
		pq.Array(message.ListIDs),
		message.TemplateID,
		message.TemplateVersion,
		message.Channel,
		message.StatusInfo,
		message.MessageData,
		channelOptionsJSON, // NEW
		attachmentsJSON,
		message.SentAt,
		message.DeliveredAt,
		message.FailedAt,
		message.OpenedAt,
		message.ClickedAt,
		message.BouncedAt,
		message.ComplainedAt,
		message.UnsubscribedAt,
		message.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update message history: %w", err)
	}

	return nil
}
```

5. **Update ListMessages query builder**

```go
func (r *MessageHistoryRepository) ListMessages(ctx context.Context, workspaceID string, params domain.MessageListParams) ([]*domain.MessageHistory, string, error) {
	// ... existing code ...

	// Build base query with squirrel
	queryBuilder := squirrel.Select(
		"id", "external_id", "contact_email", "broadcast_id", "list_ids", "template_id", "template_version",
		"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
		"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
		"unsubscribed_at", "created_at", "updated_at",
	).From("message_history")

	// ... rest of query building and execution ...
}
```

#### Files to Modify

| File | Lines Changed | Description |
|------|---------------|-------------|
| `internal/repository/message_history_postgre.go` | ~150-200 | Update queries, scanning, serialization |

### Phase 3: Service Layer

#### File: `/workspace/internal/service/email_service.go`

**Changes in `SendEmailForTemplate` method:**

```go
func (s *EmailService) SendEmailForTemplate(ctx context.Context, request domain.SendEmailRequest) error {
	// ... existing template compilation code ...

	// Convert EmailOptions to ChannelOptions for storage
	var channelOptions *domain.ChannelOptions
	if !request.EmailOptions.IsEmpty() {
		channelOptions = request.EmailOptions.ToChannelOptions()
	}

	// Create message history record
	messageHistory := &domain.MessageHistory{
		ID:              request.MessageID,
		ExternalID:      request.ExternalID,
		ContactEmail:    request.Contact.Email,
		TemplateID:      request.TemplateConfig.TemplateID,
		TemplateVersion: request.TemplateConfig.TemplateVersion,
		Channel:         "email",
		MessageData:     request.MessageData,
		ChannelOptions:  channelOptions, // NEW: Store channel options
		Attachments:     attachmentsMetadata,
		SentAt:          now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Store message history in database
	err = s.messageHistoryRepo.Create(childCtx, request.WorkspaceID, messageHistory)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"message_id": request.MessageID,
		}).Error("Failed to create message history record")
		return fmt.Errorf("failed to create message history: %w", err)
	}

	// ... rest of method ...
}
```

#### File: `/workspace/internal/service/broadcast/message_sender.go`

**Changes in `sendToRecipient` method:**

```go
func (s *BroadcastMessageSender) sendToRecipient(/* ... params ... */) error {
	// ... existing code ...

	// Convert EmailOptions to ChannelOptions if present
	var channelOptions *domain.ChannelOptions
	if emailOptions != nil {
		channelOptions = emailOptions.ToChannelOptions()
	}

	// Create message history
	messageHistory := &domain.MessageHistory{
		ID:              messageID,
		ContactEmail:    contact.Email,
		BroadcastID:     &broadcast.ID,
		ListIDs:         listIDs,
		TemplateID:      template.ID,
		TemplateVersion: template.Version,
		Channel:         "email",
		MessageData: domain.MessageData{
			Data: recipientData,
		},
		ChannelOptions: channelOptions, // NEW
		SentAt:         now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// ... rest of method ...
}
```

#### File: `/workspace/internal/service/transactional_service.go`

**Changes in `Send` method:**

```go
func (s *TransactionalNotificationService) Send(ctx context.Context, params domain.SendTransactionalParams) error {
	// ... existing code ...

	// Convert EmailOptions to ChannelOptions
	var channelOptions *domain.ChannelOptions
	if params.EmailOptions != nil {
		channelOptions = params.EmailOptions.ToChannelOptions()
	}

	// Create message history (for logging/tracking)
	messageHistory := &domain.MessageHistory{
		ID:              messageID,
		ExternalID:      params.ExternalID,
		ContactEmail:    params.Contact.Email,
		TemplateID:      notification.TemplateID,
		TemplateVersion: notification.TemplateVersion,
		Channel:         "email",
		MessageData: domain.MessageData{
			Data: messageData,
		},
		ChannelOptions: channelOptions, // NEW
		SentAt:         time.Now().UTC(),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	// ... rest of method ...
}
```

#### Files to Modify

| File | Changes | Description |
|------|---------|-------------|
| `internal/service/email_service.go` | ~20 lines | Add channel_options to MessageHistory creation |
| `internal/service/broadcast/message_sender.go` | ~15 lines | Add channel_options for broadcast messages |
| `internal/service/transactional_service.go` | ~15 lines | Add channel_options for transactional messages |

---

## Frontend Implementation

### Phase 4: TypeScript Interface Updates

#### File: `/workspace/console/src/services/api/messages_history.ts`

**Changes:**

```typescript
/**
 * Channel-specific delivery options for email
 * Structure allows future extension for SMS/push without breaking changes
 */
export interface ChannelOptions {
  // Email options
  from_name?: string
  cc?: string[]
  bcc?: string[]
  reply_to?: string
}

export interface MessageData {
  data: Record<string, any>
  metadata?: Record<string, any>
}

export interface MessageHistory {
  id: string
  external_id?: string
  contact_email: string
  broadcast_id?: string
  list_ids?: string[]
  template_id: string
  template_version: number
  channel: string
  error?: string
  message_data: MessageData
  channel_options?: ChannelOptions // NEW

  // Event timestamps
  sent_at: string
  delivered_at?: string
  failed_at?: string
  opened_at?: string
  clicked_at?: string
  bounced_at?: string
  complained_at?: string
  unsubscribed_at?: string

  // System timestamps
  created_at: string
  updated_at: string
}
```

### Phase 5: UI Component Updates

#### File: `/workspace/console/src/components/templates/TemplatePreviewDrawer.tsx`

**Changes:**

1. **Update props interface to accept MessageHistory (optional)**

```typescript
interface TemplatePreviewDrawerProps {
  record: Template
  workspace: Workspace
  templateData?: Record<string, any>
  messageHistory?: MessageHistory // NEW: Optional message history for preview
  children: React.ReactNode
}
```

2. **Add channel options display section**

Insert after line 238 (after subject preview display):

```tsx
const TemplatePreviewDrawer: React.FC<TemplatePreviewDrawerProps> = ({
  record,
  workspace,
  templateData,
  messageHistory, // NEW
  children
}) => {
  // ... existing state and hooks ...

  const drawerContent = (
    <div>
      {/* Header details */}
      <div className="mb-4 space-y-2">
        <div>
          <Text strong>From: </Text>
          {/* Existing from display logic */}
        </div>
        
        {record.email?.reply_to && (
          <div>
            <Text strong>Reply to: </Text>
            <Text type="secondary">{record.email.reply_to}</Text>
          </div>
        )}
        
        <div>
          <Text strong>Subject: </Text>
          <Text>{processedSubject ?? record.email?.subject}</Text>
        </div>
        
        {record.email?.subject_preview && (
          <div>
            <Text strong>Subject preview: </Text>
            <Text type="secondary">{record.email.subject_preview}</Text>
          </div>
        )}

        {/* NEW: Channel Options Display */}
        {messageHistory?.channel_options && (
          <>
            <Divider className="my-3" />
            <Text strong className="block mb-2">Message Delivery Options:</Text>
            
            {messageHistory.channel_options.from_name && (
              <div className="ml-2">
                <Text strong className="text-xs">From Name Override: </Text>
                <Text className="text-xs">{messageHistory.channel_options.from_name}</Text>
              </div>
            )}
            
            {messageHistory.channel_options.cc && messageHistory.channel_options.cc.length > 0 && (
              <div className="ml-2">
                <Text strong className="text-xs">CC: </Text>
                <Space size={[0, 4]} wrap className="inline-flex">
                  {messageHistory.channel_options.cc.map((email, idx) => (
                    <Tag key={idx} color="blue" className="text-xs m-0">
                      {email}
                    </Tag>
                  ))}
                </Space>
              </div>
            )}
            
            {messageHistory.channel_options.bcc && messageHistory.channel_options.bcc.length > 0 && (
              <div className="ml-2">
                <Text strong className="text-xs">BCC: </Text>
                <Space size={[0, 4]} wrap className="inline-flex">
                  {messageHistory.channel_options.bcc.map((email, idx) => (
                    <Tag key={idx} color="purple" className="text-xs m-0">
                      {email}
                    </Tag>
                  ))}
                </Space>
              </div>
            )}
            
            {messageHistory.channel_options.reply_to && (
              <div className="ml-2">
                <Text strong className="text-xs">Reply To Override: </Text>
                <Text type="secondary" className="text-xs">
                  {messageHistory.channel_options.reply_to}
                </Text>
              </div>
            )}
          </>
        )}
      </div>
      
      {/* Rest of existing content */}
    </div>
  )

  return (/* ... existing return ... */)
}
```

#### File: `/workspace/console/src/components/messages/MessageHistoryTable.tsx`

**Changes:**

Update the TemplatePreviewButton component to pass messageHistory:

```tsx
const TemplatePreviewButton: React.FC<TemplatePreviewButtonProps & { messageHistory: MessageHistory }> = ({
  templateId,
  templateVersion,
  workspace,
  templateData,
  messageHistory // NEW
}) => {
  const { data, isLoading } = useQuery({
    queryKey: ['template', workspace.id, templateId, templateVersion],
    queryFn: async () => {
      const response = await templatesApi.get({
        workspace_id: workspace.id,
        id: templateId,
        version: templateVersion
      })

      if (!response.template) {
        throw new Error('Failed to load template')
      }

      return response.template
    },
    enabled: !!workspace.id && !!templateId,
    staleTime: 60 * 60 * 1000,
    retry: 1
  })

  if (!data || isLoading) {
    return null
  }

  return (
    <TemplatePreviewDrawer 
      record={data} 
      workspace={workspace} 
      templateData={templateData}
      messageHistory={messageHistory} // NEW: Pass message history
    >
      <Tooltip title="Preview message">
        <Button type="text" className="opacity-70" icon={<FontAwesomeIcon icon={faEye} />} />
      </Tooltip>
    </TemplatePreviewDrawer>
  )
}

// Update the render function in actionsColumn
const actionsColumn = {
  // ... existing config ...
  render: (record: MessageHistory) => {
    if (!record.template_id) {
      return null
    }

    return (
      <div className="flex justify-end">
        <TemplatePreviewButton
          templateId={record.template_id}
          templateVersion={record.template_version}
          workspace={workspace}
          templateData={record.message_data.data || {}}
          messageHistory={record} // NEW: Pass the full record
        />
      </div>
    )
  }
}
```

#### Files to Modify

| File | Changes | Description |
|------|---------|-------------|
| `console/src/services/api/messages_history.ts` | ~20 lines | Add ChannelOptions interface |
| `console/src/components/templates/TemplatePreviewDrawer.tsx` | ~60 lines | Add channel options display |
| `console/src/components/messages/MessageHistoryTable.tsx` | ~10 lines | Pass messageHistory to preview |

---

## Testing Strategy

### Backend Tests

#### 1. Migration Tests

**File:** `/workspace/internal/migrations/v14_test.go`

```go
package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV14Migration_GetMajorVersion(t *testing.T) {
	migration := &V14Migration{}
	assert.Equal(t, 14.0, migration.GetMajorVersion())
}

func TestV14Migration_HasSystemUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.False(t, migration.HasSystemUpdate())
}

func TestV14Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.True(t, migration.HasWorkspaceUpdate())
}

func TestV14Migration_UpdateSystem(t *testing.T) {
	migration := &V14Migration{}
	cfg := &config.Config{}
	
	// Should return nil since no system updates
	err := migration.UpdateSystem(context.Background(), cfg, nil)
	assert.NoError(t, err)
}

func TestV14Migration_UpdateWorkspace(t *testing.T) {
	migration := &V14Migration{}
	cfg := &config.Config{}
	workspace := &domain.Workspace{ID: "test-workspace-123"}

	t.Run("successful migration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Expect ALTER TABLE for channel_options column
		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Expect CREATE INDEX for channel_options
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_channel_options").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.NoError(t, err)

		// Verify all expectations were met
		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("error adding column", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add channel_options column")
	})

	t.Run("error creating index", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("ALTER TABLE message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_channel_options").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(context.Background(), cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create index on channel_options")
	})
}
```

**Run:** `make test-migrations`

#### 2. Domain Layer Tests

**File:** `/workspace/internal/domain/message_history_test.go`

Add tests for ChannelOptions:

```go
func TestChannelOptions_Value(t *testing.T) {
	t.Run("converts to JSON for database storage", func(t *testing.T) {
		fromName := "Test Sender"
		options := domain.ChannelOptions{
			FromName: &fromName,
			CC:       []string{"cc1@example.com", "cc2@example.com"},
			BCC:      []string{"bcc@example.com"},
			ReplyTo:  "reply@example.com",
		}

		value, err := options.Value()
		require.NoError(t, err)
		require.NotNil(t, value)

		// Verify it's valid JSON
		jsonBytes, ok := value.([]byte)
		require.True(t, ok)
		
		var decoded domain.ChannelOptions
		err = json.Unmarshal(jsonBytes, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "Test Sender", *decoded.FromName)
		assert.Equal(t, 2, len(decoded.CC))
	})

	t.Run("returns nil for empty options", func(t *testing.T) {
		options := domain.ChannelOptions{}
		
		value, err := options.Value()
		require.NoError(t, err)
		assert.Nil(t, value)
	})
}

func TestChannelOptions_Scan(t *testing.T) {
	t.Run("scans from JSON bytes", func(t *testing.T) {
		jsonData := []byte(`{
			"from_name": "Test Sender",
			"cc": ["cc@example.com"],
			"bcc": ["bcc@example.com"],
			"reply_to": "reply@example.com"
		}`)

		var options domain.ChannelOptions
		err := options.Scan(jsonData)
		require.NoError(t, err)
		
		assert.Equal(t, "Test Sender", *options.FromName)
		assert.Equal(t, 1, len(options.CC))
		assert.Equal(t, "cc@example.com", options.CC[0])
	})

	t.Run("handles nil value", func(t *testing.T) {
		var options domain.ChannelOptions
		err := options.Scan(nil)
		require.NoError(t, err)
		assert.True(t, options.IsEmpty())
	})
}

func TestChannelOptions_IsEmpty(t *testing.T) {
	t.Run("returns true for empty options", func(t *testing.T) {
		options := domain.ChannelOptions{}
		assert.True(t, options.IsEmpty())
	})

	t.Run("returns false when from_name is set", func(t *testing.T) {
		fromName := "Test"
		options := domain.ChannelOptions{FromName: &fromName}
		assert.False(t, options.IsEmpty())
	})

	t.Run("returns false when CC is set", func(t *testing.T) {
		options := domain.ChannelOptions{CC: []string{"test@example.com"}}
		assert.False(t, options.IsEmpty())
	})
}

func TestEmailOptions_ToChannelOptions(t *testing.T) {
	t.Run("converts email options to channel options", func(t *testing.T) {
		fromName := "Test Sender"
		emailOptions := domain.EmailOptions{
			FromName: &fromName,
			CC:       []string{"cc@example.com"},
			BCC:      []string{"bcc@example.com"},
			ReplyTo:  "reply@example.com",
		}

		channelOptions := emailOptions.ToChannelOptions()
		require.NotNil(t, channelOptions)
		assert.Equal(t, "Test Sender", *channelOptions.FromName)
		assert.Equal(t, 1, len(channelOptions.CC))
	})

	t.Run("returns nil for empty email options", func(t *testing.T) {
		emailOptions := domain.EmailOptions{}
		channelOptions := emailOptions.ToChannelOptions()
		assert.Nil(t, channelOptions)
	})
}
```

**Run:** `make test-domain`

#### 3. Repository Layer Tests

**File:** `/workspace/internal/repository/message_history_postgre_test.go`

Add tests for channel_options:

```go
func TestMessageHistoryRepository_Create_WithChannelOptions(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := &repository.MessageHistoryRepository{
		workspaceRepo: mockWorkspaceRepo,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	fromName := "Test Sender"
	message := &domain.MessageHistory{
		ID:              "msg-123",
		ContactEmail:    "test@example.com",
		TemplateID:      "tpl-123",
		TemplateVersion: 1,
		Channel:         "email",
		MessageData:     domain.MessageData{Data: map[string]interface{}{"name": "Test"}},
		ChannelOptions: &domain.ChannelOptions{
			FromName: &fromName,
			CC:       []string{"cc@example.com"},
			BCC:      []string{"bcc@example.com"},
		},
		SentAt:    time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("creates message with channel options", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec("INSERT INTO message_history").
			WithArgs(
				message.ID,
				message.ContactEmail,
				sqlmock.AnyArg(), // external_id
				sqlmock.AnyArg(), // broadcast_id
				sqlmock.AnyArg(), // list_ids
				message.TemplateID,
				message.TemplateVersion,
				message.Channel,
				sqlmock.AnyArg(), // status_info
				sqlmock.AnyArg(), // message_data
				sqlmock.AnyArg(), // channel_options - should contain JSON
				sqlmock.AnyArg(), // attachments
				sqlmock.AnyArg(), // sent_at
				sqlmock.AnyArg(), // delivered_at
				sqlmock.AnyArg(), // failed_at
				sqlmock.AnyArg(), // opened_at
				sqlmock.AnyArg(), // clicked_at
				sqlmock.AnyArg(), // bounced_at
				sqlmock.AnyArg(), // complained_at
				sqlmock.AnyArg(), // unsubscribed_at
				sqlmock.AnyArg(), // created_at
				sqlmock.AnyArg(), // updated_at
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Create(ctx, workspaceID, message)
		assert.NoError(t, err)

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})
}

func TestMessageHistoryRepository_Get_WithChannelOptions(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := &repository.MessageHistoryRepository{
		workspaceRepo: mockWorkspaceRepo,
	}

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "msg-123"

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	channelOptionsJSON := []byte(`{
		"from_name": "Test Sender",
		"cc": ["cc@example.com"],
		"bcc": ["bcc@example.com"]
	}`)

	t.Run("retrieves message with channel options", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "list_ids",
			"template_id", "template_version", "channel", "status_info",
			"message_data", "channel_options", "attachments",
			"sent_at", "delivered_at", "failed_at", "opened_at",
			"clicked_at", "bounced_at", "complained_at", "unsubscribed_at",
			"created_at", "updated_at",
		}).AddRow(
			messageID, nil, "test@example.com", nil, "{}",
			"tpl-123", 1, "email", nil,
			`{"data": {}}`, channelOptionsJSON, nil,
			time.Now(), nil, nil, nil,
			nil, nil, nil, nil,
			time.Now(), time.Now(),
		)

		mock.ExpectQuery("SELECT (.+) FROM message_history WHERE id = ?").
			WithArgs(messageID).
			WillReturnRows(rows)

		message, err := repo.Get(ctx, workspaceID, messageID)
		require.NoError(t, err)
		require.NotNil(t, message)
		require.NotNil(t, message.ChannelOptions)
		assert.Equal(t, "Test Sender", *message.ChannelOptions.FromName)
		assert.Equal(t, 1, len(message.ChannelOptions.CC))

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})
}
```

**Run:** `make test-repo`

#### 4. Service Layer Tests

**File:** `/workspace/internal/service/email_service_test.go`

Add test for channel options storage:

```go
func TestEmailService_SendEmailForTemplate_StoresChannelOptions(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateService := mocks.NewMockTemplateService(ctrl)
	mockProviderService := mocks.NewMockEmailProviderService(ctrl)

	emailService := service.NewEmailService(
		mockMessageHistoryRepo,
		mockTemplateService,
		mockProviderService,
		logger.NewLogger("test", "info"),
	)

	ctx := context.Background()
	fromName := "Custom Sender"
	
	request := domain.SendEmailRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "msg-123",
		Contact: &domain.Contact{
			Email: "test@example.com",
		},
		TemplateConfig: domain.ChannelTemplate{
			TemplateID:      "tpl-123",
			TemplateVersion: 1,
		},
		MessageData: domain.MessageData{
			Data: map[string]interface{}{"name": "Test"},
		},
		EmailOptions: domain.EmailOptions{
			FromName: &fromName,
			CC:       []string{"cc@example.com"},
			BCC:      []string{"bcc@example.com"},
		},
		EmailProvider: &domain.EmailProvider{/* ... */},
	}

	// Mock template service
	mockTemplateService.EXPECT().
		GetTemplate(gomock.Any(), request.WorkspaceID, request.TemplateConfig.TemplateID, request.TemplateConfig.TemplateVersion).
		Return(&domain.Template{/* ... */}, nil)

	mockTemplateService.EXPECT().
		CompileTemplate(gomock.Any(), gomock.Any()).
		Return(&domain.CompiledTemplate{
			Subject: "Test Subject",
			HTML:    "Test HTML",
		}, nil)

	// Mock message history repository - verify channel_options is set
	mockMessageHistoryRepo.EXPECT().
		Create(gomock.Any(), request.WorkspaceID, gomock.Any()).
		Do(func(ctx context.Context, workspaceID string, message *domain.MessageHistory) {
			// Verify channel_options was populated
			require.NotNil(t, message.ChannelOptions)
			assert.Equal(t, "Custom Sender", *message.ChannelOptions.FromName)
			assert.Equal(t, 1, len(message.ChannelOptions.CC))
			assert.Equal(t, "cc@example.com", message.ChannelOptions.CC[0])
		}).
		Return(nil)

	// Mock provider service
	mockProviderService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		Return(nil)

	// Execute
	err := emailService.SendEmailForTemplate(ctx, request)
	assert.NoError(t, err)
}
```

**Run:** `make test-service`

### Frontend Tests

#### File: `/workspace/console/src/components/templates/__tests__/TemplatePreviewDrawer.test.tsx`

```typescript
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import TemplatePreviewDrawer from '../TemplatePreviewDrawer'
import type { MessageHistory } from '../../../services/api/messages_history'
import type { Template, Workspace } from '../../../services/api/types'

describe('TemplatePreviewDrawer - Channel Options Display', () => {
  const mockWorkspace: Workspace = {
    id: 'workspace-123',
    name: 'Test Workspace',
    settings: { timezone: 'UTC' }
  } as Workspace

  const mockTemplate: Template = {
    id: 'tpl-123',
    name: 'Test Template',
    version: 1,
    email: {
      subject: 'Test Subject',
      visual_editor_tree: {}
    }
  } as Template

  it('displays from_name override when present', () => {
    const messageHistory: MessageHistory = {
      id: 'msg-123',
      contact_email: 'test@example.com',
      template_id: 'tpl-123',
      template_version: 1,
      channel: 'email',
      message_data: { data: {} },
      channel_options: {
        from_name: 'Custom Sender Name'
      },
      sent_at: '2025-10-26T00:00:00Z',
      created_at: '2025-10-26T00:00:00Z',
      updated_at: '2025-10-26T00:00:00Z'
    }

    render(
      <TemplatePreviewDrawer
        record={mockTemplate}
        workspace={mockWorkspace}
        messageHistory={messageHistory}
      >
        <button>Preview</button>
      </TemplatePreviewDrawer>
    )

    expect(screen.getByText('From Name Override:')).toBeInTheDocument()
    expect(screen.getByText('Custom Sender Name')).toBeInTheDocument()
  })

  it('displays CC recipients as tags', () => {
    const messageHistory: MessageHistory = {
      id: 'msg-123',
      contact_email: 'test@example.com',
      template_id: 'tpl-123',
      template_version: 1,
      channel: 'email',
      message_data: { data: {} },
      channel_options: {
        cc: ['manager@example.com', 'team@example.com']
      },
      sent_at: '2025-10-26T00:00:00Z',
      created_at: '2025-10-26T00:00:00Z',
      updated_at: '2025-10-26T00:00:00Z'
    }

    render(
      <TemplatePreviewDrawer
        record={mockTemplate}
        workspace={mockWorkspace}
        messageHistory={messageHistory}
      >
        <button>Preview</button>
      </TemplatePreviewDrawer>
    )

    expect(screen.getByText('CC:')).toBeInTheDocument()
    expect(screen.getByText('manager@example.com')).toBeInTheDocument()
    expect(screen.getByText('team@example.com')).toBeInTheDocument()
  })

  it('displays BCC recipients as tags', () => {
    const messageHistory: MessageHistory = {
      id: 'msg-123',
      contact_email: 'test@example.com',
      template_id: 'tpl-123',
      template_version: 1,
      channel: 'email',
      message_data: { data: {} },
      channel_options: {
        bcc: ['archive@example.com']
      },
      sent_at: '2025-10-26T00:00:00Z',
      created_at: '2025-10-26T00:00:00Z',
      updated_at: '2025-10-26T00:00:00Z'
    }

    render(
      <TemplatePreviewDrawer
        record={mockTemplate}
        workspace={mockWorkspace}
        messageHistory={messageHistory}
      >
        <button>Preview</button>
      </TemplatePreviewDrawer>
    )

    expect(screen.getByText('BCC:')).toBeInTheDocument()
    expect(screen.getByText('archive@example.com')).toBeInTheDocument()
  })

  it('does not display channel options section when not present', () => {
    const messageHistory: MessageHistory = {
      id: 'msg-123',
      contact_email: 'test@example.com',
      template_id: 'tpl-123',
      template_version: 1,
      channel: 'email',
      message_data: { data: {} },
      channel_options: undefined, // No options
      sent_at: '2025-10-26T00:00:00Z',
      created_at: '2025-10-26T00:00:00Z',
      updated_at: '2025-10-26T00:00:00Z'
    }

    render(
      <TemplatePreviewDrawer
        record={mockTemplate}
        workspace={mockWorkspace}
        messageHistory={messageHistory}
      >
        <button>Preview</button>
      </TemplatePreviewDrawer>
    )

    expect(screen.queryByText('Message Delivery Options:')).not.toBeInTheDocument()
  })

  it('displays all channel options together', () => {
    const messageHistory: MessageHistory = {
      id: 'msg-123',
      contact_email: 'test@example.com',
      template_id: 'tpl-123',
      template_version: 1,
      channel: 'email',
      message_data: { data: {} },
      channel_options: {
        from_name: 'Support Team',
        cc: ['manager@example.com'],
        bcc: ['archive@example.com'],
        reply_to: 'support@example.com'
      },
      sent_at: '2025-10-26T00:00:00Z',
      created_at: '2025-10-26T00:00:00Z',
      updated_at: '2025-10-26T00:00:00Z'
    }

    render(
      <TemplatePreviewDrawer
        record={mockTemplate}
        workspace={mockWorkspace}
        messageHistory={messageHistory}
      >
        <button>Preview</button>
      </TemplatePreviewDrawer>
    )

    expect(screen.getByText('Message Delivery Options:')).toBeInTheDocument()
    expect(screen.getByText('From Name Override:')).toBeInTheDocument()
    expect(screen.getByText('Support Team')).toBeInTheDocument()
    expect(screen.getByText('CC:')).toBeInTheDocument()
    expect(screen.getByText('BCC:')).toBeInTheDocument()
    expect(screen.getByText('Reply To Override:')).toBeInTheDocument()
    expect(screen.getByText('support@example.com')).toBeInTheDocument()
  })
})
```

**Run:** `cd console && npm test`

### Test Commands Summary

```bash
# Backend tests
make test-migrations    # Test v14 migration
make test-domain       # Test ChannelOptions domain logic
make test-repo         # Test repository layer
make test-service      # Test service layer
make test-unit         # Run all backend unit tests
make coverage          # Generate coverage report

# Frontend tests
cd console && npm test                    # Run all tests
cd console && npm test -- TemplatePreviewDrawer  # Run specific test

# Integration tests
make test-integration  # Full end-to-end tests
```

---

## Deployment Plan

### Pre-Deployment Checklist

- [ ] **`internal/database/init.go` updated** with channel_options column and index
- [ ] All backend unit tests passing (`make test-unit`)
- [ ] All frontend tests passing (`cd console && npm test`)
- [ ] Migration tested on staging database
- [ ] Code reviewed and approved
- [ ] Documentation updated (CHANGELOG.md)
- [ ] Database backup taken

### Deployment Steps

#### Step 1: Database Migration (Automatic)

The migration will run automatically on application startup:

```bash
# Application detects version mismatch
# Current DB: 13.7, Code: 14.0
# Executes V14Migration for all workspaces
```

**Expected Behavior:**
- System database: No changes (HasSystemUpdate = false)
- Each workspace database: Adds `channel_options` column and index
- Migration time: ~1 second per workspace
- Zero downtime (column is nullable, existing code works)

#### Step 2: Verify Init Schema Update

Before deploying, verify the init.go schema includes channel_options:

```bash
# Check that init.go has been updated
grep "channel_options JSONB" internal/database/init.go

# Should see:
# channel_options JSONB,

# Check index is present
grep "idx_message_history_channel_options" internal/database/init.go
```

#### Step 3: Deploy Backend

```bash
# Build new version
make build

# Deploy to production
# (deployment method depends on your infrastructure)
```

**Verification:**
```bash
# Check version
curl https://your-api.com/api/health

# Verify database schema (connect to workspace DB)
psql -h localhost -U postgres -d notifuse_workspace_xxx
\d message_history
# Should show channel_options column
```

#### Step 4: Deploy Frontend

```bash
# Build frontend
cd console
npm run build

# Deploy static assets
# (deployment method depends on your infrastructure)
```

**Verification:**
- Open message history page
- Click preview on a recent message
- Should display channel options if present

### Post-Deployment Verification

#### Backend Health Checks

```bash
# 1. Check application logs for migration completion
grep "Migration completed" /var/log/notifuse/app.log

# 2. Verify database schema
psql -c "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'message_history' AND column_name = 'channel_options';"

# 3. Test message creation API
curl -X POST https://your-api.com/api/transactional.send \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "workspace_id": "workspace-123",
    "notification": {
      "id": "test-notification",
      "contact": {"email": "test@example.com"},
      "channels": ["email"],
      "email_options": {
        "from_name": "Test Sender",
        "cc": ["cc@example.com"]
      }
    }
  }'

# 4. Verify message was stored with channel_options
psql -c "SELECT id, channel_options FROM message_history WHERE contact_email = 'test@example.com' ORDER BY created_at DESC LIMIT 1;"
```

#### Frontend Health Checks

1. **Navigate to Messages Page:**
   - Go to workspace → Messages
   - Verify table loads without errors

2. **Test Preview Drawer:**
   - Click preview icon on any message
   - Drawer should open without errors
   - If message has channel_options, they should display

3. **Send Test Email with Options:**
   - Use transactional API or broadcast with CC/BCC
   - Preview the sent message
   - Verify CC/BCC display in preview drawer

### Rollback Plan

If critical issues are discovered:

#### Backend Rollback

```bash
# Option 1: Rollback code (database changes remain)
# Deploy previous version (13.7)
# Application will continue to work (ignores channel_options column)

# Option 2: Rollback database (not recommended)
# Connect to each workspace database
psql -c "ALTER TABLE message_history DROP COLUMN IF EXISTS channel_options;"
psql -c "DROP INDEX IF EXISTS idx_message_history_channel_options;"

# Update system version
psql -c "UPDATE settings SET value = '13.7' WHERE key = 'version';"
```

#### Frontend Rollback

```bash
# Deploy previous frontend build
# No database changes needed
```

**Note:** Since `channel_options` is nullable and all code is backward-compatible, code rollback is safe without database rollback.

---

## Risk Assessment

### High Priority Risks

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Migration fails on large workspace | High | Low | Idempotent migration with IF NOT EXISTS, transactional |
| Performance degradation on queries | Medium | Low | GIN index created; monitor query performance |
| Existing code breaks with new column | High | Very Low | Column is nullable; existing code ignores it |
| Data serialization errors | Medium | Low | Comprehensive tests; type-safe JSON handling |

### Medium Priority Risks

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Frontend doesn't display options | Low | Medium | Graceful degradation; section only shows if data present |
| JSONB index grows large | Low | Low | GIN index only on non-null values; monitor disk usage |
| Type mismatches in JSONB | Low | Low | Strict TypeScript types; backend validation |

### Low Priority Risks

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Confusion about NULL vs empty object | Low | Low | Clear documentation; IsEmpty() helper |
| SMS options stored in email messages | Low | Very Low | Channel-based conditional rendering |

---

## Success Criteria

### Technical Success Criteria

- [ ] Migration completes successfully for all workspaces
- [ ] All backend unit tests passing (100% for new code)
- [ ] All frontend tests passing
- [ ] Integration tests passing
- [ ] No performance degradation in message queries
- [ ] No errors in production logs for 24 hours post-deployment

### Functional Success Criteria

- [ ] New messages store channel_options when provided
- [ ] Preview drawer displays CC, BCC, FromName correctly
- [ ] Existing messages (with NULL channel_options) display without errors
- [ ] API responses include channel_options field
- [ ] Documentation updated and accurate

### User Experience Criteria

- [ ] Preview drawer loads in < 500ms
- [ ] Channel options display is visually clear
- [ ] No breaking changes to existing workflows
- [ ] Support team can view message delivery options

---

## Timeline

### Estimated Development Time

| Phase | Task | Estimated Time | Owner |
|-------|------|----------------|-------|
| **Phase 1** | Update init.go schema | 15 mins | Backend Dev |
| **Phase 1** | Create migration v14.go | 1 hour | Backend Dev |
| **Phase 1** | Create migration tests | 1 hour | Backend Dev |
| **Phase 1** | Update config.go version | 5 mins | Backend Dev |
| **Phase 2** | Update domain layer | 2 hours | Backend Dev |
| **Phase 2** | Update repository layer | 2 hours | Backend Dev |
| **Phase 2** | Add repository tests | 2 hours | Backend Dev |
| **Phase 3** | Update service layer | 1.5 hours | Backend Dev |
| **Phase 3** | Add service tests | 1.5 hours | Backend Dev |
| **Phase 4** | Update TypeScript interfaces | 30 mins | Frontend Dev |
| **Phase 5** | Update TemplatePreviewDrawer | 2 hours | Frontend Dev |
| **Phase 5** | Update MessageHistoryTable | 30 mins | Frontend Dev |
| **Phase 5** | Add frontend tests | 2 hours | Frontend Dev |
| **Phase 6** | Integration testing | 3 hours | QA |
| **Phase 7** | Documentation | 1 hour | Tech Writer |
| **Phase 8** | Code review | 2 hours | Team |
| **Phase 9** | Deployment prep | 1 hour | DevOps |
| **Total** | | **23.25 hours** | |

### Suggested Schedule

**Week 1:**
- Day 1-2: Backend implementation (Phases 1-3)
- Day 3: Frontend implementation (Phases 4-5)
- Day 4: Testing and bug fixes (Phase 6)
- Day 5: Code review and documentation (Phases 7-8)

**Week 2:**
- Day 1: Staging deployment and testing
- Day 2: Production deployment
- Day 3-5: Monitoring and issue resolution

---

## Appendix

### A. SQL Schema Reference

**Before Migration (v13.7):**
```sql
CREATE TABLE message_history (
    id VARCHAR(255) NOT NULL PRIMARY KEY,
    contact_email VARCHAR(255) NOT NULL,
    external_id VARCHAR(255),
    broadcast_id VARCHAR(255),
    list_ids TEXT[],
    template_id VARCHAR(32) NOT NULL,
    template_version INTEGER NOT NULL,
    channel VARCHAR(20) NOT NULL,
    status_info VARCHAR(255),
    message_data JSONB NOT NULL,
    attachments JSONB,
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delivered_at TIMESTAMP WITH TIME ZONE,
    failed_at TIMESTAMP WITH TIME ZONE,
    opened_at TIMESTAMP WITH TIME ZONE,
    clicked_at TIMESTAMP WITH TIME ZONE,
    bounced_at TIMESTAMP WITH TIME ZONE,
    complained_at TIMESTAMP WITH TIME ZONE,
    unsubscribed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**After Migration (v14.0):**
```sql
CREATE TABLE message_history (
    id VARCHAR(255) NOT NULL PRIMARY KEY,
    contact_email VARCHAR(255) NOT NULL,
    external_id VARCHAR(255),
    broadcast_id VARCHAR(255),
    list_ids TEXT[],
    template_id VARCHAR(32) NOT NULL,
    template_version INTEGER NOT NULL,
    channel VARCHAR(20) NOT NULL,
    status_info VARCHAR(255),
    message_data JSONB NOT NULL,
    channel_options JSONB,  -- NEW
    attachments JSONB,
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delivered_at TIMESTAMP WITH TIME ZONE,
    failed_at TIMESTAMP WITH TIME ZONE,
    opened_at TIMESTAMP WITH TIME ZONE,
    clicked_at TIMESTAMP WITH TIME ZONE,
    bounced_at TIMESTAMP WITH TIME ZONE,
    complained_at TIMESTAMP WITH TIME ZONE,
    unsubscribed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_message_history_channel_options 
ON message_history USING gin(channel_options);  -- NEW
```

### B. Example API Responses

**GET /api/messages.list (v14.0):**
```json
{
  "messages": [
    {
      "id": "msg-abc123",
      "contact_email": "user@example.com",
      "template_id": "tpl-welcome",
      "template_version": 2,
      "channel": "email",
      "message_data": {
        "data": {
          "firstName": "John",
          "orderNumber": "ORD-12345"
        }
      },
      "channel_options": {
        "from_name": "Support Team",
        "cc": ["manager@example.com"],
        "bcc": ["archive@example.com"],
        "reply_to": "support@example.com"
      },
      "sent_at": "2025-10-26T10:30:00Z",
      "delivered_at": "2025-10-26T10:30:15Z",
      "created_at": "2025-10-26T10:30:00Z",
      "updated_at": "2025-10-26T10:30:15Z"
    },
    {
      "id": "msg-def456",
      "contact_email": "another@example.com",
      "template_id": "tpl-newsletter",
      "template_version": 1,
      "channel": "email",
      "message_data": {
        "data": {
          "title": "Weekly Update"
        }
      },
      "channel_options": null,
      "sent_at": "2025-10-26T09:00:00Z",
      "created_at": "2025-10-26T09:00:00Z",
      "updated_at": "2025-10-26T09:00:00Z"
    }
  ],
  "next_cursor": "eyJjcmVhdGVkX2F0IjoxNzI5OTQy...",
  "has_more": true
}
```

### C. Migration Verification Queries

```sql
-- 1. Check if column exists
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name = 'message_history' 
  AND column_name = 'channel_options';

-- Expected: channel_options | jsonb | YES

-- 2. Check if index exists
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = 'message_history'
  AND indexname = 'idx_message_history_channel_options';

-- Expected: idx_message_history_channel_options | CREATE INDEX ...

-- 3. Count messages with channel_options
SELECT 
    COUNT(*) as total_messages,
    COUNT(channel_options) as messages_with_options,
    COUNT(*) - COUNT(channel_options) as messages_without_options
FROM message_history;

-- 4. Sample channel_options data
SELECT 
    id,
    contact_email,
    channel_options,
    created_at
FROM message_history
WHERE channel_options IS NOT NULL
ORDER BY created_at DESC
LIMIT 5;

-- 5. Verify index is being used
EXPLAIN ANALYZE
SELECT * FROM message_history
WHERE channel_options @> '{"from_name": "Support"}';

-- Should show "Bitmap Index Scan" with idx_message_history_channel_options
```

### D. Monitoring Queries

```sql
-- Monitor new messages with channel_options
SELECT 
    DATE(created_at) as date,
    COUNT(*) as total_messages,
    COUNT(channel_options) as with_options,
    ROUND(100.0 * COUNT(channel_options) / COUNT(*), 2) as percentage
FROM message_history
WHERE created_at > CURRENT_DATE - INTERVAL '7 days'
GROUP BY DATE(created_at)
ORDER BY date DESC;

-- Check most common channel_options patterns
SELECT 
    jsonb_object_keys(channel_options) as option_key,
    COUNT(*) as usage_count
FROM message_history
WHERE channel_options IS NOT NULL
GROUP BY option_key
ORDER BY usage_count DESC;

-- Find messages with CC recipients
SELECT 
    id,
    contact_email,
    channel_options->'cc' as cc_recipients,
    created_at
FROM message_history
WHERE channel_options ? 'cc'
ORDER BY created_at DESC
LIMIT 10;
```

---

**Document Version:** 1.0  
**Last Updated:** 2025-10-26  
**Next Review:** After deployment completion
