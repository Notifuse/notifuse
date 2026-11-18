import { useState } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SegmentColorPicker } from './SegmentColorPicker'

const renderPicker = (initial?: string) => {
  const latest: { value?: string } = { value: initial }
  const Wrapper = () => {
    const [value, setValue] = useState(initial)
    return (
      <SegmentColorPicker
        value={value}
        onChange={(next) => {
          latest.value = next
          setValue(next)
        }}
      />
    )
  }
  render(
    <I18nProvider i18n={i18n}>
      <Wrapper />
    </I18nProvider>
  )
  return latest
}

describe('SegmentColorPicker', () => {
  it('names the current colour, translated', () => {
    renderPicker('geekblue')
    expect(screen.getByRole('button', { name: 'Colour: Indigo' })).toBeTruthy()
  })

  it('opens the 12 presets on click, the current one checked', async () => {
    renderPicker('red')
    fireEvent.click(screen.getByRole('button', { name: 'Colour: Red' }))
    const radios = await screen.findAllByRole('radio')
    expect(radios).toHaveLength(12)
    expect(screen.getByRole('radio', { name: 'Red' })).toHaveAttribute('aria-checked', 'true')
  })

  it('picks a colour with the mouse', async () => {
    const latest = renderPicker('blue')
    fireEvent.click(screen.getByRole('button', { name: 'Colour: Blue' }))
    fireEvent.click(await screen.findByRole('radio', { name: 'Green' }))
    expect(latest.value).toBe('green')
  })

  it('is one tab stop: arrow keys move, Enter picks', async () => {
    const latest = renderPicker('blue')
    fireEvent.click(screen.getByRole('button', { name: 'Colour: Blue' }))
    const blue = await screen.findByRole('radio', { name: 'Blue' })
    expect(blue).toHaveAttribute('tabindex', '0')
    expect(screen.getByRole('radio', { name: 'Red' })).toHaveAttribute('tabindex', '-1')

    fireEvent.keyDown(blue, { key: 'ArrowRight' })
    const next = screen.getByRole('radio', { name: 'Indigo' })
    await waitFor(() => expect(document.activeElement).toBe(next))
    fireEvent.click(next)
    expect(latest.value).toBe('geekblue')
  })

  it('shows a colour outside the presets by its own name', () => {
    renderPicker('#123456')
    expect(screen.getByRole('button', { name: 'Colour: #123456' })).toBeTruthy()
  })
})

vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
