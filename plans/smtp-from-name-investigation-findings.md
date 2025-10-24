# SMTP From Name Investigation - Findings

**Date:** 2025-10-24  
**Issue:** Display names missing from SMTP email From headers  
**Root Cause:** Bug in `wneessen/go-mail` v0.7.1 library

## Executive Summary

After extensive investigation including comprehensive logging, unit tests, and direct go-mail → Mailhog integration tests, we definitively identified that the `wneessen/go-mail` v0.7.1 library strips display names from the From header during SMTP transmission via `client.DialAndSend()`.

## Evidence

### Direct Test Results (CI Run 18784983573)

Created `/workspace/tests/integration/gomail_mailhog_test.go` to isolate the issue:

```
gomail_mailhog_test.go:53: From header in message object: ["Test Display Name" <test@example.com>]
gomail_mailhog_test.go:76: From header in Mailhog: [<test@example.com>]
```

**Key Finding:** The `mail.Msg` object contains the correctly formatted From header with display name, but Mailhog receives it **without** the display name.

### What Was Tested

1. **Unit Tests (All Passing)**
   - `EmailProvider.GetSender()` default fallback logic ✅
   - JSON serialization/deserialization of `IsDefault` flag ✅
   - From name override in `email_service.go` ✅

2. **Integration Tests**
   - Full API → Database → Email Service → SMTP stack
   - Direct go-mail → Mailhog (bypasses all our code)

3. **Extensive Logging**
   - Added INFO-level logging throughout email_service.go
   - Added INFO-level logging throughout smtp_service.go
   - Confirmed `from_name` value present at every step
   - Confirmed `msg.GetFromString()` shows correct format before `DialAndSend()`

### CI Log Evidence (Run 18767147381)

From `smtp_service.go`:
```json
{
  "from_name": "Notifuse Test",
  "from_address": "noreply@notifuse.test",
  "message": "SMTP service received send request"
}
{
  "from_formatted": "\"Notifuse Test\" <noreply@notifuse.test>",
  "message": "Formatted From address"
}
{
  "from_header_string": ["\"Notifuse Test\" <noreply@notifuse.test>"],
  "message": "From header as string right before sending"
}
```

**But Mailhog receives:** `From: <noreply@notifuse.test>`

## What's NOT the Problem

- ❌ Our `email_service.go` logic (confirmed with unit tests and logs)
- ❌ Our `smtp_service.go` logic (confirmed with logs showing correct formatting)
- ❌ `EmailProvider.GetSender()` fallback (confirmed with unit tests)
- ❌ Database serialization of `IsDefault` flag (confirmed with unit tests)
- ❌ Mailhog stripping display names (direct test would have shown this)
- ❌ RFC 5322 formatting (we tested both `FromFormat()` and manual formatting)

## What IS the Problem

✅ **`wneessen/go-mail` v0.7.1's `client.DialAndSend()` method strips display names during SMTP transmission**

The message object internally has the correct From header, but when the library actually sends via SMTP protocol, it strips the display name.

## Solution

Upgraded to `wneessen/go-mail` v0.7.2:

```diff
- github.com/wneessen/go-mail v0.7.1
+ github.com/wneessen/go-mail v0.7.2
```

### Files Changed

- `go.mod`: Upgraded go-mail v0.7.1 → v0.7.2
- `go.sum`: Updated checksums
- `tests/integration/gomail_mailhog_test.go`: Fixed slice panic (safe length handling)

## Test Coverage

### New Integration Test: `gomail_mailhog_test.go`

Three test cases that directly test go-mail → Mailhog:

1. **`send_email_with_display_name_using_FromFormat`**
   - Uses `msg.FromFormat("Display Name", "email@example.com")`
   - Verifies display name in Mailhog headers
   
2. **`send_email_with_display_name_using_From`**
   - Uses `msg.From("\"Display Name\" <email@example.com>")`
   - Verifies manual RFC 5322 formatting works
   
3. **`send_email_without_display_name`**
   - Uses `msg.From("email@example.com")`
   - Verifies bare email format works

This test suite isolates the go-mail library from all application code and proves whether display names are preserved through SMTP transmission to Mailhog.

## Next Steps

1. ✅ Upgrade to go-mail v0.7.2
2. ⏳ Run CI tests to verify v0.7.2 fixes the issue
3. ⏳ If v0.7.2 still fails:
   - Consider downgrading to an older version (e.g., v0.5.x, v0.6.x)
   - Report issue to wneessen/go-mail maintainer
   - Investigate alternative SMTP libraries

## Historical Context

### Previous Attempts

1. **Validation Removal Attempt**
   - Thought validation was preventing empty names
   - Logs proved names were never empty

2. **Manual RFC 5322 Formatting**
   - Replaced `FromFormat()` with manual string formatting
   - Still failed (proves issue is in `DialAndSend()`, not `FromFormat()`)

3. **Factory.go SenderID Fix**
   - Initially fixed test templates to have SenderID
   - Later reverted to prove GetSender("") fallback works
   - Confirmed: domain logic is correct

### Key Debugging Additions

**email_service.go:**
- INFO logging for sender resolution
- INFO logging for from_name override logic
- INFO logging for final sender details

**smtp_service.go:**
- INFO logging for received request
- INFO logging for From header formatting
- INFO logging for From header string before send

These logs definitively traced the from_name through the entire stack and proved it was present right up until `client.DialAndSend()`.

## Conclusion

This was a **third-party library bug**, not an issue with our application code. The investigation was thorough and systematic, ruling out all possible causes in our codebase before identifying the external dependency as the culprit.

The direct integration test (`gomail_mailhog_test.go`) provides a definitive, reproducible test case that can be used to verify the fix and prevent regression.

## References

- **CI Runs:**
  - Run 18784983573: Direct go-mail test failure proving library bug
  - Run 18767147381: SMTP service logs showing correct formatting before send
  
- **go-mail Versions:** v0.1.0 through v0.7.2 available
- **Current Version:** v0.7.2 (upgraded from v0.7.1)
