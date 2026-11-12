import { describe, it, expect } from 'vitest'
import { contactNavigation, exactViewTotal } from './contactNavigation'

const rows = ['a@x.com', 'b@x.com', 'c@x.com']

describe('contactNavigation', () => {
  it('moves to both neighbours from the middle of the loaded rows', () => {
    expect(contactNavigation({ emails: rows, email: 'b@x.com', hasMore: false })).toEqual({
      position: 2,
      total: 3,
      more: false,
      prevEmail: 'a@x.com',
      nextEmail: 'c@x.com',
      needsMore: false
    })
  })

  it('has no previous on the first contact', () => {
    const nav = contactNavigation({ emails: rows, email: 'a@x.com', hasMore: false })
    expect(nav.prevEmail).toBeUndefined()
    expect(nav.nextEmail).toBe('b@x.com')
  })

  it('loads more before moving on from the last loaded contact while a cursor remains', () => {
    const nav = contactNavigation({ emails: rows, email: 'c@x.com', hasMore: true })
    expect(nav.nextEmail).toBeUndefined()
    expect(nav.needsMore).toBe(true)
    expect(nav.prevEmail).toBe('b@x.com')
  })

  it('stops at the last contact without a cursor, or at the row limit', () => {
    const nav = contactNavigation({ emails: rows, email: 'c@x.com', hasMore: false })
    expect(nav.nextEmail).toBeUndefined()
    expect(nav.needsMore).toBe(false)
  })

  it('has no position for a contact outside the loaded rows', () => {
    const nav = contactNavigation({ emails: rows, email: 'z@x.com', hasMore: true, exactTotal: 9 })
    expect(nav).toEqual({ needsMore: false })
  })

  it('shows the exact total of the view when known', () => {
    const nav = contactNavigation({ emails: rows, email: 'b@x.com', hasMore: true, exactTotal: 1288 })
    expect(nav.total).toBe(1288)
    expect(nav.more).toBe(false)
  })

  it('never shows a total below the loaded rows', () => {
    const nav = contactNavigation({ emails: rows, email: 'b@x.com', hasMore: false, exactTotal: 2 })
    expect(nav.total).toBe(3)
  })

  it('counts the loaded rows, with a plus while a cursor remains, without an exact total', () => {
    const more = contactNavigation({ emails: rows, email: 'b@x.com', hasMore: true })
    expect(more.total).toBe(3)
    expect(more.more).toBe(true)
    const done = contactNavigation({ emails: rows, email: 'b@x.com', hasMore: false })
    expect(done.more).toBe(false)
  })
})

describe('exactViewTotal', () => {
  const listStats = {
    total_active: 10,
    total_pending: 2,
    total_unsubscribed: 3,
    total_bounced: 1,
    total_complained: 0
  }

  it('is the workspace count for all contacts without filter', () => {
    expect(exactViewTotal({ view: { kind: 'all' }, hasFilters: false, contactsCount: 42 })).toBe(42)
  })

  it('is the tab count of a list, or the sum of the five statuses for All', () => {
    const view = { kind: 'list' as const, listId: 'news' }
    expect(exactViewTotal({ view, hasFilters: false, listStats, status: 'pending' })).toBe(2)
    expect(exactViewTotal({ view, hasFilters: false, listStats })).toBe(16)
    expect(exactViewTotal({ view, hasFilters: false })).toBeUndefined()
  })

  it('is the users count of a segment', () => {
    const view = { kind: 'segment' as const, segmentId: 'pro' }
    expect(exactViewTotal({ view, hasFilters: false, segmentCount: 1288 })).toBe(1288)
  })

  it('is unknown as soon as a search or a filter applies', () => {
    expect(exactViewTotal({ view: { kind: 'all' }, hasFilters: true, contactsCount: 42 })).toBeUndefined()
    expect(
      exactViewTotal({ view: { kind: 'list', listId: 'news' }, hasFilters: true, listStats })
    ).toBeUndefined()
  })
})
