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

describe('transactionalProvider', () => {
  it('is the integration the settings name', () => {
    expect(transactionalProvider(workspace('postmark'))?.name).toBe('Postmark')
    expect(transactionalProvider(workspace(''))).toBeNull()
  })
})
