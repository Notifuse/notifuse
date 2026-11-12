import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactElement } from 'react'
import { UpsertBroadcastDrawer } from './UpsertBroadcastDrawer'
import { broadcastApi, Broadcast } from '../../services/api/broadcast'
import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import type { List } from '../../services/api/list'

// The template selector pulls in template list queries; replace it with a bare
// input so the required template_id field can be filled without network calls.
vi.mock('../templates/TemplateSelectorInput', () => ({
  default: ({
    value,
    onChange,
    canReadTemplates
  }: {
    value?: string
    onChange?: (value: string) => void
    canReadTemplates?: boolean
  }) => (
    <input
      data-testid="template-selector"
      data-can-read-templates={String(canReadTemplates)}
      value={value ?? ''}
      onChange={(e) => onChange?.(e.target.value)}
    />
  )
}))

// The sender field reads the chosen template's sender from the template list.
vi.mock('../../services/api/template', () => ({
  templatesApi: {
    list: vi.fn().mockResolvedValue({ templates: [{ id: 'tmpl1', email: { sender_id: 's2' } }] })
  }
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
  renderDrawer(<UpsertBroadcastDrawer canReadTemplates workspace={workspace} broadcast={broadcast} lists={lists} />)
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
  (broadcastApi.update as Mock).mock.calls[0][0]

describe('UpsertBroadcastDrawer data feeds', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.update as Mock).mockResolvedValue({ broadcast: {} })
    ;(broadcastApi.create as Mock).mockResolvedValue({ broadcast: {} })
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
    renderDrawer(<UpsertBroadcastDrawer canReadTemplates workspace={workspace} lists={lists} />)
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
    const payload = (broadcastApi.create as Mock).mock.calls[0][0]
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
    ;(broadcastApi.update as Mock).mockResolvedValue({ broadcast: {} })
    ;(broadcastApi.create as Mock).mockResolvedValue({ broadcast: {} })
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
    renderDrawer(<UpsertBroadcastDrawer canReadTemplates workspace={workspace} lists={lists} />)
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
    const payload = (broadcastApi.create as Mock).mock.calls[0][0]
    expect(payload.schedule).toEqual({ is_scheduled: false, use_recipient_timezone: false })
  })

  it('creates a new broadcast tracked, with the "Don\'t track" box unchecked', async () => {
    renderDrawer(<UpsertBroadcastDrawer canReadTemplates workspace={workspace} lists={lists} />)
    await userEvent.click(screen.getByRole('button', { name: /Create Broadcast/i }))

    await userEvent.type(
      screen.getByPlaceholderText('E.g. Weekly Newsletter - May 2023'),
      'Weekly Newsletter'
    )
    await userEvent.click(screen.getAllByRole('combobox')[0])
    await userEvent.click(await screen.findByText('Newsletter'))

    await goToTab('2. Web Analytics')
    expect(screen.getByRole('checkbox', { name: /Don't track opens and clicks/i })).not.toBeChecked()

    await goToTab('4. Content')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.create).toHaveBeenCalled())
    expect((broadcastApi.create as Mock).mock.calls[0][0].tracking_mode).toBe('inherit')
  })
})

describe('UpsertBroadcastDrawer tracking', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.update as Mock).mockResolvedValue({ broadcast: {} })
    ;(broadcastApi.create as Mock).mockResolvedValue({ broadcast: {} })
  })

  const noTrackingCheckbox = () =>
    screen.getByRole('checkbox', { name: /Don't track opens and clicks/i })

  it('follows the workspace and each contact by default', async () => {
    await openEditDrawer(makeBroadcast())

    await save()

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().tracking_mode).toBe('inherit')
  })

  it('sends no_tracking when the user turns tracking off for the broadcast', async () => {
    await openEditDrawer(makeBroadcast())

    await goToTab('2. Web Analytics')
    await userEvent.click(noTrackingCheckbox())
    await save()

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().tracking_mode).toBe('no_tracking')
  })

  it('shows the stored no_tracking mode checked and turns it back on', async () => {
    await openEditDrawer(makeBroadcast({ tracking_mode: 'no_tracking' }))

    await goToTab('2. Web Analytics')
    expect(noTrackingCheckbox()).toBeChecked()
    await userEvent.click(noTrackingCheckbox())
    await save()

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().tracking_mode).toBe('inherit')
  })

  it('turns off and locks the automatic winner when the broadcast is not tracked', async () => {
    await openEditDrawer(
      makeBroadcast({
        test_settings: {
          enabled: true,
          sample_percentage: 50,
          auto_send_winner: true,
          auto_send_winner_metric: 'open_rate',
          test_duration_hours: 2,
          variations: [
            { variation_name: 'A', template_id: 'tmpl1' },
            { variation_name: 'B', template_id: 'tmpl2' }
          ]
        }
      } as Partial<Broadcast>)
    )

    await goToTab('2. Web Analytics')
    await userEvent.click(noTrackingCheckbox())
    await goToTab('4. Content')

    const autoWinner = within(
      screen.getByText('Automatically send winner').closest('.ant-form-item') as HTMLElement
    ).getByRole('switch')
    expect(autoWinner).not.toBeChecked()
    expect(autoWinner).toBeDisabled()

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().test_settings.auto_send_winner).toBe(false)
    expect(updatePayload().tracking_mode).toBe('no_tracking')
  })

  it('warns that only consenting contacts count when unknown contacts are not tracked', async () => {
    const strictWorkspace = {
      ...workspace,
      settings: {
        ...workspace.settings,
        email_tracking: { opens_enabled: true, clicks_enabled: true, unknown_consent: 'no_track' }
      }
    } as unknown as Workspace
    renderDrawer(
      <UpsertBroadcastDrawer
        canReadTemplates
        workspace={strictWorkspace}
        broadcast={makeBroadcast({
          test_settings: {
            enabled: true,
            sample_percentage: 50,
            auto_send_winner: false,
            variations: [
              { variation_name: 'A', template_id: 'tmpl1' },
              { variation_name: 'B', template_id: 'tmpl2' }
            ]
          }
        } as Partial<Broadcast>)}
        lists={lists}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit Broadcast/i }))
    await goToTab('4. Content')

    expect(
      screen.getByText(/only recipients who agreed to tracking count in the test results/i)
    ).toBeInTheDocument()
  })

  it('says nothing about consent when unknown contacts are tracked', async () => {
    await openEditDrawer(
      makeBroadcast({
        test_settings: {
          enabled: true,
          sample_percentage: 50,
          auto_send_winner: false,
          variations: [
            { variation_name: 'A', template_id: 'tmpl1' },
            { variation_name: 'B', template_id: 'tmpl2' }
          ]
        }
      } as Partial<Broadcast>)
    )
    await goToTab('4. Content')

    expect(
      screen.queryByText(/only recipients who agreed to tracking count in the test results/i)
    ).not.toBeInTheDocument()
  })
})

// Switching the exclusion off is how a notice every member must receive is sent (issue #484);
// the switch says what else it changes: the emails carry no List-Unsubscribe header.
describe('UpsertBroadcastDrawer unsubscribed recipients', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('explains that including unsubscribed contacts drops the List-Unsubscribe header', async () => {
    await openEditDrawer(makeBroadcast())

    expect(screen.getByText('Exclude unsubscribed recipients')).toBeInTheDocument()
    expect(
      screen.getByText(/carry no List-Unsubscribe header, so mail clients show no Unsubscribe button/)
    ).toBeInTheDocument()
  })
})

describe('UpsertBroadcastDrawer sender', () => {
  const withSenders = {
    ...workspace,
    settings: { ...workspace.settings, marketing_email_provider_id: 'mkt' },
    integrations: [
      {
        id: 'mkt',
        email_provider: {
          senders: [
            { id: 's1', name: 'Default', email: 'hello@example.com', is_default: true },
            { id: 's2', name: 'Newsletter', email: 'news@example.com', is_default: false },
            { id: 's3', name: 'Promos', email: 'promos@example.com', is_default: false }
          ]
        }
      }
    ]
  } as unknown as Workspace

  const openWithSenders = async (broadcast: Broadcast) => {
    renderDrawer(
      <UpsertBroadcastDrawer canReadTemplates workspace={withSenders} broadcast={broadcast} lists={lists} />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit Broadcast/i }))
    await goToTab('4. Content')
  }

  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.update as Mock).mockResolvedValue({ broadcast: {} })
  })

  it('sends the sender chosen for the variation', async () => {
    await openWithSenders(makeBroadcast())

    expect(await screen.findByText("Template's sender (Newsletter)")).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Sender'))
    await userEvent.click(await screen.findByText('Promos <promos@example.com>'))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().test_settings.variations[0]).toMatchObject({
      template_id: 'tmpl1',
      sender_id: 's3'
    })
  })

  it("sends no sender when the variation keeps the template's", async () => {
    await openWithSenders(makeBroadcast())
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().test_settings.variations[0].sender_id).toBeUndefined()
  })

  it('sends no sender once the chosen one is cleared', async () => {
    await openWithSenders(
      makeBroadcast({
        test_settings: {
          enabled: false,
          sample_percentage: 50,
          auto_send_winner: false,
          variations: [{ variation_name: 'default', template_id: 'tmpl1', sender_id: 's3' }]
        }
      })
    )

    const select = screen.getByLabelText('Sender').closest('.ant-select') as HTMLElement
    const clear = select.querySelector('.ant-select-clear') as HTMLElement
    fireEvent.mouseDown(clear)
    fireEvent.click(clear)
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().test_settings.variations[0].sender_id).toBeUndefined()
  })

  it('refuses to save a sender that has left the integration', async () => {
    await openWithSenders(
      makeBroadcast({
        test_settings: {
          enabled: false,
          sample_percentage: 50,
          auto_send_winner: false,
          variations: [{ variation_name: 'default', template_id: 'tmpl1', sender_id: 'gone' }]
        }
      })
    )

    expect(screen.getByText('gone (deleted)')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText(
        "This sender is no longer one of the marketing email integration's. Choose another one or clear the field to use the template's."
      )
    ).toBeInTheDocument()
    expect(broadcastApi.update).not.toHaveBeenCalled()
  })

  it('gives each A/B variation its own sender', async () => {
    await openWithSenders(
      makeBroadcast({
        test_settings: {
          enabled: true,
          sample_percentage: 20,
          auto_send_winner: false,
          variations: [
            { variation_name: 'a', template_id: 'tmpl1' },
            { variation_name: 'b', template_id: 'tmpl2', sender_id: 's3' }
          ]
        }
      })
    )

    await userEvent.click(screen.getByLabelText('Sender 1'))
    await userEvent.click(await screen.findByText('Default <hello@example.com>'))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    const variations = updatePayload().test_settings.variations
    expect(variations[0].sender_id).toBe('s1')
    expect(variations[1].sender_id).toBe('s3')
  })
})

// Without Templates read, templates.list and templates.get answer 403: each variation's template
// field gets the permission, so it shows the stored ID locked, and the sender field beside it
// requests nothing. The broadcast still saves with its templates.
describe('UpsertBroadcastDrawer without Templates read', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.update as Mock).mockResolvedValue({ broadcast: {} })
  })

  const openLocked = async (broadcast: Broadcast) => {
    renderDrawer(
      <UpsertBroadcastDrawer
        canReadTemplates={false}
        workspace={workspace}
        broadcast={broadcast}
        lists={lists}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit Broadcast/i }))
    await goToTab('4. Content')
  }

  it('locks the template of a single variation and keeps it on save', async () => {
    await openLocked(makeBroadcast())

    const selectors = screen.getAllByTestId('template-selector')
    expect(selectors.map((el) => el.getAttribute('data-can-read-templates'))).toEqual(['false'])
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalled())
    expect(updatePayload().test_settings.variations[0].template_id).toBe('tmpl1')
    expect(templatesApi.list).not.toHaveBeenCalled()
  })

  it('locks the template of every A/B variation', async () => {
    await openLocked(
      makeBroadcast({
        test_settings: {
          enabled: true,
          sample_percentage: 20,
          auto_send_winner: false,
          variations: [
            { variation_name: 'a', template_id: 'tmpl1' },
            { variation_name: 'b', template_id: 'tmpl2' }
          ]
        }
      })
    )

    const selectors = screen.getAllByTestId('template-selector')
    expect(selectors.map((el) => el.getAttribute('data-can-read-templates'))).toEqual([
      'false',
      'false'
    ])
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(templatesApi.list).not.toHaveBeenCalled()
  })

  it('gives the template fields the permission when the member has it', async () => {
    renderDrawer(
      <UpsertBroadcastDrawer canReadTemplates workspace={workspace} broadcast={makeBroadcast()} lists={lists} />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit Broadcast/i }))
    await goToTab('4. Content')

    expect(screen.getByTestId('template-selector').getAttribute('data-can-read-templates')).toBe('true')
    await waitFor(() => expect(templatesApi.list).toHaveBeenCalled())
  })
})
