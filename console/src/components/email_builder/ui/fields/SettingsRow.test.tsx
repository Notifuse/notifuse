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

  it('shows the dot and the reset only when set', () => {
    const onReset = vi.fn()
    const { rerender } = render(
      <SettingsRow label="Width" onReset={onReset}>
        <input aria-label="Width" />
      </SettingsRow>
    )
    expect(screen.queryByTestId('set-dot')).toBeNull()
    expect(screen.queryByRole('button', { name: /Reset Width/ })).toBeNull()

    rerender(
      <SettingsRow label="Width" isSet onReset={onReset}>
        <input aria-label="Width" />
      </SettingsRow>
    )
    expect(screen.getByTestId('set-dot')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Reset Width to the inherited value' }))
    expect(onReset).toHaveBeenCalledTimes(1)
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
