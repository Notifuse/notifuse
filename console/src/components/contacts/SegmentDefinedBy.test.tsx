import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { Segment, TreeNode } from '../../services/api/segment'

vi.mock('../../services/api/task', () => ({ taskApi: { findBySegmentId: vi.fn(async () => null) } }))
vi.mock('../../services/api/segment', () => ({ rebuildSegment: vi.fn(async () => ({ message: 'Rebuild started' })) }))
vi.mock('../segment/button_upsert', () => ({
  DrawerSegment: ({
    segment,
    canReadTemplates,
    totalContacts
  }: {
    segment: Segment
    canReadTemplates: boolean
    totalContacts?: number
  }) => (
    <div
      data-testid="segment-drawer"
      data-can-read-templates={String(canReadTemplates)}
      data-total-contacts={String(totalContacts)}
    >
      {segment.name}
    </div>
  )
}))

import { rebuildSegment } from '../../services/api/segment'
import { SegmentActions, SegmentDefinedBy } from './SegmentDefinedBy'

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

function renderBar(value: Segment) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <App>
        <SegmentDefinedBy workspaceId="ws" segment={value} lists={[]} customFieldLabels={{}} />
      </App>
    </QueryClientProvider>
  )
}

function renderActions(value: Segment, canReadTemplates = true, totalContacts?: number) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <App>
        <SegmentActions
          workspaceId="ws"
          segment={value}
          canReadTemplates={canReadTemplates}
          totalContacts={totalContacts}
        />
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

  it('names the conditions behind « +N » on hover and to screen readers', async () => {
    renderBar(
      segment({ kind: 'branch', branch: { operator: 'and', leaves: ['FR', 'DE', 'ES', 'IT', 'PT'].map(leaf) } })
    )
    expect(screen.getByText('and 2 more: Country is IT, Country is PT')).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByText('+2'))
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Country is IT and Country is PT')
  })

  it('shows the progress while the segment is computed', () => {
    renderBar(segment(leaf('FR'), { status: 'building' }))
    expect(screen.getByRole('img', { name: 'Computing this segment…' })).toBeInTheDocument()
  })

  it('leaves Recompute and Edit segment to the view header', () => {
    renderBar(segment(leaf('FR')))
    expect(screen.getByText('Country is FR')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Recompute/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Edit segment/ })).toBeNull()
  })

  it('recomputes the segment once confirmed', async () => {
    renderActions(segment(leaf('FR')))
    fireEvent.click(screen.getByRole('button', { name: /Recompute/ }))
    // The confirmation says segments already follow the contacts on their own.
    expect(await screen.findByText(/Segments update automatically as contacts change/)).toBeInTheDocument()
    expect(rebuildSegment).not.toHaveBeenCalled()
    fireEvent.click(await screen.findByRole('button', { name: 'Yes' }))
    await waitFor(() => expect(rebuildSegment).toHaveBeenCalledWith({ workspace_id: 'ws', segment_id: 'eu' }))
  })

  it.each([true, false])('opens the segment drawer on Edit segment, with Templates read %s', (read) => {
    renderActions(segment(leaf('FR')), read)
    expect(screen.queryByTestId('segment-drawer')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /Edit segment/ }))
    expect(screen.getByTestId('segment-drawer')).toHaveTextContent('Europe')
    expect(screen.getByTestId('segment-drawer')).toHaveAttribute('data-can-read-templates', String(read))
  })

  // The drawer's count says what share of the workspace the segment holds
  it('hands the workspace total to the drawer', () => {
    renderActions(segment(leaf('FR')), true, 18342)
    fireEvent.click(screen.getByRole('button', { name: /Edit segment/ }))
    expect(screen.getByTestId('segment-drawer')).toHaveAttribute('data-total-contacts', '18342')
  })
})

describe('SegmentDefinedBy — a failed computation', () => {
  it('says which value stopped it, that membership may be incomplete, and when', () => {
    renderBar(
      segment(leaf('FR'), {
        last_error: {
          kind: 'value_conversion',
          source: 'build',
          column: 'custom_json_1',
          path: ['renewed_at'],
          type: 'date',
          email: 'jane@example.com',
          value: '2026-02-30'
        },
        last_error_at: new Date(Date.now() - 3600 * 1000).toISOString()
      })
    )
    const banner = document.querySelector('[data-segment-last-error]') as HTMLElement
    expect(banner).toHaveTextContent(
      'jane@example.com has "2026-02-30" in Custom JSON 1 › renewed_at, which is not a date.'
    )
    expect(banner).toHaveTextContent('Membership may be incomplete: fix the value, then recompute the segment.')
    expect(banner).toHaveTextContent('Failed an hour ago')
  })

  it('asks only for a recompute when no value is at fault', () => {
    renderBar(segment(leaf('FR'), { last_error: { kind: 'other', source: 'build' } }))
    const banner = document.querySelector('[data-segment-last-error]') as HTMLElement
    expect(banner).toHaveTextContent('The last computation failed.')
    expect(banner).toHaveTextContent('Membership may be incomplete: recompute the segment.')
    expect(banner).not.toHaveTextContent('fix the value')
  })

  it('says a value met by the contact queue keeps that contact where it was until it is fixed', () => {
    renderBar(
      segment(leaf('FR'), {
        last_error: {
          kind: 'value_conversion',
          source: 'contact_queue',
          column: 'custom_json_1',
          path: ['renewed_at'],
          type: 'date',
          email: 'jane@example.com'
        }
      })
    )
    const banner = document.querySelector('[data-segment-last-error]') as HTMLElement
    // The queue names the contact even when it could not read the value back
    expect(banner).toHaveTextContent('The value of jane@example.com in Custom JSON 1 › renewed_at is not a date.')
    expect(banner).toHaveTextContent('Until the value is fixed, this contact keeps its current place in the segment.')
  })

  it('reads a redacted error without the contact or the value', () => {
    renderBar(
      segment(leaf('FR'), {
        last_error: {
          kind: 'value_conversion',
          source: 'build',
          column: 'custom_json_1',
          path: ['renewed_at'],
          type: 'number'
        }
      })
    )
    expect(document.querySelector('[data-segment-last-error]')).toHaveTextContent(
      "A contact's value in Custom JSON 1 › renewed_at is not a number."
    )
  })

  it('shows nothing of the kind for a segment computed without error', () => {
    renderBar(segment(leaf('FR')))
    expect(document.querySelector('[data-segment-last-error]')).toBeNull()
  })
})
