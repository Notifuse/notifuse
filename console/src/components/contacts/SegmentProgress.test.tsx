import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Segment } from '../../services/api/segment'

vi.mock('../../services/api/task', () => ({
  taskApi: {
    findBySegmentId: vi.fn(async () => ({
      progress: 42.4,
      state: { build_segment: { processed_count: 420, matched_count: 12, total_contacts: 1000 } }
    }))
  }
}))

import { taskApi } from '../../services/api/task'
import { SegmentBuildingIndicator } from './SegmentProgress'

describe('SegmentBuildingIndicator', () => {
  it('spins and tells how far the computation is', async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SegmentBuildingIndicator workspaceId="ws" segment={{ id: 'pro', status: 'building' } as Segment} />
      </QueryClientProvider>
    )
    fireEvent.mouseEnter(screen.getByRole('img', { name: 'Computing this segment…' }))
    await waitFor(() => expect(taskApi.findBySegmentId).toHaveBeenCalledWith('ws', 'pro'))
    expect(await screen.findByText('Processed: 420', { exact: false })).toBeInTheDocument()
    expect(screen.getByText('Matched: 12', { exact: false })).toBeInTheDocument()
    expect(screen.getByText('Total: 1,000', { exact: false })).toBeInTheDocument()
  })
})
