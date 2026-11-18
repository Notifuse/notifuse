import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import LineHeightInput from './LineHeightInput'
import { FontFields } from './fields/FontFields'
import { convertLineHeight, parseLineHeight } from './lineHeight'

const renderInput = (value: string, onChange = vi.fn(), fontSize?: number) => {
  render(
    <I18nProvider i18n={i18n}>
      <LineHeightInput value={value} onChange={onChange} fontSize={fontSize} />
    </I18nProvider>
  )
  return onChange
}

const field = () => screen.getByRole('spinbutton', { name: 'Line height' })

const selectedUnit = () => document.querySelector('.ant-segmented-item-selected')?.textContent

describe('LineHeightInput', () => {
  it('shows a line height without a unit as a multiple of the font size', () => {
    // 1.5 was shown as "2" with "%".
    renderInput('1.5')
    expect(screen.getByRole('spinbutton', { name: 'Line height' })).toHaveValue('1.5')
    expect(selectedUnit()).toBe('×')
  })

  it('keeps percentages and pixels as they are', () => {
    renderInput('120%')
    expect(screen.getByRole('spinbutton', { name: 'Line height' })).toHaveValue('120')
    expect(selectedUnit()).toBe('%')
  })

  it('saves a multiple without a unit, and turns it into a percentage when % is picked', () => {
    const onChange = renderInput('1.5')
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Line height' }), { target: { value: '1.6' } })
    expect(onChange).toHaveBeenLastCalledWith('1.6')
    fireEvent.click(screen.getByText('%'))
    expect(onChange).toHaveBeenLastCalledWith('160%')
  })

  it('keeps the rendered height when switching to and from pixels', () => {
    // 1.5 used to become "1.5px", and 150 % "150px".
    const onChange = renderInput('1.5', vi.fn(), 16)
    fireEvent.click(screen.getByText('px'))
    expect(onChange).toHaveBeenLastCalledWith('24px')
    expect(field()).toHaveValue('24')
    fireEvent.click(screen.getByText('×'))
    expect(onChange).toHaveBeenLastCalledWith('1.5')
    expect(field()).toHaveValue('1.5')
    fireEvent.click(screen.getByText('%'))
    expect(onChange).toHaveBeenLastCalledWith('150%')
    fireEvent.click(screen.getByText('px'))
    expect(onChange).toHaveBeenLastCalledWith('24px')
    expect(field()).toHaveValue('24')
  })

  it('converts with the canvas default font size when the block sets none', () => {
    // 1.5 × 13px = 19.5px, stored and shown as 20.
    const onChange = renderInput('1.5')
    fireEvent.click(screen.getByText('px'))
    expect(onChange).toHaveBeenLastCalledWith('20px')
    expect(field()).toHaveValue('20')
  })

  it('stores the number the field shows', () => {
    const onChange = renderInput('24px')
    fireEvent.change(field(), { target: { value: '22.6' } })
    expect(onChange).toHaveBeenLastCalledWith('23px')
    expect(field()).toHaveValue('23')
  })

  it('shows a stored pixel value as it is', () => {
    renderInput('22.5px')
    expect(field()).toHaveValue('22.5')
  })

  it('switches units with the font size of the block being edited', () => {
    const onChange = vi.fn()
    const values = {
      fontFamily: undefined,
      fontSize: '20px',
      fontWeight: undefined,
      fontStyle: undefined,
      textTransform: undefined,
      textDecoration: undefined,
      lineHeight: '1.5',
      letterSpacing: undefined,
      textAlign: undefined
    }
    render(
      <I18nProvider i18n={i18n}>
        <FontFields
          value={values}
          inherited={{ ...values, fontSize: '13px' }}
          fields={['fontSize', 'lineHeight']}
          onChange={onChange}
        />
      </I18nProvider>
    )
    const lineHeightControl = field().closest('.flex') as HTMLElement
    fireEvent.click(within(lineHeightControl).getByText('px'))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ lineHeight: '30px' }), expect.anything())
  })

  it('converts between the three units', () => {
    expect(convertLineHeight(150, '%', 'px', 16)).toBe(24)
    expect(convertLineHeight(21, 'px', 'x', 13)).toBe(1.62)
    expect(convertLineHeight(21, 'px', '%', 13)).toBe(162)
    expect(convertLineHeight(1.15, 'x', '%', 13)).toBe(115)
  })

  it('parses the three forms', () => {
    expect(parseLineHeight('1.5')).toEqual({ number: 1.5, unit: 'x' })
    expect(parseLineHeight('150%')).toEqual({ number: 150, unit: '%' })
    expect(parseLineHeight('22px')).toEqual({ number: 22, unit: 'px' })
    expect(parseLineHeight('normal')).toEqual({ number: undefined, unit: 'x' })
  })
})
