import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import type { Contact } from '../../services/api/contacts'
import type { Workspace } from '../../services/api/types'

vi.mock('../../services/api/contacts', () => ({ contactsApi: { upsert: vi.fn() } }))

import { ContactUpsertDrawer } from './ContactUpsertDrawer'

const workspace = { id: 'ws', settings: { timezone: 'UTC', custom_field_labels: {} } } as unknown as Workspace
const overwrite = /If a contact with this email already exists/

function renderDrawer(contact?: Contact) {
  render(
    <App>
      <ContactUpsertDrawer workspace={workspace} contact={contact} open onClose={vi.fn()} />
    </App>
  )
}

describe('ContactUpsertDrawer', () => {
  it('warns that an existing contact is overwritten when the email is typed', () => {
    renderDrawer()
    expect(screen.getByText(overwrite)).toBeInTheDocument()
  })

  it('does not speak of overwriting another contact when the email is locked to this one', () => {
    renderDrawer({ email: 'ann@example.com', first_name: 'Ann' } as Contact)
    expect(screen.getByDisplayValue('ann@example.com')).toBeDisabled()
    expect(screen.queryByText(overwrite)).toBeNull()
  })

  it('moves the focus to an optional field once it is added', async () => {
    renderDrawer()
    const picker = screen.getByRole('combobox')
    fireEvent.mouseDown(picker)
    fireEvent.change(picker, { target: { value: 'Job' } })
    await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
    const option = [...document.querySelectorAll('.ant-select-item-option')].find((el) => el.textContent === 'Job Title')
    fireEvent.click(option!)
    await waitFor(() => expect(document.getElementById('job_title')).not.toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(document.getElementById('job_title')))
  })
})
