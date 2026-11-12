import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Automation } from '../../services/api/automation'
import type { Workspace } from '../../services/api/types'
import type { AutomationEditorProps } from './AutomationEditor'

vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {}
}))

interface BlockerOptions {
  shouldBlockFn: (args: { current: { pathname: string; search: unknown }; next: { pathname: string; search: unknown } }) => boolean
  enableBeforeUnload: () => boolean
}
const blocker = vi.hoisted(() => ({
  options: null as BlockerOptions | null,
  status: 'idle' as 'idle' | 'blocked',
  proceed: vi.fn(),
  reset: vi.fn()
}))
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useBlocker: (options: BlockerOptions) => {
    blocker.options = options
    return { status: blocker.status, proceed: blocker.proceed, reset: blocker.reset }
  }
}))

vi.mock('../../services/api/client', async () => ({
  ...(await vi.importActual<typeof import('../../services/api/errors')>('../../services/api/errors')),
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('../../services/api/automation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/automation')>()),
  automationApi: {
    get: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    activate: vi.fn(() => Promise.resolve({ success: true })),
    pause: vi.fn(() => Promise.resolve({ success: true })),
    delete: vi.fn(() => Promise.resolve({ success: true })),
    nodeStats: vi.fn(() => Promise.resolve({ automation_id: 'auto-1', nodes: {} }))
  }
}))

// The canvas has its own tests: this one shows the overlay and edits through the context.
vi.mock('./AutomationFlowEditor', async () => {
  const { useAutomation } = await vi.importActual<typeof import('./context')>('./context')
  return {
    AutomationFlowEditor: ({ overlay }: { overlay?: ReactNode }) => {
      const { canvasState, pushHistory, automationId } = useAutomation()
      const dropEdges = () => {
        pushHistory()
        canvasState.setEdges([])
      }
      const configureTrigger = () =>
        canvasState.setNodes((nds) =>
          nds.map((n) =>
            n.data.nodeType === 'trigger' ? { ...n, data: { ...n.data, config: { ...n.data.config, event_kind: 'contact.created' } } } : n
          )
        )
      return (
        <div data-testid="canvas" data-session-id={automationId}>
          {overlay}
          <button onClick={dropEdges}>probe: drop edges</button>
          <button onClick={configureTrigger}>probe: configure trigger</button>
        </div>
      )
    }
  }
})

import { automationApi } from '../../services/api/automation'
import { ApiError } from '../../services/api/errors'
import { AutomationEditor } from './AutomationEditor'

const get = automationApi.get as Mock
const create = automationApi.create as Mock
const update = automationApi.update as Mock

const workspace = { id: 'ws1', name: 'Workspace', integrations: [], settings: { timezone: 'UTC' } } as unknown as Workspace

const automationOf = (over: Partial<Automation> = {}): Automation => ({
  id: 'auto-1',
  workspace_id: 'ws1',
  name: 'Welcome',
  status: 'paused',
  list_id: 'list1',
  trigger: { event_kind: 'contact.created', frequency: 'once' },
  root_node_id: 't1',
  nodes: [
    {
      id: 't1',
      automation_id: 'auto-1',
      type: 'trigger',
      config: { event_kind: 'contact.created', frequency: 'once' },
      next_node_id: 'd1',
      position: { x: 0, y: 0 },
      created_at: '2026-01-01T00:00:00Z'
    },
    {
      id: 'd1',
      automation_id: 'auto-1',
      type: 'delay',
      config: { duration: 1, unit: 'days' },
      position: { x: 0, y: 150 },
      created_at: '2026-01-01T00:00:00Z'
    }
  ],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...over
})

type Overrides = Partial<AutomationEditorProps>

function Harness(initial: Overrides) {
  const [param, setParam] = useState(initial.automationParam ?? 'auto-1')
  return (
    <AutomationEditor
      workspace={workspace}
      withStats={false}
      canWrite
      canReadHistory
      canReadTemplates
      lists={[
        { id: 'list1', name: 'Newsletter' },
        { id: 'list2', name: 'Product' }
      ] as AutomationEditorProps['lists']}
      segments={[]}
      templates={[]}
      onClose={vi.fn()}
      onToggleStats={vi.fn()}
      onViewMessages={vi.fn()}
      {...initial}
      automationParam={param}
      onCreated={(id) => {
        initial.onCreated?.(id)
        setParam(id)
      }}
    />
  )
}

const renderEditor = (props: Overrides = {}) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <Harness {...props} />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
  return { ...utils, queryClient }
}

const button = (name: string | RegExp) => screen.getByRole('button', { name })
const nameInput = () => screen.getByRole('textbox', { name: 'Automation name' })
const loaded = () => screen.findByTestId('canvas')
const unsaved = () => screen.queryByText('Unsaved changes')

beforeEach(() => {
  vi.clearAllMocks()
  blocker.options = null
  blocker.status = 'idle'
  get.mockResolvedValue({ automation: automationOf() })
  update.mockImplementation((req: { automation: Automation }) => Promise.resolve({ automation: req.automation }))
  create.mockImplementation((req: { automation: Automation }) => Promise.resolve({ automation: req.automation }))
})

describe('AutomationEditor, creating', () => {
  it('creates, then saves the same id without a new session, and moves the URL to the id', async () => {
    const onCreated = vi.fn()
    renderEditor({ automationParam: 'new', onCreated })
    const canvas = await loaded()
    const id = canvas.dataset.sessionId

    fireEvent.change(nameInput(), { target: { value: 'Onboarding' } })
    fireEvent.click(screen.getByText('probe: configure trigger'))
    fireEvent.click(screen.getByText('probe: drop edges'))
    fireEvent.click(button('Create'))

    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][0].automation.id).toBe(id)
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(id))
    // Same provider: the undo history of the edits made before the create is still there.
    expect(screen.getByTestId('canvas')).toBe(canvas)
    expect(button('Undo')).toBeEnabled()

    fireEvent.change(nameInput(), { target: { value: 'Onboarding 2' } })
    fireEvent.click(button('Save'))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][0].automation.id).toBe(id)
  })

  it('refuses an empty name', async () => {
    renderEditor({ automationParam: 'new' })
    await loaded()
    fireEvent.click(button('Create'))
    expect(await screen.findByText('Please enter an automation name')).toBeInTheDocument()
    expect(create).not.toHaveBeenCalled()
  })

  it('offers no stats before the first save', async () => {
    renderEditor({ automationParam: 'new' })
    await loaded()
    expect(screen.getByRole('radio', { name: 'With stats' })).toBeDisabled()
  })
})

describe('AutomationEditor, unsaved changes', () => {
  it('starts clean, and Save keeps the editor open with "Saved"', async () => {
    const onClose = vi.fn()
    renderEditor({ onClose })
    await loaded()
    expect(unsaved()).not.toBeInTheDocument()

    fireEvent.change(nameInput(), { target: { value: 'Welcome again' } })
    expect(unsaved()).toBeInTheDocument()
    fireEvent.click(button('Save'))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(await screen.findByText('Saved')).toBeInTheDocument()
    expect(unsaved()).not.toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('marks an edge change and clears it after undoing back to the saved state', async () => {
    renderEditor()
    await loaded()
    fireEvent.click(screen.getByText('probe: drop edges'))
    expect(unsaved()).toBeInTheDocument()
    fireEvent.click(button('Undo'))
    expect(unsaved()).not.toBeInTheDocument()
  })

  it('marks a list change', async () => {
    renderEditor()
    await loaded()
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'List' }))
    fireEvent.click(await screen.findByTitle('Product'))
    expect(unsaved()).toBeInTheDocument()
  })

  it('disables Activate while there are unsaved changes', async () => {
    renderEditor()
    await loaded()
    expect(button('Activate')).toBeEnabled()
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    expect(button('Activate')).toBeDisabled()
  })

  it('ignores the undo shortcut in the name field and undoes outside it', async () => {
    renderEditor()
    await loaded()
    fireEvent.click(screen.getByText('probe: drop edges'))
    fireEvent.keyDown(nameInput(), { key: 'z', ctrlKey: true })
    expect(unsaved()).toBeInTheDocument()
    fireEvent.keyDown(screen.getByTestId('canvas'), { key: 'z', ctrlKey: true })
    expect(unsaved()).not.toBeInTheDocument()
  })

  it('asks before closing with changes', async () => {
    const onClose = vi.fn()
    renderEditor({ onClose })
    await loaded()
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    fireEvent.click(button('Close'))
    expect(await screen.findByText('Unsaved Changes', { selector: '.ant-modal-confirm-title' })).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.click(button('Close without saving'))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('asks on Escape with changes', async () => {
    const onClose = vi.fn()
    renderEditor({ onClose })
    await loaded()
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    fireEvent.keyDown(screen.getByTestId('canvas').closest('.ant-drawer-section') ?? screen.getByTestId('canvas'), { key: 'Escape' })
    expect(await screen.findByText('Unsaved Changes', { selector: '.ant-modal-confirm-title' })).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('blocks leaving for another automation or page, never a stats toggle, and only with changes', async () => {
    renderEditor()
    await loaded()
    const here = { pathname: '/automations', search: { automation: 'auto-1' } }
    const block = (next: { pathname: string; search: unknown }) => blocker.options!.shouldBlockFn({ current: here, next })

    expect(block({ pathname: '/automations', search: { automation: 'auto-2' } })).toBe(false)
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    expect(block({ pathname: '/automations', search: { automation: 'auto-1', stats: 1 } })).toBe(false)
    expect(block({ pathname: '/automations', search: { automation: 'auto-2' } })).toBe(true)
    expect(block({ pathname: '/logs', search: {} })).toBe(true)
    expect(blocker.options!.enableBeforeUnload()).toBe(true)
  })
})

describe('AutomationEditor, stats', () => {
  it('writes the stats toggle through onToggleStats', async () => {
    const onToggleStats = vi.fn()
    renderEditor({ journeyTotal: 12, onToggleStats })
    await loaded()
    fireEvent.click(screen.getByRole('radio', { name: 'With stats' }))
    expect(onToggleStats).toHaveBeenCalledWith(true)
  })

  it('offers no stats for a draft without journeys', async () => {
    get.mockResolvedValue({ automation: automationOf({ status: 'draft' }) })
    renderEditor({ journeyTotal: 0 })
    await loaded()
    expect(screen.getByRole('radio', { name: 'With stats' })).toBeDisabled()
  })

  it('shows the period and reads the results with stats on', async () => {
    renderEditor({ journeyTotal: 12, withStats: true })
    await loaded()
    expect(screen.getByRole('radio', { name: 'Last 30 days' })).toBeChecked()
    await waitFor(() => expect(automationApi.nodeStats).toHaveBeenCalled())
  })

  it('says the results could not load, with Retry', async () => {
    vi.mocked(automationApi.nodeStats).mockRejectedValueOnce(new Error('boom'))
    renderEditor({ journeyTotal: 12, withStats: true })
    expect(await screen.findByText("Couldn't load stats")).toBeInTheDocument()
    fireEvent.click(within(screen.getByTestId('canvas')).getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(automationApi.nodeStats).toHaveBeenCalledTimes(2))
  })
})

describe('AutomationEditor, read-only', () => {
  it('shows a live automation read-only, with the banner and Pause', async () => {
    get.mockResolvedValue({ automation: automationOf({ status: 'live' }) })
    renderEditor()
    await loaded()
    expect(screen.getByText('This automation is live. Pause it to edit.')).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: 'Automation name' })).not.toBeInTheDocument()
    expect(screen.getByText('Welcome')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Undo' })).not.toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'List' })).toBeDisabled()
    expect(screen.queryByText('Add to list')).not.toBeInTheDocument()

    fireEvent.click(within(screen.getByTestId('canvas')).getByRole('button', { name: /Pause/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes, pause' }))
    await waitFor(() => expect(automationApi.pause).toHaveBeenCalledWith({ workspace_id: 'ws1', automation_id: 'auto-1' }))
  })

  it('is read-only without write, without the live banner', async () => {
    renderEditor({ canWrite: false })
    await loaded()
    expect(screen.queryByText('This automation is live. Pause it to edit.')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
    expect(button('Activate')).toBeDisabled()
    // Read-only never asks before leaving.
    expect(blocker.options!.enableBeforeUnload()).toBe(false)
  })
})

describe('AutomationEditor, server answers', () => {
  it('offers Reload after a conflict, and reloads', async () => {
    update.mockRejectedValueOnce(new ApiError('automation auto-1 is no longer paused', 409))
    renderEditor()
    await loaded()
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    fireEvent.click(button('Save'))
    expect(await screen.findByText('This automation was changed elsewhere', { selector: '.ant-alert *' })).toBeInTheDocument()

    get.mockResolvedValue({ automation: automationOf({ name: 'Changed elsewhere' }) })
    fireEvent.click(button('Reload'))
    // Unsaved changes: the reload asks first, in a modal of its own.
    expect(await screen.findByText('Reload the automation?', { selector: '.ant-modal-confirm-title' })).toBeInTheDocument()
    fireEvent.click(screen.getAllByRole('button', { name: 'Reload' }).at(-1)!)
    await waitFor(() => expect(nameInput()).toHaveValue('Changed elsewhere'))
  })

  it('reads the automation and the list again after a failed save', async () => {
    update.mockRejectedValueOnce(new ApiError('boom', 500))
    const { queryClient } = renderEditor()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    await loaded()
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    fireEvent.click(button('Save'))
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ['automation', 'ws1', 'auto-1'] }))
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['automations', 'ws1'] })
  })

  it('says an unknown automation was not found', async () => {
    get.mockRejectedValue(new ApiError('not found', 404))
    renderEditor()
    expect(await screen.findByText('Automation not found')).toBeInTheDocument()
  })

  it('deletes after confirming, then closes', async () => {
    const onClose = vi.fn()
    renderEditor({ onClose })
    await loaded()
    fireEvent.click(button('More actions'))
    fireEvent.click(await screen.findByText('Delete Automation'))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes, delete' }))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(automationApi.delete).toHaveBeenCalledWith({ workspace_id: 'ws1', automation_id: 'auto-1' })
  })

  it('activates after confirming', async () => {
    renderEditor()
    await loaded()
    await act(async () => fireEvent.click(button('Activate')))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes, activate' }))
    await waitFor(() => expect(automationApi.activate).toHaveBeenCalledWith({ workspace_id: 'ws1', automation_id: 'auto-1' }))
  })
})
