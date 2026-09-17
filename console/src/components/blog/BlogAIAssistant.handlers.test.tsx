import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import type { UseAIAssistantOptions, ToolHandler } from '../ai-assistant'
import type { LLMChatEvent } from '../../services/api/llm'
import type { Workspace } from '../../services/api/workspace'
import { BlogAIAssistant } from './BlogAIAssistant'
import { BLOG_TOOL_NAMES } from './blog-ai-tools'

/**
 * The component builds its tool handlers inline, so the only way to exercise them is to
 * capture the options it hands the shared hook.
 */
let captured: UseAIAssistantOptions | undefined

vi.mock('../ai-assistant', () => ({
  useAIAssistant: (options: UseAIAssistantOptions) => {
    captured = options
    return { open: false, bubbleItems: [], messages: [] }
  },
  AIAssistantChat: () => null
}))

const workspace = { id: 'ws1', integrations: [] } as unknown as Workspace
const onUpdateContent = vi.fn()
const onUpdateMetadata = vi.fn()
const insert = vi.fn()

function handlerFor(name: string): ToolHandler {
  render(
    <BlogAIAssistant
      workspace={workspace}
      onUpdateContent={onUpdateContent}
      onUpdateMetadata={onUpdateMetadata}
    />
  )
  const handler = captured?.toolHandlers.get(name)
  if (!handler) throw new Error(`no handler registered for ${name}`)
  return handler
}

const event = (input: unknown): LLMChatEvent =>
  ({ type: 'tool_use', tool_name: BLOG_TOOL_NAMES.UPDATE_CONTENT, tool_input: input }) as LLMChatEvent

const ctx = () => ({ progress: vi.fn(), signal: new AbortController().signal, round: 1 })

describe('BlogAIAssistant update_blog_content handler', () => {
  beforeEach(() => {
    captured = undefined
    vi.clearAllMocks()
  })

  // A response cut off by the token limit arrives as a tool call with no document. This
  // used to return silently, leaving the thread with no article and nothing to explain it.
  it('says so in the thread when the tool call carries no content', () => {
    handlerFor(BLOG_TOOL_NAMES.UPDATE_CONTENT)(event({ message: 'Done' }), insert, ctx())

    expect(onUpdateContent).not.toHaveBeenCalled()
    expect(insert).toHaveBeenCalledTimes(1)
    expect(insert.mock.calls[0][0]).toMatch(/was not applied/i)
    expect(insert.mock.calls[0][0]).toMatch(/token limit/i)
  })

  it('applies a well-formed document', () => {
    const doc = { type: 'doc', content: [] }
    handlerFor(BLOG_TOOL_NAMES.UPDATE_CONTENT)(event({ content: doc }), insert, ctx())

    expect(onUpdateContent).toHaveBeenCalledWith(doc)
    expect(insert.mock.calls[0][0]).toBe('Content updated')
  })

  // The acknowledgement is what buys the continuation round for a model that wrote no
  // prose. It stays `silent` so it never buys a round on its own.
  it('returns a silent acknowledgement so a wordless turn can still be answered', () => {
    const doc = { type: 'doc', content: [] }
    const result = handlerFor(BLOG_TOOL_NAMES.UPDATE_CONTENT)(
      event({ content: doc }),
      insert,
      ctx()
    )

    expect(result).toEqual({ content: 'Content updated', silent: true })
  })
})
