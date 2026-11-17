import type React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import enUS from 'antd/locale/en_US'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { Integrations } from './Integrations'
import { LicenseContext, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import type { Integration, Workspace } from '../../services/api/types'
import { api } from '../../services/api/client'

// Every form of Settings › Integrations, seen from the API client: what each one renders, and the
// exact body it posts. The client is the only thing mocked, so the services between the form and
// the request (workspaceService, emailService, ses, lists, webhook status) all run for real.
vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const sender = { id: 'sender_1', email: 'news@acme.com', name: 'Acme', is_default: true }

const email = (id: string, name: string, provider: Record<string, unknown>, extra: Partial<Integration> = {}) =>
  ({
    id,
    name,
    type: 'email',
    email_provider: { senders: [sender], rate_limit_per_minute: 50, ...provider },
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-01T10:00:00Z',
    ...extra
  }) as unknown as Integration

const sesIntegration = email(
  'int_ses',
  'Amazon SES',
  {
    kind: 'ses',
    ses: {
      region: 'eu-west-1',
      access_key: 'AKIAIOSFODNN7EXAMPLE',
      encrypted_secret_key: 'enc-secret',
      tenant_name: 'my-tenant',
      managed_configuration_set: 'notifuse-int_ses',
      inbound_topic_arn: 'arn:aws:sns:eu-west-1:123:replies'
    }
  },
  { credential_hints: { 'ses.secret_key': 'K7MD' } }
)

const mailjetIntegration = email('int_mailjet', 'Mailjet', {
  kind: 'mailjet',
  mailjet: { sandbox_mode: false, encrypted_api_key: 'a', encrypted_secret_key: 'b' }
})

const workspaceWith = (integrations: Integration[], settings: Record<string, unknown> = {}) =>
  ({
    id: 'ws1',
    name: 'Acme',
    settings: { custom_field_labels: { custom_json_1: 'Supabase profile' }, ...settings },
    integrations
  }) as unknown as Workspace

let current: Workspace

const routeGet = async (path: string) => {
  if (path.startsWith('/api/workspaces.get')) return { workspace: current }
  if (path.startsWith('/api/lists.list'))
    return { lists: [{ id: 'list_news', name: 'Newsletter' }, { id: 'list_product', name: 'Product updates' }] }
  if (path.startsWith('/api/webhooks.status'))
    return { status: { email_provider_kind: 'ses', is_registered: true, endpoints: [] } }
  return {}
}

const routePost = async (path: string) => {
  if (path === '/api/workspaces.createIntegration') return { integration_id: 'int_new' }
  if (path === '/api/ses.listTenants') return { tenants: [], has_more: false }
  if (path === '/api/ses.listConfigurationSets') return { configuration_sets: [], has_more: false }
  if (path === '/api/email.testProvider') return { success: true }
  if (path === '/api/workspaces.connectZapier')
    return { status: 'success', token: 'tok_once', email: 'zapier-sales-1a2b3c4d@api.acme.com', integration_id: 'int_z' }
  return {}
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.get).mockImplementation(routeGet as typeof api.get)
  vi.mocked(api.post).mockImplementation(routePost as typeof api.post)
})

const renderPage = (integrations: Integration[] = [], settings: Record<string, unknown> = {}, isOwner = true) => {
  current = workspaceWith(integrations, settings)
  let rerender: (ui: React.ReactElement) => void = () => {}
  const tree = (workspace: Workspace) => (
    <I18nProvider i18n={i18n}>
      {/* The antd locale App.tsx sets: its required message names the field ("Please enter …"). */}
      <ConfigProvider locale={enUS}>
        <App>
          <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: null }}>
            <Integrations workspace={workspace} onSave={onSave} loading={false} isOwner={isOwner} />
          </LicenseContext.Provider>
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  const onSave = vi.fn(async (workspace: Workspace) => rerender(tree(workspace)))
  const utils = render(tree(current))
  rerender = utils.rerender
  return utils
}

/** The bodies posted to one endpoint, in order. */
const posted = (path: string) =>
  vi
    .mocked(api.post)
    .mock.calls.filter(([p]) => p === path)
    .map(([, body]) => body as Record<string, unknown>)

const openFromCatalogue = async (entry: string) => {
  fireEvent.click((await screen.findAllByRole('button', { name: /Add integration/ }))[0])
  const catalogue = await screen.findByRole('dialog', { name: 'Add integration' })
  fireEvent.click(within(catalogue).getByRole('button', { name: new RegExp(`^${entry}`) }))
  await waitFor(() => expect(document.querySelector('[data-drawer-title]')).not.toBeNull())
}

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } })

// The drawer bar's Save: the sender modal has one of its own, still in the DOM once closed.
const save = () =>
  fireEvent.click(within(document.querySelector('[data-drawer-bar]') as HTMLElement).getByRole('button', { name: 'Save' }))

// Adds a sender through its modal, the only way the form takes one.
const addSender = async (address: string, name: string) => {
  fireEvent.click(screen.getByRole('button', { name: /Add sender/ }))
  const modal = await screen.findByRole('dialog', { name: 'Add sender' })
  fireEvent.change(within(modal).getByLabelText('Email'), { target: { value: address } })
  fireEvent.change(within(modal).getByLabelText('Name'), { target: { value: name } })
  fireEvent.click(within(modal).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(document.querySelector('[data-senders]')).toHaveTextContent(address))
}

const pickOption = async (label: string, search: string, title: string) => {
  const input = screen.getByLabelText(label)
  fireEvent.mouseDown(input)
  if (search) fireEvent.change(input, { target: { value: search } })
  fireEvent.click(await screen.findByTitle(title))
}

const expectRequiredError = async (message: RegExp | string) =>
  expect(await screen.findByText(message)).toBeInTheDocument()

describe('email providers — common parts', () => {
  it(
    'names a new provider as the catalogue does, and refuses to save it without a sender',
    async () => {
      renderPage()
      await openFromCatalogue('Amazon SES')

      expect(screen.getByLabelText('Name')).toHaveValue('Amazon SES')
      expect(screen.getByText('No senders configured')).toBeInTheDocument()
      type('Access key ID', 'AKIA1')
      type('Secret access key', 'secret-1')
      await pickOption('Region', 'Ireland', 'Europe (Ireland) - eu-west-1')
      save()

      expect(await screen.findByText('Please add at least one sender before saving')).toBeInTheDocument()
      expect(posted('/api/workspaces.createIntegration')).toHaveLength(0)
    }
  )

  it(
    "stacks the sender dialog above the drawer when the card's + Add opens both at once",
    async () => {
      // Opened with the drawer, the dialog mounted first at the same z-index and stayed hidden
      // behind it; Add sender and Edit then changed nothing visible.
      renderPage([sesIntegration])
      fireEvent.click(screen.getByRole('button', { name: /^Add$/ }))
      const dialog = await screen.findByRole('dialog', { name: 'Add sender' })
      const wrap = dialog.closest('.ant-modal-wrap') as HTMLElement
      const drawer = document.querySelector('.app-full-drawer') as HTMLElement
      const z = (el: HTMLElement) => Number(el.style.zIndex || getComputedStyle(el).zIndex || 1000)
      expect(z(wrap)).toBeGreaterThan(z(drawer))
    }
  )

  it(
    'adds, edits, makes default and removes senders, and saves them with the rate',
    async () => {
      renderPage([sesIntegration])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      await screen.findByRole('button', { name: 'Save' })

      await addSender('team@acme.com', 'Acme Team')
      const rows = () => document.querySelectorAll('[data-sender-row]')
      expect(rows()).toHaveLength(2)
      // The first sender stays the default; the second is offered as one.
      expect(within(rows()[0] as HTMLElement).getByText('Default')).toBeInTheDocument()
      fireEvent.click(within(rows()[1] as HTMLElement).getByRole('button', { name: 'Make default' }))
      expect(within(rows()[1] as HTMLElement).getByText('Default')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'Edit sender team@acme.com' }))
      const modal = await screen.findByRole('dialog', { name: 'Edit sender' })
      fireEvent.change(within(modal).getByLabelText('Name'), { target: { value: 'Acme Support' } })
      fireEvent.click(within(modal).getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(document.querySelector('[data-senders]')).toHaveTextContent('Acme Support'))

      fireEvent.click(screen.getByRole('button', { name: 'Remove sender news@acme.com' }))
      fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
      await waitFor(() => expect(rows()).toHaveLength(1))
      // The last sender cannot be removed.
      expect(screen.queryByRole('button', { name: /Remove sender/ })).toBeNull()

      fireEvent.change(screen.getByLabelText('Rate for marketing'), { target: { value: '100' } })
      fireEvent.blur(screen.getByLabelText('Rate for marketing'))
      expect(await screen.findByText('≈ 6,000 per hour · ≈ 144,000 per day')).toBeInTheDocument()
      save()

      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      const body = posted('/api/workspaces.updateIntegration')[0] as { provider: Record<string, unknown> }
      expect(body.provider.senders).toEqual([
        { id: expect.any(String), email: 'team@acme.com', name: 'Acme Support', is_default: true }
      ])
      expect(body.provider.rate_limit_per_minute).toBe(100)
    }
  )

  it(
    'asks before closing on an added sender, and closes at once when nothing changed',
    async () => {
      // X and Escape dropped an added sender without a word.
      const closeDrawer = () =>
        fireEvent.click(
          // The last one: a drawer just closed can linger in the page while it animates out.
          within([...document.querySelectorAll<HTMLElement>('.app-full-drawer')].at(-1)!).getByRole(
            'button',
            { name: 'Close' }
          )
        )
      renderPage([sesIntegration])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      await screen.findByRole('button', { name: 'Save' })
      closeDrawer()
      await waitFor(() => expect(document.querySelector('[data-drawer-title]')).toBeNull())
      expect(screen.queryByText('Close without saving?', { selector: '.ant-modal-confirm-title' })).toBeNull()

      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      await screen.findByRole('button', { name: 'Save' })
      await addSender('team@acme.com', 'Acme Team')
      closeDrawer()
      const confirm = (
        await screen.findByText('Close without saving?', { selector: '.ant-modal-confirm-title' })
      ).closest(
        '.ant-modal'
      ) as HTMLElement
      expect(confirm).toHaveTextContent('Your changes to this integration will be lost.')
      fireEvent.click(within(confirm).getByRole('button', { name: 'Keep editing' }))
      await waitFor(() =>
        expect(
          screen.queryByText('Close without saving?', { selector: '.ant-modal-confirm-title' })
        ).toBeNull()
      )
      expect(document.querySelector('[data-senders]')).toHaveTextContent('team@acme.com')

      closeDrawer()
      const again = (
        await screen.findByText('Close without saving?', { selector: '.ant-modal-confirm-title' })
      ).closest(
        '.ant-modal'
      ) as HTMLElement
      fireEvent.click(within(again).getByRole('button', { name: 'Close' }))
      await waitFor(() => expect(document.querySelector('[data-drawer-title]')).toBeNull())
      expect(posted('/api/workspaces.updateIntegration')).toHaveLength(0)
    }
  )

  it(
    'keeps the sender dialog open on an invalid address, with no unhandled rejection',
    async () => {
      // antd rejects validateFields with { errorFields }: uncaught, it reached the console as
      // "Uncaught (in promise) [object Object]".
      const rejections: unknown[] = []
      const onRejection = (reason: unknown) => rejections.push(reason)
      process.on('unhandledRejection', onRejection)
      try {
        renderPage([sesIntegration])
        fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
        fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
        await screen.findByRole('button', { name: 'Save' })
        fireEvent.click(screen.getByRole('button', { name: /Add sender/ }))
        const modal = await screen.findByRole('dialog', { name: 'Add sender' })
        fireEvent.change(within(modal).getByLabelText('Email'), { target: { value: 'not-an-address' } })
        fireEvent.click(within(modal).getByRole('button', { name: 'Save' }))
        await waitFor(() => expect(modal.querySelector('.ant-form-item-explain-error')).not.toBeNull())
        await new Promise((resolve) => setTimeout(resolve, 50))
        expect(rejections).toEqual([])
        expect(screen.getByRole('dialog', { name: 'Add sender' })).toBeInTheDocument()
      } finally {
        process.off('unhandledRejection', onRejection)
      }
    }
  )

  it(
    'makes the sender left the default when the default one is removed',
    async () => {
      renderPage([sesIntegration])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      await screen.findByRole('button', { name: 'Save' })
      await addSender('team@acme.com', 'Acme Team')
      const rows = () => document.querySelectorAll('[data-sender-row]')
      expect(within(rows()[0] as HTMLElement).getByText('Default')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'Remove sender news@acme.com' }))
      fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
      await waitFor(() => expect(rows()).toHaveLength(1))
      expect(within(rows()[0] as HTMLElement).getByText('Default')).toBeInTheDocument()
      save()

      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      const body = posted('/api/workspaces.updateIntegration')[0] as { provider: Record<string, unknown> }
      expect(body.provider.senders).toEqual([
        { id: expect.any(String), email: 'team@acme.com', name: 'Acme Team', is_default: true }
      ])
    }
  )

  it(
    'tests what the form holds, naming the saved integration so the server fills the blank secret',
    async () => {
      renderPage([sesIntegration])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      fireEvent.click(await screen.findByRole('button', { name: /Send a test email/ }))

      const modal = await screen.findByRole('dialog', { name: 'Send a test email' })
      expect(within(modal).getByText('A real email, sent through these settings, saved or not.')).toBeInTheDocument()
      fireEvent.change(within(modal).getByLabelText('Send to'), { target: { value: 'pierre@acme.com' } })
      fireEvent.click(within(modal).getByRole('button', { name: 'Send test email' }))

      await waitFor(() => expect(posted('/api/email.testProvider')).toHaveLength(1))
      expect(posted('/api/email.testProvider')[0]).toEqual({
        provider: {
          kind: 'ses',
          senders: [sender],
          rate_limit_per_minute: 50,
          ses: {
            region: 'eu-west-1',
            access_key: 'AKIAIOSFODNN7EXAMPLE',
            tenant_name: 'my-tenant',
            inbound_topic_arn: 'arn:aws:sns:eu-west-1:123:replies'
          }
        },
        to: 'pierre@acme.com',
        workspace_id: 'ws1',
        integration_id: 'int_ses'
      })
    }
  )

  it(
    'tests a saved provider from its card with what is stored',
    async () => {
      renderPage([mailjetIntegration])
      fireEvent.click(screen.getByRole('button', { name: /Send test/ }))
      const modal = await screen.findByRole('dialog', { name: 'Send a test email' })
      // It tests what is saved: "saved or not" was the drawer's wording.
      expect(
        within(modal).getByText("A real email, sent through this integration's saved settings.")
      ).toBeInTheDocument()
      expect(within(modal).queryByText(/saved or not/)).toBeNull()
      fireEvent.change(within(modal).getByLabelText('Send to'), { target: { value: 'pierre@acme.com' } })
      fireEvent.click(within(modal).getByRole('button', { name: 'Send test email' }))

      await waitFor(() => expect(posted('/api/email.testProvider')).toHaveLength(1))
      expect(posted('/api/email.testProvider')[0]).toMatchObject({
        provider: mailjetIntegration.email_provider,
        integration_id: 'int_mailjet'
      })
    }
  )
})

describe('email providers — the roles', () => {
  it(
    'sets the marketing provider from its row after a confirmation, and never offers Mailjet there',
    async () => {
      renderPage([sesIntegration, mailjetIntegration], { transactional_email_provider_id: 'int_mailjet' })

      const transactional = screen.getByRole('combobox', { name: 'Transactional emails' })
      expect(transactional.closest('.ant-select')).toHaveTextContent('Mailjet')

      fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Marketing emails' }))
      const listbox = await screen.findByRole('listbox')
      expect(within(listbox).queryByText('Mailjet')).toBeNull()
      fireEvent.click(await screen.findByTitle('Amazon SES'))

      expect((await screen.findAllByText('Set as marketing email provider?')).length).toBeGreaterThan(0)
      fireEvent.click(screen.getByRole('button', { name: 'Yes' }))

      await waitFor(() => expect(posted('/api/workspaces.update')).toHaveLength(1))
      expect(posted('/api/workspaces.update')[0]).toMatchObject({
        id: 'ws1',
        settings: { marketing_email_provider_id: 'int_ses', transactional_email_provider_id: 'int_mailjet' }
      })
    }
  )
})

describe('SMTP', () => {
  it(
    'saves a server with basic authentication',
    async () => {
      renderPage()
      await openFromCatalogue('SMTP')

      expect(screen.getByLabelText('Name')).toHaveValue('SMTP')
      expect(screen.getByRole('switch', { name: 'Encryption' })).toBeChecked()
      type('Host', 'smtp.acme.com')
      type('Port', '587')
      type('EHLO hostname', 'mx.acme.com')
      type('Username', 'postmaster@acme.com')
      type('Password', 'hunter22')
      await addSender('hello@acme.com', 'Acme')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        name: 'SMTP',
        type: 'email',
        provider: {
          kind: 'smtp',
          senders: [{ id: expect.any(String), email: 'hello@acme.com', name: 'Acme', is_default: true }],
          rate_limit_per_minute: 25,
          smtp: {
            host: 'smtp.acme.com',
            port: 587,
            use_tls: true,
            ehlo_hostname: 'mx.acme.com',
            auth_type: 'basic',
            username: 'postmaster@acme.com',
            password: 'hunter22'
          }
        }
      })
    }
  )

  it(
    'switches to OAuth2 for Microsoft 365 and requires its fields',
    async () => {
      renderPage()
      await openFromCatalogue('SMTP')
      type('Host', 'smtp.office365.com')
      type('Port', '587')
      fireEvent.click(screen.getByText('OAuth2'))

      expect(await screen.findByText('Mailbox address')).toBeInTheDocument()
      expect(screen.queryByLabelText('Password')).toBeNull()
      await addSender('hello@acme.com', 'Acme')
      save()
      await expectRequiredError('Please select an OAuth2 provider')
      await expectRequiredError('Email address is required for OAuth2')

      fireEvent.click(screen.getByText('Microsoft 365 / Office 365'))
      expect(await screen.findByLabelText('Tenant ID')).toBeInTheDocument()
      expect(screen.queryByLabelText('Refresh token')).toBeNull()
      type('Mailbox address', 'notifications@acme.com')
      type('Tenant ID', 'tenant-1')
      type('Client ID', 'client-1')
      save()
      await expectRequiredError('Client Secret is required')
      type('Client secret', 'secret-1')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      const body = posted('/api/workspaces.createIntegration')[0] as { provider: { smtp: unknown } }
      expect(body.provider.smtp).toEqual({
        host: 'smtp.office365.com',
        port: 587,
        use_tls: true,
        auth_type: 'oauth2',
        oauth2_provider: 'microsoft',
        username: 'notifications@acme.com',
        oauth2_tenant_id: 'tenant-1',
        oauth2_client_id: 'client-1',
        oauth2_client_secret: 'secret-1'
      })
    }
  )

  it(
    'asks Google for a refresh token, and shows the end of the stored secrets on an edit',
    async () => {
      const google = email(
        'int_smtp',
        'Workspace relay',
        {
          kind: 'smtp',
          smtp: {
            host: 'smtp.gmail.com',
            port: 465,
            use_tls: true,
            username: 'me@acme.com',
            auth_type: 'oauth2',
            oauth2_provider: 'google',
            oauth2_client_id: 'gid',
            encrypted_oauth2_client_secret: 'x',
            encrypted_oauth2_refresh_token: 'y'
          }
        },
        { credential_hints: { 'smtp.oauth2_client_secret': 'Qw2Z', 'smtp.oauth2_refresh_token': 'rT9k' } }
      )
      renderPage([google])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))

      expect(await screen.findByLabelText('Refresh token')).toHaveAttribute('placeholder', '••••••••rT9k')
      expect(screen.getByLabelText('Client secret')).toHaveAttribute('placeholder', '••••••••Qw2Z')
      expect(screen.queryByLabelText('Tenant ID')).toBeNull()
      save()

      // Blank secrets on an edit are not required: the server keeps the stored ones.
      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.updateIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        integration_id: 'int_smtp',
        name: 'Workspace relay',
        provider: {
          kind: 'smtp',
          senders: [sender],
          rate_limit_per_minute: 50,
          smtp: {
            host: 'smtp.gmail.com',
            port: 465,
            use_tls: true,
            auth_type: 'oauth2',
            oauth2_provider: 'google',
            username: 'me@acme.com',
            oauth2_client_id: 'gid'
          }
        }
      })
    }
  )

  it(
    'shows the end of a stored password, or says one is kept, and nothing when none is stored',
    async () => {
      const relay = (encrypted: boolean, hints?: Record<string, string>) =>
        email(
          'int_smtp',
          'Relay',
          {
            kind: 'smtp',
            smtp: {
              host: 'smtp.acme.com',
              port: 587,
              use_tls: true,
              username: 'me@acme.com',
              auth_type: 'basic',
              ...(encrypted ? { encrypted_password: 'x' } : {})
            }
          },
          hints ? { credential_hints: hints } : {}
        )
      const openEdit = async () => {
        fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
        fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
        return screen.findByLabelText('Password')
      }

      const hinted = renderPage([relay(true, { 'smtp.password': 'pW9x' })])
      expect(await openEdit()).toHaveAttribute('placeholder', '••••••••pW9x')
      hinted.unmount()

      const unhinted = renderPage([relay(true)])
      expect(await openEdit()).toHaveAttribute('placeholder', 'Leave blank to keep the current value')
      unhinted.unmount()

      // The demo's Mailpit relay: a username and no password at all.
      renderPage([relay(false)])
      expect(await openEdit()).not.toHaveAttribute('placeholder', 'Leave blank to keep the current value')
    }
  )
})

describe('Amazon SES', () => {
  it(
    'requires the secret key on creation and saves every field',
    async () => {
      renderPage()
      await openFromCatalogue('Amazon SES')
      await pickOption('Region', 'Ireland', 'Europe (Ireland) - eu-west-1')
      type('Access key ID', 'AKIA1')
      type('Configuration set', 'my-set')
      type('Inbound topic ARN', 'arn:aws:sns:eu-west-1:1:replies')
      await addSender('news@acme.com', 'Acme')
      save()
      await expectRequiredError('Please enter secret access key')

      type('Secret access key', 'secret-1')
      save()
      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toMatchObject({
        name: 'Amazon SES',
        provider: {
          kind: 'ses',
          ses: {
            region: 'eu-west-1',
            access_key: 'AKIA1',
            secret_key: 'secret-1',
            configuration_set_name: 'my-set',
            inbound_topic_arn: 'arn:aws:sns:eu-west-1:1:replies'
          }
        }
      })
    }
  )

  it(
    'keeps the stored secret and the server-owned names out of an edit',
    async () => {
      renderPage([sesIntegration])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))

      const secret = await screen.findByLabelText('Secret access key')
      expect(secret).toHaveAttribute('placeholder', '••••••••K7MD')
      expect(screen.getByText('Blank keeps the current one.')).toBeInTheDocument()
      expect(screen.getByLabelText('Inbound topic ARN')).toHaveValue('arn:aws:sns:eu-west-1:123:replies')
      save()

      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      const body = posted('/api/workspaces.updateIntegration')[0] as { provider: { ses: Record<string, unknown> } }
      expect(body.provider.ses).toEqual({
        region: 'eu-west-1',
        access_key: 'AKIAIOSFODNN7EXAMPLE',
        tenant_name: 'my-tenant',
        inbound_topic_arn: 'arn:aws:sns:eu-west-1:123:replies'
      })
      expect(body.provider.ses).not.toHaveProperty('managed_configuration_set')
      expect(body.provider.ses).not.toHaveProperty('encrypted_secret_key')
    }
  )

  it(
    'verifies a hand-managed tenant and says why sends would fail',
    async () => {
      vi.mocked(api.post).mockImplementation((async (path: string) =>
        path === '/api/ses.verifyTenant'
          ? {
              tenant_name: 'my-tenant',
              exists: true,
              configuration_set_associated: false,
              configuration_set_name: 'notifuse-int_ses',
              suppression_scope: 'ACCOUNT',
              fix_command: 'aws sesv2 create-tenant-resource-association --tenant-name my-tenant'
            }
          : routePost(path)) as typeof api.post)
      renderPage([sesIntegration])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      fireEvent.click(await screen.findByRole('button', { name: 'Verify' }))

      expect(
        await screen.findByText(
          'Configuration set notifuse-int_ses is not associated with tenant my-tenant: every send would fail.'
        )
      ).toBeInTheDocument()
      expect(screen.getByText('Its bounces still go to the account-wide suppression list.')).toBeInTheDocument()
      expect(screen.getByText(/create-tenant-resource-association/)).toBeInTheDocument()
      // A saved integration is checked with its stored credentials.
      expect(posted('/api/ses.verifyTenant')).toEqual([
        { workspace_id: 'ws1', integration_id: 'int_ses', tenant_name: 'my-tenant' }
      ])

      // The answer belongs to the tenant it was asked for.
      fireEvent.change(screen.getByLabelText('SES tenant'), { target: { value: 'other-tenant' } })
      await waitFor(() => expect(screen.queryByText(/every send would fail/)).toBeNull())
    }
  )

  it(
    'verifies a tenant of a new integration with the typed credentials only once they are complete',
    async () => {
      vi.mocked(api.post).mockImplementation((async (path: string) =>
        path === '/api/ses.verifyTenant'
          ? { tenant_name: 'team', exists: false, configuration_set_associated: false }
          : routePost(path)) as typeof api.post)
      renderPage()
      await openFromCatalogue('Amazon SES')
      type('SES tenant', 'team')
      expect(screen.getByRole('button', { name: 'Verify' })).toBeDisabled()

      await pickOption('Region', 'Ireland', 'Europe (Ireland) - eu-west-1')
      type('Access key ID', 'AKIA1')
      type('Secret access key', 'secret-1')
      await waitFor(() => expect(screen.getByRole('button', { name: 'Verify' })).toBeEnabled())
      fireEvent.click(screen.getByRole('button', { name: 'Verify' }))

      expect(await screen.findByText('Tenant team does not exist in this AWS account and region.')).toBeInTheDocument()
      expect(posted('/api/ses.verifyTenant')).toEqual([
        { workspace_id: 'ws1', region: 'eu-west-1', access_key: 'AKIA1', secret_key: 'secret-1', tenant_name: 'team' }
      ])
    }
  )
})

describe('SparkPost, Postmark, Mailgun, Mailjet and SendGrid', () => {
  it(
    'SparkPost: requires the endpoint and the API key on creation',
    async () => {
      renderPage()
      await openFromCatalogue('SparkPost')
      await addSender('news@acme.com', 'Acme')
      save()
      await expectRequiredError('Please enter API endpoint')
      await expectRequiredError('Please enter API key')

      fireEvent.click(screen.getByText('SparkPost EU'))
      expect(await screen.findByText('https://api.eu.sparkpost.com')).toBeInTheDocument()
      type('API key', 'sp-key-1')
      fireEvent.click(screen.getByRole('switch', { name: 'Sandbox mode' }))
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toMatchObject({
        name: 'SparkPost',
        provider: {
          kind: 'sparkpost',
          sparkpost: { endpoint: 'https://api.eu.sparkpost.com', api_key: 'sp-key-1', sandbox_mode: true }
        }
      })
    }
  )

  it(
    'Postmark: requires the server token and defaults the stream to outbound',
    async () => {
      renderPage()
      await openFromCatalogue('Postmark')
      expect(screen.getByLabelText('Message stream')).toHaveValue('outbound')
      await addSender('news@acme.com', 'Acme')
      save()
      await expectRequiredError('Please enter server token')
      type('Server token', 'pm-token')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toMatchObject({
        name: 'Postmark',
        provider: { kind: 'postmark', postmark: { server_token: 'pm-token', message_stream: 'outbound' } }
      })
    }
  )

  it(
    'Mailgun: requires the domain and the key, and takes the region',
    async () => {
      renderPage()
      await openFromCatalogue('Mailgun')
      await addSender('news@mg.acme.com', 'Acme')
      save()
      await expectRequiredError('Please enter domain')
      await expectRequiredError('Please enter API key')
      type('Domain', 'mg.acme.com')
      type('API key', 'mg-key')
      fireEvent.click(screen.getByText('EU'))
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toMatchObject({
        name: 'Mailgun',
        provider: { kind: 'mailgun', mailgun: { domain: 'mg.acme.com', api_key: 'mg-key', region: 'EU' } }
      })
    }
  )

  it(
    'Mailjet: says it is transactional only, requires both keys and saves the sandbox off',
    async () => {
      renderPage()
      await openFromCatalogue('Mailjet')
      expect(
        screen.getByText('Mailjet rewrites unsubscribe links, so it can send transactional emails only.')
      ).toBeInTheDocument()
      await addSender('orders@acme.com', 'Acme')
      save()
      await expectRequiredError('Please enter API key')
      await expectRequiredError('Please enter secret key')
      type('API key', 'mj-key')
      type('Secret key', 'mj-secret')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toMatchObject({
        name: 'Mailjet',
        provider: { kind: 'mailjet', mailjet: { api_key: 'mj-key', secret_key: 'mj-secret', sandbox_mode: false } }
      })
    }
  )

  it(
    'SendGrid: requires the key on creation, and keeps it blank on an edit',
    async () => {
      renderPage()
      await openFromCatalogue('SendGrid')
      await addSender('news@acme.com', 'Acme')
      save()
      await expectRequiredError('Please enter API key')
      type('API key', 'SG.key')
      save()
      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toMatchObject({
        name: 'SendGrid',
        provider: { kind: 'sendgrid', sendgrid: { api_key: 'SG.key' } }
      })
    }
  )

  it(
    'shows the end of each stored key on an edit, and saves without it',
    async () => {
      const mailjet = { ...mailjetIntegration, credential_hints: { 'mailjet.api_key': 'a81f', 'mailjet.secret_key': '0e6d' } }
      renderPage([mailjet])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))

      expect(await screen.findByLabelText('API key')).toHaveAttribute('placeholder', '••••••••a81f')
      expect(screen.getByLabelText('Secret key')).toHaveAttribute('placeholder', '••••••••0e6d')
      save()
      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.updateIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        integration_id: 'int_mailjet',
        name: 'Mailjet',
        provider: { kind: 'mailjet', senders: [sender], rate_limit_per_minute: 50, mailjet: { sandbox_mode: false } }
      })
    }
  )
})

describe('AI assistant', () => {
  it(
    'Anthropic: takes any model ID besides the suggestions, and requires the key on creation',
    async () => {
      renderPage()
      await openFromCatalogue('Anthropic')

      expect(screen.getByLabelText('Name')).toHaveValue('Anthropic')
      const model = screen.getByLabelText('Model')
      expect(model).toHaveValue('claude-sonnet-5-5')
      fireEvent.mouseDown(model)
      fireEvent.change(model, { target: { value: '' } })
      for (const id of ['claude-opus-5-5', 'claude-sonnet-5-5', 'claude-haiku-5-5']) {
        expect(await screen.findByTitle(id)).toBeInTheDocument()
      }
      fireEvent.change(model, { target: { value: 'claude-future-5' } })
      save()
      await expectRequiredError('Please enter your API key')
      type('API key', 'sk-ant-1')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        name: 'Anthropic',
        type: 'llm',
        llm_provider: { kind: 'anthropic', anthropic: { api_key: 'sk-ant-1', model: 'claude-future-5' } }
      })
    }
  )

  it(
    'OpenAI: starts on gpt-6.1-sol and saves the base URL and the reasoning effort',
    async () => {
      renderPage()
      await openFromCatalogue('OpenAI')
      expect(screen.getByLabelText('Model')).toHaveValue('gpt-6.1-sol')
      type('API key', 'sk-proj-1')
      type('Base URL', 'https://llm.acme.internal/v1')
      await pickOption('Reasoning effort', '', 'High')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        name: 'OpenAI',
        type: 'llm',
        llm_provider: {
          kind: 'openai',
          openai: {
            api_key: 'sk-proj-1',
            model: 'gpt-6.1-sol',
            base_url: 'https://llm.acme.internal/v1',
            reasoning_effort: 'high'
          }
        }
      })
    }
  )

  it(
    'OpenAI edit: resends the effort, keeps the key blank, and shows its end',
    async () => {
      const openai = {
        id: 'int_oai',
        name: 'Local Llama',
        type: 'llm',
        llm_provider: {
          kind: 'openai',
          openai: { model: 'llama3.3:70b', base_url: 'https://llm.acme.internal/v1', reasoning_effort: 'low', encrypted_api_key: 'e' }
        },
        credential_hints: { 'openai.api_key': 'r5Tn' },
        created_at: '2026-08-01T10:00:00Z',
        updated_at: '2026-08-01T10:00:00Z'
      } as unknown as Integration
      renderPage([openai])
      // The card: the end of the key, and the effort.
      expect(screen.getByText('••••r5Tn')).toBeInTheDocument()
      expect(screen.getByText('Low')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      expect(await screen.findByLabelText('API key')).toHaveAttribute('placeholder', '••••••••r5Tn')
      save()
      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.updateIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        integration_id: 'int_oai',
        name: 'Local Llama',
        llm_provider: {
          kind: 'openai',
          openai: { model: 'llama3.3:70b', base_url: 'https://llm.acme.internal/v1', reasoning_effort: 'low' }
        }
      })
    }
  )

  it(
    'Gemini: suggests the Gemini 4 models and saves the default one',
    async () => {
      renderPage()
      await openFromCatalogue('Google Gemini')
      const model = screen.getByLabelText('Model')
      expect(model).toHaveValue('gemini-4-argon')
      fireEvent.mouseDown(model)
      fireEvent.change(model, { target: { value: '' } })
      for (const id of ['gemini-4-carbon', 'gemini-4-argon', 'gemini-4-hydrogen']) {
        expect(await screen.findByTitle(id)).toBeInTheDocument()
      }
      fireEvent.change(model, { target: { value: 'gemini-4-argon' } })
      type('API key', 'AIza-1')
      save()
      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        name: 'Google Gemini',
        type: 'llm',
        llm_provider: { kind: 'gemini', gemini: { api_key: 'AIza-1', model: 'gemini-4-argon' } }
      })
    }
  )

  it('says an AI integration has no API key instead of calling it configured', () => {
    const keyless = {
      id: 'int_ant',
      name: 'Anthropic (staging)',
      type: 'llm',
      llm_provider: { kind: 'anthropic', anthropic: { model: 'claude-sonnet-4-6' } },
      created_at: '2026-08-01T10:00:00Z',
      updated_at: '2026-08-01T10:00:00Z'
    } as unknown as Integration
    renderPage([keyless])

    expect(screen.getAllByText('No API key').length).toBeGreaterThan(0)
    expect(screen.queryByText('Configured')).toBeNull()
  })

  it(
    'Firecrawl: requires the key, saves the base URL, then is offered no more',
    async () => {
      renderPage()
      await openFromCatalogue('Firecrawl')
      expect(screen.getByText('scrape_url')).toBeInTheDocument()
      expect(screen.getByText('search_web')).toBeInTheDocument()
      save()
      await expectRequiredError('Please enter your API key')
      type('API key', 'fc-key')
      type('Base URL', 'https://crawl.acme.com')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        name: 'Firecrawl',
        type: 'firecrawl',
        firecrawl_settings: { api_key: 'fc-key', base_url: 'https://crawl.acme.com' }
      })
    }
  )

  it('offers no second Firecrawl, which the server would never use, and shows the end of its key', async () => {
    const firecrawl = {
      id: 'int_fc',
      name: 'Firecrawl',
      type: 'firecrawl',
      firecrawl_settings: { encrypted_api_key: 'e' },
      credential_hints: { 'firecrawl.api_key': '3vTb' },
      created_at: '2026-08-01T10:00:00Z',
      updated_at: '2026-08-01T10:00:00Z'
    } as unknown as Integration
    renderPage([firecrawl])
    expect(screen.getByText('••••3vTb')).toBeInTheDocument()

    fireEvent.click(screen.getAllByRole('button', { name: /Add integration/ })[0])
    const catalogue = await screen.findByRole('dialog', { name: 'Add integration' })
    const entry = within(catalogue).getByRole('button', { name: /^Firecrawl/ })
    expect(entry).toBeDisabled()
    expect(entry).toHaveTextContent('Configured')
  })
})

describe('Supabase', () => {
  it(
    'creates the integration with both hooks, its lists and its metadata field',
    async () => {
      renderPage()
      await openFromCatalogue('Supabase')
      // The addresses carry the integration's id: none exists before the first save.
      expect(screen.getAllByText('Available after saving')).toHaveLength(2)
      type('Name', 'Auth')
      fireEvent.change(screen.getAllByLabelText('Hook secret')[0], { target: { value: 'v1,whsec_auth' } })
      fireEvent.change(screen.getAllByLabelText('Hook secret')[1], { target: { value: 'v1,whsec_user' } })
      await pickOption('Subscribe to lists', 'News', 'Newsletter')
      await pickOption('Save user metadata to', '', 'Supabase profile')
      fireEvent.click(screen.getByRole('switch', { name: 'Reject disposable emails' }))
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        name: 'Auth',
        type: 'supabase',
        supabase_settings: {
          auth_email_hook: { signature_key: 'v1,whsec_auth' },
          before_user_created_hook: {
            signature_key: 'v1,whsec_user',
            add_user_to_lists: ['list_news'],
            custom_json_field: 'custom_json_1',
            reject_disposable_email: true
          }
        }
      })
    }
  )

  const supabase = {
    id: 'int_sb',
    name: 'Supabase',
    type: 'supabase',
    supabase_settings: {
      auth_email_hook: { encrypted_signature_key: 'a' },
      before_user_created_hook: { encrypted_signature_key: 'b', add_user_to_lists: ['list_news'], reject_disposable_email: true }
    },
    credential_hints: {
      'supabase.auth_email_hook.signature_key': 'Hk8w',
      'supabase.before_user_created_hook.signature_key': 'p2Ve'
    },
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-01T10:00:00Z'
  } as unknown as Integration

  it(
    'shows the hook addresses and the end of each secret, and keeps blank secrets on an edit',
    async () => {
      renderPage([supabase])
      // The card.
      expect(screen.getByText('••••Hk8w')).toBeInTheDocument()
      expect(screen.getByText('••••p2Ve')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      await screen.findByRole('button', { name: 'Save' })
      const drawer = document.querySelector('.ant-drawer-open') as HTMLElement
      expect(within(drawer).getByText(/\/webhooks\/supabase\/auth-email\?workspace_id=ws1&integration_id=int_sb/)).toBeInTheDocument()
      expect(
        within(drawer).getByText(/\/webhooks\/supabase\/before-user-created\?workspace_id=ws1&integration_id=int_sb/)
      ).toBeInTheDocument()
      expect(within(drawer).getAllByRole('button', { name: 'Copy' })).toHaveLength(2)
      const [auth, user] = within(drawer).getAllByLabelText('Hook secret')
      expect(auth).toHaveAttribute('placeholder', '••••••••Hk8w')
      expect(user).toHaveAttribute('placeholder', '••••••••p2Ve')
      save()

      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.updateIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        integration_id: 'int_sb',
        name: 'Supabase',
        supabase_settings: {
          auth_email_hook: { signature_key: '' },
          before_user_created_hook: {
            signature_key: '',
            add_user_to_lists: ['list_news'],
            reject_disposable_email: true
          }
        }
      })
    }
  )

  it('warns that deleting it deletes its templates and notifications', async () => {
    renderPage([supabase])
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    fireEvent.click(await screen.findByText('Delete'))

    expect(
      await screen.findByText(
        "Its 6 templates and 6 transactional notifications are deleted with it, and Supabase's hooks stop working. This cannot be undone."
      )
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() =>
      expect(posted('/api/workspaces.deleteIntegration')).toEqual([{ workspace_id: 'ws1', integration_id: 'int_sb' }])
    )
  })
})

describe('Zapier', () => {
  it(
    'connects with a label, shows the token until Done, and renames on an edit',
    async () => {
      renderPage()
      await openFromCatalogue('Zapier')
      type('Label', 'Sales')
      fireEvent.click(screen.getByRole('button', { name: /Connect Zapier/ }))

      expect(await screen.findByLabelText('API key token')).toHaveValue('tok_once')
      expect(posted('/api/workspaces.connectZapier')).toEqual([{ workspace_id: 'ws1', label: 'Sales' }])
      fireEvent.click(screen.getByRole('button', { name: 'Done' }))
      await waitFor(() => expect(screen.queryByLabelText('API key token')).toBeNull())
    }
  )

  it(
    'renames a connection and sends nothing else',
    async () => {
      const zapier = {
        id: 'int_z',
        name: 'Marketing',
        type: 'zapier',
        zapier_settings: { api_key_email: 'zapier-marketing-3f9a1c02@api.acme.com' },
        created_at: '2026-08-01T10:00:00Z',
        updated_at: '2026-08-01T10:00:00Z'
      } as unknown as Integration
      renderPage([zapier])
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
      await screen.findByRole('button', { name: 'Save' })
      expect(screen.queryByRole('button', { name: /Connect Zapier/ })).toBeNull()
      type('Label', 'Sales')
      save()

      await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.updateIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        integration_id: 'int_z',
        name: 'Sales'
      })
    }
  )
})
