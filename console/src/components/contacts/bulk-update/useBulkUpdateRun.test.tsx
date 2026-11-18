import { beforeEach, describe, it, expect, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { contactsApi } from '../../../services/api/contacts'
import { contactListApi } from '../../../services/api/contact_list'
import { readBulkRun } from './bulkProgressStore'
import { useBulkUpdateRun, type BulkRunInput } from './useBulkUpdateRun'

vi.mock('../../../services/api/contacts', () => ({ contactsApi: { delete: vi.fn() } }))
vi.mock('../../../services/api/contact_list', () => ({ contactListApi: { updateStatus: vi.fn() } }))

const updateStatus = vi.mocked(contactListApi.updateStatus)
const deleteContact = vi.mocked(contactsApi.delete)

let client: QueryClient
const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={client}>{children}</QueryClientProvider>
)

const EMAILS = ['a@b.co', 'c@d.co', 'e@f.co', 'g@h.co']

const input = (emails: string[], overrides: Partial<BulkRunInput> = {}): BulkRunInput => ({
  fileName: 'u.csv',
  operation: 'unsubscribe',
  listId: 'l1',
  emailColumn: 'Email',
  emails,
  fromIndex: 0,
  previous: [],
  ...overrides
})

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  client = new QueryClient()
})

describe('useBulkUpdateRun', () => {
  it('reports a contact that is not in the list as skipped', async () => {
    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    updateStatus.mockResolvedValueOnce({ success: true, found: false, message: 'contact not in list' })
    const { result } = renderHook(() => useBulkUpdateRun('ws1'), { wrapper })
    await act(async () => result.current.start(input(['a@b.co', 'c@d.co'])))
    expect(result.current.state.results).toEqual([
      { email: 'a@b.co', outcome: 'done' },
      { email: 'c@d.co', outcome: 'skipped', detail: 'Not subscribed to this list' }
    ])
    expect(result.current.state.status).toBe('done')
    expect(updateStatus).toHaveBeenCalledWith({ workspace_id: 'ws1', email: 'a@b.co', list_id: 'l1', status: 'unsubscribed' })
  })

  it('counts a delete answered 200 as deleted, and a failed request as failed', async () => {
    deleteContact.mockResolvedValueOnce({ success: true })
    deleteContact.mockRejectedValueOnce(new Error('Internal server error'))
    const { result } = renderHook(() => useBulkUpdateRun('ws1'), { wrapper })
    await act(async () => result.current.start(input(['a@b.co', 'c@d.co'], { operation: 'delete', listId: undefined })))
    expect(result.current.state.results).toEqual([
      { email: 'a@b.co', outcome: 'done' },
      { email: 'c@d.co', outcome: 'failed', detail: 'Internal server error' }
    ])
  })

  it('sends nothing more once stopped, and saves where it stopped', async () => {
    let release: () => void = () => {}
    updateStatus.mockImplementation(() => new Promise((resolve) => (release = () => resolve({ success: true, found: true }))))
    const { result } = renderHook(() => useBulkUpdateRun('ws1'), { wrapper })
    act(() => void result.current.start(input(['a@b.co', 'c@d.co', 'e@f.co'])))
    await waitFor(() => expect(updateStatus).toHaveBeenCalledTimes(1))
    act(() => result.current.stop())
    // The request in flight is still waited for: no Resume before it is back
    expect(result.current.state.status).toBe('stopping')
    await act(async () => release())

    await waitFor(() => expect(result.current.state.status).toBe('stopped'))
    expect(updateStatus).toHaveBeenCalledTimes(1)
    expect(result.current.state.results).toHaveLength(1)
    expect(readBulkRun('ws1', { fileName: 'u.csv', operation: 'unsubscribe', listId: 'l1', emailColumn: 'Email', emails: EMAILS })).toMatchObject({
      currentIndex: 1,
      totalEmails: 3
    })
  })

  it('resumes from the saved index, keeping the earlier results, twice in a row', async () => {
    updateStatus.mockResolvedValue({ success: true, found: true })
    const emails = ['a@b.co', 'c@d.co', 'e@f.co', 'g@h.co']
    const previous = [{ email: 'a@b.co', outcome: 'skipped' as const, detail: 'x' }]
    const { result } = renderHook(() => useBulkUpdateRun('ws1'), { wrapper })

    // First resume, stopped after one more address
    let release: () => void = () => {}
    updateStatus.mockImplementationOnce(() => new Promise((resolve) => (release = () => resolve({ success: true, found: true }))))
    act(() => void result.current.start(input(emails, { fromIndex: 1, previous })))
    await waitFor(() => expect(updateStatus).toHaveBeenCalledTimes(1))
    act(() => result.current.stop())
    await act(async () => release())
    await waitFor(() => expect(result.current.state.status).toBe('stopped'))
    const saved = readBulkRun('ws1', { fileName: 'u.csv', operation: 'unsubscribe', listId: 'l1', emailColumn: 'Email', emails: EMAILS })
    expect(saved?.currentIndex).toBe(2)
    expect(saved?.results.map((r) => r.email)).toEqual(['a@b.co', 'c@d.co'])

    // Second resume, from the saved entry
    await act(async () => result.current.start(input(emails, { fromIndex: saved!.currentIndex, previous: saved!.results })))
    expect(updateStatus.mock.calls.map((call) => call[0].email)).toEqual(['c@d.co', 'e@f.co', 'g@h.co'])
    expect(result.current.state.results.map((r) => r.outcome)).toEqual(['skipped', 'done', 'done', 'done'])
    expect(result.current.state.status).toBe('done')
  })

  it('retries only the failed addresses and replaces their results', async () => {
    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    updateStatus.mockRejectedValueOnce(new Error('timeout'))
    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    const { result } = renderHook(() => useBulkUpdateRun('ws1'), { wrapper })
    await act(async () => result.current.start(input(['a@b.co', 'c@d.co', 'e@f.co'])))
    expect(result.current.state.results[1].outcome).toBe('failed')

    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    await act(async () => result.current.retryFailed())
    expect(updateStatus).toHaveBeenCalledTimes(4)
    expect(updateStatus.mock.calls[3][0].email).toBe('c@d.co')
    expect(result.current.state.results.map((r) => r.outcome)).toEqual(['done', 'done', 'done'])
  })

  it('resumes a stopped retry as a retry of the failed addresses left', async () => {
    updateStatus.mockRejectedValueOnce(new Error('timeout'))
    updateStatus.mockRejectedValueOnce(new Error('timeout'))
    const { result } = renderHook(() => useBulkUpdateRun('ws1'), { wrapper })
    await act(async () => result.current.start(input(['a@b.co', 'c@d.co'])))
    expect(result.current.state.results.map((r) => r.outcome)).toEqual(['failed', 'failed'])

    // The retry stops after its first address
    let release: () => void = () => {}
    updateStatus.mockImplementationOnce(() => new Promise((resolve) => (release = () => resolve({ success: true, found: true }))))
    act(() => void result.current.retryFailed())
    await waitFor(() => expect(updateStatus).toHaveBeenCalledTimes(3))
    act(() => result.current.stop())
    await act(async () => release())
    await waitFor(() => expect(result.current.state.status).toBe('stopped'))

    updateStatus.mockResolvedValueOnce({ success: true, found: true })
    await act(async () => result.current.resume())
    expect(updateStatus.mock.calls.map((call) => call[0].email)).toEqual(['a@b.co', 'c@d.co', 'a@b.co', 'c@d.co'])
    expect(result.current.state.results.map((r) => r.outcome)).toEqual(['done', 'done'])
    expect(result.current.state.status).toBe('done')
  })

  it('refreshes the contacts and every list count once done', async () => {
    updateStatus.mockResolvedValue({ success: true, found: true })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const { result } = renderHook(() => useBulkUpdateRun('ws1'), { wrapper })
    await act(async () => result.current.start(input(['a@b.co'])))
    expect(invalidate.mock.calls.map((call) => (call[0] as { queryKey: unknown[] }).queryKey)).toEqual([
      ['contacts', 'ws1'],
      ['lists', 'ws1'],
      ['list-stats', 'ws1'],
      ['lists-with-counts', 'ws1'],
      ['total-contacts', 'ws1']
    ])
  })
})
