import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { SendingProgress } from './SendingProgress'

afterEach(() => {
  i18n.activate('en')
})

describe('SendingProgress', () => {
  it('writes the counts with the separators of the console language', () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    render(<SendingProgress enqueuedCount={5678} sentCount={1234} failedCount={0} />)

    // French groups thousands with a narrow no-break space, which \s matches.
    expect(screen.getByText(/^1\s234 of 5\s678$/)).toBeInTheDocument()
  })

  it('takes what is left from the queue when it knows, else from the counts', () => {
    const { rerender } = render(<SendingProgress compact enqueuedCount={100} sentCount={30} failedCount={10} />)
    expect(screen.getByText('40.0%')).toBeInTheDocument()
    rerender(<SendingProgress compact enqueuedCount={100} sentCount={100} failedCount={0} remainingOverride={23} />)
    expect(screen.getByText('77.0%')).toBeInTheDocument()
  })

  it('in a table cell, shows the bar and its share, the counts in a tooltip', async () => {
    const { container } = render(<SendingProgress compact enqueuedCount={200} sentCount={50} failedCount={0} />)
    expect(screen.getByText('25.0%')).toBeInTheDocument()
    expect(container.querySelector<HTMLElement>('[data-rate-fill]')!.style.width).toBe('25%')
    // Only read out with the bar until the tooltip opens.
    const bar = container.querySelector<HTMLElement>('[data-sending-progress]')!
    expect(screen.getByText('Sending emails...').closest('.sr-only')).not.toBeNull()
    fireEvent.mouseEnter(bar)
    await waitFor(() => expect(document.querySelector('.ant-tooltip')).toHaveTextContent('50 of 200'))
  })

  it('never reads 100 % while some are left to send', () => {
    render(<SendingProgress compact enqueuedCount={10000} sentCount={9997} failedCount={0} />)
    expect(screen.getByText('99.9%')).toBeInTheDocument()
  })

  it('shows its fallback once nothing is left to send', () => {
    render(<SendingProgress compact enqueuedCount={10} sentCount={10} failedCount={0} fallback={<span>Nov 10</span>} />)
    expect(screen.getByText('Nov 10')).toBeInTheDocument()
  })

  it('keeps the full widget outside a table cell', () => {
    render(<SendingProgress enqueuedCount={200} sentCount={50} failedCount={0} />)
    expect(screen.getByText('Sending emails...')).toBeInTheDocument()
    expect(screen.getByText('50 of 200')).toBeInTheDocument()
  })

  it('renders nothing once nothing is left, in both forms', () => {
    const { container, rerender } = render(<SendingProgress enqueuedCount={10} sentCount={10} failedCount={0} />)
    expect(container).toBeEmptyDOMElement()
    rerender(<SendingProgress compact enqueuedCount={10} sentCount={10} failedCount={0} />)
    expect(container).toBeEmptyDOMElement()
  })
})
