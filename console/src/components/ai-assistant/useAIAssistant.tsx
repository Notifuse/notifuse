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

export function useAIAssistant(options: UseAIAssistantOptions): UseAIAssistantReturn {
  const { workspace, config, tools, toolHandlers, buildSystemPrompt } = options
  const { t } = useLingui()

  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [costs, setCosts] = useState({ input: 0, output: 0, total: 0 })
  const abortControllerRef = useRef<AbortController | null>(null)
  const inputContainerRef = useRef<HTMLDivElement | null>(null)

  const llmIntegration = workspace.integrations?.find((i) => i.type === 'llm')

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
        key: `tool-${Date.now()}`,
        role: 'tool',
        content,
        toolName,
        loading
      }

      if (assistantIndex === -1) {
        return [...prev, newToolMessage]
      }

      const assistant = prev[assistantIndex]
      if (!assistant.content.trim()) {
        return [...prev.slice(0, assistantIndex), newToolMessage, ...prev.slice(assistantIndex + 1)]
      }

      return [
        ...prev.slice(0, assistantIndex),
        newToolMessage,
        { ...assistant, loading: false },
        ...prev.slice(assistantIndex + 1)
      ]
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
  }

  const handleErrorEvent = (event: LLMChatEvent, assistantKey: string) => {
    setMessages((prev) =>
      prev.map((m) =>
        m.key === assistantKey ? { ...m, content: t`Error: ${event.error}`, loading: false } : m
      )
    )
    setIsStreaming(false)
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
            case 'tool_use': {
              const handler = toolHandlers.get(event.tool_name || '')
              if (handler) {
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
    } catch (error) {
      console.error('Failed to stream:', error)
      setIsStreaming(false)
    }
  }

  const resetConversation = () => {
    setMessages([])
    setCosts({ input: 0, output: 0, total: 0 })
  }

  const bubbleItems: BubbleItem[] = messages.map((m) => {
    const isServerTool =
      m.toolName === SERVER_TOOLS.SCRAPE_URL || m.toolName === SERVER_TOOLS.SEARCH_WEB

    return {
      key: m.key,
      role: m.role === 'user' ? 'user' : m.role === 'tool' ? 'system' : 'ai',
      content: m.content,
      loading: m.loading,
      ...(m.role === 'tool' && {
        styles: {
          content: isServerTool
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
    }
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
    handleCancel,
    handleSend,
    bubbleItems,
    resetConversation
  }
}
