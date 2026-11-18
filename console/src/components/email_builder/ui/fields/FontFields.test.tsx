import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { FontFields, type FontKey, type FontValues } from './FontFields'

const ALL: FontKey[] = [
  'fontFamily',
  'fontSize',
  'fontWeight',
  'fontStyle',
  'textTransform',
  'textDecoration',
  'lineHeight',
  'letterSpacing',
  'textAlign'
]

const renderFields = (props: { value?: FontValues; inherited?: FontValues; fields?: FontKey[] } = {}) => {
  const onChange = vi.fn()
  render(
    <FontFields
      value={props.value ?? {}}
      inherited={props.inherited ?? { fontSize: '13px', lineHeight: '1' }}
      fields={props.fields ?? ALL}
      importedFonts={[{ name: 'Inter', href: 'https://fonts.example.com/inter.css' }]}
      onChange={onChange}
    />
  )
  return onChange
}

const segmentedGroup = (label: string) => screen.getByLabelText(label, { selector: '.ant-segmented' })
const selectedIn = (label: string) =>
  segmentedGroup(label).querySelector('.ant-segmented-item-selected .ant-segmented-item-label')?.getAttribute('title')

describe('FontFields', () => {
  it('renders only the rows of the listed keys', () => {
    // mj-social: no text transform, no letter spacing.
    renderFields({
      fields: ['fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'textDecoration', 'lineHeight', 'textAlign']
    })
    expect(screen.getByRole('combobox', { name: 'Font' })).toBeInTheDocument()
    expect(screen.getByRole('spinbutton', { name: 'Line height' })).toBeInTheDocument()
    expect(screen.queryByText('Case')).toBeNull()
    expect(screen.queryByText('Letter spacing')).toBeNull()
  })

  it('emits only the key a row edits, never one outside the listed fields', () => {
    const onChange = renderFields({ value: { textDecoration: 'none' }, fields: ['textDecoration', 'lineHeight'] })
    fireEvent.click(within(segmentedGroup('Decoration')).getByTitle('Underline'))
    expect(onChange).toHaveBeenLastCalledWith({ textDecoration: 'underline' })

    fireEvent.change(screen.getByRole('spinbutton', { name: 'Line height' }), { target: { value: '1.4' } })
    expect(onChange).toHaveBeenLastCalledWith({ lineHeight: '1.4' }, expect.objectContaining({ coalesceKey: expect.any(String) }))
    for (const [updates] of onChange.mock.calls) {
      expect(Object.keys(updates).every((key) => key === 'textDecoration' || key === 'lineHeight')).toBe(true)
    }
  })

  it('writes a typed size in px under one coalesce key while the field keeps the focus', () => {
    const onChange = renderFields({ fields: ['fontSize'] })
    const size = screen.getByRole('spinbutton', { name: 'Font size' })
    fireEvent.focus(size)
    fireEvent.change(size, { target: { value: '1' } })
    fireEvent.change(size, { target: { value: '16' } })
    expect(onChange).toHaveBeenCalledTimes(2)
    const [first, second] = onChange.mock.calls
    expect(second[0]).toEqual({ fontSize: '16px' })
    expect(first[1].coalesceKey).toBe(second[1].coalesceKey)
  })

  it('lists the imported fonts before the system fonts', async () => {
    renderFields({ fields: ['fontFamily'] })
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Font' }))
    await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
    const groups = [...document.querySelectorAll('.ant-select-item-group')].map((el) => el.textContent)
    expect(groups).toEqual(['Imported fonts', 'System fonts'])
    const options = [...document.querySelectorAll('.ant-select-item-option')].map((el) => el.textContent)
    expect(options[0]).toBe('Inter')
    expect(options[1]).toBe('Arial, sans-serif')
  })

  it('shows oblique and overline as selected', () => {
    renderFields({ value: { fontStyle: 'oblique', textDecoration: 'overline' } })
    expect(selectedIn('Style')).toBe('Oblique')
    expect(selectedIn('Decoration')).toBe('Overline')
  })

  it('shows a saved weight outside the list as its own option', async () => {
    renderFields({ value: { fontWeight: 'lighter' }, fields: ['fontWeight'] })
    const weight = screen.getByRole('combobox', { name: 'Font weight' })
    expect(weight.closest('.ant-select')).toHaveTextContent('lighter')
    fireEvent.mouseDown(weight)
    await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
    const options = [...document.querySelectorAll('.ant-select-item-option')].map((el) => el.textContent)
    expect(options).toContain('lighter')
    expect(options).toContain('Bold')
  })

  it('shows the inherited alignment selected and dimmed while the block sets none', () => {
    renderFields({ inherited: { textAlign: 'center' }, fields: ['textAlign'] })
    expect(selectedIn('Align')).toBe('Center')
    expect(segmentedGroup('Align').querySelector('[data-inherited="true"]')).not.toBeNull()
  })

  it('keeps LineHeightInput for the line height, its px conversion included', () => {
    const onChange = renderFields({ value: { lineHeight: '150%', fontSize: '20px' }, fields: ['lineHeight'] })
    const control = screen.getByRole('spinbutton', { name: 'Line height' }).closest('.flex') as HTMLElement
    fireEvent.click(within(control).getByText('px'))
    expect(onChange).toHaveBeenLastCalledWith({ lineHeight: '30px' }, expect.anything())
  })

  it('marks a row set and resets it to the inherited value', () => {
    const onChange = renderFields({ value: { textTransform: 'uppercase' }, inherited: { textTransform: 'none' }, fields: ['textTransform'] })
    fireEvent.click(screen.getByRole('button', { name: 'Reset Case to the inherited value' }))
    expect(onChange).toHaveBeenLastCalledWith({ textTransform: 'none' })
  })
})
