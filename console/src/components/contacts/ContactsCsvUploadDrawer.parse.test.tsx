import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { contactsApi } from '../../services/api/contacts'
import { ContactsCsvUploadDrawer } from './ContactsCsvUploadDrawer'

// Unlike ContactsCsvUploadDrawer.test.tsx, papaparse is not mocked here: these tests feed the
// drawer real files, so what it counts is what the real parser makes of them.

vi.mock('../../services/api/contacts', () => ({
  contactsApi: { batchImport: vi.fn() }
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [] })
}))

const renderDrawer = () =>
  render(
    <I18nProvider i18n={i18n}>
      <ContactsCsvUploadDrawer workspaceId="ws1" isVisible onClose={vi.fn()} />
    </I18nProvider>
  )

const uploadCsv = async (content: string) => {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement
  await userEvent.upload(input, new File([content], 'contacts.csv', { type: 'text/csv' }))
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  i18n.load('en', {})
  i18n.activate('en')
  vi.mocked(contactsApi.batchImport).mockImplementation(async ({ contacts }) => ({
    operations: contacts.map((c) => ({ email: c.email, action: 'create' }))
  }) as never)
})

describe('ContactsCsvUploadDrawer with the real CSV parser', () => {
  it('does not count the newline that ends the file as a row, nor import it', async () => {
    renderDrawer()
    await uploadCsv('email,first_name\nann@example.com,Ann\nbob@example.com,Bob\n')

    expect((await screen.findByText(/^\(\d+ rows\)$/)).textContent).toBe('(2 rows)')

    await userEvent.click(screen.getByRole('button', { name: /Start Upload/ }))

    await waitFor(() =>
      expect(screen.getByText('Processed 2 contacts: 2 successful, 0 failed')).toBeInTheDocument()
    )
    const sent = vi.mocked(contactsApi.batchImport).mock.calls.flatMap(([req]) => req.contacts.map((c) => c.email))
    expect(sent).toEqual(['ann@example.com', 'bob@example.com'])
  })

  it('skips blank lines inside the file and Windows line endings', async () => {
    renderDrawer()
    await uploadCsv('email,first_name\r\nann@example.com,Ann\r\n\r\n,\r\nbob@example.com,Bob\r\n')

    expect((await screen.findByText(/^\(\d+ rows\)$/)).textContent).toBe('(2 rows)')
  })
})
