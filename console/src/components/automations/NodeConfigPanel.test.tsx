import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Node } from '@xyflow/react'
import { NodeConfigPanel } from './NodeConfigPanel'
import type { AutomationNodeData } from './utils/flowConverter'
import type { NodeType } from '../../services/api/automation'

// The per-type forms are exercised by their own tests; stubbing the barrel keeps this one about
// the description field the panel owns, and keeps a single textbox on screen to address.
vi.mock('./config', () => {
  const Stub = () => <div data-testid="type-form" />
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

vi.mock('./context', () => ({
  useAutomation: () => ({ workspace: { id: 'ws-1' } })
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

const renderPanel = (selectedNode: Node<AutomationNodeData>) => {
  const onNodeUpdate = vi.fn()
  const view = render(
    <I18nProvider i18n={i18n}>
      <NodeConfigPanel selectedNode={selectedNode} onNodeUpdate={onNodeUpdate} workspaceId="ws-1" />
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
