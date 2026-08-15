// Shared types for the web analytics console. Field names mirror
// internal/domain/web_analytics_schemas.go so a dimension or measure id can be
// pasted straight into an analytics query.

/**
 * The web analytics sections, in the order they are rendered as tabs.
 *
 * Lives here rather than beside the context that consumes it because the AI
 * tool definitions need the names as a VALUE, for the navigate_to_tab enum, and
 * value-importing them from `context.tsx` would drag services/api/workspace →
 * services/api/client → the router into a module whose whole point is that it
 * imports nothing heavy. `context.tsx` re-exports both, so every existing
 * importer is unaffected.
 */
export const WEB_ANALYTICS_TABS = [
  'dashboard',
  'explore',
  'goals',
  'filters',
  'annotations'
] as const
export type WebAnalyticsTab = (typeof WEB_ANALYTICS_TABS)[number]

export const DATE_PRESETS = [
  'today',
  'yesterday',
  'previous_7_days',
  'previous_14_days',
  'previous_28_days',
  'previous_30_days',
  'previous_90_days',
  'previous_91_days',
  'this_week',
  'previous_week',
  'this_month',
  'previous_month',
  'this_quarter',
  'previous_quarter',
  'this_year',
  'previous_year',
  'previous_12_months',
  'all_time',
  'custom'
] as const

export type DatePreset = (typeof DATE_PRESETS)[number]

/** The presets offered in the picker, grouped the way they are rendered. */
export const PRESET_GROUPS: DatePreset[][] = [
  ['today', 'yesterday'],
  ['previous_7_days', 'previous_28_days', 'previous_91_days'],
  ['this_month', 'previous_month'],
  ['this_year', 'previous_12_months'],
  ['all_time', 'custom']
]

export type ComparisonMode = 'previous_period' | 'previous_year' | 'none'

export type Granularity = 'hour' | 'day' | 'week' | 'month' | 'year'

/** Operators the dimension filter builder offers, by dimension type. */
export const DIMENSION_FILTER_OPERATORS = [
  'equals',
  'notEquals',
  'contains',
  'notContains',
  'isEmpty',
  'isNotEmpty',
  'in',
  'notIn',
  'gt',
  'gte',
  'lt',
  'lte'
] as const

export type DimensionFilterOperator = (typeof DIMENSION_FILTER_OPERATORS)[number]

/** A filter on a dimension, applied to every widget of the current tab. */
export interface WebDimensionFilter {
  dimension: string
  operator: DimensionFilterOperator
  values: (string | number)[]
}

export const METRIC_FILTER_OPERATORS = ['gt', 'gte', 'lt', 'lte'] as const

export type MetricFilterOperator = (typeof METRIC_FILTER_OPERATORS)[number]

/** A filter on an aggregated measure, rendered as SQL HAVING by the engine. */
export interface WebMetricFilter {
  metric: string
  operator: MetricFilterOperator
  values: number[]
}

/**
 * A date range resolved to concrete bounds.
 *
 * Two representations, because the analytics engine treats them differently:
 * `startDay`/`endDay` are local calendar days for `timeDimensions`, which the
 * server converts to instants using the query timezone (and which its
 * gap-filler insists on parsing as YYYY-MM-DD); `startUtc`/`endUtc` are the
 * same bounds already converted, for plain `inDateRange` filters, which the
 * server compares verbatim. Both cover the same span, so a KPI card and the
 * chart above it always agree.
 */
export interface ResolvedRange {
  startDay: string
  endDay: string
  startUtc: string
  endUtc: string
}

/** The three web analytics cube schemas. */
export type WebSchema = 'web_sessions' | 'web_pages' | 'web_goals'

/** Each schema's time dimension, used for range filtering and bucketing. */
export const SCHEMA_TIME_DIMENSION: Record<WebSchema, string> = {
  web_sessions: 'created_at',
  web_pages: 'entered_at',
  web_goals: 'goal_at'
}

/** Value formats used by KPI tiles, chart axes and table cells. */
export type ValueFormat = 'number' | 'duration' | 'percentage' | 'currency'

export interface MetricConfig {
  key: string
  label: string
  format: ValueFormat
  color: string
  /** Lower is better, so a negative change is rendered as an improvement. */
  invertTrend?: boolean
  tooltip?: string
}

export const COMPARISON_COLOR = '#9ca3af'
export const PRIMARY_COLOR = '#7763f1'
export const POSITIVE_COLOR = '#10b981'
export const NEGATIVE_COLOR = '#f97316'

/**
 * The four session metrics the dashboard and explore views are built around.
 * Labels are English here and translated at render time.
 */
export const SESSION_METRICS: MetricConfig[] = [
  { key: 'sessions', label: 'Sessions', format: 'number', color: PRIMARY_COLOR },
  {
    key: 'median_duration',
    label: 'Median TimeScore',
    format: 'duration',
    color: POSITIVE_COLOR,
    tooltip: 'TimeScore is the median engaged time across all sessions'
  },
  {
    key: 'bounce_rate',
    label: 'Bounce Rate',
    format: 'percentage',
    color: '#f59e0b',
    invertTrend: true
  },
  { key: 'median_scroll', label: 'Median Scroll Depth', format: 'percentage', color: '#3b82f6' }
]

export const SESSION_METRIC_KEYS = SESSION_METRICS.map((metric) => metric.key)

/**
 * TimeScore value (in seconds) treated as "good enough" by the heat map. The
 * scale runs white → green up to this point and green → cyan beyond it.
 */
export const TIMESCORE_REFERENCE_SECONDS = 60

/** One point of a metric time series. */
export interface ChartDataPoint {
  timestamp: string
  value: number
}

/** A measure's current and comparison totals plus the derived change. */
export interface MetricTotals {
  current: number
  previous: number
  changePercent: number
}

/** A row of a dimension breakdown, with `prev_*` fields when comparing. */
export interface DimensionRow {
  dimension_value: string
  [key: string]: string | number | undefined
}
