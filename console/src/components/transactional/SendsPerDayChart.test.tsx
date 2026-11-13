import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { DailyPoint } from './transactionalStats'

interface CapturedOption {
  xAxis: { data: string[] }
  series: { name: string; data: number[]; stack: string }[]
  tooltip: { formatter: (params: { dataIndex: number }[]) => string }
}
let captured: CapturedOption | null = null
vi.mock('echarts-for-react', () => ({
  default: ({ option }: { option: CapturedOption }) => {
    captured = option
    return <div data-testid="echart" />
  }
}))

import { SendsPerDayChart } from './SendsPerDayChart'

const points: DailyPoint[] = [
  { day: '2026-11-10', sent: 10, delivered: 9, bounced: 1, failed: 2 },
  { day: '2026-11-11', sent: 20, delivered: 20, bounced: 0, failed: 0 },
  { day: '2026-11-12', sent: 3, delivered: 1, bounced: 0, failed: 1 }
]

const renderChart = (props: Partial<Parameters<typeof SendsPerDayChart>[0]> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <SendsPerDayChart
        points={points}
        loading={false}
        onRetry={() => {}}
        smtp={false}
        today="2026-11-12"
        period="30d"
        onPeriodChange={() => {}}
        {...props}
      />
    </I18nProvider>
  )

/** The texts of a table row's header and data cells. */
const cellsOf = (row: HTMLElement) => Array.from(row.querySelectorAll('th, td')).map((cell) => cell.textContent)

beforeEach(() => {
  captured = null
})

describe('SendsPerDayChart', () => {
  it('stacks delivered and bounced or failed, one bar per day as the server keyed it', () => {
    renderChart()
    expect(captured?.xAxis.data).toEqual(['Nov 10', 'Nov 11', 'Nov 12'])
    expect(captured?.series.map((s) => [s.name, s.data])).toEqual([
      ['Delivered', [9, 20, 1]],
      ['Bounced or failed', [3, 0, 1]]
    ])
    expect(new Set(captured?.series.map((s) => s.stack))).toEqual(new Set(['sends']))
  })

  it('shows sent and failed for SMTP, which reports no delivery', () => {
    renderChart({ smtp: true })
    expect(captured?.series.map((s) => [s.name, s.data])).toEqual([
      ['Sent', [10, 20, 3]],
      ['Failed', [2, 0, 1]]
    ])
    const tooltip = captured?.tooltip.formatter([{ dataIndex: 0 }]) ?? ''
    expect(tooltip).toContain('Failed')
    expect(tooltip).not.toContain('Delivered')
  })

  it("gives every count of a day in its tooltip, and marks today's bar as partial", () => {
    renderChart()
    const first = captured?.tooltip.formatter([{ dataIndex: 0 }]) ?? ''
    expect(first).toContain('Sent: <b>10</b>')
    expect(first).toContain('Bounced: <b>1</b>')
    expect(first).not.toContain('Today, so far')
    expect(captured?.tooltip.formatter([{ dataIndex: 2 }])).toContain('Today, so far')
  })

  it('says when the period has no send', () => {
    renderChart({ points: points.map((p) => ({ ...p, sent: 0, delivered: 0, bounced: 0, failed: 0 })) })
    expect(screen.getByText('No sends in this period')).toBeInTheDocument()
    expect(screen.queryByTestId('echart')).not.toBeInTheDocument()
  })

  it('shows a skeleton while loading and a retry on error', () => {
    const onRetry = vi.fn()
    const { rerender } = renderChart({ loading: true, points: undefined })
    expect(screen.getByTestId('sends-chart-loading')).toBeInTheDocument()

    vi.spyOn(console, 'warn').mockImplementation(() => {})
    rerender(
      <I18nProvider i18n={i18n}>
        <SendsPerDayChart
          loading={false}
          error={new Error('x')}
          onRetry={onRetry}
          smtp={false}
          today=""
          period="30d"
          onPeriodChange={() => {}}
        />
      </I18nProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalled()
  })

  it('gives the days as a table for screen readers, the chart itself hidden from them', () => {
    renderChart()
    const table = screen.getByRole('table', { name: 'Sends per day' })
    const rows = within(table).getAllByRole('row')
    expect(rows.map(cellsOf)).toEqual([
      ['Day', 'Sent', 'Delivered', 'Bounced', 'Failed'],
      ['Nov 10', '10', '9', '1', '2'],
      ['Nov 11', '20', '20', '0', '0'],
      ['Nov 12 (Today, so far)', '3', '1', '0', '1']
    ])
    expect(screen.getByTestId('echart').closest('[aria-hidden="true"]')).not.toBeNull()
  })

  it('gives sent and failed only in the table for SMTP', () => {
    renderChart({ smtp: true })
    const table = screen.getByRole('table', { name: 'Sends per day' })
    const header = within(table).getAllByRole('row')[0]
    expect(
      within(header)
        .getAllByRole('columnheader')
        .map((c) => c.textContent)
    ).toEqual(['Day', 'Sent', 'Failed'])
    expect(cellsOf(within(table).getAllByRole('row')[1])).toEqual(['Nov 10', '10', '2'])
  })

  it('gives no table while loading or when the period has no send', () => {
    const { rerender } = renderChart({ loading: true })
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    rerender(
      <I18nProvider i18n={i18n}>
        <SendsPerDayChart
          points={points.map((p) => ({
            ...p,
            sent: 0,
            delivered: 0,
            bounced: 0,
            failed: 0
          }))}
          loading={false}
          onRetry={() => {}}
          smtp={false}
          today="2026-11-12"
          period="30d"
          onPeriodChange={() => {}}
        />
      </I18nProvider>
    )
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('changes the period', () => {
    const onPeriodChange = vi.fn()
    renderChart({ onPeriodChange })
    fireEvent.click(screen.getByText('7d'))
    expect(onPeriodChange).toHaveBeenCalledWith('7d')
  })
})
