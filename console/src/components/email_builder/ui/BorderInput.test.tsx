import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import BorderInput, { SingleBorderInput } from './BorderInput'

describe('SingleBorderInput', () => {
  it('writes one border shorthand, with no side switch', () => {
    const onChange = vi.fn()
    render(<SingleBorderInput value="1px solid #000000" onChange={onChange} />)
    expect(screen.queryByText('Separate sides')).toBeNull()
    expect(screen.queryByText('All sides')).toBeNull()
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '2' } })
    expect(onChange).toHaveBeenLastCalledWith('2px solid #000000')
  })

  it('writes nothing for a border of 0', () => {
    const onChange = vi.fn()
    render(<SingleBorderInput value="1px solid #000000" onChange={onChange} />)
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '0' } })
    expect(onChange).toHaveBeenLastCalledWith(undefined)
  })

  it("reads MJML's default border of 0 as none", () => {
    render(<SingleBorderInput value="0" onChange={vi.fn()} />)
    expect((screen.getByRole('spinbutton') as HTMLInputElement).value).toBe('0')
  })
})

describe('SingleBorderInput with a none value', () => {
  it('writes the none value for a border of 0, to override an inherited border', () => {
    const onChange = vi.fn()
    render(<SingleBorderInput value="1px solid #000000" onChange={onChange} noneValue="none" />)
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '0' } })
    expect(onChange).toHaveBeenLastCalledWith('none')
  })
})

describe('BorderInput', () => {
  it('still writes the four sides, with its side switch', () => {
    const onChange = vi.fn()
    const side = '1px solid #000000'
    render(
      <BorderInput
        value={{ borderTop: side, borderRight: side, borderBottom: side, borderLeft: side }}
        onChange={onChange}
      />
    )
    expect(screen.getByText('Separate sides')).toBeTruthy()
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '3' } })
    expect(onChange).toHaveBeenLastCalledWith({
      borderTop: '3px solid #000000',
      borderRight: '3px solid #000000',
      borderBottom: '3px solid #000000',
      borderLeft: '3px solid #000000'
    })
  })
})
