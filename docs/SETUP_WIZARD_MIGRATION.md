# Setup Wizard Migration Plan

## Overview

Migrate from environment-variable-heavy setup to database-driven configuration with a web-based setup wizard, reducing friction for self-hosted deployments.

## Goals

- Reduce required environment variables for initial deployment
- Store sensitive configuration in encrypted database settings
- Create intuitive first-run setup wizard
- Generate PASETO keys during setup (no external tool needed)
- Maintain backward compatibility during transition

---

## 1. Database Schema Changes

### 1.1 System Settings Keys (New)

Add to `system.settings` table (already exists):

| Key                            | Type    | Description                                             |
| ------------------------------ | ------- | ------------------------------------------------------- |
| `is_installed`                 | boolean | System installation status                              |
| `root_email`                   | string  | Root admin email (migrated from env)                    |
| `api_endpoint`                 | string  | Public API URL - used for docs, webhooks, cron commands |
| `encrypted_paseto_public_key`  | string  | Base64 public key, encrypted with SECRET_KEY            |
| `encrypted_paseto_private_key` | string  | Base64 private key, encrypted with SECRET_KEY           |
| `smtp_host`                    | string  | SMTP server host (migrated from env)                    |
| `smtp_port`                    | string  | SMTP server port (migrated from env)                    |
| `smtp_username`                | string  | SMTP username (migrated from env)                       |
| `encrypted_smtp_password`      | string  | SMTP password, encrypted with SECRET_KEY                |
| `smtp_from_email`              | string  | SMTP from email address (migrated from env)             |
| `smtp_from_name`               | string  | SMTP from name (migrated from env)                      |

**Notes:**

- All values stored as TEXT in existing `settings.value` column
- Boolean stored as "true"/"false" strings
- Encryption uses `crypto.EncryptString()` with SECRET_KEY passphrase
- Encrypted fields: `encrypted_paseto_public_key`, `encrypted_paseto_private_key`, `encrypted_smtp_password`
- `api_endpoint` NOT used for internal API calls (frontend uses relative URLs), but needed for:
  - Displaying API documentation examples to users
  - Webhook URLs for external services (SES, Mailgun, etc.)
  - Cron job setup commands
  - Email templates with full URLs

---

## 2. Backend Changes

### 2.1 Config Loading (`config/config.go`)

**Current behavior:** Loads all config from env vars, fails if PASETO keys missing

**New behavior:**

```go
type Config struct {
    // ... existing fields
    IsInstalled bool  // NEW: loaded from DB if available
}

func LoadWithOptions(opts LoadOptions) (*Config, error) {
    // 1. Load database connection config (still from env - required)
    // 2. Connect to database
    // 3. Check system.settings for "is_installed"

    if is_installed == "true" {
        // Load from DB: root_email, api_endpoint, PASETO keys, SMTP settings (decrypt secrets)
        // Env vars act as OVERRIDE if present
    } else {
        // First-run: Only DB config required
        // Skip PASETO validation, SMTP validation
        // Set IsInstalled = false
    }

    // 4. SECRET_KEY resolution (CRITICAL):
    secretKey := v.GetString("SECRET_KEY")
    if secretKey == "" {
        // Fallback for backward compatibility
        secretKey = v.GetString("PASETO_PRIVATE_KEY")
    }
    if secretKey == "" {
        // REQUIRED
        return nil, fmt.Errorf("SECRET_KEY must be set")
    }
}
```

**Key changes:**

- Add `LoadSystemSettings(db *sql.DB)` helper
- Make PASETO keys optional if `is_installed=false`
- **SECRET_KEY validation:** Fail fast if both SECRET_KEY and PASETO_PRIVATE_KEY are empty
- **Backward compatibility:** Env vars take precedence over DB settings

### 2.2 Application Initialization (`cmd/api/main.go`, `internal/app/app.go`)

**Changes:**

```go
func main() {
    cfg, err := config.Load()
    // ... error handling

    if !cfg.IsInstalled {
        logger.Info("Setup wizard required - installation not complete")
    }

    // Continue normal startup
}
```

### 2.3 New API Endpoints (`internal/http/setup_handler.go`)

**Create new handler:**

```go
// GET /api/setup.status
// Returns: { "is_installed": bool }

// POST /api/setup.initialize
// Body: {
//   "root_email": "admin@example.com",
//   "api_endpoint": "https://notifuse.example.com",  // Optional, can be set later
//   "generate_paseto_keys": true,  // or provide existing keys
//   "paseto_public_key": "...",    // if generate=false
//   "paseto_private_key": "...",   // if generate=false
//   "smtp_host": "...",
//   "smtp_port": 587,
//   "smtp_username": "...",
//   "smtp_password": "...",
//   "smtp_from_email": "...",
//   "smtp_from_name": "..."
// }
// Returns: { "success": true, "token": "..." }
// Note: api_endpoint can be auto-detected from request headers (Host) if not provided
```

**Handler logic:**

1. Check `is_installed` - reject if already setup
2. Validate inputs (email formats, SMTP connection test optional)
3. Generate PASETO keys if requested (reuse `keygen_function/function.go` logic)
4. Encrypt secrets with SECRET_KEY:
   - PASETO private/public keys
   - SMTP password
5. Store all settings in `system.settings`:
   - `is_installed=true`
   - `root_email`, `api_endpoint`
   - `encrypted_paseto_*`
   - `smtp_*` settings
6. Create root user
7. Generate auth token for root user
8. Return token for immediate login

**PASETO Key Generation:**

```go
import "aidanwoods.dev/go-paseto"

func GeneratePasetoKeys() (privateKeyB64, publicKeyB64 string, err error) {
    secretKey := paseto.NewV4AsymmetricSecretKey()
    publicKey := secretKey.Public()

    return base64.StdEncoding.EncodeToString(secretKey.ExportBytes()),
           base64.StdEncoding.EncodeToString(publicKey.ExportBytes()),
           nil
}
```

### 2.4 Root Handler Update (`internal/http/root_handler.go`)

**Modify `serveConfigJS()`:**

```go
func (h *RootHandler) serveConfigJS(w http.ResponseWriter, r *http.Request) {
    // ... existing headers

    configJS := fmt.Sprintf(
        "window.API_ENDPOINT = %q;\nwindow.VERSION = %q;\nwindow.ROOT_EMAIL = %q;\nwindow.IS_INSTALLED = %t;",
        h.apiEndpoint,
        h.version,
        h.rootEmail,
        h.isInstalled,  // NEW
    )
    w.Write([]byte(configJS))
}
```

**Update struct:**

```go
type RootHandler struct {
    // ... existing fields
    isInstalled bool  // NEW
}
```

### 2.5 Setting Service (`internal/service/setting_service.go`)

**Add helper methods:**

```go
func (s *SettingService) GetSystemConfig(ctx context.Context, secretKey string) (*SystemConfig, error) {
    // Load all system settings
    // Decrypt PASETO keys
    return &SystemConfig{...}, nil
}

func (s *SettingService) SetSystemConfig(ctx context.Context, cfg *SystemConfig, secretKey string) error {
    // Encrypt PASETO keys
    // Store all settings
    return nil
}
```

---

## 3. Frontend Changes

### 3.1 Setup Wizard Route (`console/src/pages/SetupWizard.tsx`)

**New component:**

```tsx
// Route: /setup
// Accessible without authentication

function SetupWizard() {
  const [step, setStep] = useState<'welcome' | 'keys' | 'email' | 'admin' | 'complete'>('welcome')

  return (
    <div className="setup-container">
      {step === 'welcome' && <WelcomeStep />}
      {step === 'keys' && <PasetoKeysStep />}
      {step === 'email' && <EmailConfigStep />}
      {step === 'admin' && <AdminAccountStep />}
      {step === 'complete' && <CompleteStep />}
    </div>
  )
}
```

**Steps:**

1. **Welcome:** Introduction, system requirements check
2. **PASETO Keys:**
   - Option 1: "Generate new keys" (recommended)
   - Option 2: "Use existing keys" (textarea inputs)
   - Show generated keys with copy buttons
   - Warning to save keys securely
3. **Email Config:** SMTP settings form
4. **Admin Account:** Root email confirmation, password setup
5. **Complete:** Success message, login button

### 3.2 Router Update (`console/src/App.tsx`)

**Add setup route:**

```tsx
function App() {
  const isInstalled = window.IS_INSTALLED

  return (
    <Routes>
      {!isInstalled && <Route path="/setup" element={<SetupWizard />} />}

      {/* Redirect to setup if not installed */}
      {!isInstalled && <Route path="*" element={<Navigate to="/setup" replace />} />}

      {/* Existing routes */}
      <Route path="/login" element={<Login />} />
      {/* ... */}
    </Routes>
  )
}
```

### 3.3 API Client Update (`console/src/services/api/setup.ts`)

**New service:**

```typescript
export const setupApi = {
  getStatus: async () => {
    return await fetch('/api/setup/status')
  },

  initialize: async (config: SetupConfig) => {
    return await fetch('/api/setup/initialize', {
      method: 'POST',
      body: JSON.stringify(config)
    })
  }
}
```

### 3.4 Type Definitions (`console/src/types/setup.ts`)

```typescript
interface SetupConfig {
  root_email: string
  api_endpoint: string
  generate_paseto_keys: boolean
  paseto_public_key?: string
  paseto_private_key?: string
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_from_email: string
  smtp_from_name: string
}
```

---

## 4. Migration Strategy

### 4.1 Phase 1: Database Support (v8.1)

- ✅ Settings table exists
- ✅ SettingRepository exists
- Add system settings keys
- Update Config.go to check DB
- No breaking changes (env vars still work)

### 4.2 Phase 2: Setup Wizard (v8.2)

- Add `/api/setup/*` endpoints
- Build frontend wizard
- Update config.js with `IS_INSTALLED`
- Add PASETO key generation

### 4.3 Phase 3: Documentation (v8.3)

- Update installation docs
- Update env.example (mark vars as optional)
- Migration guide for existing deployments

### 4.4 Backward Compatibility

**Critical:** Existing installations must work without changes

**Rules:**

1. If `is_installed` not in DB → check env vars (legacy mode)
2. Env vars ALWAYS override DB settings
3. First-run detection: `is_installed=false` OR not present
4. SECRET_KEY fallback: use PASETO_PRIVATE_KEY if SECRET_KEY missing
5. **REQUIRED:** At least one of SECRET_KEY or PASETO_PRIVATE_KEY must be set (system fails fast if both empty)

---

## 5. Security Considerations

### 5.1 Encryption

- PASETO keys encrypted with SECRET_KEY before storage
- Use `crypto.EncryptString()` / `crypto.DecryptFromHexString()`
- SECRET_KEY never stored in database

### 5.2 Setup Endpoint Protection

- `/api/setup/initialize` only accessible if `is_installed=false`
- Rate limiting (prevent brute force during setup)
- HTTPS enforced in production

### 5.3 Key Generation Security

- Use `paseto.NewV4AsymmetricSecretKey()` (crypto-secure RNG)
- Display keys only once during setup
- Recommend saving to password manager

---

## 6. Impact Analysis

### 6.1 Files to Modify (Backend)

- `config/config.go` (~100 lines)
- `internal/http/root_handler.go` (~20 lines)
- `internal/app/app.go` (~10 lines)
- `cmd/api/main.go` (~5 lines)

### 6.2 Files to Create (Backend)

- `internal/http/setup_handler.go` (~300 lines)
- `internal/http/setup_handler_test.go` (~200 lines)
- `internal/service/setting_service.go` (if not exists, ~150 lines)

### 6.3 Files to Modify (Frontend)

- `console/src/App.tsx` (~30 lines)
- `console/vite-env.d.ts` (add IS_INSTALLED type)

### 6.4 Files to Create (Frontend)

- `console/src/pages/SetupWizard.tsx` (~400 lines)
- `console/src/services/api/setup.ts` (~50 lines)
- `console/src/types/setup.ts` (~30 lines)
- `console/src/components/setup/*.tsx` (step components, ~600 lines total)

### 6.5 Dependencies Affected

**Backend:**

- AuthService: Must handle missing PASETO keys gracefully
- Middleware: Skip auth for `/api/setup/*`
- Database init: Check settings before creating root user

**Frontend:**

- Router: Conditional routes based on `IS_INSTALLED`
- Auth flow: Handle first-login after setup
- API client: Setup endpoints don't require auth

### 6.6 Testing Requirements

- Unit tests: Config loading with/without DB settings
- Integration tests: Setup flow end-to-end
- Migration tests: Existing .env → DB migration
- Backward compat tests: Env vars still work

---

## 7. Environment Variables After Migration

### 7.1 Required (Minimal)

```bash
# Database connection (still required for first boot)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=notifuse_system

# Secret key for encryption (REQUIRED - or use PASETO_PRIVATE_KEY as fallback)
# At least one of SECRET_KEY or PASETO_PRIVATE_KEY must be set
SECRET_KEY=your-secret-key
```

**Note:** If `SECRET_KEY` is not set, system will use `PASETO_PRIVATE_KEY` as the encryption key (backward compatibility). However, **at least one must be provided** or the system will fail to start.

### 7.2 Moved to Database

- ~~ROOT_EMAIL~~ → `system.settings.root_email`
- ~~API_ENDPOINT~~ → `system.settings.api_endpoint`
- ~~PASETO_PRIVATE_KEY~~ → `system.settings.encrypted_paseto_private_key`
- ~~PASETO_PUBLIC_KEY~~ → `system.settings.encrypted_paseto_public_key`
- ~~SMTP_HOST~~ → `system.settings.smtp_host`
- ~~SMTP_PORT~~ → `system.settings.smtp_port`
- ~~SMTP_USERNAME~~ → `system.settings.smtp_username`
- ~~SMTP_PASSWORD~~ → `system.settings.encrypted_smtp_password`
- ~~SMTP_FROM_EMAIL~~ → `system.settings.smtp_from_email`
- ~~SMTP_FROM_NAME~~ → `system.settings.smtp_from_name`

### 7.3 Stays as Env Vars (Optional)

- SERVER_PORT, SERVER_HOST (deployment-specific)
- DB_PREFIX, DB_SSLMODE (deployment-specific)
- TRACING\_\* (optional observability)
- TELEMETRY (optional)

### 7.4 Override Behavior

All settings in DB can be **overridden** by environment variables for flexibility:

- Useful for Docker secrets, Kubernetes ConfigMaps
- Allows per-environment config (dev/staging/prod)
- Environment variables checked first, then DB fallback

---

## 8. Risks & Mitigations

| Risk                          | Impact             | Mitigation                             |
| ----------------------------- | ------------------ | -------------------------------------- |
| Encryption key loss           | Data unrecoverable | Document SECRET_KEY backup             |
| Migration breaks existing     | High               | Env vars override, extensive testing   |
| Setup wizard bypass           | Security issue     | One-time token, rate limiting          |
| PASETO key generation weak    | Auth compromise    | Use library's secure RNG               |
| DB connection fails on boot   | Can't load config  | Fail fast with clear error             |
| Missing SECRET_KEY on startup | App won't start    | Fail fast, clear error message in logs |

---

## 9. Rollout Checklist

### Pre-release

- [ ] Update `config/config.go` with DB loading
- [ ] Create setup handler and tests
- [ ] Build setup wizard UI
- [ ] Update documentation
- [ ] Test migration path (env → DB)
- [ ] Test backward compatibility
- [ ] Security review

### Release

- [ ] Tag as v8.1 or v9.0 (breaking?)
- [ ] Update Docker images
- [ ] Update installation guide
- [ ] Create migration guide
- [ ] Announce in release notes

### Post-release

- [ ] Monitor for config issues
- [ ] Collect feedback on wizard UX
- [ ] Update demo instance

---

## 10. Open Questions

1. **Existing Deployments:** Auto-migrate env → DB?

   - _Recommendation:_ No, keep env vars working (override)

2. **SECRET_KEY Generation:** Require user-provided or auto-generate?

   - _Recommendation:_ **REQUIRED from env vars** - Either SECRET_KEY or PASETO_PRIVATE_KEY must be set before startup
   - Setup wizard generates PASETO keys only, not SECRET_KEY
   - SECRET_KEY remains an environment variable for infrastructure security

3. **Setup Re-run:** Allow re-running wizard?

   - _Recommendation:_ No, require manual DB edit to reset `is_installed`

4. **Multi-step vs Single Form:** Wizard or single page?

   - _Recommendation:_ Multi-step for better UX

5. **API_ENDPOINT Auto-detection:** Should setup wizard auto-detect from request headers?

   - _Recommendation:_ Yes, pre-fill with `https://${req.Host}` but allow manual override
   - Useful for deployments where public URL differs from internal URL (reverse proxies)

6. **SMTP Connection Test:** Should setup wizard test SMTP connection before completing?

   - _Recommendation:_ Optional "Test Connection" button, but don't require it
   - Some SMTP servers may not be reachable during setup (firewall rules, etc.)

---

## 11. Success Metrics

- **Primary:** Setup time reduced from 15 min to < 5 min
- **Secondary:** Support requests about env vars reduced by 80%
- **User satisfaction:** Positive feedback on first-run experience

---

## 12. Future Enhancements

- Settings UI page (edit ROOT_EMAIL, API_ENDPOINT post-install)
- PASETO key rotation tool
- Backup/restore configuration
- Environment variable export (for migration to containers)
- Setup wizard theming/branding
- **Optimize frontend**: Replace `API_ENDPOINT` usage in `client.ts` with relative URLs (e.g., `/api/...`) to simplify API calls while keeping it for documentation/webhooks

---

## Timeline Estimate

| Phase              | Tasks                   | Effort      |
| ------------------ | ----------------------- | ----------- |
| Backend foundation | Config.go, handlers     | 3 days      |
| Frontend wizard    | UI components           | 4 days      |
| Testing & polish   | Tests, docs, edge cases | 3 days      |
| **Total**          |                         | **10 days** |

---

## Conclusion

This migration significantly improves the self-hosting experience while maintaining backward compatibility. The setup wizard reduces cognitive load and eliminates the need for external PASETO key generation tools.

**Key Benefits:**
✅ Simpler first-run experience  
✅ **57% fewer required environment variables** (14 → 6)  
✅ Built-in PASETO key generation  
✅ SMTP settings configurable via UI (stored in DB)  
✅ Backward compatible (env vars override DB)  
✅ Foundation for future settings management UI

**Next Steps:**

1. Review and approve this plan
2. Create implementation tickets
3. Begin Phase 1 (backend foundation)

---

## Implementation Plan & Progress Tracker

### Phase 1: Backend Foundation (Est. 3 days)

#### 1.1 Config & Database Setup

- [ ] Update `config/config.go` to add `IsInstalled` field to Config struct
- [ ] Implement `LoadSystemSettings(db *sql.DB)` helper function
- [ ] Add DB connection logic in `LoadWithOptions()` before loading settings
- [ ] Implement SECRET_KEY fallback logic (PASETO_PRIVATE_KEY)
- [ ] Add SECRET_KEY validation (fail if both empty)
- [ ] Make PASETO keys optional when `is_installed=false`
- [ ] Add SMTP config fields to Config struct
- [ ] Implement env var override logic (env vars take precedence over DB)
- [ ] Add unit tests for config loading with/without DB settings
- [ ] Add unit tests for SECRET_KEY fallback logic

#### 1.2 System Settings Service

- [ ] Create/update `internal/service/setting_service.go`
- [ ] Implement `GetSystemConfig(ctx, secretKey)` method
- [ ] Implement `SetSystemConfig(ctx, cfg, secretKey)` method
- [ ] Add `GetSetting(key)` helper for individual settings
- [ ] Add `SetSetting(key, value)` helper for individual settings
- [ ] Implement PASETO key encryption/decryption with SECRET_KEY
- [ ] Implement SMTP password encryption/decryption with SECRET_KEY
- [ ] Add validation for system settings
- [ ] Add unit tests for SettingService
- [ ] Add unit tests for encryption/decryption logic

#### 1.3 Setup Handler

- [ ] Create `internal/http/setup_handler.go`
- [ ] Implement `GET /api/setup.status` public endpoint (no auth)
- [ ] Implement `GET /api/setup.pasetoKeys` public endpoint to return newly generated PASETO keys in base64 format
- [ ] Implement `POST /api/setup.initialize` public endpoint and nothing if already installed (204 No Content)
- [ ] Add request validation (email formats, required fields)
- [ ] Implement PASETO key encryption/decryption with SECRET_KEY
- [ ] Implement API_ENDPOINT auto-detection from Host header
- [ ] Add check to reject setup if already installed
- [ ] Implement root user creation during setup
- [ ] Generate auth token for immediate login
- [ ] Add optional SMTP connection test helper
- [ ] Create `internal/http/setup_handler_test.go`
- [ ] Add unit tests for status endpoint
- [ ] Add unit tests for initialize endpoint
- [ ] Add integration tests for complete setup flow

#### 1.4 Root Handler Updates

- [ ] Update `internal/http/root_handler.go` to add `isInstalled` field
- [ ] Update `RootHandler` struct constructor
- [ ] Modify `serveConfigJS()` to include `window.IS_INSTALLED`
- [ ] Add tests for updated config.js output

#### 1.5 Application Initialization

- [ ] Update `internal/app/app.go` to check `cfg.IsInstalled`
- [ ] Add log message when setup wizard is required
- [ ] Update `cmd/api/main.go` if needed
- [ ] Test application startup with `is_installed=false`
- [ ] Test application startup with `is_installed=true`

#### 1.6 Database Init Updates

- [ ] Update `internal/database/init.go` to check `is_installed` setting
- [ ] Skip root user creation if not installed (let wizard handle it)
- [ ] Ensure settings table is created before checking `is_installed`
- [ ] Add tests for database initialization

---

### Phase 2: Frontend Setup Wizard (Est. 4 days)

#### 2.1 TypeScript Types & API Client

- [ ] Create `console/src/types/setup.ts`
- [ ] Define `SetupConfig` interface
- [ ] Define `SetupStatus` interface
- [ ] Update `console/src/vite-env.d.ts` to add `IS_INSTALLED: boolean`
- [ ] Create `console/src/services/api/setup.ts`
- [ ] Implement `getStatus()` API method
- [ ] Implement `initialize(config)` API method
- [ ] Add error handling for setup API calls

#### 2.2 Setup Wizard Components

- [ ] Create `console/src/pages/SetupWizard.tsx` main component
- [ ] Implement wizard state management (current step, form data)
- [ ] Create `console/src/components/setup/WelcomeStep.tsx`
  - [ ] Welcome message and introduction
  - [ ] System requirements checklist
  - [ ] "Get Started" button
- [ ] Create `console/src/components/setup/PasetoKeysStep.tsx`
  - [ ] Toggle: "Generate new keys" vs "Use existing keys"
  - [ ] Key generation on mount (if generate mode)
  - [ ] Display generated keys with copy buttons
  - [ ] Textarea inputs for existing keys (if manual mode)
  - [ ] Warning message about saving keys securely
  - [ ] Download keys as text file button
- [ ] Create `console/src/components/setup/EmailConfigStep.tsx`
  - [ ] SMTP host input
  - [ ] SMTP port input (with default 587)
  - [ ] SMTP username input
  - [ ] SMTP password input (masked)
  - [ ] From email input
  - [ ] From name input
  - [ ] Optional "Test Connection" button
  - [ ] Connection status indicator
- [ ] Create `console/src/components/setup/AdminAccountStep.tsx`
  - [ ] Root email input with validation
  - [ ] API endpoint input (pre-filled with auto-detected)
  - [ ] Confirmation checkbox
- [ ] Create `console/src/components/setup/CompleteStep.tsx`
  - [ ] Success message
  - [ ] Summary of configuration
  - [ ] "Go to Dashboard" button (auto-login with token)

#### 2.3 Router & Navigation

- [ ] Update `console/src/App.tsx` to check `window.IS_INSTALLED`
- [ ] Add `/setup` route (only if not installed)
- [ ] Add redirect to `/setup` for all routes if not installed
- [ ] Ensure setup route is accessible without authentication
- [ ] Test routing logic in different states

#### 2.4 Styling & UX

- [ ] Design setup wizard layout (centered, card-based)
- [ ] Add step progress indicator
- [ ] Style form inputs consistently
- [ ] Add loading states for API calls
- [ ] Add error states and messages
- [ ] Add validation feedback (inline errors)
- [ ] Ensure mobile responsiveness
- [ ] Add keyboard navigation (Enter to continue)

---

### Phase 3: Testing & Quality Assurance (Est. 2 days)

#### 3.1 Backend Tests

- [ ] Unit tests for config loading scenarios
- [ ] Unit tests for system settings encryption
- [ ] Unit tests for PASETO key generation
- [ ] Unit tests for setup handler validation
- [ ] Integration tests for setup flow (DB → Config)
- [ ] Integration tests for env var override behavior
- [ ] Test SECRET_KEY fallback to PASETO_PRIVATE_KEY
- [ ] Test failure when both SECRET_KEY and PASETO_PRIVATE_KEY missing
- [ ] Test backward compatibility (existing env vars still work)

#### 3.2 Frontend Tests

- [ ] Unit tests for setup wizard components
- [ ] Unit tests for form validation
- [ ] Integration tests for wizard flow
- [ ] E2E test: Complete setup from scratch
- [ ] E2E test: Setup with generated keys
- [ ] E2E test: Setup with existing keys
- [ ] E2E test: API endpoint auto-detection
- [ ] Test error handling (API failures, validation errors)

#### 3.3 Manual Testing

- [ ] Fresh installation test (no env vars except DB + SECRET_KEY)
- [ ] Test setup wizard UI/UX
- [ ] Test PASETO key generation and copy functionality
- [ ] Test SMTP connection test (optional feature)
- [ ] Test API endpoint auto-detection
- [ ] Test form validation and error messages
- [ ] Test successful completion and auto-login
- [ ] Test rejection of setup when already installed
- [ ] Test backward compatibility with existing .env files
- [ ] Test env var override of DB settings
- [ ] Test in different browsers (Chrome, Firefox, Safari)
- [ ] Test on mobile devices

#### 3.4 Security Review

- [ ] Review encryption implementation
- [ ] Review rate limiting on setup endpoints
- [ ] Review PASETO key generation security
- [ ] Review validation and sanitization
- [ ] Review authentication bypass for setup routes
- [ ] Pen testing for setup wizard vulnerabilities
- [ ] Verify HTTPS enforcement in production

---

### Phase 4: Documentation (Est. 1 day)

#### 4.1 Update Existing Documentation

- [ ] Update `README.md` with new installation instructions
- [ ] Update `env.example` with minimal required vars
- [ ] Add comments to `env.example` explaining optional vars
- [ ] Update installation guide to mention setup wizard
- [ ] Update deployment guides (Docker, Kubernetes, etc.)

#### 4.2 Create New Documentation

- [ ] Write setup wizard user guide
- [ ] Create screenshots/GIFs of setup wizard
- [ ] Document environment variable override behavior
- [ ] Document SECRET_KEY requirement and fallback
- [ ] Create migration guide for existing deployments
- [ ] Document how to reset setup (DB edit)
- [ ] Document backup/restore best practices for SECRET_KEY
- [ ] Add troubleshooting section for common setup issues

#### 4.3 API Documentation

- [ ] Document `/api/setup.status` endpoint
- [ ] Document `/api/setup.initialize` endpoint
- [ ] Add request/response examples
- [ ] Update OpenAPI spec if applicable

---

### Phase 5: Deployment & Release (Est. 0.5 days)

#### 5.1 Pre-release Preparation

- [ ] Run full test suite (unit + integration + E2E)
- [ ] Fix all linter warnings
- [ ] Update CHANGELOG.md
- [ ] Update version number (decide: v8.1 or v9.0)
- [ ] Create release notes highlighting breaking changes (if any)
- [ ] Prepare migration guide for existing users

#### 5.2 Docker & Container Updates

- [ ] Update Dockerfile if needed
- [ ] Update docker-compose.yml example
- [ ] Test Docker image build
- [ ] Test Docker container startup with setup wizard
- [ ] Push updated images to registry
- [ ] Update Kubernetes manifests if applicable

#### 5.3 Release

- [ ] Create Git tag
- [ ] Create GitHub release
- [ ] Publish release notes
- [ ] Update documentation site
- [ ] Update demo instance
- [ ] Announce release in community channels

#### 5.4 Post-release Monitoring

- [ ] Monitor error logs for config issues
- [ ] Monitor setup wizard completion rates
- [ ] Collect user feedback
- [ ] Track support requests related to setup
- [ ] Address critical bugs immediately
- [ ] Plan follow-up improvements

---

### Optional Enhancements (Future Iterations)

#### Settings Management UI

- [ ] Create settings page for editing system config post-install
- [ ] Allow editing ROOT_EMAIL, API_ENDPOINT via UI
- [ ] Allow editing SMTP settings via UI
- [ ] Add PASETO key rotation tool
- [ ] Add backup/export configuration feature

#### Advanced Features

- [ ] Multi-language support for setup wizard
- [ ] Setup wizard theming/branding options
- [ ] Email preview/test send during SMTP setup
- [ ] Health check dashboard for system settings
- [ ] Automated SECRET_KEY generation option
- [ ] Import/export configuration JSON

---

### Progress Summary

**Total Tasks:** 150+  
**Completed:** 0  
**In Progress:** 0  
**Blocked:** 0

**Phase Progress:**

- [ ] Phase 1: Backend Foundation (0/50 tasks)
- [ ] Phase 2: Frontend Setup Wizard (0/35 tasks)
- [ ] Phase 3: Testing & QA (0/30 tasks)
- [ ] Phase 4: Documentation (0/15 tasks)
- [ ] Phase 5: Deployment & Release (0/15 tasks)

**Last Updated:** [DATE]  
**Target Completion:** [DATE]  
**Current Status:** Planning
