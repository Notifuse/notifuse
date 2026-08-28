import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { webhookSubscriptionApi } from './webhook_subscription'

// The module imports the api client, which pulls in the router.
vi.mock('./client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

beforeEach(() => {
  vi.mocked(api.post).mockReset()
  vi.mocked(api.post).mockResolvedValue({ subscription: {} })
})

/** The body the client hands to `fetch`: `api.post` JSON-encodes whatever it is given,
 *  and JSON.stringify drops an undefined value along with its key. */
const sentBody = (): Record<string, unknown> =>
  JSON.parse(JSON.stringify(vi.mocked(api.post).mock.calls[0][1]))

const baseUpdate = {
  workspace_id: 'ws1',
  id: 'wh1',
  name: 'Renamed',
  url: 'https://example.com/hook',
  event_types: ['list.subscribed']
}

describe('webhookSubscriptionApi.update', () => {
  it('sends nothing the caller did not name', async () => {
    await webhookSubscriptionApi.update(baseUpdate)

    const body = sentBody()
    expect(vi.mocked(api.post).mock.calls[0][0]).toBe('/api/webhookSubscriptions.update')
    // The endpoint reads an absent filter as "leave the stored one alone", which is what
    // lets a caller rename a Zapier subscription without widening it to every list in the
    // workspace. A default supplied here would remove that option from every caller at
    // once, silently: only whoever holds the form knows which filters a save means to
    // clear, so this helper states none of them.
    expect(Object.keys(body).sort()).toEqual(Object.keys(baseUpdate).sort())
    // Named individually as well, because a key set comparison reads as an accident and
    // these three are each a live hazard: two widen the subscription, and an echoed
    // enabled re-enables one somebody disabled while the drawer sat open.
    expect('list_ids' in body).toBe(false)
    expect('segment_ids' in body).toBe(false)
    expect('custom_event_filters' in body).toBe(false)
    expect('enabled' in body).toBe(false)
  })

  it('passes the filters it was given through unchanged', async () => {
    await webhookSubscriptionApi.update({
      ...baseUpdate,
      custom_event_filters: { goal_types: ['purchase'] },
      list_ids: ['list-a'],
      segment_ids: ['seg-a']
    })

    const body = sentBody()
    expect(body.custom_event_filters).toEqual({ goal_types: ['purchase'] })
    expect(body.list_ids).toEqual(['list-a'])
    expect(body.segment_ids).toEqual(['seg-a'])
  })

  it('carries an explicitly empty filter through as a removal', async () => {
    await webhookSubscriptionApi.update({ ...baseUpdate, custom_event_filters: {} })

    // The empty object has to survive the trip: it is the only way a caller can say "stop
    // filtering custom events". The drawer sends it whenever its filter fields come back
    // empty, and dropping it here would keep the filter the user just cleared, with no
    // control anywhere in the console able to remove it afterwards.
    expect(sentBody().custom_event_filters).toEqual({})
  })
})
