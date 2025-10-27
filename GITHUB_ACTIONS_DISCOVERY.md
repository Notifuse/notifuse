# Critical Discovery from GitHub Actions Integration Tests

## Date: 2025-10-27

## 🔍 What We Discovered

While the unit tests passed locally, the **integration tests on GitHub Actions revealed a second bug** that was preventing the fix from working!

### The GitHub Actions Test Output

From: https://github.com/Notifuse/notifuse/actions/runs/18846340434/job/53771288316

**Key Error Lines**:
```
{"level":"info","time":"2025-10-27T15:28:02Z","message":"Reloading configuration from database..."}
{"level":"error","error":{},"time":"2025-10-27T15:28:02Z","message":"Failed to reload configuration after setup"}
{"level":"error","email":"admin@example.com","error":"failed to set email from address: failed to parse mail address \"\\\"Notifuse\\\" <>\": mail: invalid string","time":"2025-10-27T15:28:02Z","message":"Failed to send magic code"}
```

## 🐛 The Second Bug: `ReloadConfig()` Failing

### Problem

Even though we fixed `InitMailer()` to allow reinitialization, **`ReloadConfig()` was failing**, so `InitMailer()` was never called with the updated configuration!

### Root Cause

`ReloadConfig()` calls `config.Load()`, which requires the `SECRET_KEY` environment variable:

```go
// config/config.go:378-386
secretKey := v.GetString("SECRET_KEY")
if secretKey == "" {
    secretKey = v.GetString("PASETO_PRIVATE_KEY") 
}
if secretKey == "" {
    return nil, fmt.Errorf("SECRET_KEY must be set")  // ❌ FAILS HERE!
}
```

But when `ReloadConfig()` is called from the setup wizard callback, **no environment variables are set** - the config was created programmatically!

### The Fix

Set up the necessary environment variables in `ReloadConfig()` before calling `config.Load()`:

```go
func (a *App) ReloadConfig(ctx context.Context) error {
    a.logger.Info("Reloading configuration from database...")
    
    // NEW: Set up environment variables from current config
    os.Setenv("SECRET_KEY", a.config.Security.SecretKey)
    os.Setenv("DB_HOST", a.config.Database.Host)
    os.Setenv("DB_PORT", fmt.Sprintf("%d", a.config.Database.Port))
    os.Setenv("DB_USER", a.config.Database.User)
    os.Setenv("DB_PASSWORD", a.config.Database.Password)
    os.Setenv("DB_NAME", a.config.Database.DBName)
    os.Setenv("DB_SSLMODE", a.config.Database.SSLMode)
    
    // Now config.Load() can succeed!
    newConfig, err := config.Load()
    // ...
}
```

## 📊 Why Integration Tests Caught This

| Test Type | Environment | Result |
|-----------|-------------|--------|
| **Unit Tests** | Direct function calls, mocked config | ✅ Passed (didn't test full setup flow) |
| **Integration Tests** | Full app initialization + database + HTTP | ❌ Failed (revealed ReloadConfig issue) |

### What Integration Tests Showed

1. ✅ Setup wizard completes successfully
2. ❌ Config reload fails (SECRET_KEY not in environment)
3. ❌ Mailer never reinitialized (ReloadConfig never finished)
4. ❌ Signin fails with mail parsing error (still using old mailer with empty FromEmail)

## 🎯 The Complete Fix Now Includes

### 1. Fix `InitMailer()` (Original Issue)
- **File**: `internal/app/app.go` (lines 288-313)
- **Change**: Removed early return to allow reinitialization
- **Impact**: Mailer can now be reinitialized with updated config

### 2. Fix `ReloadConfig()` (Discovered Issue)  
- **File**: `internal/app/app.go` (lines 1115-1150)
- **Change**: Set environment variables before calling `config.Load()`
- **Impact**: Config reload now succeeds, triggering mailer reinitialization

## 🔗 Flow Comparison

### Before Fixes (Both Bugs Present)

```
1. Setup Wizard Completes
   └─ Saves SMTP config to DB ✅
   
2. onSetupCompleted() Callback
   └─ Calls ReloadConfig()
      └─ Calls config.Load()
         └─ ❌ FAILS: SECRET_KEY not in environment
         └─ Returns error (logged, not propagated)
   
3. User Attempts Signin
   └─ Mailer still has FromEmail = "" (empty)
   └─ ❌ ERROR: "Notifuse" <> invalid
```

### After First Fix Only (InitMailer fixed, ReloadConfig still broken)

```
1. Setup Wizard Completes
   └─ Saves SMTP config to DB ✅
   
2. onSetupCompleted() Callback
   └─ Calls ReloadConfig()
      └─ Calls config.Load()
         └─ ❌ STILL FAILS: SECRET_KEY not in environment
         └─ InitMailer() never called!
   
3. User Attempts Signin
   └─ Mailer still has FromEmail = "" (empty)
   └─ ❌ STILL ERROR: "Notifuse" <> invalid
```

### After Both Fixes (Complete Solution)

```
1. Setup Wizard Completes
   └─ Saves SMTP config to DB ✅
   
2. onSetupCompleted() Callback
   └─ Calls ReloadConfig()
      ├─ Sets environment variables ✅
      ├─ Calls config.Load() ✅
      ├─ Loads new SMTP config from DB ✅
      └─ Calls InitMailer() ✅
         └─ Creates new mailer with FromEmail = "noreply@example.com" ✅
   
3. User Attempts Signin
   └─ Mailer has FromEmail = "noreply@example.com" ✅
   └─ ✅ SUCCESS: Email sent!
```

## 💡 Lessons Learned

### 1. Unit Tests vs Integration Tests

- **Unit tests** test individual functions in isolation → Didn't catch config loading issue
- **Integration tests** test the full flow → Revealed the chained dependency

### 2. Silent Failures

The error was logged but not propagated:

```go
if err := s.onSetupCompleted(); err != nil {
    s.logger.WithField("error", err).Error("Failed to reload configuration after setup")
    // Don't fail the request - setup was successful, just log the error
}
```

This made it harder to spot the issue in development!

### 3. Environment Variable Dependencies

Functions that call `config.Load()` implicitly depend on environment variables being set. This should be:
- Documented clearly
- Made explicit in the function signature, OR  
- Handled internally (as we did in the fix)

## ✅ Current Status

### Fixed
- [x] `InitMailer()` early return removed
- [x] `ReloadConfig()` sets environment variables
- [x] Unit tests updated and passing
- [x] Integration test updated to reproduce bug scenario

### Next Steps
- [ ] Run integration tests on GitHub Actions to verify both fixes work
- [ ] Manual testing of setup → signin flow
- [ ] Monitor logs for successful config reload

## 🙏 Thank You GitHub Actions!

Without the CI/CD integration tests, we would have missed the `ReloadConfig()` failure and thought the issue was fixed when it wasn't!

This is a perfect example of why **comprehensive testing at multiple levels** is critical.
