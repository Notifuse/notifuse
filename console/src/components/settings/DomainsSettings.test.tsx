import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DomainsSettings } from './DomainsSettings'
import { api } from '../../services/api/client'
import { ApiError } from '../../services/api/errors'
import type { DKIMDomain } from '../../services/api/dkim'

// The API client is the only thing mocked: the requests are checked as the server receives them.
vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const record = (overrides: Partial<DKIMDomain>): DKIMDomain => ({
  domain: 'acme.io',
  selector: 'notifuse202611a3f29c1b04e87d21',
  status: 'published',
  dns_record: {
    type: 'TXT',
    name: 'notifuse202611a3f29c1b04e87d21._domainkey.acme.io',
    value: 'v=DKIM1; k=rsa; p=MIIBIjANBg'
  },
  created_at: '2026-11-01T10:00:00Z',
  last_checked_at: new Date(Date.now() - 3 * 60 * 1000).toISOString(),
  consecutive_failures: 0,
  used_by: [],
  ...overrides
})

const published = record({ used_by: [{ integration_id: 'smtp1', integration_name: 'Office relay' }] })
const pending = record({
  domain: 'news.acme.io',
  status: 'pending',
  selector: 'notifuse202611ffffffffffffffff',
  dns_record: { type: 'TXT', name: 'notifuse202611ffffffffffffffff._domainkey.news.acme.io', value: 'v=DKIM1; k=rsa; p=NEWS' }
})
const mismatch = record({ domain: 'brand-b.com', status: 'mismatch', last_check_error: 'the published key is not this domain\'s key' })
const degrading = record({ domain: 'deg.io', consecutive_failures: 1, last_check_error: 'no TXT record found' })
const stopped = record({
  domain: 'stopped.io',
  status: 'pending',
  downgraded_at: '2026-11-20T08:00:00Z',
  downgrade_reason: 'no TXT record found'
})

const renderDomains = (isOwner = true) =>
  render(
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <DomainsSettings workspaceId="ws-1" isOwner={isOwner} />
        </App>
      </QueryClientProvider>
    </I18nProvider>
  )

const rowOf = (domain: string) => screen.getByText(domain, { selector: 'span.font-medium' }).closest('tr') as HTMLElement

describe('DomainsSettings', () => {
  beforeEach(() => {
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
    vi.mocked(api.get).mockResolvedValue({ domains: [published, pending, mismatch, degrading, stopped] })
  })

  it('lists the domains of the workspace with their state, selector and users', async () => {
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })
    expect(api.get).toHaveBeenCalledWith('/api/workspaces.listDKIMDomains?workspace_id=ws-1')

    expect(within(rowOf('acme.io')).getByText('Published · signing')).toBeInTheDocument()
    expect(within(rowOf('acme.io')).getByText('Checked 3 minutes ago')).toBeInTheDocument()
    expect(within(rowOf('acme.io')).getByText('Office relay')).toBeInTheDocument()
    expect(within(rowOf('news.acme.io')).getByText('DNS not found · not signing yet')).toBeInTheDocument()
    expect(within(rowOf('news.acme.io')).getByText('No SMTP integration yet')).toBeInTheDocument()
    expect(within(rowOf('brand-b.com')).getByText('Wrong key · not signing')).toBeInTheDocument()
    expect(within(rowOf('brand-b.com')).getByText("the published key is not this domain's key")).toBeInTheDocument()
    expect(within(rowOf('deg.io')).getByText('Record missing 1/3 · still signing')).toBeInTheDocument()
    expect(within(rowOf('stopped.io')).getByText('Signing stopped')).toBeInTheDocument()
    expect(within(rowOf('stopped.io')).getByText(/Stopped on Nov 20, 2026: no TXT record found/)).toBeInTheDocument()
  })

  it('shows the DNS record of the first domain that does not sign, then of the row clicked', async () => {
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })
    const dns = screen.getByTestId('dkim-dns-record')
    expect(screen.getByText('Publish this record for news.acme.io')).toBeInTheDocument()
    expect(within(dns).getByText(pending.dns_record.name)).toBeInTheDocument()
    expect(within(dns).getByText('v=DKIM1; k=rsa; p=NEWS')).toBeInTheDocument()

    fireEvent.click(rowOf('acme.io'))
    expect(screen.getByText('Publish this record for acme.io')).toBeInTheDocument()
    expect(within(dns).getByText(published.dns_record.name)).toBeInTheDocument()
  })

  it('offers Regenerate only on a key that does not sign, and blocks Delete while senders use the domain', async () => {
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })
    expect(within(rowOf('acme.io')).queryByRole('button', { name: 'Regenerate' })).toBeNull()
    expect(within(rowOf('acme.io')).getByRole('button', { name: 'Delete' })).toBeDisabled()
    expect(within(rowOf('news.acme.io')).getByRole('button', { name: 'Regenerate' })).toBeInTheDocument()
    expect(within(rowOf('brand-b.com')).getByRole('button', { name: 'Delete' })).toBeEnabled()
  })

  it('creates a key from the dialog', async () => {
    vi.mocked(api.post).mockResolvedValue({ domain: record({ domain: 'new.io', status: 'pending' }) })
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    fireEvent.click(screen.getByRole('button', { name: 'Add domain' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Domain'), { target: { value: 'new.io' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create key' }))

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/workspaces.createDKIMDomain', { workspace_id: 'ws-1', domain: 'new.io' })
    )
    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  })

  it('verifies a domain and says what the lookup found', async () => {
    vi.mocked(api.post).mockResolvedValue({ domain: pending, check: { result: 'not_found', detail: 'no TXT record found' } })
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    fireEvent.click(within(rowOf('news.acme.io')).getByRole('button', { name: 'Verify' }))
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/workspaces.verifyDKIMDomain', { workspace_id: 'ws-1', domain: 'news.acme.io' })
    )
    expect(await screen.findByText(/No DNS record found for news.acme.io yet/)).toBeInTheDocument()
  })

  it('deletes after confirmation, and shows the refusal the server gives', async () => {
    vi.mocked(api.post).mockRejectedValue(new ApiError('brand-b.com is used by senders of 1 integration(s)', 409))
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    fireEvent.click(within(rowOf('brand-b.com')).getByRole('button', { name: 'Delete' }))
    const confirm = await screen.findByRole('dialog')
    fireEvent.click(within(confirm).getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/workspaces.deleteDKIMDomain', { workspace_id: 'ws-1', domain: 'brand-b.com' })
    )
    expect(await screen.findByText(/is used by senders of 1 integration/)).toBeInTheDocument()
  })

  it('is read only for a member', async () => {
    renderDomains(false)
    await screen.findByText('acme.io', { selector: 'span.font-medium' })
    expect(screen.getByRole('button', { name: 'Add domain' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Verify' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
    expect(screen.getByTestId('dkim-dns-record')).toBeInTheDocument()
  })

  it('says when there is no domain yet', async () => {
    vi.mocked(api.get).mockResolvedValue({ domains: [] })
    renderDomains()
    expect(await screen.findByText('No domains yet')).toBeInTheDocument()
    expect(screen.queryByTestId('dkim-dns-record')).toBeNull()
  })

  it('says when the member may not read the domains', async () => {
    vi.mocked(api.get).mockRejectedValue(new ApiError('forbidden', 403))
    renderDomains(false)
    expect(await screen.findByText("You don't have permission to view the domains")).toBeInTheDocument()
  })
})
