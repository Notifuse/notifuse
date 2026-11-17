import { describe, expect, it, vi, beforeEach } from 'vitest'
import { queryPlan } from '../../__tests__/queryPlan'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { Broadcast } from '../../services/api/broadcast'

const { listBroadcasts, listLists, navigate } = vi.hoisted(() => ({
  listBroadcasts: vi.fn(),
  listLists: vi.fn(),
  navigate: vi.fn()
}))
vi.mock('../../services/api/broadcast', () => ({
  broadcastApi: { list: listBroadcasts }
}))
vi.mock('../../services/api/list', () => ({ listsApi: { list: listLists } }))
vi.mock('./dashboardClient', () => ({
  dashboardAnalytics: { query: vi.fn() },
  DASHBOARD_REFRESH_MS: 60_000
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  Link: ({
    search,
    params,
    children
  }: {
    search?: Record<string, string>
    params?: Record<string, string>
    children: ReactNode
  }) => (
    <a
      href={`${params?.section ? `/settings/${params.section}` : '/broadcasts'}${
        search ? `?${new URLSearchParams(search).toString()}` : ''
      }`}
    >
      {children}
    </a>
  )
}))

import { UpcomingSends } from './UpcomingSends'

const broadcast = (id: string, date: string, extra: Partial<Broadcast> = {}): Broadcast =>
  ({
    id,
    name: `Broadcast ${id}`,
    status: 'scheduled',
    audience: { list: 'l1' },
    schedule: {
      is_scheduled: true,
      scheduled_date: date,
      scheduled_time: '08:30',
      timezone: 'Europe/Paris'
    },
    test_settings: { enabled: false },
    ...extra
  }) as unknown as Broadcast

const renderCard = (
  props: { canReadLists?: boolean; canCreate?: boolean; marketingReady?: boolean; canConfigure?: boolean } = {},
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
) =>
  render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <UpcomingSends
          workspaceId="ws1"
          timezone="Europe/Paris"
          canReadLists={props.canReadLists ?? true}
          canCreate={props.canCreate ?? true}
          marketingReady={props.marketingReady}
          canConfigure={props.canConfigure}
        />
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => {
  listBroadcasts.mockReset()
  listLists.mockReset()
  navigate.mockReset()
  listLists.mockResolvedValue({ lists: [{ id: 'l1', name: 'Newsletter' }] })
})

describe('UpcomingSends', () => {
  it('reads the scheduled broadcasts each minute, and the lists once there is a row', async () => {
    listBroadcasts.mockResolvedValue({ broadcasts: [{ id: 'b1', name: 'Launch', status: 'scheduled' }], total_count: 1 })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderCard({}, client)
    await waitFor(() => expect(listLists).toHaveBeenCalled())
    expect(queryPlan(client)).toEqual([
      { key: ['dashboard', 'upcoming', 'ws1'], refetchInterval: 60_000 },
      { key: ['lists', 'ws1'], refetchInterval: undefined }
    ])
  })

  it('lists scheduled broadcasts soonest first, with their list and A/B test', async () => {
    listBroadcasts.mockResolvedValue({
      broadcasts: [
        broadcast('late', '2026-12-01'),
        broadcast('soon', '2026-11-27', {
          test_settings: { enabled: true }
        } as Partial<Broadcast>),
        broadcast('undated', '')
      ],
      total_count: 3
    })
    renderCard()

    expect(await screen.findByText('Broadcast soon')).toBeInTheDocument()
    expect(listBroadcasts).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      statuses: ['scheduled'],
      limit: 100
    })
    const names = screen.getAllByText(/^Broadcast /).map((node) => node.textContent)
    expect(names).toEqual(['Broadcast soon', 'Broadcast late'])
    expect(await screen.findByText('Newsletter · A/B')).toBeInTheDocument()
    expect(screen.getByText('Nov 27, 08:30')).toBeInTheDocument()
    // The undated broadcast is counted in total_count but has nothing to show.
    expect(screen.queryByText(/more$/)).not.toBeInTheDocument()
  })

  it('names the zone of a schedule in another timezone, and recipient-time sends', async () => {
    listBroadcasts.mockResolvedValue({
      broadcasts: [
        broadcast('ny', '2026-11-27', {
          schedule: {
            is_scheduled: true,
            scheduled_date: '2026-11-27',
            scheduled_time: '08:30',
            timezone: 'America/New_York'
          }
        } as Partial<Broadcast>),
        broadcast('local', '2026-11-28', {
          schedule: {
            is_scheduled: true,
            scheduled_date: '2026-11-28',
            scheduled_time: '09:00',
            use_recipient_timezone: true
          }
        } as Partial<Broadcast>)
      ],
      total_count: 2
    })
    renderCard()

    expect(await screen.findByText('Nov 27, 08:30 EST')).toBeInTheDocument()
    expect(screen.getByText('Nov 28, 09:00 local time')).toBeInTheDocument()
  })

  it('says « 1 list » without the right to read lists', async () => {
    listBroadcasts.mockResolvedValue({
      broadcasts: [broadcast('soon', '2026-11-27')],
      total_count: 1
    })
    renderCard({ canReadLists: false })

    expect(await screen.findByText('1 list')).toBeInTheDocument()
    expect(listLists).not.toHaveBeenCalled()
  })

  it('counts the scheduled broadcasts past the first five', async () => {
    listBroadcasts.mockResolvedValue({
      broadcasts: ['01', '02', '03', '04', '05', '06', '07'].map((day) => broadcast(day, `2026-12-${day}`)),
      total_count: 7
    })
    renderCard()

    expect((await screen.findByText('+2 more')).closest('a')).toHaveAttribute('href', '/broadcasts?status=scheduled')
    expect(screen.getAllByText(/^Broadcast /)).toHaveLength(5)
  })

  it('opens the scheduled broadcasts from a row', async () => {
    listBroadcasts.mockResolvedValue({
      broadcasts: [broadcast('soon', '2026-11-27')],
      total_count: 1
    })
    renderCard()

    fireEvent.click(await screen.findByText('Broadcast soon'))
    expect(navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/broadcasts',
      params: { workspaceId: 'ws1' },
      search: { status: 'scheduled' }
    })
  })

  it('offers a new broadcast when none is scheduled, to a member who may create one', async () => {
    listBroadcasts.mockResolvedValue({ broadcasts: [], total_count: 0 })
    renderCard()

    expect(await screen.findByText('Scheduled broadcasts will show here.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'New broadcast' })).toBeInTheDocument()
    // Straight to the composer, not to the list of broadcasts.
    expect(screen.getByText('New broadcast').closest('a')).toHaveAttribute('href', '/broadcasts?compose=new')
    expect(screen.queryByText(/needs an email provider/)).not.toBeInTheDocument()
    await waitFor(() => expect(listLists).not.toHaveBeenCalled())
  })

  it('shows no new broadcast button without the right to write broadcasts', async () => {
    listBroadcasts.mockResolvedValue({ broadcasts: [], total_count: 0 })
    renderCard({ canCreate: false })

    expect(await screen.findByText('Scheduled broadcasts will show here.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'New broadcast' })).not.toBeInTheDocument()
  })

  it('says a broadcast needs an email provider, with a link to configure one', async () => {
    listBroadcasts.mockResolvedValue({ broadcasts: [], total_count: 0 })
    renderCard({ marketingReady: false, canConfigure: true })

    expect(await screen.findByText(/^Sending it needs an email provider/)).toBeInTheDocument()
    expect(screen.getByText('Configure').closest('a')).toHaveAttribute('href', '/settings/integrations')
  })

  it('offers no configure link to a member who is not an owner', async () => {
    listBroadcasts.mockResolvedValue({ broadcasts: [], total_count: 0 })
    renderCard({ marketingReady: false, canConfigure: false })

    expect(await screen.findByText('Sending it needs an email provider')).toBeInTheDocument()
    expect(screen.queryByText('Configure')).not.toBeInTheDocument()
  })
})
