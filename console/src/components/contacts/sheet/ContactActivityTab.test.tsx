import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { Workspace } from '../../../services/api/types'

const state = vi.hoisted(() => ({ entries: [] as { id: string; email: string }[] }))

vi.mock('../../../services/api/contact_timeline', () => ({
  contactTimelineApi: {
    list: vi.fn(async ({ entity_types }: { entity_types?: string[] }) => ({
      timeline: entity_types ? [] : state.entries,
      next_cursor: undefined
    }))
  }
}))
// The sheet's other calls: their real modules pull the API client, hence the router.
vi.mock('../../../services/api/analytics', () => ({ analyticsService: { query: vi.fn() } }))
vi.mock('../../../services/api/web_analytics', () => ({ webAnalyticsService: { contactVisits: vi.fn() } }))
vi.mock('../../../services/api/messages_history', () => ({ listMessages: vi.fn() }))
vi.mock('../../timeline', () => ({
  ContactTimeline: ({ entries }: { entries: { id: string }[] }) => <div>{entries.length} entries</div>
}))

import { contactTimelineApi } from '../../../services/api/contact_timeline'
import { ContactActivityTab } from './ContactActivityTab'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace

function renderTab(rights: { messages?: boolean; visits?: boolean } = {}) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <App>
        <ContactActivityTab
          workspace={workspace}
          email="ann@x.com"
          timezone="Europe/Paris"
          segments={[]}
          canReadMessages={rights.messages ?? true}
          canReadVisits={rights.visits ?? true}
        />
      </App>
    </QueryClientProvider>
  )
}

describe('ContactActivityTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.entries = [{ id: '1', email: 'ann@x.com' }]
  })

  it('reads all the activity, then each filter from its first page with its types', async () => {
    renderTab()
    expect(await screen.findByText('1 entries')).toBeInTheDocument()
    expect(screen.getByText('Times in Europe/Paris')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Lists & segments'))
    await waitFor(() =>
      expect(contactTimelineApi.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ cursor: undefined, entity_types: ['contact_list', 'contact_segment'] })
      )
    )
    expect(await screen.findByText('No activity of this kind')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Events'))
    await waitFor(() =>
      expect(contactTimelineApi.list).toHaveBeenLastCalledWith(
        expect.objectContaining({
          entity_types: ['custom_event', 'inbound_webhook_event', 'webhook_event', 'automation']
        })
      )
    )
  })

  it('hides Emails without message history read and Web without web analytics', () => {
    renderTab({ messages: false, visits: false })
    expect(screen.queryByText('Emails')).toBeNull()
    expect(screen.queryByText('Web')).toBeNull()
    expect(screen.getByText('Profile')).toBeInTheDocument()
  })
})
