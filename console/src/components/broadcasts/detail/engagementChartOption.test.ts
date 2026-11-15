import { describe, it, expect } from 'vitest'
import {
  axisLabel,
  buildChartOption,
  hourStep,
  labelledIndex,
  tooltipHeading,
  type ChartMarker
} from './engagementChartOption'
import type { Bucket } from './fillBuckets'

/** Hourly buckets from a local hour, written as the server writes them. */
const hours = (from: string, count: number, value = 0): Bucket[] =>
  Array.from({ length: count }, (_, i) => ({
    key: new Date(Date.parse(from) + i * 3_600_000).toISOString().replace('.000Z', 'Z'),
    value
  }))

const build = (over: Partial<Parameters<typeof buildChartOption>[0]> = {}) =>
  buildChartOption({
    opens: hours('2026-11-10T08:00:00Z', 48, 2),
    clicks: hours('2026-11-10T08:00:00Z', 48, 1),
    granularity: 'hour',
    markers: [],
    labels: { opens: 'Opens', clicks: 'Clicks' },
    locale: 'fr',
    formatCount: (n) => `n=${n}`,
    periodHours: 48,
    ...over
  })

describe('axisLabel', () => {
  it('writes an hour alone, in the language of the console', () => {
    expect(axisLabel('2026-11-10T15:00:00Z', 'hour', 'fr')).toBe('15 h')
    expect(axisLabel('2026-11-10T15:00:00Z', 'hour', 'en')).toBe('3 PM')
  })

  it('writes a morning hour without a leading zero', () => {
    expect(axisLabel('2026-11-10T06:00:00Z', 'hour', 'fr')).toBe('6 h')
    expect(axisLabel('2026-11-10T06:00:00Z', 'hour', 'de')).toBe('6 Uhr')
  })

  it('writes the day at midnight, and for a day bucket', () => {
    expect(axisLabel('2026-11-11T00:00:00Z', 'hour', 'fr')).toBe('11 nov.')
    expect(axisLabel('2026-11-11T00:00:00Z', 'day', 'fr')).toBe('11 nov.')
    expect(axisLabel('2026-11-11T00:00:00Z', 'day', 'en')).toBe('Nov 11')
  })

  it('reads the key as written, whatever the zone of the machine', () => {
    // The key is local time with a Z: 23:00 stays 23 h, never the next day.
    expect(axisLabel('2026-11-10T23:00:00Z', 'hour', 'fr')).toBe('23 h')
  })
})

describe('tooltipHeading', () => {
  it('writes the bucket in words, never its ISO key', () => {
    const hour = tooltipHeading('2026-11-11T09:00:00Z', 'hour', 'fr')
    expect(hour).toContain('11 nov.')
    expect(hour).toContain('9 h')
    expect(hour).not.toContain('2026-11-11T')
    expect(tooltipHeading('2026-11-11T00:00:00Z', 'day', 'fr')).not.toMatch(/\bh\b/)
  })
})

describe('labelledIndex', () => {
  it('labels every 6 hours over 48 h, starting on the hours that divide the day', () => {
    const keys = hours('2026-11-10T08:00:00Z', 48).map((b) => b.key)
    expect(hourStep(48)).toBe(6)
    const shown = keys.filter((_, i) => labelledIndex(keys, 'hour', 48)(i)).map((k) => k.slice(11, 13))
    expect(new Set(shown)).toEqual(new Set(['00', '06', '12', '18']))
  })

  it('labels only midnights over 7 days, so the axis names the days', () => {
    const keys = hours('2026-11-10T08:00:00Z', 7 * 24).map((b) => b.key)
    const shown = keys.filter((_, i) => labelledIndex(keys, 'hour', 7 * 24)(i))
    expect(shown.length).toBeGreaterThan(0)
    expect(shown.every((k) => k.slice(11, 13) === '00')).toBe(true)
  })

  it('labels a recent broadcast’s week view by day too, though only a day and a half is filled', () => {
    const keys = hours('2026-11-10T08:00:00Z', 31).map((b) => b.key)
    const shown = keys.filter((_, i) => labelledIndex(keys, 'hour', 7 * 24)(i))
    expect(shown).toEqual(['2026-11-11T00:00:00Z'])
  })

  it('labels the first hour of a broadcast started minutes ago, whose hours are not on the step yet', () => {
    const keys = hours('2026-11-10T08:00:00Z', 3).map((b) => b.key)
    expect(keys.filter((_, i) => labelledIndex(keys, 'hour', 48)(i))).toEqual(['2026-11-10T08:00:00Z'])
  })

  it('never labels the first hour beside the midnight that names the first day', () => {
    const keys = hours('2026-11-11T23:00:00Z', 7 * 24).map((b) => b.key)
    expect(labelledIndex(keys, 'hour', 7 * 24)(0)).toBe(false)
    expect(labelledIndex(keys, 'hour', 7 * 24)(1)).toBe(true)
  })

  it('labels every day of a daily axis', () => {
    expect(labelledIndex(['2026-11-10T00:00:00Z', '2026-11-11T00:00:00Z'], 'day', 30 * 24)(1)).toBe(true)
  })
})

describe('buildChartOption', () => {
  it('puts opens in bars and clicks in a line, over the keys of the buckets', () => {
    const option = buildChartOption({
      opens: [
        { key: '2026-11-10T08:00:00Z', value: 3 },
        { key: '2026-11-10T09:00:00Z', value: 1 }
      ],
      clicks: [
        { key: '2026-11-10T08:00:00Z', value: 2 },
        { key: '2026-11-10T09:00:00Z', value: 0 }
      ],
      granularity: 'hour',
      markers: [],
      labels: { opens: 'Opens', clicks: 'Clicks' },
      locale: 'en',
      formatCount: String,
      periodHours: 48
    })
    expect(option.series[0].data).toEqual([3, 1])
    expect(option.series[1].data).toEqual([2, 0])
    expect(option.xAxis.data).toEqual(['2026-11-10T08:00:00Z', '2026-11-10T09:00:00Z'])
  })

  it('draws opens as bars and clicks as a smooth line that never dips below zero', () => {
    const option = build()
    expect(option.series[0]).toMatchObject({ name: 'Opens', type: 'bar' })
    expect(option.series[1]).toMatchObject({ name: 'Clicks', type: 'line', smooth: true, smoothMonotone: 'x' })
    expect(option.legend).toMatchObject({ right: 0, data: ['Opens', 'Clicks'] })
  })

  it('writes its tooltip with the heading in words and the counts formatted', () => {
    const option = build()
    const html = option.tooltip.formatter([
      { axisValue: '2026-11-11T09:00:00Z', marker: '<i></i>', seriesName: 'Opens', value: 1234 },
      { axisValue: '2026-11-11T09:00:00Z', marker: '<i></i>', seriesName: 'Clicks', value: 5 }
    ])
    expect(html).toContain('11 nov.')
    expect(html).not.toContain('2026-11-11T09:00:00Z')
    expect(html).toContain('Opens <b>n=1234</b>')
    expect(html).toContain('Clicks <b>n=5</b>')
  })

  it('writes a dash, not a 0, for a bucket still to come', () => {
    const option = build()
    const html = option.tooltip.formatter([
      { axisValue: '2026-11-11T09:00:00Z', seriesName: 'Opens', value: null },
      { axisValue: '2026-11-11T09:00:00Z', seriesName: 'Clicks', value: undefined }
    ])
    expect(html).toContain('Opens <b>—</b>')
    expect(html).toContain('Clicks <b>—</b>')
    expect(html).not.toContain('n=0')
  })

  it('writes a series name in its tooltip as text, not as HTML', () => {
    const option = build({ labels: { opens: 'Opens <unique>', clicks: 'Clicks & co' } })
    const html = option.tooltip.formatter([
      { axisValue: '2026-11-11T09:00:00Z', seriesName: 'Opens <unique>', value: 1 },
      { axisValue: '2026-11-11T09:00:00Z', seriesName: 'Clicks & co', value: 1 }
    ])
    expect(html).toContain('Opens &lt;unique&gt;')
    expect(html).toContain('Clicks &amp; co')
    expect(html).not.toContain('<unique>')
  })

  it('draws a dashed marker for each date inside the window, none outside it', () => {
    const markers: ChartMarker[] = [
      { key: '2026-11-10T20:00:00Z', label: 'Winner sent' },
      { key: '2026-11-20T09:00:00Z', label: 'Outside' }
    ]
    const option = build({ markers })
    const data = option.series[0].markLine!.data
    expect(data.map((m) => m.name)).toEqual(['Winner sent'])
    expect(option.series[0].markLine!.lineStyle.type).toBe('dashed')
  })

  it('gives two markers their own line, so their labels never overlap', () => {
    const option = build({
      markers: [
        { key: '2026-11-10T08:00:00Z', label: 'Test sent' },
        { key: '2026-11-10T09:00:00Z', label: 'Winner sent' }
      ]
    })
    const [first, second] = option.series[0].markLine!.data
    expect(first!.label.offset[1]).not.toBe(second!.label.offset[1])
    // Room above the plot for the second line.
    expect(option.grid.top).toBeGreaterThan(build().grid.top)
  })

  it('aligns a marker near an edge inwards, so its label stays in the chart', () => {
    const option = build({
      markers: [
        { key: '2026-11-10T08:00:00Z', label: 'Test sent' },
        { key: '2026-11-12T07:00:00Z', label: 'Winner sent' }
      ]
    })
    const [start, end] = option.series[0].markLine!.data
    expect(start!.label.align).toBe('left')
    expect(end!.label.align).toBe('right')
    const middle = build({ markers: [{ key: '2026-11-11T08:00:00Z', label: 'Winner sent' }] })
    expect(middle.series[0].markLine!.data[0]!.label.align).toBe('center')
  })
})
