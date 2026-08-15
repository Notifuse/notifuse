import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import ButtonUpsertSegment from './button_upsert'
import type { PreviewSegmentRequest, Segment, TreeNode } from '../../services/api/segment'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const { previewSegmentMock } = vi.hoisted(() => ({ previewSegmentMock: vi.fn() }))

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useParams: () => ({ workspaceId: 'ws1' }),
    useNavigate: () => vi.fn(),
    useMatch: () => false
  }
})

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', settings: { timezone: 'UTC', custom_field_labels: {} } }]
  })
}))

vi.mock('../../services/api/list', () => ({
  listsApi: {
    list: vi.fn().mockResolvedValue({
      lists: [
        { id: 'newsletter', name: 'Newsletter' },
        { id: 'product', name: 'Product updates' }
      ]
    })
  }
}))

vi.mock('../../services/api/template', () => ({
  templatesApi: { list: vi.fn().mockResolvedValue({ templates: [] }) }
}))

vi.mock('../../services/api/segment', async () => {
  const actual = await vi.importActual('../../services/api/segment')
  return {
    ...actual,
    previewSegment: previewSegmentMock,
    getSegment: vi.fn().mockRejectedValue(new Error('not found')),
    createSegment: vi.fn(),
    updateSegment: vi.fn()
  }
})

const countOf = (total: number) => ({
  emails: [],
  total_count: total,
  limit: 100,
  generated_sql: 'SELECT 1',
  sql_args: []
})

const listTree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contact_lists',
          contact_list: { operator: 'in', list_id: 'newsletter', status: 'active' }
        }
      }
    ]
  }
}

const goalTree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'custom_events_goals',
          custom_events_goal: {
            goal_type: 'purchase',
            negate: false,
            aggregate_operator: 'count',
            operator: 'gte',
            value: 1,
            timeframe_operator: 'anytime',
            timeframe_values: []
          }
        }
      }
    ]
  }
}

const segmentWith = (tree: TreeNode): Segment => ({
  id: 'big_spenders',
  name: 'Big spenders',
  color: 'blue',
  tree,
  timezone: 'UTC',
  version: 1,
  status: 'active',
  db_created_at: '2026-01-01T00:00:00Z',
  db_updated_at: '2026-01-01T00:00:00Z'
})

const openDrawer = (tree: TreeNode) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

  const result = render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <ButtonUpsertSegment segment={segmentWith(tree)} totalContacts={10000} />
      </I18nProvider>
    </QueryClientProvider>
  )

  fireEvent.click(screen.getByText('Edit segment'))
  return result
}

/**
 * The select currently displaying `value`, addressed by what it shows rather than by position.
 * Both the control and the matching dropdown option carry the value as a title, so the combobox
 * they wrap is what tells them apart.
 */
const selectShowing = (value: string) => {
  const control = screen
    .getAllByTitle(value)
    .find((element) => element.querySelector('[role="combobox"]'))

  if (!control) throw new Error(`no select is showing "${value}"`)
  return control
}

const editFirstCondition = () =>
  fireEvent.click(document.querySelectorAll('[data-icon="pen-to-square"]')[0])

const previewedTrees = () =>
  previewSegmentMock.mock.calls.map((call) => (call[0] as PreviewSegmentRequest).tree)

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// Suite-wide timeout: these exercise the preview debounce against real timers, so each test
// spends most of its budget waiting on purpose — several already allow 3000ms per assertion.
// The slowest lands ~2.1s locally, which the GitHub runner stretches past vitest's 5s default
// (observed CI/local ratios across this suite range 1.4x to 4.5x depending on contention).
// Raised here rather than globally so the rest of the suite keeps a tight bound.
describe('segment drawer — contacts preview', { timeout: 20_000 }, () => {
  beforeEach(() => {
    previewSegmentMock.mockReset()
    previewSegmentMock.mockResolvedValue(countOf(1243))
    vi.mocked(message.error).mockClear()
  })

  it('counts the segment as soon as the drawer opens', async () => {
    openDrawer(listTree)

    expect(await screen.findByText('1243 contacts')).toBeTruthy()
    expect(previewSegmentMock).toHaveBeenCalledTimes(1)
    expect(previewedTrees()[0]).toEqual(listTree)
  })

  it('refreshes the count from a condition that is still open in its form', async () => {
    openDrawer(listTree)
    await screen.findByText('1243 contacts')

    previewSegmentMock.mockResolvedValue(countOf(87))
    editFirstCondition()

    fireEvent.mouseDown(selectShowing('Newsletter'))
    fireEvent.click(await screen.findByTitle('Product updates'))

    // Nothing was confirmed, yet the count follows the condition being written
    expect(await screen.findByText('87 contacts', undefined, { timeout: 3000 })).toBeTruthy()

    const previewed = previewedTrees()
    expect(previewed[previewed.length - 1].branch?.leaves[0].leaf?.contact_list?.list_id).toBe(
      'product'
    )
  })

  it('asks once for a burst of edits', async () => {
    openDrawer(listTree)
    await screen.findByText('1243 contacts')

    editFirstCondition()

    fireEvent.mouseDown(selectShowing('Newsletter'))
    fireEvent.click(await screen.findByTitle('Product updates'))
    fireEvent.mouseDown(selectShowing('Product updates'))
    fireEvent.click(await screen.findByTitle('Newsletter'))
    fireEvent.mouseDown(selectShowing('Newsletter'))
    fireEvent.click(await screen.findByTitle('Product updates'))

    await wait(1200)

    // The opening count, then a single one for the whole burst
    expect(previewSegmentMock).toHaveBeenCalledTimes(2)
  })

  it('keeps the last count while the condition is incomplete', async () => {
    openDrawer(goalTree)
    await screen.findByText('1243 contacts')

    editFirstCondition()

    // Switching the timeframe drops the dates of the previous one, leaving the condition
    // unanswerable until a range is picked
    fireEvent.mouseDown(selectShowing('anytime'))
    fireEvent.click(await screen.findByTitle('in date range'))

    await wait(1200)

    expect(previewSegmentMock).toHaveBeenCalledTimes(1)
    expect(screen.getByText('1243 contacts')).toBeTruthy()
    expect(screen.getByText('Complete the condition to refresh the count')).toBeTruthy()
  })

  it('covers the circle with a spinner for as long as a count is on its way', async () => {
    openDrawer(listTree)
    await screen.findByText('1243 contacts')
    expect(document.querySelector('.ant-spin-spinning')).toBeNull()

    let releaseSecond: (value: unknown) => void = () => {}
    previewSegmentMock.mockImplementationOnce(
      () => new Promise((resolve) => (releaseSecond = resolve))
    )

    editFirstCondition()
    fireEvent.mouseDown(selectShowing('Newsletter'))
    fireEvent.click(await screen.findByTitle('Product updates'))

    // Spinning from the edit itself. The count still stands at one call, so this is the wait
    // before the request even goes out — the stretch that used to have no feedback at all.
    await waitFor(() => expect(document.querySelector('.ant-spin-spinning')).toBeTruthy())
    expect(previewSegmentMock).toHaveBeenCalledTimes(1)

    // ...and still spinning once the request it queued is in flight
    await wait(900)
    expect(document.querySelector('.ant-spin-spinning')).toBeTruthy()
    expect(previewSegmentMock).toHaveBeenCalledTimes(2)

    releaseSecond(countOf(87))

    await screen.findByText('87 contacts', undefined, { timeout: 3000 })
    await waitFor(() => expect(document.querySelector('.ant-spin-spinning')).toBeNull())
  })

  it('does not ask for the condition to be completed while a refresh is on its way', async () => {
    openDrawer(listTree)
    await screen.findByText('1243 contacts')

    // Hold the answer back so the refresh stays on its way for the whole assertion
    let releaseSecond: (value: unknown) => void = () => {}
    previewSegmentMock.mockImplementationOnce(
      () => new Promise((resolve) => (releaseSecond = resolve))
    )

    editFirstCondition()
    fireEvent.mouseDown(selectShowing('Newsletter'))
    fireEvent.click(await screen.findByTitle('Product updates'))

    // The condition is complete — the count is merely out of date, which the dimming already says
    expect(screen.queryByText('Complete the condition to refresh the count')).toBeNull()
    await wait(900)
    expect(screen.queryByText('Complete the condition to refresh the count')).toBeNull()

    releaseSecond(countOf(87))
    expect(await screen.findByText('87 contacts', undefined, { timeout: 3000 })).toBeTruthy()
  })

  it('does not strand the count of an edit that was undone', async () => {
    openDrawer(listTree)
    await screen.findByText('1243 contacts')

    // Hold the second answer back so the edit can be undone while it is in flight
    let releaseSecond: (value: unknown) => void = () => {}
    previewSegmentMock.mockImplementationOnce(
      () => new Promise((resolve) => (releaseSecond = resolve))
    )

    editFirstCondition()
    fireEvent.mouseDown(selectShowing('Newsletter'))
    fireEvent.click(await screen.findByTitle('Product updates'))
    await waitFor(() => expect(previewSegmentMock).toHaveBeenCalledTimes(2), { timeout: 3000 })

    // Undo the edit, then let the answer to it land
    fireEvent.mouseDown(selectShowing('Product updates'))
    fireEvent.click(await screen.findByTitle('Newsletter'))
    releaseSecond(countOf(87))

    await wait(1200)

    // The count for the abandoned edit must not take over the circle, and the count that does
    // match the form must not be left marked as out of date
    expect(screen.getByText('1243 contacts')).toBeTruthy()
    expect(screen.queryByText('87 contacts')).toBeNull()
    expect(screen.queryByText('Complete the condition to refresh the count')).toBeNull()
  })

  it('drops a value the new event kind hides, as Confirm would', async () => {
    const clickTree: TreeNode = {
      kind: 'branch',
      branch: {
        operator: 'and',
        leaves: [
          {
            kind: 'leaf',
            leaf: {
              source: 'contact_timeline',
              contact_timeline: {
                kind: 'email.clicked',
                count_operator: 'at_least',
                count_value: 1,
                link_url: '/pricing',
                timeframe_operator: 'anytime',
                timeframe_values: []
              }
            }
          }
        ]
      }
    }

    openDrawer(clickTree)
    await screen.findByText('1243 contacts')

    editFirstCondition()
    fireEvent.mouseDown(selectShowing('Click email'))
    fireEvent.click(await screen.findByTitle('Open email'))

    await waitFor(() => expect(previewSegmentMock).toHaveBeenCalledTimes(2), { timeout: 3000 })

    // link_url only means anything for a click, and the backend rejects it on any other kind
    const previewed = previewedTrees()
    const timeline = previewed[previewed.length - 1].branch?.leaves[0].leaf?.contact_timeline
    expect(timeline?.kind).toBe('email.opened')
    expect(timeline?.link_url).toBeUndefined()
  })

  it('reports a failed refresh on the circle instead of a toast', async () => {
    openDrawer(listTree)
    await screen.findByText('1243 contacts')

    previewSegmentMock.mockRejectedValue(new Error('segment query timed out'))
    editFirstCondition()

    fireEvent.mouseDown(selectShowing('Newsletter'))
    fireEvent.click(await screen.findByTitle('Product updates'))

    await waitFor(
      () => expect(document.querySelector('[data-icon="triangle-exclamation"]')).toBeTruthy(),
      { timeout: 3000 }
    )
    // The last known count stays on screen rather than being wiped by the failure
    expect(screen.getByText('1243 contacts')).toBeTruthy()
    expect(message.error).not.toHaveBeenCalled()
  })
})
