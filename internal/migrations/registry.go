package migrations

import (
	"sort"
	"sync"
)

// DefaultRegistry is the global migration registry
var DefaultRegistry = &MigrationRegistryImpl{
	migrations: make(map[float64]MajorMigrationInterface),
}

// MigrationRegistryImpl implements MigrationRegistry
type MigrationRegistryImpl struct {
	mu         sync.RWMutex
	migrations map[float64]MajorMigrationInterface
}

// Register adds a migration to the registry
func (r *MigrationRegistryImpl) Register(migration MajorMigrationInterface) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.migrations[migration.GetMajorVersion()] = migration
}

// GetMigrations returns all registered migrations sorted by version
func (r *MigrationRegistryImpl) GetMigrations() []MajorMigrationInterface {
	r.mu.RLock()
	defer r.mu.RUnlock()

	migrations := make([]MajorMigrationInterface, 0, len(r.migrations))
	for _, migration := range r.migrations {
		migrations = append(migrations, migration)
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].GetMajorVersion() < migrations[j].GetMajorVersion()
	})

	return migrations
}

// GetMigration returns a specific migration by version
func (r *MigrationRegistryImpl) GetMigration(version float64) (MajorMigrationInterface, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	migration, exists := r.migrations[version]
	return migration, exists
}

// Register is a convenience function to register migrations with the default registry
func Register(migration MajorMigrationInterface) {
	DefaultRegistry.Register(migration)
}

// GetRegisteredMigrations returns all registered migrations from the default registry
func GetRegisteredMigrations() []MajorMigrationInterface {
	return DefaultRegistry.GetMigrations()
}

// GetRegisteredMigration returns a specific migration from the default registry
func GetRegisteredMigration(version float64) (MajorMigrationInterface, bool) {
	return DefaultRegistry.GetMigration(version)
}
