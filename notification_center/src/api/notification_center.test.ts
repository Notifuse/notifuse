import { describe, it, expect, vi, afterEach } from 'vitest'
import { parseNotificationCenterParams, resubscribeToList } from './notification_center'
import { api } from './client'
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

describe('resubscribeToList', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('POSTs the undo payload to /resubscribe', async () => {
    const postSpy = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ success: true, list: { id: 'vip', name: 'VIP' } })

    const payload = {
      wid: 'ws123',
      email: 'test@example.com',
      email_hmac: 'deadbeef',
      lid: 'vip',
      token: '1795800000.sig',
    }
    const result = await resubscribeToList(payload)

    expect(postSpy).toHaveBeenCalledWith('/resubscribe', payload)
    expect(result).toEqual({ success: true, list: { id: 'vip', name: 'VIP' } })
  })
})
