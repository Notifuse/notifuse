import { format } from 'echarts/core'
import dayjs from '../../../lib/dayjs'
import { ChartDataPoint, Granularity } from './types'

/**
 * The only annotation field these helpers need. Declared structurally so the
 * chart maths stays independent of the API client's `Annotation`.
 */
export interface AnnotatedInstant {
  /** RFC3339 instant, e.g. "2026-08-15T02:00:00Z". */
  annotated_at: string
}

interface BucketSpan {
  amount: number
  unit: 'hour' | 'day' | 'month' | 'year'
}

/** How much wall-clock time one bucket covers, per granularity. */
const BUCKET_SPAN: Record<Granularity, BucketSpan> = {
  hour: { amount: 1, unit: 'hour' },
  day: { amount: 1, unit: 'day' },
  // Spelled out rather than added as a "week": the SQL side truncates to a
  // 7-day block, and a calendar unit would let the two definitions drift.
  week: { amount: 7, unit: 'day' },
  month: { amount: 1, unit: 'month' },
  year: { amount: 1, unit: 'year' }
}

/** Longest title rendered inside a markLine rich label before it is truncated. */
export const RICH_LABEL_MAX_LENGTH = 20

/**
 * An unknown zone makes `dayjs.tz` throw, and a throw inside the chart's
 * useMemo blanks the whole dashboard. Degrade to UTC instead.
 */
function resolveZone(timezone?: string): string {
  if (!timezone) return 'UTC'
  try {
    dayjs.tz('2020-01-01T00:00:00', timezone)
    return timezone
  } catch {
    return 'UTC'
  }
}

/**
 * Index of the bucket an annotation belongs to, or null when it falls outside
 * the series. Out-of-range is never clamped — an annotation from before the
 * chart starts is not an annotation on its first bucket.
 *
 * Returns an INDEX, never a label. `formatXAxisLabel` produces duplicates (at
 * hour granularity "3am" repeats every 24 buckets, and hour granularity is
 * offered up to 7 days = 168 buckets); echarts resolves a category name
 * through a hash map where a repeat overwrites its predecessor, so a label
 * would silently drop every annotation onto the last matching bucket.
 *
 * `timezone` is the QUERY timezone, which the toolbar can override away from
 * the workspace's. `annotation.timezone` is display intent only — it is what
 * renders "9am in Tokyo" back as 9am — and must never be used to re-parse the
 * instant, which `annotated_at` already fixes on its own.
 */
export function bucketIndexForAnnotation(
  annotation: AnnotatedInstant,
  buckets: ChartDataPoint[],
  granularity: Granularity,
  timezone?: string
): number | null {
  if (buckets.length === 0) return null

  const at = dayjs(annotation.annotated_at)
  if (!at.isValid()) return null

  const zone = resolveZone(timezone)
  const span = BUCKET_SPAN[granularity] ?? BUCKET_SPAN.day

  for (let i = 0; i < buckets.length; i++) {
    // Bucket timestamps are wall clocks carrying a "Z" they did not earn: the
    // engine truncates in the query's timezone and serialises the naive local
    // time. Read as UTC they would be shifted by the workspace/viewer gap, so
    // they are parsed as wall clocks and localised back to a real instant.
    const startWall = dayjs.utc(buckets[i].timestamp)
    if (!startWall.isValid()) continue
    // Adding in wall-clock space before localising keeps a bucket a calendar
    // bucket across a DST change, which adding to a fixed-offset instant would not.
    const endWall = startWall.add(span.amount, span.unit)

    const start = dayjs.tz(startWall.format('YYYY-MM-DDTHH:mm:ss'), zone)
    const end = dayjs.tz(endWall.format('YYYY-MM-DDTHH:mm:ss'), zone)

    // Half-open on the right, so an annotation on a boundary belongs to the
    // bucket that starts there. A gap in a sparse series belongs to no bucket
    // and yields null rather than being absorbed by the bucket before it.
    if (!at.isBefore(start) && at.isBefore(end)) return i
  }

  return null
}

type GraphemeSegmenterCtor = new (
  locales?: string | string[],
  options?: { granularity: 'grapheme' }
) => { segment(input: string): Iterable<{ segment: string }> }

/**
 * `Intl.Segmenter` is ES2022 while the app compiles against ES2020, so it is
 * read off the global behind a feature check rather than through its type.
 */
const segmenterCtor = (Intl as unknown as { Segmenter?: GraphemeSegmenterCtor }).Segmenter
const graphemeSegmenter = segmenterCtor
  ? new segmenterCtor(undefined, { granularity: 'grapheme' })
  : null

/**
 * Splits into user-perceived characters. Cutting by UTF-16 code units can
 * split an emoji between its surrogate halves, and the orphaned half renders
 * as a replacement character on the chart. Where `Segmenter` is missing,
 * `Array.from` at least keeps code points whole.
 */
function toGraphemes(value: string): string[] {
  if (!graphemeSegmenter) return Array.from(value)
  return Array.from(graphemeSegmenter.segment(value), (entry) => entry.segment)
}

/**
 * A title safe to inline in an echarts rich label. `{`, `}` and `|` open,
 * close and split a rich-text style block there, so they are markup rather
 * than text and a title carrying them would render as broken styling — a
 * problem entirely separate from HTML escaping.
 *
 * The length budget is counted in user-perceived characters, so an emoji
 * costs one slot rather than its two code units and is never cut in half.
 */
export function sanitizeRichLabelText(title: string, maxLength = RICH_LABEL_MAX_LENGTH): string {
  const flattened = (title ?? '')
    .replace(/[{}|]/g, ' ')
    // Newlines are line breaks in a label, and a tab shifts the whole block.
    .replace(/\s+/g, ' ')
    .trim()

  // Code units are an upper bound on graphemes, so a title that already fits
  // needs no segmenting at all.
  if (flattened.length <= maxLength) return flattened

  const graphemes = toGraphemes(flattened)
  if (graphemes.length <= maxLength) return flattened
  return `${graphemes.slice(0, maxLength - 1).join('').trimEnd()}…`
}

/**
 * Escapes a value for the tooltip's HTML string. Delegates to echarts' own
 * escaper rather than a hand-rolled one so the tooltip and the library agree
 * on what counts as markup.
 */
export function escapeTooltipHTML(value: string): string {
  return format.encodeHTML(value ?? '')
}
