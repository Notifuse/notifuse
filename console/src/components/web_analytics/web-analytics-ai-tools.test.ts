import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  BLOCKED_DIMENSIONS,
  DEFAULT_BREAKDOWN_ROWS,
  MAX_BREAKDOWN_ROWS,
  MEASURES_BY_SCHEMA,
  NAVIGABLE_TABS,
  REDACTED_FILTER_VALUE,
  SCHEMAS,
  TOOL_COMPARISON_CHOICES,
  ToolInputError,
  WEB_ANALYTICS_AI_TOOLS,
  WEB_TOOL_NAMES,
  assertMeasures,
  assertOrderKey,
  assertQueryableDimension,
  bucketColumnFor,
  clampLimit,
  dropBlockedFilters,
  formatChangePercent,
  formatRows,
  parseComparisonMode,
  parseFilters,
  parseMetricFilters,
  redactBlockedFilterValues,
  renderCatalog,
  resolveComparisonRange,
  resolveToolRange,
  type ToolDateContext
} from './web-analytics-ai-tools'
import { DIMENSIONS } from './lib/dimensions'
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
  it('offers every section except the one the assistant is hidden on', () => {
    // shouldHideAssistant hides the panel on `filters`; honouring "show me the
    // attribution rules" by navigating there would make the assistant vanish
    // mid-turn and write its answer into an invisible element.
    expect(NAVIGABLE_TABS).not.toContain('filters')
    for (const tab of WEB_ANALYTICS_TABS) {
      if (tab === 'filters') continue
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
