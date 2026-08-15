import { describe, expect, it } from 'vitest'
import dayjs from '../../../lib/dayjs'
import { bucketIndexForAnnotation, escapeTooltipHTML, sanitizeRichLabelText } from './annotations'
import { ChartDataPoint, Granularity } from './types'

/** Buckets as the engine serialises them: wall clocks stamped with a "Z". */
function buckets(start: string, count: number, granularity: Granularity): ChartDataPoint[] {
  const step: Record<string, [number, 'hour' | 'day' | 'month' | 'year']> = {
    hour: [1, 'hour'],
    day: [1, 'day'],
    week: [7, 'day'],
    month: [1, 'month'],
    year: [1, 'year']
  }
  const [amount, unit] = step[granularity]
  const first = dayjs.utc(start)
  return Array.from({ length: count }, (_, i) => ({
    timestamp: first.add(i * amount, unit).format('YYYY-MM-DDTHH:mm:ss[Z]'),
    value: i
  }))
}

/**
 * True when every surrogate still has its partner. `Array.from` iterates by
 * code point, so a valid pair arrives as one two-unit string and only an
 * orphan can be a single unit inside the surrogate range.
 */
function hasNoLoneSurrogate(value: string): boolean {
  return Array.from(value).every((codePoint) => {
    if (codePoint.length > 1) return true
    const unit = codePoint.charCodeAt(0)
    return unit < 0xd800 || unit > 0xdfff
  })
}

describe('bucketIndexForAnnotation', () => {
  it('separates repeated hour labels by returning distinct indices', () => {
    // 48 hourly buckets over two days: both annotations sit at 03:00, which
    // formatXAxisLabel renders as "3am" for either. Keying by label would
    // resolve both to the last "3am" category.
    const series = buckets('2026-08-14T00:00:00Z', 48, 'hour')

    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-14T03:00:00Z' }, series, 'hour', 'UTC')
    ).toBe(3)
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-15T03:00:00Z' }, series, 'hour', 'UTC')
    ).toBe(27)
  })

  it('reads bucket timestamps as wall clocks in the query timezone', () => {
    // New York buckets: "2026-08-14T00:00:00Z" means midnight in New York,
    // i.e. the instant 2026-08-14T04:00:00Z. An annotation at 02:00Z on the
    // 15th is 22:00 on the 14th there, so it belongs to the Aug 14 bucket.
    // Comparing the raw strings as instants would answer 2 (Aug 15).
    const series = buckets('2026-08-13T00:00:00Z', 3, 'day')

    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-08-15T02:00:00Z' },
        series,
        'day',
        'America/New_York'
      )
    ).toBe(1)
  })

  it('places an annotation on the boundary in the bucket that starts there', () => {
    const series = buckets('2026-08-13T00:00:00Z', 3, 'day')

    // Midnight in New York on the 14th, to the millisecond.
    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-08-14T04:00:00Z' },
        series,
        'day',
        'America/New_York'
      )
    ).toBe(1)
  })

  it('returns null outside the range instead of clamping to an edge', () => {
    const series = buckets('2026-08-13T00:00:00Z', 3, 'day')

    // One second before the first bucket opens (Aug 13 00:00 New York).
    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-08-13T03:59:59Z' },
        series,
        'day',
        'America/New_York'
      )
    ).toBeNull()

    // The range is half-open on the right: the last bucket ends at Aug 16
    // 00:00 New York, and that instant is already outside it.
    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-08-16T03:59:59Z' },
        series,
        'day',
        'America/New_York'
      )
    ).toBe(2)
    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-08-16T04:00:00Z' },
        series,
        'day',
        'America/New_York'
      )
    ).toBeNull()
  })

  it('leaves an annotation in a hole of a sparse series unplaced', () => {
    const series: ChartDataPoint[] = [
      { timestamp: '2026-08-10T00:00:00Z', value: 1 },
      { timestamp: '2026-08-11T00:00:00Z', value: 2 },
      { timestamp: '2026-08-14T00:00:00Z', value: 3 }
    ]

    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-12T09:00:00Z' }, series, 'day', 'UTC')
    ).toBeNull()
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-14T09:00:00Z' }, series, 'day', 'UTC')
    ).toBe(2)
  })

  it('spans a week bucket over seven days', () => {
    const series = buckets('2026-08-03T00:00:00Z', 3, 'week')

    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-16T23:59:59Z' }, series, 'week', 'UTC')
    ).toBe(1)
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-23T23:59:59Z' }, series, 'week', 'UTC')
    ).toBe(2)
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-24T00:00:00Z' }, series, 'week', 'UTC')
    ).toBeNull()
  })

  it('spans a month bucket over a calendar month, not thirty days', () => {
    const series = buckets('2026-01-01T00:00:00Z', 2, 'month')

    // Feb 28 is inside February even though it is past day 30 of the series.
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-02-28T12:00:00Z' }, series, 'month', 'UTC')
    ).toBe(1)
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-03-01T00:00:00Z' }, series, 'month', 'UTC')
    ).toBeNull()
  })

  it('keeps a bucket a calendar day across a DST change', () => {
    // US DST ends on 2026-11-01, making that New York day 25 hours long.
    const series = buckets('2026-10-31T00:00:00Z', 3, 'day')

    // That bucket runs from 2026-11-01T04:00:00Z to 2026-11-02T05:00:00Z.
    // Adding 24 fixed hours to its start would close it an hour early and
    // spill the last hour of Nov 1 into the Nov 2 bucket.
    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-11-02T04:59:59Z' },
        series,
        'day',
        'America/New_York'
      )
    ).toBe(1)
    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-11-02T05:00:00Z' },
        series,
        'day',
        'America/New_York'
      )
    ).toBe(2)
  })

  it('returns null for an empty series, an unparseable instant or a bad zone', () => {
    const series = buckets('2026-08-13T00:00:00Z', 3, 'day')

    expect(bucketIndexForAnnotation({ annotated_at: '2026-08-13T10:00:00Z' }, [], 'day', 'UTC')).toBeNull()
    expect(bucketIndexForAnnotation({ annotated_at: 'not-a-date' }, series, 'day', 'UTC')).toBeNull()
    // An unusable timezone falls back to UTC rather than throwing inside the chart.
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-14T10:00:00Z' }, series, 'day', 'Mars/Olympus')
    ).toBe(1)
    expect(
      bucketIndexForAnnotation({ annotated_at: '2026-08-14T10:00:00Z' }, series, 'day', undefined)
    ).toBe(1)
    // A granularity the map does not know falls back to a day-wide bucket
    // rather than throwing on an undefined span.
    expect(
      bucketIndexForAnnotation(
        { annotated_at: '2026-08-14T10:00:00Z' },
        series,
        'quarter' as Granularity,
        'UTC'
      )
    ).toBe(1)
  })
})

describe('sanitizeRichLabelText', () => {
  it('neutralises the characters echarts reads as rich-text markup', () => {
    expect(sanitizeRichLabelText('{a|b}')).toBe('a b')
    expect(sanitizeRichLabelText('Launch {promo|v2}')).toBe('Launch promo v2')
  })

  it('flattens whitespace so a label stays on one line', () => {
    expect(sanitizeRichLabelText('Black\nFriday\tsale')).toBe('Black Friday sale')
    expect(sanitizeRichLabelText('   ')).toBe('')
  })

  it('truncates to the label budget, ellipsis included', () => {
    const long = 'Summer clearance campaign kickoff'
    const short = sanitizeRichLabelText(long)

    expect(short).toBe('Summer clearance ca…')
    expect(short.length).toBeLessThanOrEqual(20)
    expect(sanitizeRichLabelText('Exactly twenty chars', 20)).toBe('Exactly twenty chars')
  })

  it('keeps an emoji whole when the cut lands on it', () => {
    // "🎉" is the 19th grapheme — the last slot before the ellipsis — and two
    // UTF-16 code units wide, so a code-unit slice ends on its high half.
    // Asserting on `.length` would not notice: the orphan still counts as one.
    const short = sanitizeRichLabelText('Black Friday: 40% 🎉 off')

    expect(short).toBe('Black Friday: 40% 🎉…')
    expect(hasNoLoneSurrogate(short)).toBe(true)
    expect(Array.from(short).length).toBe(20)
  })

  it('spends one slot on an accented letter rather than one per code point', () => {
    // "é" as e + U+0301: one grapheme, two code units. Budgeting by code
    // units would drop the last letter of the word to pay for the mark.
    expect(sanitizeRichLabelText('Campagne de rentrée scolaire')).toBe(
      'Campagne de rentrée…'
    )
  })
})

describe('escapeTooltipHTML', () => {
  it('escapes markup a title could smuggle into the tooltip', () => {
    expect(escapeTooltipHTML('<img src=x onerror=alert(1)>')).toBe(
      '&lt;img src=x onerror=alert(1)&gt;'
    )
    expect(escapeTooltipHTML('Tom & "Jerry"')).toBe('Tom &amp; &quot;Jerry&quot;')
  })
})
