import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useSESDiscovery } from './useSESDiscovery'
import * as sesApi from '../../services/api/ses'

vi.mock('../../services/api/ses', async () => {
  const actual = await vi.importActual<typeof sesApi>('../../services/api/ses')
  return {
    ...actual,
    listSESTenants: vi.fn(),
    listSESConfigurationSets: vi.fn()
  }
})

const listSESTenants = vi.mocked(sesApi.listSESTenants)
const listSESConfigurationSets = vi.mocked(sesApi.listSESConfigurationSets)

function resolveWith(tenants: string[], sets: string[]) {
  listSESTenants.mockResolvedValue({
    tenants: tenants.map((name) => ({ name })),
    has_more: false
  })
  listSESConfigurationSets.mockResolvedValue({ configuration_sets: sets, has_more: false })
}

describe('useSESDiscovery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resolveWith([], [])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not call AWS while the SES form is not on screen', async () => {
    renderHook(() =>
      useSESDiscovery({ active: false, workspaceId: 'ws', integrationId: 'int-1', debounceMs: 0 })
    )

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(listSESTenants).not.toHaveBeenCalled()
  })

  it('offers the names from a saved integration', async () => {
    resolveWith(['notifuse-int-1', 'team-acme'], ['notifuse-int-1'])

    const { result } = renderHook(() =>
      useSESDiscovery({ active: true, workspaceId: 'ws', integrationId: 'int-1', debounceMs: 0 })
    )

    await waitFor(() => {
      expect(result.current.tenantOptions).toEqual([
        { value: 'notifuse-int-1' },
        { value: 'team-acme' }
      ])
    })
    expect(result.current.configurationSetOptions).toEqual([{ value: 'notifuse-int-1' }])
    expect(listSESTenants).toHaveBeenCalledWith({
      workspace_id: 'ws',
      integration_id: 'int-1'
    })
  })

  // The create drawer starts empty: credentials arrive after the form is already open. Reading
  // them once on mount would leave the pickers permanently empty for every new integration.
  it('populates once credentials are typed into a new integration', async () => {
    resolveWith(['team-acme'], [])

    const { result, rerender } = renderHook((props: Record<string, unknown>) =>
      useSESDiscovery({
        active: true,
        workspaceId: 'ws',
        integrationId: null,
        debounceMs: 0,
        ...props
      })
    )

    // Nothing to ask with yet.
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(listSESTenants).not.toHaveBeenCalled()
    expect(result.current.tenantOptions).toEqual([])

    rerender({ region: 'eu-west-3', accessKey: 'AKIA', secretKey: 'secret' })

    await waitFor(() => {
      expect(result.current.tenantOptions).toEqual([{ value: 'team-acme' }])
    })
    expect(listSESTenants).toHaveBeenCalledWith({
      workspace_id: 'ws',
      region: 'eu-west-3',
      access_key: 'AKIA',
      secret_key: 'secret'
    })
  })

  it('waits for a complete credential triple', async () => {
    const { rerender } = renderHook((props: Record<string, unknown>) =>
      useSESDiscovery({
        active: true,
        workspaceId: 'ws',
        integrationId: null,
        debounceMs: 0,
        ...props
      })
    )

    rerender({ region: 'eu-west-3' })
    rerender({ region: 'eu-west-3', accessKey: 'AKIA' })

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(listSESTenants).not.toHaveBeenCalled()
  })

  // Offering integration A's tenants while integration B is open would let an operator pick a
  // tenant that does not exist in B's AWS account — after which every send from B fails.
  it('never offers the previous integration\'s names', async () => {
    resolveWith(['tenant-from-A'], ['set-from-A'])

    const { result, rerender } = renderHook((props: { integrationId: string }) =>
      useSESDiscovery({
        active: true,
        workspaceId: 'ws',
        debounceMs: 0,
        ...props
      })
    )
    rerender({ integrationId: 'int-A' })

    await waitFor(() => {
      expect(result.current.tenantOptions).toEqual([{ value: 'tenant-from-A' }])
    })

    // Switching to another integration whose fetch never resolves must not leave A's names up.
    listSESTenants.mockImplementation(() => new Promise(() => {}))
    listSESConfigurationSets.mockImplementation(() => new Promise(() => {}))
    rerender({ integrationId: 'int-B' })

    await waitFor(() => {
      expect(result.current.tenantOptions).toEqual([])
      expect(result.current.configurationSetOptions).toEqual([])
    })
  })

  it('reports a denial as a degraded state, not a failure', async () => {
    listSESTenants.mockRejectedValue(new sesApi.SESAccessDeniedError('needs ses:ListTenants'))
    listSESConfigurationSets.mockResolvedValue({ configuration_sets: [], has_more: false })

    const { result } = renderHook(() =>
      useSESDiscovery({ active: true, workspaceId: 'ws', integrationId: 'int-1', debounceMs: 0 })
    )

    await waitFor(() => {
      expect(result.current.denied).toBe(true)
    })
    expect(result.current.tenantOptions).toEqual([])
  })

  it('clears the denial hint when the target changes', async () => {
    listSESTenants.mockRejectedValue(new sesApi.SESAccessDeniedError('nope'))
    listSESConfigurationSets.mockResolvedValue({ configuration_sets: [], has_more: false })

    const { result, rerender } = renderHook((props: { integrationId: string }) =>
      useSESDiscovery({ active: true, workspaceId: 'ws', debounceMs: 0, ...props })
    )
    rerender({ integrationId: 'int-A' })
    await waitFor(() => expect(result.current.denied).toBe(true))

    // A different integration may have perfectly good permissions; a stale hint would lie.
    listSESTenants.mockImplementation(() => new Promise(() => {}))
    rerender({ integrationId: 'int-B' })

    await waitFor(() => expect(result.current.denied).toBe(false))
  })

  it('coalesces keystrokes into a single AWS call', async () => {
    vi.useFakeTimers()
    resolveWith(['team-acme'], [])

    const { rerender } = renderHook((props: { secretKey: string }) =>
      useSESDiscovery({
        active: true,
        workspaceId: 'ws',
        integrationId: null,
        region: 'eu-west-3',
        accessKey: 'AKIA',
        debounceMs: 500,
        ...props
      })
    )

    // Typing a secret key one character at a time must not fire a request per character.
    rerender({ secretKey: 's' })
    rerender({ secretKey: 'se' })
    rerender({ secretKey: 'sec' })
    rerender({ secretKey: 'secret' })

    act(() => {
      vi.advanceTimersByTime(499)
    })
    expect(listSESTenants).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(1)
      await vi.runAllTimersAsync()
    })
    expect(listSESTenants).toHaveBeenCalledTimes(1)
    expect(listSESTenants).toHaveBeenCalledWith(
      expect.objectContaining({ secret_key: 'secret' })
    )

    vi.useRealTimers()
  })

  it('ignores a response that arrives after the target changed', async () => {
    // A slow fetch for integration A must not overwrite integration B's state.
    let resolveA: (value: { tenants: { name: string }[]; has_more: boolean }) => void = () => {}
    listSESTenants.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveA = resolve
        })
    )
    listSESConfigurationSets.mockResolvedValue({ configuration_sets: [], has_more: false })

    const { result, rerender } = renderHook((props: { integrationId: string }) =>
      useSESDiscovery({ active: true, workspaceId: 'ws', debounceMs: 0, ...props })
    )
    rerender({ integrationId: 'int-A' })

    listSESTenants.mockImplementation(() => new Promise(() => {}))
    rerender({ integrationId: 'int-B' })

    await act(async () => {
      resolveA({ tenants: [{ name: 'tenant-from-A' }], has_more: false })
      await Promise.resolve()
    })

    expect(result.current.tenantOptions).toEqual([])
  })
})
