import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SMTPDkimPanel } from './SMTPDkimPanel'
import { api } from '../../../services/api/client'
import type { DKIMDomain } from '../../../services/api/dkim'

vi.mock('../../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, params }: { children: ReactNode; params: { workspaceId: string; section: string } }) => (
    <a href={`/console/workspace/${params.workspaceId}/settings/${params.section}`}>{children}</a>
  )
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const key = (domain: string, overrides: Partial<DKIMDomain> = {}): DKIMDomain => ({
  domain,
  selector: `sel-${domain}`,
  status: 'published',
  dns_record: { type: 'TXT', name: '', value: '' },
  created_at: '2026-11-01T10:00:00Z',
  consecutive_failures: 0,
  used_by: [],
  ...overrides
})

const renderPanel = (senders: { email: string }[]) =>
  render(
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <SMTPDkimPanel workspaceId="ws-1" senders={senders} />
        </App>
      </QueryClientProvider>
    </I18nProvider>
  )

const rowOf = (domain: string) => screen.getByText(domain).closest('tr') as HTMLElement

describe('SMTPDkimPanel', () => {
  beforeEach(() => {
    vi.mocked(api.get).mockReset()
    vi.mocked(api.get).mockResolvedValue({
      domains: [
        key('acme.io'),
        key('pending.io', { status: 'pending' }),
        key('wrong.io', { status: 'mismatch' }),
        key('stopped.io', { status: 'pending', downgraded_at: '2026-11-20T08:00:00Z' }),
        key('degrading.io', { consecutive_failures: 2 }),
        key('xn--bcher-kva.de'),
        key('later.io', { status: 'mismatch', downgraded_at: '2026-11-20T08:00:00Z' })
      ]
    })
  })

  it('says for each sender domain whether it is signed, once per domain, by exact domain', async () => {
    renderPanel([
      { email: 'news@acme.io' },
      { email: 'team@ACME.io' },
      { email: 'help@support.acme.io' },
      { email: 'a@pending.io' },
      { email: 'a@wrong.io' },
      { email: 'a@stopped.io' },
      { email: 'a@degrading.io' },
      { email: 'info@bücher.de' },
      { email: 'a@later.io' }
    ])
    await screen.findAllByText('Signed')
    expect(api.get).toHaveBeenCalledWith('/api/workspaces.listDKIMDomains?workspace_id=ws-1')

    expect(screen.getAllByText('acme.io')).toHaveLength(1)
    expect(within(rowOf('acme.io')).getByText('Signed')).toBeInTheDocument()
    expect(screen.queryByText('sel-acme.io')).toBeNull()
    expect(within(rowOf('support.acme.io')).getByText('No key: sent unsigned')).toBeInTheDocument()
    expect(within(rowOf('pending.io')).getByText('DNS not found: sent unsigned')).toBeInTheDocument()
    expect(within(rowOf('wrong.io')).getByText('Wrong key: sent unsigned')).toBeInTheDocument()
    expect(within(rowOf('stopped.io')).getByText('Signing stopped: sent unsigned')).toBeInTheDocument()
    expect(within(rowOf('degrading.io')).getByText('Signed · record missing 2/3')).toBeInTheDocument()
    expect(within(rowOf('xn--bcher-kva.de')).getByText('Signed')).toBeInTheDocument()
    expect(within(rowOf('later.io')).getByText('Wrong key: sent unsigned')).toBeInTheDocument()
  })

  it('links to Settings › Domains', async () => {
    renderPanel([{ email: 'news@acme.io' }])
    expect(screen.getByRole('link', { name: 'Manage domains' })).toHaveAttribute(
      'href',
      '/console/workspace/ws-1/settings/domains'
    )
  })

  it('asks for a sender when there is none', () => {
    renderPanel([])
    expect(screen.getByText('Add a sender to see whether its domain is signed.')).toBeInTheDocument()
  })
})
