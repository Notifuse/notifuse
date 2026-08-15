import { useState, useRef, useEffect } from 'react'
import { Search, Globe } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { llmApi, LLMChatEvent, LLMMessage } from '../../services/api/llm'
import type {
  ChatMessage,
  UseAIAssistantOptions,
  UseAIAssistantReturn,
  BubbleItem,
  ToolHandler,
  ToolResult,
  ToolBubbleHandle
} from './types'
import {
  describeCalls,
  encodeToolResults,
  normalizeWire,
  stableStringify,
  toWireMessages,
  type SettledToolCall
} from './wire'

// Server-side tool names (for styling)
const SERVER_TOOLS = {
  SCRAPE_URL: 'scrape_url',
  SEARCH_WEB: 'search_web'
} as const

// Marker toolName for persistent error bubbles (styled distinctly).
const ERROR_TOOL_NAME = '__error__'

// Hard ceiling on assistant round trips per user turn, whatever a consumer asks for.
const MAX_TOOL_ROUNDS_CEILING = 5
// Total handler executions across all rounds of one turn: a model that emits 40
// tool_use blocks must not be able to fire 40 analytics queries.
const MAX_TOOL_CALLS_PER_TURN = 12
// A handler that never settles must not pin the turn.
const TOOL_TIMEOUT_MS = 20_000

const toErrorText = (e: unknown) => (e instanceof Error ? e.message : String(e))

interface RoundOutcome {
  text: string
  settled: SettledToolCall[]
  fingerprints: string[]
  terminated: boolean
  ranHandler: boolean
}

export function useAIAssistant(options: UseAIAssistantOptions): UseAIAssistantReturn {
  const {
    workspace,
    config,
    tools,
    toolHandlers,
    buildSystemPrompt,
    validateOnComplete,
    maxToolRounds
  } = options
  const { t } = useLingui()

  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [costs, setCosts] = useState({ input: 0, output: 0, total: 0 })
  const abortControllerRef = useRef<AbortController | null>(null)
  const inputContainerRef = useRef<HTMLDivElement | null>(null)

  // A turn now spans several seconds and several round trips, so the closure captured
  // at click time goes stale: round 2 must rebuild the system prompt rather than resend
  // round 1's. Refs are refreshed on every render, so this makes the prompt as fresh as
  // the last render - it cannot make the render happen. The consumer side of that
  // bargain is that UI handlers await their navigation (applyUiState in
  // WebAnalyticsAIAssistant.tsx), so a round cannot settle before the state it changed
  // has committed. There is no provider prompt cache to invalidate: no cache_control is
  // set anywhere in internal/service/llm_service*.go.
  const buildSystemPromptRef = useRef(buildSystemPrompt)
  buildSystemPromptRef.current = buildSystemPrompt
  const toolHandlersRef = useRef(toolHandlers)
  toolHandlersRef.current = toolHandlers
  const toolsRef = useRef(tools)
  toolsRef.current = tools

  // Monotonic turn identity. Bumped by handleSend, handleCancel and resetConversation,
  // so a superseded turn unwinding late cannot mutate the state of the turn that
  // replaced it. Also gives unique keys for messages created within the same ms.
  const turnIdRef = useRef(0)
  const seqRef = useRef(0)
  const nextKey = (prefix: string) => `${prefix}-${Date.now()}-${++seqRef.current}`

  // Unmount ends the turn, exactly as Stop does.
  //
  // Before the loop, a turn was one fetch and unmounting merely wasted its response.
  // Now runToolHandler arms a TOOL_TIMEOUT_MS timer and runRound awaits Promise.all
  // over the pending handlers, so an unmount mid-turn would otherwise leave a live
  // timer, a chain of setMessages calls into a dead component, and - worst - further
  // continuation POSTs for a panel nobody is looking at.
  //
  // This is the ordinary case, not an exotic one: WebAnalyticsAIAssistant mounts inside
  // WebAnalyticsSection, the route body, so leaving Web Analytics for any other page
  // unmounts it. The `hidden` prop only covers moving BETWEEN the section's own tabs.
  //
  // Bumping the turn id is what makes it airtight: every post-await write in handleSend
  // is already guarded by `turnIdRef.current === myTurn`, so one increment invalidates
  // them all, and the abort settles the pending handlers through their abort listener
  // rather than at the timeout. It is the hook's SECOND useEffect - the focus effect
  // below has no cleanup and is untouched.
  useEffect(() => {
    return () => {
      turnIdRef.current += 1
      abortControllerRef.current?.abort()
    }
  }, [])

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
    // Orphan the in-flight turn even if its fetch has already resolved.
    turnIdRef.current += 1
    abortControllerRef.current?.abort()
    setIsStreaming(false)
    setMessages((prev) =>
      prev
        .map((m) => {
          if (!m.loading) return m
          // A tool bubble mid-query says what happened instead of freezing on
          // "Querying sessions by day" with no spinner and no outcome.
          if (m.role === 'tool')
            return { ...m, loading: false, content: `${m.content} ${t`- cancelled`}` }
          return { ...m, loading: false, content: m.content || t`(Cancelled)` }
        })
        .filter((m) => m.content.trim())
    )
  }

  const insertToolMessage = (
    assistantKey: string,
    content: string,
    toolName: string,
    loading = false,
    // Caller-supplied key, so a progress bubble can be rewritten in place later.
    key?: string
  ) => {
    setMessages((prev) => {
      const assistantIndex = prev.findIndex((m) => m.key === assistantKey)
      const newToolMessage: ChatMessage = {
        // Unique even when several tool calls resolve within the same millisecond.
        key: key ?? `tool-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
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

  // Two-phase tool bubble: posted with a spinner the moment the model calls the tool,
  // rewritten in place when the async work finishes. The empty-assistant replacement
  // inside insertToolMessage means a text-less round hands its spinner to this bubble
  // rather than leaving a blank assistant bubble above it.
  const startToolProgress = (
    assistantKey: string,
    content: string,
    toolName: string
  ): ToolBubbleHandle => {
    const key = nextKey('tool')
    insertToolMessage(assistantKey, content, toolName, true, key)
    return {
      update: (next, opts) =>
        setMessages((prev) =>
          prev.map((m) =>
            m.key === key ? { ...m, content: next, loading: false, failed: opts?.failed } : m
          )
        )
    }
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
    // isStreaming is owned by handleSend, which clears it the moment the round loop
    // exits: a turn can span several rounds, and this event fires once per round. It
    // also used to be the ONLY place the flag was cleared on success, so a stream that
    // resolved without a terminal event (dropped connection, or a split SSE frame
    // swallowed by the JSON.parse in llm.ts) left isStreaming true forever and
    // handleSend early-returned for the rest of the session.
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
    // isStreaming is cleared by handleSend when the round loop exits (see handleDoneEvent).
  }

  // Append a persistent error bubble (distinct from the transient antd toast) so a
  // failure stays visible in the conversation rather than vanishing.
  const appendErrorMessage = (content: string) => {
    setMessages((prev) => [
      ...prev,
      { key: `error-${Date.now()}`, role: 'tool', toolName: ERROR_TOOL_NAME, content }
    ])
  }

  // One tool execution that cannot throw, cannot hang, and cannot outlive the turn.
  const runToolHandler = (
    handler: ToolHandler,
    event: LLMChatEvent,
    assistantKey: string,
    signal: AbortSignal,
    round: number,
    name: string
  ): Promise<ToolResult | void> => {
    let returned: void | ToolResult | Promise<ToolResult | void>
    try {
      returned = handler(
        event,
        (content, toolName) => insertToolMessage(assistantKey, content, toolName),
        {
          signal,
          round,
          progress: (content, toolName = name) => startToolProgress(assistantKey, content, toolName)
        }
      )
    } catch (err) {
      // A synchronous throw would otherwise be swallowed by streamChat's parse
      // try/catch, leaving the model waiting for a result that never comes.
      return Promise.resolve({ content: toErrorText(err), isError: true })
    }

    // Synchronous handler (every Blog and Email handler): resolve immediately, no
    // timer, no listener, no behaviour change.
    if (!returned || typeof (returned as Promise<unknown>).then !== 'function') {
      return Promise.resolve(returned as ToolResult | void)
    }

    // addEventListener('abort', …) never fires on a signal that is ALREADY aborted, so
    // without this line a tool_use arriving after Stop parks the turn for the full
    // TOOL_TIMEOUT_MS with nothing left to cancel it.
    if (signal.aborted) {
      return Promise.resolve({ content: `tool "${name}" cancelled`, isError: true })
    }

    return new Promise((resolve) => {
      let settled = false
      const finish = (r: ToolResult | void) => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        signal.removeEventListener('abort', onAbort)
        resolve(r)
      }
      const onAbort = () => finish({ content: `tool "${name}" cancelled`, isError: true })
      const timer = setTimeout(
        () =>
          finish({ content: `tool "${name}" timed out after ${TOOL_TIMEOUT_MS}ms`, isError: true }),
        TOOL_TIMEOUT_MS
      )
      signal.addEventListener('abort', onAbort, { once: true })
      ;(returned as Promise<ToolResult | void>).then(finish, (err) =>
        finish({ content: toErrorText(err), isError: true })
      )
    })
  }

  const runRound = async (args: {
    transcript: LLMMessage[]
    assistantKey: string
    controller: AbortController
    round: number
    looping: boolean // maxRounds > 1: results can actually reach the model
    alreadyRun: Set<string> // fingerprints executed in EARLIER rounds of this turn
    budget: { left: number }
    integrationId: string
  }): Promise<RoundOutcome> => {
    const { transcript, assistantKey, controller, round, looping, alreadyRun, budget } = args
    let text = ''
    let terminated = false
    let ranHandler = false
    // llmApi.streamChat reports an SSE `error` FRAME twice: onEvent first, then onError
    // for the same frame (services/api/llm.ts:115-118). Today that is harmless because
    // onError only console.errors and clears the flag; under the new onError body,
    // which writes a visible bubble, it would produce two bubbles for one failure.
    let sawErrorEvent = false
    const fingerprints: string[] = []
    const pending: Array<{
      id: string
      name: string
      input: Record<string, unknown>
      promise: Promise<ToolResult | void>
    }> = []

    await llmApi.streamChat(
      {
        workspace_id: workspace.id,
        integration_id: args.integrationId,
        // normalizeWire is applied at the exact point the invariant matters.
        messages: normalizeWire(transcript),
        system_prompt: buildSystemPromptRef.current(),
        max_tokens: config.maxTokens,
        // Re-sent verbatim every round: the model must retain the ability to call again.
        tools: toolsRef.current
      },
      (event: LLMChatEvent) => {
        switch (event.type) {
          case 'text':
            text += event.content || ''
            handleTextEvent(event, assistantKey)
            break
          case 'thinking':
            handleThinkingEvent(event, assistantKey)
            break
          case 'tool_use': {
            const name = event.tool_name || ''
            const input = event.tool_input || {}
            const id = `c${pending.length + 1}`
            const handler = toolHandlersRef.current.get(name)

            if (!handler) {
              // Pre-existing behaviour when the loop is off: a no-op. When the loop is
              // on, tell the model so it can self-correct instead of waiting forever.
              //
              // silent: true on ALL THREE refusal branches below, and this is the
              // property that makes the budgets bound cost instead of inflating it. A
              // refusal costs nothing to produce, so a round in which every call was
              // refused must not buy another round: without `silent`, a model that
              // keeps calling a tool it has already exhausted the budget for would be
              // answered with a fresh POST carrying nothing but refusals, up to the
              // round cap - more requests in exactly the runaway case the budget
              // exists to bound. Refusals still ride along in the payload whenever a
              // real result buys the round, which is where the model can act on them.
              if (looping) {
                pending.push({
                  id,
                  name,
                  input,
                  promise: Promise.resolve({
                    content: `unknown tool "${name}"`,
                    isError: true,
                    silent: true
                  })
                })
              }
              break
            }

            const fingerprint = `${name}:${stableStringify(input)}`
            // Dedupe ACROSS rounds only. Within a round, an identical repeat is a
            // legitimate instruction ("add two identical buttons"); across rounds it is
            // the model re-asking for data it has already been given.
            if (looping && alreadyRun.has(fingerprint)) {
              pending.push({
                id,
                name,
                input,
                promise: Promise.resolve({
                  content: 'duplicate of a call already made in this turn; the earlier result stands',
                  isError: true,
                  silent: true
                })
              })
              break
            }
            if (looping && budget.left <= 0) {
              pending.push({
                id,
                name,
                input,
                promise: Promise.resolve({
                  content: 'tool budget for this turn is exhausted; this call was not executed',
                  isError: true,
                  silent: true
                })
              })
              break
            }

            budget.left -= 1
            fingerprints.push(fingerprint)
            ranHandler = true
            pending.push({
              id,
              name,
              input,
              promise: runToolHandler(handler, event, assistantKey, controller.signal, round, name)
            })
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
            // A truncated round can carry a half-parsed tool input and will very likely
            // truncate again; stop rather than burn the round budget. The existing
            // truncation warning has already been appended by handleDoneEvent.
            if (event.truncated) terminated = true
            break
          case 'error':
            handleErrorEvent(event, assistantKey)
            terminated = true
            sawErrorEvent = true
            break
        }
      },
      (error) => {
        console.error('LLM error:', error)
        terminated = true
        // An SSE `error` frame reaches BOTH callbacks: streamChat calls onEvent and
        // then, for that frame only, onError with the same message
        // (services/api/llm.ts:115-118). handleErrorEvent has already rewritten the
        // assistant bubble; appending here too would show the same failure twice. The
        // transport-level failures this callback exists for - a non-2xx response, a
        // dropped socket - set no such flag and still write.
        if (sawErrorEvent) return
        // MUST write something visible. llmApi.streamChat does NOT reject on a non-2xx:
        // it throws internally (services/api/llm.ts:88-92), catches at :121-131, calls
        // this callback and returns normally. A request rejected by req.Validate() is a
        // plain HTTP 400 JSON body, not an SSE `error` event
        // (internal/http/llm_handler.go:57-60), so this is the ordinary failure path.
        // Without this write the round's empty assistant bubble is cleared of `loading`
        // and then skipped by the bubbleItems predicate, and the user watches the
        // spinner stop with no message at all.
        if (text.trim()) {
          // A round that already streamed prose keeps it; the failure is appended.
          appendErrorMessage(t`Error: ${toErrorText(error)}`)
        } else {
          setMessages((prev) =>
            prev.map((m) =>
              m.key === assistantKey
                ? { ...m, content: t`Error: ${toErrorText(error)}`, loading: false }
                : m
            )
          )
        }
      },
      { signal: controller.signal }
    )

    // Anthropic and OpenAI emit tool_use only after the stream completes, and `done` is
    // emitted last in all three providers, so this settles almost immediately unless a
    // handler is genuinely async.
    const settled = await Promise.all(
      pending.map((p) =>
        p.promise.then(
          (r) => ({ id: p.id, name: p.name, input: p.input, result: r ?? undefined }),
          (err: unknown) => ({
            id: p.id,
            name: p.name,
            input: p.input,
            result: { content: toErrorText(err), isError: true } as ToolResult
          })
        )
      )
    )

    return {
      text,
      settled: settled.filter(
        (c): c is SettledToolCall => !!c.result && typeof c.result.content === 'string'
      ),
      fingerprints,
      terminated,
      ranHandler
    }
  }

  const handleSend = async () => {
    if (!inputValue.trim() || !llmIntegration || isStreaming) return

    const integration = llmIntegration
    const maxRounds = Math.min(Math.max(1, maxToolRounds ?? 1), MAX_TOOL_ROUNDS_CEILING)
    const looping = maxRounds > 1
    const question = inputValue

    const myTurn = ++turnIdRef.current
    const controller = new AbortController()
    abortControllerRef.current = controller
    const alive = () => turnIdRef.current === myTurn && !controller.signal.aborted

    setMessages((prev) => [...prev, { key: nextKey('user'), role: 'user', content: question }])
    setInputValue('')
    setIsStreaming(true)

    // Owned by this turn: seeded once from the rendered history, then extended locally
    // per round. `messages` is the render-time snapshot, so it does not contain the
    // question just appended - which is why it is pushed explicitly, exactly as before.
    const transcript: LLMMessage[] = normalizeWire([
      ...toWireMessages(messages),
      { role: 'user', content: question }
    ])

    const alreadyRun = new Set<string>()
    const budget = { left: MAX_TOOL_CALLS_PER_TURN }
    // Track whether the assistant actually edited anything this turn; validation
    // only matters when a client-side tool ran.
    let clientToolRan = false
    let hitRoundCap = false

    try {
      for (let round = 1; round <= maxRounds; round++) {
        const assistantKey = nextKey('assistant')
        setMessages((prev) => [
          ...prev,
          { key: assistantKey, role: 'assistant', content: '', loading: true }
        ])

        const outcome = await runRound({
          transcript,
          assistantKey,
          controller,
          round,
          looping,
          alreadyRun,
          budget,
          integrationId: integration.id
        })
        clientToolRan = clientToolRan || outcome.ranHandler
        outcome.fingerprints.forEach((f) => alreadyRun.add(f))

        // FENCE: a stop, a reset, or a newer turn during the stream.
        if (!alive()) return
        if (outcome.terminated) break

        if (!looping) {
          if (import.meta.env.DEV && outcome.settled.some((c) => !c.result.isError)) {
            console.warn(
              '[useAIAssistant] a tool returned a ToolResult but maxToolRounds is 1; the result was discarded'
            )
          }
          break
        }

        const returning = outcome.settled
        if (returning.length === 0) break
        // A round in which no handler actually executed produced no new information,
        // whatever it returned: everything in `returning` is then a refusal (unknown
        // tool, cross-round duplicate, budget exhausted). Belt and braces with the
        // `silent` flags those refusals carry - either one alone closes the hole, and
        // this one closes it even if a future refusal branch forgets the flag.
        if (!outcome.ranHandler) break
        // Acknowledgements alone (UI mutations) do not justify a billed round trip.
        if (!returning.some((c) => !c.result.silent)) break

        // One alternating pair per round. The assistant turn is non-empty by
        // construction, so no two user turns can ever end up adjacent.
        transcript.push({
          role: 'assistant',
          content: outcome.text.trim() || describeCalls(returning)
        })
        transcript.push({ role: 'user', content: encodeToolResults(returning) })

        if (round === maxRounds) {
          hitRoundCap = true
          break
        }
      }

      // THE TURN IS OVER HERE, and the streaming state ends with it - before, never
      // after, validateOnComplete. That order is what ships today (handleDoneEvent
      // cleared the flag, validation ran afterwards). The real validateOnComplete awaits
      // templatesApi.compile(...) with no timeout and no AbortSignal
      // (CreateTemplateDrawer.tsx): parked behind it in a `finally`, a validation that
      // never settles would leave isStreaming true forever and handleSend would
      // early-return for the rest of the session.
      if (turnIdRef.current === myTurn) {
        setIsStreaming(false)
        setMessages((prev) => prev.map((m) => (m.loading ? { ...m, loading: false } : m)))
      }

      if (hitRoundCap && alive()) {
        appendErrorMessage(
          t`I ran ${maxRounds} rounds of tool calls without reaching an answer and stopped there. Ask a narrower question and I will try again.`
        )
      }

      // After the turn: if the assistant edited the document, validate the result
      // (e.g. compile MJML) and surface a persistent error rather than letting a
      // broken output be presented as success.
      if (clientToolRan && validateOnComplete && alive()) {
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
    } finally {
      // Message cleanup only - never isStreaming, which was already cleared above.
      // Both writes are guarded by turn identity so a stale turn unwinding late cannot
      // touch the state of the turn that replaced it. This path matters when the round
      // loop threw: the flag was then never cleared above, so clear it here too.
      if (turnIdRef.current === myTurn) {
        setIsStreaming(false)
        setMessages((prev) => prev.map((m) => (m.loading ? { ...m, loading: false } : m)))
      }
    }
  }

  const resetConversation = () => {
    // The button is disabled while streaming, so this is belt-and-braces for a
    // programmatic caller: orphan and abort the in-flight turn so it cannot append
    // anything to the list we just cleared.
    turnIdRef.current += 1
    abortControllerRef.current?.abort()
    setMessages([])
    setCosts({ input: 0, output: 0, total: 0 })
    setIsStreaming(false)
  }

  const bubbleItems: BubbleItem[] = messages.flatMap((m) => {
    const items: BubbleItem[] = []

    // Render accumulated reasoning as a collapsible bubble above the answer.
    if (m.thinking && m.thinking.trim()) {
      items.push({ key: `${m.key}-thinking`, role: 'thinking', content: m.thinking })
    }

    // Skip a finished assistant message with no answer text: it either produced only
    // reasoning (the thinking bubble above is enough) or only tool calls (a continuation
    // round). An empty answer bubble looks broken in both cases.
    if (m.role === 'assistant' && !m.content.trim() && !m.loading) {
      return items
    }

    const isServerTool =
      m.toolName === SERVER_TOOLS.SCRAPE_URL || m.toolName === SERVER_TOOLS.SEARCH_WEB
    const isError = m.toolName === ERROR_TOOL_NAME || m.failed === true

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
      ...(m.role === 'tool' &&
        isServerTool && {
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
