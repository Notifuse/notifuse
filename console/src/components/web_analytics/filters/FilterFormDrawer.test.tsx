import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { WebAnalyticsBackfillStatus, WebFilter } from '../../../services/api/web_analytics'

// The API client imports the router, which builds the whole route graph at import time.
vi.mock('../../../router', () => ({ router: { navigate: vi.fn() } }))

import { FilterFormDrawer, type FilterFormDrawerProps } from './FilterFormDrawer'

const rule = (id: string, priority: number, overrides: Partial<WebFilter> = {}): WebFilter => ({
  id,
  name: id,
  priority,
  order: 0,
  conditions: [{ field: 'utm_source', operator: 'equals', value: id }],
  operations: [{ dimension: 'channel', action: 'set_value', value: id }],
  enabled: true,
  ...overrides
})

const GOOGLE_ADS: WebFilter = {
  id: 'google',
  name: 'Google Ads (UTM)',
  priority: 830,
  order: 5,
  conditions: [
    { field: 'utm_source', operator: 'regex', value: '^google$' },
    { field: 'utm_medium', operator: 'regex', value: '^(cpc|ppc|paid)$' },
    { field: 'utm_source', operator: 'is_not_empty' }
  ],
  operations: [
    { dimension: 'channel_group', action: 'set_value', value: 'search-paid' },
    { dimension: 'channel', action: 'set_value', value: 'google-ads' }
  ],
  enabled: true
}

const RULES: WebFilter[] = [
  rule('Reddit Ads', 833),
  rule('Quora Ads', 831),
  GOOGLE_ADS,
  rule('Microsoft Ads', 820),
  rule('Facebook Ads', 810),
  rule('Product line', 450, { operations: [{ dimension: 'custom_1', action: 'set_value', value: 'iPhone' }] })
]

let backfill: WebAnalyticsBackfillStatus | null = null

const fetchMock = vi.fn(async (url: string) => {
  const body = url.endsWith('/api/webAnalytics.backfillStatus') ? { backfill } : {}
  return { ok: true, status: 200, json: async () => body }
})

/** Every endpoint the editor called. */
const calledEndpoints = () => fetchMock.mock.calls.map(([url]) => String(url).replace(/^.*\/api\//, ''))

function renderEditor(props: Partial<FilterFormDrawerProps> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onSubmit = vi.fn()
  const onClose = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <App>
        <FilterFormDrawer
          open
          filter={GOOGLE_ADS}
          rules={RULES}
          workspaceId="ws1"
          existingTags={[]}
          onClose={onClose}
          onSubmit={onSubmit}
          {...props}
        />
      </App>
    </QueryClientProvider>
  )
  return { onSubmit, onClose }
}

const editor = () => screen.getByRole('dialog', { name: 'Edit rule' })
const card = (title: string) =>
  within(editor()).getByRole('heading', { name: title }).closest('section') as HTMLElement

describe('FilterFormDrawer', () => {
  beforeEach(() => {
    backfill = null
    fetchMock.mockClear()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('quick-tests only the fields the rule reads, in the browser', async () => {
    renderEditor()
    const quick = card('Quick test')
    // One input per field read, utm_source once although two conditions read it.
    expect(within(quick).getAllByRole('textbox').map((input) => input.id)).toEqual([
      'quick-test-utm_source',
      'quick-test-utm_medium'
    ])
    const verdict = () => quick.querySelector('[data-panel-head]') as HTMLElement
    expect(verdict()).toHaveTextContent('No match')

    fireEvent.change(within(quick).getByLabelText('UTM Source utm_source'), { target: { value: 'google' } })
    fireEvent.change(within(quick).getByLabelText('UTM Medium utm_medium'), { target: { value: 'cpc' } })

    expect(verdict()).toHaveTextContent('Matches')
    expect(within(quick).queryByText('No match')).toBeNull()
    expect(within(quick).getByText('search-paid')).toBeInTheDocument()
    await waitFor(() => expect(calledEndpoints()).toContain('webAnalytics.backfillStatus'))
    expect(calledEndpoints().filter((endpoint) => endpoint !== 'webAnalytics.backfillStatus')).toEqual([])
  })

  it('lists the rules writing the same dimensions in engine order, and moves this one with its priority', async () => {
    renderEditor()
    const order = card('Rule order')
    const names = () =>
      [...order.querySelectorAll('tbody tr')].map((row) => row.querySelector('td:nth-child(2)')?.textContent)
    expect(names()).toEqual(['Reddit Ads', 'Quora Ads', 'Google Ads (UTM)This rule', 'Microsoft Ads', 'Facebook Ads'])
    // Product line writes a custom dimension only: not a neighbour.
    expect(within(order).queryByText('Product line')).toBeNull()
    expect(within(order).getByText('2 rules above and 2 below this one')).toBeInTheDocument()

    fireEvent.change(within(card('Details')).getByRole('spinbutton'), { target: { value: '832' } })
    await waitFor(() =>
      expect(names()).toEqual(['Reddit Ads', 'Google Ads (UTM)This rule', 'Quora Ads', 'Microsoft Ads'])
    )
    expect(within(order).getByText('1 rule above and 3 below this one')).toBeInTheDocument()
  })

  it('warns about a regular expression that does not compile, until it is fixed', async () => {
    renderEditor()
    const regex = () => card('Checks').querySelector('[data-check="regex"]') as HTMLElement
    expect(regex()).toHaveAttribute('data-ok', 'true')
    expect(regex()).toHaveTextContent('The 2 regular expressions are valid')

    const [pattern] = within(card('Conditions')).getAllByPlaceholderText('Regular expression')
    fireEvent.change(pattern, { target: { value: '([' } })
    await waitFor(() => expect(regex()).toHaveAttribute('data-ok', 'false'))
    expect(regex()).toHaveTextContent('Condition 1 is not a valid regular expression and may be refused on save')

    fireEvent.change(pattern, { target: { value: '(google)' } })
    await waitFor(() => expect(regex()).toHaveAttribute('data-ok', 'true'))
  })

  it('says a regex in syntax only the server reads cannot be tested, and the Quick test skips it', async () => {
    renderEditor()
    const [pattern] = within(card('Conditions')).getAllByPlaceholderText('Regular expression')
    fireEvent.change(pattern, { target: { value: 'google\\z' } })
    const regex = () => card('Checks').querySelector('[data-check="regex"]') as HTMLElement
    await waitFor(() =>
      expect(regex()).toHaveTextContent("Condition 1 can't be tested in the browser; the server checks it on save")
    )
    expect(regex()).toHaveAttribute('data-tone', 'info')

    const quick = card('Quick test')
    fireEvent.change(within(quick).getByLabelText('UTM Source utm_source'), { target: { value: 'bing' } })
    fireEvent.change(within(quick).getByLabelText('UTM Medium utm_medium'), { target: { value: 'cpc' } })
    const [first] = quick.querySelectorAll<HTMLElement>('[data-condition-result]')
    expect(within(first).getByText('Not tested')).toBeInTheDocument()
    expect(within(first).queryByText('No match')).toBeNull()
    // Every other condition holds: the rule's result is the server's to say, and nothing is written.
    const verdict = () => quick.querySelector('[data-panel-head]') as HTMLElement
    expect(verdict()).toHaveTextContent("Can't test in the browser")
    expect(verdict()).not.toHaveTextContent('Matches')
    expect(within(quick).queryByText('Values written')).toBeNull()

    // Another condition fails: no match, whatever the untested one says.
    fireEvent.change(within(quick).getByLabelText('UTM Medium utm_medium'), { target: { value: 'email' } })
    expect(verdict()).toHaveTextContent('No match')
    expect(verdict()).not.toHaveTextContent("Can't test in the browser")
  })

  it('splits the rules that override this one into those run first and those run later, ignoring a higher default and a disabled rule', () => {
    const rules = [
      rule('Higher default', 900, {
        operations: [{ dimension: 'channel', action: 'set_default_value', value: 'other' }]
      }),
      rule('Disabled higher', 900, { enabled: false }),
      rule('Higher set', 850),
      GOOGLE_ADS,
      rule('Equal, later', 830)
    ]
    renderEditor({ rules })
    const checks = card('Checks')
    const before = checks.querySelector('[data-check="priority-before"]') as HTMLElement
    expect(before).toHaveTextContent('1 rule runs first and can keep this rule from writing Channel')
    expect(before).toHaveTextContent("When one of them also matches a session, this rule's value is not written.")
    const after = checks.querySelector('[data-check="priority-after"]') as HTMLElement
    expect(after).toHaveTextContent('1 rule runs later and can overwrite this rule on Channel')
    expect(after).toHaveTextContent("When one of them also matches a session, its value replaces this rule's.")
  })

  it('says a disabled rule does not run, with no override warning', () => {
    renderEditor({ rules: [rule('Higher set', 850), GOOGLE_ADS], filter: { ...GOOGLE_ADS, enabled: false } })
    const checks = card('Checks')
    expect(checks.querySelector('[data-check="priority"]')).toHaveTextContent(
      'This rule is disabled: it does not run, so it writes nothing'
    )
    expect(checks.querySelector('[data-check^="priority-"]')).toBeNull()
  })

  it('places nothing and warns of nothing while the priority is cleared', async () => {
    renderEditor()
    fireEvent.change(within(card('Details')).getByRole('spinbutton'), { target: { value: '' } })
    await waitFor(() =>
      expect(card('Checks').querySelector('[data-check="priority"]')).toHaveTextContent(
        'Set a priority to see which rules override this one'
      )
    )
    expect(within(card('Rule order')).getByText('Set a priority to place this rule among the others.')).toBeInTheDocument()
    expect(card('Rule order').querySelector('tbody')).toBeNull()
  })

  it('reads the last backfill, a failure with its message', async () => {
    backfill = { task_id: 't1', status: 'failed', progress: 40, error_message: 'partition 3 timed out' }
    renderEditor()
    const applies = card('When it applies')
    expect(await within(applies).findByText('Failed')).toBeInTheDocument()
    expect(within(applies).getByText('partition 3 timed out')).toBeInTheDocument()
    expect(within(applies).getByText('From the moment you save')).toBeInTheDocument()
    // Read only: nothing starts or cancels a run from here.
    expect(within(applies).queryByRole('button')).toBeNull()
  })

  it('reads a cancelled backfill as cancelled, in a neutral pill, not as a failure', async () => {
    // The server records a cancelled run as failed, with this message.
    backfill = { task_id: 't1', status: 'failed', progress: 40, cancelled: true, error_message: 'cancelled by user' }
    renderEditor()
    const applies = card('When it applies')
    const pill = await within(applies).findByText('Cancelled')
    expect(pill.closest('.bg-neutral-bg')).not.toBeNull()
    expect(within(applies).queryByText('Failed')).toBeNull()
    expect(within(applies).queryByText('cancelled by user')).toBeNull()
    expect(applies.querySelector('.bg-danger-bg')).toBeNull()
  })

  it('opens the tester over the editor, which stays mounted, with the Quick test values', async () => {
    renderEditor()
    fireEvent.change(within(card('Quick test')).getByLabelText('UTM Source utm_source'), {
      target: { value: 'google' }
    })
    fireEvent.click(within(card('Quick test')).getByRole('button', { name: 'Open tester' }))

    const tester = await screen.findByRole('dialog', { name: 'Test rule' })
    expect(within(tester).getByRole('textbox', { name: 'UTM Source utm_source' })).toHaveValue('google')
    expect(within(tester).getByText('· Google Ads (UTM)')).toBeInTheDocument()
    expect(screen.getByRole('dialog', { name: 'Edit rule' })).toBeInTheDocument()

    fireEvent.click(within(tester).getByRole('button', { name: 'Edit rule' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Test rule' })).toBeNull())
    expect(screen.getByRole('dialog', { name: 'Edit rule' })).toBeInTheDocument()
  })
})
