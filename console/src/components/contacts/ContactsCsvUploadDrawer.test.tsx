import '../../__tests__/resizeObserverMock'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import { activateDayjsLocale } from '../../i18n/dayjsLocales'
import { contactsApi, type BatchImportContactsResponse } from '../../services/api/contacts'
import { downloadCsv } from '../../lib/csv'
import type { List } from '../../services/api/types'
import { ContactsCsvUploadDrawer } from './ContactsCsvUploadDrawer'

const parse = vi.hoisted(() => vi.fn())
vi.mock('papaparse', () => ({ default: { parse } }))

vi.mock('../../services/api/contacts', () => ({ contactsApi: { batchImport: vi.fn() } }))

vi.mock('../../lib/csv', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/csv')>()),
  downloadCsv: vi.fn()
}))

const permissions = vi.hoisted(() => ({
  value: { contacts: { write: true }, lists: { write: true } } as Record<string, { write: boolean }> | null,
  loading: false
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [] }),
  useWorkspacePermissions: () => ({ permissions: permissions.value, loading: permissions.loading })
}))

const batchImport = vi.mocked(contactsApi.batchImport)
const FILE_NAME = 'contacts.csv'
const LISTS = [{ id: 'l1', name: 'Newsletter' }] as List[]

/** The CSV the parser hands back: a header row, then the rows. */
const csv = (headers: string[], rows: string[][]) =>
  parse.mockImplementation((_file: File, options: { complete: (r: { data: string[][] }) => void }) =>
    options.complete({ data: [headers, ...rows] })
  )
const csvWithRows = (count: number) =>
  csv(['email'], Array.from({ length: count }, (_, i) => [`c${i}@example.com`]))

const createdAll = () =>
  batchImport.mockImplementation(
    async ({ contacts }) =>
      ({ operations: contacts.map((c) => ({ email: c.email, action: 'create' })) }) as BatchImportContactsResponse
  )

/** batchImport answers only when the test releases it. */
function heldBatches() {
  const releases: Array<() => void> = []
  batchImport.mockImplementation(
    ({ contacts }) =>
      new Promise((resolve) =>
        releases.push(() =>
          resolve({ operations: contacts.map((c) => ({ email: c.email, action: 'create' })) } as BatchImportContactsResponse)
        )
      )
  )
  return { release: () => releases.shift()?.() }
}

const saveProgress = (overrides: Record<string, unknown> = {}) =>
  localStorage.setItem(
    `csv_upload_progress_ws1_${FILE_NAME}`,
    JSON.stringify({
      fileName: FILE_NAME,
      currentRow: 100,
      totalRows: 250,
      currentBatch: 2,
      totalBatches: 3,
      mappings: { email: 'email' },
      selectedListIds: [],
      timestamp: Date.now() - 60_000,
      ...overrides
    })
  )

const renderDrawer = (props: { onClose?: () => void; onSuccess?: () => void; selectedList?: string } = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <ContactsCsvUploadDrawer
          workspaceId="ws1"
          lists={LISTS}
          isVisible
          onClose={props.onClose ?? vi.fn()}
          onSuccess={props.onSuccess}
          selectedList={props.selectedList}
        />
      </App>
    </I18nProvider>
  )

const uploadCsv = async () => {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement
  await userEvent.upload(input, new File(['email\n'], FILE_NAME, { type: 'text/csv' }))
}

const importButton = () => screen.getByRole('button', { name: /^Import \S+ contacts?$/ })

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  permissions.value = { contacts: { write: true }, lists: { write: true } }
  permissions.loading = false
  i18n.activate('en')
})

afterEach(() => {
  vi.useRealTimers()
  i18n.activate('en')
  activateDayjsLocale('en')
})

describe('ContactsCsvUploadDrawer in French', () => {
  beforeEach(() => {
    i18n.load('fr', {})
    i18n.activate('fr')
    activateDayjsLocale('fr')
  })

  it('dates and counts an unfinished import in French, in the list and in the resume banner', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 10, 14, 12, 0))
    csvWithRows(12345)
    saveProgress({ currentRow: 1200, totalRows: 12345, timestamp: new Date(2026, 10, 14, 10, 30).getTime() })

    renderDrawer()
    const unfinished = screen.getByRole('list', { name: 'Unfinished imports' })
    // "lll" in French: day, short month, year, then the 24-hour time
    expect(within(unfinished).getByText('14 nov. 2026 10:30')).toBeInTheDocument()
    expect(within(unfinished).getByText(/^row 1\s200 of 12\s345$/)).toBeInTheDocument()

    await uploadCsv()
    expect(await screen.findByText(/partly imported on 14 nov\. 2026 10:30/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Resume from row 1\s200$/ })).toBeInTheDocument()
  })

  it('counts the rows and the imported contacts with the French thousands separator', async () => {
    csvWithRows(1500)
    createdAll()

    renderDrawer()
    await uploadCsv()
    // French groups thousands with a narrow no-break space
    expect(await screen.findByText(/^1\s500 rows · 1 column$/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /^Import 1\s500 contacts$/ }))

    const strip = await waitFor(() => {
      const element = document.querySelector('[data-kpi-strip]') as HTMLElement
      expect(within(element).getByText('Created').parentElement).toHaveTextContent(/1\s500/)
      return element
    }, { timeout: 8000 })
    expect(within(strip).getByText('Processed').parentElement).toHaveTextContent(/1\s500/)
  }, 15000)
})

describe('ContactsCsvUploadDrawer', () => {
  it('says "contact" for a file of one row', async () => {
    csvWithRows(1)
    renderDrawer()
    await uploadCsv()
    expect(await screen.findByRole('button', { name: 'Import 1 contact' })).toBeInTheDocument()
  })

  it('lists an unfinished import and forgets it on request', async () => {
    saveProgress()
    renderDrawer()
    await userEvent.click(screen.getByRole('button', { name: `Forget ${FILE_NAME}` }))
    expect(screen.queryByRole('list', { name: 'Unfinished imports' })).toBeNull()
    expect(localStorage.getItem(`csv_upload_progress_ws1_${FILE_NAME}`)).toBeNull()
  })

  it('resumes the same file from the row it stopped at, with the lists that still exist', async () => {
    csvWithRows(250)
    saveProgress({ selectedListIds: ['gone', 'l1'] })
    createdAll()
    renderDrawer()
    await uploadCsv()

    await userEvent.click(await screen.findByRole('button', { name: 'Resume from row 100' }))

    await waitFor(() => expect(batchImport).toHaveBeenCalledTimes(2))
    expect(batchImport.mock.calls[0][0].contacts[0].email).toBe('c100@example.com')
    expect(batchImport.mock.calls[0][0].subscribe_to_lists).toEqual(['l1'])
  })

  it('offers only a fresh start when the file no longer has the saved number of rows', async () => {
    csvWithRows(300)
    saveProgress()
    renderDrawer()
    await uploadCsv()
    expect(await screen.findByText(/different number of rows/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Resume from row/ })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Start over' }))
    expect(screen.queryByText(/partly imported/)).toBeNull()
  })

  it('falls back to mapping by hand when the saved mapping does not fit the file', async () => {
    csvWithRows(250)
    saveProgress({ mappings: { email: 'Email address' } })
    renderDrawer()
    await uploadCsv()
    await userEvent.click(await screen.findByRole('button', { name: 'Resume from row 100' }))
    expect(await screen.findByText(/saved column mapping does not fit this file/)).toBeInTheDocument()
    expect(batchImport).not.toHaveBeenCalled()
    expect(importButton()).toBeEnabled()
  })

  it('keeps Import disabled until a column is mapped to Email', async () => {
    csv(['adresse', 'nom'], [['a@b.co', 'Anna']])
    renderDrawer()
    await uploadCsv()
    expect(await screen.findByText('Map a column to Email to import')).toBeInTheDocument()
    expect(importButton()).toBeDisabled()

    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Field for the column adresse' }))
    await userEvent.click(await screen.findByTitle('Email'))
    expect(importButton()).toBeEnabled()
    expect(screen.getByText('Email column mapped')).toBeInTheDocument()
  })

  it('imports into no list, with the list picker disabled, for a member who cannot write lists', async () => {
    permissions.value = { contacts: { write: true }, lists: { write: false } }
    csvWithRows(3)
    createdAll()
    renderDrawer({ selectedList: 'l1' })
    await uploadCsv()
    expect(await screen.findByRole('combobox', { name: 'Lists' })).toBeDisabled()
    await userEvent.click(importButton())
    await waitFor(() => expect(batchImport).toHaveBeenCalled())
    expect(batchImport.mock.calls[0][0].subscribe_to_lists).toBeUndefined()
  })

  it('waits for the permissions before importing, keeping the preset list', async () => {
    permissions.value = null
    permissions.loading = true
    csvWithRows(3)
    renderDrawer({ selectedList: 'l1' })
    await uploadCsv()
    expect(await screen.findByRole('button', { name: 'Import 3 contacts' })).toBeDisabled()
    expect(screen.getByRole('combobox', { name: 'Lists' })).toBeEnabled()
  })

  it('counts created, updated and failed contacts, and downloads the failed rows with their error', async () => {
    csv(['email', 'name'], [['a@x.co', 'Anna'], ['b@x.co', 'Ben'], ['c@x', 'Cy']])
    batchImport.mockResolvedValue({
      operations: [
        { email: 'c@x', action: 'error', error: 'invalid contact at index 2: invalid email address' },
        { email: 'a@x.co', action: 'create' },
        { email: 'b@x.co', action: 'update' }
      ]
    } as BatchImportContactsResponse)
    const onSuccess = vi.fn()
    renderDrawer({ onSuccess })
    await uploadCsv()
    await userEvent.click(importButton())

    expect(await screen.findByText('Done · 1 failed')).toBeInTheDocument()
    const strip = document.querySelector('[data-kpi-strip]') as HTMLElement
    expect(within(strip).getByText('Created').parentElement).toHaveTextContent('1')
    expect(within(strip).getByText('Updated').parentElement).toHaveTextContent('1')
    expect(screen.getByText('invalid email address')).toBeInTheDocument()
    expect(onSuccess).toHaveBeenCalledTimes(1)

    await userEvent.click(screen.getByRole('button', { name: /Download failed rows/ }))
    expect(downloadCsv).toHaveBeenCalledWith('email,name,error\nc@x,Cy,invalid email address', 'contacts-failed.csv')
  })

  it('starts again from the file step after "Import another file"', async () => {
    csvWithRows(2)
    createdAll()
    renderDrawer()
    await uploadCsv()
    await userEvent.click(importButton())
    await userEvent.click(await screen.findByRole('button', { name: 'Import another file' }))
    expect(screen.getByText('Drop a CSV file here, or click to browse')).toBeInTheDocument()
    expect(screen.queryByText('Done')).toBeNull()
  })

  it('sends no further batch once stopped, and offers to resume from where it stopped', async () => {
    csvWithRows(250)
    const held = heldBatches()
    renderDrawer()
    await uploadCsv()
    await userEvent.click(importButton())
    await waitFor(() => expect(batchImport).toHaveBeenCalledTimes(1))

    await userEvent.click(screen.getByRole('button', { name: 'Stop' }))
    held.release()

    expect(await screen.findByRole('button', { name: 'Resume from row 100' })).toBeInTheDocument()
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(batchImport).toHaveBeenCalledTimes(1)
  })

  it('asks before closing while the import runs, then stops it', async () => {
    csvWithRows(250)
    const held = heldBatches()
    const onClose = vi.fn()
    renderDrawer({ onClose })
    await uploadCsv()
    await userEvent.click(importButton())
    await waitFor(() => expect(batchImport).toHaveBeenCalledTimes(1))

    await userEvent.click(screen.getByRole('button', { name: 'Contacts' }))
    expect((await screen.findAllByText('Stop the import?')).length).toBeGreaterThan(0)
    expect(onClose).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Stop and close' }))
    expect(onClose).toHaveBeenCalledTimes(1)

    held.release()
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(batchImport).toHaveBeenCalledTimes(1)
  })
})
