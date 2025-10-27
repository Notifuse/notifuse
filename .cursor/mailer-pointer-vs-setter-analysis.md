# Mailer Passing Strategy: Pointer vs Setter Pattern

## Current Approach: Setter Pattern

### Implementation

```go
// Service stores interface directly
type UserService struct {
    emailSender EmailSender  // Interface value
    // ...
}

// When config changes, explicitly update
func (a *App) ReloadConfig() error {
    a.InitMailer()  // Creates new mailer
    
    // Explicit updates required
    a.userService.SetEmailSender(a.mailer)
    a.workspaceService.SetMailer(a.mailer)
    a.systemNotificationService.SetMailer(a.mailer)
    // ...
}

// Each service needs a setter
func (s *UserService) SetEmailSender(emailSender EmailSender) {
    s.emailSender = emailSender
}
```

## Alternative Approach: Pointer to App

### Implementation

```go
// Service stores reference to App
type UserService struct {
    app *App  // Pointer to parent
    // ...
}

// Access mailer via app
func (s *UserService) SendMagicCode(email, code string) error {
    return s.app.mailer.SendMagicCode(email, code)
    // Or: s.app.GetMailer().SendMagicCode(email, code)
}

// When config changes, no updates needed
func (a *App) ReloadConfig() error {
    a.InitMailer()  // Creates new mailer
    // That's it! Services automatically see new mailer
}
```

### Alternative: Pointer to Mailer

```go
// Service stores pointer to mailer pointer
type UserService struct {
    mailer *mailer.Mailer  // Pointer to pointer location
    // ...
}

// In App
type App struct {
    mailer mailer.Mailer  // Not a pointer
    // ...
}

// Problem: Can't take address of interface!
// This doesn't work in Go
userService := NewUserService(&a.mailer)  // ERROR
```

### Alternative: Getter Function

```go
// Service stores a getter function
type UserService struct {
    getMailer func() mailer.Mailer
    // ...
}

func NewUserService(getMailer func() mailer.Mailer) *UserService {
    return &UserService{
        getMailer: getMailer,
    }
}

func (s *UserService) SendMagicCode(email, code string) error {
    m := s.getMailer()  // Get current mailer
    return m.SendMagicCode(email, code)
}

// In App
app.userService = NewUserService(app.GetMailer)
```

## Comparative Analysis

### 1. Concurrency Safety

#### Setter Pattern ✅
```go
// Thread-safe: atomic interface assignment
func (s *UserService) SetEmailSender(emailSender EmailSender) {
    s.emailSender = emailSender  // Atomic in Go
}

// Safe concurrent reads
func (s *UserService) SendMagicCode() error {
    return s.emailSender.SendMagicCode(...)  // Safe
}
```

#### App Pointer ❌
```go
// Race condition possible
func (a *App) ReloadConfig() {
    a.mailer = newMailer  // Write
}

func (s *UserService) SendMagicCode() {
    return s.app.mailer.SendMagicCode()  // Read - RACE!
}

// Would need mutex
type App struct {
    mu     sync.RWMutex
    mailer mailer.Mailer
}

func (a *App) GetMailer() mailer.Mailer {
    a.mu.RLock()
    defer a.mu.RUnlock()
    return a.mailer
}
```

#### Getter Function ⚠️
```go
// Safe if getter uses mutex
func (a *App) GetMailer() mailer.Mailer {
    a.mu.RLock()
    defer a.mu.RUnlock()
    return a.mailer
}

// But adds overhead on every email send
func (s *UserService) SendMagicCode() {
    m := s.getMailer()  // Mutex lock/unlock
    return m.SendMagicCode()  // Then actual work
}
```

### 2. Testability

#### Setter Pattern ✅
```go
func TestUserService_SendMagicCode(t *testing.T) {
    mockMailer := mocks.NewMockMailer(t)
    mockMailer.EXPECT().SendMagicCode(...)
    
    service := NewUserService(...)
    service.SetEmailSender(mockMailer)  // Easy to inject
    
    service.SendMagicCode(...)
}
```

#### App Pointer ❌
```go
func TestUserService_SendMagicCode(t *testing.T) {
    // Must create entire App just to test one service
    mockMailer := mocks.NewMockMailer(t)
    
    app := &App{mailer: mockMailer}  // Heavyweight
    service := NewUserService(app)   // Tight coupling
    
    service.SendMagicCode(...)
}
```

#### Getter Function ✅
```go
func TestUserService_SendMagicCode(t *testing.T) {
    mockMailer := mocks.NewMockMailer(t)
    
    getMailer := func() mailer.Mailer {
        return mockMailer
    }
    
    service := NewUserService(getMailer)  // Easy to inject
    service.SendMagicCode(...)
}
```

### 3. Dependency Injection

#### Setter Pattern ✅
```go
// Clear dependencies at construction
func NewUserService(cfg UserServiceConfig) *UserService {
    return &UserService{
        repo:        cfg.Repository,      // Dependency 1
        emailSender: cfg.EmailSender,     // Dependency 2
        logger:      cfg.Logger,          // Dependency 3
    }
}

// Constructor signature documents all dependencies
```

#### App Pointer ❌
```go
// Hidden dependencies
func NewUserService(app *App) *UserService {
    return &UserService{
        app: app,  // What does service need from app?
    }
}

// Service uses: app.mailer, app.logger, app.config, etc.
// Dependencies not visible in constructor
```

#### Getter Function ⚠️
```go
// Partial dependency injection
func NewUserService(
    repo Repository,
    getMailer func() mailer.Mailer,
    logger logger.Logger,
) *UserService {
    // Better than app pointer, but awkward
}
```

### 4. Code Complexity

#### Setter Pattern ⚠️
```go
// More code: setter methods needed
func (s *UserService) SetEmailSender(e EmailSender) {
    s.emailSender = e
}

// More code: must call all setters
func (a *App) ReloadConfig() {
    a.InitMailer()
    a.userService.SetEmailSender(a.mailer)
    a.workspaceService.SetMailer(a.mailer)
    a.systemNotificationService.SetMailer(a.mailer)
}

// Risk: Forget to call setter
```

#### App Pointer ✅
```go
// Less code: no setters needed
func (a *App) ReloadConfig() {
    a.InitMailer()
    // Done! Services auto-update
}

// Simpler reload logic
```

#### Getter Function ⚠️
```go
// Moderate code: function wrappers
getMailer := func() mailer.Mailer {
    return a.GetMailer()
}

service := NewUserService(getMailer)

// Overhead on every call
```

### 5. Coupling

#### Setter Pattern ✅
```go
// Loose coupling: Service only knows about interface
type UserService struct {
    emailSender EmailSender  // Interface only
}

// Service doesn't know where mailer comes from
// Service doesn't know App exists
```

#### App Pointer ❌
```go
// Tight coupling: Service depends on App structure
type UserService struct {
    app *App  // Knows about App internals
}

// Service coupled to:
// - App structure
// - App lifecycle
// - App's other dependencies
```

#### Getter Function ✅
```go
// Loose coupling: Service doesn't know source
type UserService struct {
    getMailer func() mailer.Mailer
}

// Could come from App, could come from anywhere
```

### 6. Clean Architecture

#### Setter Pattern ✅
```
┌─────────────────┐
│  Domain Layer   │ ← EmailSender interface defined here
└────────┬────────┘
         │
┌────────▼────────┐
│ Service Layer   │ ← UserService uses interface
└────────┬────────┘
         │
┌────────▼────────┐
│   App Layer     │ ← App provides concrete mailer
└─────────────────┘

Dependencies point inward ✅
```

#### App Pointer ❌
```
┌─────────────────┐
│  Domain Layer   │
└────────┬────────┘
         │
┌────────▼────────┐
│ Service Layer   │ ───┐
└─────────────────┘    │
         ▲             │
         │             │
         └─────────────┘
      Circular dependency! ❌
      
Service depends on App
App depends on Service
```

### 7. nil Pointer Safety

#### Setter Pattern ✅
```go
// Interface can be nil-checked
if s.emailSender != nil {
    s.emailSender.SendMagicCode(...)
}

// Clear ownership
```

#### App Pointer ❌
```go
// Multiple levels of nil checking
if s.app != nil {
    if s.app.mailer != nil {
        s.app.mailer.SendMagicCode(...)
    }
}

// Unclear: Who ensures app.mailer is set?
```

## Performance Comparison

### Setter Pattern
```go
// Direct call: 1 indirection
s.emailSender.SendMagicCode()  
// ↓
// mailer.SendMagicCode()
```

**Cost**: 1 interface method call (~1-2ns overhead)

### App Pointer (No Mutex)
```go
// Indirect call: 2 indirections
s.app.mailer.SendMagicCode()
// ↓
// app → mailer
// ↓
// mailer.SendMagicCode()
```

**Cost**: 1 pointer dereference + 1 interface call (~2-3ns overhead)

### App Pointer (With Mutex)
```go
// Must lock for safety
m := s.app.GetMailer()  // RWMutex.RLock() + RUnlock()
m.SendMagicCode()
```

**Cost**: ~30-50ns for mutex operations + interface call

### Getter Function
```go
// Function call + interface call
m := s.getMailer()  // Function call overhead
m.SendMagicCode()
```

**Cost**: 1 function call + 1 interface call (~3-5ns)

**Verdict**: Setter pattern is fastest and safest

## Real-World Scenarios

### Scenario 1: Setup Wizard (Current Use Case)

**Setter Pattern** ✅
```go
// Setup completes
setupService.Initialize()

// Config reloads
app.ReloadConfig()
  ↓
app.InitMailer()  // New mailer with SMTP settings
app.userService.SetEmailSender(app.mailer)

// Signin immediately works
userService.SendMagicCode()  // Uses new mailer
```

**App Pointer** ❌
```go
// Setup completes
setupService.Initialize()

// Config reloads
app.ReloadConfig()
  ↓
app.mailer = newMailer  // RACE if user signing in!

// Signin might use old or new mailer
userService.SendMagicCode()  // s.app.mailer - which one?
```

### Scenario 2: Runtime SMTP Update

**Setter Pattern** ✅
```go
// Admin updates SMTP via API
POST /api/settings/smtp

// Handler reloads config
app.ReloadConfig()
  ↓
app.userService.SetEmailSender(newMailer)

// All subsequent emails use new settings
// In-flight emails use old mailer (correct!)
```

**App Pointer** ⚠️
```go
// Admin updates SMTP via API
POST /api/settings/smtp

// Handler reloads config
app.mailer = newMailer

// In-flight emails might fail!
// goroutine 1: sending with old mailer (half-done)
// goroutine 2: mailer swapped
// goroutine 1: continues with ??? (undefined)
```

### Scenario 3: Multiple Services

**Setter Pattern** ⚠️
```go
// Must update all services
app.userService.SetEmailSender(newMailer)
app.workspaceService.SetMailer(newMailer)
app.notificationService.SetMailer(newMailer)
app.broadcastService.SetMailer(newMailer)
// Easy to forget one!
```

**App Pointer** ✅
```go
// Automatic update
app.mailer = newMailer
// All services see new mailer immediately
```

## Hybrid Approach: Mailer Manager

```go
// Central mailer manager with thread-safety
type MailerManager struct {
    mu     sync.RWMutex
    mailer mailer.Mailer
}

func (mm *MailerManager) GetMailer() mailer.Mailer {
    mm.mu.RLock()
    defer mm.mu.RUnlock()
    return mm.mailer
}

func (mm *MailerManager) SetMailer(m mailer.Mailer) {
    mm.mu.Lock()
    defer mm.mu.Unlock()
    mm.mailer = m
}

// Services get manager
type UserService struct {
    mailerMgr *MailerManager
}

func (s *UserService) SendMagicCode() error {
    m := s.mailerMgr.GetMailer()  // Thread-safe
    return m.SendMagicCode(...)
}

// App updates once
func (a *App) ReloadConfig() {
    a.InitMailer()
    a.mailerManager.SetMailer(a.mailer)  // Single update
    // All services automatically use new mailer
}
```

### Hybrid Approach Analysis

**Pros**:
- ✅ Thread-safe (mutex protected)
- ✅ Single update point (no multiple setters)
- ✅ Automatic propagation to all services
- ✅ Better than raw app pointer

**Cons**:
- ⚠️ Mutex overhead on every email send
- ⚠️ More complex than direct interface
- ⚠️ Harder to test (need to mock manager)

## Recommendation

### For Notifuse: **Stick with Setter Pattern** ✅

**Why**:
1. **Thread Safety**: No race conditions, no mutex needed
2. **Clean Architecture**: Services don't depend on App
3. **Testability**: Easy to inject mocks
4. **Explicitness**: Clear when mailer changes
5. **Performance**: Fastest approach (direct interface call)

**Mitigation for "Forgetting to Call Setter"**:

```go
// Option 1: Compile-time enforcement via interface
type ServiceWithMailer interface {
    SetMailer(mailer.Mailer)
}

func (a *App) ReloadConfig() {
    a.InitMailer()
    
    // Iterate all services that need mailer
    services := []ServiceWithMailer{
        a.userService,
        a.workspaceService,
        a.systemNotificationService,
    }
    
    for _, svc := range services {
        svc.SetMailer(a.mailer)
    }
}
```

```go
// Option 2: Code generation (go:generate)
//go:generate go run scripts/generate_mailer_setters.go

func (a *App) UpdateAllMailers() {
    // Auto-generated code that calls all setters
    a.userService.SetEmailSender(a.mailer)
    a.workspaceService.SetMailer(a.mailer)
    // ... all services automatically included
}
```

```go
// Option 3: Registry pattern
type MailerRegistry struct {
    services []interface{ SetMailer(mailer.Mailer) }
}

func (r *MailerRegistry) Register(s interface{ SetMailer(mailer.Mailer) }) {
    r.services = append(r.services, s)
}

func (r *MailerRegistry) UpdateAll(m mailer.Mailer) {
    for _, svc := range r.services {
        svc.SetMailer(m)
    }
}

// In App initialization
registry.Register(userService)
registry.Register(workspaceService)

// In ReloadConfig
registry.UpdateAll(a.mailer)
```

## When to Consider Alternatives

### Use Getter Function If:
- You need dynamic behavior (different mailer per request)
- Mailer selection based on runtime conditions
- A/B testing different mailers

### Use App Pointer If:
- Rapid prototyping (quick and dirty)
- Single-threaded application
- Services need many App dependencies

### Use Mailer Manager If:
- Very frequent mailer changes (unlikely)
- Complex mailer lifecycle management
- Need centralized mailer metrics/logging

## Conclusion

| Criteria | Setter | App Ptr | Getter Fn | Manager |
|----------|--------|---------|-----------|---------|
| Thread Safety | ✅ | ❌ | ⚠️ | ✅ |
| Testability | ✅ | ❌ | ✅ | ⚠️ |
| Performance | ✅ | ⚠️ | ⚠️ | ⚠️ |
| Clean Arch | ✅ | ❌ | ✅ | ⚠️ |
| Simplicity | ⚠️ | ✅ | ⚠️ | ❌ |
| Coupling | ✅ | ❌ | ✅ | ⚠️ |

**Winner**: Setter Pattern ✅

**Best Practice**: Keep current approach, add registry pattern to avoid forgetting setters.
