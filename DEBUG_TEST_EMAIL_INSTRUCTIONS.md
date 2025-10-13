# 🔍 Debug Instructions: Test Email Missing From Name

## What We Just Confirmed ✅

The **raw SMTP output test proves** that go-mail library works perfectly:

```
✅ WITH name 'hello':    From: "hello" <test@notifuse.com>
❌ WITHOUT name (empty): From: <test@notifuse.com>
```

**Test file:** `internal/service/smtp_service_raw_output_test.go`

## The Problem

If you're seeing `From: <email>` without the name, it means **`FromName` is an empty string** when it reaches the SMTP service.

## Debug Logging Added

I've added debug logging to **track exactly what value** is being used:

### 1. In `email_service.go` (TestEmailProvider method)
**Lines 113-118:** Logs the sender being used:
```go
s.logger.WithFields(map[string]interface{}{
    "sender_id":    defaultSender.ID,
    "sender_email": defaultSender.Email,
    "sender_name":  defaultSender.Name,  // ← CHECK THIS VALUE
    "is_default":   defaultSender.IsDefault,
}).Info("🔍 DEBUG: TestEmailProvider using sender")
```

**Lines 152-158:** Logs the request being sent:
```go
s.logger.WithFields(map[string]interface{}{
    "from_address": request.FromAddress,
    "from_name":    request.FromName,  // ← CHECK THIS VALUE
    ...
}).Info("🔍 DEBUG: Sending test email with SendEmailProviderRequest")
```

### 2. In `smtp_service.go` (SendEmail method)
**Lines 99-103:** Logs what's passed to `FromFormat()`:
```go
s.logger.WithFields(map[string]interface{}{
    "from_name":    request.FromName,    // ← CHECK THIS VALUE
    "from_address": request.FromAddress,
    "message_id":   request.MessageID,
}).Info("🔍 DEBUG: Setting From header in SMTP service")
```

## How to Debug

### Step 1: Rebuild and Run Your Application

```bash
# Rebuild the application
make build

# Run with logging level set to INFO or DEBUG
LOG_LEVEL=info ./notifuse
```

### Step 2: Send a Test Email

1. Go to Settings → Integrations
2. Find your integration with the "hello" sender
3. Click **"Test"**
4. Enter a test email address
5. Click "Send Test Email"

### Step 3: Check the Logs

Look for the 🔍 DEBUG log messages. You should see 3 log entries:

```
🔍 DEBUG: TestEmailProvider using sender
   sender_name: "hello"         ← What does this show?

🔍 DEBUG: Sending test email with SendEmailProviderRequest  
   from_name: "hello"           ← What does this show?

🔍 DEBUG: Setting From header in SMTP service
   from_name: "hello"           ← What does this show?
```

### Step 4: Analyze the Results

**If all 3 logs show `from_name: "hello"`:**
- The data is correct all the way through
- Something else is stripping the name (unlikely given our tests)
- Share the logs with us

**If any log shows `from_name: ""`:**
- That's where the name is getting lost!
- Check which step shows empty name:
  - Step 1 empty → Data not in the provider sent from frontend
  - Step 2 empty → Issue between sender and request creation
  - Step 3 empty → Issue in SendEmail() method

## Most Likely Scenarios

### Scenario A: Frontend Sends Empty Name
**If Step 1 logs show `sender_name: ""`**

The provider object from frontend doesn't have the name. This means:
1. Integration wasn't saved after editing sender
2. Page wasn't refreshed after saving  
3. Database doesn't have the name

**Solution:** 
```sql
-- Check database
SELECT id, name, jsonb_pretty(integrations)
FROM workspaces 
WHERE id = 'YOUR_WORKSPACE_ID';
```

Look for your sender and verify `"name": "hello"` exists.

### Scenario B: Browser DevTools Check

Open browser DevTools (F12) → Network tab:

1. Click "Test" on your integration
2. Find the POST request to `/api/email.testProvider`
3. Click on it → Payload tab
4. Look for:
```json
{
  "provider": {
    "senders": [
      {
        "email": "...",
        "name": "hello"    ← Is this present?
      }
    ]
  }
}
```

If `"name": ""` or missing → **Frontend is sending empty name**

## Remove Debug Logging Later

After you've identified the issue, you can remove the debug logging:

1. Remove the 3 debug log blocks added
2. Or keep them but change `.Info()` to `.Debug()` so they only show with DEBUG log level

## Next Steps

1. **Run your app with debug logging**
2. **Send a test email**  
3. **Check the logs** for the 🔍 DEBUG messages
4. **Share the output** - tell me what each log shows for `from_name` / `sender_name`

This will pinpoint EXACTLY where the name is getting lost!

## Quick Test

Before running the full app, verify the test still passes:

```bash
cd /workspace && go test -v ./internal/service -run TestGoMailRawOutput
```

Should show:
```
✅✅ SUCCESS: From header INCLUDES the name 'hello'
```

## Summary

- ✅ Go-mail library works perfectly (proven by raw output test)
- ✅ Code is correct (all paths preserve sender name)
- ❓ **Need to find: WHERE is the name empty in YOUR specific case?**
- 🔍 Debug logging will reveal this!

Run the app, send a test email, and check the logs!
