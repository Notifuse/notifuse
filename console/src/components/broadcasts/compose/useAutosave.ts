import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  broadcastApi,
  type Broadcast,
  type BroadcastStatus,
  type CreateBroadcastRequest,
  type UpdateBroadcastRequest
} from '../../../services/api/broadcast'
import { buildBroadcastPayload } from './buildBroadcastPayload'
import type { ComposeValues } from './composeValues'
import { validateDraft, type DraftContext, type DraftIssue } from './validateDraft'

/** Time after the last valid change before a draft is saved. */
export const AUTOSAVE_DELAY_MS = 1000

export type SaveState =
  | { kind: 'idle' }
  | { kind: 'saving' }
  | { kind: 'saved'; at: number }
  | { kind: 'incomplete'; issue: DraftIssue }
  | { kind: 'error'; message: string }

export interface AutosaveOptions {
  workspaceId: string
  values: ComposeValues
  context: DraftContext
  /** The broadcast being edited; absent for a new one until its create answers. */
  broadcast?: Broadcast
  /** Saves on its own after each valid change (a draft); otherwise only on flush (a scheduled one). */
  auto: boolean
  /** The id of a broadcast just created, for the URL. */
  onCreated?: (broadcast: Broadcast) => void
  /** A save the server refused. */
  onError?: (error: unknown) => void
}

export interface Autosave {
  state: SaveState
  /** The id once the broadcast exists. */
  id?: string
  /** Changes not yet on the server. */
  dirty: boolean
  /** Sends what is pending and waits for it; ok is false when nothing could be saved. */
  flush: () => Promise<{ ok: boolean }>
  /** Stops saving: the broadcast is being sent, or has left the statuses the composer edits. */
  freeze: () => void
  /** Saves again after a send the server refused. */
  unfreeze: () => void
}

type Payload = CreateBroadcastRequest | UpdateBroadcastRequest

// The request as compared between saves: once created, what an update would send.
const keyOf = (values: ComposeValues, workspaceId: string, id: string | undefined, status: BroadcastStatus | undefined) =>
  JSON.stringify(buildBroadcastPayload(values, { workspaceId, id: id ?? 'new', status: status ?? 'draft' }))

const errorMessage = (error: unknown) => (error instanceof Error && error.message ? error.message : String(error))

/**
 * The composer's saves: broadcasts.create on the first valid state of a new broadcast, then
 * broadcasts.update, one request at a time. A change during a request is saved at its end. Each
 * answer is written to the cache of the broadcast, keeping the templates the cache had joined.
 */
export function useAutosave({ workspaceId, values, context, broadcast, auto, onCreated, onError }: AutosaveOptions): Autosave {
  const queryClient = useQueryClient()
  const valuesRef = useRef(values)
  const contextRef = useRef(context)
  const callbacks = useRef({ onCreated, onError })
  // Before any effect or event of this render reads them.
  useLayoutEffect(() => {
    valuesRef.current = values
    contextRef.current = context
    callbacks.current = { onCreated, onError }
  })

  const idRef = useRef<string | undefined>(broadcast?.id)
  const statusRef = useRef<BroadcastStatus | undefined>(broadcast?.status)
  // What the server holds, as the key of the request that would write it. The composer mounts
  // with the stored broadcast, or without one for a new broadcast.
  const [initialKey] = useState(() =>
    broadcast ? keyOf(values, workspaceId, broadcast.id, broadcast.status) : null
  )
  const savedKeyRef = useRef<string | null>(initialKey)
  // The same, for render: `dirty` compares it with the values shown.
  const [saved, setSaved] = useState<{ key: string | null; status?: BroadcastStatus }>({
    key: initialKey,
    status: broadcast?.status
  })
  const markSaved = useCallback((key: string, status: BroadcastStatus) => {
    savedKeyRef.current = key
    setSaved({ key, status })
  }, [])
  const frozenRef = useRef(false)
  const runningRef = useRef<Promise<boolean> | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const failedRef = useRef(false)

  const [state, setState] = useState<SaveState>({ kind: 'idle' })
  const [id, setId] = useState<string | undefined>(broadcast?.id)

  const writeCache = useCallback(
    (saved: Broadcast) => {
      queryClient.setQueryData<Broadcast>(['broadcast', workspaceId, saved.id], (previous) => {
        if (!previous) return saved
        // The answer carries no joined template: keep the ones the cache has for the same ids.
        const templates = new Map(previous.test_settings?.variations?.map((v) => [v.template_id, v.template]) ?? [])
        return {
          ...saved,
          test_settings: {
            ...saved.test_settings,
            variations: (saved.test_settings?.variations ?? []).map((v) => ({
              ...v,
              template: v.template ?? templates.get(v.template_id)
            }))
          }
        }
      })
    },
    [queryClient, workspaceId]
  )

  const currentKey = useCallback(
    () => keyOf(valuesRef.current, workspaceId, idRef.current, statusRef.current),
    [workspaceId]
  )

  const saveOnce = useCallback(async (): Promise<boolean> => {
    const sent = valuesRef.current
    const issue = validateDraft(sent, contextRef.current)
    if (issue) {
      setState({ kind: 'incomplete', issue })
      return false
    }
    const key = currentKey()
    if (key === savedKeyRef.current) return true
    setState({ kind: 'saving' })
    const payload: Payload = buildBroadcastPayload(sent, { workspaceId, id: idRef.current, status: statusRef.current })
    try {
      if (!idRef.current) {
        const { broadcast: created } = await broadcastApi.create(payload as CreateBroadcastRequest)
        idRef.current = created.id
        statusRef.current = created.status
        setId(created.id)
        writeCache(created)
        // create stores an empty schedule: the key says what the server holds, so a schedule
        // choice differs from it and is written by the update that follows.
        markSaved(keyOf({ ...sent, sendChoice: 'now' }, workspaceId, created.id, created.status), created.status)
        callbacks.current.onCreated?.(created)
      } else {
        const { broadcast: updated } = await broadcastApi.update(payload as UpdateBroadcastRequest)
        statusRef.current = updated.status
        writeCache(updated)
        markSaved(keyOf(sent, workspaceId, updated.id, updated.status), updated.status)
      }
      failedRef.current = false
      setState({ kind: 'saved', at: Date.now() })
      return true
    } catch (error) {
      failedRef.current = true
      setState({ kind: 'error', message: errorMessage(error) })
      callbacks.current.onError?.(error)
      return false
    }
  }, [currentKey, markSaved, workspaceId, writeCache])

  const run = useCallback((): Promise<boolean> => {
    if (runningRef.current) {
      // One request at a time: the running loop picks the latest values up at its end.
      return runningRef.current
    }
    const loop = (async () => {
      let ok = true
      // Saves again while the values moved during the request, or a create left a schedule to write.
      while (!frozenRef.current) {
        ok = await saveOnce()
        if (!ok || currentKey() === savedKeyRef.current) break
      }
      return ok
    })()
    runningRef.current = loop
    loop.finally(() => {
      runningRef.current = null
    })
    return loop
  }, [currentKey, saveOnce])

  // The context is rebuilt on each render of the composer: only its content counts.
  const contextKey = context.senderIds.join('\n')
  // A change after a refusal is a new attempt.
  const lastValues = useRef(values)
  useEffect(() => {
    if (lastValues.current !== values) {
      lastValues.current = values
      failedRef.current = false
    }
    if (frozenRef.current) return
    const issue = validateDraft(values, contextRef.current)
    if (issue) {
      if (timerRef.current) clearTimeout(timerRef.current)
      // A broadcast loaded incomplete stays quiet until it is changed: nothing is lost.
      setState((previous) =>
        previous.kind === 'idle' && savedKeyRef.current === currentKey() ? previous : { kind: 'incomplete', issue }
      )
      return
    }
    if (currentKey() === savedKeyRef.current) {
      setState((previous) => (previous.kind === 'incomplete' ? { kind: 'idle' } : previous))
      return
    }
    if (!auto) {
      setState((previous) => (previous.kind === 'incomplete' ? { kind: 'idle' } : previous))
      return
    }
    // After a refusal, the next save waits for a change or a Retry, which calls flush.
    if (failedRef.current) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => {
      timerRef.current = null
      void run()
    }, AUTOSAVE_DELAY_MS)
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [values, contextKey, auto, currentKey, run])

  const flush = useCallback(async () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    if (frozenRef.current) return { ok: true }
    failedRef.current = false
    const ok = await run()
    return { ok: ok && currentKey() === savedKeyRef.current }
  }, [currentKey, run])

  const freeze = useCallback(() => {
    frozenRef.current = true
    if (timerRef.current) clearTimeout(timerRef.current)
  }, [])

  const unfreeze = useCallback(() => {
    frozenRef.current = false
  }, [])

  const dirty = saved.key !== keyOf(values, workspaceId, id, saved.status)

  return { state, id, dirty, flush, freeze, unfreeze }
}
