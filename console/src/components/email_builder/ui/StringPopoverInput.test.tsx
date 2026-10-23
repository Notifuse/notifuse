import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
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

  it('marks a value holding a Liquid output as Liquid', () => {
    render(<StringPopoverInput value="{{ cta_url }}" onChange={vi.fn()} validateUri />)

    expect(screen.getByText('(liquid)')).toBeInTheDocument()
  })

  it('does not mark a value holding only Liquid tags as Liquid, which its editor refuses', () => {
    const value = '{% if vip %}https://vip.example.com{% else %}https://example.com{% endif %}'
    render(<StringPopoverInput value={value} onChange={vi.fn()} validateUri />)

    expect(screen.getByText(value)).toBeInTheDocument()
    expect(screen.queryByText('(liquid)')).toBeNull()
    expect(screen.queryByRole('link')).toBeNull()
  })

  it('shows an address holding a Liquid tag as text, not as a link', () => {
    const value = 'https://x.example/{% if vip %}vip{% endif %}'
    render(<StringPopoverInput value={value} onChange={vi.fn()} validateUri />)

    expect(screen.getByText(value)).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
  })
})

describe('StringPopoverInput link value', () => {
  const enter = async (typed: string) => {
    const onChange = vi.fn()
    render(<StringPopoverInput onChange={onChange} validateUri />)
    fireEvent.click(screen.getByRole('button', { name: 'Set value' }))
    fireEvent.change(await screen.findByRole('textbox'), { target: { value: typed } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    return onChange
  }

  it('saves a Liquid output in place of an address', async () => {
    expect(await enter('{{ cta_url }}')).toHaveBeenCalledWith('{{ cta_url }}')
  })

  it('refuses a malformed address even when it holds a Liquid tag', async () => {
    const onChange = await enter('https//shop.example.com/{% if vip %}vip{% endif %}')
    expect(onChange).not.toHaveBeenCalled()
  })
})

