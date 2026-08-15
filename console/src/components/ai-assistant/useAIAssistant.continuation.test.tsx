import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { Globe, Search } from 'lucide-react'
import { useAIAssistant } from './useAIAssistant'
import { llmApi, type LLMChatEvent, type LLMChatRequest, type LLMTool } from '../../services/api/llm'
import type {
  AIAssistantConfig,
  BubbleItem,
  ToolHandler,
  ToolResult,
  UseAIAssistantOptions
} from './types'
import { TOOL_RESULTS_HEADER } from './wire'
import { buildWebAnalyticsToolHandlers } from '../web_analytics/web-analytics-ai-handlers'
import type { AnalyticsResponse } from '../../services/api/analytics'

vi.mock('../../services/api/llm', () => ({
  llmApi: { streamChat: vi.fn() }
}))

// web-analytics-ai-handlers reaches services/api/analytics, which imports the api
// client; the client imports the router and cycles back into the pages.
vi.mock('../../services/api/client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

const config: AIAssistantConfig = {
  title: 'AI',
  icon: null,
  iconButton: null,
  iconLarge: null,
  iconColor: '#000',
  avatarColor: '#000',
  placeholder: 'Ask...',
  maxTokens: 1024,
  notConfiguredGradient: ''
}

// Without an llm integration handleSend early-returns and every case here is vacuous.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const workspace: any = {
  id: 'ws1',
  integrations: [{ id: 'llm1', type: 'llm', name: 'LLM' }]
}

/** The events one POST /api/llm.chat replays, in order. */
type Round = LLMChatEvent[]

/**
 * One scripted round per streamChat call. Params are deep-copied because the hook
 * keeps ONE transcript array alive across the rounds of a turn and pushes onto it:
 * holding the reference would make every recorded round look like the last one.
 */
function scriptRounds(rounds: Round[]) {
  const calls: LLMChatRequest[] = []
  vi.mocked(llmApi.streamChat).mockImplementation(async (params, onEvent) => {
    calls.push(JSON.parse(JSON.stringify(params)) as LLMChatRequest)
    const script = rounds[calls.length - 1] ?? [{ type: 'done' } as LLMChatEvent]
    for (const event of script) onEvent(event)
  })
  return { calls, messagesAt: (round: number) => calls[round - 1].messages }
}

const toolUse = (name: string, input: Record<string, unknown> = {}): LLMChatEvent =>
  ({ type: 'tool_use', tool_name: name, tool_input: input }) as LLMChatEvent
const text = (content: string): LLMChatEvent => ({ type: 'text', content }) as LLMChatEvent
const done = (extra: Partial<LLMChatEvent> = {}): LLMChatEvent =>
  ({ type: 'done', ...extra }) as LLMChatEvent

/** A macrotask turn: flushes the whole microtask chain a round unwinds through. */
const flush = () => new Promise<void>((resolve) => setTimeout(resolve, 0))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

type Assistant = { current: ReturnType<typeof useAIAssistant> }

function renderAssistant(options: Partial<UseAIAssistantOptions> = {}) {
  return renderHook(() =>
    useAIAssistant({
      workspace,
      config,
      tools: [],
      toolHandlers: new Map(),
      buildSystemPrompt: () => 'SYSTEM',
      ...options
    })
  )
}

/** Send and wait for the whole turn to unwind. */
async function send(result: Assistant, question: string) {
  act(() => result.current.setInputValue(question))
  await act(async () => {
    await result.current.handleSend()
  })
}

/** Send without waiting for the turn to finish (it may never finish). */
async function sendWithoutWaiting(result: Assistant, question: string) {
  act(() => result.current.setInputValue(question))
  await act(async () => {
    void result.current.handleSend()
    await flush()
  })
}

const lastTurnOf = (messages: LLMChatRequest['messages']) => messages[messages.length - 1].content

beforeEach(() => {
  vi.mocked(llmApi.streamChat).mockReset()
})

describe('useAIAssistant continuation loop', () => {
  it('answers using the rows a tool returned instead of stopping at the tool call', async () => {
    const { calls, messagesAt } = scriptRounds([
      [toolUse('query_web_analytics', { measures: ['sessions'] }), done()],
      [text('Sessions are up.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'day,sessions\n2026-08-14,4210' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 3 })
    await send(result, 'how many sessions?')

    expect(calls).toHaveLength(2)
    expect(messagesAt(2).map((m) => m.role)).toEqual(['user', 'assistant', 'user'])
    const results = lastTurnOf(messagesAt(2))
    expect(results).toContain('<tool_results>')
    expect(results).toContain('2026-08-14,4210')
  })

  it('carries the round-1 prose into the transcript as the assistant turn before the results', async () => {
    const { messagesAt } = scriptRounds([
      [text('Let me pull the numbers.'), toolUse('query_web_analytics'), done()],
      [text('Here they are.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 3 })
    await send(result, 'how many sessions?')

    expect(messagesAt(2)[1]).toEqual({ role: 'assistant', content: 'Let me pull the numbers.' })
  })

  it('substitutes a description of the calls when a round produced no prose', async () => {
    // A wire transcript with an empty assistant turn is rejected by the backend, so a
    // text-less round must still contribute a non-empty assistant message.
    const { messagesAt } = scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('Here they are.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 3 })
    await send(result, 'how many sessions?')

    expect(messagesAt(2)[1]).toEqual({
      role: 'assistant',
      content: '(Calling query_web_analytics.)'
    })
  })

  it('stops as soon as a round comes back without tool results', async () => {
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('Traffic grew 12%.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 5 })
    await send(result, 'how many sessions?')

    expect(calls).toHaveLength(2)
  })

  it('keeps the tools available on the continuation round', async () => {
    const tools: LLMTool[] = [
      { name: 'query_web_analytics', description: 'run a query', input_schema: { type: 'object' } }
    ]
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('Done.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ tools, toolHandlers: handlers, maxToolRounds: 3 })
    await send(result, 'how many sessions?')

    expect(calls).toHaveLength(2)
    expect(calls[1].tools).toEqual(tools)
  })

  it('rebuilds the system prompt for each round rather than resending round 1s', async () => {
    // The dashboard state the prompt describes can be changed by a round-1 UI tool;
    // resending the captured prompt would describe the page as it was before.
    let built = 0
    const buildSystemPrompt = vi.fn(() => `PROMPT-${++built}`)
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('Done.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({
      toolHandlers: handlers,
      buildSystemPrompt,
      maxToolRounds: 3
    })
    await send(result, 'how many sessions?')

    expect(calls).toHaveLength(2)
    expect(buildSystemPrompt).toHaveBeenCalledTimes(calls.length)
    expect(calls[0].system_prompt).toBe('PROMPT-1')
    expect(calls[1].system_prompt).toBe('PROMPT-2')
  })

  it('tells the model what a UI tool changed, through the results payload', async () => {
    // The rebuilt system prompt depends on a render having happened; the tool-result
    // channel does not, so the state change reaches round 2 either way.
    const { calls } = scriptRounds([
      [
        toolUse('set_dashboard_period', { period: 'previous_30_days' }),
        toolUse('query_web_analytics'),
        done()
      ],
      [text('Done.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      [
        'set_dashboard_period',
        () => ({ content: 'dashboard updated: period previous_30_days', silent: true })
      ],
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 3 })
    await send(result, 'switch to the last 30 days and tell me the traffic')

    expect(calls).toHaveLength(2)
    expect(lastTurnOf(calls[1].messages)).toContain('dashboard updated: period previous_30_days')
  })

  it('renders one assistant bubble per round so the thread reads chronologically', async () => {
    scriptRounds([
      [text('Checking the numbers.'), toolUse('query_web_analytics'), done()],
      [text('Sessions are up 12%.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 3 })
    await send(result, 'how many sessions?')

    expect(
      result.current.messages.filter((m) => m.role === 'assistant').map((m) => m.content)
    ).toEqual(['Checking the numbers.', 'Sessions are up 12%.'])
  })

  it('never shows the synthetic tool-results turn to the operator', async () => {
    scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('Sessions are up.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 3 })
    await send(result, 'how many sessions?')

    const rendered = [
      ...result.current.messages.map((m) => m.content),
      ...result.current.bubbleItems.map((b) => b.content)
    ].join('\n')
    expect(rendered).not.toContain(TOOL_RESULTS_HEADER)
    expect(rendered).not.toContain('<tool_results>')
  })
})

describe('useAIAssistant continuation bounds', () => {
  /** A model that keeps calling, with a different input each round so nothing dedupes. */
  function distinctCallScript(rounds: number): Round[] {
    return Array.from({ length: rounds }, (_unused, index) => [
      toolUse('query_web_analytics', { round: index + 1 }),
      done()
    ])
  }

  it('stops after the caller-configured number of rounds and says so in the thread', async () => {
    const maxToolRounds = 3
    const { calls } = scriptRounds(distinctCallScript(6))
    let n = 0
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: `rows ${++n}` })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds })
    await send(result, 'keep going forever')

    expect(calls).toHaveLength(maxToolRounds)
    const notice = result.current.messages.find(
      (m) => m.toolName === '__error__' && m.content.includes('rounds of tool calls')
    )
    expect(notice, 'the operator should be told the turn was capped').toBeTruthy()
    expect(notice?.content).toContain(`I ran ${maxToolRounds} rounds`)
  })

  it('clamps a caller asking for more rounds than the hard ceiling', async () => {
    const { calls } = scriptRounds(distinctCallScript(20))
    let n = 0
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: `rows ${++n}` })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 99 })
    await send(result, 'run away')

    expect(calls).toHaveLength(5)
  })

  it('stops executing handlers once the per-turn call budget is spent', async () => {
    // Five distinct calls per round: the budget runs out mid-round-3, and round 4 is
    // refused wholesale.
    const fiveCalls = (round: number): Round => [
      ...[1, 2, 3, 4, 5].map((i) => toolUse('query_web_analytics', { r: round, i })),
      done()
    ]
    const { calls } = scriptRounds([1, 2, 3, 4, 5].map(fiveCalls))
    const executed: Array<Record<string, unknown>> = []
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (event) => {
          executed.push(event.tool_input ?? {})
          return { content: 'sessions\n42' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 5 })
    await send(result, 'query everything')

    expect(executed).toHaveLength(12)
    // Round 4 was all refusals, so it bought no further request.
    expect(calls).toHaveLength(4)
    expect(lastTurnOf(calls[3].messages)).toContain('tool budget for this turn is exhausted')
  })

  it('refuses to re-run a call the model already made earlier in the turn', async () => {
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics', { measures: ['sessions'] }), done()],
      // Same input, keys reordered: the fingerprint is key-order stable.
      [toolUse('query_web_analytics', { measures: ['sessions'] }), done()]
    ])
    const executed: unknown[] = []
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (event) => {
          executed.push(event.tool_input)
          return { content: 'sessions\n42' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    expect(executed).toHaveLength(1)
    expect(calls).toHaveLength(2)
  })

  it('runs an identical call twice when both come from the same round', async () => {
    // "Add two identical buttons" is a legitimate instruction; only a cross-round
    // repeat is the model re-asking for data it already has.
    scriptRounds([
      [toolUse('addBlock', { type: 'button' }), toolUse('addBlock', { type: 'button' }), done()],
      [text('Both added.'), done()]
    ])
    const executed: unknown[] = []
    const handlers = new Map<string, ToolHandler>([
      [
        'addBlock',
        (event) => {
          executed.push(event.tool_input)
          return { content: 'block added' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'add two buttons')

    expect(executed).toHaveLength(2)
  })

  it('buys no extra request for a round in which every call was refused', async () => {
    // A refusal costs nothing to produce, so it must not be able to pay for a round
    // trip of its own - that would turn the budgets into cost amplifiers.
    const { calls } = scriptRounds([
      [toolUse('teleport_to_mars'), toolUse('read_minds'), done()],
      [text('never reached'), done()]
    ])

    const { result } = renderAssistant({
      toolHandlers: new Map<string, ToolHandler>([['query_web_analytics', () => ({ content: 'x' })]]),
      maxToolRounds: 4
    })
    await send(result, 'do the impossible')

    expect(calls).toHaveLength(1)
  })

  it('reports a refused call to the model when a real result pays for the round', async () => {
    // The model needs to learn the tool name was wrong, or it will keep calling it.
    const { calls } = scriptRounds([
      [toolUse('teleport_to_mars'), toolUse('query_web_analytics'), done()],
      [text('Sorry about that.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'do the impossible, then count sessions')

    expect(calls).toHaveLength(2)
    const results = lastTurnOf(calls[1].messages)
    expect(results).toContain('unknown tool')
    expect(results).toContain('teleport_to_mars')
    expect(results).toContain('sessions')
  })

  it('ends the turn when a round was truncated instead of driving another request', async () => {
    // A truncated round can carry a half-parsed tool input and will very likely
    // truncate again.
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), done({ truncated: true })],
      [text('never reached'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'write a very long report')

    expect(calls).toHaveLength(1)
    expect(
      result.current.messages.some(
        (m) => m.toolName === '__error__' && m.content.includes('token limit')
      )
    ).toBe(true)
  })

  it('ends the turn when the provider reports an error mid-round', async () => {
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), { type: 'error', error: 'rate limited' } as LLMChatEvent],
      [text('never reached'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    expect(calls).toHaveLength(1)
    expect(result.current.messages.some((m) => m.content.includes('rate limited'))).toBe(true)
  })
})

describe('useAIAssistant handler failure modes', () => {
  it('reports a handler that throws synchronously and keeps the turn alive', async () => {
    // A synchronous throw would otherwise be swallowed by streamChat's parse
    // try/catch, leaving the model waiting for a result that never comes.
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('That query was invalid, sorry.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        () => {
          throw new Error('unknown dimension "wat"')
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'group by wat')

    expect(calls).toHaveLength(2)
    const results = lastTurnOf(calls[1].messages)
    expect(results).toContain('unknown dimension')
    expect(results).toContain('"ok":false')
    expect(result.current.messages.some((m) => m.content === 'That query was invalid, sorry.')).toBe(
      true
    )
  })

  it('reports a handler whose promise rejects', async () => {
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('The query failed.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        async () => {
          throw new Error('analytics backend unreachable')
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    expect(calls).toHaveLength(2)
    const results = lastTurnOf(calls[1].messages)
    expect(results).toContain('analytics backend unreachable')
    expect(results).toContain('"ok":false')
  })

  it('times out a handler that never settles so the turn ends with an answer', async () => {
    // summarize_period fans out into ~17 analytics queries, and it is the realistic
    // way to reach the hook's tool timeout. The timer lives in the hook, not in the
    // handlers module, which is why this case is exercised through the real handler.
    vi.useFakeTimers()
    try {
      const { calls } = scriptRounds([
        [toolUse('summarize_period'), done()],
        [text('The summary timed out; try a shorter period.'), done()]
      ])
      const handlers = buildWebAnalyticsToolHandlers({
        workspaceId: 'ws1',
        timezone: 'UTC',
        currentPeriod: 'previous_7_days',
        currentResolved: {
          startDay: '2026-08-08',
          endDay: '2026-08-14',
          startUtc: '2026-08-08T00:00:00.000Z',
          endUtc: '2026-08-14T23:59:59.999Z'
        },
        currentComparison: 'previous_period',
        currentFilters: [],
        currentGranularity: 'day',
        // Never resolves: a cold workspace whose queries simply do not come back.
        query: () => new Promise<AnalyticsResponse>(() => {}),
        applyUiState: async () => {},
        // Nothing staged: this turn runs one tool, and the overlay only ever holds
        // what a sibling tool of the same round already asked the page for.
        pendingUiState: () => ({}),
        labels: {
          running: (what) => `Querying ${what}`,
          rows: (what, count) => `${what} - ${count} rows`,
          // Unused by this case, which only runs summarize_period - but the labels
          // are a complete contract, and an absent one is a typecheck error rather
          // than an `undefined` that would surface as a blank step line.
          series: (what, granularity) => `${what} per ${granularity}`,
          cancelled: (what) => `${what} - cancelled`,
          failed: (what) => `${what} - failed`,
          summary: () => 'Summarising the period',
          periodSet: (summary) => `Period set to ${summary}`,
          filtersApplied: (count) => `${count} filters applied`,
          filtersCleared: () => 'Filters cleared',
          reportOpened: (dimensions) => `Report grouped by ${dimensions}`,
          navigated: (section) => `Opened the ${section} section`,
          catalogRead: () => 'Reading the available metrics'
        }
      })

      const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
      let turn!: Promise<void>
      act(() => result.current.setInputValue('summarise this period'))
      await act(async () => {
        turn = result.current.handleSend()
        await vi.advanceTimersByTimeAsync(0)
      })

      expect(calls).toHaveLength(1)

      await act(async () => {
        await vi.advanceTimersByTimeAsync(20_000)
        await turn
      })

      expect(calls).toHaveLength(2)
      expect(lastTurnOf(calls[1].messages)).toContain('timed out after 20000ms')
    } finally {
      vi.useRealTimers()
    }
  })

  it('drops a handler return that is not a tool result and starts no continuation', async () => {
    const { calls } = scriptRounds([
      [toolUse('setEmailTree'), done()],
      [text('never reached'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['setEmailTree', () => ({ status: 'ok' }) as unknown as ToolResult]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'build it')

    expect(calls).toHaveLength(1)
  })

  it('does not spend a round on acknowledgements alone', async () => {
    // "I navigated the page for you" tells the model nothing it did not already know.
    const { calls } = scriptRounds([
      [toolUse('navigate_to_tab', { tab: 'pages' }), done()],
      [text('never reached'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['navigate_to_tab', () => ({ content: 'now showing the pages section', silent: true })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'open the pages tab')

    expect(calls).toHaveLength(1)
  })

  it('sends an acknowledgement along when a real result pays for the round', async () => {
    const { calls } = scriptRounds([
      [toolUse('navigate_to_tab', { tab: 'pages' }), toolUse('query_web_analytics'), done()],
      [text('Here are the top pages.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['navigate_to_tab', () => ({ content: 'now showing the pages section', silent: true })],
      ['query_web_analytics', () => ({ content: 'path,views\n/pricing,120' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'show me the top pages')

    expect(calls).toHaveLength(2)
    const results = lastTurnOf(calls[1].messages)
    expect(results).toContain('now showing the pages section')
    expect(results).toContain('/pricing,120')
  })
})

describe('useAIAssistant failure visibility', () => {
  let consoleError: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleError.mockRestore()
  })

  it('shows the failure when the stream errors after a tool bubble took the assistant slot', async () => {
    // A round whose first output is a tool call has its empty assistant bubble spliced
    // out by insertToolMessage, so an error frame arriving afterwards had nothing left
    // to rewrite - and onError declines to write twice for the same frame. The operator
    // watched the bubble stop with no error and no answer.
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent, onError) => {
      onEvent(toolUse('query_web_analytics'))
      onEvent({ type: 'error', error: 'provider exploded' } as LLMChatEvent)
      onError?.(new Error('provider exploded'))
    })
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          ctx.progress('Querying sessions by day')
          return { content: 'sessions\n42' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    const visible = result.current.bubbleItems.filter((b) => b.content.includes('provider exploded'))
    expect(visible, 'the failure must be visible exactly once').toHaveLength(1)
  })

  it('shows the failure when the request is refused after a tool bubble took the assistant slot', async () => {
    // Same key mismatch on the transport path: a plain HTTP 400 reaches onError only.
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent, onError) => {
      onEvent(toolUse('query_web_analytics'))
      onError?.(new Error('HTTP error: 400'))
    })
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          ctx.progress('Querying sessions by day')
          return { content: 'sessions\n42' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    const visible = result.current.bubbleItems.filter((b) => b.content.includes('HTTP error: 400'))
    expect(visible).toHaveLength(1)
  })

  it('shows answer text that streams after a tool call, not only sends it to the model', async () => {
    // The text is accrued into the round and pushed onto the wire transcript either
    // way, so losing it on screen tells the model it said something the user never saw.
    const { messagesAt } = scriptRounds([
      [toolUse('query_web_analytics'), text('Sessions are up 12%.'), done()],
      [text('Anything else?'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          ctx.progress('Querying sessions by day')
          return { content: 'sessions\n42' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    expect(messagesAt(2)[1]).toEqual({ role: 'assistant', content: 'Sessions are up 12%.' })
    expect(result.current.bubbleItems.some((b) => b.content === 'Sessions are up 12%.')).toBe(true)
  })

  it('leaves something on screen when a turn produces no text and no tool bubble', async () => {
    // bubbleItems skips a finished assistant message with no answer text - right for a
    // continuation round, where the tool bubble is the output. A whole turn that ends
    // that way rendered nothing at all, leaving the operator on their own question.
    scriptRounds([[done()]])

    const { result } = renderAssistant()
    await send(result, 'hello')

    const replies = result.current.bubbleItems.filter((b) => b.role !== 'user')
    expect(replies).toHaveLength(1)
    expect(replies[0].content.trim()).not.toBe('')
  })

  it('does not claim it never reached an answer when the capped round did answer', async () => {
    // The last round may stream a complete answer AND ask for one more query; a red
    // "I ran out of rounds" under a good answer reads as a bug.
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics', { r: 1 }), done()],
      [text('Sessions are up 12%.'), toolUse('query_web_analytics', { r: 2 }), done()]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 2 })
    await send(result, 'how many sessions?')

    expect(calls).toHaveLength(2)
    expect(result.current.messages.some((m) => m.content === 'Sessions are up 12%.')).toBe(true)
    expect(result.current.messages.some((m) => m.toolName === '__error__')).toBe(false)
  })
})

describe('useAIAssistant abandoned tool calls', () => {
  it('stops a timed-out handler and drops the writes it makes afterwards', async () => {
    vi.useFakeTimers()
    try {
      const gate = deferred<void>()
      let handlerSignal: AbortSignal | undefined
      const { calls } = scriptRounds([
        [toolUse('query_web_analytics'), done()],
        [text('That query timed out; try a shorter period.'), done()]
      ])
      const handlers = new Map<string, ToolHandler>([
        [
          'query_web_analytics',
          async (_event, _insert, ctx) => {
            handlerSignal = ctx.signal
            const bubble = ctx.progress('Querying sessions by day')
            await gate.promise
            bubble.update('Querying sessions by day - 7 rows')
            return { content: 'sessions\n42' }
          }
        ]
      ])

      const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
      act(() => result.current.setInputValue('sessions by day'))
      let turn!: Promise<void>
      await act(async () => {
        turn = result.current.handleSend()
        await vi.advanceTimersByTimeAsync(0)
      })

      await act(async () => {
        await vi.advanceTimersByTimeAsync(20_000)
        gate.resolve()
        await turn
      })

      expect(lastTurnOf(calls[1].messages)).toContain('timed out after 20000ms')
      // The handler is told to stop, rather than left querying behind the apology...
      expect(handlerSignal?.aborted).toBe(true)
      // ...and its late repaint never lands beside it.
      const bubble = result.current.messages.find((m) => m.role === 'tool')
      expect(bubble?.content).toBe('Querying sessions by day')
      expect(bubble?.loading).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('drops a late handler write into a conversation that was reset', async () => {
    // insertToolMessage APPENDS when it cannot find the round's assistant bubble, so an
    // unguarded late narration reappears in a thread the operator has just cleared.
    const gate = deferred<void>()
    scriptRounds([[toolUse('setEmailTree'), done()]])
    const handlers = new Map<string, ToolHandler>([
      [
        'setEmailTree',
        async (_event, insert) => {
          await gate.promise
          insert('Email updated', 'setEmailTree')
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await sendWithoutWaiting(result, 'build it')

    act(() => result.current.resetConversation())
    await act(async () => {
      gate.resolve()
      await flush()
    })

    expect(result.current.messages).toEqual([])
  })

  it('lets the model retry a call that failed instead of refusing it as a duplicate', async () => {
    // The fingerprint used to be recorded at dispatch, so a call that came back with an
    // error still poisoned the retry: the model was told "the earlier result stands"
    // when the earlier result was a failure.
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics', { measures: ['sessions'] }), done()],
      [toolUse('query_web_analytics', { measures: ['sessions'] }), done()],
      [text('Sessions are up.'), done()]
    ])
    let attempts = 0
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        () => {
          attempts += 1
          return attempts === 1
            ? { content: 'analytics backend unreachable', isError: true }
            : { content: 'day,sessions\n2026-08-14,4210' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    expect(attempts).toBe(2)
    expect(calls).toHaveLength(3)
    expect(lastTurnOf(calls[2].messages)).toContain('4210')
  })

  it('still refuses to repeat a call that succeeded', async () => {
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics', { measures: ['sessions'] }), done()],
      [toolUse('query_web_analytics', { measures: ['sessions'] }), done()]
    ])
    let attempts = 0
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        () => {
          attempts += 1
          return { content: 'sessions\n42' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    expect(attempts).toBe(1)
    expect(calls).toHaveLength(2)
  })
})

describe('useAIAssistant tool progress bubbles', () => {
  it('rewrites the running bubble in place when the query comes back', async () => {
    const gate = deferred<void>()
    scriptRounds([[toolUse('query_web_analytics'), done()]])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        async (_event, _insert, ctx) => {
          const bubble = ctx.progress('Querying sessions by day')
          await gate.promise
          bubble.update('Querying sessions by day - 7 rows')
          return { content: 'sessions\n42' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await sendWithoutWaiting(result, 'sessions by day')

    const running = result.current.messages.filter((m) => m.role === 'tool')
    expect(running).toHaveLength(1)
    expect(running[0].loading).toBe(true)
    const bubbleKey = running[0].key

    await act(async () => {
      gate.resolve()
      await flush()
    })

    const finished = result.current.messages.filter((m) => m.role === 'tool')
    expect(finished).toHaveLength(1)
    expect(finished[0].key).toBe(bubbleKey)
    expect(finished[0].content).toBe('Querying sessions by day - 7 rows')
    expect(finished[0].loading).toBe(false)
  })

  it('styles a failed tool bubble as an error while keeping its tool name', async () => {
    scriptRounds([[toolUse('query_web_analytics'), done()]])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        async (_event, _insert, ctx) => {
          const bubble = ctx.progress('Querying sessions by day')
          bubble.update('Querying sessions by day - failed', { failed: true })
          return { content: 'boom', isError: true }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'sessions by day')

    const bubble = result.current.messages.find((m) => m.role === 'tool')
    expect(bubble?.failed).toBe(true)
    expect(bubble?.toolName).toBe('query_web_analytics')
    const rendered = result.current.bubbleItems.find((b) => b.key === bubble?.key)
    expect(rendered?.styles?.content?.background).toBe('#fff2f0')
  })

  it('marks a running tool bubble cancelled rather than freezing it mid-query', async () => {
    scriptRounds([[toolUse('query_web_analytics'), done()]])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          ctx.progress('Querying sessions by day')
          return new Promise<ToolResult>(() => {})
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await sendWithoutWaiting(result, 'sessions by day')

    await act(async () => {
      result.current.handleCancel()
      await flush()
    })

    const bubble = result.current.messages.find((m) => m.role === 'tool')
    expect(bubble?.loading).toBe(false)
    expect(bubble?.content).toBe('Querying sessions by day - cancelled')
  })
})

describe('useAIAssistant turn identity and streaming state', () => {
  let consoleError: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleError.mockRestore()
  })

  it('stops streaming when the stream ends without a terminal event', async () => {
    // A dropped connection, or a split SSE frame swallowed by the JSON.parse in
    // llm.ts, used to leave the composer disabled for the rest of the session.
    scriptRounds([[text('Partial answer')]])

    const { result } = renderAssistant()
    await send(result, 'hello')

    expect(result.current.isStreaming).toBe(false)
  })

  it('leaves the composer usable when post-turn validation never resolves', async () => {
    const { calls } = scriptRounds([[toolUse('setEmailTree'), done()], [text('Second.'), done()]])
    const handlers = new Map<string, ToolHandler>([['setEmailTree', () => {}]])
    const validateOnComplete = vi.fn(() => new Promise<{ ok: boolean }>(() => {}))

    const { result } = renderAssistant({ toolHandlers: handlers, validateOnComplete })
    await sendWithoutWaiting(result, 'build it')

    expect(validateOnComplete).toHaveBeenCalled()
    expect(result.current.isStreaming).toBe(false)

    await sendWithoutWaiting(result, 'now tweak it')
    expect(calls).toHaveLength(2)
  })

  it('shows the failure when the request is rejected before any SSE frame arrives', async () => {
    // llmApi.streamChat does not reject on a non-2xx: it calls onError and returns
    // normally, and a request refused by Validate() is a plain HTTP 400 JSON body.
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, _onEvent, onError) => {
      onError?.(new Error('HTTP error: 400'))
    })

    const { result } = renderAssistant()
    await send(result, 'hello')

    const visible = result.current.bubbleItems.filter((b) => b.content.includes('HTTP error: 400'))
    expect(visible).toHaveLength(1)
    expect(result.current.isStreaming).toBe(false)
  })

  it('shows a single error bubble when an SSE error frame reaches both callbacks', async () => {
    // streamChat reports an error FRAME twice - onEvent first, then onError for the
    // same frame - and one failure must not read as two.
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent, onError) => {
      onEvent({ type: 'error', error: 'provider exploded' } as LLMChatEvent)
      onError?.(new Error('provider exploded'))
    })

    const { result } = renderAssistant()
    await send(result, 'hello')

    const visible = result.current.bubbleItems.filter((b) =>
      b.content.includes('provider exploded')
    )
    expect(visible).toHaveLength(1)
  })

  it('leaves nothing behind when the operator stops mid-round', async () => {
    const { calls } = scriptRounds([[toolUse('query_web_analytics'), done()]])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => new Promise<ToolResult>(() => {})]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await sendWithoutWaiting(result, 'how many sessions?')

    await act(async () => {
      result.current.handleCancel()
      await flush()
    })

    expect(result.current.isStreaming).toBe(false)
    expect(result.current.messages.some((m) => m.loading)).toBe(false)
    expect(calls).toHaveLength(1)
    expect(result.current.messages.some((m) => m.toolName === '__error__')).toBe(false)
  })

  it('appends nothing when the operator stops between rounds', async () => {
    const secondRound = deferred<void>()
    const calls: LLMChatRequest[] = []
    vi.mocked(llmApi.streamChat).mockImplementation(async (params, onEvent, _onError, options) => {
      calls.push(JSON.parse(JSON.stringify(params)) as LLMChatRequest)
      if (calls.length === 1) {
        onEvent(toolUse('query_web_analytics'))
        onEvent(done())
        return
      }
      // Round 2 is in flight when Stop is pressed. The real transport returns without
      // emitting once its fetch is aborted (llm.ts swallows the AbortError), so the
      // request resolves having delivered nothing at all.
      await secondRound.promise
      if (options?.signal?.aborted) return
      onEvent(text('Sessions are up 12%.'))
      onEvent(done())
    })
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await sendWithoutWaiting(result, 'how many sessions?')
    expect(calls).toHaveLength(2)

    await act(async () => {
      result.current.handleCancel()
      await flush()
    })
    const afterStop = JSON.stringify(result.current.messages)

    await act(async () => {
      secondRound.resolve()
      await flush()
    })

    expect(JSON.stringify(result.current.messages)).toBe(afterStop)
    expect(result.current.messages.some((m) => m.content.includes('Sessions are up 12%.'))).toBe(
      false
    )
    // No round-cap notice, no third request, no resurrected spinner.
    expect(result.current.messages.some((m) => m.toolName === '__error__')).toBe(false)
    expect(calls).toHaveLength(2)
    expect(result.current.isStreaming).toBe(false)
  })

  it('resolves a tool call that arrives on an already-stopped turn instead of parking it', async () => {
    // addEventListener('abort') never fires on a signal that is already aborted, so
    // without the pre-check the turn would sit on the tool timeout with nothing left
    // to cancel it.
    let stop: () => void = () => {}
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent(text('Looking that up.'))
      stop()
      onEvent(toolUse('query_web_analytics'))
      onEvent(done())
    })
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => new Promise<ToolResult>(() => {})]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    stop = () => result.current.handleCancel()

    act(() => result.current.setInputValue('how many sessions?'))
    let outcome = 'parked'
    await act(async () => {
      const turn = result.current.handleSend().then(() => {
        outcome = 'settled'
      })
      await Promise.race([turn, new Promise((resolve) => setTimeout(resolve, 250))])
    })

    expect(outcome).toBe('settled')
  })

  it('lets a stale turn unwind without disturbing the turn that replaced it', async () => {
    const staleRound = deferred<void>()
    const calls: LLMChatRequest[] = []
    vi.mocked(llmApi.streamChat).mockImplementation(async (params, onEvent) => {
      calls.push(JSON.parse(JSON.stringify(params)) as LLMChatRequest)
      if (calls.length === 1) {
        await staleRound.promise
        onEvent(text('Stale answer from the abandoned turn.'))
        onEvent(done())
        return
      }
      onEvent(text('Fresh answer.'))
      onEvent(done())
    })

    const { result } = renderAssistant()
    await sendWithoutWaiting(result, 'first question')
    act(() => result.current.handleCancel())
    await send(result, 'second question')

    await act(async () => {
      staleRound.resolve()
      await flush()
    })

    expect(result.current.isStreaming).toBe(false)
    expect(result.current.messages.map((m) => m.content)).not.toContain(
      'Stale answer from the abandoned turn.'
    )
    expect(result.current.messages.some((m) => m.content === 'Fresh answer.')).toBe(true)
  })

  it('clears the conversation and orphans the in-flight turn on reset', async () => {
    const round = deferred<void>()
    const calls: LLMChatRequest[] = []
    vi.mocked(llmApi.streamChat).mockImplementation(async (params, onEvent) => {
      calls.push(JSON.parse(JSON.stringify(params)) as LLMChatRequest)
      await round.promise
      onEvent(text('Answer nobody asked for any more.'))
      onEvent(done())
    })

    const { result } = renderAssistant()
    await sendWithoutWaiting(result, 'first question')

    act(() => result.current.resetConversation())
    expect(result.current.messages).toEqual([])
    expect(result.current.isStreaming).toBe(false)

    await act(async () => {
      round.resolve()
      await flush()
    })

    expect(result.current.messages).toEqual([])
    expect(calls).toHaveLength(1)
  })

  it('ends the turn when the panel unmounts mid-round', async () => {
    // WebAnalyticsAIAssistant lives inside the route body, so leaving the page
    // unmounts it: a live turn must not keep issuing continuation requests, or
    // writing, for a panel nobody is looking at.
    const gate = deferred<ToolResult>()
    const { calls } = scriptRounds([
      [toolUse('query_web_analytics'), done()],
      [text('never reached'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([['query_web_analytics', () => gate.promise]])

    const { result, unmount } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await sendWithoutWaiting(result, 'how many sessions?')

    const lastRender = JSON.stringify(result.current.messages)
    unmount()

    await act(async () => {
      gate.resolve({ content: 'sessions\n42' })
      await flush()
    })

    expect(calls).toHaveLength(1)
    expect(JSON.stringify(result.current.messages)).toBe(lastRender)
    expect(
      consoleError.mock.calls.some((args) => String(args[0]).includes('unmounted component'))
    ).toBe(false)
  })

  it('runs one turn when two submits land in the same tick', async () => {
    // The web analytics chip auto-send effect firing beside a manual Enter: both read
    // the pre-render isStreaming === false, and the second turn used to leave the first
    // one streaming, accruing cost and running client tools.
    const { calls } = scriptRounds([
      [text('First.'), done()],
      [text('Second.'), done()]
    ])

    const { result } = renderAssistant()
    act(() => result.current.setInputValue('hello'))
    await act(async () => {
      void result.current.handleSend()
      void result.current.handleSend()
      await flush()
    })

    expect(calls).toHaveLength(1)
    expect(result.current.messages.filter((m) => m.role === 'user')).toHaveLength(1)
  })

  it('ends the previous turn when a new one starts', async () => {
    // Bumping the turn id orphans the old turn's writes; only the abort stops its
    // request. handleCancel, resetConversation and unmount all pair the two.
    const signals: Array<AbortSignal | undefined> = []
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent, _onError, options) => {
      signals.push(options?.signal)
      onEvent(text('Answer.'))
      onEvent(done())
    })

    const { result } = renderAssistant()
    await send(result, 'first question')
    expect(signals[0]?.aborted).toBe(false)

    await send(result, 'second question')
    expect(signals[0]?.aborted).toBe(true)
    expect(signals[1]?.aborted).toBe(false)
  })

  it('drops a validation error that resolves after the turn was replaced', async () => {
    // templatesApi.compile has no timeout and no AbortSignal, so this resolves whenever
    // it resolves - possibly into a conversation two questions further on.
    const gate = deferred<{ ok: boolean; errorText?: string }>()
    scriptRounds([
      [toolUse('setEmailTree'), text('Done.'), done()],
      [text('Second answer.'), done()]
    ])
    const handlers = new Map<string, ToolHandler>([['setEmailTree', () => {}]])
    const validateOnComplete = vi.fn(() => gate.promise)

    const { result } = renderAssistant({ toolHandlers: handlers, validateOnComplete })
    await sendWithoutWaiting(result, 'build it')
    expect(validateOnComplete).toHaveBeenCalledTimes(1)

    await send(result, 'now tweak it')

    await act(async () => {
      gate.resolve({ ok: false, errorText: 'line 3: mj-button width must be px' })
      await flush()
    })

    expect(result.current.messages.some((m) => m.content.includes('mj-button width'))).toBe(false)
  })

  it('accumulates cost across every round of a turn', async () => {
    scriptRounds([
      [
        toolUse('query_web_analytics'),
        done({ input_cost: 0.01, output_cost: 0.02, total_cost: 0.03 })
      ],
      [text('Sessions are up.'), done({ input_cost: 0.04, output_cost: 0.05, total_cost: 0.09 })]
    ])
    const handlers = new Map<string, ToolHandler>([
      ['query_web_analytics', () => ({ content: 'sessions\n42' })]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'how many sessions?')

    expect(result.current.costs.input).toBeCloseTo(0.05)
    expect(result.current.costs.output).toBeCloseTo(0.07)
    expect(result.current.costs.total).toBeCloseTo(0.12)
  })
})

describe('useAIAssistant quiet tool steps', () => {
  /** Run one tool that narrates itself, and return the step bubble it left behind. */
  async function runStep(options: {
    toolName?: string
    update?: { text: string; failed?: boolean }
    toolIcons?: Record<string, ReactNode>
  }) {
    const name = options.toolName ?? 'query_web_analytics'
    scriptRounds([[toolUse(name), done()]])
    const handlers = new Map<string, ToolHandler>([
      [
        name,
        (_event, _insert, ctx) => {
          const bubble = ctx.progress('Channel Group')
          if (options.update) bubble.update(options.update.text, { failed: options.update.failed })
          return { content: 'rows' }
        }
      ]
    ])

    const { result } = renderAssistant({
      toolHandlers: handlers,
      maxToolRounds: 4,
      toolIcons: options.toolIcons
    })
    await send(result, 'sessions by channel group')

    const message = result.current.messages.find((m) => m.role === 'tool')
    const item = result.current.bubbleItems.find((b) => b.key === message?.key)
    expect(item).toBeDefined()
    return { item: item as BubbleItem, message, result }
  }

  it('renders a finished tool step as a borderless line with no fill and no border', async () => {
    const { item } = await runStep({ update: { text: 'Channel Group - 10 rows' } })

    expect(item.variant).toBe('borderless')
    expect(item.styles?.content?.background).toBe('transparent')
    expect(item.styles?.content?.border).toBe('none')
  })

  it('gives a step smaller, secondary type so the answer keeps the weight', async () => {
    const { item, result } = await runStep({ update: { text: 'Channel Group - 10 rows' } })
    const answer = result.current.bubbleItems.find((b) => b.role === 'ai')

    expect(item.styles?.content?.fontSize).toBe(12)
    expect(item.styles?.content?.color).toBe('#8c8c8c')
    // The answer is left entirely alone: no style, no variant, no avatar override.
    expect(answer?.styles).toBeUndefined()
    expect(answer?.variant).toBeUndefined()
  })

  it('keeps the step visible after the turn ends instead of clearing the trace', async () => {
    const { item, result } = await runStep({ update: { text: 'Channel Group - 10 rows' } })

    expect(result.current.isStreaming).toBe(false)
    expect(item.content).toBe('Channel Group - 10 rows')
  })

  it('keeps a failed step loud: filled red, never the quiet variant', async () => {
    const { item } = await runStep({ update: { text: 'Channel Group - failed', failed: true } })

    expect(item.variant).toBeUndefined()
    expect(item.styles?.content?.background).toBe('#fff2f0')
    expect(item.styles?.content?.border).toBe('1px solid #ffccc7')
  })

  it('keeps an error bubble loud when the round itself fails', async () => {
    // The round's assistant bubble is spliced out by the step line, so the failure is
    // appended as its own error bubble - the one place the loud treatment has to hold
    // without a `failed` flag to key off.
    scriptRounds([
      [toolUse('query_web_analytics'), { type: 'error', error: 'provider exploded' } as LLMChatEvent]
    ])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          ctx.progress('Channel Group')
          return { content: 'rows' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'sessions by channel group')

    const errorItem = result.current.bubbleItems.find((b) => b.content.includes('provider exploded'))
    expect(errorItem?.variant).toBeUndefined()
    expect(errorItem?.styles?.content?.background).toBe('#fff2f0')
  })
})

describe('useAIAssistant step icons', () => {
  const serverToolStart = (name: string): LLMChatEvent =>
    ({ type: 'server_tool_start', tool_name: name, tool_input: { url: 'https://ex.io' } }) as LLMChatEvent

  async function iconFor(options: {
    events: LLMChatEvent[]
    toolIcons?: Record<string, ReactNode>
    handlers?: Map<string, ToolHandler>
  }) {
    scriptRounds([[...options.events, done()]])
    const { result } = renderAssistant({
      toolHandlers: options.handlers ?? new Map(),
      maxToolRounds: 4,
      toolIcons: options.toolIcons
    })
    await send(result, 'look it up')

    const message = result.current.messages.find((m) => m.role === 'tool')
    return result.current.bubbleItems.find((b) => b.key === message?.key)?.avatar
  }

  it('keeps the built-in icons for the two tools the server runs', async () => {
    const scrape = await iconFor({ events: [serverToolStart('scrape_url')] })
    const search = await iconFor({ events: [serverToolStart('search_web')] })

    expect((scrape?.icon as ReactElement)?.type).toBe(Globe)
    expect((search?.icon as ReactElement)?.type).toBe(Search)
  })

  it('sizes the step avatar below the 20px chat-bubble avatar', async () => {
    const avatar = await iconFor({ events: [serverToolStart('scrape_url')] })

    expect(avatar?.size).toBeLessThan(20)
    expect(avatar?.style.background).toBe('transparent')
  })

  it('lets a consumer icon win over the built-in default for the same tool', async () => {
    const consumerIcon = <span data-testid="consumer-globe" />
    const avatar = await iconFor({
      events: [serverToolStart('scrape_url')],
      toolIcons: { scrape_url: consumerIcon }
    })

    expect(avatar?.icon).toBe(consumerIcon)
  })

  it('resolves a feature tool the hook has never heard of from the consumer map', async () => {
    const consumerIcon = <span data-testid="consumer-chart" />
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          ctx.progress('Channel Group')
          return { content: 'rows' }
        }
      ]
    ])
    const avatar = await iconFor({
      events: [toolUse('query_web_analytics')],
      handlers,
      toolIcons: { query_web_analytics: consumerIcon }
    })

    expect(avatar?.icon).toBe(consumerIcon)
  })

  it('falls back to a neutral mark, keeping an unlabelled tool in the step column', async () => {
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          ctx.progress('Channel Group')
          return { content: 'rows' }
        }
      ]
    ])
    const fallback = await iconFor({ events: [toolUse('query_web_analytics')], handlers })
    const known = await iconFor({ events: [serverToolStart('scrape_url')] })

    expect(fallback?.icon).toBeTruthy()
    // Same avatar box as an iconned step, or the lines lose their shared left edge.
    expect(fallback?.size).toBe(known?.size)
  })
})

describe('useAIAssistant step duration', () => {
  let nowSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    nowSpy = vi.spyOn(Date, 'now')
  })

  afterEach(() => {
    nowSpy.mockRestore()
  })

  /** Run a step that takes `elapsedMs` of wall clock between progress() and update(). */
  async function stepTaking(elapsedMs: number, opts: { failed?: boolean } = {}) {
    const startedAt = 1_700_000_000_000
    nowSpy.mockReturnValue(startedAt)
    scriptRounds([[toolUse('query_web_analytics'), done()]])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, _insert, ctx) => {
          const bubble = ctx.progress('Channel Group')
          nowSpy.mockReturnValue(startedAt + elapsedMs)
          bubble.update('Channel Group - 10 rows', opts)
          return { content: 'rows' }
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'sessions by channel group')
    return result.current.messages.find((m) => m.role === 'tool')?.content
  }

  it('appends the elapsed time to a step slow enough to be worth reading', async () => {
    expect(await stepTaking(1400)).toBe('Channel Group - 10 rows · 1.4s')
  })

  it('drops the decimal once the wait is measured in whole seconds', async () => {
    expect(await stepTaking(12_400)).toBe('Channel Group - 10 rows · 12s')
  })

  it('says nothing about a step that finished before the number could mean anything', async () => {
    // A stopwatch on every line is noise; it earns its place only when it explains a wait.
    expect(await stepTaking(180)).toBe('Channel Group - 10 rows')
  })

  it('leaves the boundary case unmarked and the one just past it marked', async () => {
    expect(await stepTaking(999)).toBe('Channel Group - 10 rows')
    expect(await stepTaking(1000)).toBe('Channel Group - 10 rows · 1.0s')
  })

  it('never appends a duration to a failure, whose text is the thing to read', async () => {
    expect(await stepTaking(4200, { failed: true })).toBe('Channel Group - 10 rows')
  })

  it('does not time a bubble the consumer never finishes', async () => {
    // insertToolMessage's one-shot path posts a line with no handle to update, so there
    // is nothing to append to and nothing to corrupt.
    const startedAt = 1_700_000_000_000
    nowSpy.mockReturnValue(startedAt)
    scriptRounds([[toolUse('query_web_analytics'), done()]])
    const handlers = new Map<string, ToolHandler>([
      [
        'query_web_analytics',
        (_event, insert) => {
          nowSpy.mockReturnValue(startedAt + 5_000)
          insert('Applied the channel group filter', 'query_web_analytics')
        }
      ]
    ])

    const { result } = renderAssistant({ toolHandlers: handlers, maxToolRounds: 4 })
    await send(result, 'filter by channel group')

    expect(result.current.messages.find((m) => m.role === 'tool')?.content).toBe(
      'Applied the channel group filter'
    )
  })
})
