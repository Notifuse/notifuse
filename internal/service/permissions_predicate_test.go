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

// An opt-in resource (domain.OptInPermissionResources) is invisible to the predicate in both
// directions: a map that lacks it is still full access, and a map that carries it — read-only,
// as the console writes it — is still full access. This is what keeps every pre-v41 stored map
// "full" on upgrade, and what lets an owner grant audit-log read on an unlicensed deployment
// without the RBAC gate answering 402.
func TestGrantsFullPermissions_IgnoresOptInResources(t *testing.T) {
	require.NotEmpty(t, domain.OptInPermissionResources)

	without := domain.NewFullPermissions()
	assert.True(t, grantsFullPermissions(without), "a map that predates the opt-in resource is full access")

	with := domain.NewFullPermissions()
	for _, resource := range domain.OptInPermissionResources {
		with[resource] = domain.ResourcePermissions{Read: true, Write: false}
	}
	assert.True(t, grantsFullPermissions(with), "granting an opt-in resource does not make a set restricted")

	// And the opposite still holds: dropping a real resource is restricted.
	restricted := domain.NewFullPermissions()
	restricted[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true, Write: false}
	assert.False(t, grantsFullPermissions(restricted))
}
