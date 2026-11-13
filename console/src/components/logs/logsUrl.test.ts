import { describe, it, expect } from 'vitest'
import { LOGS_TAB } from './logsUrl'
import { LOGS_TABS } from './logsSearch'

describe('LOGS_TAB', () => {
  it('names the tabs with the keys the Logs page uses', () => {
    expect(LOGS_TAB).toEqual({
      messages: 'messages',
      incomingWebhooks: 'incoming-webhooks',
      outgoingWebhooks: 'outgoing-webhooks',
      audit: 'audit'
    })
    expect(Object.values(LOGS_TAB)).toEqual([...LOGS_TABS])
  })
})
