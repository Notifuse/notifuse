import { describe, it, expect } from 'vitest'
import type { Workspace } from '../../services/api/workspace'
import { countsDeliveries, messageOrigin, rowIsSmtp } from './messageProvider'

const workspace = (marketing: 'smtp' | 'postmark', transactional: 'smtp' | 'postmark') =>
  ({
    id: 'ws1',
    settings: { marketing_email_provider_id: 'm', transactional_email_provider_id: 't' },
    integrations: [
      { id: 'm', email_provider: { kind: marketing } },
      { id: 't', email_provider: { kind: transactional } }
    ]
  }) as unknown as Workspace

describe('messageOrigin', () => {
  it('reads the newsletter, then the automation, then the notification', () => {
    expect(messageOrigin({ broadcast_id: 'b', automation_id: 'a' })).toBe('broadcast')
    expect(messageOrigin({ automation_id: 'a', transactional_notification_id: 'n' })).toBe(
      'automation'
    )
    expect(messageOrigin({ transactional_notification_id: 'n' })).toBe('transactional')
    expect(messageOrigin({})).toBe('other')
  })
})

describe('rowIsSmtp', () => {
  it('uses the marketing provider for a newsletter and an automation', () => {
    const ws = workspace('smtp', 'postmark')
    expect(rowIsSmtp({ broadcast_id: 'b' }, ws)).toBe(true)
    expect(rowIsSmtp({ automation_id: 'a' }, ws)).toBe(true)
    expect(rowIsSmtp({ transactional_notification_id: 'n' }, ws)).toBe(false)
  })

  it('uses the transactional provider for a notification', () => {
    const ws = workspace('postmark', 'smtp')
    expect(rowIsSmtp({ transactional_notification_id: 'n' }, ws)).toBe(true)
    expect(rowIsSmtp({ broadcast_id: 'b' }, ws)).toBe(false)
  })

  it('takes either provider for a message of no origin', () => {
    expect(rowIsSmtp({}, workspace('smtp', 'postmark'))).toBe(true)
    expect(rowIsSmtp({}, workspace('postmark', 'smtp'))).toBe(true)
    expect(rowIsSmtp({}, workspace('postmark', 'postmark'))).toBe(false)
  })

  it('is false without a workspace or a provider', () => {
    expect(rowIsSmtp({ broadcast_id: 'b' }, undefined)).toBe(false)
    expect(
      rowIsSmtp({ broadcast_id: 'b' }, { id: 'ws1', settings: {} } as unknown as Workspace)
    ).toBe(false)
  })
})

describe('countsDeliveries', () => {
  it('is false when both providers are SMTP', () => {
    expect(countsDeliveries(workspace('smtp', 'smtp'), undefined)).toBe(false)
  })

  it('is true when one provider reports deliveries and no origin is filtered', () => {
    expect(countsDeliveries(workspace('smtp', 'postmark'), undefined)).toBe(true)
  })

  it('is false with an SMTP marketing provider and no transactional one', () => {
    const ws = {
      id: 'ws1',
      settings: { marketing_email_provider_id: 'm' },
      integrations: [{ id: 'm', email_provider: { kind: 'smtp' } }]
    } as unknown as Workspace
    expect(countsDeliveries(ws, undefined)).toBe(false)
  })

  it('follows the provider of the filtered origin', () => {
    const ws = workspace('smtp', 'postmark')
    expect(countsDeliveries(ws, 'broadcast')).toBe(false)
    expect(countsDeliveries(ws, 'automation')).toBe(false)
    expect(countsDeliveries(ws, 'transactional')).toBe(true)
    expect(countsDeliveries(ws, 'other')).toBe(true)
  })
})
