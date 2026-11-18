import { describe, it, expect, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import BorderField, { SingleBorderField } from './BorderField'
import { borderWithColor, borderWithStyle, borderWithWidth } from './borderRules'

// The colour picker is a popover; its own tests cover it. Here it is two buttons: pick red, clear.
vi.mock('./ColorField', () => ({
  ColorField: ({ value, onChange, compact }: { value?: string; onChange: (value: string | undefined) => void; compact?: boolean }) => (
    <span data-testid="color" data-value={value} data-compact={compact ? 'true' : undefined}>
      <button type="button" onClick={() => onChange('#ff0000')}>
        Pick red
      </button>
      <button type="button" onClick={() => onChange('transparent')}>
        Clear colour
      </button>
    </span>
  )
}))

const all = (side: string | undefined) => ({ borderTop: side, borderRight: side, borderBottom: side, borderLeft: side })

const width = (name = 'Border width') => screen.getByRole('spinbutton', { name }) as HTMLInputElement
const style = (name = 'Border style') => screen.getByRole('combobox', { name })

// Picks an option of the antd select labelled `label`.
async function choose(label: string, option: string) {
  await userEvent.click(style(label))
  const options = await screen.findAllByText(option, { selector: '.ant-select-item-option-content' })
  await userEvent.click(options[options.length - 1])
}

describe('SingleBorderField (from SingleBorderInput)', () => {
  it('writes one border shorthand, with no side switch', () => {
    const onChange = vi.fn()
    render(<SingleBorderField value="1px solid #000000" onChange={onChange} />)
    expect(screen.queryByText('Set each side')).toBeNull()
    expect(screen.queryByRole('switch')).toBeNull()
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '2' } })
    expect(onChange).toHaveBeenLastCalledWith('2px solid #000000', expect.anything())
  })

  it('writes nothing for a border of 0', () => {
    const onChange = vi.fn()
    render(<SingleBorderField value="1px solid #000000" onChange={onChange} />)
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '0' } })
    expect(onChange.mock.calls.at(-1)![0]).toBeUndefined()
  })

  it("reads MJML's default border of 0 as none", () => {
    render(<SingleBorderField value="0" onChange={vi.fn()} />)
    expect((screen.getByRole('spinbutton') as HTMLInputElement).value).toBe('0')
  })

  it('writes the none value for a border of 0, to override an inherited border', () => {
    const onChange = vi.fn()
    render(<SingleBorderField value="1px solid #000000" onChange={onChange} noneValue="none" />)
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '0' } })
    expect(onChange.mock.calls.at(-1)![0]).toBe('none')
  })

  it('writes the none value when the colour is cleared', () => {
    const onChange = vi.fn()
    render(<SingleBorderField value="2px solid #d1d5db" onChange={onChange} noneValue="none" />)
    fireEvent.click(screen.getByRole('button', { name: 'Clear colour' }))
    expect(onChange).toHaveBeenLastCalledWith('none', undefined)
  })
})

describe('BorderField (from BorderInput)', () => {
  it('still writes the four sides, with its side switch', () => {
    const onChange = vi.fn()
    render(<BorderField value={all('1px solid #000000')} onChange={onChange} />)
    expect(screen.getByRole('switch', { name: 'Set each side' })).toBeTruthy()
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '3' } })
    expect(onChange.mock.calls.at(-1)![0]).toEqual(all('3px solid #000000'))
  })
})

describe('BorderField rules', () => {
  it('a transparent colour disables width and style', () => {
    render(<BorderField value={{}} onChange={vi.fn()} />)
    expect(width()).toBeDisabled()
    expect(style()).toBeDisabled()
  })

  it('style none disables width, not style', () => {
    render(<BorderField value={all('1px none #ff0000')} onChange={vi.fn()} />)
    expect(width()).toBeDisabled()
    expect(style()).not.toBeDisabled()
  })

  it('a width above 0 sets style solid', () => {
    expect(borderWithWidth({ width: '0px', style: 'none', color: '#ff0000' }, 2)).toEqual({
      width: '2px',
      style: 'solid',
      color: '#ff0000'
    })
  })

  it('a width above 0 sets colour #000000 on a transparent colour', () => {
    expect(borderWithWidth({ width: '0px', style: 'dashed', color: 'transparent' }, 2)).toEqual({
      width: '2px',
      style: 'dashed',
      color: '#000000'
    })
  })

  it('a visible style sets width 1px', async () => {
    const onChange = vi.fn()
    render(<BorderField value={all('0px none #ff0000')} onChange={onChange} />)
    await choose('Border style', 'Dashed')
    expect(onChange).toHaveBeenLastCalledWith(all('1px dashed #ff0000'), undefined)
  })

  it('a visible style sets colour #000000 on a transparent colour', () => {
    expect(borderWithStyle({ width: '0px', style: 'none', color: 'transparent' }, 'dotted')).toEqual({
      width: '1px',
      style: 'dotted',
      color: '#000000'
    })
  })

  it('style none resets width to 0, which writes no border', async () => {
    const onChange = vi.fn()
    render(<BorderField value={all('2px solid #ff0000')} onChange={onChange} />)
    await choose('Border style', 'None')
    expect(onChange).toHaveBeenLastCalledWith(all(undefined), undefined)
  })

  it('picking a colour sets 1px and solid when they are unset', () => {
    const onChange = vi.fn()
    render(<BorderField value={{}} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Pick red' }))
    expect(onChange).toHaveBeenLastCalledWith(all('1px solid #ff0000'), undefined)
  })

  it('picking a colour keeps a width and style already set', () => {
    expect(borderWithColor({ width: '3px', style: 'dashed', color: '#000000' }, '#ff0000')).toEqual({
      width: '3px',
      style: 'dashed',
      color: '#ff0000'
    })
  })

  it('clearing the colour resets width and style', () => {
    const onChange = vi.fn()
    render(<BorderField value={all('2px dashed #ff0000')} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Clear colour' }))
    expect(onChange).toHaveBeenLastCalledWith(all(undefined), undefined)
    expect(borderWithColor({ width: '2px', style: 'dashed', color: '#ff0000' }, undefined)).toEqual({
      width: '0px',
      style: 'none',
      color: 'transparent'
    })
  })
})

describe('BorderField sides', () => {
  it('opens on each side by itself when the four sides differ', () => {
    render(
      <BorderField value={{ borderTop: '1px solid #e5e7eb', borderBottom: '3px solid #6a5af9' }} onChange={vi.fn()} />
    )
    expect(screen.getByRole('switch', { name: 'Set each side' })).toBeChecked()
    expect(width('Top border width').value).toBe('1')
    expect(width('Right border width')).toBeDisabled()
    expect(width('Bottom border width').value).toBe('3')
  })

  it('stays on all sides when the four sides are equal', () => {
    render(<BorderField value={all('1px solid #000000')} onChange={vi.fn()} />)
    expect(screen.getByRole('switch', { name: 'Set each side' })).not.toBeChecked()
    expect(screen.queryByRole('spinbutton', { name: 'Top border width' })).toBeNull()
  })

  it('edits one side without touching the others', () => {
    const onChange = vi.fn()
    render(
      <BorderField value={{ borderTop: '1px solid #e5e7eb', borderBottom: '3px solid #6a5af9' }} onChange={onChange} />
    )
    fireEvent.change(width('Bottom border width'), { target: { value: '4' } })
    expect(onChange.mock.calls.at(-1)![0]).toEqual({
      borderTop: '1px solid #e5e7eb',
      borderRight: undefined,
      borderBottom: '4px solid #6a5af9',
      borderLeft: undefined
    })
  })

  it('switching each side on writes nothing; back off writes the top border on the four, as one click', () => {
    const onChange = vi.fn()
    const { rerender } = render(<BorderField value={all('1px solid #000000')} onChange={onChange} />)
    const toggle = screen.getByRole('switch', { name: 'Set each side' })
    fireEvent.click(toggle)
    expect(onChange).not.toHaveBeenCalled()
    expect(width('Left border width').value).toBe('1')

    const sides = { ...all('1px solid #000000'), borderLeft: '5px dotted #ff0000' }
    rerender(<BorderField value={sides} onChange={onChange} />)
    fireEvent.click(screen.getByRole('switch', { name: 'Set each side' }))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenLastCalledWith(all('1px solid #000000'), undefined)
  })
})

describe('BorderField history', () => {
  it('width keystrokes share a coalesce key, a new focus gets a new one; colour and style carry none', async () => {
    const onChange = vi.fn()
    render(<BorderField value={all('1px solid #000000')} onChange={onChange} />)
    const input = width()
    act(() => input.focus())
    fireEvent.change(input, { target: { value: '2' } })
    fireEvent.change(input, { target: { value: '12' } })
    const first = onChange.mock.calls[0][1]?.coalesceKey
    expect(first).toEqual(expect.any(String))
    expect(onChange.mock.calls[1][1]?.coalesceKey).toBe(first)

    act(() => input.blur())
    act(() => input.focus())
    fireEvent.change(input, { target: { value: '3' } })
    expect(onChange.mock.calls.at(-1)![1]?.coalesceKey).not.toBe(first)

    fireEvent.click(screen.getByRole('button', { name: 'Pick red' }))
    expect(onChange.mock.calls.at(-1)![1]).toBeUndefined()
    await choose('Border style', 'Dotted')
    expect(onChange.mock.calls.at(-1)![1]).toBeUndefined()
  })
})

describe('BorderField keeps the sides it does not edit', () => {
  it('reads a colour with spaces as one colour', async () => {
    const { parseBorderValue } = await import('./borderRules')
    expect(parseBorderValue('1px solid rgb(0, 0, 0)')).toEqual({ width: '1px', style: 'solid', color: 'rgb(0, 0, 0)' })
    expect(parseBorderValue('solid 2px #ff0000')).toEqual({ width: '2px', style: 'solid', color: '#ff0000' })
  })

  it('writes only the edited side and leaves the others as they were written', () => {
    const onChange = vi.fn()
    render(
      <BorderField
        value={{ borderTop: '1px solid #000000', borderRight: '2px dashed', borderBottom: '1px solid rgb(0, 0, 0)', borderLeft: undefined }}
        onChange={onChange}
      />
    )
    const top = screen.getByRole('spinbutton', { name: 'Top border width' })
    fireEvent.focus(top)
    fireEvent.change(top, { target: { value: '3' } })
    const [written] = onChange.mock.calls.at(-1)!
    expect(written.borderTop).toBe('3px solid #000000')
    expect(written.borderRight).toBe('2px dashed')
    expect(written.borderBottom).toBe('1px solid rgb(0, 0, 0)')
    expect(written.borderLeft).toBeUndefined()
  })
})

describe('BorderField colour', () => {
  it('uses the compact colour field of the kit', () => {
    render(<BorderField value={{ borderTop: '1px solid #000000', borderRight: '1px solid #000000', borderBottom: '1px solid #000000', borderLeft: '1px solid #000000' }} onChange={vi.fn()} />)
    expect(screen.getAllByTestId('color')[0]).toHaveAttribute('data-compact', 'true')
  })
})
