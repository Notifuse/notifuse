import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'

const { params, context, stub } = vi.hoisted(() => ({
  // Every section and control is a marker: this suite checks where the page puts them.
  stub: (name: string) => () => <div data-testid={name} />,
  params: { workspaceId: 'ws1', tab: 'dashboard' },
  context: {
    settings: { enabled: true } as { enabled: boolean } | null,
    dimensions: [] as string[],
    setDimensions: vi.fn(),
    workspace: null
  }
}))

vi.mock('@tanstack/react-router', () => ({ useParams: () => params }))

vi.mock('../components/web_analytics/context', () => ({
  WEB_ANALYTICS_TABS: ['dashboard', 'explore', 'goals', 'filters', 'annotations'],
  WebAnalyticsProvider: ({ children }: { children: ReactNode }) => <>{children}</>
}))
vi.mock('../components/web_analytics/useWebAnalytics', () => ({ useWebAnalytics: () => context }))
vi.mock('../components/web_analytics/InstallOverlay', () => ({
  WebAnalyticsGate: ({ children }: { children: ReactNode }) => <div data-testid="gate">{children}</div>
}))

vi.mock('../components/web_analytics/toolbar', () => ({
  DateRangePicker: stub('date-range'),
  ComparisonPicker: stub('comparison')
}))
vi.mock('../components/web_analytics/explore/CsvExportModal', () => ({ CsvExportModal: () => null }))
vi.mock('../components/web_analytics/explore/DimensionSelector', () => ({ DimensionSelector: stub('dimensions') }))
vi.mock('../components/web_analytics/explore/MinSessionsInput', () => ({ MinSessionsInput: stub('min-sessions') }))
vi.mock('../components/web_analytics/FilterBuilder', () => ({ FilterBuilder: stub('filter-builder') }))
vi.mock('../components/web_analytics/tabs/DashboardTab', () => ({ DashboardTab: stub('dashboard-tab') }))
vi.mock('../components/web_analytics/tabs/FiltersTab', () => ({ FiltersTab: stub('filters-tab') }))
vi.mock('../components/web_analytics/tabs/ExploreTab', () => ({ ExploreTab: stub('explore-tab') }))
vi.mock('../components/web_analytics/tabs/GoalsTab', () => ({ GoalsTab: stub('goals-tab') }))
vi.mock('../components/web_analytics/tabs/AnnotationsTab', () => ({ AnnotationsTab: stub('annotations-tab') }))
vi.mock('../components/web_analytics/WebAnalyticsAIAssistant', () => ({ WebAnalyticsAIAssistant: () => null }))
vi.mock('../components/web_analytics/LiveButton', () => ({ LiveButton: stub('live-button') }))

import { WebAnalyticsPage } from './WebAnalyticsPage'

function renderPage(tab: string) {
  params.tab = tab
  const utils = render(<WebAnalyticsPage />)
  const header = utils.container.firstElementChild?.firstElementChild as HTMLElement
  return { ...utils, header }
}

beforeEach(() => {
  context.settings = { enabled: true }
  context.dimensions = []
})

describe('WebAnalyticsPage', () => {
  it('shows the live button in the header of the dashboard, the period beside the filters', async () => {
    const { header } = renderPage('dashboard')
    await screen.findByTestId('dashboard-tab')
    expect(within(header).getByTestId('live-button')).toBeInTheDocument()
    expect(within(header).queryByTestId('date-range')).toBeNull()
    const filterRow = screen.getByTestId('filter-builder').parentElement as HTMLElement
    expect(within(filterRow).getByTestId('date-range')).toBeInTheDocument()
  })

  it('leaves the live button out of the header without web analytics enabled', async () => {
    context.settings = { enabled: false }
    const { header } = renderPage('dashboard')
    await screen.findByTestId('dashboard-tab')
    expect(within(header).queryByTestId('live-button')).toBeNull()
  })

  it('puts the period controls and the export in the header of an Explore report', async () => {
    context.dimensions = ['device']
    const { header } = renderPage('explore')
    await screen.findByTestId('explore-tab')
    expect(within(header).getByTestId('date-range')).toBeInTheDocument()
    expect(within(header).getByRole('button')).toBeInTheDocument()
    expect(within(header).queryByTestId('live-button')).toBeNull()
    expect(screen.getByTestId('dimensions')).toBeInTheDocument()
    const filterRow = screen.getByTestId('filter-builder').parentElement as HTMLElement
    expect(within(filterRow).queryByTestId('date-range')).toBeNull()
  })

  it('puts the period controls beside the filter bar on Goals', async () => {
    const { header } = renderPage('goals')
    await screen.findByTestId('goals-tab')
    expect(within(header).queryByTestId('date-range')).toBeNull()
    const filterRow = screen.getByTestId('filter-builder').parentElement as HTMLElement
    expect(within(filterRow).getByTestId('date-range')).toBeInTheDocument()
  })

  it('stacks the bars and the section in one column spaced like the rest of the console', async () => {
    renderPage('goals')
    const section = await screen.findByTestId('goals-tab')
    const column = section.parentElement as HTMLElement
    expect(column).toHaveClass('flex', 'flex-col', 'gap-3')
    expect(screen.getByTestId('filter-builder').parentElement).not.toHaveClass('mb-4')
  })

  it('shows no period toolbar on the filters section', async () => {
    renderPage('filters')
    await screen.findByTestId('filters-tab')
    expect(screen.queryByTestId('date-range')).toBeNull()
  })
})
