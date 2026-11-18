import { beforeEach, describe, it, expect } from 'vitest'
import { bulkProgressKey, forgetBulkRun, readBulkRun, saveBulkRun, type SavedBulkRun } from './bulkProgressStore'

const NOW = new Date(2026, 10, 18, 12, 0).getTime()
const EMAILS = ['a@b.co', 'c@d.co', 'e@f.co', 'g@h.co', 'i@j.co']
const match = { fileName: 'u.csv', operation: 'unsubscribe' as const, listId: 'l1', emailColumn: 'Email', emails: EMAILS }
const run = (overrides: Partial<SavedBulkRun> = {}): SavedBulkRun => ({
  fileName: 'u.csv',
  operation: 'unsubscribe',
  listId: 'l1',
  emailColumn: 'Email',
  currentIndex: 2,
  totalEmails: 5,
  results: [
    { email: 'a@b.co', outcome: 'done' },
    { email: 'c@d.co', outcome: 'skipped', detail: 'Not subscribed to this list' }
  ],
  timestamp: NOW - 1000,
  ...overrides
})

beforeEach(() => localStorage.clear())

describe('bulkProgressStore', () => {
  it('reads back what it saved, under the key 43.x already used', () => {
    saveBulkRun('ws1', run())
    expect(localStorage.getItem('bulk_update_progress_ws1_u.csv_unsubscribe')).not.toBeNull()
    expect(bulkProgressKey('ws1', 'u.csv', 'unsubscribe')).toBe('bulk_update_progress_ws1_u.csv_unsubscribe')
    expect(readBulkRun('ws1', match, NOW)).toEqual(run())
  })

  it('writes only the skipped and failed addresses, and rebuilds the others from the file', () => {
    saveBulkRun(
      'ws1',
      run({
        currentIndex: 3,
        results: [
          { email: 'a@b.co', outcome: 'done' },
          { email: 'c@d.co', outcome: 'failed', detail: 'timeout' },
          { email: 'e@f.co', outcome: 'done' }
        ]
      })
    )
    const stored = JSON.parse(localStorage.getItem(bulkProgressKey('ws1', 'u.csv', 'unsubscribe'))!)
    expect(stored.results).toBeUndefined()
    expect(stored.exceptions).toEqual([[1, 'failed', 'timeout']])
    expect(readBulkRun('ws1', match, NOW)?.results).toEqual([
      { email: 'a@b.co', outcome: 'done' },
      { email: 'c@d.co', outcome: 'failed', detail: 'timeout' },
      { email: 'e@f.co', outcome: 'done' }
    ])
  })

  it('reads an entry of 43.x, a success with an error being a skip', () => {
    localStorage.setItem(
      bulkProgressKey('ws1', 'u.csv', 'unsubscribe'),
      JSON.stringify({
        fileName: 'u.csv',
        operation: 'unsubscribe',
        listId: 'l1',
        currentIndex: 3,
        totalEmails: 5,
        results: [
          { email: 'a@b.co', success: true },
          { email: 'c@d.co', success: true, error: 'Not subscribed (skipped)' },
          { email: 'e@f.co', success: false, error: 'boom' }
        ],
        timestamp: NOW - 1000
      })
    )
    expect(readBulkRun('ws1', match, NOW)?.results).toEqual([
      { email: 'a@b.co', outcome: 'done' },
      { email: 'c@d.co', outcome: 'skipped', detail: 'Not subscribed (skipped)' },
      { email: 'e@f.co', outcome: 'failed', detail: 'boom' }
    ])
  })

  it('offers the run only for the same action, list and email column', () => {
    saveBulkRun('ws1', run())
    expect(readBulkRun('ws1', { ...match, listId: 'l2' }, NOW)).toBeNull()
    expect(readBulkRun('ws1', { ...match, emailColumn: 'Work email' }, NOW)).toBeNull()
    expect(readBulkRun('ws1', { ...match, operation: 'delete' }, NOW)).toBeNull()
  })

  it('drops a run older than 7 days, or one with nothing left or nothing done', () => {
    saveBulkRun('ws1', run({ timestamp: NOW - 8 * 24 * 60 * 60 * 1000 }))
    expect(readBulkRun('ws1', match, NOW)).toBeNull()
    saveBulkRun('ws1', run({ currentIndex: 5 }))
    expect(readBulkRun('ws1', match, NOW)).toBeNull()
    saveBulkRun('ws1', run({ currentIndex: 0 }))
    expect(readBulkRun('ws1', match, NOW)).toBeNull()
  })

  it('forgets a run', () => {
    saveBulkRun('ws1', run())
    forgetBulkRun('ws1', 'u.csv', 'unsubscribe')
    expect(readBulkRun('ws1', match, NOW)).toBeNull()
  })
})
