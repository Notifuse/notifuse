import { describe, it, expect } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { HelpTip } from './HelpTip'

describe('HelpTip', () => {
  it('is a button the keyboard reaches, whose tooltip opens on focus', async () => {
    render(<HelpTip label="About segments">Leave empty for the whole list.</HelpTip>)
    const button = screen.getByRole('button', { name: 'About segments' })
    expect(button).toHaveAttribute('type', 'button')
    button.focus()
    expect(button).toHaveFocus()
    fireEvent.focus(button)
    expect(await screen.findByText('Leave empty for the whole list.')).toBeInTheDocument()
  })

  it('sets its title beside the "?" when given one', () => {
    render(
      <HelpTip title="Clicked links" label="About these numbers">
        Text
      </HelpTip>
    )
    const button = screen.getByRole('button', { name: 'About these numbers' })
    expect(button.parentElement).toHaveTextContent('Clicked links')
  })

  it('keeps a hidden copy under the id a control can point to, only when given one', () => {
    const { container, rerender } = render(<HelpTip label="About segments">Text</HelpTip>)
    expect(container.querySelector('[hidden]')).toBeNull()
    rerender(
      <>
        <HelpTip label="About segments" id="help-1">
          Text
        </HelpTip>
        <input aria-describedby="help-1" aria-label="Segments" />
      </>
    )
    expect(container.querySelector('#help-1')).toHaveAttribute('hidden')
    expect(screen.getByRole('textbox', { name: 'Segments' })).toHaveAccessibleDescription('Text')
  })

  it('shows a "?" by default and an "i" with icon="info", with the same name and tooltip', async () => {
    const { container, rerender } = render(<HelpTip label="About the width">Up to 999 px.</HelpTip>)
    expect(container.querySelector('svg')).toHaveAttribute('data-icon', 'help')
    rerender(
      <HelpTip label="About the width" icon="info">
        Up to 999 px.
      </HelpTip>
    )
    expect(container.querySelector('svg')).toHaveAttribute('data-icon', 'info')
    const button = screen.getByRole('button', { name: 'About the width' })
    fireEvent.focus(button)
    expect(await screen.findByText('Up to 999 px.')).toBeInTheDocument()
  })
})
