import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import type { WebFilter } from '../../../services/api/web_analytics'
import { TestFilterPanel, type TestFilterPanelProps } from './TestFilterPanel'

// The real evaluator, watched: how often the tester runs every rule.
const { testAllFiltersSpy } = vi.hoisted(() => ({ testAllFiltersSpy: vi.fn() }))
vi.mock('../lib/filterEvaluator', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/filterEvaluator')>()
  testAllFiltersSpy.mockImplementation(actual.testAllFilters)
  return { ...actual, testAllFilters: testAllFiltersSpy }
})

const RULES: WebFilter[] = [
  {
    id: 'r1',
    name: 'Newsletter traffic',
    priority: 10,
    order: 1,
    conditions: [{ field: 'utm_source', operator: 'equals', value: 'newsletter' }],
    operations: [{ dimension: 'channel', action: 'set_value', value: 'Email' }],
    enabled: true
  },
  {
    id: 'r2',
    name: 'Paused fallback',
    priority: 20,
    order: 2,
    conditions: [],
    operations: [{ dimension: 'channel_group', action: 'set_value', value: 'other' }],
    enabled: false
  },
  {
    id: 'r3',
    name: 'Paid search',
    priority: 30,
    order: 3,
    conditions: [
      { field: 'utm_source', operator: 'equals', value: 'google' },
      { field: 'utm_medium', operator: 'equals', value: 'cpc' }
    ],
    operations: [{ dimension: 'channel', action: 'set_value', value: 'Paid' }],
    enabled: true
  }
]

function renderTester(props: Partial<TestFilterPanelProps> = {}) {
  const onClose = vi.fn()
  render(
    <App>
      <TestFilterPanel open onClose={onClose} mode="all" filters={RULES} {...props} />
    </App>
  )
  const dialog = screen.getByRole('dialog', { name: props.mode === 'single' ? 'Test rule' : 'Test all rules' })
  return { dialog, onClose }
}

/** A field of the visit, by its label and the key the snippet sends. */
const field = (dialog: HTMLElement, label: string, key: string) =>
  within(dialog).getByRole('textbox', { name: `${label} ${key}` }) as HTMLInputElement

const type = (input: HTMLElement, value: string) => fireEvent.change(input, { target: { value } })

/** The counter tabs, as "Matching1". */
const counters = (dialog: HTMLElement) =>
  [...dialog.querySelectorAll('.ant-segmented-item')].map((item) => item.textContent)

const ruleBlocks = (dialog: HTMLElement, kind: string) => [
  ...dialog.querySelectorAll<HTMLElement>(`[data-rule-result="${kind}"]`)
]

describe('TestFilterPanel, all rules', () => {
  it('runs the rules as the visit is typed, with no button to press', async () => {
    const { dialog } = renderTester()
    expect(within(dialog).queryByRole('button', { name: 'Run test' })).toBeNull()
    expect(counters(dialog)).toEqual(['Matching0', 'Not matching2', 'Disabled1'])

    type(field(dialog, 'UTM Source', 'utm_source'), ' newsletter ')

    expect(await within(dialog).findByText('Newsletter traffic')).toBeInTheDocument()
    expect(counters(dialog)).toEqual(['Matching1', 'Not matching1', 'Disabled1'])
    const [matched] = ruleBlocks(dialog, 'matched')
    expect(within(matched).getByText('This visit: newsletter')).toBeInTheDocument()
    expect(matched.querySelector('[data-written-value]')).toHaveTextContent('Channelset toEmail')
  })

  it('shows on a rule that does not match the first condition the visit fails', async () => {
    const { dialog } = renderTester()
    type(field(dialog, 'UTM Source', 'utm_source'), 'newsletter')
    type(field(dialog, 'UTM Medium', 'utm_medium'), 'email')
    await waitFor(() => expect(counters(dialog)[1]).toBe('Not matching2'))
    fireEvent.click(within(dialog).getByText('Not matching', { selector: '.ant-segmented-item *' }))

    const [unmatched] = ruleBlocks(dialog, 'unmatched')
    expect(unmatched).toHaveTextContent('Paid search')
    const failing = unmatched.querySelector('[data-first-failing]') as HTMLElement
    expect(failing).toHaveTextContent('UTM Source')
    expect(within(failing).getByText('No match')).toBeInTheDocument()
    expect(within(failing).getByText('This visit: newsletter')).toBeInTheDocument()
    expect(unmatched.querySelector('[data-written-value]')).toBeNull()
  })

  it('lists a disabled rule apart, not applied and writing nothing', async () => {
    const { dialog } = renderTester()
    fireEvent.click(within(dialog).getByText('Disabled', { selector: '.ant-segmented-item *' }))

    const [disabled] = ruleBlocks(dialog, 'disabled')
    expect(disabled).toHaveTextContent('Paused fallback')
    expect(within(disabled).getByText('Not applied')).toBeInTheDocument()
    expect(disabled.querySelector('[data-written-value]')).toBeNull()
    // It is not counted with the rules that run.
    expect(within(dialog).getByText('2 enabled · 1 disabled')).toBeInTheDocument()
  })

  it('fills the Pages and UTM fields from a link, the referrer left as typed', async () => {
    const { dialog } = renderTester()
    type(field(dialog, 'Referrer', 'referrer'), 'https://www.google.com/search')
    type(field(dialog, 'Landing Path', 'landing_path'), '/old/')
    type(
      within(dialog).getByLabelText('Landing URL'),
      'https://Shop.Example.com/iphone/?utm_source=google&utm_medium=cpc&fbclid=x&GCLID=y'
    )
    fireEvent.click(within(dialog).getByRole('button', { name: 'Fill fields' }))

    expect(field(dialog, 'Landing Page', 'landing_page')).toHaveValue(
      'https://shop.example.com/iphone/?utm_source=google&utm_medium=cpc&fbclid=x&GCLID=y'
    )
    expect(field(dialog, 'Landing Domain', 'landing_domain')).toHaveValue('shop.example.com')
    expect(field(dialog, 'Landing Path', 'landing_path')).toHaveValue('/iphone/')
    expect(field(dialog, 'Current Path', 'path')).toHaveValue('/iphone/')
    expect(field(dialog, 'UTM Source', 'utm_source')).toHaveValue('google')
    expect(field(dialog, 'UTM Medium', 'utm_medium')).toHaveValue('cpc')
    expect(field(dialog, 'UTM Campaign', 'utm_campaign')).toHaveValue('')
    expect(field(dialog, 'UTM ID', 'utm_id')).toHaveValue('y')
    expect(field(dialog, 'UTM ID From', 'utm_id_from')).toHaveValue('gclid')
    expect(field(dialog, 'Referrer', 'referrer')).toHaveValue('https://www.google.com/search')
    // The filled visit runs at once.
    await waitFor(() => expect(counters(dialog)[0]).toBe('Matching1'))
  })

  it('runs the rules again only when the visit changes, not on an unrelated render', async () => {
    const { dialog } = renderTester()
    type(field(dialog, 'UTM Source', 'utm_source'), 'newsletter')
    await waitFor(() => expect(counters(dialog)[0]).toBe('Matching1'))
    const runs = testAllFiltersSpy.mock.calls.length

    // Typing a link and picking a tab render the tester without touching the visit.
    type(within(dialog).getByLabelText('Landing URL'), 'https://example.com/')
    type(within(dialog).getByLabelText('Landing URL'), 'https://example.com/pricing')
    fireEvent.click(within(dialog).getByText('Not matching', { selector: '.ant-segmented-item *' }))
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(testAllFiltersSpy.mock.calls.length).toBe(runs)

    type(field(dialog, 'UTM Medium', 'utm_medium'), 'cpc')
    await waitFor(() => expect(testAllFiltersSpy.mock.calls.length).toBeGreaterThan(runs))
  })

  it('fills an empty landing domain and path for a link the server cannot parse, and says why', async () => {
    const { dialog } = renderTester()
    type(field(dialog, 'Landing Path', 'landing_path'), '/old/')
    type(within(dialog).getByLabelText('Landing URL'), 'https://example.com/100%/off?utm_source=newsletter')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Fill fields' }))

    expect(field(dialog, 'Landing Domain', 'landing_domain')).toHaveValue('')
    expect(field(dialog, 'Landing Path', 'landing_path')).toHaveValue('')
    expect(field(dialog, 'UTM Source', 'utm_source')).toHaveValue('newsletter')
    expect(dialog.querySelector('[data-link-unreadable]')).toHaveTextContent(
      'The server cannot read it and stores an empty landing domain and path'
    )

    // A readable link clears the note.
    type(within(dialog).getByLabelText('Landing URL'), 'https://example.com/off')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Fill fields' }))
    expect(dialog.querySelector('[data-link-unreadable]')).toBeNull()
  })

  it('counts a rule the browser cannot test apart, writing nothing', async () => {
    const untestable: WebFilter = {
      id: 'r4',
      name: 'Google, end anchored',
      priority: 40,
      order: 4,
      conditions: [{ field: 'utm_source', operator: 'regex', value: 'google\\z' }],
      operations: [{ dimension: 'channel', action: 'set_value', value: 'Search' }],
      enabled: true
    }
    const { dialog } = renderTester({ filters: [...RULES, untestable] })
    // An empty value never matches a regex: the browser can say so, and no Can't test tab shows.
    expect(counters(dialog)).toEqual(['Matching0', 'Not matching3', 'Disabled1'])

    type(field(dialog, 'UTM Source', 'utm_source'), 'google')
    await waitFor(() => expect(counters(dialog)).toEqual(['Matching0', 'Not matching2', "Can't test1", 'Disabled1']))
    fireEvent.click(within(dialog).getByText("Can't test", { selector: '.ant-segmented-item *' }))
    const [row] = ruleBlocks(dialog, 'untested')
    expect(row).toHaveTextContent('Google, end anchored')
    expect(within(row).getByText("Can't test in the browser")).toBeInTheDocument()
    expect(within(row).getByText('Not tested')).toBeInTheDocument()
    expect(row.querySelector('[data-written-value]')).toBeNull()
    expect(ruleBlocks(dialog, 'matched')).toHaveLength(0)
  })

  it('says so when the link is not a web address, and fills nothing', async () => {
    const { dialog } = renderTester()
    type(within(dialog).getByLabelText('Landing URL'), 'not a link')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Fill fields' }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('This is not a web address')
    expect(field(dialog, 'Landing Page', 'landing_page')).toHaveValue('')
  })

  it('clears the whole visit without closing', async () => {
    const { dialog, onClose } = renderTester()
    type(field(dialog, 'UTM Source', 'utm_source'), 'newsletter')
    type(field(dialog, 'Referrer', 'referrer'), 'https://www.google.com/')
    await waitFor(() => expect(counters(dialog)[0]).toBe('Matching1'))

    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear visit' }))

    for (const input of within(dialog).getAllByRole('textbox')) expect(input).toHaveValue('')
    await waitFor(() => expect(counters(dialog)).toEqual(['Matching0', 'Not matching2', 'Disabled1']))
    expect(onClose).not.toHaveBeenCalled()
  })

  it('counts the filled fields of each card', async () => {
    const { dialog } = renderTester()
    const utmCard = within(dialog).getByRole('heading', { name: 'UTM' }).closest('section') as HTMLElement
    expect(within(utmCard).getByText('0 of 7 filled')).toBeInTheDocument()
    type(field(dialog, 'UTM Source', 'utm_source'), 'google')
    type(field(dialog, 'UTM Medium', 'utm_medium'), 'cpc')
    expect(await within(utmCard).findByText('2 of 7 filled')).toBeInTheDocument()
  })

  it('stays open on Escape, which would erase the visit', async () => {
    const { dialog, onClose } = renderTester()
    const input = field(dialog, 'UTM Source', 'utm_source')
    input.focus()
    fireEvent.keyDown(input, { key: 'Escape', code: 'Escape', keyCode: 27 })
    fireEvent.keyDown(window, { key: 'Escape', code: 'Escape', keyCode: 27 })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('takes "Is Direct" from a yes/no list, not as free text', async () => {
    const { dialog } = renderTester()
    expect(within(dialog).getByRole('combobox', { name: 'Is Direct is_direct' })).toBeInTheDocument()
    expect(within(dialog).queryByRole('textbox', { name: 'Is Direct is_direct' })).toBeNull()
  })
})

describe('TestFilterPanel, one rule', () => {
  const single: Partial<TestFilterPanelProps> = {
    mode: 'single',
    ruleName: 'Newsletter traffic',
    conditions: RULES[0].conditions,
    operations: RULES[0].operations,
    filters: undefined
  }

  it('tests the draft condition by condition, the values written only once it matches', async () => {
    const { dialog, onClose } = renderTester(single)
    expect(within(dialog).getByText('· Newsletter traffic')).toBeInTheDocument()
    expect(within(dialog).queryByText('Not matching')).toBeNull()

    type(field(dialog, 'UTM Source', 'utm_source'), 'partner')
    expect(await within(dialog).findByText('This visit: partner')).toBeInTheDocument()
    const [condition] = dialog.querySelectorAll<HTMLElement>('[data-condition-result]')
    expect(within(condition).getByText('No match')).toBeInTheDocument()
    // One rule has its result where the all-rules tester puts its counters, so both columns start level.
    expect(within(dialog.querySelector<HTMLElement>('[data-single-result]')!).getByText('No match')).toBeInTheDocument()
    expect(within(dialog).queryByText('Values written')).toBeNull()

    type(field(dialog, 'UTM Source', 'utm_source'), 'newsletter')
    expect(await within(dialog).findByText('Values written')).toBeInTheDocument()
    // The result row facing the hint, the Conditions header and the condition itself.
    expect(within(dialog).getAllByText('Matches')).toHaveLength(3)
    const resultRow = dialog.querySelector<HTMLElement>('[data-single-result]')!
    expect(within(resultRow).getByText('Result')).toBeInTheDocument()
    expect(within(resultRow).getByText('Matches')).toBeInTheDocument()
    expect(within(dialog).getByText('Values written')).toBeInTheDocument()
    expect(within(dialog).getByText('Email')).toBeInTheDocument()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Edit rule' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('says the browser cannot test the rule, and writes nothing, when only an untestable condition is left', async () => {
    const { dialog } = renderTester({
      ...single,
      conditions: [
        { field: 'utm_source', operator: 'regex', value: 'newsletter\\z' },
        { field: 'utm_medium', operator: 'equals', value: 'email' }
      ]
    })
    type(field(dialog, 'UTM Source', 'utm_source'), 'newsletter')
    type(field(dialog, 'UTM Medium', 'utm_medium'), 'email')
    const conditions = () => within(dialog).getByRole('heading', { name: 'Conditions' }).closest('section') as HTMLElement
    const verdict = () => conditions().querySelector('[data-panel-head]') as HTMLElement
    await waitFor(() => expect(verdict()).toHaveTextContent("Can't test in the browser"))
    expect(within(dialog).queryByText('Values written')).toBeNull()
    expect(within(dialog).queryByText('Email')).toBeNull()

    type(field(dialog, 'UTM Medium', 'utm_medium'), 'cpc')
    await waitFor(() => expect(verdict()).toHaveTextContent('No match'))
  })

  it('starts from the values it is given', async () => {
    const { dialog } = renderTester({ ...single, initialValues: { utm_source: 'newsletter' } })
    expect(field(dialog, 'UTM Source', 'utm_source')).toHaveValue('newsletter')
    expect(await within(dialog).findByText('Values written')).toBeInTheDocument()
  })
})
