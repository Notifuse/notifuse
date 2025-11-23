import { describe, it, expect, vi, beforeEach } from 'vitest'
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

  beforeEach(() => {
    vi.clearAllMocks()
    // @ts-ignore - we're mocking the return value
    useNavigate.mockReturnValue(mockNavigate)
  })

  it('shows loading state when auth is loading', () => {
    // @ts-ignore - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: false,
      loading: true,
      workspaces: []
    })

    // Mock all matches as false
    // @ts-ignore - we're mocking the return value
    useMatch.mockImplementation(() => false)

    const { container } = render(<RootLayout />)
    // Check for the ant-spin class on any element in the container
    expect(container.querySelector('.ant-spin')).toBeInTheDocument()
  })

  it('redirects to signin when not authenticated and not on public route', () => {
    // @ts-ignore - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: false,
      loading: false,
      workspaces: []
    })

    // Mock all matches as false
    // @ts-ignore - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(mockNavigate).toHaveBeenCalledWith({ to: '/console/signin' })
  })

  it('redirects to workspace create when authenticated but has no workspaces', () => {
    // @ts-ignore - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: true,
      loading: false,
      workspaces: []
    })

    // Mock all matches as false
    // @ts-ignore - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(mockNavigate).toHaveBeenCalledWith({ to: '/console/workspace/create' })
  })

  it('renders outlet when authenticated and has workspaces', () => {
    // @ts-ignore - we're mocking the return value
    useAuth.mockReturnValue({
      isAuthenticated: true,
      loading: false,
      workspaces: [{ id: '1', name: 'Test Workspace' }]
    })

    // Mock all matches as false
    // @ts-ignore - we're mocking the return value
    useMatch.mockImplementation(() => false)

    render(<RootLayout />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })
})
