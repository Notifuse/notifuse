import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import ImportFontInput from './ImportFontInput'

describe('ImportFontInput', () => {
  it('links a font stylesheet address', () => {
    const href = 'https://fonts.googleapis.com/css2?family=Inter'
    render(<ImportFontInput value={{ name: 'Inter', href }} onChange={vi.fn()} />)

    expect(screen.getByRole('link', { name: href })).toHaveAttribute('href', href)
  })

  it('shows any other address as text, not as a link', () => {
    const href = 'javascript:window.__probe=1'
    render(<ImportFontInput value={{ name: 'Inter', href }} onChange={vi.fn()} />)

    expect(screen.getByText(href)).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
  })
})
