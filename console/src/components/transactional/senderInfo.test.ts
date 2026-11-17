import { describe, expect, it } from 'vitest'
import { resolveSenderInfo, transactionalProvider } from './senderInfo'
import type { Workspace } from '../../services/api/workspace'
import type { Template } from '../../services/api/template'

const senders = [
  { id: 's1', email: 'hello@acme.test', name: 'Acme', is_default: false },
  { id: 's2', email: 'noreply@acme.test', name: 'Acme', is_default: true },
  { id: 's3', email: 'billing@acme.test', name: 'Billing', is_default: false }
]

const workspace = (providerId?: string, providerSenders = senders) =>
  ({
    settings: { transactional_email_provider_id: providerId },
    integrations: [
      {
        id: 'postmark',
        name: 'Postmark',
        type: 'email',
        email_provider: { kind: 'postmark', senders: providerSenders }
      }
    ]
  }) as unknown as Workspace

const template = (email: Record<string, string>) => ({ email: { subject: 'Hi', ...email } }) as unknown as Template

describe('resolveSenderInfo', () => {
  it("uses the template's sender when the provider has it", () => {
    const info = resolveSenderInfo(workspace('postmark'), template({ sender_id: 's3', reply_to: 'help@acme.test' }))
    expect(info.sender?.email).toBe('billing@acme.test')
    expect(info.replyTo).toBe('help@acme.test')
    expect(info.providerDefaultOnly).toBe(false)
  })

  it("falls back to the provider's default sender", () => {
    expect(resolveSenderInfo(workspace('postmark'), template({ sender_id: 'gone' })).sender?.email).toBe(
      'noreply@acme.test'
    )
    expect(resolveSenderInfo(workspace('postmark'), template({})).sender?.email).toBe('noreply@acme.test')
  })

  it('finds no sender when none is the default, as the server', () => {
    const plain = senders.map((sender) => ({ ...sender, is_default: false }))
    expect(resolveSenderInfo(workspace('postmark', plain), template({})).sender).toBeNull()
    // The template's own sender still counts.
    expect(resolveSenderInfo(workspace('postmark', plain), template({ sender_id: 's3' })).sender?.email).toBe(
      'billing@acme.test'
    )
  })

  it('finds no sender without a provider, or a provider gone or without senders', () => {
    expect(resolveSenderInfo(workspace(undefined), template({ sender_id: 's1' })).sender).toBeNull()
    expect(resolveSenderInfo(workspace('removed'), template({})).provider).toBeNull()
    expect(resolveSenderInfo(workspace('postmark', []), template({})).sender).toBeNull()
  })

  it("shows the provider's default, flagged, when the template could not be read", () => {
    const info = resolveSenderInfo(workspace('postmark'), undefined)
    expect(info.sender?.email).toBe('noreply@acme.test')
    expect(info.providerDefaultOnly).toBe(true)
    expect(info.replyTo).toBeNull()
  })

  it('reads an empty Reply-To as none', () => {
    expect(resolveSenderInfo(workspace('postmark'), template({ reply_to: ' ' })).replyTo).toBeNull()
  })
})

describe('resolveSenderInfo for a given send path', () => {
  const two = {
    settings: { transactional_email_provider_id: 'postmark', marketing_email_provider_id: 'ses' },
    integrations: [
      { id: 'postmark', name: 'Postmark', type: 'email', email_provider: { kind: 'postmark', senders } },
      {
        id: 'ses',
        name: 'SES',
        type: 'email',
        email_provider: { kind: 'ses', senders: [{ id: 'm1', email: 'news@acme.test', name: 'News', is_default: true }] }
      }
    ]
  } as unknown as Workspace

  it('uses the provider of the send path, and looks for the template sender in it alone', () => {
    // s3 belongs to the transactional provider: a marketing send falls back to its own default.
    expect(resolveSenderInfo(two, template({ sender_id: 's3' }), 'marketing').sender?.email).toBe('news@acme.test')
    expect(resolveSenderInfo(two, template({ sender_id: 's3' }), 'transactional').sender?.email).toBe('billing@acme.test')
    expect(resolveSenderInfo(two, template({ sender_id: 's3' })).sender?.email).toBe('billing@acme.test')
    expect(resolveSenderInfo(two, template({}), { integrationId: 'ses' }).provider?.name).toBe('SES')
    expect(resolveSenderInfo(two, template({}), { integrationId: 'gone' }).sender).toBeNull()
  })

  it("puts a sender set by the send path (a broadcast variant's) before the template's", () => {
    // As EmailProvider.ResolveSender(variationSenderID, templateSenderID).
    const info = resolveSenderInfo(two, template({ sender_id: 's3' }), 'transactional', 's1')
    expect(info.sender?.email).toBe('hello@acme.test')
    // One the provider no longer has gives no sender: the send fails (ErrSenderNotFound), it does
    // not fall back to the template's sender nor to the provider's default.
    expect(resolveSenderInfo(two, template({ sender_id: 's3' }), 'transactional', 'gone').sender).toBeNull()
  })
})

describe('transactionalProvider', () => {
  it('is the integration the settings name', () => {
    expect(transactionalProvider(workspace('postmark'))?.name).toBe('Postmark')
    expect(transactionalProvider(workspace(''))).toBeNull()
  })
})
