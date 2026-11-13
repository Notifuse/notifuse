import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AutomationProvider } from './AutomationContext'
import { useAutomation } from './automationContextValue'
import { automationApi, type Automation } from '../../../services/api/automation'
import { ApiError } from '../../../services/api/errors'
import type { Workspace } from '../../../services/api/types'

vi.mock('../../../services/api/automation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../services/api/automation')>()
  return { ...actual, automationApi: { ...actual.automationApi, update: vi.fn(), create: vi.fn() } }
})

const mockMessage = { success: vi.fn(), error: vi.fn(), warning: vi.fn() }

const workspace = { id: 'ws1', integrations: [] } as unknown as Workspace

// A live automation as the editor loads it: a trigger and nothing else, which saves as it is.
const liveAutomation: Automation = {
  id: 'auto-1',
  workspace_id: 'ws1',
  name: 'Welcome',
  status: 'live',
  list_id: '',
  trigger: { event_kind: 'custom_event', custom_event_name: 'signed_up', frequency: 'every_time' },
  root_node_id: 'trigger-1',
  nodes: [
    {
      id: 'trigger-1',
      automation_id: 'auto-1',
      type: 'trigger',
      config: {},
      position: { x: 0, y: 0 },
      created_at: '2026-01-01T00:00:00Z'
    }
  ],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

// A failed save rejects as well as telling the user; what they are told is what is tested here.
function SaveButton() {
  const { save } = useAutomation()
  return <button onClick={() => save().catch(() => undefined)}>Save</button>
}

const renderEditor = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } }
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <AutomationProvider
            workspace={workspace}
            automation={liveAutomation}
            lists={[]}
            canReadTemplates
          >
            <SaveButton />
          </AutomationProvider>
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const save = () => fireEvent.click(screen.getByRole('button', { name: 'Save' }))

// The answer of an update that failed, as the API client throws it: the message, the status and
// the body.
const failedWith = (status: number, body: { error: string } & Record<string, unknown>) =>
  new ApiError(body.error, status, body)

describe('AutomationProvider saving an automation the editor loaded live', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(App, 'useApp').mockReturnValue({
      message: mockMessage,
      notification: {},
      modal: {}
    } as unknown as ReturnType<typeof App.useApp>)
  })

  // Saving keeps the stored status, and a save that failed earlier can have paused it: saved,
  // but not running, which a plain success would hide.
  it('warns when the automation comes back paused', async () => {
    vi.mocked(automationApi.update).mockResolvedValue({
      automation: { ...liveAutomation, status: 'paused' }
    })
    renderEditor()
    save()

    await waitFor(() =>
      expect(mockMessage.warning).toHaveBeenCalledWith({
        content: 'Automation updated, but it is paused: activate it to run this version.',
        duration: 10
      })
    )
    expect(mockMessage.success).not.toHaveBeenCalled()
  })

  it('confirms a save that leaves it live', async () => {
    vi.mocked(automationApi.update).mockResolvedValue({ automation: liveAutomation })
    renderEditor()
    save()

    await waitFor(() =>
      expect(mockMessage.success).toHaveBeenCalledWith('Automation updated successfully')
    )
    expect(mockMessage.warning).not.toHaveBeenCalled()
  })

  // The message starts with the failure and goes on to say what became of the automation, which
  // the user has to act on: shown as sent, rather than after a prefix that repeats the failure,
  // and long enough to read.
  it.each([
    {
      case: 'paused',
      error: failedWith(500, {
        error:
          'Failed to update automation; the automation was paused, because its previous version could not be put back',
        paused: true
      })
    },
    {
      case: 'paused on a deleted list',
      error: failedWith(400, {
        error:
          'invalid trigger conditions: operator does not exist; the automation was paused, because its previous version uses the deleted list "Newsletter"',
        paused: true,
        lists: [{ id: 'newsletter', name: 'Newsletter' }]
      })
    },
    {
      case: 'running its previous trigger',
      error: failedWith(500, {
        error:
          "Failed to update automation; the automation is still live, storing this version but running the previous version's trigger: pause it and activate it again",
        running_previous_trigger: true
      })
    }
  ])('shows what became of the automation when a save fails: $case', async ({ error }) => {
    vi.mocked(automationApi.update).mockRejectedValue(error)
    renderEditor()
    save()

    await waitFor(() =>
      expect(mockMessage.error).toHaveBeenCalledWith({ content: error.message, duration: 10 })
    )
    expect(mockMessage.error).toHaveBeenCalledTimes(1)
  })

  it('keeps the usual message for a failure that changed nothing', async () => {
    vi.mocked(automationApi.update).mockRejectedValue(
      failedWith(400, { error: 'invalid trigger conditions: operator does not exist' })
    )
    renderEditor()
    save()

    await waitFor(() =>
      expect(mockMessage.error).toHaveBeenCalledWith(
        'Failed to update automation: invalid trigger conditions: operator does not exist'
      )
    )
  })
})

function TemplatesReadProbe() {
  const { canReadTemplates } = useAutomation()
  return <span data-testid="probe">{String(canReadTemplates)}</span>
}

// The editor's template fields read the permission from the context.
describe('AutomationProvider Templates read', () => {
  it.each([false, true])('exposes canReadTemplates %s', (canRead) => {
    const queryClient = new QueryClient()
    render(
      <QueryClientProvider client={queryClient}>
        <I18nProvider i18n={i18n}>
          <App>
            <AutomationProvider workspace={workspace} lists={[]} canReadTemplates={canRead}>
              <TemplatesReadProbe />
            </AutomationProvider>
          </App>
        </I18nProvider>
      </QueryClientProvider>
    )

    expect(screen.getByTestId('probe')).toHaveTextContent(String(canRead))
  })

  // The permissions load after the page: an editor opened before then must unlock once they do.
  it('follows the permission once it loads', () => {
    const queryClient = new QueryClient()
    const tree = (canRead: boolean) => (
      <QueryClientProvider client={queryClient}>
        <I18nProvider i18n={i18n}>
          <App>
            <AutomationProvider workspace={workspace} lists={[]} canReadTemplates={canRead}>
              <TemplatesReadProbe />
            </AutomationProvider>
          </App>
        </I18nProvider>
      </QueryClientProvider>
    )
    const { rerender } = render(tree(false))
    expect(screen.getByTestId('probe')).toHaveTextContent('false')

    rerender(tree(true))
    expect(screen.getByTestId('probe')).toHaveTextContent('true')
  })
})

function SessionProbe() {
  const { automationId, isEditing, hasUnsavedChanges, frequency, setFrequency, setName, conflict, save, canUndo, canvasState } =
    useAutomation()
  const configureTrigger = () =>
    canvasState.setNodes((nds) =>
      nds.map((n) =>
        n.data.nodeType === 'trigger' ? { ...n, data: { ...n.data, config: { ...n.data.config, event_kind: 'contact.created' } } } : n
      )
    )
  return (
    <div>
      <span data-testid="id">{automationId}</span>
      <span data-testid="editing">{String(isEditing)}</span>
      <span data-testid="dirty">{String(hasUnsavedChanges)}</span>
      <span data-testid="frequency">{frequency}</span>
      <span data-testid="conflict">{String(conflict)}</span>
      <span data-testid="undo">{String(canUndo)}</span>
      <button onClick={() => setName('Renamed')}>Rename</button>
      <button onClick={() => setName('Welcome')}>Name back</button>
      <button onClick={() => setFrequency('once')}>Once</button>
      <button onClick={() => void save()}>Save</button>
      <button onClick={configureTrigger}>Configure</button>
    </div>
  )
}

const renderSession = (props: { automation?: Automation; readOnly?: boolean; onSaved?: (a: Automation) => void } = {}) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const tree = (automation?: Automation) => (
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <AutomationProvider
            workspace={workspace}
            automation={automation}
            lists={[]}
            canReadTemplates
            readOnly={props.readOnly}
            onSaved={props.onSaved}
          >
            <SessionProbe />
          </AutomationProvider>
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
  const utils = render(tree(props.automation))
  return { ...utils, queryClient, rerenderWith: (a?: Automation) => utils.rerender(tree(a)) }
}

const text = (id: string) => screen.getByTestId(id).textContent
const click = (name: string) => fireEvent.click(screen.getByRole('button', { name }))
const paused: Automation = { ...liveAutomation, status: 'paused' }

describe('AutomationProvider unsaved changes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(App, 'useApp').mockReturnValue({
      message: mockMessage,
      notification: {},
      modal: {}
    } as unknown as ReturnType<typeof App.useApp>)
  })

  it('is clean when loaded and when new and untouched', () => {
    const { unmount } = renderSession({ automation: paused })
    expect(text('dirty')).toBe('false')
    unmount()
    renderSession()
    expect(text('dirty')).toBe('false')
  })

  it('follows the difference with what was loaded, not the edits made', () => {
    renderSession({ automation: paused })
    click('Rename')
    expect(text('dirty')).toBe('true')
    click('Name back')
    expect(text('dirty')).toBe('false')
  })

  it('stays clean when the automation is paused or activated from the editor', () => {
    const { rerenderWith } = renderSession({ automation: paused })
    rerenderWith({ ...paused, status: 'live', updated_at: '2026-02-01T00:00:00Z' })
    expect(text('dirty')).toBe('false')
  })

  it('writes the frequency to the trigger, as one undo step', () => {
    renderSession({ automation: paused })
    expect(text('frequency')).toBe('every_time')
    click('Once')
    expect(text('frequency')).toBe('once')
    expect(text('undo')).toBe('true')
    expect(text('dirty')).toBe('true')
  })

  it('changes nothing read-only', () => {
    renderSession({ automation: liveAutomation, readOnly: true })
    click('Rename')
    click('Once')
    expect(text('frequency')).toBe('every_time')
    expect(text('dirty')).toBe('false')
    click('Save')
    expect(automationApi.update).not.toHaveBeenCalled()
  })

  it('creates a new automation, then updates it under the same id', async () => {
    vi.mocked(automationApi.create).mockImplementation(async ({ automation }) => ({ automation }))
    vi.mocked(automationApi.update).mockImplementation(async ({ automation }) => ({ automation }))
    const onSaved = vi.fn()
    renderSession({ onSaved })
    const id = text('id')
    expect(text('editing')).toBe('false')
    click('Rename')
    click('Configure')
    click('Save')
    await waitFor(() => expect(text('editing')).toBe('true'))
    expect(vi.mocked(automationApi.create).mock.calls[0][0].automation.id).toBe(id)
    expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ id, name: 'Renamed' }))
    expect(text('dirty')).toBe('false')

    click('Once')
    click('Save')
    await waitFor(() => expect(automationApi.update).toHaveBeenCalled())
    expect(vi.mocked(automationApi.update).mock.calls[0][0].automation.id).toBe(id)
    expect(text('id')).toBe(id)
  })

  it('keeps the edits and asks for a reload on a conflict', async () => {
    vi.mocked(automationApi.update).mockRejectedValue(
      failedWith(409, { error: 'automation auto-1 is no longer paused' })
    )
    const { queryClient } = renderSession({ automation: paused })
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    click('Rename')
    click('Save')
    await waitFor(() => expect(text('conflict')).toBe('true'))
    expect(mockMessage.error).toHaveBeenCalledWith('This automation was changed elsewhere')
    expect(text('dirty')).toBe('true')
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['automation', 'ws1', 'auto-1'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['automations', 'ws1'] })
  })
})
