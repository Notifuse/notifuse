import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { FieldPicker } from './FieldPicker'
import { ContactsTableSchema, WebSessionChangesSchema } from './table_schemas'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const open = (props: Partial<React.ComponentProps<typeof FieldPicker>> = {}) => {
  render(
    <I18nProvider i18n={i18n}>
      <FieldPicker schema={ContactsTableSchema} {...props} />
    </I18nProvider>
  )
  fireEvent.mouseDown(screen.getByRole('combobox'))
}

// The option rows of the open dropdown, by their visible text.
const optionTexts = () =>
  [...document.querySelectorAll('.ant-select-item-option')].map((element) => element.textContent ?? '')

describe('FieldPicker', () => {
  it('groups the contact fields', () => {
    open()
    for (const group of ['Identity', 'Location', 'Email tracking', 'Custom fields', 'Contact dates']) {
      expect(screen.getAllByText(group).length, group).toBeGreaterThan(0)
    }
    expect(optionTexts().some((text) => text.includes('Email'))).toBe(true)
  })

  it('shows a renamed custom field with its technical name', () => {
    open({ customFieldLabels: { custom_string_1: 'Plan' } })
    expect(optionTexts().some((text) => text.includes('Plan') && text.includes('custom_string_1'))).toBe(true)
  })

  it('narrows the list as the user types', () => {
    open()
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'postc' } })
    const texts = optionTexts()
    expect(texts.some((text) => text.includes('Postcode'))).toBe(true)
    expect(texts.some((text) => text.includes('First Name'))).toBe(false)
  })

  it('shows a field another row filters as already filtered, not selectable', () => {
    open({ usedFields: ['country'] })
    const option = [...document.querySelectorAll('.ant-select-item-option')].find((element) =>
      element.textContent?.includes('Country')
    )
    expect(option?.textContent).toContain('already filtered')
    expect(option?.className).toContain('ant-select-item-option-disabled')
  })

  it('leaves the hidden fields out of a new filter', () => {
    open()
    const texts = optionTexts()
    // Its neighbours are rendered, so the absence is the filter's, not the virtual list's
    expect(texts.some((text) => text.includes('Address Line 1'))).toBe(true)
    expect(texts.some((text) => text.includes('Created At'))).toBe(true)
    expect(texts.some((text) => text.includes('Address Line 2'))).toBe(false)
    expect(texts.some((text) => text.includes('Updated At'))).toBe(false)
  })

  it('keeps a hidden field selectable in the row that already holds it', () => {
    open({ value: 'address_line_2' })
    expect(optionTexts().some((text) => text.includes('Address Line 2'))).toBe(true)
    expect(optionTexts().some((text) => text.includes('Updated At'))).toBe(false)
  })

  it('lists the keys of a web schema without groups', () => {
    open({ schema: WebSessionChangesSchema })
    expect(optionTexts().some((text) => text.includes('Entry page'))).toBe(true)
    expect(screen.queryByText('Identity')).toBeNull()
  })
})
