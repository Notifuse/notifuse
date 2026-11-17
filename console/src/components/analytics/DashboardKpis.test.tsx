import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { AnalyticsQuery } from '../../services/api/analytics'
import type { StoredPermissions } from '../../services/api/permissions'
import type { Workspace } from '../../services/api/types'
import { queryPlan } from '../../__tests__/queryPlan'

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
import { emptyTotals, type MessageOrigin, type MessageTotals } from './dashboardKpisModel'
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
  },
  // 30 days ending today, 11 November
  recent: {
    startDay: '2026-10-13',
    endDay: '2026-11-11',
    startUtc: '2026-10-12T22:00:00.000Z',
    endUtc: '2026-11-11T22:59:59.999Z'
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
  automation: { ...emptyTotals(), ...overrides.automation },
  other: { ...emptyTotals(), ...overrides.other }
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

  it('shows no change while either side is still the period before', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const tree = (shown: DashboardPeriods, seriesPlaceholder: boolean) => (
      <QueryClientProvider client={client}>
        <I18nProvider i18n={i18n}>
          <DashboardKpis
            workspace={workspace(true)}
            permissions={fullPermissions}
            periods={shown}
            timezone="Europe/Paris"
            totals={totals()}
            seriesError={false}
            seriesPlaceholder={seriesPlaceholder}
            onRetrySeries={() => {}}
          />
        </I18nProvider>
      </QueryClientProvider>
    )
    const { rerender } = render(tree(periods, false))
    expect(await screen.findByText('+10.0 pts vs prev. 30 d')).toBeInTheDocument()
    expect(await screen.findByText('+17.5% vs prev. 30 d')).toBeInTheDocument()

    // The 7 days ask again; nothing of theirs has arrived yet.
    query.mockImplementation(() => new Promise(() => {}))
    const week: DashboardPeriods = {
      days: 7,
      preset: 'previous_7_days',
      current: { ...periods.current, startDay: '2026-11-04', startUtc: '2026-11-03T23:00:00.000Z' },
      previous: { ...periods.previous, startDay: '2026-10-28', startUtc: '2026-10-27T23:00:00.000Z' },
      recent: { ...periods.recent, startDay: '2026-11-05', startUtc: '2026-11-04T23:00:00.000Z' }
    }
    rerender(tree(week, true))
    expect(screen.queryByText(/vs prev\./)).not.toBeInTheDocument()
    expect(screen.queryByText(/over \d+ d/)).not.toBeInTheDocument()
    // The figures of the period before stay on screen meanwhile.
    expect(screen.getByText('18,342')).toBeInTheDocument()
  })

  describe('each guard on its own', () => {
    const strip = (client: QueryClient, shown: DashboardPeriods, seriesPlaceholder = false) => (
      <QueryClientProvider client={client}>
        <I18nProvider i18n={i18n}>
          <DashboardKpis
            workspace={workspace(true)}
            permissions={fullPermissions}
            periods={shown}
            timezone="Europe/Paris"
            totals={totals()}
            seriesError={false}
            seriesPlaceholder={seriesPlaceholder}
            onRetrySeries={() => {}}
          />
        </I18nProvider>
      </QueryClientProvider>
    )
    const newClient = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
    // Everything answers but the visits of the bounds given, which never arrive
    const visitsPendingFrom = (startUtc: string) =>
      query.mockImplementation(async (q: AnalyticsQuery) => {
        const range = q.filters?.find((f) => f.operator === 'inDateRange')?.values
        if (q.schema === 'web_sessions' && range?.[0] === startUtc) return new Promise(() => {})
        return answer(q)
      })

    it('shows no rate change while only the series is the period before', async () => {
      render(strip(newClient(), periods, true))
      expect(await screen.findByText('+17.5% vs prev. 30 d')).toBeInTheDocument()
      expect(screen.queryByText(/pts vs prev\./)).not.toBeInTheDocument()
    })

    it('shows no visits change while only the current visits are the period before', async () => {
      const client = newClient()
      const { rerender } = render(strip(client, periods))
      expect(await screen.findByText('+17.5% vs prev. 30 d')).toBeInTheDocument()
      const shifted: DashboardPeriods = {
        ...periods,
        current: { ...periods.current, startUtc: '2026-10-12T22:00:00.000Z' }
      }
      visitsPendingFrom(shifted.current.startUtc)
      rerender(strip(client, shifted))
      await waitFor(() => expect(screen.queryByText('+17.5% vs prev. 30 d')).not.toBeInTheDocument())
      expect(screen.queryByText(/% vs prev\. 30 d$/)).not.toBeInTheDocument()
    })

    it('shows no visits change while only the previous visits are the period before', async () => {
      const client = newClient()
      const { rerender } = render(strip(client, periods))
      expect(await screen.findByText('+17.5% vs prev. 30 d')).toBeInTheDocument()
      const shifted: DashboardPeriods = {
        ...periods,
        previous: { ...periods.previous, startUtc: '2026-09-12T22:00:00.000Z' }
      }
      visitsPendingFrom(shifted.previous.startUtc)
      rerender(strip(client, shifted))
      await waitFor(() => expect(screen.queryByText('+17.5% vs prev. 30 d')).not.toBeInTheDocument())
    })
  })

  it('refreshes every block each minute but the previous period, under its own key', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <I18nProvider i18n={i18n}>
          <DashboardKpis
            workspace={workspace(true)}
            permissions={fullPermissions}
            periods={periods}
            timezone="Europe/Paris"
            totals={totals()}
            seriesError={false}
            onRetrySeries={() => {}}
          />
        </I18nProvider>
      </QueryClientProvider>
    )
    await screen.findByText('+17.5% vs prev. 30 d')
    const { current, previous } = periods
    expect(queryPlan(client)).toEqual(
      expect.arrayContaining([
        {
          key: ['dashboard', 'previous-rates', 'ws1', 'Europe/Paris', previous.startUtc, previous.endUtc],
          refetchInterval: false
        },
        { key: ['contacts', 'ws1', 'dashboard-total'], refetchInterval: 60_000 },
        {
          key: ['contacts', 'ws1', 'dashboard-new', 'Europe/Paris', periods.recent.startUtc, periods.recent.endUtc],
          refetchInterval: 60_000
        },
        {
          key: ['dashboard', 'visits', 'ws1', 'Europe/Paris', current.startUtc, current.endUtc],
          refetchInterval: 60_000
        },
        {
          key: ['dashboard', 'visits', 'ws1', 'Europe/Paris', previous.startUtc, previous.endUtc],
          refetchInterval: false
        }
      ])
    )
    expect(queryPlan(client)).toHaveLength(5)
  })

  it('asks counts within the period bounds, emails only for the previous rates', async () => {
    renderKpis()
    await screen.findByText('+17.5% vs prev. 30 d')

    const queries = query.mock.calls.map(([q]) => q as AnalyticsQuery)
    const newContacts = queries.find((q) => q.schema === 'contacts' && q.filters?.length)
    // New contacts count today too: the period ending yesterday showed « +0 » right after an import.
    expect(newContacts?.filters).toEqual([
      {
        member: 'created_at',
        operator: 'inDateRange',
        values: [periods.recent.startUtc, periods.recent.endUtc]
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

  it('reads no delivered rate when every provider is SMTP, as Logs shows none', async () => {
    const smtp = {
      ...workspace(true),
      settings: {
        ...workspace(true).settings,
        marketing_email_provider_id: 'smtp1',
        transactional_email_provider_id: 'smtp1'
      },
      integrations: [{ id: 'smtp1', type: 'email', email_provider: { kind: 'smtp', senders: [] } }]
    } as unknown as Workspace
    renderKpis({
      workspace: smtp,
      totals: totals({ broadcast: { delivered: 0 }, transactional: { delivered: 0 } })
    })
    const sub = await screen.findByText("Delivery isn't reported by SMTP providers")
    const tile = sub.closest('[data-kpi-cell]') as HTMLElement
    await waitFor(() => expect(tile).toHaveTextContent('Delivered—'))
    expect(tile).not.toHaveTextContent('%')
  })

  it('reads the delivered rate of SMTP providers when deliveries came back all the same', async () => {
    // Through the SMTP webhook endpoint, or from a provider used before (the demo's SES history).
    const smtp = {
      ...workspace(true),
      settings: {
        ...workspace(true).settings,
        marketing_email_provider_id: 'smtp1',
        transactional_email_provider_id: 'smtp1'
      },
      integrations: [{ id: 'smtp1', type: 'email', email_provider: { kind: 'smtp', senders: [] } }]
    } as unknown as Workspace
    renderKpis({
      workspace: smtp,
      totals: totals({ broadcast: { delivered: 588 }, transactional: { delivered: 392 } })
    })
    const label = await screen.findByText('Delivered')
    const tile = label.closest('[data-kpi-cell]') as HTMLElement
    await waitFor(() => expect(tile).toHaveTextContent('98.0%'))
    expect(tile).not.toHaveTextContent("Delivery isn't reported by SMTP providers")
  })

  it('counts the emails of no origin in the delivered rate as Logs does, when a provider reports deliveries', async () => {
    // An SMTP marketing provider and a Postmark transactional one: an email of no origin (a test,
    // a confirmation) may have gone through either, and Logs counts its deliveries.
    const mixed = {
      ...workspace(true),
      settings: {
        ...workspace(true).settings,
        marketing_email_provider_id: 'smtp1',
        transactional_email_provider_id: 'pm1'
      },
      integrations: [
        { id: 'smtp1', type: 'email', email_provider: { kind: 'smtp', senders: [] } },
        { id: 'pm1', type: 'email', email_provider: { kind: 'postmark', senders: [] } }
      ]
    } as unknown as Workspace
    renderKpis({
      workspace: mixed,
      totals: totals({ broadcast: { delivered: 0 }, other: { sent: 100, delivered: 0 } })
    })
    const label = await screen.findByText('Delivered')
    const tile = label.closest('[data-kpi-cell]') as HTMLElement
    // 400 delivered of (400 + 100) sent: the emails of no origin count, as Logs filtered on them
    // shows their Delivered count; the SMTP newsletters stay out.
    await waitFor(() => expect(tile).toHaveTextContent('80.0%'))
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

  describe('before the first send', () => {
    const emailTiles = ['Emails sent', 'Delivered', 'Open rate', 'Click rate']
    const tile = (label: string) => screen.getByText(label).closest('[data-kpi-cell]') as HTMLElement

    it('says when each email tile fills, rather than 0 or a dash', async () => {
      renderKpis({ sendState: 'never', totals: totals({ broadcast: { ...emptyTotals() }, transactional: { ...emptyTotals() } }) })
      expect(await screen.findByText('18,342')).toBeInTheDocument()
      for (const label of emailTiles) {
        expect(tile(label)).toHaveTextContent(`${label}After your first send`)
        expect(tile(label)).not.toHaveTextContent('—')
        expect(tile(label).querySelector('.text-kpi')).toBeNull()
      }
      expect(screen.getAllByText('After your first send')).toHaveLength(4)
    })

    it('needs no series to say so', async () => {
      renderKpis({ sendState: 'never', totals: undefined })
      expect(await screen.findAllByText('After your first send')).toHaveLength(4)
    })

    it('says the emails of today show tomorrow', async () => {
      renderKpis({ sendState: 'today', totals: totals({ broadcast: { ...emptyTotals() }, transactional: { ...emptyTotals() } }) })
      expect(await screen.findAllByText('From tomorrow')).toHaveLength(4)
      fireEvent.mouseEnter(screen.getAllByText('From tomorrow')[0])
      expect(
        await screen.findByText("The dashboard counts complete days; today's emails show tomorrow")
      ).toBeInTheDocument()
    })

    it('still offers a retry when the series failed', async () => {
      renderKpis({ sendState: 'never', totals: undefined, seriesError: true })
      expect(await screen.findAllByRole('button', { name: 'Retry' })).toHaveLength(4)
      expect(screen.queryByText('After your first send')).not.toBeInTheDocument()
    })

    it('shows the figures once sends are counted', async () => {
      renderKpis({ sendState: 'counted' })
      expect(await screen.findByText('1,000')).toBeInTheDocument()
      expect(screen.queryByText('After your first send')).not.toBeInTheDocument()
    })
  })

  it('counts the contacts added today in the new contacts', async () => {
    // A contact created at 9:00 on 11 November, today: inside the bounds asked for.
    const createdToday = Date.parse('2026-11-11T09:00:00Z')
    query.mockImplementation(async (q: AnalyticsQuery) => {
      const range = q.filters?.find((f) => f.operator === 'inDateRange')?.values as string[] | undefined
      if (q.schema === 'contacts' && range) {
        const counted = Date.parse(range[0]) <= createdToday && createdToday <= Date.parse(range[1])
        return row({ count: counted ? 1 : 0 })
      }
      return answer(q)
    })
    renderKpis()
    const sub = await screen.findByText('+1 over 30 d')
    fireEvent.mouseEnter(sub)
    expect(await screen.findByText('Added in the last 30 days, today included')).toBeInTheDocument()
  })

  it('says when visits will show, with the link to set web analytics up', async () => {
    renderKpis({ workspace: workspace(false) })
    const placeholder = await screen.findByText('Once your site is tracked')
    const cell = placeholder.closest('[data-kpi-cell]') as HTMLElement
    expect(cell).not.toHaveTextContent('—')
    expect(screen.getByText('Set up web analytics').closest('a')).toHaveAttribute(
      'href',
      '/console/workspace/ws1/settings/web-analytics'
    )
  })
})
