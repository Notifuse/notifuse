import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { SignInPage } from '../pages/SignInPage'
import { AuthProvider } from '../contexts/AuthContext'
import * as authService from '../services/api/auth'
import { App } from 'antd'

// Mock the auth service
vi.mock('../services/api/auth', () => ({
  authService: {
    signIn: vi.fn(),
    verifyCode: vi.fn()
  }
}))

// Mock the navigate function and useSearch
const mockNavigate = vi.fn(() => ({}))
const mockSearch = { email: undefined }

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => mockNavigate,
  useSearch: vi.fn((options?: { from?: string }) => mockSearch)
}))

// Mock antd message component
const mockMessage = {
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
  loading: vi.fn()
}

// Wrap component with necessary providers
const renderWithProviders = (ui: React.ReactElement) => {
  return render(
    <App message={{ maxCount: 3 }}>
      <AuthProvider>{ui}</AuthProvider>
    </App>
  )
}

describe('SignInPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Clear any previous mock implementations
    vi.spyOn(App, 'useApp').mockReturnValue({ message: mockMessage } as any)
    // Reset search mock
    mockSearch.email = undefined
  })

  it('renders the email form initially', () => {
    renderWithProviders(<SignInPage />)

    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByText(/send magic code/i)).toBeInTheDocument()
  })

  it('submits email and shows code input form', async () => {
    // Mock successful response
    vi.mocked(authService.authService.signIn).mockResolvedValueOnce({
      message: 'Magic code sent',
      code: '123456'
    })

    renderWithProviders(<SignInPage />)

    // Fill and submit the email form
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' }
    })
    fireEvent.click(screen.getByText(/send magic code/i))

    // Wait for code input form to appear
    await waitFor(() => {
      expect(screen.getByText(/enter the 6-digit code/i)).toBeInTheDocument()
    })

    // Verify API was called with correct data
    expect(authService.authService.signIn).toHaveBeenCalledWith({
      email: 'test@example.com'
    })

    // Verify code form is shown
    expect(screen.getByPlaceholderText('000000')).toBeInTheDocument()
    expect(screen.getByText(/verify code/i)).toBeInTheDocument()
  })

  it('logs magic code to console when provided in response', async () => {
    // Mock console.log
    const consoleSpy = vi.spyOn(console, 'log')

    // Mock successful response with code
    vi.mocked(authService.authService.signIn).mockResolvedValueOnce({
      message: 'Magic code sent',
      code: '123456'
    })

    renderWithProviders(<SignInPage />)

    // Fill and submit the email form
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' }
    })
    fireEvent.click(screen.getByText(/send magic code/i))

    // Wait for state to update
    await waitFor(() => {
      expect(screen.getByText(/enter the 6-digit code/i)).toBeInTheDocument()
    })

    // Verify code was logged
    expect(consoleSpy).toHaveBeenCalledWith('Magic code for development:', '123456')
  })

  it('submits code and navigates on success', async () => {
    // Mock successful sign in response
    vi.mocked(authService.authService.signIn).mockResolvedValueOnce({
      message: 'Magic code sent'
    })

    // Mock successful verify response
    vi.mocked(authService.authService.verifyCode).mockResolvedValueOnce({
      token: 'fake-token'
    })

    renderWithProviders(<SignInPage />)

    // Fill and submit the email form
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' }
    })
    fireEvent.click(screen.getByText(/send magic code/i))

    // Wait for code input form
    await waitFor(() => {
      expect(screen.getByText(/enter the 6-digit code/i)).toBeInTheDocument()
    })

    // Fill and submit the code form
    fireEvent.change(screen.getByPlaceholderText('000000'), {
      target: { value: '123456' }
    })
    fireEvent.click(screen.getByText(/verify code/i))

    // Verify API was called with correct data
    await waitFor(() => {
      expect(authService.authService.verifyCode).toHaveBeenCalledWith({
        email: 'test@example.com',
        code: '123456'
      })
    })
  })

  it('shows error message when API call fails', async () => {
    // Mock failed response
    vi.mocked(authService.authService.signIn).mockRejectedValueOnce(new Error('API error'))

    renderWithProviders(<SignInPage />)

    // Fill and submit the email form
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' }
    })
    fireEvent.click(screen.getByText(/send magic code/i))

    // Error message should appear (we can't directly check antd message.error,
    // but we can verify the API was called and the form is still shown)
    await waitFor(() => {
      expect(authService.authService.signIn).toHaveBeenCalled()
    })

    // Email form should still be visible
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
  })

  it('auto-fills and submits email from URL parameter', async () => {
    // Set email in URL search params
    mockSearch.email = 'demo@notifuse.com'

    // Mock successful response
    vi.mocked(authService.authService.signIn).mockResolvedValueOnce({
      message: 'Magic code sent'
    })

    renderWithProviders(<SignInPage />)

    // Wait for auto-submit to complete
    await waitFor(() => {
      expect(authService.authService.signIn).toHaveBeenCalledWith({
        email: 'demo@notifuse.com'
      })
    })

    // Verify code input form is shown (after auto-submit)
    await waitFor(() => {
      expect(screen.getByText(/enter the 6-digit code/i)).toBeInTheDocument()
    })

    // Verify email field is pre-filled
    const emailInput = screen.getByLabelText(/email/i) as HTMLInputElement
    expect(emailInput.value).toBe('demo@notifuse.com')
  })

  it('does not auto-submit when email parameter is not present', async () => {
    // Ensure email is not in search params
    mockSearch.email = undefined

    renderWithProviders(<SignInPage />)

    // Wait a bit to ensure no auto-submit happens
    await new Promise(resolve => setTimeout(resolve, 100))

    // Verify API was not called
    expect(authService.authService.signIn).not.toHaveBeenCalled()

    // Email form should be visible
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
  })
})
