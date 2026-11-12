import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { AnalyticsQuery } from '../../services/api/analytics'
import type { StoredPermissions } from '../../services/api/permissions'
import type { Workspace } from '../../services/api/types'

const { query } = vi.hoisted(() => ({ query: vi.fn() }))
vi.mock('./dashboardClient', () => ({
  dashboardAnalytics: { query },
  DASHBOARD_REFRESH_MS: 60_000
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: ({
    to,
    params,
    search,
    children,
    className
  }: {
    to: string
    params?: Record<string, string>
    search?: Record<string, string>
    children: ReactNode
    className?: string
  }) => {
    let href = to
    for (const [key, value] of Object.entries(params ?? {})) href = href.replace(`$${key}`, value)
    const qs = new URLSearchParams(search ?? {}).toString()
    return (
      <a href={qs ? `${href}?${qs}` : href} className={className}>
        {children}
      </a>
    )
  }
}))

import { DashboardKpis } from './DashboardKpis'
import { emptyTotals, type MessageOrigin, type MessageTotals } from './dashboardKpis'
import type { DashboardPeriods } from './dashboardPeriods'

const periods: DashboardPeriods = {
  days: 30,
  preset: 'previous_30_days',
  current: {
    startDay: '2026-10-12',
    endDay: '2026-11-10',
    startUtc: '2026-10-11T22:00:00.000Z',
    endUtc: '2026-11-10T22:59:59.999Z'
  },
  previous: {
    startDay: '2026-09-12',
    endDay: '2026-10-11',
    startUtc: '2026-09-11T22:00:00.000Z',
    endUtc: '2026-10-11T21:59:59.999Z'
  }
}

const all = { read: true, write: true }
const fullPermissions: StoredPermissions = {
  contacts: all,
  web_analytics: all,
  workspace: all,
  message_history: all
}

const workspace = (webEnabled: boolean) =>
  ({
    id: 'ws1',
    name: 'Acme',
    settings: {
      timezone: 'Europe/Paris',
      web_analytics: { enabled: webEnabled }
    }
  }) as unknown as Workspace

const totals = (overrides: Partial<Record<MessageOrigin, Partial<MessageTotals>>> = {}) => ({
  broadcast: {
    ...emptyTotals(),
    sent: 600,
    delivered: 600,
    opened: 300,
    opensTracked: 600,
    clicked: 60,
    clicksTracked: 600,
    ...overrides.broadcast
  },
  transactional: {
    ...emptyTotals(),
    sent: 400,
    delivered: 400,
    opened: 200,
    opensTracked: 400,
    clicked: 40,
    clicksTracked: 400,
    ...overrides.transactional
  },
  automation: { ...emptyTotals(), ...overrides.automation }
})

const row = (data: Record<string, unknown>) => ({
  data: [data],
  meta: { total: 1, query: '', params: [] }
})

/** Answers each query of the strip from its schema and bounds. */
function answer(q: AnalyticsQuery) {
  const range = q.filters?.find((f) => f.operator === 'inDateRange')?.values
  const isCurrent = range?.[0] === periods.current.startUtc
  if (q.schema === 'contacts') return row({ count: range ? 1204 : 18342 })
  if (q.schema === 'web_sessions') return row({ sessions: isCurrent ? 4700 : 4000 })
  // Previous period rates: 40 % opens, 8 % clicks.
  return row({
    count_opened: 400,
    count_opens_tracked: 1000,
    count_clicked: 80,
    count_clicks_tracked: 1000
  })
}

function renderKpis(props: Partial<Parameters<typeof DashboardKpis>[0]> = {}) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <DashboardKpis
          workspace={workspace(true)}
          permissions={fullPermissions}
          periods={periods}
          timezone="Europe/Paris"
          totals={totals()}
          seriesError={false}
          onRetrySeries={() => {}}
          {...props}
        />
      </I18nProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  query.mockReset()
  query.mockImplementation(async (q: AnalyticsQuery) => answer(q))
})

describe('DashboardKpis', () => {
  it('shows contacts, sends, rates and visits with their change', async () => {
    renderKpis()

    expect(await screen.findByText('18,342')).toBeInTheDocument()
    expect(screen.getByText('+1,204 over 30 d')).toBeInTheDocument()
    expect(screen.getByText('1,000')).toBeInTheDocument()
    expect(screen.getByText('60% broadcasts')).toBeInTheDocument()
    expect(screen.getByText('50.0%')).toBeInTheDocument()
    expect(await screen.findByText('+10.0 pts vs prev. 30 d')).toBeInTheDocument()
    expect(screen.getByText('10.0%')).toBeInTheDocument()
    expect(screen.getByText('+2.0 pts vs prev. 30 d')).toBeInTheDocument()
    expect(await screen.findByText('+17.5% vs prev. 30 d')).toBeInTheDocument()

    expect(screen.getByText('18,342').closest('a')).toHaveAttribute('href', '/console/workspace/ws1/contacts')
    expect(screen.getByText('4.7K').closest('a')).toHaveAttribute(
      'href',
      '/console/workspace/ws1/web-analytics/dashboard?period=previous_30_days&comparison=previous_period'
    )
  })

  it('asks counts within the period bounds, emails only for the previous rates', async () => {
    renderKpis()
    await screen.findByText('+17.5% vs prev. 30 d')

    const queries = query.mock.calls.map(([q]) => q as AnalyticsQuery)
    const newContacts = queries.find((q) => q.schema === 'contacts' && q.filters?.length)
    expect(newContacts?.filters).toEqual([
      {
        member: 'created_at',
        operator: 'inDateRange',
        values: [periods.current.startUtc, periods.current.endUtc]
      }
    ])
    const previousRates = queries.find((q) => q.schema === 'message_history')
    expect(previousRates?.filters).toEqual([
      {
        member: 'created_at',
        operator: 'inDateRange',
        values: [periods.previous.startUtc, periods.previous.endUtc]
      },
      { member: 'channel', operator: 'equals', values: ['email'] }
    ])
    expect(queries.filter((q) => q.schema === 'web_sessions')).toHaveLength(2)
  })

  it('leaves out the contacts and visits a member may not read', async () => {
    renderKpis({
      permissions: { message_history: { read: true, write: false } }
    })

    expect(await screen.findByText('1,000')).toBeInTheDocument()
    expect(screen.queryByText('Contacts')).not.toBeInTheDocument()
    expect(screen.queryByText('Website visits')).not.toBeInTheDocument()
    await waitFor(() => expect(query).toHaveBeenCalled())
    const schemas = query.mock.calls.map(([q]) => (q as AnalyticsQuery).schema)
    expect(schemas).not.toContain('contacts')
    expect(schemas).not.toContain('web_sessions')
  })

  it('offers to set up web analytics to a workspace writer when it is off', async () => {
    renderKpis({ workspace: workspace(false) })

    const setup = await screen.findByText('Set up web analytics')
    expect(setup.closest('a')).toHaveAttribute('href', '/console/workspace/ws1/settings/web-analytics')
    expect(query.mock.calls.map(([q]) => (q as AnalyticsQuery).schema)).not.toContain('web_sessions')
  })

  it('hides visits when web analytics is off and the member cannot set it up', async () => {
    renderKpis({
      workspace: workspace(false),
      permissions: {
        ...fullPermissions,
        workspace: { read: true, write: false }
      }
    })

    expect(await screen.findByText('18,342')).toBeInTheDocument()
    expect(screen.queryByText('Website visits')).not.toBeInTheDocument()
  })

  it('says over how many emails a partly tracked rate is computed', async () => {
    renderKpis({
      totals: totals({ transactional: { opensTracked: 200, opened: 100 } })
    })

    // 400 opens over 800 tracked emails.
    const value = await screen.findByText('50.0%')
    fireEvent.mouseEnter(value)
    expect(await screen.findByText('of 800 tracked')).toBeInTheDocument()
  })

  it('offers a retry on the email KPIs when the series failed', async () => {
    const onRetrySeries = vi.fn()
    renderKpis({ totals: undefined, seriesError: true, onRetrySeries })

    expect(await screen.findByText('18,342')).toBeInTheDocument()
    const retries = screen.getAllByRole('button', { name: 'Retry' })
    // Sent, delivered, open rate and click rate.
    expect(retries).toHaveLength(4)
    fireEvent.click(retries[0])
    expect(onRetrySeries).toHaveBeenCalledTimes(1)
  })
})
