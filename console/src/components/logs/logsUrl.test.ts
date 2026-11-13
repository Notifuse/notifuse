import { describe, it, expect } from 'vitest'
import { buildLogsTabUrl, LOGS_TAB } from './logsUrl'

describe('buildLogsTabUrl', () => {
  const pathname = '/console/workspace/ws1/logs'

  it('keeps the tab with the filters, so a copied link reopens the same tab', () => {
    expect(
      buildLogsTabUrl(pathname, LOGS_TAB.incomingWebhooks, [
        { field: 'event_type', value: 'bounce' }
      ])
    ).toBe(`${pathname}?tab=incoming-webhooks&event_type=bounce`)
  })

  it('keeps the tab when no filter is set', () => {
    expect(buildLogsTabUrl(pathname, LOGS_TAB.outgoingWebhooks, [])).toBe(
      `${pathname}?tab=outgoing-webhooks`
    )
  })

  it('writes only the tab and the given filters', () => {
    const url = buildLogsTabUrl(pathname, LOGS_TAB.messages, [
      { field: 'is_failed', value: 'true' },
      { field: 'contact_email', value: 'a+b@example.com' }
    ])
    const params = new URL(url, 'http://localhost').searchParams
    expect([...params.keys()]).toEqual(['tab', 'is_failed', 'contact_email'])
    expect(params.get('tab')).toBe('messages')
    expect(params.get('contact_email')).toBe('a+b@example.com')
  })

  it('names the tabs with the keys the Logs page uses', () => {
    expect(LOGS_TAB).toEqual({
      messages: 'messages',
      incomingWebhooks: 'incoming-webhooks',
      outgoingWebhooks: 'outgoing-webhooks',
      audit: 'audit'
    })
  })
})
