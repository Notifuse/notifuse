import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import ImportFontInput from './ImportFontInput'

describe('ImportFontInput', () => {
  it('links a font stylesheet address', () => {
    const href = 'https://fonts.googleapis.com/css2?family=Inter'
    render(<ImportFontInput value={{ name: 'Inter', href }} onChange={vi.fn()} />)

    expect(screen.getByRole('link', { name: href })).toHaveAttribute('href', href)
  })

  it('links a protocol-relative address, as a web address', () => {
    const href = '//fonts.googleapis.com/css?family=Lato'
    render(<ImportFontInput value={{ name: 'Lato', href }} onChange={vi.fn()} />)

    expect(screen.getByRole('link', { name: href })).toHaveAttribute(
      'href',
      'https://fonts.googleapis.com/css?family=Lato'
    )
  })

  it.each([
    'javascript:window.__probe=1',
    'data:text/css,body{}',
    'webcal://example.com/fonts.css',
    'https://{{ cdn_host }}/fonts.css',
    'https://{{cdn_host}}/fonts.css',
    '{% if dark %}https://example.com/dark.css{% endif %}'
  ])('shows any other address as text, not as a link: %s', (href) => {
    render(<ImportFontInput value={{ name: 'Inter', href }} onChange={vi.fn()} />)

    expect(screen.getByText(href)).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
  })
})

describe('ImportFontInput stylesheet address', () => {
  const enter = async (href: string) => {
    const onChange = vi.fn()
    render(<ImportFontInput onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Import Font' }))
    fireEvent.change(await screen.findByPlaceholderText('e.g., Raleway, Open Sans'), {
      target: { value: 'Inter' }
    })
    fireEvent.change(screen.getByPlaceholderText(/^https:\/\/fonts\.googleapis\.com/), {
      target: { value: href }
    })
    const buttons = screen.getAllByRole('button', { name: 'Import Font' })
    fireEvent.click(buttons[buttons.length - 1])
    return onChange
  }

  it('imports a Liquid output in place of an address', async () => {
    expect(await enter('{{ font_url }}')).toHaveBeenCalledWith({
      name: 'Inter',
      href: '{{ font_url }}'
    })
  })

  it('imports a protocol-relative address, which the panel links', async () => {
    expect(await enter('//fonts.googleapis.com/css?family=Lato')).toHaveBeenCalledWith({
      name: 'Inter',
      href: '//fonts.googleapis.com/css?family=Lato'
    })
  })

  it.each([
    'https://example.com/{% if dark %}dark{% endif %}.css',
    '//cdn.example.com/{% if dark %}dark{% endif %}.css',
    'https://{{ cdn_host }}/fonts.css'
  ])('imports a web address holding Liquid: %s', async (href) => {
    expect(await enter(href)).toHaveBeenCalledWith({ name: 'Inter', href })
  })

  it.each(['javascript:{{ code }}', 'javascript:{% if x %}y{% endif %}', 'data:text/css,{{ css }}'])(
    'refuses an address of another scheme, Liquid or not: %s',
    async (href) => {
      expect(await enter(href)).not.toHaveBeenCalled()
    }
  )

  it('imports an address pasted after a no-break space, as it saves it', async () => {
    expect(await enter('\u00a0https://fonts.googleapis.com/css?family=Lato')).toHaveBeenCalledWith({
      name: 'Inter',
      href: 'https://fonts.googleapis.com/css?family=Lato'
    })
  })

  it("imports an address on the console's own origin", async () => {
    const href = `${window.location.origin}/fonts/brand.css`
    expect(await enter(href)).toHaveBeenCalledWith({ name: 'Inter', href })
  })

  it.each(['/fonts/inter.css', 'javascript:alert(1)', 'data:text/css,body{}', 'webcal://example.com/a.css'])(
    'refuses an address that is no web address: %s',
    async (href) => {
      expect(await enter(href)).not.toHaveBeenCalled()
    }
  )

  it('refuses a malformed address even when it holds a Liquid tag', async () => {
    const onChange = await enter('https//example.com/{% if dark %}dark{% endif %}.css')
    expect(onChange).not.toHaveBeenCalled()
  })
})

