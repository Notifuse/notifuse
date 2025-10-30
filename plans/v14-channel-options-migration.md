# V14 Migration: Channel Options Storage

## Summary

Version 14.0 adds a `channel_options` JSONB column to the `message_history` table to store channel-specific delivery options like CC, BCC, FromName, and ReplyTo for email messages. This enables the message preview UI to display these options and provides a future-proof structure for SMS/push notification options.

## Database Changes

### Schema Updates

**Table: `message_history`**
- Added column: `channel_options JSONB` (nullable)

**Migration Strategy:**
- Migration is idempotent using `IF NOT EXISTS` clauses
- Existing messages will have `channel_options = NULL` (no backfill)
- New messages will store options when provided via API
- Estimated migration time: < 1 second per workspace

## API Changes

### Domain Types

**New Type: `ChannelOptions`** (in `internal/domain/message_history.go`)
```go
type ChannelOptions struct {
    FromName *string  `json:"from_name,omitempty"`
    CC       []string `json:"cc,omitempty"`
    BCC      []string `json:"bcc,omitempty"`
    ReplyTo  string   `json:"reply_to,omitempty"`
    // Future: SMS options would go here
    // Future: Push notification options would go here
}
```

**Updated Type: `MessageHistory`**
- Added field: `ChannelOptions *ChannelOptions`

**EmailOptions Enhancement:**
- Added method: `IsEmpty() bool` - checks if any options are set
- Added method: `ToChannelOptions() *ChannelOptions` - converts to storage format

### Frontend Types

**TypeScript Interface: `ChannelOptions`** (in `console/src/services/api/messages_history.ts`)
```typescript
interface ChannelOptions {
  from_name?: string
  cc?: string[]
  bcc?: string[]
  reply_to?: string
}
```

**Updated Interface: `MessageHistory`**
- Added field: `channel_options?: ChannelOptions`

## UI Changes

### Message Preview Drawer

The template preview drawer now displays channel options when viewing message history:

**Display Sections:**
1. **From Name Override** - Shows custom sender name if provided
2. **CC Recipients** - Blue tags for carbon copy recipients
3. **BCC Recipients** - Purple tags for blind carbon copy recipients  
4. **Reply To Override** - Shows custom reply-to address if provided

**Implementation:** `console/src/components/templates/TemplatePreviewDrawer.tsx`

## Backend Changes

### Repository Layer

**File: `internal/repository/message_history_postgre.go`**
- Updated `scanMessage()` to scan `channel_options` column
- Updated `messageHistorySelectFields()` to include `channel_options`
- Updated `Create()` INSERT statement to include `channel_options` parameter

### Service Layer

**File: `internal/service/email_service.go`**
- Added conversion: `channelOptions := request.EmailOptions.ToChannelOptions()`
- Stores channel options in message history record
- Only stores when options are provided (null otherwise)

**File: `internal/service/transactional_service.go`**
- Already passes `EmailOptions` through to email service
- No changes required (uses existing `params.EmailOptions`)

## Migration Files

### Created Files

1. **`internal/migrations/v14.go`**
   - Implements `V14Migration` struct
   - Adds `channel_options` column to workspace databases
   - Version: 14.0, Workspace-only migration

2. **`internal/migrations/v14_test.go`**
   - Tests migration version, flags, and execution
   - Verifies column and index creation
   - Tests idempotency (can run multiple times safely)

3. **`internal/database/init.go`**
   - Updated workspace database initialization
   - Includes `channel_options` column for new workspaces

## Testing

### Domain Layer Tests

**File: `internal/domain/email_provider_test.go`**
- `TestEmailOptions_IsEmpty()` - Tests empty detection logic
- `TestEmailOptions_ToChannelOptions()` - Tests conversion to storage format

**File: `internal/domain/message_history_test.go`**
- `TestChannelOptions_Value()` - Tests database Value() method (JSON encoding)
- `TestChannelOptions_Scan()` - Tests database Scan() method (JSON decoding)
- `TestChannelOptions_Scan_SQLErrors()` - Tests error handling

### Migration Tests

**File: `internal/migrations/v14_test.go`**
- Verifies migration metadata (version, flags)
- Tests workspace database column addition
- Verifies idempotency

**File: `internal/migrations/manager_test.go`**
- Updated version assertions from 13 to 14

## Backward Compatibility

### Data Safety
- ✅ Existing messages are not modified
- ✅ Null `channel_options` are handled correctly
- ✅ No breaking changes to API contracts
- ✅ Frontend gracefully handles missing `channel_options`

### Rollback Strategy
- Migration adds a nullable column (safe)
- Can be rolled back by removing the column
- No data loss on rollback (new data not used by older versions)

## Performance Considerations

### Database
- **Nullable Column**: No storage overhead for messages without options
- **JSONB Storage**: Efficient binary JSON format

### Application
- **Minimal Overhead**: Only converts options when provided
- **No N+1 Queries**: Options loaded with message in single query
- **Frontend**: React renders options conditionally (no performance impact)

## Future Enhancements

The `ChannelOptions` structure is designed for extensibility:

```go
type ChannelOptions struct {
    // Email options (v14)
    FromName *string  `json:"from_name,omitempty"`
    CC       []string `json:"cc,omitempty"`
    BCC      []string `json:"bcc,omitempty"`
    ReplyTo  string   `json:"reply_to,omitempty"`
    
    // Future: SMS options (v15+)
    // SMSFrom  *string  `json:"sms_from,omitempty"`
    // SMSType  *string  `json:"sms_type,omitempty"`
    
    // Future: Push options (v16+)
    // PushSound   *string `json:"push_sound,omitempty"`
    // PushBadge   *int    `json:"push_badge,omitempty"`
}
```

## Deployment Checklist

- [x] Update `config/config.go` VERSION to "14.0"
- [x] Add CHANGELOG.md entry for v14.0
- [x] Create v14 migration file
- [x] Update database init.go for new workspaces
- [x] Add ChannelOptions to domain types
- [x] Update repository layer for storage/retrieval
- [x] Update service layer to store options
- [x] Add frontend TypeScript types
- [x] Update UI to display channel options
- [x] Add comprehensive unit tests
- [x] Add migration tests
- [x] Update migration manager tests

## Files Changed

### Backend (Go)
- `config/config.go` - Version bump
- `internal/database/init.go` - Schema update
- `internal/domain/email_provider.go` - Helper methods
- `internal/domain/email_provider_test.go` - New tests
- `internal/domain/message_history.go` - ChannelOptions type
- `internal/domain/message_history_test.go` - New tests
- `internal/migrations/v14.go` - New migration
- `internal/migrations/v14_test.go` - New tests
- `internal/migrations/manager_test.go` - Version updates
- `internal/repository/message_history_postgre.go` - Storage updates
- `internal/service/email_service.go` - Conversion logic

### Frontend (TypeScript/React)
- `console/src/services/api/messages_history.ts` - Type definitions
- `console/src/components/messages/MessageHistoryTable.tsx` - Pass messageHistory prop
- `console/src/components/templates/TemplatePreviewDrawer.tsx` - UI display

### Documentation
- `CHANGELOG.md` - Release notes for v14.0

## Rollout Strategy

1. **Pre-deployment**: Backup databases
2. **Deployment**: Deploy application with v14.0
3. **Migration**: Automatic on first startup per workspace
4. **Verification**: Check logs for successful migrations
5. **Monitoring**: Monitor query performance on new index

## Success Criteria

✅ All workspaces migrated successfully  
✅ New messages store channel_options when provided  
✅ Message preview shows channel options correctly  
✅ No performance degradation  
✅ All tests passing  
✅ No breaking changes to existing functionality
