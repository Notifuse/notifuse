import { describe, it, expect } from 'vitest'
import { buildDefaultTestData } from './defaultTestData'

describe('buildDefaultTestData', () => {
  const data = buildDefaultTestData('https://mail.example.com', 'ws1')

  it('gives the preview a web version link on the workspace endpoint', () => {
    expect(data.view_in_browser_url).toBe('https://mail.example.com/v/preview')
  })

  it('keeps the notification center links on the workspace endpoint', () => {
    expect(data.notification_center_url).toBe(
      'https://mail.example.com/notification-center?email=john.doe@example.com&email_hmac=abc123&wid=ws1'
    )
    expect(data.unsubscribe_url).toContain('&wid=ws1&mid=preview&')
  })
})
