import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { MessageHistory } from '../../services/api/messages_history'
import { ApiError } from '../../services/api/errors'

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  listMessages: vi.fn(),
  getTemplate: vi.fn(),
  preview: vi.fn(),
  permissions: {} as Record<string, { read: boolean; write: boolean }>
}))

vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [
      {
        id: 'ws1',
        settings: {
          timezone: 'UTC',
          default_language: 'en',
          marketing_email_provider_id: 'm',
          transactional_email_provider_id: 't'
        },
        integrations: [
          {
            id: 'm',
            email_provider: {
              kind: 'ses',
              senders: [{ id: 's9', name: 'News', email: 'news@acme.io', is_default: true }]
            }
          },
          {
            id: 't',
            email_provider: {
              kind: 'postmark',
              senders: [
                { id: 's1', name: 'Acme', email: 'hello@acme.io', is_default: false },
                { id: 's2', name: 'Default', email: 'default@acme.io', is_default: true }
              ]
            }
          }
        ]
      }
    ]
  }),
  useWorkspacePermissions: () => ({ permissions: state.permissions, loading: false })
}))
vi.mock('../../services/api/messages_history', () => ({
  listMessages: state.listMessages
}))
vi.mock('../../services/api/template', () => ({
  templatesApi: { get: state.getTemplate }
}))
vi.mock('../templates/compileTemplatePreview', () => ({
  compileTemplatePreview: state.preview
}))

import { MessageDetail, type MessageNames } from './MessageDetail'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const message = (overrides: Partial<MessageHistory> = {}): MessageHistory => ({
  id: 'm1',
  external_id: 'order-42',
  contact_email: 'ann@example.com',
  template_id: 'tpl_welcome',
  template_version: 3,
  channel: 'email',
  message_data: { data: { name: 'Ann' }, metadata: { order: 42 } },
  sent_at: '2026-11-12T09:00:00Z',
  created_at: '2026-11-12T09:00:00Z',
  updated_at: '2026-11-12T09:00:00Z',
  ...overrides
})

const template = (overrides: Record<string, unknown> = {}) => ({
  id: 'tpl_welcome',
  name: 'Welcome',
  version: 3,
  category: 'transactional',
  email: { subject: 'Hello {{ name }}', subject_preview: 'Start here', sender_id: 's1' },
  ...overrides
})

const allRead = {
  templates: { read: true, write: false },
  contacts: { read: true, write: false },
  broadcasts: { read: true, write: false },
  automations: { read: true, write: false },
  transactional: { read: true, write: false }
}

const names: MessageNames = {
  broadcast: (id) => (id === 'b1' ? 'November newsletter' : undefined),
  automation: (id) => (id === 'a1' ? 'Onboarding' : undefined),
  list: (id) => (id === 'l1' ? 'Customers' : undefined)
}

const compiled = {
  html: '<p>Hello Ann</p>',
  mjml: '<mjml>Hello</mjml>',
  text: 'Hello Ann',
  subject: 'Hello Ann',
  subject_preview: 'Start here'
}

interface DetailProps {
  id?: string
  row?: MessageHistory
  onClose: () => void
  onNext: () => void
  onPrev: () => void
}

const renderDetail = (props: Partial<DetailProps> = {}) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const makeUi = (current: Partial<DetailProps>) => (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <MessageDetail
              workspaceId="ws1"
              permissions={state.permissions as never}
              id={'id' in current ? current.id : 'm1'}
              row={current.row}
              names={names}
              nav={{ position: 1, total: 2, onNext: current.onNext, onPrev: current.onPrev }}
              onClose={current.onClose ?? vi.fn()}
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
  const view = render(makeUi(props))
  // Another message in the same panel, as J and K open it: the tab passes its row.
  const show = (id: string, row?: MessageHistory) => view.rerender(makeUi({ ...props, id, row }))
  return { ...view, show }
}

const value = (label: string) => {
  const term = screen.getByText(label, { selector: 'dt' })
  return term.nextElementSibling as HTMLElement
}

describe('MessageDetail', () => {
  beforeEach(() => {
    state.navigate.mockClear()
    state.permissions = allRead
    state.listMessages.mockReset()
    state.listMessages.mockResolvedValue({ messages: [message()], has_more: false })
    state.getTemplate.mockReset()
    state.getTemplate.mockResolvedValue({ template: template() })
    state.preview.mockReset()
    state.preview.mockResolvedValue(compiled)
  })

  it('loads the message by its id, without a period', async () => {
    renderDetail()
    expect(await screen.findByText('To ann@example.com')).toBeInTheDocument()
    expect(state.listMessages).toHaveBeenCalledWith('ws1', { id: 'm1', limit: 1 })
    expect(state.getTemplate).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      id: 'tpl_welcome',
      version: 3
    })
  })

  it('says a message is not found, with a way out', async () => {
    state.listMessages.mockResolvedValue({ messages: [], has_more: false })
    const onClose = vi.fn()
    renderDetail({ onClose })
    expect(await screen.findByText('Message not found')).toBeInTheDocument()
    // The panel has its own close button too: this is the one in the empty state.
    const empty = screen.getByText('Message not found').closest('.ant-empty') as HTMLElement
    await userEvent.click(within(empty).getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalled()
  })

  it('goes to the next message with J and the previous one with K', async () => {
    const onNext = vi.fn()
    const onPrev = vi.fn()
    renderDetail({ onNext, onPrev })
    await screen.findByText('To ann@example.com')
    fireEvent.keyDown(document, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(document, { key: 'k' })
    expect(onPrev).toHaveBeenCalledTimes(1)
  })

  it('keeps the panel when another message opens, back on its Events tab', async () => {
    state.listMessages.mockImplementation(async (_ws: string, params: { id: string }) => ({
      messages: [message({ id: params.id, contact_email: `${params.id}@example.com` })],
      has_more: false
    }))
    const { show } = renderDetail()
    await screen.findByText('To m1@example.com')
    const drawer = document.querySelector('.ant-drawer')
    await userEvent.click(screen.getByRole('tab', { name: 'Data' }))
    show('m2', message({ id: 'm2', contact_email: 'm2@example.com' }))
    expect(await screen.findByText('To m2@example.com')).toBeInTheDocument()
    expect(document.querySelector('.ant-drawer')).toBe(drawer)
    expect(screen.getByRole('tab', { name: 'Events' })).toHaveAttribute('aria-selected', 'true')
  })

  it('says when the message cannot be loaded, with a retry', async () => {
    state.listMessages.mockRejectedValueOnce(new Error('database is down'))
    renderDetail()
    expect(await screen.findByText("Couldn't load the message")).toBeInTheDocument()
    expect(screen.getByText('database is down')).toBeInTheDocument()
    expect(screen.queryByText('Message not found')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('To ann@example.com')).toBeInTheDocument()
  })

  it('says when the template cannot be loaded for another reason than being gone', async () => {
    state.getTemplate.mockRejectedValue(new Error('server error'))
    renderDetail()
    await screen.findByText('To ann@example.com')
    await userEvent.click(screen.getByRole('tab', { name: 'Sent content' }))
    expect(await screen.findByText("Couldn't load the template")).toBeInTheDocument()
    expect(screen.getByText('server error')).toBeInTheDocument()
  })

  it('takes the sender of the provider the message went through', async () => {
    // A newsletter goes through the marketing provider, whatever is known of its template.
    state.permissions = { ...allRead, templates: { read: false, write: false } }
    state.listMessages.mockResolvedValue({
      messages: [message({ broadcast_id: 'b1' })],
      has_more: false
    })
    const { unmount } = renderDetail()
    await screen.findByText('To ann@example.com')
    expect(value('From')).toHaveTextContent('News <news@acme.io>')
    unmount()

    // A double opt-in confirmation too: no origin, and an opt-in template.
    state.permissions = allRead
    state.getTemplate.mockResolvedValue({ template: template({ category: 'opt_in', email: {} }) })
    state.listMessages.mockResolvedValue({ messages: [message()], has_more: false })
    renderDetail()
    await screen.findByText('To ann@example.com')
    await waitFor(() => expect(value('From')).toHaveTextContent('News <news@acme.io>'))
  })

  it('lists the events in time order, without a line for the send', async () => {
    state.listMessages.mockResolvedValue({
      messages: [
        message({
          broadcast_id: 'b1',
          delivered_at: '2026-11-12T09:00:02Z',
          clicked_at: '2026-11-12T09:30:00Z',
          opened_at: '2026-11-12T09:10:00Z',
          bounced_at: '2026-11-13T08:00:00Z',
          status_info: 'Mailbox full'
        })
      ],
      has_more: false
    })
    renderDetail()
    await screen.findByText('Sent by the newsletter November newsletter')
    await waitFor(() => expect(screen.getByText('Template Welcome · v3')).toBeInTheDocument())
    const order = Array.from(document.querySelectorAll('[data-event]')).map((item) =>
      item.getAttribute('data-event')
    )
    expect(order).toEqual(['created', 'delivered', 'opened', 'clicked', 'bounced'])
    expect(screen.getByText('First open')).toBeInTheDocument()
    expect(screen.getByText('Mailbox full')).toBeInTheDocument()
    // Another day than the send: the date goes with the time.
    expect(screen.getByText('13 Nov, 08:00:00')).toBeInTheDocument()
  })

  it('names a deleted template as such', async () => {
    state.getTemplate.mockResolvedValue({
      template: template({ deleted_at: '2026-11-01T00:00:00Z' })
    })
    renderDetail()
    expect(await screen.findByText('Welcome · deleted')).toBeInTheDocument()
  })

  it('shows the template id and no subject without templates read', async () => {
    state.permissions = { ...allRead, templates: { read: false, write: false } }
    renderDetail()
    await screen.findByText('To ann@example.com')
    expect(value('Template').querySelector('code')).toHaveTextContent('tpl_welcome')
    expect(screen.queryByText('Subject', { selector: 'dt' })).toBeNull()
    expect(state.getTemplate).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('tab', { name: 'Sent content' }))
    expect(screen.getByText('You need access to templates')).toBeInTheDocument()
  })

  it('says when the template behind the message is gone', async () => {
    state.getTemplate.mockRejectedValue(new ApiError('not found', 404))
    renderDetail()
    await screen.findByText('To ann@example.com')
    await userEvent.click(screen.getByRole('tab', { name: 'Sent content' }))
    expect(
      await screen.findByText("The template behind this message isn't available")
    ).toBeInTheDocument()
  })

  it('shows an erased contact: no contact to open, no content, no data', async () => {
    state.listMessages.mockResolvedValue({
      // The server answers an erased message's data as null.
      messages: [
        message({ contact_email: 'DELETED_EMAIL', message_data: { data: null as never } })
      ],
      has_more: false
    })
    renderDetail()
    expect(await screen.findByText('Contact erased')).toBeInTheDocument()
    expect(screen.queryByText('To DELETED_EMAIL')).toBeNull()
    expect(screen.getByRole('button', { name: /Open contact/ })).toBeDisabled()
    await userEvent.click(screen.getByRole('tab', { name: 'Sent content' }))
    expect(screen.getByText('The contact was erased; the message data is gone')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: 'Data' }))
    expect(document.querySelector('.ant-empty-description')).toHaveTextContent('No data')
  })

  it('opens the contact, which needs contacts read', async () => {
    const { unmount } = renderDetail()
    await userEvent.click(await screen.findByRole('button', { name: /Open contact/ }))
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/console/workspace/$workspaceId/contacts',
        search: { contact: 'ann@example.com' }
      })
    )
    unmount()
    state.permissions = { ...allRead, contacts: { read: false, write: false } }
    renderDetail()
    expect(await screen.findByRole('button', { name: /Open contact/ })).toBeDisabled()
  })

  it('links to what sent the message, as text without its read right', async () => {
    state.listMessages.mockResolvedValue({
      messages: [message({ automation_id: 'a1' })],
      has_more: false
    })
    const { unmount } = renderDetail()
    await screen.findByText('Sent by the automation Onboarding')
    await userEvent.click(screen.getByRole('link', { name: 'Onboarding' }))
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/console/workspace/$workspaceId/automations',
        search: { automation: 'a1' }
      })
    )
    unmount()
    state.permissions = { ...allRead, automations: { read: false, write: false } }
    renderDetail()
    await screen.findByText('Sent by the automation Onboarding')
    expect(screen.queryByRole('link', { name: 'Onboarding' })).toBeNull()
    expect(value('Sent by')).toHaveTextContent('Onboarding')
  })

  it('shows a subject forced at send time as stored', async () => {
    state.listMessages.mockResolvedValue({
      messages: [
        message({
          transactional_notification_id: 'reset',
          channel_options: { subject: 'Reset for {{ name }}', from_name: 'Support' }
        })
      ],
      has_more: false
    })
    renderDetail()
    await screen.findByText('To ann@example.com')
    await waitFor(() => expect(value('Subject')).toHaveTextContent('Reset for {{ name }}'))
    expect(value('Subject')).toHaveTextContent('Set at send time')
    expect(value('From')).toHaveTextContent('Support <hello@acme.io>')
  })

  it('takes the template subject and the sender of the template otherwise', async () => {
    renderDetail()
    await screen.findByText('To ann@example.com')
    await waitFor(() => expect(value('Subject')).toHaveTextContent('Hello {{ name }}'))
    expect(value('From')).toHaveTextContent('Acme <hello@acme.io>')
  })

  it('counts a message without tracking fields as tracked', async () => {
    const { unmount } = renderDetail()
    await screen.findByText('To ann@example.com')
    expect(value('Tracking')).toHaveTextContent('On (default)')
    unmount()
    state.listMessages.mockResolvedValue({
      messages: [message({ opens_tracked: true, clicks_tracked: false })],
      has_more: false
    })
    renderDetail()
    await screen.findByText('To ann@example.com')
    expect(value('Tracking')).toHaveTextContent('Opens on · clicks off')
  })

  it('renders the content sent again, with its HTML, text and MJML', async () => {
    renderDetail()
    await screen.findByText('To ann@example.com')
    await waitFor(() => expect(state.getTemplate).toHaveBeenCalled())
    expect(state.preview).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('tab', { name: 'Sent content' }))
    expect(await screen.findByTitle('Sent content')).toHaveAttribute('srcdoc', '<p>Hello Ann</p>')
    // Compiled with the message's own data.
    expect(state.preview).toHaveBeenCalledWith(
      expect.objectContaining({
        email: expect.objectContaining({ subject: 'Hello {{ name }}' }),
        // The recorded open pixel is blanked: viewing must not record an open.
        testData: { name: 'Ann', tracking_opens_url: '' }
      })
    )
    expect(
      screen.getByText(
        "Rendered again from the template and the message data; Notifuse doesn't store the sent HTML"
      )
    ).toBeInTheDocument()
    await userEvent.click(screen.getByText('Text version'))
    expect(screen.getByText('Hello Ann', { selector: 'pre' })).toBeInTheDocument()
    await userEvent.click(screen.getByText('MJML Source'))
    expect(screen.getByText('<mjml>Hello</mjml>', { selector: 'pre' })).toBeInTheDocument()
    // Once compiled, the subject is the rendered one.
    await userEvent.click(screen.getByRole('tab', { name: 'Events' }))
    expect(value('Subject')).toHaveTextContent('Hello Ann')
  })

  it('renders a message sent in a translation in that language', async () => {
    state.listMessages.mockResolvedValue({
      messages: [message({ message_data: { data: { name: 'Léa', contact: { language: 'fr' } } } })],
      has_more: false
    })
    state.getTemplate.mockResolvedValue({
      template: template({
        translations: { fr: { email: { subject: 'Bonjour {{ name }}', sender_id: '' } } }
      })
    })
    renderDetail()
    await screen.findByText('To ann@example.com')
    await waitFor(() => expect(value('Subject')).toHaveTextContent('Bonjour {{ name }}'))
    expect(value('Language')).toHaveTextContent('French')
    // The translation leaves its sender empty: the template's.
    expect(value('From')).toHaveTextContent('Acme <hello@acme.io>')
    await userEvent.click(screen.getByRole('tab', { name: 'Sent content' }))
    await waitFor(() =>
      expect(state.preview).toHaveBeenCalledWith(
        expect.objectContaining({
          email: expect.objectContaining({ subject: 'Bonjour {{ name }}' })
        })
      )
    )
  })

  it('says when the content cannot be rendered', async () => {
    state.preview.mockResolvedValue({
      html: '',
      mjml: '',
      error: { message: 'Unclosed tag', details: [] }
    })
    renderDetail()
    await screen.findByText('To ann@example.com')
    await waitFor(() => expect(state.getTemplate).toHaveBeenCalled())
    await userEvent.click(screen.getByRole('tab', { name: 'Sent content' }))
    expect(await screen.findByText("Couldn't render this message")).toBeInTheDocument()
    expect(screen.getByText('Unclosed tag')).toBeInTheDocument()
  })

  it('shows the template data and the metadata', async () => {
    renderDetail()
    await screen.findByText('To ann@example.com')
    await userEvent.click(screen.getByRole('tab', { name: 'Data' }))
    expect(screen.getByText('Template data')).toBeInTheDocument()
    expect(screen.getByText(/"name": "Ann"/)).toBeInTheDocument()
    expect(screen.getByText(/"order": 42/)).toBeInTheDocument()
  })
})
