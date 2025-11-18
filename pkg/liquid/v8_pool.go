package liquid

import (
	_ "embed"
	"sync"

	v8 "rogchap.com/v8go"
)

//go:embed liquid.bundle.js
var liquidJSBundle string

// V8Pool manages a pool of V8 contexts for concurrent template rendering
type V8Pool struct {
	isolate  *v8.Isolate
	contexts chan *v8.Context
	mu       sync.Mutex
	closed   bool
}

var (
	globalPool *V8Pool
	poolOnce   sync.Once
)

// GetPool returns the singleton V8 pool, initializing it on first use
func GetPool() *V8Pool {
	poolOnce.Do(func() {
		globalPool = newV8Pool(10) // Pool size: 10
	})
	return globalPool
}

// newV8Pool creates a new V8 pool with the specified number of pre-warmed contexts
func newV8Pool(size int) *V8Pool {
	iso := v8.NewIsolate()

	pool := &V8Pool{
		isolate:  iso,
		contexts: make(chan *v8.Context, size),
	}

	// Pre-warm: create contexts and load liquidjs bundle into each
	successCount := 0
	for i := 0; i < size; i++ {
		ctx := v8.NewContext(iso)

		// Load liquidjs bundle into this context
		_, err := ctx.RunScript(liquidJSBundle, "liquid.bundle.js")
		if err != nil {
			// Fatal error - cannot initialize V8 pool without liquidjs
			panic("failed to load liquidjs bundle into V8 context: " + err.Error())
		}

		pool.contexts <- ctx
		successCount++
	}

	if successCount == 0 {
		panic("failed to initialize any V8 contexts in the pool")
	}

	return pool
}

// Acquire gets a context from the pool (blocks if none available)
func (p *V8Pool) Acquire() *v8.Context {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	// Block until a context is available
	return <-p.contexts
}

// Release returns a context to the pool
func (p *V8Pool) Release(ctx *v8.Context) {
	if ctx == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		// Pool is closed, don't return context
		return
	}

	// Return to pool (non-blocking)
	select {
	case p.contexts <- ctx:
		// Successfully returned to pool
	default:
		// Pool is full (shouldn't happen with proper usage), discard
	}
}

// Close shuts down the pool and disposes of all contexts
func (p *V8Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	// Drain and close all contexts
	close(p.contexts)
	for ctx := range p.contexts {
		// V8 contexts are garbage collected, no explicit cleanup needed
		_ = ctx
	}

	// Dispose of isolate
	p.isolate.Dispose()
	return nil
}

// Stats returns pool statistics for monitoring
type PoolStats struct {
	PoolSize  int
	Available int
	InUse     int
}

// Stats returns current pool statistics
func (p *V8Pool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	available := len(p.contexts)
	poolSize := cap(p.contexts)

	return PoolStats{
		PoolSize:  poolSize,
		Available: available,
		InUse:     poolSize - available,
	}
}
