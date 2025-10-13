# Sender Name Diagnostic Tools

This document describes the tools available to diagnose and fix sender name issues.

## Quick Start

### 1. Check if you have any issues:
```bash
make -f Makefile.audit quick-check
```

### 2. Run detailed audit:
```bash
# Option A: Using Go tool (recommended)
make -f Makefile.audit audit-senders

# Option B: Using SQL directly
make -f Makefile.audit audit-sql
```

### 3. If issues found, fix them:
```bash
# This will:
# - Ask for confirmation
# - Create a backup
# - Apply fixes
make -f Makefile.audit fix-senders
```

### 4. Verify the fix:
```bash
make -f Makefile.audit audit-senders
```

## Tools Overview

### 1. Go-Based Audit Tool
**File:** `cmd/tools/audit_senders.go`

**What it does:**
- Scans all workspaces and integrations
- Identifies senders with empty or missing names
- Highlights CRITICAL issues (active providers with empty names)
- Shows less critical issues (unused integrations)
- Provides actionable recommendations

**Usage:**
```bash
go run cmd/tools/audit_senders.go
```

**Output:**
```
==============================
SENDER NAME AUDIT RESULTS
==============================

Total issues found: 5
Critical issues (active providers): 2

🚨 CRITICAL: Active Providers with Empty Sender Names
--------------------------------------------------
❌ Workspace: My Company (ws-123)
   Integration: SMTP Provider (int-456) [Marketing]
   Sender: noreply@company.com
   Name: '' (EMPTY)
   ⚠️  This is the DEFAULT sender
```

### 2. SQL Audit Scripts
**File:** `scripts/audit_sender_names.sql`

**What it does:**
- 6 different SQL queries to analyze sender names
- Total counts of issues
- Detailed lists of problematic senders
- Focus on active providers (marketing/transactional)

**Key queries:**
1. Total workspaces with integrations
2. Count of integrations with empty sender names
3. Detailed list of problematic senders
4. Sender status breakdown
5. Default senders with missing names (HIGH PRIORITY)
6. Active provider sender analysis

**Usage:**
```bash
psql $DATABASE_URL -f scripts/audit_sender_names.sql
```

### 3. Automated Fix Script
**File:** `scripts/fix_empty_sender_names.sql`

**What it does:**
- Creates a backup of workspace data
- Updates senders with empty names to use their email address as the name
- Provides detailed logging of changes
- Runs in a transaction (can be rolled back)

**Safety features:**
- Creates temporary backup table
- Uses transaction (ROLLBACK by default)
- Logs every change
- Verify step before commit

**Usage:**
```bash
# IMPORTANT: Backup first!
pg_dump $DATABASE_URL > backup_before_fix.sql

# Run the fix (will ROLLBACK by default)
psql $DATABASE_URL -f scripts/fix_empty_sender_names.sql

# If you're satisfied with the changes, edit the file:
# Change last line from ROLLBACK to COMMIT
```

### 4. Debug Logging Functions
**File:** `internal/service/email_service_debug.go`

**What it does:**
- Helper functions to log sender details
- Can be added to email service methods for debugging
- Logs warnings when senders have empty names

**Functions:**
```go
// Log detailed sender information
logSenderDetails(ctx, log, sender, "sending email")

// Validate sender has name and log warning if not
validateSenderHasName(log, sender, "template processing")

// Debug all senders in a provider
debugEmailProviderSenders(log, provider, "integration setup")
```

**To enable debug logging:**
Add to `email_service.go` after getting sender:
```go
emailSender := request.EmailProvider.GetSender(template.Email.SenderID)
if emailSender == nil {
    return fmt.Errorf("sender not found: %s", template.Email.SenderID)
}

// Add this line:
logSenderDetails(ctx, s.logger, emailSender, "SendEmailForTemplate")
```

## Makefile Commands

**File:** `Makefile.audit`

### Available Commands:

```bash
# Show help
make -f Makefile.audit help

# Run Go audit tool
make -f Makefile.audit audit-senders

# Run SQL audit
make -f Makefile.audit audit-sql

# Apply automated fixes (interactive, with safety checks)
make -f Makefile.audit fix-senders

# Run all From name unit tests
make -f Makefile.audit test-from-name

# Quick check (just count issues)
make -f Makefile.audit quick-check
```

## Common Scenarios

### Scenario 1: "I'm seeing emails without From names"

```bash
# Step 1: Check if the issue is in your database
make -f Makefile.audit audit-senders

# Step 2: If issues found, check which providers are affected
psql $DATABASE_URL -f scripts/audit_sender_names.sql

# Step 3: Fix the data
make -f Makefile.audit fix-senders

# Step 4: Verify
make -f Makefile.audit audit-senders
```

### Scenario 2: "I want to prevent this in the future"

The validation is already in place! Both frontend and backend require sender names.

To add extra monitoring:
```go
// Add to email_service.go after getting sender:
if emailSender.Name == "" {
    s.logger.WithFields(map[string]interface{}{
        "sender_id":    emailSender.ID,
        "sender_email": emailSender.Email,
        "workspace_id": request.WorkspaceID,
        "template_id":  template.ID,
    }).Error("⚠️ ALERT: Attempting to send email with empty sender name!")
}
```

### Scenario 3: "I want to check before deployment"

Add to your CI/CD pipeline:
```bash
# In your CI script:
make -f Makefile.audit audit-senders
# This exits with code 1 if critical issues found
```

## SQL Queries for Manual Investigation

### Find all senders without names:
```sql
SELECT 
    w.id,
    w.name,
    sender->>'email' as sender_email,
    sender->>'name' as sender_name
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE sender->>'name' = '' OR sender->>'name' IS NULL;
```

### Find active providers with empty sender names:
```sql
SELECT 
    w.id,
    w.name,
    CASE 
        WHEN w.settings->>'marketing_email_provider_id' = integration->>'id' 
        THEN 'Marketing'
        WHEN w.settings->>'transactional_email_provider_id' = integration->>'id' 
        THEN 'Transactional'
    END as provider_type,
    sender->>'email' as sender_email
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE (sender->>'name' = '' OR sender->>'name' IS NULL)
  AND (
    w.settings->>'marketing_email_provider_id' = integration->>'id'
    OR w.settings->>'transactional_email_provider_id' = integration->>'id'
  );
```

### Count senders by workspace:
```sql
SELECT 
    w.id,
    w.name,
    COUNT(*) FILTER (WHERE sender->>'name' = '' OR sender->>'name' IS NULL) as empty_names,
    COUNT(*) as total_senders
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
GROUP BY w.id, w.name
HAVING COUNT(*) FILTER (WHERE sender->>'name' = '' OR sender->>'name' IS NULL) > 0;
```

## Testing

### Run unit tests:
```bash
# All From name tests
make -f Makefile.audit test-from-name

# Specific test files
go test -v ./internal/service -run TestGoMailFromFormat
go test -v ./internal/service -run TestEmailSenderNamePreservation
go test -v ./internal/service -run TestDebugFromNameInActualEmail
```

### Test the fix script safely:
```bash
# 1. Create a test database
createdb notifuse_test

# 2. Copy production data
pg_dump $DATABASE_URL | psql $DATABASE_URL_TEST

# 3. Run fix script on test database
psql $DATABASE_URL_TEST -f scripts/fix_empty_sender_names.sql

# 4. Verify results
psql $DATABASE_URL_TEST -f scripts/audit_sender_names.sql

# 5. If satisfied, run on production (with backup!)
```

## Monitoring

### Set up alerts:

Add this to your monitoring system (Prometheus/Datadog/etc):

```sql
-- Query to monitor empty sender names
SELECT COUNT(*) 
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE (sender->>'name' = '' OR sender->>'name' IS NULL)
  AND (
    w.settings->>'marketing_email_provider_id' = integration->>'id'
    OR w.settings->>'transactional_email_provider_id' = integration->>'id'
  );
```

Alert if this count > 0.

## Support

If you encounter issues:

1. Check the investigation report: `SMTP_FROM_NAME_INVESTIGATION.md`
2. Review test files for expected behavior
3. Run the diagnostic tools
4. Check application logs for sender-related warnings

## Files Reference

- `SMTP_FROM_NAME_INVESTIGATION.md` - Detailed investigation report
- `cmd/tools/audit_senders.go` - Go-based audit tool
- `scripts/audit_sender_names.sql` - SQL audit queries
- `scripts/fix_empty_sender_names.sql` - Automated fix script
- `internal/service/email_service_debug.go` - Debug logging helpers
- `Makefile.audit` - Convenient make commands
- `internal/service/smtp_service_from_format_test.go` - Library behavior tests
- `internal/service/smtp_service_sender_name_integration_test.go` - Integration tests
- `internal/service/smtp_service_debug_test.go` - Diagnostic tests
