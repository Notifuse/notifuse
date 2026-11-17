import type React from 'react'
import { afterAll, describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { Integrations } from './Integrations'
import { LicenseContext, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import type { Entitlements } from '../../types/license'
import type { Integration, Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { activateDayjsLocale } from '../../i18n/dayjsLocales'
import { getWebhookStatus, registerWebhook } from '../../services/api/webhook_registration'

// The dates here are read on the browser's clock. Pinned to UTC so the day the tests expect is the
// day shown on any runner, including one east of UTC+11 or west of UTC-10. Restored after the
// file: a worker can go on to run other files.
const runnerTimeZone = process.env.TZ
process.env.TZ = 'UTC'
afterAll(() => {
  if (runnerTimeZone === undefined) delete process.env.TZ
  else process.env.TZ = runnerTimeZone
})

i18n.loadAndActivate({ locale: 'en', messages: {} })

// The component reaches for lists and for AWS on mount; neither is what these tests are about.
vi.mock('../../services/api/list', () => ({
  listsApi: { list: vi.fn().mockResolvedValue({ lists: [] }) }
}))
vi.mock('./useSESDiscovery', () => ({
  useSESDiscovery: () => ({
    tenantOptions: [],
    configurationSetOptions: [],
    denied: false,
    loading: false
  })
}))
// These are the names Integrations.tsx actually imports. An earlier version of this mock stubbed
// getEmailProviderWebhookStatus/registerEmailProviderWebhooks, which the module has not exported
// for some time: the stub was inert and the real functions would have run on the first test that
// opened a provider drawer.
vi.mock('../../services/api/webhook_registration', () => ({
  getWebhookStatus: vi.fn().mockResolvedValue({ status: { endpoints: [] } }),
  registerWebhook: vi.fn(),
  WebhookRegistrationStatus: {}
}))
vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    get: vi.fn(),
    connectZapier: vi.fn(),
    createIntegration: vi.fn(),
    updateIntegration: vi.fn(),
    deleteIntegration: vi.fn(),
    update: vi.fn()
  }
}))

const ZAPIER_KEY_EMAIL = 'zapier-marketing-3f9a1c02@api.notifuse.com'

const zapierIntegration = {
  id: 'int_zapier_1',
  name: 'Marketing',
  type: 'zapier',
  zapier_settings: { api_key_email: ZAPIER_KEY_EMAIL },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z'
} as unknown as Integration

const makeWorkspace = (integrations: Integration[]) =>
  ({
    id: 'ws1',
    name: 'Acme',
    settings: { integrations: [] },
    integrations
  }) as unknown as Workspace

interface RenderOptions {
  integrations?: Integration[]
  isOwner?: boolean
  // What the deployment is licensed for. Omitted, the console has not been told and every
  // advisory read answers "licensed", which is the state every other test here runs in.
  licensedFor?: Entitlements['features']
}

const entitlementsFor = (features: Entitlements['features']): Entitlements => ({
  tier: 'Studio',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 5,
  features,
  state: 'active',
  expires_at: '2027-01-01T00:00:00Z'
})

// The <App> wrapper is load-bearing, not decoration. ZapierSettings reads `message` from
// App.useApp(), whose default context is an empty object, and calls it from an async handler
// invoked as a floating promise — so without a provider the TypeError surfaces as an
// unattributed unhandled rejection and the test still reports green.
const renderIntegrations = ({
  integrations = [],
  isOwner = true,
  licensedFor
}: RenderOptions = {}) => {
  let rerender: (ui: React.ReactElement) => void = () => {}
  const tree = (workspace: Workspace) => (
    <I18nProvider i18n={i18n}>
      <App>
        <LicenseContext.Provider
          value={{
            ...UNKNOWN_LICENSE,
            entitlements: licensedFor ? entitlementsFor(licensedFor) : null
          }}
        >
          <Integrations workspace={workspace} onSave={onSave} loading={false} isOwner={isOwner} />
        </LicenseContext.Provider>
      </App>
    </I18nProvider>
  )
  // Mirrors WorkspaceSettingsPage: onSave swaps the workspace prop, which re-renders Integrations.
  // A bare vi.fn() here would mean the refresh never re-enters the component, so any test claiming
  // to survive one would assert nothing about the render it is named after.
  const onSave = vi.fn(async (workspace: Workspace) => {
    rerender(tree(workspace))
  })
  const utils = render(tree(makeWorkspace(integrations)))
  rerender = utils.rerender
  return { ...utils, onSave }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(workspaceService.get).mockResolvedValue({
    workspace: makeWorkspace([zapierIntegration])
  } as unknown as Awaited<ReturnType<typeof workspaceService.get>>)
  vi.mocked(workspaceService.connectZapier).mockResolvedValue({
    status: 'success',
    token: 'tok_secret_value',
    email: ZAPIER_KEY_EMAIL,
    integration_id: 'int_zapier_1'
  })
})

// The one way in: "Add integration" (the page's, first; an empty block repeats it), then the
// catalogue entry.
const openFromCatalogue = async (user: ReturnType<typeof userEvent.setup>, entry: string) => {
  await user.click((await screen.findAllByRole('button', { name: /Add integration/ }))[0])
  const catalogue = await screen.findByRole('dialog', { name: 'Add integration' })
  await user.click(within(catalogue).getByRole('button', { name: new RegExp(`^${entry}`) }))
}

// openSESDrawer walks the path an operator takes: pick Amazon SES from the catalogue.
// The tenant fields only exist once that drawer is open, so every test starts here.
const openSESDrawer = async (options: RenderOptions = {}) => {
  const user = userEvent.setup()
  renderIntegrations(options)
  await openFromCatalogue(user, 'Amazon SES')
  await waitFor(() => expect(screen.getByText('Tenant isolation')).toBeInTheDocument())
  return user
}

const sesTenantField = () => screen.getByLabelText('SES tenant')
const isolationSwitch = () => screen.getByRole('switch', { name: 'Tenant isolation' })

describe('Integrations — marketing rate limit', () => {
  // The queue now sends at this rate, so the field has to say what it governs and what it
  // must stay under: a limit above the provider's leads to throttling, which slows sending down.
  it('says what the rate applies to and what it must stay under', async () => {
    await openSESDrawer()

    const field = screen.getByLabelText('Rate for marketing')
    expect(field).toHaveValue('25')
    const row = field.closest('[data-settings-row]') as HTMLElement
    expect(within(row).getByText('Broadcasts and automation emails go out at up to this rate')).toBeInTheDocument()
    expect(
      within(row).getByText("Keep it at or below your provider's limit, minus your transactional traffic.")
    ).toBeInTheDocument()
  })
})

describe('Integrations — marketing rate limit in the console language', () => {
  afterEach(() => {
    i18n.activate('en')
  })

  it('writes the hourly and daily volume with the separators of the console language', async () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    await openSESDrawer()

    // 25 per minute by default. French groups thousands with a narrow no-break space.
    expect(screen.getByText(/^≈ 1\s500 per hour · ≈ 36\s000 per day$/)).toBeInTheDocument()
  })
})

describe('Integrations — SES tenant isolation', () => {
  // The gate is EnsureTenantIsolation's, and it still answers 402. What this pins is the order
  // of events: an owner on an unlicensed deployment meets a locked switch that names the plan,
  // not a live one that refuses on Save.
  it('locks switching isolation on when the deployment is not licensed for it', async () => {
    await openSESDrawer({ licensedFor: ['rbac'] })

    expect(
      screen.getByText('SES tenant isolation requires a Notifuse Studio licence.')
    ).toBeInTheDocument()
    expect(isolationSwitch()).toBeDisabled()
  })

  it('leaves the switch live when the licence covers tenant isolation', async () => {
    await openSESDrawer({ licensedFor: ['ses_tenant'] })

    expect(screen.queryByText(/requires? a Notifuse/i)).not.toBeInTheDocument()
    expect(isolationSwitch()).toBeEnabled()
  })

  it('shows the tenant fields without expanding anything', async () => {
    // They used to live behind an "Advanced" collapse. The Advanced panel is a plain panel, not a
    // collapse: both must be on screen as soon as the SES form is.
    await openSESDrawer()

    expect(screen.getByLabelText('Configuration set')).toBeVisible()
    expect(sesTenantField()).toBeVisible()
    expect(screen.getByLabelText('Inbound topic ARN')).toBeVisible()
    expect(document.querySelector('.ant-collapse')).toBeNull()
  })

  it('reveals the IAM permissions only once isolation is switched on', async () => {
    const user = await openSESDrawer()

    // Off by default: the permissions list is three lines of reference material that only matters
    // to someone turning the switch on.
    expect(screen.queryByText(/ses:CreateTenant/)).not.toBeInTheDocument()

    await user.click(isolationSwitch())
    expect(await screen.findByText(/ses:CreateTenant/)).toBeInTheDocument()

    await user.click(isolationSwitch())
    await waitFor(() => expect(screen.queryByText(/ses:CreateTenant/)).not.toBeInTheDocument())
  })

  it('explains why an invalid tenant name is rejected', async () => {
    // The hint on this field used to be passed as `help`, which replaces the whole explain area
    // and left a rejected name with no message at all.
    const user = await openSESDrawer()

    await user.type(sesTenantField(), 'my tenant!')

    expect(
      await screen.findByText('Up to 64 letters, numbers, hyphens or underscores.')
    ).toBeInTheDocument()
    // The hint stays put alongside the error rather than being replaced by it.
    expect(screen.getByText(/Use a tenant you manage yourself/)).toBeInTheDocument()
  })

  it('names the conflict when isolation is on and a tenant is typed', async () => {
    // The server refuses this combination (AmazonSESSettings.Validate); without a client rule the
    // operator only found out when saving failed.
    const user = await openSESDrawer()

    await user.click(isolationSwitch())
    await user.type(sesTenantField(), 'team-acme')

    expect(
      await screen.findByText(
        'Turn off SES tenant isolation to use your own tenant, or clear this field.'
      )
    ).toBeInTheDocument()
  })

  it('re-checks the conflict when the switch moves, not just when the field is typed in', async () => {
    // dependencies on the switch: a tenant typed first and isolation enabled after is the same
    // invalid state, and has to be reported without touching the field again.
    const user = await openSESDrawer()

    await user.type(sesTenantField(), 'team-acme')
    await waitFor(() =>
      expect(screen.queryByText(/Turn off SES tenant isolation/)).not.toBeInTheDocument()
    )

    await user.click(isolationSwitch())

    expect(await screen.findByText(/Turn off SES tenant isolation/)).toBeInTheDocument()
  })

  it('accepts a tenant name on its own', async () => {
    const user = await openSESDrawer()

    await user.type(sesTenantField(), 'team-acme')

    await waitFor(() =>
      expect(screen.queryByText(/Turn off SES tenant isolation/)).not.toBeInTheDocument()
    )
    expect(
      screen.queryByText('Up to 64 letters, numbers, hyphens or underscores.')
    ).not.toBeInTheDocument()
  })
})

describe('Integrations — Zapier', () => {
  it('offers Zapier in the catalogue of a fresh workspace', async () => {
    const user = userEvent.setup()
    renderIntegrations()

    await user.click((await screen.findAllByRole('button', { name: /Add integration/ }))[0])

    expect(await screen.findByRole('button', { name: /^Zapier/ })).toBeInTheDocument()
  })

  it('still offers Zapier once a connection exists: several are allowed', async () => {
    const user = userEvent.setup()
    renderIntegrations({ integrations: [zapierIntegration] })

    await user.click(screen.getAllByRole('button', { name: /Add integration/ })[0])

    expect(await screen.findByRole('button', { name: /^Zapier/ })).toBeEnabled()
  })

  it('renders a connected Zapier card with its label and key address', async () => {
    renderIntegrations({ integrations: [zapierIntegration] })

    expect(await screen.findByText('Marketing')).toBeInTheDocument()
    expect(screen.getByText(ZAPIER_KEY_EMAIL)).toBeInTheDocument()
  })

  it('falls back to the generic card when a Zapier record carries no settings', async () => {
    // Reachable: a rollback to a build without the type, or a record written by hand. Reading
    // api_key_email off it unguarded would take the whole Integrations screen down.
    const settingsless = { ...zapierIntegration, zapier_settings: undefined }
    renderIntegrations({ integrations: [settingsless] })

    // Named as a reader knows it, not by its raw type
    expect(await screen.findByText('Type: Zapier')).toBeInTheDocument()
    expect(screen.queryByText(ZAPIER_KEY_EMAIL)).not.toBeInTheDocument()
  })

  it('names the type of an integration that has no card of its own', async () => {
    renderIntegrations({ integrations: [{ ...zapierIntegration, id: 'sms-1', name: 'Texts', type: 'sms' }] })

    expect(await screen.findByText('Type: SMS')).toBeInTheDocument()
  })

  it('connects Zapier and refreshes the workspace without closing the token panel', async () => {
    const user = userEvent.setup()
    const { onSave } = renderIntegrations()

    await openFromCatalogue(user, 'Zapier')
    const label = await screen.findByLabelText('Label')
    await user.clear(label)
    await user.type(label, 'Marketing')
    await user.click(screen.getByRole('button', { name: /Connect Zapier/ }))

    await waitFor(() =>
      expect(workspaceService.connectZapier).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        label: 'Marketing'
      })
    )
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(workspaceService.get).toHaveBeenCalledWith('ws1')
    // The token exists in exactly one response. A refresh that closed the drawer would discard
    // the only copy the user will ever be shown.
    expect(screen.getByLabelText('API key token')).toHaveValue('tok_secret_value')
  })

  it('shows a member the Zapier card without Edit nor its menu', async () => {
    renderIntegrations({
      integrations: [zapierIntegration],
      isOwner: false
    })

    expect(await screen.findByText(ZAPIER_KEY_EMAIL)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Add integration/ })).toBeNull()
  })

  it('warns that disconnecting Zapier revokes its key', async () => {
    const user = userEvent.setup()
    renderIntegrations({ integrations: [zapierIntegration] })

    await user.click(screen.getByRole('button', { name: 'More actions' }))
    await user.click(await screen.findByText('Disconnect'))

    expect((await screen.findAllByText('Delete this connection?')).length).toBeGreaterThan(0)
    const warning = await screen.findByText(/revokes the API key/)
    expect(warning).toHaveTextContent(/stops working/)
  })
})

const mailjetIntegration = {
  id: 'int_mailjet_1',
  name: 'Transactional',
  type: 'email',
  email_provider: {
    kind: 'mailjet',
    senders: [{ id: 'sender_1', email: 'hello@acme.com', name: 'Acme', is_default: true }],
    rate_limit_per_minute: 25,
    mailjet: { sandbox_mode: false }
  },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z'
} as unknown as Integration

describe('Integrations — webhook registration', () => {
  // The console names no event, so the provider registers its default ones; for Mailjet those
  // include the unsub webhook, registered with 'complaint'. Nothing here can ask for
  // 'unsubscribe', which no provider registers.
  it('registers the provider default events, naming none', async () => {
    vi.mocked(registerWebhook).mockResolvedValue({
      status: { email_provider_kind: 'mailjet', is_registered: true, endpoints: [] }
    })
    const user = userEvent.setup()
    renderIntegrations({ integrations: [mailjetIntegration] })
    await waitFor(() => expect(getWebhookStatus).toHaveBeenCalledTimes(1))

    await user.click(await screen.findByRole('button', { name: 'More actions' }))
    // Not registered yet: the first registration asks nothing, as before.
    await user.click(await screen.findByText('Register webhooks'))

    await waitFor(() => expect(registerWebhook).toHaveBeenCalledTimes(1))
    expect(vi.mocked(registerWebhook).mock.calls[0][0]).toEqual({
      workspace_id: 'ws1',
      integration_id: 'int_mailjet_1',
      base_url: expect.any(String)
    })
  })
})

describe('Integrations — blocks', () => {
  it('opens every integration from one catalogue, grouped by use', async () => {
    const user = userEvent.setup()
    renderIntegrations()

    await user.click((await screen.findAllByRole('button', { name: /Add integration/ }))[0])
    const catalogue = await screen.findByRole('dialog', { name: 'Add integration' })

    expect(within(catalogue).getByText('Email sending')).toBeInTheDocument()
    expect(within(catalogue).getByText('AI assistant')).toBeInTheDocument()
    expect(within(catalogue).getByText('Connected apps')).toBeInTheDocument()
    // The 7 email providers, the 3 AI models, Firecrawl, Supabase and Zapier.
    expect(catalogue.querySelectorAll('[data-catalogue-entry]')).toHaveLength(13)
    // The server refuses Mailjet for marketing: the catalogue says so before it is picked.
    expect(within(catalogue).getByRole('button', { name: /^Mailjet/ })).toHaveTextContent('Transactional only')
  })

  it('marks SparkPost and Mailjet deprecated in the catalogue, and says why', async () => {
    const user = userEvent.setup()
    renderIntegrations()

    await user.click((await screen.findAllByRole('button', { name: /Add integration/ }))[0])
    const catalogue = await screen.findByRole('dialog', { name: 'Add integration' })
    const entry = (name: RegExp) => within(catalogue).getByRole('button', { name })

    expect(entry(/^Postmark/)).not.toHaveTextContent('Deprecated')
    expect(entry(/^Mailjet/)).toHaveTextContent('Deprecated')
    await user.hover(within(entry(/^SparkPost/)).getByText('Deprecated'))
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'This integration will be removed soon: SparkPost no longer accepts new customers.'
    )
  })

  it('shows a member no way to add an integration', async () => {
    renderIntegrations({ isOwner: false })

    expect(await screen.findByText('No email provider yet')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Add integration/ })).toBeNull()
    expect(screen.getByRole('combobox', { name: 'Marketing emails' })).toBeDisabled()
  })

  it('says which role has no provider in its own row, without the blue box', async () => {
    renderIntegrations({ integrations: [mailjetIntegration] })

    const marketing = (await screen.findByText('Marketing emails')).closest('[data-settings-row]') as HTMLElement
    expect(within(marketing).getByText('Not set')).toBeInTheDocument()
    expect(screen.queryByText('Email Provider Configuration Needed')).toBeNull()
  })

  it('invites to add an email provider when only other integrations exist', async () => {
    renderIntegrations({ integrations: [zapierIntegration] })

    expect(await screen.findByText('No email provider yet')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Add integration/ }).length).toBeGreaterThan(1)
    expect(screen.getByText('Connected apps')).toBeInTheDocument()
  })
})

describe('Integrations — dates', () => {
  afterEach(() => {
    activateDayjsLocale('en')
  })

  it('writes the day a Zapier card was connected in the console language', async () => {
    activateDayjsLocale('fr')
    renderIntegrations({ integrations: [zapierIntegration] })

    expect(await screen.findByText('1 août 2026')).toBeInTheDocument()
  })
})

describe('Integrations — drawer titles', () => {
  const drawerTitle = () => document.querySelector('[data-drawer-title]')?.textContent

  it.each([
    ['Mailgun', 'Add Mailgun integration'],
    ['Amazon SES', 'Add Amazon SES integration'],
    ['Supabase', 'Add Supabase integration'],
    ['OpenAI', 'Add OpenAI integration'],
    ['Firecrawl', 'Add Firecrawl integration'],
    ['Zapier', 'Connect Zapier']
  ])('names %s in the title of its drawer', async (item, title) => {
    const user = userEvent.setup()
    renderIntegrations({ integrations: [zapierIntegration] })
    await openFromCatalogue(user, item)

    await waitFor(() => expect(drawerTitle()).toBe(title))
    // The bar's way back names the page it returns to.
    expect(screen.getByRole('button', { name: /Integrations/ })).toBeInTheDocument()
  })
})
