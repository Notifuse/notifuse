import type { ReactNode, RefObject } from 'react'
import type { LLMTool, LLMChatEvent } from '../../services/api/llm'
import type { Workspace, Integration } from '../../services/api/workspace'

export interface ChatMessage {
  key: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  thinking?: string // accumulated reasoning ("thinking") tokens for assistant messages
  loading?: boolean
  toolName?: string
  // A tool bubble whose work failed: rendered with the error style, keeps its toolName.
  failed?: boolean
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

/**
 * What a tool hands BACK to the model.
 *
 * A handler that returns nothing is a pure side effect and behaves exactly as it
 * always has: the model never learns the outcome within this turn. A handler that
 * returns a ToolResult asks the hook to issue a follow-up request carrying that
 * output, so the model can reason over it before answering - but only if the
 * consumer also opted in with `maxToolRounds > 1`.
 *
 * `content` is model-facing text, sent verbatim as prompt tokens on every
 * subsequent round of the turn. Keep it compact, and never put a rendered SQL
 * string (`meta.query`) or bind values (`meta.params`) in it.
 */
export interface ToolResult {
  content: string
  // The tool failed. Reported to the model as ok:false so it can recover or apologise,
  // instead of the failure vanishing.
  isError?: boolean
  // Included in the payload when a continuation happens anyway, but never enough to
  // justify a billed round trip on its own. Two kinds of result set it: UI-mutation
  // acknowledgements, and the hook's own refusals (unknown tool, cross-round
  // duplicate, budget exhausted) - a call that was never executed must not be able to
  // buy the round the budget just denied it.
  silent?: boolean
}

export interface ToolBubbleHandle {
  update: (content: string, opts?: { failed?: boolean }) => void
}

export interface ToolRunContext {
  // Post a live tool bubble with a spinner and get a handle to finish it. The only way
  // an async browser query can narrate itself; insertToolMessage stays a one-shot.
  progress: (content: string, toolName?: string) => ToolBubbleHandle
  // Aborted when the user cancels, resets, or starts a new turn.
  signal: AbortSignal
  // 1-based; 1 is the user's own turn.
  round: number
}

// The third parameter and the widened return type are both source-compatible: JS
// ignores extra arguments, TypeScript accepts a shorter parameter list, and `void`
// is assignable to the union. Existing 2-parameter void handlers need no edit.
export type ToolHandler = (
  event: LLMChatEvent,
  insertToolMessage: (content: string, toolName: string) => void,
  ctx: ToolRunContext
) => void | ToolResult | Promise<ToolResult | void>

export interface UseAIAssistantOptions {
  workspace: Workspace
  config: AIAssistantConfig
  tools: LLMTool[]
  toolHandlers: Map<string, ToolHandler>
  buildSystemPrompt: () => string
  // Optional post-completion validation. Runs after a turn in which the assistant
  // ran at least one client-side tool (i.e. it edited something). When it reports
  // !ok, the returned errorText is surfaced as a persistent error in the chat so a
  // broken result is never presented as success.
  validateOnComplete?: () => Promise<{ ok: boolean; errorText?: string }>
  // Maximum assistant round trips inside ONE user turn.
  //
  // 1 (the default) disables the tool-result continuation loop entirely: a handler's
  // return value is discarded and exactly one POST /api/llm.chat is issued, which is
  // the historical behaviour. Above 1, a round in which at least one handler returned
  // a non-silent ToolResult is followed by another request carrying that output, up to
  // this many rounds. Clamped to 1..5 by the hook.
  maxToolRounds?: number
  // Icon per tool name, for the step line a tool posts while it runs.
  //
  // The hook knows only the two tools the SERVER runs (scrape_url, search_web) and
  // supplies their icons itself; every feature tool is named by the consumer, so its
  // icon comes from here rather than from a list the hook would have to grow for each
  // new feature. An entry here wins over the built-in default for the same name, and a
  // tool with no entry at all still gets a neutral mark so the step lines stay aligned.
  toolIcons?: Record<string, ReactNode>
}

export interface UseAIAssistantReturn {
  open: boolean
  setOpen: (open: boolean) => void
  messages: ChatMessage[]
  inputValue: string
  setInputValue: (value: string) => void
  isStreaming: boolean
  costs: { input: number; output: number; total: number }
  inputContainerRef: RefObject<HTMLDivElement>
  llmIntegration: Integration | undefined
  llmIntegrations: Integration[]
  setSelectedLLMIntegrationId: (id: string) => void
  handleCancel: () => void
  handleSend: () => Promise<void>
  bubbleItems: BubbleItem[]
  resetConversation: () => void
}

export interface BubbleItem {
  key: string
  role: 'user' | 'ai' | 'system' | 'thinking'
  content: string
  loading?: boolean
  // antd's Bubble variant, set per item so one role can carry two weights: a tool step
  // is 'borderless' - no fill, no border, it recedes behind the answer - while a failed
  // or errored tool keeps the default filled treatment and stays loud.
  variant?: 'filled' | 'outlined' | 'shadow' | 'borderless'
  styles?: {
    content?: React.CSSProperties
  }
  avatar?: {
    icon: ReactNode
    size: number
    style: React.CSSProperties
  }
}

/** A one-click starter shown only while the conversation is empty. */
export interface AIAssistantSuggestion {
  key: string
  /** Chip text. User-facing: translate. */
  label: string
  /** Text placed in the composer as if typed. User-facing: translate. */
  prompt: string
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
  inputContainerRef: RefObject<HTMLDivElement>
  llmIntegration: Integration | undefined
  llmIntegrations: Integration[]
  setSelectedLLMIntegrationId: (id: string) => void
  handleCancel: () => void
  handleSend: () => Promise<void>
  bubbleItems: BubbleItem[]
  resetConversation: () => void
  hidden?: boolean
  chatBoxTop?: number
  /**
   * Panel width in px. Defaults to the historical 420, which suits a prose
   * assistant; a feature whose answers carry small metric tables can ask for more
   * without changing the panel for the others.
   */
  width?: number
  /** Starters for the empty state. Omit for the historical blank panel. */
  suggestions?: AIAssistantSuggestion[]
  /** Chip click. Defaults to filling the composer without sending. */
  onSuggestion?: (prompt: string) => void
}
