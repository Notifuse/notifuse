import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactElement } from 'react'
import { Plus } from 'lucide-react'
import { UpsertTransactionalNotificationDrawer } from './UpsertTransactionalNotificationDrawer'
import {
  transactionalNotificationsApi,
  type TransactionalNotification
} from '../../services/api/transactional_notifications'
import { templatesApi, type Template } from '../../services/api/template'
import { ApiError } from '../../services/api/client'
import type { Workspace } from '../../services/api/types'

// The template selector pulls in template list queries and its own window; replace it with a bare
// input so the required template_id field can be filled without them.
vi.mock('../templates/TemplateSelectorInput', () => ({
  default: ({
    value,
    onChange,
    disabled,
    canReadTemplates,
    sendsWith
  }: {
    value?: string | null
    onChange?: (value: string | null) => void
    disabled?: boolean
    canReadTemplates?: boolean
    sendsWith?: unknown
  }) => (
    <input
      data-testid="template-selector"
      data-sends-with={JSON.stringify(sendsWith ?? null)}
      data-can-read-templates={String(canReadTemplates)}
      disabled={disabled}
      value={value ?? ''}
      onChange={(e) => onChange?.(e.target.value || null)}
    />
  )
}))

const { compile } = vi.hoisted(() => ({ compile: vi.fn() }))
vi.mock('../templates/compileTemplatePreview', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../templates/compileTemplatePreview')>()
  return { ...actual, compileTemplatePreview: compile }
})

vi.mock('../../services/api/transactional_notifications', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/transactional_notifications')>()
  return {
    ...actual,
    transactionalNotificationsApi: {
      list: vi.fn(),
      get: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      send: vi.fn(),
      testTemplate: vi.fn()
    }
  }
})

vi.mock('../../services/api/template', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/template')>()
  return { ...actual, templatesApi: { ...actual.templatesApi, get: vi.fn() } }
})

const workspaceWith = (settings: Record<string, unknown> = {}, integrations?: unknown[]) =>
  ({
    id: 'ws1',
    name: 'Acme',
    settings: {
      website_url: 'https://www.example.com',
      default_language: 'en',
      languages: ['en', 'fr'],
      email_tracking_enabled: true,
      transactional_email_provider_id: 'ses',
      ...settings
    },
    integrations: integrations ?? [
      {
        id: 'ses',
        name: 'Amazon SES',
        type: 'email',
        email_provider: {
          kind: 'ses',
          senders: [
            { id: 'default', email: 'hello@example.com', name: 'Acme', is_default: true },
            { id: 'orders', email: 'orders@example.com', name: 'Acme Orders', is_default: false }
          ]
        }
      },
      { id: 'supabase-integration-1', name: 'Supabase', type: 'supabase' }
    ]
  }) as unknown as Workspace

const workspace = workspaceWith()

const orderShipped = {
  id: 'tmpl1',
  name: 'Order Shipped',
  version: 2,
  channel: 'email',
  category: 'transactional',
  email: { subject: 'Your order {{order.number}} is on its way', sender_id: 'orders', compiled_preview: '' },
  translations: {
    fr: { email: { subject: 'Votre commande', visual_editor_tree: { id: 'root' } } },
    // A translation started but never written: sends in German fall back to the main language.
    de: { email: { subject: 'Ihre Bestellung' } }
  },
  test_data: { order: { number: 'A-1042', carrier: 'Colissimo' }, contact: { first_name: 'Jane' } },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
} as unknown as Template

const makeNotification = (
  trackingMode?: 'inherit' | 'no_tracking' | 'disabled',
  overrides: Partial<TransactionalNotification> = {}
): TransactionalNotification =>
  ({
    id: 'welcome_email',
    name: 'Welcome Email',
    description: 'Sent when a user signs up',
    channels: { email: { template_id: 'tmpl1' } },
    tracking_settings: {
      enable_tracking: true,
      ...(trackingMode ? { tracking_mode: trackingMode } : {}),
      utm_source: 'example.com',
      utm_medium: 'email',
      utm_campaign: 'transactional',
      utm_content: '',
      // Set through the API, the old form had no field for it
      utm_term: 'spring_sale'
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides
  }) as unknown as TransactionalNotification

const managedNotification = (overrides: Partial<TransactionalNotification> = {}) =>
  makeNotification(undefined, {
    id: 'supabase_recovery_482913',
    name: 'Password Recovery',
    integration_id: 'supabase-integration-1',
    ...overrides
  })

function renderUi(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  const view = render(
    <QueryClientProvider client={queryClient}>
      <AntApp>{ui}</AntApp>
    </QueryClientProvider>
  )
  return { ...view, invalidate }
}

/** The editor opened from outside, as the page opens it. */
function renderEditor(
  props: Partial<Parameters<typeof UpsertTransactionalNotificationDrawer>[0]> = {},
  onClose = vi.fn()
) {
  const view = renderUi(
    <UpsertTransactionalNotificationDrawer workspace={workspace} canReadTemplates open onClose={onClose} {...props} />
  )
  return { ...view, onClose }
}

const openCreate = async (ws: Workspace = workspace) => {
  renderUi(<UpsertTransactionalNotificationDrawer workspace={ws} canReadTemplates buttonContent="New notification" />)
  await userEvent.click(screen.getByRole('button', { name: 'New notification' }))
  await screen.findByText('Not created')
}

const nameInput = () => screen.getByPlaceholderText('E.g. Password Reset Email') as HTMLInputElement
const idInput = () => screen.getByPlaceholderText('E.g. password_reset') as HTMLInputElement
const utm = (field: string) => screen.getByLabelText(field) as HTMLInputElement
const description = () => screen.getByPlaceholderText("A brief description of this notification's purpose")
const check = (key: string) => {
  const item = within(screen.getByTestId('notification-checks'))
    .getAllByRole('listitem')
    .find((li) => li.getAttribute('data-check') === key)
  if (!item) throw new Error(`no check ${key}`)
  return item
}
const createPayload = () => (transactionalNotificationsApi.create as Mock).mock.calls[0][0]
const updatePayload = () => (transactionalNotificationsApi.update as Mock).mock.calls[0][0]

const chooseMode = async (label: string) => {
  await userEvent.click(screen.getByRole('combobox'))
  const options = await screen.findAllByText(label)
  await userEvent.click(options[options.length - 1])
}

beforeEach(() => {
  vi.clearAllMocks()
  window.API_ENDPOINT = 'https://api.example.test'
  ;(transactionalNotificationsApi.create as Mock).mockResolvedValue({ notification: {} })
  ;(transactionalNotificationsApi.update as Mock).mockResolvedValue({ notification: {} })
  // Every identifier typed is free unless a test says otherwise.
  ;(transactionalNotificationsApi.get as Mock).mockRejectedValue(new ApiError('not found', 404))
  ;(templatesApi.get as Mock).mockImplementation(({ id }: { id: string }) =>
    id === 'tmpl1' ? Promise.resolve({ template: orderShipped }) : Promise.reject(new ApiError('not found', 404))
  )
  compile.mockResolvedValue({ html: '<p>Hi</p>', subject: 'Your order A-1042 is on its way' })
})

describe('UpsertTransactionalNotificationDrawer: creating', () => {
  it('sends what was typed, with the UTM defaults, utm_term and no metadata', async () => {
    await openCreate()
    await userEvent.type(nameInput(), 'Order shipped')
    await userEvent.type(description(), 'Sent when the order leaves')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')

    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))

    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalledTimes(1))
    expect(createPayload()).toEqual({
      workspace_id: 'ws1',
      notification: {
        id: 'order_shipped',
        name: 'Order shipped',
        description: 'Sent when the order leaves',
        channels: { email: { template_id: 'tmpl1' } },
        tracking_settings: {
          tracking_mode: 'inherit',
          utm_source: 'example.com',
          utm_medium: 'email',
          utm_campaign: 'transactional',
          utm_content: 'order_shipped',
          utm_term: ''
        }
      }
    })
  })

  it('sends the tracking mode chosen', async () => {
    await openCreate()
    await userEvent.type(nameInput(), 'Welcome Email')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await chooseMode('Disabled for this notification')

    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))

    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalled())
    expect(createPayload().notification.tracking_settings.tracking_mode).toBe('disabled')
  })

  it('says once what an invalid identifier needs, and the checks send to it rather than ask to add it', async () => {
    await openCreate()
    await userEvent.type(nameInput(), 'Order')
    await userEvent.clear(idInput())
    await userEvent.type(idInput(), 'Order-Shipped')

    // The error alone: the hint under the field said the same thing a second time.
    expect(await screen.findByText('ID can only contain lowercase letters, numbers, and underscores')).toBeInTheDocument()
    expect(screen.queryByText('Lowercase letters, numbers and underscores')).toBeNull()
    // Typed, so not "Required … Add".
    expect(within(check('identity')).getByText('The API identifier is not valid')).toBeInTheDocument()
    expect(within(check('identity')).queryByText('Required to create the notification')).toBeNull()
    expect(within(check('identity')).getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  })

  it('drops the template hint once a template is chosen', async () => {
    await openCreate()
    expect(screen.getByText('Transactional templates. Its subject, sender and languages show here once chosen.')).toBeInTheDocument()
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await waitFor(() =>
      expect(screen.queryByText('Transactional templates. Its subject, sender and languages show here once chosen.')).toBeNull()
    )
  })

  it('stops following the name once the identifier is typed in, and utm_content follows the identifier', async () => {
    await openCreate()
    await userEvent.type(nameInput(), 'Order')
    expect(idInput().value).toBe('order')
    expect(utm('utm_content').value).toBe('order')
    expect(screen.getByText('Follows the name until you edit it. Lowercase letters, numbers and underscores')).toBeInTheDocument()

    await userEvent.clear(idInput())
    await userEvent.type(idInput(), 'shipping_update')
    await userEvent.type(nameInput(), ' shipped')

    expect(idInput().value).toBe('shipping_update')
    expect(utm('utm_content').value).toBe('shipping_update')
    expect(screen.getByText('Lowercase letters, numbers and underscores')).toBeInTheDocument()

    // utm_content typed in stops following too
    await userEvent.clear(utm('utm_content'))
    await userEvent.type(utm('utm_content'), 'cta')
    await userEvent.type(idInput(), '_v2')
    expect(utm('utm_content').value).toBe('cta')

    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))
    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalled())
    expect(createPayload().notification).toMatchObject({
      id: 'shipping_update_v2',
      name: 'Order shipped',
      tracking_settings: { utm_content: 'cta' }
    })
  })

  it('limits the name to 255 characters and cuts an identifier filled in from a long name to 32', async () => {
    await openCreate()
    expect(nameInput()).toHaveAttribute('maxlength', '255')
    expect(idInput()).toHaveAttribute('maxlength', '32')

    // its generated identifier's first 32 characters end on an underscore
    await userEvent.type(nameInput(), 'Password reset email for a user list')
    expect(idInput().value).toBe('password_reset_email_for_a_user')
    expect(utm('utm_content').value).toBe('password_reset_email_for_a_user')
  })

  it('says the identifier is free, or already used, once typing stops', async () => {
    ;(transactionalNotificationsApi.get as Mock).mockImplementation(({ id }: { id: string }) =>
      id === 'password_reset'
        ? Promise.resolve({ notification: makeNotification(undefined, { id: 'password_reset' }) })
        : Promise.reject(new ApiError('not found', 404))
    )
    await openCreate()
    await userEvent.type(nameInput(), 'Order shipped')
    expect(await within(check('identity')).findByText('order_shipped is free')).toBeInTheDocument()
    expect(transactionalNotificationsApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'order_shipped' })

    await userEvent.clear(nameInput())
    await userEvent.type(nameInput(), 'Password reset')
    await waitFor(() => expect(check('identity')).toHaveAttribute('data-tone', 'warn'))
    expect(within(check('identity')).getByText('password_reset is already used by another notification')).toBeInTheDocument()
    // Said under the field as well
    expect(screen.getAllByText('password_reset is already used by another notification')).toHaveLength(2)
  })

  it('refuses to create with an identifier already used, and says so on the field', async () => {
    ;(transactionalNotificationsApi.get as Mock).mockImplementation(({ id }: { id: string }) =>
      id === 'password_reset'
        ? Promise.resolve({ notification: makeNotification(undefined, { id: 'password_reset' }) })
        : Promise.reject(new ApiError('not found', 404))
    )
    await openCreate()
    await userEvent.type(nameInput(), 'Password reset')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await waitFor(() => expect(check('identity')).toHaveAttribute('data-tone', 'warn'))

    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))

    expect(await screen.findByText('Please check the form for errors.')).toBeInTheDocument()
    expect(idInput().closest('.ant-form-item')).toHaveClass('ant-form-item-has-error')
    expect(transactionalNotificationsApi.create).not.toHaveBeenCalled()

    // Another identifier, once known free, creates
    await userEvent.type(idInput(), '_v2')
    expect(await within(check('identity')).findByText('password_reset_v2 is free')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))
    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalledTimes(1))
    expect(createPayload().notification.id).toBe('password_reset_v2')
  })

  it('does not refuse an identifier still being checked, nor one whose lookup failed', async () => {
    ;(transactionalNotificationsApi.get as Mock).mockRejectedValue(new ApiError('server error', 500))
    await openCreate()
    await userEvent.type(nameInput(), 'Order shipped')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await waitFor(() => expect(transactionalNotificationsApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'order_shipped' }))
    await waitFor(() => expect(check('identity')).toHaveAttribute('data-tone', 'ok'))
    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))
    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalledTimes(1))
    expect(createPayload().notification.id).toBe('order_shipped')
  })

  it('does not wait for the identifier check to create', async () => {
    ;(transactionalNotificationsApi.get as Mock).mockReturnValue(new Promise(() => {}))
    await openCreate()
    await userEvent.type(nameInput(), 'Order shipped')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await waitFor(() => expect(transactionalNotificationsApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'order_shipped' }))
    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))
    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalledTimes(1))
  })

  it('lists what is missing, then is ready once a template is chosen', async () => {
    await openCreate()
    // A notification goes out through the transactional provider: the window names its sender.
    expect(screen.getByTestId('template-selector').dataset.sendsWith).toBe('"transactional"')
    const checks = screen.getByTestId('notification-checks')
    expect(within(checks).getByText('No email template')).toBeInTheDocument()
    expect(check('template')).toHaveAttribute('data-tone', 'warn')
    expect(check('identity')).toHaveAttribute('data-tone', 'warn')
    expect(screen.getByText('2 missing')).toBeInTheDocument()
    // Without a template: the provider and its default sender
    expect(within(check('sender')).getByText('Transactional provider')).toBeInTheDocument()
    expect(within(check('sender')).getByText('Amazon SES · default sender hello@example.com')).toBeInTheDocument()

    await userEvent.type(nameInput(), 'Order shipped')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')

    expect(await within(check('template')).findByText('Order Shipped · Transactional')).toBeInTheDocument()
    expect(check('template')).toHaveAttribute('data-tone', 'ok')
    expect(within(check('sender')).getByText("orders@example.com, the template's, on Amazon SES")).toBeInTheDocument()
    expect(screen.getByText('Ready')).toBeInTheDocument()
  })

  it('says a send cannot work without a transactional provider', async () => {
    await openCreate(workspaceWith({ transactional_email_provider_id: undefined }))
    expect(within(check('sender')).getByText('No transactional provider')).toBeInTheDocument()
    expect(check('sender')).toHaveAttribute('data-tone', 'warn')
    expect(within(check('sender')).getByRole('link', { name: 'Settings' })).toHaveAttribute(
      'href',
      '/console/workspace/ws1/settings/integrations'
    )
  })

  it('summarizes the chosen template and fills the API call with its data', async () => {
    await openCreate()
    expect(screen.getByTestId('api-call-code').textContent).toContain('your_template_variable')
    expect(screen.getByText('Works once created')).toBeInTheDocument()

    await userEvent.type(nameInput(), 'Order shipped')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')

    const summary = await screen.findByTestId('template-summary')
    expect(within(summary).getByText('Your order {{order.number}} is on its way')).toBeInTheDocument()
    expect(within(summary).getByText('Acme Orders · orders@example.com')).toBeInTheDocument()
    expect(within(summary).getByText('English (default), French')).toBeInTheDocument()
    expect(within(summary).queryByText(/German/)).toBeNull()
    // What every send fills in (contact) is left out
    expect(within(summary).getByText('order.number')).toBeInTheDocument()
    expect(within(summary).getByText('order.carrier')).toBeInTheDocument()
    expect(within(summary).queryByText('contact.first_name')).toBeNull()

    const code = screen.getByTestId('api-call-code').textContent ?? ''
    expect(code).toContain('"id": "order_shipped"')
    expect(code).toContain('"number": "A-1042"')
    expect(code).not.toContain('your_template_variable')
    expect(code).not.toContain('"first_name": "Jane"')
  })
})

describe('UpsertTransactionalNotificationDrawer: editing', () => {
  it('saves nothing new until a field changes, then sends the notification', async () => {
    renderEditor({ notification: makeNotification('no_tracking') })
    expect(await screen.findByDisplayValue('Welcome Email')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
    expect(screen.queryByText('Unsaved changes')).toBeNull()

    await userEvent.type(nameInput(), '!')
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalledTimes(1))
    expect(updatePayload()).toEqual({
      workspace_id: 'ws1',
      id: 'welcome_email',
      updates: {
        name: 'Welcome Email!',
        description: 'Sent when a user signs up',
        channels: { email: { template_id: 'tmpl1' } },
        tracking_settings: {
          enable_tracking: true,
          tracking_mode: 'no_tracking',
          utm_source: 'example.com',
          utm_medium: 'email',
          utm_campaign: 'transactional',
          utm_content: '',
          utm_term: 'spring_sale'
        }
      }
    })
    expect('metadata' in updatePayload().updates).toBe(false)
  })

  it('keeps the utm_term set through the API, shows it, and keeps tracking fields it has no input for', async () => {
    const notification = makeNotification('inherit')
    notification.tracking_settings.endpoint = 'https://track.example.com'
    renderEditor({ notification })
    expect(await screen.findByDisplayValue('spring_sale')).toBe(utm('utm_term'))

    await userEvent.type(utm('utm_campaign'), '_2026')
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalled())
    expect(updatePayload().updates.tracking_settings).toMatchObject({
      utm_campaign: 'transactional_2026',
      utm_term: 'spring_sale',
      endpoint: 'https://track.example.com'
    })
  })

  it('sends an emptied description, which clears it', async () => {
    renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Sent when a user signs up')

    await userEvent.clear(description())
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalled())
    expect(updatePayload().updates.description).toBe('')
  })

  it('locks the identifier with its reason and copies it', async () => {
    renderEditor({ notification: makeNotification() })
    const id = await screen.findByLabelText('API identifier')
    expect(id).toBeDisabled()
    expect(id).toHaveValue('welcome_email')
    expect(screen.getByText('Fixed once created: API calls send it')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy ID' })).toBeInTheDocument()
  })

  it('reads a stored mode-less notification as following the workspace, and resets an opt-out to inherit', async () => {
    renderEditor({ notification: makeNotification('disabled') })
    await screen.findByDisplayValue('Welcome Email')
    expect(screen.getByText('Disabled for this notification')).toBeInTheDocument()

    await chooseMode('Follow workspace setting')
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalled())
    expect(updatePayload().updates.tracking_settings.tracking_mode).toBe('inherit')
  })

  it('shows an absent stored mode as « Follow workspace setting »', async () => {
    renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    expect(screen.getByText('Follow workspace setting')).toBeInTheDocument()
  })

  it('says a change of tracking applies to the next sends, and Discard puts it back', async () => {
    renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    expect(screen.getByText('Sending')).toBeInTheDocument()
    expect(screen.queryByText('Changed: Tracking')).toBeNull()

    await chooseMode('No open or click tracking, keep UTM parameters')
    expect(screen.getByText('Changed: Tracking')).toBeInTheDocument()
    expect(check('changed-tracking')).toHaveAttribute('data-tone', 'info')

    await userEvent.click(screen.getByRole('button', { name: 'Discard' }))
    expect(screen.queryByText('Changed: Tracking')).toBeNull()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })

  it('clearing the template empties the draft: no summary, a missing template, and Save refuses with the field error', async () => {
    renderEditor({ notification: makeNotification() })
    await screen.findByTestId('template-summary')
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()

    // The selector's clear cross sends null
    await userEvent.clear(screen.getByTestId('template-selector'))

    await waitFor(() => expect(screen.queryByTestId('template-summary')).toBeNull())
    expect(check('template')).toHaveAttribute('data-tone', 'warn')
    expect(within(check('template')).getByText('No email template')).toBeInTheDocument()
    expect(within(check('template')).getByText("Emails can't be sent until you choose one")).toBeInTheDocument()
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()

    const save = screen.getByRole('button', { name: 'Save changes' })
    expect(save).toBeEnabled()
    await userEvent.click(save)
    expect(await screen.findByText('Please select an email template')).toBeInTheDocument()
    expect(transactionalNotificationsApi.update).not.toHaveBeenCalled()
  })

  it('warns that the template was deleted and still sends', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: { ...orderShipped, deleted_at: '2026-02-01T00:00:00Z' } })
    renderEditor({ notification: makeNotification() })

    const warning = await screen.findByTestId('deleted-template-warning')
    expect(within(warning).getByText('Deleted template')).toBeInTheDocument()
    expect(
      within(warning).getByText(
        'This notification still sends with it. Choose another template to stop depending on a deleted one.'
      )
    ).toBeInTheDocument()
    expect(check('template')).toHaveAttribute('data-tone', 'warn')
  })

  it('goes back to where it was opened from', async () => {
    renderEditor({ notification: makeNotification(), backLabel: 'Welcome Email' })
    expect(await screen.findByRole('button', { name: 'Welcome Email' })).toBeInTheDocument()
  })

  it('shows no API call: the sheet has it', async () => {
    renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    expect(screen.queryByTestId('api-call-code')).toBeNull()
  })
})

describe('UpsertTransactionalNotificationDrawer: managed by an integration', () => {
  it('locks the name and the template with the reason under each, and omits channels from the update', async () => {
    renderEditor({ notification: managedNotification() })
    await screen.findByDisplayValue('Password Recovery')

    expect(screen.getByText('Managed by Supabase (supabase integration)')).toBeInTheDocument()
    expect(nameInput()).toBeDisabled()
    expect(screen.getByText('Set by Supabase')).toBeInTheDocument()
    expect(screen.getByTestId('template-selector')).toBeDisabled()
    expect(
      screen.getByText('Set by Supabase. Edit the template itself to change its content.')
    ).toBeInTheDocument()

    await chooseMode('Disabled for this notification')
    expect(check('tracking')).toHaveAttribute('data-tone', 'off')
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalled())
    const { updates } = updatePayload()
    expect(updates.channels).toBeUndefined()
    // The unchanged name rides along, which the server accepts
    expect(updates.name).toBe('Password Recovery')
    expect(updates.tracking_settings.tracking_mode).toBe('disabled')
  })

  it('without Templates read, says why the template shows its ID only', async () => {
    renderEditor({ notification: managedNotification(), canReadTemplates: false })
    await screen.findByDisplayValue('Password Recovery')

    expect(screen.getByTestId('template-selector')).toHaveAttribute('data-can-read-templates', 'false')
    expect(
      screen.getByText("You need Templates read permission to see this template's name and preview")
    ).toBeInTheDocument()
    expect(templatesApi.get).not.toHaveBeenCalled()
    expect(within(check('template')).getByText('tmpl1')).toBeInTheDocument()
  })

  it('gives the selector Templates read and no permission hint when the member has it', async () => {
    renderEditor({ notification: managedNotification() })
    await screen.findByDisplayValue('Password Recovery')
    expect(screen.getByTestId('template-selector')).toHaveAttribute('data-can-read-templates', 'true')
    expect(screen.queryByText("You need Templates read permission to see this template's name and preview")).toBeNull()
  })
})

describe('UpsertTransactionalNotificationDrawer: tracking effect', () => {
  const pills = () => within(screen.getByTestId('tracking-effect')).getAllByText(/./).map((node) => node.textContent)

  it('follows the workspace, and says what it turned off', async () => {
    renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    expect(pills()).toEqual(['Opens on', 'Clicks on', 'UTM added'])
  })

  it('names the workspace when it turned tracking off', async () => {
    renderEditor({
      notification: makeNotification(),
      workspace: workspaceWith({ email_tracking: { opens_enabled: false, clicks_enabled: true } })
    })
    await screen.findByDisplayValue('Welcome Email')
    expect(pills()).toEqual(['Opens off (workspace)', 'Clicks on', 'UTM added'])
  })

  it('colours "UTM added" the same in every mode, and says what each mode does', async () => {
    // The pill was grey following the workspace and green without tracking; the help under the
    // mode said the same thing whatever the mode.
    renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    const utmPill = () => within(screen.getByTestId('tracking-effect')).getByText('UTM added').closest('span[class]')!
    const followingClass = utmPill().className
    expect(screen.getByText("Following the workspace setting also respects each contact's tracking consent.")).toBeInTheDocument()

    await chooseMode('No open or click tracking, keep UTM parameters')
    expect(utmPill().className).toBe(followingClass)
    expect(screen.getByText('Opens and clicks are not tracked; links keep their UTM parameters.')).toBeInTheDocument()

    await chooseMode('Disabled for this notification')
    expect(screen.getByText('Links are left as written: no tracking and no UTM parameters.')).toBeInTheDocument()
  })

  it('keeps UTM parameters without opens and clicks, and greys them out when disabled', async () => {
    renderEditor({ notification: makeNotification('no_tracking') })
    await screen.findByDisplayValue('Welcome Email')
    expect(pills()).toEqual(['Opens off', 'Clicks off', 'UTM added'])
    expect(utm('utm_source')).toBeEnabled()

    await chooseMode('Disabled for this notification')
    expect(pills()).toEqual(['Opens off', 'Clicks off', 'No UTM'])
    expect(utm('utm_source')).toBeDisabled()
    expect(utm('utm_term')).toBeDisabled()
    // Kept, not cleared
    expect(utm('utm_source').value).toBe('example.com')
    expect(screen.getByText(/Kept for when you turn it back on/)).toBeInTheDocument()
  })

  it('says an empty utm_content falls back to the template ID', async () => {
    renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    expect(utm('utm_content')).toHaveAttribute('placeholder', 'Defaults to the template ID')
  })
})

describe('UpsertTransactionalNotificationDrawer: opening and closing', () => {
  it('passes its button props, the icon included, to its own button', () => {
    renderUi(
      <UpsertTransactionalNotificationDrawer
        workspace={workspace}
        canReadTemplates
        buttonContent="New notification"
        buttonProps={{ type: 'primary', icon: <Plus size={14} aria-hidden="true" /> }}
      />
    )
    const button = screen.getByRole('button', { name: 'New notification' })
    expect(button).toHaveClass('ant-btn-primary')
    expect(button.querySelector('svg.lucide-plus')).not.toBeNull()
  })

  it('renders no button of its own and stays closed while not open', () => {
    renderUi(
      <UpsertTransactionalNotificationDrawer
        workspace={workspace}
        canReadTemplates
        notification={makeNotification()}
        open={false}
        onClose={vi.fn()}
      />
    )
    expect(screen.queryByRole('button', { name: /Edit Notification/i })).not.toBeInTheDocument()
    expect(screen.queryByText('Edit notification')).not.toBeInTheDocument()
  })

  it('closes without asking when nothing changed', async () => {
    const { onClose } = renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('asks before closing over unsaved changes', async () => {
    const { onClose } = renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    await userEvent.type(nameInput(), '!')

    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    // The template editor's question and buttons, not "close this drawer" with No and Yes.
    expect(await screen.findByText('Discard your changes?', { selector: '.ant-modal-confirm-title' })).toBeInTheDocument()
    expect(screen.getByText('The changes you made since opening this notification will be lost.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Yes' })).toBeNull()
    expect(onClose).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    expect(onClose).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    await userEvent.click((await screen.findAllByRole('button', { name: 'Discard' })).at(-1)!)
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  })

  it('does not close on Escape', async () => {
    const { onClose } = renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    fireEvent.keyDown(nameInput(), { key: 'Escape', code: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('refreshes the list and the open sheet after saving, then closes', async () => {
    const { invalidate, onClose } = renderEditor({ notification: makeNotification() })
    await screen.findByDisplayValue('Welcome Email')
    await userEvent.type(nameInput(), '!')

    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['transactional-notifications', 'ws1'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['transactional-notification', 'ws1', 'welcome_email'] })
  })

  it('keeps the editor open and says why when the save fails', async () => {
    ;(transactionalNotificationsApi.create as Mock).mockRejectedValue(new Error('id already exists'))
    await openCreate()
    await userEvent.type(nameInput(), 'Order shipped')
    await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))

    expect(await screen.findByText('Failed to create notification: id already exists')).toBeInTheDocument()
    expect(screen.getByText('Not created')).toBeInTheDocument()
  })

  it('refuses to create without a template', async () => {
    await openCreate()
    await userEvent.type(nameInput(), 'Order shipped')
    await userEvent.click(screen.getByRole('button', { name: 'Create notification' }))

    expect(await screen.findByText('Please select an email template')).toBeInTheDocument()
    expect(transactionalNotificationsApi.create).not.toHaveBeenCalled()
  })
})
