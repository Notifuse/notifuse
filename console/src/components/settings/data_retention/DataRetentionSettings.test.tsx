import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DataRetentionSettings } from './DataRetentionSettings'
import { api } from '../../../services/api/client'
import type { DataRetentionView, RetentionCategory, RetentionCategorySpec } from '../../../services/api/dataRetention'
import { LicenseContext, UNKNOWN_LICENSE } from '../../../contexts/licenseState'
import type { Entitlements } from '../../../types/license'

// The API client is the only thing mocked: the requests are checked as the server receives them.
vi.mock('../../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', proceed: undefined, reset: undefined }),
  // The licence notice's button routes to Settings › Licence.
  useNavigate: () => vi.fn()
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const specs: RetentionCategorySpec[] = [
  { category: 'inactive_contacts', min_days: 180, suggested_days: 1095, default_days: 0 },
  { category: 'message_history', min_days: 90, suggested_days: 1095, default_days: 0 },
  { category: 'webhook_deliveries', min_days: 7, suggested_days: 7, default_days: 7 }
]
const days = (values: Partial<Record<RetentionCategory, number>>) =>
  ({ inactive_contacts: 0, message_history: 0, webhook_deliveries: 7, ...values }) as Record<RetentionCategory, number>

const baseView = (overrides: Partial<DataRetentionView> = {}): DataRetentionView => ({
  policy: { categories: { inactive_contacts: 1095 } },
  in_force: days({ inactive_contacts: 1095 }),
  defaults: days({}),
  categories: specs,
  backfill_ready: true,
  runs: [],
  event_names: ['purchase', 'login'],
  ...overrides
})

const mockServer = (view: DataRetentionView) => {
  vi.mocked(api.get).mockImplementation(async (endpoint: string) => {
    if (endpoint.startsWith('/api/dataRetention.get')) return view
    if (endpoint.startsWith('/api/lists.list')) return { lists: [{ id: 'vip', name: 'Customers' }] }
    if (endpoint.startsWith('/api/segments.list')) return { segments: [] }
    if (endpoint.startsWith('/api/tasks.list')) return { tasks: [], total_count: 0, limit: 1, offset: 0, has_more: false }
    throw new Error(`unexpected GET ${endpoint}`)
  })
  vi.mocked(api.post).mockImplementation(async (endpoint: string) => {
    if (endpoint === '/api/dataRetention.preview') {
      return {
        pending: true,
        pending_at: '2026-11-30T12:00:00Z',
        categories: {
          inactive_contacts: { days: 1095, affected: 1284, opt_outs_kept: 211 },
          message_history: { days: 1095, affected: 48210 },
          webhook_deliveries: { days: 7, affected: 3 }
        }
      }
    }
    if (endpoint === '/api/dataRetention.set') return { data_retention: { pending: { effective_at: '2026-11-30T12:00:00Z' } } }
    if (endpoint === '/api/dataRetention.cancelPending') return { data_retention: {} }
    if (endpoint === '/api/dataRetention.retryBackfill') return { success: true }
    throw new Error(`unexpected POST ${endpoint}`)
  })
}

// A deployment whose licence is known and grants only these features.
const licensedFor = (features: Entitlements['features']): Entitlements => ({
  tier: features.length ? 'Enterprise' : '',
  org: '',
  sub: '',
  max_workspaces: 3,
  features,
  state: features.length ? 'active' : 'none',
  expires_at: '0001-01-01T00:00:00Z'
})

const renderSection = (isOwner = true, features?: Entitlements['features']) => {
  const page = (
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <DataRetentionSettings workspaceId="ws-1" isOwner={isOwner} />
        </App>
      </QueryClientProvider>
    </I18nProvider>
  )
  return render(
    features ? (
      <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(features) }}>{page}</LicenseContext.Provider>
    ) : (
      page
    )
  )
}

const rowOf = (label: string) => screen.getByText(label, { selector: 'div.font-medium' }).closest('tr') as HTMLElement

describe('DataRetentionSettings', () => {
  beforeEach(() => {
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
  })

  it('shows each period in the largest unit, and a category left to the deployment default', async () => {
    mockServer(baseView())
    renderSection()
    const contacts = await waitFor(() => rowOf('Inactive contacts'))
    expect(within(contacts).getByRole('spinbutton')).toHaveValue('3')
    expect(within(rowOf('Webhook deliveries')).getByText('Deployment default')).toBeInTheDocument()
  })

  it('is read-only for a member who is not an owner', async () => {
    mockServer(baseView())
    renderSection(false)
    const contacts = await waitFor(() => rowOf('Inactive contacts'))
    expect(within(contacts).getByRole('switch')).toBeDisabled()
    expect(screen.queryByRole('button', { name: /preview and save/i })).not.toBeInTheDocument()
  })

  it('without a licence, a category can be switched off but not on, and its period is fixed', async () => {
    mockServer(baseView())
    renderSection(true, [])
    const contacts = await waitFor(() => rowOf('Inactive contacts'))
    expect(screen.getByText('Data retention policies require a Notifuse Enterprise licence.')).toBeInTheDocument()
    expect(within(contacts).getByRole('switch')).toBeEnabled()
    expect(within(contacts).getByRole('spinbutton')).toBeDisabled()
    expect(within(rowOf('Message history')).getByRole('switch')).toBeDisabled()
    // A built-in default is not a configuration: the technical logs can come back on.
    expect(within(rowOf('Webhook deliveries')).getByRole('switch')).toBeEnabled()
    expect(screen.getByRole('combobox', { name: 'Custom events that count as activity' })).toBeDisabled()
  })

  it('with the licence, no notice and nothing locked', async () => {
    mockServer(baseView())
    renderSection(true, ['data_retention'])
    await waitFor(() => rowOf('Inactive contacts'))
    expect(screen.queryByText('Data retention policies require a Notifuse Enterprise licence.')).not.toBeInTheDocument()
    expect(within(rowOf('Message history')).getByRole('switch')).toBeEnabled()
  })

  it('refuses a period shorter than its category allows', async () => {
    mockServer(baseView())
    renderSection()
    const contacts = await waitFor(() => rowOf('Inactive contacts'))
    fireEvent.mouseDown(within(contacts).getByRole('combobox'))
    const options = await screen.findAllByTitle('days')
    fireEvent.click(options[options.length - 1])
    fireEvent.change(within(contacts).getByRole('spinbutton'), { target: { value: '30' } })
    expect(await within(contacts).findByText('At least 180 days')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /preview and save/i })).toBeDisabled()
  })

  it('saves through the preview, which shows what the first run removes', async () => {
    mockServer(baseView())
    renderSection()
    const messages = await waitFor(() => rowOf('Message history'))
    fireEvent.click(within(messages).getByRole('switch'))
    fireEvent.click(screen.getByRole('button', { name: /preview and save/i }))

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('48,210')).toBeInTheDocument()
    expect(within(dialog).getByText('211 unsubscribes kept as hashes')).toBeInTheDocument()
    expect(within(dialog).getByText(/deletes more than the policy in force/)).toBeInTheDocument()
    expect(vi.mocked(api.post).mock.calls.find(([endpoint]) => endpoint === '/api/dataRetention.preview')?.[1]).toEqual({
      workspace_id: 'ws-1',
      policy: {
        categories: { inactive_contacts: 1095, message_history: 1095 },
        exempt_list_ids: [],
        exempt_segment_ids: [],
        engagement_event_names: []
      }
    })

    fireEvent.click(within(dialog).getByRole('button', { name: /save the policy/i }))
    await waitFor(() =>
      expect(vi.mocked(api.post).mock.calls.some(([endpoint]) => endpoint === '/api/dataRetention.set')).toBe(true)
    )
  })

  it('shows the change that waits, and an owner can cancel it', async () => {
    mockServer(
      baseView({
        pending: {
          policy: { categories: { inactive_contacts: 1095, message_history: 1095 } },
          effective_at: '2026-11-30T12:00:00Z',
          requested_at: '2026-11-23T12:00:00Z',
          requested_by: 'owner@acme.com'
        },
        pending_at: '2026-11-30T12:00:00Z',
        pending_days: days({ inactive_contacts: 1095, message_history: 1095 })
      })
    )
    renderSection()
    expect(await screen.findByText(/takes effect on/)).toBeInTheDocument()
    expect(screen.getByText(/Message history: Off → 3 years/)).toBeInTheDocument()
    expect(screen.getByText(/Requested by owner@acme.com/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /cancel the change/i }))
    await waitFor(() =>
      expect(vi.mocked(api.post)).toHaveBeenCalledWith('/api/dataRetention.cancelPending', { workspace_id: 'ws-1' })
    )
  })

  it('reports a partial run and what each category did', async () => {
    mockServer(
      baseView({
        runs: [
          {
            id: 'run-1',
            started_at: '2026-11-22T05:00:00Z',
            finished_at: '2026-11-22T05:04:00Z',
            status: 'partial',
            policy: {},
            results: {
              inactive_contacts: { days: 1095, affected: 5000, remaining: true },
              message_history: { days: 1095, affected: 0, error: 'lock timeout' }
            }
          }
        ]
      })
    )
    renderSection()
    expect(await screen.findByText('Partial')).toBeInTheDocument()
    expect(screen.getByText(/more at the next run/)).toBeInTheDocument()
    expect(screen.getByText(/lock timeout/)).toBeInTheDocument()
  })

  it('offers to run a failed backfill again', async () => {
    mockServer(baseView({ backfill_ready: false }))
    vi.mocked(api.get).mockImplementation(async (endpoint: string) => {
      if (endpoint.startsWith('/api/dataRetention.get')) return baseView({ backfill_ready: false })
      if (endpoint.startsWith('/api/tasks.list')) {
        return { tasks: [{ id: 'task-1', status: 'failed', error_message: 'connection reset' }], total_count: 1, limit: 1, offset: 0, has_more: false }
      }
      return { lists: [], segments: [] }
    })
    renderSection()
    fireEvent.click(await screen.findByRole('button', { name: /run it again/i }))
    await waitFor(() => expect(vi.mocked(api.post)).toHaveBeenCalledWith('/api/dataRetention.retryBackfill', { workspace_id: 'ws-1' }))
  })

  it('keeps the unit chosen while a number is typed', async () => {
    mockServer(baseView())
    renderSection()
    const contacts = await waitFor(() => rowOf('Inactive contacts'))
    fireEvent.mouseDown(within(contacts).getByRole('combobox'))
    const options = await screen.findAllByTitle('days')
    fireEvent.click(options[options.length - 1])
    const input = within(contacts).getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '30' } })
    fireEvent.change(input, { target: { value: '300' } })
    expect(input).toHaveValue('300')
    expect(within(contacts).getByTitle('days')).toBeInTheDocument()
  })

  it('shows a waiting change that only removes an exemption, and an owner can cancel it', async () => {
    mockServer(
      baseView({
        policy: { categories: { inactive_contacts: 1095 }, exempt_list_ids: ['vip'] },
        pending: {
          policy: { categories: { inactive_contacts: 1095 } },
          effective_at: '2026-11-30T12:00:00Z',
          requested_at: '2026-11-23T12:00:00Z'
        },
        pending_at: '2026-11-30T12:00:00Z',
        pending_days: days({ inactive_contacts: 1095 })
      })
    )
    renderSection()
    expect(await screen.findByText(/No longer exempt: the list Customers/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /cancel the change/i })).toBeInTheDocument()
  })
})
