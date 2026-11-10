import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { transactionalNotificationsApi } from './transactional_notifications'

vi.mock('./client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

const post = api.post as unknown as ReturnType<typeof vi.fn>

describe('transactionalNotificationsApi.testTemplate', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ success: true })
  })

  it('sends the list and the header choice when a list is given', async () => {
    await transactionalNotificationsApi.testTemplate(
      'ws1',
      'tpl1',
      'int1',
      'sender1',
      'to@example.com',
      undefined,
      undefined,
      { listId: 'news', omitListUnsubscribeHeader: true }
    )

    expect(post).toHaveBeenCalledWith('/api/transactional.testTemplate', {
      workspace_id: 'ws1',
      template_id: 'tpl1',
      integration_id: 'int1',
      sender_id: 'sender1',
      recipient_email: 'to@example.com',
      language: undefined,
      email_options: undefined,
      list_id: 'news',
      omit_list_unsubscribe_header: true
    })
  })

  it('leaves the list fields out of the body without a list', async () => {
    await transactionalNotificationsApi.testTemplate('ws1', 'tpl1', 'int1', 'sender1', 'to@example.com')

    const body = post.mock.calls[0][1] as Record<string, unknown>
    expect(body).not.toHaveProperty('list_id')
    expect(body).not.toHaveProperty('omit_list_unsubscribe_header')
  })
})
