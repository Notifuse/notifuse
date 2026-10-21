import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import StringPopoverInput from './StringPopoverInput'

describe('StringPopoverInput', () => {
  it('links a web address', () => {
    render(<StringPopoverInput value="https://example.com/a" onChange={vi.fn()} validateUri />)

    expect(screen.getByRole('link', { name: 'https://example.com/a' })).toHaveAttribute(
      'href',
      'https://example.com/a'
    )
  })

  it.each(['javascript:window.__probe=1', 'data:text/html,<p>x</p>'])(
    'shows %s as text, not as a link',
    (value) => {
      render(<StringPopoverInput value={value} onChange={vi.fn()} validateUri />)

      expect(screen.getByText(value)).toBeInTheDocument()
      expect(screen.queryByRole('link')).toBeNull()
    }
  )
})
