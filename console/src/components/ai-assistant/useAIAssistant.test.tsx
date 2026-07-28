import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useAIAssistant } from './useAIAssistant'
import { llmApi, type LLMChatEvent } from '../../services/api/llm'
import type { AIAssistantConfig } from './types'

vi.mock('../../services/api/llm', () => ({
  llmApi: { streamChat: vi.fn() }
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const workspace: any = {
  id: 'ws1',
  integrations: [{ id: 'llm1', type: 'llm', name: 'LLM' }]
}

function setup() {
  return renderHook(() =>
    useAIAssistant({
      workspace,
      config,
      tools: [],
      toolHandlers: new Map(),
      buildSystemPrompt: () => 'SYSTEM'
    })
  )
}

async function send(result: { current: ReturnType<typeof useAIAssistant> }, text: string) {
  act(() => result.current.setInputValue(text))
  await act(async () => {
    await result.current.handleSend()
  })
}

describe('useAIAssistant reasoning channel', () => {
  beforeEach(() => {
    vi.mocked(llmApi.streamChat).mockReset()
  })

  it('accumulates thinking events onto the assistant message, separate from the answer', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'thinking', content: 'Let me plan ' } as LLMChatEvent)
      onEvent({ type: 'thinking', content: 'the layout.' } as LLMChatEvent)
      onEvent({ type: 'text', content: 'Here is your email.' } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setup()
    await send(result, 'design an email')

    const assistant = result.current.messages.find((m) => m.role === 'assistant')
    expect(assistant?.thinking).toBe('Let me plan the layout.')
    expect(assistant?.content).toBe('Here is your email.')
  })

  it('places tool messages after the assistant reply, in call order', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      // Text streams first, then the tool calls resolve (as providers emit them).
      onEvent({ type: 'text', content: "I'll update the colors!" } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'updateBlock', tool_input: { id: 'A' } } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'updateBlock', tool_input: { id: 'B' } } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'updateBlock', tool_input: { id: 'C' } } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })
    let n = 0
    const handlers = new Map([
      [
        'updateBlock',
        (_e: LLMChatEvent, insert: (c: string, t: string) => void) => insert(`Updated ${++n}`, 'updateBlock')
      ]
    ])
    const { result } = renderHook(() =>
      useAIAssistant({ workspace, config, tools: [], toolHandlers: handlers, buildSystemPrompt: () => 'S' })
    )
    await send(result, 'change primary colors to blue')

    const roles = result.current.messages.map((m) => `${m.role}:${m.content}`)
    // user, assistant text, then the three tool results in order — never tools above text.
    expect(roles).toEqual([
      'user:change primary colors to blue',
      "assistant:I'll update the colors!",
      'tool:Updated 1',
      'tool:Updated 2',
      'tool:Updated 3'
    ])
  })

  it('keeps the reasoning bubble when a tool-only turn produces no answer text', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'thinking', content: 'planning the layout' } as LLMChatEvent)
      onEvent({ type: 'tool_use', tool_name: 'setEmailTree', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })
    // A handler that actually inserts a tool message (the email tools do this).
    const handlers = new Map([
      ['setEmailTree', (_e: LLMChatEvent, insert: (c: string, n: string) => void) => insert('Email updated', 'setEmailTree')]
    ])
    const { result } = renderHook(() =>
      useAIAssistant({
        workspace,
        config,
        tools: [],
        toolHandlers: handlers,
        buildSystemPrompt: () => 'SYSTEM'
      })
    )
    await send(result, 'design it')

    // The assistant message carrying reasoning must survive (not be replaced by the
    // tool message, which is what happens for an empty assistant with no thinking).
    const assistant = result.current.messages.find((m) => m.role === 'assistant' && m.thinking)
    expect(assistant?.thinking).toBe('planning the layout')
    expect(
      result.current.messages.some((m) => m.role === 'tool' && m.content === 'Email updated')
    ).toBe(true)
  })

  it('does not send accumulated thinking back to the model on the next turn', async () => {
    const sentMessages: Array<Array<{ role: string; content: string }>> = []
    vi.mocked(llmApi.streamChat).mockImplementation(async (params, onEvent) => {
      sentMessages.push(params.messages)
      onEvent({ type: 'thinking', content: 'secret reasoning' } as LLMChatEvent)
      onEvent({ type: 'text', content: 'Answer one' } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const { result } = setup()
    await send(result, 'first question')
    await send(result, 'second question')

    // The second request carries the prior turn but never the reasoning text.
    const second = sentMessages[1]
    const serialized = JSON.stringify(second)
    expect(serialized).toContain('Answer one')
    expect(serialized).toContain('first question')
    expect(serialized).not.toContain('secret reasoning')
  })
})

describe('useAIAssistant truncation notice', () => {
  beforeEach(() => {
    vi.mocked(llmApi.streamChat).mockReset()
  })

  it('appends a non-destructive warning and keeps streamed text when done.truncated is set', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'Partial answer that was building' } as LLMChatEvent)
      onEvent({ type: 'done', truncated: true } as LLMChatEvent)
    })

    const { result } = setup()
    await send(result, 'write a long thing')

    // The streamed assistant content must survive (not be replaced by an error).
    const assistant = result.current.messages.find((m) => m.role === 'assistant')
    expect(assistant?.content).toBe('Partial answer that was building')
    // ...and a persistent truncation warning is appended separately.
    const warning = result.current.messages.find(
      (m) => m.toolName === '__error__' && m.content.includes('token limit')
    )
    expect(warning, 'a truncation warning should be appended').toBeTruthy()
  })

  it('does not warn when done is not truncated', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'Complete answer.' } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })
    const { result } = setup()
    await send(result, 'short thing')
    expect(result.current.messages.some((m) => m.toolName === '__error__')).toBe(false)
  })
})

describe('useAIAssistant post-completion validation', () => {
  beforeEach(() => {
    vi.mocked(llmApi.streamChat).mockReset()
  })

  function setupWithValidation(validateOnComplete: () => Promise<{ ok: boolean; errorText?: string }>) {
    return renderHook(() =>
      useAIAssistant({
        workspace,
        config,
        tools: [],
        toolHandlers: new Map([['setEmailTree', () => {}]]),
        buildSystemPrompt: () => 'SYSTEM',
        validateOnComplete
      })
    )
  }

  it('surfaces a persistent error when the edited result fails validation', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'tool_use', tool_name: 'setEmailTree', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'text', content: 'Done! Your email is ready.' } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })

    const validateOnComplete = vi
      .fn()
      .mockResolvedValue({ ok: false, errorText: 'line 3: mj-button width must be px' })

    const { result } = setupWithValidation(validateOnComplete)
    await send(result, 'make it earthy')

    expect(validateOnComplete).toHaveBeenCalledTimes(1)
    const errorBubble = result.current.messages.find(
      (m) => m.role === 'tool' && m.content.includes('mj-button width must be px')
    )
    expect(errorBubble, 'a persistent error message should be appended').toBeTruthy()
  })

  it('does not validate or warn when no tool ran (plain answer)', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'text', content: 'Here is some advice.' } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })
    const validateOnComplete = vi.fn().mockResolvedValue({ ok: true })

    const { result } = setupWithValidation(validateOnComplete)
    await send(result, 'what makes a good email?')

    expect(validateOnComplete).not.toHaveBeenCalled()
    expect(result.current.messages.some((m) => m.toolName === '__error__')).toBe(false)
  })

  it('does not warn when the edited result validates cleanly', async () => {
    vi.mocked(llmApi.streamChat).mockImplementation(async (_params, onEvent) => {
      onEvent({ type: 'tool_use', tool_name: 'setEmailTree', tool_input: {} } as LLMChatEvent)
      onEvent({ type: 'done' } as LLMChatEvent)
    })
    const validateOnComplete = vi.fn().mockResolvedValue({ ok: true })

    const { result } = setupWithValidation(validateOnComplete)
    await send(result, 'build it')

    expect(validateOnComplete).toHaveBeenCalledTimes(1)
    expect(result.current.messages.some((m) => m.toolName === '__error__')).toBe(false)
  })
})
