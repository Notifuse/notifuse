import { useState } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { FilterRow } from './FilterRow'
import { ContactsTableSchema, WebSessionChangesSchema } from './table_schemas'
import type { DimensionFilter } from '../../services/api/segment'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// A row kept in state, as the condition's form keeps it, with what it last handed back.
const renderRow = (initial: DimensionFilter, extra: { showErrors?: boolean } = {}) => {
  const latest: { value: DimensionFilter; changes: number } = { value: initial, changes: 0 }
  const Wrapper = () => {
    const [filter, setFilter] = useState(initial)
    return (
      <FilterRow
        schema={ContactsTableSchema}
        filter={filter}
        onChange={(next) => {
          latest.value = next
          latest.changes++
          setFilter(next)
        }}
        onDelete={vi.fn()}
        {...extra}
      />
    )
  }
  const result = render(
    <I18nProvider i18n={i18n}>
      <Wrapper />
    </I18nProvider>
  )
  return { latest, ...result }
}

const comboboxes = () => screen.getAllByRole('combobox')

const openAt = (index: number) => fireEvent.mouseDown(comboboxes()[index])

// The labels of the options of the open dropdown.
const openOptions = () =>
  [...document.querySelectorAll('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option')].map(
    (element) => element.textContent ?? ''
  )

const blank = { field_name: '', field_type: 'string' } as unknown as DimensionFilter

describe('FilterRow', () => {
  it.each([
    ['email', ['is set', 'is not set', 'equals', "doesn't equal", 'contains', "doesn't contain"]],
    [
      'custom_number_1',
      [
        'is set',
        'is not set',
        'equals',
        "doesn't equal",
        'greater than',
        'less than',
        'greater than or equal to',
        'less than or equal to'
      ]
    ],
    [
      'created_at',
      ['is set', 'is not set', 'before', 'after', 'between', 'not between', 'in the last', 'not in the last']
    ]
  ])('offers the operators of %s', (fieldName, operators) => {
    renderRow({ field_name: fieldName, field_type: ContactsTableSchema.fields[fieldName].type } as DimensionFilter)
    openAt(1)
    expect(openOptions()).toEqual(operators)
  })

  it('sets the field type from the schema when a field is picked', async () => {
    const { latest } = renderRow(blank)
    openAt(0)
    fireEvent.change(comboboxes()[0], { target: { value: 'Created' } })
    const option = await waitFor(() => {
      const found = [...document.querySelectorAll('.ant-select-item-option')].find((element) =>
        element.textContent?.includes('Created At')
      )
      expect(found).toBeTruthy()
      return found as HTMLElement
    })
    fireEvent.click(option)
    await waitFor(() => expect(latest.value).toEqual({ field_name: 'created_at', field_type: 'time' }))
  })

  it('takes several values for contains', async () => {
    const { latest } = renderRow({ field_name: 'email', field_type: 'string', operator: 'contains' })
    const input = comboboxes()[2]
    fireEvent.change(input, { target: { value: 'gmail' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter' })
    fireEvent.change(input, { target: { value: 'yahoo' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(latest.value.string_values).toEqual(['gmail', 'yahoo']))
  })

  it('keeps each value as typed, trimmed, commas included', async () => {
    const { latest } = renderRow({ field_name: 'job_title', field_type: 'string', operator: 'contains' })
    const input = comboboxes()[2]
    fireEvent.change(input, { target: { value: 'Director, Sales ' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter' })
    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(latest.value.string_values).toEqual(['Director, Sales']))
  })

  it('picks the country of a web visit from the list', async () => {
    const latest: { value?: DimensionFilter } = {}
    const Wrapper = () => {
      const [filter, setFilter] = useState<DimensionFilter>({
        field_name: 'country',
        field_type: 'string',
        operator: 'equals'
      })
      return (
        <FilterRow
          schema={WebSessionChangesSchema}
          filter={filter}
          onChange={(next) => {
            latest.value = next
            setFilter(next)
          }}
          onDelete={vi.fn()}
        />
      )
    }
    render(
      <I18nProvider i18n={i18n}>
        <Wrapper />
      </I18nProvider>
    )
    openAt(2)
    fireEvent.change(comboboxes()[2], { target: { value: 'France' } })
    fireEvent.click(await screen.findByTitle('FR - France'))
    await waitFor(() => expect(latest.value?.string_values).toEqual(['FR']))
  })

  it('picks a country from the list', async () => {
    const { latest } = renderRow({ field_name: 'country', field_type: 'string', operator: 'equals' })
    openAt(2)
    fireEvent.change(comboboxes()[2], { target: { value: 'France' } })
    fireEvent.click(await screen.findByTitle('FR - France'))
    await waitFor(() => expect(latest.value.string_values).toEqual(['FR']))
  })

  it('offers the consent values as a closed list', () => {
    renderRow({ field_name: 'tracking_opens_consent', field_type: 'string', operator: 'equals' })
    openAt(2)
    expect(openOptions()).toEqual(['Granted', 'Denied'])
  })

  it('offers the known consent sources', () => {
    renderRow({ field_name: 'tracking_consent_source', field_type: 'string', operator: 'equals' })
    openAt(2)
    expect(openOptions()).toContain('Notification center')
    expect(openOptions()).toContain('Email link')
  })

  it('refuses a day count of zero', () => {
    renderRow({ field_name: 'created_at', field_type: 'time', operator: 'in_the_last_days' })
    const input = screen.getByRole('spinbutton')
    expect(input.getAttribute('aria-valuemin')).toBe('1')
    expect(screen.getByText('days')).toBeTruthy()
  })

  it('says that never-set dates match « not in the last »', () => {
    renderRow({
      field_name: 'created_at',
      field_type: 'time',
      operator: 'not_in_the_last_days',
      string_values: ['30']
    })
    expect(screen.getByText('days (or never)')).toBeTruthy()
  })

  it('clears the value when the operator changes', async () => {
    const { latest } = renderRow({
      field_name: 'email',
      field_type: 'string',
      operator: 'equals',
      string_values: ['a@b.c']
    })
    openAt(1)
    fireEvent.click(await screen.findByTitle('contains'))
    await waitFor(() =>
      expect(latest.value).toEqual({ field_name: 'email', field_type: 'string', operator: 'contains' })
    )
  })

  it('hands nothing back for a row nobody touches', () => {
    const { latest } = renderRow({
      field_name: 'custom_json_1',
      field_type: 'json',
      operator: 'equals',
      json_path: ['plan'],
      string_values: ['pro']
    })
    expect(latest.changes).toBe(0)
    // field_type « json » (written through the API) reads as text, unchanged
    expect(document.querySelector('.ant-segmented-item-selected')?.textContent).toBe('Text')
  })

  it('accepts a JSON existence check without a path', () => {
    renderRow({ field_name: 'custom_json_1', field_type: 'string', operator: 'is_set' })
    expect(document.querySelector('.ant-alert-error')).toBeNull()
  })

  it('shows an operator it does not know as an error, without rewriting it', () => {
    const { latest } = renderRow({
      field_name: 'email',
      field_type: 'string',
      operator: 'matches' as DimensionFilter['operator'],
      string_values: ['x']
    })
    expect(screen.getByText('The operator matches is not supported here')).toBeTruthy()
    expect(latest.changes).toBe(0)
  })

  it('marks what is missing once Confirm was refused', () => {
    const { container } = renderRow(
      { field_name: 'email', field_type: 'string', operator: 'equals' },
      { showErrors: true }
    )
    expect(container.querySelector('.ant-input-status-error')).toBeTruthy()
  })
})

describe('FilterRow on a JSON value read as a date', () => {
  const jsonDate = { field_name: 'custom_json_1', field_type: 'time', json_path: ['renewed_at'] } as DimensionFilter

  it('offers no range and no greater / less than', () => {
    renderRow(jsonDate)
    // Field, then the row's operator (the value type is a segmented control)
    openAt(1)
    expect(openOptions()).toEqual([
      'is set',
      'is not set',
      'equals',
      "doesn't equal",
      'before',
      'after',
      'in the last',
      'not in the last'
    ])
  })

  it('saves an after-date comparison as an ISO string, never a number', async () => {
    const { latest } = renderRow({ ...jsonDate, operator: 'after_date' })
    const input = screen.getByPlaceholderText('Select date')
    fireEvent.mouseDown(input)
    fireEvent.change(input, { target: { value: '2026-10-01 00:00:00' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(latest.value.string_values?.[0]).toMatch(/^2026-\d{2}-\d{2}T/))
    expect(latest.value.number_values).toBeUndefined()
  })

  it('still shows a saved « greater than » with a date picker', () => {
    renderRow({ ...jsonDate, operator: 'gt', string_values: ['2026-01-01T00:00:00.000Z'] })
    expect(screen.getByTitle('greater than')).toBeTruthy()
    expect(screen.getByPlaceholderText('Select date')).toBeTruthy()
  })

  // A JSON value reads as a date everywhere, automation entry conditions included, since their
  // casts read the value without its quotes (44.0).
  it('offers the Date reading of a JSON value', () => {
    renderRow({ field_name: 'custom_json_1', field_type: 'string', json_path: ['plan'] } as DimensionFilter)
    const types = [...document.querySelectorAll('.ant-segmented-item')].map((element) => element.textContent)
    expect(types).toEqual(['Text', 'Number', 'Date'])
  })
})
