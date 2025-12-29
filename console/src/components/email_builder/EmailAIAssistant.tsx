import { message } from 'antd'
import { Wand2 } from 'lucide-react'
import { useAIAssistant, AIAssistantChat } from '../ai-assistant'
import type { AIAssistantConfig, ToolHandler } from '../ai-assistant'
import type { Workspace } from '../../services/api/workspace'
import type { EmailBlock, MJMLComponentType } from './types'
import { EmailBlockClass } from './EmailBlockClass'
import {
  EMAIL_AI_TOOLS,
  TOOL_NAMES,
  serializeEmailTree,
  type EmailAIAgentCallbacks
} from './email-ai-tools'
import { EMAIL_AI_SYSTEM_PROMPT } from './email-ai-system-prompt'

interface EmailAIAssistantProps {
  workspace: Workspace
  callbacks: EmailAIAgentCallbacks
  currentSubject?: string
  currentPreviewText?: string
  onUpdateSubject?: (subject: string) => void
  onUpdatePreviewText?: (preview: string) => void
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
  maxTokens: 8192,
  notConfiguredGradient: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)'
}

export function EmailAIAssistant({
  workspace,
  callbacks,
  currentSubject,
  currentPreviewText,
  onUpdateSubject,
  onUpdatePreviewText,
  hidden = false
}: EmailAIAssistantProps) {
  const buildSystemPrompt = () => {
    let systemPrompt = EMAIL_AI_SYSTEM_PROMPT

    const currentTree = callbacks.getEmailTree()
    if (currentTree) {
      systemPrompt += `\n\n## Current Email Structure\n\n${serializeEmailTree(currentTree)}`
    }
    if (currentSubject) {
      systemPrompt += `\n\nCurrent subject: "${currentSubject}"`
    }
    if (currentPreviewText) {
      systemPrompt += `\nCurrent preview text: "${currentPreviewText}"`
    }

    return systemPrompt
  }

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
      }
    ],
    [
      TOOL_NAMES.SELECT_BLOCK,
      (event, insert) => {
        const input = event.tool_input as { blockId: string }
        if (!input?.blockId) return
        callbacks.onSelectBlock(input.blockId)
        insert(`Selected block ${input.blockId}`, TOOL_NAMES.SELECT_BLOCK)
      }
    ],
    [
      TOOL_NAMES.SET_EMAIL_TREE,
      (event, insert) => {
        const input = event.tool_input as { tree: EmailBlock }
        if (!input?.tree) return

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
        }
      }
    ]
  ])

  const assistant = useAIAssistant({
    workspace,
    config,
    tools: EMAIL_AI_TOOLS,
    toolHandlers,
    buildSystemPrompt
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
