import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { WebAnalyticsSettings, WebFilter } from '../../../services/api/web_analytics'

// The API client imports the router, which builds the whole route graph at import time.
vi.mock('../../../router', () => ({ router: { navigate: vi.fn() } }))

const { refreshWorkspaces } = vi.hoisted(() => ({ refreshWorkspaces: vi.fn() }))
vi.mock('../../../contexts/AuthContext', () => ({ useAuth: () => ({ refreshWorkspaces }) }))

const RULES: WebFilter[] = [
  {
    id: 'r1',
    name: 'Newsletter traffic',
    priority: 10,
    order: 1,
    tags: ['email'],
    conditions: [{ field: 'utm_source', operator: 'equals', value: 'newsletter' }],
    operations: [{ dimension: 'channel', action: 'set_value', value: 'Email' }],
    enabled: true,
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z'
  },
  {
    id: 'r2',
    name: 'Paused fallback',
    priority: 20,
    order: 2,
    tags: [],
    conditions: [],
    operations: [{ dimension: 'channel_group', action: 'unset_value' }],
    enabled: false
  },
  {
    id: 'r3',
    name: 'Paid search',
    priority: 30,
    order: 3,
    tags: [],
    conditions: [{ field: 'utm_medium', operator: 'equals', value: 'cpc' }],
    operations: [{ dimension: 'channel', action: 'set_value', value: 'Paid' }],
    enabled: true
  }
]

const SETTINGS: WebAnalyticsSettings = {
  enabled: true,
  allowed_domains: ['example.com'],
  filters: RULES,
  filters_version: 'abcd1234',
  geo_enabled: false,
  geo_store_city: false,
  geo_store_region: false,
  geo_coordinates_precision: 2
}

vi.mock('../useWebAnalytics', () => ({
  useWebAnalytics: () => ({
    workspaceId: 'ws1',
    settings: SETTINGS,
    customDimensionLabels: {},
    tag: undefined,
    setTag: vi.fn()
  })
}))

import { FiltersTab } from './FiltersTab'

const NOW = '2026-11-16T09:30:00.000Z'

const fetchMock = vi.fn(async (url: string) => {
  const body = url.endsWith('/api/webAnalytics.backfillStatus') ? { backfill: null } : {}
  return { ok: true, status: 200, json: async () => body }
})

/** The bodies the console posted to the settings endpoint, as the server receives them. */
const settingsRequests = () =>
  fetchMock.mock.calls
    .filter(([url]) => String(url).endsWith('/api/workspaces.setWebAnalyticsSettings'))
    .map((call) => JSON.parse(String((call as unknown as [string, RequestInit])[1].body)))

function renderTab() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <App>
        <FiltersTab />
      </App>
    </QueryClientProvider>
  )
}

/** The open side panel whose header reads `title`. */
const panel = (title: string) =>
  screen.getByRole('heading', { level: 2, name: title }).closest('.ant-drawer-content-wrapper') as HTMLElement

const desktopRow = (name: string) => {
  const desktop = document.querySelector('.hidden.md\\:block') as HTMLElement
  return within(desktop).getByText(name)
}

describe('FiltersTab', () => {
  beforeEach(() => {
    fetchMock.mockClear()
    refreshWorkspaces.mockReset().mockResolvedValue(undefined)
    vi.stubGlobal('fetch', fetchMock)
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(NOW))
    vi.spyOn(crypto, 'randomUUID').mockReturnValue('00000000-0000-4000-8000-000000000001')
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('opens the rule editor in a side panel, its fields in cards', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))

    const editor = panel('Create rule')
    // A side panel at the shared width, not a full-width drawer.
    expect(editor.style.width).toBe('520px')
    expect(editor.querySelector('[data-sidepanel-header]')).not.toBeNull()
    const sections = [...editor.querySelectorAll('[data-panel-head] h3')].map((head) => head.textContent)
    expect(sections).toEqual(['Details', 'Conditions', 'Operations'])
    // The priority's help is written under its field rather than hidden in a tooltip.
    expect(within(editor).getByText('Higher priority rules are evaluated first (0-1000)')).toBeVisible()
    expect(editor.querySelectorAll('[data-condition-row]')).toHaveLength(1)
    expect(editor.querySelectorAll('[data-operation-row]')).toHaveLength(1)
    expect(within(editor).getByRole('button', { name: /Test$/ })).toBeInTheDocument()
  })

  it('creates a rule with the same settings body as before', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))
    const editor = panel('Create rule')

    fireEvent.change(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads'), {
      target: { value: '  Google search  ' }
    })
    const [conditionValue, operationValue] = within(editor).getAllByPlaceholderText('Value')
    fireEvent.change(conditionValue, { target: { value: 'google' } })
    fireEvent.change(operationValue, { target: { value: 'Search' } })
    fireEvent.click(within(editor).getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(settingsRequests()).toHaveLength(1))
    expect(settingsRequests()[0]).toEqual({
      workspace_id: 'ws1',
      settings: {
        ...SETTINGS,
        filters: [
          ...RULES,
          {
            id: '00000000-0000-4000-8000-000000000001',
            order: 4,
            created_at: NOW,
            updated_at: NOW,
            name: 'Google search',
            priority: 500,
            tags: [],
            enabled: true,
            conditions: [{ field: 'utm_source', operator: 'equals', value: 'google' }],
            operations: [{ dimension: 'channel', action: 'set_value', value: 'Search' }]
          }
        ]
      }
    })
    await waitFor(() => expect(refreshWorkspaces).toHaveBeenCalled())
  })

  it('saves an edited rule in place, the others untouched', async () => {
    renderTab()
    fireEvent.click(desktopRow('Newsletter traffic'))
    const editor = panel('Edit rule')
    const name = within(editor).getByPlaceholderText('e.g. Set channel for Google Ads')
    expect(name).toHaveValue('Newsletter traffic')

    fireEvent.change(name, { target: { value: 'Newsletter' } })
    fireEvent.click(within(editor).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(settingsRequests()).toHaveLength(1))
    expect(settingsRequests()[0]).toEqual({
      workspace_id: 'ws1',
      settings: {
        ...SETTINGS,
        filters: [{ ...RULES[0], name: 'Newsletter', updated_at: NOW }, RULES[1], RULES[2]]
      }
    })
  })

  it('sends nothing while a field is missing, and says which', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))
    const editor = panel('Create rule')
    fireEvent.click(within(editor).getByRole('button', { name: 'Create' }))

    expect(await within(editor).findByText('Name is required')).toBeInTheDocument()
    expect(within(editor).getByText('All conditions must have a value')).toBeInTheDocument()
    expect(within(editor).getByText('Every "set" operation needs a value')).toBeInTheDocument()
    expect(settingsRequests()).toHaveLength(0)
  })

  it('tests every rule in a side panel and lists, per rule, whether it matched and what it writes', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Test$/ }))
    const tester = panel('Test all rules')
    expect(tester.style.width).toBe('520px')
    // The field the snippet sends is written beside its label.
    expect(within(tester).getByText('utm_source')).toBeVisible()

    fireEvent.change(within(tester).getByPlaceholderText('UTM Source'), { target: { value: ' newsletter ' } })
    fireEvent.click(within(tester).getByRole('button', { name: 'Run test' }))

    expect(within(tester).getByText('2 of 3 rules match')).toBeInTheDocument()
    const matched = [...tester.querySelectorAll<HTMLElement>('[data-rule-result="matched"]')]
    // Highest priority first, as the engine evaluates them.
    expect(matched.map((row) => row.querySelector('.font-medium')?.textContent)).toEqual([
      'Paused fallback',
      'Newsletter traffic'
    ])
    expect(within(matched[0]).getByText('Disabled')).toBeInTheDocument()
    expect(within(matched[0]).getByText('(always matches)')).toBeInTheDocument()
    expect(matched[0].querySelector('[data-written-value]')).toHaveTextContent('Channel Groupunset')
    expect(within(matched[1]).getByText('newsletter')).toBeInTheDocument()
    expect(matched[1].querySelector('[data-written-value]')).toHaveTextContent('Channelset toEmail')

    // The rules that did not match wait behind a toggle.
    expect(tester.querySelector('[data-rule-result="unmatched"]')).toBeNull()
    fireEvent.click(within(tester).getByRole('button', { name: 'Show' }))
    const unmatched = [...tester.querySelectorAll<HTMLElement>('[data-rule-result="unmatched"]')]
    expect(unmatched).toHaveLength(1)
    expect(unmatched[0]).toHaveTextContent('Paid search')

    // Back returns to the visit as it was typed.
    fireEvent.click(within(tester).getByRole('button', { name: 'Back' }))
    expect(within(tester).getByPlaceholderText('UTM Source')).toHaveValue(' newsletter ')
  })

  it('tests the draft from the editor, condition by condition', async () => {
    renderTab()
    fireEvent.click(desktopRow('Newsletter traffic'))
    fireEvent.click(within(panel('Edit rule')).getByRole('button', { name: /Test$/ }))

    // Opened over the editor, the second panel mounts a frame later.
    await screen.findByRole('heading', { level: 2, name: 'Test rule' })
    const tester = panel('Test rule')
    fireEvent.change(within(tester).getByPlaceholderText('UTM Source'), { target: { value: 'partner' } })
    fireEvent.click(within(tester).getByRole('button', { name: 'Run test' }))

    const [condition] = tester.querySelectorAll<HTMLElement>('[data-condition-result]')
    expect(condition).toHaveTextContent('UTM Source')
    expect(within(condition).getByText('This visit: partner')).toBeInTheDocument()
    expect(within(condition).getByText('No match')).toBeInTheDocument()
    expect(within(tester).queryByText('Values written')).toBeNull()

    fireEvent.click(within(tester).getByRole('button', { name: 'Back' }))
    fireEvent.change(within(tester).getByPlaceholderText('UTM Source'), { target: { value: 'newsletter' } })
    fireEvent.click(within(tester).getByRole('button', { name: 'Run test' }))
    expect(within(tester).getAllByText('Matches')).toHaveLength(2)
    expect(within(tester).getByText('Values written')).toBeInTheDocument()
    expect(within(tester).getByText('Email')).toBeInTheDocument()
    expect(within(tester).getByText('Set value')).toBeInTheDocument()
  })
})
