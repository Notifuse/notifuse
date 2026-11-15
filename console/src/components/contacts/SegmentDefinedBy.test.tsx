import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { Segment, TreeNode } from '../../services/api/segment'

vi.mock('../../services/api/task', () => ({ taskApi: { findBySegmentId: vi.fn(async () => null) } }))
vi.mock('../../services/api/segment', () => ({ rebuildSegment: vi.fn(async () => ({ message: 'Rebuild started' })) }))
vi.mock('../segment/button_upsert', () => ({
  DrawerSegment: ({ segment }: { segment: Segment }) => <div data-testid="segment-drawer">{segment.name}</div>
}))

import { rebuildSegment } from '../../services/api/segment'
import { SegmentDefinedBy } from './SegmentDefinedBy'

const leaf = (value: string): TreeNode => ({
  kind: 'leaf',
  leaf: {
    source: 'contacts',
    contact: {
      filters: [{ field_name: 'country', field_type: 'string', operator: 'equals', string_values: [value] }]
    }
  }
})

const segment = (tree: TreeNode, patch: Partial<Segment> = {}): Segment =>
  ({
    id: 'eu',
    name: 'Europe',
    color: '#000',
    status: 'active',
    tree,
    timezone: 'Europe/Paris',
    version: 1,
    db_created_at: '2026-01-01T00:00:00Z',
    db_updated_at: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
    ...patch
  }) as Segment

function renderBar(value: Segment, canWrite = false) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <App>
        <SegmentDefinedBy workspaceId="ws" segment={value} lists={[]} customFieldLabels={{}} canWrite={canWrite} />
      </App>
    </QueryClientProvider>
  )
}

describe('SegmentDefinedBy', () => {
  it('shows the first conditions, the joiner, the others counted and when it was updated', () => {
    renderBar(segment({ kind: 'branch', branch: { operator: 'or', leaves: ['FR', 'DE', 'ES', 'IT'].map(leaf) } }))
    expect(screen.getByText('Defined by')).toBeInTheDocument()
    expect(screen.getByText('Country is FR')).toBeInTheDocument()
    expect(screen.getByText('Country is ES')).toBeInTheDocument()
    expect(screen.queryByText('Country is IT')).toBeNull()
    expect(screen.getAllByText('or')).toHaveLength(2)
    expect(screen.getByText('+1')).toBeInTheDocument()
    expect(screen.getByText('Updated 2 hours ago', { exact: false })).toBeInTheDocument()
  })

  it('shows the progress while the segment is computed', () => {
    renderBar(segment(leaf('FR'), { status: 'building' }))
    expect(screen.getByRole('img', { name: 'Computing this segment…' })).toBeInTheDocument()
  })

  it('offers Recompute and Edit segment only with segments write', () => {
    renderBar(segment(leaf('FR')))
    expect(screen.queryByRole('button', { name: /Recompute/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Edit segment/ })).toBeNull()
  })

  it('recomputes the segment once confirmed', async () => {
    renderBar(segment(leaf('FR')), true)
    fireEvent.click(screen.getByRole('button', { name: /Recompute/ }))
    expect(rebuildSegment).not.toHaveBeenCalled()
    fireEvent.click(await screen.findByRole('button', { name: 'Yes' }))
    await waitFor(() => expect(rebuildSegment).toHaveBeenCalledWith({ workspace_id: 'ws', segment_id: 'eu' }))
  })

  it('opens the segment drawer on Edit segment', () => {
    renderBar(segment(leaf('FR')), true)
    expect(screen.queryByTestId('segment-drawer')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /Edit segment/ }))
    expect(screen.getByTestId('segment-drawer')).toHaveTextContent('Europe')
  })
})
