import { createContext, useContext, useState, useEffect, ReactNode } from 'react'
import { authService } from '../services/api/auth'
import { workspaceService } from '../services/api/workspace'
import { Workspace, WorkspaceMember, UserPermissions } from '../services/api/types'
import { isRootUser } from '../services/api/auth'

export interface User {
  id: string
  email: string
}

interface AuthContextType {
  user: User | null
  workspaces: Workspace[]
  isAuthenticated: boolean
  signin: (token: string) => Promise<void>
  signout: () => Promise<void>
  loading: boolean
  refreshWorkspaces: () => Promise<void>
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // Check for existing session on component mount
    checkAuth()
  }, [])

  const checkAuth = async () => {
    // console.log('checkAuth')
    try {
      // Check if a token exists in localStorage
      const token = localStorage.getItem('auth_token')
      if (!token) {
        setLoading(false)
        return
      }

      // Token exists, fetch current user data
      const { user, workspaces } = await authService.getCurrentUser()
      setUser(user)
      setWorkspaces(workspaces)
      setLoading(false)
    } catch (error) {
      // If there's an error (like an expired token), clear the storage
      localStorage.removeItem('auth_token')
      setUser(null)
      setWorkspaces([])
      setLoading(false)
    }
  }

  const signin = async (token: string) => {
    // console.log('signin')
    try {
      // Store token in localStorage for persistence
      localStorage.setItem('auth_token', token)

      // Fetch current user data using the token
      const { user, workspaces } = await authService.getCurrentUser()
      setUser(user)
      setWorkspaces(workspaces)
    } catch (error) {
      // If there's an error, clear the storage
      localStorage.removeItem('auth_token')
      throw error
    }
  }

  const signout = async () => {
    // Remove token from localStorage
    localStorage.removeItem('auth_token')

    // Clear user data
    setUser(null)
    setWorkspaces([])
  }

  const refreshWorkspaces = async () => {
    const { workspaces } = await authService.getCurrentUser()
    setWorkspaces(workspaces)
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
        refreshWorkspaces
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}

// Custom hook to get user permissions for a specific workspace
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
        setPermissions({
          contacts: { read: true, write: true },
          lists: { read: true, write: true },
          templates: { read: true, write: true },
          broadcasts: { read: true, write: true },
          transactional: { read: true, write: true },
          workspace: { read: true, write: true },
          message_history: { read: true, write: true }
        })
        setLoading(false)
        return
      }

      try {
        const response = await workspaceService.getMembers(workspaceId)
        const currentUserMember = response.members.find((member) => member.user_id === user.id)

        if (currentUserMember) {
          setPermissions(currentUserMember.permissions)
        } else {
          // User is not a member of this workspace, set empty permissions
          setPermissions({
            contacts: { read: false, write: false },
            lists: { read: false, write: false },
            templates: { read: false, write: false },
            broadcasts: { read: false, write: false },
            transactional: { read: false, write: false },
            workspace: { read: false, write: false },
            message_history: { read: false, write: false }
          })
        }
      } catch (error) {
        console.error('Failed to fetch user permissions', error)
        // On error, assume no permissions
        setPermissions({
          contacts: { read: false, write: false },
          lists: { read: false, write: false },
          templates: { read: false, write: false },
          broadcasts: { read: false, write: false },
          transactional: { read: false, write: false },
          workspace: { read: false, write: false },
          message_history: { read: false, write: false }
        })
      } finally {
        setLoading(false)
      }
    }

    fetchPermissions()
  }, [workspaceId, user])

  return { permissions, loading }
}
