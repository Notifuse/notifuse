import { describe, it, expect } from 'vitest'
import { validateDraft, feedUrlIsValid } from './validateDraft'
import { newDraft, type ComposeValues } from './composeValues'
import type { Workspace } from '../../../services/api/types'

const ctx = { senderIds: ['s1'] }
const base = (): ComposeValues => ({
  ...newDraft('News', { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace),
  list: 'news',
  variants: [{ variation_name: 'A', template_id: 't1' }]
})
const ab = (over: Partial<ComposeValues> = {}): ComposeValues => ({
  ...base(),
  abEnabled: true,
  variants: [
    { variation_name: 'A', template_id: 't1' },
    { variation_name: 'B', template_id: 't2' }
  ],
  ...over
})

describe('validateDraft', () => {
  it('accepts a complete draft', () => {
    expect(validateDraft(base(), ctx)).toBeNull()
    expect(validateDraft(ab(), ctx)).toBeNull()
  })

  it('accepts a single variant without a template yet', () => {
    expect(validateDraft({ ...base(), variants: [{ variation_name: 'A', template_id: '' }] }, ctx)).toBeNull()
  })

  it('wants a name of at most 255 bytes', () => {
    expect(validateDraft({ ...base(), name: '  ' }, ctx)).toEqual({ code: 'name_missing' })
    expect(validateDraft({ ...base(), name: 'a'.repeat(255) }, ctx)).toBeNull()
    // 128 two-byte characters: 128 characters but 256 bytes, which the server refuses.
    expect(validateDraft({ ...base(), name: 'é'.repeat(128) }, ctx)).toEqual({ code: 'name_too_long' })
  })

  it('wants a list', () => {
    expect(validateDraft({ ...base(), list: undefined }, ctx)).toEqual({ code: 'list_missing' })
  })

  it('wants 2 to 5 variants with a template each, used once', () => {
    expect(validateDraft(ab({ variants: [{ variation_name: 'A', template_id: 't1' }] }), ctx)).toEqual({
      code: 'ab_too_few'
    })
    const six = ['A', 'B', 'C', 'D', 'E', 'F'].map((l, i) => ({ variation_name: l, template_id: `t${i}` }))
    expect(validateDraft(ab({ variants: six }), ctx)).toEqual({ code: 'ab_too_many' })
    expect(
      validateDraft(
        ab({
          variants: [
            { variation_name: 'A', template_id: 't1' },
            { variation_name: 'B', template_id: '' }
          ]
        }),
        ctx
      )
    ).toEqual({ code: 'variant_no_template', variant: 'B' })
    expect(
      validateDraft(
        ab({
          variants: [
            { variation_name: 'A', template_id: 't1' },
            { variation_name: 'B', template_id: 't1' }
          ]
        }),
        ctx
      )
    ).toEqual({ code: 'template_reused', variant: 'B' })
  })

  it('refuses a sender that has left the integration', () => {
    expect(
      validateDraft({ ...base(), variants: [{ variation_name: 'A', template_id: 't1', sender_id: 'gone' }] }, ctx)
    ).toEqual({ code: 'sender_gone', variant: 'A' })
    expect(
      validateDraft({ ...base(), variants: [{ variation_name: 'A', template_id: 't1', sender_id: 's1' }] }, ctx)
    ).toBeNull()
  })

  it('wants a sample from 1 to 100 %', () => {
    expect(validateDraft(ab({ samplePercentage: 0 }), ctx)).toEqual({ code: 'sample_out_of_range' })
    expect(validateDraft(ab({ samplePercentage: 101 }), ctx)).toEqual({ code: 'sample_out_of_range' })
  })

  it('wants a duration of 1 to 168 hours and a metric for an automatic winner, with tracking', () => {
    const auto = { autoSendWinner: true, winnerMetric: 'open_rate' as const, testDurationHours: 24 }
    expect(validateDraft(ab(auto), ctx)).toBeNull()
    expect(validateDraft(ab({ ...auto, testDurationHours: 0 }), ctx)).toEqual({ code: 'duration_out_of_range' })
    expect(validateDraft(ab({ ...auto, testDurationHours: 169 }), ctx)).toEqual({ code: 'duration_out_of_range' })
    expect(validateDraft(ab({ ...auto, winnerMetric: undefined }), ctx)).toEqual({ code: 'metric_missing' })
    expect(validateDraft(ab({ ...auto, noTracking: true }), ctx)).toEqual({ code: 'auto_winner_untracked' })
  })

  it('wants a valid URL and complete headers on an enabled feed', () => {
    const feed = (over: object) => ({ enabled: true, url: 'https://feed.io/data', headers: [], ...over })
    expect(validateDraft({ ...base(), globalFeed: feed({ url: '' }) }, ctx)).toEqual({
      code: 'feed_url_missing',
      feed: 'global'
    })
    expect(validateDraft({ ...base(), recipientFeed: feed({ url: 'ftp://feed.io' }) }, ctx)).toEqual({
      code: 'feed_url_invalid',
      feed: 'recipient'
    })
    expect(
      validateDraft({ ...base(), globalFeed: feed({ headers: [{ name: 'X-Key', value: '' }] }) }, ctx)
    ).toEqual({ code: 'feed_header_incomplete', feed: 'global' })
    expect(
      validateDraft({ ...base(), globalFeed: feed({ headers: [{ name: '', value: 'v' }] }) }, ctx)
    ).toEqual({ code: 'feed_header_incomplete', feed: 'global' })
    expect(validateDraft({ ...base(), globalFeed: { enabled: false, url: '', headers: [] } }, ctx)).toBeNull()
  })
})

describe('feedUrlIsValid', () => {
  it.each([
    ['https://feed.io/data', true],
    ['http://feed.io:8080/data', true],
    ['https:///nohost', false],
    ['https://feed.io/data?key=1', false],
    ['https://feed.io/data?', false],
    ['https://feed.io/data#top', false],
    ['feed.io/data', false]
  ])('%s → %s', (url, valid) => {
    expect(feedUrlIsValid(url)).toBe(valid)
  })
})
