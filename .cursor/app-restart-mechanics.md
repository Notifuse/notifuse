# How App Restart Works: The Full Picture

**Your Question:** "After calling app.Shutdown(), is it possible to start the app? Otherwise, how will the app restart?"

**Short Answer:** No, the app doesn't restart itself. The **process supervisor** restarts it externally.

---

## 🔄 The Complete Restart Flow

### What Actually Happens

```
1. Setup handler calls app.Shutdown()
   ↓
2. App gracefully shuts down (closes connections, saves state)
   ↓
3. main() function returns
   ↓
4. Go process EXITS (os.Exit(0))
   ↓
5. Process supervisor detects exit
   ↓
6. Supervisor restarts the process
   ↓
7. main() runs again
   ↓
8. Config loads fresh from database
   ↓
9. App starts with new config ✅
```

**Key Insight:** The restart happens **outside** the application, not from within.

---

## 📝 The Code Flow

### 1. **Setup Handler Triggers Shutdown**

```go
// internal/http/root_handler.go
func (h *RootHandler) CompleteSetup(w http.ResponseWriter, r *http.Request) {
    // Save config to database
    h.setupService.CompleteSetup(ctx, req)
    
    // Send success response
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "success",
        "message": "Setup completed. Server restarting...",
    })
    
    // Trigger shutdown (in background to let response complete)
    go func() {
        time.Sleep(500 * time.Millisecond)
        h.app.Shutdown(context.Background()) // ← This starts the shutdown
    }()
}
```

### 2. **App Shutdown Process**

```go
// internal/app/app.go (simplified from your actual code)
func (a *App) Shutdown(ctx context.Context) error {
    a.logger.Info("Starting graceful shutdown...")
    
    // 1. Signal shutdown to middleware (reject new requests)
    a.shutdownCancel()
    
    // 2. Stop accepting new connections
    if a.server != nil {
        a.server.Shutdown(ctx)
    }
    
    // 3. Wait for active requests to finish (with timeout)
    a.requestWg.Wait()
    
    // 4. Clean up resources
    a.db.Close()
    
    a.logger.Info("Shutdown complete")
    return nil  // ← This returns to main()
}
```

### 3. **Main Function Returns (Process Exits)**

```go
// cmd/api/main.go (your actual code)
func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }
    
    appLogger := logger.NewLoggerWithLevel(cfg.LogLevel)
    
    // Run the server
    if err := runServer(cfg, appLogger); err != nil {
        osExit(1)  // ← Process exits with error code
    }
    // ← Or exits with code 0 if no error
}

func runServer(cfg *config.Config, appLogger logger.Logger) error {
    appInstance := app.NewApp(cfg)
    appInstance.Initialize()
    
    // ... setup signal handlers ...
    
    // Wait for shutdown
    select {
    case sig := <-shutdown:
        ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
        defer cancel()
        
        return appInstance.Shutdown(ctx)  // ← Returns to main(), which exits
    }
}
```

**When `Shutdown()` returns → `runServer()` returns → `main()` returns → Process exits**

---

## 🔄 Who Restarts the Process?

### Your Current Setup: Docker Compose

**In your `docker-compose.yml` (line 45):**

```yaml
services:
  api:
    # ... other config ...
    restart: unless-stopped  # ← This is the magic line!
```

**What `restart: unless-stopped` does:**

```
Process exits → Docker detects exit → Docker runs container again
```

**Restart Policies:**
- `no` - Never restart (default)
- `always` - Always restart on exit
- `on-failure` - Restart only if exit code != 0
- `unless-stopped` - Restart unless explicitly stopped by `docker stop`

**You're using `unless-stopped`** which is perfect for this use case.

---

## 🎯 Complete Example: Setup Wizard Flow

### Step-by-Step with Timing

```
User clicks "Complete Setup"
    ↓
    HTTP POST /setup/complete
    ↓
    Handler saves config to database (200ms)
    ↓
    Handler sends HTTP 200 response (50ms)
    ↓
    Handler starts background goroutine
        ↓
        Sleep 500ms (let response reach client)
        ↓
        Call app.Shutdown()
            ↓
            Reject new requests (immediate)
            ↓
            Wait for active requests (0-1s, none during setup)
            ↓
            Close database connections (100ms)
            ↓
            Log "Shutdown complete" (10ms)
            ↓
            Return to main() (immediate)
    ↓
    main() exits
    ↓
    Go process terminates (exit code 0)
    ↓
    Docker detects container stopped
    ↓
    Docker starts container again (500ms)
    ↓
    Container runs /app/server
    ↓
    main() starts fresh
    ↓
    config.Load() reads from database (200ms)
        ↓ New config has SMTP settings! ✅
    ↓
    app.Initialize() creates services with new config (300ms)
    ↓
    app.Start() starts HTTP server (100ms)
    ↓
    Server ready with fresh config! 🎉

Total time: ~2-3 seconds
```

---

## 🐳 Docker Restart Behavior

### Testing the Restart

```bash
# Start your app
docker-compose up -d

# Watch logs
docker-compose logs -f api

# Trigger shutdown from inside container
docker-compose exec api kill -TERM 1

# You'll see:
# 1. "Shutdown signal received"
# 2. "Server shut down gracefully"  
# 3. Container exits
# 4. Docker restarts it
# 5. "Starting API server..." (fresh start)
```

### Docker Restart Statistics

```bash
# Check restart count
docker inspect notifuse-api-1 | grep RestartCount

# Output:
# "RestartCount": 1  ← After setup wizard
```

---

## 🚀 Other Process Supervisors

Docker isn't the only way. Here are alternatives:

### 1. **Systemd** (Linux servers)

```ini
# /etc/systemd/system/notifuse.service
[Unit]
Description=Notifuse Email Platform
After=network.target postgresql.service

[Service]
Type=simple
User=notifuse
WorkingDirectory=/opt/notifuse
ExecStart=/opt/notifuse/server
Restart=always           # ← Auto-restart
RestartSec=5s           # ← Wait 5s before restart
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Usage:**
```bash
sudo systemctl start notifuse
sudo systemctl status notifuse

# After setup triggers shutdown:
# systemd automatically restarts it
```

### 2. **Kubernetes**

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notifuse
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: notifuse
        image: notifuse:latest
        restartPolicy: Always  # ← Auto-restart
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
```

**What happens:**
```
Pod exits → Kubernetes sees pod is down
         → Kubernetes starts new pod with fresh config
         → Zero-downtime if replicas > 1
```

### 3. **PM2** (Process Manager)

```javascript
// ecosystem.config.js
module.exports = {
  apps: [{
    name: 'notifuse',
    script: './server',
    instances: 1,
    autorestart: true,     // ← Auto-restart
    watch: false,
    max_restarts: 10,
    min_uptime: '10s',
  }]
};
```

### 4. **supervisord**

```ini
# /etc/supervisor/conf.d/notifuse.conf
[program:notifuse]
command=/opt/notifuse/server
directory=/opt/notifuse
autostart=true
autorestart=true        ; ← Auto-restart
startretries=3
user=notifuse
redirect_stderr=true
stdout_logfile=/var/log/notifuse/output.log
```

---

## ⚠️ Important Considerations

### 1. **Exit Code Matters**

```go
// If you want supervisor to restart
func main() {
    if err := runServer(cfg, appLogger); err != nil {
        osExit(0)  // ← Exit cleanly (restart with docker: always/unless-stopped)
    }
}

// If you don't want restart (critical error)
func main() {
    if err := runServer(cfg, appLogger); err != nil {
        osExit(1)  // ← Exit with error (restart only with docker: on-failure)
    }
}
```

**For setup wizard:** Use `osExit(0)` (clean exit) so Docker restarts it

### 2. **Database Must Be Ready**

```go
// Your code already handles this (InitDB waits for connection)
func (a *App) InitDB() error {
    db, err := sql.Open(driverName, database.GetSystemDSN(&a.config.Database))
    if err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }
    // Will retry if connection fails
}
```

### 3. **Health Checks**

```yaml
# docker-compose.yml
services:
  api:
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 5s
      timeout: 3s
      retries: 3
      start_period: 10s  # ← Give app time to start
```

**Why this matters:**
- Frontend can poll `/health` to know when app is back
- Load balancers know when to route traffic
- Orchestrators know if restart succeeded

---

## 🎨 Frontend Implementation

### Handling the Restart from UI

```typescript
// console/src/services/setup.ts

interface SetupResponse {
    status: 'success';
    message: string;
}

export async function completeSetup(setupData: SetupRequest): Promise<void> {
    try {
        // 1. Submit setup (this triggers shutdown)
        const response = await api.post<SetupResponse>('/setup/complete', setupData);
        
        // 2. Show restart message to user
        message.loading({
            content: 'Setup completed! Restarting server...',
            duration: 0,  // Don't auto-dismiss
            key: 'setup-restart'
        });
        
        // 3. Wait for server to restart
        await waitForServerRestart();
        
        // 4. Show success
        message.success({
            content: 'Server ready!',
            key: 'setup-restart',
            duration: 3
        });
        
        // 5. Redirect to login
        window.location.href = '/login';
        
    } catch (error) {
        message.error('Setup failed: ' + error.message);
        throw error;
    }
}

async function waitForServerRestart(): Promise<void> {
    const maxAttempts = 30;  // 30 seconds max
    const delayMs = 1000;    // Check every second
    
    // Wait for server to start shutting down
    await sleep(2000);
    
    // Poll health endpoint
    for (let i = 0; i < maxAttempts; i++) {
        try {
            // Try to reach health endpoint
            const response = await fetch('/health', { 
                method: 'GET',
                cache: 'no-cache'
            });
            
            if (response.ok) {
                // Server is back!
                return;
            }
        } catch (error) {
            // Expected during restart - server is down
            console.log(`Waiting for server... attempt ${i + 1}/${maxAttempts}`);
        }
        
        await sleep(delayMs);
    }
    
    throw new Error('Server restart timeout - please refresh manually');
}

function sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
}
```

### User Experience

```
┌─────────────────────────────────────────┐
│  Setup Wizard                           │
│                                         │
│  [Root Email]  admin@example.com       │
│  [SMTP Host]   smtp.example.com        │
│  [SMTP Port]   587                      │
│                                         │
│  [Complete Setup]  ← User clicks       │
└─────────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────┐
│  ⏳ Setup completed!                    │
│     Restarting server...                │
│                                         │
│     (This may take a few seconds)       │
└─────────────────────────────────────────┘
                 ↓ (2-3 seconds)
┌─────────────────────────────────────────┐
│  ✅ Server ready!                       │
│     Redirecting to login...             │
└─────────────────────────────────────────┘
                 ↓
      [Login Page with new config]
```

---

## 🧪 Testing the Restart

### Local Testing

```bash
# 1. Start with docker-compose
docker-compose up -d

# 2. Watch logs
docker-compose logs -f api

# 3. Trigger shutdown (simulates setup completion)
docker-compose exec api kill -TERM 1

# 4. Observe:
# - "Shutdown signal received"
# - "Server shut down gracefully"
# - Container restarts
# - "Starting API server..." (fresh start)

# 5. Check restart count
docker inspect notifuse-api-1 --format '{{.RestartCount}}'
# Output: 1
```

### Manual Test

```bash
# Inside container, trigger programmatic shutdown
docker-compose exec api sh -c '
  curl -X POST http://localhost:8080/setup/complete \
    -H "Content-Type: application/json" \
    -d "{...setup data...}"
'

# Watch it restart
docker-compose logs -f api
```

---

## 📊 Comparison: Dynamic Reload vs Process Restart

| Aspect | Dynamic Reload | Process Restart |
|--------|---------------|-----------------|
| **How it works** | Update config in memory | Exit process, supervisor restarts |
| **Code changes needed** | Many (setters, reload logic) | Minimal (shutdown trigger) |
| **State consistency** | Manual coordination | Automatic (fresh start) |
| **Thread safety** | Must implement | Built-in (no concurrent access) |
| **Hidden state** | May persist | All cleared |
| **Downtime** | 0 seconds | 1-2 seconds |
| **Reliability** | Depends on code | Guaranteed by OS/container |
| **Testing** | Complex | Simple |
| **Industry standard** | Custom solution | Standard practice |
| **Lines of code** | ~400 | ~5 |

---

## ✅ Why Process Restart Works for Notifuse

### Your Current Infrastructure Supports It

1. **Docker Compose:**
   ```yaml
   restart: unless-stopped  ✅ Already configured
   ```

2. **Container Design:**
   - Stateless application ✅
   - Config in database ✅
   - No lost state on restart ✅

3. **Use Case:**
   - Setup happens ONCE ✅
   - No active users during setup ✅
   - 2-second delay is acceptable ✅

4. **Code Already Handles:**
   - Graceful shutdown ✅
   - Database reconnection ✅
   - Health checks ✅

**You don't need to add anything** - your infrastructure already supports automatic restart!

---

## 🎯 Implementation for Setup Wizard

### Only 3 Changes Needed

**1. Setup Handler (10 lines):**
```go
func (h *RootHandler) CompleteSetup(w http.ResponseWriter, r *http.Request) {
    h.setupService.CompleteSetup(ctx, req)
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "success",
    })
    go func() {
        time.Sleep(500 * time.Millisecond)
        h.app.Shutdown(context.Background())
    }()
}
```

**2. Frontend Polling (30 lines):**
```typescript
async function waitForServerRestart() {
    await sleep(2000);
    for (let i = 0; i < 30; i++) {
        try {
            await fetch('/health');
            return;
        } catch {
            await sleep(1000);
        }
    }
}
```

**3. User Message:**
```typescript
message.loading('Restarting server...');
await waitForServerRestart();
message.success('Ready!');
window.location.href = '/login';
```

**Total:** ~50 lines of code vs 400+ for dynamic reload

---

## 🚀 Production Considerations

### 1. **Multiple Replicas** (if scaling)

```yaml
# docker-compose.yml
services:
  api:
    deploy:
      replicas: 3  # Multiple instances
      
# Only ONE needs to restart for setup
# Others can handle traffic during restart
```

### 2. **Health Check Timeout**

```yaml
healthcheck:
  start_period: 10s  # Give app time to initialize
  interval: 5s
  timeout: 3s
  retries: 3
```

### 3. **Logging**

```go
// Log the restart reason
appLogger.WithField("reason", "setup_completed").
    Info("Graceful shutdown initiated")
```

---

## 🎓 Summary

### The Key Point

**Your app doesn't restart itself. The process supervisor restarts it.**

```
App.Shutdown() → Process exits → Docker/systemd/K8s restarts it → Fresh start with new config
```

### Why This Works

1. ✅ Docker Compose already configured with `restart: unless-stopped`
2. ✅ Your app already handles graceful shutdown
3. ✅ Config already loads from database on startup
4. ✅ No additional infrastructure needed

### What You Need to Do

1. Trigger `app.Shutdown()` after setup completes
2. Add frontend polling to wait for restart
3. Show user a "Restarting..." message

**That's it!** The restart mechanism already exists in your infrastructure.

---

**Bottom line:** Process restart is simpler AND more reliable than dynamic reload for your use case.
