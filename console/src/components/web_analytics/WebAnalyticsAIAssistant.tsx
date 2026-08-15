import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import {
  BookOpen,
  CalendarRange,
  ChartColumn,
  ChartLine,
  FileText,
  Funnel,
  GitCompare,
  PanelsTopLeft,
  Table2
} from 'lucide-react'
import { AIAssistantChat, useAIAssistant } from '../ai-assistant'
import type { AIAssistantConfig, AIAssistantSuggestion } from '../ai-assistant'
import type { Workspace } from '../../services/api/workspace'
import { AnalyticsService } from '../../services/api/analytics'
import { useInstallStatus } from './lib/installStatus'
import { usePeriodLabels } from './toolbar'
import { PRIMARY_COLOR } from './lib/types'
import {
  WEB_ANALYTICS_AI_TOOLS,
  WEB_TOOL_NAMES,
  type PendingUiState
} from './web-analytics-ai-tools'
import {
  buildWebAnalyticsToolHandlers,
  type WebAnalyticsAiLabels
} from './web-analytics-ai-handlers'
import { buildWebAnalyticsSystemPrompt } from './web-analytics-ai-system-prompt'
import {
  DEFAULT_MIN_SESSIONS,
  useWebAnalytics,
  type WebAnalyticsSearch,
  type WebAnalyticsTab
} from './context'
import type { ComparisonMode, WebDimensionFilter } from './lib/types'

/**
 * One mark per tool, in TWO FAMILIES.
 *
 * The four reading tools carry chart and document glyphs; the four writing tools
 * carry the dashboard's own controls - a calendar, a funnel, a table, a panel - so
 * "asked your data a question" separates from "changed what is on your screen"
 * before the label is read at all. Sized to the marks the shared panel gives its own
 * server tools, so a mixed column stays one column.
 *
 * Module-level: an icon is not a translated string and nothing here depends on
 * render state.
 */
const WEB_TOOL_ICONS: Record<string, ReactNode> = {
  [WEB_TOOL_NAMES.QUERY]: <ChartColumn size={11} />,
  [WEB_TOOL_NAMES.COMPARE]: <GitCompare size={11} />,
  [WEB_TOOL_NAMES.SUMMARIZE]: <FileText size={11} />,
  [WEB_TOOL_NAMES.CATALOG]: <BookOpen size={11} />,
  [WEB_TOOL_NAMES.SET_PERIOD]: <CalendarRange size={11} />,
  [WEB_TOOL_NAMES.SET_FILTERS]: <Funnel size={11} />,
  [WEB_TOOL_NAMES.SET_REPORT]: <Table2 size={11} />,
  [WEB_TOOL_NAMES.NAVIGATE]: <PanelsTopLeft size={11} />
}

/**
 * Two tabs configure rather than read, and the assistant is hidden on both.
 *
 * The filters tab configures attribution rewrite rules rather than reading a
 * report: its gate runs in config mode, the period picker and filter bar are not
 * on the page at all, and "filter" there means a snake_case attribution rule, not
 * a camelCase query filter. Every tool the assistant owns would mutate state the
 * operator cannot see.
 *
 * The annotations tab is a CRUD list over rows the assistant can neither read nor
 * write: it has no period, no filter bar and no report, so the same argument holds
 * - and a panel floating over a table of edit buttons only covers them up.
 *
 * Both tabs are excluded from NAVIGABLE_TABS (web-analytics-ai-tools.ts), so
 * navigate_to_tab cannot send the operator to a place the panel is invisible.
 * The two rules are written independently and cross-checked by a test rather than
 * defined in terms of each other, so the check has something to catch.
 */
export function shouldHideAssistant(tab: WebAnalyticsTab): boolean {
  return tab === 'filters' || tab === 'annotations'
}

/**
 * The assistant's own query lane, module-scoped like the two clients in
 * lib/query.ts and created the same way.
 *
 * It must NOT share `webAnalyticsClient`: that client is what the visible
 * widgets queue on (lib/query.ts:24-27, maxConcurrency 4), and a
 * summarize_period fan-out of ~17 queries would put the operator's own
 * dashboard behind the assistant on the screen they are reading. Two lanes at
 * 2 leave the page responsive while the summary builds. The 60s TTL matches the
 * dashboard's, so a follow-up question that re-asks the same thing is free.
 *
 * What this does NOT buy: cancellation. AnalyticsService.query takes no
 * AbortSignal and its queue has no cancel path (services/api/analytics.ts
 * :160-206), so Stop and the tool timeout ABANDON the in-flight queries rather
 * than stopping them - the work still runs, its result is dropped. A private
 * lane is what keeps that abandoned work off the dashboard's queue.
 */
const assistantAnalyticsClient = AnalyticsService.create({
  maxConcurrency: 2,
  cacheTTL: 60_000
})

type UiStateChange = { tab?: WebAnalyticsTab; search?: Partial<WebAnalyticsSearch> }

/**
 * The URL carries dimension filters JSON-encoded; this is the same read
 * context.tsx:100-108 does when the router hands the params back, kept here
 * because that one is module-private and this is only the decode half of the
 * encode a handler performed one line earlier.
 */
function decodeFilters(raw: string | undefined): WebDimensionFilter[] {
  if (!raw) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as WebDimensionFilter[]) : []
  } catch {
    return []
  }
}

/** The filter identity rule the dashboard itself uses (context.tsx:110-116). */
function sameFilter(a: WebDimensionFilter, b: WebDimensionFilter): boolean {
  return (
    a.dimension === b.dimension &&
    a.operator === b.operator &&
    JSON.stringify([...a.values].sort()) === JSON.stringify([...b.values].sort())
  )
}

/**
 * Coalesces two writes made in the same tick, key by key.
 *
 * A shallow spread is right for every scalar - period, comparison, timezone,
 * dimensions, minSessions - because there the later write SUPERSEDES the earlier
 * one: the second handler computed its value against the first through the
 * pending overlay below, so last-write-wins is the state both acknowledgements
 * describe.
 *
 * `filters` is the exception, because it is a SET and two handlers of one round
 * can each contribute to it. Spreading drops the first while its ToolResult has
 * already told the model those filters are on the dashboard, so the two are
 * unioned instead - deduplicated, since the additive handler reads the overlay
 * and carries the earlier filters forward itself. An empty later write is a
 * clear rather than an absence ("Filters cleared" went to the model), so that
 * one wins outright.
 */
function mergeSearch(
  previous: Partial<WebAnalyticsSearch> | undefined,
  next: Partial<WebAnalyticsSearch> | undefined
): Partial<WebAnalyticsSearch> {
  const merged: Partial<WebAnalyticsSearch> = { ...(previous ?? {}), ...(next ?? {}) }
  if (!previous || !next || !('filters' in previous) || !('filters' in next)) return merged

  const incoming = decodeFilters(next.filters)
  if (incoming.length === 0) return merged

  const union = decodeFilters(previous.filters)
  for (const filter of incoming) {
    if (!union.some((candidate) => sameFilter(candidate, filter))) union.push(filter)
  }
  merged.filters = JSON.stringify(union)
  return merged
}

/**
 * Reads a staged navigation back out of the URL vocabulary the handlers encode
 * it in, into the dashboard vocabulary they compare against `deps.currentX`:
 * decoded filters, a dimension array, a real minSessions.
 *
 * Only keys the change actually carries appear, so a handler reading
 * `pending.X ?? deps.currentX` still falls through to the live dashboard for
 * everything the round has not touched.
 */
function toPendingUiState(change: {
  tab?: WebAnalyticsTab
  search: Partial<WebAnalyticsSearch>
}): PendingUiState {
  const { search } = change
  const pending: PendingUiState = {}
  if (change.tab) pending.tab = change.tab
  if (search.period !== undefined) pending.period = search.period
  if (search.comparison !== undefined) pending.comparison = search.comparison
  // The custom bounds are the one pair a handler clears on purpose - picking a
  // preset drops them, exactly as context.tsx's own setPeriod does - so here an
  // absent value is a value, not a key the round left alone.
  if ('customStart' in search) pending.customStart = search.customStart
  if ('customEnd' in search) pending.customEnd = search.customEnd
  if ('filters' in search) pending.filters = decodeFilters(search.filters)
  if ('dimensions' in search) {
    pending.dimensions = search.dimensions ? search.dimensions.split(',').filter(Boolean) : []
  }
  // An absent minSessions in the URL means the dashboard's default rather than
  // "unknown" (context.tsx:265), and handing the getter `undefined` would send the
  // handler back to the pre-change value through its own `??`.
  if ('minSessions' in search) pending.minSessions = search.minSessions ?? DEFAULT_MIN_SESSIONS
  return pending
}

export function WebAnalyticsAIAssistant(props: {
  workspace: Workspace
  tab: WebAnalyticsTab
}) {
  const { workspace, tab } = props
  const { t } = useLingui()
  const context = useWebAnalytics()
  const installState = useInstallStatus()
  const navigate = useNavigate()
  const periodLabels = usePeriodLabels()

  const config: AIAssistantConfig = {
    title: t`Analytics Assistant`,
    icon: <ChartLine size={18} />,
    iconButton: <ChartLine size={24} />,
    iconLarge: <ChartLine size={32} />,
    // The section's own accent (lib/types.ts:120), so the panel reads as part of
    // the dashboard rather than as a bolted-on chat widget.
    iconColor: PRIMARY_COLOR,
    avatarColor: PRIMARY_COLOR,
    placeholder: t`Ask about your traffic...`,
    // The period summary is a large tool result and the model answers over it in
    // prose. Kept at the service default rather than raised: DeepSeek-reasoner
    // rejects anything above 8192.
    maxTokens: 8192,
    notConfiguredGradient: `linear-gradient(135deg, ${PRIMARY_COLOR} 0%, #4f46e5 100%)`
  }

  // ---------------------------------------------------------------------------
  // The ONE place a tool-driven navigation happens.
  //
  // Every tool of a round runs synchronously inside the SSE callback, and two
  // navigations in one tick lose the first: the second search updater reads the
  // params from before the first landed (context.tsx:131-140). Merging into a
  // single deferred call is the same fix ExploreTab makes for its own two-setter
  // case (tabs/ExploreTab.tsx:324-340), generalised.
  // ---------------------------------------------------------------------------
  const pendingRef = useRef<{ tab?: WebAnalyticsTab; search: Partial<WebAnalyticsSearch> } | null>(
    null
  )

  // ---------------------------------------------------------------------------
  // The overlay every handler reads its effective state through.
  //
  // A round's tools are dispatched synchronously inside the SSE callback against
  // ONE frozen deps snapshot, and the navigation below is deferred on top of that.
  // So the batch the system prompt actively encourages - set_dashboard_period
  // beside query_web_analytics - would query the OLD window while its sibling
  // acknowledgement announces the new one. Staging every change here, and reading
  // it as `pending.X ?? deps.currentX`, is what makes the second tool of a round
  // see the first one's write.
  //
  // It must outlive the microtask that issues the navigation: the router commits
  // search state asynchronously, so it is cleared only once navigate() has
  // resolved - and only by the LAST call to stage into it, which the version
  // counter identifies.
  // ---------------------------------------------------------------------------
  const overlayRef = useRef<PendingUiState>({})
  const overlayVersionRef = useRef(0)

  // A getter, not a snapshot, and stable for the life of the panel: the handler
  // map is built once per deps change, long before any of its handlers run.
  const pendingUiState = useCallback(() => overlayRef.current, [])

  // The tab a tool has already navigated to but the router has not handed back as
  // a prop yet. Two tool frames can land in separate reader.read() iterations with
  // no macrotask between them, in which case the coalescer has already flushed and
  // the second call carries no tab of its own - and the render-time prop would send
  // the operator back to the tab they were on before navigate_to_tab moved them,
  // while the bubble says the section was opened.
  const appliedTabRef = useRef<WebAnalyticsTab | null>(null)
  useEffect(() => {
    // The prop is the router's answer and it outranks the ref the moment it
    // changes: either the tool's navigation committed, and the two now agree, or
    // the operator moved themselves, which supersedes anything a tool staged.
    appliedTabRef.current = null
  }, [tab])

  // It RETURNS A PROMISE, and every UI handler awaits it. Without that, the round's
  // tool promises settle while the navigation is still two hops away - the write is
  // deferred into a microtask here, and TanStack Router commits search state
  // asynchronously on top of that - so round 2's POST is issued before the router has
  // moved, and buildSystemPromptRef (refreshed on render, not on call) hands the model
  // the state from BEFORE its own UI tool ran. Awaiting the navigate makes the round
  // unable to settle first, which is the only ordering that makes the rebuilt prompt
  // mean anything.
  const applyUiState = useCallback(
    (change: UiStateChange): Promise<void> => {
      const merged = {
        tab: change.tab ?? pendingRef.current?.tab,
        search: mergeSearch(pendingRef.current?.search, change.search)
      }
      pendingRef.current = merged

      // Staged from the MERGED write, not from the raw change, so the overlay is
      // always the state the pending navigation will produce - a third handler
      // reading it sees the same filter set the URL is about to carry.
      const version = ++overlayVersionRef.current
      overlayRef.current = {
        ...overlayRef.current,
        ...toPendingUiState({ tab: merged.tab, search: merged.search })
      }

      // Mirror the two writes context.tsx's own setters make: the localStorage.setItem
      // calls inside setPeriod (context.tsx:266) and setComparison (context.tsx:276).
      // Going around setPeriod/setComparison to coalesce the navigation also goes
      // around their persistence, and the mount effect at :149-159 restores the stored
      // period whenever the URL names none - so without these two lines an AI-set
      // period is forgotten on reload and can be patched back over the URL. The keys
      // are module-private in context.tsx:51-52 and are repeated here deliberately
      // rather than exporting them: two string literals against a behavioural change
      // to a context every widget consumes.
      if (change.search?.period) localStorage.setItem('web_analytics_period', change.search.period)
      if (change.search?.comparison) {
        localStorage.setItem('web_analytics_comparison', change.search.comparison)
      }

      return new Promise<void>((resolve) => {
        queueMicrotask(() => {
          // A later call in the same tick supersedes this one; only the last wins, and
          // the superseded caller resolves at once because the change it asked for is
          // carried by the call that replaced it.
          if (pendingRef.current !== merged) return resolve()
          pendingRef.current = null
          // The prop is only the tab of the last COMMITTED render, so a tab an
          // earlier call of the same round already navigated to wins over it.
          const target = merged.tab ?? appliedTabRef.current ?? tab
          if (merged.tab) appliedTabRef.current = merged.tab
          resolve(
            Promise.resolve(
              navigate({
                to: '/console/workspace/$workspaceId/web-analytics/$tab',
                params: { workspaceId: context.workspaceId, tab: target },
                search: (previous: Record<string, unknown>) => ({ ...previous, ...merged.search }),
                replace: true
              })
            ).then(() => {
              // Only the last stager clears: a call made while this navigation was
              // in flight has already staged state the router still has not seen.
              if (overlayVersionRef.current === version) overlayRef.current = {}
            })
          )
        })
      })
    },
    [navigate, context.workspaceId, tab]
  )

  // ---------------------------------------------------------------------------
  // The step lines, read as a SET.
  //
  // Each one is a quiet caption under its own icon, not a sentence, so none of them
  // narrates ("Querying...", "Opened the...") - the icon already says what kind of
  // step it is. Two shapes, matching the two icon families: a reading step is
  // `subject - outcome`, a writing step is `what changed - its new value`. Both put
  // the varying half where the eye lands, which is the point of the redesign.
  // ---------------------------------------------------------------------------
  const labels = useMemo<WebAnalyticsAiLabels>(() => {
    const comparisonLabels: Record<ComparisonMode, string> = {
      // The comparison picker's own wording (toolbar.tsx), so the line and the
      // control it describes cannot disagree.
      previous_period: t`Previous period`,
      previous_year: t`Previous year`,
      none: t`No comparison`
    }
    // The headings WebAnalyticsPage puts at the top of each section.
    const sectionLabels: Record<WebAnalyticsTab, string> = {
      dashboard: t`Web Analytics`,
      explore: t`Explore`,
      goals: t`Goals`,
      filters: t`Filters`,
      annotations: t`Annotations`
    }
    return {
      // Rarely seen: antd renders the bubble's loading dots IN PLACE OF its content,
      // so this surfaces only when a stop uncovers a step still in flight. The bare
      // descriptor, so the line's identity does not shift sideways when the outcome
      // is appended to it.
      running: (what) => what,
      rows: (what, count) => (count === 1 ? t`${what} — 1 row` : t`${what} — ${count} rows`),
      cancelled: (what) => t`${what} — cancelled`,
      failed: (what) => t`${what} — failed`,
      series: (what, granularity) =>
        ({
          hour: t`${what} per hour`,
          day: t`${what} per day`,
          week: t`${what} per week`,
          month: t`${what} per month`,
          year: t`${what} per year`
        })[granularity],
      summary: () => t`Summary of ${periodLabels[context.period]}`,
      periodSet: (change) => {
        const parts: string[] = []
        if (change.period) {
          parts.push(
            change.period === 'custom' && change.customStart && change.customEnd
              ? `${change.customStart} → ${change.customEnd}`
              : periodLabels[change.period]
          )
        }
        if (change.comparison) parts.push(comparisonLabels[change.comparison])
        if (change.timezone) parts.push(change.timezone)
        const detail = parts.join(', ')
        // Named after the most significant thing the call actually wrote: a
        // timezone-only change is not a period change, and calling it one is a claim
        // the operator can check against the picker and find false.
        if (change.period) return t`Period — ${detail}`
        if (change.comparison) return t`Comparison — ${detail}`
        return t`Timezone — ${detail}`
      },
      filtersApplied: (count) =>
        count === 1 ? t`Filters — 1 applied` : t`Filters — ${count} applied`,
      filtersCleared: () => t`Filters — cleared`,
      reportOpened: (dimensions) => t`Report — ${dimensions}`,
      navigated: (section) => t`Section — ${sectionLabels[section]}`,
      catalogRead: () => t`Metrics and dimensions`
    }
  }, [t, periodLabels, context.period])

  const toolHandlers = useMemo(
    () =>
      buildWebAnalyticsToolHandlers({
        workspaceId: context.workspaceId,
        timezone: context.timezone,
        workspaceCreatedAt: workspace.created_at,
        currentPeriod: context.period,
        currentCustomStart: context.customStart,
        currentCustomEnd: context.customEnd,
        currentResolved: context.resolved,
        currentComparison: context.comparison,
        currentFilters: context.filters,
        currentGranularity: context.granularity,
        currentMinSessions: context.minSessions,
        currentDimensions: context.dimensions,
        currentTab: tab,
        customDimensionLabels: context.customDimensionLabels,
        query: (query) => assistantAnalyticsClient.query(query, context.workspaceId),
        applyUiState,
        // The `current*` fields above are a render-time snapshot; this is the
        // escape hatch for the state a sibling tool of the same round has already
        // changed but the router has not committed.
        pendingUiState,
        labels
      }),
    [context, workspace.created_at, tab, applyUiState, pendingUiState, labels]
  )

  const assistant = useAIAssistant({
    workspace,
    config,
    tools: WEB_ANALYTICS_AI_TOOLS,
    toolHandlers,
    toolIcons: WEB_TOOL_ICONS,
    buildSystemPrompt: () =>
      buildWebAnalyticsSystemPrompt({
        tab,
        installState,
        timezone: context.timezone,
        now: new Date().toISOString(),
        period: context.period,
        customStart: context.customStart,
        customEnd: context.customEnd,
        resolved: context.resolved,
        comparison: context.comparison,
        resolvedCompare: context.resolvedCompare,
        granularity: context.granularity,
        availableGranularities: context.availableGranularities,
        filters: context.filters,
        metricFilters: context.metricFilters,
        minSessions: context.minSessions,
        dimensions: context.dimensions,
        tag: context.tag,
        bounceThresholdSeconds: context.settings?.bounce_threshold_seconds ?? 10,
        customDimensionLabels: context.customDimensionLabels
      }),
    // summarize_period -> answer is 2 rounds; query -> schema correction -> answer
    // is 3; 4 leaves one round of slack under the hook's ceiling of 5.
    maxToolRounds: 4
  })

  const suggestions: AIAssistantSuggestion[] = [
    {
      key: 'summary',
      label: t`Summarise this period`,
      prompt: t`Summarise the current period and tell me what changed versus the comparison period.`
    },
    {
      key: 'change',
      label: t`Why did traffic change?`,
      prompt: t`Compare this period with the previous one and explain the biggest drivers of the change in sessions.`
    },
    {
      key: 'sources',
      label: t`Top traffic sources`,
      prompt: t`Which acquisition channels and campaigns brought the most sessions this period, and which ones grew or shrank?`
    },
    {
      key: 'pages',
      label: t`Best and worst pages`,
      prompt: t`Which landing pages bring the most sessions, and which ones have the worst bounce rate or engagement?`
    }
  ]

  // setInputValue is state, so handleSend's closure only sees the new text on the
  // NEXT render - calling both in one tick sends the previous value, which is
  // usually the empty string handleSend early-returns on. Sending is therefore
  // deferred to the render in which the composer actually holds the prompt.
  const [pendingPrompt, setPendingPrompt] = useState<string | null>(null)
  useEffect(() => {
    if (!pendingPrompt || assistant.inputValue !== pendingPrompt) return
    setPendingPrompt(null)
    void assistant.handleSend()
  }, [pendingPrompt, assistant])

  return (
    <AIAssistantChat
      {...assistant}
      workspace={workspace}
      config={config}
      hidden={shouldHideAssistant(tab)}
      // Wider than the 420 the prose assistants use: this one's answers carry small
      // metric tables, and 420 leaves ~330px of text once the padding and avatar are
      // taken out - not enough for "metric | now | change" without wrapping. Still
      // narrow enough to sit beside the dashboard rather than cover it.
      width={520}
      suggestions={suggestions}
      onSuggestion={(prompt) => {
        assistant.setInputValue(prompt)
        setPendingPrompt(prompt)
      }}
    />
  )
}
