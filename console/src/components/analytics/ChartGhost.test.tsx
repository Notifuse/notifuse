import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ChartGhost } from './ChartGhost'

describe('ChartGhost', () => {
  it('shows its message and its action', () => {
    render(<ChartGhost message="Your emails will show here" action={<button type="button">New broadcast</button>} />)
    expect(screen.getByText('Your emails will show here')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'New broadcast' })).toBeInTheDocument()
  })

  it('hides its bars from assistive technologies', () => {
    const { container } = render(<ChartGhost message="Nothing yet" />)
    const bars = container.querySelector('[aria-hidden="true"]') as HTMLElement
    expect(bars.querySelectorAll('span').length).toBeGreaterThan(10)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
