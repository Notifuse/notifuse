import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within, waitFor } from '@testing-library/react'
import type { ContactsSearch } from '../../router'
import type { Segment } from '../../services/api/segment'
import { ContactsFilters, type ContactsFiltersProps } from './ContactsFilters'
import type { ContactFilterField } from './contactFilterFields'

const fields: ContactFilterField[] = [
  { key: 'email', group: 'identity', label: 'Email', type: 'string' },
  { key: 'first_name', group: 'identity', label: 'First Name', type: 'string' },
  {
    key: 'country',
    group: 'location',
    label: 'Country',
    type: 'string',
    options: [{ value: 'FR', label: 'FR - France' }]
  },
  {
    key: 'tracking_opens_consent',
    group: 'tracking',
    exact: true,
    label: 'Opens tracking consent',
    type: 'string',
    options: [{ value: 'granted', label: 'Granted' }]
  }
]

const segments = [
  { id: 'eu', name: 'Europe' },
  { id: 'pro', name: 'Plan Pro' }
] as Segment[]

function renderFilters(props: Partial<ContactsFiltersProps> = {}) {
  const handlers = { onApply: vi.fn(), onRemove: vi.fn(), onClear: vi.fn(), onCreateSegment: vi.fn() }
  render(
    <ContactsFilters
      search={{} as ContactsSearch}
      fields={fields}
      segments={segments}
      canCreateSegment={false}
      {...handlers}
      {...props}
    />
  )
  return handlers
}

const openPicker = () => fireEvent.click(screen.getByRole('button', { name: /Filter/ }))
const picker = () => document.querySelector('[data-filter-picker]') as HTMLElement
const editor = () => document.querySelector('[data-filter-editor]') as HTMLElement

describe('ContactsFilters', () => {
  it('groups the fields and narrows them with the search', () => {
    renderFilters()
    openPicker()
    expect(within(picker()).getByRole('group', { name: 'Identity' })).toBeInTheDocument()
    expect(within(picker()).getByRole('group', { name: 'Tracking' })).toBeInTheDocument()
    fireEvent.change(within(picker()).getByLabelText('Search the fields'), { target: { value: 'coun' } })
    expect(within(picker()).getByRole('button', { name: 'Country' })).toBeInTheDocument()
    expect(within(picker()).queryByRole('button', { name: 'Email' })).toBeNull()
    expect(within(picker()).queryByRole('group', { name: 'Identity' })).toBeNull()
    fireEvent.change(within(picker()).getByLabelText('Search the fields'), { target: { value: 'zzz' } })
    expect(within(picker()).getByText('No field matches')).toBeInTheDocument()
  })

  it('applies a text filter with Apply, which is disabled while the value is empty', () => {
    const { onApply } = renderFilters()
    openPicker()
    fireEvent.click(within(picker()).getByRole('button', { name: 'Email' }))
    expect(within(editor()).getByText('contains')).toBeInTheDocument()
    const apply = within(editor()).getByRole('button', { name: 'Apply' })
    expect(apply).toBeDisabled()
    fireEvent.change(within(editor()).getByPlaceholderText('Filter by Email'), { target: { value: ' @acme ' } })
    fireEvent.click(apply)
    expect(onApply).toHaveBeenCalledWith('email', '@acme')
  })

  it('applies a text filter with Enter, then gives the focus back to Filter', async () => {
    const { onApply } = renderFilters()
    openPicker()
    fireEvent.click(within(picker()).getByRole('button', { name: 'First Name' }))
    const input = within(editor()).getByPlaceholderText('Filter by First Name')
    fireEvent.change(input, { target: { value: 'Ann' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', keyCode: 13 })
    expect(onApply).toHaveBeenCalledWith('first_name', 'Ann')
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: /Filter/ })))
  })

  it('applies a field with options, Country, by its value', async () => {
    const { onApply } = renderFilters()
    openPicker()
    fireEvent.click(within(picker()).getByRole('button', { name: 'Country' }))
    expect(within(editor()).getByText('contains')).toBeInTheDocument()
    fireEvent.mouseDown(within(editor()).getByRole('combobox'))
    fireEvent.click(await screen.findByTitle('FR - France'))
    fireEvent.click(within(editor()).getByRole('button', { name: 'Apply' }))
    expect(onApply).toHaveBeenCalledWith('country', 'FR')
  })

  it('goes back to the fields and gives them the focus', async () => {
    renderFilters()
    openPicker()
    fireEvent.click(within(picker()).getByRole('button', { name: 'Email' }))
    fireEvent.click(within(editor()).getByRole('button', { name: 'Back' }))
    expect(picker()).not.toBeNull()
    await waitFor(() => expect(document.activeElement).toBe(within(picker()).getByLabelText('Search the fields')))
  })

  it('describes the active filters with what the server does, and reopens one prefilled', () => {
    renderFilters({
      search: { email: 'acme', country: 'FR', tracking_opens_consent: 'granted' } as unknown as ContactsSearch
    })
    expect(screen.getByRole('button', { name: 'Email contains acme' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Country contains FR - France' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Opens tracking consent is Granted' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Email contains acme' }))
    expect(within(editor()).getByPlaceholderText('Filter by Email')).toHaveValue('acme')
  })

  it('removes one filter with its ✕ and every filter with Clear', () => {
    const { onRemove, onClear } = renderFilters({ search: { email: 'acme' } as unknown as ContactsSearch })
    fireEvent.click(screen.getByRole('button', { name: 'Remove filter Email' }))
    expect(onRemove).toHaveBeenCalledWith('email')
    fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
    expect(onClear).toHaveBeenCalled()
  })

  it('shows no Clear without any filter', () => {
    renderFilters()
    expect(screen.queryByRole('button', { name: 'Clear' })).toBeNull()
  })

  it('shows the legacy segments as one pill removed as a whole', () => {
    const { onRemove } = renderFilters({ search: { segments: ['eu', 'pro'] } as unknown as ContactsSearch })
    expect(screen.getByText('In any of: Europe, Plan Pro')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Remove filter Segments' }))
    expect(onRemove).toHaveBeenCalledWith('segments')
  })

  it('offers to create a segment only with segments write', () => {
    renderFilters()
    openPicker()
    expect(within(picker()).queryByText('Advanced conditions: create a segment')).toBeNull()
  })

  it('creates a segment from Advanced conditions', () => {
    const { onCreateSegment } = renderFilters({ canCreateSegment: true })
    openPicker()
    fireEvent.click(within(picker()).getByText('Advanced conditions: create a segment'))
    expect(onCreateSegment).toHaveBeenCalled()
  })
})
