import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

const state = vi.hoisted(() => ({
  stats: undefined as Record<string, number> | undefined,
  timeline: [] as Record<string, unknown>[],
  summary: undefined as Record<string, number> | undefined,
  visitsEnabled: [] as boolean[],
  statsEnabled: [] as boolean[]
}))

vi.mock('./useContactSheetQueries', () => ({
  useContactMessageStats: (_ws: string, _email: string, enabled: boolean) => {
    state.statsEnabled.push(enabled)
    return { data: enabled ? state.stats : undefined, isLoading: false }
  },
  useContactTimeline: (_ws: string, _email: string, filter: string) => ({
    data: filter === 'all' ? { pages: [{ timeline: state.timeline }] } : undefined,
    isLoading: false
  }),
  useContactVisits: (_ws: string, _email: string, filter: string, enabled: boolean) => {
    state.visitsEnabled.push(enabled)
    return {
      data: enabled && filter === 'all' ? { pages: [{ summary: state.summary }] } : undefined,
      isLoading: false
    }
  }
}))

import { ContactKpis } from './ContactKpis'

const cell = (label: string) => screen.getByText(label).closest('[data-kpi-cell]') as HTMLElement

describe('ContactKpis', () => {
  beforeEach(() => {
    state.stats = {
      sent: 20,
      sentLast30Days: 3,
      opened: 5,
      opensTracked: 10,
      clicked: 0,
      clicksTracked: 0,
      messages: 22
    }
    state.timeline = [
      {
        entity_type: 'message_history',
        kind: 'email.link_clicked',
        changes: {},
        created_at: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
        entity_data: { template_name: 'Newsletter' }
      }
    ]
    state.summary = { sessions: 7, median_duration_ms: 168000 }
    state.statsEnabled = []
    state.visitsEnabled = []
  })

  it('shows emails received, the rates, the last activity and the site visits', () => {
    render(<ContactKpis workspaceId="ws" email="a@x.com" canReadMessages canReadVisits />)
    expect(cell('Emails received')).toHaveTextContent('20')
    expect(cell('Emails received')).toHaveTextContent('Last 30 days: 3')
    expect(cell('Open rate')).toHaveTextContent('50')
    expect(cell('Open rate')).toHaveTextContent('5 opened')
    // No email tracked for clicks: no rate.
    expect(cell('Click rate')).toHaveTextContent('—')
    expect(cell('Last activity')).toHaveTextContent('2 hours ago')
    expect(cell('Last activity')).toHaveTextContent('Click · Newsletter')
    expect(cell('Site visits')).toHaveTextContent('7')
    expect(cell('Site visits')).toHaveTextContent('Median 2 min 48 s')
  })

  it('hides the message cells without message history read, and the visits without web analytics', () => {
    render(<ContactKpis workspaceId="ws" email="a@x.com" canReadMessages={false} canReadVisits={false} />)
    expect(screen.queryByText('Emails received')).toBeNull()
    expect(screen.queryByText('Open rate')).toBeNull()
    expect(screen.queryByText('Click rate')).toBeNull()
    expect(screen.queryByText('Site visits')).toBeNull()
    expect(screen.getByText('Last activity')).toBeInTheDocument()
    expect(state.statsEnabled.every((enabled) => !enabled)).toBe(true)
    expect(state.visitsEnabled.every((enabled) => !enabled)).toBe(true)
  })

  it('shows a dash without any activity', () => {
    state.timeline = []
    render(<ContactKpis workspaceId="ws" email="a@x.com" canReadMessages canReadVisits />)
    expect(cell('Last activity')).toHaveTextContent('—')
  })
})
