import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('./client', () => ({
  api: { post: vi.fn(), get: vi.fn(async () => ({})) }
}))

import { api } from './client'
import { contactsApi } from './contacts'
import { listsApi } from './list'
import { contactTimelineApi } from './contact_timeline'
import { webAnalyticsService } from './web_analytics'

const lastUrl = () => {
  const calls = vi.mocked(api.get).mock.calls
  return new URL(String(calls[calls.length - 1][0]), 'https://x.test')
}

describe('contacts redesign API calls', () => {
  beforeEach(() => vi.mocked(api.get).mockClear())

  it('sends the unified search of contacts.list', async () => {
    await contactsApi.list({ workspace_id: 'ws', search: 'ann doe', limit: 50 })
    expect(lastUrl().pathname).toBe('/api/contacts.list')
    expect(lastUrl().searchParams.get('search')).toBe('ann doe')
  })

  it('omits an empty search', async () => {
    await contactsApi.list({ workspace_id: 'ws', search: '' })
    expect(lastUrl().searchParams.has('search')).toBe(false)
  })

  it('reads one contact by its exact email', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({ contact: { email: 'ann@x.com' } })
    const response = await contactsApi.getByEmail({ workspace_id: 'ws', email: 'ann+1@x.com' })
    expect(lastUrl().pathname).toBe('/api/contacts.getByEmail')
    expect(lastUrl().searchParams.get('email')).toBe('ann+1@x.com')
    expect(lastUrl().searchParams.get('workspace_id')).toBe('ws')
    expect(response.contact.email).toBe('ann@x.com')
  })

  it('asks lists.list for the active counts only when told', async () => {
    await listsApi.list({ workspace_id: 'ws', with_counts: true })
    expect(lastUrl().searchParams.get('with_counts')).toBe('true')
    await listsApi.list({ workspace_id: 'ws' })
    expect(lastUrl().searchParams.has('with_counts')).toBe(false)
  })

  it('sends timeline entity types comma separated', async () => {
    await contactTimelineApi.list({
      workspace_id: 'ws',
      email: 'ann@x.com',
      entity_types: ['contact_list', 'contact_segment']
    })
    expect(lastUrl().searchParams.get('entity_types')).toBe('contact_list,contact_segment')
    await contactTimelineApi.list({ workspace_id: 'ws', email: 'ann@x.com', entity_types: [] })
    expect(lastUrl().searchParams.has('entity_types')).toBe(false)
  })

  it('reads a contact web visits page', async () => {
    await webAnalyticsService.contactVisits({
      workspace_id: 'ws',
      email: 'ann@x.com',
      filter: 'goal',
      limit: 20,
      cursor: 'abc'
    })
    const url = lastUrl()
    expect(url.pathname).toBe('/api/webAnalytics.contactVisits')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      workspace_id: 'ws',
      email: 'ann@x.com',
      filter: 'goal',
      limit: '20',
      cursor: 'abc'
    })
  })

  it('reads the pages of one session', async () => {
    await webAnalyticsService.contactSessionPages({
      workspace_id: 'ws',
      email: 'ann@x.com',
      session_id: '0b8f0a52-6a3a-4e0b-9a3c-2b1f8b0e6f11',
      session_date: '2026-11-08'
    })
    const url = lastUrl()
    expect(url.pathname).toBe('/api/webAnalytics.contactSessionPages')
    expect(url.searchParams.get('session_id')).toBe('0b8f0a52-6a3a-4e0b-9a3c-2b1f8b0e6f11')
    expect(url.searchParams.get('session_date')).toBe('2026-11-08')
  })
})
