import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { ChartLine } from 'lucide-react'
import { AIAssistantChat, useAIAssistant } from '../ai-assistant'
import type { AIAssistantConfig, AIAssistantSuggestion } from '../ai-assistant'
import type { Workspace } from '../../services/api/workspace'
import { AnalyticsService } from '../../services/api/analytics'
import { useInstallStatus } from './lib/installStatus'
import { usePeriodLabels } from './toolbar'
import { PRIMARY_COLOR } from './lib/types'
import { WEB_ANALYTICS_AI_TOOLS } from './web-analytics-ai-tools'
import { buildWebAnalyticsToolHandlers } from './web-analytics-ai-handlers'
import { buildWebAnalyticsSystemPrompt } from './web-analytics-ai-system-prompt'
import { useWebAnalytics, type WebAnalyticsSearch, type WebAnalyticsTab } from './context'

/**
 * The filters tab configures attribution rewrite rules rather than reading a
 * report: its gate runs in config mode, the period picker and filter bar are not
 * on the page at all, and "filter" there means a snake_case attribution rule, not
 * a camelCase query filter. Every tool the assistant owns would mutate state the
 * operator cannot see.
 *
 * The same tab is excluded from NAVIGABLE_TABS (web-analytics-ai-tools.ts), so
 * navigate_to_tab cannot send the operator to the one place the panel is invisible.
 * The two are written independently and cross-checked by a test rather than defined
 * in terms of each other, so the check has something to catch.
 */
export function shouldHideAssistant(tab: WebAnalyticsTab): boolean {
  return tab === 'filters'
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

  // It RETURNS A PROMISE, and every UI handler awaits it. Without that, the round's
  // tool promises settle while the navigation is still two hops away - the write is
  // deferred into a microtask here, and TanStack Router commits search state
  // asynchronously on top of that - so round 2's POST is issued before the router has
  // moved, and buildSystemPromptRef (refreshed on render, not on call) hands the model
  // the state from BEFORE its own UI tool ran. Awaiting the navigate makes the round
  // unable to settle first, which is the only ordering that makes the rebuilt prompt
  // mean anything.
  const applyUiState = useCallback(
    (change: { tab?: WebAnalyticsTab; search?: Partial<WebAnalyticsSearch> }): Promise<void> => {
      const merged = {
        tab: change.tab ?? pendingRef.current?.tab,
        search: { ...(pendingRef.current?.search ?? {}), ...(change.search ?? {}) }
      }
      pendingRef.current = merged

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
          resolve(
            navigate({
              to: '/console/workspace/$workspaceId/web-analytics/$tab',
              params: { workspaceId: context.workspaceId, tab: merged.tab ?? tab },
              search: (previous: Record<string, unknown>) => ({ ...previous, ...merged.search }),
              replace: true
            })
          )
        })
      })
    },
    [navigate, context.workspaceId, tab]
  )

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
        customDimensionLabels: context.customDimensionLabels,
        query: (query) => assistantAnalyticsClient.query(query, context.workspaceId),
        applyUiState,
        labels: {
          running: (what) => t`Querying ${what}`,
          rows: (what, count) => t`${what} - ${count} rows`,
          cancelled: (what) => t`${what} - cancelled`,
          failed: (what) => t`${what} - failed`,
          summary: () => t`Summarising ${periodLabels[context.period]}`,
          periodSet: (summary) => t`Period set to ${summary}`,
          filtersApplied: (count) => t`${count} filters applied`,
          filtersCleared: () => t`Filters cleared`,
          reportOpened: (dimensions) => t`Report grouped by ${dimensions}`,
          navigated: (section) => t`Opened the ${section} section`,
          catalogRead: () => t`Reading the available metrics`
        }
      }),
    [context, workspace.created_at, applyUiState, periodLabels, t]
  )

  const assistant = useAIAssistant({
    workspace,
    config,
    tools: WEB_ANALYTICS_AI_TOOLS,
    toolHandlers,
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
      suggestions={suggestions}
      onSuggestion={(prompt) => {
        assistant.setInputValue(prompt)
        setPendingPrompt(prompt)
      }}
    />
  )
}
