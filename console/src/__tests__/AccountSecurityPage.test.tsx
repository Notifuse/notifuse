import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AccountSecurityPage } from '../pages/AccountSecurityPage'

const mockAuth = {
  user: { email: 'jane@example.com' },
  workspaces: [{ id: 'acme', name: 'Acme' }],
  twoFactorRequired: false,
  rootSessionWithoutCode: false,
  refreshWorkspaces: vi.fn(() => Promise.resolve())
}
vi.mock('../contexts/AuthContext', () => ({ useAuth: () => mockAuth }))

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

// The real panel pulls the API client, and through it the router and every page. This one
// reports the lock the way the real one does while new recovery codes are on screen.
let reportLock: ((locked: boolean) => void) | undefined
vi.mock('../components/two_factor/AccountSecurityPanel', () => ({
  AccountSecurityPanel: ({ onLockChange }: { onLockChange?: (locked: boolean) => void }) => {
    reportLock = onLockChange
    return <div data-testid="account-security-panel" />
  }
}))

vi.mock('../components/two_factor/RootSessionRefused', () => ({
  RootSessionRefused: () => <div data-testid="root-session-refused" />
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderPage = () =>
  render(
    <I18nProvider i18n={i18n}>
      <AccountSecurityPage />
    </I18nProvider>
  )

describe('AccountSecurityPage', () => {
  beforeEach(() => {
    reportLock = undefined
    mockAuth.twoFactorRequired = false
  })

  it('leads back to the first workspace', () => {
    renderPage()
    expect(screen.getByRole('button', { name: /Back to Acme/ })).toBeInTheDocument()
  })

  it('holds the ways out while new recovery codes are on screen and not marked as saved', () => {
    renderPage()
    act(() => reportLock?.(true))
    expect(screen.queryByRole('button', { name: /Back to Acme/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Logout/ })).not.toBeInTheDocument()

    act(() => reportLock?.(false))
    expect(screen.getByRole('button', { name: /Back to Acme/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Logout/ })).toBeInTheDocument()
  })

  it('asks a new member to turn it on, without saying "again"', () => {
    mockAuth.twoFactorRequired = true
    renderPage()
    expect(
      screen.getByText('Turn it on to open your workspaces. Until then, this page is the only one you can use.')
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Back to/ })).not.toBeInTheDocument()
  })
})
