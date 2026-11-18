import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { DraftTextField } from './DraftTextField'

const field = () => screen.getByLabelText('Link') as HTMLInputElement
const type = (text: string) => fireEvent.change(field(), { target: { value: text } })

const setup = (props: Partial<React.ComponentProps<typeof DraftTextField>> = {}) => {
  const onChange = vi.fn()
  const utils = render(<DraftTextField aria-label="Link" value="https://a.test" onChange={onChange} {...props} />)
  return { onChange, ...utils }
}

describe('DraftTextField', () => {
  it('writes nothing while typing, once on blur', () => {
    const { onChange } = setup()
    fireEvent.focus(field())
    type('https://b.test')
    type('https://b.test/x')
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.blur(field())
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('https://b.test/x')
  })

  it('writes on Enter, and once only when blur follows', () => {
    const { onChange } = setup()
    fireEvent.focus(field())
    type('https://b.test')
    fireEvent.keyDown(field(), { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('https://b.test')
    fireEvent.blur(field())
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('writes nothing for an unchanged text, trimmed or not', () => {
    const { onChange } = setup()
    fireEvent.focus(field())
    type('  https://a.test ')
    fireEvent.blur(field())
    expect(onChange).not.toHaveBeenCalled()
  })

  it('reverts on Escape and writes nothing', () => {
    const { onChange } = setup()
    fireEvent.focus(field())
    type('https://b.test')
    fireEvent.keyDown(field(), { key: 'Escape' })
    expect(field().value).toBe('https://a.test')
    fireEvent.blur(field())
    expect(onChange).not.toHaveBeenCalled()
  })

  it('keeps an invalid URL in the field, in error, without writing it', () => {
    const { onChange } = setup({ validate: 'url' })
    fireEvent.focus(field())
    type('acme shop/offer')
    expect(screen.getByText(/Invalid URL format/)).toBeInTheDocument()
    fireEvent.blur(field())
    expect(onChange).not.toHaveBeenCalled()
    // Still there after the blur, until corrected
    expect(field().value).toBe('acme shop/offer')
    expect(screen.getByText(/Invalid URL format/)).toBeInTheDocument()
    fireEvent.focus(field())
    type('https://acme.shop/offer')
    fireEvent.blur(field())
    expect(onChange).toHaveBeenCalledWith('https://acme.shop/offer')
  })

  it('reverts a refused text on Escape', () => {
    const { onChange } = setup({ validate: 'url' })
    fireEvent.focus(field())
    type('acme shop/offer')
    fireEvent.blur(field())
    fireEvent.keyDown(field(), { key: 'Escape' })
    expect(field().value).toBe('https://a.test')
    expect(screen.queryByText(/Invalid URL format/)).toBeNull()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('accepts a Liquid value, without a link button', () => {
    const { onChange, rerender } = setup({ validate: 'url' })
    fireEvent.focus(field())
    type('{{ cart.url }}')
    fireEvent.blur(field())
    expect(onChange).toHaveBeenCalledWith('{{ cart.url }}')
    rerender(<DraftTextField aria-label="Link" validate="url" value="{{ cart.url }}" onChange={onChange} />)
    expect(screen.queryByRole('link', { name: 'Open the link' })).toBeNull()
  })

  it('offers the link of an openable URL, never of a javascript: one', () => {
    const { rerender } = setup({ validate: 'url' })
    expect(screen.getByRole('link', { name: 'Open the link' })).toHaveAttribute('href', 'https://a.test')
    rerender(<DraftTextField aria-label="Link" validate="url" value="javascript:alert(1)" onChange={vi.fn()} />)
    expect(screen.queryByRole('link', { name: 'Open the link' })).toBeNull()
  })

  it('follows the address linkOf gives for the saved value, and none when it gives null', () => {
    const { rerender } = setup({ value: '//a.test/x.css', linkOf: (value) => `https:${value}` })
    expect(screen.getByRole('link', { name: 'Open the link' })).toHaveAttribute('href', 'https://a.test/x.css')
    rerender(<DraftTextField aria-label="Link" value="https://a.test" linkOf={() => null} onChange={vi.fn()} />)
    expect(screen.queryByRole('link', { name: 'Open the link' })).toBeNull()
  })

  it('writes undefined for an emptied field', () => {
    const { onChange } = setup()
    fireEvent.focus(field())
    type('   ')
    fireEvent.blur(field())
    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('follows a new value while unfocused, never while being typed in', () => {
    const { rerender } = setup()
    rerender(<DraftTextField aria-label="Link" value="https://c.test" onChange={vi.fn()} />)
    expect(field().value).toBe('https://c.test')
    fireEvent.focus(field())
    type('https://typing.test')
    rerender(<DraftTextField aria-label="Link" value="https://d.test" onChange={vi.fn()} />)
    expect(field().value).toBe('https://typing.test')
  })

  it('writes a changed text when it goes away without a blur, and nothing when unchanged', () => {
    const first = setup()
    fireEvent.focus(field())
    type('https://b.test')
    first.unmount()
    expect(first.onChange).toHaveBeenCalledWith('https://b.test')

    const second = setup()
    fireEvent.focus(field())
    second.unmount()
    expect(second.onChange).not.toHaveBeenCalled()
  })

  it('adds a line on Enter when multiline, and writes on blur', () => {
    const onChange = vi.fn()
    render(<DraftTextField aria-label="Link" multiline value="Hello" onChange={onChange} />)
    fireEvent.focus(field())
    fireEvent.change(field(), { target: { value: 'Hello\nWorld' } })
    fireEvent.keyDown(field(), { key: 'Enter' })
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.blur(field())
    expect(onChange).toHaveBeenCalledWith('Hello\nWorld')
  })

  it('keeps the same input, and its focus, while the text turns invalid and valid again', () => {
    setup({ validate: 'url' })
    const input = field()
    input.focus()
    fireEvent.focus(input)
    type('h')
    expect(field()).toBe(input)
    expect(document.activeElement).toBe(input)
    type('https://b.test')
    expect(field()).toBe(input)
    expect(document.activeElement).toBe(input)
  })

  it('shows the Liquid example of its URL error with its braces', () => {
    setup({ validate: 'url' })
    fireEvent.focus(field())
    type('acme shop')
    expect(screen.getByText(/like \{\{ variable \}\}/)).toBeInTheDocument()
  })
})
