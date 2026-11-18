import { describe, it, expect, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { CornersField } from './CornersField'

const corners = (value?: string, inherited?: string) => {
  const onChange = vi.fn()
  const view = render(
    <CornersField aria-label="Corners" coalesceBase="block-1:borderRadius" value={value} inherited={inherited} onChange={onChange} />
  )
  return { onChange, ...view }
}

const text = () => screen.getByRole('textbox', { name: 'Corners' }) as HTMLInputElement
const number = () => screen.getByRole('spinbutton', { name: 'Corners' }) as HTMLInputElement

describe('CornersField', () => {
  it('shows a px value in the number field', () => {
    corners('8px')
    expect(number().value).toBe('8')
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('writes a typed number in px with the focus session key', () => {
    const { onChange } = corners('8px')
    act(() => number().focus())
    fireEvent.change(number(), { target: { value: '1' } })
    fireEvent.change(number(), { target: { value: '12' } })
    expect(onChange).toHaveBeenNthCalledWith(1, '1px', { coalesceKey: expect.any(String) })
    expect(onChange).toHaveBeenNthCalledWith(2, '12px', { coalesceKey: expect.any(String) })
    expect(onChange.mock.calls[0][1].coalesceKey).toBe(onChange.mock.calls[1][1].coalesceKey)
    expect(onChange.mock.calls[0][1].coalesceKey).toMatch(/^block-1:borderRadius:/)
  })

  it('writes undefined when the number is emptied', () => {
    const { onChange } = corners('8px')
    fireEvent.change(number(), { target: { value: '' } })
    expect(onChange).toHaveBeenLastCalledWith(undefined, { coalesceKey: expect.any(String) })
  })

  it('shows the inherited radius as the placeholder while unset', () => {
    corners(undefined, '4px')
    expect(number().value).toBe('')
    expect(number().placeholder).toBe('4')
  })

  it.each(['50%', '10px 0', '4px / 8px'])('shows %s as text, which a render without a change keeps', (value) => {
    const { onChange, rerender } = corners(value)
    expect(text().value).toBe(value)
    expect(screen.queryByRole('spinbutton')).toBeNull()
    rerender(<CornersField aria-label="Corners" coalesceBase="b" value={value} onChange={onChange} />)
    expect(text().value).toBe(value)
    expect(onChange).not.toHaveBeenCalled()
  })

  it('writes the text once on blur, nothing while it is typed', () => {
    const { onChange } = corners('50%')
    act(() => text().focus())
    fireEvent.change(text(), { target: { value: '25' } })
    fireEvent.change(text(), { target: { value: '25%' } })
    expect(onChange).not.toHaveBeenCalled()
    act(() => text().blur())
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('25%')
  })

  it('writes the text on Enter, and the blur after it writes nothing more', () => {
    const { onChange } = corners('50%')
    act(() => text().focus())
    fireEvent.change(text(), { target: { value: '10px 0' } })
    fireEvent.keyDown(text(), { key: 'Enter' })
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('10px 0')
    act(() => text().blur())
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('keeps the text field while the draft reads as a number', () => {
    corners('50%')
    act(() => text().focus())
    fireEvent.change(text(), { target: { value: '50' } })
    expect(text().value).toBe('50')
    expect(screen.queryByRole('spinbutton')).toBeNull()
  })

  it('reverts the draft on Escape and writes nothing', () => {
    const { onChange } = corners('50%')
    act(() => text().focus())
    fireEvent.change(text(), { target: { value: '30%' } })
    fireEvent.keyDown(text(), { key: 'Escape' })
    expect(text().value).toBe('50%')
    act(() => text().blur())
    expect(onChange).not.toHaveBeenCalled()
  })

  it('writes undefined for an emptied text, without a coalesce key', () => {
    const { onChange } = corners('50%')
    act(() => text().focus())
    fireEvent.change(text(), { target: { value: ' ' } })
    act(() => text().blur())
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('goes back to the number field, which writes px', () => {
    const { onChange } = corners('50%', '6px')
    fireEvent.click(screen.getByRole('button', { name: 'Enter a number of pixels instead' }))
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(number().placeholder).toBe('6')
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.change(number(), { target: { value: '6' } })
    expect(onChange).toHaveBeenLastCalledWith('6px', { coalesceKey: expect.any(String) })
  })
})
