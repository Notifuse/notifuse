import { describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { refreshBroadcastStats } from './refreshBroadcastStats'

describe('refreshBroadcastStats', () => {
  it('reads again the broadcast, its stats, task, test results, variation stats and links', () => {
    const client = new QueryClient()
    const spy = vi.spyOn(client, 'invalidateQueries')
    refreshBroadcastStats(client, 'ws1', 'bc1')
    expect(spy.mock.calls.map(([filters]) => filters?.queryKey)).toEqual([
      ['broadcast', 'ws1', 'bc1'],
      ['broadcast-stats', 'ws1', 'bc1'],
      ['task', 'ws1', 'bc1'],
      ['testResults', 'ws1', 'bc1'],
      ['broadcast-variation-stats', 'ws1', 'bc1'],
      ['broadcast-link-stats', 'ws1', 'bc1']
    ])
  })
})
