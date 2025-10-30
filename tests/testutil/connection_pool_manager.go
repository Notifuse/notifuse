package testutil

import (
	"fmt"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/config"
)

// TestConnectionPoolManager manages multiple isolated connection pools for tests
// Each test can get its own isolated pool to prevent connection leaks between tests
type TestConnectionPoolManager struct {
	pools map[string]*TestConnectionPool
	mutex sync.RWMutex
}

// NewTestConnectionPoolManager creates a new connection pool manager for tests
func NewTestConnectionPoolManager() *TestConnectionPoolManager {
	return &TestConnectionPoolManager{
		pools: make(map[string]*TestConnectionPool),
	}
}

// GetOrCreatePool gets or creates an isolated connection pool for a specific test
func (m *TestConnectionPoolManager) GetOrCreatePool(testID string, config *config.DatabaseConfig) *TestConnectionPool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Return existing pool if already created for this test
	if pool, exists := m.pools[testID]; exists {
		return pool
	}

	// Create new pool for this test
	pool := NewTestConnectionPool(config)
	m.pools[testID] = pool

	return pool
}

// CleanupPool cleans up a specific test's connection pool
func (m *TestConnectionPoolManager) CleanupPool(testID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	pool, exists := m.pools[testID]
	if !exists {
		return nil // Already cleaned up
	}

	// Cleanup the pool
	err := pool.Cleanup()

	// Remove from registry even if cleanup failed
	delete(m.pools, testID)

	return err
}

// CleanupAll closes all test connection pools
func (m *TestConnectionPoolManager) CleanupAll() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errors []error

	for testID, pool := range m.pools {
		if err := pool.Cleanup(); err != nil {
			errors = append(errors, fmt.Errorf("failed to cleanup pool for test %s: %w", testID, err))
		}
		delete(m.pools, testID)
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors cleaning up pools: %v", errors)
	}

	return nil
}

// GetPoolCount returns the number of active test pools
func (m *TestConnectionPoolManager) GetPoolCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.pools)
}

// ConnectionPoolMetrics tracks connection pool usage during tests
type ConnectionPoolMetrics struct {
	TestName           string
	InitialConnections int
	PeakConnections    int
	FinalConnections   int
	LeakedConnections  int
	PoolCreations      int
	PoolDestructions   int
	Duration           time.Duration
	StartTime          time.Time
}

// NewConnectionPoolMetrics creates a new metrics tracker
func NewConnectionPoolMetrics(testName string, initialConnections int) *ConnectionPoolMetrics {
	return &ConnectionPoolMetrics{
		TestName:           testName,
		InitialConnections: initialConnections,
		PeakConnections:    initialConnections,
		FinalConnections:   0,
		LeakedConnections:  0,
		PoolCreations:      0,
		PoolDestructions:   0,
		StartTime:          time.Now(),
	}
}

// RecordPoolCreation records a pool creation
func (m *ConnectionPoolMetrics) RecordPoolCreation() {
	m.PoolCreations++
}

// RecordPoolDestruction records a pool destruction
func (m *ConnectionPoolMetrics) RecordPoolDestruction() {
	m.PoolDestructions++
}

// UpdateConnections updates connection counts
func (m *ConnectionPoolMetrics) UpdateConnections(current int) {
	if current > m.PeakConnections {
		m.PeakConnections = current
	}
}

// Finalize finalizes the metrics
func (m *ConnectionPoolMetrics) Finalize(finalConnections int) {
	m.FinalConnections = finalConnections
	m.LeakedConnections = finalConnections - m.InitialConnections
	if m.LeakedConnections < 0 {
		m.LeakedConnections = 0
	}
	m.Duration = time.Since(m.StartTime)
}

// Report logs the metrics (for testing.T)
func (m *ConnectionPoolMetrics) Report(logFunc func(format string, args ...interface{})) {
	logFunc("Connection Pool Metrics for %s:", m.TestName)
	logFunc("  Initial: %d, Peak: %d, Final: %d",
		m.InitialConnections, m.PeakConnections, m.FinalConnections)
	logFunc("  Leaked: %d, Created: %d, Destroyed: %d",
		m.LeakedConnections, m.PoolCreations, m.PoolDestructions)
	logFunc("  Duration: %v", m.Duration)

	if m.LeakedConnections > 0 {
		logFunc("WARNING: %d connections may have leaked", m.LeakedConnections)
	}
}

// HasLeaks returns true if connections were leaked
func (m *ConnectionPoolMetrics) HasLeaks() bool {
	return m.LeakedConnections > 0
}
