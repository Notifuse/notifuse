import { useState, useRef, useEffect, type ReactNode, type CSSProperties } from 'react'
import { Search, Globe, Circle } from 'lucide-react'
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

// Icons for the tools the SERVER runs. A consumer entry for the same name wins, so a
// feature that scrapes with its own affordance can say so.
const DEFAULT_TOOL_ICONS: Record<string, ReactNode> = {
  [SERVER_TOOLS.SCRAPE_URL]: <Globe size={11} />,
  [SERVER_TOOLS.SEARCH_WEB]: <Search size={11} />
}
// A tool nobody supplied an icon for still gets a mark: an avatar-less line would sit
// flush against the panel edge while its neighbours are indented, and the column of
// steps is what makes the trace scannable.
const FALLBACK_TOOL_ICON = <Circle size={7} />

// A step is a caption, not a bubble: no fill, no border, secondary colour, small type.
// Process must recede so the answer is the only thing on the thread with weight.
const STEP_CONTENT_STYLE: CSSProperties = {
  background: 'transparent',
  border: 'none',
  padding: 0,
  fontSize: 12,
  lineHeight: 1.6,
  color: '#8c8c8c',
  whiteSpace: 'pre-wrap'
}
// A failure is the one thing on the process side that must NOT recede: it keeps the
// filled red bubble it has always had, so a step that broke cannot read as a step that
// merely finished.
const ERROR_CONTENT_STYLE: CSSProperties = {
  background: '#fff2f0',
  border: '1px solid #ffccc7',
  whiteSpace: 'pre-wrap'
}
// Sized for a step line rather than a chat bubble: it marks the line, it does not
// announce a speaker.
const STEP_AVATAR_SIZE = 16
const STEP_AVATAR_STYLE: CSSProperties = {
  background: 'transparent',
  color: '#bfbfbf',
  minWidth: STEP_AVATAR_SIZE,
  minHeight: STEP_AVATAR_SIZE
}

// Elapsed time earns its place on a step only when it explains a wait; under a second
// the number is noise repeated on every line.
const MIN_ELAPSED_SUFFIX_MS = 1000

// Hard ceiling on assistant round trips per user turn, whatever a consumer asks for.
const MAX_TOOL_ROUNDS_CEILING = 5
// Total handler executions across all rounds of one turn: a model that emits 40
// tool_use blocks must not be able to fire 40 analytics queries.
const MAX_TOOL_CALLS_PER_TURN = 12
// A handler that never settles must not pin the turn.
const TOOL_TIMEOUT_MS = 20_000

const toErrorText = (e: unknown) => (e instanceof Error ? e.message : String(e))

// A dispatched call once its handler has settled. `result` is absent for a pure
// side-effect handler; `fingerprint` for a call that was refused rather than run.
interface SettledEntry {
  id: string
  name: string
  input: Record<string, unknown>
  fingerprint?: string
  result?: ToolResult
}

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
    maxToolRounds,
    toolIcons
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

  // Synchronous twin of isStreaming, owned by handleSend and cleared the moment the
  // turn is over. State cannot guard re-entrancy: two submits dispatched in the same
  // tick - the web analytics chip auto-send effect firing beside a manual Enter - both
  // read the pre-render `isStreaming === false` and both pass, leaving turn A streaming,
  // accruing cost and running client tools with nobody watching it.
  const sendingRef = useRef(false)

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
    // The cancelled turn's own `finally` is turn-guarded and will no longer clear this,
    // so Stop has to - or the composer stays blocked for the rest of the session.
    sendingRef.current = false
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

  // Compact elapsed time for a finished step, or '' when the step was quick enough that
  // the number would tell the reader nothing. One decimal while the digit still carries
  // information, whole seconds past ten.
  const elapsedSuffix = (startedAt: number) => {
    const ms = Date.now() - startedAt
    if (ms < MIN_ELAPSED_SUFFIX_MS) return ''
    const seconds = ms < 10_000 ? (ms / 1000).toFixed(1) : String(Math.round(ms / 1000))
    return t` · ${seconds}s`
  }

  // Two-phase tool bubble: posted with a spinner the moment the model calls the tool,
  // rewritten in place when the async work finishes. The empty-assistant replacement
  // inside insertToolMessage means a text-less round hands its spinner to this bubble
  // rather than leaving a blank assistant bubble above it.
  const startToolProgress = (
    assistantKey: string,
    content: string,
    toolName: string,
    // False once the hook has stopped waiting for the call that owns this bubble, or
    // once a newer turn replaced the one it belongs to. See runToolHandler.
    writable: () => boolean = () => true
  ): ToolBubbleHandle => {
    const key = nextKey('tool')
    // Timed here rather than by the consumer: every handler that narrates itself gets
    // the duration for free, and none of them can measure the gap between the model
    // asking for the tool and the handler starting.
    const startedAt = Date.now()
    insertToolMessage(assistantKey, content, toolName, true, key)
    return {
      update: (next, opts) => {
        if (!writable()) return
        // A failed step already says what went wrong; a stopwatch appended to it reads
        // as a measurement of the failure and pushes the reason further from the eye.
        const text = opts?.failed ? next : next + elapsedSuffix(startedAt)
        setMessages((prev) =>
          prev.map((m) =>
            m.key === key ? { ...m, content: text, loading: false, failed: opts?.failed } : m
          )
        )
      }
    }
  }

  // `alive` gates the branches that APPEND rather than rewrite. Rewriting on a key was
  // self-guarding - a stopped or reset turn has no such message left, so the map was a
  // no-op - but an append would land in a conversation that has moved on.
  const handleTextEvent = (event: LLMChatEvent, assistantKey: string, alive: () => boolean) => {
    if (!event.content) return
    setMessages((prev) => {
      if (prev.some((m) => m.key === assistantKey)) {
        return prev.map((m) =>
          m.key === assistantKey ? { ...m, content: m.content + event.content, loading: false } : m
        )
      }
      if (!alive()) return prev
      // The round's assistant bubble is SPLICED OUT by insertToolMessage when the model
      // called a tool before writing any prose. Text streaming after that tool_use would
      // otherwise match nothing and vanish from the thread - while still being accrued
      // into the round's `text` and pushed onto the wire transcript, so the model would
      // be told it said something the user never saw. Re-open the bubble below the tool
      // result instead; the key is reused so the rest of the round accumulates on it.
      return [
        ...prev,
        { key: assistantKey, role: 'assistant', content: event.content ?? '', loading: false }
      ]
    })
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

  // Write a failure into the round's assistant bubble - or, when that bubble is gone,
  // append a persistent error one instead.
  //
  // The bubble really does disappear on the ordinary path: a round whose model output
  // was a tool call before any prose has its empty assistant message SPLICED OUT by
  // insertToolMessage, so a `map` on assistantKey matches nothing and the failure is
  // written nowhere. onError then declines to write a second time (sawErrorEvent), and
  // the operator watches the progress bubble stop with no error and no answer.
  const writeErrorForRound = (assistantKey: string, content: string, alive: () => boolean) => {
    setMessages((prev) => {
      if (prev.some((m) => m.key === assistantKey)) {
        return prev.map((m) => (m.key === assistantKey ? { ...m, content, loading: false } : m))
      }
      // Nothing to rewrite AND the turn is over: a stopped or reset turn must not push
      // its failure into the conversation that replaced it.
      if (!alive()) return prev
      return [...prev, { key: nextKey('error'), role: 'tool', toolName: ERROR_TOOL_NAME, content }]
    })
  }

  const handleErrorEvent = (event: LLMChatEvent, assistantKey: string, alive: () => boolean) => {
    writeErrorForRound(assistantKey, t`Error: ${event.error}`, alive)
    // isStreaming is cleared by handleSend when the round loop exits (see handleDoneEvent).
  }

  // Append a persistent error bubble (distinct from the transient antd toast) so a
  // failure stays visible in the conversation rather than vanishing.
  const appendErrorMessage = (content: string) => {
    setMessages((prev) => [
      ...prev,
      { key: nextKey('error'), role: 'tool', toolName: ERROR_TOOL_NAME, content }
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
    const turn = turnIdRef.current
    // Per-call abort, chained to the turn's. The turn signal cancels every handler of
    // the round at once, which is not what a timeout means: only the call that overran
    // must be stopped. And it has to be actually stopped - resolving the wrapper alone
    // left the handler querying, and repainting, long after the model was handed the
    // timeout and apologised for it.
    const call = new AbortController()
    const onTurnAbort = () => call.abort()
    if (signal.aborted) call.abort()
    else signal.addEventListener('abort', onTurnAbort, { once: true })
    const releaseTurnListener = () => signal.removeEventListener('abort', onTurnAbort)

    // Every write a handler makes goes through this. It is false once the hook has
    // stopped waiting for the call (timed out, cancelled) and false once a newer turn -
    // or resetConversation - has replaced this one, so a late handler can neither
    // repaint its bubble as a success beside the model's timeout apology, nor drop a
    // stale line into a conversation that has just been cleared.
    const writable = () => turnIdRef.current === turn && !call.signal.aborted

    let returned: void | ToolResult | Promise<ToolResult | void>
    try {
      returned = handler(
        event,
        (content, toolName) => {
          if (writable()) insertToolMessage(assistantKey, content, toolName)
        },
        {
          signal: call.signal,
          round,
          progress: (content, toolName = name) =>
            startToolProgress(assistantKey, content, toolName, writable)
        }
      )
    } catch (err) {
      // A synchronous throw would otherwise be swallowed by streamChat's parse
      // try/catch, leaving the model waiting for a result that never comes.
      releaseTurnListener()
      return Promise.resolve({ content: toErrorText(err), isError: true })
    }

    // Synchronous handler (every Blog and Email handler): resolve immediately, no
    // timer, no listener, no behaviour change.
    if (!returned || typeof (returned as Promise<unknown>).then !== 'function') {
      releaseTurnListener()
      return Promise.resolve(returned as ToolResult | void)
    }

    // addEventListener('abort', …) never fires on a signal that is ALREADY aborted, so
    // without this line a tool_use arriving after Stop parks the turn for the full
    // TOOL_TIMEOUT_MS with nothing left to cancel it.
    if (call.signal.aborted) {
      releaseTurnListener()
      return Promise.resolve({ content: `tool "${name}" cancelled`, isError: true })
    }

    return new Promise((resolve) => {
      let settled = false
      const finish = (r: ToolResult | void) => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        call.signal.removeEventListener('abort', onAbort)
        releaseTurnListener()
        resolve(r)
      }
      const onAbort = () => finish({ content: `tool "${name}" cancelled`, isError: true })
      const timer = setTimeout(() => {
        finish({ content: `tool "${name}" timed out after ${TOOL_TIMEOUT_MS}ms`, isError: true })
        // Aborted AFTER the wrapper settled, so the outcome stays "timed out" rather
        // than being rewritten as "cancelled" by the listener above: settling first
        // detaches it. What the abort still does is stop the abandoned handler's work
        // and close its writes.
        call.abort()
      }, TOOL_TIMEOUT_MS)
      call.signal.addEventListener('abort', onAbort, { once: true })
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
    alive: () => boolean // false once this turn has been stopped, reset or superseded
  }): Promise<RoundOutcome> => {
    const { transcript, assistantKey, controller, round, looping, alreadyRun, budget, alive } = args
    let text = ''
    let terminated = false
    let ranHandler = false
    // llmApi.streamChat reports an SSE `error` FRAME twice: onEvent first, then onError
    // for the same frame (services/api/llm.ts:115-118). Today that is harmless because
    // onError only console.errors and clears the flag; under the new onError body,
    // which writes a visible bubble, it would produce two bubbles for one failure.
    let sawErrorEvent = false
    const pending: Array<{
      id: string
      name: string
      input: Record<string, unknown>
      // Set only for calls a handler actually ran; refusals carry none, and never did.
      fingerprint?: string
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
            handleTextEvent(event, assistantKey, alive)
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
            ranHandler = true
            pending.push({
              id,
              name,
              input,
              fingerprint,
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
            handleErrorEvent(event, assistantKey, alive)
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
        if (!alive()) return
        if (text.trim()) {
          // A round that already streamed prose keeps it; the failure is appended.
          appendErrorMessage(t`Error: ${toErrorText(error)}`)
        } else {
          // Not a plain map on the key: the round's assistant bubble is gone whenever a
          // tool call preceded the failure, and the error would be written nowhere.
          writeErrorForRound(assistantKey, t`Error: ${toErrorText(error)}`, alive)
        }
      },
      { signal: controller.signal }
    )

    // Anthropic and OpenAI emit tool_use only after the stream completes, and `done` is
    // emitted last in all three providers, so this settles almost immediately unless a
    // handler is genuinely async.
    const settled: SettledEntry[] = await Promise.all(
      pending.map(
        (p): Promise<SettledEntry> =>
          p.promise.then(
            (r) => ({
              id: p.id,
              name: p.name,
              input: p.input,
              fingerprint: p.fingerprint,
              result: r ?? undefined
            }),
            (err: unknown) => ({
              id: p.id,
              name: p.name,
              input: p.input,
              fingerprint: p.fingerprint,
              result: { content: toErrorText(err), isError: true }
            })
          )
      )
    )

    return {
      text,
      settled: settled.filter(
        (c): c is SettledToolCall => !!c.result && typeof c.result.content === 'string'
      ),
      // Recorded from the OUTCOME, not from the dispatch. A call that timed out or
      // failed handed the model nothing, so the retry it makes next round is a first
      // attempt at getting the data - refusing it as a cross-round duplicate would
      // leave the turn with no way to recover from one slow query.
      fingerprints: settled
        .filter((c) => c.fingerprint && !c.result?.isError)
        .map((c) => c.fingerprint as string),
      terminated,
      ranHandler
    }
  }

  const handleSend = async () => {
    if (!inputValue.trim() || !llmIntegration || isStreaming || sendingRef.current) return
    sendingRef.current = true

    const integration = llmIntegration
    const maxRounds = Math.min(Math.max(1, maxToolRounds ?? 1), MAX_TOOL_ROUNDS_CEILING)
    const looping = maxRounds > 1
    const question = inputValue

    const myTurn = ++turnIdRef.current
    // Bumping the turn id orphans the previous turn's writes; only the abort actually
    // ENDS it. handleCancel, resetConversation and the unmount effect all pair the two,
    // and so must this: without it a turn superseded here keeps streaming, keeps
    // accruing cost, and keeps running client tools against the page.
    abortControllerRef.current?.abort()
    const controller = new AbortController()
    abortControllerRef.current = controller
    const alive = () => turnIdRef.current === myTurn && !controller.signal.aborted

    const userKey = nextKey('user')
    setMessages((prev) => [...prev, { key: userKey, role: 'user', content: question }])
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

    // Drop the spinners and guarantee the turn left SOMETHING on screen.
    //
    // bubbleItems is right to skip a finished assistant message with no answer text: on
    // a continuation round the tool bubble IS the output, and an empty bubble beside it
    // looks broken. The gap is a whole turn that ends that way - a provider returning an
    // empty completion, or a handler that narrates nothing - where every bubble the turn
    // produced is skipped and the operator is left looking at their own question with no
    // reply and no error. Idempotent: the `finally` path runs it again and finds the
    // placeholder already standing.
    const endTurn = () => {
      setMessages((prev) => {
        const cleared = prev.map((m) => (m.loading ? { ...m, loading: false } : m))
        const userIndex = cleared.findIndex((m) => m.key === userKey)
        if (userIndex === -1) return cleared
        const answered = cleared
          .slice(userIndex + 1)
          .some((m) => m.content.trim() || m.thinking?.trim())
        if (answered) return cleared
        return [
          ...cleared,
          {
            key: nextKey('assistant'),
            role: 'assistant',
            content: t`No answer was returned. Please try again.`
          }
        ]
      })
    }

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
          integrationId: integration.id,
          alive
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
          // Reaching the last round is not the same as failing to answer: a model that
          // wrote its conclusion AND asked for one more query in the same round has
          // already answered, and "I never reached an answer" printed in red under a
          // complete answer reads as a bug. Warn only when the round produced no prose.
          hitRoundCap = !outcome.text.trim()
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
        sendingRef.current = false
        setIsStreaming(false)
        endTurn()
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
          // Re-checked AFTER the await, not only before it: templatesApi.compile has no
          // timeout and no AbortSignal, so a validation that resolves late would
          // otherwise drop its error into whatever turn has since replaced this one.
          if (!result.ok && alive()) {
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
        sendingRef.current = false
        setIsStreaming(false)
        endTurn()
      }
    }
  }

  const resetConversation = () => {
    // The button is disabled while streaming, so this is belt-and-braces for a
    // programmatic caller: orphan and abort the in-flight turn so it cannot append
    // anything to the list we just cleared.
    turnIdRef.current += 1
    abortControllerRef.current?.abort()
    // As in handleCancel: the orphaned turn will no longer clear this for us.
    sendingRef.current = false
    setMessages([])
    setCosts({ input: 0, output: 0, total: 0 })
    setIsStreaming(false)
  }

  // Consumer first, then the two server tools, then a neutral mark. `??` rather than
  // `||` so a consumer can deliberately pass an icon React treats as falsy.
  const resolveToolIcon = (toolName?: string): ReactNode => {
    if (!toolName) return FALLBACK_TOOL_ICON
    return toolIcons?.[toolName] ?? DEFAULT_TOOL_ICONS[toolName] ?? FALLBACK_TOOL_ICON
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

    const isError = m.toolName === ERROR_TOOL_NAME || m.failed === true

    items.push({
      key: m.key,
      role: m.role === 'user' ? 'user' : m.role === 'tool' ? 'system' : 'ai',
      content: m.content,
      loading: m.loading,
      ...(m.role === 'tool' && {
        // A running or finished step is a quiet line; only a failure keeps the weight
        // of a bubble.
        ...(isError ? {} : { variant: 'borderless' as const }),
        styles: { content: isError ? ERROR_CONTENT_STYLE : STEP_CONTENT_STYLE },
        // An error carries no avatar: it is not part of the step column, and a mark
        // beside it would only compete with the red.
        ...(isError
          ? {}
          : {
              avatar: {
                icon: resolveToolIcon(m.toolName),
                size: STEP_AVATAR_SIZE,
                style: STEP_AVATAR_STYLE
              }
            })
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
