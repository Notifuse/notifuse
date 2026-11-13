import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import { BroadcastCompose, type BroadcastComposeProps } from './BroadcastCompose'
import { broadcastApi, type Broadcast } from '../../../services/api/broadcast'
import { templatesApi } from '../../../services/api/template'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { ApiError } from '../../../services/api/errors'
import type { Workspace } from '../../../services/api/types'
import type { UserPermissions } from '../../../services/api/workspace'

vi.mock('../../../services/api/client', async () => ({
  ...(await vi.importActual<typeof import('../../../services/api/errors')>('../../../services/api/errors')),
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('../../../services/api/broadcast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../services/api/broadcast')>()),
  broadcastApi: { get: vi.fn(), create: vi.fn(), update: vi.fn(), schedule: vi.fn(), estimateAudience: vi.fn() }
}))
vi.mock('../../../services/api/template', () => ({ templatesApi: { get: vi.fn() } }))
vi.mock('../../../services/api/list', () => ({ listsApi: { list: vi.fn() } }))
vi.mock('../../../services/api/segment', () => ({ listSegments: vi.fn() }))
vi.mock('../detail/TemplatePreviewFrame', () => ({
  TemplatePreviewFrame: () => <div data-testid="frame" />
}))
vi.mock('../detail/useTemplatePreview', () => ({
  useTemplatePreview: () => ({ data: undefined })
}))
vi.mock('../../templates/TemplateSelectorInput', () => ({ default: () => <div data-testid="template-selector" /> }))
vi.mock('../VariationSenderSelect', () => ({ VariationSenderSelect: () => <div data-testid="sender-select" /> }))
vi.mock('../../templates/SendTemplateModal', () => ({
  default: ({ isOpen, defaultListId }: { isOpen: boolean; defaultListId?: string }) => (isOpen ? <div data-testid="send-test">{defaultListId}</div> : null)
}))
let shouldBlockFn: (args: { current: { pathname: string; search: object }; next: { pathname: string; search: object } }) => boolean | Promise<boolean>
vi.mock('@tanstack/react-router', () => ({
  useBlocker: (opts: { shouldBlockFn: typeof shouldBlockFn }) => {
    shouldBlockFn = opts.shouldBlockFn
    return { status: 'idle' }
  }
}))

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: {
    timezone: 'UTC',
    marketing_email_provider_id: 'int1',
    email_tracking: { opens_enabled: true, clicks_enabled: true, unknown_consent: 'track' }
  },
  integrations: [{ id: 'int1', email_provider: { senders: [{ id: 's1', name: 'Acme', email: 'a@acme.test', is_default: true }] } }]
} as unknown as Workspace

const permissions = {
  broadcasts: { read: true, write: true },
  lists: { read: true, write: false },
  segments: { read: true, write: false },
  templates: { read: true, write: false },
  transactional: { read: true, write: true },
  contacts: { read: true, write: false }
} as unknown as UserPermissions

const stored = (over: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'bc1',
    workspace_id: 'ws1',
    name: 'News',
    status: 'draft',
    audience: { list: 'l1', segments: [], exclude_unsubscribed: true },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: { enabled: false, sample_percentage: 50, auto_send_winner: false, variations: [{ variation_name: 'A', template_id: 'tpl-a' }] },
    utm_parameters: { source: 'acme.test', medium: 'email', campaign: 'news' },
    tracking_mode: 'inherit',
    data_feed: { global_feed: { enabled: false, url: '', headers: [] }, recipient_feed: { enabled: false, url: '', headers: [] } },
    created_at: '',
    updated_at: '',
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    ...over
  }) as Broadcast

// The drawer is a dialog itself: a confirmation is found by its title.
const confirmation = async (title: string | RegExp) => {
  // antd writes a confirmation's title twice: in a hidden modal title and in the confirm title.
  const found = await screen.findAllByText(title)
  return found[0].closest('.ant-modal-container, .ant-modal-content') as HTMLElement
}

const renderCompose = (props: Partial<BroadcastComposeProps> = {}, client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) => {
  const handlers = { onClose: vi.fn(), onCreated: vi.fn(), onOpenDetail: vi.fn() }
  render(
    <QueryClientProvider client={client}>
      <AntApp>
        <BroadcastCompose workspace={workspace} permissions={permissions} composeId="bc1" {...handlers} {...props} />
      </AntApp>
    </QueryClientProvider>
  )
  return { ...handlers, client }
}

describe('BroadcastCompose', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: stored() })
    ;(broadcastApi.estimateAudience as Mock).mockResolvedValue({ count: 1200 })
    ;(broadcastApi.schedule as Mock).mockResolvedValue({ success: true })
    ;(broadcastApi.update as Mock).mockImplementation(async (p: Broadcast) => ({ broadcast: stored({ ...p, status: 'scheduled' } as Partial<Broadcast>) }))
    ;(templatesApi.get as Mock).mockResolvedValue({
      template: { id: 'tpl-a', name: 'Alpha', email: { subject: 'Hello', editor_mode: 'code', mjml_source: '{{ unsubscribe_url }}' } }
    })
    ;(listsApi.list as Mock).mockResolvedValue({ lists: [{ id: 'l1', name: 'Newsletter', active_count: 1500 }] })
    ;(listSegments as Mock).mockResolvedValue({ segments: [] })
  })

  it('opens a new broadcast named after today, and creates nothing while it is incomplete', async () => {
    renderCompose({ composeId: 'new' })
    const name = (await screen.findByRole('textbox', { name: 'Broadcast name' })) as HTMLInputElement
    expect(name.value).toMatch(/^Broadcast /)
    expect(await screen.findByText('Incomplete: choose a list')).toBeInTheDocument()
    expect(broadcastApi.create).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Send test' })).toBeDisabled()
  })

  it('closes a new broadcast left untouched without asking or creating it', async () => {
    const { onClose } = renderCompose({ composeId: 'new' })
    await userEvent.click(await screen.findByRole('button', { name: /Broadcasts/ }))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(screen.queryByText('Close without saving?')).not.toBeInTheDocument()
    expect(broadcastApi.create).not.toHaveBeenCalled()
  })

  it('asks before closing on an incomplete change, with its reason', async () => {
    const { onClose } = renderCompose({ composeId: 'new' })
    const name = await screen.findByRole('textbox', { name: 'Broadcast name' })
    await userEvent.type(name, ' x')
    await userEvent.click(screen.getByRole('button', { name: /Broadcasts/ }))
    const dialog = await confirmation('Close without saving?')
    expect(within(dialog).getByText('The last changes are not saved: choose a list')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('opens the detail instead for a broadcast that is no longer a draft or scheduled', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({ broadcast: stored({ status: 'processed' }) })
    const { onOpenDetail } = renderCompose()
    await waitFor(() => expect(onOpenDetail).toHaveBeenCalledWith('bc1'))
  })

  it('opens the detail, or closes, for a member who may not write', async () => {
    const readOnly = { ...permissions, broadcasts: { read: true, write: false } } as unknown as UserPermissions
    const first = renderCompose({ permissions: readOnly })
    await waitFor(() => expect(first.onOpenDetail).toHaveBeenCalledWith('bc1'))
    const second = renderCompose({ permissions: readOnly, composeId: 'new' })
    await waitFor(() => expect(second.onClose).toHaveBeenCalled())
  })

  it('waits for the permissions before gating a link to the composer', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const handlers = { onClose: vi.fn(), onCreated: vi.fn(), onOpenDetail: vi.fn() }
    const tree = (perms: UserPermissions | null, composeId: string) => (
      <QueryClientProvider client={client}>
        <AntApp>
          <BroadcastCompose workspace={workspace} permissions={perms} composeId={composeId} {...handlers} />
        </AntApp>
      </QueryClientProvider>
    )
    const { rerender } = render(tree(null, 'bc1'))
    await waitFor(() => expect(broadcastApi.get).toHaveBeenCalled())
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(handlers.onOpenDetail).not.toHaveBeenCalled()
    expect(screen.queryByRole('textbox', { name: 'Broadcast name' })).toBeNull()

    rerender(tree(permissions as unknown as UserPermissions, 'bc1'))
    expect(await screen.findByRole('textbox', { name: 'Broadcast name' })).toBeInTheDocument()
    expect(handlers.onOpenDetail).not.toHaveBeenCalled()
    expect(handlers.onClose).not.toHaveBeenCalled()
  })

  it('says Broadcast not found for an unknown id', async () => {
    ;(broadcastApi.get as Mock).mockRejectedValue(new ApiError('not found', 404))
    renderCompose()
    expect(await screen.findByText('Broadcast not found')).toBeInTheDocument()
  })

  it('sends a complete draft now after confirming, then opens its detail', async () => {
    const { onOpenDetail } = renderCompose()
    const send = await screen.findByRole('button', { name: /Send now/ })
    await waitFor(() => expect(send).toBeEnabled())
    await userEvent.click(send)
    const dialog = await confirmation('Send to ≈ 1200 recipients now?')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Send now' }))
    await waitFor(() => expect(broadcastApi.schedule).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'bc1', send_now: true }))
    expect(onOpenDetail).toHaveBeenCalledWith('bc1')
  })

  it('schedules a draft at its date, time and timezone, without asking', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({
      broadcast: stored({ schedule: { is_scheduled: false, scheduled_date: '2030-01-02', scheduled_time: '09:00', timezone: 'Europe/Paris', use_recipient_timezone: false } })
    })
    const { onOpenDetail } = renderCompose()
    const button = await screen.findByRole('button', { name: /^Schedule$/ })
    await waitFor(() => expect(button).toBeEnabled())
    await userEvent.click(button)
    await waitFor(() =>
      expect(broadcastApi.schedule).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        id: 'bc1',
        send_now: false,
        scheduled_date: '2030-01-02',
        scheduled_time: '09:00',
        timezone: 'Europe/Paris',
        use_recipient_timezone: false
      })
    )
    expect(onOpenDetail).toHaveBeenCalledWith('bc1')
  })

  it('shows the server refusal of a send and stays', async () => {
    ;(broadcastApi.schedule as Mock).mockRejectedValue(new Error('Restricted in demo mode'))
    const { onOpenDetail } = renderCompose()
    const send = await screen.findByRole('button', { name: /Send now/ })
    await waitFor(() => expect(send).toBeEnabled())
    await userEvent.click(send)
    await userEvent.click(within(await confirmation(/now\?$/)).getByRole('button', { name: 'Send now' }))
    expect(await screen.findByText('Restricted in demo mode')).toBeInTheDocument()
    expect(onOpenDetail).not.toHaveBeenCalled()
  })

  it('disables the send without a marketing provider', async () => {
    renderCompose({ workspace: { ...workspace, settings: { ...workspace.settings, marketing_email_provider_id: undefined } } as unknown as Workspace })
    const send = await screen.findByRole('button', { name: /Send now/ })
    expect(send).toBeDisabled()
    expect(screen.getByText('No marketing email provider')).toBeInTheDocument()
  })

  it('offers no send for a scheduled broadcast: its changes are saved with Save changes, schedule kept', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({
      broadcast: stored({ status: 'scheduled', schedule: { is_scheduled: true, scheduled_date: '2030-01-02', scheduled_time: '09:00', timezone: 'UTC', use_recipient_timezone: false } })
    })
    renderCompose()
    const name = await screen.findByRole('textbox', { name: 'Broadcast name' })
    expect(screen.queryByRole('button', { name: /Send now|^Schedule$/ })).not.toBeInTheDocument()
    expect(screen.getByText(/^Scheduled for .* at /)).toBeInTheDocument()
    fireEvent.change(name, { target: { value: 'News fixed' } })
    await userEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalledTimes(1))
    const payload = (broadcastApi.update as Mock).mock.calls[0][0]
    expect(payload).toMatchObject({ id: 'bc1', name: 'News fixed' })
    expect(payload).not.toHaveProperty('schedule')
  })

  it('shows the refusal of Save changes on a scheduled broadcast', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({
      broadcast: stored({ status: 'scheduled', schedule: { is_scheduled: true, scheduled_date: '2030-01-02', scheduled_time: '09:00', timezone: 'UTC', use_recipient_timezone: false } })
    })
    ;(broadcastApi.update as Mock).mockRejectedValue(new ApiError('The list no longer exists', 400))
    renderCompose()
    fireEvent.change(await screen.findByRole('textbox', { name: 'Broadcast name' }), { target: { value: 'News fixed' } })
    await userEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    expect(await screen.findByText('The list no longer exists')).toBeInTheDocument()
  })

  it('sends a test of a scheduled broadcast without saving its pending changes', async () => {
    ;(broadcastApi.get as Mock).mockResolvedValue({
      broadcast: stored({ status: 'scheduled', schedule: { is_scheduled: true, scheduled_date: '2030-01-02', scheduled_time: '09:00', timezone: 'UTC', use_recipient_timezone: false } })
    })
    renderCompose()
    fireEvent.change(await screen.findByRole('textbox', { name: 'Broadcast name' }), { target: { value: 'News fixed' } })
    const button = screen.getByRole('button', { name: 'Send test' })
    await waitFor(() => expect(button).toBeEnabled())
    await userEvent.click(button)
    expect(await screen.findByTestId('send-test')).toBeInTheDocument()
    expect(broadcastApi.update).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeInTheDocument()
  })

  it('still opens the test of an incomplete draft: nothing was refused', async () => {
    renderCompose()
    fireEvent.change(await screen.findByRole('textbox', { name: 'Broadcast name' }), { target: { value: '' } })
    const button = screen.getByRole('button', { name: 'Send test' })
    await waitFor(() => expect(button).toBeEnabled())
    await userEvent.click(button)
    expect(await screen.findByTestId('send-test')).toBeInTheDocument()
    expect(broadcastApi.update).not.toHaveBeenCalled()
  })

  it('opens no test when the draft cannot be saved', async () => {
    ;(broadcastApi.update as Mock).mockRejectedValue(new ApiError('Refused', 400))
    renderCompose()
    fireEvent.change(await screen.findByRole('textbox', { name: 'Broadcast name' }), { target: { value: 'News 2' } })
    const button = screen.getByRole('button', { name: 'Send test' })
    await waitFor(() => expect(button).toBeEnabled())
    await userEvent.click(button)
    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect((await screen.findAllByText('Refused')).length).toBeGreaterThan(0)
    expect(screen.queryByTestId('send-test')).toBeNull()
  })

  it('sends a test of the previewed template for the broadcast list', async () => {
    renderCompose()
    const button = await screen.findByRole('button', { name: 'Send test' })
    await waitFor(() => expect(button).toBeEnabled())
    await userEvent.click(button)
    expect(await screen.findByTestId('send-test')).toHaveTextContent('l1')
  })

  it('requests no template without Templates read', async () => {
    renderCompose({ permissions: { ...permissions, templates: { read: false, write: false } } as unknown as UserPermissions })
    await screen.findByRole('textbox', { name: 'Broadcast name' })
    expect(await screen.findByText("You need Templates read permission to see this template's name and preview")).toBeInTheDocument()
    expect(templatesApi.get).not.toHaveBeenCalled()
  })

  it('guards leaving the screen, not its own change of URL after a create', async () => {
    renderCompose({ composeId: 'new' })
    await userEvent.type(await screen.findByRole('textbox', { name: 'Broadcast name' }), ' x')
    const here = { pathname: '/console/workspace/ws1/broadcasts', search: { compose: 'new' } }
    expect(await shouldBlockFn({ current: here, next: { ...here, search: { compose: 'bc1' } } })).toBe(false)
    expect(await shouldBlockFn({ current: here, next: { ...here, search: {} } })).toBe(true)
    expect(await shouldBlockFn({ current: here, next: { pathname: '/console/workspace/ws1/contacts', search: {} } })).toBe(true)
  })
})
