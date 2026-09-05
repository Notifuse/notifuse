import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'

// The router module builds the whole route graph at import time, and RouterProvider would
// mount it. Neither is the subject: what is being asserted is the provider stack App wraps
// around whatever the router renders, so the router is replaced by a probe.
const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))
vi.mock('../router', () => ({ router: { navigate } }))

vi.mock('@tanstack/react-router', async () => {
  const actual =
    await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  const { LicenceProbe } = await import('./licenseWiringProbe')
  return { ...actual, RouterProvider: () => <LicenceProbe /> }
})

import { App } from '../App'

/**
 * The composition test the console did not have.
 *
 * Every leaf is well covered — LicenseContext, useLicense, licenseErrors, LicenseBanner,
 * SsoLicenceNotice all catch their own mutations. What nothing covered is the wiring between
 * them: delete `<LicenseProvider>` from App.tsx and useLicense quietly falls back to the
 * unknown state, has() answers true for everything, `tsc -b` exits 0 and every one of the
 * other 1400 tests still passes. That is the Round-1 and Round-2 failure class — a gate dead
 * because nobody wired it, with a green build — relocated into the console.
 *
 * It renders the real App, so it pins the whole chain in one assertion: /api/user.me answers
 * entitlements, AuthProvider adopts them, LicenseProvider is mounted inside it and publishes
 * them, and useLicense reads them.
 */
describe('the console licence wiring', () => {
  beforeEach(() => {
    navigate.mockClear()
    localStorage.clear()
    localStorage.setItem('auth_token', 'valid-token')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const userMe = (features: string[]) => ({
    ok: true,
    status: 200,
    json: async () => ({
      user: { id: 'u1', email: 'someone@example.com' },
      workspaces: [],
      entitlements: {
        tier: 'agency',
        org: 'ACME SAS',
        sub: 'billing@acme.com',
        max_workspaces: 15,
        features,
        state: 'active',
        expires_at: '2027-01-01T00:00:00Z'
      }
    })
  })

  it('carries the grant from /api/user.me all the way to useLicense', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(userMe(['rbac', 'ses_tenant'])))

    render(<App />)

    await waitFor(() => expect(screen.getByTestId('tier')).toHaveTextContent('agency'))
    expect(screen.getByTestId('has-rbac')).toHaveTextContent('true')
  })

  // The half that actually restricts. A console that answered `true` here for a deployment
  // that never bought SSO would grey nothing out and offer no purchase — which is exactly
  // what an unmounted provider does, silently, because unknown deliberately means "no
  // restrictions".
  it('reports a capability the deployment did not buy as absent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(userMe(['rbac', 'ses_tenant'])))

    render(<App />)

    await waitFor(() => expect(screen.getByTestId('tier')).toHaveTextContent('agency'))
    expect(screen.getByTestId('has-sso')).toHaveTextContent('false')
  })
})
