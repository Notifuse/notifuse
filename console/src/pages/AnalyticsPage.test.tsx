import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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
  seriesEnabled: [] as boolean[]
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search,
  useNavigate: () => state.navigate,
  Navigate: ({ to, params }: { to: string; params: Record<string, string> }) => (
    <div data-testid="redirect">{to.replace('$workspaceId', params.workspaceId)}</div>
  )
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', name: 'Acme', settings: { timezone: 'Europe/Paris' } }]
  }),
  useWorkspacePermissions: () => ({
    permissions: state.permissions,
    loading: state.loading
  })
}))
vi.mock('../components/analytics/useOriginSeries', () => ({
  useOriginSeries: ({ enabled }: { enabled: boolean }) => {
    state.seriesEnabled.push(enabled)
    return { isLoading: false, isError: false, refetch: () => {} }
  }
}))
vi.mock('../components/analytics/DashboardKpis', () => ({
  DashboardKpis: ({ periods }: { periods: DashboardPeriods }) => {
    state.kpiPeriods.push(periods)
    return <div data-testid="kpis" />
  }
}))
vi.mock('../components/analytics/OriginVolumeChart', () => ({
  OriginVolumeChart: ({ metric, onMetricChange }: { metric: string; onMetricChange: (m: string) => void }) => (
    <button type="button" data-testid="chart" onClick={() => onMetricChange('opens')}>
      {metric}
    </button>
  )
}))
vi.mock('../components/analytics/RecentFailures', () => ({
  RecentFailures: () => <div data-testid="failures" />
}))
vi.mock('../components/analytics/NewContactsTable', () => ({
  NewContactsTable: ({ canImport }: { canImport: boolean }) => <div data-testid="contacts">{String(canImport)}</div>
}))
vi.mock('../components/analytics/SendingCard', () => ({
  SendingCard: ({ canConfigure }: { canConfigure: boolean }) => <div data-testid="sending">{String(canConfigure)}</div>
}))
vi.mock('../components/analytics/UpcomingSends', () => ({
  UpcomingSends: () => <div data-testid="upcoming" />
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
})
