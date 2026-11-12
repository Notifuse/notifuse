import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useNodeStats, type UseNodeStatsOptions } from './useNodeStats'
import { automationApi } from '../../services/api/automation'
import { last30Days } from './last30Days'

vi.mock('../../services/api/automation', () => ({
  automationApi: { nodeStats: vi.fn().mockResolvedValue({ automation_id: 'a1', nodes: {} }) }
}))

const base: UseNodeStatsOptions = {
  workspaceId: 'ws',
  automationId: 'a1',
  timezone: 'Europe/Paris',
  period: 'last30',
  available: true,
  watched: true,
  live: false
}

function run(options: Partial<UseNodeStatsOptions>) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const hook = renderHook(() => useNodeStats({ ...base, ...options }), { wrapper })
  return { ...hook, client }
}

describe('useNodeStats', () => {
  beforeEach(() => vi.mocked(automationApi.nodeStats).mockClear())

  it('asks the last 30 days with the bounds of resolveRange', async () => {
    run({})
    const range = last30Days('Europe/Paris')
    await waitFor(() => expect(automationApi.nodeStats).toHaveBeenCalled())
    expect(automationApi.nodeStats).toHaveBeenCalledWith({
      workspace_id: 'ws',
      automation_id: 'a1',
      since: range.startUtc,
      until: range.endUtc
    })
  })

  it('asks all time without bounds', async () => {
    run({ period: 'all' })
    await waitFor(() => expect(automationApi.nodeStats).toHaveBeenCalled())
    expect(automationApi.nodeStats).toHaveBeenCalledWith({ workspace_id: 'ws', automation_id: 'a1' })
  })

  it.each([
    { case: 'unsaved or never run', options: { available: false } },
    { case: 'not on screen', options: { watched: false } }
  ])('asks nothing when $case', async ({ options }) => {
    run(options)
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(automationApi.nodeStats).not.toHaveBeenCalled()
  })

  it.each([
    { live: true, watched: true, interval: 60_000 },
    { live: false, watched: true, interval: false },
    { live: true, watched: false, interval: false }
  ])('refetches every minute only live and watched (live $live, watched $watched)', ({ live, watched, interval }) => {
    const { client } = run({ live, watched })
    const query = client.getQueryCache().getAll()[0]
    const options = query.options as { refetchInterval?: unknown }
    expect(options.refetchInterval).toBe(interval)
  })
})
