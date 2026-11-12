import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Node } from '@xyflow/react'
import { NodeConfigPanel } from './NodeConfigPanel'
import type { AutomationNodeData } from './utils/flowConverter'
import type { NodeStats, NodeType } from '../../services/api/automation'
import { NO_NODE_STATS, NodeStatsContext, type NodeStatsState } from './stats/nodeStatsState'

// The per-type forms are exercised by their own tests; stubbing the barrel keeps this one about
// the description field the panel owns, and keeps a single textbox on screen to address.
vi.mock('./config', () => {
  const Stub = ({ canReadTemplates }: { canReadTemplates?: boolean }) => (
    <div data-testid="type-form" data-can-read-templates={String(canReadTemplates)} />
  )
  return {
    TriggerConfigForm: Stub,
    DelayConfigForm: Stub,
    EmailConfigForm: Stub,
    ABTestConfigForm: Stub,
    AddToListConfigForm: Stub,
    RemoveFromListConfigForm: Stub,
    FilterConfigForm: Stub,
    WebhookConfigForm: Stub,
    ListStatusBranchConfigForm: Stub
  }
})

// The preview reads the template: it has its own test.
vi.mock('./stats/EmailStepPreview', () => ({
  EmailStepPreview: ({ canReadTemplates }: { canReadTemplates: boolean }) => (
    <div data-testid="email-preview" data-can-read-templates={String(canReadTemplates)} />
  )
}))

const context = vi.hoisted(() => ({
  canReadTemplates: true,
  readOnly: false,
  workspace: { id: 'ws-1' } as Record<string, unknown>
}))

vi.mock('./context', () => ({
  useAutomation: () => ({
    workspace: context.workspace,
    canReadTemplates: context.canReadTemplates,
    readOnly: context.readOnly,
    automationId: 'auto-1'
  })
}))

// The NodeType union has no runtime counterpart; build the list from a map typed against it so a
// new node type is a compile error here rather than a node that quietly cannot be described.
const ALL_NODE_TYPES = Object.keys({
  trigger: true,
  delay: true,
  email: true,
  branch: true,
  filter: true,
  add_to_list: true,
  remove_from_list: true,
  ab_test: true,
  webhook: true,
  list_status_branch: true
} satisfies Record<NodeType, true>) as NodeType[]

const makeNode = (
  id: string,
  nodeType: NodeType,
  config: Record<string, unknown>
): Node<AutomationNodeData> => ({
  id,
  type: nodeType,
  position: { x: 0, y: 0 },
  data: { nodeType, config, label: nodeType }
})

const renderPanel = (
  selectedNode: Node<AutomationNodeData>,
  { stats = NO_NODE_STATS, onDelete }: { stats?: NodeStatsState; onDelete?: (id: string) => void } = {}
) => {
  const onNodeUpdate = vi.fn()
  const view = render(
    <I18nProvider i18n={i18n}>
      <App>
        <NodeStatsContext.Provider value={stats}>
          <NodeConfigPanel selectedNode={selectedNode} onNodeUpdate={onNodeUpdate} workspaceId="ws-1" onDelete={onDelete} />
        </NodeStatsContext.Provider>
      </App>
    </I18nProvider>
  )
  return { onNodeUpdate, ...view }
}

describe('NodeConfigPanel description', () => {
  it('offers a description for every node type', () => {
    ALL_NODE_TYPES.forEach((nodeType) => {
      const { unmount } = renderPanel(makeNode('n1', nodeType, {}))

      expect(screen.getByLabelText('Description'), `${nodeType} description field`).toBeInTheDocument()
      unmount()
    })
  })

  it('seeds the field from the node config', () => {
    renderPanel(makeNode('n1', 'delay', { duration: 2, description: 'Let them read it first' }))

    expect(screen.getByLabelText('Description')).toHaveValue('Let them read it first')
  })

  it('shows an empty field for a node that has no description', () => {
    renderPanel(makeNode('n1', 'delay', { duration: 2 }))

    expect(screen.getByLabelText('Description')).toHaveValue('')
  })

  it('writes the description without disturbing the type-specific settings', () => {
    const { onNodeUpdate } = renderPanel(makeNode('n1', 'delay', { duration: 2, unit: 'days' }))

    fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Let them read it first' } })

    expect(onNodeUpdate).toHaveBeenCalledWith(
      'n1',
      expect.objectContaining({
        config: { duration: 2, unit: 'days', description: 'Let them read it first' }
      })
    )
  })

  it('drops the key rather than storing a blank when the field is cleared', () => {
    const { onNodeUpdate } = renderPanel(
      makeNode('n1', 'delay', { duration: 2, description: 'Let them read it first' })
    )

    fireEvent.change(screen.getByLabelText('Description'), { target: { value: '' } })

    const [, update] = onNodeUpdate.mock.calls[0]
    expect(update.config).toEqual({ duration: 2, description: undefined })
    expect(JSON.parse(JSON.stringify(update.config))).toEqual({ duration: 2 })
  })

  it('does not persist a description made only of whitespace', () => {
    // getNodeDescription treats a blank description as absent, so storing one would put a key in
    // the saved config that nothing ever renders and no amount of editing removes.
    const { onNodeUpdate } = renderPanel(makeNode('n1', 'delay', { duration: 2 }))

    fireEvent.change(screen.getByLabelText('Description'), { target: { value: '   ' } })

    const [, update] = onNodeUpdate.mock.calls[0]
    expect(update.config.description).toBeUndefined()
  })

  it('keeps the spacing the author typed around real text', () => {
    const { onNodeUpdate } = renderPanel(makeNode('n1', 'delay', { duration: 2 }))

    fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Day 1 ' } })

    const [, update] = onNodeUpdate.mock.calls[0]
    expect(update.config.description).toBe('Day 1 ')
  })

  it('reseeds the field when a different node is selected', () => {
    const { rerender } = renderPanel(makeNode('n1', 'delay', { description: 'First node' }))
    expect(screen.getByLabelText('Description')).toHaveValue('First node')

    rerender(
      <I18nProvider i18n={i18n}>
        <NodeConfigPanel
          selectedNode={makeNode('n2', 'delay', { description: 'Second node' })}
          onNodeUpdate={vi.fn()}
          workspaceId="ws-1"
        />
      </I18nProvider>
    )

    expect(screen.getByLabelText('Description')).toHaveValue('Second node')
  })

  it('renders nothing at all when no node is selected', () => {
    const { container } = render(
      <I18nProvider i18n={i18n}>
        <NodeConfigPanel selectedNode={null} onNodeUpdate={vi.fn()} workspaceId="ws-1" />
      </I18nProvider>
    )

    expect(container).toBeEmptyDOMElement()
  })
})

// Without Templates read the trigger's template filter and the email node's template are locked:
// the panel hands the editor's permission to both forms.
describe('NodeConfigPanel Templates read', () => {
  afterEach(() => {
    context.canReadTemplates = true
  })

  it.each([
    { nodeType: 'trigger' as NodeType, canRead: false },
    { nodeType: 'email' as NodeType, canRead: false },
    { nodeType: 'trigger' as NodeType, canRead: true },
    { nodeType: 'email' as NodeType, canRead: true }
  ])('passes $canRead to the $nodeType form', ({ nodeType, canRead }) => {
    context.canReadTemplates = canRead
    renderPanel(makeNode('n1', nodeType, {}))

    expect(screen.getByTestId('type-form').getAttribute('data-can-read-templates')).toBe(String(canRead))
  })
})

const statsOn = (over: Partial<NodeStatsState> = {}): NodeStatsState => ({
  ...NO_NODE_STATS,
  enabled: true,
  neverRan: false,
  canReadHistory: true,
  savedNodeIds: new Set(['n1']),
  ...over
})

const nodeStats = (over: Partial<NodeStats> = {}): NodeStats => ({
  node_type: 'email',
  visits: 10,
  completed: 8,
  failed_attempts: 0,
  skipped: 0,
  skip_reasons: {},
  errors: 0,
  failed_journeys: 1,
  in_progress: 1,
  waiting: 0,
  retrying: 0,
  branches: {},
  ...over
})

describe('NodeConfigPanel Delete step', () => {
  afterEach(() => {
    context.readOnly = false
  })

  it('deletes a step after confirming', async () => {
    const onDelete = vi.fn()
    renderPanel(makeNode('n1', 'delay', {}), { onDelete })
    fireEvent.click(screen.getByRole('button', { name: /Delete step/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(onDelete).toHaveBeenCalledWith('n1'))
  })

  it('offers no Delete step on the trigger', () => {
    renderPanel(makeNode('n1', 'trigger', {}), { onDelete: vi.fn() })
    expect(screen.queryByRole('button', { name: /Delete step/ })).not.toBeInTheDocument()
  })

  it('offers no Delete step read-only', () => {
    context.readOnly = true
    renderPanel(makeNode('n1', 'delay', {}), { onDelete: vi.fn() })
    expect(screen.queryByRole('button', { name: /Delete step/ })).not.toBeInTheDocument()
  })

  it('says a branch is edited through the API', () => {
    renderPanel(makeNode('n1', 'branch', {}))
    expect(screen.getByText('This step is edited through the API.')).toBeInTheDocument()
  })
})

describe('NodeConfigPanel results', () => {
  afterEach(() => {
    context.workspace = { id: 'ws-1' }
  })

  it('shows no results for a draft that never ran', () => {
    renderPanel(makeNode('n1', 'delay', {}))
    expect(document.querySelector('[data-step-results]')).toBeNull()
  })

  it('asks to save an unsaved step', () => {
    renderPanel(makeNode('n2', 'delay', {}), { stats: statsOn() })
    expect(screen.getByText('Save to start counting this step')).toBeInTheDocument()
  })

  it('names the period', () => {
    const { unmount } = renderPanel(makeNode('n1', 'delay', {}), { stats: statsOn({ nodes: { n1: nodeStats() } }) })
    expect(screen.getByRole('region', { name: 'Last 30 days' })).toBeInTheDocument()
    unmount()
    renderPanel(makeNode('n1', 'delay', {}), { stats: statsOn({ period: 'all', nodes: { n1: nodeStats() } }) })
    expect(screen.getByRole('region', { name: 'All time' })).toBeInTheDocument()
  })

  it('says the results could not load', () => {
    renderPanel(makeNode('n1', 'delay', {}), { stats: statsOn({ error: true }) })
    expect(screen.getByText("Couldn't load stats")).toBeInTheDocument()
  })

  it('links an email step to its messages, with its template', () => {
    renderPanel(makeNode('n1', 'email', { template_id: 'tpl-1' }), { stats: statsOn({ nodes: { n1: nodeStats() } }) })
    expect(screen.getByRole('link', { name: 'View messages' })).toHaveAttribute(
      'href',
      '/console/workspace/ws-1/logs?automation_id=auto-1&template_id=tpl-1'
    )
    expect(screen.getByTestId('email-preview')).toBeInTheDocument()
  })

  it.each([
    { case: "the step's SMTP provider", integration_id: 'smtp', marketing: 'ses', smtp: true },
    { case: "the step's API provider over the workspace's SMTP", integration_id: 'ses', marketing: 'smtp', smtp: false },
    { case: "the workspace's SMTP provider", integration_id: undefined, marketing: 'smtp', smtp: true }
  ])('reads deliveries by $case', ({ integration_id, marketing, smtp }) => {
    context.workspace = {
      id: 'ws-1',
      settings: { marketing_email_provider_id: marketing },
      integrations: [
        { id: 'smtp', email_provider: { kind: 'smtp' } },
        { id: 'ses', email_provider: { kind: 'ses' } }
      ]
    }
    renderPanel(makeNode('n1', 'email', { template_id: 'tpl-1', integration_id }), {
      stats: statsOn({ nodes: { n1: nodeStats({ email: { ...EMAIL, sent: 10, delivered: 9 } }) } })
    })
    const delivered = document.querySelector('[data-email-row="delivered"]')!
    expect(delivered.textContent?.includes('—')).toBe(smtp)
  })

  it('shows the journeys and splits of a filter', () => {
    renderPanel(makeNode('n1', 'filter', {}), {
      stats: statsOn({ nodes: { n1: nodeStats({ node_type: 'filter', branches: { yes: 3, no: 1 } }) } })
    })
    expect(screen.getByText('Reached')).toBeInTheDocument()
    const splits = document.querySelector('[data-step-splits]')!
    expect(splits).toHaveTextContent('Yes')
    expect(splits).toHaveTextContent('No')
  })
})

const EMAIL = {
  executions: 0,
  queued: 0,
  not_sent: 0,
  sent: 0,
  delivered: 0,
  opened: 0,
  opens_tracked: 0,
  clicked: 0,
  clicks_tracked: 0,
  failed: 0,
  bounced: 0,
  complained: 0,
  unsubscribed: 0
}
