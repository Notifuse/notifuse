# Architectural Improvement: Config Reload Without Environment Variables

## Problem Identified

The original implementation of `ReloadConfig()` had a significant design flaw:

```go
// BAD: Original implementation
func (a *App) ReloadConfig() error {
    // Had to set OS environment variables!
    os.Setenv("SECRET_KEY", a.config.Security.SecretKey)
    os.Setenv("DB_HOST", a.config.Database.Host)
    // ... more env vars
    
    // Then reload everything from scratch
    newConfig, err := config.Load()  // ← Creates new Viper, reads env vars
    // ...
}
```

### Why This Was Wrong

1. **Conceptual Issue**: The app already has a running `Config` struct with all necessary values
2. **Side Effects**: Modifying global OS environment variables during runtime
3. **Unnecessary Work**: Re-reading environment variables that haven't changed
4. **Tight Coupling**: `config.Load()` designed for initial startup, not runtime reloads
5. **Hidden Dependencies**: Not obvious that `config.Load()` needs env vars to be set

## Solution: Partial Configuration Reload

Created a new method that reloads **only database-sourced settings** without touching environment variables:

### New Method: `Config.ReloadDatabaseSettings()`

```go
// GOOD: New implementation
// CRITICAL: Environment variables ALWAYS take precedence over database values
func (c *Config) ReloadDatabaseSettings() error {
    // 1. Use existing DB config to connect
    db, err := sql.Open("postgres", getSystemDSN(&c.Database))
    
    // 2. Load settings from database using existing secret key
    systemSettings, err := loadSystemSettings(db, c.Security.SecretKey)
    
    // 3. Update ONLY database-sourced settings, RESPECTING ENV VAR PRECEDENCE
    c.IsInstalled = systemSettings.IsInstalled
    
    // Only update if NOT set via environment variable
    if c.envValues.SMTPHost == "" {
        c.SMTP.Host = systemSettings.SMTPHost
    }
    if c.envValues.SMTPFromEmail == "" {
        c.SMTP.FromEmail = systemSettings.SMTPFromEmail
    }
    // ... apply same pattern for all settings
    
    // 4. Parse PASETO keys properly (if not from env vars)
    if c.envValues.PasetoPrivateKey == "" && c.envValues.PasetoPublicKey == "" {
        privateKey, _ := paseto.NewV4AsymmetricSecretKeyFromBytes(privateKeyBytes)
        c.Security.PasetoPrivateKey = privateKey
    }
    
    return nil
}
```

### Updated `App.ReloadConfig()`

```go
func (a *App) ReloadConfig(ctx context.Context) error {
    // Clean: Just reload database settings
    if err := a.config.ReloadDatabaseSettings(); err != nil {
        return err
    }
    
    // Update services with new config
    a.InitMailer()
    a.userService.SetEmailSender(a.mailer)
    // ...
}
```

## Benefits

### 1. **Separation of Concerns**
- `config.Load()` → Initial startup (reads env vars + database)
- `config.ReloadDatabaseSettings()` → Runtime updates (reads database only)

### 2. **No Side Effects**
- No modification of global OS environment
- Existing environment variables remain untouched
- Predictable behavior

### 3. **Better Performance**
- Only reloads what actually changed (database settings)
- No unnecessary Viper initialization
- No re-parsing of environment variables

### 4. **Clearer Intent**
```go
// Clear: This reloads from database
config.ReloadDatabaseSettings()

// vs. Confusing: Does this need env vars set?
config.Load()
```

### 5. **Type Safety**
- Properly parses PASETO keys from base64 strings
- Uses correct types throughout (`paseto.V4AsymmetricSecretKey`, not `string`)

### 6. **Environment Variable Precedence (CRITICAL)**
- **Rule**: Environment variables ALWAYS override database values
- **Why**: Production security (SMTP credentials via env vars)
- **How**: Check `c.envValues` before applying database values
- **Benefit**: Admin can enforce configuration via env vars that can't be changed via UI

## What Gets Reloaded

### Always Updated from Database
- ✅ `IsInstalled` flag (state, not configuration)

### From Database (ONLY if not set via env var)
- ⚙️ `RootEmail` (env var takes precedence)
- ⚙️ `APIEndpoint` (env var takes precedence)
- ⚙️ SMTP Host (env var takes precedence)
- ⚙️ SMTP Port (env var takes precedence)
- ⚙️ SMTP Username (env var takes precedence)
- ⚙️ SMTP Password (env var takes precedence)
- ⚙️ SMTP From Email (env var takes precedence)
- ⚙️ SMTP From Name (env var takes precedence)
- ⚙️ PASETO keys (env var takes precedence)

### Never Reloaded (Startup Only)
- 🔒 `SECRET_KEY` (encryption key)
- 🔒 Database connection settings
- 🔒 Server port and host
- 🔒 Tracing configuration
- 🔒 Log level

### Precedence Rule
```
Environment Variable > Database Value > Default Value
```

**Example Scenarios:**

1. **SMTP Host via Env Var**:
   - Env: `SMTP_HOST=smtp.production.com`
   - DB: `smtp_host=smtp.database.com`
   - **Result**: Uses `smtp.production.com` (env var wins)

2. **SMTP Host via Database Only**:
   - Env: Not set
   - DB: `smtp_host=smtp.database.com`
   - **Result**: Uses `smtp.database.com` (database used)

3. **SMTP Port Not Set Anywhere**:
   - Env: Not set
   - DB: Not set
   - **Result**: Uses `587` (default)

## Use Cases

### Setup Wizard Flow
```
1. App starts → config.Load() reads env vars + empty database
2. Setup wizard completes → Saves SMTP settings to database
3. ReloadConfig() called → ReloadDatabaseSettings() fetches new SMTP config
4. Mailer reinitialized with correct settings
5. User can signin immediately (no restart needed)
```

### Runtime Configuration Changes
```
1. Admin updates SMTP settings via API
2. Settings saved to database
3. ReloadConfig() called
4. New settings take effect immediately
5. No restart required
```

## Code Changes

### New Files
- None (method added to existing `config/config.go`)

### Modified Files
1. **`config/config.go`**
   - Added `ReloadDatabaseSettings()` method
   - Properly parses PASETO keys from base64

2. **`internal/app/app.go`**
   - Updated `ReloadConfig()` to use new method
   - Removed `os.Setenv()` calls
   - Removed `os` import (no longer needed)

### Test Results
```
✅ TestAppInitMailer - PASS
✅ TestNewApp - PASS
✅ TestSetupWizardSigninImmediatelyAfterCompletion - PASS
✅ TestReloadDatabaseSettings_EnvVarPrecedence - PASS (NEW)
✅ TestReloadDatabaseSettings_DatabaseOnlyValues - PASS (NEW)
```

### New Tests for Environment Variable Precedence

**`TestReloadDatabaseSettings_EnvVarPrecedence`**:
- Sets env vars for some SMTP settings
- Saves different values to database
- Reloads configuration
- **Verifies**: Env vars are preserved, database fills gaps

**`TestReloadDatabaseSettings_DatabaseOnlyValues`**:
- No env vars set
- Saves values to database only
- Reloads configuration
- **Verifies**: Database values are used, defaults applied when needed

## Design Principles Applied

1. **Single Responsibility**: Each method has one clear purpose
2. **Separation of Concerns**: Startup vs. runtime reload
3. **Immutability**: Environment variables not modified at runtime
4. **Explicit is Better Than Implicit**: Clear method name shows intent
5. **Don't Repeat Yourself**: Reuses existing `loadSystemSettings()` helper

## Future Improvements

### Potential Enhancements
1. **Selective Reload**: Allow reloading specific settings only
   ```go
   config.ReloadSMTPSettings()
   config.ReloadPasetoKeys()
   ```

2. **Change Detection**: Return what changed
   ```go
   changes, err := config.ReloadDatabaseSettings()
   if changes.SMTPChanged {
       app.InitMailer()
   }
   ```

3. **Validation**: Validate settings before applying
   ```go
   if err := config.ValidateSMTPSettings(); err != nil {
       return err
   }
   ```

4. **Atomic Updates**: Transaction-like config updates
   ```go
   config.BeginUpdate()
   config.ReloadDatabaseSettings()
   config.CommitUpdate() // or RollbackUpdate()
   ```

## Lessons Learned

1. **Question the Design**: Always ask "why do we need this?" when seeing `os.Setenv()` in business logic
2. **Read Before Write**: Understand what `config.Load()` does before calling it
3. **Separate Concerns**: Startup configuration ≠ Runtime configuration changes
4. **Type Safety Matters**: Use proper types (PASETO keys) instead of strings
5. **Test Drives Design**: Integration test revealed the environment variable requirement

## Conclusion

This refactoring improves code quality by:
- ✅ Removing global state modification
- ✅ Making dependencies explicit
- ✅ Improving testability
- ✅ Better separation of concerns
- ✅ Clearer intent

**Result**: Cleaner, more maintainable code that does exactly what it needs to do—no more, no less.
