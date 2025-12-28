// LLM Chat API with SSE Streaming

export interface LLMMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface LLMTool {
  name: string
  description: string
  input_schema: object
}

export interface LLMChatRequest {
  workspace_id: string
  integration_id: string
  messages: LLMMessage[]
  max_tokens?: number
  system_prompt?: string
  tools?: LLMTool[]
}

export interface LLMChatEvent {
  type: 'text' | 'tool_use' | 'server_tool_start' | 'server_tool_result' | 'done' | 'error'
  content?: string
  error?: string
  tool_name?: string
  tool_input?: Record<string, unknown>
  input_tokens?: number
  output_tokens?: number
  input_cost?: number   // USD
  output_cost?: number  // USD
  total_cost?: number   // USD
  model?: string
}

export interface StreamChatOptions {
  signal?: AbortSignal
}

export const llmApi = {
  /**
   * Stream chat completion from the LLM API
   * Uses Server-Sent Events (SSE) for real-time streaming
   *
   * @param params - The chat request parameters
   * @param onEvent - Callback for each streamed event
   * @param onError - Optional error callback
   * @param options - Optional options including AbortSignal for cancellation
   *
   * @example
   * const controller = new AbortController()
   * llmApi.streamChat(
   *   { workspace_id: 'ws1', integration_id: 'int1', messages: [{role: 'user', content: 'Hello'}] },
   *   (event) => {
   *     if (event.type === 'text') console.log(event.content)
   *   },
   *   (error) => console.error(error),
   *   { signal: controller.signal }
   * )
   * // To cancel: controller.abort()
   */
  streamChat: async (
    params: LLMChatRequest,
    onEvent: (event: LLMChatEvent) => void,
    onError?: (error: Error) => void,
    options?: StreamChatOptions
  ): Promise<void> => {
    const authToken = localStorage.getItem('auth_token')

    let defaultOrigin = window.location.origin
    if (defaultOrigin.includes('notifusedev.com')) {
      defaultOrigin = 'https://localapi.notifuse.com:4000'
    }
    const apiEndpoint = window.API_ENDPOINT?.trim() || defaultOrigin

    try {
      const response = await fetch(`${apiEndpoint}/api/llm.chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(authToken ? { Authorization: `Bearer ${authToken}` } : {})
        },
        body: JSON.stringify(params),
        signal: options?.signal
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => null)
        throw new Error(errorData?.error || `HTTP error: ${response.status}`)
      }

      if (!response.body) {
        throw new Error('No response body')
      }

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              try {
                const event: LLMChatEvent = JSON.parse(line.slice(6))
                onEvent(event)

                if (event.type === 'error' && onError) {
                  onError(new Error(event.error || 'Unknown error'))
                }
              } catch {
                // Ignore JSON parse errors for incomplete data
              }
            }
          }
        }
      } finally {
        reader.releaseLock()
      }
    } catch (error) {
      // Don't report AbortError as an error - it's expected when cancelling
      if (error instanceof Error && error.name === 'AbortError') {
        return
      }
      if (onError) {
        onError(error instanceof Error ? error : new Error(String(error)))
      }
    }
  }
}
