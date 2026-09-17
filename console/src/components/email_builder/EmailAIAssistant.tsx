import { message } from 'antd'
import { Wand2 } from 'lucide-react'
import { useAIAssistant, AIAssistantChat } from '../ai-assistant'
import type { AIAssistantConfig, ToolHandler } from '../ai-assistant'
import type { Workspace } from '../../services/api/workspace'
import type { EmailBlock, MJMLComponentType } from './types'
import { EmailBlockClass } from './EmailBlockClass'
import { EMAIL_AI_TOOLS, TOOL_NAMES, type EmailAIAgentCallbacks } from './email-ai-tools'
import { buildEmailSystemPrompt } from './email-ai-system-prompt'

export interface EmailAIAssistantProps {
  workspace: Workspace
  callbacks: EmailAIAgentCallbacks
  currentSubject?: string
  currentPreviewText?: string
  // Template category picked in the Settings tab. It selects the compliance guidance in
  // the system prompt — a marketing email needs an unsubscribe footer, a transactional
  // one must not have one — and is undefined on a new template until the user picks it.
  category?: string
  onUpdateSubject?: (subject: string) => void
  onUpdatePreviewText?: (preview: string) => void
  // Validates the current email after the assistant edits it (e.g. compiles MJML)
  // and reports any errors so they can be surfaced instead of a silent broken result.
  validateOnComplete?: () => Promise<{ ok: boolean; errorText?: string }>
  hidden?: boolean
}

const config: AIAssistantConfig = {
  title: 'AI Email Designer',
  icon: <Wand2 size={18} />,
  iconButton: <Wand2 size={24} />,
  iconLarge: <Wand2 size={32} />,
  iconColor: '#764ba2',
  avatarColor: '#764ba2',
  placeholder: 'Ask me to design your email...',
  // Kept at 8192: this is sent as max_tokens to every provider, and some reasoning
  // models (e.g. DeepSeek-reasoner) hard-cap output at 8192 and 400 on higher values.
  // When reasoning still exhausts the budget the backend flags `truncated` and the
  // user is told to lower the reasoning effort rather than the request silently failing.
  maxTokens: 8192,
  notConfiguredGradient: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)'
}

export function EmailAIAssistant({
  workspace,
  callbacks,
  currentSubject,
  currentPreviewText,
  category,
  onUpdateSubject,
  onUpdatePreviewText,
  validateOnComplete,
  hidden = false
}: EmailAIAssistantProps) {
  // Deliberately not memoized: the hook refreshes its ref on every render and calls this
  // once per request round, so a category or a tree changed mid-conversation reaches the
  // next round. A useCallback here would pin a stale closure instead.
  const buildSystemPrompt = () =>
    buildEmailSystemPrompt({
      tree: callbacks.getEmailTree(),
      subject: currentSubject,
      previewText: currentPreviewText,
      category
    })

  const toolHandlers = new Map<string, ToolHandler>([
    [
      TOOL_NAMES.UPDATE_BLOCK,
      (event, insert) => {
        const input = event.tool_input as {
          blockId: string
          updates: Partial<EmailBlock>
        }
        if (!input?.blockId || !input?.updates) return
        callbacks.onUpdateBlock(input.blockId, input.updates)
        insert(`Updated block ${input.blockId}`, TOOL_NAMES.UPDATE_BLOCK)
        message.success('Block updated')
        return { content: `Updated block ${input.blockId}`, silent: true }
      }
    ],
    [
      TOOL_NAMES.ADD_BLOCK,
      (event, insert) => {
        const input = event.tool_input as {
          parentId: string
          blockType: MJMLComponentType
          position?: number
          content?: string
          attributes?: Record<string, unknown>
        }
        if (!input?.parentId || !input?.blockType) return
        callbacks.onAddBlock(
          input.parentId,
          input.blockType,
          input.position,
          input.content,
          input.attributes
        )
        insert(`Added ${input.blockType} to ${input.parentId}`, TOOL_NAMES.ADD_BLOCK)
        message.success(`Added ${input.blockType}`)
        return { content: `Added ${input.blockType} to ${input.parentId}`, silent: true }
      }
    ],
    [
      TOOL_NAMES.DELETE_BLOCK,
      (event, insert) => {
        const input = event.tool_input as { blockId: string }
        if (!input?.blockId) return
        callbacks.onDeleteBlock(input.blockId)
        insert(`Deleted block ${input.blockId}`, TOOL_NAMES.DELETE_BLOCK)
        message.success('Block deleted')
        return { content: `Deleted block ${input.blockId}`, silent: true }
      }
    ],
    [
      TOOL_NAMES.MOVE_BLOCK,
      (event, insert) => {
        const input = event.tool_input as {
          blockId: string
          newParentId: string
          position: number
        }
        if (!input?.blockId || !input?.newParentId || input?.position === undefined) return
        callbacks.onMoveBlock(input.blockId, input.newParentId, input.position)
        insert(`Moved block ${input.blockId} to ${input.newParentId}`, TOOL_NAMES.MOVE_BLOCK)
        message.success('Block moved')
        return { content: `Moved block ${input.blockId} to ${input.newParentId}`, silent: true }
      }
    ],
    [
      TOOL_NAMES.SELECT_BLOCK,
      (event, insert) => {
        const input = event.tool_input as { blockId: string }
        if (!input?.blockId) return
        callbacks.onSelectBlock(input.blockId)
        insert(`Selected block ${input.blockId}`, TOOL_NAMES.SELECT_BLOCK)
        return { content: `Selected block ${input.blockId}`, silent: true }
      }
    ],
    [
      TOOL_NAMES.SET_EMAIL_TREE,
      (event, insert) => {
        const input = event.tool_input as { tree: EmailBlock }
        if (!input?.tree) {
          // A tool call with no tree is what a response cut off by the token limit
          // looks like from here. Returning quietly left the thread showing a
          // successful subject update and no email, with nothing to explain it.
          insert(
            'The email structure was not applied: the request arrived without a tree, which usually means the response hit the token limit. Ask me again, or ask for a simpler layout.',
            TOOL_NAMES.SET_EMAIL_TREE
          )
          message.error('Email structure not applied')
          return
        }

        const errors = EmailBlockClass.validateStructure(input.tree)
        if (errors.length > 0) {
          insert(`Tree validation failed: ${errors.join(', ')}`, TOOL_NAMES.SET_EMAIL_TREE)
          message.error('Invalid tree structure')
          return
        }

        const treeWithNewIds = EmailBlockClass.regenerateIds(input.tree)
        callbacks.setEmailTree(treeWithNewIds)
        insert('Email structure replaced', TOOL_NAMES.SET_EMAIL_TREE)
        message.success('Email template updated')
        return { content: 'Email structure replaced', silent: true }
      }
    ],
    [
      TOOL_NAMES.UPDATE_EMAIL_METADATA,
      (event, insert) => {
        const input = event.tool_input as {
          subject?: string
          preview_text?: string
        }
        if (!input) return

        const updates: string[] = []
        if (input.subject && onUpdateSubject) {
          onUpdateSubject(input.subject)
          updates.push('subject')
        }
        if (input.preview_text && onUpdatePreviewText) {
          onUpdatePreviewText(input.preview_text)
          updates.push('preview text')
        }

        if (updates.length > 0) {
          insert(`Updated ${updates.join(' and ')}`, TOOL_NAMES.UPDATE_EMAIL_METADATA)
          message.success(`Updated ${updates.join(' and ')}`)
          return { content: `Updated ${updates.join(' and ')}`, silent: true }
        }
      }
    ]
  ])

  const assistant = useAIAssistant({
    workspace,
    config,
    tools: EMAIL_AI_TOOLS,
    toolHandlers,
    buildSystemPrompt,
    validateOnComplete,
    // Two, not one: the second round exists only for a model that emitted tool calls and
    // no prose, so it can finally write its reply. A model that already answered never
    // reaches it - the loop stops as soon as a round comes back without tool calls.
    maxToolRounds: 2
  })

  return (
    <AIAssistantChat
      {...assistant}
      workspace={workspace}
      config={config}
      hidden={hidden}
      chatBoxTop={116}
    />
  )
}
