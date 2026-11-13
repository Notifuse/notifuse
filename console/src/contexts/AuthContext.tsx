import { createContext, useContext, useState, useEffect, useMemo, ReactNode, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { authService } from '../services/api/auth'
import { workspaceService } from '../services/api/workspace'
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import { Workspace, UserPermissions } from '../services/api/types'
import { isRootUser } from '../services/api/auth'
import type { Entitlements } from '../types/license'

export interface User {
  id: string
  email: string
  /** The account's display name, when the server sends one: the avatar's initials. */
  name?: string
  language?: string
}

interface AuthContextType {
  user: User | null
  workspaces: Workspace[]
  isAuthenticated: boolean
  signin: (token: string) => Promise<void>
  signout: () => Promise<void>
  loading: boolean
  refreshWorkspaces: () => Promise<void>
  // Licence state as /api/user.me reported it, or null when it said nothing — a server older
  // than this bundle, or one that keeps the licence on its root-only endpoint. Null is "not
  // told", not "unlicensed"; LicenseProvider is what turns either into an answer.
  licenseEntitlements: Entitlements | null
}

// eslint-disable-next-line react-refresh/only-export-components -- Context co-located with provider
export const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [loading, setLoading] = useState(true)
  const [licenseEntitlements, setLicenseEntitlements] = useState<Entitlements | null>(null)

  // Kept next to the setter it always accompanies, so a fourth call site of getCurrentUser()
  // cannot pick up the workspaces and forget the licence — which is precisely how
  // refreshWorkspaces() would have gone stale.
  const adoptLicense = (payload: { entitlements?: Entitlements }) => {
    setLicenseEntitlements(payload.entitlements ?? null)
  }

  const checkAuth = useCallback(async () => {
    // console.log('checkAuth')
    try {
      // Check if a token exists in localStorage
      const token = localStorage.getItem('auth_token')
      if (!token) {
        setLoading(false)
        return
      }

      // Token exists, fetch current user data
      const response = await authService.getCurrentUser()
      setUser(response.user)
      setWorkspaces(response.workspaces ?? [])
      adoptLicense(response)
      setLoading(false)
    } catch {
      // If there's an error (like an expired token), clear the storage
      localStorage.removeItem('auth_token')
      setUser(null)
      setWorkspaces([])
      adoptLicense({})
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    // Check for existing session on component mount
    void checkAuth()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const signin = async (token: string) => {
    // console.log('signin')
    try {
      // Store token in localStorage for persistence
      localStorage.setItem('auth_token', token)

      // Fetch current user data using the token
      const response = await authService.getCurrentUser()
      setUser(response.user)
      setWorkspaces(response.workspaces ?? [])
      adoptLicense(response)
    } catch (error) {
      // If there's an error, clear the storage
      localStorage.removeItem('auth_token')
      throw error
    }
  }

  const signout = async () => {
    try {
      // Call backend to invalidate all sessions
      await authService.logout()
    } catch (error) {
      // Even if backend call fails, we still logout locally
      console.error('Failed to logout on backend:', error)
    }

    // Remove token from localStorage
    localStorage.removeItem('auth_token')

    // Clear user data
    setUser(null)
    setWorkspaces([])
    adoptLicense({})
  }

  const refreshWorkspaces = async () => {
    const response = await authService.getCurrentUser()
    // getCurrentUser declares workspaces as Workspace[] | null; normalize before it reaches
    // state that consumers index and map over.
    setWorkspaces(response.workspaces ?? [])
    adoptLicense(response)
  }

  // console.log('user', user)

  return (
    <AuthContext.Provider
      value={{
        user,
        workspaces,
        isAuthenticated: !!user,
        signin,
        signout,
        loading,
        refreshWorkspaces,
        licenseEntitlements
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components -- Hook co-located with context
export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}

// Fallback permission sets used when there is no member record to read from: a root user (who
// has everything) and a non-member or failed lookup (who has nothing). Both are built from
// ALL_PERMISSION_RESOURCES, so a resource added to the type cannot end up silently absent here,
// which every permission gate reads as "denied".
const ROOT_USER_PERMISSIONS: UserPermissions = createFullPermissions()

const NO_PERMISSIONS: UserPermissions = createEmptyPermissions()

export interface WorkspacePermissions {
  // Null while the workspace's members load, and when there is no user or no workspace.
  permissions: UserPermissions | null
  // The user's role in the workspace; undefined while loading and for a non-member.
  role: 'root' | 'owner' | 'member' | undefined
  loading: boolean
}

// The signed-in user's permissions in a workspace, read from its members list. Pages and the
// workspace layout stay mounted when the workspace changes, so the list is a query keyed by the
// workspace: after a switch the hook answers "loading" until the new workspace's list arrives,
// never the previous workspace's permissions, and a late answer for a previous workspace only
// fills that workspace's entry. The key is shared with useAuditLogAccess and the audit log, so
// they fetch the list once.
// eslint-disable-next-line react-refresh/only-export-components -- Hook co-located with context
export function useWorkspacePermissions(workspaceId: string): WorkspacePermissions {
  const { user } = useAuth()
  const isRoot = isRootUser(user?.email)

  const userId = user?.id

  // The list depends on who asks (a member without Workspace read gets only their own entry),
  // hence the user in the key.
  const { data, isFetchedAfterMount } = useQuery({
    queryKey: ['workspace-members', workspaceId, userId],
    queryFn: () => workspaceService.getMembers(workspaceId),
    enabled: !!userId && !!workspaceId && !isRoot
  })

  // Memoised, so a caller that depends on the permissions object sees it change only when the
  // answer does.
  return useMemo<WorkspacePermissions>(() => {
    if (!userId || !workspaceId) {
      return { permissions: null, role: undefined, loading: false }
    }
    // A root user has every permission, with no request.
    if (isRoot) {
      return { permissions: ROOT_USER_PERMISSIONS, role: 'root', loading: false }
    }
    // A cached list may predate a change of permissions: wait for this mount's own answer, as
    // when each caller fetched its own. (resetQueries on this key would set its answer count
    // back and leave a caller mounted over a cached list loading: nothing resets it today.)
    if (!isFetchedAfterMount) {
      return { permissions: null, role: undefined, loading: true }
    }
    // On error with no list, assume no permissions. A failed refetch keeps the list the query
    // holds, so a network blip neither empties the menu nor sends a page away.
    if (!data) {
      return { permissions: NO_PERMISSIONS, role: undefined, loading: false }
    }

    const currentUserMember = data.members.find((member) => member.user_id === userId)
    if (!currentUserMember) {
      // User is not a member of this workspace, set empty permissions
      return { permissions: NO_PERMISSIONS, role: undefined, loading: false }
    }
    return {
      // The stored map may be partial or null; a resource it does not mention is denied, which
      // is what the empty base spells out.
      permissions: { ...createEmptyPermissions(), ...currentUserMember.permissions },
      role: currentUserMember.role === 'owner' ? 'owner' : 'member',
      loading: false
    }
  }, [userId, workspaceId, isRoot, data, isFetchedAfterMount])
}
