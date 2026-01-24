# Admin Role Implementation Plan

## Context

**GitHub Issue**: [#208 - Allow adding users as workspace owners via API](https://github.com/Notifuse/notifuse/issues/208)

**Problem**: SaaS platforms provisioning Notifuse workspaces for customers need to give those customers autonomy to manage their workspace (invite team, configure integrations, manage permissions) without making them full owners.

**Current State**: Only two roles exist (`owner` and `member`). Members cannot perform administrative tasks even with full permissions because these operations have role-based checks, not permission-based checks.

**Solution**: Introduce an `admin` role that provides operational autonomy without the risks associated with multiple owners.

---

## Role Hierarchy

```
owner (exactly 1 per workspace)
  └── admin (unlimited)
        └── member (unlimited, with granular permissions)
```

| Role | Description |
|------|-------------|
| **owner** | Full control including destructive operations. Single point of accountability. |
| **admin** | Operational autonomy. Can manage team, permissions, and integrations. Cannot perform destructive workspace operations. |
| **member** | Access based on granular permissions. No administrative capabilities. |

---

## Permission Matrix

### Role-Based Operations

| Operation | Owner | Admin | Member |
|-----------|:-----:|:-----:|:------:|
| **Workspace Management** ||||
| Update workspace settings | ✅ | ❌ | ❌ |
| Delete workspace | ✅ | ❌ | ❌ |
| Transfer ownership | ✅ | ❌ | ❌ |
| **Member Management** ||||
| Invite members | ✅ | ✅ | ❌ |
| Remove members | ✅ | ✅ | ❌ |
| Set member permissions | ✅ | ✅ | ❌ |
| Promote member to admin | ✅ | ❌ | ❌ |
| Demote admin to member | ✅ | ❌ | ❌ |
| Remove admins | ✅ | ❌ | ❌ |
| **API Keys** ||||
| Create API keys | ✅ | ✅ | ❌ |
| Delete API keys | ✅ | ✅ | ❌ |
| **Integrations** ||||
| Create integrations | ✅ | ✅ | ❌ |
| Update integrations | ✅ | ✅ | ❌ |
| Delete integrations | ✅ | ✅ | ❌ |
| **Resource Access** ||||
| Access resources | ✅ (all) | ✅ (all) | Per permissions |

### Summary

- **Owner-only**: Workspace deletion, ownership transfer, workspace settings update, admin management
- **Admin+**: Member invite/remove, permission management, API keys, integrations
- **All roles**: Resource access (contacts, lists, templates, etc.) based on permissions

---

## Implementation Steps

### Step 1: Update Domain Layer

**File**: `internal/domain/workspace.go`

1. Update role validation to accept three values:
```go
// Line ~820 in Validate()
if uw.Role != "owner" && uw.Role != "admin" && uw.Role != "member" {
    return fmt.Errorf("invalid user workspace: role must be 'owner', 'admin', or 'member'")
}
```

2. Update `HasPermission` method to grant full permissions to admins:
```go
// Line ~830 in HasPermission()
func (uw *UserWorkspace) HasPermission(resource PermissionResource, permissionType PermissionType) bool {
    if uw.Role == "owner" || uw.Role == "admin" {
        return true // Owners and admins have all resource permissions
    }
    // Members: check individual permissions...
}
```

3. Add helper methods for role checks:
```go
func (uw *UserWorkspace) IsOwner() bool {
    return uw.Role == "owner"
}

func (uw *UserWorkspace) IsAdmin() bool {
    return uw.Role == "admin"
}

func (uw *UserWorkspace) IsAdminOrOwner() bool {
    return uw.Role == "owner" || uw.Role == "admin"
}

func (uw *UserWorkspace) CanManageMembers() bool {
    return uw.Role == "owner" || uw.Role == "admin"
}

func (uw *UserWorkspace) CanManageAdmins() bool {
    return uw.Role == "owner"
}
```

### Step 2: Update Service Layer

**File**: `internal/service/workspace_service.go`

#### 2.1 Update InviteMember to accept role parameter

```go
// Update function signature (~line 589)
func (s *WorkspaceService) InviteMember(
    ctx context.Context,
    workspaceID string,
    email string,
    permissions domain.UserPermissions,
    role string,  // NEW: optional role parameter
) (*domain.WorkspaceInvitation, string, error)
```

- Default role to `"member"` if empty
- Validate role is one of: `owner`, `admin`, `member`
- Only owners can invite as `admin`
- Nobody can invite as `owner` (use TransferOwnership instead)

#### 2.2 Update authorization checks

Replace `Role != "owner"` checks with appropriate helper methods:

| Location | Current Check | New Check |
|----------|---------------|-----------|
| `UpdateWorkspace` (~302) | `Role != "owner"` | `!IsOwner()` (unchanged behavior) |
| `DeleteWorkspace` (~419) | `Role != "owner"` | `!IsOwner()` (unchanged behavior) |
| `InviteMember` (~617) | `Role != "owner"` | `!CanManageMembers()` |
| `RemoveMember` (~704) | `Role != "owner"` | `!CanManageMembers()` + special handling |
| `SetUserPermissions` (~722) | `Role != "owner"` | `!CanManageMembers()` |
| `CreateAPIKey` (~854) | `Role != "owner"` | `!IsAdminOrOwner()` |
| `DeleteAPIKey` (~900) | `Role != "owner"` | `!IsAdminOrOwner()` |
| `CreateIntegration` (~1000) | `Role != "owner"` | `!IsAdminOrOwner()` |
| `UpdateIntegration` (~1070) | `Role != "owner"` | `!IsAdminOrOwner()` |
| `DeleteIntegration` (~1238) | `Role != "owner"` | `!IsAdminOrOwner()` |
| `TransferOwnership` (~550) | `Role != "owner"` | `!IsOwner()` (unchanged behavior) |

#### 2.3 Add role management constraints

In `RemoveMember`:
```go
// Admins cannot remove other admins or the owner
if callerWorkspace.Role == "admin" {
    if targetWorkspace.Role == "owner" || targetWorkspace.Role == "admin" {
        return fmt.Errorf("admins cannot remove owners or other admins")
    }
}
```

In `SetUserPermissions`:
```go
// Admins can only modify member permissions, not admin/owner
if callerWorkspace.Role == "admin" && targetWorkspace.Role != "member" {
    return fmt.Errorf("admins can only modify member permissions")
}
// Owners can modify member and admin permissions, but not other owners
if targetWorkspace.Role == "owner" {
    return fmt.Errorf("cannot modify owner permissions")
}
```

#### 2.4 Add PromoteToAdmin and DemoteToMember methods

```go
// PromoteToAdmin promotes a member to admin role (owner only)
func (s *WorkspaceService) PromoteToAdmin(ctx context.Context, workspaceID, userID string) error {
    // 1. Verify caller is owner
    // 2. Verify target is member
    // 3. Update role to "admin" with FullPermissions
}

// DemoteToMember demotes an admin to member role (owner only)
func (s *WorkspaceService) DemoteToMember(ctx context.Context, workspaceID, userID string, permissions domain.UserPermissions) error {
    // 1. Verify caller is owner
    // 2. Verify target is admin
    // 3. Update role to "member" with specified permissions
}
```

### Step 3: Update HTTP Handler

**File**: `internal/http/workspace_handler.go`

#### 3.1 Update InviteMemberRequest

```go
type InviteMemberRequest struct {
    WorkspaceID string                 `json:"workspace_id"`
    Email       string                 `json:"email"`
    Permissions domain.UserPermissions `json:"permissions"`
    Role        string                 `json:"role,omitempty"` // NEW: "member" (default), "admin"
}
```

#### 3.2 Add new endpoints

```go
// POST /api/workspaces.promoteToAdmin
type PromoteToAdminRequest struct {
    WorkspaceID string `json:"workspace_id"`
    UserID      string `json:"user_id"`
}

// POST /api/workspaces.demoteToMember
type DemoteToMemberRequest struct {
    WorkspaceID string                 `json:"workspace_id"`
    UserID      string                 `json:"user_id"`
    Permissions domain.UserPermissions `json:"permissions"`
}
```

### Step 4: Database Migration

**File**: `internal/migrations/v27.go` (or next version)

No schema changes required. The `role` column is already `VARCHAR(20)` and can store `"admin"`.

Migration should update documentation/changelog only.

### Step 5: Update Frontend

**File**: `console/src/components/settings/WorkspaceMembers.tsx`

1. Display admin role with distinct styling (e.g., purple tag)
2. Show promote/demote buttons for owners viewing members/admins
3. Update permission management UI to respect role hierarchy

**File**: `console/src/services/api/workspace.ts`

1. Add `role` parameter to `inviteMember` function
2. Add `promoteToAdmin` and `demoteToMember` API calls

---

## API Examples

### Invite member as admin
```bash
POST /api/workspaces.inviteMember
{
  "workspace_id": "myworkspace",
  "email": "admin@example.com",
  "role": "admin",
  "permissions": {}  # ignored for admin, gets FullPermissions
}
```

### Promote existing member to admin
```bash
POST /api/workspaces.promoteToAdmin
{
  "workspace_id": "myworkspace",
  "user_id": "user-uuid"
}
```

### Demote admin to member
```bash
POST /api/workspaces.demoteToMember
{
  "workspace_id": "myworkspace",
  "user_id": "user-uuid",
  "permissions": {
    "contacts": { "read": true, "write": true },
    "templates": { "read": true, "write": false }
  }
}
```

---

## Testing Requirements

### Backend Unit Tests

**Domain tests** (`internal/domain/workspace_test.go`):
- Test `Validate()` accepts "owner", "admin", "member"
- Test `Validate()` rejects invalid roles
- Test `HasPermission()` returns true for admin role
- Test `IsOwner()`, `IsAdmin()`, `IsAdminOrOwner()`, `CanManageMembers()`, `CanManageAdmins()`

**Service tests** (`internal/service/workspace_service_test.go`):
- Test `InviteMember` with role parameter
- Test admin can invite members but not admins
- Test admin can remove members but not admins/owner
- Test admin can set member permissions but not admin/owner permissions
- Test admin can create/delete API keys
- Test admin can manage integrations
- Test admin cannot update/delete workspace
- Test admin cannot transfer ownership
- Test `PromoteToAdmin` only works for owners
- Test `DemoteToMember` only works for owners

**HTTP tests** (`internal/http/workspace_handler_test.go`):
- Test new request structs parse correctly
- Test new endpoints are properly registered
- Test authorization middleware for new endpoints

### Frontend Tests

- Test role badge displays correctly for admin
- Test promote/demote buttons appear only for owners
- Test API calls include role parameter

### Integration Tests

- Full flow: create workspace → invite admin → admin invites member → admin manages permissions
- Verify admin cannot perform owner-only operations

---

## Rollout Considerations

1. **Backward Compatibility**: Existing workspaces continue to work. No migration needed for existing data.

2. **API Versioning**: The `role` parameter in `inviteMember` is optional and defaults to `"member"`.

3. **Documentation**: Update API docs to describe new role and endpoints.

4. **GitHub Issue Resolution**: This addresses #208 by providing a clean way to give users administrative access without full ownership.

---

## Files to Modify

| File | Changes |
|------|---------|
| `internal/domain/workspace.go` | Role validation, HasPermission, helper methods |
| `internal/domain/workspace_test.go` | Tests for new role logic |
| `internal/service/workspace_service.go` | Authorization checks, new methods |
| `internal/service/workspace_service_test.go` | Tests for new service methods |
| `internal/http/workspace_handler.go` | New endpoints, updated request structs |
| `internal/http/workspace_handler_test.go` | Tests for new endpoints |
| `console/src/components/settings/WorkspaceMembers.tsx` | UI for admin role |
| `console/src/services/api/workspace.ts` | New API functions |
| `CHANGELOG.md` | Document new feature |
