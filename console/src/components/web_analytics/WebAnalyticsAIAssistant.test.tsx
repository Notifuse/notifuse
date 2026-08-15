import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WebAnalyticsAIAssistant, shouldHideAssistant } from './WebAnalyticsAIAssistant'
import { WEB_ANALYTICS_AI_TOOLS, NAVIGABLE_TABS } from './web-analytics-ai-tools'
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

const renderAssistant = (
  overrides: { workspace?: Workspace; tab?: WebAnalyticsTab } = {}
) =>
  render(
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
    expect(typeof deps.query).toBe('function')
    expect(typeof deps.applyUiState).toBe('function')
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

describe('shouldHideAssistant', () => {
  it('keeps the assistant off the attribution-rules tab', () => {
    // The filters tab has no period picker and no query filters; every tool would
    // mutate state the operator cannot see.
    expect(shouldHideAssistant('filters')).toBe(true)
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
