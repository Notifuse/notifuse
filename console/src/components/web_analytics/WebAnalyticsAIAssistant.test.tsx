import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WebAnalyticsAIAssistant, shouldHideAssistant } from './WebAnalyticsAIAssistant'
import { isValidElement } from 'react'
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
import { WEB_ANALYTICS_AI_TOOLS, NAVIGABLE_TABS, WEB_TOOL_NAMES } from './web-analytics-ai-tools'
import type { WebAnalyticsAiLabels } from './web-analytics-ai-handlers'
import { WEB_ANALYTICS_TABS, type WebAnalyticsTab } from './lib/types'
import { llmApi, type LLMChatEvent } from '../../services/api/llm'
import type { Workspace } from '../../services/api/workspace'
import type { UseAIAssistantOptions } from '../ai-assistant'

// The Sender's auto-sizing textarea mounts a ResizeObserver; jsdom has none.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// Bubble.List watches a sentinel to decide whether it is scrolled to the bottom.
class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}
vi.stubGlobal('IntersectionObserver', IntersectionObserverStub)

// services/api/client imports the router, which imports every page and so cycles
// back into the module under test. Stubbing the client keeps that graph out.
vi.mock('../../services/api/client', () => ({
  api: { post: vi.fn().mockResolvedValue({}), get: vi.fn().mockResolvedValue({}) }
}))

vi.mock('../../services/api/llm', () => ({
  llmApi: { streamChat: vi.fn() }
}))

const {
  navigate,
  contextRef,
  buildHandlers,
  handlersMap,
  buildSystemPrompt,
  assistantOptions,
  analyticsQuery
} = vi.hoisted(() => ({
  navigate: vi.fn(),
  contextRef: { current: null as Record<string, unknown> | null },
  buildHandlers: vi.fn(),
  handlersMap: new Map(),
  // The parameter is declared even though the body ignores it: an untyped stub makes
  // mock.calls an empty tuple, so the assertion on the context this component builds
  // would not typecheck.
  buildSystemPrompt: vi.fn((_context: unknown) => 'SYSTEM PROMPT'),
  assistantOptions: { current: null as UseAIAssistantOptions | null },
  analyticsQuery: vi.fn()
}))

// The global mock in src/__tests__/setup.tsx returns a FRESH vi.fn() from every
// useNavigate() call, so the navigation applyUiState issues is unobservable there.
// Everything else about the router stays real: ./context is imported for real in
// the "outside the provider" case and pulls useSearch in with it.
vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>(
    '@tanstack/react-router'
  )
  return { ...actual, useNavigate: () => navigate, useMatch: () => false }
})

// Falls through to the REAL hook when no fixture is installed, so the
// no-provider case exercises the actual guard rather than a stubbed throw.
vi.mock('./context', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./context')>()
  return {
    ...actual,
    useWebAnalytics: () => contextRef.current ?? actual.useWebAnalytics()
  }
})

vi.mock('./lib/installStatus', () => ({
  useInstallStatus: () => 'ok'
}))

vi.mock('../../services/api/analytics', () => ({
  AnalyticsService: { create: () => ({ query: analyticsQuery }) }
}))

vi.mock('./web-analytics-ai-handlers', () => ({
  buildWebAnalyticsToolHandlers: (deps: Record<string, unknown>) => {
    buildHandlers(deps)
    return handlersMap
  }
}))

vi.mock('./web-analytics-ai-system-prompt', () => ({
  buildWebAnalyticsSystemPrompt: (context: unknown) => buildSystemPrompt(context)
}))

// Keeps the real chat panel and the real hook, and captures what the component
// asks the hook for.
vi.mock('../ai-assistant', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../ai-assistant')>()
  return {
    ...actual,
    useAIAssistant: (options: UseAIAssistantOptions) => {
      assistantOptions.current = options
      return actual.useAIAssistant(options)
    }
  }
})

const workspaceWithLLM = {
  id: 'ws1',
  name: 'My WS',
  created_at: '2024-03-04T00:00:00Z',
  integrations: [
    { id: 'llm1', name: 'Claude', type: 'llm', llm_provider: { kind: 'anthropic' } }
  ]
} as unknown as Workspace

const workspaceWithoutLLM = {
  id: 'ws1',
  name: 'My WS',
  created_at: '2024-03-04T00:00:00Z',
  integrations: []
} as unknown as Workspace

const filters = [{ dimension: 'country', operator: 'equals', values: ['FR'] }]

const makeContext = () => ({
  workspaceId: 'ws1',
  timezone: 'Europe/Paris',
  period: 'previous_7_days',
  comparison: 'previous_period',
  customStart: undefined,
  customEnd: undefined,
  resolved: { start: '2024-05-01', end: '2024-05-07' },
  resolvedCompare: { start: '2024-04-24', end: '2024-04-30' },
  granularity: 'day',
  availableGranularities: ['hour', 'day'],
  filters,
  metricFilters: [],
  minSessions: 10,
  dimensions: ['country'],
  tag: undefined,
  customDimensionLabels: { custom1: 'Plan' },
  settings: { bounce_threshold_seconds: 10 }
})

const assistantTree = (overrides: { workspace?: Workspace; tab?: WebAnalyticsTab } = {}) => (
  <I18nProvider i18n={i18n}>
    <ConfigProvider>
      <App>
        <WebAnalyticsAIAssistant
          workspace={overrides.workspace ?? workspaceWithLLM}
          tab={overrides.tab ?? 'dashboard'}
        />
      </App>
    </ConfigProvider>
  </I18nProvider>
)

const renderAssistant = (overrides: { workspace?: Workspace; tab?: WebAnalyticsTab } = {}) =>
  render(assistantTree(overrides))

/** Opens the floating panel; the FAB is the only circle button on the page. */
const openPanel = (container: HTMLElement) => {
  const fab = container.querySelector<HTMLElement>('.ant-btn-circle')
  expect(fab, 'the floating trigger should be rendered').not.toBeNull()
  fireEvent.click(fab as HTMLElement)
}

/** Lets every queued microtask (and the queueMicrotask deferral) run. */
const flush = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)) })

beforeEach(() => {
  navigate.mockReset()
  buildHandlers.mockClear()
  buildSystemPrompt.mockClear()
  vi.mocked(llmApi.streamChat).mockReset()
  window.localStorage.clear()
  contextRef.current = makeContext()
  assistantOptions.current = null
  // Empty catalog: the Lingui macro mock falls back to the source text.
  i18n.loadAndActivate({ locale: 'en', messages: {} })
})

describe('WebAnalyticsAIAssistant wiring', () => {
  it('tells an operator with no LLM integration how to configure one', () => {
    const { container } = renderAssistant({ workspace: workspaceWithoutLLM })
    openPanel(container)

    expect(screen.getByText('AI Assistant Not Configured')).toBeInTheDocument()
    expect(screen.getByText('Analytics Assistant')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Configure Integration' })).toHaveAttribute(
      'href',
      '/console/workspace/ws1/settings/integrations'
    )
  })

  it('gives the model the web analytics tools and handlers built from the live dashboard state', () => {
    renderAssistant()

    expect(assistantOptions.current?.tools).toBe(WEB_ANALYTICS_AI_TOOLS)
    expect(assistantOptions.current?.toolHandlers).toBe(handlersMap)

    const deps = buildHandlers.mock.calls.at(-1)?.[0]
    expect(deps).toMatchObject({
      workspaceId: 'ws1',
      timezone: 'Europe/Paris',
      workspaceCreatedAt: '2024-03-04T00:00:00Z',
      currentPeriod: 'previous_7_days',
      currentComparison: 'previous_period',
      currentGranularity: 'day',
      customDimensionLabels: { custom1: 'Plan' }
    })
    // The live array itself, not a copy: a stale snapshot would have the model
    // reasoning about filters the operator already removed.
    expect(deps.currentFilters).toBe(filters)
    expect(deps.currentMinSessions).toBe(10)
    expect(deps.currentDimensions).toBe(contextRef.current?.dimensions)
    expect(deps.currentTab).toBe('dashboard')
    expect(typeof deps.query).toBe('function')
    expect(typeof deps.applyUiState).toBe('function')
    // A getter, never a snapshot: it is read when a handler runs, which is after
    // the sibling tools of the same round have already staged their changes.
    expect(typeof deps.pendingUiState).toBe('function')
  })

  it('routes its queries through a lane of its own, away from the dashboard widgets', () => {
    renderAssistant()
    const deps = buildHandlers.mock.calls.at(-1)?.[0]

    deps.query({ schema: 'web_sessions' })
    expect(analyticsQuery).toHaveBeenCalledWith({ schema: 'web_sessions' }, 'ws1')
  })

  it('lets the model answer over its own tool output instead of stopping at the first round', () => {
    renderAssistant()
    // A query whose result the model never sees is a query it cannot explain.
    expect(assistantOptions.current?.maxToolRounds).toBeGreaterThan(1)
  })

  it('builds the system prompt from the tab and the dashboard state', () => {
    renderAssistant({ tab: 'explore' })

    expect(assistantOptions.current?.buildSystemPrompt()).toBe('SYSTEM PROMPT')
    expect(buildSystemPrompt.mock.calls.at(-1)?.[0]).toMatchObject({
      tab: 'explore',
      installState: 'ok',
      timezone: 'Europe/Paris',
      period: 'previous_7_days',
      bounceThresholdSeconds: 10
    })
  })

  it('refuses to mount outside the web analytics provider rather than half-working', () => {
    contextRef.current = null
    // React re-throws the render error after logging it.
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      expect(() => renderAssistant()).toThrow(/WebAnalyticsProvider/)
    } finally {
      consoleError.mockRestore()
    }
  })
})

describe('WebAnalyticsAIAssistant translated surfaces', () => {
  it('shows the operator translated chrome, not the English source strings', () => {
    // The config object and the chips are built inside the component so they go
    // through the macro on every render; a module-level constant would freeze the
    // English text for every locale.
    i18n.loadAndActivate({
      locale: 'en',
      messages: {
        'Analytics Assistant': 'Assistant analytique',
        'Ask about your traffic...': 'Parlez-moi de votre trafic',
        'Summarise this period': 'Resumer cette periode'
      }
    })

    const { container } = renderAssistant()
    openPanel(container)

    expect(screen.getByText('Assistant analytique')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Parlez-moi de votre trafic')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Resumer cette periode' })).toBeInTheDocument()
    expect(screen.queryByText('Analytics Assistant')).not.toBeInTheDocument()
  })
})

describe('WebAnalyticsAIAssistant suggestion chips', () => {
  it('sends a chip prompt exactly once, with the chip text as the message', async () => {
    let sentText = ''
    vi.mocked(llmApi.streamChat).mockImplementation(async (params, onEvent) => {
      sentText = String(params.messages.at(-1)?.content ?? '')
      onEvent({ type: 'text', content: 'Sessions are up 12%.' } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { container } = renderAssistant()
    openPanel(container)

    fireEvent.click(screen.getByRole('button', { name: 'Summarise this period' }))

    // A second send would double-bill the operator for one click.
    await waitFor(() => expect(llmApi.streamChat).toHaveBeenCalledTimes(1))
    await flush()
    expect(llmApi.streamChat).toHaveBeenCalledTimes(1)
    expect(sentText).toContain('Summarise the current period')
  })

  it('stops offering starters once the conversation has content', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'Sessions are up 12%.' } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { container } = renderAssistant()
    openPanel(container)
    expect(screen.getByRole('button', { name: 'Top traffic sources' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Summarise this period' }))

    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Top traffic sources' })).not.toBeInTheDocument()
    )
    expect(screen.getByText('Sessions are up 12%.')).toBeInTheDocument()
  })
})

describe('WebAnalyticsAIAssistant applyUiState', () => {
  const getApplyUiState = () => {
    renderAssistant()
    return buildHandlers.mock.calls.at(-1)?.[0].applyUiState as (change: {
      tab?: WebAnalyticsTab
      search?: Record<string, unknown>
    }) => Promise<void>
  }

  it('issues a single navigation for two UI tools that ran in the same round', async () => {
    const applyUiState = getApplyUiState()

    const first = applyUiState({ search: { period: 'previous_30_days' } })
    const second = applyUiState({ tab: 'explore', search: { dimensions: 'country' } })
    await act(async () => {
      await Promise.all([first, second])
    })

    // Two navigations in one tick lose the first: the second search updater reads
    // the params from before the first landed.
    expect(navigate).toHaveBeenCalledTimes(1)
    const call = navigate.mock.calls[0][0]
    expect(call.params).toEqual({ workspaceId: 'ws1', tab: 'explore' })
    expect(call.replace).toBe(true)
    expect(call.search({})).toEqual({ period: 'previous_30_days', dimensions: 'country' })
  })

  it('keeps the tab the operator is on when no tool asked to move', async () => {
    renderAssistant({ tab: 'goals' })
    const applyUiState = buildHandlers.mock.calls.at(-1)?.[0].applyUiState

    await act(async () => {
      await applyUiState({ search: { period: 'today' } })
    })

    expect(navigate.mock.calls[0][0].params).toEqual({ workspaceId: 'ws1', tab: 'goals' })
  })

  it('leaves search params it was not given untouched', async () => {
    const applyUiState = getApplyUiState()

    await act(async () => {
      await applyUiState({ search: { period: 'today' } })
    })

    // Setting a period must not drop the timezone or the filters the operator chose.
    const updated = navigate.mock.calls[0][0].search({
      timezone: 'Europe/Paris',
      filters: '[{"dimension":"country"}]'
    })
    expect(updated).toEqual({
      timezone: 'Europe/Paris',
      filters: '[{"dimension":"country"}]',
      period: 'today'
    })
  })

  it('does not resolve before the navigation has been issued', async () => {
    const applyUiState = getApplyUiState()

    let release: () => void = () => {}
    navigate.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          release = resolve
        })
    )

    let settled = false
    const pending = applyUiState({ search: { period: 'today' } }).then(() => {
      settled = true
    })

    await flush()
    expect(navigate).toHaveBeenCalledTimes(1)
    // A handler that settles first lets round 2 rebuild the system prompt from the
    // state that existed BEFORE its own UI tool ran.
    expect(settled).toBe(false)

    release()
    await act(async () => {
      await pending
    })
    expect(settled).toBe(true)
  })

  it('persists an AI-set period and comparison where the dashboard looks for them on reload', async () => {
    const applyUiState = getApplyUiState()

    await act(async () => {
      await applyUiState({
        search: { period: 'previous_30_days', comparison: 'previous_year' }
      })
    })

    // The mount effect restores the stored period whenever the URL names none, so
    // without these writes an AI-set period is forgotten and patched back over.
    expect(window.localStorage.getItem('web_analytics_period')).toBe('previous_30_days')
    expect(window.localStorage.getItem('web_analytics_comparison')).toBe('previous_year')
  })

  it('does not overwrite the stored period when the change carries none', async () => {
    window.localStorage.setItem('web_analytics_period', 'previous_90_days')
    const applyUiState = getApplyUiState()

    await act(async () => {
      await applyUiState({ tab: 'explore', search: { dimensions: 'country' } })
    })

    expect(window.localStorage.getItem('web_analytics_period')).toBe('previous_90_days')
  })
})

describe('WebAnalyticsAIAssistant pending UI state', () => {
  const country = { dimension: 'country', operator: 'equals', values: ['FR'] }

  const getDeps = () => buildHandlers.mock.calls.at(-1)?.[0]
  const getApplyUiState = () =>
    getDeps().applyUiState as (change: {
      tab?: WebAnalyticsTab
      search?: Record<string, unknown>
    }) => Promise<void>
  const getPending = () => getDeps().pendingUiState() as Record<string, unknown>

  /** navigate() left in flight, so the overlay is observed while it still matters. */
  const holdNavigation = () => {
    let release: () => void = () => {}
    navigate.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          release = resolve
        })
    )
    return () => release()
  }

  it('gives the handlers the same getter on every render', () => {
    const { rerender } = renderAssistant()
    const first = getDeps().pendingUiState

    rerender(assistantTree())

    // The handler map is rebuilt whenever the deps change; a getter that changed
    // identity per render would make that rebuild unconditional.
    expect(getDeps().pendingUiState).toBe(first)
  })

  it('stages a period change where a sibling tool of the same round can read it', async () => {
    renderAssistant()
    const release = holdNavigation()
    const applyUiState = getApplyUiState()

    expect(getPending()).toEqual({})
    void applyUiState({
      search: { period: 'previous_30_days', customStart: undefined, customEnd: undefined }
    })

    // Read BEFORE the navigation commits: this is the window in which a batched
    // query_web_analytics would otherwise resolve against the old period.
    expect(getPending().period).toBe('previous_30_days')
    // Picking a preset clears the custom bounds, and the getter has to carry the
    // clear - not the absence of one - or the handler falls back to stale dates.
    expect(getPending()).toStrictEqual({
      period: 'previous_30_days',
      customStart: undefined,
      customEnd: undefined
    })

    release()
    await flush()
  })

  it('stages filters, dimensions and a tab in the vocabulary the handlers compare against', async () => {
    renderAssistant()
    const release = holdNavigation()
    const applyUiState = getApplyUiState()

    void applyUiState({
      tab: 'explore',
      search: {
        dimensions: 'country,device_type',
        filters: JSON.stringify([country]),
        minSessions: 25
      }
    })

    // Decoded, not the URL's JSON string and comma list: the handlers diff this
    // against deps.currentFilters / deps.currentDimensions.
    expect(getPending()).toStrictEqual({
      tab: 'explore',
      dimensions: ['country', 'device_type'],
      filters: [country],
      minSessions: 25
    })

    release()
    await flush()
  })

  it('stages a cleared filter bar as an empty list and a dropped minSessions as the dashboard default', async () => {
    renderAssistant()
    const release = holdNavigation()
    const applyUiState = getApplyUiState()

    void applyUiState({ search: { filters: undefined, minSessions: undefined } })

    // undefined here would send the handler back through its own `??` to the
    // filters the operator had before the tool cleared them.
    expect(getPending().filters).toEqual([])
    // The dashboard reads an absent minSessions as 10 (context.tsx's default), so
    // the overlay has to say 10 rather than "unknown".
    expect(getPending().minSessions).toBe(10)

    release()
    await flush()
  })

  it('holds the staged state until the navigation has actually committed', async () => {
    renderAssistant()
    const release = holdNavigation()
    const applyUiState = getApplyUiState()

    const pending = applyUiState({ search: { period: 'today' } })
    await flush()

    // The router commits search state asynchronously; clearing on the microtask
    // that issued the navigation would reopen the staleness window.
    expect(getPending().period).toBe('today')

    release()
    await act(async () => {
      await pending
    })
    expect(getPending()).toEqual({})
  })

  it('keeps state staged by a later call when an earlier navigation commits', async () => {
    renderAssistant()
    const release = holdNavigation()
    const applyUiState = getApplyUiState()

    const first = applyUiState({ search: { period: 'today' } })
    await flush()
    void applyUiState({ search: { comparison: 'previous_year' } })

    release()
    await act(async () => {
      await first
    })

    // The second change has not reached the router either; the call that staged it
    // is the one that gets to clear it.
    expect(getPending().comparison).toBe('previous_year')
  })

  it('does not send the operator back to the tab a previous tool already left', async () => {
    renderAssistant({ tab: 'dashboard' })
    const applyUiState = getApplyUiState()

    // Two tool frames from separate reader.read() iterations: the coalescer has
    // already flushed, so the second change carries no tab of its own.
    await act(async () => {
      await applyUiState({ tab: 'explore' })
    })
    await act(async () => {
      await applyUiState({ search: { period: 'today' } })
    })

    expect(navigate).toHaveBeenCalledTimes(2)
    // The render-time prop is still 'dashboard' - navigating on it would undo the
    // move the model has already told the operator about.
    expect(navigate.mock.calls[1][0].params).toEqual({ workspaceId: 'ws1', tab: 'explore' })
  })

  it('yields to the tab the operator picked after the tool navigation landed', async () => {
    const { rerender } = renderAssistant({ tab: 'dashboard' })

    await act(async () => {
      await getApplyUiState()({ tab: 'explore' })
    })
    // The router hands the committed tab back as a prop; the operator then clicks
    // another tab themselves.
    rerender(assistantTree({ tab: 'goals' }))

    await act(async () => {
      await getApplyUiState()({ search: { period: 'today' } })
    })

    expect(navigate.mock.calls.at(-1)?.[0].params).toEqual({ workspaceId: 'ws1', tab: 'goals' })
  })
})

describe('WebAnalyticsAIAssistant same-tick coalescing', () => {
  const country = { dimension: 'country', operator: 'equals', values: ['FR'] }
  const mobile = { dimension: 'device_type', operator: 'equals', values: ['mobile'] }

  const getApplyUiState = () =>
    buildHandlers.mock.calls.at(-1)?.[0].applyUiState as (change: {
      tab?: WebAnalyticsTab
      search?: Record<string, unknown>
    }) => Promise<void>

  const searchAfterNavigation = () =>
    navigate.mock.calls[0][0].search({}) as Record<string, unknown>

  it('keeps both filter writes when two tools of one round touch the bar', async () => {
    renderAssistant()
    const applyUiState = getApplyUiState()

    // set_dashboard_filters and set_explore_report both write the bar, and the
    // second builds its list from its own arguments rather than from the first.
    const first = applyUiState({ search: { filters: JSON.stringify([country]) } })
    const second = applyUiState({ search: { filters: JSON.stringify([mobile]) } })

    // A third handler of the same round has to read the bar the navigation will
    // actually carry, not just the last write.
    expect(buildHandlers.mock.calls.at(-1)?.[0].pendingUiState().filters).toEqual([country, mobile])

    await act(async () => {
      await Promise.all([first, second])
    })

    expect(navigate).toHaveBeenCalledTimes(1)
    // Spreading would drop the first write while its ToolResult already told the
    // model the filter was applied.
    expect(JSON.parse(String(searchAfterNavigation().filters))).toEqual([country, mobile])
  })

  it('counts a filter both writes carry once', async () => {
    renderAssistant()
    const applyUiState = getApplyUiState()

    const first = applyUiState({ search: { filters: JSON.stringify([country]) } })
    // The additive handler read the pending overlay, so it carries the first
    // filter forward itself - the union must not double it.
    const second = applyUiState({ search: { filters: JSON.stringify([country, mobile]) } })
    await act(async () => {
      await Promise.all([first, second])
    })

    expect(JSON.parse(String(searchAfterNavigation().filters))).toEqual([country, mobile])
  })

  it('lets an explicit clear win over a filter staged earlier in the same tick', async () => {
    renderAssistant()
    const applyUiState = getApplyUiState()

    const first = applyUiState({ search: { filters: JSON.stringify([country]) } })
    // "Filters cleared" went to the model; resurrecting the earlier filter would
    // make that acknowledgement false.
    const second = applyUiState({ search: { filters: undefined } })
    await act(async () => {
      await Promise.all([first, second])
    })

    expect(searchAfterNavigation().filters).toBeUndefined()
  })

  it('lets the later value win for a scalar written twice in one tick', async () => {
    renderAssistant()
    const applyUiState = getApplyUiState()

    const first = applyUiState({ search: { period: 'today' } })
    const second = applyUiState({ search: { period: 'previous_30_days' } })
    await act(async () => {
      await Promise.all([first, second])
    })

    // A period is not a set: the second tool changed its mind, and its
    // acknowledgement is the one describing the dashboard.
    expect(searchAfterNavigation().period).toBe('previous_30_days')
  })
})

describe('shouldHideAssistant', () => {
  it('keeps the assistant off the attribution-rules tab', () => {
    // The filters tab has no period picker and no query filters; every tool would
    // mutate state the operator cannot see.
    expect(shouldHideAssistant('filters')).toBe(true)
  })

  it('keeps the assistant off the annotations tab', () => {
    // Annotations is a CRUD list, not a report: no period, no filter bar, nothing
    // any tool could read or write, and the floating panel would sit over the row
    // actions.
    expect(shouldHideAssistant('annotations')).toBe(true)
  })

  it('offers the assistant on every tab that shows a report', () => {
    const shown = WEB_ANALYTICS_TABS.filter((tab) => !shouldHideAssistant(tab))
    expect(shown).toEqual(['dashboard', 'explore', 'goals'])
  })

  it('never sends the model to a tab where the panel is invisible', () => {
    // navigate_to_tab's enum and the visibility rule live in different modules and
    // are written independently; a future hidden tab must not stay navigable.
    expect(NAVIGABLE_TABS.length).toBeGreaterThan(0)
    expect(NAVIGABLE_TABS.filter((tab) => shouldHideAssistant(tab))).toEqual([])
  })

  it('does not float the trigger over the attribution-rules tab', () => {
    const { container } = renderAssistant({ tab: 'filters' })
    expect(container.querySelector('.ant-btn-circle')).toBeNull()
  })

  it('floats the trigger on a report tab', () => {
    const { container } = renderAssistant({ tab: 'dashboard' })
    expect(container.querySelector('.ant-btn-circle')).not.toBeNull()
  })
})

/** The step-line labels the component hands the handlers, as a handler sees them. */
const stepLabels = (): WebAnalyticsAiLabels => {
  const deps = buildHandlers.mock.calls.at(-1)?.[0] as { labels: WebAnalyticsAiLabels } | undefined
  if (!deps) throw new Error('the component never built its tool handlers')
  return deps.labels
}

describe('WebAnalyticsAIAssistant step lines', () => {
  beforeEach(() => {
    renderAssistant()
  })

  it('leads a finished query with what it grouped by and ends with the outcome', () => {
    expect(stepLabels().rows('Channel Group', 10)).toBe('Channel Group — 10 rows')
  })

  it('counts a single row as one', () => {
    expect(stepLabels().rows('Channel Group', 1)).toBe('Channel Group — 1 row')
  })

  it('leaves the descriptor alone while the step is still running', () => {
    // antd renders the loading dots IN PLACE OF the content, so this text is seen
    // only when a stop uncovers a step mid-flight - and the outcome is APPENDED when
    // it arrives, so the identity of the line never moves sideways under an operator
    // who is already reading it.
    expect(stepLabels().running('Channel Group')).toBe('Channel Group')
    expect(stepLabels().rows('Channel Group', 10)).toContain(stepLabels().running('Channel Group'))
  })

  it('marks a step that broke and a step that was stopped without moving it', () => {
    expect(stepLabels().failed('Channel Group')).toBe('Channel Group — failed')
    expect(stepLabels().cancelled('Channel Group')).toBe('Channel Group — cancelled')
  })

  it('reads a bucketed query as a series', () => {
    expect(stepLabels().series('Sessions', 'day')).toBe('Sessions per day')
    expect(stepLabels().series('Channel Group', 'month')).toBe('Channel Group per month')
  })

  it('gives every bucket size a wording of its own', () => {
    // A missing arm resolves to undefined and prints the word at the operator.
    const lines = (['hour', 'day', 'week', 'month', 'year'] as const).map((granularity) =>
      stepLabels().series('Sessions', granularity)
    )
    expect(lines.every((line) => line.startsWith('Sessions '))).toBe(true)
    expect(new Set(lines).size).toBe(lines.length)
  })

  it('names the period a summary covers the way the date picker names it', () => {
    // makeContext() is on previous_7_days; the raw preset never reaches the line.
    expect(stepLabels().summary()).toBe('Summary of Previous 7 days')
  })

  it("reports a period change in the picker's words, not the model's tokens", () => {
    const line = stepLabels().periodSet({ period: 'previous_28_days' })
    expect(line).toBe('Period — Previous 28 days')
    expect(line).not.toContain('previous_28_days')
  })

  it('spells out a custom range instead of the word "custom"', () => {
    expect(
      stepLabels().periodSet({
        period: 'custom',
        customStart: '2024-01-01',
        customEnd: '2024-01-31'
      })
    ).toBe('Period — 2024-01-01 → 2024-01-31')
  })

  it('carries every field one period call changed on the one line', () => {
    expect(stepLabels().periodSet({ period: 'previous_28_days', comparison: 'previous_year' })).toBe(
      'Period — Previous 28 days, Previous year'
    )
  })

  it('names the field the call actually wrote when it did not write a period', () => {
    // "Period — Previous period" would be a claim the operator can check against the
    // date picker and find false.
    expect(stepLabels().periodSet({ comparison: 'previous_period' })).toBe(
      'Comparison — Previous period'
    )
    expect(stepLabels().periodSet({ timezone: 'Europe/Paris' })).toBe('Timezone — Europe/Paris')
  })

  it('says what changed on screen and what it changed to', () => {
    const labels = stepLabels()
    expect(labels.filtersApplied(3)).toBe('Filters — 3 applied')
    expect(labels.filtersApplied(1)).toBe('Filters — 1 applied')
    expect(labels.filtersCleared()).toBe('Filters — cleared')
    expect(labels.reportOpened('Device / Browser')).toBe('Report — Device / Browser')
    expect(labels.catalogRead()).toBe('Metrics and dimensions')
  })

  it('names a section the way its own page heading does, not by its route segment', () => {
    expect(stepLabels().navigated('explore')).toBe('Section — Explore')
    expect(stepLabels().navigated('dashboard')).toBe('Section — Web Analytics')
  })

  it('builds every finished line the same way, so the column scans down its separator', () => {
    const labels = stepLabels()
    const lines = [
      labels.rows('Channel Group', 10),
      labels.cancelled('Channel Group'),
      labels.failed('Channel Group'),
      labels.periodSet({ period: 'previous_28_days' }),
      labels.filtersApplied(3),
      labels.filtersCleared(),
      labels.reportOpened('Device'),
      labels.navigated('goals')
    ]
    for (const line of lines) expect(line).toContain(' — ')
  })
})

describe('WebAnalyticsAIAssistant translated step lines', () => {
  it('translates the step lines rather than hard-coding the English', () => {
    // The macro mock keys on the reconstructed template, so this is the same message
    // the extractor picks up - a plain string here would ignore the catalog.
    i18n.loadAndActivate({
      locale: 'en',
      messages: {
        '{0} — {1} rows': '{0} : {1} lignes',
        'Filters — cleared': 'Filtres retires'
      }
    })
    renderAssistant()

    expect(stepLabels().rows('Channel Group', 10)).toBe('Channel Group : 10 lignes')
    expect(stepLabels().filtersCleared()).toBe('Filtres retires')
  })
})

describe('WebAnalyticsAIAssistant step marks', () => {
  /** The lucide component behind a mark, which is what identifies the glyph. */
  const glyphOf = (tool: string) => {
    const icon = (assistantOptions.current?.toolIcons ?? {})[tool]
    if (!isValidElement(icon)) throw new Error(`the mark for ${tool} is not an element`)
    return icon.type
  }

  // Reading tools carry chart and document glyphs; writing tools carry the
  // dashboard's own controls. The families are what lets "asked your data a
  // question" separate from "changed what is on your screen" before the label is
  // read at all, so the membership is pinned rather than left to whoever edits next.
  const READING = {
    [WEB_TOOL_NAMES.QUERY]: ChartColumn,
    [WEB_TOOL_NAMES.COMPARE]: GitCompare,
    [WEB_TOOL_NAMES.SUMMARIZE]: FileText,
    [WEB_TOOL_NAMES.CATALOG]: BookOpen
  }
  const WRITING = {
    [WEB_TOOL_NAMES.SET_PERIOD]: CalendarRange,
    [WEB_TOOL_NAMES.SET_FILTERS]: Funnel,
    [WEB_TOOL_NAMES.SET_REPORT]: Table2,
    [WEB_TOOL_NAMES.NAVIGATE]: PanelsTopLeft
  }

  it('draws each tool with the glyph its family calls for', () => {
    renderAssistant()
    for (const [tool, expected] of Object.entries({ ...READING, ...WRITING })) {
      expect(glyphOf(tool), `mark for ${tool}`).toBe(expected)
    }
  })

  it('leaves no tool outside the two families, and no two of them sharing a glyph', () => {
    renderAssistant()
    // A tool added later has to be placed in a family rather than quietly inheriting
    // the shared panel's neutral fallback dot.
    expect(Object.keys({ ...READING, ...WRITING }).sort()).toEqual(
      [...Object.values(WEB_TOOL_NAMES)].sort()
    )
    const glyphs = Object.values(WEB_TOOL_NAMES).map(glyphOf)
    expect(new Set(glyphs).size).toBe(glyphs.length)
    // Not the panel's own brand mark: that one is in the header and on the launcher,
    // and a step wearing it would read as the assistant rather than as its work.
    expect(glyphs).not.toContain(ChartLine)
  })
})
