import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useAutosave, AUTOSAVE_DELAY_MS, type AutosaveOptions } from './useAutosave'
import { broadcastApi, type Broadcast } from '../../../services/api/broadcast'
import { fromBroadcast, newDraft, type ComposeValues } from './composeValues'
import type { Workspace } from '../../../services/api/types'

vi.mock('../../../services/api/broadcast', () => ({ broadcastApi: { create: vi.fn(), update: vi.fn() } }))

const create = broadcastApi.create as Mock
const update = broadcastApi.update as Mock
const workspace = { id: 'ws1', settings: { timezone: 'UTC', website_url: 'https://acme.test' } } as unknown as Workspace
const context = { senderIds: ['s1'] }

const stored = (overrides: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'bc1',
    workspace_id: 'ws1',
    name: 'News',
    status: 'draft',
    audience: { list: 'l1', segments: [], exclude_unsubscribed: true },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: {
      enabled: false,
      sample_percentage: 50,
      auto_send_winner: false,
      variations: [{ variation_name: 'A', template_id: 'tpl-a', template: { id: 'tpl-a', name: 'Alpha' } }]
    },
    utm_parameters: { source: 'acme.test', medium: 'email', campaign: 'news' },
    tracking_mode: 'inherit',
    created_at: '',
    updated_at: '',
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    ...overrides
  }) as Broadcast

const answer = (b: Broadcast) => {
  // The server answers without the joined templates.
  const { test_settings } = b
  return {
    broadcast: {
      ...b,
      test_settings: { ...test_settings, variations: test_settings.variations.map(({ template: _t, ...v }) => v) }
    }
  }
}

function setup(initial: ComposeValues, options: Partial<AutosaveOptions> = {}) {
  const client = new QueryClient()
  if (options.broadcast) client.setQueryData(['broadcast', 'ws1', options.broadcast.id], options.broadcast)
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const view = renderHook(
    (props: { values: ComposeValues }) =>
      useAutosave({ workspaceId: 'ws1', values: props.values, context, auto: true, ...options }),
    { wrapper, initialProps: { values: initial } }
  )
  return { ...view, client }
}

const tick = async (ms = AUTOSAVE_DELAY_MS) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

describe('useAutosave', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })
  afterEach(() => vi.useRealTimers())

  const fresh = () => newDraft('News', workspace)
  const complete = (): ComposeValues => ({ ...fresh(), list: 'l1', variants: [{ variation_name: 'A', template_id: 'tpl-a' }] })

  it('sends nothing while the draft is incomplete, and says why', async () => {
    const { result } = setup(fresh())
    await tick(5000)
    expect(create).not.toHaveBeenCalled()
    expect(result.current.state).toEqual({ kind: 'incomplete', issue: { code: 'list_missing' } })
  })

  it('creates the broadcast on its first valid state, a second after the change', async () => {
    create.mockResolvedValue(answer(stored()))
    const onCreated = vi.fn()
    const { result, rerender, client } = setup(fresh(), { onCreated })
    rerender({ values: complete() })
    await tick(AUTOSAVE_DELAY_MS - 1)
    expect(create).not.toHaveBeenCalled()
    await tick(1)
    expect(create).toHaveBeenCalledTimes(1)
    expect(create.mock.calls[0][0]).not.toHaveProperty('id')
    expect(onCreated).toHaveBeenCalledWith(expect.objectContaining({ id: 'bc1' }))
    expect(result.current.id).toBe('bc1')
    expect(result.current.state.kind).toBe('saved')
    expect(client.getQueryData(['broadcast', 'ws1', 'bc1'])).toMatchObject({ id: 'bc1' })
    // The create stored everything the values say: no update follows.
    expect(update).not.toHaveBeenCalled()
  })

  it('writes the schedule choice of a new broadcast with an update after its create', async () => {
    create.mockResolvedValue(answer(stored()))
    update.mockResolvedValue(answer(stored()))
    const { rerender } = setup(fresh())
    rerender({ values: { ...complete(), sendChoice: 'schedule', scheduledDate: '2030-01-02', scheduledTime: '09:00' } })
    await tick()
    expect(create).toHaveBeenCalledTimes(1)
    expect(update).toHaveBeenCalledTimes(1)
    expect(update.mock.calls[0][0]).toMatchObject({
      id: 'bc1',
      schedule: { is_scheduled: false, scheduled_date: '2030-01-02', scheduled_time: '09:00', timezone: 'UTC' }
    })
  })

  it('debounces a draft: several changes within a second make one update with the last values', async () => {
    const b = stored()
    update.mockResolvedValue(answer(b))
    const initial = fromBroadcast(b, workspace)
    const { rerender } = setup(initial, { broadcast: b })
    await tick(5000)
    expect(update).not.toHaveBeenCalled()
    rerender({ values: { ...initial, name: 'News 1' } })
    await tick(500)
    rerender({ values: { ...initial, name: 'News 12' } })
    await tick()
    expect(update).toHaveBeenCalledTimes(1)
    expect(update.mock.calls[0][0]).toMatchObject({ id: 'bc1', name: 'News 12' })
  })

  it('sends one request at a time and saves the change made during it at its end', async () => {
    const b = stored()
    let release: (value: unknown) => void = () => {}
    update.mockImplementationOnce(() => new Promise((resolve) => (release = resolve))).mockResolvedValue(answer(b))
    const initial = fromBroadcast(b, workspace)
    const { rerender, result } = setup(initial, { broadcast: b })
    rerender({ values: { ...initial, name: 'First' } })
    await tick()
    expect(update).toHaveBeenCalledTimes(1)
    rerender({ values: { ...initial, name: 'Second' } })
    await tick()
    expect(update).toHaveBeenCalledTimes(1)
    await act(async () => {
      release(answer(b))
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(update).toHaveBeenCalledTimes(2)
    expect(update.mock.calls[1][0]).toMatchObject({ name: 'Second' })
    expect(result.current.dirty).toBe(false)
  })

  it('flush sends what is pending at once and resolves once saved', async () => {
    const b = stored()
    update.mockResolvedValue(answer(b))
    const initial = fromBroadcast(b, workspace)
    const { rerender, result } = setup(initial, { broadcast: b })
    rerender({ values: { ...initial, name: 'Now' } })
    let outcome: { ok: boolean } | undefined
    await act(async () => {
      outcome = await result.current.flush()
    })
    expect(update).toHaveBeenCalledTimes(1)
    expect(outcome).toEqual({ ok: true })
    await tick()
    expect(update).toHaveBeenCalledTimes(1)
  })

  it('flush of an incomplete draft says it could not save', async () => {
    const { result } = setup(fresh())
    let outcome: { ok: boolean } | undefined
    await act(async () => {
      outcome = await result.current.flush()
    })
    expect(outcome).toEqual({ ok: false })
    expect(create).not.toHaveBeenCalled()
  })

  it('keeps the values after a refusal, says Not saved, and waits for a change or a retry', async () => {
    const b = stored()
    update.mockRejectedValueOnce(new Error('sender gone')).mockResolvedValue(answer(b))
    const onError = vi.fn()
    const initial = fromBroadcast(b, workspace)
    const { rerender, result } = setup(initial, { broadcast: b, onError })
    const changed = { ...initial, name: 'Changed' }
    rerender({ values: changed })
    await tick()
    expect(result.current.state).toEqual({ kind: 'error', message: 'sender gone' })
    expect(onError).toHaveBeenCalled()
    await tick(5000)
    expect(update).toHaveBeenCalledTimes(1)
    await act(async () => {
      await result.current.flush()
    })
    expect(update).toHaveBeenCalledTimes(2)
    expect(result.current.state.kind).toBe('saved')
  })

  it('writes each answer to the cache, keeping the joined templates', async () => {
    const b = stored()
    update.mockResolvedValue(answer({ ...b, name: 'Renamed' }))
    const initial = fromBroadcast(b, workspace)
    const { rerender, client } = setup(initial, { broadcast: b })
    rerender({ values: { ...initial, name: 'Renamed' } })
    await tick()
    const cached = client.getQueryData<Broadcast>(['broadcast', 'ws1', 'bc1'])
    expect(cached?.name).toBe('Renamed')
    expect(cached?.test_settings.variations[0].template).toEqual({ id: 'tpl-a', name: 'Alpha' })
  })

  it('stops saving once frozen', async () => {
    const b = stored()
    const initial = fromBroadcast(b, workspace)
    const { rerender, result } = setup(initial, { broadcast: b })
    act(() => result.current.freeze())
    rerender({ values: { ...initial, name: 'Ignored' } })
    await tick(5000)
    expect(update).not.toHaveBeenCalled()
  })

  it('saves a scheduled broadcast only on flush, without its schedule', async () => {
    const b = stored({
      status: 'scheduled',
      schedule: { is_scheduled: true, scheduled_date: '2030-01-02', scheduled_time: '09:00', timezone: 'UTC', use_recipient_timezone: false }
    })
    update.mockResolvedValue(answer(b))
    const initial = fromBroadcast(b, workspace)
    const { rerender, result } = setup(initial, { broadcast: b, auto: false })
    expect(result.current.dirty).toBe(false)
    rerender({ values: { ...initial, name: 'Fixed typo' } })
    await tick(5000)
    expect(update).not.toHaveBeenCalled()
    expect(result.current.dirty).toBe(true)
    await act(async () => {
      await result.current.flush()
    })
    expect(update).toHaveBeenCalledTimes(1)
    expect(update.mock.calls[0][0]).not.toHaveProperty('schedule')
    expect(result.current.dirty).toBe(false)
  })
})
