import { describe, expect, it } from 'vitest'
import { parseLogsSearch } from './logsSearch'

describe('parseLogsSearch', () => {
  it('accepts an empty URL', () => {
    expect(parseLogsSearch({})).toEqual({})
  })

  it('keeps a boolean flag from a link', () => {
    expect(parseLogsSearch({ is_failed: true, is_opened: false })).toEqual({
      is_failed: true,
      is_opened: false
    })
  })

  it('reads the text "true" and "false" as booleans', () => {
    // A page that wrote the string 'true' got is_failed=%22true%22, which the router parses to "true".
    expect(parseLogsSearch({ is_failed: 'true', is_bounced: 'false' })).toEqual({
      is_failed: true,
      is_bounced: false
    })
  })

  it('drops a flag of any other value', () => {
    expect(parseLogsSearch({ is_failed: 'yes', is_clicked: 1, is_delivered: '' })).toEqual({})
  })

  it('drops the old Sent flag, which filtered nothing', () => {
    expect(parseLogsSearch({ is_sent: true })).toEqual({})
  })

  it('reads a numeric identifier back as text', () => {
    // The router's parser turns ?template_id=2024 into the number 2024.
    expect(parseLogsSearch({ template_id: 2024, transactional_notification_id: 7 })).toEqual({
      template_id: '2024',
      transactional_notification_id: '7'
    })
  })

  it('takes the first element of an array and trims text', () => {
    expect(parseLogsSearch({ broadcast_id: [' b1 ', 'b2'], contact_email: ' a@b.co ' })).toEqual({
      broadcast_id: 'b1',
      contact_email: 'a@b.co'
    })
  })

  it('drops empty and whitespace-only values', () => {
    expect(parseLogsSearch({ id: '', target_id: '   ', list_id: [] })).toEqual({})
  })

  it('keeps known closed values', () => {
    expect(
      parseLogsSearch({
        tab: 'incoming-webhooks',
        source: 'postmark',
        event_type: 'reply',
        period: '7d'
      })
    ).toEqual({ tab: 'incoming-webhooks', source: 'postmark', event_type: 'reply', period: '7d' })
    expect(
      parseLogsSearch({ tab: 'messages', origin: 'automation', channel: 'email' })
    ).toEqual({ tab: 'messages', origin: 'automation', channel: 'email' })
    expect(
      parseLogsSearch({ tab: 'outgoing-webhooks', status: 'failed', subscription_id: 'sub1' })
    ).toEqual({ tab: 'outgoing-webhooks', status: 'failed', subscription_id: 'sub1' })
    expect(
      parseLogsSearch({
        tab: 'audit',
        outcome: 'denied',
        scope: 'deployment',
        actor_type: 'api_key',
        period: 'custom',
        from: '2026-11-01T00:00:00Z',
        to: '2026-11-02T00:00:00Z'
      })
    ).toEqual({
      tab: 'audit',
      outcome: 'denied',
      scope: 'deployment',
      actor_type: 'api_key',
      period: 'custom',
      from: '2026-11-01T00:00:00Z',
      to: '2026-11-02T00:00:00Z'
    })
  })

  it('drops closed values outside their list', () => {
    expect(
      parseLogsSearch({
        tab: 'settings',
        origin: 'campaign',
        channel: 'fax',
        source: 'mandrill',
        event_type: 'open',
        status: 'retrying',
        outcome: 'ok',
        scope: 'everything',
        actor_type: 'robot'
      })
    ).toEqual({})
  })

  it('checks the period against the tab of the URL', () => {
    expect(parseLogsSearch({ tab: 'messages', period: '90d' })).toEqual({ tab: 'messages' })
    expect(parseLogsSearch({ tab: 'messages', period: 'custom' })).toEqual({ tab: 'messages' })
    expect(parseLogsSearch({ tab: 'audit', period: '1h' })).toEqual({ tab: 'audit' })
    expect(parseLogsSearch({ tab: 'audit', period: '90d' })).toEqual({ tab: 'audit', period: '90d' })
    // Without a tab the page opens the messages, so their periods apply.
    expect(parseLogsSearch({ period: '1h' })).toEqual({ period: '1h' })
    expect(parseLogsSearch({ period: '90d' })).toEqual({})
  })

  it('drops unknown keys', () => {
    expect(parseLogsSearch({ page: 2, foo: 'bar', transactional_id: 't1' })).toEqual({})
  })

  it('keeps the audit lists as written', () => {
    expect(
      parseLogsSearch({ tab: 'audit', categories: 'templates,blog', actions: 'templates.create' })
    ).toEqual({ tab: 'audit', categories: 'templates,blog', actions: 'templates.create' })
  })
})
