import type { ToolBubbleHandle, ToolHandler } from '../ai-assistant'
import type { AnalyticsQuery, AnalyticsResponse } from '../../services/api/analytics'
import { areTimezonesLoaded, isValidTimezone } from '../../lib/timezones'
import { normalizeTimezone } from '../../lib/timezoneNormalizer'
import type { WebAnalyticsSearch, WebAnalyticsTab } from './context'
import { filtersForSchema, mergeWidgetFilters } from './lib/dimensions'
import { toNumber } from './lib/format'
import { buildWebQuery, mergeComparisonRows, type WebQueryOptions } from './lib/query'
import {
  DATE_PRESETS,
  type ComparisonMode,
  type DatePreset,
  type Granularity,
  type WebDimensionFilter
} from './lib/types'
import {
  assertDashboardFilterDimension,
  assertMeasures,
  assertOrderKey,
  assertQueryableDimension,
  bucketColumnFor,
  clampLimit,
  describeDimensions,
  describeFilters,
  describeQuery,
  dropBlockedFilters,
  fail,
  formatChangePercent,
  formatRows,
  MAX_SERIES_ROWS,
  NAVIGABLE_TABS,
  orderedColumns,
  parseComparisonMode,
  parseFilters,
  parseGranularity,
  parseMetricFilters,
  redactBlockedFilterValues,
  renderCatalog,
  renderCell,
  resolveComparisonRange,
  resolveSchema,
  resolveToolRange,
  SCHEMAS,
  TOOL_COMPARISON_CHOICES,
  WEB_TOOL_NAMES,
  withPendingDates,
  type PendingUiState,
  type ToolDateContext
} from './web-analytics-ai-tools'
import { buildPeriodSummary } from './web-analytics-insights'

/* ---------------------------------------------------------------------------
 * Everything the handlers need is INJECTED, so this whole file is testable with
 * a stub query runner, a recording applyUiState and a fake ctx - no hook, no
 * network, no provider.
 *
 * Not literally React-free: buildWebQuery lives in lib/query.ts, whose first
 * line is `import { useQueries, useQuery, … } from '@tanstack/react-query'`, so
 * the module graph pulls React Query in even though nothing here calls a hook.
 * That is inert in a test - but it is why the claim is worded this way, and why
 * WEB_ANALYTICS_TABS had to leave context.tsx: react-query is inert, the router
 * cycle is not.
 * ------------------------------------------------------------------------- */

export interface WebAnalyticsAiLabels {
  running: (what: string) => string
  rows: (what: string, count: number) => string
  cancelled: (what: string) => string
  failed: (what: string) => string
  /**
   * Wraps a descriptor when the query is bucketed. Here rather than inside
   * describeQuery because "per day" is a word, and every word an operator reads on a
   * step line is built with `t` by the component that owns the panel.
   */
  series: (what: string, granularity: Granularity) => string
  summary: () => string
  /**
   * The search params the call actually wrote, NOT the model-facing summary built
   * beside them: that one is a list of tokens ("period previous_7_days"), and the
   * operator's line names the period the way the date picker does.
   */
  periodSet: (change: Partial<WebAnalyticsSearch>) => string
  filtersApplied: (count: number) => string
  filtersCleared: () => string
  reportOpened: (dimensions: string) => string
  navigated: (tab: WebAnalyticsTab) => string
  catalogRead: () => string
}

export interface WebAnalyticsAiDeps extends ToolDateContext {
  workspaceId: string
  currentComparison: ComparisonMode
  /**
   * The dashboard's filter bar. Read by FOUR handlers, not one: query, compare and
   * summarize all apply it by default so their numbers match the charts on screen,
   * and set_dashboard_filters computes replace/add/clear against it.
   *
   * It is also UNTRUSTED for PII purposes: it is parsed out of the URL and the
   * console's FilterBuilder can put contact_email in it. Every handler that lets it
   * influence what the MODEL reads passes it through dropBlockedFilters (queries) or
   * redactBlockedFilterValues (acknowledgements) first.
   *
   * Never read directly: `pendingUiState().filters ?? currentFilters` is the bar as
   * of this round, so a query batched behind set_dashboard_filters is computed under
   * the filters its sibling just applied rather than the ones they replaced.
   */
  currentFilters: WebDimensionFilter[]
  currentGranularity: Granularity
  /** The explore report's row threshold; absent means the page shows every row. */
  currentMinSessions?: number
  /** The explore drill-down levels, broadest first, as the URL carries them. */
  currentDimensions?: string[]
  /** The section the operator is looking at. */
  currentTab?: WebAnalyticsTab
  /**
   * What a tool of THIS round has already asked the page for and the router has
   * not committed yet - read as `pending.X ?? deps.currentX` by every handler
   * whose answer depends on X.
   *
   * A GETTER, deliberately: the whole round is dispatched synchronously against
   * the one deps snapshot above, so a value read here at build time would be the
   * pre-round state again and the overlay would fix nothing. Every field is
   * optional and absent means "this round did not touch it".
   */
  pendingUiState: () => PendingUiState
  customDimensionLabels?: Record<string, string>
  /**
   * Runs one analytics query. Injected rather than imported so handler tests
   * need no network and no hook.
   *
   * Production passes the ASSISTANT'S OWN client, never `webAnalyticsClient`.
   * That singleton (lib/query.ts:24-27) is the one the visible widgets queue on,
   * at maxConcurrency 4: a summarize_period fan-out of ~17 queries would park
   * the operator's own dashboard behind the assistant, on the very screen they
   * are reading. A second AnalyticsService is the whole fix - it is the same
   * separation lib/query.ts already makes for the live view.
   */
  query: (query: AnalyticsQuery) => Promise<AnalyticsResponse>
  /**
   * The ONLY way a tool changes the page. Exactly one call per handler, and the
   * implementation coalesces calls made in the same tick into one navigation:
   * two navigations in one tick lose the first, because the second search
   * updater reads the params from before the first landed.
   *
   * It returns a promise that settles once the router has committed, and every
   * UI handler AWAITS it. A round whose promises settle before the navigation
   * lands would let the next round's system prompt be rebuilt from the state
   * before its own UI tool ran.
   */
  applyUiState: (change: {
    tab?: WebAnalyticsTab
    search?: Partial<WebAnalyticsSearch>
  }) => Promise<void>
  /** User-facing bubble text, built with `t` in the component (this file has no hook). */
  labels: WebAnalyticsAiLabels
}

/**
 * A time series: oldest bucket first, and DOWNSAMPLED rather than truncated.
 *
 * Neither half can be left to the engine or to formatRows.
 *
 * The order cannot be asked for. A series comes back under an ALIAS built from
 * the granularity (`created_at_day`), which the query validator refuses as an
 * order key because it is neither a measure nor a dimension of the schema, and
 * ordering by the bare `created_at` it is derived from is not groupable - so
 * buildWebQuery emits no ORDER BY for an ungrouped series (lib/query.ts:147-148
 * orders only what is grouped by a dimension) and the row order is whatever the
 * plan produced. Sorting here is the only place it can happen.
 *
 * And keeping the FIRST 200 of a 720-bucket hourly month drops the recent end,
 * which the model reads as traffic having stopped mid-period. Sampling across
 * the whole span instead - anchored on the newest bucket, so the end the operator
 * asked about is always present - is the trade the period summary already makes
 * (web-analytics-insights.ts renderSeries), and the note says which one happened.
 */
function renderSeriesRows(
  rows: Record<string, unknown>[],
  columns: string[],
  bucketColumn: string
): string {
  // Lexicographic IS chronological for both encodings a bucket arrives in - a
  // zero-padded calendar day and an RFC3339 instant - and a response never mixes
  // them, since the encoding follows from the granularity.
  const ordered = [...rows].sort((a, b) => {
    const left = String(a[bucketColumn] ?? '')
    const right = String(b[bucketColumn] ?? '')
    return left < right ? -1 : left > right ? 1 : 0
  })
  const stride = Math.ceil(ordered.length / MAX_SERIES_ROWS)
  const kept =
    stride > 1
      ? ordered.filter((_row, index) => (ordered.length - 1 - index) % stride === 0)
      : ordered
  return formatRows(kept, columns, {
    maxRows: MAX_SERIES_ROWS,
    note:
      stride > 1
        ? `note: downsampled, not truncated - one bucket in every ${stride} is shown, ` +
          `${kept.length} of ${ordered.length}, ending on the most recent`
        : undefined
  })
}

export function buildWebAnalyticsToolHandlers(deps: WebAnalyticsAiDeps): Map<string, ToolHandler> {
  const runQuery = (options: WebQueryOptions) => deps.query(buildWebQuery(options))

  const handleQuery: ToolHandler = async (event, _insert, ctx) => {
    const args = (event.tool_input ?? {}) as Record<string, unknown>
    let label = 'query'
    let bubble: ToolBubbleHandle | undefined
    try {
      const schema = resolveSchema(args.schema)
      const measures = assertMeasures(args.measures, schema)
      const dimensions = (Array.isArray(args.dimensions) ? args.dimensions : []).map((name) =>
        assertQueryableDimension(name, schema)
      )
      const granularity = parseGranularity(args.granularity)
      // The round's own writes first, then the page: a set_dashboard_period sibling
      // dispatched moments ago in this same round has changed the window the operator
      // is looking at, and answering under the old one would contradict the
      // acknowledgement the model already read.
      const pending = deps.pendingUiState()
      const { range, label: rangeLabel } = resolveToolRange(withPendingDates(deps, pending), args)

      // The dashboard's own filters apply BY DEFAULT, with the model's taking
      // precedence per dimension. mergeWidgetFilters (lib/dimensions.ts:198) is the
      // existing precedent for exactly this shape - a more specific filter replaces the
      // page filter on the same dimension rather than ANDing into an impossible
      // condition - and it narrows to the schema on the way through. Ignoring the
      // filter bar here would let query_web_analytics and summarize_period report
      // different numbers for the same period in one conversation, with the data tool
      // silently contradicting the chart on screen.
      const own = parseFilters(args.filters, schema)
      const filters =
        args.ignore_dashboard_filters === true
          ? filtersForSchema(own, schema)
          : mergeWidgetFilters(
              dropBlockedFilters(pending.filters ?? deps.currentFilters),
              own,
              schema
            )

      const metricFilters = parseMetricFilters(args.metric_filters, schema)
      // Not a pass-through: order_by is the one input no other helper validates.
      const orderBy =
        args.order_by === undefined
          ? undefined
          : assertOrderKey(args.order_by, measures, dimensions)
      const limit = clampLimit(args.limit)
      // A series with no grouping is the one query shape the engine returns in no
      // particular order and does not shorten; see renderSeriesRows.
      const isPureSeries = granularity !== undefined && dimensions.length === 0

      const what = describeQuery({ measures, dimensions, labels: deps.customDimensionLabels })
      label = granularity ? deps.labels.series(what, granularity) : what
      bubble = ctx.progress(deps.labels.running(label))

      const response = await runQuery({
        schema,
        measures,
        dimensions,
        range,
        granularity,
        filters,
        metricFilters,
        minSessions: toNumber(args.min_sessions) || undefined,
        order: orderBy ? { [orderBy]: args.order_direction === 'asc' ? 'asc' : 'desc' } : undefined,
        // A time series is one number per bucket, so it may exceed the breakdown
        // cap; a grouped one is ordered by its first measure and re-capped when
        // rendered. An UNGROUPED series carries no limit at all: it is not ordered,
        // so a limit would keep an arbitrary subset of the buckets rather than the
        // interesting ones - and the engine gap-fills the whole range afterwards, so
        // the clipped buckets come back as zeros instead of coming back short. The
        // row count is bounded by the period and the granularity instead, and the
        // payload by the downsampling in renderSeriesRows.
        limit: isPureSeries ? undefined : granularity ? MAX_SERIES_ROWS * 2 : limit,
        timezone: deps.timezone
      })

      if (ctx.signal.aborted) {
        bubble.update(deps.labels.cancelled(label))
        return
      }

      // Only `data` crosses into the formatter. meta.query / meta.params never do.
      const rows = response.data ?? []
      const bucketColumn = granularity ? bucketColumnFor(schema, granularity) : undefined
      const columns = orderedColumns({ bucket: bucketColumn, dimensions, measures })
      const body =
        bucketColumn && dimensions.length === 0
          ? renderSeriesRows(rows, columns, bucketColumn)
          : formatRows(rows, columns, { maxRows: granularity ? MAX_SERIES_ROWS : limit })
      bubble.update(deps.labels.rows(label, rows.length))
      // The header states the filter set actually applied, so the model can cite it
      // ("across mobile sessions only") instead of presenting a segment as the whole
      // site. It is also how the model notices a dashboard filter it did not choose.
      return {
        content:
          `${schema} | ${rangeLabel} | tz ${deps.timezone} | filters: ${describeFilters(filters)}\n${body}`
      }
    } catch (error) {
      bubble?.update(deps.labels.failed(label), { failed: true })
      return { content: error instanceof Error ? error.message : String(error), isError: true }
    }
  }

  const handleCompare: ToolHandler = async (event, _insert, ctx) => {
    const args = (event.tool_input ?? {}) as Record<string, unknown>
    let label = 'comparison'
    let bubble: ToolBubbleHandle | undefined
    try {
      const schema = resolveSchema(args.schema)
      const measures = assertMeasures(args.measures, schema)
      const requested = Array.isArray(args.dimensions) ? args.dimensions : []
      // One dimension at most: mergeComparisonRows joins on a SINGLE dimension value
      // (lib/query.ts:266-272), so a second grouping would silently collapse rows into
      // whichever one hashed last. Refused loudly instead.
      if (requested.length > 1) {
        fail('compare_periods takes at most one dimension; call it once per dimension instead')
      }
      const dimensions = requested.map((name) => assertQueryableDimension(name, schema))

      // Same-round state, same as handleQuery: both windows are derived from the
      // period a sibling may have just set, or the two tools of one round disagree
      // about which period "current" names.
      const pending = deps.pendingUiState()
      const dates = withPendingDates(deps, pending)
      const resolved = resolveToolRange(dates, args)
      const mode = parseComparisonMode(args.comparison, 'previous_period')
      // "off" is a set_dashboard_period token and is not in this tool's enum, but an
      // enum is a hint: a model that passes it anyway must be told what it did, not
      // told the period has no predecessor - which is a different, false statement.
      if (mode === 'none') {
        fail(
          `compare_periods always reports two windows; pass one of ${TOOL_COMPARISON_CHOICES.join(', ')}, ` +
            `or call ${WEB_TOOL_NAMES.QUERY} for a single window`
        )
      }
      const compareRange = resolveComparisonRange(dates, resolved, mode)
      // all_time has nothing before it, and neither has any window that precedes the
      // workspace. An error the model can narrate beats two windows silently reported
      // as "no change".
      if (!compareRange) {
        fail(`period "${resolved.preset}" has no window before it to compare against`)
      }

      // Identical filter policy to handleQuery, deliberately: the two tools answering
      // the same question under different filter sets is the failure this shares away.
      const own = parseFilters(args.filters, schema)
      const filters =
        args.ignore_dashboard_filters === true
          ? filtersForSchema(own, schema)
          : mergeWidgetFilters(
              dropBlockedFilters(pending.filters ?? deps.currentFilters),
              own,
              schema
            )
      const limit = clampLimit(args.limit)

      label = describeQuery({ measures, dimensions, labels: deps.customDimensionLabels })
      bubble = ctx.progress(deps.labels.running(label))

      // Both windows in one wave. They are independent, and the whole reason this tool
      // exists is that the model never sees them apart.
      const [currentResponse, previousResponse] = await Promise.all([
        runQuery({
          schema, measures, dimensions, filters,
          range: resolved.range, limit, timezone: deps.timezone
        }),
        runQuery({
          schema, measures, dimensions, filters,
          range: compareRange, limit, timezone: deps.timezone
        })
      ])

      if (ctx.signal.aborted) {
        bubble.update(deps.labels.cancelled(label))
        return
      }

      // Only `data` crosses into the formatters, here as everywhere.
      const current = currentResponse.data ?? []
      const previous = previousResponse.data ?? []

      let body: string
      let rowCount: number
      if (dimensions.length === 0) {
        // TRANSPOSED, like the insight battery's totals: one row per measure, so the
        // thing that moved reads down the page instead of across an ever-wider row.
        // The header is the packer's own construction and not a row key at all.
        const currentRow = current[0] ?? {}
        const previousRow = previous[0] ?? {}
        const lines = ['measure,current,previous,change_pct']
        for (const measure of measures) {
          const now = toNumber(currentRow[measure])
          const before = toNumber(previousRow[measure])
          lines.push(
            [measure, renderCell(now), renderCell(before), formatChangePercent(now, before)].join(',')
          )
        }
        rowCount = measures.length
        body = lines.join('\n')
      } else {
        const dimension = dimensions[0]
        const merged = mergeComparisonRows(current, previous, dimension, measures)
        // DECLARED columns, not the row's keys: mergeComparisonRows copies every key of
        // the source row and adds a prev_/_change pair per measure, so a three-measure
        // call arrives eleven keys wide (lib/query.ts:271-287). Printed width is decided
        // here.
        const columns = measures.flatMap((measure) => [
          measure,
          `prev_${measure}`,
          `${measure}_change`
        ])
        const lines = [[dimension, ...columns].join(',')]
        for (const row of merged.slice(0, limit)) {
          lines.push(
            [
              renderCell(row.dimension_value),
              ...columns.map((column) => {
                // Change cells are recomputed through the shared formatter rather than
                // printed from the row: the merge stores the raw quotient, and the
                // payload budget is counted in characters.
                if (!column.endsWith('_change')) return renderCell(row[column])
                const measure = column.slice(0, -'_change'.length)
                return formatChangePercent(toNumber(row[measure]), toNumber(row[`prev_${measure}`]))
              })
            ].join(',')
          )
        }
        if (merged.length > limit) {
          lines.push(`(showing first ${limit} of ${merged.length} rows)`)
        }
        rowCount = Math.min(merged.length, limit)
        body = lines.join('\n')
      }

      bubble.update(deps.labels.rows(label, rowCount))
      // Both windows named in the header, so the model cannot quietly attribute the
      // change to a window it never asked for - and the filter set stated once, since
      // it applied to both.
      return {
        content:
          `${schema} | current ${resolved.range.startDay}..${resolved.range.endDay} | ` +
          `previous ${compareRange.startDay}..${compareRange.endDay} (${mode}) | ` +
          `tz ${deps.timezone} | filters: ${describeFilters(filters)}\n${body}`
      }
    } catch (error) {
      bubble?.update(deps.labels.failed(label), { failed: true })
      return { content: error instanceof Error ? error.message : String(error), isError: true }
    }
  }

  const handleCatalog: ToolHandler = (event, insert) => {
    const raw = ((event.tool_input ?? {}) as Record<string, unknown>).schema
    insert(deps.labels.catalogRead(), WEB_TOOL_NAMES.CATALOG)
    const schemas = typeof raw === 'string' ? [resolveSchema(raw)] : SCHEMAS
    return { content: renderCatalog(schemas, deps.customDimensionLabels) }
  }

  const handleSummarize: ToolHandler = async (event, _insert, ctx) => {
    const args = (event.tool_input ?? {}) as Record<string, unknown>
    // The only translated string in this handler: the bubble is read by the operator.
    // Everything inside the report body is model-facing English produced by the
    // resolvers, so the model's parsing of "## by channel_group" never becomes
    // locale-dependent.
    const what = deps.labels.summary()
    const bubble = ctx.progress(what, WEB_TOOL_NAMES.SUMMARIZE)
    try {
      // The period is ALWAYS the dashboard's - including the one a sibling tool of
      // this round has just set. The tool takes no period argument precisely so a
      // summary cannot describe a window the operator is not looking at.
      const pending = deps.pendingUiState()
      const dates = withPendingDates(deps, pending)
      const resolved = resolveToolRange(dates, { period: 'current' })

      // Comparison is FORCED ON: "what changed?" has no answer without one, and the
      // dashboard's comparison is frequently off. Precedence: what the model asked
      // for, then what the dashboard is comparing to, then the preceding window.
      const dashboardComparison = pending.comparison ?? deps.currentComparison
      const requested = parseComparisonMode(
        args.comparison,
        dashboardComparison === 'none' ? 'previous_period' : dashboardComparison
      )
      // ...except all_time, which has nothing before it.
      const mode: ComparisonMode = resolved.preset === 'all_time' ? 'none' : requested
      const compareRange = resolveComparisonRange(dates, resolved, mode)

      const body = await buildPeriodSummary({
        timezone: deps.timezone,
        granularity: deps.currentGranularity,
        // Blocked dimensions are dropped BEFORE the battery narrows per schema, so a
        // contact_email filter someone put in the URL reaches neither a query nor the
        // report header.
        filters: dropBlockedFilters(pending.filters ?? deps.currentFilters),
        // Model-facing labels, built from the resolvers, never from `t`.
        periodLabel: `${resolved.preset} (${resolved.range.startDay}..${resolved.range.endDay})`,
        range: resolved.range,
        compareRange,
        compareLabel: compareRange
          ? `${mode} (${compareRange.startDay}..${compareRange.endDay})`
          : 'none (nothing precedes this range)',
        run: deps.query
      })

      if (ctx.signal.aborted) {
        bubble.update(deps.labels.cancelled(what))
        return
      }
      bubble.update(what)
      return { content: body }
    } catch (error) {
      bubble.update(deps.labels.failed(what), { failed: true })
      return { content: error instanceof Error ? error.message : String(error), isError: true }
    }
  }

  const handleSetReport: ToolHandler = async (event, insert) => {
    try {
      const args = (event.tool_input ?? {}) as Record<string, unknown>
      const pending = deps.pendingUiState()
      const dimensions = (Array.isArray(args.dimensions) ? args.dimensions : []).map((name) =>
        assertQueryableDimension(name, 'web_sessions')
      )
      if (dimensions.length === 0) fail('at least one dimension is required')
      if (dimensions.length > 4) fail('at most four drill-down levels')

      // ONLY the keys the model actually supplied are written. `filters` and
      // `min_sessions` are optional in the tool schema, and a search param written as
      // undefined is DROPPED by the route's validateSearch - so writing them
      // unconditionally silently wiped the operator's own filter bar and threshold
      // every time the model opened a report, while the acknowledgement went on
      // describing a segment that no longer existed. Clearing them is still possible,
      // but only by passing an empty array or a threshold of 1 on purpose.
      const search: Partial<WebAnalyticsSearch> = { dimensions: dimensions.join(',') }

      let filters = pending.filters ?? deps.currentFilters
      if (args.filters !== undefined) {
        filters = parseFilters(args.filters, 'web_sessions')
        search.filters = filters.length > 0 ? JSON.stringify(filters) : undefined
      }

      let minSessions = pending.minSessions ?? deps.currentMinSessions
      if (args.min_sessions !== undefined) {
        const requested = Math.floor(toNumber(args.min_sessions))
        // 1 or less hides nothing, which is how the dashboard itself encodes "no
        // threshold" (context.tsx:288) - so it is written as an absent param.
        minSessions = Number.isFinite(requested) && requested > 1 ? requested : undefined
        search.minSessions = minSessions
      }

      // The showcase for the single-write rule: a route param plus the search params
      // this call actually changes, applied by one navigation - and AWAITED, so this
      // round cannot settle (and start the next POST) before the router has committed.
      await deps.applyUiState({ tab: 'explore', search })

      // The operator's own names for the drill-down levels; the `>` chain below is the
      // model's copy and stays in ids.
      insert(
        deps.labels.reportOpened(describeDimensions(dimensions, deps.customDimensionLabels)),
        WEB_TOOL_NAMES.SET_REPORT
      )
      // silent: a UI acknowledgement rides along with a continuation but never buys
      // one of its own. The state is reported anyway, as a second channel beside the
      // rebuilt system prompt: the await above makes the prompt reliable, and this
      // makes it verifiable.
      //
      // The RESULTING state, not the arguments: what this call left untouched is
      // exactly what the model would otherwise describe wrongly. Redacted like every
      // other sentence built from the operator's own filter bar.
      return {
        content:
          `explore report opened: drill-down ${dimensions.join(' > ')}` +
          `, filters: ${describeFilters(redactBlockedFilterValues(filters))}` +
          `, minimum sessions per row: ${minSessions ?? 'none'}`,
        silent: true
      }
    } catch (error) {
      return { content: error instanceof Error ? error.message : String(error), isError: true }
    }
  }

  const handleSetPeriod: ToolHandler = async (event, insert) => {
    try {
      const args = (event.tool_input ?? {}) as Record<string, unknown>
      const search: Partial<WebAnalyticsSearch> = {}
      // Model-facing, and the operator's bubble is built from the same list: every
      // field actually changed, so a partial call cannot read as a full one.
      const parts: string[] = []

      if (args.period !== undefined) {
        const raw = String(args.period)
        if (raw === 'custom') {
          const start = typeof args.start_date === 'string' ? args.start_date.trim() : ''
          const end = typeof args.end_date === 'string' ? args.end_date.trim() : ''
          if (!/^\d{4}-\d{2}-\d{2}$/.test(start) || !/^\d{4}-\d{2}-\d{2}$/.test(end)) {
            fail('period "custom" needs start_date and end_date as YYYY-MM-DD')
          }
          if (end < start) fail(`end_date ${end} is before start_date ${start}`)
          search.period = 'custom'
          search.customStart = start
          search.customEnd = end
          parts.push(`period custom (${start}..${end})`)
        } else {
          // "current" is a query-time word, not a period the dashboard can be set to;
          // the tool's enum already excludes it, and this is the re-validation.
          if (raw === 'current' || !DATE_PRESETS.includes(raw as DatePreset)) {
            fail(
              `unknown period "${raw}"; expected one of ` +
                `${DATE_PRESETS.filter((preset) => preset !== 'custom').join(', ')}, or "custom" with dates`
            )
          }
          search.period = raw as DatePreset
          // Clearing the custom bounds is what context.tsx's own setPeriod does
          // (context.tsx:265-272) for the same reason: left behind, they make the date
          // picker display a range the period no longer means.
          search.customStart = undefined
          search.customEnd = undefined
          parts.push(`period ${raw}`)
        }
      }

      if (args.comparison !== undefined) {
        // "off" -> 'none' lives in the token map; the two comparison tokens map onto
        // the dashboard's own ComparisonMode, and "previous_year" is refused here as a
        // period wearing a comparison's clothes. The fallback is what THIS round has
        // set, so a second call in the same round does not undo the first.
        const mode = parseComparisonMode(
          args.comparison,
          deps.pendingUiState().comparison ?? deps.currentComparison
        )
        search.comparison = mode
        parts.push(`comparison ${mode}`)
      }

      if (args.timezone !== undefined) {
        const timezone = normalizeTimezone(String(args.timezone).trim())
        // VALID_TIMEZONES comes from window.TIMEZONES, served by /config.js
        // (lib/timezones.ts:26) - so it is EMPTY in a unit test and in any page loaded
        // without that script. Validating unconditionally would refuse every timezone
        // there; areTimezonesLoaded() is the guard the list ships with.
        if (areTimezonesLoaded() && !isValidTimezone(timezone)) {
          fail(`unknown timezone "${timezone}"; use an IANA name such as "Europe/Paris"`)
        }
        search.timezone = timezone
        parts.push(`timezone ${timezone}`)
      }

      if (parts.length === 0) {
        fail('set_dashboard_period needs at least one of period, comparison or timezone')
      }

      // ONE call, carrying up to four search params. The coalescer would merge two, but
      // a handler that navigates twice is a handler that can lose its own first write.
      await deps.applyUiState({ search })

      const summary = parts.join(', ')
      // `search`, not `summary`: the operator's line is built from the values the
      // dashboard was actually set to, so it can name them the way the pickers do.
      insert(deps.labels.periodSet(search), WEB_TOOL_NAMES.SET_PERIOD)
      return { content: `dashboard updated: ${summary}`, silent: true }
    } catch (error) {
      return { content: error instanceof Error ? error.message : String(error), isError: true }
    }
  }

  const handleSetFilters: ToolHandler = async (event, insert) => {
    try {
      const args = (event.tool_input ?? {}) as Record<string, unknown>
      const mode = args.mode === undefined ? 'replace' : String(args.mode)
      if (mode !== 'replace' && mode !== 'add' && mode !== 'clear') {
        fail(`unknown mode "${mode}"; expected replace, add or clear`)
      }
      // schema null: the filter bar is shared by every tab, so a filter is checked for
      // CATALOG membership rather than against one schema, and each widget narrows it
      // with filtersForSchema at read time (lib/dimensions.ts:183-188). parseFilters
      // still refuses a blocked dimension, so nothing the model wrote can be one.
      const own = mode === 'clear' ? [] : parseFilters(args.filters, null)
      if (mode !== 'clear' && own.length === 0) {
        fail(`mode "${mode}" needs at least one filter; use mode "clear" to remove them all`)
      }
      // ...but catalog membership is not enough for THIS bar. A dimension no visible
      // widget can group by is dropped by every one of them at read time, so the
      // screen does not change while the acknowledgement reports the filter as
      // applied. Refusing it with the usable scope named is what lets the model
      // correct itself instead of narrating a segment nobody is subject to.
      for (const filter of own) assertDashboardFilterDimension(filter.dimension)

      // "add" accumulates onto the bar as of THIS round, not as of the last committed
      // render: two set_dashboard_filters calls in one round both have to land, or the
      // second silently drops the first while its acknowledgement claims both.
      const base = deps.pendingUiState().filters ?? deps.currentFilters

      // Deliberately NOT context.toggleFilter (context.tsx:221-239): it REMOVES an
      // identical filter. Right for a click, catastrophic for a model that re-states a
      // filter it already set - the prompt tells it to do exactly that.
      const next =
        mode === 'clear'
          ? []
          : mode === 'replace'
            ? own
            : [
                // "add" is add-or-replace per dimension, the same precedence
                // mergeWidgetFilters encodes: two filters on one dimension AND into a
                // condition nothing matches, which renders as an empty dashboard.
                ...base.filter(
                  (existing) => !own.some((added) => added.dimension === existing.dimension)
                ),
                ...own
              ]

      await deps.applyUiState({
        search: { filters: next.length > 0 ? JSON.stringify(next) : undefined }
      })

      insert(
        next.length > 0 ? deps.labels.filtersApplied(next.length) : deps.labels.filtersCleared(),
        WEB_TOOL_NAMES.SET_FILTERS
      )
      // THE FOURTH PII DOOR, and the reason this handler is not "the same shape". In
      // "add" mode `next` carries the operator's OWN filter bar forward, and that bar
      // can hold `contact_email equals someone@example.com` - the console's FilterBuilder
      // offers the dimension and the URL carries it (context.tsx:201-204). The URL write
      // above keeps it, because dropping the operator's filter out of their own dashboard
      // would be the worse bug; the sentence handed to the MODEL is redacted, so it reads
      // the dimension and the operator and never the address.
      return {
        content:
          next.length > 0
            ? `dashboard filters (${mode}): ${describeFilters(redactBlockedFilterValues(next))}`
            : 'dashboard filters cleared',
        silent: true
      }
    } catch (error) {
      return { content: error instanceof Error ? error.message : String(error), isError: true }
    }
  }

  const handleNavigate: ToolHandler = async (event, insert) => {
    try {
      const args = (event.tool_input ?? {}) as Record<string, unknown>
      const tab = String(args.tab ?? '')
      if (!NAVIGABLE_TABS.includes(tab as WebAnalyticsTab)) {
        // `filters` is never offered by the tool's enum, but an enum is a hint: a model
        // that invents it must get an ANSWER, not a navigation to the one tab where the
        // assistant sets display:'none' on itself mid-turn.
        fail(
          `cannot open "${tab}"; the sections available are ${NAVIGABLE_TABS.join(', ')}` +
            (tab === 'filters'
              ? '. The attribution-rule filters are not one you can open - tell the operator where to find them instead.'
              : '')
        )
      }
      const target = tab as WebAnalyticsTab
      const pending = deps.pendingUiState()
      // Where the page is as of this round: a sibling that already opened this
      // section makes the navigation a no-op, and saying "now showing" twice invites
      // the model to narrate a move that did not happen.
      const wasAlreadyThere = (pending.tab ?? deps.currentTab) === target
      await deps.applyUiState({ tab: target })
      insert(deps.labels.navigated(target), WEB_TOOL_NAMES.NAVIGATE)
      // Explore renders whatever drill-down the URL carries, which is frequently
      // none at all; the model cannot see the screen, so the report it just opened
      // the operator onto is stated with it.
      const dimensions = pending.dimensions ?? deps.currentDimensions ?? []
      const report =
        target !== 'explore'
          ? ''
          : dimensions.length > 0
            ? `, grouped by ${dimensions.join(' > ')}`
            : `, with no drill-down configured yet - use ${WEB_TOOL_NAMES.SET_REPORT} to build one`
      return {
        content: `${wasAlreadyThere ? 'already' : 'now'} showing the ${tab} section${report}`,
        silent: true
      }
    } catch (error) {
      return { content: error instanceof Error ? error.message : String(error), isError: true }
    }
  }

  return new Map<string, ToolHandler>([
    [WEB_TOOL_NAMES.QUERY, handleQuery],
    [WEB_TOOL_NAMES.COMPARE, handleCompare],
    [WEB_TOOL_NAMES.SUMMARIZE, handleSummarize],
    [WEB_TOOL_NAMES.CATALOG, handleCatalog],
    [WEB_TOOL_NAMES.SET_PERIOD, handleSetPeriod],
    [WEB_TOOL_NAMES.SET_FILTERS, handleSetFilters],
    [WEB_TOOL_NAMES.SET_REPORT, handleSetReport],
    [WEB_TOOL_NAMES.NAVIGATE, handleNavigate]
  ])
}
