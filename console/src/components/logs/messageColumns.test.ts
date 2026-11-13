import { describe, it, expect, beforeEach } from 'vitest'
import { LEGACY_COLUMNS_KEY, MESSAGE_COLUMNS_KEY, loadMessageColumns } from './messageColumns'

describe('message columns', () => {
  beforeEach(() => localStorage.clear())

  it('hides the optional columns by default', () => {
    expect(loadMessageColumns()).toEqual({
      recipient: true,
      error: true,
      id: false,
      external_id: false,
      metadata: false,
      template: false,
      list: false,
      created: false
    })
  })

  it('takes the old choices once, where they differ from the old defaults', () => {
    localStorage.setItem(
      LEGACY_COLUMNS_KEY,
      JSON.stringify({
        // The old defaults, stored with the first change: not choices.
        id: true,
        template_id: true,
        list_id: true,
        created_at: true,
        // Choices.
        external_id: true,
        contact_email: false,
        status_info: false,
        events: false
      })
    )
    const columns = loadMessageColumns()
    expect(columns).toEqual({
      recipient: false,
      error: false,
      id: false,
      external_id: true,
      metadata: false,
      template: false,
      list: false,
      created: false
    })
    expect(JSON.parse(localStorage.getItem(MESSAGE_COLUMNS_KEY) ?? '{}')).toEqual(columns)
    // A later change of the old key is no longer read.
    localStorage.setItem(LEGACY_COLUMNS_KEY, JSON.stringify({ external_id: false }))
    expect(loadMessageColumns().external_id).toBe(true)
  })

  it('falls back to the defaults on unreadable storage', () => {
    localStorage.setItem(MESSAGE_COLUMNS_KEY, '{oops')
    expect(loadMessageColumns().recipient).toBe(true)
  })
})
