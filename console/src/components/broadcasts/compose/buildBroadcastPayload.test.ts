import { describe, it, expect } from 'vitest'
import { buildBroadcastPayload, hasScheduleChoice } from './buildBroadcastPayload'
import { newDraft, type ComposeValues } from './composeValues'
import type { Workspace } from '../../../services/api/types'

const base = (over: Partial<ComposeValues> = {}): ComposeValues => ({
  ...newDraft('News', { id: 'ws', settings: { timezone: 'Europe/Paris', website_url: 'acme.io' } } as unknown as Workspace),
  list: 'news',
  variants: [{ variation_name: 'A', template_id: 't1' }],
  ...over
})
const ctx = { workspaceId: 'ws' }

describe('buildBroadcastPayload', () => {
  it('creates a broadcast unscheduled, tracked, with both feeds off', () => {
    expect(buildBroadcastPayload(base(), ctx)).toEqual({
      workspace_id: 'ws',
      name: 'News',
      audience: { list: 'news', segments: [], exclude_unsubscribed: true },
      schedule: { is_scheduled: false, use_recipient_timezone: false },
      test_settings: {
        enabled: false,
        sample_percentage: 50,
        auto_send_winner: false,
        variations: [{ variation_name: 'A', template_id: 't1' }]
      },
      utm_parameters: { source: 'acme.io', medium: 'email', campaign: 'news' },
      tracking_mode: 'inherit',
      data_feed: {
        global_feed: { enabled: false, url: '', headers: [] },
        recipient_feed: { enabled: false, url: '', headers: [] }
      }
    })
  })

  it('never sends metadata', () => {
    expect(buildBroadcastPayload(base(), { ...ctx, id: 'b1', status: 'draft' })).not.toHaveProperty('metadata')
  })

  it('sends no schedule when a scheduled broadcast is edited', () => {
    expect(buildBroadcastPayload(base({ sendChoice: 'schedule', scheduledDate: '2026-12-01' }), { ...ctx, id: 'b1', status: 'scheduled' })).not.toHaveProperty(
      'schedule'
    )
  })

  it('stores the schedule choice of a draft unbooked', () => {
    const payload = buildBroadcastPayload(
      base({ sendChoice: 'schedule', scheduledDate: '2026-12-01', scheduledTime: '09:30', timezone: 'America/New_York' }),
      { ...ctx, id: 'b1', status: 'draft' }
    )
    expect(payload.schedule).toEqual({
      is_scheduled: false,
      use_recipient_timezone: false,
      scheduled_date: '2026-12-01',
      scheduled_time: '09:30',
      timezone: 'America/New_York'
    })
    expect(buildBroadcastPayload(base(), { ...ctx, id: 'b1', status: 'draft' }).schedule).toEqual({
      is_scheduled: false,
      use_recipient_timezone: false
    })
  })

  it('creates unscheduled even with a schedule choice, which an update then writes', () => {
    const values = base({ sendChoice: 'schedule', scheduledDate: '2026-12-01' })
    expect(buildBroadcastPayload(values, ctx).schedule).toEqual({ is_scheduled: false, use_recipient_timezone: false })
    expect(hasScheduleChoice(values)).toBe(true)
    expect(hasScheduleChoice(base())).toBe(false)
  })

  it('turns stored feeds off and keeps the one left on', () => {
    const payload = buildBroadcastPayload(
      base({
        globalFeed: { enabled: true, url: 'https://feed.io/a', headers: [{ name: 'X', value: 'y' }] },
        recipientFeed: { enabled: false, url: 'https://feed.io/b', headers: [] }
      }),
      { ...ctx, id: 'b1', status: 'draft' }
    )
    expect(payload.data_feed).toEqual({
      global_feed: { enabled: true, url: 'https://feed.io/a', headers: [{ name: 'X', value: 'y' }] },
      recipient_feed: { enabled: false, url: 'https://feed.io/b', headers: [] }
    })
  })

  it('sends the sender chosen for the variation, none when kept or cleared', () => {
    const chosen = buildBroadcastPayload(base({ variants: [{ variation_name: 'A', template_id: 't1', sender_id: 's1' }] }), ctx)
    expect(chosen.test_settings.variations).toEqual([{ variation_name: 'A', template_id: 't1', sender_id: 's1' }])
    const cleared = buildBroadcastPayload(base({ variants: [{ variation_name: 'A', template_id: 't1', sender_id: '' }] }), ctx)
    expect(cleared.test_settings.variations[0]).not.toHaveProperty('sender_id')
  })

  it('gives each A/B variation its own sender and only three keys', () => {
    const payload = buildBroadcastPayload(
      base({
        abEnabled: true,
        variants: [
          { variation_name: 'A', template_id: 't1', sender_id: 's1' },
          { variation_name: 'B', template_id: 't2', sender_id: 's2' }
        ]
      }),
      ctx
    )
    expect(payload.test_settings.variations).toEqual([
      { variation_name: 'A', template_id: 't1', sender_id: 's1' },
      { variation_name: 'B', template_id: 't2', sender_id: 's2' }
    ])
    payload.test_settings.variations.forEach((v) =>
      Object.keys(v).forEach((key) => expect(['variation_name', 'template_id', 'sender_id']).toContain(key))
    )
  })

  it('keeps only the first variant without A/B', () => {
    const payload = buildBroadcastPayload(
      base({
        abEnabled: false,
        variants: [
          { variation_name: 'A', template_id: 't1' },
          { variation_name: 'B', template_id: 't2' }
        ]
      }),
      ctx
    )
    expect(payload.test_settings.variations).toEqual([{ variation_name: 'A', template_id: 't1' }])
  })

  it('sends no_tracking and turns the automatic winner off with it', () => {
    const payload = buildBroadcastPayload(
      base({
        abEnabled: true,
        noTracking: true,
        autoSendWinner: true,
        winnerMetric: 'open_rate',
        testDurationHours: 4,
        variants: [
          { variation_name: 'A', template_id: 't1' },
          { variation_name: 'B', template_id: 't2' }
        ]
      }),
      ctx
    )
    expect(payload.tracking_mode).toBe('no_tracking')
    expect(payload.test_settings.auto_send_winner).toBe(false)
    expect(payload.test_settings).not.toHaveProperty('auto_send_winner_metric')
  })

  it('sends the winner metric and duration with an automatic winner', () => {
    const payload = buildBroadcastPayload(
      base({
        abEnabled: true,
        autoSendWinner: true,
        winnerMetric: 'click_rate',
        testDurationHours: 12,
        variants: [
          { variation_name: 'A', template_id: 't1' },
          { variation_name: 'B', template_id: 't2' }
        ]
      }),
      ctx
    )
    expect(payload.test_settings).toMatchObject({
      auto_send_winner: true,
      auto_send_winner_metric: 'click_rate',
      test_duration_hours: 12
    })
  })

  it('keeps the stored utm term and content', () => {
    const payload = buildBroadcastPayload(base({ utmTerm: 'kw', utmContent: 'c' }), ctx)
    expect(payload.utm_parameters).toMatchObject({ term: 'kw', content: 'c' })
  })
})
