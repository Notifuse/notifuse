import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { ApiError } from '../../services/api/errors'
import type { Segment, TreeNode } from '../../services/api/segment'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const { api, blockerState } = vi.hoisted(() => ({
  api: {
    previewSegment: vi.fn(),
    getSegment: vi.fn(),
    createSegment: vi.fn(),
    updateSegment: vi.fn()
  },
  blockerState: {
    shouldBlockFn: undefined as undefined | ((args: { current: { pathname: string }; next: { pathname: string }; action: string }) => boolean),
    // Simulates the router blocking a navigation (the browser's Back)
    block: () => {},
    proceed: vi.fn(),
    reset: vi.fn()
  }
}))

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useParams: () => ({ workspaceId: 'ws1' }),
    useNavigate: () => vi.fn(),
    useMatch: () => false,
    useBlocker: (opts: { shouldBlockFn: typeof blockerState.shouldBlockFn }) => {
      const { useState } = (globalThis as unknown as { __react: typeof import('react') }).__react
      const [status, setStatus] = useState<'idle' | 'blocked'>('idle')
      blockerState.shouldBlockFn = opts.shouldBlockFn
      blockerState.block = () => setStatus('blocked')
      return {
        status,
        proceed: () => {
          blockerState.proceed()
          setStatus('idle')
        },
        reset: () => {
          blockerState.reset()
          setStatus('idle')
        }
      }
    }
  }
})
vi.mock('../../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ permissions: null }),
  useAuth: () => ({ workspaces: [{ id: 'ws1', settings: { timezone: 'Europe/Paris', custom_field_labels: {} } }] })
}))
vi.mock('../../services/api/list', () => ({
  listsApi: { list: vi.fn().mockResolvedValue({ lists: [{ id: 'news', name: 'Newsletter' }] }) }
}))
vi.mock('../../services/api/template', () => ({ templatesApi: { list: vi.fn().mockResolvedValue({ templates: [] }) } }))
vi.mock('../../services/api/segment', async () => {
  const actual = await vi.importActual('../../services/api/segment')
  return { ...actual, ...api }
})

import * as React from 'react'
import { act } from '@testing-library/react'
;(globalThis as unknown as { __react: typeof React }).__react = React
import { SegmentDrawer } from './SegmentDrawer'

const listLeaf: TreeNode = {
  kind: 'leaf',
  leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news', status: 'active' } }
}
const treeOf = (...leaves: TreeNode[]): TreeNode => ({ kind: 'branch', branch: { operator: 'and', leaves } })

const segment = (overrides: Partial<Segment> = {}): Segment => ({
  id: 'big_spenders',
  name: 'Big spenders',
  color: 'green',
  tree: treeOf(listLeaf),
  timezone: 'UTC',
  version: 1,
  status: 'active',
  db_created_at: '2026-01-01T00:00:00Z',
  db_updated_at: '2026-01-01T00:00:00Z',
  ...overrides
})

const renderDrawer = (props: Partial<React.ComponentProps<typeof SegmentDrawer>> = {}) => {
  const setVisible = vi.fn()
  const onSuccess = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <App>
          <SegmentDrawer canReadTemplates setDrawserVisible={setVisible} onSuccess={onSuccess} {...props} />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
  return { setVisible, onSuccess }
}

const nameInput = () => screen.getByRole('textbox', { name: 'Segment name' })
const saveButton = (label = 'Create') => screen.getByRole('button', { name: label })
const typeName = (value: string) => fireEvent.change(nameInput(), { target: { value } })
const summaryItems = () => screen.getAllByRole('listitem').map((item) => item.textContent)

beforeEach(() => {
  api.previewSegment.mockReset().mockResolvedValue({ emails: [], total_count: 12, limit: 100, generated_sql: 'SELECT 1', sql_args: [] })
  api.getSegment.mockReset().mockRejectedValue(new ApiError('Segment not found', 404))
  api.createSegment.mockReset().mockResolvedValue({ segment: segment() })
  api.updateSegment.mockReset().mockResolvedValue({ segment: segment() })
  blockerState.shouldBlockFn = undefined
  blockerState.proceed.mockReset()
  blockerState.reset.mockReset()
})

const pressBack = () => act(() => blockerState.block())

describe('SegmentDrawer — a new segment', () => {
  it('opens on an empty tree with what is missing, Create disabled', () => {
    renderDrawer()
    expect(screen.getByRole('dialog', { name: 'New segment' })).toBeTruthy()
    expect(screen.getByText('Who should be in this segment?')).toBeTruthy()
    expect(screen.getByText('New')).toBeTruthy()
    expect(saveButton()).toBeDisabled()
    expect(summaryItems()).toEqual(['Add at least one condition.', 'Give the segment a name.'])
  })

  it('takes the conditions of the contacts filters, and says so', async () => {
    renderDrawer({ initialTree: treeOf(listLeaf) })
    expect(screen.getByText('These conditions come from the filters of the contacts view.')).toBeTruthy()
    expect(await screen.findByText('Newsletter')).toBeTruthy()
  })

  it('says nothing about filters when none were set', () => {
    renderDrawer({ initialTree: treeOf() })
    expect(screen.queryByText('These conditions come from the filters of the contacts view.')).toBeNull()
  })

  it('shows the ID the name gives, and waits for its check before Create', async () => {
    renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Clients à forte valeur')
    expect(await screen.findByText('clients_a_forte_valeur')).toBeTruthy()
    expect(saveButton()).toBeDisabled()
    expect(summaryItems()[0]).toBe('Checking that the ID clients_a_forte_valeur is free…')
    await waitFor(() => expect(saveButton()).toBeEnabled(), { timeout: 2000 })
    expect(api.getSegment).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'clients_a_forte_valeur' })
  })

  it('refuses a taken ID', async () => {
    api.getSegment.mockResolvedValue({ segment: segment() })
    renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Big spenders')
    await waitFor(() => expect(summaryItems()[0]).toBe('A segment already has the ID big_spenders: change the name.'), {
      timeout: 2000
    })
    expect(saveButton()).toBeDisabled()
  })

  it('does not read a failed check as a free ID, nor block on it', async () => {
    api.getSegment.mockRejectedValue(new ApiError('Internal error', 500))
    renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Big spenders')
    expect(
      await screen.findByText('The ID big_spenders could not be checked; saving will tell if it is taken.', undefined, {
        timeout: 2000
      })
    ).toBeTruthy()
    expect(saveButton()).toBeEnabled()
  })

  it('refuses a name that gives no ID', async () => {
    renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('会員')
    await waitFor(() =>
      expect(summaryItems()[0]).toBe("The name needs at least one Latin letter or digit, to make the segment's ID.")
    )
    expect(saveButton()).toBeDisabled()
  })

  it('creates the segment with the generated ID, then closes', async () => {
    const { setVisible, onSuccess } = renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Big spenders')
    await waitFor(() => expect(saveButton()).toBeEnabled(), { timeout: 2000 })
    fireEvent.click(saveButton())
    await waitFor(() => expect(onSuccess).toHaveBeenCalled())
    expect(api.createSegment).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'big_spenders', name: 'Big spenders', color: 'blue', timezone: 'Europe/Paris' })
    )
    expect(setVisible).toHaveBeenCalledWith(false)
  })

  it('keeps a failed save in the drawer, and lets it be retried', async () => {
    api.createSegment.mockRejectedValueOnce(new ApiError('Failed to create segment', 500))
    const { setVisible, onSuccess } = renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Big spenders')
    await waitFor(() => expect(saveButton()).toBeEnabled(), { timeout: 2000 })
    fireEvent.click(saveButton())

    expect(await screen.findByText('Failed to create segment')).toBeTruthy()
    expect(setVisible).not.toHaveBeenCalled()
    expect(saveButton()).toBeEnabled()

    fireEvent.click(saveButton())
    await waitFor(() => expect(onSuccess).toHaveBeenCalled())
  })

  it('adds the server text when it says more than the fixed sentence', async () => {
    api.createSegment.mockRejectedValueOnce(new ApiError('invalid tree: branch has no leaves', 400))
    renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Big spenders')
    await waitFor(() => expect(saveButton()).toBeEnabled(), { timeout: 2000 })
    fireEvent.click(saveButton())
    expect(await screen.findByText('Failed to create segment: invalid tree: branch has no leaves')).toBeTruthy()
  })

  it('ignores a second click and a close while saving', async () => {
    let resolve: (value: unknown) => void = () => {}
    api.createSegment.mockReturnValue(new Promise((r) => (resolve = r)))
    const { setVisible } = renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Big spenders')
    await waitFor(() => expect(saveButton()).toBeEnabled(), { timeout: 2000 })
    // The same element both times: once loading, its accessible name gains the spinner's
    const button = saveButton()
    fireEvent.click(button)
    fireEvent.click(button)
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(api.createSegment).toHaveBeenCalledTimes(1)
    expect(setVisible).not.toHaveBeenCalled()
    resolve({ segment: segment() })
    await waitFor(() => expect(setVisible).toHaveBeenCalledWith(false))
  })
})

describe('SegmentDrawer — an existing segment', () => {
  it('opens on its name, colour, status and own ID, without checking it', () => {
    renderDrawer({ segment: segment() })
    expect(screen.getByRole('dialog', { name: 'Edit segment' })).toBeTruthy()
    expect(nameInput()).toHaveValue('Big spenders')
    expect(screen.getByRole('button', { name: 'Colour: Green' })).toBeTruthy()
    expect(document.querySelector('[data-segment-id]')?.textContent).toBe('big_spenders')
    expect(screen.getAllByText('Active').length).toBeGreaterThan(0)
    expect(api.getSegment).not.toHaveBeenCalled()
    expect(screen.getByText('The segment is recomputed when you save.')).toBeTruthy()
  })

  it('shows a segment being computed', () => {
    renderDrawer({ segment: segment({ status: 'building' }) })
    expect(screen.getByText('Computing')).toBeTruthy()
  })

  it('saves a rename to any name under its own ID', async () => {
    const { onSuccess } = renderDrawer({ segment: segment() })
    typeName('会員')
    fireEvent.click(saveButton('Save'))
    await waitFor(() => expect(onSuccess).toHaveBeenCalled())
    expect(api.updateSegment).toHaveBeenCalledWith(expect.objectContaining({ id: 'big_spenders', name: '会員' }))
  })

  it('refuses to save while a condition is open', async () => {
    renderDrawer({ segment: segment() })
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    await waitFor(() => expect(saveButton('Save')).toBeDisabled())
    expect(summaryItems()[0]).toBe('A condition is open: confirm or cancel it.')
  })

  it('refuses to save a group left empty, and says how to fix it', async () => {
    renderDrawer({ segment: segment({ tree: treeOf(listLeaf, treeOf()) }) })
    await waitFor(() => expect(saveButton('Save')).toBeDisabled())
    expect(summaryItems()).toContain('A group is empty: add a condition to it or remove it.')
  })
})

describe('SegmentDrawer — closing', () => {
  // The API accepts a single condition at the root; the console only edits a group there
  it('opens a segment saved with a single condition at its root, unchanged until edited', () => {
    const { setVisible } = renderDrawer({ segment: segment({ tree: listLeaf }) })
    expect(screen.queryByText('A branch condition is required...')).toBeNull()
    expect(summaryItems()).toContain('1 condition')
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(setVisible).toHaveBeenCalledWith(false)
  })

  it('closes at once when nothing changed', () => {
    const { setVisible } = renderDrawer({ segment: segment() })
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(setVisible).toHaveBeenCalledWith(false)
  })

  it('asks before losing a change, and keeps editing on request', async () => {
    const { setVisible } = renderDrawer({ segment: segment() })
    typeName('Bigger spenders')
    await waitFor(() => expect(nameInput()).toHaveValue('Bigger spenders'))

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    const confirms = await screen.findAllByText('Discard your changes?')
    fireEvent.click(
      within(confirms[confirms.length - 1].closest('.ant-modal') as HTMLElement).getByRole('button', {
        name: 'Keep editing'
      })
    )
    expect(setVisible).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    const again = await screen.findAllByText('Discard your changes?')
    fireEvent.click(
      within(again[again.length - 1].closest('.ant-modal') as HTMLElement).getByRole('button', { name: 'Discard' })
    )
    await waitFor(() => expect(setVisible).toHaveBeenCalledWith(false))
  })

  it('asks while a condition is open, even unchanged', async () => {
    const { setVisible } = renderDrawer({ segment: segment() })
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    await screen.findByText('Confirm')
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(await screen.findByText(/A condition is still open/)).toBeTruthy()
    expect(setVisible).not.toHaveBeenCalled()
  })

  it('sees no change after a condition is opened and cancelled', async () => {
    const { setVisible } = renderDrawer({ segment: segment() })
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    const cancel = await screen.findAllByRole('button', { name: 'Cancel' })
    // The condition's own Cancel, not the bar's
    fireEvent.click(cancel[cancel.length - 1])
    await waitFor(() => expect(screen.queryByText('Confirm')).toBeNull())
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(setVisible).toHaveBeenCalledWith(false)
  })
})

describe('SegmentDrawer — the browser Back', () => {
  it('closes a clean drawer and lets the navigation go', () => {
    const { setVisible } = renderDrawer({ segment: segment() })
    expect(blockerState.shouldBlockFn?.({ current: { pathname: '/a' }, next: { pathname: '/a' }, action: 'BACK' })).toBe(true)
    pressBack()
    expect(setVisible).toHaveBeenCalledWith(false)
    expect(blockerState.proceed).toHaveBeenCalled()
  })

  it('asks first when there are changes; Discard closes the drawer and goes back', async () => {
    const { setVisible } = renderDrawer({ segment: segment() })
    typeName('Renamed')
    await waitFor(() => expect(nameInput()).toHaveValue('Renamed'))
    await waitFor(() => expect(screen.getByText('Save')).toBeTruthy())
    pressBack()
    const confirms = await screen.findAllByText('Discard your changes?')
    fireEvent.click(within(confirms[confirms.length - 1].closest('.ant-modal') as HTMLElement).getByRole('button', { name: 'Discard' }))
    await waitFor(() => expect(setVisible).toHaveBeenCalledWith(false))
    expect(blockerState.proceed).toHaveBeenCalled()
  })

  it('stays, and stays put, on Keep editing', async () => {
    const { setVisible } = renderDrawer({ segment: segment() })
    typeName('Renamed')
    await waitFor(() => expect(nameInput()).toHaveValue('Renamed'))
    pressBack()
    const confirms = await screen.findAllByText('Discard your changes?')
    fireEvent.click(
      within(confirms[confirms.length - 1].closest('.ant-modal') as HTMLElement).getByRole('button', { name: 'Keep editing' })
    )
    await waitFor(() => expect(blockerState.reset).toHaveBeenCalled())
    expect(setVisible).not.toHaveBeenCalled()
    expect(blockerState.proceed).not.toHaveBeenCalled()
  })

  it('does not stack a second question when Back comes while × asks', async () => {
    renderDrawer({ segment: segment() })
    typeName('Renamed')
    await waitFor(() => expect(nameInput()).toHaveValue('Renamed'))
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    await screen.findAllByText('Discard your changes?')
    pressBack()
    await waitFor(() => expect(blockerState.reset).toHaveBeenCalled())
    expect(document.querySelectorAll('.ant-modal-confirm').length).toBe(1)
  })
})

describe('SegmentDrawer — the ID check', () => {
  it('drops a late answer about a name typed earlier', async () => {
    let answerOld: (value: unknown) => void = () => {}
    api.getSegment.mockImplementation(({ id }: { id: string }) =>
      id === 'vipx' ? new Promise((resolve) => (answerOld = () => resolve({ segment: segment() }))) : Promise.reject(new ApiError('Segment not found', 404))
    )
    renderDrawer({ initialTree: treeOf(listLeaf) })
    typeName('Vipx')
    await waitFor(() => expect(api.getSegment).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'vipx' }), { timeout: 2000 })
    typeName('Vip')
    await waitFor(() => expect(saveButton()).toBeEnabled(), { timeout: 2000 })
    // The answer for « vipx » (taken) comes back now: it says nothing about « vip »
    await act(async () => answerOld(undefined))
    expect(saveButton()).toBeEnabled()
    expect(screen.queryByText(/already has the ID/)).toBeNull()
  })
})

describe('SegmentDrawer — the page writing its own URL', () => {
  it('lets a replace on the same page through, and stops a real departure', () => {
    renderDrawer({ segment: segment() })
    const page = { pathname: '/console/workspace/ws1/contacts' }
    // The Contacts search commits its text with a replace on the same page
    expect(blockerState.shouldBlockFn?.({ current: page, next: page, action: 'REPLACE' })).toBe(false)
    expect(blockerState.shouldBlockFn?.({ current: page, next: page, action: 'PUSH' })).toBe(true)
    expect(blockerState.shouldBlockFn?.({ current: page, next: { pathname: '/console/signin' }, action: 'REPLACE' })).toBe(true)
  })
})

describe('SegmentDrawer — a segment refetched while open', () => {
  it('sees no change when the segment it opened on is fetched again', () => {
    const setVisible = vi.fn()
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const tree = (value: Segment) => (
      <QueryClientProvider client={client}>
        <I18nProvider i18n={i18n}>
          <App>
            <SegmentDrawer canReadTemplates setDrawserVisible={setVisible} segment={value} />
          </App>
        </I18nProvider>
      </QueryClientProvider>
    )
    const { rerender } = render(tree(segment()))
    // Polled while it builds: a new object, a new status, a new date
    rerender(tree(segment({ status: 'building', db_updated_at: '2026-02-01T00:00:00Z', name: 'Renamed elsewhere' })))
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(setVisible).toHaveBeenCalledWith(false)
    // The form kept what it opened on
    expect(nameInput()).toHaveValue('Big spenders')
  })
})
