import type { StoredPermissions } from '../../services/api/permissions'

export type WorkspaceRole = 'root' | 'owner' | 'member' | undefined

// The rule internal/domain/file_storage.go applies (UserWorkspace.CanReadFiles and
// CanWriteFiles): the files belong to the two screens that pick images, templates and the blog.

/** May list the workspace's files: any right, read or write, on templates or the blog. */
export function canReadFiles(permissions: StoredPermissions | null | undefined, role: WorkspaceRole): boolean {
  if (role === 'owner' || role === 'root') return true
  return (
    canWriteFiles(permissions, role) ||
    Boolean(permissions?.templates?.read) ||
    Boolean(permissions?.blog?.read)
  )
}

/** May upload and delete files: write access to templates or to the blog. */
export function canWriteFiles(permissions: StoredPermissions | null | undefined, role: WorkspaceRole): boolean {
  if (role === 'owner' || role === 'root') return true
  return Boolean(permissions?.templates?.write) || Boolean(permissions?.blog?.write)
}

/** May change the storage settings: they are saved and tested by owner-only endpoints. */
export function canEditStorageSettings(role: WorkspaceRole): boolean {
  return role === 'owner' || role === 'root'
}
