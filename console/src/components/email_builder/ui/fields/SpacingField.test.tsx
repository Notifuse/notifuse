import { describe, it, expect, vi } from 'vitest'
import { act, render, screen, fireEvent, within } from '@testing-library/react'
import { SpacingField } from './SpacingField'
import { toSideAttributes, formatPaddingShorthand, parsePaddingShorthand } from './spacing'

const side = (name: string, box?: string) =>
  (box ? within(screen.getByRole('group', { name: box })) : screen).getByRole('spinbutton', {
    name: name
  }) as HTMLInputElement

const NO_SIDES = { top: undefined, right: undefined, bottom: undefined, left: undefined }

/**
 * Regression tests for issue #369, moved from ui/PaddingInput.test.tsx.
 *
 * mj-button / mj-social edit their `inner-padding` in "shorthand string" mode. Before a value is
 * set, `value` is `undefined`. The field must still treat this as shorthand mode and write a
 * STRING: writing an object {top, right, bottom, left} is what got serialized by the backend as
 * the Go map literal `map[bottom:0px top:0px]`, which Gmail rejects.
 */
describe('SpacingField (from PaddingInput)', () => {
  it('emits a string in shorthand mode when value is undefined (issue #369)', () => {
    const onChange = vi.fn()
    // Shorthand mode is signalled by a string inherited value (the mj-button case).
    render(<SpacingField label="Inside the button" value={undefined} inherited="10px 25px" onChange={onChange} />)

    fireEvent.change(side('Top'), { target: { value: '5' } })

    expect(onChange).toHaveBeenCalled()
    const arg = onChange.mock.calls.at(-1)![0]
    expect(typeof arg).toBe('string')
    expect(arg).not.toContain('map[')
  })

  it('emits a string in shorthand mode when value is a string', () => {
    const onChange = vi.fn()
    render(<SpacingField label="Inside the button" value="10px 25px" inherited="10px 25px" onChange={onChange} />)

    fireEvent.change(side('Top'), { target: { value: '8' } })

    expect(onChange).toHaveBeenCalled()
    const arg = onChange.mock.calls.at(-1)![0]
    expect(typeof arg).toBe('string')
  })

  it('emits an object in individual mode (container padding case)', () => {
    const onChange = vi.fn()
    render(
      <SpacingField
        label="Around the text"
        value={{ top: '10px', right: '25px', bottom: '10px', left: '25px' }}
        onChange={onChange}
      />
    )

    fireEvent.change(side('Top'), { target: { value: '12' } })

    expect(onChange).toHaveBeenCalled()
    const arg = onChange.mock.calls.at(-1)![0]
    expect(typeof arg).toBe('object')
    expect(arg).toHaveProperty('top', '12px')
  })
})

describe('SpacingField', () => {
  it('shows an unset side as the inherited placeholder, not 0', () => {
    render(
      <SpacingField
        label="Around the text"
        value={{ ...NO_SIDES, bottom: '4px' }}
        inherited={{ top: '10px', right: '25px', bottom: '10px', left: '25px' }}
        onChange={vi.fn()}
      />
    )
    expect(side('Top').value).toBe('')
    expect(side('Top').placeholder).toBe('10')
    expect(side('Right').value).toBe('')
    expect(side('Right').placeholder).toBe('25')
    expect(side('Bottom').value).toBe('4')
  })

  it('shows an unset shorthand as the inherited placeholders', () => {
    render(<SpacingField label="Inside the button" value={undefined} inherited="10px 25px" onChange={vi.fn()} />)
    expect(side('Top').value).toBe('')
    expect(side('Top').placeholder).toBe('10')
    expect(side('Left').placeholder).toBe('25')
  })

  it('writes only the edited key in per-side mode, leaving undefined sides undefined', () => {
    const onChange = vi.fn()
    render(
      <SpacingField
        label="Around the text"
        value={{ ...NO_SIDES, top: '10px' }}
        inherited={{ top: '10px', right: '25px', bottom: '10px', left: '25px' }}
        onChange={onChange}
      />
    )
    fireEvent.change(side('Right'), { target: { value: '30' } })
    const written = onChange.mock.calls.at(-1)![0]
    expect(written).toEqual({ right: '30px' })
    expect(toSideAttributes(written, 'padding')).toEqual({ paddingRight: '30px' })
  })

  it('writes undefined for a cleared side, back to the inherited value', () => {
    const onChange = vi.fn()
    render(<SpacingField label="Around the text" value={{ ...NO_SIDES, left: '8px' }} onChange={onChange} />)
    fireEvent.change(side('Left'), { target: { value: '' } })
    expect(onChange).toHaveBeenLastCalledWith({ left: undefined }, expect.anything())
  })

  it('fills the other sides of an unset shorthand from the inherited value, not 0px', () => {
    const onChange = vi.fn()
    render(<SpacingField label="Inside the button" value={undefined} inherited="10px 25px" onChange={onChange} />)
    fireEvent.change(side('Top'), { target: { value: '5' } })
    expect(onChange.mock.calls.at(-1)![0]).toBe('5px 25px 10px 25px')
  })

  it('keeps the shorthand short: two values when top equals bottom and right equals left', () => {
    const onChange = vi.fn()
    render(<SpacingField label="Inside the button" value="10px 25px" inherited="10px 25px" onChange={onChange} />)
    fireEvent.change(side('Right'), { target: { value: '30' } })
    expect(onChange.mock.calls.at(-1)![0]).toBe('10px 30px 10px 25px')
    expect(formatPaddingShorthand({ top: '4px', right: '8px', bottom: '4px', left: '8px' })).toBe('4px 8px')
    expect(formatPaddingShorthand({ top: '4px', right: '4px', bottom: '4px', left: '4px' })).toBe('4px')
    expect(parsePaddingShorthand('1px 2px 3px')).toEqual({ top: '1px', right: '2px', bottom: '3px', left: '2px' })
  })

  it('linking writes the four sides at once, as one click', () => {
    const onChange = vi.fn()
    render(
      <SpacingField
        label="Around the text"
        value={{ top: '24px', right: '10px', bottom: undefined, left: undefined }}
        inherited={{ top: '10px', right: '25px', bottom: '10px', left: '25px' }}
        onChange={onChange}
      />
    )
    const link = screen.getByRole('button', { name: 'Link the four sides' })
    expect(link).toHaveAttribute('aria-pressed', 'false')
    fireEvent.click(link)
    expect(link).toHaveAttribute('aria-pressed', 'true')
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenLastCalledWith({ top: '24px', right: '24px', bottom: '24px', left: '24px' }, undefined)
  })

  it('a side typed while linked writes the four sides', () => {
    const onChange = vi.fn()
    const four = { top: '24px', right: '24px', bottom: '24px', left: '24px' }
    render(<SpacingField label="Around the text" value={four} onChange={onChange} />)
    expect(screen.getByRole('button', { name: 'Link the four sides' })).toHaveAttribute('aria-pressed', 'true')
    fireEvent.change(side('Left'), { target: { value: '12' } })
    expect(onChange.mock.calls.at(-1)![0]).toEqual({ top: '12px', right: '12px', bottom: '12px', left: '12px' })
  })

  it('a side typed while linked writes a one-value shorthand', () => {
    const onChange = vi.fn()
    render(<SpacingField label="Inside the button" value={undefined} inherited="10px 25px" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Link the four sides' }))
    expect(onChange.mock.calls.at(-1)![0]).toBe('10px')
    fireEvent.change(side('Bottom'), { target: { value: '6' } })
    expect(onChange.mock.calls.at(-1)![0]).toBe('6px')
  })

  it('keystrokes share a coalesce key, a new focus gets a new one', () => {
    const onChange = vi.fn()
    render(<SpacingField label="Around the text" value={NO_SIDES} onChange={onChange} />)
    const top = side('Top')
    act(() => top.focus())
    fireEvent.change(top, { target: { value: '1' } })
    fireEvent.change(top, { target: { value: '12' } })
    const first = onChange.mock.calls[0][1]?.coalesceKey
    expect(first).toEqual(expect.any(String))
    expect(onChange.mock.calls[1][1]?.coalesceKey).toBe(first)

    act(() => top.blur())
    act(() => top.focus())
    fireEvent.change(top, { target: { value: '3' } })
    expect(onChange.mock.calls.at(-1)![1]?.coalesceKey).not.toBe(first)
  })

  it('nests the inner box, each box writing its own value', () => {
    const outer = vi.fn()
    const inner = vi.fn()
    render(
      <SpacingField
        label="Around the button"
        value={NO_SIDES}
        inherited={{ top: '10px', right: '25px', bottom: '10px', left: '25px' }}
        onChange={outer}
        inner={{ label: 'Inside the button', centerLabel: 'Text', value: undefined, inherited: '10px 25px', onChange: inner }}
      />
    )
    expect(screen.getByText('Text')).toBeInTheDocument()
    fireEvent.change(side('Bottom', 'Inside the button'), { target: { value: '11' } })
    expect(inner.mock.calls.at(-1)![0]).toBe('10px 25px 11px 25px')
    expect(outer).not.toHaveBeenCalled()
  })
})
