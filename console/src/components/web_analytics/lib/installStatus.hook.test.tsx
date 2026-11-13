import { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AnalyticsQuery, AnalyticsResponse } from '../../../services/api/analytics'
import { useInstallStatus } from './installStatus'

/**
 * Exercises the hook against a stub engine rather than mocking it away: the
 * shape of the two probes, and the decision to run the second one at all, is
 * the part that has to be right.
 */
const { queryMock } = vi.hoisted(() => ({ queryMock: vi.fn() }))

vi.mock('../../../services/api/analytics', () => ({
  AnalyticsService: { create: () => ({ query: queryMock }) }
}))

interface TestContext {
  workspaceId: string
  workspace?: { id: string; created_at?: string }
  settings?: { enabled: boolean }
  timezone: string
}

const context: { current: TestContext } = { current: { workspaceId: 'ws1', timezone: 'UTC' } }

vi.mock('../useWebAnalytics', () => ({
  useWebAnalytics: () => context.current
}))

const sessions = (count: number): AnalyticsResponse => ({
  data: [{ sessions: count }],
  meta: { total: 1, query: '', params: [] }
})

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } }
  })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

const withSettings = (enabled: boolean): TestContext => ({
  workspaceId: 'ws1',
  workspace: { id: 'ws1', created_at: '2026-01-01T00:00:00Z' },
  settings: { enabled },
  timezone: 'UTC'
})

const issuedQueries = (): AnalyticsQuery[] => queryMock.mock.calls.map((call) => call[0])

describe('useInstallStatus', () => {
  beforeEach(() => {
    queryMock.mockReset()
    context.current = withSettings(true)
  })

  it('passes through, and skips the lifetime probe, when the last day has traffic', async () => {
    queryMock.mockResolvedValue(sessions(12))

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    await waitFor(() => expect(result.current).toBe('ok'))
    expect(queryMock).toHaveBeenCalledTimes(1)
  })

  it('measures the last 24 hours on last activity, ignoring the report filters', async () => {
    queryMock.mockResolvedValue(sessions(1))

    renderHook(() => useInstallStatus(), { wrapper })

    await waitFor(() => expect(queryMock).toHaveBeenCalled())
    const [probe] = issuedQueries()
    expect(probe.schema).toBe('web_sessions')
    expect(probe.measures).toEqual(['sessions'])
    expect(probe.dimensions).toEqual([])
    expect(probe.filters).toHaveLength(1)

    const [range] = probe.filters ?? []
    expect(range.member).toBe('updated_at')
    expect(range.operator).toBe('inDateRange')
    const [start, end] = range.values.map((value) => new Date(value).getTime())
    expect(Math.round((end - start) / 3_600_000)).toBe(25)
  })

  it('reports a quiet day on a workspace that has history', async () => {
    queryMock.mockResolvedValueOnce(sessions(0)).mockResolvedValueOnce(sessions(500))

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    await waitFor(() => expect(result.current).toBe('stalled'))
    expect(queryMock).toHaveBeenCalledTimes(2)

    // The lifetime probe is bounded by the workspace, not left open-ended.
    const [, lifetime] = issuedQueries()
    const [range] = lifetime.filters ?? []
    expect(new Date(range.values[0]).toISOString()).toBe('2026-01-01T00:00:00.000Z')
  })

  it('asks for an install when nothing was ever recorded', async () => {
    queryMock.mockResolvedValue(sessions(0))

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    await waitFor(() => expect(result.current).toBe('never_received'))
  })

  it('leaves the page visible when a probe fails', async () => {
    queryMock.mockRejectedValue(new Error('engine unavailable'))

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    await waitFor(() => expect(result.current).toBe('ok'))
  })

  it('reports an unconfigured workspace without querying anything', async () => {
    context.current = { workspaceId: 'ws1', workspace: { id: 'ws1' }, timezone: 'UTC' }

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    expect(result.current).toBe('not_configured')
    await waitFor(() => expect(queryMock).not.toHaveBeenCalled())
  })

  it('reports a switched-off workspace without querying anything', async () => {
    context.current = withSettings(false)

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    expect(result.current).toBe('disabled')
    await waitFor(() => expect(queryMock).not.toHaveBeenCalled())
  })

  it('lets a config screen through a switched-off workspace', async () => {
    // The Filters tab: attribution rules are written before collection starts.
    context.current = withSettings(false)

    const { result } = renderHook(() => useInstallStatus({ checkTraffic: false }), { wrapper })

    expect(result.current).toBe('ok')
    await waitFor(() => expect(queryMock).not.toHaveBeenCalled())
  })

  it('never gates the demo workspace on traffic', async () => {
    // Demo history is generated once per reset and never beats again, so both
    // probes read as a dead install a day later — on a site nobody can install.
    context.current = {
      ...withSettings(true),
      workspaceId: 'demo',
      workspace: { id: 'demo', created_at: '2026-01-01T00:00:00Z' }
    }

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    expect(result.current).toBe('ok')
    await waitFor(() => expect(queryMock).not.toHaveBeenCalled())
  })

  it('still tells the demo workspace when web analytics was never set up', async () => {
    context.current = { workspaceId: 'demo', workspace: { id: 'demo' }, timezone: 'UTC' }

    const { result } = renderHook(() => useInstallStatus(), { wrapper })

    expect(result.current).toBe('not_configured')
  })

  it('still gates a config screen on a workspace with no settings', async () => {
    context.current = { workspaceId: 'ws1', workspace: { id: 'ws1' }, timezone: 'UTC' }

    const { result } = renderHook(() => useInstallStatus({ checkTraffic: false }), { wrapper })

    expect(result.current).toBe('not_configured')
  })
})
