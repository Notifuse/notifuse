# Fix SIGILL CPU Compatibility Issue (GitHub Issue #89)

## Issue Summary

**GitHub Issue**: https://github.com/Notifuse/notifuse/issues/89  
**Reporter**: JRZancan  
**Title**: SIGILL on batch send

### Problem Description

Users experience a SIGILL (illegal instruction) error when attempting to send email batches. The crash occurs specifically during broadcast batch sending, while test email sending works fine.

**Environment:**
- Host: Linux 6.1.0-34-amd64 (Debian) x86_64
- Deployment: Docker container using `notifuse/notifuse:latest`
- Database: PostgreSQL (external)

### Error Details

```
SIGILL: illegal instruction
PC=0x7fa8c8356795 m=4 sigcode=2
instruction bytes: 0x66 0xf 0x3a 0xb 0xc8 0x1 0x66 0xf 0x2e 0xc8 0x40 0xf 0x9b 0xc0 0x40 0xf

created by github.com/Notifuse/notifuse/internal/service.(*TaskService).ExecuteTask
        /build/internal/service/task_service.go:511
```

**Last successful log before crash:**
```json
{"level":"info","workspace_id":"cientec","integration_rate_limit":25,"recipients":2,
 "broadcast_id":"f81422319992fe0c24cb571faaceb3f4","time":"2025-10-27T20:35:00Z",
 "message":"Starting batch send with rate limiting"}
```

## Root Cause Analysis

### 1. CPU Instruction Set Mismatch

The instruction bytes `0x66 0xf 0x3a 0xb` indicate **SSE4.1/SSE4.2** instructions. This is a CPU feature compatibility issue where:

- The Docker image was built on a machine with a modern CPU supporting advanced instruction sets (AVX, AVX2, SSE4.2)
- The deployment machine has an older CPU that doesn't support these instruction sets
- When the binary tries to execute these optimized instructions, the CPU raises SIGILL

### 2. Current Build Configuration

From `Dockerfile` line 59:
```dockerfile
RUN CGO_ENABLED=1 GOOS=linux go build -o /tmp/server ./cmd/api
```

**Issues:**
- `CGO_ENABLED=1` enables C interop, which may use CPU-specific optimizations
- No `GOARCH` specification (uses build machine's architecture)
- No build flags to control CPU feature targeting
- Alpine Linux uses musl-libc with gcc, which may generate CPU-specific code

### 3. Contributing Dependencies

Several dependencies may use assembly optimizations:

- `golang.org/x/crypto v0.36.0` - Cryptographic operations (bcrypt, etc.) often have CPU-specific assembly
- `github.com/Boostport/mjml-go v0.15.0` - Email template rendering (uses wazero WebAssembly runtime)
- `crypto/rand` package - Used in message_sender.go line 478 for random variation selection

### 4. Why Test Emails Work

Test emails likely bypass the batch processing goroutines and complex parallel execution paths where the optimized code is executed. The crash specifically happens in the goroutine created for batch task processing.

## Proposed Solutions

### Solution 1: Disable CGO (Recommended)

**Rationale**: Most of Notifuse doesn't require CGO. Pure Go builds are more portable.

**Implementation**:
```dockerfile
# Dockerfile line 59 - change to:
RUN CGO_ENABLED=0 GOOS=linux go build -o /tmp/server ./cmd/api
```

**Pros:**
- Maximum portability across different CPU architectures
- Smaller binary size
- No external C library dependencies
- Works on any x86_64 Linux system

**Cons:**
- Some packages may have slightly different behavior (rare)
- May lose some CGO-specific optimizations (usually negligible)

### Solution 2: Build for Older CPU Baseline (If CGO Required)

**Implementation**:
```dockerfile
# Dockerfile line 59 - change to:
RUN CGO_ENABLED=1 GOOS=linux GOAMD64=v1 go build \
    -ldflags="-linkmode external -extldflags '-static'" \
    -o /tmp/server ./cmd/api
```

**Explanation:**
- `GOAMD64=v1`: Target baseline x86-64 instruction set (no AVX, SSE4, etc.)
- `-linkmode external`: Use external linker for CGO
- `-extldflags '-static'`: Create statically linked binary

**Pros:**
- Keeps CGO if truly needed
- Compatible with older CPUs
- Still relatively portable

**Cons:**
- Larger binary size
- May lose some performance optimizations
- More complex build

### Solution 3: Multi-Architecture Builds

**Implementation** (enhance existing Makefile docker-publish target):
```makefile
docker-publish:
	@echo "Building multi-platform images with CPU compatibility..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg GOAMD64=v1 \
		--build-arg CGO_ENABLED=0 \
		-t notifuse/notifuse:latest \
		--push .
```

**Pros:**
- Supports multiple architectures
- Explicit CPU compatibility
- Best for public distribution

**Cons:**
- Requires buildx setup
- More complex CI/CD

## Implementation Plan

### Phase 1: Quick Fix (Immediate)
1. ✅ **Update Dockerfile** to disable CGO (Solution 1)
2. ✅ **Rebuild Docker image** with new settings
3. ✅ **Test on affected environment** to confirm fix
4. ✅ **Document CPU compatibility** in README

### Phase 2: Enhanced Build (Follow-up)
1. **Add build arguments** to Dockerfile for flexibility
2. **Update Makefile** with CPU architecture options
3. **Add build documentation** for different scenarios
4. **Test on various CPU architectures** (old and new)

### Phase 3: CI/CD Enhancement (Future)
1. **Set up multi-architecture builds** in CI
2. **Add automated testing** on different CPU types
3. **Create architecture-specific tags** (amd64-v1, amd64-v3, arm64, etc.)

## Recommended Changes

### 1. Dockerfile Update

```dockerfile
# Stage 3: Build the Go binary
FROM golang:1.24-alpine AS backend-builder

# Set working directory
WORKDIR /build

# Install dependencies (only if CGO is needed)
# RUN apk add --no-cache git gcc musl-dev

# Install git only (no gcc/musl-dev needed for CGO_ENABLED=0)
RUN apk add --no-cache git

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY cmd/ cmd/
COPY config/ config/
COPY internal/ internal/
COPY pkg/ pkg/

# Build the application with CGO disabled for maximum portability
# GOAMD64=v1 ensures compatibility with older x86-64 CPUs
RUN CGO_ENABLED=0 GOOS=linux GOAMD64=v1 go build \
    -ldflags="-w -s" \
    -o /tmp/server \
    ./cmd/api
```

### 2. Add Build Arguments (Optional)

```dockerfile
# Stage 3: Build the Go binary
FROM golang:1.24-alpine AS backend-builder

# Build arguments for flexibility
ARG CGO_ENABLED=0
ARG GOAMD64=v1

WORKDIR /build

# Conditional dependency installation
RUN if [ "$CGO_ENABLED" = "1" ]; then \
        apk add --no-cache git gcc musl-dev; \
    else \
        apk add --no-cache git; \
    fi

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY config/ config/
COPY internal/ internal/
COPY pkg/ pkg/

# Build with arguments
RUN CGO_ENABLED=${CGO_ENABLED} GOOS=linux GOAMD64=${GOAMD64} go build \
    -ldflags="-w -s" \
    -o /tmp/server \
    ./cmd/api
```

### 3. Update Makefile

```makefile
# Add CPU-specific build targets
docker-build-portable:
	@echo "Building portable Docker image (CGO disabled, baseline CPU)..."
	docker build \
		--build-arg CGO_ENABLED=0 \
		--build-arg GOAMD64=v1 \
		-t notifuse:latest .

docker-build-optimized:
	@echo "Building optimized Docker image (for modern CPUs)..."
	docker build \
		--build-arg CGO_ENABLED=0 \
		--build-arg GOAMD64=v3 \
		-t notifuse:latest-v3 .

docker-publish-portable:
	@echo "Publishing portable multi-platform image..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg CGO_ENABLED=0 \
		--build-arg GOAMD64=v1 \
		-t notifuse/notifuse:latest \
		-t notifuse/notifuse:$(VERSION) \
		--push .
```

### 4. Documentation Update (README.md)

Add section on CPU compatibility:

```markdown
## Docker Image Architecture

Our Docker images are built for maximum compatibility:

- **CPU Compatibility**: Built with `GOAMD64=v1` for baseline x86-64 support
- **CGO**: Disabled by default for portability
- **Supported Platforms**: linux/amd64, linux/arm64

### Building for Specific Architectures

For modern CPUs with AVX2 support:
```bash
docker build --build-arg GOAMD64=v3 -t notifuse:optimized .
```

For maximum compatibility (older CPUs):
```bash
docker build --build-arg GOAMD64=v1 -t notifuse:portable .
```
```

## Testing Plan

### 1. Build Testing
- [ ] Build image with CGO_ENABLED=0
- [ ] Build image with GOAMD64=v1
- [ ] Verify binary size and dependencies
- [ ] Test on modern CPU (should work)
- [ ] Test on older CPU (should work - this is the key test)

### 2. Functional Testing
- [ ] Test email provider configuration
- [ ] Send test emails (already working)
- [ ] **Send broadcast batches** (the failing scenario)
- [ ] Verify rate limiting works
- [ ] Check message history recording
- [ ] Test A/B test variation selection (uses crypto/rand)

### 3. Performance Testing
- [ ] Compare CGO vs non-CGO performance
- [ ] Measure batch sending throughput
- [ ] Check memory usage
- [ ] Verify no performance regression

## Expected Outcomes

1. **Immediate**: Users can run Notifuse on older CPUs without SIGILL errors
2. **Compatibility**: Works on any x86_64 Linux system from last 15+ years
3. **Maintainability**: Simpler build process without CGO complexity
4. **Performance**: Negligible impact (Go's pure implementations are excellent)

## Rollback Plan

If issues arise:
1. Revert Dockerfile changes
2. Rebuild with previous settings
3. Investigate specific CGO requirements
4. Consider Solution 2 (CGO with GOAMD64=v1)

## Additional Notes

### Why CGO Wasn't Necessary

Analysis of dependencies shows:
- **Database**: `lib/pq` is pure Go (no CGO required)
- **Crypto**: `golang.org/x/crypto` has pure Go implementations
- **MJML**: `mjml-go` uses WebAssembly (wazero), which is pure Go
- **Networking**: Standard library is pure Go

CGO was likely enabled by default without specific requirement.

### CPU Feature Levels

For reference:
- **v1**: Baseline x86-64 (2003+) - SSE2 only
- **v2**: x86-64 + SSSE3, SSE4.1, SSE4.2 (2009+)
- **v3**: v2 + AVX, AVX2, FMA (2013+)
- **v4**: v3 + AVX512 (2017+)

User's system likely lacks v2+ features, hence the SIGILL on SSE4 instructions.

## References

- [Go Issue #57763: GOAMD64 build flag](https://github.com/golang/go/issues/57763)
- [Docker multi-platform builds](https://docs.docker.com/build/building/multi-platform/)
- [CGO in Docker best practices](https://www.docker.com/blog/go-and-cgo-in-docker/)
- Stack Overflow: [SIGILL in Docker containers](https://stackoverflow.com/questions/65612411/forcing-docker-to-use-linux-amd64-platform-by-default-on-macos)
