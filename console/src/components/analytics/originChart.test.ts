import { describe, expect, it } from 'vitest'
import { buildOriginChart } from './originChart'
import { emptyTotals, type SeriesPoint } from './dashboardKpis'

const point = (day: string, values: Partial<SeriesPoint>): SeriesPoint => ({
  ...emptyTotals(),
  day,
  ...values
})
const days = ['2026-11-08', '2026-11-09', '2026-11-10']

describe('buildOriginChart', () => {
  it('reads the measure of the chosen metric', () => {
    const chart = buildOriginChart(
      {
        broadcast: days.map((day) => point(day, { sent: 5, opened: 2, clicked: 1 })),
        transactional: days.map((day) => point(day, { sent: 3, opened: 1 })),
        automation: days.map((day) => point(day, { sent: 4 })),
        other: days.map((day) => point(day, { sent: 1, opened: 3 }))
      },
      'opens'
    )
    expect(chart.days).toEqual(days)
    expect(chart.values.broadcast).toEqual([2, 2, 2])
    expect(chart.values.transactional).toEqual([1, 1, 1])
    expect(chart.values.automation).toEqual([0, 0, 0])
    expect(chart.values.other).toEqual([3, 3, 3])
    expect(chart.log).toBe(false)
    expect(chart.empty).toBe(false)
    expect(
      buildOriginChart(
        {
          broadcast: [point('2026-11-08', { clicked: 7 })],
          transactional: [],
          automation: [],
          other: []
        },
        'clicks'
      ).values.broadcast
    ).toEqual([7])
  })

  it('switches to a log axis when one day dwarfs the median, without drawing zeros', () => {
    const chart = buildOriginChart(
      {
        broadcast: [point(days[0], { sent: 0 }), point(days[1], { sent: 18000 }), point(days[2], { sent: 0 })],
        transactional: days.map((day) => point(day, { sent: 500 })),
        automation: days.map((day) => point(day, { sent: 300 })),
        other: days.map((day) => point(day, { sent: 0 }))
      },
      'sent'
    )
    expect(chart.log).toBe(true)
    expect(chart.values.broadcast).toEqual([null, 18000, null])
  })

  it('stays linear when the peak is within ten times the median', () => {
    const chart = buildOriginChart(
      {
        broadcast: [point(days[0], { sent: 0 }), point(days[1], { sent: 4000 }), point(days[2], { sent: 0 })],
        transactional: days.map((day) => point(day, { sent: 500 })),
        automation: days.map((day) => point(day, { sent: 400 })),
        other: days.map((day) => point(day, { sent: 0 }))
      },
      'sent'
    )
    expect(chart.log).toBe(false)
    expect(chart.values.broadcast).toEqual([0, 4000, 0])
  })

  it('is empty when nothing was sent', () => {
    const chart = buildOriginChart(
      {
        broadcast: days.map((day) => point(day, {})),
        transactional: [],
        automation: [],
        other: []
      },
      'sent'
    )
    expect(chart.empty).toBe(true)
    expect(chart.values.transactional).toEqual([0, 0, 0])
    expect(chart.values.other).toEqual([0, 0, 0])
  })

  it('is not empty when only template tests or double opt-in confirmations left', () => {
    const chart = buildOriginChart(
      {
        broadcast: days.map((day) => point(day, {})),
        transactional: [],
        automation: [],
        other: [point(days[1], { sent: 2 })]
      },
      'sent'
    )
    expect(chart.empty).toBe(false)
    expect(chart.values.other).toEqual([0, 2, 0])
  })
})
