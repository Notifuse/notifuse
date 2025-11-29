import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RootLayout } from '../layouts/RootLayout'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate, useMatch } from '@tanstack/react-router'

// Mock the auth context
vi.mock('../contexts/AuthContext', () => ({
  useAuth: vi.fn()
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

  it('redirects to workspace create when authenticated but has no workspaces', () => {
    // @ts-expect-error - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: true,
      loading: false,
      workspaces: []
    })

    // Mock all matches as false
    // @ts-expect-error - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(mockNavigate).toHaveBeenCalledWith({ to: '/console/workspace/create' })
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
