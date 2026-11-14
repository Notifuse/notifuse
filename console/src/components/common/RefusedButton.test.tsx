import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { RefusedButton } from './RefusedButton'

describe('RefusedButton', () => {
  it('acts and carries no refusal when it has none', () => {
    const onClick = vi.fn()
    render(<RefusedButton onClick={onClick}>Edit</RefusedButton>)
    const button = screen.getByRole('button', { name: 'Edit' })
    expect(button).not.toHaveAttribute('aria-disabled')
    expect(button).not.toHaveAttribute('aria-describedby')
    expect(button).not.toHaveClass('ant-btn-disabled')
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('stays focusable when refused, draws itself disabled, says why and does nothing', () => {
    const onClick = vi.fn()
    render(
      <RefusedButton refusal="You need write access" onClick={onClick}>
        Edit
      </RefusedButton>
    )
    const button = screen.getByRole('button', { name: 'Edit' })
    expect(button).not.toBeDisabled()
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(button).toHaveClass('ant-btn-disabled')
    expect(button).toHaveAccessibleDescription('You need write access')
    button.focus()
    expect(button).toHaveFocus()
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('refuses without a description while the reason is still loading', () => {
    const onClick = vi.fn()
    render(
      <RefusedButton refusal="" onClick={onClick}>
        Edit
      </RefusedButton>
    )
    const button = screen.getByRole('button', { name: 'Edit' })
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(button).not.toHaveAttribute('aria-describedby')
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
  })
})
