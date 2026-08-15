import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  BLOCKED_DIMENSIONS,
  DASHBOARD_FILTER_SCHEMAS,
  DEFAULT_BREAKDOWN_ROWS,
  GRANULARITIES,
  MAX_BREAKDOWN_ROWS,
  MEASURES_BY_SCHEMA,
  NAVIGABLE_TABS,
  REDACTED_FILTER_VALUE,
  SCHEMAS,
  TOOL_COMPARISON_CHOICES,
  ToolInputError,
  WEB_ANALYTICS_AI_TOOLS,
  WEB_TOOL_NAMES,
  assertDashboardFilterDimension,
  assertMeasures,
  assertOrderKey,
  assertQueryableDimension,
  bucketColumnFor,
  clampLimit,
  describeDimensions,
  describeQuery,
  dropBlockedFilters,
  getMeasureLabel,
  formatChangePercent,
  formatRows,
  parseComparisonMode,
  parseFilters,
  parseGranularity,
  parseMetricFilters,
  redactBlockedFilterValues,
  renderCatalog,
  resolveComparisonRange,
  resolveToolRange,
  withPendingDates,
  type ToolDateContext
} from './web-analytics-ai-tools'
import { DIMENSIONS, dimensionsForSchema, getDimensionLabel } from './lib/dimensions'
import { SESSION_METRICS } from './lib/types'
import {
  DATE_PRESETS,
  PRESET_GROUPS,
  WEB_ANALYTICS_TABS,
  type DatePreset,
  type WebDimensionFilter,
  type WebSchema
} from './lib/types'

/* ===========================================================================
 * SCHEMA SHAPE — a recursive walker, so a tool added later is checked too.
 *
 * The same definitions are shipped verbatim to Anthropic, OpenAI-compatible
 * endpoints and Gemini, and each truncates a schema differently: Gemini drops
 * what it does not understand with no error at all, so a constraint expressed in
 * an unsupported keyword is enforced on two providers and silently absent on the
 * third. Every failure below reports the PATH, because "some tool has an array
 * without items" is not actionable when there are eight of them.
 * ========================================================================= */

type SchemaNode = Record<string, unknown>

function isNode(value: unknown): value is SchemaNode {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** Visits every object node of a tool schema, deepest-last, with its path. */
function walkSchema(value: unknown, path: string, visit: (node: SchemaNode, path: string) => void) {
  if (Array.isArray(value)) {
    value.forEach((entry, index) => walkSchema(entry, `${path}[${index}]`, visit))
    return
  }
  if (!isNode(value)) return
  visit(value, path)
  for (const [key, child] of Object.entries(value)) {
    walkSchema(child, `${path}.${key}`, visit)
  }
}

function walkAllTools(visit: (node: SchemaNode, path: string) => void) {
  for (const tool of WEB_ANALYTICS_AI_TOOLS) {
    walkSchema(tool.input_schema, `${tool.name}.input_schema`, visit)
  }
}

describe('tool schemas as the three providers read them', () => {
  it('gives every tool an object schema with properties and required', () => {
    const problems: string[] = []
    for (const tool of WEB_ANALYTICS_AI_TOOLS) {
      const schema = tool.input_schema as SchemaNode
      if (schema.type !== 'object') problems.push(`${tool.name}: type is ${String(schema.type)}`)
      if (!isNode(schema.properties)) problems.push(`${tool.name}: no properties object`)
      if (!Array.isArray(schema.required)) problems.push(`${tool.name}: no required array`)
    }
    expect(problems).toEqual([])
  })

  it('declares items on every array property', () => {
    // OpenAI-compatible endpoints reject the whole request when items is
    // missing; Gemini silently rewrites the property to a plain string.
    const problems: string[] = []
    walkAllTools((node, path) => {
      if (node.type === 'array' && !isNode(node.items)) problems.push(`${path}: array without items`)
    })
    expect(problems).toEqual([])
  })

  it('uses no schema keyword Gemini would drop without saying so', () => {
    const forbidden = ['oneOf', 'anyOf', 'allOf', '$ref', 'additionalProperties']
    const problems: string[] = []
    walkAllTools((node, path) => {
      for (const keyword of forbidden) {
        if (Object.prototype.hasOwnProperty.call(node, keyword)) problems.push(`${path}.${keyword}`)
      }
    })
    expect(problems).toEqual([])
  })

  it('keeps every enum a list of strings', () => {
    // A numeric enum member is dropped the same silent way an unsupported
    // keyword is, leaving that provider with an unconstrained property.
    const problems: string[] = []
    walkAllTools((node, path) => {
      if (!Array.isArray(node.enum)) return
      node.enum.forEach((entry, index) => {
        if (typeof entry !== 'string') {
          problems.push(`${path}.enum[${index}] is ${typeof entry}`)
        }
      })
    })
    expect(problems).toEqual([])
  })

  it('exposes exactly the tools the handler map is keyed by, each name once', () => {
    // A name in one place and not the other is a tool the model can call and
    // nothing answers, or a handler that is never reachable.
    const names = WEB_ANALYTICS_AI_TOOLS.map((tool) => tool.name)
    expect(new Set(names).size).toBe(names.length)
    expect(new Set(names)).toEqual(new Set(Object.values(WEB_TOOL_NAMES)))
  })

  it('asks for calendar days rather than instants wherever a date is named', () => {
    // The gap filler parses these with layout 2006-01-02; an RFC3339 instant
    // fails the whole query server-side, long after the model looks right.
    const problems: string[] = []
    walkAllTools((node, path) => {
      if (!isNode(node.properties)) return
      for (const key of ['start_date', 'end_date']) {
        const property = node.properties[key]
        if (!isNode(property)) continue
        const description = String(property.description ?? '')
        if (!description.includes('YYYY-MM-DD')) {
          problems.push(`${path}.properties.${key}: does not name the YYYY-MM-DD shape`)
        }
        if (/rfc\s?3339|iso\s?8601|timestamp|instant/i.test(description)) {
          problems.push(`${path}.properties.${key}: describes an instant`)
        }
      }
    })
    expect(problems).toEqual([])
  })

  it('offers exactly the granularities the validator accepts', () => {
    // A bucket offered in the schema and refused by the validator is a tool
    // call the model is invited to make and that always fails.
    const problems: string[] = []
    walkAllTools((node, path) => {
      if (!isNode(node.properties)) return
      const granularity = node.properties.granularity
      if (!isNode(granularity) || !Array.isArray(granularity.enum)) return
      if (granularity.enum.join(',') !== GRANULARITIES.join(',')) {
        problems.push(`${path}.properties.granularity.enum: ${granularity.enum.join(',')}`)
      }
    })
    expect(problems).toEqual([])
  })

  it('keeps the comparison vocabulary disjoint from the period presets', () => {
    // compare_periods exposes `period` and `comparison` side by side, and
    // "previous_year" is legal in both with two different meanings: a model
    // that copies one into the other would produce a plausible wrong report
    // with no error anywhere.
    const presets = new Set<string>(DATE_PRESETS)
    const problems: string[] = []
    walkAllTools((node, path) => {
      if (!isNode(node.properties)) return
      const comparison = node.properties.comparison
      if (!isNode(comparison) || !Array.isArray(comparison.enum)) return
      for (const choice of comparison.enum) {
        if (presets.has(String(choice))) {
          problems.push(`${path}.properties.comparison.enum: "${String(choice)}" is also a preset`)
        }
      }
    })
    expect(problems).toEqual([])
  })
})

/* ===========================================================================
 * DATE RESOLUTION
 * ========================================================================= */

// Well east of UTC, so a resolution that used the browser's zone instead of the
// workspace's is off by a whole day rather than by an ambiguous hour.
const TZ = 'Asia/Tokyo'

function dateContext(overrides: Partial<ToolDateContext> = {}): ToolDateContext {
  return {
    timezone: TZ,
    currentPeriod: 'previous_7_days',
    currentResolved: {
      startDay: '2026-03-08',
      endDay: '2026-03-14',
      startUtc: '2026-03-07T15:00:00.000Z',
      endUtc: '2026-03-14T14:59:59.999Z'
    },
    ...overrides
  }
}

describe('resolveToolRange', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // 2026-03-15 05:00 in Tokyo, still 2026-03-14 in UTC and in Europe: the
    // three zones disagree about what day it is right now.
    vi.setSystemTime(new Date('2026-03-14T20:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('resolves every preset the picker offers into concrete bounds', () => {
    const offered = PRESET_GROUPS.flat().filter((preset) => preset !== 'custom')
    const problems: string[] = []
    for (const preset of offered) {
      const { range } = resolveToolRange(dateContext(), { period: preset })
      if (!/^\d{4}-\d{2}-\d{2}$/.test(range.startDay)) problems.push(`${preset}: startDay`)
      if (!/^\d{4}-\d{2}-\d{2}$/.test(range.endDay)) problems.push(`${preset}: endDay`)
      if (range.endDay < range.startDay) problems.push(`${preset}: ends before it starts`)
    }
    expect(problems).toEqual([])
    expect(offered.length).toBeGreaterThan(0)
  })

  it('emits bare calendar days for bucketing and full instants for range filters', () => {
    const { range } = resolveToolRange(dateContext(), { period: 'previous_7_days' })
    expect(range.startDay).toBe('2026-03-08')
    expect(range.endDay).toBe('2026-03-14')
    // A bare date as the end bound would truncate at midnight and lose the
    // last day, so the instants must span the whole final local day.
    expect(range.startUtc).toBe('2026-03-07T15:00:00.000Z')
    expect(range.endUtc).toBe('2026-03-14T14:59:59.999Z')
  })

  it('resolves in the workspace timezone rather than the browser one', () => {
    const { range } = resolveToolRange(dateContext(), { period: 'today' })
    // It is already 2026-03-15 in Tokyo while UTC is still on 2026-03-14.
    expect(range.startDay).toBe('2026-03-15')
    expect(range.endDay).toBe('2026-03-15')
  })

  it('returns the range the page already resolved for "current", custom range included', () => {
    const context = dateContext({
      currentPeriod: 'custom',
      currentCustomStart: '2026-01-05',
      currentCustomEnd: '2026-01-09',
      currentResolved: {
        startDay: '2026-01-05',
        endDay: '2026-01-09',
        startUtc: '2026-01-04T15:00:00.000Z',
        endUtc: '2026-01-09T14:59:59.999Z'
      }
    })
    const resolved = resolveToolRange(context, {})
    expect(resolved.range).toBe(context.currentResolved)
    expect(resolved.preset).toBe('custom')
    expect(resolved.custom).toEqual({ start: '2026-01-05', end: '2026-01-09' })
  })

  it('rejects an unknown period name and lists what it accepts', () => {
    let message = ''
    try {
      resolveToolRange(dateContext(), { period: 'last_fortnight' })
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('last_fortnight')
    expect(message).toContain('current')
    expect(message).toContain('previous_7_days')
    expect(message).toContain('custom')
  })

  it('rejects a custom period without two calendar dates', () => {
    expect(() => resolveToolRange(dateContext(), { period: 'custom' })).toThrow(ToolInputError)
    expect(() =>
      resolveToolRange(dateContext(), { period: 'custom', start_date: '2026-01-05' })
    ).toThrow(/YYYY-MM-DD/)
    expect(() =>
      resolveToolRange(dateContext(), {
        period: 'custom',
        start_date: '5 Jan 2026',
        end_date: '9 Jan 2026'
      })
    ).toThrow(/YYYY-MM-DD/)
  })

  it('rejects a custom period that ends before it starts', () => {
    expect(() =>
      resolveToolRange(dateContext(), {
        period: 'custom',
        start_date: '2026-01-09',
        end_date: '2026-01-05'
      })
    ).toThrow(/before start_date/)
  })
})

describe('resolveComparisonRange', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-14T20:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('abuts the preceding window without overlapping it', () => {
    const context = dateContext()
    const current = resolveToolRange(context, { period: 'previous_7_days' })
    const previous = resolveComparisonRange(context, current, 'previous_period')
    expect(previous).not.toBeNull()
    // Current is 03-08..03-14, so the comparison must end the day before it
    // starts and cover the same number of days.
    expect(previous?.endDay).toBe('2026-03-07')
    expect(previous?.startDay).toBe('2026-03-01')
  })

  it('keeps the same calendar dates for a year-over-year comparison', () => {
    const context = dateContext()
    const current = resolveToolRange(context, { period: 'previous_7_days' })
    const previous = resolveComparisonRange(context, current, 'previous_year')
    expect(previous?.startDay).toBe('2025-03-08')
    expect(previous?.endDay).toBe('2025-03-14')
  })

  it('returns no comparison window when comparison is switched off', () => {
    const context = dateContext()
    const current = resolveToolRange(context, { period: 'previous_7_days' })
    expect(resolveComparisonRange(context, current, 'none')).toBeNull()
  })

  it('refuses to compare all_time against a window that precedes the first session', () => {
    // all_time already starts at the workspace's first session, so whatever
    // comes before it is empty by construction. Returning a range would query
    // it, come back with zeroes, and render every change cell blank - which
    // reads as "no change" rather than "there is nothing to compare against".
    const context = dateContext({ workspaceCreatedAt: '2026-01-10T00:00:00.000Z' })
    const current = resolveToolRange(context, { period: 'all_time' })
    for (const mode of ['previous_period', 'previous_year'] as const) {
      expect(resolveComparisonRange(context, current, mode)).toBeNull()
    }
  })

  it('compares against a window older than the workspace record, where imported and seeded data lives', () => {
    // The workspace ROW is younger than its analytics data whenever the data was
    // seeded, imported or backfilled - which is every demo workspace. Refusing on
    // the creation date told an operator holding thousands of sessions in the
    // preceding month that nothing preceded their range, while the dashboard beside
    // them charted that same comparison.
    const context = dateContext({ workspaceCreatedAt: '2026-01-10T00:00:00.000Z' })
    const current = resolveToolRange(context, { period: 'previous_7_days' })
    const previous = resolveComparisonRange(context, current, 'previous_year')
    expect(previous).not.toBeNull()
    // previous_year is the same calendar dates a year back, not the preceding window.
    expect(previous?.startDay).toBe('2025-03-08')
    expect(previous?.endDay).toBe('2025-03-14')
  })

  it('still compares against the ordinary preceding window', () => {
    const context = dateContext({ workspaceCreatedAt: '2026-01-10T00:00:00.000Z' })
    const current = resolveToolRange(context, { period: 'previous_7_days' })
    const previous = resolveComparisonRange(context, current, 'previous_period')
    expect(previous?.startDay).toBe('2026-03-01')
    expect(previous?.endDay).toBe('2026-03-07')
  })
})

describe('withPendingDates', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-14T20:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('resolves "current" against a period a sibling tool asked for this round', () => {
    // set_dashboard_period and query_web_analytics are dispatched in the same
    // synchronous round against one frozen snapshot: without the overlay the
    // query answers for the window the dashboard showed BEFORE its sibling ran,
    // while that sibling announces the new one.
    const context = dateContext()
    const effective = withPendingDates(context, { period: 'previous_30_days' })
    const resolved = resolveToolRange(effective, { period: 'current' })
    expect(resolved.preset).toBe('previous_30_days')
    expect(resolved.range.startDay).toBe('2026-02-13')
    expect(resolved.range.endDay).toBe('2026-03-14')
    expect(resolved.range).not.toBe(context.currentResolved)
  })

  it('carries a pending custom range into both the bounds and the label', () => {
    const effective = withPendingDates(dateContext(), {
      period: 'custom',
      customStart: '2026-01-05',
      customEnd: '2026-01-09'
    })
    const resolved = resolveToolRange(effective, { period: 'current' })
    expect(resolved.preset).toBe('custom')
    expect(resolved.custom).toEqual({ start: '2026-01-05', end: '2026-01-09' })
    expect(resolved.range.startDay).toBe('2026-01-05')
    expect(resolved.range.endDay).toBe('2026-01-09')
  })

  it('keeps the page-resolved range when nothing about the period is pending', () => {
    const context = dateContext()
    expect(withPendingDates(context, {})).toBe(context)
    expect(withPendingDates(context, { filters: [], tab: 'explore' })).toBe(context)
    // A custom period with no dates would silently fall back to "the last 7
    // days" inside computeDateRange, a window nobody asked for.
    expect(withPendingDates(context, { period: 'custom' })).toBe(context)
  })
})

/* ===========================================================================
 * PII BOUNDARY
 * ========================================================================= */

describe('withheld dimensions', () => {
  it('refuses to group by a visitor email and points at the aggregate measure', () => {
    let message = ''
    try {
      assertQueryableDimension('contact_email', 'web_sessions')
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('identifies individual visitors')
    expect(message).toContain('contacts')
  })

  it('refuses to group by per-visitor coordinates', () => {
    for (const dimension of ['latitude', 'longitude']) {
      expect(() => assertQueryableDimension(dimension, 'web_sessions')).toThrow(
        /identifies individual visitors/
      )
    }
  })

  it('refuses a withheld dimension as an order key', () => {
    // The order key is the one door no other validator covers: it is neither a
    // dimensions member nor a filter member.
    for (const key of [...BLOCKED_DIMENSIONS]) {
      expect(() => assertOrderKey(key, ['sessions'], ['country'])).toThrow(
        /identifies individual visitors/
      )
    }
  })

  it('refuses a withheld dimension as a filter member, on every schema and schema-free', () => {
    const schemas: (WebSchema | null)[] = [...SCHEMAS, null]
    for (const schema of schemas) {
      for (const dimension of [...BLOCKED_DIMENSIONS]) {
        expect(() =>
          parseFilters([{ dimension, operator: 'equals', values: ['a@example.com'] }], schema)
        ).toThrow(/identifies individual visitors/)
      }
    }
  })

  it('never names a withheld dimension in the catalog the model reads', () => {
    // Withheld is stronger than refused: the model must not learn the column
    // exists, or it spends turns trying to reach it.
    const catalog = renderCatalog(SCHEMAS)
    for (const dimension of [...BLOCKED_DIMENSIONS]) {
      expect(catalog).not.toContain(dimension)
    }
    // The aggregate replacement is still offered.
    expect(catalog).toContain('contacts - Distinct identified contacts')
  })
})

/* ===========================================================================
 * PROTOTYPE KEYS — DIMENSIONS and the measure maps are plain objects, so a bare
 * index lookup on "toString" or "constructor" returns something truthy and a
 * prototype key would sail through validation into a query.
 * ========================================================================= */

const PROTOTYPE_KEYS = ['toString', 'constructor', 'valueOf']

describe('prototype keys', () => {
  it('rejects a prototype key named as a dimension and sends the model to the catalog', () => {
    for (const key of PROTOTYPE_KEYS) {
      expect(() => assertQueryableDimension(key, 'web_sessions')).toThrow(
        new RegExp(`unknown dimension "${key}"; call ${WEB_TOOL_NAMES.CATALOG}`)
      )
    }
  })

  it('rejects a prototype key named as a dimension filter member', () => {
    for (const key of PROTOTYPE_KEYS) {
      expect(() => parseFilters([{ dimension: key, operator: 'equals', values: ['x'] }], null)).toThrow(
        /unknown dimension/
      )
    }
  })

  it('rejects a prototype key named as a measure', () => {
    for (const key of PROTOTYPE_KEYS) {
      expect(() => assertMeasures([key], 'web_sessions')).toThrow(
        new RegExp(`unknown measure "${key}"`)
      )
    }
  })

  it('rejects a prototype key named as a metric filter', () => {
    for (const key of PROTOTYPE_KEYS) {
      expect(() =>
        parseMetricFilters([{ metric: key, operator: 'gt', value: 1 }], 'web_sessions')
      ).toThrow(/metric filter on unknown measure/)
    }
  })
})

/* ===========================================================================
 * CATALOG VALIDATION
 * ========================================================================= */

describe('assertQueryableDimension', () => {
  it('refuses a dimension of another schema and names the schemas that carry it', () => {
    let message = ''
    try {
      assertQueryableDimension('utm_source', 'web_pages')
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('does not exist on schema web_pages')
    expect(message).toContain('web_sessions')
    expect(message).toContain('web_goals')
  })

  it('sends the model to the catalog tool for an unknown dimension', () => {
    expect(() => assertQueryableDimension('bounce_source', 'web_sessions')).toThrow(
      new RegExp(`unknown dimension "bounce_source"; call ${WEB_TOOL_NAMES.CATALOG}`)
    )
  })

  it('accepts a dimension the schema really carries', () => {
    expect(assertQueryableDimension('  country  ', 'web_sessions')).toBe('country')
  })
})

describe('assertMeasures', () => {
  it('refuses the invented conversion-rate measure with the arithmetic to use instead', () => {
    expect(() => assertMeasures(['conversion_rate'], 'web_sessions')).toThrow(
      /divide goal_conversions by sessions yourself/
    )
  })

  it('refuses a measure belonging to another schema and lists the available ones', () => {
    let message = ''
    try {
      assertMeasures(['sessions'], 'web_pages')
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('unknown measure "sessions" on schema web_pages')
    expect(message).toContain('page_count')
  })

  it('requires at least one measure', () => {
    expect(() => assertMeasures([], 'web_sessions')).toThrow(/at least one measure/)
    expect(() => assertMeasures(undefined, 'web_sessions')).toThrow(/at least one measure/)
  })

  it('accepts measures of the queried schema', () => {
    expect(assertMeasures(['sessions', 'bounce_rate'], 'web_sessions')).toEqual([
      'sessions',
      'bounce_rate'
    ])
  })
})

describe('assertOrderKey', () => {
  it('refuses a key that is neither a selected measure nor a selected dimension', () => {
    let message = ''
    try {
      assertOrderKey('pageviews', ['sessions'], ['country'])
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('order_by must name one of the measures or dimensions')
    expect(message).toContain('sessions')
    expect(message).toContain('country')
  })

  it('accepts a selected measure and a selected dimension', () => {
    expect(assertOrderKey('sessions', ['sessions'], ['country'])).toBe('sessions')
    expect(assertOrderKey('country', ['sessions'], ['country'])).toBe('country')
  })

  it('requires an order key when one is asked for', () => {
    expect(() => assertOrderKey('   ', ['sessions'], [])).toThrow(/order key is required/)
  })
})

/* ===========================================================================
 * FILTER BAR HYGIENE — the dashboard's own filters are parsed out of the URL and
 * can legitimately contain contact_email, so anything that lets them reach the
 * model has to launder them first.
 * ========================================================================= */

const FILTER_BAR: WebDimensionFilter[] = [
  { dimension: 'contact_email', operator: 'equals', values: ['alice@example.com'] },
  { dimension: 'device', operator: 'in', values: ['mobile', 'tablet'] }
]

describe('redactBlockedFilterValues', () => {
  it('keeps the shape of a withheld filter but replaces its values', () => {
    // The model still has to know a narrowing filter is in force, or it reads
    // every number on screen as the whole site's.
    const redacted = redactBlockedFilterValues(FILTER_BAR)
    expect(redacted[0]).toEqual({
      dimension: 'contact_email',
      operator: 'equals',
      values: [REDACTED_FILTER_VALUE]
    })
    expect(redacted[0].values).not.toContain('alice@example.com')
  })

  it('leaves every other filter untouched', () => {
    const redacted = redactBlockedFilterValues(FILTER_BAR)
    expect(redacted[1]).toBe(FILTER_BAR[1])
    expect(redacted).toHaveLength(FILTER_BAR.length)
  })
})

describe('dropBlockedFilters', () => {
  it('removes the withheld filter and nothing else', () => {
    expect(dropBlockedFilters(FILTER_BAR)).toEqual([FILTER_BAR[1]])
  })
})

describe('parseComparisonMode', () => {
  it('maps the tool tokens onto the dashboard comparison modes', () => {
    expect(parseComparisonMode('vs_preceding_window', 'none')).toBe('previous_period')
    expect(parseComparisonMode('vs_same_dates_last_year', 'none')).toBe('previous_year')
    expect(parseComparisonMode('off', 'previous_period')).toBe('none')
  })

  it('falls back to what the dashboard is comparing when the model says nothing', () => {
    expect(parseComparisonMode(undefined, 'previous_year')).toBe('previous_year')
    expect(parseComparisonMode(null, 'none')).toBe('none')
  })

  it('refuses a period preset used as a comparison', () => {
    let message = ''
    try {
      parseComparisonMode('previous_year', 'none')
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('is a PERIOD, not a comparison')
    for (const choice of TOOL_COMPARISON_CHOICES) expect(message).toContain(choice)
  })
})

describe('parseFilters', () => {
  it('refuses an operator the console does not offer', () => {
    // There is no set/notSet: web dimensions are NOT NULL DEFAULT '', so "is
    // empty" is an equality with the empty string.
    for (const operator of ['set', 'notSet', 'startsWith']) {
      expect(() =>
        parseFilters([{ dimension: 'device', operator, values: ['mobile'] }], 'web_sessions')
      ).toThrow(new RegExp(`unknown filter operator "${operator}"`))
    }
  })

  it('requires a value for every operator except the emptiness ones', () => {
    expect(() =>
      parseFilters([{ dimension: 'device', operator: 'equals', values: [] }], 'web_sessions')
    ).toThrow(/needs at least one value/)
    expect(
      parseFilters([{ dimension: 'device', operator: 'isEmpty', values: [] }], 'web_sessions')
    ).toEqual([{ dimension: 'device', operator: 'isEmpty', values: [] }])
  })

  it('stringifies values, because the columns store booleans and numbers as text', () => {
    expect(
      parseFilters([{ dimension: 'is_weekend', operator: 'in', values: [true, 42] }], 'web_sessions')
    ).toEqual([{ dimension: 'is_weekend', operator: 'in', values: ['true', '42'] }])
  })

  it('treats an absent filter list as no filters', () => {
    expect(parseFilters(undefined, 'web_sessions')).toEqual([])
    expect(parseFilters(null, 'web_sessions')).toEqual([])
    expect(() => parseFilters('device=mobile', 'web_sessions')).toThrow(/must be an array/)
  })
})

describe('parseMetricFilters', () => {
  it('refuses a threshold on something that is not a measure of the queried schema', () => {
    let message = ''
    try {
      parseMetricFilters([{ metric: 'sessions', operator: 'gt', value: 100 }], 'web_pages')
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('metric filter on unknown measure "sessions"')
    expect(message).toContain('page_count')
  })

  it('accepts a threshold on a measure of the queried schema', () => {
    expect(parseMetricFilters([{ metric: 'page_count', operator: 'gt', value: '100' }], 'web_pages')).toEqual(
      [{ metric: 'page_count', operator: 'gt', values: [100] }]
    )
  })

  it('refuses an operator outside the metric operator list', () => {
    expect(() =>
      parseMetricFilters([{ metric: 'page_count', operator: 'equals', value: 1 }], 'web_pages')
    ).toThrow(/metric filter operator must be one of/)
  })

  it('refuses a threshold that is missing or not a number', () => {
    // The schema's `required` is a hint no provider is obliged to enforce, and
    // a value that coerced to 0 would compile to HAVING page_count > 0: a
    // filter that removes nothing, while the model is told the threshold
    // applied and reports the long tail as excluded.
    const bad: unknown[] = [undefined, null, '', '   ', 'one hundred', true, {}, [], Infinity]
    for (const value of bad) {
      let message = ''
      try {
        parseMetricFilters([{ metric: 'page_count', operator: 'gt', value }], 'web_pages')
      } catch (error) {
        message = (error as Error).message
      }
      expect(message).toContain('needs a numeric value')
      expect(message).toContain('page_count')
    }
  })

  it('keeps zero as a threshold, which is a real one', () => {
    expect(parseMetricFilters([{ metric: 'page_count', operator: 'gt', value: 0 }], 'web_pages')).toEqual(
      [{ metric: 'page_count', operator: 'gt', values: [0] }]
    )
  })
})

describe('parseGranularity', () => {
  it('accepts every bucket the schema offers and nothing else', () => {
    for (const granularity of GRANULARITIES) {
      expect(parseGranularity(granularity)).toBe(granularity)
    }
    // It reaches both the engine query and the name of the output column the
    // model is told to read, so an unvalidated value produces a table whose
    // bucket column is missing from every row.
    for (const bad of ['daily', 'minute', 'DAY', 'constructor', 'toString', 7]) {
      expect(() => parseGranularity(bad)).toThrow(ToolInputError)
    }
  })

  it('lists the buckets it accepts when refusing one', () => {
    let message = ''
    try {
      parseGranularity('daily')
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('daily')
    for (const granularity of GRANULARITIES) expect(message).toContain(granularity)
  })

  it('treats an absent granularity as an aggregate query rather than an error', () => {
    expect(parseGranularity(undefined)).toBeUndefined()
    expect(parseGranularity(null)).toBeUndefined()
    expect(parseGranularity('')).toBeUndefined()
  })

  it('names an output column for every bucket it accepts', () => {
    // The validator and bucketColumnFor must cover the same vocabulary, or a
    // granularity passes validation and then names a column nothing returns.
    for (const schema of SCHEMAS) {
      for (const granularity of GRANULARITIES) {
        expect(bucketColumnFor(schema, granularity)).toMatch(new RegExp(`_${granularity}$`))
      }
    }
  })
})

/* ===========================================================================
 * DASHBOARD FILTER SCOPE — the filter bar is page-wide, so it can only carry
 * dimensions every visible widget can express.
 * ========================================================================= */

describe('assertDashboardFilterDimension', () => {
  it('refuses a dimension the visible widgets cannot use and names the usable scope', () => {
    // page_path lives only on web_pages: applied to the bar, every widget drops
    // it, the screen does not change, and the acknowledgement still reports the
    // filter as applied.
    let message = ''
    try {
      assertDashboardFilterDimension('page_path')
    } catch (error) {
      message = (error as Error).message
    }
    expect(message).toContain('page_path')
    for (const schema of DASHBOARD_FILTER_SCHEMAS) expect(message).toContain(schema)
    expect(message).toContain(WEB_TOOL_NAMES.CATALOG)
  })

  it('accepts every dimension the dashboard widgets can express, and no page-only one', () => {
    const usable = new Set(
      DASHBOARD_FILTER_SCHEMAS.flatMap((schema) =>
        dimensionsForSchema(schema).map((dimension) => dimension.name)
      )
    )
    const problems: string[] = []
    for (const name of Object.keys(DIMENSIONS)) {
      if (BLOCKED_DIMENSIONS.has(name)) continue
      let accepted = true
      try {
        assertDashboardFilterDimension(name)
      } catch {
        accepted = false
      }
      if (accepted !== usable.has(name)) {
        problems.push(`${name}: ${accepted ? 'accepted' : 'refused'} but usable=${usable.has(name)}`)
      }
    }
    expect(problems).toEqual([])
    // The check is only meaningful if both sides are non-empty.
    expect(usable.size).toBeGreaterThan(0)
    expect(Object.keys(DIMENSIONS).length).toBeGreaterThan(usable.size)
  })

  it('reports a withheld dimension as PII rather than as the wrong schema', () => {
    // contact_email IS valid on the dashboard schemas, so the scope check would
    // wave it through; the reason a refusal gives is what the model repeats to
    // the operator.
    for (const dimension of [...BLOCKED_DIMENSIONS]) {
      expect(() => assertDashboardFilterDimension(dimension)).toThrow(
        /identifies individual visitors/
      )
    }
  })

  it('sends the model to the catalog for a dimension that does not exist', () => {
    for (const name of ['bounce_source', ...PROTOTYPE_KEYS]) {
      expect(() => assertDashboardFilterDimension(name)).toThrow(/unknown dimension/)
    }
  })
})

describe('clampLimit', () => {
  it('caps a limit above the ceiling instead of letting the payload grow', () => {
    expect(clampLimit(5000)).toBe(MAX_BREAKDOWN_ROWS)
    expect(clampLimit(MAX_BREAKDOWN_ROWS + 1)).toBe(MAX_BREAKDOWN_ROWS)
  })

  it('falls back to the default for an absent or non-positive limit', () => {
    expect(clampLimit(undefined)).toBe(DEFAULT_BREAKDOWN_ROWS)
    expect(clampLimit(0)).toBe(DEFAULT_BREAKDOWN_ROWS)
    expect(clampLimit(-10)).toBe(DEFAULT_BREAKDOWN_ROWS)
    expect(clampLimit('not a number')).toBe(DEFAULT_BREAKDOWN_ROWS)
  })

  it('keeps a sensible limit as asked', () => {
    expect(clampLimit(7)).toBe(7)
    expect(clampLimit('7')).toBe(7)
  })
})

/* ===========================================================================
 * OUTPUT FORMATTING
 * ========================================================================= */

describe('bucketColumnFor', () => {
  it('names the column a bucketed query actually comes back under', () => {
    expect(bucketColumnFor('web_sessions', 'day')).toBe('created_at_day')
    expect(bucketColumnFor('web_pages', 'hour')).toBe('entered_at_hour')
    expect(bucketColumnFor('web_goals', 'day')).toBe('goal_at_day')
  })

  it('names time dimensions that are deliberately not groupable dimensions', () => {
    // A time bucket is asked for with `granularity`, never by grouping. If one
    // of these ever became a catalog dimension the prompt would start teaching
    // it as a grouping and every such query would be refused by the validator.
    for (const schema of SCHEMAS) {
      const timeDimension = bucketColumnFor(schema, 'day').replace(/_day$/, '')
      expect(Object.prototype.hasOwnProperty.call(DIMENSIONS, timeDimension)).toBe(false)
      expect(() => assertQueryableDimension(timeDimension, schema)).toThrow(/unknown dimension/)
    }
  })
})

describe('formatChangePercent', () => {
  it('rounds a change to one decimal rather than spending a line on float noise', () => {
    expect(formatChangePercent(35, 60)).toBe('-41.7')
    expect(formatChangePercent(120, 100)).toBe('20')
  })

  it('prints nothing for a zero baseline, since "0" would read as "no change"', () => {
    expect(formatChangePercent(42, 0)).toBe('')
  })
})

describe('formatRows', () => {
  const columns = ['country', 'sessions']

  it('emits a header, the rows and a row count', () => {
    const output = formatRows(
      [
        { country: 'US', sessions: 120 },
        { country: 'FR', sessions: 40 }
      ],
      columns,
      { maxRows: 10 }
    )
    expect(output).toBe('country,sessions\nUS,120\nFR,40\n(2 rows)')
  })

  it('quotes a value containing a comma, a quote or a newline', () => {
    const output = formatRows(
      [
        { country: 'Paris, France', sessions: 1 },
        { country: 'the "best" city', sessions: 2 },
        { country: 'two\nlines', sessions: 3 }
      ],
      columns,
      { maxRows: 10 }
    )
    expect(output).toContain('"Paris, France",1')
    expect(output).toContain('"the ""best"" city",2')
    expect(output).toContain('"two\nlines",3')
  })

  it('announces a truncated list so it cannot be read as a complete one', () => {
    const rows = Array.from({ length: 5 }, (_, index) => ({ country: `C${index}`, sessions: index }))
    const output = formatRows(rows, columns, { maxRows: 2 })
    expect(output).toContain('(showing first 2 of 5 rows')
    expect(output).not.toContain('(5 rows)')
  })

  it('reports an empty result as no rows rather than as a truncation', () => {
    const output = formatRows([], columns, { maxRows: 10 })
    expect(output).toContain('no rows')
    expect(output).not.toContain('showing first')
  })
})

describe('renderCatalog', () => {
  it('lists every measure and dimension of the schemas it is asked for', () => {
    const catalog = renderCatalog(['web_goals'])
    expect(catalog).toContain('## web_goals')
    for (const measure of Object.keys(MEASURES_BY_SCHEMA.web_goals)) {
      expect(catalog).toContain(`  ${measure} - `)
    }
    expect(catalog).toContain('goal_name (string, Goal)')
    // Only the requested schema, so the model is not shown page measures for a
    // goals question.
    expect(catalog).not.toContain('## web_sessions')
  })

  it("prints the workspace's own labels for its custom dimensions", () => {
    const catalog = renderCatalog(['web_sessions'], { custom_1: 'Plan' })
    expect(catalog).toContain('custom_1 (string, Custom) - Plan')
    expect(catalog).not.toContain('custom_1 (string, Custom) - Custom 1')
  })
})

/* ===========================================================================
 * NAVIGATION
 * ========================================================================= */

describe('NAVIGABLE_TABS', () => {
  it('offers every section except the ones the assistant is hidden on', () => {
    // shouldHideAssistant hides the panel on `filters` and `annotations`;
    // honouring "show me the attribution rules" by navigating there would make
    // the assistant vanish mid-turn and write its answer into an invisible
    // element. Spelled out here rather than imported from the component so this
    // stays an independent statement of the same rule.
    const HIDDEN: string[] = ['filters', 'annotations']
    for (const tab of HIDDEN) {
      expect(NAVIGABLE_TABS).not.toContain(tab)
    }
    for (const tab of WEB_ANALYTICS_TABS) {
      if (HIDDEN.includes(tab)) continue
      expect(NAVIGABLE_TABS).toContain(tab)
    }
  })

  it('offers the model exactly those tabs in navigate_to_tab', () => {
    const navigate = WEB_ANALYTICS_AI_TOOLS.find((tool) => tool.name === WEB_TOOL_NAMES.NAVIGATE)
    const schema = navigate?.input_schema as SchemaNode
    const properties = schema.properties as SchemaNode
    expect((properties.tab as SchemaNode).enum).toEqual([...NAVIGABLE_TABS])
  })
})

/* ===========================================================================
 * PERIOD ENUM — the model may only name what the resolver accepts.
 * ========================================================================= */

describe('period enums', () => {
  it('offers only period names the resolver can resolve', () => {
    const context = dateContext()
    const problems: string[] = []
    walkAllTools((node, path) => {
      if (!isNode(node.properties)) return
      const period = node.properties.period
      if (!isNode(period) || !Array.isArray(period.enum)) return
      for (const choice of period.enum) {
        const name = String(choice)
        try {
          resolveToolRange(context, {
            period: name,
            start_date: '2026-01-05',
            end_date: '2026-01-09'
          })
        } catch (error) {
          problems.push(`${path}.properties.period: "${name}" — ${(error as Error).message}`)
        }
      }
    })
    expect(problems).toEqual([])
  })

  it('keeps every offered preset a real DatePreset apart from the "current" meta-value', () => {
    const presets = new Set<string>(DATE_PRESETS)
    const problems: string[] = []
    walkAllTools((node, path) => {
      if (!isNode(node.properties)) return
      const period = node.properties.period
      if (!isNode(period) || !Array.isArray(period.enum)) return
      for (const choice of period.enum) {
        const name = String(choice) as DatePreset | 'current'
        if (name !== 'current' && !presets.has(name)) problems.push(`${path}: "${name}"`)
      }
    })
    expect(problems).toEqual([])
  })
})


describe('step line descriptors', () => {
  it('leads with the grouping, so two steps of one batch differ at the first character', () => {
    const measures = ['sessions', 'bounce_rate', 'median_duration']
    const byChannel = describeQuery({ measures, dimensions: ['channel_group'] })
    const byCampaign = describeQuery({ measures, dimensions: ['utm_campaign'] })

    // The defect this replaces: both lines opened with the same measure list and
    // wrapped before the dimension, so the only part that differed was the part the
    // operator could not see.
    expect(byChannel).toBe('Channel Group')
    expect(byCampaign).toBe('UTM Campaign')
    expect(byChannel[0]).not.toBe(byCampaign[0])
  })

  it('names a drill-down by every level it groups on', () => {
    expect(describeQuery({ measures: ['sessions'], dimensions: ['device', 'browser'] })).toBe(
      'Device / Browser'
    )
  })

  it("uses the workspace's own name for a custom slot rather than the slot number", () => {
    expect(
      describeQuery({
        measures: ['sessions'],
        dimensions: ['custom_3'],
        labels: { custom_3: 'Plan' }
      })
    ).toBe('Plan')
  })

  it('falls back to what an ungrouped query measured, named rather than identified', () => {
    expect(describeQuery({ measures: ['sessions', 'median_page_duration'], dimensions: [] })).toBe(
      'Sessions, Median Time on Page'
    )
  })

  it('never puts a raw measure id in front of an operator', () => {
    const raw: string[] = []
    for (const schema of SCHEMAS) {
      for (const measure of Object.keys(MEASURES_BY_SCHEMA[schema])) {
        const label = getMeasureLabel(measure)
        if (label.includes('_') || label === '') raw.push(`${schema}.${measure} -> "${label}"`)
      }
    }
    expect(raw).toEqual([])
  })

  it('calls the dashboard metrics what the KPI tiles above the panel call them', () => {
    // The link, not the wording: rename a tile and the assistant follows it rather
    // than growing a second vocabulary for the same number.
    for (const metric of SESSION_METRICS) {
      expect(getMeasureLabel(metric.key)).toBe(metric.label)
    }
  })

  it('title-cases a measure nobody has named yet instead of leaking the column', () => {
    expect(getMeasureLabel('refund_amount')).toBe('Refund Amount')
  })

  it('describes dimensions the same way wherever they are listed', () => {
    // set_explore_report builds its line from the same helper, so a drill-down and a
    // query that group on the same thing cannot read differently.
    expect(describeDimensions(['channel_group', 'custom_1'], { custom_1: 'Plan' })).toBe(
      `${getDimensionLabel('channel_group')} / Plan`
    )
  })
})
