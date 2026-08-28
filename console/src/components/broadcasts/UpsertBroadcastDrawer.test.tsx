import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactElement } from 'react'
import { UpsertBroadcastDrawer } from './UpsertBroadcastDrawer'
import { broadcastApi, Broadcast } from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'
import type { List } from '../../services/api/list'

// The template selector pulls in template list queries; replace it with a bare
// input so the required template_id field can be filled without network calls.
vi.mock('../templates/TemplateSelectorInput', () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (value: string) => void }) => (
    <input
      data-testid="template-selector"
      value={value ?? ''}
      onChange={(e) => onChange?.(e.target.value)}
    />
  )
}))

vi.mock('../../services/api/broadcast', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/broadcast')>()
  return {
    ...actual,
    broadcastApi: {
      list: vi.fn(),
      get: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      schedule: vi.fn(),
      refreshGlobalFeed: vi.fn(),
      testRecipientFeed: vi.fn()
    }
  }
})

const workspace = {
  id: 'ws1',
  settings: {
    website_url: 'https://example.com',
    email_tracking_enabled: true
  }
} as unknown as Workspace

const lists = [{ id: 'list1', name: 'Newsletter' }] as unknown as List[]

const makeBroadcast = (overrides: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'bc1',
    workspace_id: 'ws1',
    name: 'Weekly Newsletter',
    channel_type: 'email',
    status: 'draft',
    audience: { list: 'list1', segments: [], exclude_unsubscribed: true },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: {
      enabled: false,
      sample_percentage: 50,
      auto_send_winner: false,
      variations: [{ variation_name: 'default', template_id: 'tmpl1' }]
    },
    data_feed: {
      global_feed: { enabled: true, url: 'https://api.example.com/global', headers: [] },
      recipient_feed: { enabled: true, url: 'https://api.example.com/recipient', headers: [] }
    },
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides
  }) as unknown as Broadcast

function renderDrawer(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AntApp>{ui}</AntApp>
    </QueryClientProvider>
  )
}

const openEditDrawer = async (broadcast: Broadcast) => {
  renderDrawer(<UpsertBroadcastDrawer workspace={workspace} broadcast={broadcast} lists={lists} />)
  await userEvent.click(screen.getByRole('button', { name: /Edit Broadcast/i }))
}

// The two feed switches carry no accessible name of their own; each sits in the
// heading row next to its title.
const feedSwitch = (title: string) =>
  within(screen.getByText(title).closest('div') as HTMLElement).getByRole('switch')

const goToTab = async (label: string) => {
  await userEvent.click(screen.getByRole('tab', { name: label }))
}

const save = async () => {
  await goToTab('4. Content')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
}

const updatePayload = () =>
  (broadcastApi.update as ReturnType<typeof vi.fn>).mock.calls[0][0]

describe('UpsertBroadcastDrawer data feeds', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.update as ReturnType<typeof vi.fn>).mockResolvedValue({ broadcast: {} })
    ;(broadcastApi.create as ReturnType<typeof vi.fn>).mockResolvedValue({ broadcast: {} })
  })

  it('turns a stored feed off when the user unchecks both toggles', async () => {
    // An absent data_feed - and an absent sub-feed inside it - means "keep what is
    // stored" on the server, so the payload has to spell out that both feeds are off
    // or the broadcast keeps calling the feed URLs on every send.
    await openEditDrawer(makeBroadcast())

    await goToTab('3. Data Feeds')
    await userEvent.click(feedSwitch('Global Data Feed'))
    await userEvent.click(feedSwitch('Per-Recipient Data Feed'))

    await save()

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    const payload = updatePayload()
    expect(payload.data_feed).toBeDefined()
    expect(payload.data_feed.global_feed).toBeDefined()
    expect(payload.data_feed.global_feed.enabled).toBe(false)
    expect(payload.data_feed.recipient_feed).toBeDefined()
    expect(payload.data_feed.recipient_feed.enabled).toBe(false)
  })

  it('records both feeds as off on a broadcast created without any feed', async () => {
    renderDrawer(<UpsertBroadcastDrawer workspace={workspace} lists={lists} />)
    await userEvent.click(screen.getByRole('button', { name: /Create Broadcast/i }))

    await userEvent.type(
      screen.getByPlaceholderText('E.g. Weekly Newsletter - May 2023'),
      'Weekly Newsletter'
    )
    await userEvent.click(screen.getAllByRole('combobox')[0])
    await userEvent.click(await screen.findByText('Newsletter'))

    await goToTab('4. Content')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.create).toHaveBeenCalled())
    const payload = (broadcastApi.create as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.data_feed.global_feed.enabled).toBe(false)
    expect(payload.data_feed.recipient_feed.enabled).toBe(false)
  })

  it('keeps the feed the user left on while turning the other one off', async () => {
    await openEditDrawer(makeBroadcast())

    await goToTab('3. Data Feeds')
    await userEvent.click(feedSwitch('Per-Recipient Data Feed'))

    await save()

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    const payload = updatePayload()
    expect(payload.data_feed.global_feed).toEqual({
      enabled: true,
      url: 'https://api.example.com/global',
      headers: []
    })
    expect(payload.data_feed.recipient_feed.enabled).toBe(false)
  })
})

// The drawer edits the audience, the templates and the UTM tags; the send date is owned by
// the send-or-schedule modal and broadcasts.schedule. The pencil is offered on a scheduled
// broadcast too, so a typo fix must not be able to move the date it is already booked for.
describe('UpsertBroadcastDrawer schedule', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.update as ReturnType<typeof vi.fn>).mockResolvedValue({ broadcast: {} })
    ;(broadcastApi.create as ReturnType<typeof vi.fn>).mockResolvedValue({ broadcast: {} })
  })

  it('sends no schedule when a scheduled broadcast is edited', async () => {
    await openEditDrawer(
      makeBroadcast({
        status: 'scheduled',
        schedule: {
          is_scheduled: true,
          scheduled_date: '2026-09-01',
          scheduled_time: '09:00',
          timezone: 'Europe/Paris',
          use_recipient_timezone: false
        }
      })
    )

    await save()

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    // An absent schedule is the server's "leave the stored one alone". Sending the drawer's
    // empty default instead clears is_scheduled, and a later resume reads that as "send now".
    expect('schedule' in updatePayload()).toBe(false)
  })

  it('sends no schedule when a draft broadcast is edited', async () => {
    await openEditDrawer(makeBroadcast())

    await save()

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect('schedule' in updatePayload()).toBe(false)
  })

  it('creates a new broadcast unscheduled', async () => {
    renderDrawer(<UpsertBroadcastDrawer workspace={workspace} lists={lists} />)
    await userEvent.click(screen.getByRole('button', { name: /Create Broadcast/i }))

    await userEvent.type(
      screen.getByPlaceholderText('E.g. Weekly Newsletter - May 2023'),
      'Weekly Newsletter'
    )
    await userEvent.click(screen.getAllByRole('combobox')[0])
    await userEvent.click(await screen.findByText('Newsletter'))

    await goToTab('4. Content')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.create).toHaveBeenCalled())
    const payload = (broadcastApi.create as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.schedule).toEqual({ is_scheduled: false, use_recipient_timezone: false })
  })
})
