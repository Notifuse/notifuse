import { describe, it, expect } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
import type { EmailBlock } from '../types'
import { blur, fresh, openSection, renderPanel, setRows } from './settingsPanelTestUtils'

describe('Button settings panel', () => {
  const button = (attributes: Record<string, unknown> = {}) =>
    ({ id: 'button-1', type: 'mj-button', attributes, content: 'Buy' }) as unknown as EmailBlock

  it('shows no « set » mark on a button straight out of createBlock', () => {
    renderPanel(fresh('mj-button'))
    expect(setRows()).toHaveLength(0)
  })

  it('writes the link, where it opens, its rel and its tooltip', () => {
    const { onUpdate } = renderPanel(button())
    blur('Link URL', 'https://acme.test/cart')
    expect(onUpdate).toHaveBeenLastCalledWith({ href: 'https://acme.test/cart' }, undefined)
    blur('Rel', 'noopener')
    expect(onUpdate).toHaveBeenLastCalledWith({ rel: 'noopener' }, undefined)
    blur('Tooltip', 'Go to the cart')
    expect(onUpdate).toHaveBeenLastCalledWith({ title: 'Go to the cart' }, undefined)
  })

  it('refuses an invalid link without writing it', () => {
    const { onUpdate } = renderPanel(button())
    blur('Link URL', 'acme cart')
    expect(onUpdate).not.toHaveBeenCalled()
    expect(screen.getByText(/Invalid URL format/)).toBeInTheDocument()
  })

  it('writes a typed width with a coalesce key, in px', () => {
    const { onUpdate } = renderPanel(button({ width: '200px' }), {})
    openSection(/Size and border/)
    const width = screen.getByLabelText('Button width')
    fireEvent.focus(width)
    fireEvent.change(width, { target: { value: '120' } })
    const [updates, options] = onUpdate.mock.calls.at(-1)!
    expect(updates).toEqual({ width: '120px' })
    expect(options?.coalesceKey).toMatch(/^button-1:width:/)
  })

  it('marks a value that differs from the inherited one, and resets it to the inherited value', () => {
    const { onUpdate } = renderPanel(button({ href: 'https://acme.test' }))
    const row = screen.getByLabelText('Link URL').closest('[data-testid="settings-row"]') as HTMLElement
    expect(row).toHaveAttribute('data-set', 'true')
    fireEvent.click(within(row).getByRole('button', { name: /Reset URL/ }))
    // The reset writes the inherited value (the editor's default link is empty)
    expect(onUpdate).toHaveBeenLastCalledWith({ href: '' })
  })

  it("takes the type's mj-attributes as inherited values: equal is not marked", () => {
    renderPanel(button({ backgroundColor: '#123456' }), { backgroundColor: '#123456' })
    const colors = screen.getByRole('button', { name: /Colors/ })
    expect(colors).toBeInTheDocument()
    expect(setRows()).toHaveLength(0)
  })

  it('writes the inner padding as a shorthand string, never an object', () => {
    const { onUpdate } = renderPanel(button())
    openSection(/Spacing/)
    const inner = screen.getByRole('group', { name: 'Inside the button' })
    const top = within(inner).getByLabelText('Top')
    fireEvent.focus(top)
    fireEvent.change(top, { target: { value: '14' } })
    const [updates] = onUpdate.mock.calls.at(-1)!
    expect(typeof updates.innerPadding).toBe('string')
    expect(updates.innerPadding).toMatch(/^14px/)
  })
})
