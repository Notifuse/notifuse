import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, onTestFinished } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { UserPermissions, Workspace } from '../../../services/api/types'

const state = vi.hoisted(() => ({
  pages: [] as {
    messages: unknown[]
    next_cursor?: string
    has_more: boolean
  }[]
}))

// The sheet's other calls: their real modules pull the API client, hence the router.
vi.mock('../../../services/api/analytics', () => ({
  analyticsService: { query: vi.fn() },
  AnalyticsService: vi.fn()
}))
vi.mock('../../../services/api/web_analytics', () => ({
  webAnalyticsService: {}
}))
vi.mock('../../../services/api/contact_timeline', () => ({
  contactTimelineApi: { list: vi.fn() }
}))
vi.mock('../../../services/api/messages_history', () => ({
  listMessages: vi.fn(async (_ws: string, { cursor }: { cursor?: string }) => state.pages[cursor ? 1 : 0])
}))
vi.mock('../../../services/api/broadcast', () => ({
  broadcastApi: {
    list: vi.fn(async () => ({
      broadcasts: [
        {
          id: 'b1',
          name: 'Black Friday',
          test_settings: {
            enabled: true,
            variations: [
              { variation_name: 'A', template_id: 't1' },
              { variation_name: 'B', template_id: 't2' }
            ]
          }
        }
      ]
    }))
  }
}))
vi.mock('../../../services/api/automation', () => ({
  automationApi: {
    list: vi.fn(async () => ({
      automations: [{ id: 'a1', name: 'Welcome series' }]
    }))
  }
}))
vi.mock('../../../services/api/transactional_notifications', () => ({
  transactionalNotificationsApi: {
    list: vi.fn(async () => ({ notifications: [] }))
  }
}))
vi.mock('../../../services/api/template', () => ({
  templatesApi: {
    get: vi.fn(async () => ({
      template: {
        id: 't2',
        name: 'BF template',
        email: { subject: 'Hi {{ contact.first_name }}' },
        translations: { fr: { email: { subject: 'Bonjour {{ contact.first_name }}' } } }
      }
    }))
  }
}))
vi.mock('../../templates/TemplatePreviewDrawer', () => ({
  default: ({ record }: { record: { email?: { subject?: string } } }) => (
    <div>preview drawer: {record.email?.subject}</div>
  )
}))
vi.mock('./MessageJourneySteps', () => ({
  MessageJourneySteps: () => <span>journey</span>
}))

import { listMessages } from '../../../services/api/messages_history'
import { broadcastApi } from '../../../services/api/broadcast'
import { templatesApi } from '../../../services/api/template'
import { ContactMessagesTab } from './ContactMessagesTab'

const workspace = {
  id: 'ws',
  integrations: [],
  settings: { timezone: 'UTC', default_language: 'en' }
} as unknown as Workspace
// The message data a send stores: the contact, with the language it was sent in.
const sentTo = (language: string) => ({ data: { contact: { first_name: 'Ann', language } } })
const read = { read: true, write: false }
const allRights = {
  broadcasts: read,
  automations: read,
  transactional: read,
  templates: read
} as unknown as UserPermissions

const message = (id: string, extra: Record<string, unknown>) => ({
  id,
  contact_email: 'ann@x.com',
  template_id: 't2',
  template_version: 1,
  channel: 'email',
  sent_at: '2026-11-10T09:00:00Z',
  created_at: '2026-11-10T09:00:00Z',
  ...extra
})

function renderTab(permissions: UserPermissions | null = allRights, total?: number) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ContactMessagesTab
        workspace={workspace}
        email="ann@x.com"
        permissions={permissions}
        timezone="UTC"
        total={total}
      />
    </QueryClientProvider>
  )
}

describe('ContactMessagesTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.pages = [
      {
        messages: [message('m1', { broadcast_id: 'b1' }), message('m2', { automation_id: 'a1' })],
        next_cursor: 'next',
        has_more: true
      },
      { messages: [message('m3', { automation_id: 'a1' })], has_more: false }
    ]
  })

  it('names each message by what sent it, with its A/B variant', async () => {
    renderTab()
    expect(await screen.findByText('Black Friday')).toBeInTheDocument()
    expect(screen.getByText('Variant B')).toBeInTheDocument()
    expect(screen.getByText('Welcome series')).toBeInTheDocument()
    expect(listMessages).toHaveBeenCalledWith('ws', {
      contact_email: 'ann@x.com',
      limit: 10,
      cursor: undefined
    })
  })

  it('counts the shown messages against the total and loads the next page', async () => {
    renderTab(allRights, 48)
    expect(await screen.findByText('2 of 48 messages')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await screen.findByText('3 of 48 messages')).toBeInTheDocument()
    expect(listMessages).toHaveBeenLastCalledWith('ws', {
      contact_email: 'ann@x.com',
      limit: 10,
      cursor: 'next'
    })
    expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull()
  })

  it('reads no names nor templates without the rights, and offers no preview', async () => {
    renderTab({} as UserPermissions)
    expect(await screen.findByText('2 messages')).toBeInTheDocument()
    expect(broadcastApi.list).not.toHaveBeenCalled()
    expect(templatesApi.get).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: 'View the sent email' })).toBeNull()
  })

  it('opens the sent email with its template', async () => {
    state.pages = [{ messages: [message('m1', { broadcast_id: 'b1', message_data: sentTo('fr') })], has_more: false }]
    renderTab()
    const [first] = await screen.findAllByRole('button', {
      name: 'View the sent email'
    })
    fireEvent.click(first)
    // The email in the language it was sent in.
    expect(await screen.findByText('preview drawer: Bonjour {{ contact.first_name }}')).toBeInTheDocument()
    await waitFor(() =>
      expect(templatesApi.get).toHaveBeenCalledWith({
        workspace_id: 'ws',
        id: 't2',
        version: 1
      })
    )
  })

  it('opens the sent email from anywhere on its row, not only from the icon', async () => {
    state.pages = [{ messages: [message('m1', { broadcast_id: 'b1', message_data: sentTo('fr') })], has_more: false }]
    renderTab()
    fireEvent.click(await screen.findByText('Bonjour Ann'))
    expect(await screen.findByText('preview drawer: Bonjour {{ contact.first_name }}')).toBeInTheDocument()
  })

  it('shows the subject sent: in the contact language, rendered with the message data', async () => {
    state.pages = [
      {
        messages: [
          message('m1', { broadcast_id: 'b1', message_data: sentTo('fr') }),
          message('m2', { automation_id: 'a1', message_data: sentTo('en') }),
          // Forced at send time, rendered with the message data as the server did before sending.
          message('m3', {
            automation_id: 'a1',
            channel_options: { subject: 'Your order {{ order.id }} has shipped' },
            message_data: { data: { order: { id: 1042 } } }
          })
        ],
        has_more: false
      }
    ]
    renderTab()
    expect(await screen.findByText('Bonjour Ann')).toBeInTheDocument()
    expect(await screen.findByText('Hi Ann')).toBeInTheDocument()
    expect(await screen.findByText('Your order 1042 has shipped')).toBeInTheDocument()
  })

  it('says so when the email sent cannot be loaded', async () => {
    const loads = vi.mocked(templatesApi.get).getMockImplementation()
    vi.mocked(templatesApi.get).mockRejectedValue(new Error('gone'))
    onTestFinished(() => {
      vi.mocked(templatesApi.get).mockImplementation(loads!)
    })
    state.pages = [{ messages: [message('m1', { broadcast_id: 'b1' })], has_more: false }]
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <ContactMessagesTab workspace={workspace} email="ann@x.com" permissions={allRights} timezone="UTC" />
        </App>
      </QueryClientProvider>
    )
    fireEvent.click(await screen.findByRole('button', { name: 'View the sent email' }))
    // After the query's own retry.
    expect(await screen.findByText('Could not load the email sent', {}, { timeout: 3000 })).toBeInTheDocument()
    expect(screen.queryByText(/preview drawer/)).toBeNull()
  })

  it('dates a message by its day and time, or by its day and year for an earlier year', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2027-01-01T10:00:00Z'))
    onTestFinished(() => {
      vi.useRealTimers()
    })
    state.pages = [
      {
        messages: [
          message('m1', { broadcast_id: 'b1', sent_at: '2026-03-01T09:00:00Z' }),
          // Yesterday names no year: the time stays.
          message('m2', { automation_id: 'a1', sent_at: '2026-12-31T20:00:00Z' }),
          message('m3', { automation_id: 'a1', sent_at: '2027-01-01T08:00:00Z' })
        ],
        has_more: false
      }
    ]
    renderTab()
    const earlier = await screen.findByText(/2026/)
    expect(earlier.textContent).not.toMatch(/9:00/)
    expect(screen.getByText('Yesterday, 8:00 PM')).toBeInTheDocument()
    expect(screen.getByText('Today, 8:00 AM')).toBeInTheDocument()
  })

  it('counts a single message in the singular', async () => {
    state.pages = [{ messages: [message('m1', { broadcast_id: 'b1' })], has_more: false }]
    renderTab(allRights, 1)
    expect(await screen.findByText('1 of 1 message')).toBeInTheDocument()
  })
})
