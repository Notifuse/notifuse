import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ReactNode } from 'react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { StoredPermissions } from '../services/api/permissions'
import type { DashboardPeriods } from '../components/analytics/dashboardPeriods'

const state = vi.hoisted(() => ({
  search: {} as Record<string, unknown>,
  permissions: null as StoredPermissions | null,
  loading: false,
  navigate: vi.fn(),
  kpiPeriods: [] as DashboardPeriods[],
  seriesEnabled: [] as boolean[],
  role: 'member' as 'root' | 'owner' | 'member' | undefined,
  latestAt: null as string | null | undefined,
  latestEnabled: [] as boolean[],
  settings: { timezone: 'Europe/Paris' } as Record<string, unknown>,
  props: {} as Record<string, Record<string, unknown>>
}))
/** Records the props a mocked block got, under its name. */
const seen = (name: string, props: Record<string, unknown>) => {
  state.props[name] = props
}

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search,
  useNavigate: () => state.navigate,
  Navigate: ({ to, params }: { to: string; params: Record<string, string> }) => (
    <div data-testid="redirect">{to.replace('$workspaceId', params.workspaceId)}</div>
  )
}))
// The API client imports the router, and through it every page: the dashboard asks nothing here.
vi.mock('../services/api/client', () => ({ api: {}, ApiError: class extends Error {} }))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'u1', email: 'root@acme.com' },
    workspaces: [
      {
        id: 'ws1',
        name: 'Acme',
        settings: state.settings,
        integrations: [
          {
            id: 'ses1',
            type: 'email',
            email_provider: { kind: 'ses', senders: [{ id: 's', email: 'hi@acme.com', name: '', is_default: true }] }
          }
        ]
      }
    ]
  }),
  useWorkspacePermissions: () => ({
    permissions: state.permissions,
    role: state.role,
    loading: state.loading
  })
}))
vi.mock('../components/analytics/useLatestMessage', () => ({
  useLatestMessage: (_ws: string, enabled: boolean) => {
    state.latestEnabled.push(enabled)
    return { latestAt: enabled ? state.latestAt : undefined, isError: false }
  }
}))
vi.mock('../components/analytics/GetStartedChecklist', () => ({
  GetStartedChecklist: (props: Record<string, unknown>) => {
    seen('checklist', props)
    return <div data-testid="checklist" />
  }
}))
vi.mock('../components/analytics/ButtonLink', () => ({
  ButtonLink: ({ to, params, search, children }: { to: string; params: Record<string, string>; search?: Record<string, string>; children: ReactNode }) => {
    let href = to
    for (const [key, value] of Object.entries(params)) href = href.replace(`$${key}`, value)
    return <a href={search ? `${href}?${new URLSearchParams(search).toString()}` : href}>{children}</a>
  }
}))
vi.mock('../components/analytics/useOriginSeries', () => ({
  useOriginSeries: ({ enabled }: { enabled: boolean }) => {
    state.seriesEnabled.push(enabled)
    return { isLoading: false, isError: false, refetch: () => {} }
  }
}))
vi.mock('../components/analytics/DashboardKpis', () => ({
  DashboardKpis: (props: { periods: DashboardPeriods }) => {
    state.kpiPeriods.push(props.periods)
    seen('kpis', props)
    return <div data-testid="kpis" />
  }
}))
vi.mock('../components/analytics/OriginVolumeChart', () => ({
  OriginVolumeChart: ({
    metric,
    onMetricChange,
    ghost
  }: {
    metric: string
    onMetricChange: (m: string) => void
    ghost?: { message: string; action?: ReactNode }
  }) => (
    <>
      <button type="button" data-testid="chart" onClick={() => onMetricChange('opens')}>
        {metric}
      </button>
      {ghost ? (
        <div data-testid="ghost">
          {ghost.message}
          {ghost.action}
        </div>
      ) : null}
    </>
  )
}))
vi.mock('../components/analytics/RecentFailures', () => ({
  RecentFailures: (props: Record<string, unknown>) => {
    seen('failures', props)
    return <div data-testid="failures" />
  }
}))
vi.mock('../components/analytics/NewContactsTable', () => ({
  NewContactsTable: ({ canImport }: { canImport: boolean }) => <div data-testid="contacts">{String(canImport)}</div>
}))
vi.mock('../components/analytics/SendingCard', () => ({
  SendingCard: ({ canConfigure }: { canConfigure: boolean }) => <div data-testid="sending">{String(canConfigure)}</div>
}))
vi.mock('../components/analytics/UpcomingSends', () => ({
  UpcomingSends: (props: Record<string, unknown>) => {
    seen('upcoming', props)
    return <div data-testid="upcoming" />
  }
}))

import { AnalyticsPage } from './AnalyticsPage'

const all = { read: true, write: true }
const readOnly = { read: true, write: false }

const renderPage = () =>
  render(
    <I18nProvider i18n={i18n}>
      <AnalyticsPage />
    </I18nProvider>
  )

/** The search the last navigation wrote, from the reducer it was given. */
const lastSearch = (previous: Record<string, unknown> = {}) => {
  const [options] = state.navigate.mock.calls.at(-1) as [
    {
      search: (p: Record<string, unknown>) => Record<string, unknown>
      replace?: boolean
    }
  ]
  // A period or metric is a step the browser's back button undoes.
  expect(options.replace).toBeUndefined()
  return options.search(previous)
}

beforeEach(() => {
  state.search = {}
  state.permissions = {
    message_history: readOnly,
    contacts: all,
    broadcasts: all,
    lists: all,
    workspace: all
  }
  state.loading = false
  state.navigate.mockReset()
  state.kpiPeriods = []
  state.seriesEnabled = []
  state.role = 'member'
  // A workspace that has sent, in the period: the dashboard as it always was.
  state.latestAt = '2026-01-01T00:00:00Z'
  state.latestEnabled = []
  state.settings = { timezone: 'Europe/Paris', marketing_email_provider_id: 'ses1' }
  state.props = {}
})

describe('AnalyticsPage', () => {
  it('shows the last 30 days by default', () => {
    renderPage()

    expect(state.kpiPeriods.at(-1)?.days).toBe(30)
    expect(screen.getByTestId('chart')).toHaveTextContent('sent')
    for (const block of ['kpis', 'failures', 'contacts', 'sending', 'upcoming']) {
      expect(screen.getByTestId(block)).toBeInTheDocument()
    }
  })

  it('says which days the period covers: complete days up to yesterday', () => {
    // Logs counts a rolling "Last 7 days"; the dashboard's 7 d ends yesterday, and nothing said so.
    state.search = { period: '7d' }
    renderPage()
    const { startDay, endDay } = state.kpiPeriods.at(-1)!.current
    const shown = document.querySelector('[data-dashboard-days]')
    const format = new Intl.DateTimeFormat('en', { day: 'numeric', month: 'short', timeZone: 'UTC' })
    expect(shown).toHaveTextContent(
      format
        .formatRange(new Date(`${startDay}T00:00:00Z`), new Date(`${endDay}T00:00:00Z`))
        // The thin spaces around the dash, as the matcher reads the page's text.
        .replace(/\s+/g, ' ')
    )
    expect(endDay < new Date().toISOString().slice(0, 10) || endDay === startDay).toBe(true)
  })

  it('reads the period and the metric from the URL', () => {
    state.search = { period: '7d', metric: 'clicks' }
    renderPage()

    expect(state.kpiPeriods.at(-1)?.days).toBe(7)
    expect(screen.getByTestId('chart')).toHaveTextContent('clicks')
  })

  it('ignores a period or a metric it does not know', () => {
    state.search = { period: '14d', metric: 'bounces' }
    renderPage()

    expect(state.kpiPeriods.at(-1)?.days).toBe(30)
    expect(screen.getByTestId('chart')).toHaveTextContent('sent')
  })

  it('writes the period and the metric to the URL, leaving the defaults out', () => {
    state.search = { period: '7d' }
    renderPage()

    fireEvent.click(screen.getByText('90 d'))
    expect(lastSearch({ period: '7d' })).toEqual({ period: '90d' })

    fireEvent.click(screen.getByText('30 d'))
    expect(lastSearch({ period: '7d', metric: 'opens' })).toEqual({
      period: undefined,
      metric: 'opens'
    })

    fireEvent.click(screen.getByTestId('chart'))
    expect(lastSearch({ period: '7d' })).toEqual({
      period: '7d',
      metric: 'opens'
    })
  })

  it('shows skeletons and asks nothing while the permissions load', () => {
    state.permissions = null
    state.loading = true
    renderPage()

    expect(screen.getByTestId('dashboard-loading')).toBeInTheDocument()
    expect(screen.queryByTestId('kpis')).not.toBeInTheDocument()
    expect(state.seriesEnabled.every((enabled) => !enabled)).toBe(true)
  })

  it('sends a member who cannot read messages to the first page they can reach', () => {
    state.permissions = { contacts: readOnly, workspace: readOnly }
    renderPage()

    expect(screen.getByTestId('redirect')).toHaveTextContent('/console/workspace/ws1/contacts')
  })

  it('never redirects a member who may only write message history to the dashboard or the logs', () => {
    state.permissions = {
      message_history: { read: false, write: true },
      workspace: readOnly
    }
    renderPage()

    // The workspace read right reaches the blog, the first content page.
    expect(screen.getByTestId('redirect')).toHaveTextContent(/^\/console\/workspace\/ws1\/blog$/)
    expect(state.seriesEnabled.every((enabled) => !enabled)).toBe(true)
  })

  it('leaves out the cards a member may not read', () => {
    state.permissions = { message_history: readOnly, workspace: readOnly }
    renderPage()

    expect(screen.queryByTestId('contacts')).not.toBeInTheDocument()
    expect(screen.queryByTestId('upcoming')).not.toBeInTheDocument()
    expect(screen.getByTestId('sending')).toHaveTextContent('false')
  })

  describe('before the first send', () => {
    it('hides the period switch and the days it covers', () => {
      state.latestAt = null
      renderPage()

      expect(screen.queryByText('30 d')).not.toBeInTheDocument()
      expect(document.querySelector('[data-dashboard-days]')).toBeNull()
      expect(state.props.kpis.sendState).toBe('never')
      expect(state.props.failures.sendState).toBe('never')
      expect(screen.getByTestId('ghost')).toHaveTextContent('Your emails will show here, day by day, from your first send.')
    })

    it('shows them once an email was sent', () => {
      renderPage()

      expect(screen.getByText('30 d')).toBeInTheDocument()
      expect(document.querySelector('[data-dashboard-days]')).not.toBeNull()
      expect(state.props.kpis.sendState).toBe('counted')
      expect(state.props.failures.sendState).toBe('counted')
      expect(screen.queryByTestId('ghost')).not.toBeInTheDocument()
    })

    it('keeps them while it is not known yet, so the header does not flicker', () => {
      state.latestAt = undefined
      renderPage()

      expect(screen.getByText('30 d')).toBeInTheDocument()
      expect(document.querySelector('[data-dashboard-days]')).not.toBeNull()
      expect(state.props.kpis.sendState).toBe('loading')
      expect(state.latestEnabled.at(-1)).toBe(true)
    })

    it('asks for the latest message only with the right to read message history', () => {
      state.permissions = null
      state.loading = true
      renderPage()
      expect(state.latestEnabled.every((enabled) => !enabled)).toBe(true)
    })

    it('offers a new broadcast from the chart when marketing can send', () => {
      state.latestAt = null
      renderPage()
      expect(screen.getByText('New broadcast').closest('a')).toHaveAttribute(
        'href',
        '/console/workspace/ws1/broadcasts?compose=new'
      )
    })

    it('offers an owner to connect a provider from the chart when marketing cannot send', () => {
      state.latestAt = null
      state.role = 'owner'
      state.settings = { timezone: 'Europe/Paris' }
      renderPage()
      expect(screen.getByText('Connect an email provider').closest('a')).toHaveAttribute(
        'href',
        '/console/workspace/ws1/settings/integrations'
      )
      expect(screen.queryByText('New broadcast')).not.toBeInTheDocument()
    })

    it('offers nothing from the chart to a writer who is not an owner when marketing cannot send', () => {
      state.latestAt = null
      state.settings = { timezone: 'Europe/Paris' }
      renderPage()
      expect(screen.getByTestId('ghost')).not.toContainElement(screen.queryByRole('link'))
      expect(screen.queryByText('Connect an email provider')).not.toBeInTheDocument()
    })
  })

  it('puts the checklist above the KPIs, with the role and the user', () => {
    state.role = 'owner'
    state.latestAt = null
    renderPage()

    const checklist = screen.getByTestId('checklist')
    expect(checklist.compareDocumentPosition(screen.getByTestId('kpis')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(state.props.checklist).toMatchObject({
      role: 'owner',
      user: { id: 'u1', email: 'root@acme.com' },
      latestAt: null
    })
  })

  it('lets only an owner configure providers from the cards, whatever the workspace rights', () => {
    renderPage()
    expect(screen.getByTestId('sending')).toHaveTextContent('false')
    expect(state.props.upcoming.canConfigure).toBe(false)
    expect(state.props.upcoming.marketingReady).toBe(true)
  })

  it('lets an owner configure providers from the cards', () => {
    state.role = 'owner'
    renderPage()
    expect(screen.getByTestId('sending')).toHaveTextContent('true')
    expect(state.props.upcoming.canConfigure).toBe(true)
  })
})
