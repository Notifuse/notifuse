import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SettingsRow } from './SettingsRow'

describe('SettingsRow', () => {
  it('puts its help in an info button whose tooltip opens on focus', async () => {
    render(
      <SettingsRow label="Width" help="Up to 999 px.">
        <input aria-label="Width" />
      </SettingsRow>
    )
    const button = screen.getByRole('button', { name: 'About Width' })
    fireEvent.focus(button)
    expect(await screen.findByText('Up to 999 px.')).toBeInTheDocument()
    // On the left, over the canvas, never over the fields below
    expect(document.querySelector('.ant-tooltip')?.className).toMatch(/placement-left/)
  })

  it('marks the row and shows the reset only when set', () => {
    const onReset = vi.fn()
    const { rerender } = render(
      <SettingsRow label="Width" onReset={onReset}>
        <input aria-label="Width" />
      </SettingsRow>
    )
    expect(screen.getByTestId('settings-row')).not.toHaveAttribute('data-set')
    expect(screen.queryByRole('button', { name: /Reset Width/ })).toBeNull()

    rerender(
      <SettingsRow label="Width" isSet onReset={onReset}>
        <input aria-label="Width" />
      </SettingsRow>
    )
    expect(screen.getByTestId('settings-row')).toHaveAttribute('data-set', 'true')
    fireEvent.click(screen.getByRole('button', { name: 'Reset Width to the inherited value' }))
    expect(onReset).toHaveBeenCalledTimes(1)
  })

  it('draws no dot for a set row: the reset, in the primary colour, marks it', () => {
    render(
      <SettingsRow label="Width" isSet onReset={() => {}}>
        <input aria-label="Width" />
      </SettingsRow>
    )
    // Nothing drawn in the primary colour but the reset icon
    const row = screen.getByTestId('settings-row')
    const primary = [...row.querySelectorAll('*')].filter((el) => /(^|\s)(bg|text)-primary-/.test(el.getAttribute('class') ?? ''))
    expect(primary).toEqual([row.querySelector('button svg')])
    // On the icon: a colour class on the button loses to the console's button reset
    expect(screen.getByRole('button', { name: 'Reset Width to the inherited value' }).querySelector('svg')).toHaveClass(
      'text-primary-500'
    )
  })

  it("keeps the reset's place on a row that is not set, so fields keep one width", () => {
    render(
      <SettingsRow label="Width">
        <input aria-label="Width" />
      </SettingsRow>
    )
    expect(screen.getByTestId('reset-slot')).toBeEmptyDOMElement()
  })

  it('has no reset slot on a vertical row: its reset sits beside the label', () => {
    render(
      <SettingsRow label="Spacing" vertical isSet onReset={() => {}}>
        <div />
      </SettingsRow>
    )
    expect(screen.queryByTestId('reset-slot')).toBeNull()
    expect(screen.getByRole('button', { name: /Reset Spacing/ })).toBeInTheDocument()
  })

  it('keeps an alert visible under the field', () => {
    render(
      <SettingsRow label="Width" alert={<span>Too wide</span>}>
        <input aria-label="Width" />
      </SettingsRow>
    )
    expect(screen.getByText('Too wide')).toBeVisible()
  })
})
