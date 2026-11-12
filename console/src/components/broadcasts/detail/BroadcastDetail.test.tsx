import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import { BroadcastDetail, type BroadcastDetailProps } from './BroadcastDetail'
import { broadcastApi, type Broadcast } from '../../../services/api/broadcast'
import { getBroadcastStats, getBroadcastVariationStats, getBroadcastLinkStats } from '../../../services/api/messages_history'
import { taskApi } from '../../../services/api/task'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { ApiError } from '../../../services/api/client'
import type { Workspace } from '../../../services/api/types'
import type { UserPermissions } from '../../../services/api/workspace'

vi.mock('../../../services/api/broadcast', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../services/api/broadcast')>()
  return {
    ...actual,
    broadcastApi: { get: vi.fn(), getTestResults: vi.fn(), delete: vi.fn(), pause: vi.fn(), selectWinner: vi.fn() }
  }
})
vi.mock('../../../services/api/client', async () => ({
  ...(await vi.importActual<typeof import('../../../services/api/errors')>('../../../services/api/errors')),
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('../../../services/api/messages_history', () => ({
  getBroadcastStats: vi.fn(),
  getBroadcastVariationStats: vi.fn(),
  getBroadcastLinkStats: vi.fn()
}))
vi.mock('../../../services/api/task', () => ({ taskApi: { findByBroadcastId: vi.fn() } }))
vi.mock('../../../services/api/list', () => ({ listsApi: { list: vi.fn() } }))
vi.mock('../../../services/api/segment', () => ({ listSegments: vi.fn() }))
vi.mock('../broadcastQueryClient', () => ({ runBroadcastQuery: vi.fn().mockResolvedValue({ data: [] }) }))
vi.mock('../ChangeVariationSenderModal', () => ({
  ChangeVariationSenderModal: ({ canReadTemplates }: { canReadTemplates: boolean }) => (
    <span data-testid="sender-modal" data-can-read-templates={String(canReadTemplates)} />
  )
}))
vi.mock('./EngagementChart', () => ({ EngagementChart: () => <div data-testid="engagement-chart" /> }))
vi.mock('./TemplatePreviewFrame', () => ({ TemplatePreviewFrame: () => <div data-testid="frame" /> }))
vi.mock('../../templates/SendTemplateModal', () => ({
  default: ({ isOpen, template }: { isOpen: boolean; template: { id: string } | null }) =>
    isOpen ? <div data-testid="send-test-modal">{template?.id}</div> : null
}))
const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  useRouter: () => ({ buildLocation: () => ({ href: '/analytics' }) })
}))

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { marketing_email_provider_id: 'int1', timezone: 'UTC' },
  integrations: [
    {
      id: 'int1',
      email_provider: {
        kind: 'sparkpost',
        senders: [
          { id: 's1', name: 'Acme', email: 'hello@acme.test', is_default: true },
          { id: 's2', name: 'Acme News', email: 'news@acme.test', is_default: false }
        ]
      }
    }
  ]
} as unknown as Workspace

const allPermissions = {
  broadcasts: { read: true, write: true },
  message_history: { read: true, write: false },
  lists: { read: true, write: false },
  segments: { read: true, write: false },
  templates: { read: true, write: true },
  transactional: { read: true, write: true },
  contacts: { read: true, write: false }
} as unknown as UserPermissions

const tpl = (id: string) => ({ id, name: `Template ${id}`, email: { subject: `Subject ${id}` } })

const broadcastOf = (overrides: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'bc1',
    workspace_id: 'ws1',
    name: 'November news',
    status: 'processed',
    audience: { list: 'l1', exclude_unsubscribed: true },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: {
      enabled: false,
      sample_percentage: 0,
      auto_send_winner: false,
      variations: [{ variation_name: 'A', template_id: 'tpl-a', template: tpl('tpl-a') }]
    },
    utm_parameters: { campaign: 'november_news' },
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    started_at: '2026-11-02T10:00:00Z',
    completed_at: '2026-11-02T11:00:00Z',
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    ...overrides
  }) as Broadcast

const statsOf = (overrides: Record<string, number> = {}) => ({
  broadcast_id: 'bc1',
  stats: {
    total_sent: 1000,
    total_delivered: 990,
    total_opened: 400,
    total_clicked: 100,
    total_opens_tracked: 1000,
    total_clicks_tracked: 1000,
    total_bounced: 10,
    total_complained: 0,
    total_failed: 0,
    total_unsubscribed: 2,
    ...overrides
  },
  queue: { pending: 0, processing: 0, paused: 0, failed_retrying: 0, failed_terminal: 0, sent: 1000 }
})

const renderDetail = (props: Partial<BroadcastDetailProps> = {}) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onEdit = vi.fn()
  const onClose = vi.fn()
  const view = render(
    <QueryClientProvider client={client}>
      <AntApp>
        <BroadcastDetail
          workspace={workspace}
          permissions={allPermissions}
          broadcastId="bc1"
          onEdit={onEdit}
          onClose={onClose}
          {...props}
        />
      </AntApp>
    </QueryClientProvider>
  )
  return { ...view, client, onEdit, onClose }
}

describe('BroadcastDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: broadcastOf() })
    ;(getBroadcastStats as Mock).mockResolvedValue(statsOf())
    ;(getBroadcastVariationStats as Mock).mockResolvedValue({ stats: statsOf().stats })
    ;(getBroadcastLinkStats as Mock).mockResolvedValue({ link_stats: [] })
    ;(taskApi.findByBroadcastId as Mock).mockResolvedValue(null)
    ;(listsApi.list as Mock).mockResolvedValue({ lists: [{ id: 'l1', name: 'Newsletter' }] })
    ;(listSegments as Mock).mockResolvedValue({ segments: [] })
  })

  it('loads the broadcast by id with its templates, not from the list', async () => {
    renderDetail()
    expect(await screen.findByRole('heading', { name: 'November news', level: 1 })).toBeInTheDocument()
    expect(broadcastApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'bc1', with_templates: true })
  })

  it('says Broadcast not found for an unknown id, with a way back', async () => {
    ;(broadcastApi.get as Mock).mockRejectedValue(new ApiError('not found', 404))
    const { onClose } = renderDetail()
    expect(await screen.findByText('Broadcast not found')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Back to broadcasts' }))
    expect(onClose).toHaveBeenCalled()
  })

  it('shows the sent date, list and sender on the meta line of a sent broadcast', async () => {
    renderDetail()
    expect(await screen.findByText(/^Sent on .* at /)).toBeInTheDocument()
    expect(await screen.findByText('List Newsletter, unsubscribed excluded')).toBeInTheDocument()
    expect(screen.getAllByText('Acme <hello@acme.test>').length).toBeGreaterThan(0)
  })

  it('says Several senders when the variants leave from different senders', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({
      broadcast: broadcastOf({
        test_settings: {
          enabled: true,
          sample_percentage: 20,
          auto_send_winner: false,
          variations: [
            { variation_name: 'A', template_id: 'tpl-a', template: tpl('tpl-a') as never },
            { variation_name: 'B', template_id: 'tpl-b', sender_id: 's2', template: tpl('tpl-b') as never }
          ]
        }
      })
    })
    renderDetail()
    expect(await screen.findByText('Several senders')).toBeInTheDocument()
    expect(screen.getByText('A/B test')).toBeInTheDocument()
  })

  it('shows the funnel from the broadcast stats, and opens the logs of a cell', async () => {
    renderDetail()
    const sent = await screen.findByText('1,000')
    await userEvent.click(sent)
    expect(navigate).toHaveBeenCalledWith({ to: '/console/workspace/ws1/logs?broadcast_id=bc1' })
    expect(screen.getByTestId('engagement-chart')).toBeInTheDocument()
  })

  it('waits for the start of a draft or scheduled broadcast before any stats', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: broadcastOf({ status: 'draft', started_at: undefined, completed_at: undefined }) })
    const { onEdit } = renderDetail()
    expect(await screen.findByText('Stats appear once the broadcast starts sending')).toBeInTheDocument()
    expect(getBroadcastStats).not.toHaveBeenCalled()
    expect(screen.queryByTestId('engagement-chart')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Edit' }))
    expect(onEdit).toHaveBeenCalledWith('bc1')
  })

  it('shows opens and clicks as not tracked, with no chart, for a broadcast sent without tracking', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: broadcastOf({ tracking_mode: 'no_tracking' }) })
    renderDetail()
    expect(await screen.findByText('Sent without click tracking')).toBeInTheDocument()
    expect(screen.getAllByText('Not tracked').length).toBeGreaterThan(0)
    expect(screen.queryByTestId('engagement-chart')).not.toBeInTheDocument()
  })

  it('shows no funnel, chart nor links without the message history right', async () => {
    renderDetail({
      permissions: { ...allPermissions, message_history: { read: false, write: false } } as unknown as UserPermissions
    })
    await screen.findByRole('heading', { name: 'November news', level: 1 })
    expect(getBroadcastStats).not.toHaveBeenCalled()
    expect(screen.queryByText('Delivered')).not.toBeInTheDocument()
    expect(screen.queryByTestId('engagement-chart')).not.toBeInTheDocument()
    expect(screen.queryByText('Clicked links')).not.toBeInTheDocument()
  })

  it.each([true, false])('passes Templates read %s to the change of sender', async (read) => {
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: broadcastOf({ status: 'paused' }) })
    renderDetail({ permissions: { ...allPermissions, templates: { read, write: false } } as unknown as UserPermissions })
    expect(await screen.findByTestId('sender-modal')).toHaveAttribute('data-can-read-templates', String(read))
  })

  it('shows the task error under the header', async () => {
    ;(taskApi.findByBroadcastId as Mock).mockResolvedValue({ id: 't1', type: 'send_broadcast', status: 'failed', progress: 0, error_message: 'Provider refused the batch' })
    renderDetail()
    expect(await screen.findByRole('alert')).toHaveTextContent('Provider refused the batch')
  })

  it('offers Pause and Cancel while sending, disabled without write permission', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: broadcastOf({ status: 'processing', completed_at: undefined }) })
    renderDetail({ permissions: { ...allPermissions, broadcasts: { read: true, write: false } } as unknown as UserPermissions })
    expect(await screen.findByRole('button', { name: /Pause/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument()
  })

  it('opens the test modal on the single template', async () => {
    renderDetail()
    await userEvent.click(await screen.findByRole('button', { name: /Send test/ }))
    expect(screen.getByTestId('send-test-modal')).toHaveTextContent('tpl-a')
  })

  it('refreshes the broadcast, its stats, task, test results, variation stats and links', async () => {
    const { client } = renderDetail()
    await screen.findByText('1,000')
    const spy = vi.spyOn(client, 'invalidateQueries')
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    await userEvent.click(await screen.findByText('Refresh stats'))
    const keys = spy.mock.calls.map(([filters]) => (filters as { queryKey: unknown[] }).queryKey)
    expect(keys).toEqual(
      expect.arrayContaining([
        ['broadcast', 'ws1', 'bc1'],
        ['broadcast-stats', 'ws1', 'bc1'],
        ['task', 'ws1', 'bc1'],
        ['testResults', 'ws1', 'bc1'],
        ['broadcast-variation-stats', 'ws1', 'bc1'],
        ['broadcast-link-stats', 'ws1', 'bc1']
      ])
    )
  })

  it('closes before the delete refreshes the list', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: broadcastOf({ status: 'draft', started_at: undefined, completed_at: undefined }) })
    const order: string[] = []
    ;(broadcastApi.delete as Mock).mockImplementation(async () => {
      order.push('delete')
      return { success: true }
    })
    const { onClose } = renderDetail()
    onClose.mockImplementation(() => order.push('close'))
    await userEvent.click(await screen.findByRole('button', { name: 'More actions' }))
    await userEvent.click(await screen.findByText('Delete'))
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(within(dialog).getByRole('textbox'), 'bc1')
    await userEvent.click(within(dialog).getByRole('button', { name: /Delete/ }))
    await waitFor(() => expect(order).toEqual(['close', 'delete']))
  })

  it('refetches the broadcast every 30 seconds while it sends', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: broadcastOf({ status: 'processing', completed_at: undefined }) })
      renderDetail()
      await screen.findByRole('heading', { name: 'November news', level: 1 })
      expect(broadcastApi.get).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(30_000)
      await waitFor(() => expect(broadcastApi.get).toHaveBeenCalledTimes(2))
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not refetch a sent broadcast', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      renderDetail()
      await screen.findByText('1,000')
      await vi.advanceTimersByTimeAsync(31_000)
      expect(broadcastApi.get).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })
})
