import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { EmailMetricsChart } from './EmailMetricsChart'
import { analyticsService } from '../../services/api/analytics'
import type { AnalyticsQuery } from '../../services/api/analytics'
import type { Workspace } from '../../services/api/types'

vi.mock('./ChartVisualization', () => ({ ChartVisualization: () => null }))

vi.mock('../../services/api/analytics', () => ({ analyticsService: { query: vi.fn() } }))

const workspace = { id: 'ws1', settings: { timezone: 'UTC' } } as unknown as Workspace

const row = (over: Record<string, number>) => ({
  count_sent: 100,
  count_delivered: 100,
  count_opened: 10,
  count_clicked: 5,
  count_bounced: 0,
  count_complained: 0,
  count_unsubscribed: 0,
  count_failed: 0,
  ...over
})

const renderChart = () =>
  render(
    <I18nProvider i18n={i18n}>
      <EmailMetricsChart workspace={workspace} timeRange={['2026-01-01', '2026-01-31']} />
    </I18nProvider>
  )

const tile = (title: string) => screen.getByText(title).closest('.ant-statistic') as HTMLElement

describe('EmailMetricsChart tracked rates', () => {
  beforeEach(() => vi.clearAllMocks())

  it('asks for the tracked totals and computes the rates over them', async () => {
    vi.mocked(analyticsService.query).mockResolvedValue({
      data: [row({ count_opens_tracked: 40, count_clicks_tracked: 20 })]
    } as never)
    renderChart()

    await waitFor(() => expect(within(tile('Opens')).getByText('25%')).toBeInTheDocument())
    expect(within(tile('Clicks')).getByText('25%')).toBeInTheDocument()
    expect(screen.getByText('of 40 tracked')).toBeInTheDocument()
    expect(screen.getByText('of 20 tracked')).toBeInTheDocument()

    const measures = vi.mocked(analyticsService.query).mock.calls.map(
      ([query]) => (query as AnalyticsQuery).measures
    )
    expect(measures.some((m) => m.includes('count_opens_tracked') && m.includes('count_clicks_tracked'))).toBe(true)
  })

  it('adds no note when every email was tracked', async () => {
    vi.mocked(analyticsService.query).mockResolvedValue({
      data: [row({ count_opens_tracked: 100, count_clicks_tracked: 100 })]
    } as never)
    renderChart()

    await waitFor(() => expect(within(tile('Opens')).getByText('10%')).toBeInTheDocument())
    expect(screen.queryByText(/tracked$/)).not.toBeInTheDocument()
  })
})
