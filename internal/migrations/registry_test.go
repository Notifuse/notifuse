package migrations

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMigration is a mock implementation of MajorMigrationInterface for testing
type mockMigration struct {
	version            float64
	hasSystemUpdate    bool
	hasWorkspaceUpdate bool
}

func (m *mockMigration) GetMajorVersion() float64 {
	return m.version
}

func (m *mockMigration) HasSystemUpdate() bool {
	return m.hasSystemUpdate
}

func (m *mockMigration) HasWorkspaceUpdate() bool {
	return m.hasWorkspaceUpdate
}

func (m *mockMigration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	return nil
}

func (m *mockMigration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

func TestMigrationRegistryImpl_Register(t *testing.T) {
	registry := &MigrationRegistryImpl{
		migrations: make(map[float64]MajorMigrationInterface),
	}

	migration := &mockMigration{version: 3.0}

	registry.Register(migration)

	assert.Len(t, registry.migrations, 1)
	assert.Equal(t, migration, registry.migrations[3.0])
}

func TestMigrationRegistryImpl_GetMigrations(t *testing.T) {
	registry := &MigrationRegistryImpl{
		migrations: make(map[float64]MajorMigrationInterface),
	}

	// Add migrations in random order
	migration1 := &mockMigration{version: 3.0}
	migration2 := &mockMigration{version: 1.0}
	migration3 := &mockMigration{version: 2.0}

	registry.Register(migration1)
	registry.Register(migration2)
	registry.Register(migration3)

	migrations := registry.GetMigrations()

	require.Len(t, migrations, 3)

	// Should be sorted by version
	assert.Equal(t, 1.0, migrations[0].GetMajorVersion())
	assert.Equal(t, 2.0, migrations[1].GetMajorVersion())
	assert.Equal(t, 3.0, migrations[2].GetMajorVersion())
}

func TestMigrationRegistryImpl_GetMigrations_Empty(t *testing.T) {
	registry := &MigrationRegistryImpl{
		migrations: make(map[float64]MajorMigrationInterface),
	}

	migrations := registry.GetMigrations()

	assert.Len(t, migrations, 0)
}

func TestMigrationRegistryImpl_GetMigration(t *testing.T) {
	registry := &MigrationRegistryImpl{
		migrations: make(map[float64]MajorMigrationInterface),
	}

	migration := &mockMigration{version: 3.0}
	registry.Register(migration)

	// Test existing migration
	result, exists := registry.GetMigration(3.0)
	assert.True(t, exists)
	assert.Equal(t, migration, result)

	// Test non-existing migration
	result, exists = registry.GetMigration(5.0)
	assert.False(t, exists)
	assert.Nil(t, result)
}

func TestMigrationRegistryImpl_RegisterOverwrite(t *testing.T) {
	registry := &MigrationRegistryImpl{
		migrations: make(map[float64]MajorMigrationInterface),
	}

	migration1 := &mockMigration{version: 3.0, hasSystemUpdate: true}
	migration2 := &mockMigration{version: 3.0, hasSystemUpdate: false}

	registry.Register(migration1)
	registry.Register(migration2)

	// Should overwrite the first migration
	assert.Len(t, registry.migrations, 1)

	result, exists := registry.GetMigration(3.0)
	assert.True(t, exists)
	assert.Equal(t, migration2, result)
	assert.False(t, result.HasSystemUpdate())
}

func TestDefaultRegistry_Register(t *testing.T) {
	// Save original state
	originalMigrations := make(map[float64]MajorMigrationInterface)
	for k, v := range DefaultRegistry.migrations {
		originalMigrations[k] = v
	}

	// Clean up after test
	defer func() {
		DefaultRegistry.migrations = originalMigrations
	}()

	// Clear registry for test
	DefaultRegistry.migrations = make(map[float64]MajorMigrationInterface)

	migration := &mockMigration{version: 4.0}

	Register(migration)

	assert.Len(t, DefaultRegistry.migrations, 1)
	assert.Equal(t, migration, DefaultRegistry.migrations[4.0])
}

func TestGetRegisteredMigrations(t *testing.T) {
	// Save original state
	originalMigrations := make(map[float64]MajorMigrationInterface)
	for k, v := range DefaultRegistry.migrations {
		originalMigrations[k] = v
	}

	// Clean up after test
	defer func() {
		DefaultRegistry.migrations = originalMigrations
	}()

	// Clear registry for test
	DefaultRegistry.migrations = make(map[float64]MajorMigrationInterface)

	migration1 := &mockMigration{version: 2.0}
	migration2 := &mockMigration{version: 1.0}

	Register(migration1)
	Register(migration2)

	migrations := GetRegisteredMigrations()

	require.Len(t, migrations, 2)

	// Should be sorted by version
	assert.Equal(t, 1.0, migrations[0].GetMajorVersion())
	assert.Equal(t, 2.0, migrations[1].GetMajorVersion())
}

func TestGetRegisteredMigration(t *testing.T) {
	// Save original state
	originalMigrations := make(map[float64]MajorMigrationInterface)
	for k, v := range DefaultRegistry.migrations {
		originalMigrations[k] = v
	}

	// Clean up after test
	defer func() {
		DefaultRegistry.migrations = originalMigrations
	}()

	// Clear registry for test
	DefaultRegistry.migrations = make(map[float64]MajorMigrationInterface)

	migration := &mockMigration{version: 3.0}
	Register(migration)

	// Test existing migration
	result, exists := GetRegisteredMigration(3.0)
	assert.True(t, exists)
	assert.Equal(t, migration, result)

	// Test non-existing migration
	result, exists = GetRegisteredMigration(5.0)
	assert.False(t, exists)
	assert.Nil(t, result)
}

func TestMigrationRegistryImpl_ConcurrentAccess(t *testing.T) {
	registry := &MigrationRegistryImpl{
		migrations: make(map[float64]MajorMigrationInterface),
	}

	migration := &mockMigration{version: 1.0}
	registry.Register(migration)

	// Test concurrent reads don't panic
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			registry.GetMigrations()
			registry.GetMigration(1.0)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			registry.GetMigrations()
			registry.GetMigration(1.0)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify registry is still consistent
	migrations := registry.GetMigrations()
	assert.Len(t, migrations, 1)
	assert.Equal(t, 1.0, migrations[0].GetMajorVersion())
}
