# Investigation Summary: Signin Mail Parsing Error After Setup

## Date
2025-10-27

## Issue Description
After completing the web setup wizard in the self-hosted version, users are redirected to `/signin` page. When they input their root email address, they receive the following error:

```
failed to set email from address: failed to parse mail address "\"Notifuse\" <>": mail: invalid string
```

The system requires a restart of the Notifuse service for the setup information to be properly picked up. After restart, signin works correctly.

## Root Cause Identified

### Location
`internal/app/app.go` - `InitMailer()` function (lines 288-314)

### The Bug
The `InitMailer()` function has an early return that prevents reinitialization:

```go
func (a *App) InitMailer() error {
    // Skip if mailer already set (e.g., by mock)
    if a.mailer != nil {
        return nil  // ❌ BUG: Returns without reinitializing
    }
    // ... mailer initialization code never reached
}
```

### Flow Analysis

1. **App Startup (Before Setup)**:
   - `InitMailer()` is called
   - No SMTP settings in database yet
   - Mailer created with defaults: `FromName: "Notifuse"`, `FromEmail: ""`
   - `a.mailer` is set to this mailer instance

2. **User Completes Setup**:
   - SMTP settings saved to database (including `smtp_from_email: "noreply@example.com"`)
   - Setup service calls `onSetupCompleted` callback
   - Callback triggers `ReloadConfig()`

3. **Config Reload**:
   - `ReloadConfig()` successfully loads new SMTP settings from database
   - `ReloadConfig()` calls `InitMailer()` to reinitialize mailer
   - **BUG**: `a.mailer != nil` so function returns immediately
   - Old mailer with empty `FromEmail` is still in use

4. **User Attempts Signin**:
   - System calls `SendMagicCode()`
   - Mailer tries `msg.FromFormat("Notifuse", "")` 
   - go-mail library fails: `"Notifuse" <>` is invalid
   - Error: `failed to parse mail address "\"Notifuse\" <>": mail: invalid string"`

5. **After Restart**:
   - `a.mailer` starts as `nil`
   - `InitMailer()` doesn't hit early return
   - New mailer created with correct SMTP settings from database
   - Everything works ✓

## Solution

Modify `InitMailer()` to allow reinitialization of production mailers while preserving mock behavior for tests.

### Recommended Fix

```go
func (a *App) InitMailer() error {
    // Skip only if mailer is a mock (for testing)
    // Allow reinitialization of production mailers (ConsoleMailer, SMTPMailer)
    if a.mailer != nil {
        // Check if it's a mock by checking if it's NOT a known production type
        _, isConsole := a.mailer.(*mailer.ConsoleMailer)
        _, isSMTP := a.mailer.(*mailer.SMTPMailer)
        if !isConsole && !isSMTP {
            // It's a mock or unknown type - preserve it
            return nil
        }
        // It's a production mailer - allow reinitialization
        a.logger.Info("Reinitializing mailer with updated configuration")
    }

    // ... rest of function remains the same
}
```

## Test Created and Updated

An integration test has been added to verify this bug:

**File**: `tests/integration/setup_wizard_test.go`  
**Function**: `TestSetupWizardSigninImmediatelyAfterCompletion`

### Important Discovery

The test initially **passed incorrectly** because the test environment was configured with valid SMTP settings from the start (`FromEmail: "test@example.com"`), not replicating the production scenario.

### Test Fixed

The test has been updated to properly reproduce the bug:

```go
cfg.Environment = "production"  // Use SMTPMailer (not ConsoleMailer)
cfg.SMTP.FromEmail = ""         // Empty email (like production before setup)
cfg.SMTP.FromName = "Notifuse"  // Default name only
```

The test now:
1. Creates uninstalled test environment **with empty SMTP config**
2. Uses **production environment** to trigger SMTPMailer
3. Completes setup wizard with SMTP configuration
4. Immediately attempts signin (NO restart)
5. Verifies no mail parsing errors

**Expected Behavior**:
- **Before Fix**: Test fails with mail parsing error ✓ (Bug confirmed)
- **After Fix**: Test passes, signin succeeds ✓ (Bug fixed)

### Why The Original Test Passed

| Scenario | FromEmail at Start | After Setup | Signin Result |
|----------|-------------------|-------------|---------------|
| **Production** | `""` (empty) | `"noreply@example.com"` | ❌ Parse error (bug!) |
| **Original Test** | `"test@example.com"` | `"noreply@example.com"` | ✓ Works (wrong email, but valid) |
| **Updated Test** | `""` (empty) | `"noreply@example.com"` | ❌ Parse error (bug reproduced!) |

## Files Analyzed

### Code Files
- `/workspace/internal/app/app.go` - Main application, `InitMailer()` and `ReloadConfig()`
- `/workspace/internal/service/setup_service.go` - Setup wizard service with `onSetupCompleted` callback
- `/workspace/internal/http/setup_handler.go` - Setup HTTP handler
- `/workspace/pkg/mailer/mailer.go` - Mailer implementation, `SendMagicCode()`
- `/workspace/config/config.go` - Configuration loading

### Test Files
- `/workspace/tests/integration/setup_wizard_test.go` - Integration tests (test added)
- `/workspace/tests/testutil/helpers.go` - Test utilities
- `/workspace/tests/testutil/client.go` - API client for tests
- `/workspace/tests/testutil/server.go` - Test server manager

## Implementation Plan

A comprehensive implementation plan has been created at:
`/workspace/plans/fix-signin-mail-parsing-error-after-setup.md`

The plan includes:
- Detailed root cause analysis
- Implementation steps with code examples
- Testing strategy (unit + integration tests)
- Verification checklist
- Rollback plan
- Risk assessment

## Next Steps

1. Implement the fix in `internal/app/app.go`
2. Add unit tests for mailer reinitialization
3. Run integration test to verify fix
4. Ensure all existing tests pass
5. Perform manual testing of setup → signin flow
6. Review and merge changes

## Impact

- **User Experience**: Eliminates confusing error and need for manual restart
- **Risk**: Low - Fix only affects production mailer reinitialization, preserves test mocks
- **Backward Compatibility**: No API or config changes required
- **Performance**: Negligible - mailer reinitialization is fast

## References

- Issue workflow: Setup wizard → Config reload → Mailer stuck with old config
- Error source: `pkg/mailer/mailer.go:131` - `msg.FromFormat()`
- Fix location: `internal/app/app.go:288-314` - `InitMailer()`
- Test verification: `tests/integration/setup_wizard_test.go` - New test added
