import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { message } from 'antd'
import { activateDayjsLocale } from '../../i18n/dayjsLocales'
import { contactsApi } from '../../services/api/contacts'
import { ContactsCsvUploadDrawer } from './ContactsCsvUploadDrawer'

const parse = vi.hoisted(() => vi.fn())
vi.mock('papaparse', () => ({ default: { parse } }))

vi.mock('../../services/api/contacts', () => ({
  contactsApi: { batchImport: vi.fn() }
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [] })
}))

const FILE_NAME = 'contacts.csv'

/** The CSV the parser hands back: a header row, then one row per address. */
const csvWithRows = (count: number) => {
  const rows = Array.from({ length: count }, (_, i) => [`c${i}@example.com`])
  parse.mockImplementation((_file: File, options: { complete: (r: { data: string[][] }) => void }) =>
    options.complete({ data: [['email'], ...rows] })
  )
}

const renderDrawer = () =>
  render(
    <I18nProvider i18n={i18n}>
      <ContactsCsvUploadDrawer workspaceId="ws1" isVisible onClose={vi.fn()} />
    </I18nProvider>
  )

const uploadCsv = async () => {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement
  await userEvent.upload(input, new File(['email\n'], FILE_NAME, { type: 'text/csv' }))
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  i18n.load('fr', {})
  i18n.activate('fr')
  activateDayjsLocale('fr')
})

afterEach(() => {
  vi.useRealTimers()
  i18n.activate('en')
  activateDayjsLocale('en')
})

describe('ContactsCsvUploadDrawer in French', () => {
  it('dates the upload it offers to resume in French, and counts its rows in French', async () => {
    // Only Date is faked: the upload and the confirm dialog keep their real timers
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 10, 14, 12, 0))
    csvWithRows(12345)
    localStorage.setItem(
      `csv_upload_progress_ws1_${FILE_NAME}`,
      JSON.stringify({
        fileName: FILE_NAME,
        currentRow: 1200,
        totalRows: 12345,
        currentBatch: 13,
        totalBatches: 124,
        mappings: { email: 'email' },
        selectedListIds: [],
        timestamp: new Date(2026, 10, 14, 10, 30).getTime()
      })
    )

    renderDrawer()
    await uploadCsv()

    // "lll" in French: day, short month, year, then the 24-hour time
    expect(await screen.findByText(/\(14 nov\. 2026 10:30\)/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Resume' }))
    expect(
      await screen.findByText(/The upload will resume from row 1\s201 of 12\s345\.$/)
    ).toBeInTheDocument()
  })

  it('counts the rows and the imported contacts with the French thousands separator', async () => {
    const success = vi.spyOn(message, 'success')
    csvWithRows(12345)
    vi.mocked(contactsApi.batchImport).mockImplementation(async ({ contacts }) => ({
      operations: contacts.map((c) => ({ email: c.email, action: 'create' }))
    }) as never)

    renderDrawer()
    await uploadCsv()

    // French groups thousands with a narrow no-break space
    expect(await screen.findByText(/^\(12\s345 rows\)$/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Start Upload/ }))

    await waitFor(
      () =>
        expect(
          screen.getByText(/^Processed 12\s345 contacts: 12\s345 successful, 0 failed$/)
        ).toBeInTheDocument(),
      { timeout: 10000 }
    )
    expect(success).toHaveBeenCalledWith(expect.stringMatching(/^Imported 12\s345 contacts$/))
  }, 20000)
})
