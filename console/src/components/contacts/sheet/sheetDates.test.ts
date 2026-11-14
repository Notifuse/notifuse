import { describe, it, expect } from 'vitest'
import { sheetTimezone } from './sheetDates'

describe('sheetTimezone', () => {
  it("keeps the contact's timezone when the browser knows it", () => {
    expect(sheetTimezone('Asia/Tokyo', 'Europe/Paris')).toBe('Asia/Tokyo')
  })

  it("falls back to the workspace's for a missing or unknown one", () => {
    expect(sheetTimezone(undefined, 'Europe/Paris')).toBe('Europe/Paris')
    expect(sheetTimezone('', 'Europe/Paris')).toBe('Europe/Paris')
    // The server does not check a contact's timezone: an import can store « Paris ».
    expect(sheetTimezone('Paris', 'Europe/Paris')).toBe('Europe/Paris')
  })
})
