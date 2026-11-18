import { useState } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { InputEventPropertyFilters } from './input_event_property_filters'
import { arePropertyFiltersValid } from './filterOperators'
import type { DimensionFilter } from '../../services/api/segment'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const renderInput = (
  value: DimensionFilter[] | undefined,
  onChange?: (v: DimensionFilter[]) => void
) =>
  render(
    <I18nProvider i18n={i18n}>
      <InputEventPropertyFilters value={value} onChange={onChange} />
    </I18nProvider>
  )

// The rows are controlled: this keeps what they hand back, as the condition's form does.
const renderEditable = (initial: DimensionFilter[] = []) => {
  const latest: { value: DimensionFilter[] } = { value: initial }
  const Wrapper = () => {
    const [value, setValue] = useState(initial)
    return (
      <InputEventPropertyFilters
        value={value}
        onChange={(next) => {
          latest.value = next
          setValue(next)
        }}
      />
    )
  }
  render(
    <I18nProvider i18n={i18n}>
      <Wrapper />
    </I18nProvider>
  )
  return latest
}

// Each row: the value type select, then the operator select.
const rowSelects = () => screen.getAllByRole('combobox')

const pick = async (combobox: HTMLElement, title: string) => {
  fireEvent.mouseDown(combobox)
  fireEvent.click(await screen.findByTitle(title))
}

describe('InputEventPropertyFilters', () => {
  it('renders an arbitrary property key that no schema declares', () => {
    // The whole point of this component: keys come from the caller's event payload, so a
    // schema lookup (which InputDimensionFilters does) would throw here.
    renderInput([{ field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] }])

    expect(screen.getByDisplayValue('sku')).toBeTruthy()
    expect(screen.getByDisplayValue('A-1')).toBeTruthy()
  })

  it('renders each of the supported value types without an error alert', () => {
    const { container } = renderInput([
      { field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] },
      { field_name: 'qty', field_type: 'number', operator: 'gte', number_values: [2] },
      {
        field_name: 'renewed_at',
        field_type: 'time',
        operator: 'not_in_the_last_days',
        string_values: ['30']
      }
    ])

    expect(container.querySelector('.ant-alert-error')).toBeNull()
    expect(screen.getByDisplayValue('2')).toBeTruthy()
    expect(screen.getByText('days (or never)')).toBeTruthy()
  })

  it('removes a filter without disturbing the others', async () => {
    const onChange = vi.fn()
    renderInput(
      [
        { field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] },
        { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'] }
      ],
      onChange
    )

    fireEvent.click(screen.getAllByRole('button', { name: 'Remove this filter' })[0])

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls[0][0]).toEqual([
      { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'] }
    ])
  })

  it('shows the add button when there is nothing to filter yet', () => {
    renderInput(undefined)
    expect(screen.getByText('Add a property')).toBeTruthy()
  })
})

describe('InputEventPropertyFilters editing', () => {
  it('produces a text filter for a key that collides with a contact schema field', async () => {
    // "country" is a contact field the contact rows offer as an ISO country picker. As an event
    // property it is just a key, and an event may well store "United Kingdom" rather than "GB" —
    // so the row must offer a free text input, not a fixed list.
    const latest = renderEditable()

    fireEvent.click(screen.getByText('Add a property'))
    fireEvent.change(screen.getByPlaceholderText('e.g. sku'), { target: { value: 'country' } })
    await pick(rowSelects()[1], 'equals')

    const valueInput = screen.getByPlaceholderText('Enter a value')
    expect(valueInput.tagName).toBe('INPUT')
    // No third select: the value is not picked from a list
    expect(rowSelects()).toHaveLength(2)
    fireEvent.change(valueInput, { target: { value: 'United Kingdom' } })

    await waitFor(() =>
      expect(latest.value).toEqual([
        { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['United Kingdom'] }
      ])
    )
  })

  it('keeps a key ending in _score a plain number input', async () => {
    const latest = renderEditable()

    fireEvent.click(screen.getByText('Add a property'))
    fireEvent.change(screen.getByPlaceholderText('e.g. sku'), { target: { value: 'analysis_score' } })
    await pick(rowSelects()[0], 'Number')
    await pick(rowSelects()[1], 'greater than')
    fireEvent.change(screen.getByPlaceholderText('Enter a number'), { target: { value: '7' } })

    await waitFor(() =>
      expect(latest.value).toEqual([
        { field_name: 'analysis_score', field_type: 'number', operator: 'gt', number_values: [7] }
      ])
    )
  })

  it('offers the not_in_the_last_days operator for a Date property', async () => {
    renderEditable()

    fireEvent.click(screen.getByText('Add a property'))
    await pick(rowSelects()[0], 'Date')
    fireEvent.mouseDown(rowSelects()[1])

    expect(await screen.findByTitle('not in the last')).toBeTruthy()
  })

  it('clears the operator and the value when the type changes', async () => {
    const latest = renderEditable([
      { field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] }
    ])

    await pick(rowSelects()[0], 'Number')

    await waitFor(() => expect(latest.value).toEqual([{ field_name: 'sku', field_type: 'number' }]))
  })

  it('flags a duplicate property key', () => {
    const filters: DimensionFilter[] = [
      { field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] },
      { field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['B-2'] }
    ]
    renderInput(filters)

    expect(screen.getAllByText('This property is already filtered').length).toBeGreaterThan(0)
    expect(arePropertyFiltersValid(filters)).toBe(false)
  })

  it('refuses an incomplete row', () => {
    expect(arePropertyFiltersValid([{ field_name: 'sku', field_type: 'string' } as DimensionFilter])).toBe(false)
    expect(
      arePropertyFiltersValid([{ field_name: 'sku', field_type: 'string', operator: 'equals', string_values: [' '] }])
    ).toBe(false)
    expect(
      arePropertyFiltersValid([{ field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A'] }])
    ).toBe(true)
  })
})

describe('InputEventPropertyFilters focus', () => {
  it('hands the keyboard to the name of a property it adds', async () => {
    renderEditable([{ field_name: 'sku', field_type: 'string', operator: 'is_set' }])
    fireEvent.click(screen.getByText('Add a property'))
    await waitFor(() => {
      const keys = screen.getAllByRole('textbox', { name: 'Property name' })
      expect(keys).toHaveLength(2)
      expect(document.activeElement).toBe(keys[1])
    })
  })
})
