import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import BorderRadiusInput from './BorderRadiusInput'

describe('BorderRadiusInput', () => {
  it('shows a px value in the number field and writes a typed number in px', () => {
    const onChange = vi.fn()
    render(<BorderRadiusInput value="8px" onChange={onChange} />)
    const input = screen.getByRole('spinbutton') as HTMLInputElement
    expect(input.value).toBe('8')
    fireEvent.change(input, { target: { value: '12' } })
    expect(onChange).toHaveBeenLastCalledWith('12px')
  })

  it.each(['50%', '10px 0', '4px / 8px'])('shows %s as text, which a render without a change keeps', (value) => {
    const onChange = vi.fn()
    render(<BorderRadiusInput value={value} onChange={onChange} />)
    expect((screen.getByLabelText('Border radius') as HTMLInputElement).value).toBe(value)
    expect(screen.queryByRole('spinbutton')).toBeNull()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('writes the text typed, and nothing for an empty text', () => {
    const onChange = vi.fn()
    render(<BorderRadiusInput value="50%" onChange={onChange} />)
    const input = screen.getByLabelText('Border radius')
    fireEvent.change(input, { target: { value: '25%' } })
    expect(onChange).toHaveBeenLastCalledWith('25%')
    fireEvent.change(input, { target: { value: ' ' } })
    expect(onChange).toHaveBeenLastCalledWith(undefined)
  })

  it('goes back to the number field, which writes px', () => {
    const onChange = vi.fn()
    render(<BorderRadiusInput value="50%" onChange={onChange} />)
    fireEvent.click(screen.getByText('px'))
    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '6' } })
    expect(onChange).toHaveBeenLastCalledWith('6px')
  })

  it('keeps the text field while a value typed passes through a number', () => {
    const onChange = vi.fn()
    const { rerender } = render(<BorderRadiusInput value="50%" onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('Border radius'), { target: { value: '50' } })
    rerender(<BorderRadiusInput value="50" onChange={onChange} />)
    expect((screen.getByLabelText('Border radius') as HTMLInputElement).value).toBe('50')
    expect(screen.queryByRole('spinbutton')).toBeNull()
  })
})
