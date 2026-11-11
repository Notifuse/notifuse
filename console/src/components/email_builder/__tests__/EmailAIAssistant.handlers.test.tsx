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
const onUpdateBlock = vi.fn()
const insert = vi.fn()

function handlerFor(name: string, current: EmailBlock = tree): ToolHandler {
  render(
    <EmailAIAssistant
      workspace={workspace}
      callbacks={{
        getEmailTree: () => current,
        setEmailTree,
        onAddBlock: vi.fn(),
        onUpdateBlock,
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

const context = () => ({
  progress: vi.fn(),
  signal: new AbortController().signal,
  round: 1
})

const textBlock: EmailBlock = {
  id: 'text-1',
  type: 'mj-text',
  content: '<p>Hi</p>',
  attributes: {}
}

const treeWithText: EmailBlock = {
  id: 'mjml-1',
  type: 'mjml',
  children: [
    {
      id: 'body-1',
      type: 'mj-body',
      children: [
        {
          id: 'section-1',
          type: 'mj-section',
          children: [{ id: 'column-1', type: 'mj-column', children: [textBlock] }]
        }
      ]
    }
  ]
} as EmailBlock

const updateEvent = (input: unknown): LLMChatEvent =>
  ({ type: 'tool_use', tool_name: TOOL_NAMES.UPDATE_BLOCK, tool_input: input }) as LLMChatEvent

// A condition the server would reject must not reach the editor: the template would
// then fail to save, far from the turn that wrote it.
describe('EmailAIAssistant display conditions', () => {
  beforeEach(() => {
    captured = undefined
    vi.clearAllMocks()
  })

  it('refuses a tree carrying an invalid display condition', () => {
    const invalid = JSON.parse(JSON.stringify(treeWithText)) as EmailBlock
    const text = invalid.children![0].children![0].children![0].children![0]
    text.display_condition = { match: 'all', rules: [] }

    handlerFor(TOOL_NAMES.SET_EMAIL_TREE)(event({ tree: invalid }), insert, context())

    expect(setEmailTree).not.toHaveBeenCalled()
    expect(insert.mock.calls[0][0]).toMatch(/Tree validation failed/)
    expect(insert.mock.calls[0][0]).toMatch(/text-1/)
  })

  it('refuses a display condition on a column', () => {
    const invalid = JSON.parse(JSON.stringify(treeWithText)) as EmailBlock
    invalid.children![0].children![0].children![0].display_condition = {
      match: 'all',
      rules: [{ variable: 'contact.email', operator: 'is_present' }]
    }

    handlerFor(TOOL_NAMES.SET_EMAIL_TREE)(event({ tree: invalid }), insert, context())

    expect(setEmailTree).not.toHaveBeenCalled()
    expect(insert.mock.calls[0][0]).toMatch(
      /column-1 \(mj-column\) cannot have a display condition/
    )
  })

  it('applies a tree with a valid display condition and keeps it', () => {
    const valid = JSON.parse(JSON.stringify(treeWithText)) as EmailBlock
    const condition = {
      match: 'any' as const,
      rules: [{ variable: 'order.total', operator: 'gt' as const, value: '100' }]
    }
    valid.children![0].children![0].children![0].children![0].display_condition = condition

    handlerFor(TOOL_NAMES.SET_EMAIL_TREE)(event({ tree: valid }), insert, context())

    expect(setEmailTree).toHaveBeenCalledTimes(1)
    const applied = setEmailTree.mock.calls[0][0] as EmailBlock
    expect(applied.children![0].children![0].children![0].children![0].display_condition).toEqual(
      condition
    )
  })

  it('refuses an update with an invalid display condition', () => {
    handlerFor(TOOL_NAMES.UPDATE_BLOCK, treeWithText)(
      updateEvent({
        blockId: 'text-1',
        updates: {
          display_condition: {
            match: 'all',
            rules: [{ variable: 'x', operator: 'gt', value: 'abc' }]
          }
        }
      }),
      insert,
      context()
    )

    expect(onUpdateBlock).not.toHaveBeenCalled()
    expect(insert.mock.calls[0][0]).toMatch(/^Block not updated: /)
  })

  it('refuses a display condition on an ineligible block', () => {
    handlerFor(TOOL_NAMES.UPDATE_BLOCK, treeWithText)(
      updateEvent({
        blockId: 'column-1',
        updates: {
          display_condition: { match: 'all', rules: [{ variable: 'x', operator: 'is_true' }] }
        }
      }),
      insert,
      context()
    )

    expect(onUpdateBlock).not.toHaveBeenCalled()
    expect(insert.mock.calls[0][0]).toMatch(/cannot have a display condition/)
  })

  it('passes a valid condition, and its removal, through to the editor', () => {
    const handler = handlerFor(TOOL_NAMES.UPDATE_BLOCK, treeWithText)
    const condition = {
      match: 'all',
      rules: [{ variable: 'contact.first_name', operator: 'is_present' }]
    }

    handler(
      updateEvent({ blockId: 'text-1', updates: { display_condition: condition } }),
      insert,
      context()
    )
    handler(
      updateEvent({ blockId: 'text-1', updates: { display_condition: null } }),
      insert,
      context()
    )

    expect(onUpdateBlock).toHaveBeenNthCalledWith(1, 'text-1', { display_condition: condition })
    expect(onUpdateBlock).toHaveBeenNthCalledWith(2, 'text-1', { display_condition: null })
  })
})
