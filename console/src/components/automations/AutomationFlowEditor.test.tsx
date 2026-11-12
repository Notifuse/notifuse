import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Edge, Node, NodeTypes } from '@xyflow/react'
import type { Automation, AutomationNode, NodeType } from '../../services/api/automation'
import type { Workspace } from '../../services/api/types'

// What the editor hands to ReactFlow is captured instead of mounting the canvas: the props, the
// nodes, the edges and the nodeTypes map are what these tests are about. Handle is an inert marker
// so a node component mounted on its own tells which handle ids it declares.
interface CapturedFlow {
  nodes: Node[]
  edges: Edge[]
  nodeTypes: NodeTypes
  nodesDraggable: boolean
  nodesConnectable: boolean
  edgesReconnectable: boolean
  deleteKeyCode: string[] | null
  onDrop: (event: unknown) => void
}
const flow = vi.hoisted(() => ({
  captured: null as CapturedFlow | null,
  instance: {
    getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
    setViewport: () => {},
    fitView: () => {},
    zoomIn: () => {},
    zoomOut: () => {},
    screenToFlowPosition: (p: { x: number; y: number }) => p
  }
}))

vi.mock('@xyflow/react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@xyflow/react')>()),
  ReactFlow: (props: CapturedFlow & { children?: ReactNode }) => {
    flow.captured = props
    return <div data-testid="flow">{props.children}</div>
  },
  ReactFlowProvider: ({ children }: { children: ReactNode }) => children,
  Background: () => null,
  MiniMap: () => null,
  Panel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  // Stable like the real one: the editor's effects depend on these functions.
  useReactFlow: () => flow.instance,
  useViewport: () => ({ x: 0, y: 0, zoom: 1 }),
  useConnection: () => ({ inProgress: false }),
  Handle: ({ type, id }: { type: string; id?: string }) => (
    <div data-testid="handle" data-handle-type={type} data-handle-id={id ?? ''} />
  )
}))

import { AutomationProvider } from './context'
import { AutomationFlowEditor } from './AutomationFlowEditor'

const workspace = { id: 'ws1', integrations: [], settings: { timezone: 'UTC' } } as unknown as Workspace

const makeNode = (id: string, type: NodeType, config: Record<string, unknown> = {}, next_node_id?: string): AutomationNode => ({
  id,
  automation_id: 'auto-1',
  type,
  config,
  next_node_id,
  position: { x: 0, y: 0 },
  created_at: '2026-01-01T00:00:00Z'
})

const makeAutomation = (nodes: AutomationNode[], status: Automation['status'] = 'paused'): Automation => ({
  id: 'auto-1',
  workspace_id: 'ws1',
  name: 'Welcome Series',
  status,
  list_id: 'newsletter',
  trigger: { event_kind: 'contact.created', frequency: 'once' },
  root_node_id: nodes[0].id,
  nodes,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
})

// Every kind of output: a filter, an A/B test, a list status, a branch with its default path.
const everyBranch = () =>
  makeAutomation([
    makeNode('t', 'trigger', { event_kind: 'contact.created' }, 'f'),
    makeNode('f', 'filter', { continue_node_id: 'ab', exit_node_id: 'stop' }),
    makeNode('ab', 'ab_test', {
      variants: [
        { id: 'v-a', name: 'A', weight: 50, next_node_id: 'ls' },
        { id: 'v-b', name: 'B', weight: 50, next_node_id: 'br' }
      ]
    }),
    makeNode('ls', 'list_status_branch', {
      list_id: 'newsletter',
      not_in_list_node_id: 'mail-a',
      active_node_id: 'mail-b',
      non_active_node_id: 'stop'
    }),
    makeNode('br', 'branch', {
      paths: [{ id: 'p-1', name: 'Pro', conditions: null, next_node_id: 'mail-a' }],
      default_path_id: 'p-default'
    }),
    makeNode('mail-a', 'email', { template_id: 'a' }),
    makeNode('mail-b', 'email', { template_id: 'b' }),
    makeNode('stop', 'remove_from_list', { list_id: 'newsletter' })
  ])

const Providers = ({ automation, readOnly, children }: { automation: Automation; readOnly?: boolean; children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    <I18nProvider i18n={i18n}>
      <App>
        <AutomationProvider workspace={workspace} automation={automation} lists={[]} canReadTemplates readOnly={readOnly}>
          {children}
        </AutomationProvider>
      </App>
    </I18nProvider>
  </QueryClientProvider>
)

const renderEditor = (automation: Automation, readOnly = false) => {
  render(
    <Providers automation={automation} readOnly={readOnly}>
      <AutomationFlowEditor />
    </Providers>
  )
  if (!flow.captured) throw new Error('ReactFlow was never rendered')
  return flow.captured
}

// A node component mounted on its own: the source handle ids it declares, which is what React Flow
// resolves an edge's sourceHandle against.
const declaredSourceHandles = (automation: Automation, nodeTypes: NodeTypes, node: Node): (string | null)[] => {
  const Component = nodeTypes[node.type as string] as unknown as React.FC<{ id: string; data: unknown; selected: boolean }>
  expect(Component, `no component registered for node type ${node.type}`).toBeDefined()
  const { container, unmount } = render(
    <Providers automation={automation}>
      <Component id={node.id} data={node.data} selected={false} />
    </Providers>
  )
  const ids = Array.from(container.querySelectorAll('[data-handle-type="source"]')).map((el) => el.getAttribute('data-handle-id') || null)
  unmount()
  return ids
}

beforeEach(() => {
  flow.captured = null
})

describe('AutomationFlowEditor', () => {
  it('registers a component for every node type', () => {
    const { nodeTypes } = renderEditor(everyBranch())
    const every = Object.keys({
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
    every.forEach((type) => expect(nodeTypes[type], `no component registered for ${type}`).toBeDefined())
  })

  it('only emits sourceHandles that the source node declares', () => {
    // React Flow drops an edge whose sourceHandle no handle matches (error #008).
    const automation = everyBranch()
    const { nodes, edges, nodeTypes } = renderEditor(automation)
    const real = edges.filter((e) => nodes.some((n) => n.id === e.target && n.type !== 'placeholder'))
    expect(real.length).toBeGreaterThan(0)

    const byId = new Map(nodes.map((n) => [n.id, n]))
    const handles = new Map<string, (string | null)[]>()
    for (const edge of real) {
      const source = byId.get(edge.source)
      expect(source, `edge ${edge.id} has no source node`).toBeDefined()
      if (!source) continue
      if (!handles.has(source.id)) handles.set(source.id, declaredSourceHandles(automation, nodeTypes, source))
      expect(handles.get(source.id), `edge ${edge.id} (${source.type})`).toContain(edge.sourceHandle ?? null)
    }
  })

  it('can be edited off read-only', () => {
    const captured = renderEditor(everyBranch())
    expect(captured.nodesDraggable).toBe(true)
    expect(captured.nodesConnectable).toBe(true)
    expect(captured.edgesReconnectable).toBe(true)
    expect(captured.deleteKeyCode).toEqual(['Backspace', 'Delete'])
    expect(screen.getByRole('button', { name: 'Reorganize layout' })).toBeInTheDocument()
  })

  it('is not draggable, connectable or deletable read-only, without "+", Reorganize or drop', () => {
    const automation = makeAutomation([makeNode('t', 'trigger', { event_kind: 'contact.created' }, 'd'), makeNode('d', 'delay', { duration: 1, unit: 'days' })], 'live')
    const captured = renderEditor(automation, true)
    expect(captured.nodesDraggable).toBe(false)
    expect(captured.nodesConnectable).toBe(false)
    expect(captured.edgesReconnectable).toBe(false)
    expect(captured.deleteKeyCode).toBeNull()
    expect(screen.queryByRole('button', { name: 'Reorganize layout' })).not.toBeInTheDocument()
    // The delay's open output gets no placeholder, and no node can be deleted.
    expect(captured.nodes.some((n) => n.type === 'placeholder')).toBe(false)
    expect(captured.nodes.every((n) => (n.data as { onDelete?: unknown }).onDelete === undefined)).toBe(true)

    const before = captured.nodes.length
    captured.onDrop({ preventDefault() {}, clientX: 0, clientY: 0, dataTransfer: { getData: () => 'delay' } })
    expect(flow.captured!.nodes.length).toBe(before)
  })

  it('offers a "+" under an open output off read-only', () => {
    const automation = makeAutomation([makeNode('t', 'trigger', { event_kind: 'contact.created' }, 'd'), makeNode('d', 'delay', { duration: 1, unit: 'days' })])
    const captured = renderEditor(automation)
    expect(captured.nodes.some((n) => n.type === 'placeholder')).toBe(true)
  })
})
