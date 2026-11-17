import { beforeEach, describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { FilterBuilder } from './FilterBuilder'
import { WebDimensionFilter, WebMetricFilter } from './lib/types'

const { context } = vi.hoisted(() => ({
  context: {
    filters: [] as WebDimensionFilter[],
    metricFilters: [] as WebMetricFilter[],
    setFilters: vi.fn(),
    setMetricFilters: vi.fn()
  }
}))

vi.mock('./useWebAnalytics', () => ({
  useWebAnalytics: () => ({ ...context, customDimensionLabels: {} })
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

describe('FilterBuilder dimension filters', () => {
  beforeEach(() => {
    context.filters = []
    context.metricFilters = []
    context.setFilters.mockReset()
    context.setMetricFilters.mockReset()
  })

  it('edits a filter from its chip, replacing it where it stands', async () => {
    context.filters = [
      { dimension: 'device', operator: 'equals', values: ['mobile'] },
      { dimension: 'utm_source', operator: 'contains', values: ['news'] }
    ]
    render(<FilterBuilder />)
    fireEvent.click(screen.getByText(/UTM Source contains news/))

    const value = await screen.findByDisplayValue('news')
    fireEvent.change(value, { target: { value: 'newsletter' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filter' }))

    expect(context.setFilters).toHaveBeenCalledWith([
      { dimension: 'device', operator: 'equals', values: ['mobile'] },
      { dimension: 'utm_source', operator: 'contains', values: ['newsletter'] }
    ])
  })

  it('removes a filter from its close icon without opening it', async () => {
    context.filters = [{ dimension: 'utm_source', operator: 'contains', values: ['news'] }]
    render(<FilterBuilder />)
    const chip = screen.getByText(/UTM Source contains news/).closest('.ant-tag') as HTMLElement
    fireEvent.click(chip.querySelector('.ant-tag-close-icon') as HTMLElement)
    expect(context.setFilters).toHaveBeenCalledWith([])
    expect(screen.queryByRole('button', { name: 'Apply filter' })).toBeNull()
  })

  it('says a value is missing instead of ignoring Apply', async () => {
    render(<FilterBuilder />)
    fireEvent.click(screen.getByRole('button', { name: /Add filter/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'UTM Source' }))
    fireEvent.click(screen.getByRole('button', { name: 'Apply filter' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Enter a value')
    expect(screen.getByPlaceholderText('Value')).toHaveClass('ant-input-status-error')
    expect(context.setFilters).not.toHaveBeenCalled()

    fireEvent.change(screen.getByPlaceholderText('Value'), { target: { value: 'google' } })
    expect(screen.queryByRole('alert')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Apply filter' }))
    expect(context.setFilters).toHaveBeenCalledWith([
      { dimension: 'utm_source', operator: 'equals', values: ['google'] }
    ])
  })
})
