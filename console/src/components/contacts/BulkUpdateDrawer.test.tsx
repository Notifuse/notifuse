import '../../__tests__/resizeObserverMock'
import { beforeEach, describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { ReactNode } from 'react'
import { contactsApi } from '../../services/api/contacts'
import { contactListApi } from '../../services/api/contact_list'
import { downloadCsv } from '../../lib/csv'
import type { List } from '../../services/api/types'
import { BulkUpdateDrawer } from './BulkUpdateDrawer'

vi.mock('../../services/api/contacts', () => ({ contactsApi: { delete: vi.fn() } }))
vi.mock('../../services/api/contact_list', () => ({ contactListApi: { updateStatus: vi.fn() } }))
vi.mock('../../lib/csv', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/csv')>()),
  downloadCsv: vi.fn()
}))

const permissions = vi.hoisted(() => ({ value: { contacts: { write: true }, lists: { write: true } } }))
vi.mock('../../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ permissions: permissions.value, loading: false })
}))

const updateStatus = vi.mocked(contactListApi.updateStatus)
const LISTS = [{ id: 'l1', name: 'Newsletter' }] as List[]

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    <App>{children}</App>
  </QueryClientProvider>
)

const renderDrawer = (onClose = vi.fn()) =>
  render(<BulkUpdateDrawer workspaceId="ws" lists={LISTS} open onClose={onClose} />, { wrapper })

const dropFile = async (content: string, name = 'u.csv') => {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement
  await userEvent.upload(input, new File([content], name, { type: 'text/csv' }))
}

const pickList = async () => {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'List' }))
  await userEvent.click(await screen.findByTitle('Newsletter'))
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  permissions.value = { contacts: { write: true }, lists: { write: true } }
})

describe('BulkUpdateDrawer', () => {
  it('opens from its prop and asks the caller to close', async () => {
    const onClose = vi.fn()
    renderDrawer(onClose)
    expect(screen.getByRole('dialog', { name: 'Bulk update from CSV' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Contacts' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('starts on Unsubscribe from a list, the reversible action, with a primary button', async () => {
    renderDrawer()
    expect(screen.getByRole('radio', { name: /Unsubscribe from a list/ })).toBeChecked()
    expect(screen.getByRole('radio', { name: /Delete contacts/ })).not.toBeChecked()
    await dropFile('email\na@b.co\nc@d.co\n')
    const button = await screen.findByRole('button', { name: 'Unsubscribe 2 contacts' })
    // No list yet
    expect(button).toBeDisabled()
    await pickList()
    expect(button).toBeEnabled()
    expect(button.className).not.toContain('dangerous')
  })

  it('offers no Unsubscribe, and no default action, to a member who cannot write lists', async () => {
    permissions.value = { contacts: { write: true }, lists: { write: false } }
    renderDrawer()
    expect(screen.getByRole('radio', { name: /Unsubscribe from a list/ })).toBeDisabled()
    expect(screen.getAllByRole('radio').some((radio) => (radio as HTMLInputElement).checked)).toBe(false)
  })

  it('asks before deleting, with a danger button that names the count', async () => {
    vi.mocked(contactsApi.delete).mockResolvedValue({ success: true })
    renderDrawer()
    await userEvent.click(screen.getByRole('radio', { name: /Delete contacts/ }))
    await dropFile('email\na@b.co\n')
    const button = await screen.findByRole('button', { name: 'Delete 1 contact' })
    expect(button.className).toContain('dangerous')
    await userEvent.click(button)
    expect((await screen.findAllByText('Delete 1 contact?')).length).toBeGreaterThan(0)
    expect(contactsApi.delete).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(contactsApi.delete).toHaveBeenCalledWith({ workspace_id: 'ws', email: 'a@b.co' }))
  })

  it('counts rows, duplicates and invalid addresses, and follows a change of email column', async () => {
    renderDrawer()
    await dropFile('Email,Backup mail\nA@b.co,x@y.co\na@b.co,z@y.co\nnope,w@y.co\n')
    const summary = (await screen.findByText('Rows in the file')).closest('dl') as HTMLElement
    expect(within(summary).getByText('Duplicates removed').nextSibling).toHaveTextContent('1')
    expect(within(summary).getByText('Without a valid email').nextSibling).toHaveTextContent('1')
    expect(screen.getByText('1 contact')).toBeInTheDocument()

    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Email column' }))
    await userEvent.click(await screen.findByTitle('Backup mail'))
    expect(screen.getByText('3 contacts')).toBeInTheDocument()
    expect(within(summary).getByText('Duplicates removed').nextSibling).toHaveTextContent('0')
  })

  it('reports a contact that is not in the list as skipped, filters the results, and downloads them', async () => {
    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    updateStatus.mockResolvedValueOnce({ success: true, found: false, message: 'contact not in list' })
    updateStatus.mockRejectedValueOnce(new Error('timeout'))
    renderDrawer()
    await dropFile('email\na@b.co\nc@d.co\ne@f.co\n')
    await pickList()
    await userEvent.click(screen.getByRole('button', { name: 'Unsubscribe 3 contacts' }))

    expect(await screen.findByText('Done · 1 failed', {}, { timeout: 3000 })).toBeInTheDocument()
    // The run ended with a failure: the Failed tab is open
    const table = document.querySelector('.ant-table') as HTMLElement
    expect(within(table).getByText('e@f.co')).toBeInTheDocument()
    expect(within(table).queryByText('a@b.co')).toBeNull()

    await userEvent.click(screen.getByText('Skipped', { selector: '.ant-segmented-item-label span' }))
    expect(within(table).getByText('c@d.co')).toBeInTheDocument()
    expect(within(table).getByText('Not subscribed to this list')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Download as CSV/ }))
    expect(downloadCsv).toHaveBeenCalledWith(
      'email,result,detail\na@b.co,Unsubscribed,\nc@d.co,Skipped,Not subscribed to this list\ne@f.co,Failed,timeout',
      'u-results.csv'
    )
  })

  it('retries the failed addresses only', async () => {
    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    updateStatus.mockRejectedValueOnce(new Error('timeout'))
    renderDrawer()
    await dropFile('email\na@b.co\nc@d.co\n')
    await pickList()
    await userEvent.click(screen.getByRole('button', { name: 'Unsubscribe 2 contacts' }))
    const retry = await screen.findByRole('button', { name: 'Retry failed' }, { timeout: 3000 })
    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    await userEvent.click(retry)
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Retry failed' })).toBeNull())
    expect(updateStatus.mock.calls.map((call) => call[0].email)).toEqual(['a@b.co', 'c@d.co', 'c@d.co'])
    const strip = document.querySelector('[data-kpi-strip]') as HTMLElement
    expect(within(strip).getByText('Unsubscribed').parentElement).toHaveTextContent('2')
  })

  it('offers to resume a run of the same file, action, list and column', async () => {
    localStorage.setItem(
      'bulk_update_progress_ws_u.csv_unsubscribe',
      JSON.stringify({
        fileName: 'u.csv',
        operation: 'unsubscribe',
        listId: 'l1',
        emailColumn: 'email',
        currentIndex: 1,
        totalEmails: 2,
        results: [{ email: 'a@b.co', outcome: 'done' }],
        timestamp: Date.now() - 1000
      })
    )
    updateStatus.mockResolvedValue({ success: true, found: true })
    renderDrawer()
    await dropFile('email\na@b.co\nc@d.co\n')
    await pickList()
    await userEvent.click(await screen.findByRole('button', { name: 'Resume from address 1' }))
    await waitFor(() => expect(updateStatus).toHaveBeenCalledTimes(1))
    expect(updateStatus.mock.calls[0][0].email).toBe('c@d.co')
  })
})
