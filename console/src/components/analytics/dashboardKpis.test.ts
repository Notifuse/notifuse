import { describe, expect, it } from 'vitest'
import { computeDashboardKpis, emptyTotals, parseSeries, sumSeries, type MessageTotals } from './dashboardKpis'

const totals = (values: Partial<MessageTotals>): MessageTotals => ({
  ...emptyTotals(),
  ...values
})

describe('parseSeries', () => {
  it('reads the day from the first ten characters and converts numbers', () => {
    const points = parseSeries([
      {
        created_at_day: '2026-11-09T00:00:00Z',
        count_sent: '12',
        count_delivered: 10,
        count_opened: null
      },
      { created_at_day: '2026-11-10T00:00:00Z' }
    ])
    expect(points).toEqual([
      { day: '2026-11-09', ...totals({ sent: 12, delivered: 10 }) },
      { day: '2026-11-10', ...emptyTotals() }
    ])
  })

  it('accepts a missing response', () => {
    expect(parseSeries(undefined)).toEqual([])
  })
})

describe('sumSeries', () => {
  it('adds every day', () => {
    const points = parseSeries([
      {
        created_at_day: '2026-11-09T00:00:00Z',
        count_sent: 2,
        count_opens_tracked: 1
      },
      {
        created_at_day: '2026-11-10T00:00:00Z',
        count_sent: 3,
        count_opens_tracked: 3
      }
    ])
    expect(sumSeries(points)).toEqual(totals({ sent: 5, opensTracked: 4 }))
  })
})

describe('computeDashboardKpis', () => {
  const series = {
    broadcast: totals({
      sent: 600,
      delivered: 594,
      bounced: 6,
      opened: 300,
      opensTracked: 600,
      clicked: 60,
      clicksTracked: 600
    }),
    automation: totals({
      sent: 300,
      delivered: 297,
      bounced: 3,
      opened: 90,
      opensTracked: 300,
      clicked: 30,
      clicksTracked: 300
    }),
    transactional: totals({
      sent: 80,
      delivered: 80,
      opened: 50,
      opensTracked: 80,
      clicked: 8,
      clicksTracked: 80
    }),
    // Template tests and double opt-in confirmations: in the totals like any other email.
    other: totals({
      sent: 20,
      delivered: 20,
      opened: 10,
      opensTracked: 20,
      clicked: 2,
      clicksTracked: 20
    })
  }

  it('sums every origin', () => {
    const kpis = computeDashboardKpis({ series })
    expect(kpis.sent.total).toBe(1000)
    expect(kpis.sent.broadcastShare).toBeCloseTo(0.6)
    expect(kpis.delivered.rate).toBeCloseTo(0.991)
    expect(kpis.delivered.bounceRate).toBeCloseTo(0.009)
    expect(kpis.openRate.rate).toBeCloseTo(0.45)
    expect(kpis.clickRate.rate).toBeCloseTo(0.1)
  })

  it('gives no rate on a zero denominator', () => {
    const kpis = computeDashboardKpis({
      series: {
        broadcast: emptyTotals(),
        automation: emptyTotals(),
        transactional: emptyTotals(),
        other: emptyTotals()
      }
    })
    expect(kpis.sent.total).toBe(0)
    expect(kpis.sent.broadcastShare).toBeNull()
    expect(kpis.delivered.rate).toBeNull()
    expect(kpis.delivered.noDeliveryEvents).toBe(false)
    expect(kpis.openRate.rate).toBeNull()
    expect(kpis.openRate.deltaPoints).toBeNull()
  })

  it('says when no delivery event came back', () => {
    const kpis = computeDashboardKpis({
      series: {
        ...series,
        broadcast: totals({ sent: 10 }),
        automation: emptyTotals(),
        transactional: emptyTotals(),
        other: emptyTotals()
      }
    })
    expect(kpis.delivered.noDeliveryEvents).toBe(true)
    expect(kpis.delivered.rate).toBeNull()
  })

  it('warns above 2 % bounces or 0.1 % complaints', () => {
    const only = { automation: emptyTotals(), transactional: emptyTotals(), other: emptyTotals() }
    const ok = computeDashboardKpis({
      series: {
        ...only,
        broadcast: totals({
          sent: 1000,
          delivered: 980,
          bounced: 20,
          complained: 1
        })
      }
    })
    expect(ok.delivered.tone).toBe('neutral')
    const bounces = computeDashboardKpis({
      series: {
        ...only,
        broadcast: totals({ sent: 1000, delivered: 979, bounced: 21 })
      }
    })
    expect(bounces.delivered.tone).toBe('warning')
    const complaints = computeDashboardKpis({
      series: {
        ...only,
        broadcast: totals({ sent: 1000, delivered: 998, complained: 2 })
      }
    })
    expect(complaints.delivered.tone).toBe('warning')
  })

  it('compares rates in points with a ±0.05 neutral band', () => {
    // current open rate 45 %
    const up = computeDashboardKpis({
      series,
      previous: totals({
        opened: 431,
        opensTracked: 1000,
        clicked: 100,
        clicksTracked: 1000
      })
    })
    expect(up.openRate.deltaPoints).toBeCloseTo(1.9)
    expect(up.openRate.tone).toBe('positive')
    expect(up.clickRate.deltaPoints).toBeCloseTo(0)
    expect(up.clickRate.tone).toBe('neutral')
    const down = computeDashboardKpis({
      series,
      previous: totals({ opened: 4506, opensTracked: 10000 })
    })
    expect(down.openRate.tone).toBe('negative')
    const flat = computeDashboardKpis({
      series,
      previous: totals({ opened: 4504, opensTracked: 10000 })
    })
    expect(flat.openRate.tone).toBe('neutral')
  })

  it('gives no delta when the previous period tracked nothing', () => {
    const kpis = computeDashboardKpis({ series, previous: emptyTotals() })
    expect(kpis.openRate.deltaPoints).toBeNull()
    expect(kpis.openRate.tone).toBe('neutral')
  })

  it('flags partial tracking with the tracked count', () => {
    const kpis = computeDashboardKpis({
      series: {
        ...series,
        broadcast: totals({ sent: 600, opensTracked: 400, clicksTracked: 600 })
      }
    })
    expect(kpis.openRate.partial).toBe(true)
    expect(kpis.openRate.tracked).toBe(800)
    expect(kpis.clickRate.partial).toBe(false)
  })

  it('counts new contacts', () => {
    expect(computeDashboardKpis({ series, contactsTotal: 18342, contactsNew: 1204 }).contacts).toEqual({
      total: 18342,
      added: 1204,
      tone: 'positive'
    })
    expect(computeDashboardKpis({ series, contactsTotal: 5, contactsNew: 0 }).contacts.tone).toBe('neutral')
  })

  it('compares visits relatively, with the tone of the shown value', () => {
    const up = computeDashboardKpis({
      series,
      visits: 1175,
      previousVisits: 1000
    })
    expect(up.visits.deltaRatio).toBeCloseTo(0.175)
    expect(up.visits.tone).toBe('positive')
    expect(computeDashboardKpis({ series, visits: 900, previousVisits: 1000 }).visits.tone).toBe('negative')
    // +0.04 % shows as +0.0 %: not green
    expect(computeDashboardKpis({ series, visits: 10004, previousVisits: 10000 }).visits.tone).toBe('neutral')
    const empty = computeDashboardKpis({
      series,
      visits: 50,
      previousVisits: 0
    })
    expect(empty.visits.deltaRatio).toBeNull()
    expect(empty.visits.tone).toBe('neutral')
  })
})
