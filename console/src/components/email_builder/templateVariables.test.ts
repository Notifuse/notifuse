import { describe, it, expect } from 'vitest'
import { COMMON_TEMPLATE_VARIABLES } from './templateVariables'

describe('COMMON_TEMPLATE_VARIABLES', () => {
  it('suggests the web version link', () => {
    expect(COMMON_TEMPLATE_VARIABLES).toContain('view_in_browser_url')
  })

  it('keeps suggesting the links a send fills in', () => {
    for (const name of ['unsubscribe_url', 'notification_center_url', 'tracking_optout_url', 'contact.email']) {
      expect(COMMON_TEMPLATE_VARIABLES).toContain(name)
    }
  })

  it('lists each variable once', () => {
    expect(new Set(COMMON_TEMPLATE_VARIABLES).size).toBe(COMMON_TEMPLATE_VARIABLES.length)
  })
})
