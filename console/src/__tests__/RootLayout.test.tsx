import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RootLayout } from '../layouts/RootLayout'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate, useMatch } from '@tanstack/react-router'

// Mock the auth context
vi.mock('../contexts/AuthContext', () => ({
  useAuth: vi.fn()
}))

// Root is whoever ROOT_EMAIL names; the real matcher is covered in services/api/auth.test.ts.
vi.mock('../services/api/auth', () => ({
  isRootUser: (email?: string) => email === 'root@example.com'
}))

// Mock the react router
vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet">Outlet content</div>,
  useNavigate: vi.fn(),
  useMatch: vi.fn()
}))

describe('RootLayout', () => {
  const mockNavigate = vi.fn()
  const originalLocation = window.location
  const originalIsInstalled = (window as unknown as { IS_INSTALLED?: boolean }).IS_INSTALLED

  beforeEach(() => {
    vi.clearAllMocks()
    // @ts-expect-error - we're mocking the return value
    useNavigate.mockReturnValue(mockNavigate)

    // Mock window.IS_INSTALLED to prevent setup redirect
    ;(window as unknown as { IS_INSTALLED?: boolean }).IS_INSTALLED = true

    // Mock window.location
    delete (window as unknown as { location?: Location }).location
    ;(window as unknown as { location: Location }).location = {
      ...originalLocation,
      pathname: '/console/',
      search: '',
      href: 'http://localhost:3000/console/'
    } as Location
  })

  afterEach(() => {
    ;(window as unknown as { location: Location }).location = originalLocation
    ;(window as unknown as { IS_INSTALLED?: boolean }).IS_INSTALLED = originalIsInstalled
  })

  it('shows loading state when auth is loading', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: false,
      loading: true,
      workspaces: []
    })

    // Mock all matches as false
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    const { container } = render(<RootLayout />)
    // Check for the ant-spin class on any element in the container
    expect(container.querySelector('.ant-spin')).toBeInTheDocument()
  })

  it('redirects to signin when not authenticated and not on public route', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: false,
      loading: false,
      workspaces: []
    })

    // Mock all matches as false
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/signin',
      search: undefined,
      replace: true
    })
  })

  it('preserves email parameter when redirecting to signin', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: false,
      loading: false,
      workspaces: []
    })

    // Mock all matches as false
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    // Set up window.location with email parameter
    ;(window as unknown as { location: Location }).location = {
      ...originalLocation,
      pathname: '/console/',
      search: '?email=demo@notifuse.com',
      href: 'http://localhost:3000/console/?email=demo@notifuse.com'
    } as Location

    render(<RootLayout />)
    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/signin',
      search: { email: 'demo@notifuse.com' },
      replace: true
    })
  })

  it('does not navigate when already on signin route with email parameter', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: false,
      loading: false,
      workspaces: []
    })

    // Mock all matches as false (simulating race condition)
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    // Set up window.location to be on signin route
    ;(window as unknown as { location: Location }).location = {
      ...originalLocation,
      pathname: '/console/signin',
      search: '?email=demo@notifuse.com',
      href: 'http://localhost:3000/console/signin?email=demo@notifuse.com'
    } as Location

    render(<RootLayout />)
    // Should not navigate since we're already on signin route
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('does not navigate when already on signin route without email parameter', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: false,
      loading: false,
      workspaces: []
    })

    // Mock all matches as false (simulating race condition)
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    // Set up window.location to be on signin route
    ;(window as unknown as { location: Location }).location = {
      ...originalLocation,
      pathname: '/console/signin',
      search: '',
      href: 'http://localhost:3000/console/signin'
    } as Location

    render(<RootLayout />)
    // Should not navigate since we're already on signin route
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('redirects root to workspace create when it has no workspaces', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: true,
      loading: false,
      workspaces: [],
      user: { email: 'root@example.com' }
    })

    // Mock all matches as false
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(mockNavigate).toHaveBeenCalledWith({ to: '/console/workspace/create' })
  })

  // Only root can create a workspace. Anyone else with none — an invitee who signed in with
  // SSO before joining, typically — stays on the dashboard, where their pending invitations are.
  it('keeps a non-root user with no workspaces on the dashboard', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: true,
      loading: false,
      workspaces: [],
      user: { email: 'invitee@example.com' }
    })

    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  it('renders outlet when authenticated and has workspaces', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: true,
      loading: false,
      workspaces: [{ id: '1', name: 'Test Workspace' }]
    })

    // Mock all matches as false
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })
})

// The instance requires two-factor authentication and the session has not passed it: only the
// account security page works.
describe('RootLayout two-factor requirement', () => {
  const navigate = vi.fn()
  const originalIsInstalled = (window as unknown as { IS_INSTALLED?: boolean }).IS_INSTALLED
  const onRoute = (route: string) => {
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(({ from }: { from: string }) => from === route)
  }

  beforeEach(() => {
    vi.clearAllMocks()
    // @ts-expect-error - we're mocking the return value
    useNavigate.mockReturnValue(navigate)
    ;(window as unknown as { IS_INSTALLED?: boolean }).IS_INSTALLED = true
  })
  afterEach(() => {
    ;(window as unknown as { IS_INSTALLED?: boolean }).IS_INSTALLED = originalIsInstalled
  })

  it('sends a session that has not passed it to the account security page', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({ isAuthenticated: true, loading: false, workspaces: [], user: { email: 'ann@example.com' }, twoFactorRequired: true })
    onRoute('/console/workspace/$workspaceId')
    render(<RootLayout />)
    expect(navigate).toHaveBeenCalledWith({ to: '/console/account/security', replace: true })
  })

  // A root user with no workspace is normally sent to create one: not while the requirement
  // holds, and never away from the security page.
  it('leaves a root user without a workspace on the security page', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({ isAuthenticated: true, loading: false, workspaces: [], user: { email: 'root@example.com' }, twoFactorRequired: false })
    onRoute('/console/account/security')
    render(<RootLayout />)
    expect(navigate).not.toHaveBeenCalled()
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  it('lets the session sign out', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({ isAuthenticated: true, loading: false, workspaces: [], user: { email: 'ann@example.com' }, twoFactorRequired: true })
    onRoute('/console/logout')
    render(<RootLayout />)
    expect(navigate).not.toHaveBeenCalled()
  })

  // The MCP connection page needs no session: the API key pasted there is the proof.
  it('leaves a visitor without a session on the MCP connection page', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({ isAuthenticated: false, loading: false, workspaces: [], user: undefined, twoFactorRequired: false })
    onRoute('/console/oauth/authorize')
    render(<RootLayout />)
    expect(navigate).not.toHaveBeenCalled()
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })

  // A root user with no workspace, or a session short of the second factor, stays there too.
  it.each([
    ['a root user without a workspace', { email: 'root@example.com' }, false],
    ['a session short of the second factor', { email: 'ann@example.com' }, true]
  ])('leaves %s on the MCP connection page', (_label, user, twoFactorRequired) => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({ isAuthenticated: true, loading: false, workspaces: [], user, twoFactorRequired })
    onRoute('/console/oauth/authorize')
    render(<RootLayout />)
    expect(navigate).not.toHaveBeenCalled()
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })
})
