import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, renderHook, screen, fireEvent, act } from '@testing-library/react'
import { useAIAssistant, AIAssistantChat } from './index'
import { llmApi, type LLMChatEvent } from '../../services/api/llm'
// './index' and not './types': the barrel is what the widened tool contract added to
// and what a new consumer imports from, so the barrel is what this file exercises. A
// dropped re-export is a compile error here, caught by `tsc -b` rather than by an
// assertion.
import type {
  AIAssistantConfig,
  AIAssistantSuggestion,
  ToolBubbleHandle,
  ToolHandler,
  ToolResult,
  ToolRunContext
} from './index'

vi.mock('../../services/api/llm', () => ({
  llmApi: { streamChat: vi.fn() }
}))

// services/api/client imports the router, which imports every page and cycles back
// into this graph. Stubbing it keeps the chat render cheap.
vi.mock('../../services/api/client', () => ({
  api: { post: vi.fn().mockResolvedValue({}), get: vi.fn().mockResolvedValue({}) }
}))

// Bubble.List watches a sentinel to decide whether it is scrolled to the bottom;
// jsdom has no IntersectionObserver.
class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}
vi.stubGlobal('IntersectionObserver', IntersectionObserverStub)

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

// handleSend early-returns without an llm integration, so every case would pass
// vacuously without this.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const workspace: any = {
  id: 'ws1',
  integrations: [{ id: 'llm1', type: 'llm', name: 'LLM' }]
}

// The registry literal IS the backward-compatibility claim: the exact shape
// BlogAIAssistant.tsx and EmailAIAssistant.tsx build (a 2-parameter handler with a
// void body) sitting in the same Map as a 3-parameter async handler returning a
// ToolResult. A widening that stopped accepting the legacy shape could not compile
// this file.
const legacyHandler = (_event: LLMChatEvent, insert: (c: string, n: string) => void): void => {
  insert('done', 'legacy_tool')
}
const asyncHandler = async (
  _event: LLMChatEvent,
  _insert: (c: string, n: string) => void,
  ctx: ToolRunContext
): Promise<ToolResult> => {
  const bubble: ToolBubbleHandle = ctx.progress('working')
  bubble.update('finished')
  return { content: 'rows', silent: false }
}
const handlers = new Map<string, ToolHandler>([
  ['legacy_tool', legacyHandler],
  ['async_tool', asyncHandler]
])

// The chip type travels through the same barrel; this annotation is what fails the
// typecheck if the re-export is dropped.
const chips: AIAssistantSuggestion[] = [
  { key: 'k', label: 'Which pages grew?', prompt: 'Which pages grew last week?' }
]

// Exactly how Blog and Email configure the hook: no maxToolRounds, so the
// continuation loop must stay off.
function setupLegacy(
  extra: Partial<Parameters<typeof useAIAssistant>[0]> = {},
  toolHandlers: Map<string, ToolHandler> = handlers
) {
  return renderHook(() =>
    useAIAssistant({
      workspace,
      config,
      tools: [],
      toolHandlers,
      buildSystemPrompt: () => 'SYSTEM',
      ...extra
    })
  )
}

async function send(result: { current: ReturnType<typeof useAIAssistant> }, text: string) {
  act(() => result.current.setInputValue(text))
  await act(async () => {
    await result.current.handleSend()
  })
}

function sentTranscripts() {
  return vi.mocked(llmApi.streamChat).mock.calls.map((call) => call[0].messages)
}

describe('useAIAssistant legacy (no maxToolRounds) contract', () => {
  beforeEach(() => {
    vi.mocked(llmApi.streamChat).mockReset()
  })

  it('runs a two-parameter handler and still shows its tool bubble', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'On it.' } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'legacy_tool', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setupLegacy()
    await send(result, 'do the legacy thing')

    const bubble = result.current.messages.find((m) => m.toolName === 'legacy_tool')
    expect(bubble?.content).toBe('done')
    expect(bubble?.loading).toBeFalsy()
  })

  it('discards a tool result instead of billing a second round trip', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      // Providers emit tool_use after the text of the round, which is the order the
      // hook's bubble placement assumes.
      onEvent({ type: 'text', content: 'Here you go.' } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'async_tool', tool_input: { a: 1 } } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setupLegacy()
    await send(result, 'query something')

    // One POST for the turn: a consumer that never opted into rounds must not have
    // its bill doubled by a handler that happens to return a value.
    expect(llmApi.streamChat).toHaveBeenCalledTimes(1)
    // The async handler's own narration still lands in the chat.
    expect(result.current.messages.some((m) => m.content === 'finished')).toBe(true)
    // ...but its payload never reaches the model.
    expect(JSON.stringify(sentTranscripts())).not.toContain('rows')
  })

  it('issues one request when a handler returns a thenable that is not a tool result', async () => {
    // antd's message.success returns a thenable MessageType, so a concise-bodied
    // handler (`(e, insert) => message.success('x')`) hands the hook a promise whose
    // value is not a ToolResult. That must not be mistaken for a result to continue on.
    const toastHandler: ToolHandler = () => Promise.resolve(true) as unknown as Promise<void>
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'tool_use', tool_name: 'toast_tool', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setupLegacy({}, new Map<string, ToolHandler>([['toast_tool', toastHandler]]))
    await send(result, 'show a toast')

    expect(llmApi.streamChat).toHaveBeenCalledTimes(1)
    expect(result.current.isStreaming).toBe(false)
  })

  it('hands a two-parameter handler a third argument it can safely ignore', async () => {
    const legacySpy = vi.fn((_event: LLMChatEvent, insert: (c: string, n: string) => void) => {
      insert('applied', 'legacy_tool')
    })
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'tool_use', tool_name: 'legacy_tool', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setupLegacy({}, new Map<string, ToolHandler>([['legacy_tool', legacySpy]]))
    await send(result, 'edit it')

    const args = legacySpy.mock.calls[0] as unknown as unknown[]
    expect(args).toHaveLength(3)
    const ctx = args[2] as ToolRunContext
    expect(typeof ctx.progress).toBe('function')
    expect(ctx.signal).toBeInstanceOf(AbortSignal)
    expect(ctx.round).toBe(1)
    // The extra argument changed nothing about what the handler did.
    expect(result.current.messages.some((m) => m.content === 'applied')).toBe(true)
  })

  it('replays the prior turn as plain user and assistant messages, with no tool-results envelope', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'First answer.' } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'async_tool', tool_input: { a: 1 } } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setupLegacy()
    await send(result, 'first question')
    await send(result, 'second question')

    const second = sentTranscripts()[1]
    expect(second.map((m) => m.role)).toEqual(['user', 'assistant', 'user'])
    const serialized = JSON.stringify(second)
    expect(serialized).toContain('First answer.')
    expect(serialized).not.toContain('<tool_results>')
    expect(serialized).not.toContain('TOOL RESULTS')
  })

  it('validates once after a turn in which a tool ran', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'Edited.' } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'legacy_tool', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })
    const validateOnComplete = vi.fn().mockResolvedValue({ ok: true })

    const { result } = setupLegacy({ validateOnComplete })
    await send(result, 'restyle it')

    // Email compiles MJML here: a second call per turn would double the compile
    // requests, a missing one would let a broken email be presented as success.
    expect(validateOnComplete).toHaveBeenCalledTimes(1)
  })

  it('ends the turn when the stream closes without a terminal event', async () => {
    // A dropped connection, or a split SSE frame swallowed by the JSON.parse in
    // llm.ts, delivers no `done`. If isStreaming stayed true the composer would be
    // dead for the rest of the session.
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'Half an answer' } as LLMChatEvent)
    })

    const { result } = setupLegacy()
    await send(result, 'ask something')

    expect(result.current.isStreaming).toBe(false)
    expect(result.current.messages.some((m) => m.content === 'Half an answer')).toBe(true)

    // ...and the next question is actually sent, rather than early-returning.
    await send(result, 'ask again')
    expect(llmApi.streamChat).toHaveBeenCalledTimes(2)
  })

  it('shows a single error bubble when the stream fails after streaming text', async () => {
    // streamChat delivers an SSE `error` frame to onEvent AND then to onError; the
    // user must see one failure, not the same one twice.
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent, onError) => {
      onEvent({ type: 'text', content: 'Starting to answer' } as LLMChatEvent)
      onEvent({ type: 'error', error: 'rate limited' } as LLMChatEvent)
      onError?.(new Error('rate limited'))
    })

    const { result } = setupLegacy()
    await send(result, 'ask during an outage')

    const failures = result.current.messages.filter((m) => m.content.includes('rate limited'))
    expect(failures).toHaveLength(1)
  })

  it('never sends two consecutive user turns after a turn whose only output was tool calls', async () => {
    // A tool-only round leaves no assistant message in state (the empty bubble is
    // replaced by the tool bubble) and tool rows never go on the wire, so the naive
    // transcript for the next turn is [user, user] - which every provider rejects.
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'tool_use', tool_name: 'legacy_tool', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setupLegacy()
    await send(result, 'first question')
    await send(result, 'second question')

    const second = sentTranscripts()[1]
    const adjacentSameRole = second.some((m, i) => i > 0 && second[i - 1].role === m.role)
    expect(adjacentSameRole).toBe(false)
    expect(second.every((m) => m.content.trim().length > 0)).toBe(true)
    // Neither question may be dropped in the process.
    const serialized = JSON.stringify(second)
    expect(serialized).toContain('first question')
    expect(serialized).toContain('second question')
  })
})

// Mirrors how BlogAIAssistant/EmailAIAssistant wire the hook into the panel, with the
// one addition a new consumer makes: starter chips.
function LegacyAssistantPanel({ suggestions }: { suggestions?: AIAssistantSuggestion[] }) {
  const assistant = useAIAssistant({
    workspace,
    config,
    tools: [],
    toolHandlers: handlers,
    buildSystemPrompt: () => 'SYSTEM'
  })
  return (
    <AIAssistantChat {...assistant} workspace={workspace} config={config} suggestions={suggestions} />
  )
}

describe('AIAssistantChat starter chips', () => {
  beforeEach(() => {
    vi.mocked(llmApi.streamChat).mockReset()
  })

  it('fills the composer without sending when a starter is clicked', async () => {
    render(<LegacyAssistantPanel suggestions={chips} />)
    // Closed panel renders only the floating button.
    fireEvent.click(screen.getByRole('button'))

    fireEvent.click(screen.getByText(chips[0].label))

    expect(screen.getByRole('textbox')).toHaveValue(chips[0].prompt)
    // Fill, never send: a starter must not spend the workspace's tokens on a click.
    expect(llmApi.streamChat).not.toHaveBeenCalled()
  })

  it('renders no starters for a consumer that supplies none', () => {
    render(<LegacyAssistantPanel />)
    fireEvent.click(screen.getByRole('button'))

    expect(screen.queryByText(chips[0].label)).toBeNull()
  })
})
