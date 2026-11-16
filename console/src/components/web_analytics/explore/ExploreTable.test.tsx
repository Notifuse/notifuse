import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import type { ExploreRow } from '../lib/exploreRows'

const { context } = vi.hoisted(() => ({
  context: { customDimensionLabels: {}, minSessions: 1, showComparison: false }
}))

vi.mock('../useWebAnalytics', () => ({ useWebAnalytics: () => context }))

import { ExploreTable } from './ExploreTable'

const row = (key: string, device: string, sessions: number, extra: Partial<ExploreRow> = {}): ExploreRow => ({
  key,
  dimensionIndex: 0,
  childrenLoaded: false,
  device,
  sessions,
  median_duration: 60,
  bounce_rate: 40,
  median_scroll: 50,
  ...extra
})

const ROWS = [row('a', 'mobile', 30), row('b', 'desktop', 70, { sessions_change: 25 })]

function renderTable(props: Partial<React.ComponentProps<typeof ExploreTable>> = {}) {
  const handlers = {
    onExpand: vi.fn(),
    onExpandedRowsChange: vi.fn(),
    onBreakdownClick: vi.fn(),
    onBreakdownHover: vi.fn()
  }
  const utils = render(
    <ExploreTable
      data={ROWS}
      dimensions={['device', 'browser']}
      expandedRowKeys={[]}
      loadingRows={new Set()}
      bestValue={60}
      totals={{ sessions: 100, median_duration: 60, bounce_rate: 40, median_scroll: 50 }}
      {...handlers}
      {...props}
    />
  )
  const bodyRows = () => [...document.querySelectorAll<HTMLElement>('.ant-table-tbody tr.ant-table-row')]
  return { ...utils, ...handlers, bodyRows }
}

beforeEach(() => {
  context.showComparison = false
})

describe('ExploreTable', () => {
  it('renders its rows inside a flush panel, the table without classes of its own', () => {
    const { container, bodyRows } = renderTable()
    const panel = container.querySelector('section') as HTMLElement
    expect(panel.querySelector('[data-panel-body]')).not.toHaveClass('px-3.5')
    expect(panel.querySelector('.ant-table-wrapper')?.className).not.toMatch(/border|rounded/)
    expect(bodyRows().map((tr) => within(tr).getByText(/mobile|desktop/).textContent)).toEqual([
      'desktop',
      'mobile'
    ])
    // The top level carries its share of the period total.
    expect(screen.getByText('(70.0%)')).toBeInTheDocument()
  })

  it('re-sorts by a measure clicked in the header', () => {
    const { bodyRows } = renderTable()
    // Sessions sort descending by default; one click on the header flips it to ascending.
    const header = [...document.querySelectorAll<HTMLElement>('thead th')].find(
      (th) => th.textContent === 'Sessions'
    ) as HTMLElement
    fireEvent.click(header)
    expect(bodyRows().map((tr) => within(tr).getByText(/mobile|desktop/).textContent)).toEqual([
      'mobile',
      'desktop'
    ])
  })

  it('shows the change of a cell when comparing', () => {
    context.showComparison = true
    renderTable()
    expect(screen.getByText('25%')).toHaveClass('text-success-fg')
  })

  it('opens the breakdown of a row from its eye button without expanding it', () => {
    const { bodyRows, onBreakdownClick, onExpand } = renderTable()
    fireEvent.click(within(bodyRows()[0]).getByRole('button'))
    expect(onBreakdownClick).toHaveBeenCalledWith(ROWS[1])
    expect(onExpand).not.toHaveBeenCalled()
  })

  it('expands a row when it is clicked', () => {
    const { bodyRows, onExpand } = renderTable()
    fireEvent.click(bodyRows()[0])
    expect(onExpand).toHaveBeenCalledWith(true, ROWS[1])
  })
})
