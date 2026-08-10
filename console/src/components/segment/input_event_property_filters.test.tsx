import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { InputEventPropertyFilters } from './input_event_property_filters'
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

describe('InputEventPropertyFilters', () => {
  it('renders an arbitrary property key that no schema declares', () => {
    // The whole point of this component: keys come from the caller's event payload, so a
    // schema lookup (which InputDimensionFilters does) would throw here.
    renderInput([
      {
        field_name: 'sku',
        field_type: 'string',
        operator: 'equals',
        string_values: ['A-1']
      }
    ])

    expect(screen.getByText('sku')).toBeTruthy()
    expect(screen.getByText('A-1')).toBeTruthy()
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

    fireEvent.click(screen.getAllByRole('button')[0])
    fireEvent.click(await screen.findByText('OK'))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls[0][0]).toEqual([
      { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'] }
    ])
  })

  it('shows the add button when there is nothing to filter yet', () => {
    renderInput(undefined)
    expect(screen.getByText('+ Add property filter')).toBeTruthy()
  })
})

describe('InputEventPropertyFilters add flow', () => {
  const openModal = async () => {
    fireEvent.click(screen.getByText('+ Add property filter'))
    return screen.findByText('Filter on an event property')
  }

  const modalSelects = () => within(screen.getByRole('dialog')).getAllByRole('combobox')

  const pickOperator = async (label: string) => {
    // The operator select is the second combobox in the modal (after the value type)
    const selects = modalSelects()
    fireEvent.mouseDown(selects[selects.length - 1])
    fireEvent.click(await screen.findByTitle(label))
  }

  it('produces a text filter for a key that collides with a contact schema field', async () => {
    // "country" is a contact field the shared renderer swaps for an ISO country picker. As an
    // event property it is just a key, and an event may well store "United Kingdom" rather than
    // "GB" — so the modal must offer a free text input, not a fixed list.
    const onChange = vi.fn()
    renderInput([], onChange)

    await openModal()
    fireEvent.change(screen.getByPlaceholderText('e.g. sku'), { target: { value: 'country' } })
    await pickOperator('equals')

    const valueInput = screen.getByPlaceholderText('enter a value')
    expect(valueInput.tagName).toBe('INPUT')
    fireEvent.change(valueInput, { target: { value: 'United Kingdom' } })

    fireEvent.click(screen.getByText('Confirm'))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls[0][0]).toEqual([
      {
        field_name: 'country',
        field_type: 'string',
        operator: 'equals',
        string_values: ['United Kingdom']
      }
    ])
  })

  it('offers the new not_in_the_last_days operator for a Date property', async () => {
    renderInput([])
    await openModal()
    fireEvent.change(screen.getByPlaceholderText('e.g. sku'), { target: { value: 'renewed_at' } })

    // Switch the value type to Date
    fireEvent.mouseDown(modalSelects()[0])
    fireEvent.click(await screen.findByTitle('Date'))

    const selects = modalSelects()
    fireEvent.mouseDown(selects[selects.length - 1])

    expect(await screen.findByTitle('not in the last')).toBeTruthy()
  })

  it('rejects a duplicate property key', async () => {
    const onChange = vi.fn()
    renderInput(
      [{ field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] }],
      onChange
    )

    await openModal()
    fireEvent.change(screen.getByPlaceholderText('e.g. sku'), { target: { value: 'sku' } })
    fireEvent.click(screen.getByText('Confirm'))

    expect(await screen.findByText('This property is already filtered')).toBeTruthy()
    expect(onChange).not.toHaveBeenCalled()
  })
})
