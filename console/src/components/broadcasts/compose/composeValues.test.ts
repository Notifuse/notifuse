import { describe, it, expect } from 'vitest'
import { campaignFromName, fromBroadcast, newDraft, withName, normalizeTime } from './composeValues'
import type { Broadcast } from '../../../services/api/broadcast'
import type { Workspace } from '../../../services/api/types'

const workspace = {
  id: 'ws',
  settings: { timezone: 'Europe/Paris', website_url: 'https://www.acme.io/blog' }
} as unknown as Workspace

const stored = (over: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'b1',
    name: 'Weekly News',
    status: 'draft',
    audience: { list: 'news', segments: ['vip'], exclude_unsubscribed: false },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: {
      enabled: true,
      sample_percentage: 20,
      auto_send_winner: true,
      auto_send_winner_metric: 'click_rate',
      test_duration_hours: 6,
      variations: [
        { variation_name: 'A', template_id: 't1', sender_id: 's1', template: { id: 't1' } },
        { variation_name: 'B', template_id: 't2' }
      ]
    },
    utm_parameters: { source: 'acme.io', medium: 'email', campaign: 'weekly_news', content: 'x' },
    tracking_mode: 'inherit',
    data_feed: { global_feed: { enabled: true, url: 'https://feed.io/a', headers: [] } },
    ...over
  }) as unknown as Broadcast

describe('campaignFromName', () => {
  it('keeps the current transformation', () => {
    expect(campaignFromName('Black Friday -- 2026!')).toBe('black_friday_2026_')
    expect(campaignFromName('Weekly News')).toBe('weekly_news')
  })
})

describe('newDraft', () => {
  it('starts with the defaults of the current drawer', () => {
    const values = newDraft('Broadcast 11/12/2026', workspace)
    expect(values).toMatchObject({
      name: 'Broadcast 11/12/2026',
      segments: [],
      excludeUnsubscribed: true,
      abEnabled: false,
      variants: [{ variation_name: 'A', template_id: '' }],
      samplePercentage: 50,
      sendChoice: 'now',
      scheduledTime: '11:00',
      timezone: 'Europe/Paris',
      utmSource: 'acme.io',
      utmMedium: 'email',
      utmCampaign: 'broadcast_11_12_2026',
      utmCampaignEdited: false,
      noTracking: false
    })
  })

  it('falls back to UTC and no source without workspace settings', () => {
    const values = newDraft('X', { id: 'ws', settings: {} } as unknown as Workspace)
    expect(values.timezone).toBe('UTC')
    expect(values.utmSource).toBeUndefined()
  })
})

describe('fromBroadcast', () => {
  it('reads every stored value', () => {
    const values = fromBroadcast(stored(), workspace)
    expect(values).toMatchObject({
      name: 'Weekly News',
      list: 'news',
      segments: ['vip'],
      excludeUnsubscribed: false,
      abEnabled: true,
      variants: [
        { variation_name: 'A', template_id: 't1', sender_id: 's1' },
        { variation_name: 'B', template_id: 't2' }
      ],
      samplePercentage: 20,
      autoSendWinner: true,
      winnerMetric: 'click_rate',
      testDurationHours: 6,
      sendChoice: 'now',
      utmCampaign: 'weekly_news',
      utmContent: 'x',
      utmCampaignEdited: false,
      noTracking: false
    })
    expect(values.variants[0]).not.toHaveProperty('template')
    expect(values.globalFeed.url).toBe('https://feed.io/a')
    expect(values.recipientFeed).toEqual({ enabled: false, url: '', headers: [] })
  })

  it('considers utm_campaign edited when it differs from the transformed name', () => {
    const values = fromBroadcast(stored({ utm_parameters: { campaign: 'promo' } }), workspace)
    expect(values.utmCampaignEdited).toBe(true)
    expect(withName(values, 'Other').utmCampaign).toBe('promo')
  })

  it('reads the schedule choice of a draft back', () => {
    const values = fromBroadcast(
      stored({
        schedule: {
          is_scheduled: false,
          scheduled_date: '2026-12-01',
          scheduled_time: '09:30',
          timezone: 'America/New_York',
          use_recipient_timezone: false
        }
      }),
      workspace
    )
    expect(values).toMatchObject({
      sendChoice: 'schedule',
      scheduledDate: '2026-12-01',
      scheduledTime: '09:30',
      timezone: 'America/New_York'
    })
  })

  it('proposes the stored timezone, else the workspace one', () => {
    expect(fromBroadcast(stored(), workspace).timezone).toBe('Europe/Paris')
  })

  it('keeps only the first variation when A/B is off', () => {
    const values = fromBroadcast(
      stored({ test_settings: { ...stored().test_settings, enabled: false } }),
      workspace
    )
    expect(values.variants).toHaveLength(1)
  })

  it('reads no_tracking', () => {
    expect(fromBroadcast(stored({ tracking_mode: 'no_tracking' }), workspace).noTracking).toBe(true)
  })
})

describe('withName', () => {
  it('makes utm_campaign follow the name until edited', () => {
    const values = newDraft('A', workspace)
    expect(withName(values, 'Spring Sale').utmCampaign).toBe('spring_sale')
    expect(withName({ ...values, utmCampaignEdited: true, utmCampaign: 'mine' }, 'Spring Sale').utmCampaign).toBe(
      'mine'
    )
  })
})

describe('normalizeTime', () => {
  it.each([
    ['9:5', '09:05'],
    ['09:30', '09:30'],
    ['14', '14:00'],
    ['7h15', '07:15'],
    ['24:00', null],
    ['10:75', null],
    ['noon', null]
  ])('%s → %s', (input, expected) => {
    expect(normalizeTime(input)).toBe(expected)
  })
})
