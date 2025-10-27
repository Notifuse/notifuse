# Fix: Signin Mail Parsing Error After Setup

## Problem Description

In the self-hosted version, after completing the web setup wizard, users are redirected to the `/signin` page. When they input their root email address to sign in, they receive the following error:

```
failed to set email from address: failed to parse mail address "\"Notifuse\" <>": mail: invalid string
```

The system requires a restart of the Notifuse service for the setup information to be properly picked up by the application. After restart, signin works correctly.

## Root Cause Analysis

### Issue Location

The bug is in the mailer reinitialization logic in `internal/app/app.go`.

### Flow Breakdown

1. **Setup Completion** (`internal/service/setup_service.go:193-311`)
   - User completes setup wizard with SMTP configuration
   - Settings are saved to database including `smtp_from_email`
   - Setup service triggers `onSetupCompleted` callback (line 303)

2. **Config Reload Attempt** (`internal/app/app.go:1113-1137`)
   - `ReloadConfig()` is called to load fresh configuration from database
   - New config is successfully loaded from database (line 1117)
   - `InitMailer()` is called to reinitialize mailer with new settings (line 1127)

3. **Bug: InitMailer Early Return** (`internal/app/app.go:288-314`)
   ```go
   func (a *App) InitMailer() error {
       // Skip if mailer already set (e.g., by mock)
       if a.mailer != nil {
           return nil  // ❌ BUG: Returns without reinitializing
       }
       // ... mailer initialization code never reached
   }
   ```
   - The function has an early return if `a.mailer != nil`
   - Since mailer was initialized during app startup (before setup), it's not nil
   - Function returns immediately without creating new mailer with updated config

4. **Stale Configuration Persists**
   - Old mailer instance continues using pre-setup configuration:
     - `FromName: "Notifuse"` (default from `config/config.go:303`)
     - `FromEmail: ""` (empty, because setup wasn't completed at startup)

5. **Error on Signin** (`pkg/mailer/mailer.go:125-187`)
   - User attempts to sign in with email
   - System sends magic code via `SendMagicCode()`
   - Line 131: `msg.FromFormat(m.config.FromName, m.config.FromEmail)`
   - `go-mail` library fails to parse `"Notifuse" <>` (empty email address)
   - Error bubbles up to user

### Why Restart Works

After service restart:
- `a.mailer` starts as `nil`
- `InitMailer()` doesn't hit early return
- New mailer created with correct SMTP settings loaded from database
- Signin works correctly

## Solution

Modify `InitMailer()` to allow reinitialization when called from `ReloadConfig()`, while preserving mock behavior for tests.

### Implementation Approach

**Option 1: Remove Early Return for Production Mailers** (Recommended)
- Check if mailer is a mock type before early return
- Allow production mailers to be reinitialized
- Preserves test behavior

**Option 2: Force Reinitialization Flag**
- Add boolean parameter to `InitMailer(force bool)`
- Skip early return when `force=true`
- More explicit but requires signature change

**Option 3: Nil Assignment Before Reinit**
- Set `a.mailer = nil` in `ReloadConfig()` before calling `InitMailer()`
- Simple but less elegant

**Chosen Approach: Option 1** - Most robust and maintains backward compatibility with tests.

## Implementation Steps

### Step 1: Modify `InitMailer()` to Allow Reinitialization

**File:** `internal/app/app.go`

**Current Code** (lines 288-314):
```go
func (a *App) InitMailer() error {
	// Skip if mailer already set (e.g., by mock)
	if a.mailer != nil {
		return nil
	}
	// ... rest of function
}
```

**New Code:**
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

	if a.config.IsDevelopment() {
		// Use console mailer in development
		a.mailer = mailer.NewConsoleMailer()
		a.logger.Info("Using console mailer for development")
	} else {
		// Use SMTP mailer in production
		a.mailer = mailer.NewSMTPMailer(&mailer.Config{
			SMTPHost:     a.config.SMTP.Host,
			SMTPPort:     a.config.SMTP.Port,
			SMTPUsername: a.config.SMTP.Username,
			SMTPPassword: a.config.SMTP.Password,
			FromEmail:    a.config.SMTP.FromEmail,
			FromName:     a.config.SMTP.FromName,
			APIEndpoint:  a.config.APIEndpoint,
		})
		a.logger.Info("Using SMTP mailer for production")
	}

	return nil
}
```

### Step 2: Update Unit Tests

**File:** `internal/app/app_test.go`

Add test case to verify mailer reinitialization after config reload:

```go
func TestApp_ReloadConfig_ReinitializesMailer(t *testing.T) {
	// Create app with initial empty SMTP config
	cfg := getTestConfig()
	cfg.SMTP.FromEmail = "" // Simulate pre-setup state
	cfg.SMTP.FromName = "Notifuse"
	
	app := NewApp(cfg)
	require.NoError(t, app.Initialize())
	
	// Get initial mailer
	initialMailer := app.GetMailer()
	require.NotNil(t, initialMailer)
	
	// Simulate setup completion by updating database settings
	// (In real scenario, this would be done by setup service)
	_, err := app.GetDB().Exec(`
		INSERT INTO settings (key, value) VALUES 
		('is_installed', 'true'),
		('smtp_from_email', 'noreply@example.com'),
		('smtp_from_name', 'Test Mailer')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`)
	require.NoError(t, err)
	
	// Reload config
	ctx := context.Background()
	err = app.ReloadConfig(ctx)
	require.NoError(t, err)
	
	// Verify mailer was reinitialized with new config
	// Note: We can't directly inspect the mailer config (it's private),
	// but we can verify it's a new instance or test by sending an email
	reloadedMailer := app.GetMailer()
	require.NotNil(t, reloadedMailer)
	
	// In production, the new mailer should have updated FromEmail
	// Test by attempting to send a magic code
	err = reloadedMailer.SendMagicCode("test@example.com", "123456")
	require.NoError(t, err) // Should not error with empty FromEmail anymore
}

func TestApp_InitMailer_PreservesMocks(t *testing.T) {
	// Verify that mock mailers are NOT replaced during reinit
	cfg := getTestConfig()
	mockMailer := pkgmocks.NewMockMailer(gomock.NewController(t))
	
	app := NewApp(cfg, WithMockMailer(mockMailer))
	
	// First initialization should preserve mock
	err := app.InitMailer()
	require.NoError(t, err)
	assert.Equal(t, mockMailer, app.GetMailer())
	
	// Second initialization should still preserve mock
	err = app.InitMailer()
	require.NoError(t, err)
	assert.Equal(t, mockMailer, app.GetMailer())
}
```

### Step 3: Add Integration Test

**File:** `tests/integration/setup_wizard_test.go`

Add test to verify end-to-end flow:

```go
func TestSetupWizard_SigninAfterSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// Setup test app and database
	app, cleanup := setupTestApp(t)
	defer cleanup()
	
	// Complete setup wizard
	setupReq := map[string]interface{}{
		"root_email":           "admin@example.com",
		"api_endpoint":         "http://localhost:8080",
		"generate_paseto_keys": true,
		"smtp_host":            "smtp.example.com",
		"smtp_port":            587,
		"smtp_username":        "user@example.com",
		"smtp_password":        "password",
		"smtp_from_email":      "noreply@example.com",
		"smtp_from_name":       "Test System",
	}
	
	resp := makeRequest(t, app, "POST", "/api/setup.initialize", setupReq)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	// Attempt signin immediately after setup (without restart)
	signinReq := map[string]interface{}{
		"email": "admin@example.com",
	}
	
	resp = makeRequest(t, app, "POST", "/api/user.signin", signinReq)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	// Verify no mail parsing error occurred
	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	// Should not contain error about mail address parsing
	if errorMsg, ok := result["error"].(string); ok {
		assert.NotContains(t, errorMsg, "failed to parse mail address")
		assert.NotContains(t, errorMsg, "mail: invalid string")
	}
}
```

## Testing Strategy

### Manual Testing

1. **Setup and Signin Flow**
   - Start fresh Notifuse instance (clear database)
   - Complete setup wizard with SMTP configuration
   - Immediately attempt signin (DO NOT restart)
   - Verify magic code email is sent successfully
   - Verify no mail parsing errors

2. **Config Reload Verification**
   - Check logs for "Reinitializing mailer with updated configuration" message
   - Verify SMTP settings are correctly applied after reload

3. **Mock Preservation Test**
   - Run existing unit tests with mocked mailers
   - Verify mocks are not replaced during initialization

### Automated Testing

1. **Unit Tests** (run with `make test-app`)
   - `TestApp_ReloadConfig_ReinitializesMailer` - Verify mailer reinit after config reload
   - `TestApp_InitMailer_PreservesMocks` - Verify mocks are preserved
   - Existing `TestApp_InitMailer` tests should continue to pass

2. **Integration Tests** (run with `make test-integration`)
   - `TestSetupWizard_SigninAfterSetup` - Full end-to-end test
   - Existing `TestSetupWizardFlow` should continue to pass

3. **Regression Tests**
   - All existing mailer-related tests must pass
   - No changes to mailer interface or behavior

### Test Execution Order

```bash
# 1. Run unit tests for app layer
make test-app

# 2. Run unit tests for mailer package
make test-pkg

# 3. Run integration tests
make test-integration

# 4. Verify no regressions
make test-unit
```

## Files Modified

1. **`internal/app/app.go`**
   - Modify `InitMailer()` method (lines 288-314)
   - Add logic to detect mock vs production mailers
   - Allow reinitialization of production mailers

2. **`internal/app/app_test.go`**
   - Add `TestApp_ReloadConfig_ReinitializesMailer`
   - Add `TestApp_InitMailer_PreservesMocks`

3. **`tests/integration/setup_wizard_test.go`**
   - Add `TestSetupWizard_SigninAfterSetup` integration test

## Risks and Considerations

### Potential Issues

1. **Race Conditions**
   - Mailer could be in use when reinitialization occurs
   - Mitigation: ReloadConfig is only called during setup (single-threaded) or admin action

2. **Memory Leaks**
   - Old mailer instances could remain in memory
   - Mitigation: Go's garbage collector will clean up unreferenced mailers

3. **SMTP Connection Pooling**
   - New mailer creates new SMTP connections
   - Mitigation: SMTP mailer creates connections per-message (no persistent pool)

### Backward Compatibility

- No API changes
- No config schema changes
- Existing tests should pass
- Mock behavior preserved

## Verification Checklist

- [ ] Code changes implemented
- [ ] Unit tests added and passing
- [ ] Integration test added and passing
- [ ] Existing tests still passing
- [ ] Manual testing completed successfully
- [ ] No linter errors
- [ ] Logs reviewed for correct behavior
- [ ] Documentation updated (if needed)

## Rollback Plan

If issues arise:
1. Revert changes to `internal/app/app.go`
2. Restore original `InitMailer()` logic with early return
3. Document issue for future investigation
4. Temporary workaround: Instruct users to restart after setup

## Success Criteria

1. ✅ Users can sign in immediately after completing setup wizard (without restart)
2. ✅ Magic code emails are sent successfully with correct from address
3. ✅ No "failed to parse mail address" errors occur
4. ✅ All existing tests pass
5. ✅ Mock mailers in tests continue to work correctly
6. ✅ Config reload properly reinitializes mailer in all scenarios
