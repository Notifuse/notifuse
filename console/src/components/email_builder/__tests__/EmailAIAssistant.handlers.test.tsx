import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import type { UseAIAssistantOptions, ToolHandler } from '../../ai-assistant'
import type { LLMChatEvent } from '../../../services/api/llm'
import type { EmailBlock } from '../types'
import { EmailAIAssistant } from '../EmailAIAssistant'
import { TOOL_NAMES } from '../email-ai-tools'
import type { Workspace } from '../../../services/api/workspace'

/**
 * The component builds its tool handlers inline, so the only way to exercise them is to
 * capture the options it hands the shared hook. Rendering the real chat is not the point
 * here and pulls in the whole Ant Design X surface, so the hook and the chat are stubbed.
 */
let captured: UseAIAssistantOptions | undefined

vi.mock('../../ai-assistant', () => ({
  useAIAssistant: (options: UseAIAssistantOptions) => {
    captured = options
    return { open: false, bubbleItems: [], messages: [] }
  },
  AIAssistantChat: () => null
}))

const tree: EmailBlock = {
  id: 'mjml-1',
  type: 'mjml',
  children: [{ id: 'body-1', type: 'mj-body', children: [] }]
}

const workspace = { id: 'ws1', integrations: [] } as unknown as Workspace

const setEmailTree = vi.fn()
const insert = vi.fn()

function handlerFor(name: string): ToolHandler {
  render(
    <EmailAIAssistant
      workspace={workspace}
      callbacks={{
        getEmailTree: () => tree,
        setEmailTree,
        onAddBlock: vi.fn(),
        onUpdateBlock: vi.fn(),
        onDeleteBlock: vi.fn(),
        onMoveBlock: vi.fn(),
        onSelectBlock: vi.fn()
      }}
    />
  )
  const handler = captured?.toolHandlers.get(name)
  if (!handler) throw new Error(`no handler registered for ${name}`)
  return handler
}

const event = (input: unknown): LLMChatEvent =>
  ({ type: 'tool_use', tool_name: TOOL_NAMES.SET_EMAIL_TREE, tool_input: input }) as LLMChatEvent

describe('EmailAIAssistant setEmailTree handler', () => {
  beforeEach(() => {
    captured = undefined
    vi.clearAllMocks()
  })

  // A response cut off by the token limit arrives as a tool call with no tree. This used
  // to return silently, so the thread showed an updated subject, no email, and nothing
  // at all to say why.
  it('says so in the thread when the tool call carries no tree', () => {
    handlerFor(TOOL_NAMES.SET_EMAIL_TREE)(event({}), insert, {
      progress: vi.fn(),
      signal: new AbortController().signal,
      round: 1
    })

    expect(setEmailTree).not.toHaveBeenCalled()
    expect(insert).toHaveBeenCalledTimes(1)
    expect(insert.mock.calls[0][0]).toMatch(/was not applied/i)
    expect(insert.mock.calls[0][0]).toMatch(/token limit/i)
  })

  it('applies a well-formed tree', () => {
    handlerFor(TOOL_NAMES.SET_EMAIL_TREE)(event({ tree }), insert, {
      progress: vi.fn(),
      signal: new AbortController().signal,
      round: 1
    })

    expect(setEmailTree).toHaveBeenCalledTimes(1)
    expect(insert.mock.calls[0][0]).toBe('Email structure replaced')
  })

  // The acknowledgement is what buys the continuation round for a model that wrote no
  // prose; returning nothing leaves the hook with an empty result set and the turn ends
  // silently. It stays `silent` so it never buys a round on its own.
  it('returns a silent acknowledgement so a wordless turn can still be answered', () => {
    const result = handlerFor(TOOL_NAMES.SET_EMAIL_TREE)(event({ tree }), insert, {
      progress: vi.fn(),
      signal: new AbortController().signal,
      round: 1
    })

    expect(result).toEqual({ content: 'Email structure replaced', silent: true })
  })
})
