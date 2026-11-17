import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, waitFor } from '@testing-library/react'
import { AuthProvider, useAuth } from '../contexts/AuthContext'
import { ReactNode } from 'react'
import { authService } from '../services/api/auth'

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => {
      store[key] = value
    },
    removeItem: (key: string) => {
      delete store[key]
    },
    clear: () => {
      store = {}
    }
  }
})()

Object.defineProperty(window, 'localStorage', { value: localStorageMock })

// Mock authService
vi.mock('../services/api/auth', () => ({
  authService: {
    getCurrentUser: vi.fn().mockResolvedValue({
      user: { id: '123', email: 'test@example.com' },
      workspaces: [{ id: 'workspace1', name: 'Test Workspace' }]
    })
  }
}))

// Create a test component that uses the auth context
const TestComponent = () => {
  const { user, workspaces, isAuthenticated, signin, signout, loading } = useAuth()

  return (
    <div>
      <div data-testid="loading">{loading ? 'Loading' : 'Not Loading'}</div>
      <div data-testid="authenticated">
        {isAuthenticated ? 'Authenticated' : 'Not Authenticated'}
      </div>
      <div data-testid="workspaces-count">{workspaces.length}</div>
      <div data-testid="user">{user ? JSON.stringify(user) : 'No User'}</div>
      <button data-testid="signin" onClick={() => signin('fake-token')}>
        Sign In
      </button>
      <button data-testid="signout" onClick={() => signout()}>
        Sign Out
      </button>
    </div>
  )
}

const wrapper = ({ children }: { children: ReactNode }) => <AuthProvider>{children}</AuthProvider>

describe('AuthContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorageMock.clear()
  })

  it('provides initial auth state', async () => {
    render(<TestComponent />, { wrapper })

    // Initial state should be loading
    expect(screen.getByTestId('loading')).toHaveTextContent('Loading')

    // Wait for check auth to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('Not Loading')
    })

    // Initial state should be not authenticated
    expect(screen.getByTestId('authenticated')).toHaveTextContent('Not Authenticated')
    expect(screen.getByTestId('user')).toHaveTextContent('No User')
  })

  it('handles signin action', async () => {
    render(<TestComponent />, { wrapper })

    // Wait for check auth to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('Not Loading')
    })

    // Initial state
    expect(screen.getByTestId('authenticated')).toHaveTextContent('Not Authenticated')

    // Trigger signin
    await act(async () => {
      screen.getByTestId('signin').click()
    })

    // Check localStorage has token
    expect(localStorageMock.getItem('auth_token')).toBe('fake-token')

    // User should be authenticated
    expect(screen.getByTestId('authenticated')).toHaveTextContent('Authenticated')
    expect(authService.getCurrentUser).toHaveBeenCalled()
    expect(screen.getByTestId('user')).not.toHaveTextContent('No User')
  })

  it('handles signout action', async () => {
    // Set initial token
    localStorageMock.setItem('auth_token', 'fake-token')

    render(<TestComponent />, { wrapper })

    // Wait for check auth to complete and auto-login from token
    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('Not Loading')
      expect(screen.getByTestId('authenticated')).toHaveTextContent('Authenticated')
    })

    // Trigger signout
    await act(async () => {
      screen.getByTestId('signout').click()
    })

    // Token should be removed
    expect(localStorageMock.getItem('auth_token')).toBeNull()

    // User should be signed out
    expect(screen.getByTestId('authenticated')).toHaveTextContent('Not Authenticated')
    expect(screen.getByTestId('user')).toHaveTextContent('No User')
  })

  it('checks for token on initialization', async () => {
    // Set token before render
    localStorageMock.setItem('auth_token', 'existing-token')

    render(<TestComponent />, { wrapper })

    // Should load user from token
    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('Not Loading')
      expect(screen.getByTestId('authenticated')).toHaveTextContent('Authenticated')
    })

    expect(authService.getCurrentUser).toHaveBeenCalled()
  })

  it('normalizes null workspaces from the API to an empty array', async () => {
    // A freshly installed root account gets `workspaces: null` from user.me.
    // The context must coerce it to [] so consumers can safely read .length.
    vi.mocked(authService.getCurrentUser).mockResolvedValueOnce({
      user: { id: '123', email: 'root@example.com', timezone: 'UTC', language: 'en' },
      workspaces: null
    })

    localStorageMock.setItem('auth_token', 'existing-token')

    render(<TestComponent />, { wrapper })

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('Not Loading')
      expect(screen.getByTestId('authenticated')).toHaveTextContent('Authenticated')
    })

    // Should not crash and should expose an empty array (length 0)
    expect(screen.getByTestId('workspaces-count')).toHaveTextContent('0')
  })

  describe('update check flag', () => {
    const UpdateProbe = () => {
      const { checkForUpdates, serverVersion, signout, loading } = useAuth()
      return (
        <div>
          <div data-testid="loading">{loading ? 'Loading' : 'Not Loading'}</div>
          <div data-testid="check">{String(checkForUpdates)}</div>
          <div data-testid="version">{serverVersion ?? 'none'}</div>
          <button data-testid="signout" onClick={() => signout()}>
            Sign Out
          </button>
        </div>
      )
    }

    it('is off when user.me does not send it', async () => {
      localStorageMock.setItem('auth_token', 'existing-token')
      render(<UpdateProbe />, { wrapper })
      await waitFor(() => expect(screen.getByTestId('loading')).toHaveTextContent('Not Loading'))
      expect(screen.getByTestId('check')).toHaveTextContent('false')
      expect(screen.getByTestId('version')).toHaveTextContent('none')
    })

    it('follows user.me, and is cleared on sign-out', async () => {
      vi.mocked(authService.getCurrentUser).mockResolvedValueOnce({
        user: { id: '123', email: 'root@example.com', timezone: 'UTC', language: 'en' },
        workspaces: [],
        check_for_updates: true,
        version: '44.0'
      })
      localStorageMock.setItem('auth_token', 'existing-token')
      render(<UpdateProbe />, { wrapper })
      await waitFor(() => expect(screen.getByTestId('check')).toHaveTextContent('true'))
      expect(screen.getByTestId('version')).toHaveTextContent('44.0')

      await act(async () => {
        screen.getByTestId('signout').click()
      })
      await waitFor(() => expect(screen.getByTestId('check')).toHaveTextContent('false'))
      expect(screen.getByTestId('version')).toHaveTextContent('none')
    })
  })

  it('throws error when useAuth is used outside AuthProvider', () => {
    // Suppress console.error for this test
    const originalConsoleError = console.error
    console.error = vi.fn()

    // Using useAuth outside provider should throw
    expect(() => {
      render(<TestComponent />)
    }).toThrow('useAuth must be used within an AuthProvider')

    // Restore console.error
    console.error = originalConsoleError
  })
})
