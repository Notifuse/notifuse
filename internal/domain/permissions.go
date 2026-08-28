package domain

// UnenforcedPermission names one verb of one resource.
type UnenforcedPermission struct {
	Resource PermissionResource
	Type     PermissionType
}

// UnenforcedPermissions are the verbs no gate can enforce as the API stands: /api/llm.chat is
// the only LLM route, and all four message-history service methods are reads.
//
// They are granted rather than denied because a stored false here is permanent: every
// permission backfill only ever adds the keys a row is missing — v39's `defaults || permissions`
// is the shape, with the stored map on the right so it always wins — so nothing would widen a
// stored false if a later release gives the verb a real gate.
//
// The console renders these granted and locked for the same reason; its copy of this list is
// UNENFORCED_PERMISSIONS in console/src/services/api/permissions.ts.
var UnenforcedPermissions = []UnenforcedPermission{
	{Resource: PermissionResourceLLM, Type: PermissionTypeRead},
	{Resource: PermissionResourceMessageHistory, Type: PermissionTypeWrite},
	{Resource: PermissionResourceWebhookEvents, Type: PermissionTypeWrite},
}

// IsPermissionEnforced reports whether any endpoint gates this verb today. A caller building a
// scoped permission set uses it to tell a deliberate denial from one that means nothing.
func IsPermissionEnforced(resource PermissionResource, permissionType PermissionType) bool {
	for _, unenforced := range UnenforcedPermissions {
		if unenforced.Resource == resource && unenforced.Type == permissionType {
			return false
		}
	}
	return true
}

// NewEmptyPermissions returns a fresh map denying read and write on every resource. It is the
// base for a scoped grant, and the opposite of NewFullPermissions — which the same warning
// applies to: never hand a caller a shared map, mutating it corrupts every set built from it.
//
// Every resource is spelled out rather than left absent: a permission gate reads a missing key
// as denied either way, but a set that is stored keeps the denial visible to the console's
// matrix and to the next backfill.
func NewEmptyPermissions() UserPermissions {
	permissions := make(UserPermissions, len(AllPermissionResources))
	for _, resource := range AllPermissionResources {
		permissions[resource] = ResourcePermissions{}
	}
	return permissions
}

// GrantUnenforcedPermissions returns a copy of permissions with every unenforced verb granted,
// leaving the rest of the set alone. Applied to any set about to be persisted, so an
// unenforceable verb never stores as a false no future backfill would widen.
func GrantUnenforcedPermissions(permissions UserPermissions) UserPermissions {
	granted := make(UserPermissions, len(permissions)+len(UnenforcedPermissions))
	for resource, verbs := range permissions {
		granted[resource] = verbs
	}
	for _, unenforced := range UnenforcedPermissions {
		verbs := granted[unenforced.Resource]
		switch unenforced.Type {
		case PermissionTypeRead:
			verbs.Read = true
		case PermissionTypeWrite:
			verbs.Write = true
		}
		granted[unenforced.Resource] = verbs
	}
	return granted
}
