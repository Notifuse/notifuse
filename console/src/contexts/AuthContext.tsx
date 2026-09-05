import { createContext, useContext, useState, useEffect, ReactNode, useCallback } from 'react'
import { authService } from '../services/api/auth'
import { workspaceService } from '../services/api/workspace'
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import { Workspace, UserPermissions } from '../services/api/types'
import { isRootUser } from '../services/api/auth'
import type { Entitlements } from '../types/license'

export interface User {
  id: string
  email: string
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

// Custom hook to get user permissions for a specific workspace
// eslint-disable-next-line react-refresh/only-export-components -- Hook co-located with context
export function useWorkspacePermissions(workspaceId: string) {
  const { user } = useAuth()
  const [permissions, setPermissions] = useState<UserPermissions | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchPermissions = async () => {
      if (!user || !workspaceId) {
        setLoading(false)
        return
      }

      // If user is root, they have full permissions
      if (isRootUser(user.email)) {
        setPermissions(ROOT_USER_PERMISSIONS)
        setLoading(false)
        return
      }

      try {
        const response = await workspaceService.getMembers(workspaceId)
        const currentUserMember = response.members.find((member) => member.user_id === user.id)

        if (currentUserMember) {
          // The stored map may be partial or null; a resource it does not mention is denied,
          // which is what the empty base spells out.
          setPermissions({ ...createEmptyPermissions(), ...currentUserMember.permissions })
        } else {
          // User is not a member of this workspace, set empty permissions
          setPermissions(NO_PERMISSIONS)
        }
      } catch (error) {
        console.error('Failed to fetch user permissions', error)
        // On error, assume no permissions
        setPermissions(NO_PERMISSIONS)
      } finally {
        setLoading(false)
      }
    }

    fetchPermissions()
  }, [workspaceId, user])

  return { permissions, loading }
}
