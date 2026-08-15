import { describe, expect, it } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import {
  INSTALL_STATE_NOTES,
  WEB_ANALYTICS_AI_SYSTEM_PROMPT,
  buildWebAnalyticsSystemPrompt,
  serializeWebAnalyticsState,
  type WebAnalyticsPromptContext
} from './web-analytics-ai-system-prompt'
import {
  BLOCKED_DIMENSIONS,
  MEASURES_BY_SCHEMA,
  REDACTED_FILTER_VALUE,
  SCHEMAS,
  WEB_TOOL_NAMES
} from './web-analytics-ai-tools'
import { DIMENSIONS } from './lib/dimensions'
import { TOOL_RESULT_PROTOCOL_PROMPT } from '../ai-assistant/wire'
import type { InstallState } from './lib/installStatus'
import type { WebSchema } from './lib/types'

const REFUSAL_HEADING = '## What you must not do'

/** Everything the prompt TEACHES, i.e. everything before the refusal section. */
const teachingText = WEB_ANALYTICS_AI_SYSTEM_PROMPT.slice(
  0,
  WEB_ANALYTICS_AI_SYSTEM_PROMPT.indexOf(REFUSAL_HEADING)
)
const refusalText = WEB_ANALYTICS_AI_SYSTEM_PROMPT.slice(
  WEB_ANALYTICS_AI_SYSTEM_PROMPT.indexOf(REFUSAL_HEADING)
)

const baseContext: WebAnalyticsPromptContext = {
  tab: 'dashboard',
  installState: 'ok',
  timezone: 'Europe/Paris',
  now: '2026-08-15T09:30:00Z',
  period: 'previous_7_days',
  resolved: {
    startDay: '2026-08-08',
    endDay: '2026-08-14',
    startUtc: '2026-08-07T22:00:00Z',
    endUtc: '2026-08-14T21:59:59Z'
  },
  comparison: 'previous_period',
  resolvedCompare: {
    startDay: '2026-08-01',
    endDay: '2026-08-07',
    startUtc: '2026-07-31T22:00:00Z',
    endUtc: '2026-08-07T21:59:59Z'
  },
  granularity: 'day',
  availableGranularities: ['hour', 'day'],
  filters: [],
  metricFilters: [],
  minSessions: 1,
  dimensions: [],
  bounceThresholdSeconds: 10
}

function stateLines(context: Partial<WebAnalyticsPromptContext> = {}): string[] {
  return serializeWebAnalyticsState({ ...baseContext, ...context }).split('\n')
}

function lineStartingWith(prefix: string, context?: Partial<WebAnalyticsPromptContext>): string {
  const line = stateLines(context).find((candidate) => candidate.startsWith(prefix))
  expect(line, `no state line starting with "${prefix}"`).toBeDefined()
  return line as string
}

/* ---------------------------------------------------------------------------
 * Harvesting the dimension vocabulary out of the prompt itself.
 *
 * The point of harvesting rather than hand-copying is that these cases fail
 * when the prompt drifts from the catalog the tool validator enforces - a
 * second hand-maintained list would just drift alongside it.
 * ------------------------------------------------------------------------- */

/** Glosses carry commas of their own ("country (ISO code, e.g. US, FR)"). */
function stripGlosses(text: string): string {
  return text.replace(/\([^)]*\)/g, ' ')
}

/** The prompt writes the ten custom slots as a range: "custom_1 .. custom_10". */
function expandRange(token: string): string[] {
  const range = token.match(/^([a-z_]+?)(\d+)\s*\.\.\s*\1(\d+)$/)
  if (!range) return [token]
  const names: string[] = []
  for (let slot = Number(range[2]); slot <= Number(range[3]); slot++) {
    names.push(`${range[1]}${slot}`)
  }
  return names
}

function harvestNames(list: string): string[] {
  return stripGlosses(list)
    .split(',')
    .map((part) => part.trim().replace(/\.$/, ''))
    .flatMap(expandRange)
    .filter((token) => /^[a-z][a-z0-9_]*$/.test(token))
}

/** The `Dimensions - ONLY these five: ...` line under `### web_pages`. */
function harvestPageDimensions(): string[] {
  const match = WEB_ANALYTICS_AI_SYSTEM_PROMPT.match(/^Dimensions - ONLY these [a-z]+: (.+)$/m)
  expect(match, 'web_pages dimension line not found in the prompt').not.toBeNull()
  return harvestNames((match as RegExpMatchArray)[1])
}

/** The `Own dimensions: ...` line under `### web_goals`, cut at its prose tail. */
function harvestGoalDimensions(): string[] {
  const match = WEB_ANALYTICS_AI_SYSTEM_PROMPT.match(/^Own dimensions: (.+)$/m)
  expect(match, 'web_goals dimension line not found in the prompt').not.toBeNull()
  return harvestNames((match as RegExpMatchArray)[1].split(' - ')[0])
}

/** The `web_sessions also has: ...` line. */
function harvestSessionDimensions(): string[] {
  const match = WEB_ANALYTICS_AI_SYSTEM_PROMPT.match(/^web_sessions also has: (.+)$/m)
  expect(match, 'web_sessions dimension line not found in the prompt').not.toBeNull()
  return harvestNames((match as RegExpMatchArray)[1])
}

/** Every `- Category: a, b, c` bullet of the attribution list. */
function harvestAttributionDimensions(): string[] {
  const section = WEB_ANALYTICS_AI_SYSTEM_PROMPT.slice(
    WEB_ANALYTICS_AI_SYSTEM_PROMPT.indexOf('Attribution dimensions'),
    WEB_ANALYTICS_AI_SYSTEM_PROMPT.indexOf('web_sessions also has:')
  )
  expect(section.length, 'attribution dimension section not found in the prompt').toBeGreaterThan(0)
  const names: string[] = []
  for (const bullet of section.matchAll(/^- [^:\n]+: (.+)$/gm)) {
    names.push(...harvestNames(bullet[1]))
  }
  return names
}

describe('WEB_ANALYTICS_AI_SYSTEM_PROMPT withheld dimensions', () => {
  // The teaching sections must not put a name in the model's mouth that the tool
  // helpers will then refuse: a model taught to group web_pages by contact_email
  // spends every turn retrying a call that can only error.
  it('teaches no dimension the tool helpers refuse', () => {
    // Without the heading the split is meaningless and every case here is vacuous.
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toContain(REFUSAL_HEADING)
    expect(BLOCKED_DIMENSIONS.size).toBeGreaterThan(0)
    for (const blocked of BLOCKED_DIMENSIONS) {
      expect(teachingText, `"${blocked}" is taught above "${REFUSAL_HEADING}"`).not.toContain(
        blocked
      )
    }
  })

  // A refusal that will not name what it refuses is not a refusal: the model has
  // to be able to recognise the name it just tried to use.
  it('names every withheld dimension in its refusal', () => {
    for (const blocked of BLOCKED_DIMENSIONS) {
      expect(refusalText).toContain(blocked)
    }
  })

  it('points the operator at the contacts measure instead of the withheld roster', () => {
    expect(refusalText).toContain('contacts')
    expect(refusalText).toMatch(/distinct identified contacts/i)
  })
})

describe('WEB_ANALYTICS_AI_SYSTEM_PROMPT tool vocabulary', () => {
  const toolNames = Object.values(WEB_TOOL_NAMES) as string[]
  /** The two the backend injects into every llm.chat request of a Firecrawl workspace. */
  const forbiddenServerTools = ['scrape_url', 'search_web']

  // Catches the prompt promising a tool that does not ship - the model would
  // call it, the handler map would not have it, and the turn dies on an error
  // the operator cannot act on.
  it('names no tool that does not exist', () => {
    const matches = WEB_ANALYTICS_AI_SYSTEM_PROMPT.match(
      /\b(?:query|summarize|compare|list|set|get|navigate|scrape|search)_[a-z_]+\b/g
    )
    expect(matches, 'no tool-shaped token found; the scan cannot pass vacuously').not.toBeNull()
    const seen = new Set(matches as string[])
    expect(seen.size).toBeGreaterThan(0)
    for (const token of seen) {
      expect(
        [...toolNames, ...forbiddenServerTools],
        `prompt names "${token}", which is neither a shipped tool nor a forbidden server tool`
      ).toContain(token)
    }
  })

  // The reverse drift: a shipped tool the model is never told about is dead weight
  // in every request body and never gets called.
  it('names every tool it ships with', () => {
    expect(toolNames.length).toBeGreaterThan(0)
    for (const name of toolNames) {
      expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT, `"${name}" is never mentioned`).toContain(name)
    }
  })

  it('names the query and the period-summary tools as the way to learn a number', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toContain(WEB_TOOL_NAMES.QUERY)
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toContain(WEB_TOOL_NAMES.SUMMARIZE)
  })

  it('tells the model granularity is a query argument, not a dashboard tool', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/NO tool for the chart granularity/)
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(
      /argument on the data tools, chosen per query, not a piece of screen state/
    )
  })

  // The backend injects these into any llm.chat request of a workspace with a
  // Firecrawl integration and no caller can decline them, so the prompt is the
  // only lever that stops the assistant answering a traffic question by
  // scraping the public internet.
  it('forbids the scraping tools the platform injects', () => {
    for (const tool of forbiddenServerTools) {
      expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toContain(tool)
    }
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/NEVER call them here/)
  })

  it('embeds the tool-result protocol so the synthetic turn is not read as the operator speaking', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toContain(TOOL_RESULT_PROTOCOL_PROMPT)
  })
})

describe('WEB_ANALYTICS_AI_SYSTEM_PROMPT dimension vocabulary', () => {
  function expectQueryable(name: string, schema?: WebSchema): void {
    expect(
      Object.prototype.hasOwnProperty.call(DIMENSIONS, name),
      `prompt teaches "${name}", which is not in the console dimension catalog`
    ).toBe(true)
    expect(
      BLOCKED_DIMENSIONS.has(name),
      `prompt teaches "${name}", which the tool helpers refuse`
    ).toBe(false)
    if (schema) {
      expect(
        DIMENSIONS[name].schemas,
        `prompt teaches "${name}" under ${schema}, which the catalog does not put there`
      ).toContain(schema)
    }
  }

  // The case that pins the prompt's dimension vocabulary to the catalog
  // assertQueryableDimension validates against: teach a name that is not a key
  // of DIMENSIONS - a Go-side time column such as entered_at, say - and every
  // call the model makes from that teaching is refused before a query is built.
  it('teaches only page dimensions the page schema can group by', () => {
    const names = harvestPageDimensions()
    expect(names.length).toBeGreaterThan(0)
    for (const name of names) expectQueryable(name, 'web_pages')
  })

  it('teaches only goal dimensions the goal schema can group by', () => {
    const names = harvestGoalDimensions()
    expect(names.length).toBeGreaterThan(0)
    for (const name of names) expectQueryable(name, 'web_goals')
  })

  it('teaches only session dimensions the session schema can group by', () => {
    const names = harvestSessionDimensions()
    expect(names.length).toBeGreaterThan(0)
    for (const name of names) expectQueryable(name, 'web_sessions')
  })

  it('teaches attribution dimensions that really are on both sessions and goals', () => {
    const names = harvestAttributionDimensions()
    expect(names.length).toBeGreaterThan(0)
    for (const name of names) {
      expectQueryable(name, 'web_sessions')
      expectQueryable(name, 'web_goals')
    }
  })

  it('warns that a wrong-schema dimension rejects the whole query', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(
      /does not belong to the schema you queried makes the engine reject the WHOLE query/
    )
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/no partial result comes back/)
  })

  it('lists every measure each schema supports, under that schema', () => {
    for (const schema of SCHEMAS) {
      const start = WEB_ANALYTICS_AI_SYSTEM_PROMPT.indexOf(`### ${schema}`)
      expect(start, `no "### ${schema}" heading in the prompt`).toBeGreaterThan(-1)
      const rest = WEB_ANALYTICS_AI_SYSTEM_PROMPT.slice(start + 4)
      // Bound the section at the next heading of either level, so the last
      // schema's measures cannot be "found" in a later section of the prompt.
      const ends = ['\n### ', '\n## ']
        .map((heading) => rest.indexOf(heading))
        .filter((index) => index > -1)
      const section = ends.length > 0 ? rest.slice(0, Math.min(...ends)) : rest
      const measures = Object.keys(MEASURES_BY_SCHEMA[schema])
      expect(measures.length).toBeGreaterThan(0)
      for (const measure of measures) {
        expect(
          new RegExp(`\\b${measure}\\b`).test(section),
          `${schema} supports "${measure}" but the prompt never names it in that schema's section`
        ).toBe(true)
      }
    }
  })

  // previous_year is a PERIOD and vs_same_dates_last_year is a COMPARISON;
  // writing the former where the latter belongs silently reports on the wrong
  // window instead of erroring.
  it('separates the previous_year period from the same-dates-last-year comparison', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toContain('vs_same_dates_last_year')
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(
      /Do NOT write previous_year there: previous_year is a PERIOD/
    )
  })

  // Every dimension column is NOT NULL DEFAULT '', so a model reasoning about
  // nulls or a "(none)" bucket writes filters that match nothing.
  it('explains that an unknown value is the empty string, never null', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/NOT NULL DEFAULT ''/)
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/"is empty" is equality with ''/)
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/never expect a "\(none\)" bucket/)
  })

  // Without this the model reads the ingestion lag as a traffic collapse and
  // tells the operator their site just died.
  it('states the ingestion lag so a partial period is not read as a drop', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/60 to 70 seconds/)
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/"today" is always incomplete/)
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/ingestion lag, not the site/)
  })
})

describe('web-analytics-ai-system-prompt module', () => {
  // The prompt is read by the model, never rendered: running it through Lingui
  // would ship the operator's locale to the model and put the vocabulary the
  // tools enforce into a translator's hands.
  it('keeps the model-facing prompt out of the translation catalog', () => {
    const source = readFileSync(
      resolve(__dirname, 'web-analytics-ai-system-prompt.ts'),
      'utf8'
    )
    expect(source).not.toMatch(/@lingui/)
    expect(source).not.toMatch(/\bt`/)
    expect(source).not.toMatch(/\b(?:msg|defineMessage|useLingui|Trans)\b/)
  })
})

describe('serializeWebAnalyticsState', () => {
  it('redacts a withheld filter value while still reporting the filter is in force', () => {
    const line = lineStartingWith('Active filters', {
      filters: [
        { dimension: 'contact_email', operator: 'equals', values: ['someone@example.com'] }
      ]
    })
    expect(line).toContain('contact_email')
    expect(line).toContain('equals')
    expect(line).toContain(REDACTED_FILTER_VALUE)
    // The whole block, not just the filter line: the address must not reach the
    // model through any other line either.
    expect(serializeWebAnalyticsState({
      ...baseContext,
      filters: [
        { dimension: 'contact_email', operator: 'equals', values: ['someone@example.com'] }
      ]
    })).not.toContain('someone@example.com')
  })

  it('leaves an ordinary filter value untouched', () => {
    const line = lineStartingWith('Active filters', {
      filters: [
        { dimension: 'country', operator: 'in', values: ['FR', 'US'] },
        { dimension: 'device', operator: 'equals', values: ['mobile'] }
      ]
    })
    expect(line).toContain('country in FR, US')
    expect(line).toContain('device equals mobile')
    expect(line).toContain(' AND ')
    expect(line).not.toContain(REDACTED_FILTER_VALUE)
  })

  it('says filters are off rather than leaving the model to assume it', () => {
    expect(stateLines()).toContain('Active filters: none')
    expect(stateLines()).toContain('Active metric thresholds: none')
  })

  it('renders a metric threshold in force', () => {
    const line = lineStartingWith('Active metric thresholds', {
      metricFilters: [{ metric: 'sessions', operator: 'gte', values: [100] }]
    })
    expect(line).toContain('sessions gte 100')
  })

  it('renders a custom period with both of its bounds', () => {
    const line = lineStartingWith('Period:', {
      period: 'custom',
      customStart: '2026-01-01',
      customEnd: '2026-01-31'
    })
    expect(line).toBe('Period: custom (2026-01-01 to 2026-01-31)')
  })

  it('falls back to the bare preset name when no custom bounds are set', () => {
    expect(lineStartingWith('Period:')).toBe('Period: previous_7_days')
  })

  it('reports the resolved range as local calendar days', () => {
    expect(lineStartingWith('Resolved range:')).toBe(
      'Resolved range: 2026-08-08 to 2026-08-14 (local days)'
    )
  })

  it('reports comparison off when the operator turned it off', () => {
    expect(lineStartingWith('Comparison:', { comparison: 'none' })).toBe('Comparison: off')
  })

  // A comparison mode with no resolved window is not a comparison: reporting
  // the mode alone would have the model narrate a change against nothing.
  it('reports comparison off when no compare window resolved', () => {
    expect(lineStartingWith('Comparison:', { resolvedCompare: null })).toBe('Comparison: off')
  })

  it('reports the comparison window when one is active', () => {
    expect(lineStartingWith('Comparison:')).toBe(
      'Comparison: previous_period (2026-08-01 to 2026-08-07)'
    )
  })

  // bounce_rate is computed against this threshold, so a model told the wrong
  // one explains the number with the wrong definition.
  it('names the bounce threshold actually in force', () => {
    expect(lineStartingWith('Bounce threshold:', { bounceThresholdSeconds: 25 })).toBe(
      'Bounce threshold: 25s of engaged time'
    )
  })

  it('reports the granularity on screen and the ones this range allows', () => {
    expect(lineStartingWith('Chart granularity:')).toBe(
      'Chart granularity: day (available for this range: hour, day)'
    )
  })

  it('gives each install state its own note', () => {
    const notes = Object.values(INSTALL_STATE_NOTES)
    const states = Object.keys(INSTALL_STATE_NOTES) as InstallState[]
    expect(new Set(notes).size).toBe(states.length)
    for (const installState of states) {
      expect(lineStartingWith('Tracking:', { installState })).toBe(
        `Tracking: ${INSTALL_STATE_NOTES[installState]}`
      )
    }
  })

  // A dead install makes every number the assistant could report meaningless,
  // so those states have to read as a warning and not as a status word.
  it('flags the install states that make every number meaningless', () => {
    expect(lineStartingWith('Tracking:', { installState: 'never_received' })).toContain(
      'NOT INSTALLED'
    )
    expect(lineStartingWith('Tracking:', { installState: 'not_configured' })).toContain(
      'NOT CONFIGURED'
    )
    expect(lineStartingWith('Tracking:', { installState: 'disabled' })).toContain('DISABLED')
    expect(lineStartingWith('Tracking:', { installState: 'stalled' })).toContain('STALLED')
  })

  it('lists the workspace custom dimension labels when the workspace set them', () => {
    const line = lineStartingWith('Custom dimension labels:', {
      customDimensionLabels: { custom_1: 'Plan', custom_2: '', custom_3: 'Signup source' }
    })
    expect(line).toContain('custom_1 = Plan')
    expect(line).toContain('custom_3 = Signup source')
    // An empty slot is not a label; naming it would invent a dimension meaning.
    expect(line).not.toContain('custom_2')
  })

  it('omits the custom label line when no slot is labelled', () => {
    const emitted = serializeWebAnalyticsState({
      ...baseContext,
      customDimensionLabels: { custom_1: '' }
    })
    expect(emitted).not.toContain('Custom dimension labels')
  })

  it('reports the explore drill-down order, and says so when nothing is selected', () => {
    expect(lineStartingWith('Explore breakdown dimensions:')).toBe(
      'Explore breakdown dimensions: none selected'
    )
    expect(
      lineStartingWith('Explore breakdown dimensions:', { dimensions: ['country', 'device'] })
    ).toBe('Explore breakdown dimensions: country > device')
  })

  it('reports the minimum-sessions floor only when it actually filters rows out', () => {
    expect(serializeWebAnalyticsState(baseContext)).not.toContain('Minimum sessions')
    expect(lineStartingWith('Minimum sessions', { minSessions: 50 })).toBe(
      'Minimum sessions per breakdown row: 50'
    )
  })

  it('reports the attribution tag only while one is in view', () => {
    expect(serializeWebAnalyticsState(baseContext)).not.toContain('Attribution-rule tag')
    expect(lineStartingWith('Attribution-rule tag', { tag: 'paid' })).toBe(
      'Attribution-rule tag in view: paid'
    )
  })

  it('reports the tab, timezone and current instant the model must reason from', () => {
    const lines = stateLines({ tab: 'explore' })
    expect(lines).toContain('Tab: explore')
    expect(lines).toContain('Timezone: Europe/Paris')
    expect(lines).toContain('Now: 2026-08-15T09:30:00Z')
  })
})

describe('buildWebAnalyticsSystemPrompt', () => {
  it('appends the live dashboard state to the static template', () => {
    const built = buildWebAnalyticsSystemPrompt(baseContext)
    expect(built.startsWith(WEB_ANALYTICS_AI_SYSTEM_PROMPT)).toBe(true)
    expect(built).toContain(serializeWebAnalyticsState(baseContext))
    expect(built).toContain('# CURRENT DASHBOARD STATE')
  })

  // The state block is appended to the template, so a withheld filter value has
  // to stay redacted in the string that is actually sent.
  it('never ships a withheld filter value in the assembled prompt', () => {
    const built = buildWebAnalyticsSystemPrompt({
      ...baseContext,
      filters: [{ dimension: 'contact_email', operator: 'equals', values: ['vip@example.com'] }]
    })
    expect(built).not.toContain('vip@example.com')
    expect(built).toContain(REDACTED_FILTER_VALUE)
  })
})

describe('WEB_ANALYTICS_AI_SYSTEM_PROMPT output width', () => {
  // The panel is a fixed 420px, so roughly 330px of usable text. A model left to
  // its own devices reaches for a markdown table and picks the column count from
  // the data, not from the container - which is how a six-column breakdown ends up
  // as a strip the reader has to drag sideways. The rendering side of this is the
  // scroll wrapper in AIAssistantChat; this is the side that stops it happening.
  it('tells the model the reply is read in a narrow panel, not a document', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/narrow side panel/i)
  })

  it('caps a table at the column count the panel can actually show', () => {
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/TWO OR THREE columns/i)
    // The alternative has to be named, or "no wide table" reads as "no table".
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toMatch(/bullets instead/i)
  })

  it('sends a long breakdown to the explore report rather than into the reply', () => {
    // The tool exists and is wired; without this the model pastes twenty rows into
    // a bubble instead of opening the report the operator can sort and export.
    expect(WEB_ANALYTICS_AI_SYSTEM_PROMPT).toContain(WEB_TOOL_NAMES.SET_REPORT)
  })
})
