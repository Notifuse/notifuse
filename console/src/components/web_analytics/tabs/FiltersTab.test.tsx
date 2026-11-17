import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { WebAnalyticsSettings, WebFilter } from '../../../services/api/web_analytics'

// The API client imports the router, which builds the whole route graph at import time.
vi.mock('../../../router', () => ({ router: { navigate: vi.fn() } }))

const { refreshWorkspaces, access } = vi.hoisted(() => ({
  refreshWorkspaces: vi.fn(),
  access: { write: true, role: 'member' as string | undefined, loading: false }
}))
vi.mock('../../../contexts/AuthContext', () => ({
  useAuth: () => ({ refreshWorkspaces }),
  // While loading, the hook answers no permissions and no role, as AuthContext does.
  useWorkspacePermissions: () =>
    access.loading
      ? { permissions: null, role: undefined, loading: true }
      : { permissions: { web_analytics: { read: true, write: access.write } }, role: access.role, loading: false }
}))

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

let currentSettings = SETTINGS

vi.mock('../useWebAnalytics', () => ({
  useWebAnalytics: () => ({
    workspaceId: 'ws1',
    settings: currentSettings,
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

/** The open drawer named `name`. */
const panel = (name: string) => screen.getByRole('dialog', { name })

/** A card of the editor, by its title. */
const card = (editor: HTMLElement, title: string) =>
  within(editor).getByRole('heading', { name: title }).closest('section') as HTMLElement

/** Every endpoint the console posted to, in order. */
const requestedEndpoints = () => fetchMock.mock.calls.map(([url]) => String(url).replace(/^.*\/api\//, ''))

const NO_WRITE = "You don't have write permission for web analytics"

const desktopRow = (name: string) => {
  const desktop = document.querySelector('.hidden.md\\:block') as HTMLElement
  return within(desktop).getByText(name)
}

describe('FiltersTab', () => {
  beforeEach(() => {
    fetchMock.mockClear()
    refreshWorkspaces.mockReset().mockResolvedValue(undefined)
    access.write = true
    access.role = 'member'
    access.loading = false
    currentSettings = SETTINGS
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

  it('opens the rule editor full width, conditions and operations side by side', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))

    const editor = panel('New rule')
    expect(editor.closest('.app-full-drawer')).not.toBeNull()
    const sections = [...editor.querySelectorAll('[data-panel-head] h3')].map((head) => head.textContent)
    expect(sections).toEqual(['Details', 'Conditions', 'Operations', 'Rule order', 'Checks', 'Quick test', 'When it applies'])
    // Conditions and operations share a row from 1200px.
    const conditions = within(editor).getByRole('heading', { name: 'Conditions' }).closest('section') as HTMLElement
    const operations = within(editor).getByRole('heading', { name: 'Operations' }).closest('section') as HTMLElement
    expect(conditions.parentElement).toBe(operations.parentElement)
    expect(conditions.parentElement).toHaveClass('min-[1200px]:grid-cols-2')
    // The priority's help says how the engine settles two rules writing the same dimension.
    expect(
      within(editor).getByText(
        'Higher priority rules are evaluated first (0-1000). On the same dimension, the higher priority wins; at equal priority, the rule lower in the list wins.'
      )
    ).toBeVisible()
    expect(editor.querySelectorAll('[data-condition-row]')).toHaveLength(1)
    expect(editor.querySelectorAll('[data-operation-row]')).toHaveLength(1)
    expect(within(editor).getByText('Not created')).toBeInTheDocument()
  })

  it('creates a rule with the same settings body as before', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))
    const editor = panel('New rule')

    fireEvent.change(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads'), {
      target: { value: '  Google search  ' }
    })
    const [conditionValue, operationValue] = within(editor).getAllByPlaceholderText('Value')
    fireEvent.change(conditionValue, { target: { value: 'google' } })
    fireEvent.change(operationValue, { target: { value: 'Search' } })
    fireEvent.click(within(editor).getByRole('button', { name: 'Create rule' }))

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
    await waitFor(() => expect(within(editor).getByRole('button', { name: 'Save changes' })).toBeEnabled())
    fireEvent.click(within(editor).getByRole('button', { name: 'Save changes' }))

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
    const editor = panel('New rule')
    fireEvent.click(within(editor).getByRole('button', { name: 'Create rule' }))

    expect(await within(editor).findByText('Name is required')).toBeInTheDocument()
    expect(within(card(editor, 'Conditions')).getByText('All conditions must have a value')).toBeInTheDocument()
    expect(within(card(editor, 'Operations')).getByText('Every "set" operation needs a value')).toBeInTheDocument()
    expect(settingsRequests()).toHaveLength(0)
  })

  it('clears a condition or operation error once its value is typed, and marks only the empty field', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))
    const editor = panel('New rule')
    fireEvent.click(within(editor).getByRole('button', { name: 'Create rule' }))
    expect(await within(card(editor, 'Conditions')).findByText('All conditions must have a value')).toBeInTheDocument()

    const [conditionValue, operationValue] = within(editor).getAllByPlaceholderText('Value')
    // The empty value is the field in error; the field and operator picked beside it are not.
    expect(conditionValue).toHaveClass('ant-input-status-error')
    const conditionRow = editor.querySelector('[data-condition-row]') as HTMLElement
    expect(conditionRow.querySelector('.ant-select-status-error')).toBeNull()

    fireEvent.change(conditionValue, { target: { value: 'google' } })
    await waitFor(() =>
      expect(within(card(editor, 'Conditions')).queryByText('All conditions must have a value')).toBeNull()
    )
    expect(conditionValue).not.toHaveClass('ant-input-status-error')
    // The operation still has no value, so its error stays until it gets one.
    expect(within(card(editor, 'Operations')).getByText('Every "set" operation needs a value')).toBeInTheDocument()
    fireEvent.change(operationValue, { target: { value: 'Search' } })
    await waitFor(() =>
      expect(within(card(editor, 'Operations')).queryByText('Every "set" operation needs a value')).toBeNull()
    )
    expect(settingsRequests()).toHaveLength(0)
  })

  it('writes "Is Direct" as true or false, picked from a list rather than typed', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))
    const editor = panel('New rule')
    fireEvent.change(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads'), {
      target: { value: 'Newsletter is not direct' }
    })
    fireEvent.change(within(editor).getAllByPlaceholderText('Value')[0], { target: { value: 'newsletter' } })

    const operation = editor.querySelector('[data-operation-row]') as HTMLElement
    const dimension = within(operation).getByRole('combobox', { name: 'Dimension' })
    fireEvent.mouseDown(dimension)
    fireEvent.change(dimension, { target: { value: 'Is Direct' } })
    fireEvent.click(await screen.findByTitle('Is Direct'))

    // No text box to type "No" or "FALSE" in: the value is one of the two the engine compares.
    expect(within(operation).queryByPlaceholderText('Value')).toBeNull()
    const value = within(operation).getByRole('combobox', { name: 'Value' })
    fireEvent.mouseDown(value)
    fireEvent.click(await screen.findByTitle('No'))
    fireEvent.click(within(editor).getByRole('button', { name: 'Create rule' }))

    await waitFor(() => expect(settingsRequests()).toHaveLength(1))
    const created = settingsRequests()[0].settings.filters.at(-1)
    expect(created.operations).toEqual([{ dimension: 'is_direct', action: 'set_value', value: 'false' }])
  })

  it('tests every rule in a full-width drawer, live, and lists per rule whether it matched and what it writes', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Test$/ }))
    const tester = await screen.findByRole('dialog', { name: 'Test all rules' })
    expect(tester.closest('.app-full-drawer')).not.toBeNull()
    // The field the snippet sends is written beside its label.
    expect(within(tester).getByText('utm_source')).toBeVisible()
    expect(within(tester).queryByRole('button', { name: 'Run test' })).toBeNull()

    fireEvent.change(within(tester).getByRole('textbox', { name: 'UTM Source utm_source' }), {
      target: { value: ' newsletter ' }
    })

    // Only the enabled rules run on a visit: the disabled one is neither counted nor matched.
    const counters = () => [...tester.querySelectorAll('.ant-segmented-item')].map((item) => item.textContent)
    await waitFor(() => expect(counters()).toEqual(['Matching1', 'Not matching1', 'Disabled1']))
    const matched = [...tester.querySelectorAll<HTMLElement>('[data-rule-result="matched"]')]
    expect(matched.map((row) => row.querySelector('[data-rule-name]')?.textContent)).toEqual(['Newsletter traffic'])
    expect(within(matched[0]).getByText('newsletter')).toBeInTheDocument()
    expect(matched[0].querySelector('[data-written-value]')).toHaveTextContent('Channelset toEmail')

    // The disabled rule is listed apart, greyed, said not applied, and writes nothing.
    fireEvent.click(within(tester).getByText('Disabled', { selector: '.ant-segmented-item *' }))
    const disabled = [...tester.querySelectorAll<HTMLElement>('[data-rule-result="disabled"]')]
    expect(disabled).toHaveLength(1)
    expect(disabled[0]).toHaveTextContent('Paused fallback')
    expect(within(disabled[0]).getByText('Not applied')).toBeInTheDocument()
    expect(disabled[0]).toHaveClass('opacity-60')
    expect(disabled[0].querySelector('[data-written-value]')).toBeNull()

    fireEvent.click(within(tester).getByText('Not matching', { selector: '.ant-segmented-item *' }))
    const unmatched = [...tester.querySelectorAll<HTMLElement>('[data-rule-result="unmatched"]')]
    expect(unmatched).toHaveLength(1)
    expect(unmatched[0]).toHaveTextContent('Paid search')
  })

  it('takes "Is Direct" from a yes/no list in the tester, not as free text', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Test$/ }))
    const tester = await screen.findByRole('dialog', { name: 'Test all rules' })
    const item = within(tester).getByText('is_direct').closest('.ant-form-item') as HTMLElement
    expect(item.querySelector('.ant-select')).not.toBeNull()
    expect(item.querySelector('input.ant-input')).toBeNull()
  })

  it('tests the draft from the editor, condition by condition', async () => {
    renderTab()
    fireEvent.click(desktopRow('Newsletter traffic'))
    fireEvent.click(within(panel('Edit rule')).getByRole('button', { name: 'Open tester' }))

    const tester = await screen.findByRole('dialog', { name: 'Test rule' })
    const source = within(tester).getByRole('textbox', { name: 'UTM Source utm_source' })
    fireEvent.change(source, { target: { value: 'partner' } })

    expect(await within(tester).findByText('This visit: partner')).toBeInTheDocument()
    const [condition] = tester.querySelectorAll<HTMLElement>('[data-condition-result]')
    expect(condition).toHaveTextContent('UTM Source')
    expect(within(condition).getByText('No match')).toBeInTheDocument()
    expect(within(tester).queryByText('Values written')).toBeNull()

    fireEvent.change(source, { target: { value: 'newsletter' } })
    expect(await within(tester).findByText('Values written')).toBeInTheDocument()
    // The result row, the Conditions header and the condition.
    expect(within(tester).getAllByText('Matches')).toHaveLength(3)
    expect(within(tester).getByText('Email')).toBeInTheDocument()
    expect(within(tester).getByText('Set value')).toBeInTheDocument()
  })

  it('asks before closing an edited rule, and sends nothing', async () => {
    renderTab()
    fireEvent.click(desktopRow('Newsletter traffic'))
    const editor = panel('Edit rule')
    fireEvent.change(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads'), {
      target: { value: 'Newsletter' }
    })
    expect(await within(editor).findByText('Unsaved changes')).toBeInTheDocument()

    fireEvent.click(within(editor).getByRole('button', { name: 'Close' }))
    expect((await screen.findAllByText('Discard your changes?')).length).toBeGreaterThan(0)
    expect(screen.getByText('The changes you made to this rule will be lost.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    await waitFor(() => expect(screen.queryAllByText('Discard your changes?')).toHaveLength(0))
    expect(panel('Edit rule')).toBeInTheDocument()

    // The back button asks the same.
    fireEvent.click(within(editor).getByRole('button', { name: 'Attribution rules' }))
    await screen.findByText('The changes you made to this rule will be lost.')
    const confirm = document.querySelector('.ant-modal-confirm') as HTMLElement
    fireEvent.click(within(confirm).getByRole('button', { name: 'Discard' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit rule' })).toBeNull())
    expect(settingsRequests()).toHaveLength(0)
  })

  it('closes an untouched rule at once', async () => {
    renderTab()
    fireEvent.click(desktopRow('Newsletter traffic'))
    fireEvent.click(within(panel('Edit rule')).getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit rule' })).toBeNull())
    expect(screen.queryAllByText('Discard your changes?')).toHaveLength(0)
  })

  it('saves only once something changed, and Discard restores the rule as saved', async () => {
    renderTab()
    fireEvent.click(desktopRow('Newsletter traffic'))
    const editor = panel('Edit rule')
    expect(within(editor).getByRole('button', { name: 'Save changes' })).toBeDisabled()
    expect(within(editor).queryByRole('button', { name: 'Discard' })).toBeNull()

    const name = within(editor).getByPlaceholderText('e.g. Set channel for Google Ads')
    fireEvent.change(name, { target: { value: 'Newsletter' } })
    await waitFor(() => expect(within(editor).getByRole('button', { name: 'Save changes' })).toBeEnabled())

    fireEvent.click(within(editor).getByRole('button', { name: 'Discard' }))
    // The reset mounts the field again.
    await waitFor(() =>
      expect(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads')).toHaveValue('Newsletter traffic')
    )
    await waitFor(() => expect(within(editor).getByRole('button', { name: 'Save changes' })).toBeDisabled())
    expect(within(editor).queryByText('Unsaved changes')).toBeNull()
  })

  it('stays open on Escape', async () => {
    renderTab()
    fireEvent.click(desktopRow('Newsletter traffic'))
    const editor = panel('Edit rule')
    const name = within(editor).getByPlaceholderText('e.g. Set channel for Google Ads')
    name.focus()
    fireEvent.keyDown(name, { key: 'Escape', code: 'Escape', keyCode: 27 })
    fireEvent.keyDown(window, { key: 'Escape', code: 'Escape', keyCode: 27 })
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(screen.getByRole('dialog', { name: 'Edit rule' })).toBeInTheDocument()
  })

  it('saves a rule without conditions, which always matches', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('button', { name: /Add$/ }))
    const editor = panel('New rule')
    fireEvent.change(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads'), {
      target: { value: 'Everything else' }
    })
    fireEvent.change(within(card(editor, 'Operations')).getByPlaceholderText('Value'), { target: { value: 'Other' } })
    fireEvent.click(within(editor).getByRole('button', { name: 'Delete condition' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(editor.querySelectorAll('[data-condition-row]')).toHaveLength(0))
    expect(within(editor).getByText('Always matches: with no condition, this rule runs on every session.')).toBeInTheDocument()
    expect(editor.querySelector('[data-check="always"]')).toHaveTextContent('Always matches')

    fireEvent.click(within(editor).getByRole('button', { name: 'Create rule' }))
    await waitFor(() => expect(settingsRequests()).toHaveLength(1))
    const created = settingsRequests()[0].settings.filters.at(-1)
    expect(created.conditions).toEqual([])
    expect(created.operations).toEqual([{ dimension: 'channel', action: 'set_value', value: 'Other' }])
  })

  it('saves an edited rule that has no condition, as the seeded default rules', async () => {
    renderTab()
    fireEvent.click(desktopRow('Paused fallback'))
    const editor = panel('Edit rule')
    fireEvent.change(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads'), {
      target: { value: 'Default Channel' }
    })
    await waitFor(() => expect(within(editor).getByRole('button', { name: 'Save changes' })).toBeEnabled())
    fireEvent.click(within(editor).getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(settingsRequests()).toHaveLength(1))
    expect(settingsRequests()[0].settings.filters[1]).toEqual({
      ...RULES[1],
      name: 'Default Channel',
      updated_at: NOW
    })
  })

  it('opens a rule read only for a member who cannot write, and never saves', async () => {
    access.write = false
    renderTab()
    expect(screen.queryByRole('button', { name: /Add$/ })).toBeNull()

    fireEvent.click(desktopRow('Newsletter traffic'))
    const editor = panel('Edit rule')
    expect(within(editor).getByText('Read only')).toBeInTheDocument()
    expect(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads')).toBeDisabled()
    expect(within(editor).getAllByPlaceholderText('Value')[0]).toBeDisabled()
    expect(within(editor).getByRole('button', { name: /Add condition$/ })).toBeDisabled()
    const save = within(editor).getByRole('button', { name: 'Save changes' })
    expect(save).toBeDisabled()
    fireEvent.click(save)

    // The tester stays available.
    fireEvent.click(within(editor).getByRole('button', { name: 'Open tester' }))
    expect(await screen.findByRole('dialog', { name: 'Test rule' })).toBeInTheDocument()
    expect(settingsRequests()).toHaveLength(0)
  })

  it('keeps the toggle, delete and backfill disabled for a member who cannot write, and sends nothing', async () => {
    access.write = false
    renderTab()
    const row = desktopRow('Newsletter traffic').closest('tr') as HTMLElement
    const toggle = within(row).getByRole('switch')
    const remove = within(row).getByRole('button', { name: 'Delete rule' })
    const backfill = await screen.findByRole('button', { name: /Run backfill/ })
    expect(toggle).toBeDisabled()
    expect(remove).toBeDisabled()
    expect(backfill).toBeDisabled()

    fireEvent.mouseEnter(remove.parentElement as HTMLElement)
    expect(await screen.findByText(NO_WRITE)).toBeInTheDocument()
    fireEvent.mouseEnter(backfill.parentElement as HTMLElement)
    await waitFor(() => expect(screen.getAllByText(NO_WRITE).length).toBeGreaterThan(1))

    for (const control of [toggle, remove, backfill]) {
      fireEvent.click(control)
      fireEvent.click(control.parentElement as HTMLElement)
    }
    expect(screen.queryByRole('button', { name: 'Yes' })).toBeNull()
    expect(screen.queryByText('Delete this rule?')).toBeNull()
    // Nothing reaches a write endpoint: the status read is the only call.
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(requestedEndpoints().filter((endpoint) => endpoint !== 'webAnalytics.backfillStatus')).toEqual([])
  })

  it('lets the owner toggle a rule and start a backfill without the explicit permission', async () => {
    access.write = false
    access.role = 'owner'
    renderTab()
    const row = desktopRow('Newsletter traffic').closest('tr') as HTMLElement
    const toggle = within(row).getByRole('switch')
    expect(toggle).toBeEnabled()
    expect(within(row).getByRole('button', { name: 'Delete rule' })).toBeEnabled()
    fireEvent.click(toggle)
    fireEvent.click(await screen.findByRole('button', { name: 'Yes' }))
    await waitFor(() => expect(settingsRequests()).toHaveLength(1))
    expect(settingsRequests()[0].settings.filters.find((rule: WebFilter) => rule.id === 'r1').enabled).toBe(
      false
    )

    const backfill = await screen.findByRole('button', { name: /Run backfill/ })
    expect(backfill).toBeEnabled()
    fireEvent.click(backfill)
    await waitFor(() => expect(requestedEndpoints()).toContain('webAnalytics.backfillStart'))
  })

  it('says nothing about read only while the permissions load, then lets the owner write', async () => {
    access.loading = true
    access.role = 'owner'
    const view = renderTab()
    const row = desktopRow('Newsletter traffic').closest('tr') as HTMLElement
    const toggle = within(row).getByRole('switch')
    const remove = within(row).getByRole('button', { name: 'Delete rule' })
    // Unknown is not read only: disabled for now, with no reason given, and no backfill banner.
    expect(toggle).toBeDisabled()
    fireEvent.mouseEnter(remove.parentElement as HTMLElement)
    fireEvent.mouseEnter(toggle.parentElement as HTMLElement)
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(screen.queryByText(NO_WRITE)).toBeNull()
    expect(screen.queryByRole('button', { name: /Run backfill/ })).toBeNull()

    fireEvent.click(desktopRow('Newsletter traffic'))
    const editor = panel('Edit rule')
    expect(within(editor).queryByText('Read only')).toBeNull()
    expect(within(editor).getByPlaceholderText('e.g. Set channel for Google Ads')).toBeEnabled()
    fireEvent.click(within(editor).getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit rule' })).toBeNull())

    // The answer lands: the owner writes.
    access.loading = false
    view.rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <FiltersTab />
        </App>
      </QueryClientProvider>
    )
    expect(within(desktopRow('Newsletter traffic').closest('tr') as HTMLElement).getByRole('switch')).toBeEnabled()
    expect(screen.getByRole('button', { name: /Add$/ })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /Run backfill/ })).toBeEnabled()
    expect(screen.queryByText(NO_WRITE)).toBeNull()
  })

  it('offers no rule creation on an empty list to a member who cannot write', () => {
    access.write = false
    currentSettings = { ...SETTINGS, filters: [] }
    renderTab()
    expect(screen.getByText(/No attribution rules yet/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Create rule/ })).toBeNull()
  })
})
