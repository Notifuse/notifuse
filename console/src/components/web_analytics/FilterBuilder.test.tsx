import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { FilterBuilder } from './FilterBuilder'
import { WebMetricFilter } from './lib/types'

const context: { metricFilters: WebMetricFilter[] } = { metricFilters: [] }

vi.mock('./useWebAnalytics', () => ({
  useWebAnalytics: () => ({
    filters: [],
    setFilters: vi.fn(),
    metricFilters: context.metricFilters,
    setMetricFilters: vi.fn(),
    customDimensionLabels: {}
  })
}))

describe('FilterBuilder metric filter tags', () => {
  it('writes a bounce-rate threshold as typed, without floating-point noise', () => {
    context.metricFilters = [
      { metric: 'bounce_rate', operator: 'gt', values: [12.3] },
      { metric: 'bounce_rate', operator: 'lt', values: [33.333] },
      { metric: 'median_scroll', operator: 'gte', values: [0.07] },
      { metric: 'median_scroll', operator: 'lte', values: [50] }
    ]
    render(<FilterBuilder allowMetricFilters />)

    expect(screen.getByText(/12\.3%/).textContent).not.toMatch(/12\.30/)
    expect(screen.getByText(/33\.333%/)).toBeTruthy()
    expect(screen.getByText(/0\.07%/)).toBeTruthy()
    expect(screen.getByText(/50%/)).toBeTruthy()
    expect(document.body.textContent).not.toMatch(/0000|9999/)
  })
})

describe('FilterBuilder metric threshold', () => {
  const openMetric = async (label: string) => {
    context.metricFilters = []
    render(<FilterBuilder allowMetricFilters />)
    fireEvent.click(screen.getByRole('button', { name: /Add filter/ }))
    fireEvent.click(await screen.findByRole('button', { name: label }))
    return screen.getByRole('spinbutton')
  }

  it('bounds a rate to 100 with one decimal', async () => {
    const input = await openMetric('Bounce Rate (%)')
    expect(input).toHaveAttribute('aria-valuemax', '100')
    fireEvent.change(input, { target: { value: '12.345' } })
    fireEvent.blur(input)
    expect(input).toHaveValue('12.3')
  })

  it('counts a duration in whole seconds, without an upper bound', async () => {
    const input = await openMetric('TimeScore (seconds)')
    expect(input).not.toHaveAttribute('aria-valuemax')
    fireEvent.change(input, { target: { value: '12.6' } })
    fireEvent.blur(input)
    expect(input).toHaveValue('13')
  })
})
