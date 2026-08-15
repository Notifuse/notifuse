import type { LLMTool } from '../../services/api/llm'
import {
  computeDateRange,
  computeComparisonRange,
  resolveRange,
  timeBucketColumn
} from './lib/dates'
import {
  DIMENSIONS,
  DIMENSION_EXAMPLES,
  dimensionsForSchema,
  getDimensionLabel
} from './lib/dimensions'
import { changePercent, toNumber } from './lib/format'
import {
  DATE_PRESETS,
  DIMENSION_FILTER_OPERATORS,
  METRIC_FILTER_OPERATORS,
  SESSION_METRICS,
  WEB_ANALYTICS_TABS,
  type ComparisonMode,
  type DatePreset,
  type DimensionFilterOperator,
  type Granularity,
  type MetricFilterOperator,
  type ResolvedRange,
  type WebAnalyticsTab,
  type WebDimensionFilter,
  type WebMetricFilter,
  type WebSchema
} from './lib/types'

/* ---------------------------------------------------------------------------
 * PROVIDER SCHEMA CONTRACT — read before editing any input_schema below.
 *
 * The same tool definitions are shipped verbatim to three providers, and each
 * one truncates them differently. The dangerous case is not the loud one:
 *
 *  - Anthropic reads ONLY `properties` and `required` from the schema root.
 *    Anything else written there (title, examples, additionalProperties, a
 *    root-level anyOf) is silently discarded.
 *  - Gemini converts the schema with `mapToGenaiSchema`
 *    (internal/service/llm_service_gemini.go:329-378), which understands ONLY
 *    `type`, `description`, `properties`, `required`, `items` and STRING
 *    `enum`. It returns *genai.Schema with NO error return: a oneOf, a $ref, a
 *    numeric enum or additionalProperties is SILENTLY DROPPED, and an ARRAY
 *    with no `items` is silently REPAIRED to a string schema (:373-376). The
 *    only local hard error is malformed JSON at unmarshal time
 *    (`jsonSchemaToGenaiSchema`, :313-318). So the failure mode is not a dead
 *    stream - it is that Gemini is shown a WEAKER schema than Anthropic and
 *    OpenAI are, a constraint expressed in an unsupported keyword is simply
 *    not enforced for that provider, and the divergence is invisible in
 *    review because Anthropic behaves.
 *  - OpenAI-compatible endpoints are the strict ones: they REJECT the whole
 *    request when an array property omits `items`.
 *
 * Therefore, in this file:
 *   * no oneOf, no anyOf, no allOf, no $ref, no additionalProperties
 *   * every enum is a list of STRINGS (numeric choices are described in prose)
 *   * every property of type "array" declares `items`
 *   * variants are sibling optional properties plus a discriminator value
 *     (period: "custom" alongside start_date/end_date), never a union
 *   * nested object items use only type/description/properties/required
 *
 * A schema is a hint, never a guarantee. Everything the handlers depend on is
 * re-validated in TypeScript below.
 * ------------------------------------------------------------------------- */

/* ---------------------------------------------------------------------------
 * BOUNDS. Every byte returned to the model is prompt tokens on every remaining
 * round of the turn, so results are capped here as well as in the hook's wire
 * encoder. Truncation is always announced: a partial list must never be
 * readable as a complete one.
 * ------------------------------------------------------------------------- */
export const MAX_BREAKDOWN_ROWS = 50
export const DEFAULT_BREAKDOWN_ROWS = 20
export const MAX_SERIES_ROWS = 200

/* ---------------------------------------------------------------------------
 * PII AT THE TOOL BOUNDARY. Refused as a grouping, as an order key and as a
 * filter member. The console exposes these to a human who asked, in their own
 * browser; a tool ships them into a third-party model provider's request body,
 * which is an egress path nobody consented to by granting web_analytics:read.
 *
 * contact_email: grouping by it is a bulk export of identified visitors'
 *   addresses. The aggregate question keeps working through the `contacts`
 *   measure (count_distinct), so only the roster is lost.
 * latitude / longitude: per-session coordinates. De-anonymising, and useless
 *   as an analytics grouping. They are also absent from the console catalog,
 *   so the membership check already rejects them; they are named here so the
 *   error is honest and the refusal survives a catalog addition.
 *
 * city / region / country stay available: coarse, standard geo reporting, and
 * blocking them would remove a whole capability for no real privacy gain.
 * ------------------------------------------------------------------------- */
export const BLOCKED_DIMENSIONS = new Set(['contact_email', 'latitude', 'longitude'])

/**
 * The OTHER three doors into the same data, none of which is a tool INPUT.
 *
 * `contact_email` is a first-class filter dimension (lib/dimensions.ts:120, on
 * ALL_SCHEMAS) offered by the console's own FilterBuilder, and page filters are
 * read straight out of the URL (context.tsx:201-204). So an operator - or a
 * pasted link - can put `contact_email equals someone@example.com` on screen,
 * and from there it reaches the model three times without any tool argument
 * naming it: the live-state block of the system prompt renders the active
 * filters verbatim, the insight battery applies them to every query and prints
 * them in its header, and a UI tool's acknowledgement describes the filter set
 * it just computed FROM that bar. Blocking the tool boundary alone leaves all
 * three open.
 *
 * Two shapes because the callers need different things: the prompt and the UI
 * acknowledgements must still tell the model that a narrowing filter is in
 * force (or it will misread every number on screen), while the summary must not
 * query or quote one at all.
 */
export const REDACTED_FILTER_VALUE = '[withheld: identifies an individual]'

/**
 * `Object.hasOwn` under another name: the console compiles against lib ES2020,
 * which predates the declaration, so the call does not typecheck even though
 * every runtime this ships to has it.
 *
 * The semantics are what matters and they are identical - an OWN-property check.
 * Every catalog membership test below goes through this rather than `in` or a
 * bare index, because `'toString' in known` is true on any object literal and
 * `DIMENSIONS['constructor']` returns something truthy: a prototype key would
 * otherwise sail straight through validation into a query.
 */
export function hasOwn(target: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(target, key)
}

/** For display: keep the dimension and operator, drop the values. */
export function redactBlockedFilterValues(filters: WebDimensionFilter[]): WebDimensionFilter[] {
  return filters.map((filter) =>
    BLOCKED_DIMENSIONS.has(filter.dimension)
      ? { ...filter, values: [REDACTED_FILTER_VALUE] }
      : filter
  )
}

/** For querying: the filter is removed entirely, before any query or header. */
export function dropBlockedFilters(filters: WebDimensionFilter[]): WebDimensionFilter[] {
  return filters.filter((filter) => !BLOCKED_DIMENSIONS.has(filter.dimension))
}

/**
 * The order key is the third way a blocked dimension gets into a query, and the
 * only one no other helper covers: it is neither a `dimensions` member nor a
 * filter member, so nothing else validates it. It must be a measure the query
 * selects or a dimension it groups by - anything else is either PII, a typo, or
 * a column the engine will reject anyway.
 */
export function assertOrderKey(raw: unknown, measures: string[], dimensions: string[]): string {
  const key = typeof raw === 'string' ? raw.trim() : ''
  if (!key) fail('an order key is required')
  if (BLOCKED_DIMENSIONS.has(key)) {
    fail(`cannot order by "${key}": it identifies individual visitors`)
  }
  if (!measures.includes(key) && !dimensions.includes(key)) {
    fail(
      `cannot order by "${key}": order_by must name one of the measures or dimensions this query ` +
        `selects (${[...measures, ...dimensions].join(', ')})`
    )
  }
  return key
}

/**
 * Measures per schema. Mirrors internal/domain/web_analytics_schemas.go; the
 * console has no measure catalog of its own, and the model needs one to avoid
 * inventing `conversion_rate`, which the engine rejects outright.
 */
export const MEASURES_BY_SCHEMA: Record<WebSchema, Record<string, string>> = {
  web_sessions: {
    sessions: 'Number of sessions',
    median_duration: 'Median engaged time per session, in seconds (the "TimeScore")',
    avg_scroll: 'Average maximum scroll depth, in percent',
    median_scroll: 'Median maximum scroll depth, in percent',
    bounce_rate: 'Share of sessions below the workspace engaged-time threshold, in percent',
    pageviews: 'Total pageviews across sessions',
    pages_per_session: 'Average pageviews per session',
    median_page_duration: 'Median time on a page, in seconds',
    goal_conversions: 'Sessions that fired at least one goal',
    goal_value: 'Sum of goal value attributed to sessions',
    contacts: 'Distinct identified contacts'
  },
  web_pages: {
    page_count: 'Page views',
    unique_pages: 'Distinct page paths',
    page_duration: 'Median time on page, in seconds',
    page_scroll: 'Median scroll depth, in percent',
    landing_page_count: 'Views that were the first page of a session',
    exit_page_count: 'Views that were the last page of a session',
    exit_rate: 'Share of views that ended the session, in percent'
  },
  web_goals: {
    goals: 'Number of goal conversions',
    sum_goal_value: 'Total goal value',
    avg_goal_value: 'Average goal value',
    median_goal_value: 'Median goal value',
    unique_sessions_with_goals: 'Distinct sessions that converted'
  }
}

export const SCHEMAS: WebSchema[] = ['web_sessions', 'web_pages', 'web_goals']

/** Presets the model may name, plus the two meta-values the handlers resolve. */
const PERIOD_CHOICES: string[] = [
  'current',
  ...DATE_PRESETS.filter((preset) => preset !== 'custom'),
  'custom'
]

/**
 * Tabs navigate_to_tab may open: the data tabs, and deliberately NOT `filters`
 * or `annotations`.
 *
 * The assistant is HIDDEN on both (shouldHideAssistant in
 * WebAnalyticsAIAssistant.tsx, passed as `hidden`, which sets display:'none' on the
 * whole panel at AIAssistantChat.tsx:176). Offering either value would let the model
 * honour "show me the attribution rules" by making itself disappear mid-turn, with the
 * continuation round then writing its answer into an element nobody can see. The same
 * reasoning that hides the assistant there - every tool it owns would mutate state the
 * operator cannot see, and neither tab reads a report at all - says it must not send
 * the operator there either.
 *
 * Derived from WEB_ANALYTICS_TABS rather than written out, so a tab added later is
 * navigable by default; the two exclusions are cross-checked against
 * shouldHideAssistant by a test, which is why the rules are defined independently
 * instead of one in terms of the other.
 */
export const NAVIGABLE_TABS: WebAnalyticsTab[] = WEB_ANALYTICS_TABS.filter(
  (tab) => tab !== 'filters' && tab !== 'annotations'
)

export const WEB_TOOL_NAMES = {
  QUERY: 'query_web_analytics',
  COMPARE: 'compare_periods',
  SUMMARIZE: 'summarize_period',
  CATALOG: 'list_dimensions_and_measures',
  SET_PERIOD: 'set_dashboard_period',
  SET_FILTERS: 'set_dashboard_filters',
  SET_REPORT: 'set_explore_report',
  NAVIGATE: 'navigate_to_tab'
} as const

/* ===========================================================================
 * SHARED SCHEMA FRAGMENTS
 * ========================================================================= */

const PERIOD_PROPERTIES = {
  period: {
    type: 'string',
    enum: PERIOD_CHOICES,
    description:
      'Which period to report on. "current" (the default) means the period the dashboard is currently showing, including a custom range the user picked themselves. Every "previous_N_days" preset ends yesterday, so it never mixes complete days with today\'s partial one. Use "custom" together with start_date and end_date only when the user names explicit dates.'
  },
  start_date: {
    type: 'string',
    description: 'First day of the range as YYYY-MM-DD. Only read when period is "custom".'
  },
  end_date: {
    type: 'string',
    description: 'Last day of the range as YYYY-MM-DD, inclusive. Only read when period is "custom".'
  }
} as const

const FILTERS_PROPERTY = {
  type: 'array',
  description:
    'Conditions every row must satisfy. All conditions are combined with AND. Values are always strings, including numbers and booleans: booleans are "true" or "false", and "not set" is the empty string "" with the equals operator.',
  items: {
    type: 'object',
    properties: {
      dimension: {
        type: 'string',
        description:
          'Dimension name. Must be supported by the schema being queried; call list_dimensions_and_measures when unsure.'
      },
      operator: {
        type: 'string',
        enum: [...DIMENSION_FILTER_OPERATORS],
        description:
          'How to compare. Use "in"/"notIn" with several values, "isEmpty"/"isNotEmpty" with an empty values array.'
      },
      values: {
        type: 'array',
        items: { type: 'string' },
        description:
          'Values to compare against. One entry for most operators, several for "in"/"notIn", none for "isEmpty"/"isNotEmpty".'
      }
    },
    required: ['dimension', 'operator', 'values']
  }
} as const

/**
 * COMPARISON TOKENS ARE DELIBERATELY NOT THE ComparisonMode VALUES.
 *
 * `previous_year` is legal in BOTH enums with two different meanings: as a
 * PERIOD it selects last year as the window being reported on; as a COMPARISON
 * it means "the same calendar dates one year earlier". compare_periods exposes
 * both properties side by side, so a model that reads "previous_year" in one
 * and writes it into the other produces a plausible, wrong report with no
 * error anywhere. Renaming the comparison tokens makes the confusion
 * unexpressible; `parseComparisonMode` maps them back to the ComparisonMode the
 * dashboard's own arithmetic takes.
 */
export const TOOL_COMPARISON_CHOICES = ['vs_preceding_window', 'vs_same_dates_last_year'] as const

const TOOL_COMPARISON_TO_MODE: Record<string, ComparisonMode> = {
  vs_preceding_window: 'previous_period',
  vs_same_dates_last_year: 'previous_year',
  // Only set_dashboard_period offers "off" in its enum; the data tools do not,
  // because a comparison tool with the comparison switched off is not a query.
  off: 'none'
}

export function parseComparisonMode(raw: unknown, fallback: ComparisonMode): ComparisonMode {
  if (raw === undefined || raw === null) return fallback
  const token = String(raw)
  const mode = hasOwn(TOOL_COMPARISON_TO_MODE, token)
    ? TOOL_COMPARISON_TO_MODE[token]
    : undefined
  if (!mode) {
    fail(
      `unknown comparison "${String(raw)}"; expected one of ${TOOL_COMPARISON_CHOICES.join(', ')} ` +
        `(note that "previous_year" is a PERIOD, not a comparison)`
    )
  }
  return mode
}

const COMPARISON_PROPERTY = {
  type: 'string',
  enum: [...TOOL_COMPARISON_CHOICES],
  description:
    'Which window to compare the period against. "vs_preceding_window" is the period immediately before it, of the same length; "vs_same_dates_last_year" is the same calendar dates one year earlier. Defaults to vs_preceding_window. This is NOT the period being reported on - that is the "period" property.'
} as const

/**
 * The escape hatch for the one legitimate cross-segment question. Default false:
 * a data tool that silently ignored the filter bar would report numbers that
 * contradict the chart three inches away, in the same conversation.
 */
const IGNORE_DASHBOARD_FILTERS_PROPERTY = {
  type: 'boolean',
  description:
    'Set to true ONLY to answer a question about the whole site while the dashboard is filtered to a segment ("how does mobile compare to everyone?"). Leave it out to stay consistent with what is on screen.'
} as const

/**
 * The granularity vocabulary, written as a Record KEYED BY the Granularity type
 * so a bucket added to lib/types.ts fails to compile here instead of silently
 * becoming a value the schema never offers and the validator always refuses.
 *
 * It feeds the schema enum and `parseGranularity` from the same place: the
 * granularity flows into the engine query AND into the name of the output
 * column the model is told to read, so a value the two disagree about produces
 * a table whose bucket column is missing from every row.
 */
const GRANULARITY_CHOICES: Record<Granularity, true> = {
  hour: true,
  day: true,
  week: true,
  month: true,
  year: true
}

export const GRANULARITIES = Object.keys(GRANULARITY_CHOICES) as Granularity[]

const METRIC_FILTERS_PROPERTY = {
  type: 'array',
  description:
    'Thresholds applied to the aggregated numbers after grouping, for example "only pages with more than 100 views". The metric must be one of the schema\'s measures.',
  items: {
    type: 'object',
    properties: {
      metric: { type: 'string', description: 'A measure of the schema being queried.' },
      operator: { type: 'string', enum: [...METRIC_FILTER_OPERATORS] },
      value: { type: 'number', description: 'The threshold to compare the measure against.' }
    },
    required: ['metric', 'operator', 'value']
  }
} as const

/* ===========================================================================
 * TOOL DEFINITIONS
 * ========================================================================= */

/** DATA — the workhorse. Totals, breakdowns and time series in one shape. */
export const QUERY_WEB_ANALYTICS_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.QUERY,
  description:
    'Run one web analytics query and read the numbers back. This is how you answer any question about traffic, engagement, pages or goals: call it, wait for the rows, then explain them in prose. ' +
    'Three shapes come out of the same call. Omit "dimensions" and "granularity" for a single total row ("how many sessions last week"). Give "dimensions" for a ranked breakdown ("top referrers"). Give "granularity" for a time series ("sessions per day"). Giving both produces one row per bucket per value, which grows fast - prefer one or the other. ' +
    'Pick the schema by what you are counting: web_sessions for visits, engagement, traffic sources, devices and geography; web_pages for individual page performance; web_goals for conversions. The three schemas do not share dimensions - web_pages in particular has only page-level ones, and asking it for utm_source fails the whole query. ' +
    'The filters currently applied to the dashboard are applied to this query too, so your numbers match the charts the operator is looking at. Any filter you pass on the same dimension replaces the dashboard one; set ignore_dashboard_filters to true only when the question is explicitly about the site as a whole. Either way the filter set actually used is printed in the first line of the result. ' +
    'Results are capped and the cap is stated in the output; ask for a narrower period or a smaller limit rather than assuming a truncated list is complete. A value that reads as an empty string means the dimension was not set on that session.',
  input_schema: {
    type: 'object',
    properties: {
      schema: {
        type: 'string',
        enum: SCHEMAS,
        description:
          'web_sessions for visits and engagement, web_pages for page-level performance, web_goals for conversions.'
      },
      measures: {
        type: 'array',
        items: { type: 'string' },
        description:
          'The numbers to compute, for example ["sessions","bounce_rate"]. Must belong to the chosen schema. There is no conversion-rate measure: divide goal_conversions by sessions yourself.'
      },
      dimensions: {
        type: 'array',
        items: { type: 'string' },
        description:
          'Group the numbers by these, for example ["referrer_domain"]. Omit for a single total row. Two dimensions produce a cross-tabulation; more than two is rarely readable.'
      },
      ...PERIOD_PROPERTIES,
      granularity: {
        type: 'string',
        enum: [...GRANULARITIES],
        description:
          'Set this to get one row per time bucket instead of an aggregate. Choose a bucket that yields a readable number of rows: hour for a day or two, day up to a few months, week or month beyond that.'
      },
      filters: FILTERS_PROPERTY,
      ignore_dashboard_filters: IGNORE_DASHBOARD_FILTERS_PROPERTY,
      metric_filters: METRIC_FILTERS_PROPERTY,
      min_sessions: {
        type: 'number',
        description:
          'Drop breakdown rows below this many sessions, to keep long-tail noise out of a ranking. Only applies to web_sessions with at least one dimension.'
      },
      order_by: {
        type: 'string',
        description:
          'Measure or dimension to sort by. Defaults to the first measure, descending, which is what a ranking usually wants.'
      },
      order_direction: { type: 'string', enum: ['asc', 'desc'] },
      limit: {
        type: 'number',
        description: `Maximum breakdown rows to return. Defaults to ${DEFAULT_BREAKDOWN_ROWS}, capped at ${MAX_BREAKDOWN_ROWS}.`
      }
    },
    required: ['schema', 'measures']
  }
}

/** DATA — period over period, with the comparison window computed for you. */
export const COMPARE_PERIODS_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.COMPARE,
  description:
    'Run the same query over a period and over the period it should be compared against, and read both back joined together with the percentage change. Use this for any "is it up or down", "did it drop", "how does this week compare" question instead of issuing two separate queries - the comparison window is computed here, so you never have to work out where the previous period starts. ' +
    '"vs_preceding_window" shifts back by the period\'s own length so the two windows abut without overlapping; "vs_same_dates_last_year" keeps the same calendar dates one year earlier. Note that these are the COMPARISON choices, and they are deliberately named differently from the "previous_year" PERIOD preset, which selects last year as the period being reported on - the two are not the same thing and appear side by side in this call. Omit "dimensions" to compare totals, or give one to see which entries moved. ' +
    'The dashboard\'s active filters apply to both windows unless you set ignore_dashboard_filters; the filter set used is printed in the result header.',
  input_schema: {
    type: 'object',
    properties: {
      schema: { type: 'string', enum: SCHEMAS, description: 'Which schema to query.' },
      measures: {
        type: 'array',
        items: { type: 'string' },
        description: 'The numbers to compare. Must belong to the chosen schema.'
      },
      dimensions: {
        type: 'array',
        items: { type: 'string' },
        description:
          'Optional single grouping. Rows are joined on this dimension\'s value; entries present only in the earlier period are dropped, because the report says what happened now and then how it moved.'
      },
      ...PERIOD_PROPERTIES,
      comparison: COMPARISON_PROPERTY,
      filters: FILTERS_PROPERTY,
      ignore_dashboard_filters: IGNORE_DASHBOARD_FILTERS_PROPERTY,
      limit: {
        type: 'number',
        description: `Maximum rows. Defaults to ${DEFAULT_BREAKDOWN_ROWS}, capped at ${MAX_BREAKDOWN_ROWS}.`
      }
    },
    required: ['schema', 'measures']
  }
}

/** DATA — the whole picture in one call, for summaries and insights. */
export const SUMMARIZE_PERIOD_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.SUMMARIZE,
  description:
    'Run the standard period report for the dashboard the user is currently looking at: headline totals versus the comparison period, the session time series, goal conversions, and the top breakdowns by channel, landing page, country, device, referrer and campaign. ' +
    'The period, timezone and active filters come from the dashboard - you do not choose them. ' +
    'Call this first for any question about what happened, what changed, or how the period went, then use query_web_analytics only for what the summary does not already answer. ' +
    'Sections that return no data are reported as empty rather than omitted, so an empty goals section means the site records no conversions, not that the call failed.',
  input_schema: {
    type: 'object',
    properties: {
      comparison: {
        type: 'string',
        enum: [...TOOL_COMPARISON_CHOICES],
        description:
          'Which window to compare against: "vs_preceding_window" for the period immediately before, "vs_same_dates_last_year" for the same calendar dates a year earlier. Omit to use whatever the dashboard is comparing to, or the preceding window when the dashboard has comparison switched off.'
      }
    },
    required: []
  }
}

/** DATA — static catalog, no network call. The recovery path for a bad query. */
export const LIST_DIMENSIONS_AND_MEASURES_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.CATALOG,
  description:
    'List the measures and dimensions a schema actually supports, with their types and example values. Call this when a query failed because a name was not recognised, when the user asks what can be measured or sliced, or before grouping by anything outside the common set. ' +
    'It is also the only way to learn what this workspace\'s custom dimensions are named, since those labels are configured per workspace. It reads no data and costs nothing.',
  input_schema: {
    type: 'object',
    properties: {
      schema: {
        type: 'string',
        enum: SCHEMAS,
        description: 'Restrict the listing to one schema. Omit to list all three.'
      }
    },
    required: []
  }
}

/** UI — period, custom range, comparison and timezone in one write. */
export const SET_DASHBOARD_PERIOD_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.SET_PERIOD,
  description:
    'Change the period the dashboard is showing, and optionally its comparison window and timezone, in a single update. Use this when the user wants the screen itself to change ("show me last 30 days"), not merely to be told a number - asking a question does not require changing the dashboard. ' +
    'Set everything you want changed in one call: several separate calls in the same turn are merged, but a call that omits a field leaves that field alone.',
  input_schema: {
    type: 'object',
    properties: {
      period: {
        type: 'string',
        enum: PERIOD_CHOICES.filter((choice) => choice !== 'current'),
        description:
          'The period to display. Use "custom" together with start_date and end_date for explicit dates.'
      },
      start_date: { type: 'string', description: 'First day as YYYY-MM-DD, when period is "custom".' },
      end_date: {
        type: 'string',
        description: 'Last day as YYYY-MM-DD, inclusive, when period is "custom".'
      },
      comparison: {
        type: 'string',
        enum: [...TOOL_COMPARISON_CHOICES, 'off'],
        description:
          'Comparison window shown beside every metric: the preceding window of the same length, the same calendar dates last year, or "off" for no comparison. Same vocabulary as compare_periods, and again distinct from the "period" property above.'
      },
      timezone: {
        type: 'string',
        description:
          'IANA timezone the report is read in, for example "Europe/Paris". Only set this when the user asks for it.'
      }
    },
    required: []
  }
}

/** UI — the page-wide filter bar. */
export const SET_DASHBOARD_FILTERS_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.SET_FILTERS,
  description:
    'Change the filters applied to every widget on the page. "replace" swaps the whole filter bar for what you pass, "add" keeps the existing filters and adds yours (replacing any filter on the same dimension), "clear" removes them all. ' +
    'Use this to narrow what the user is looking at ("just show mobile"). It does not answer a question - pass filters to query_web_analytics instead when you only need the number.',
  input_schema: {
    type: 'object',
    properties: {
      mode: {
        type: 'string',
        enum: ['replace', 'add', 'clear'],
        description: 'How to combine with the filters already applied. Defaults to "replace".'
      },
      filters: FILTERS_PROPERTY
    },
    required: []
  }
}

/** UI — opens Explore with a full report configured, in one navigation. */
export const SET_EXPLORE_REPORT_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.SET_REPORT,
  description:
    'Open the Explore section with a drill-down report built for the user: the dimensions to break down by, in order from broadest to narrowest, plus optional filters and a minimum session threshold. ' +
    'Use this when the user wants to explore something themselves rather than be given one number - "let me see traffic by source and then by landing page". The section, the dimensions, the filters and the threshold are all applied in a single navigation.',
  input_schema: {
    type: 'object',
    properties: {
      dimensions: {
        type: 'array',
        items: { type: 'string' },
        description:
          'Drill-down levels, broadest first, for example ["channel_group","landing_path"]. One to four; more is unreadable. Must be dimensions of web_sessions.'
      },
      filters: FILTERS_PROPERTY,
      min_sessions: {
        type: 'number',
        description: 'Hide rows below this many sessions, to keep the long tail out of the report.'
      }
    },
    required: ['dimensions']
  }
}

/** UI — section switch. */
export const NAVIGATE_TO_TAB_TOOL: LLMTool = {
  name: WEB_TOOL_NAMES.NAVIGATE,
  description:
    'Switch the web analytics section the user is looking at: "dashboard" for the standard overview, "explore" for the custom drill-down report, "goals" for conversions. Prefer set_explore_report when you also want to configure the report.',
  input_schema: {
    type: 'object',
    properties: {
      tab: { type: 'string', enum: [...NAVIGABLE_TABS], description: 'The section to open.' }
    },
    required: ['tab']
  }
}

export const WEB_ANALYTICS_AI_TOOLS: LLMTool[] = [
  QUERY_WEB_ANALYTICS_TOOL,
  COMPARE_PERIODS_TOOL,
  SUMMARIZE_PERIOD_TOOL,
  LIST_DIMENSIONS_AND_MEASURES_TOOL,
  SET_DASHBOARD_PERIOD_TOOL,
  SET_DASHBOARD_FILTERS_TOOL,
  SET_EXPLORE_REPORT_TOOL,
  NAVIGATE_TO_TAB_TOOL
]

/* ===========================================================================
 * PURE HELPERS — exported for direct unit testing
 * ========================================================================= */

export class ToolInputError extends Error {}

export function fail(message: string): never {
  throw new ToolInputError(message)
}

/**
 * UI state a tool of THIS round has already asked for, which the page has not
 * committed yet.
 *
 * Every tool of a round is dispatched synchronously against one frozen snapshot
 * of the page, and the system prompt actively encourages batching a UI write
 * with the query that reads it: `set_dashboard_period` + `query_web_analytics`
 * in the same round leaves the query reading the OLD window while its sibling
 * acknowledgement announces the new one. The overlay is how a handler sees what
 * its siblings asked for, and it is read through a GETTER at handler-execution
 * time - snapshotting it would reintroduce the very staleness it exists to fix.
 *
 * Every field is optional and means "unchanged this round"; an absent key is
 * never a request to clear.
 */
export interface PendingUiState {
  period?: DatePreset
  customStart?: string
  customEnd?: string
  comparison?: ComparisonMode
  filters?: WebDimensionFilter[]
  minSessions?: number
  dimensions?: string[]
  tab?: WebAnalyticsTab
}

/** What every date-taking handler needs from the page it is embedded in. */
export interface ToolDateContext {
  timezone: string
  workspaceCreatedAt?: string
  currentPeriod: DatePreset
  currentCustomStart?: string
  currentCustomEnd?: string
  /** Already resolved by the page, so "current" honours a user-picked range. */
  currentResolved: ResolvedRange
}

/**
 * The date context as of the pending overlay, for a round that changes the
 * period and reads it in the same breath.
 *
 * `currentResolved` is pre-baked by the page, so handing it back for
 * period:"current" answers with the window the dashboard showed BEFORE the
 * sibling tool ran. Recomputing here - through the same two functions the
 * dashboard itself uses - is what makes the two tools of one round agree.
 * With nothing pending the context is returned unchanged, so the ordinary case
 * still answers with the exact range the page resolved.
 */
export function withPendingDates(
  context: ToolDateContext,
  pending: PendingUiState
): ToolDateContext {
  if (!pending.period) return context
  const custom =
    pending.period === 'custom'
      ? {
          start: pending.customStart ?? context.currentCustomStart ?? '',
          end: pending.customEnd ?? context.currentCustomEnd ?? ''
        }
      : undefined
  // A custom period with no dates would fall back to "the last 7 days" inside
  // computeDateRange, which is a window nobody asked for; keeping what the page
  // resolved is the honest answer until the navigation lands.
  if (custom && (!custom.start || !custom.end)) return context
  return {
    ...context,
    currentPeriod: pending.period,
    currentCustomStart: custom?.start,
    currentCustomEnd: custom?.end,
    currentResolved: resolveRange(
      computeDateRange(pending.period, context.timezone, custom, context.workspaceCreatedAt),
      context.timezone
    )
  }
}

export function resolveSchema(raw: unknown): WebSchema {
  if (SCHEMAS.includes(raw as WebSchema)) return raw as WebSchema
  return fail(`unknown schema "${String(raw)}"; expected one of ${SCHEMAS.join(', ')}`)
}

/**
 * Compiles what the model named into concrete bounds.
 *
 * The model only ever names a preset or two plain YYYY-MM-DD dates. Both of the
 * engine's date encodings - bare calendar days on a bucketed time dimension and
 * absolute instants on a range filter - are produced here by the same functions
 * the dashboard uses, so a tool answer and a widget cover exactly the same span
 * and the model never has to know that two encodings exist.
 */
export function resolveToolRange(
  context: ToolDateContext,
  input: { period?: unknown; start_date?: unknown; end_date?: unknown }
): { range: ResolvedRange; preset: DatePreset; custom?: { start: string; end: string }; label: string } {
  const raw = typeof input.period === 'string' ? input.period : 'current'

  if (raw === 'current') {
    return {
      range: context.currentResolved,
      preset: context.currentPeriod,
      custom:
        context.currentCustomStart && context.currentCustomEnd
          ? { start: context.currentCustomStart, end: context.currentCustomEnd }
          : undefined,
      label: `${context.currentResolved.startDay}..${context.currentResolved.endDay}`
    }
  }

  if (raw === 'custom') {
    const start = typeof input.start_date === 'string' ? input.start_date.trim() : ''
    const end = typeof input.end_date === 'string' ? input.end_date.trim() : ''
    if (!/^\d{4}-\d{2}-\d{2}$/.test(start) || !/^\d{4}-\d{2}-\d{2}$/.test(end)) {
      fail('period "custom" needs start_date and end_date as YYYY-MM-DD')
    }
    if (end < start) fail(`end_date ${end} is before start_date ${start}`)
    const range = resolveRange(
      computeDateRange('custom', context.timezone, { start, end }, context.workspaceCreatedAt),
      context.timezone
    )
    return { range, preset: 'custom', custom: { start, end }, label: `${range.startDay}..${range.endDay}` }
  }

  if (!DATE_PRESETS.includes(raw as DatePreset)) {
    fail(`unknown period "${raw}"; expected one of ${PERIOD_CHOICES.join(', ')}`)
  }
  const preset = raw as DatePreset
  const range = resolveRange(
    computeDateRange(preset, context.timezone, undefined, context.workspaceCreatedAt),
    context.timezone
  )
  return { range, preset, label: `${preset} (${range.startDay}..${range.endDay})` }
}

/**
 * The comparison window for a resolved tool range, using the dashboard's own
 * arithmetic - and null whenever that arithmetic would invent a window that
 * cannot hold data.
 *
 * computeComparisonRange (lib/dates.ts:120-130) returns null for mode "none"
 * and otherwise ALWAYS subtracts, so it happily produces a window entirely
 * before the workspace existed. Two ways that happens: `all_time` already
 * starts at the first session, so anything preceding it is empty by
 * construction; and any early period compared year-over-year lands before the
 * first session too. Querying it is not merely wasteful - every change cell
 * comes back blank, which reads as "no change" when the truth is "there is
 * nothing to compare against". Returning null lets the caller say so.
 */
export function resolveComparisonRange(
  context: ToolDateContext,
  resolved: { preset: DatePreset; custom?: { start: string; end: string } },
  mode: ComparisonMode
): ResolvedRange | null {
  if (mode === 'none') return null
  if (resolved.preset === 'all_time') return null
  const currentRaw = computeDateRange(
    resolved.preset,
    context.timezone,
    resolved.custom,
    context.workspaceCreatedAt
  )
  const previousRaw = computeComparisonRange(currentRaw, mode)
  if (!previousRaw) return null
  // Deliberately NOT also refused when the window predates the workspace record.
  // `workspaceCreatedAt` is when the ROW was written, not when the earliest session
  // happened, and analytics data is routinely older than that: a seeded demo, an
  // import, a historical backfill. Refusing on it told an operator with 7,764
  // sessions in the preceding month that "nothing precedes this range", while the
  // dashboard beside them - whose computeComparisonRange applies no such test -
  // charted that same comparison happily. A window with genuinely no data reports
  // zeroes, which is what the dashboard shows and what the operator can read.
  return resolveRange(previousRaw, context.timezone)
}

/** Rejects a dimension that is unknown, wrong-schema, or withheld as PII. */
export function assertQueryableDimension(name: unknown, schema: WebSchema): string {
  const dimension = typeof name === 'string' ? name.trim() : ''
  if (!dimension) fail('a dimension name is required')
  if (BLOCKED_DIMENSIONS.has(dimension)) {
    fail(
      `dimension "${dimension}" is not available to the assistant because it identifies individual visitors; ` +
        `use the "contacts" measure for a count, or tell the user to open the Explore section to inspect it themselves`
    )
  }
  // hasOwn, not `DIMENSIONS[name]`: DIMENSIONS is a plain object from
  // Object.fromEntries, so "constructor", "toString" and "__proto__" all index to
  // something truthy and would sail through a bare lookup into buildWebQuery.
  const info = hasOwn(DIMENSIONS, dimension) ? DIMENSIONS[dimension] : undefined
  if (!info) fail(`unknown dimension "${dimension}"; call ${WEB_TOOL_NAMES.CATALOG} for the list`)
  if (!info.schemas.includes(schema)) {
    fail(
      `dimension "${dimension}" does not exist on schema ${schema}; it is available on ${info.schemas.join(', ')}`
    )
  }
  return dimension
}

/** Catalog-membership check for a filter that is not bound to one schema. */
export function assertCatalogDimension(name: unknown): string {
  const dimension = typeof name === 'string' ? name.trim() : ''
  if (BLOCKED_DIMENSIONS.has(dimension)) {
    fail(
      `dimension "${dimension}" is not available to the assistant because it identifies individual visitors`
    )
  }
  if (!hasOwn(DIMENSIONS, dimension)) {
    fail(`unknown dimension "${dimension}"; call ${WEB_TOOL_NAMES.CATALOG} for the list`)
  }
  return dimension
}

/**
 * The schemas every widget of the dashboard reads. The filter bar is page-wide,
 * so a filter it carries has to be expressible on ALL of them.
 */
export const DASHBOARD_FILTER_SCHEMAS: WebSchema[] = ['web_sessions', 'web_goals']

/**
 * Scope check for the page-wide filter bar.
 *
 * A dimension the visible widgets cannot use (`page_path`, which lives only on
 * web_pages) is not an error anywhere downstream: every widget simply drops it,
 * the screen does not change, and the acknowledgement still reports the filter
 * as applied. Refusing it here with the usable scope named is what lets the
 * model correct itself instead of narrating a filter nobody is subject to.
 *
 * assertCatalogDimension runs FIRST so a withheld name is still refused as PII
 * rather than as the wrong schema - the reason matters to what the model does
 * next, and to what it repeats to the operator.
 */
export function assertDashboardFilterDimension(name: unknown): string {
  const dimension = assertCatalogDimension(name)
  const info = DIMENSIONS[dimension]
  if (!DASHBOARD_FILTER_SCHEMAS.some((schema) => info.schemas.includes(schema))) {
    fail(
      `dimension "${dimension}" cannot be applied to the dashboard filter bar: every widget on the ` +
        `page reads ${DASHBOARD_FILTER_SCHEMAS.join(' or ')}, and "${dimension}" exists only on ` +
        `${info.schemas.join(', ')}. Usable here are the dimensions ${WEB_TOOL_NAMES.CATALOG} lists ` +
        `for ${DASHBOARD_FILTER_SCHEMAS.join(' or ')}; to narrow page-level data instead, pass the ` +
        `filter to ${WEB_TOOL_NAMES.QUERY} with the schema that carries it`
    )
  }
  return dimension
}

/** Rejects a bucket the engine has no column for; undefined means "no bucketing". */
export function parseGranularity(raw: unknown): Granularity | undefined {
  if (raw === undefined || raw === null) return undefined
  const value = typeof raw === 'string' ? raw.trim() : raw
  // An omitted granularity is the aggregate query, not an error; anything else
  // that is not a bucket name is refused rather than quietly dropped, since
  // dropping it answers a different question from the one that was asked.
  if (value === '') return undefined
  // hasOwn, not a bare index: GRANULARITY_CHOICES is a plain object, so
  // "constructor" and "toString" both index to something truthy.
  if (typeof value !== 'string' || !hasOwn(GRANULARITY_CHOICES, value)) {
    fail(`unknown granularity "${String(raw)}"; expected one of ${GRANULARITIES.join(', ')}`)
  }
  return value as Granularity
}

export function assertMeasures(raw: unknown, schema: WebSchema): string[] {
  const list = Array.isArray(raw) ? raw.filter((m): m is string => typeof m === 'string') : []
  if (list.length === 0) fail('at least one measure is required')
  const known = MEASURES_BY_SCHEMA[schema]
  for (const measure of list) {
    // hasOwn, not `in`: `'toString' in known` is true on any object literal.
    if (!hasOwn(known, measure)) {
      fail(
        `unknown measure "${measure}" on schema ${schema}; available: ${Object.keys(known).join(', ')}` +
          (measure.includes('rate') || measure.includes('conversion')
            ? '. There is no conversion-rate measure: divide goal_conversions by sessions yourself.'
            : '')
      )
    }
  }
  return list
}

/**
 * Parses model-supplied filters.
 *
 * Note what is unreachable by construction: the operator list is the console's
 * own, which has no `set`/`notSet` - and those two would be wrong on every web
 * schema anyway, since dimensions are NOT NULL DEFAULT '' and contact_email is
 * COALESCEd, so "is empty" is an equality with the empty string. buildWebQuery
 * performs that mapping for isEmpty/isNotEmpty.
 */
export function parseFilters(raw: unknown, schema: WebSchema | null): WebDimensionFilter[] {
  if (raw === undefined || raw === null) return []
  if (!Array.isArray(raw)) fail('filters must be an array')
  return raw.map((entry) => {
    const record = (entry ?? {}) as Record<string, unknown>
    const dimension =
      schema === null
        ? assertCatalogDimension(record.dimension)
        : assertQueryableDimension(record.dimension, schema)
    const operator = record.operator as DimensionFilterOperator
    if (!DIMENSION_FILTER_OPERATORS.includes(operator)) {
      fail(
        `unknown filter operator "${String(record.operator)}"; expected one of ${DIMENSION_FILTER_OPERATORS.join(', ')}`
      )
    }
    const values = Array.isArray(record.values) ? record.values.map((v) => String(v)) : []
    if (operator !== 'isEmpty' && operator !== 'isNotEmpty' && values.length === 0) {
      fail(`filter on "${dimension}" with operator "${operator}" needs at least one value`)
    }
    return { dimension, operator, values }
  })
}

export function parseMetricFilters(raw: unknown, schema: WebSchema): WebMetricFilter[] {
  if (raw === undefined || raw === null) return []
  if (!Array.isArray(raw)) fail('metric_filters must be an array')
  const known = MEASURES_BY_SCHEMA[schema]
  return raw.map((entry) => {
    const record = (entry ?? {}) as Record<string, unknown>
    const metric = String(record.metric ?? '')
    if (!hasOwn(known, metric)) {
      fail(`metric filter on unknown measure "${metric}"; available: ${Object.keys(known).join(', ')}`)
    }
    const operator = record.operator as MetricFilterOperator
    if (!METRIC_FILTER_OPERATORS.includes(operator)) {
      fail(`metric filter operator must be one of ${METRIC_FILTER_OPERATORS.join(', ')}`)
    }
    // The threshold is checked as hard as the metric and the operator are.
    // `required` in the schema is a hint no provider is obliged to enforce, and
    // toNumber maps both an absent value and "one hundred" to 0: the query then
    // compiles to HAVING sessions >= 0, which removes nothing at all, while the
    // model is told the threshold applied and reports the long tail as filtered.
    // A numeric string is still accepted - that is how models usually send one.
    const raw = record.value
    const value =
      typeof raw === 'number'
        ? raw
        : typeof raw === 'string' && raw.trim() !== ''
          ? Number(raw.trim())
          : Number.NaN
    if (!Number.isFinite(value)) {
      fail(
        `metric filter on "${metric}" needs a numeric value; received "${String(raw)}". ` +
          `Pass the threshold as a number, for example {"metric":"${metric}","operator":"${operator}","value":100}`
      )
    }
    return { metric, operator, values: [value] }
  })
}

export function clampLimit(raw: unknown): number {
  const value = Math.floor(toNumber(raw))
  if (!Number.isFinite(value) || value <= 0) return DEFAULT_BREAKDOWN_ROWS
  return Math.min(value, MAX_BREAKDOWN_ROWS)
}

/** Compact numeric rendering: models read "1240" and "42.5", not "1240.0000". */
export function renderCell(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'number') {
    return Number.isInteger(value) ? String(value) : String(Math.round(value * 100) / 100)
  }
  const text = String(value)
  return /[",\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}

/**
 * A change cell as printed, everywhere one is printed: compare_periods' own tables
 * and every `_change` column of the insight battery.
 *
 * changePercent is lib/format.ts:92 - the same arithmetic the dashboard's own change
 * badges use, so a percentage in a tool result and one on screen cannot disagree. Two
 * things it does that a raw quotient does not: it rounds to ONE decimal (the raw value
 * is -41.66666666666667, ~15 characters of noise in every change cell of every row,
 * against payload budgets counted in characters), and it prints an EMPTY cell for a
 * zero baseline, because "0" there reads as "no change" when the truth is "no previous
 * data" - changePercent itself returns 0 for that case.
 */
export function formatChangePercent(current: number, previous: number): string {
  if (previous === 0) return ''
  return String(Math.round(changePercent(current, previous) * 10) / 10)
}

/**
 * CSV is the cheapest tabular encoding a model reads reliably.
 *
 * The parameter is the ROW ARRAY, never the AnalyticsResponse: meta.query is the
 * fully rendered SQL and meta.params the bind values (services/api/analytics.ts
 * :57-65), and neither may ever reach a model. Keeping the response object out
 * of every formatter makes that a structural property of this file rather than
 * a rule someone must remember.
 */
export function formatRows(
  rows: Record<string, unknown>[],
  columns: string[],
  options: { maxRows: number; note?: string }
): string {
  if (rows.length === 0) return '(no rows: nothing matched this query in this period)'
  const shown = rows.slice(0, options.maxRows)
  const lines = [columns.join(',')]
  for (const row of shown) lines.push(columns.map((column) => renderCell(row[column])).join(','))
  lines.push(
    shown.length < rows.length
      ? `(showing first ${shown.length} of ${rows.length} rows; ask for a narrower query to see the rest)`
      : `(${rows.length} rows)`
  )
  if (options.note) lines.push(options.note)
  return lines.join('\n')
}

/**
 * The filter set a result was actually computed under, for the result header.
 * A data tool that quietly inherits the dashboard's filters must SAY so, or the
 * model presents a segment's numbers as the whole site's.
 */
export function describeFilters(filters: WebDimensionFilter[]): string {
  if (filters.length === 0) return 'none'
  return filters
    .map((filter) =>
      filter.operator === 'isEmpty' || filter.operator === 'isNotEmpty'
        ? `${filter.dimension} ${filter.operator}`
        : `${filter.dimension} ${filter.operator} ${filter.values.join('|')}`
    )
    .join(' AND ')
}

/** Column order the model can rely on: time bucket, then groupings, then numbers. */
export function orderedColumns(spec: {
  bucket?: string
  dimensions: string[]
  measures: string[]
}): string[] {
  return [...(spec.bucket ? [spec.bucket] : []), ...spec.dimensions, ...spec.measures]
}

/**
 * The column a bucketed query comes back under: the schema's time dimension suffixed
 * with the granularity (lib/dates.ts:199-201), e.g. `created_at_day`.
 *
 * THESE FOUR NAMES ARE NOT CATALOG DIMENSIONS. `created_at`, `updated_at`,
 * `entered_at` and `goal_at` exist in the Go schema and nowhere in
 * lib/dimensions.ts, so assertQueryableDimension refuses every one of them - which is
 * correct, because a time bucket is asked for with `granularity`, never by grouping.
 * They live here, in an output-column helper, and must never appear in the prompt's
 * dimension lists or in any tool description; the prompt drift test in Step 8 is what
 * keeps that true.
 */
export function bucketColumnFor(schema: WebSchema, granularity: Granularity): string {
  const dimension =
    schema === 'web_pages' ? 'entered_at' : schema === 'web_goals' ? 'goal_at' : 'created_at'
  return timeBucketColumn(dimension, granularity)
}

/**
 * Operator-facing name for a measure id.
 *
 * The four session metrics take their wording from SESSION_METRICS, so a step line
 * and the KPI tile above it call the same number the same thing rather than drifting
 * into two vocabularies. The rest are named here because MEASURES_BY_SCHEMA holds
 * sentence-long descriptions written FOR THE MODEL - "Median engaged time per
 * session, in seconds (the "TimeScore")" is not a label - and only the ids that
 * title-case badly need an entry.
 *
 * An unknown id title-cases rather than falling through raw, the same shape
 * getDimensionLabel uses, so a measure added to a schema is readable here before
 * anyone remembers this map exists.
 */
const MEASURE_LABELS: Record<string, string> = {
  ...Object.fromEntries(SESSION_METRICS.map((metric) => [metric.key, metric.label])),
  avg_scroll: 'Average Scroll Depth',
  pages_per_session: 'Pages per Session',
  median_page_duration: 'Median Time on Page',
  page_count: 'Page Views',
  page_duration: 'Time on Page',
  page_scroll: 'Scroll Depth',
  landing_page_count: 'Landing Page Views',
  exit_page_count: 'Exit Page Views',
  sum_goal_value: 'Total Goal Value',
  avg_goal_value: 'Average Goal Value',
  unique_sessions_with_goals: 'Converting Sessions'
}

export function getMeasureLabel(name: string): string {
  const known = MEASURE_LABELS[name]
  if (known) return known
  return name
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

/** Dimension ids as the operator's own names, in the drill-down order they were asked in. */
export function describeDimensions(names: string[], labels?: Record<string, string>): string {
  return names.map((name) => getDimensionLabel(name, labels)).join(' / ')
}

/**
 * What a query step line is ABOUT, in the operator's words.
 *
 * DIMENSION-LED, because the grouping is the part that differs between two steps of
 * one batch: three measures by channel_group and the same three by utm_campaign are
 * the same sentence for forty-five characters, and the panel wraps well before the
 * end of it - so the measure list buried the one word the operator was scanning for.
 * The measures are what an ungrouped query is about, so they lead only when there is
 * no grouping to lead with.
 *
 * Never an id on either side: a workspace that renamed custom_3 to "Plan" reads
 * "Plan", and `median_page_duration` reads as a metric name rather than a column.
 *
 * Bucketing is deliberately NOT expressed here. "per day" is a word, and the words
 * on this line are built with `t` by the component that owns the panel.
 */
export function describeQuery(spec: {
  measures: string[]
  dimensions: string[]
  labels?: Record<string, string>
}): string {
  if (spec.dimensions.length > 0) return describeDimensions(spec.dimensions, spec.labels)
  return spec.measures.map(getMeasureLabel).join(', ')
}

/** The catalog tool's body. Pure, so the PII omission is directly assertable. */
export function renderCatalog(
  schemas: WebSchema[],
  customDimensionLabels?: Record<string, string>
): string {
  const blocks = schemas.map((schema) => {
    const measures = Object.entries(MEASURES_BY_SCHEMA[schema])
      .map(([name, description]) => `  ${name} - ${description}`)
      .join('\n')
    const dimensions = dimensionsForSchema(schema)
      // Withheld dimensions are not merely refused: they are never named, so the
      // model does not learn they exist and does not try.
      .filter((dimension) => !BLOCKED_DIMENSIONS.has(dimension.name))
      .map((dimension) => {
        const label = getDimensionLabel(dimension.name, customDimensionLabels)
        const examples = DIMENSION_EXAMPLES[dimension.name]
        return (
          `  ${dimension.name} (${dimension.type}, ${dimension.category}) - ${label}` +
          (examples ? ` e.g. ${examples.map((value) => `"${value}"`).join(', ')}` : '')
        )
      })
      .join('\n')
    return `## ${schema}\nMEASURES:\n${measures}\nDIMENSIONS:\n${dimensions}`
  })
  return (
    blocks.join('\n\n') +
    '\n\nAll filter and dimension values are strings. Booleans are "true"/"false"; ' +
    '"not set" is the empty string. There is no conversion-rate measure.'
  )
}
