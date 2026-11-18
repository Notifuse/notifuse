import { describe, it, expect, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { NumberUnitField } from './NumberUnitField'

const width = (props: Partial<React.ComponentProps<typeof NumberUnitField>> = {}) => {
  const onChange = vi.fn()
  const view = render(
    <NumberUnitField
      aria-label="Width"
      coalesceBase="block-1:width"
      units={['%', 'px']}
      onChange={onChange}
      {...props}
    />
  )
  return { onChange, ...view }
}

const input = () => screen.getByRole('spinbutton', { name: 'Width' }) as HTMLInputElement

describe('NumberUnitField', () => {
  it('keeps the number when the unit is switched, as a click without a coalesce key', () => {
    const { onChange } = width({ value: '50%' })
    expect(input().value).toBe('50')
    fireEvent.click(screen.getByText('px'))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('50px')
  })

  it('writes every keystroke with the focus session key, and a new focus gives a new key', () => {
    const { onChange } = width({ value: '50%' })
    act(() => input().focus())
    fireEvent.change(input(), { target: { value: '6' } })
    fireEvent.change(input(), { target: { value: '60' } })
    expect(onChange).toHaveBeenNthCalledWith(1, '6%', { coalesceKey: expect.any(String) })
    expect(onChange).toHaveBeenNthCalledWith(2, '60%', { coalesceKey: expect.any(String) })
    const first = onChange.mock.calls[0][1].coalesceKey
    expect(onChange.mock.calls[1][1].coalesceKey).toBe(first)
    expect(first).toMatch(/^block-1:width:/)

    act(() => input().blur())
    act(() => input().focus())
    fireEvent.change(input(), { target: { value: '70' } })
    expect(onChange).toHaveBeenLastCalledWith('70%', { coalesceKey: expect.any(String) })
    expect(onChange.mock.lastCall?.[1].coalesceKey).not.toBe(first)
  })

  it('clamps an out-of-bounds number on blur, not while it is typed', () => {
    const { onChange } = width({ value: '50px', units: ['px'], min: 10, max: 100 })
    act(() => input().focus())
    fireEvent.change(input(), { target: { value: '5' } })
    fireEvent.change(input(), { target: { value: '500' } })
    expect(onChange.mock.calls.map((call) => call[0])).not.toContain('10px')
    expect(onChange.mock.calls.map((call) => call[0])).not.toContain('100px')
    act(() => input().blur())
    expect(onChange).toHaveBeenLastCalledWith('100px', { coalesceKey: expect.any(String) })
  })

  it('writes undefined when the number is emptied', () => {
    const { onChange } = width({ value: '50%' })
    fireEvent.change(input(), { target: { value: '' } })
    expect(onChange).toHaveBeenLastCalledWith(undefined, { coalesceKey: expect.any(String) })
  })

  it('shows the inherited value as the placeholder while unset', () => {
    width({ inherited: '600px', units: ['px'] })
    expect(input().value).toBe('')
    expect(input().placeholder).toBe('600')
  })

  it('shows the inherited text when its unit is not the one shown, and the placeholder when nothing is inherited', () => {
    const { rerender, onChange } = width({ inherited: 'auto', placeholder: 'Auto' })
    expect(input().placeholder).toBe('auto')
    rerender(
      <NumberUnitField aria-label="Width" coalesceBase="b" units={['%', 'px']} placeholder="None" onChange={onChange} />
    )
    expect(input().placeholder).toBe('None')
  })

  it('uses the default unit while the value is undefined', () => {
    const { onChange } = width({ defaultUnit: 'px' })
    fireEvent.change(input(), { target: { value: '12' } })
    expect(onChange).toHaveBeenLastCalledWith('12px', { coalesceKey: expect.any(String) })
  })

  it('reads a bare number in the default unit', () => {
    const { onChange } = width({ value: '20', defaultUnit: 'px' })
    expect(input().value).toBe('20')
    fireEvent.change(input(), { target: { value: '24' } })
    expect(onChange).toHaveBeenLastCalledWith('24px', { coalesceKey: expect.any(String) })
  })

  it('switches the unit of an empty field without writing, and types in that unit', () => {
    const { onChange } = width({})
    fireEvent.click(screen.getByText('px'))
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.change(input(), { target: { value: '30' } })
    expect(onChange).toHaveBeenLastCalledWith('30px', { coalesceKey: expect.any(String) })
  })

  describe('height mode', () => {
    const height = (value?: string, inherited?: string) => {
      const onChange = vi.fn()
      render(
        <NumberUnitField
          aria-label="Height"
          coalesceBase="block-1:height"
          mode="height"
          value={value}
          inherited={inherited}
          onChange={onChange}
        />
      )
      return onChange
    }
    const heightInput = () => screen.getByRole('spinbutton', { name: 'Height' }) as HTMLInputElement

    it('starts at 100px when px is picked from Auto', () => {
      const onChange = height(undefined)
      expect(heightInput()).toBeDisabled()
      fireEvent.click(screen.getByText('px'))
      expect(onChange).toHaveBeenCalledTimes(1)
      expect(onChange).toHaveBeenCalledWith('100px')
    })

    it('writes auto when Auto is picked from a px height', () => {
      const onChange = height('120px')
      expect(heightInput().value).toBe('120')
      fireEvent.click(screen.getByText('Auto'))
      expect(onChange).toHaveBeenCalledTimes(1)
      expect(onChange).toHaveBeenCalledWith('auto')
    })

    it('writes the typed px height with a coalesce key, and undefined when emptied', () => {
      const onChange = height('120px')
      fireEvent.change(heightInput(), { target: { value: '80' } })
      expect(onChange).toHaveBeenLastCalledWith('80px', { coalesceKey: expect.any(String) })
      fireEvent.change(heightInput(), { target: { value: '' } })
      expect(onChange).toHaveBeenLastCalledWith(undefined, { coalesceKey: expect.any(String) })
    })

    it('shows an auto value as Auto with the number disabled', () => {
      height('auto')
      expect(heightInput()).toBeDisabled()
      expect(heightInput().value).toBe('')
    })

    it('follows an inherited px height while unset', () => {
      const onChange = height(undefined, '200px')
      expect(heightInput()).not.toBeDisabled()
      expect(heightInput().placeholder).toBe('200')
      fireEvent.change(heightInput(), { target: { value: '90' } })
      expect(onChange).toHaveBeenLastCalledWith('90px', { coalesceKey: expect.any(String) })
    })
  })
})
