import type { ReactNode, RefObject } from 'react'
import type { LLMTool, LLMChatEvent } from '../../services/api/llm'
import type { Workspace, Integration } from '../../services/api/workspace'

export interface ChatMessage {
  key: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  loading?: boolean
  toolName?: string
}

export interface AIAssistantConfig {
  title: string
  icon: ReactNode           // 18px - for header
  iconButton: ReactNode     // 24px - for floating button
  iconLarge: ReactNode      // 32px - for "not configured" popup
  iconColor: string
  avatarColor: string
  placeholder: string
  maxTokens: number
  notConfiguredGradient: string
}

export type ToolHandler = (
  event: LLMChatEvent,
  insertToolMessage: (content: string, toolName: string) => void
) => void

export interface UseAIAssistantOptions {
  workspace: Workspace
  config: AIAssistantConfig
  tools: LLMTool[]
  toolHandlers: Map<string, ToolHandler>
  buildSystemPrompt: () => string
}

export interface UseAIAssistantReturn {
  open: boolean
  setOpen: (open: boolean) => void
  messages: ChatMessage[]
  inputValue: string
  setInputValue: (value: string) => void
  isStreaming: boolean
  costs: { input: number; output: number; total: number }
  inputContainerRef: RefObject<HTMLDivElement | null>
  llmIntegration: Integration | undefined
  handleCancel: () => void
  handleSend: () => Promise<void>
  bubbleItems: BubbleItem[]
  resetConversation: () => void
}

export interface BubbleItem {
  key: string
  role: 'user' | 'ai' | 'system'
  content: string
  loading?: boolean
  styles?: {
    content?: React.CSSProperties
  }
  avatar?: {
    icon: ReactNode
    size: number
    style: React.CSSProperties
  }
}

export interface AIAssistantChatProps {
  workspace: Workspace
  config: AIAssistantConfig
  open: boolean
  setOpen: (open: boolean) => void
  messages: ChatMessage[]
  inputValue: string
  setInputValue: (value: string) => void
  isStreaming: boolean
  costs: { input: number; output: number; total: number }
  inputContainerRef: RefObject<HTMLDivElement | null>
  llmIntegration: Integration | undefined
  handleCancel: () => void
  handleSend: () => Promise<void>
  bubbleItems: BubbleItem[]
  resetConversation: () => void
  hidden?: boolean
  chatBoxTop?: number
}
