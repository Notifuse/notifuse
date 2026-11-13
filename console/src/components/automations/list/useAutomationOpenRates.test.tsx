import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { Automation, AutomationNode } from '../../../services/api/automation'
import type { Workspace } from '../../../services/api/types'
import { runBroadcastQuery } from '../../broadcasts/broadcastQueryClient'
import { last30Days } from '../last30Days'
import { useAutomationOpenRates } from './useAutomationOpenRates'

vi.mock('../../broadcasts/broadcastQueryClient', () => ({ runBroadcastQuery: vi.fn() }))

const workspace = { id: 'ws', settings: { timezone: 'Europe/Paris' } } as unknown as Workspace
const withNodes = (id: string, types: AutomationNode['type'][]) =>
  ({ id, nodes: types.map((type, i) => ({ id: `${id}-${i}`, type })) }) as unknown as Automation

function run(automations: Automation[], canReadHistory = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  return renderHook(() => useAutomationOpenRates(workspace, automations, canReadHistory), { wrapper })
}

describe('useAutomationOpenRates', () => {
  beforeEach(() => {
    vi.mocked(runBroadcastQuery).mockReset()
    vi.mocked(runBroadcastQuery).mockResolvedValue({ data: [] } as never)
  })

  it('asks opens over tracked opens by automation, for the sorted email automations, over the last 30 days', async () => {
    run([withNodes('b', ['trigger', 'email']), withNodes('c', ['trigger', 'delay']), withNodes('a', ['email'])])
    await waitFor(() => expect(runBroadcastQuery).toHaveBeenCalled())
    const range = last30Days('Europe/Paris')
    expect(runBroadcastQuery).toHaveBeenCalledWith(
      {
        schema: 'message_history',
        measures: ['count_opened', 'count_opens_tracked'],
        dimensions: ['automation_id'],
        timezone: 'Europe/Paris',
        filters: [
          { member: 'automation_id', operator: 'in', values: ['a', 'b'] },
          { member: 'channel', operator: 'equals', values: ['email'] },
          { member: 'created_at', operator: 'inDateRange', values: [range.startUtc, range.endUtc] }
        ]
      },
      'ws'
    )
  })

  it('asks nothing without an email automation', async () => {
    const { result } = run([withNodes('c', ['trigger', 'delay'])])
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(runBroadcastQuery).not.toHaveBeenCalled()
    expect(result.current.rates.size).toBe(0)
  })

  it('asks nothing without message history read', async () => {
    run([withNodes('a', ['email'])], false)
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(runBroadcastQuery).not.toHaveBeenCalled()
  })

  it('maps each automation to its rate, null when nothing tracked was sent', async () => {
    vi.mocked(runBroadcastQuery).mockResolvedValue({
      data: [
        { automation_id: 'a', count_opened: 25, count_opens_tracked: 100 },
        { automation_id: 'b', count_opened: 0, count_opens_tracked: 0 }
      ]
    } as never)
    const { result } = run([withNodes('a', ['email']), withNodes('b', ['email']), withNodes('c', ['email'])])
    await waitFor(() => expect(result.current.rates.get('a')).toBe(0.25))
    expect(result.current.rates.get('b')).toBeNull()
    expect(result.current.rates.get('c')).toBeNull()
    expect(result.current.error).toBe(false)
  })

  it('reports an error', async () => {
    vi.mocked(runBroadcastQuery).mockRejectedValue(new Error('boom'))
    const { result } = run([withNodes('a', ['email'])])
    await waitFor(() => expect(result.current.error).toBe(true))
  })
})
