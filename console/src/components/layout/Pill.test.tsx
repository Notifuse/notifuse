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
})
