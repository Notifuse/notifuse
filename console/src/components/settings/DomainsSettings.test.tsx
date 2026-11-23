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

/** Opens the panel of a domain by clicking its row, and returns the panel. */
const openPanel = async (domain: string) => {
  fireEvent.click(rowOf(domain))
  const record = await screen.findByTestId('dkim-dns-record')
  return record.closest('[role="dialog"]') as HTMLElement
}

describe('DomainsSettings', () => {
  beforeEach(() => {
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
    vi.mocked(api.get).mockResolvedValue({ domains: [published, pending, mismatch, degrading, stopped] })
  })

  it('lists the domains of the workspace with their state and users, without the selector', async () => {
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })
    expect(api.get).toHaveBeenCalledWith('/api/workspaces.listDKIMDomains?workspace_id=ws-1')

    expect(within(rowOf('acme.io')).getByText('Published · signing')).toBeInTheDocument()
    expect(within(rowOf('acme.io')).getByText('3 minutes ago')).toBeInTheDocument()
    expect(within(rowOf('acme.io')).getByText('Office relay')).toBeInTheDocument()
    expect(within(rowOf('news.acme.io')).getByText('DNS not found · not signing yet')).toBeInTheDocument()
    expect(within(rowOf('news.acme.io')).getByText('No SMTP integration yet')).toBeInTheDocument()
    expect(within(rowOf('brand-b.com')).getByText('Wrong key · not signing')).toBeInTheDocument()
    expect(within(rowOf('deg.io')).getByText('Record missing 1/3 · still signing')).toBeInTheDocument()
    expect(within(rowOf('stopped.io')).getByText('Signing stopped')).toBeInTheDocument()
    expect(screen.queryByText('notifuse202611a3f29c1b04e87d21')).toBeNull()

    // The reason behind a status is in the panel of the domain.
    let panel = await openPanel('brand-b.com')
    expect(within(panel).getByText("the published key is not this domain's key")).toBeInTheDocument()
    panel = await openPanel('stopped.io')
    expect(within(panel).getByText(/Stopped on Nov 20, 2026: no TXT record found/)).toBeInTheDocument()
  })

  it('opens a panel with the whole DNS record when a row is clicked', async () => {
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })
    expect(screen.queryByTestId('dkim-dns-record')).toBeNull()

    const panel = await openPanel('news.acme.io')
    const record = within(panel).getByTestId('dkim-dns-record')
    expect(within(record).getByText(pending.dns_record.name)).toBeInTheDocument()
    expect(within(record).getByText('v=DKIM1; k=rsa; p=NEWS')).toBeInTheDocument()
    expect(within(panel).getByText('DNS not found · not signing yet')).toBeInTheDocument()
  })

  it('offers Regenerate only on a key that does not sign, and blocks Delete while senders use the domain', async () => {
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })
    let panel = await openPanel('acme.io')
    expect(within(panel).queryByRole('button', { name: 'Regenerate' })).toBeNull()
    expect(within(panel).getByRole('button', { name: 'Delete' })).toBeDisabled()

    panel = await openPanel('news.acme.io')
    expect(within(panel).getByRole('button', { name: 'Regenerate' })).toBeEnabled()

    panel = await openPanel('brand-b.com')
    expect(within(panel).getByRole('button', { name: 'Delete' })).toBeEnabled()
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

  it('opens the panel of the new domain, with the record to publish', async () => {
    const created = record({ domain: 'new.io', status: 'pending', dns_record: { type: 'TXT', name: 's._domainkey.new.io', value: 'v=DKIM1; k=rsa; p=NEW' } })
    vi.mocked(api.post).mockResolvedValue({ domain: created })
    vi.mocked(api.get)
      .mockResolvedValueOnce({ domains: [published] })
      .mockResolvedValue({ domains: [published, created] })
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    fireEvent.click(screen.getByRole('button', { name: 'Add domain' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Domain'), { target: { value: 'new.io' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create key' }))

    const recordBlock = await screen.findByTestId('dkim-dns-record')
    expect(within(recordBlock).getByText('s._domainkey.new.io')).toBeInTheDocument()
  })

  it('sends one request when Enter is pressed twice, and none for an empty field', async () => {
    let finish: (value: unknown) => void = () => {}
    vi.mocked(api.post).mockReturnValue(new Promise((resolve) => (finish = resolve)))
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    fireEvent.click(screen.getByRole('button', { name: 'Add domain' }))
    const dialog = await screen.findByRole('dialog')
    const input = within(dialog).getByLabelText('Domain')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create key' }))
    expect(await within(dialog).findByText('Please enter a domain')).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()

    fireEvent.change(input, { target: { value: 'new.io' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', keyCode: 13 })
    fireEvent.keyUp(input, { key: 'Enter', code: 'Enter', keyCode: 13 })
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', keyCode: 13 })
    fireEvent.keyUp(input, { key: 'Enter', code: 'Enter', keyCode: 13 })
    finish({ domain: record({ domain: 'new.io', status: 'pending' }) })
    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
    expect(api.post).toHaveBeenCalledTimes(1)
  })

  it('verifies a domain and says what the lookup found', async () => {
    vi.mocked(api.post).mockResolvedValue({ domain: pending, check: { result: 'not_found', detail: 'no TXT record found' } })
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    const panel = await openPanel('news.acme.io')
    fireEvent.click(within(panel).getByRole('button', { name: 'Verify' }))
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/workspaces.verifyDKIMDomain', { workspace_id: 'ws-1', domain: 'news.acme.io' })
    )
    expect(await screen.findByText(/No DNS record found for news.acme.io yet/)).toBeInTheDocument()
  })

  it("shows a check's result only in the panel of the domain checked", async () => {
    let finish: (value: unknown) => void = () => {}
    vi.mocked(api.post).mockReturnValue(new Promise((resolve) => (finish = resolve)))
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    let panel = await openPanel('news.acme.io')
    fireEvent.click(within(panel).getByRole('button', { name: 'Verify' }))
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
    panel = await openPanel('brand-b.com')
    finish({ domain: pending, check: { result: 'ok' } })
    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
    expect(within(panel).queryByText(/DNS record found/)).toBeNull()
  })

  it('deletes after confirmation, and shows the refusal the server gives', async () => {
    vi.mocked(api.post).mockRejectedValue(new ApiError('brand-b.com is used by senders of 1 integration(s)', 409))
    renderDomains()
    await screen.findByText('acme.io', { selector: 'span.font-medium' })

    const panel = await openPanel('brand-b.com')
    fireEvent.click(within(panel).getByRole('button', { name: 'Delete' }))
    const confirms = await screen.findAllByRole('dialog')
    const confirm = confirms[confirms.length - 1]
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
    const panel = await openPanel('news.acme.io')
    expect(within(panel).getByTestId('dkim-dns-record')).toBeInTheDocument()
    expect(within(panel).getByRole('button', { name: 'Verify' })).toBeDisabled()
    expect(within(panel).getByRole('button', { name: 'Regenerate' })).toBeDisabled()
    expect(within(panel).getByRole('button', { name: 'Delete' })).toBeDisabled()
  })

  it('after a downgrade, shows the latest check, keeping the date signing stopped', async () => {
    vi.mocked(api.get).mockResolvedValue({
      domains: [
        record({
          domain: 'later.io',
          status: 'mismatch',
          downgraded_at: '2026-11-20T08:00:00Z',
          downgrade_reason: 'no TXT record found',
          last_check_error: '2 TXT records found, a selector must publish one'
        })
      ]
    })
    renderDomains()
    await screen.findByText('later.io', { selector: 'span.font-medium' })
    expect(within(rowOf('later.io')).getByText('Wrong key · not signing')).toBeInTheDocument()
    const panel = await openPanel('later.io')
    expect(
      within(panel).getByText(/Stopped on Nov 20, 2026: 2 TXT records found, a selector must publish one/)
    ).toBeInTheDocument()
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
