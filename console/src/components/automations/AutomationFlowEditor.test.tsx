import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
  onNodeClick: (event: unknown, node: Node) => void
  onMoveStart: (event: unknown) => void
  onInit: () => void
  onNodeDragStart: () => void
  onNodeDragStop: (...args: unknown[]) => void
  onNodesChange: (changes: unknown[]) => void
}
const flow = vi.hoisted(() => ({
  captured: null as CapturedFlow | null,
  instance: {
    getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
    // Resolves true like the real one once its pan-zoom is ready.
    setViewport: vi.fn(async () => true),
    fitView: vi.fn(),
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
  Panel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  // Stable like the real one: the editor's effects depend on these functions.
  useReactFlow: () => flow.instance,
  useViewport: () => ({ x: 0, y: 0, zoom: 1 }),
  useConnection: () => ({ inProgress: false }),
  // Nodes whose handles change (an A/B test's variants) ask ReactFlow to re-measure them.
  useUpdateNodeInternals: () => () => {},
  Handle: ({ type, id }: { type: string; id?: string }) => (
    <div data-testid="handle" data-handle-type={type} data-handle-id={id ?? ''} />
  )
}))

import { AutomationProvider } from './context'
import { AutomationFlowEditor } from './AutomationFlowEditor'
import { REVEAL_MARGIN, TOP_GAP } from './utils/anchorViewport'
import type { AutomationEdgeData } from './edges/AutomationEdge'

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

const dropOf = (type: string) => ({ preventDefault() {}, clientX: 0, clientY: 0, dataTransfer: { getData: () => type } })

beforeEach(() => {
  flow.captured = null
  flow.instance.getViewport.mockReset().mockReturnValue({ x: 0, y: 0, zoom: 1 })
  flow.instance.setViewport.mockReset().mockResolvedValue(true)
  flow.instance.fitView.mockReset()
  // jsdom lays nothing out: the canvas gets a width so the view can be centred.
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ width: 900, height: 700 } as DOMRect)
})

// ReactFlow is ready (onInit) and has measured the trigger, as the real canvas reports them.
const ready = (measured = true) =>
  act(() => {
    flow.captured!.onInit()
    if (measured)
      flow.captured!.onNodesChange([
        { type: 'dimensions', id: 't', dimensions: { width: 300, height: 80 }, setAttributes: true }
      ])
  })

const triggerAndDelay = () =>
  makeAutomation([
    makeNode('t', 'trigger', { event_kind: 'contact.created' }, 'd'),
    makeNode('d', 'delay', { duration: 1, unit: 'days' })
  ])

describe('AutomationFlowEditor, the view', () => {
  it('opens at 100 % with the trigger centred at the top, once', async () => {
    const captured = renderEditor(triggerAndDelay())
    ready()
    const trigger = captured.nodes.find((n) => n.id === 't')!
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    // Its middle (measured 300px wide) lands on the canvas's middle.
    expect(flow.instance.setViewport).toHaveBeenCalledWith({
      x: 450 - (trigger.position.x + 150),
      y: TOP_GAP - trigger.position.y,
      zoom: 1
    })
    // A later edit does not move the view.
    act(() => captured.onDrop(dropOf('delay')))
    await act(async () => {})
    expect(flow.instance.setViewport).toHaveBeenCalledTimes(1)
    expect(flow.instance.fitView).not.toHaveBeenCalled()
  })

  it('waits for ReactFlow to be ready and the trigger to be measured', async () => {
    renderEditor(triggerAndDelay())
    await act(async () => {})
    expect(flow.instance.setViewport).not.toHaveBeenCalled()
    ready(false)
    await act(async () => {})
    expect(flow.instance.setViewport).not.toHaveBeenCalled()
    act(() =>
      flow.captured!.onNodesChange([
        { type: 'dimensions', id: 't', dimensions: { width: 300, height: 80 }, setAttributes: true }
      ])
    )
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
  })

  it('places the view again when ReactFlow was not ready for the first try', async () => {
    flow.instance.setViewport.mockResolvedValueOnce(false)
    const captured = renderEditor(triggerAndDelay())
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    act(() => captured.onDrop(dropOf('delay')))
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(2))
    expect(flow.instance.setViewport).toHaveBeenLastCalledWith(expect.objectContaining({ zoom: 1 }))
  })

  it('stops placing the view once the author moved it first', async () => {
    flow.instance.setViewport.mockResolvedValue(false)
    const captured = renderEditor(triggerAndDelay())
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    // A pan by the author (an event), then an edit: no late jump to the trigger, from a retry
    // already scheduled or from the edit.
    act(() => captured.onMoveStart({ type: 'mousedown' }))
    flow.instance.setViewport.mockResolvedValue(true)
    act(() => captured.onDrop(dropOf('delay')))
    await act(() => new Promise((resolve) => setTimeout(resolve, 250)))
    expect(flow.instance.setViewport).toHaveBeenCalledTimes(1)
  })

  it('stops trying to place the view once the editor is closed', async () => {
    flow.instance.setViewport.mockResolvedValue(false)
    const { unmount } = render(
      <Providers automation={triggerAndDelay()}>
        <AutomationFlowEditor />
      </Providers>
    )
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    unmount()
    await act(() => new Promise((resolve) => setTimeout(resolve, 250)))
    expect(flow.instance.setViewport).toHaveBeenCalledTimes(1)
  })

  it('opens a flow without a trigger on its topmost step', async () => {
    // Made through the API, rooted on a delay.
    const automation = makeAutomation([
      makeNode('d1', 'delay', { duration: 1, unit: 'days' }, 'd2'),
      makeNode('d2', 'delay', { duration: 2, unit: 'days' })
    ])
    const captured = renderEditor(automation)
    act(() => {
      flow.captured!.onInit()
      flow.captured!.onNodesChange(
        ['d1', 'd2'].map((id) => ({
          type: 'dimensions',
          id,
          dimensions: { width: 300, height: 80 },
          setAttributes: true
        }))
      )
    })
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    const top = captured.nodes
      .filter((n) => n.type !== 'placeholder')
      .reduce((a, b) => (b.position.y < a.position.y ? b : a))
    expect(flow.instance.setViewport).toHaveBeenCalledWith({
      x: 450 - (top.position.x + 150),
      y: TOP_GAP - top.position.y,
      zoom: 1
    })
  })

  it('keeps retrying after a programmatic move', async () => {
    flow.instance.setViewport.mockResolvedValueOnce(false)
    const captured = renderEditor(triggerAndDelay())
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    act(() => captured.onMoveStart(null))
    act(() => captured.onDrop(dropOf('delay')))
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(2))
  })

  it('keeps the view after the automatic layout while the selected step is in view', async () => {
    const captured = renderEditor(triggerAndDelay())
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    // A view that shows every place the steps can take, before and after the layout.
    flow.instance.getViewport.mockReturnValue({ x: 100, y: 100, zoom: 0.1 })
    // Inserting a filter on a link lays the steps out again.
    const link = captured.edges.find((e) => e.source === 't' && e.target === 'd')!
    act(() => (link.data as AutomationEdgeData).onInsert?.('filter'))
    await act(() => new Promise((resolve) => setTimeout(resolve, 200)))
    expect(flow.instance.setViewport).toHaveBeenCalledTimes(1)
    expect(flow.instance.fitView).not.toHaveBeenCalled()
  })

  it('brings the selected step back when the automatic layout moved it out of view', async () => {
    const captured = renderEditor(triggerAndDelay())
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    act(() =>
      captured.onNodeClick(
        {},
        captured.nodes.find((n) => n.id === 'd')!
      )
    )
    await act(() => new Promise((resolve) => setTimeout(resolve, 20)))
    const calls = flow.instance.setViewport.mock.calls.length
    // The author scrolled far down: the delay, laid out again, ends above the canvas.
    flow.instance.getViewport.mockReturnValue({ x: 0, y: -2000, zoom: 1 })
    const link = flow.captured!.edges.find((e) => e.source === 't' && e.target === 'd')!
    act(() => (link.data as AutomationEdgeData).onInsert?.('filter'))
    await waitFor(() => expect(flow.instance.setViewport.mock.calls.length).toBe(calls + 1))
    const [view, options] = flow.instance.setViewport.mock.calls.at(-1)! as unknown as [
      { x: number; y: number; zoom: number },
      unknown
    ]
    // Brought back down, at the same zoom, animated; never fitted.
    expect(view.zoom).toBe(1)
    expect(view.y).toBeGreaterThan(-2000)
    expect(options).toEqual({ duration: 200 })
    expect(flow.instance.fitView).not.toHaveBeenCalled()
  })

  it('puts Reorganize, the banner and the stats control in one row across the top', () => {
    render(
      <Providers automation={triggerAndDelay()}>
        <AutomationFlowEditor overlay={<div data-testid="banner" />} statsControl={<div data-testid="stats" />} />
      </Providers>
    )
    const row = document.querySelector('[data-canvas-top]')!
    const reorganize = screen.getByRole('button', { name: 'Reorganize layout' })
    const banner = screen.getByTestId('banner')
    const stats = screen.getByTestId('stats')
    for (const el of [reorganize, banner, stats]) expect(row).toContainElement(el)
    expect(reorganize.compareDocumentPosition(banner) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(banner.compareDocumentPosition(stats) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('places the trigger under the banner when one shows', async () => {
    // The canvas at the top of the page, the banner 15 to 95px down.
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      return (
        this.hasAttribute('data-canvas-overlay')
          ? { top: 15, bottom: 95, width: 576, height: 80 }
          : { top: 0, bottom: 700, width: 900, height: 700 }
      ) as DOMRect
    })
    render(
      <Providers automation={triggerAndDelay()}>
        <AutomationFlowEditor overlay={<div>live</div>} />
      </Providers>
    )
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    const trigger = flow.captured!.nodes.find((n) => n.id === 't')!
    expect(flow.instance.setViewport).toHaveBeenCalledWith(expect.objectContaining({ y: 95 + 16 - trigger.position.y }))
  })

  it('tries the opening placement again while ReactFlow cannot move the view', async () => {
    flow.instance.setViewport.mockResolvedValueOnce(false).mockResolvedValueOnce(false)
    renderEditor(triggerAndDelay())
    ready()
    // No edit in between: the retries alone place it.
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(3))
    await act(() => new Promise((resolve) => setTimeout(resolve, 250)))
    expect(flow.instance.setViewport).toHaveBeenCalledTimes(3)
  })

  it('does not pan under the pointer when a drag selects a step', async () => {
    const captured = renderEditor(triggerAndDelay())
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    const delay = captured.nodes.find((n) => n.id === 'd')!
    // Hidden behind the panel, but selected by the drag that starts on it.
    flow.instance.getViewport.mockReturnValue({ x: 800 - delay.position.x, y: 100 - delay.position.y, zoom: 1 })
    act(() => captured.onNodeDragStart())
    act(() => captured.onNodeClick({}, delay))
    await act(() => new Promise((resolve) => setTimeout(resolve, 20)))
    expect(flow.instance.setViewport).toHaveBeenCalledTimes(1)
    act(() => flow.captured!.onNodeDragStop({}, delay, [delay]))
  })

  it('slides a step the opening panel hides back into view', async () => {
    const captured = renderEditor(triggerAndDelay())
    ready()
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(1))
    const delay = captured.nodes.find((n) => n.id === 'd')!
    // The canvas is 900px wide: a view that puts the delay at 800px hides most of it (its top at
    // 100px is in view).
    const y = 100 - delay.position.y
    flow.instance.getViewport.mockReturnValue({ x: 800 - delay.position.x, y, zoom: 1 })
    act(() => captured.onNodeClick({}, delay))
    await waitFor(() => expect(flow.instance.setViewport).toHaveBeenCalledTimes(2))
    // Unmeasured, the step counts 300px wide: its right edge comes back to 900 - REVEAL_MARGIN.
    expect(flow.instance.setViewport).toHaveBeenLastCalledWith(
      { x: 800 - delay.position.x + (900 - REVEAL_MARGIN - 1100), y, zoom: 1 },
      { duration: 200 }
    )
  })

  it('fits the whole flow on Reorganize layout and on Fit to screen', async () => {
    renderEditor(triggerAndDelay())
    ready()
    fireEvent.click(screen.getByRole('button', { name: 'Reorganize layout' }))
    await waitFor(() => expect(flow.instance.fitView).toHaveBeenCalledTimes(1))
    fireEvent.click(screen.getByRole('button', { name: 'Fit to screen' }))
    expect(flow.instance.fitView).toHaveBeenCalledTimes(2)
  })
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
    act(() => captured.onDrop(dropOf('delay')))
    expect(flow.captured!.nodes.length).toBe(before)
  })

  it('adds the dropped step off read-only', () => {
    const automation = makeAutomation([makeNode('t', 'trigger', { event_kind: 'contact.created' }, 'd'), makeNode('d', 'delay', { duration: 1, unit: 'days' })])
    const captured = renderEditor(automation)
    const real = () => flow.captured!.nodes.filter((n) => n.type !== 'placeholder').length
    const before = real()
    act(() => captured.onDrop(dropOf('delay')))
    expect(real()).toBe(before + 1)
  })

  it('offers a "+" under an open output off read-only', () => {
    const automation = makeAutomation([makeNode('t', 'trigger', { event_kind: 'contact.created' }, 'd'), makeNode('d', 'delay', { duration: 1, unit: 'days' })])
    const captured = renderEditor(automation)
    expect(captured.nodes.some((n) => n.type === 'placeholder')).toBe(true)
  })

  it('keeps the step panel in its own column beside a clipped canvas, at every width', async () => {
    // A new automation opens with its trigger selected, so its panel is shown.
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nProvider i18n={i18n}>
          <App>
            <AutomationProvider workspace={workspace} lists={[]} canReadTemplates>
              <AutomationFlowEditor />
            </AutomationProvider>
          </App>
        </I18nProvider>
      </QueryClientProvider>
    )
    const aside = await screen.findByRole('complementary', { name: 'Step' })
    expect(aside.className).not.toMatch(/\babsolute\b/)
    expect(document.querySelector('[data-canvas]')?.className).toMatch(/\boverflow-hidden\b/)
  })
})
