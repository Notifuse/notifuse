import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, render, screen, fireEvent } from '@testing-library/react'
import { KpiStrip, type KpiItem } from './KpiStrip'

type Callback = (entries: { contentRect: { width: number } }[]) => void
let observers: Callback[] = []
const originalResizeObserver = globalThis.ResizeObserver

function resizeTo(width: number) {
  act(() => {
    observers.forEach((callback) => callback([{ contentRect: { width } }]))
  })
}

const items = (n: number): KpiItem[] =>
  Array.from({ length: n }, (_, i) => ({ key: `k${i}`, label: `Label ${i}`, value: `${i * 10}` }))

function grid(container: HTMLElement) {
  return container.querySelector<HTMLElement>('[data-kpi-grid]')!
}

describe('KpiStrip', () => {
  beforeEach(() => {
    observers = []
    globalThis.ResizeObserver = class {
      constructor(callback: Callback) {
        observers.push(callback)
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver
  })

  afterEach(() => {
    globalThis.ResizeObserver = originalResizeObserver
  })

  it('lays out one column per KPI while each keeps 140px', () => {
    const { container } = render(<KpiStrip items={items(5)} />)
    resizeTo(700)
    expect(grid(container).style.gridTemplateColumns).toBe('repeat(5, minmax(0, 1fr))')
    expect(screen.getByText('Label 0')).toBeInTheDocument()
    expect(screen.getByText('40')).toBeInTheDocument()
  })

  it('wraps onto ceil(N/2) columns under 140px a cell, the last cell spanning the rest', () => {
    const { container } = render(<KpiStrip items={items(5)} />)
    resizeTo(699)
    expect(grid(container).style.gridTemplateColumns).toBe('repeat(3, minmax(0, 1fr))')
    const cells = container.querySelectorAll<HTMLElement>('[data-kpi-cell]')
    expect(cells[4].style.gridColumn).toBe('span 2 / span 2')
    expect(cells[3].style.gridColumn).toBe('')
  })

  it('does not stretch the last cell when the wrapped rows are full', () => {
    const { container } = render(<KpiStrip items={items(4)} />)
    resizeTo(400)
    expect(grid(container).style.gridTemplateColumns).toBe('repeat(2, minmax(0, 1fr))')
    const cells = container.querySelectorAll<HTMLElement>('[data-kpi-cell]')
    expect(cells[3].style.gridColumn).toBe('')
  })

  it('counts the aside as a cell when it decides to wrap, and puts it under the grid once wrapped', () => {
    const { container } = render(<KpiStrip items={items(3)} aside={<span>Bounces</span>} />)
    resizeTo(560)
    const aside = container.querySelector<HTMLElement>('[data-kpi-aside]')!
    expect(screen.getByText('Bounces')).toBeInTheDocument()
    expect(aside.className).toContain('bg-subtle')
    expect(container.querySelector('[data-kpi-strip]')!.className).not.toContain('flex-col')
    resizeTo(559)
    expect(container.querySelector('[data-kpi-strip]')!.className).toContain('flex-col')
  })

  it('makes a clickable cell a button with aria-pressed', () => {
    const onClick = vi.fn()
    render(
      <KpiStrip
        items={[
          { key: 'errors', label: 'Errors', value: '12', onClick, active: true },
          { key: 'all', label: 'All', value: '40', onClick: vi.fn() },
          { key: 'static', label: 'Static', value: '3' }
        ]}
      />
    )
    const errors = screen.getByRole('button', { name: /Errors/ })
    expect(errors).toHaveAttribute('aria-pressed', 'true')
    expect(errors.className).toContain('bg-primary-50')
    expect(errors.querySelector('[data-kpi-active-bar]')).not.toBeNull()
    expect(screen.getByRole('button', { name: /All/ })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.queryByRole('button', { name: /Static/ })).toBeNull()
    fireEvent.click(errors)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('draws an arrow on each separator with connector="arrow", and hides them once wrapped', () => {
    const { container } = render(<KpiStrip items={items(4)} connector="arrow" />)
    resizeTo(800)
    expect(container.querySelectorAll('[data-kpi-arrow]')).toHaveLength(3)
    resizeTo(500)
    expect(container.querySelectorAll('[data-kpi-arrow]')).toHaveLength(0)
  })

  it('draws no arrow without connector', () => {
    const { container } = render(<KpiStrip items={items(4)} />)
    resizeTo(800)
    expect(container.querySelectorAll('[data-kpi-arrow]')).toHaveLength(0)
  })

  it('replaces each value with a skeleton while loading', () => {
    const { container } = render(<KpiStrip items={items(3)} loading />)
    expect(container.querySelectorAll('.ant-skeleton')).toHaveLength(3)
    expect(screen.queryByText('20')).toBeNull()
    expect(screen.getByText('Label 2')).toBeInTheDocument()
  })

  it('draws a border and a shadow unless embedded', () => {
    const { container, rerender } = render(<KpiStrip items={items(2)} />)
    const box = () => container.querySelector<HTMLElement>('[data-kpi-strip]')!
    expect(box().className).toContain('border-line')
    expect(box().className).toContain('shadow-card')
    rerender(<KpiStrip items={items(2)} embedded />)
    expect(box().className).not.toContain('border-line')
    expect(box().className).not.toContain('shadow-card')
  })

  it.each([
    ['neutral', 'text-ink-3'],
    ['positive', 'text-success-fg'],
    ['negative', 'text-danger-fg'],
    ['warning', 'text-warning-fg']
  ] as const)('paints a %s sub in %s', (tone, className) => {
    render(<KpiStrip items={[{ key: 'a', label: 'Open rate', value: '49%', sub: 'vs last', tone }]} />)
    expect(screen.getByText('vs last').className).toContain(className)
  })

  it('renders the trend slot and a hint tooltip on the value', async () => {
    render(
      <KpiStrip
        items={[
          {
            key: 'a',
            label: 'Open rate',
            value: '49%',
            hint: 'Partial tracking',
            trend: <svg data-testid="spark" />
          }
        ]}
      />
    )
    expect(screen.getByTestId('spark')).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByText('49%'))
    expect(await screen.findByText('Partial tracking')).toBeInTheDocument()
  })
})
