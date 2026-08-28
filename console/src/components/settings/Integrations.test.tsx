import type React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { Integrations } from './Integrations'
import type { Integration, Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'

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
}

// The <App> wrapper is load-bearing, not decoration. ZapierSettings reads `message` from
// App.useApp(), whose default context is an empty object, and calls it from an async handler
// invoked as a floating promise — so without a provider the TypeError surfaces as an
// unattributed unhandled rejection and the test still reports green.
const renderIntegrations = ({ integrations = [], isOwner = true }: RenderOptions = {}) => {
  let rerender: (ui: React.ReactElement) => void = () => {}
  const tree = (workspace: Workspace) => (
    <I18nProvider i18n={i18n}>
      <App>
        <Integrations workspace={workspace} onSave={onSave} loading={false} isOwner={isOwner} />
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

// openSESDrawer walks the path an operator takes: pick Amazon SES from the available providers.
// The tenant fields only exist once that drawer is open, so every test starts here.
const openSESDrawer = async () => {
  const user = userEvent.setup()
  renderIntegrations()
  await user.click(await screen.findByText('Amazon SES'))
  await waitFor(() => expect(screen.getByText('SES tenant isolation')).toBeInTheDocument())
  return user
}

const sesTenantField = () => screen.getByLabelText('SES tenant')
const isolationSwitch = () =>
  within(screen.getByText('SES tenant isolation').closest('.ant-form-item') as HTMLElement).getByRole(
    'switch'
  )

describe('Integrations — SES tenant isolation', () => {
  it('shows the tenant fields without expanding anything', async () => {
    // They used to live behind an "Advanced" collapse; the collapse is gone, so both must be on
    // screen as soon as the SES form is.
    await openSESDrawer()

    expect(screen.getByLabelText('Configuration set')).toBeVisible()
    expect(sesTenantField()).toBeVisible()
    expect(screen.queryByText('Advanced')).not.toBeInTheDocument()
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
  it('offers Zapier in the empty-state catalogue', async () => {
    // The Add Integration dropdown is hidden until the workspace has at least one integration,
    // so on a fresh workspace the catalogue is the only way in.
    renderIntegrations()

    expect(await screen.findByText('Zapier')).toBeInTheDocument()
  })

  it('offers Zapier in the Add Integration dropdown once an integration exists', async () => {
    const user = userEvent.setup()
    renderIntegrations({ integrations: [zapierIntegration] })

    await user.click(screen.getByRole('button', { name: /Add Integration/ }))

    expect(await screen.findByRole('menuitem', { name: /Zapier/ })).toBeInTheDocument()
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

    expect(await screen.findByText('Type: zapier')).toBeInTheDocument()
    expect(screen.queryByText(ZAPIER_KEY_EMAIL)).not.toBeInTheDocument()
  })

  it('connects Zapier and refreshes the workspace without closing the token panel', async () => {
    const user = userEvent.setup()
    const { onSave } = renderIntegrations()

    await user.click(await screen.findByText('Zapier'))
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

  it('shows a member the Zapier card without Edit or Delete', async () => {
    const { container } = renderIntegrations({
      integrations: [zapierIntegration],
      isOwner: false
    })

    expect(await screen.findByText(ZAPIER_KEY_EMAIL)).toBeInTheDocument()
    expect(container.querySelector('[data-icon="pen-to-square"]')).toBeNull()
    expect(container.querySelector('[data-icon="trash-can"]')).toBeNull()
  })

  it('warns that deleting a Zapier connection revokes its key', async () => {
    const user = userEvent.setup()
    const { container } = renderIntegrations({ integrations: [zapierIntegration] })

    await user.click(
      container.querySelector('[data-icon="trash-can"]')?.closest('button') as HTMLElement
    )

    const warning = await screen.findByText(/revokes the API key/)
    expect(warning).toHaveTextContent(/stops working/)
  })
})
