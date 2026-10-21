import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import StringPopoverInput from './StringPopoverInput'

describe('StringPopoverInput', () => {
  it.each(['https://example.com/a', 'mailto:team@example.com', 'tel:+33100000000'])(
    'links %s',
    (value) => {
      render(<StringPopoverInput value={value} onChange={vi.fn()} validateUri />)

      expect(screen.getByRole('link', { name: value })).toHaveAttribute('href', value)
    }
  )

  // An email may carry these, but following one from the console would hand the
  // click to an app on the member's computer
  it.each([
    'javascript:window.__probe=1',
    'data:text/html,<p>x</p>',
    'search-ms:query=invoice',
    'webcal://example.com/cal.ics'
  ])(
    'shows %s as text, not as a link',
    (value) => {
      render(<StringPopoverInput value={value} onChange={vi.fn()} validateUri />)

      expect(screen.getByText(value)).toBeInTheDocument()
      expect(screen.queryByRole('link')).toBeNull()
    }
  )
})
