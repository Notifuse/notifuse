import { describe, it, expect } from 'vitest'
import { contactsListParams, hasRowFilters, toTableSearch, viewOf } from './contactsView'

describe('contactsView', () => {
  it('reads the view from the URL, a list first', () => {
    expect(viewOf({})).toEqual({ kind: 'all' })
    expect(viewOf({ list_id: 'news' })).toEqual({ kind: 'list', listId: 'news' })
    expect(viewOf({ segment_id: 'pro' })).toEqual({ kind: 'segment', segmentId: 'pro' })
    expect(viewOf({ list_id: 'news', segment_id: 'pro' })).toEqual({ kind: 'list', listId: 'news' })
  })

  it('leaves the open contact out of the table search', () => {
    expect(toTableSearch({ list_id: 'news', contact: 'ann@x.com', limit: 50 })).toEqual({
      list_id: 'news',
      limit: 50
    })
  })

  it('counts the search, the chips and the legacy segments as row filters, not the view', () => {
    expect(
      hasRowFilters({ list_id: 'news', contact_list_status: 'active', segment_id: undefined, limit: 50 })
    ).toBe(false)
    expect(hasRowFilters({ segment_id: 'pro' })).toBe(false)
    expect(hasRowFilters({ search: 'ann' })).toBe(true)
    expect(hasRowFilters({ country: 'FR' })).toBe(true)
    expect(hasRowFilters({ segments: ['a', 'b'] })).toBe(true)
    expect(hasRowFilters({ search: '' })).toBe(false)
  })

  it('sends a segment view as segments, with the search and the lists of each contact', () => {
    expect(contactsListParams('ws', { segment_id: 'pro', search: 'ann' })).toMatchObject({
      workspace_id: 'ws',
      segments: ['pro'],
      search: 'ann',
      with_contact_lists: true
    })
  })

  it('sends the list status only with a list', () => {
    expect(contactsListParams('ws', { contact_list_status: 'active' }).contact_list_status).toBeUndefined()
    expect(
      contactsListParams('ws', { list_id: 'news', contact_list_status: 'active' }).contact_list_status
    ).toBe('active')
  })
})
