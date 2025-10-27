# Environment Variable Precedence Fix

## Critical Issue Discovered

While refactoring the configuration reload system, a critical bug was identified: the new `ReloadDatabaseSettings()` method was **blindly overwriting configuration with database values**, ignoring environment variables.

## The Rule

**Environment variables ALWAYS take precedence over database values.**

This is a fundamental principle in Notifuse:
- Admins can enforce configuration via environment variables
- Database settings are used as fallback
- UI changes cannot override env-configured values

## The Bug

### Original Implementation (WRONG)

```go
func (c *Config) ReloadDatabaseSettings() error {
    // Load from database
    systemSettings, err := loadSystemSettings(db, c.Security.SecretKey)
    
    // WRONG: Blindly overwrite with database values
    c.SMTP.Host = systemSettings.SMTPHost
    c.SMTP.Port = systemSettings.SMTPPort
    c.SMTP.FromEmail = systemSettings.SMTPFromEmail
    // ...
}
```

**Problem**: If admin set `SMTP_HOST=smtp.production.com` via environment variable, the reload would overwrite it with the database value.

## The Fix

### Corrected Implementation

```go
func (c *Config) ReloadDatabaseSettings() error {
    systemSettings, err := loadSystemSettings(db, c.Security.SecretKey)
    
    // CORRECT: Only update if NOT set via environment variable
    if c.envValues.SMTPHost == "" {
        c.SMTP.Host = systemSettings.SMTPHost
    }
    if c.envValues.SMTPPort == 0 {
        c.SMTP.Port = systemSettings.SMTPPort
    }
    if c.envValues.SMTPFromEmail == "" {
        c.SMTP.FromEmail = systemSettings.SMTPFromEmail
    }
    // ... same pattern for all configurable values
    
    // Apply defaults if still empty
    if c.SMTP.Port == 0 {
        c.SMTP.Port = 587  // Default
    }
}
```

## Configuration Precedence Hierarchy

```
┌─────────────────────────────────────┐
│   1. Environment Variables          │  ← Highest Priority
│      (Admin-enforced, immutable)    │
├─────────────────────────────────────┤
│   2. Database Values                │  ← Fallback
│      (User-configurable via UI)     │
├─────────────────────────────────────┤
│   3. Default Values                 │  ← Last Resort
│      (Hardcoded in application)     │
└─────────────────────────────────────┘
```

## Real-World Scenarios

### Scenario 1: Production Security

**Setup**:
```bash
# Admin sets SMTP credentials via secure env vars
export SMTP_HOST=smtp.production.com
export SMTP_USERNAME=prod_user
export SMTP_PASSWORD=secure_password_from_vault
```

**User Action**: Changes SMTP settings via web UI

**Expected Behavior**:
- SMTP Host: `smtp.production.com` (env var, unchanged)
- SMTP Username: `prod_user` (env var, unchanged)  
- SMTP Password: `secure_password_from_vault` (env var, unchanged)
- SMTP From Email: Updated (not set via env var)
- SMTP From Name: Updated (not set via env var)

**Result**: Security-critical settings remain protected ✅

### Scenario 2: Mixed Configuration

**Setup**:
```bash
# Admin only sets host via env var
export SMTP_HOST=smtp.production.com

# Other settings configured via setup wizard
# - From Email: setup@example.com
# - From Name: MyApp
```

**Config Reload**:
- SMTP Host: `smtp.production.com` (env var wins)
- SMTP From Email: `setup@example.com` (from database)
- SMTP From Name: `MyApp` (from database)

**Result**: Hybrid configuration works correctly ✅

### Scenario 3: Database-Only Configuration

**Setup**:
```bash
# No environment variables set
# All configuration via web UI
```

**Config Reload**:
- All values loaded from database
- Defaults applied where database is empty

**Result**: Database-driven configuration works ✅

## Impact

### Security Benefits
1. **Credential Protection**: SMTP passwords in env vars can't be changed via UI
2. **Compliance**: Meets security requirements for credential management
3. **Audit Trail**: Environment-based config is traceable in deployment pipelines

### Operational Benefits
1. **Multi-Environment**: Same code, different env vars per environment
2. **Override Control**: DevOps can enforce critical settings
3. **Disaster Recovery**: Env vars survive database restore/reset

### Developer Experience
1. **Clear Precedence**: Documented, testable behavior
2. **No Surprises**: UI changes don't break env-configured deployments
3. **Flexibility**: Choose per-setting whether to use env var or database

## Testing Strategy

### Test 1: Environment Variable Precedence

```go
// Setup
envValues.SMTPHost = "env.example.com"
database.SMTPHost = "db.example.com"

// Reload
config.ReloadDatabaseSettings()

// Assert
assert.Equal("env.example.com", config.SMTP.Host)  // Env var wins
```

### Test 2: Database Fallback

```go
// Setup
envValues.SMTPHost = ""  // Not set
database.SMTPHost = "db.example.com"

// Reload
config.ReloadDatabaseSettings()

// Assert
assert.Equal("db.example.com", config.SMTP.Host)  // Database used
```

### Test 3: Default Values

```go
// Setup
envValues.SMTPPort = 0  // Not set
database.SMTPPort = 0   // Not set

// Reload
config.ReloadDatabaseSettings()

// Assert
assert.Equal(587, config.SMTP.Port)  // Default applied
```

## Files Modified

1. **`config/config.go`**:
   - Updated `ReloadDatabaseSettings()` to check `c.envValues` before applying database values
   - Added comprehensive comments explaining precedence

2. **`config/config_reload_test.go`** (NEW):
   - `TestReloadDatabaseSettings_EnvVarPrecedence`: Verifies env var preservation
   - `TestReloadDatabaseSettings_DatabaseOnlyValues`: Verifies database fallback

## How to Use

### In Production (Environment Variables)

```bash
# docker-compose.yml or Kubernetes deployment
environment:
  - SMTP_HOST=smtp.production.com
  - SMTP_USERNAME=prod_user
  - SMTP_PASSWORD=${SMTP_PASSWORD_SECRET}  # From secret manager
  - DB_HOST=postgres.prod.local
  - SECRET_KEY=${SECRET_KEY}  # From vault
```

### In Development (Database)

```bash
# No env vars, configure via web UI
docker-compose up
# Visit http://localhost:8080/setup
# Configure SMTP settings via wizard
```

### Hybrid Approach (Recommended)

```bash
# Security-critical via env vars
environment:
  - SMTP_HOST=smtp.company.com
  - SMTP_PASSWORD=${SMTP_PASSWORD_SECRET}
  - SECRET_KEY=${SECRET_KEY}

# User-configurable via database
# - SMTP From Email (per environment)
# - SMTP From Name (branding)
# - API Endpoint (varies by deployment)
```

## Verification Checklist

When reviewing configuration changes:

- [ ] Check if env var is set before applying database value
- [ ] Apply default if both env var and database are empty
- [ ] Document which settings can be overridden
- [ ] Test with env vars set and unset
- [ ] Verify UI doesn't show "grayed out" for env-configured values
- [ ] Update documentation with env var precedence

## Documentation Updates

Updated these documents:
- ✅ `ARCHITECTURAL_IMPROVEMENT.md` - Added precedence section
- ✅ `ENV_VAR_PRECEDENCE_FIX.md` - This document
- ✅ Code comments in `config/config.go`
- ✅ Integration tests demonstrating behavior

## Conclusion

**Before Fix**: Database values could override environment variables ❌

**After Fix**: Environment variables always take precedence ✅

This fix ensures Notifuse can be deployed securely in production environments where administrators need to enforce configuration via environment variables while still allowing non-critical settings to be managed via the web UI.

**Impact**: High - Affects security, compliance, and operational deployments

**Risk**: Low - Well-tested, follows existing patterns in `config.Load()`

**Recommendation**: Include in next release with release notes highlighting the precedence behavior
