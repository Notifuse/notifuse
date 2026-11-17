import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import type { ExploreTotals } from '../lib/exploreRows'

const { context } = vi.hoisted(() => ({
  context: { customDimensionLabels: {}, minSessions: 1 }
}))

vi.mock('../useWebAnalytics', () => ({ useWebAnalytics: () => context }))

import { ExploreSummary } from './ExploreSummary'

const TOTALS: ExploreTotals = {
  sessions: 1200,
  median_duration: 75,
  bounce_rate: 40,
  median_scroll: 50,
  sessions_change: 10,
  median_duration_change: 0,
  bounce_rate_change: 5,
  median_scroll_change: -8
}

function renderSummary(props: Partial<React.ComponentProps<typeof ExploreSummary>> = {}) {
  const utils = render(
    <ExploreSummary
      totals={TOTALS}
      showComparison={false}
      bestValue={120}
      dimensions={['device']}
      {...props}
    />
  )
  const cell = (label: string) => screen.getByText(label).closest<HTMLElement>('[data-kpi-cell]')!
  return { ...utils, cell }
}

beforeEach(() => {
  context.minSessions = 1
})

describe('ExploreSummary', () => {
  it('shows one cell per session measure, none of them clickable', () => {
    const { container } = renderSummary()
    const cells = container.querySelectorAll('[data-kpi-strip] [data-kpi-cell]')
    expect(cells).toHaveLength(4)
    expect(container.querySelector('button[data-kpi-cell]')).toBeNull()
    expect(screen.getByText('1.2K')).toBeInTheDocument()
  })

  it('adds the Best TimeScore cell right after the median, with the combination on hover', async () => {
    const { container } = renderSummary({
      bestTimeScore: 110,
      bestCombination: { device: 'desktop' }
    })
    const labels = [...container.querySelectorAll('[data-kpi-cell] > span:first-child')].map(
      (label) => label.textContent
    )
    expect(labels).toEqual([
      'Sessions',
      'Median TimeScore',
      'Best TimeScore',
      'Bounce Rate',
      'Median Scroll Depth'
    ])
    fireEvent.mouseEnter(screen.getByText('1 min 50 s'))
    expect(await screen.findByText('Best performing combination:')).toBeInTheDocument()
    expect(screen.getByText('desktop')).toBeInTheDocument()
  })

  it('leaves the Best TimeScore cell out when the report has no engaged time', () => {
    renderSummary({ bestTimeScore: 0 })
    expect(screen.queryByText('Best TimeScore')).toBeNull()
  })

  it('writes "—" in the Best TimeScore cell when its query failed, and says why on hover', async () => {
    const { cell } = renderSummary({ bestError: true })
    fireEvent.mouseEnter(within(cell('Best TimeScore')).getByText('—'))
    expect(
      await screen.findByText('The best-performing combination could not be loaded.')
    ).toBeInTheDocument()
  })

  it('keeps the TimeScore heat dot and the info icon on the median cell', () => {
    const { cell } = renderSummary()
    const median = cell('Median TimeScore')
    expect(median.querySelector('.anticon-info-circle')).not.toBeNull()
    const dot = [...median.querySelectorAll<HTMLElement>('span')].find((span) => span.style.borderRadius === '50%')
    expect(dot).toBeDefined()
  })

  it('colours each change by whether it is good, a rise in bounce rate being bad', () => {
    const { cell } = renderSummary({ showComparison: true })
    expect(within(cell('Sessions')).getByText('10.0%')).toHaveClass('text-success-fg')
    expect(within(cell('Bounce Rate')).getByText('5.0%')).toHaveClass('text-danger-fg')
    expect(within(cell('Median Scroll Depth')).getByText('8.0%')).toHaveClass('text-danger-fg')
    expect(cell('Median TimeScore').querySelector('.text-success-fg, .text-danger-fg')).toBeNull()
  })

  it('shows no change without comparison', () => {
    const { container } = renderSummary({ showComparison: false })
    expect(container.querySelector('.text-success-fg, .text-danger-fg')).toBeNull()
  })

  it('shows the skeleton until the totals land', () => {
    const { container } = renderSummary({ totals: undefined, loading: true })
    expect(container.querySelectorAll('.ant-skeleton')).toHaveLength(4)
  })
})
