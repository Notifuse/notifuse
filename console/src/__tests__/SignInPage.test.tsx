import { readFileSync } from 'node:fs'
import { join } from 'node:path'
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
    verifyCode: vi.fn(),
    oidcExchange: vi.fn(),
    getCurrentUser: vi.fn().mockRejectedValue(new Error('Not authenticated')),
    logout: vi.fn().mockResolvedValue(undefined)
  },
  isRootUser: vi.fn().mockReturnValue(false)
}))

// Mock the navigate function and useSearch
const mockNavigate = vi.fn(() => ({}))
const mockSearch: { email?: string; oidc_error?: string } = { email: undefined }

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useSearch: vi.fn(() => mockSearch)
  }
})

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
    vi.spyOn(App, 'useApp').mockReturnValue({
      message: { ...mockMessage, open: vi.fn(), destroy: vi.fn() },
      notification: {} as ReturnType<typeof App.useApp>['notification'],
      modal: {} as ReturnType<typeof App.useApp>['modal']
    } as ReturnType<typeof App.useApp>)
    // Reset search mock
    mockSearch.email = undefined
    mockSearch.oidc_error = undefined
    // Reset OIDC window globals + URL fragment between tests
    window.OIDC_ENABLED = false
    window.OIDC_BUTTON_LABEL = ''
    window.API_ENDPOINT = 'https://api.example.com'
    window.location.hash = ''
  })

  it('renders the email form initially', () => {
    renderWithProviders(<SignInPage />)

    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByText(/send magic code/i)).toBeInTheDocument()
  })

  it('submits email and shows code input form', async () => {
    // Mock successful response without code (normal flow)
    vi.mocked(authService.authService.signIn).mockResolvedValueOnce({
      message: 'Magic code sent'
      // No code property - normal flow
    })

    renderWithProviders(<SignInPage />)

    // Fill and submit the email form
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' }
    })
    fireEvent.click(screen.getByText(/send magic code/i))

    // Wait for code input form to appear
    await waitFor(() => {
      expect(screen.getByText(/enter the 6-digit code sent to/i)).toBeInTheDocument()
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

    // Mock successful response with code (auto-submits in dev mode)
    vi.mocked(authService.authService.signIn).mockResolvedValueOnce({
      message: 'Magic code sent',
      code: '123456'
    })

    // Mock verifyCode to prevent actual navigation
    vi.mocked(authService.authService.verifyCode).mockResolvedValueOnce({
      token: 'fake-token'
    })

    renderWithProviders(<SignInPage />)

    // Fill and submit the email form
    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' }
    })
    fireEvent.click(screen.getByText(/send magic code/i))

    // Wait for auto-submit to complete (code form should appear briefly, then auto-submit)
    await waitFor(
      () => {
        expect(consoleSpy).toHaveBeenCalledWith('Magic code for development:', '123456')
      },
      { timeout: 2000 }
    )

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
    (mockSearch as { email: string | undefined }).email = 'demo@notifuse.com'

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
      expect(screen.getByText(/enter the 6-digit code sent to/i)).toBeInTheDocument()
    })

    // Verify the email is shown in the code form message
    expect(screen.getByText(/demo@notifuse.com/i)).toBeInTheDocument()
  })

  it('does not auto-submit when email parameter is not present', async () => {
    // Ensure email is not in search params
    mockSearch.email = undefined

    renderWithProviders(<SignInPage />)

    // Wait a bit to ensure no auto-submit happens
    await new Promise((resolve) => setTimeout(resolve, 100))

    // Verify API was not called
    expect(authService.authService.signIn).not.toHaveBeenCalled()

    // Email form should be visible
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
  })

  describe('OIDC SSO', () => {
    it('renders the SSO button + divider when window.OIDC_ENABLED', () => {
      window.OIDC_ENABLED = true
      renderWithProviders(<SignInPage />)
      expect(screen.getByText(/sign in with sso/i)).toBeInTheDocument()
      expect(screen.getByText(/^or$/i)).toBeInTheDocument()
    })

    it('hides the SSO button when OIDC disabled', () => {
      window.OIDC_ENABLED = false
      renderWithProviders(<SignInPage />)
      expect(screen.queryByText(/sign in with sso/i)).not.toBeInTheDocument()
    })

    it('uses OIDC_BUTTON_LABEL as plain text when set', () => {
      window.OIDC_ENABLED = true
      window.OIDC_BUTTON_LABEL = 'Continue with Acme'
      renderWithProviders(<SignInPage />)
      expect(screen.getByText('Continue with Acme')).toBeInTheDocument()
    })

    it('SSO button click navigates to the API start endpoint', () => {
      window.OIDC_ENABLED = true
      const assignSpy = vi.fn()
      Object.defineProperty(window, 'location', {
        value: { ...window.location, assign: assignSpy, hash: '' },
        writable: true
      })
      renderWithProviders(<SignInPage />)
      fireEvent.click(screen.getByText(/sign in with sso/i))
      expect(assignSpy).toHaveBeenCalledWith('https://api.example.com/api/user.oidc.start')
    })

    it('on #oidc_code=<code> exchanges, signs in, strips the fragment, and navigates', async () => {
      const replaceStateSpy = vi.spyOn(window.history, 'replaceState')
      Object.defineProperty(window, 'location', {
        value: { ...window.location, hash: '#oidc_code=one-time-xyz', pathname: '/console/signin', search: '' },
        writable: true
      })
      vi.mocked(authService.authService.oidcExchange).mockResolvedValueOnce({ token: 'jwt-from-oidc' })
      // signin(token) calls getCurrentUser; resolve it so the success path navigates.
      vi.mocked(authService.authService.getCurrentUser).mockResolvedValueOnce({
        user: { id: 'u1', email: 'u1@corp.com', timezone: 'UTC', language: 'en' },
        workspaces: []
      })

      renderWithProviders(<SignInPage />)

      await waitFor(() => {
        expect(authService.authService.oidcExchange).toHaveBeenCalledWith('one-time-xyz')
      })
      // Fragment stripped before the exchange resolves.
      expect(replaceStateSpy).toHaveBeenCalled()
      await waitFor(
        () => {
          expect(mockNavigate).toHaveBeenCalledWith({ to: '/console' })
        },
        { timeout: 1000 }
      )
    })

    it('runs the exchange exactly once even if re-rendered', async () => {
      Object.defineProperty(window, 'location', {
        value: { ...window.location, hash: '#oidc_code=abc', pathname: '/console/signin', search: '' },
        writable: true
      })
      vi.mocked(authService.authService.oidcExchange).mockResolvedValue({ token: 'jwt' })

      const { rerender } = renderWithProviders(<SignInPage />)
      rerender(
        <App message={{ maxCount: 3 }}>
          <AuthProvider>
            <SignInPage />
          </AuthProvider>
        </App>
      )

      await waitFor(() => {
        expect(authService.authService.oidcExchange).toHaveBeenCalledTimes(1)
      })
    })

    it('on ?oidc_error shows a message and does not call oidcExchange', async () => {
      mockSearch.oidc_error = 'not_provisioned'
      renderWithProviders(<SignInPage />)
      await waitFor(() => {
        expect(mockMessage.error).toHaveBeenCalled()
      })
      expect(authService.authService.oidcExchange).not.toHaveBeenCalled()
    })

    // Asserting only that message.error fired lets an EMPTY toast pass, which is
    // exactly what a rejected user would see if the localization stopped resolving.
    it('renders actual text in the error toast, not an empty string', async () => {
      mockSearch.oidc_error = 'not_provisioned'
      renderWithProviders(<SignInPage />)
      await waitFor(() => {
        expect(mockMessage.error).toHaveBeenCalled()
      })
      const shown = mockMessage.error.mock.calls[0][0]
      expect(typeof shown).toBe('string')
      expect(shown).not.toBe('')
      expect(shown).toMatch(/account/i)
    })

    // The empty toast the test above guards against is exactly what shipped, and that
    // test could not see it: the suite's macro mock resolves t`…` through i18n._ per
    // template string, whatever scope it is written in. The real macro rewrites only what
    // it can resolve to the useLingui() binding, and what it leaves alone reaches i18n._
    // as a tagged template, which answers "". Extraction is where that becomes visible.
    it('extracts every OIDC error message into the source catalog', () => {
      const catalog = readFileSync(join(import.meta.dirname, '../i18n/locales/en.po'), 'utf8')
      const messages = [
        'No Notifuse account is linked to that identity. Ask an administrator to invite you first.',
        'Your identity provider has not verified your email address.',
        'This account is already linked to a different single sign-on identity.',
        'Single sign-on is temporarily unavailable. Please try a magic code.',
        'Too many sign-in attempts. Please wait a moment and try again.',
        'Single sign-on failed. Please try again or use a magic code.'
      ]
      // Report the missing ones rather than asserting per message: a failing toContain
      // against the catalog dumps the whole half-megabyte file as the received value.
      expect(messages.filter((m) => !catalog.includes(`msgid "${m}"`))).toEqual([])
    })

    it('on oidcExchange rejection shows an error toast and does not navigate to /console', async () => {
      Object.defineProperty(window, 'location', {
        value: { ...window.location, hash: '#oidc_code=bad', pathname: '/console/signin', search: '' },
        writable: true
      })
      vi.mocked(authService.authService.oidcExchange).mockRejectedValueOnce(new Error('boom'))

      renderWithProviders(<SignInPage />)
      await waitFor(() => {
        expect(mockMessage.error).toHaveBeenCalled()
      })
      expect(mockNavigate).not.toHaveBeenCalledWith({ to: '/console' })
    })
  })
})
