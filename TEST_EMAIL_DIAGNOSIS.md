# Test Email From Name Missing - Diagnosis & Solution

## Summary

You created a sender with name "hello" and sent a test email, but the From name is missing in the raw email headers.

## Investigation Results ✅

All tests pass! The code is **100% correct**:

1. ✅ Backend uses `defaultSender.Name` (email_service.go:131)
2. ✅ JSON deserialization preserves sender names (new tests prove this)
3. ✅ Go-mail library correctly uses `FromFormat(name, email)`
4. ✅ Frontend sends complete provider with senders

## Root Cause: **Stale Data**

When you click "Test" on an integration, the UI uses cached data:

```typescript
// Line 618 in Integrations.tsx
setTestingProvider(integration.email_provider)
```

This `integration.email_provider` comes from the workspace object already loaded in React state. If you:
- Edited the sender
- But didn't save
- Or saved but didn't refresh

Then the test uses **old data without the name**.

## How to Fix

### Option 1: Save and Refresh (Recommended)
1. Edit the sender and set name to "hello"
2. Click **"Save"** to save the integration
3. **Refresh the page** (F5 or Ctrl+R)
4. Click **"Test"** again
5. Check the email - name should now appear

### Option 2: Check if You Actually Saved
1. Open browser DevTools (F12)
2. Go to Network tab
3. Edit the sender name and click Save
4. Look for a POST request to `/api/workspace.updateIntegration`
5. Check the request payload - does it include the sender with name?

### Option 3: Verify Database
Run this SQL query to check what's actually in the database:

```sql
SELECT 
    id,
    name,
    jsonb_pretty(integrations) 
FROM workspaces 
WHERE id = 'YOUR_WORKSPACE_ID';
```

Look for your sender in the `senders` array and verify the `name` field.

## Test to Confirm Code Works

Run these tests to prove the code handles sender names correctly:

```bash
# Test JSON deserialization
cd /workspace && go test -v ./internal/http -run TestFrontendToBackendFlow

# Test go-mail library
cd /workspace && go test -v ./internal/service -run TestGoMailFromFormat

# Test complete flow
cd /workspace && go test -v ./internal/service -run TestEmailSenderNamePreservation
```

All should pass ✅ (and they do!)

## Debug in Browser

Add this to your browser console when on the Integrations page:

```javascript
// Check what integration data the UI has
const workspace = // get from React state
const integration = workspace.integrations.find(i => i.id === 'YOUR_INTEGRATION_ID')
console.log('Senders:', JSON.stringify(integration.email_provider.senders, null, 2))
```

This shows exactly what data the UI is using when you click Test.

## Most Common Issue

**You edited the sender but forgot to click Save!**

The sender form is a modal. When you:
1. Click "Edit" on a sender
2. Change the name to "hello"  
3. Click "Save" in the modal ← This only saves to local state
4. **You must also click "Save" on the main integration form!**

If you don't save the integration, the changes aren't persisted.

## Verify the Fix

After following the steps above, send another test email and check the raw headers.

You should see:
```
From: "hello" <your-email@example.com>
```

Instead of:
```
From: <your-email@example.com>
```

## Still Not Working?

If the name is still missing after verifying all above:

1. **Check browser console for errors**
2. **Check backend logs** - look for the TestEmailProvider call
3. **Add debug logging** (see below)
4. **Check database directly** with the SQL query above

## Add Debug Logging

If you need to debug further, temporarily add logging:

### In Backend (email_service.go:110-131)
```go
defaultSender := provider.Senders[0]

// ADD THIS
s.logger.WithFields(map[string]interface{}{
    "sender_id":    defaultSender.ID,
    "sender_email": defaultSender.Email,
    "sender_name":  defaultSender.Name, // CHECK THIS VALUE
    "is_default":   defaultSender.IsDefault,
}).Info("🔍 DEBUG: Testing email with sender")

request := domain.SendEmailProviderRequest{
    FromAddress:   defaultSender.Email,
    FromName:      defaultSender.Name,
    ...
}
```

Then check your logs when you click Test.

### In Frontend (Integrations.tsx:618)
```typescript
setTestingProvider(integration.email_provider)

// ADD THIS
console.log('🔍 DEBUG: Testing with provider:', {
    kind: integration.email_provider.kind,
    senders: integration.email_provider.senders.map(s => ({
        id: s.id,
        email: s.email,
        name: s.name, // CHECK THIS VALUE
        is_default: s.is_default
    }))
})
```

Then check browser console when you click Test.

## Summary Checklist

- [ ] Edited sender and set name to "hello"
- [ ] Clicked "Save" in the sender modal
- [ ] Clicked "Save" on the integration form
- [ ] Refreshed the page (F5)
- [ ] Clicked "Test" button
- [ ] Checked raw email headers
- [ ] Verified sender name appears as: `"hello" <email>`

If all checkboxes are ✅ and name is still missing, add debug logging and share the output.

## Related Files

- Investigation: `SMTP_FROM_NAME_INVESTIGATION.md`
- Test results: `SESSION_SUMMARY.md`
- New tests: `internal/http/email_handler_debug_test.go`
