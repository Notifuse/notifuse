import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { LicenseProvider } from './LicenseContext'
import { useLicense } from '../hooks/useLicense'
import type { Entitlements } from '../types/license'

const { getLicense, isRoot, authState } = vi.hoisted(() => ({
  getLicense: vi.fn(),
  isRoot: vi.fn(),
  authState: {
    value: {
      isAuthenticated: true,
      user: { id: 'u1', email: 'someone@example.com' },
      workspaces: [],
      licenseEntitlements: null as Entitlements | null
    }
  }
}))

vi.mock('../services/api/license', () => ({
  licenseApi: { get: getLicense, set: vi.fn() }
}))

vi.mock('../services/api/auth', () => ({
  isRootUser: isRoot
}))

vi.mock('./AuthContext', () => ({
  useAuth: () => authState.value
}))

const entitlements = (overrides: Partial<Entitlements> = {}): Entitlements => ({
  tier: 'studio',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 5,
  features: ['rbac'],
  state: 'active',
  expires_at: '2027-01-01T00:00:00Z',
  ...overrides
})

function Probe() {
  const { entitlements: resolved, has } = useLicense()
  return (
    <div>
      <span data-testid="tier">{resolved?.tier ?? 'unknown'}</span>
      <span data-testid="sso">{String(has('sso'))}</span>
    </div>
  )
}

describe('useLicense outside a provider', () => {
  // Not throwing, unlike useAuth. A licence subsystem that failed to mount must not be able to
  // take out a console whose licence is perfectly fine — and every page under test renders
  // without this provider.
  it('reports an unknown, unrestricted state', () => {
    render(<Probe />)

    expect(screen.getByTestId('tier')).toHaveTextContent('unknown')
    expect(screen.getByTestId('sso')).toHaveTextContent('true')
  })
})

describe('LicenseProvider', () => {
  beforeEach(() => {
    getLicense.mockReset()
    isRoot.mockReset().mockReturnValue(false)
    authState.value = {
      isAuthenticated: true,
      user: { id: 'u1', email: 'someone@example.com' },
      workspaces: [],
      licenseEntitlements: null
    }
  })

  const renderProvider = () =>
    render(
      <LicenseProvider>
        <Probe />
      </LicenseProvider>
    )

  it('prefers what /api/user.me already said, without a second round trip', async () => {
    authState.value.licenseEntitlements = entitlements({ tier: 'agency' })
    isRoot.mockReturnValue(true)

    renderProvider()

    await waitFor(() => expect(screen.getByTestId('tier')).toHaveTextContent('agency'))
    expect(getLicense).not.toHaveBeenCalled()
  })

  it('falls back to the root-only endpoint for a root user', async () => {
    isRoot.mockReturnValue(true)
    getLicense.mockResolvedValue({ entitlements: entitlements({ tier: 'enterprise' }) })

    renderProvider()

    await waitFor(() => expect(screen.getByTestId('tier')).toHaveTextContent('enterprise'))
  })

  // /api/licence.get answers 403 to anyone but root. Calling it anyway would put a denial in the
  // log of every console load, for every member, and learn nothing.
  it('does not call the root-only endpoint for anyone else', async () => {
    renderProvider()

    await waitFor(() => expect(screen.getByTestId('tier')).toHaveTextContent('unknown'))
    expect(getLicense).not.toHaveBeenCalled()
  })

  // A signed-out console must not keep reporting the deployment the previous session was on.
  it('forgets the licence when the session ends', async () => {
    authState.value.licenseEntitlements = entitlements({ tier: 'agency' })
    const { rerender } = renderProvider()

    await waitFor(() => expect(screen.getByTestId('tier')).toHaveTextContent('agency'))

    authState.value = { ...authState.value, isAuthenticated: false }
    rerender(
      <LicenseProvider>
        <Probe />
      </LicenseProvider>
    )

    await waitFor(() => expect(screen.getByTestId('tier')).toHaveTextContent('unknown'))
  })

  // Fail safe: a licence read that fails leaves the console exactly as usable as it was.
  it('restricts nothing when the licence read fails', async () => {
    isRoot.mockReturnValue(true)
    getLicense.mockRejectedValue(new Error('network down'))

    renderProvider()

    await waitFor(() => expect(getLicense).toHaveBeenCalled())
    expect(screen.getByTestId('tier')).toHaveTextContent('unknown')
    expect(screen.getByTestId('sso')).toHaveTextContent('true')
  })
})
