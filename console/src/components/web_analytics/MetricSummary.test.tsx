import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MetricSummary } from './MetricSummary'
import { SESSION_METRICS, type MetricTotals } from './lib/types'

const LABELS = {
  sessions: 'Sessions',
  median_duration: 'Median TimeScore',
  bounce_rate: 'Bounce Rate',
  median_scroll: 'Median Scroll Depth'
}

const TOTALS: Record<string, MetricTotals> = {
  sessions: { current: 1200, previous: 1000, changePercent: 20 },
  median_duration: { current: 75, previous: 75, changePercent: 0 },
  bounce_rate: { current: 55, previous: 50, changePercent: 10 },
  median_scroll: { current: 40, previous: 40, changePercent: 0 }
}

function renderSummary(props: Partial<React.ComponentProps<typeof MetricSummary>> = {}) {
  const onSelect = vi.fn()
  const utils = render(
    <MetricSummary
      metrics={SESSION_METRICS}
      totals={TOTALS}
      selected="sessions"
      onSelect={onSelect}
      showComparison={false}
      labels={LABELS}
      tooltips={{ median_duration: 'TimeScore is the median engaged time' }}
      {...props}
    />
  )
  const cell = (label: string) => screen.getByText(label).closest<HTMLElement>('[data-kpi-cell]')!
  return { ...utils, onSelect, cell }
}

describe('MetricSummary', () => {
  it('renders one KPI cell per metric inside a KpiStrip', () => {
    const { container } = renderSummary()
    expect(container.querySelector('[data-kpi-strip]')).not.toBeNull()
    expect(container.querySelectorAll('[data-kpi-cell]')).toHaveLength(SESSION_METRICS.length)
  })

  it('selects the metric of the clicked cell', () => {
    const { onSelect, cell } = renderSummary()
    fireEvent.click(cell('Bounce Rate'))
    expect(onSelect).toHaveBeenCalledWith('bounce_rate')
  })

  it('presses only the selected cell', () => {
    const { cell } = renderSummary({ selected: 'median_scroll' })
    expect(cell('Median Scroll Depth')).toHaveAttribute('aria-pressed', 'true')
    expect(cell('Sessions')).toHaveAttribute('aria-pressed', 'false')
  })

  it('keeps the info icon of a metric that has a tooltip', () => {
    const { cell } = renderSummary()
    expect(cell('Median TimeScore').querySelector('.anticon-info-circle')).not.toBeNull()
    expect(cell('Sessions').querySelector('.anticon-info-circle')).toBeNull()
  })

  it('colours a rise in sessions as good and a rise in bounce rate as bad', () => {
    renderSummary({ showComparison: true })
    expect(screen.getByText('20.0%').closest('.text-success-fg')).not.toBeNull()
    expect(screen.getByText('10.0%').closest('.text-danger-fg')).not.toBeNull()
  })

  it('prints no change line without comparison', () => {
    const { container } = renderSummary({ showComparison: false })
    expect(screen.queryByText('20.0%')).toBeNull()
    expect(container.querySelector('.text-success-fg, .text-danger-fg')).toBeNull()
  })

  it('prints no change line for a zero change while comparing', () => {
    const { cell } = renderSummary({ showComparison: true })
    expect(cell('Median TimeScore').querySelector('.text-success-fg, .text-danger-fg')).toBeNull()
    expect(cell('Sessions').querySelector('.text-success-fg')).not.toBeNull()
  })

  it('shows the skeleton instead of the values while loading', () => {
    const { container } = renderSummary({ loading: true })
    expect(container.querySelectorAll('.ant-skeleton')).toHaveLength(SESSION_METRICS.length)
    expect(screen.queryByText('1.2K')).toBeNull()
  })
})

describe('MetricSummary, figures the chart cannot plot', () => {
  const metrics = [
    ...SESSION_METRICS.slice(0, 1),
    { key: 'conversion_rate', label: 'Conv. Rate', format: 'percentage' as const, color: '#000', selectable: false as const },
    { key: 'sum_goal_value', label: 'Total Value', format: 'currency' as const, color: '#000' }
  ]
  const totals: Record<string, MetricTotals> = {
    sessions: TOTALS.sessions,
    conversion_rate: { current: 6, previous: 0, changePercent: 0 },
    sum_goal_value: { current: 0, previous: 0, changePercent: 0 }
  }

  it('renders a metric that cannot be selected as a plain cell, deaf to clicks', () => {
    const { onSelect, cell } = renderSummary({ metrics, totals, labels: { conversion_rate: 'Conv. Rate' } })
    const rate = cell('Conv. Rate')
    expect(rate.tagName).not.toBe('BUTTON')
    expect(rate).not.toHaveAttribute('aria-pressed')
    fireEvent.click(rate)
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('writes a currency of 0 as "-", as the goal cards do', () => {
    const { cell } = renderSummary({ metrics, totals, currency: 'EUR' })
    expect(cell('Total Value')).toHaveTextContent('-')
    expect(cell('Total Value')).not.toHaveTextContent('€')
  })

  it('prints the sub line given for a metric that has no change to show', () => {
    renderSummary({ subs: { sessions: '21.4% of search-paid' } })
    expect(screen.getByText('21.4% of search-paid')).toBeInTheDocument()
  })

  it('keeps the change over the sub line while comparing', () => {
    renderSummary({ showComparison: true, subs: { sessions: '21.4% of search-paid' } })
    expect(screen.queryByText('21.4% of search-paid')).toBeNull()
    expect(screen.getByText('20.0%')).toBeInTheDocument()
  })
})
