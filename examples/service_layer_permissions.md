# Service-Layer Permission Implementation

## Overview

Following clean architecture principles, all permission validation has been moved to the service layer rather than HTTP middleware. This maintains proper separation of concerns and avoids adding repository dependencies to the HTTP layer.

## Architecture Decision

### Why Service-Layer Authorization?

1. **Clean Architecture**: HTTP layer remains thin, focused only on transport concerns
2. **Separation of Concerns**: Business logic (including permissions) stays in business layer
3. **No Circular Dependencies**: Avoids HTTP layer depending on repositories
4. **Testability**: Each layer can be tested independently
5. **Future-Proof**: Easy to add new transport layers (gRPC, GraphQL) later

### Architecture Layers

```
HTTP Layer (Handlers)     ← Thin: parsing, response formatting
    ↓ (delegates to)
Service Layer             ← Rich: business logic + authorization
    ↓ (depends on)
Repository Layer          ← Pure: data access only
```

## Implementation Pattern

### Service Method Structure

Every service method follows this pattern:

```go
func (s *SomeService) SomeMethod(ctx context.Context, req *SomeRequest) (*SomeResponse, error) {
    // 1. Authenticate user for workspace (single DB query, loads permissions)
    ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
    if err != nil {
        return nil, err // Auth error (401)
    }

    // 2. Check specific permission (in-memory check, no DB call)
    if !userWorkspace.HasPermission(domain.PermissionResourceSome, domain.PermissionTypeWrite) {
        return nil, domain.ErrInsufficientPermissions // Permission error (403)
    }

    // 3. Business logic continues...
    return s.repo.DoSomething(ctx, req)
}
```

### HTTP Handler Structure

HTTP handlers remain thin and focused on transport:

```go
func (h *SomeHandler) handleSomething(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request (HTTP concern)
    req, err := parseRequest(r)
    if err != nil {
        writeJSONError(w, "Invalid request", http.StatusBadRequest)
        return
    }

    // 2. Delegate to service (business logic)
    response, err := h.service.DoSomething(r.Context(), req)

    // 3. Handle errors and format response (HTTP concern)
    switch e := err.(type) {
    case *domain.PermissionError:
        writeJSONError(w, e.Message, http.StatusForbidden)
    case *domain.AuthError:
        writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
    case nil:
        writeJSONResponse(w, response)
    default:
        writeJSONError(w, "Internal error", http.StatusInternalServerError)
    }
}
```

## Permission Resources and Types

### Resources

- `contacts`: Contact management operations
- `lists`: List management operations
- `templates`: Template management operations
- `broadcasts`: Broadcast management operations
- `transactional`: Transactional email operations
- `workspace`: Workspace management operations
- `message_history`: Message history access

### Permission Types

- `read`: View/list access to resources
- `write`: Create/update/delete access to resources

### Owner Privileges

- Users with `role = "owner"` automatically have all permissions
- `userWorkspace.HasPermission()` returns `true` for owners regardless of explicit permissions

## Performance Characteristics

### Single Database Query

- `AuthenticateUserForWorkspace` loads user + workspace + permissions in one query
- Subsequent permission checks are in-memory only
- No additional database calls for permission validation

### Context Caching

- `UserWorkspace` stored in context after first authentication
- Multiple service calls in same request reuse cached permissions
- Maintains high performance while ensuring security

## Error Handling

### Domain Errors

```go
// Define in domain layer
type PermissionError struct {
    Resource   PermissionResource
    Permission PermissionType
    Message    string
}

func (e *PermissionError) Error() string {
    return e.Message
}

var ErrInsufficientPermissions = &PermissionError{
    Message: "Insufficient permissions",
}
```

### HTTP Translation

```go
// In HTTP handlers
switch e := err.(type) {
case *domain.PermissionError:
    writeJSONError(w, fmt.Sprintf("Insufficient permissions: %s access to %s required",
        e.Permission, e.Resource), http.StatusForbidden)
case *domain.AuthError:
    writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
}
```

## Current Implementation Status

### ✅ Completed Components

1. **Authentication Infrastructure**

   - `AuthenticateUserForWorkspace` returns `UserWorkspace` with permissions
   - Context storage for performance optimization
   - Owner privilege handling

2. **Permission Model**

   - Resource and permission type definitions
   - `HasPermission()` method with owner bypass
   - Database storage in JSONB columns

3. **HTTP Layer**

   - All handlers use only `RequireAuth` middleware
   - Clean separation: no permission logic in HTTP layer
   - Proper error handling and response formatting

4. **Service Layer Integration**
   - All services call `AuthenticateUserForWorkspace`
   - Permission checks implemented where needed
   - Consistent error handling

### 🔄 Service Methods Needing Permission Checks

The following service methods currently call `AuthenticateUserForWorkspace` but may need explicit permission checks added:

#### Contact Service

- `GetContacts` - needs `contacts:read`
- `GetContactByEmail` - needs `contacts:read`
- `GetContactByExternalID` - needs `contacts:read`
- `DeleteContact` - needs `contacts:write`
- `UpsertContact` - needs `contacts:write`
- `BatchImportContacts` - needs `contacts:write`

#### List Service

- `CreateList` - needs `lists:write`
- `GetListByID` - needs `lists:read`
- `GetLists` - needs `lists:read`
- `UpdateList` - needs `lists:write`
- `DeleteList` - needs `lists:write`
- `GetListStats` - needs `lists:read`
- `SubscribeToLists` - needs `lists:write`
- `UnsubscribeFromLists` - needs `lists:write`

#### Template Service

- `CreateTemplate` - needs `templates:write`
- `GetTemplateByID` - needs `templates:read`
- `GetTemplates` - needs `templates:read`
- `UpdateTemplate` - needs `templates:write`
- `DeleteTemplate` - needs `templates:write`
- `CompileTemplate` - needs `templates:read`

#### Broadcast Service

- Similar pattern for broadcast operations

#### Transactional Service

- Similar pattern for transactional operations

#### Message History Service

- `ListMessages` - needs `message_history:read`
- `GetBroadcastStats` - needs `message_history:read`

### 📋 Next Steps

1. **Add Explicit Permission Checks**: Go through each service method and add appropriate permission validation
2. **Define Permission Requirements**: Document which permission each service method requires
3. **Add Permission Tests**: Test both success and failure cases for each permission check
4. **Update Documentation**: Document the service-layer permission pattern for new developers

## Benefits of This Approach

1. **Clean Architecture**: Proper layer separation maintained
2. **High Performance**: Single database query + in-memory checks
3. **Consistent Security**: All business logic protected at service layer
4. **Easy Testing**: Each layer tested independently
5. **Future-Proof**: Easy to add new interfaces without changing business logic
6. **Owner Safety**: Workspace owners maintain full access automatically

## Example Usage

### Adding Permission Check to New Service Method

```go
func (s *NewService) CreateSomething(ctx context.Context, req *CreateSomethingRequest) (*CreateSomethingResponse, error) {
    // Authenticate and load permissions
    ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
    if err != nil {
        return nil, err
    }

    // Check permission
    if !userWorkspace.HasPermission(domain.PermissionResourceSomething, domain.PermissionTypeWrite) {
        return nil, &domain.PermissionError{
            Resource:   domain.PermissionResourceSomething,
            Permission: domain.PermissionTypeWrite,
            Message:    "Insufficient permissions: write access to something required",
        }
    }

    // Business logic
    result, err := s.repo.CreateSomething(ctx, req)
    if err != nil {
        return nil, err
    }

    return &CreateSomethingResponse{Something: result}, nil
}
```

This approach provides robust, performant, and architecturally sound permission management while maintaining clean separation of concerns.
