import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RateBar } from './RateBar'

const fill = (container: HTMLElement) => container.querySelector<HTMLElement>('[data-rate-fill]')

describe('RateBar', () => {
  it('fills the bar with the rate and writes it', () => {
    const { container } = render(<RateBar rate={0.493} />)
    expect(fill(container)!.style.width).toBe('49.3%')
    expect(screen.getByText('49.3%')).toBeInTheDocument()
  })

  it('fills relative to a scale, capped at the full bar', () => {
    const { container, rerender } = render(<RateBar rate={0.1} scale={0.2} />)
    expect(fill(container)!.style.width).toBe('50%')
    rerender(<RateBar rate={0.3} scale={0.2} />)
    expect(fill(container)!.style.width).toBe('100%')
  })

  it('shows a dash without a bar when there is no rate', () => {
    const { container } = render(<RateBar rate={null} />)
    expect(screen.getByText('—')).toBeInTheDocument()
    expect(fill(container)).toBeNull()
  })

  it('lightens a partial rate and warns in amber', () => {
    const { container, rerender } = render(<RateBar rate={0.2} tone="partial" />)
    expect(fill(container)!.className).toContain('bg-primary-300')
    expect(screen.getByText('20.0%').className).toContain('text-ink-3')
    rerender(<RateBar rate={0.2} tone="warning" />)
    expect(screen.getByText('20.0%').className).toContain('text-warning-fg')
  })
})
