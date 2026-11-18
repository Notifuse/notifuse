import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, renderHook, screen } from '@testing-library/react'
import { SegmentedField } from './SegmentedField'
import { useAlignJustifyOptions, useAlignOptions, useDirectionOptions, useVerticalAlignOptions } from './segmentedOptions'
import type { SegmentedFieldOption } from './SegmentedField'

function AlignField(props: { value?: string; inherited?: string; onChange: (v: string | undefined) => void }) {
  return <SegmentedField aria-label="Align" options={useAlignOptions()} {...props} />
}

const radio = (name: string) => screen.getByRole('radio', { name })

describe('SegmentedField', () => {
  it('writes each clicked value once, without a coalesce key', () => {
    const onChange = vi.fn()
    render(<AlignField value="left" onChange={onChange} />)
    fireEvent.click(radio('Center'))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenLastCalledWith('center')
    fireEvent.click(radio('Right'))
    expect(onChange).toHaveBeenCalledTimes(2)
    expect(onChange).toHaveBeenLastCalledWith('right')
  })

  it('shows the inherited value selected while unset, and a click on it writes nothing', () => {
    const onChange = vi.fn()
    const { container } = render(<AlignField inherited="center" onChange={onChange} />)
    expect(radio('Center')).toBeChecked()
    const dimmed = container.querySelectorAll('[data-inherited="true"]')
    expect(dimmed).toHaveLength(1)
    expect(dimmed[0].closest('label')?.contains(radio('Center'))).toBe(true)

    fireEvent.click(radio('Center'))
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.click(radio('Left'))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('left')
  })

  it('does not dim the block’s own value', () => {
    const { container } = render(<AlignField value="right" inherited="center" onChange={vi.fn()} />)
    expect(radio('Right')).toBeChecked()
    expect(container.querySelector('[data-inherited="true"]')).toBeNull()
  })

  it('selects nothing when neither the block nor its inheritance holds a value', () => {
    render(<AlignField onChange={vi.fn()} />)
    for (const name of ['Left', 'Center', 'Right']) expect(radio(name)).not.toBeChecked()
  })

  it('shows a saved value outside the options as an extra option with its raw text', () => {
    const options: SegmentedFieldOption[] = [
      { value: 'normal', label: 'Normal' },
      { value: 'bold', label: 'Bold' }
    ]
    const onChange = vi.fn()
    render(<SegmentedField aria-label="Weight" value="lighter" options={options} onChange={onChange} />)
    expect(radio('lighter')).toBeChecked()
    expect(screen.getByText('lighter')).toBeInTheDocument()
    fireEvent.click(radio('Bold'))
    expect(onChange).toHaveBeenCalledWith('bold')
  })

  it('names the group and gives each option a name and a tip', () => {
    render(<AlignField onChange={vi.fn()} />)
    expect(screen.getByRole('radiogroup', { name: 'Align' })).toBeInTheDocument()
    expect(screen.getByTitle('Left')).toBeInTheDocument()
  })

  it('ready-made sets hold their values, names and labels', () => {
    const values = (options: SegmentedFieldOption[]) => options.map((option) => option.value)
    const align = renderHook(() => useAlignOptions()).result.current
    expect(values(align)).toEqual(['left', 'center', 'right'])
    expect(align.map((option) => option.title)).toEqual(['Left', 'Center', 'Right'])

    const justify = renderHook(() => useAlignJustifyOptions()).result.current
    expect(values(justify)).toEqual(['left', 'center', 'right', 'justify'])
    expect(justify[3].title).toBe('Justify')

    const vertical = renderHook(() => useVerticalAlignOptions()).result.current
    expect(values(vertical)).toEqual(['top', 'middle', 'bottom'])
    expect(vertical.map((option) => option.label)).toEqual(['Top', 'Middle', 'Bottom'])

    const direction = renderHook(() => useDirectionOptions()).result.current
    expect(values(direction)).toEqual(['ltr', 'rtl'])
    expect(direction.map((option) => option.label)).toEqual(['Left to right', 'Right to left'])
  })

  it('renders labelled options with their visible text', () => {
    function Direction() {
      return <SegmentedField aria-label="Direction" options={useDirectionOptions()} value="ltr" onChange={vi.fn()} />
    }
    render(<Direction />)
    expect(radio('Left to right')).toBeChecked()
    expect(screen.getByText('Right to left')).toBeVisible()
  })
})
