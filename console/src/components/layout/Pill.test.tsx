import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Pill } from './Pill'

describe('Pill', () => {
  it.each([
    ['neutral', 'bg-neutral-bg', 'text-neutral-fg'],
    ['primary', 'bg-primary-100', 'text-primary-700'],
    ['info', 'bg-info-bg', 'text-info-fg'],
    ['success', 'bg-success-bg', 'text-success-fg'],
    ['warning', 'bg-warning-bg', 'text-warning-fg'],
    ['danger', 'bg-danger-bg', 'text-danger-fg']
  ] as const)('paints the %s tone', (tone, bg, fg) => {
    render(<Pill tone={tone}>State</Pill>)
    const pill = screen.getByText('State')
    expect(pill.className).toContain(bg)
    expect(pill.className).toContain(fg)
  })

  it('shows a dot by default, none on request, and an icon in its place', () => {
    const { container, rerender } = render(<Pill tone="success">Sent</Pill>)
    expect(container.querySelector('[data-pill-dot]')).not.toBeNull()
    rerender(<Pill tone="success" dot={false}>Sent</Pill>)
    expect(container.querySelector('[data-pill-dot]')).toBeNull()
    rerender(<Pill tone="primary" icon={<svg data-testid="trophy" />}>Winner</Pill>)
    expect(container.querySelector('[data-pill-dot]')).toBeNull()
    expect(screen.getByTestId('trophy')).toBeInTheDocument()
  })

  it('draws a neutral dot in ink-4', () => {
    const { container } = render(<Pill tone="neutral">Draft</Pill>)
    expect(container.querySelector('[data-pill-dot]')!.className).toContain('text-ink-4')
  })

  it('pulses only when motion is allowed', () => {
    const { container } = render(
      <Pill tone="info" pulse>
        Sending
      </Pill>
    )
    expect(container.querySelector('[data-pill-dot]')!.className).toContain('motion-safe:animate-pill-pulse')
  })

  it('keeps its width by default, and shrinks with an ellipsis and its dot when truncated', () => {
    const { container, rerender } = render(<Pill tone="warning">Pick a winner</Pill>)
    const pill = container.firstElementChild!
    expect(pill.className).toContain('shrink-0')
    expect(pill.className).not.toContain('min-w-0')
    expect(screen.getByText('Pick a winner')).toBe(pill)

    rerender(
      <Pill tone="warning" truncate>
        Pick a winner
      </Pill>
    )
    expect(pill.className).toContain('min-w-0')
    expect(pill.className).toContain('max-w-full')
    expect(pill.className).not.toContain('shrink-0')
    expect(screen.getByText('Pick a winner').className).toContain('truncate')
    expect(container.querySelector('[data-pill-dot]')).not.toBeNull()
  })
})
