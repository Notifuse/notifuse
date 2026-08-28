package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPermissionEnforced(t *testing.T) {
	t.Run("the verbs no endpoint gates are not enforced", func(t *testing.T) {
		assert.False(t, IsPermissionEnforced(PermissionResourceLLM, PermissionTypeRead))
		assert.False(t, IsPermissionEnforced(PermissionResourceMessageHistory, PermissionTypeWrite))
		assert.False(t, IsPermissionEnforced(PermissionResourceWebhookEvents, PermissionTypeWrite))
	})

	t.Run("the other verb of the same resource is enforced", func(t *testing.T) {
		assert.True(t, IsPermissionEnforced(PermissionResourceLLM, PermissionTypeWrite))
		assert.True(t, IsPermissionEnforced(PermissionResourceMessageHistory, PermissionTypeRead))
		assert.True(t, IsPermissionEnforced(PermissionResourceWebhookEvents, PermissionTypeRead))
	})

	t.Run("gated verbs are enforced", func(t *testing.T) {
		assert.True(t, IsPermissionEnforced(PermissionResourceContacts, PermissionTypeRead))
		assert.True(t, IsPermissionEnforced(PermissionResourceContacts, PermissionTypeWrite))
		assert.True(t, IsPermissionEnforced(PermissionResourceLists, PermissionTypeWrite))
		assert.True(t, IsPermissionEnforced(PermissionResourceSegments, PermissionTypeRead))
		assert.True(t, IsPermissionEnforced(PermissionResourceWorkspace, PermissionTypeWrite))
	})
}

func TestGrantUnenforcedPermissions(t *testing.T) {
	t.Run("grants the unenforced verbs and leaves the rest of the set alone", func(t *testing.T) {
		input := NewEmptyPermissions()
		input[PermissionResourceContacts] = ResourcePermissions{Read: true}
		input[PermissionResourceLLM] = ResourcePermissions{Write: true}

		granted := GrantUnenforcedPermissions(input)

		// Restated rather than read out of UnenforcedPermissions: a diff computed from the very
		// list it is checking passes whatever that list happens to hold.
		expectedGrants := map[PermissionResource]ResourcePermissions{
			PermissionResourceLLM:            {Read: true},
			PermissionResourceMessageHistory: {Write: true},
			PermissionResourceWebhookEvents:  {Write: true},
		}
		for resource, verbs := range input {
			want := verbs
			if grant, ok := expectedGrants[resource]; ok {
				want.Read = want.Read || grant.Read
				want.Write = want.Write || grant.Write
			}
			assert.Equal(t, want, granted[resource], "resource %s", resource)
		}
		assert.Len(t, granted, len(input), "no resource beyond the input's own may appear")
	})

	t.Run("returns a map that does not alias its input", func(t *testing.T) {
		input := NewEmptyPermissions()

		granted := GrantUnenforcedPermissions(input)
		granted[PermissionResourceContacts] = ResourcePermissions{Read: true, Write: true}
		granted[PermissionResourceLLM] = ResourcePermissions{Read: true, Write: true}

		assert.Equal(t, ResourcePermissions{}, input[PermissionResourceContacts])
		assert.Equal(t, ResourcePermissions{}, input[PermissionResourceLLM])
	})
}
