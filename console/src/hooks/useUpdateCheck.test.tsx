import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { UpdateCheckResponse } from '../services/updates'

const state = vi.hoisted(() => ({
  auth: { checkForUpdates: true, serverVersion: '42.0' as string | null, licenseEntitlements: null },
  locale: 'en',
  timezone: 'Europe/Paris',
  check: vi.fn()
}))

vi.mock('../contexts/AuthContext', () => ({ useAuth: () => state.auth }))
vi.mock('../services/updates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/updates')>()),
  checkForUpdates: state.check,
  browserLocale: () => state.locale,
  browserTimeZone: () => state.timezone
}))

import { useUpdateCheck, UPDATE_CHECK_CACHE_KEY, UPDATE_CHECK_IGNORED_KEY } from './useUpdateCheck'

// A consistent answer: one missing release, the latest, flagged as security when the answer is.
const answer = (over: Partial<UpdateCheckResponse> = {}): UpdateCheckResponse => {
  const latest = over.latest ?? '44.0'
  const security = over.security ?? false
  return {
    current: '42.0',
    latest,
    update_available: true,
    behind: 1,
    security,
    upgrade_url: 'https://docs.notifuse.com/self-hosting/installation',
    releases: [
      {
        version: latest,
        released_at: '2026-11-12',
        security,
        security_note: security ? 'Fix.' : '',
        summary: 'Summary.',
        highlights: [],
        before_upgrade: [],
        changelog_url: 'https://github.com/Notifuse/notifuse/blob/main/CHANGELOG.md'
      }
    ],
    ...over
  }
}

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
)

const cache = (over: { at?: number; version?: string; locale?: string; response?: UpdateCheckResponse }) =>
  localStorage.setItem(
    UPDATE_CHECK_CACHE_KEY,
    JSON.stringify({ at: Date.now(), version: '42.0', locale: 'en', response: answer(), ...over })
  )

describe('useUpdateCheck', () => {
  beforeEach(() => {
    localStorage.clear()
    state.auth = { checkForUpdates: true, serverVersion: '42.0', licenseEntitlements: null }
    state.locale = 'en'
    state.check.mockReset()
    state.check.mockResolvedValue(answer())
  })

  it('does not ask when user.me says this console should not', async () => {
    state.auth.checkForUpdates = false
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await new Promise((r) => setTimeout(r, 20))
    expect(state.check).not.toHaveBeenCalled()
    expect(result.current.data).toBeNull()
  })

  it('does not ask without a server version', async () => {
    state.auth.serverVersion = null
    renderHook(() => useUpdateCheck(), { wrapper })
    await new Promise((r) => setTimeout(r, 20))
    expect(state.check).not.toHaveBeenCalled()
  })

  it('asks with the server version, the locale, the time zone and the entitlements, then caches the answer', async () => {
    state.locale = 'fr'
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(result.current.available).toBe(true))
    expect(state.check).toHaveBeenCalledWith({
      version: '42.0',
      locale: 'fr',
      timezone: 'Europe/Paris',
      entitlements: null
    })
    const stored = JSON.parse(localStorage.getItem(UPDATE_CHECK_CACHE_KEY) as string)
    expect(stored).toMatchObject({ version: '42.0', locale: 'fr' })
  })

  it('uses a fresh cached answer for the same version and locale without asking', async () => {
    cache({})
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(result.current.available).toBe(true))
    expect(state.check).not.toHaveBeenCalled()
  })

  it('asks again when the cached answer is older than a day', async () => {
    cache({ at: Date.now() - 25 * 60 * 60 * 1000 })
    renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(state.check).toHaveBeenCalledTimes(1))
  })

  it('asks again after an upgrade, even with a fresh answer for the old version', async () => {
    cache({ version: '41.0' })
    renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(state.check).toHaveBeenCalledTimes(1))
  })

  it('asks again in another language', async () => {
    cache({ locale: 'fr' })
    renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(state.check).toHaveBeenCalledTimes(1))
  })

  it('ignoring the latest release dismisses it and remembers it', async () => {
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(result.current.available).toBe(true))
    expect(result.current.dismissed).toBe(false)
    act(() => result.current.ignore('44.0'))
    expect(result.current.dismissed).toBe(true)
    expect(localStorage.getItem(UPDATE_CHECK_IGNORED_KEY)).toBe('44.0')
  })

  it('a newer release than the ignored one is announced again', async () => {
    localStorage.setItem(UPDATE_CHECK_IGNORED_KEY, '44.0')
    state.check.mockResolvedValue(answer({ latest: '44.1', behind: 3 }))
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(result.current.available).toBe(true))
    expect(result.current.dismissed).toBe(false)
  })

  it('a missing security release is never dismissed', async () => {
    localStorage.setItem(UPDATE_CHECK_IGNORED_KEY, '44.0')
    state.check.mockResolvedValue(answer({ security: true }))
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(result.current.available).toBe(true))
    expect(result.current.dismissed).toBe(false)
  })

  it('a failed check is not retried for hours, then is', async () => {
    state.check.mockRejectedValue(new Error('offline'))
    const first = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(state.check).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(JSON.parse(localStorage.getItem(UPDATE_CHECK_CACHE_KEY) as string).response).toBeNull())
    first.unmount()

    renderHook(() => useUpdateCheck(), { wrapper })
    await new Promise((r) => setTimeout(r, 20))
    expect(state.check).toHaveBeenCalledTimes(1)

    cache({ at: Date.now() - 7 * 60 * 60 * 1000, response: null as unknown as UpdateCheckResponse })
    renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(state.check).toHaveBeenCalledTimes(2))
  })

  it('a kept answer in an unexpected shape is asked again instead of reaching the page', async () => {
    cache({ response: { latest: '44.0', update_available: true } as unknown as UpdateCheckResponse })
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(state.check).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(result.current.available).toBe(true))
  })

  it('a failed check shows nothing', async () => {
    state.check.mockRejectedValue(new Error('offline'))
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(state.check).toHaveBeenCalled())
    await new Promise((r) => setTimeout(r, 20))
    expect(result.current.data).toBeNull()
    expect(result.current.available).toBe(false)
  })

  it('still works when storage throws', async () => {
    const getItem = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied')
    })
    const { result } = renderHook(() => useUpdateCheck(), { wrapper })
    await waitFor(() => expect(result.current.available).toBe(true))
    act(() => result.current.ignore('44.0'))
    expect(result.current.dismissed).toBe(true)
    getItem.mockRestore()
    setItem.mockRestore()
  })
})
