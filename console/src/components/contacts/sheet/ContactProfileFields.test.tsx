import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { App } from 'antd'
import type { ReactNode } from 'react'
import type { Contact } from '../../../services/api/contacts'
import type { Workspace } from '../../../services/api/types'

vi.mock('@tanstack/react-router', async () => ({
  ...(await vi.importActual('@tanstack/react-router')),
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>
}))
vi.mock('../InlineEditableField', async () => {
  const { useState } = await import('react')
  return {
    InlineEditableField: ({
      fieldKey,
      value,
      disabled,
      hideLabel
    }: {
      fieldKey: string
      value: unknown
      disabled?: boolean
      hideLabel?: boolean
    }) => {
      // The field this instance was mounted for, as an open editor's state would remember it.
      const [mountedFor] = useState(fieldKey)
      return (
        <span
          data-field={fieldKey}
          data-mounted-for={mountedFor}
          data-disabled={String(!!disabled)}
          data-hide-label={String(!!hideLabel)}
        >
          {value === undefined || value === null || value === '' ? '' : String(value)}
        </span>
      )
    }
  }
})

import { ContactProfileFields } from './ContactProfileFields'

const workspace = {
  id: 'ws',
  settings: { timezone: 'UTC', custom_field_labels: { custom_string_3: 'Plan' } }
} as unknown as Workspace

const contact = {
  email: 'ann@x.com',
  first_name: 'Ann',
  timezone: 'Europe/Paris',
  custom_string_1: 'gold',
  contact_lists: []
} as unknown as Contact

function renderFields(canWrite = true, shown: Contact = contact) {
  const ui = (c: Contact) => (
    <App>
      <ContactProfileFields workspace={workspace} contact={c} canWrite={canWrite} onSave={vi.fn()} />
    </App>
  )
  const result = render(ui(shown))
  return { rerender: (c: Contact) => result.rerender(ui(c)) }
}

const fieldKeys = () =>
  Array.from(document.querySelectorAll('[data-field]')).map((el) => el.getAttribute('data-field'))

describe('ContactProfileFields', () => {
  it('shows the filled fields and folds the empty ones behind a count', () => {
    renderFields()
    expect(fieldKeys()).toEqual(['first_name', 'timezone', 'custom_string_1'])
    fireEvent.click(screen.getByRole('button', { name: '11 empty fields' }))
    expect(fieldKeys()).toContain('last_name')
    expect(document.querySelectorAll('[data-hide-label="true"]').length).toBe(fieldKeys().length)
  })

  it('keeps each field on its own row when a field moves from empty to filled', () => {
    const { rerender } = renderFields()
    fireEvent.click(screen.getByRole('button', { name: '11 empty fields' }))
    rerender({ ...contact, last_name: 'Lee' } as Contact)
    const rows = Array.from(document.querySelectorAll('[data-field]'))
    expect(rows.map((el) => el.getAttribute('data-field'))).toContain('last_name')
    for (const el of rows) expect(el.getAttribute('data-mounted-for')).toBe(el.getAttribute('data-field'))
  })

  it('lists the named empty custom fields first', () => {
    renderFields()
    fireEvent.click(screen.getByRole('button', { name: '19 empty fields' }))
    const custom = fieldKeys().filter((key) => key?.startsWith('custom_'))
    expect(custom.slice(0, 2)).toEqual(['custom_string_1', 'custom_string_3'])
  })

  it('shows the contact local time next to its timezone', () => {
    renderFields()
    const row = screen.getByText('Europe/Paris').closest('dd') as HTMLElement
    expect(within(row).getByText(/^Local time \d{2}:\d{2}$/)).toBeInTheDocument()
  })

  it('makes the fields read-only without contacts write', () => {
    renderFields(false)
    expect(document.querySelectorAll('[data-disabled="false"]').length).toBe(0)
  })
})
