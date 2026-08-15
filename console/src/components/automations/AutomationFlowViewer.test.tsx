import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Edge, Node, NodeTypes } from '@xyflow/react'
import { AutomationFlowViewer } from './AutomationFlowViewer'
import type { StatNodeData } from './nodes/StatNode'
import type { Automation, AutomationNode, NodeType } from '../../services/api/automation'

// Capture what the viewer hands to ReactFlow instead of mounting the canvas:
// the nodes, the edges and the nodeTypes map are exactly what these tests are
// about. Handle is rendered as an inert marker so the node components can be
// mounted on their own to discover which handle ids they declare. Node is left
// unparameterised on purpose: StatNodeData has no index signature, so writing
// Node<StatNodeData> is itself a type error in this codebase.
interface CapturedFlow {
  nodes: Node[]
  edges: Edge[]
  nodeTypes: NodeTypes
}

type StatNodeComponent = React.FC<{ data: StatNodeData }>

let captured: CapturedFlow | null = null

vi.mock('@xyflow/react', () => ({
  ReactFlow: (props: CapturedFlow) => {
    captured = { nodes: props.nodes, edges: props.edges, nodeTypes: props.nodeTypes }
    return null
  },
  ReactFlowProvider: ({ children }: { children: React.ReactNode }) => children,
  Background: () => null,
  Controls: () => null,
  BackgroundVariant: { Dots: 'dots' },
  Handle: ({ type, id }: { type: string; id?: string; position?: unknown; style?: unknown }) => (
    <div data-testid="handle" data-handle-type={type} data-handle-id={id ?? ''} />
  ),
  Position: { Top: 'top', Bottom: 'bottom' }
}))

const makeNode = (
  id: string,
  type: NodeType,
  config: Record<string, unknown> = {},
  next_node_id?: string
): AutomationNode => ({
  id,
  automation_id: 'auto-1',
  type,
  config,
  next_node_id,
  position: { x: 0, y: 0 },
  created_at: '2026-01-01T00:00:00Z'
})

const makeAutomation = (nodes: AutomationNode[]): Automation => ({
  id: 'auto-1',
  workspace_id: 'ws-1',
  name: 'Welcome Series',
  status: 'paused',
  list_id: 'newsletter',
  root_node_id: nodes[0].id,
  nodes,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
})

const renderViewer = (automation: Automation) => {
  render(
    <I18nProvider i18n={i18n}>
      <AutomationFlowViewer automation={automation} nodeStats={null} />
    </I18nProvider>
  )
  if (!captured) throw new Error('ReactFlow was never rendered')
  return captured
}

// Mount a node component on its own and read back the source handle ids it
// declares, which is what React Flow resolves an edge's sourceHandle against.
const declaredSourceHandles = (nodeTypes: NodeTypes, node: Node): (string | null)[] => {
  const Component = nodeTypes[node.type as string] as unknown as StatNodeComponent
  expect(Component, `no component registered for node type ${node.type}`).toBeDefined()
  const { container } = render(
    <I18nProvider i18n={i18n}>
      <Component data={node.data as unknown as StatNodeData} />
    </I18nProvider>
  )
  return Array.from(container.querySelectorAll('[data-handle-type="source"]')).map((el) => {
    const id = el.getAttribute('data-handle-id')
    return id ? id : null
  })
}

// A list_status_branch flow: trigger -> list check -> one node per branch.
const listStatusAutomation = () =>
  makeAutomation([
    makeNode('ws-trigger', 'trigger', {}, 'ws-check'),
    makeNode('ws-check', 'list_status_branch', {
      list_id: 'newsletter',
      not_in_list_node_id: 'ws-add',
      active_node_id: 'ws-email',
      non_active_node_id: 'ws-remove'
    }),
    makeNode('ws-add', 'add_to_list', { list_id: 'newsletter', status: 'active' }),
    makeNode('ws-email', 'email', { template_id: 'welcome' }),
    makeNode('ws-remove', 'remove_from_list', { list_id: 'newsletter' })
  ])

beforeEach(() => {
  captured = null
})

describe('AutomationFlowViewer', () => {
  it('emits the three branch edges of a list_status_branch node and leaves no orphan', () => {
    const { nodes, edges } = renderViewer(listStatusAutomation())

    const branchEdges = edges
      .filter((e) => e.source === 'ws-check')
      .map((e) => ({ handle: e.sourceHandle, target: e.target, label: e.label }))

    expect(branchEdges).toEqual([
      { handle: 'not_in_list', target: 'ws-add', label: 'Not in List' },
      { handle: 'active', target: 'ws-email', label: 'Active' },
      { handle: 'non_active', target: 'ws-remove', label: 'Non-Active' }
    ])

    // Every node but the root must be reached by an edge, otherwise the subtree
    // floats disconnected in the drawer.
    const reached = new Set(edges.map((e) => e.target))
    const orphans = nodes.map((n) => n.id).filter((id) => id !== 'ws-trigger' && !reached.has(id))
    expect(orphans).toEqual([])
  })

  it('only emits sourceHandles that the source node actually declares', () => {
    // React Flow drops an edge whose sourceHandle no node handle matches
    // (error #008) — that is how list_status_branch lost its branches while
    // being rendered by the single unnamed-handle StatNode.
    const automation = makeAutomation([
      makeNode('t', 'trigger', {}, 'f'),
      makeNode('f', 'filter', { continue_node_id: 'ab', exit_node_id: 'stop' }),
      makeNode('ab', 'ab_test', {
        variants: [
          { id: 'v-a', name: 'A', weight: 50, next_node_id: 'ls' },
          { id: 'v-b', name: 'B', weight: 50, next_node_id: 'mail-b' }
        ]
      }),
      makeNode('ls', 'list_status_branch', {
        list_id: 'newsletter',
        not_in_list_node_id: 'mail-a',
        active_node_id: 'mail-b',
        non_active_node_id: 'stop'
      }),
      makeNode('mail-a', 'email', { template_id: 'a' }),
      makeNode('mail-b', 'email', { template_id: 'b' }),
      makeNode('stop', 'remove_from_list', { list_id: 'newsletter' })
    ])

    const { nodes, edges } = renderViewer(automation)
    expect(edges.length).toBeGreaterThan(0)

    const byId = new Map(nodes.map((n) => [n.id, n]))
    const handlesByNode = new Map<string, (string | null)[]>()

    edges.forEach((edge) => {
      const source = byId.get(edge.source)
      expect(source, `edge ${edge.id} has no source node`).toBeDefined()
      if (!source) return
      if (!handlesByNode.has(source.id)) {
        handlesByNode.set(source.id, declaredSourceHandles(captured!.nodeTypes, source))
      }
      const declared = handlesByNode.get(source.id) as (string | null)[]
      const sourceType = (source.data as unknown as StatNodeData).nodeType
      // An edge with no sourceHandle binds to the node's unnamed handle.
      expect(declared, `edge ${edge.id} (${sourceType})`).toContain(edge.sourceHandle ?? null)
    })
  })

  it('registers a component for every node type it can be handed', () => {
    const { nodeTypes } = renderViewer(listStatusAutomation())

    const registered = Object.keys({
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
    } satisfies Record<NodeType, true>)

    registered.forEach((type) => {
      expect(nodeTypes[type], `no component registered for ${type}`).toBeDefined()
    })
  })
})
