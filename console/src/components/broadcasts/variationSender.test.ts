import { describe, it, expect } from 'vitest'
import { formatSender, getMarketingSenders, resolveVariationSender } from './variationSender'
import type { Sender, Workspace } from '../../services/api/types'

const byDefault: Sender = {
  id: 's1',
  name: 'Default',
  email: 'hello@example.com',
  is_default: true
}
const newsletter: Sender = {
  id: 's2',
  name: 'Newsletter',
  email: 'news@example.com',
  is_default: false
}
const promos: Sender = { id: 's3', name: 'Promos', email: 'promos@example.com', is_default: false }
const senders = [byDefault, newsletter, promos]

const workspace = (marketingId?: string) =>
  ({
    id: 'ws1',
    settings: { marketing_email_provider_id: marketingId, transactional_email_provider_id: 'tx' },
    integrations: [
      { id: 'mkt', email_provider: { senders } },
      { id: 'tx', email_provider: { senders: [byDefault] } }
    ]
  }) as unknown as Workspace

describe('getMarketingSenders', () => {
  it("lists the marketing integration's senders, never the transactional one's", () => {
    expect(getMarketingSenders(workspace('mkt'))).toEqual(senders)
  })

  it('is empty without a marketing integration', () => {
    expect(getMarketingSenders(workspace(undefined))).toEqual([])
    expect(getMarketingSenders(undefined)).toEqual([])
  })
})

describe('resolveVariationSender', () => {
  it("uses the variation's sender over the template's", () => {
    expect(resolveVariationSender(senders, 's3', 's2')).toEqual({
      sender: promos,
      chosen: true,
      missing: false
    })
  })

  it("uses the template's sender when the variation chooses none", () => {
    expect(resolveVariationSender(senders, undefined, 's2')).toEqual({
      sender: newsletter,
      chosen: false,
      missing: false
    })
  })

  it("falls back to the default sender when the template's is not on the integration", () => {
    expect(resolveVariationSender(senders, '', 'elsewhere').sender).toEqual(byDefault)
    expect(resolveVariationSender(senders).sender).toEqual(byDefault)
  })

  it('reports a chosen sender that has left the integration, without a fallback', () => {
    expect(resolveVariationSender(senders, 'gone', 's2')).toEqual({
      sender: undefined,
      chosen: true,
      missing: true
    })
  })
})

describe('formatSender', () => {
  it('shows the name and the address', () => {
    expect(formatSender(promos)).toBe('Promos <promos@example.com>')
  })
})
