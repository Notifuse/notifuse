import { describe, it, expect } from 'vitest'
import { describeVariationRate, toVariationMetrics } from './variationMetrics'
import type { MessageHistoryStatusSum } from '../../services/api/messages_history'

const sum = (over: Partial<MessageHistoryStatusSum> = {}): MessageHistoryStatusSum => ({
  total_sent: 100,
  total_delivered: 100,
  total_bounced: 0,
  total_complained: 0,
  total_failed: 0,
  total_opened: 10,
  total_clicked: 5,
  total_unsubscribed: 0,
  ...over
})

describe('toVariationMetrics', () => {
  it('leaves an unsent variation without metrics', () => {
    expect(toVariationMetrics(undefined)).toBeUndefined()
    expect(toVariationMetrics(sum({ total_sent: 0 }))).toBeUndefined()
  })

  it('computes the rates over the tracked emails', () => {
    const m = toVariationMetrics(sum({ total_opens_tracked: 40, total_clicks_tracked: 20 }))
    expect(m).toMatchObject({
      recipients: 100,
      opens: 10,
      open_rate: 0.25,
      opens_tracked: 40,
      clicks: 5,
      click_rate: 0.25,
      clicks_tracked: 20
    })
  })

  it('uses the sent total when the stats carry no tracked total', () => {
    const m = toVariationMetrics(sum())
    expect(m?.open_rate).toBe(0.1)
    expect(m?.opens_tracked).toBe(100)
  })

  it('gives a zero rate, not NaN, when nothing was tracked', () => {
    const m = toVariationMetrics(sum({ total_opened: 0, total_opens_tracked: 0 }))
    expect(m?.open_rate).toBe(0)
  })
})

describe('describeVariationRate', () => {
  it('shows the rate with no share when every recipient was tracked', () => {
    expect(describeVariationRate(0.1, 100, 100)).toEqual({
      text: '10.0%',
      tracked: 100,
      trackedShare: undefined,
      lowTrackedShare: false
    })
  })

  it('reports the tracked share and flags it under half', () => {
    expect(describeVariationRate(0.25, 100, 40)).toMatchObject({
      text: '25.0%',
      tracked: 40,
      trackedShare: 0.4,
      lowTrackedShare: true
    })
    expect(describeVariationRate(0.25, 100, 60).lowTrackedShare).toBe(false)
  })

  it('shows no rate when nothing was tracked', () => {
    expect(describeVariationRate(0, 100, 0)).toMatchObject({ text: '-', tracked: 0, lowTrackedShare: true })
  })

  it('reads a missing tracked total as every recipient tracked', () => {
    expect(describeVariationRate(0.1, 100, undefined)).toMatchObject({ text: '10.0%', tracked: 100 })
  })
})
