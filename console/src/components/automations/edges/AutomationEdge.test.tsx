import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import type { EdgeProps } from '@xyflow/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeStats, NodeType } from '../../../services/api/automation'
import { NO_NODE_STATS, NodeStatsContext, type NodeStatsState } from '../stats/nodeStatsState'
import { AutomationEdge, type AutomationFlowEdge } from './AutomationEdge'
import { AddNodeEdge } from './AddNodeEdge'
import { pointDownLink } from './pointDownLink'

vi.mock('@xyflow/react', () => ({
  BaseEdge: () => null,
  EdgeLabelRenderer: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  getBezierPath: () => ['M0 0', 0, 0],
  getStraightPath: () => ['M0 0', 0, 0]
}))

const statsOf = (branches: Record<string, number>) => ({ branches }) as unknown as NodeStats

const statsOn = (nodes: Record<string, NodeStats>, over: Partial<NodeStatsState> = {}): NodeStatsState => ({
  ...NO_NODE_STATS,
  enabled: true,
  neverRan: false,
  nodes,
  savedNodeIds: new Set(Object.keys(nodes)),
  ...over
})

const renderEdge = (
  sourceType: NodeType,
  sourceHandleId: string | null,
  sourceConfig: Record<string, unknown> = {},
  stats: NodeStatsState = NO_NODE_STATS,
  at: { sourceX?: number; targetX?: number } = {},
  onDelete?: () => void
) => {
  const props = {
    id: 'e',
    source: 'src',
    target: 'dst',
    sourceX: at.sourceX ?? 0,
    sourceY: 0,
    targetX: at.targetX ?? 0,
    targetY: 100,
    sourceHandleId,
    data: { sourceType, sourceConfig, onDelete }
  } as unknown as EdgeProps<AutomationFlowEdge>
  return render(
    <I18nProvider i18n={i18n}>
      <NodeStatsContext.Provider value={stats}>
        <AutomationEdge {...props} />
      </NodeStatsContext.Provider>
    </I18nProvider>
  )
}

const label = () => document.querySelector('[data-edge-label]')

describe('AutomationEdge label', () => {
  it('shows only the share on a filter path, its name being in the card', () => {
    const stats = statsOn({ src: statsOf({ yes: 3, no: 1 }) })
    const { unmount } = renderEdge('filter', 'continue', {}, stats)
    expect(label()?.textContent).toBe('75.0%')
    unmount()
    renderEdge('filter', 'exit', {}, stats)
    expect(label()?.textContent).toBe('25.0%')
  })

  it('shows the share of an A/B variant and of a branch default path', () => {
    const ab = {
      variants: [
        { id: 'v-a', name: 'A', weight: 70 },
        { id: 'v-b', name: 'B', weight: 30 }
      ]
    }
    const { unmount } = renderEdge('ab_test', 'v-a', ab, statsOn({ src: statsOf({ 'v-a': 1, 'v-b': 1 }) }))
    expect(label()?.textContent).toBe('50.0%')
    unmount()
    const branch = {
      paths: [
        { id: 'p1', name: 'Pro' },
        { id: 'p2', name: 'Others' }
      ],
      default_path_id: 'p2'
    }
    renderEdge('branch', 'p2', branch, statsOn({ src: statsOf({ p1: 1, p2: 1, default: 2 }) }))
    expect(label()?.textContent).toBe('75.0%')
  })

  it('puts the share just under its own output, not halfway along the link', () => {
    renderEdge('list_status_branch', 'active', {}, statsOn({ src: statsOf({ active: 1 }) }), {
      sourceX: 100,
      targetX: 0
    })
    const box = label()!.closest('[style]') as HTMLElement
    // 26px down, on the curve from 100 to 0 (a target 100px lower).
    const { x } = pointDownLink({ x: 100, y: 0 }, { x: 0, y: 100 }, 26, { x: 0, y: 0 })
    expect(box.style.transform).toContain(`translate(${x}px,26px)`)
  })

  it('keeps the delete button halfway along the link while the share sits under its output', () => {
    renderEdge('filter', 'continue', {}, statsOn({ src: statsOf({ yes: 1 }) }), { sourceX: 100, targetX: 0 }, vi.fn())
    const at = (el: Element) => (el.closest('[style]') as HTMLElement).style.transform
    const { x } = pointDownLink({ x: 100, y: 0 }, { x: 0, y: 100 }, 26, { x: 0, y: 0 })
    expect(at(label()!)).toContain(`translate(${x}px,26px)`)
    // getBezierPath is mocked to put the middle of the link at 0, 0.
    expect(at(screen.getByRole('button', { name: 'Delete edge' }))).toContain('translate(0px,0px)')
  })

  it('writes no stray text on an editable link', () => {
    const { container } = renderEdge('delay', null, {}, NO_NODE_STATS, {}, vi.fn())
    expect(container.textContent).toBe('')
  })

  it('shows the journeys in a tooltip', async () => {
    renderEdge('filter', 'continue', {}, statsOn({ src: statsOf({ yes: 2, no: 0 }) }))
    fireEvent.mouseOver(label()!)
    expect(await screen.findByText('2 journeys')).toBeInTheDocument()
  })

  it('has no label without a share: no pass, stats off, loading or an unsaved step', () => {
    const nodes = { src: statsOf({ yes: 1 }) }
    for (const stats of [
      statsOn({ src: statsOf({}) }),
      NO_NODE_STATS,
      statsOn(nodes, { loading: true }),
      statsOn(nodes, { savedNodeIds: new Set() })
    ]) {
      const { unmount } = renderEdge('filter', 'continue', {}, stats)
      expect(label()).toBeNull()
      unmount()
    }
  })

  it('has no label for a single-exit step or the unnamed handle', () => {
    renderEdge('delay', null)
    expect(label()).toBeNull()
  })
})

describe('AddNodeEdge', () => {
  it('draws no label: the card names the output', () => {
    const props = {
      id: 'p',
      source: 'src',
      target: 'placeholder',
      sourceX: 0,
      sourceY: 0,
      targetX: 0,
      targetY: 100,
      data: { sourceNodeId: 'src', sourceHandle: 'exit', sourceType: 'filter', sourceConfig: {}, label: 'No' }
    } as unknown as EdgeProps
    render(
      <I18nProvider i18n={i18n}>
        <AddNodeEdge {...(props as Parameters<typeof AddNodeEdge>[0])} />
      </I18nProvider>
    )
    expect(document.querySelector('[data-edge-label]')).toBeNull()
    expect(screen.queryByText('No')).not.toBeInTheDocument()
  })
})
