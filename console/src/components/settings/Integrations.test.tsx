import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { Integrations } from './Integrations'
import type { Workspace } from '../../services/api/types'

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
vi.mock('../../services/api/webhook_registration', () => ({
  getEmailProviderWebhookStatus: vi.fn().mockResolvedValue({ status: 'unregistered' }),
  registerEmailProviderWebhooks: vi.fn(),
  unregisterEmailProviderWebhooks: vi.fn(),
  EmailProviderWebhookStatus: {}
}))

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { integrations: [] },
  integrations: []
} as unknown as Workspace

const renderIntegrations = () =>
  render(
    <I18nProvider i18n={i18n}>
      <Integrations workspace={workspace} onSave={vi.fn()} loading={false} isOwner={true} />
    </I18nProvider>
  )

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
  beforeEach(() => {
    vi.clearAllMocks()
  })

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
