import { useState, useRef, useEffect } from 'react'
import { Search, Globe } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { llmApi, LLMChatEvent, LLMMessage } from '../../services/api/llm'
import type {
  ChatMessage,
  UseAIAssistantOptions,
  UseAIAssistantReturn,
  BubbleItem
} from './types'

// Server-side tool names (for styling)
const SERVER_TOOLS = {
  SCRAPE_URL: 'scrape_url',
  SEARCH_WEB: 'search_web'
} as const

// Marker toolName for persistent error bubbles (styled distinctly).
const ERROR_TOOL_NAME = '__error__'

export function useAIAssistant(options: UseAIAssistantOptions): UseAIAssistantReturn {
  const { workspace, config, tools, toolHandlers, buildSystemPrompt, validateOnComplete } = options
  const { t } = useLingui()

  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [costs, setCosts] = useState({ input: 0, output: 0, total: 0 })
  const abortControllerRef = useRef<AbortController | null>(null)
  const inputContainerRef = useRef<HTMLDivElement | null>(null)

  const llmIntegrations = workspace.integrations?.filter((i) => i.type === 'llm') ?? []
  const [selectedLLMIntegrationId, setSelectedLLMIntegrationId] = useState<string | undefined>(
    undefined
  )
  // Resolve the active integration from the selection, defaulting to the first configured one
  const llmIntegration =
    llmIntegrations.find((i) => i.id === selectedLLMIntegrationId) ?? llmIntegrations[0]

  // Focus the input when opening
  useEffect(() => {
    if (open) {
      setTimeout(() => {
        const textarea = inputContainerRef.current?.querySelector('textarea')
        textarea?.focus()
      }, 100)
    }
  }, [open])

  const handleCancel = () => {
    abortControllerRef.current?.abort()
    setIsStreaming(false)
    setMessages((prev) =>
      prev
        .map((m) => (m.loading ? { ...m, loading: false, content: m.content || t`(Cancelled)` } : m))
        .filter((m) => m.content.trim())
    )
  }

  const insertToolMessage = (
    assistantKey: string,
    content: string,
    toolName: string,
    loading = false
  ) => {
    setMessages((prev) => {
      const assistantIndex = prev.findIndex((m) => m.key === assistantKey)
      const newToolMessage: ChatMessage = {
        // Unique even when several tool calls resolve within the same millisecond.
        key: `tool-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        role: 'tool',
        content,
        toolName,
        loading
      }

      if (assistantIndex === -1) {
        return [...prev, newToolMessage]
      }

      const assistant = prev[assistantIndex]
      // If the assistant produced nothing visible (no text, no reasoning), replace its
      // empty bubble with the tool result so no blank bubble is shown above it.
      if (!assistant.content.trim() && !assistant.thinking?.trim()) {
        return [...prev.slice(0, assistantIndex), newToolMessage, ...prev.slice(assistantIndex + 1)]
      }

      // Otherwise keep the assistant's text/reasoning and append the tool result AFTER
      // it: tool calls stream after the assistant's message, so they belong below it,
      // and appending preserves the order of multiple tool calls within one turn.
      const cleared = prev.map((m) => (m.key === assistantKey ? { ...m, loading: false } : m))
      return [...cleared, newToolMessage]
    })
  }

  const handleTextEvent = (event: LLMChatEvent, assistantKey: string) => {
    if (!event.content) return
    setMessages((prev) =>
      prev.map((m) =>
        m.key === assistantKey ? { ...m, content: m.content + event.content, loading: false } : m
      )
    )
  }

  const handleThinkingEvent = (event: LLMChatEvent, assistantKey: string) => {
    if (!event.content) return
    // Accumulate reasoning on a separate field; keep `loading` until the answer
    // (text/tool) starts, so the assistant bubble still shows progress.
    setMessages((prev) =>
      prev.map((m) =>
        m.key === assistantKey ? { ...m, thinking: (m.thinking || '') + event.content } : m
      )
    )
  }

  const handleServerToolStart = (event: LLMChatEvent, assistantKey: string) => {
    const toolInput = event.tool_input || {}
    let displayText = t`Using ${event.tool_name}...`
    if (event.tool_name === SERVER_TOOLS.SCRAPE_URL && toolInput.url) {
      displayText = t`Fetching: ${toolInput.url}`
    } else if (event.tool_name === SERVER_TOOLS.SEARCH_WEB && toolInput.query) {
      displayText = t`Searching: "${toolInput.query}"`
    }
    insertToolMessage(assistantKey, displayText, event.tool_name || '', true)
  }

  const handleServerToolResult = (event: LLMChatEvent) => {
    setMessages((prev) => {
      const lastToolIndex = [...prev]
        .reverse()
        .findIndex((m) => m.role === 'tool' && m.toolName === event.tool_name && m.loading)
      if (lastToolIndex === -1) return prev
      const actualIndex = prev.length - 1 - lastToolIndex
      const currentMessage = prev[actualIndex]
      let statusText = currentMessage.content.replace('...', '')
      statusText += event.error ? t` - Failed` : t` - Done`
      return prev.map((m, i) =>
        i === actualIndex ? { ...m, content: statusText, loading: false } : m
      )
    })
  }

  const handleDoneEvent = (event: LLMChatEvent, assistantKey: string) => {
    if (event.input_cost !== undefined || event.output_cost !== undefined) {
      setCosts((prev) => ({
        input: prev.input + (event.input_cost || 0),
        output: prev.output + (event.output_cost || 0),
        total: prev.total + (event.total_cost || 0)
      }))
    }
    setMessages((prev) => prev.map((m) => (m.key === assistantKey ? { ...m, loading: false } : m)))
    setIsStreaming(false)
    // Non-destructive notice: the response hit the token cap before finishing
    // (common with reasoning models whose thinking eats the budget). The streamed
    // content is kept; we just append a warning.
    if (event.truncated) {
      appendErrorMessage(
        t`The response was cut off because it reached the token limit. Lower the reasoning effort, simplify the request, or raise the token limit, then try again.`
      )
    }
  }

  const handleErrorEvent = (event: LLMChatEvent, assistantKey: string) => {
    setMessages((prev) =>
      prev.map((m) =>
        m.key === assistantKey ? { ...m, content: t`Error: ${event.error}`, loading: false } : m
      )
    )
    setIsStreaming(false)
  }

  // Append a persistent error bubble (distinct from the transient antd toast) so a
  // failure stays visible in the conversation rather than vanishing.
  const appendErrorMessage = (content: string) => {
    setMessages((prev) => [
      ...prev,
      { key: `error-${Date.now()}`, role: 'tool', toolName: ERROR_TOOL_NAME, content }
    ])
  }

  const handleSend = async () => {
    if (!inputValue.trim() || !llmIntegration || isStreaming) return

    const userMessage: ChatMessage = {
      key: `user-${Date.now()}`,
      role: 'user',
      content: inputValue
    }

    const assistantKey = `assistant-${Date.now()}`
    const assistantMessage: ChatMessage = {
      key: assistantKey,
      role: 'assistant',
      content: '',
      loading: true
    }

    setMessages((prev) => [...prev, userMessage, assistantMessage])
    setInputValue('')
    setIsStreaming(true)

    const systemPrompt = buildSystemPrompt()

    const apiMessages: LLMMessage[] = messages
      .filter((m) => m.role !== 'tool' && m.content.trim())
      .map((m) => ({ role: m.role as 'user' | 'assistant', content: m.content }))
    apiMessages.push({ role: 'user', content: inputValue })

    abortControllerRef.current = new AbortController()

    // Track whether the assistant actually edited anything this turn; validation
    // only matters when a client-side tool ran.
    let clientToolRan = false

    try {
      await llmApi.streamChat(
        {
          workspace_id: workspace.id,
          integration_id: llmIntegration.id,
          messages: apiMessages,
          system_prompt: systemPrompt,
          max_tokens: config.maxTokens,
          tools
        },
        (event: LLMChatEvent) => {
          switch (event.type) {
            case 'text':
              handleTextEvent(event, assistantKey)
              break
            case 'thinking':
              handleThinkingEvent(event, assistantKey)
              break
            case 'tool_use': {
              const handler = toolHandlers.get(event.tool_name || '')
              if (handler) {
                clientToolRan = true
                handler(event, (content, name) => insertToolMessage(assistantKey, content, name))
              }
              break
            }
            case 'server_tool_start':
              handleServerToolStart(event, assistantKey)
              break
            case 'server_tool_result':
              handleServerToolResult(event)
              break
            case 'done':
              handleDoneEvent(event, assistantKey)
              break
            case 'error':
              handleErrorEvent(event, assistantKey)
              break
          }
        },
        (error) => {
          console.error('LLM error:', error)
          setIsStreaming(false)
        },
        { signal: abortControllerRef.current.signal }
      )

      // After the turn: if the assistant edited the document, validate the result
      // (e.g. compile MJML) and surface a persistent error rather than letting a
      // broken output be presented as success.
      if (
        clientToolRan &&
        validateOnComplete &&
        !abortControllerRef.current.signal.aborted
      ) {
        try {
          const result = await validateOnComplete()
          if (!result.ok) {
            appendErrorMessage(
              t`The generated email has issues that prevent it from rendering:` +
                (result.errorText ? `\n\n${result.errorText}` : '') +
                '\n\n' +
                t`Ask me to fix these issues.`
            )
          }
        } catch (validationError) {
          console.error('Validation after completion failed:', validationError)
        }
      }
    } catch (error) {
      console.error('Failed to stream:', error)
      setIsStreaming(false)
    }
  }

  const resetConversation = () => {
    setMessages([])
    setCosts({ input: 0, output: 0, total: 0 })
  }

  const bubbleItems: BubbleItem[] = messages.flatMap((m) => {
    const items: BubbleItem[] = []

    // Render accumulated reasoning as a collapsible bubble above the answer.
    if (m.thinking && m.thinking.trim()) {
      items.push({ key: `${m.key}-thinking`, role: 'thinking', content: m.thinking })
    }

    // Skip a finished assistant message that only produced reasoning (no answer text);
    // its thinking bubble above is enough and an empty answer bubble looks broken.
    if (m.role === 'assistant' && !m.content.trim() && !m.loading && m.thinking?.trim()) {
      return items
    }

    const isServerTool =
      m.toolName === SERVER_TOOLS.SCRAPE_URL || m.toolName === SERVER_TOOLS.SEARCH_WEB
    const isError = m.toolName === ERROR_TOOL_NAME

    items.push({
      key: m.key,
      role: m.role === 'user' ? 'user' : m.role === 'tool' ? 'system' : 'ai',
      content: m.content,
      loading: m.loading,
      ...(m.role === 'tool' && {
        styles: {
          content: isError
            ? { background: '#fff2f0', border: '1px solid #ffccc7', whiteSpace: 'pre-wrap' }
            : isServerTool
              ? { background: '#e6f4ff' }
              : { background: '#f6ffed', border: '1px solid #b7eb8f' }
        }
      }),
      ...(m.role === 'tool' && isServerTool && {
        avatar: {
          icon: m.toolName === 'search_web' ? <Search size={10} /> : <Globe size={10} />,
          size: 20,
          style: { background: '#1890ff', minWidth: 20, minHeight: 20 }
        }
      })
    })

    return items
  })

  return {
    open,
    setOpen,
    messages,
    inputValue,
    setInputValue,
    isStreaming,
    costs,
    inputContainerRef,
    llmIntegration,
    llmIntegrations,
    setSelectedLLMIntegrationId,
    handleCancel,
    handleSend,
    bubbleItems,
    resetConversation
  }
}
