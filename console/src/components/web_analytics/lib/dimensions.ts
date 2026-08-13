import { weekdayLabel } from './dictionaries'
import { WebDimensionFilter, WebSchema } from './types'

export type DimensionType = 'string' | 'number' | 'boolean'

export interface DimensionInfo {
  name: string
  type: DimensionType
  /** Grouping used by the dimension pickers. */
  category: DimensionCategory
  /** Labels mirror the titles in internal/domain/web_analytics_schemas.go. */
  label: string
  schemas: WebSchema[]
}

export type DimensionCategory =
  | 'Channel'
  | 'UTM'
  | 'Traffic'
  | 'Pages'
  | 'Device'
  | 'Geo'
  | 'Time'
  | 'Session'
  | 'Goal'
  | 'User'
  | 'Custom'

/** Order the categories are rendered in, everywhere a picker groups them. */
export const DIMENSION_CATEGORY_ORDER: DimensionCategory[] = [
  'Channel',
  'UTM',
  'Traffic',
  'Pages',
  'Device',
  'Geo',
  'Time',
  'Session',
  'Goal',
  'User',
  'Custom'
]

const ALL_SCHEMAS: WebSchema[] = ['web_sessions', 'web_pages', 'web_goals']
const SESSIONS_AND_GOALS: WebSchema[] = ['web_sessions', 'web_goals']

function define(
  name: string,
  label: string,
  category: DimensionCategory,
  type: DimensionType,
  schemas: WebSchema[]
): DimensionInfo {
  return { name, label, category, type, schemas }
}

/**
 * Attribution dimensions live on both web_sessions and web_goals: the goal
 * table denormalizes the session's attribution snapshot precisely so goal
 * reports can group by them without a join.
 */
const CATALOG: DimensionInfo[] = [
  define('channel', 'Channel', 'Channel', 'string', SESSIONS_AND_GOALS),
  define('channel_group', 'Channel Group', 'Channel', 'string', SESSIONS_AND_GOALS),

  define('utm_source', 'UTM Source', 'UTM', 'string', SESSIONS_AND_GOALS),
  define('utm_medium', 'UTM Medium', 'UTM', 'string', SESSIONS_AND_GOALS),
  define('utm_campaign', 'UTM Campaign', 'UTM', 'string', SESSIONS_AND_GOALS),
  define('utm_term', 'UTM Term', 'UTM', 'string', SESSIONS_AND_GOALS),
  define('utm_content', 'UTM Content', 'UTM', 'string', SESSIONS_AND_GOALS),

  define('referrer', 'Referrer', 'Traffic', 'string', SESSIONS_AND_GOALS),
  define('referrer_domain', 'Referrer Domain', 'Traffic', 'string', SESSIONS_AND_GOALS),
  define('referrer_path', 'Referrer Path', 'Traffic', 'string', SESSIONS_AND_GOALS),
  define('is_direct', 'Is Direct', 'Traffic', 'boolean', SESSIONS_AND_GOALS),

  define('landing_page', 'Landing Page', 'Pages', 'string', SESSIONS_AND_GOALS),
  define('landing_domain', 'Landing Domain', 'Pages', 'string', SESSIONS_AND_GOALS),
  define('landing_path', 'Landing Path', 'Pages', 'string', SESSIONS_AND_GOALS),
  define('exit_path', 'Exit Path', 'Pages', 'string', ['web_sessions']),
  define('page_path', 'Page Path', 'Pages', 'string', ['web_pages']),
  define('page_number', 'Page Number', 'Pages', 'number', ['web_pages']),
  define('is_landing_page', 'Is Landing Page', 'Pages', 'boolean', ['web_pages']),
  define('is_exit_page', 'Is Exit Page', 'Pages', 'boolean', ['web_pages']),
  define('page_entry_type', 'Entry Type', 'Pages', 'string', ['web_pages']),

  define('device', 'Device', 'Device', 'string', SESSIONS_AND_GOALS),
  define('browser', 'Browser', 'Device', 'string', SESSIONS_AND_GOALS),
  define('browser_type', 'Browser Type', 'Device', 'string', SESSIONS_AND_GOALS),
  define('os', 'Operating System', 'Device', 'string', SESSIONS_AND_GOALS),
  define('connection_type', 'Connection Type', 'Device', 'string', SESSIONS_AND_GOALS),
  define('screen_width', 'Screen Width', 'Device', 'number', SESSIONS_AND_GOALS),
  define('screen_height', 'Screen Height', 'Device', 'number', SESSIONS_AND_GOALS),
  define('viewport_width', 'Viewport Width', 'Device', 'number', SESSIONS_AND_GOALS),
  define('viewport_height', 'Viewport Height', 'Device', 'number', SESSIONS_AND_GOALS),

  define('country', 'Country', 'Geo', 'string', SESSIONS_AND_GOALS),
  define('region', 'Region', 'Geo', 'string', SESSIONS_AND_GOALS),
  define('city', 'City', 'Geo', 'string', SESSIONS_AND_GOALS),
  define('language', 'Language', 'Geo', 'string', SESSIONS_AND_GOALS),
  define('timezone', 'Timezone', 'Geo', 'string', SESSIONS_AND_GOALS),

  define('hour_of_day', 'Hour of Day', 'Time', 'number', SESSIONS_AND_GOALS),
  define('day_of_week', 'Day of Week', 'Time', 'number', SESSIONS_AND_GOALS),
  define('is_weekend', 'Is Weekend', 'Time', 'boolean', SESSIONS_AND_GOALS),
  define('year', 'Year', 'Time', 'number', SESSIONS_AND_GOALS),
  define('month', 'Month', 'Time', 'number', SESSIONS_AND_GOALS),
  define('day', 'Day', 'Time', 'number', SESSIONS_AND_GOALS),
  define('week_number', 'Week Number', 'Time', 'number', SESSIONS_AND_GOALS),

  define('pageview_count', 'Pageview Count', 'Session', 'number', ['web_sessions']),
  define('duration', 'Duration (ms)', 'Session', 'number', ['web_sessions']),
  define('sdk_version', 'SDK Version', 'Session', 'string', ['web_sessions']),

  define('goal_name', 'Goal Name', 'Goal', 'string', ['web_goals']),
  define('goal_type', 'Goal Type', 'Goal', 'string', ['web_goals']),
  define('goal_path', 'Goal Path', 'Goal', 'string', ['web_goals']),
  define('goal_value', 'Goal Value', 'Goal', 'number', ['web_goals']),

  define('contact_email', 'Contact Email', 'User', 'string', ALL_SCHEMAS)
]

for (let slot = 1; slot <= 10; slot++) {
  CATALOG.push(define(`custom_${slot}`, `Custom ${slot}`, 'Custom', 'string', SESSIONS_AND_GOALS))
}

export const DIMENSIONS: Record<string, DimensionInfo> = Object.fromEntries(
  CATALOG.map((dimension) => [dimension.name, dimension])
)

/** Dimensions groupable on a schema, in catalog order. */
export function dimensionsForSchema(schema: WebSchema): DimensionInfo[] {
  return CATALOG.filter((dimension) => dimension.schemas.includes(schema))
}

/**
 * Display label for a dimension. Custom slots take the workspace's own label
 * when one is configured, so a dashboard reads "Plan" rather than "Custom 3".
 */
export function getDimensionLabel(
  name: string,
  customLabels?: Record<string, string>
): string {
  const custom = customLabels?.[name]
  if (custom) return custom
  const known = DIMENSIONS[name]
  if (known) return known.label
  return name
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

export function getDimensionType(name: string): DimensionType {
  return DIMENSIONS[name]?.type ?? 'string'
}

/** Groups dimensions by category, preserving DIMENSION_CATEGORY_ORDER. */
export function groupByCategory(
  dimensions: DimensionInfo[]
): { category: DimensionCategory; dimensions: DimensionInfo[] }[] {
  const groups = new Map<DimensionCategory, DimensionInfo[]>()
  for (const dimension of dimensions) {
    const bucket = groups.get(dimension.category)
    if (bucket) bucket.push(dimension)
    else groups.set(dimension.category, [dimension])
  }
  return DIMENSION_CATEGORY_ORDER.filter((category) => groups.has(category)).map((category) => ({
    category,
    dimensions: groups.get(category) as DimensionInfo[]
  }))
}

/**
 * Narrows page filters to the ones a schema can actually group by.
 *
 * Filters are shared by every tab, so the goals tab inherits whatever the
 * dashboard was drilled into. A dimension that only exists on the other schema
 * — a `goal_name` filter sent to web_sessions, an `exit_path` one sent to
 * web_goals — makes the engine reject the entire query, which would blank the
 * widget instead of merely ignoring an inapplicable filter.
 */
export function filtersForSchema(
  filters: WebDimensionFilter[],
  schema: WebSchema
): WebDimensionFilter[] {
  return filters.filter((filter) => DIMENSIONS[filter.dimension]?.schemas.includes(schema) ?? false)
}

/**
 * Combines the page-wide filters with a widget's own.
 *
 * A widget filter is the more specific statement — "this tab is about
 * campaigns that are set", "this drawer is about one goal" — so it replaces a
 * page filter on the same dimension rather than ANDing into an impossible
 * condition that renders as an empty table.
 */
export function mergeWidgetFilters(
  pageFilters: WebDimensionFilter[],
  widgetFilters: WebDimensionFilter[] | undefined,
  schema: WebSchema
): WebDimensionFilter[] {
  const own = widgetFilters ?? []
  const ownDimensions = new Set(own.map((filter) => filter.dimension))
  return [
    ...filtersForSchema(pageFilters, schema).filter(
      (filter) => !ownDimensions.has(filter.dimension)
    ),
    ...own
  ]
}

/** Example values shown in the dimension picker's tooltips. */
export const DIMENSION_EXAMPLES: Record<string, string[]> = {
  referrer: ['https://google.com/search', 'https://news.ycombinator.com/'],
  referrer_domain: ['google.com', 'reddit.com'],
  referrer_path: ['/search', '/r/webdev'],
  is_direct: ['true', 'false'],
  utm_source: ['google', 'newsletter'],
  utm_medium: ['cpc', 'email'],
  utm_campaign: ['black-friday', 'launch'],
  utm_term: ['analytics software', 'web tracking'],
  utm_content: ['banner-top', 'text-link'],
  channel: ['google-ads', 'organic-search'],
  channel_group: ['search-paid', 'social-organic'],
  landing_page: ['https://example.com/pricing', 'https://example.com/'],
  landing_domain: ['example.com', 'blog.example.com'],
  landing_path: ['/pricing', '/blog/getting-started'],
  exit_path: ['/checkout', '/pricing'],
  page_path: ['/pricing', '/docs/install'],
  page_entry_type: ['navigation', 'spa'],
  device: ['desktop', 'mobile'],
  browser: ['Chrome', 'Safari'],
  browser_type: ['browser', 'in-app'],
  os: ['Mac OS', 'Windows'],
  connection_type: ['4g', 'wifi'],
  screen_width: ['1920', '390'],
  screen_height: ['1080', '844'],
  viewport_width: ['1440', '390'],
  viewport_height: ['900', '664'],
  country: ['US', 'FR'],
  region: ['California', 'Île-de-France'],
  city: ['San Francisco', 'Paris'],
  language: ['en-US', 'fr'],
  timezone: ['America/New_York', 'Europe/Paris'],
  hour_of_day: ['0', '14'],
  day_of_week: ['1', '7'],
  week_number: ['12', '48'],
  is_weekend: ['true', 'false'],
  sdk_version: ['1.0.0', '1.1.2'],
  goal_name: ['signup', 'purchase'],
  goal_type: ['purchase', 'lead'],
  goal_path: ['/thank-you', '/checkout/success'],
  contact_email: ['alice@example.com', 'bob@example.com']
}

/**
 * Display form of a dimension value.
 *
 * The locale and the empty label are passed in rather than resolved here so
 * the function stays pure and testable; the caller owns both. Shared by the
 * drill-down, the breakdown drawer and the summary tooltip so they cannot
 * disagree about how a weekday or a missing value reads.
 *
 * Note the strict comparisons: 0 and false are values a numeric or boolean
 * dimension really can take, and must not be mistaken for absent ones.
 */
export function formatDimensionValue(
  dimension: string,
  value: unknown,
  options: { emptyLabel: string; locale: string }
): string {
  if (value === null || value === undefined || value === '') return options.emptyLabel
  if (dimension === 'day_of_week' && typeof value === 'number') {
    return weekdayLabel(value, options.locale) ?? String(value)
  }
  return String(value)
}
