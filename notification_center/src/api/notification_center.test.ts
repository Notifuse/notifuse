import { describe, it, expect } from 'vitest'
import { parseNotificationCenterParams } from './notification_center'
import { mockURLSearchParams } from '../test/utils'

describe('parseNotificationCenterParams', () => {
  // The App tests mock this parser, so this is what pins the parameter names the server
  // redirects a GET on the List-Unsubscribe header URL with (redirectOneClickToConfirmation
  // in internal/http/public_handler.go asserts the same names on its side).
  it('reads the confirm_unsubscribe redirect of the List-Unsubscribe header link', () => {
    mockURLSearchParams({
      action: 'confirm_unsubscribe',
      email: 'test@example.com',
      email_hmac: 'deadbeef',
      lid: 'list1',
      mid: 'msg-1',
      wid: 'ws123',
    })

    expect(parseNotificationCenterParams()).toEqual({
      action: 'confirm_unsubscribe',
      email: 'test@example.com',
      email_hmac: 'deadbeef',
      lid: 'list1',
      mid: 'msg-1',
      wid: 'ws123',
    })
  })

  it('rejects a link without the email_hmac that authorizes it', () => {
    mockURLSearchParams({ action: 'confirm_unsubscribe', email: 'test@example.com', wid: 'ws123', lid: 'list1' })

    expect(parseNotificationCenterParams()).toBeNull()
  })
})
