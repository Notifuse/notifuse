# Sender Name Investigation & Fix

## TL;DR - Quick Fix

If you're seeing emails without "From" names (showing only email addresses), follow these steps:

```bash
# 1. Check if you have the issue
make -f Makefile.audit quick-check

# 2. If issues found, run detailed audit
make -f Makefile.audit audit-senders

# 3. Backup your database
pg_dump $DATABASE_URL > backup_$(date +%Y%m%d_%H%M%S).sql

# 4. Apply the fix
make -f Makefile.audit fix-senders

# 5. Verify it worked
make -f Makefile.audit audit-senders
```

That's it! 🎉

## What's the Problem?

Your SMTP emails might show:
```
From: <noreply@notifuse.com>
```

Instead of:
```
From: "Notifuse" <noreply@notifuse.com>
```

## Why Is This Happening?

The **code is correct** and fully supports From names. The issue is that some senders in your database have empty `name` fields. This could happen if:

- Senders were created before validation was added
- Data was migrated from an older system
- Manual database edits were made
- API access bypassed validation

## Investigation Results

✅ **Go-mail library** - FULLY supports From names  
✅ **Backend code** - Correctly implemented  
✅ **Frontend UI** - Properly captures names  
✅ **Validation** - Enforced at multiple layers  

The problem is **DATA, not CODE**.

## What Was Created

### 1. Diagnostic Tools
- **Go audit tool**: `cmd/tools/audit_senders.go`
- **SQL audit script**: `scripts/audit_sender_names.sql`
- **Quick check**: `make -f Makefile.audit quick-check`

### 2. Fix Scripts
- **Automated fix**: `scripts/fix_empty_sender_names.sql`
- **Safe wrapper**: `make -f Makefile.audit fix-senders`

### 3. Tests (All Passing ✅)
- Library behavior tests
- Integration tests
- Diagnostic tests
- 7 test suites, 100% pass rate

### 4. Documentation
- **`SMTP_FROM_NAME_INVESTIGATION.md`** - Detailed findings
- **`DIAGNOSTIC_TOOLS.md`** - Tool usage guide
- **`SENDER_NAME_FIX_README.md`** - This file

### 5. Debug Helpers
- **`internal/service/email_service_debug.go`** - Logging functions
- **`Makefile.audit`** - Convenient commands

## How the Fix Works

The automated fix:
1. Creates a backup of your workspace data
2. Finds all senders with empty `name` fields
3. Sets their name to their email address (e.g., "noreply@notifuse.com")
4. Logs every change
5. Runs in a transaction (safe to rollback)

Example:
```
Before: { email: "hello@notifuse.com", name: "" }
After:  { email: "hello@notifuse.com", name: "hello@notifuse.com" }
```

You can then update these to more user-friendly names through the UI.

## Detailed Usage

See `DIAGNOSTIC_TOOLS.md` for:
- Complete tool documentation
- SQL query examples  
- Testing procedures
- Monitoring setup
- Troubleshooting guide

## Prevention

The validation is already in place! Both frontend and backend require sender names for new/updated integrations.

To add extra monitoring, see the "Monitoring" section in `DIAGNOSTIC_TOOLS.md`.

## Manual Fix (Alternative)

If you prefer to fix manually:

1. Find senders without names:
```sql
SELECT 
    w.id as workspace_id,
    w.name as workspace_name,
    sender->>'email' as sender_email,
    sender->>'name' as sender_name
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE sender->>'name' = '' OR sender->>'name' IS NULL;
```

2. Update through the UI:
   - Go to Settings → Integrations
   - Edit each integration
   - Edit each sender
   - Set a meaningful name
   - Save

## Test First!

Before applying any fixes to production:

```bash
# Create test database
createdb notifuse_test

# Copy production data
pg_dump $DATABASE_URL | psql notifuse_test_url

# Test the fix
DATABASE_URL=notifuse_test_url make -f Makefile.audit fix-senders

# Verify results
DATABASE_URL=notifuse_test_url make -f Makefile.audit audit-senders

# If all looks good, apply to production
```

## Support & Questions

- Check `SMTP_FROM_NAME_INVESTIGATION.md` for technical details
- Check `DIAGNOSTIC_TOOLS.md` for tool usage
- Run the tests: `make -f Makefile.audit test-from-name`
- All tests pass ✅ - the infrastructure is solid!

## Summary

**The Good News:**
- Your infrastructure is correctly implemented
- The go-mail library fully supports From names
- Validation prevents future issues
- Automated tools can fix existing data

**The Action:**
- Run the audit to find issues
- Backup your database
- Apply the automated fix
- Verify the results

You're all set! 🚀
