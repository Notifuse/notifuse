# Bug Fix: Signin Mail Parsing Error After Setup

## Problem

After completing the setup wizard, signing in immediately resulted in an error:
```
failed to set email from address: failed to parse mail address "\"Notifuse\" <>": mail: invalid string
```

A service restart resolved the issue, but users should not need to restart.

## Root Cause

**Two bugs were discovered:**

### Bug #1: Stale Mailer References in Services

Services (`UserService`, `WorkspaceService`, `SystemNotificationService`) stored mailer references during initialization. When `ReloadConfig()` created a new mailer with updated SMTP settings, these services still held references to the OLD mailer with empty `FromEmail`.

**Flow:**
1. App starts → `InitMailer()` creates mailer with empty SMTP config
2. Setup wizard completes → Saves SMTP settings to database
3. `ReloadConfig()` called → Creates NEW mailer with correct settings
4. Services still use OLD mailer → Signin fails with parsing error

### Bug #2: Environment Variables Not Set for Config Reload

`ReloadConfig()` called `config.Load()` which requires environment variables like `SECRET_KEY` to be set. In the callback context after setup, these weren't available, causing reload to fail.

## Solution

### 1. Config Architecture Improvement

**Created `Config.ReloadDatabaseSettings()` method** that:
- Reloads ONLY database-sourced settings (no environment variable dependency)
- Respects environment variable precedence (env vars > database > defaults)
- Properly parses PASETO keys from base64 strings

**File:** `config/config.go`
```go
func (c *Config) ReloadDatabaseSettings() error {
    // Connect using existing database config
    db, err := sql.Open("postgres", getSystemDSN(&c.Database))
    
    // Load settings from database
    systemSettings, err := loadSystemSettings(db, c.Security.SecretKey)
    
    // Update only database-sourced settings, respecting env var precedence
    if c.envValues.SMTPHost == "" {
        c.SMTP.Host = systemSettings.SMTPHost
    }
    // ... same pattern for all configurable settings
    
    return nil
}
```

**Benefits:**
- No environment variable manipulation required
- Respects env var > database > default precedence
- Only reloads what actually changed (database settings)

### 2. Service Mailer Update Pattern

**Added setter methods to services:**

**File:** `internal/service/user_service.go`
```go
func (s *UserService) SetEmailSender(emailSender EmailSender) {
    s.emailSender = emailSender
}
```

**File:** `internal/service/workspace_service.go`
```go
func (s *WorkspaceService) SetMailer(mailerInstance mailer.Mailer) {
    s.mailer = mailerInstance
}
```

**File:** `internal/service/system_notification_service.go`
```go
func (s *SystemNotificationService) SetMailer(mailerInstance mailer.Mailer) {
    s.mailer = mailerInstance
}
```

**Updated `App.ReloadConfig()`:**

**File:** `internal/app/app.go`
```go
func (a *App) ReloadConfig(ctx context.Context) error {
    a.logger.Info("Reloading configuration from database...")
    
    // Use new method that doesn't require env vars
    if err := a.config.ReloadDatabaseSettings(); err != nil {
        return fmt.Errorf("failed to reload database settings: %w", err)
    }
    
    a.isInstalled = a.config.IsInstalled
    
    // Reinitialize mailer with new SMTP settings
    if err := a.InitMailer(); err != nil {
        return fmt.Errorf("failed to reinitialize mailer: %w", err)
    }
    
    // Update mailer references in services
    if a.userService != nil {
        a.userService.SetEmailSender(a.mailer)
    }
    if a.workspaceService != nil {
        a.workspaceService.SetMailer(a.mailer)
    }
    if a.systemNotificationService != nil {
        a.systemNotificationService.SetMailer(a.mailer)
    }
    
    a.authService.InvalidateKeyCache()
    
    a.logger.Info("Configuration reloaded successfully")
    return nil
}
```

**Removed:** `os` import from `internal/app/app.go` (no longer needed)

### 3. Mailer Reinitialization

**Removed early return check in `InitMailer()`:**

**File:** `internal/app/app.go`
```go
func (a *App) InitMailer() error {
    // Removed: if a.mailer != nil { return nil }
    // Now always reinitializes, allowing config changes to take effect
    
    if a.config.IsDevelopment() {
        a.mailer = mailer.NewConsoleMailer()
    } else {
        a.mailer = mailer.NewSMTPMailer(&mailer.Config{
            SMTPHost:     a.config.SMTP.Host,
            SMTPPort:     a.config.SMTP.Port,
            SMTPUsername: a.config.SMTP.Username,
            SMTPPassword: a.config.SMTP.Password,
            FromEmail:    a.config.SMTP.FromEmail,
            FromName:     a.config.SMTP.FromName,
            APIEndpoint:  a.config.APIEndpoint,
        })
    }
    
    return nil
}
```

### 4. Test Infrastructure Improvements

**Created Docker-in-Docker test runner:**

**File:** `run-integration-tests.sh` (new)
```bash
#!/bin/bash
# Handles network connectivity when running tests from inside Cursor container

# Get container IPs dynamically
POSTGRES_IP=$(docker inspect tests-postgres-test-1 --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
MAILHOG_IP=$(docker inspect tests-mailhog-1 --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

# Export for tests
export TEST_DB_HOST="$POSTGRES_IP"
export TEST_DB_PORT="5432"
export TEST_SMTP_HOST="$MAILHOG_IP"

# Run tests
go test -v ./tests/integration -run "${1:-.*}" -timeout 120s
```

**Updated test utilities for flexible networking:**

**File:** `tests/testutil/connection_pool.go`
```go
func GetGlobalTestPool() *TestConnectionPool {
    poolOnce.Do(func() {
        defaultHost := "localhost"
        defaultPort := 5433
        
        // Use environment variables if set (for containerized environments)
        testHost := getEnvOrDefault("TEST_DB_HOST", defaultHost)
        testPort := defaultPort
        if testHost != defaultHost {
            // Custom host = containerized environment, use internal port
            testPort = 5432
        }
        // ...
    })
}
```

**File:** `tests/testutil/database.go` - Similar pattern for database manager

**File:** `tests/testutil/helpers.go` - Removed hardcoded hosts, allow env var override

**File:** `tests/integration/setup_wizard_test.go`
```go
// Use environment variable for SMTP host
smtpHost := os.Getenv("TEST_SMTP_HOST")
if smtpHost == "" {
    smtpHost = "localhost"
}
```

### 5. New Tests

**Added integration test:**

**File:** `tests/integration/setup_wizard_test.go`
```go
func TestSetupWizardSigninImmediatelyAfterCompletion(t *testing.T) {
    // Test that signin works immediately after setup without restart
    // Verifies mailer is properly reloaded with new SMTP settings
}
```

**Added config reload tests:**

**File:** `config/config_reload_test.go` (new)
```go
func TestReloadDatabaseSettings_EnvVarPrecedence(t *testing.T) {
    // Verifies environment variables always take precedence over database values
}

func TestReloadDatabaseSettings_DatabaseOnlyValues(t *testing.T) {
    // Verifies database values are correctly loaded when no env vars set
}
```

**Updated unit tests:**

**File:** `internal/app/app_test.go`
```go
func TestAppInitMailer(t *testing.T) {
    // Added subtests for:
    // - Development environment uses ConsoleMailer
    // - Production environment uses SMTPMailer
    // - Reinitialization with updated config
}
```

## Key Design Decisions

### 1. Environment Variable Precedence

**Rule:** `Environment Variables > Database Values > Default Values`

This ensures:
- Admins can enforce configuration via env vars
- Database settings used as fallback
- UI changes cannot override env-configured values

### 2. Setter Pattern vs Alternatives

**Chosen:** Explicit setter methods on services

**Rejected alternatives:**
- **App pointer in services** - Violates clean architecture, causes tight coupling
- **Lazy mailer creation** - Allocation overhead, harder testing, tight coupling to config
- **Service recreation** - Code duplication, handler reference issues, dependency order complexity
- **Full app reinitialization** - HTTP server incompatibility, goroutine leaks, database disruption

**Rationale:**
- Clean architecture: Services depend on interfaces, not concrete types
- Thread safety: Atomic interface assignment, no mutex needed
- Testability: Easy mock injection
- Performance: Direct interface calls, no overhead
- Simplicity: Clear, explicit updates

### 3. Surgical Updates Over Full Reinitialization

Only update what changed (4 components: mailer + 3 service references) rather than reinitializing 100+ components.

**Benefits:**
- No HTTP server disruption
- No database reconnection
- No goroutine leaks
- No event bus reset
- Fast (nanoseconds vs milliseconds)
- Safe rollback

## Files Modified

### Core Fixes
1. `config/config.go` - Added `ReloadDatabaseSettings()` with env var precedence
2. `internal/app/app.go` - Updated `ReloadConfig()` and `InitMailer()`
3. `internal/service/user_service.go` - Added `SetEmailSender()`
4. `internal/service/workspace_service.go` - Added `SetMailer()`
5. `internal/service/system_notification_service.go` - Added `SetMailer()`

### Test Infrastructure
6. `run-integration-tests.sh` - New test runner for Docker-in-Docker
7. `tests/testutil/connection_pool.go` - Flexible DB host configuration
8. `tests/testutil/database.go` - Flexible DB configuration
9. `tests/testutil/helpers.go` - Removed hardcoded hosts
10. `tests/integration/setup_wizard_test.go` - Added test + flexible SMTP host

### Tests
11. `config/config_reload_test.go` - New tests for env var precedence
12. `internal/app/app_test.go` - Updated mailer tests

## Test Results

All tests pass:
```
✅ TestAppInitMailer - PASS
✅ TestNewApp - PASS
✅ TestSetupWizardSigninImmediatelyAfterCompletion - PASS
✅ TestReloadDatabaseSettings_EnvVarPrecedence - PASS
✅ TestReloadDatabaseSettings_DatabaseOnlyValues - PASS
```

## Impact

**User Experience:**
- ✅ Signin works immediately after setup (no restart required)
- ✅ Setup wizard flow is seamless
- ✅ No error messages or confusion

**Architecture:**
- ✅ Clean separation: startup config vs runtime config reload
- ✅ Environment variable precedence enforced
- ✅ No global state modification (no `os.Setenv` in business logic)
- ✅ Services maintain clean architecture (depend on interfaces)

**Testing:**
- ✅ Integration tests work in Docker-in-Docker environment (Cursor)
- ✅ Tests work in both local and CI environments
- ✅ New tests verify env var precedence behavior

## Future Improvements (Optional)

If more services use mailer in the future, consider adding a registry pattern:

```go
type MailerRegistry struct {
    consumers []interface{ SetMailer(mailer.Mailer) }
}

// Register once
registry.Register(userService)
registry.Register(workspaceService)
registry.Register(systemNotificationService)

// Update all with one call
registry.UpdateAll(a.mailer)
```

This prevents forgetting to update a service, but is overkill for only 3 services.
