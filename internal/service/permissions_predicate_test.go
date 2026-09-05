package service

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGrantsFullPermissions_DerivedFromResourceList pins the property that the hand-picked
// cases elsewhere cannot: the predicate is derived from domain.AllPermissionResources, not from
// a literal list of resources someone typed once.
//
// This is the failure mode that matters. A hardcoded expectation keeps passing every existing
// test on the day a resource is added to the domain, while silently answering false for every
// deployment in the world — which would gate every RBAC write and report every installation as
// customised. Looping over the canonical list means a new resource is covered the moment it is
// declared.
func TestGrantsFullPermissions_DerivedFromResourceList(t *testing.T) {
	require.NotEmpty(t, domain.AllPermissionResources, "the canonical resource list must not be empty")

	t.Run("a map built from the canonical list is full", func(t *testing.T) {
		assert.True(t, grantsFullPermissions(domain.NewFullPermissions()))
	})

	t.Run("dropping the write verb on any single resource makes it not full", func(t *testing.T) {
		for _, resource := range domain.AllPermissionResources {
			permissions := domain.NewFullPermissions()
			permissions[resource] = domain.ResourcePermissions{Read: true, Write: false}
			assert.Falsef(t, grantsFullPermissions(permissions),
				"read-only on %q must not count as full permissions", resource)
		}
	})

	t.Run("dropping the read verb on any single resource makes it not full", func(t *testing.T) {
		for _, resource := range domain.AllPermissionResources {
			permissions := domain.NewFullPermissions()
			permissions[resource] = domain.ResourcePermissions{Read: false, Write: true}
			assert.Falsef(t, grantsFullPermissions(permissions),
				"write-only on %q must not count as full permissions", resource)
		}
	})

	t.Run("removing any single resource entirely makes it not full", func(t *testing.T) {
		for _, resource := range domain.AllPermissionResources {
			permissions := domain.NewFullPermissions()
			delete(permissions, resource)
			assert.Falsef(t, grantsFullPermissions(permissions),
				"a map missing %q must not count as full permissions", resource)
		}
	})

	t.Run("an unknown extra resource does not stop a full map from being full", func(t *testing.T) {
		// Validate() rejects unknown resources on every write path, so this can only arrive
		// from a row written by a newer build. Reporting such a member as restricted would be
		// a lie in the opposite direction: they hold everything this build knows about.
		permissions := domain.NewFullPermissions()
		permissions[domain.PermissionResource("resource_from_the_future")] = domain.ResourcePermissions{}
		assert.True(t, grantsFullPermissions(permissions))
	})
}
