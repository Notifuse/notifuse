import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import dayjs from '../../../lib/dayjs'
import type { ReactNode } from 'react'
import type { ContactVisitSession, ContactVisitsSummary } from '../../../services/api/web_analytics'
import type { Workspace } from '../../../services/api/types'

const state = vi.hoisted(() => ({
  summary: null as unknown,
  sessions: [] as unknown[],
  linkSearch: [] as unknown[]
}))

vi.mock('@tanstack/react-router', async () => ({
  ...(await vi.importActual('@tanstack/react-router')),
  Link: ({ children, search, params }: { children?: ReactNode; search?: unknown; params?: unknown }) => {
    state.linkSearch.push({ search, params })
    return <a>{children}</a>
  }
}))
vi.mock('../../../services/api/analytics', () => ({ analyticsService: { query: vi.fn() } }))
vi.mock('../../../services/api/messages_history', () => ({ listMessages: vi.fn() }))
vi.mock('../../../services/api/contact_timeline', () => ({ contactTimelineApi: { list: vi.fn() } }))
vi.mock('../../../services/api/web_analytics', () => ({
  webAnalyticsService: {
    contactVisits: vi.fn(async ({ filter, cursor }: { filter: string; cursor?: string }) => ({
      summary: cursor ? undefined : state.summary,
      sessions: filter === 'goal' ? [] : state.sessions,
      next_cursor: undefined
    })),
    contactSessionPages: vi.fn(async () => ({
      pages: [
        { entered_at: '2026-11-12T10:00:00Z', path: '/pricing', duration_ms: 30000, page_number: 1 },
        { entered_at: '2026-11-12T10:02:00Z', path: '/signup', duration_ms: 60000, page_number: 2 }
      ],
      goals: [{ goal_at: '2026-11-12T10:01:00Z', goal_name: 'Upgrade', goal_value: 290, path: '/pricing' }]
    }))
  }
}))

import { webAnalyticsService } from '../../../services/api/web_analytics'
import { ContactWebVisits, webAnalyticsContactSearch } from './ContactWebVisits'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace

const session = (patch: Partial<ContactVisitSession>): ContactVisitSession => ({
  id: 's1',
  session_date: '2026-11-12',
  created_at: new Date().toISOString(),
  duration_ms: 168000,
  pageview_count: 3,
  landing_path: '/pricing',
  exit_path: '/signup',
  channel: 'Email',
  channel_group: 'email',
  referrer_domain: '',
  utm_source: 'newsletter',
  utm_campaign: 'November',
  device: 'tablet',
  os: 'iOS',
  browser: 'Safari',
  country: 'FR',
  goal_count: 2,
  goal_value: 290,
  first_goal_name: 'Upgrade',
  ...patch
})

const summary = (patch: Partial<ContactVisitsSummary> = {}): ContactVisitsSummary => ({
  sessions: 4,
  goal_sessions: 1,
  email_sessions: 2,
  median_duration_ms: 60000,
  first_session: session({ id: 'first', created_at: '2026-01-05T09:00:00Z', device: 'desktop' }),
  goals: [{ goal_name: 'Upgrade', count: 1, value: 290 }],
  top_pages: [{ path: '/pricing', views: 9 }],
  ...patch
})

function renderVisits() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <App>
        <ContactWebVisits workspace={workspace} email="ann@x.com" />
      </App>
    </QueryClientProvider>
  )
}

describe('ContactWebVisits', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.summary = summary()
    state.sessions = [
      session({}),
      session({ id: 's2', device: 'smart-tv', channel_group: 'direct', first_goal_name: null, goal_count: 0 })
    ]
    state.linkSearch = []
  })

  it('sums up the visits: first identified visit, goals reached, most viewed pages', async () => {
    renderVisits()
    expect(await screen.findByText('First identified visit')).toBeInTheDocument()
    expect(screen.getByText('Upgrade · 1 · 290')).toBeInTheDocument()
    expect(screen.getAllByText('/pricing').length).toBeGreaterThan(0)
    expect(screen.getByText('9')).toBeInTheDocument()
  })

  it('lists the sessions with their source, entry, duration, device and goal', async () => {
    renderVisits()
    const first = (await screen.findAllByText('November'))[0]
    expect(first).toBeInTheDocument()
    expect(screen.getAllByText('+2').length).toBeGreaterThan(0)
    expect(screen.getAllByText('2 min 48 s').length).toBeGreaterThan(0)
    expect(document.querySelector('[data-device="tablet"]')).not.toBeNull()
    expect(document.querySelector('[data-device="other"]')).not.toBeNull()
    expect(screen.getByText('Direct')).toBeInTheDocument()
    expect(screen.getAllByText(/^Today · /).length).toBe(2)
  })

  it('loads a session journey once when expanded, with its goal at its time', async () => {
    renderVisits()
    await screen.findAllByText('November')
    const expand = screen.getAllByRole('button', { name: /expand row/i })[0]
    fireEvent.click(expand)
    const journey = await waitFor(() => {
      const element = document.querySelector('[data-session-journey]')
      expect(element).not.toBeNull()
      return element as HTMLElement
    })
    const lines = within(journey)
      .getAllByText(/^\/|Upgrade/)
      .map((el) => el.textContent)
    expect(lines.slice(0, 3)).toEqual(['/pricing', 'Upgrade· 290', '/signup'])
    expect(webAnalyticsService.contactSessionPages).toHaveBeenCalledWith({
      workspace_id: 'ws',
      email: 'ann@x.com',
      session_id: 's1',
      session_date: '2026-11-12'
    })
    fireEvent.click(screen.getAllByRole('button', { name: /collapse row/i })[0])
    fireEvent.click(screen.getAllByRole('button', { name: /expand row/i })[0])
    expect(webAnalyticsService.contactSessionPages).toHaveBeenCalledTimes(1)
    expect(state.linkSearch.at(-1)).toMatchObject({ params: { workspaceId: 'ws', tab: 'dashboard' } })
  })

  it('links to web analytics over the last 30 days, filtered on the contact', () => {
    expect(webAnalyticsContactSearch('ann@x.com', 'UTC', dayjs('2026-11-12T12:00:00Z'))).toEqual({
      period: 'custom',
      customStart: '2026-10-13',
      customEnd: '2026-11-12',
      filters: JSON.stringify([{ dimension: 'contact_email', operator: 'equals', values: ['ann@x.com'] }])
    })
  })

  it('filters the sessions and says when none match', async () => {
    renderVisits()
    fireEvent.click(await screen.findByText(/With a goal/))
    expect(await screen.findByText('No sessions match this filter')).toBeInTheDocument()
    expect(webAnalyticsService.contactVisits).toHaveBeenCalledWith(
      expect.objectContaining({ filter: 'goal' })
    )
  })

  it('says when the contact has no visit', async () => {
    state.summary = summary({ sessions: 0, first_session: null, goals: [], top_pages: [] })
    state.sessions = []
    renderVisits()
    expect(await screen.findByText('No visits recorded for this contact')).toBeInTheDocument()
    expect(screen.getByText('Visits show up once the visitor is identified')).toHaveAttribute(
      'href',
      'https://docs.notifuse.com/web-analytics/identifying-visitors'
    )
  })
})
