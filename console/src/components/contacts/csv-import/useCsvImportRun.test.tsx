import { beforeEach, describe, it, expect, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { contactsApi, type BatchImportContactsResponse } from '../../../services/api/contacts'
import { readImport } from './importProgressStore'
import { indexInError, useCsvImportRun, withoutIndex, type ImportRunInput } from './useCsvImportRun'

vi.mock('../../../services/api/contacts', () => ({ contactsApi: { batchImport: vi.fn() } }))

const batchImport = vi.mocked(contactsApi.batchImport)

const input = (count: number, overrides: Partial<ImportRunInput> = {}): ImportRunInput => ({
  fileName: 'c.csv',
  headers: ['email'],
  rows: Array.from({ length: count }, (_, i) => [`c${i}@example.com`]),
  mappings: { email: 'email' },
  listIds: [],
  fromRow: 0,
  ...overrides
})

const created = (contacts: Array<{ email?: string }>): BatchImportContactsResponse =>
  ({ operations: contacts.map((c) => ({ email: c.email, action: 'create' })) }) as BatchImportContactsResponse

/** batchImport answers only when the test releases it, one call at a time. */
function heldBatches() {
  const releases: Array<() => void> = []
  batchImport.mockImplementation(
    ({ contacts }) => new Promise((resolve) => releases.push(() => resolve(created(contacts))))
  )
  return {
    releaseNext: async () => {
      await waitFor(() => expect(releases.length).toBeGreaterThan(0))
      await act(async () => releases.shift()!())
    }
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
})

describe('useCsvImportRun', () => {
  it('sends no further batch once stopped, and still counts the batch in flight', async () => {
    const held = heldBatches()
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    act(() => void result.current.start(input(250)))
    await waitFor(() => expect(batchImport).toHaveBeenCalledTimes(1))

    act(() => result.current.stop())
    // The batch in flight is still waited for: no Resume before it is back
    expect(result.current.state.status).toBe('stopping')
    await held.releaseNext()

    await waitFor(() => expect(result.current.state.created).toBe(100))
    expect(result.current.state.status).toBe('stopped')
    expect(batchImport).toHaveBeenCalledTimes(1)
    expect(readImport('ws1', 'c.csv')?.currentRow).toBe(100)
  })

  it('stops sending when unmounted', async () => {
    const held = heldBatches()
    const { result, unmount } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    act(() => void result.current.start(input(250)))
    await waitFor(() => expect(batchImport).toHaveBeenCalledTimes(1))
    unmount()
    await held.releaseNext()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(batchImport).toHaveBeenCalledTimes(1)
  })

  it('saves the row reached after each batch', async () => {
    const held = heldBatches()
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    act(() => void result.current.start(input(350, { listIds: ['l1'] })))
    await held.releaseNext()
    await held.releaseNext()
    await waitFor(() => expect(readImport('ws1', 'c.csv')?.currentRow).toBe(200))
    expect(readImport('ws1', 'c.csv')).toMatchObject({ totalRows: 350, selectedListIds: ['l1'], mappings: { email: 'email' } })
    act(() => result.current.stop())
  })

  it('runs again after a stop, from the row it is given', async () => {
    batchImport.mockImplementation(async ({ contacts }) => created(contacts))
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    act(() => result.current.stop())
    await act(async () => result.current.start(input(250, { fromRow: 200 })))
    expect(batchImport).toHaveBeenCalledTimes(1)
    expect(batchImport.mock.calls[0][0].contacts[0].email).toBe('c200@example.com')
    expect(result.current.state).toMatchObject({ status: 'done', row: 250, created: 50 })
  })

  it('counts created, updated and failed apart, each failure on its own line of the file', async () => {
    // As the server answers: validation errors first, naming their contact by index, then the upserts.
    batchImport.mockResolvedValue({
      operations: [
        { email: 'c@x', action: 'error', error: 'invalid contact at index 2: invalid email' },
        { email: 'b@x.co', action: 'update' },
        { email: 'a@x.co', action: 'create' }
      ]
    } as BatchImportContactsResponse)
    const rows = [['a@x.co'], [''], ['b@x.co'], ['c@x']]
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    await act(async () => result.current.start(input(0, { rows })))

    // The row without an email is not sent: index 2 of the batch is the file's fourth row.
    expect(batchImport.mock.calls[0][0].contacts.map((c) => c.email)).toEqual(['a@x.co', 'b@x.co', 'c@x'])
    expect(result.current.state).toMatchObject({ created: 1, updated: 1, failed: 2 })
    expect(result.current.state.failures).toEqual([
      { rowIndex: 1, line: 3, email: '', error: 'no email address' },
      { rowIndex: 3, line: 5, email: 'c@x', error: 'invalid email' }
    ])
  })

  it('counts the rows the server merged into a later row of the same email', async () => {
    batchImport.mockResolvedValue({
      operations: [
        { email: 'a@x.co', action: 'create' },
        { email: 'b@x.co', action: 'create' }
      ]
    } as BatchImportContactsResponse)
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    await act(async () => result.current.start(input(0, { rows: [['a@x.co'], ['b@x.co'], ['a@x.co']] })))
    expect(result.current.state).toMatchObject({ created: 2, merged: 1, failed: 0, row: 3 })
  })

  it('reports stopped when stopped while paused, with nothing in flight', async () => {
    const held = heldBatches()
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    act(() => void result.current.start(input(250)))
    act(() => result.current.pause())
    await held.releaseNext()
    await waitFor(() => expect(result.current.state.status).toBe('paused'))
    act(() => result.current.stop())
    await waitFor(() => expect(result.current.state.status).toBe('stopped'))
    expect(batchImport).toHaveBeenCalledTimes(1)
  })

  it('finds the row of an error without an index by its email', async () => {
    batchImport.mockResolvedValue({
      operations: [
        { email: 'a@x.co', action: 'create' },
        { email: 'b@x.co', action: 'error', error: 'failed' }
      ]
    } as BatchImportContactsResponse)
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    await act(async () => result.current.start(input(0, { rows: [['a@x.co'], ['b@x.co']] })))
    expect(result.current.state.failures).toEqual([{ rowIndex: 1, line: 3, email: 'b@x.co', error: 'failed' }])
  })

  it('reads the contact index of a contacts.import error', () => {
    expect(indexInError('invalid contact at index 17: email is required')).toBe(17)
    expect(indexInError('failed to upsert contact at index 3: timeout')).toBe(3)
    expect(indexInError('failed')).toBeUndefined()
    expect(indexInError(undefined)).toBeUndefined()
  })

  it('shows an error without the index, which is not a line of the file', () => {
    expect(withoutIndex('invalid contact at index 17: email is required')).toBe('email is required')
    expect(withoutIndex('failed')).toBe('failed')
  })

  it('fails the whole batch on a batch-level error or a failed request', async () => {
    batchImport.mockResolvedValueOnce({ operations: [], error: 'quota exceeded' } as BatchImportContactsResponse)
    batchImport.mockRejectedValueOnce(new Error('network error'))
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    await act(async () => result.current.start(input(150)))
    expect(result.current.state.failed).toBe(150)
    expect(result.current.state.failures[0].error).toBe('quota exceeded')
    expect(result.current.state.failures[149].error).toBe('network error')
  })

  it('holds the next batch while paused', async () => {
    const held = heldBatches()
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1' }))
    act(() => void result.current.start(input(250)))
    act(() => result.current.pause())
    await held.releaseNext()
    await new Promise((resolve) => setTimeout(resolve, 250))
    expect(batchImport).toHaveBeenCalledTimes(1)
    expect(result.current.state.status).toBe('paused')
    act(() => result.current.resume())
    await waitFor(() => expect(batchImport).toHaveBeenCalledTimes(2))
    act(() => result.current.stop())
  })

  it('forgets the saved row and reports the import once it is done', async () => {
    batchImport.mockImplementation(async ({ contacts }) => created(contacts))
    const onImported = vi.fn()
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1', onImported }))
    await act(async () => result.current.start(input(150)))
    expect(result.current.state.status).toBe('done')
    expect(readImport('ws1', 'c.csv')).toBeNull()
    expect(onImported).toHaveBeenCalledTimes(1)
  })

  it('reports the import after a stop that imported rows', async () => {
    const held = heldBatches()
    const onImported = vi.fn()
    const { result } = renderHook(() => useCsvImportRun({ workspaceId: 'ws1', onImported }))
    act(() => void result.current.start(input(250)))
    await waitFor(() => expect(batchImport).toHaveBeenCalledTimes(1))
    act(() => result.current.stop())
    await held.releaseNext()
    await waitFor(() => expect(onImported).toHaveBeenCalledTimes(1))
  })
})
