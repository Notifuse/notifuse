import { describe, it, expect, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

vi.mock('../../../services/api/web_analytics', () => ({ webAnalyticsService: { contactVisits: vi.fn() } }))
vi.mock('../../../services/api/messages_history', () => ({ listMessages: vi.fn() }))
vi.mock('../../../services/api/analytics', () => ({
  analyticsService: {
    query: vi.fn(async (query: { measures: string[] }) => ({
      data: [
        query.measures.length === 1
          ? { count_sent_emails: '3' }
          : {
              count_sent_emails: '10',
              count_opened: 4,
              count_opens_tracked: 8,
              count_clicked: '1',
              count_clicks_tracked: 8,
              count: 12
            }
      ],
      meta: { total: 1, query: '', params: [] }
    }))
  }
}))
vi.mock('../../../services/api/contact_timeline', () => ({
  contactTimelineApi: { list: vi.fn(async () => ({ timeline: [], next_cursor: undefined })) }
}))

import { analyticsService } from '../../../services/api/analytics'
import { contactTimelineApi } from '../../../services/api/contact_timeline'
import {
  ACTIVITY_ENTITY_TYPES,
  last30DaysRange,
  useContactMessageStats,
  useContactTimeline
} from './useContactSheetQueries'

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
)

describe('useContactSheetQueries', () => {
  it('maps each activity filter to its entity types', () => {
    expect(ACTIVITY_ENTITY_TYPES).toEqual({
      all: undefined,
      emails: ['message_history'],
      lists: ['contact_list', 'contact_segment'],
      profile: ['contact'],
      events: ['custom_event', 'inbound_webhook_event', 'webhook_event', 'automation'],
      web: ['web_session', 'web_page']
    })
  })

  it('takes the last 30 days as ISO instants', () => {
    expect(last30DaysRange(new Date('2026-11-12T15:00:00Z'))).toEqual([
      '2026-10-13T15:00:00.000Z',
      '2026-11-12T15:00:00.000Z'
    ])
  })

  it('reads the contact message counts since always and over 30 days, filtered on the email', async () => {
    const { result } = renderHook(() => useContactMessageStats('ws', 'ann@x.com', true), { wrapper })
    await waitFor(() => expect(result.current.data).toBeDefined())
    expect(result.current.data).toEqual({
      sent: 10,
      sentLast30Days: 3,
      opened: 4,
      opensTracked: 8,
      clicked: 1,
      clicksTracked: 8,
      messages: 12
    })
    const [all, recent] = vi.mocked(analyticsService.query).mock.calls
    expect(all[0]).toMatchObject({
      schema: 'message_history',
      dimensions: [],
      filters: [{ member: 'contact_email', operator: 'equals', values: ['ann@x.com'] }]
    })
    expect(all[0].timeDimensions).toBeUndefined()
    expect(recent[0].filters?.[1]).toMatchObject({ member: 'sent_at', operator: 'inDateRange' })
    expect(recent[0].filters?.[1].values[0]).toMatch(/^\d{4}-\d{2}-\d{2}T.*Z$/)
  })

  it('sends the filter types to timeline.list, and no type for All', async () => {
    const { result } = renderHook(() => useContactTimeline('ws', 'ann@x.com', 'lists'), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(contactTimelineApi.list).toHaveBeenCalledWith(
      expect.objectContaining({
        email: 'ann@x.com',
        entity_types: ['contact_list', 'contact_segment'],
        cursor: undefined
      })
    )
    renderHook(() => useContactTimeline('ws', 'ann@x.com', 'all'), { wrapper })
    await waitFor(() => expect(contactTimelineApi.list).toHaveBeenCalledTimes(2))
    expect(vi.mocked(contactTimelineApi.list).mock.calls[1][0].entity_types).toBeUndefined()
  })
})
