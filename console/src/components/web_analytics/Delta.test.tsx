import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { Delta } from './Delta'
import { deltaTone } from './lib/delta'

/** The rendered Delta, its arrow icon's lucide class, and its text. */
function renderDelta(props: React.ComponentProps<typeof Delta>) {
  const { container } = render(<Delta {...props} />)
  const span = container.firstElementChild as HTMLElement | null
  const icon = span?.querySelector('svg')
  return { container, span, icon, text: span?.textContent ?? '' }
}

describe('Delta', () => {
  it.each([
    ['zero', 0],
    ['undefined', undefined],
    ['NaN', Number.NaN]
  ])('renders nothing for %s', (_, change) => {
    const { container } = renderDelta({ change })
    expect(container).toBeEmptyDOMElement()
  })

  it('points up in the success colour for a rise', () => {
    const { span, icon, text } = renderDelta({ change: 12 })
    expect(span).toHaveClass('text-success-fg')
    expect(span).not.toHaveClass('text-danger-fg')
    expect(icon).toHaveClass('lucide-chevron-up')
    expect(text).toBe('12%')
  })

  it('points down in the danger colour for a fall', () => {
    const { span, icon, text } = renderDelta({ change: -12 })
    expect(span).toHaveClass('text-danger-fg')
    expect(icon).toHaveClass('lucide-chevron-down')
    expect(text).toBe('12%')
  })

  it('keeps the arrow on the sign but colours a fall as good with invertTrend', () => {
    const { span, icon } = renderDelta({ change: -12, invertTrend: true })
    expect(span).toHaveClass('text-success-fg')
    expect(icon).toHaveClass('lucide-chevron-down')
  })

  it('colours a rise as bad with invertTrend', () => {
    const { span, icon } = renderDelta({ change: 12, invertTrend: true })
    expect(span).toHaveClass('text-danger-fg')
    expect(icon).toHaveClass('lucide-chevron-up')
  })

  it('takes its size from the text tokens rather than an inline style', () => {
    const regular = renderDelta({ change: 5 }).span
    expect(regular).toHaveClass('text-xs')
    expect(regular?.getAttribute('style')).toBeNull()
    const small = renderDelta({ change: 5, small: true }).span
    expect(small).toHaveClass('text-2xs')
  })
})

describe('deltaTone', () => {
  it('follows whether the change is good, not its sign', () => {
    expect(deltaTone(12)).toBe('positive')
    expect(deltaTone(-12)).toBe('negative')
    expect(deltaTone(-12, true)).toBe('positive')
    expect(deltaTone(12, true)).toBe('negative')
  })

  it('is neutral when the Delta would print nothing', () => {
    expect(deltaTone(0)).toBe('neutral')
    expect(deltaTone(undefined)).toBe('neutral')
    expect(deltaTone(Number.NaN)).toBe('neutral')
  })
})
