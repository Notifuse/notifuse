import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import {
  forgetImport,
  importProgressKey,
  listUnfinishedImports,
  readImport,
  saveImport,
  type SavedImport
} from './importProgressStore'

const NOW = new Date(2026, 10, 18, 12, 0).getTime()
const DAY = 24 * 60 * 60 * 1000

const entry = (overrides: Partial<SavedImport> = {}): SavedImport => ({
  fileName: 'customers.csv',
  currentRow: 1200,
  totalRows: 2481,
  currentBatch: 13,
  totalBatches: 25,
  mappings: { email: 'Email' },
  selectedListIds: ['newsletter'],
  timestamp: NOW - DAY,
  ...overrides
})

beforeEach(() => localStorage.clear())
afterEach(() => vi.restoreAllMocks())

describe('importProgressStore', () => {
  it('reads back what it saved, under the key 43.x already used', () => {
    saveImport('ws1', entry())
    expect(localStorage.getItem('csv_upload_progress_ws1_customers.csv')).not.toBeNull()
    expect(importProgressKey('ws1', 'customers.csv')).toBe('csv_upload_progress_ws1_customers.csv')
    expect(readImport('ws1', 'customers.csv', NOW)).toEqual(entry())
  })

  it('reads an entry written by 43.x', () => {
    localStorage.setItem(
      'csv_upload_progress_ws1_old.csv',
      JSON.stringify({
        fileName: 'old.csv',
        currentRow: 100,
        totalRows: 300,
        currentBatch: 2,
        totalBatches: 3,
        mappings: { email: 'email' },
        selectedListIds: [],
        timestamp: NOW - 1000
      })
    )
    expect(readImport('ws1', 'old.csv', NOW)?.currentRow).toBe(100)
  })

  it('lists only this workspace’s imports that can still be resumed, the latest first', () => {
    saveImport('ws1', entry({ fileName: 'a.csv', timestamp: NOW - 2 * DAY }))
    saveImport('ws1', entry({ fileName: 'b.csv', timestamp: NOW - DAY }))
    saveImport('ws1', entry({ fileName: 'old.csv', timestamp: NOW - 8 * DAY }))
    saveImport('ws1', entry({ fileName: 'finished.csv', currentRow: 2481 }))
    saveImport('ws1_other', entry({ fileName: 'x.csv' }))
    localStorage.setItem('csv_upload_progress_ws1_broken.csv', '{not json')
    localStorage.setItem('unrelated', 'x')

    expect(listUnfinishedImports('ws1', NOW).map((s) => s.fileName)).toEqual(['b.csv', 'a.csv'])
  })

  it('drops an entry whose row is out of bounds or whose shape is wrong', () => {
    saveImport('ws1', entry({ fileName: 'neg.csv', currentRow: -1 }))
    localStorage.setItem(importProgressKey('ws1', 'shape.csv'), JSON.stringify({ fileName: 'shape.csv', currentRow: 1 }))
    expect(readImport('ws1', 'neg.csv', NOW)).toBeNull()
    expect(readImport('ws1', 'shape.csv', NOW)).toBeNull()
  })

  it('forgets an import', () => {
    saveImport('ws1', entry())
    forgetImport('ws1', 'customers.csv')
    expect(readImport('ws1', 'customers.csv', NOW)).toBeNull()
  })

  it('answers nothing, without throwing, when the storage cannot be read', () => {
    saveImport('ws1', entry())
    const blocked = () => {
      throw new Error('blocked')
    }
    vi.spyOn(localStorage, 'getItem').mockImplementation(blocked)
    vi.spyOn(localStorage, 'key').mockImplementation(blocked)
    vi.spyOn(localStorage, 'setItem').mockImplementation(blocked)
    vi.spyOn(localStorage, 'removeItem').mockImplementation(blocked)
    expect(listUnfinishedImports('ws1', NOW)).toEqual([])
    expect(readImport('ws1', 'customers.csv', NOW)).toBeNull()
    expect(() => saveImport('ws1', entry())).not.toThrow()
    expect(() => forgetImport('ws1', 'customers.csv')).not.toThrow()
  })
})
