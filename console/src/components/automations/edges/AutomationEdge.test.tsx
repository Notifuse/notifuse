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
  stats: NodeStatsState = NO_NODE_STATS
) => {
  const props = {
    id: 'e',
    source: 'src',
    target: 'dst',
    sourceX: 0,
    sourceY: 0,
    targetX: 0,
    targetY: 100,
    sourceHandleId,
    data: { sourceType, sourceConfig }
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
  it('names the filter paths Yes and No, with their share of the passes', () => {
    const stats = statsOn({ src: statsOf({ yes: 3, no: 1 }) })
    const { unmount } = renderEdge('filter', 'continue', {}, stats)
    expect(label()).toHaveTextContent('Yes · 75.0%')
    unmount()
    renderEdge('filter', 'exit', {}, stats)
    expect(label()).toHaveTextContent('No · 25.0%')
  })

  it('names an A/B variant with its weight and share', () => {
    const config = { variants: [{ id: 'v-a', name: 'A', weight: 70 }, { id: 'v-b', name: 'B', weight: 30 }] }
    renderEdge('ab_test', 'v-a', config, statsOn({ src: statsOf({ 'v-a': 1, 'v-b': 1 }) }))
    expect(label()).toHaveTextContent('A (70 %) · 50.0%')
  })

  it('counts the default path of a branch with its own id', () => {
    const config = { paths: [{ id: 'p1', name: 'Pro' }, { id: 'p2', name: 'Others' }], default_path_id: 'p2' }
    renderEdge('branch', 'p2', config, statsOn({ src: statsOf({ p1: 1, p2: 1, default: 2 }) }))
    expect(label()).toHaveTextContent('Others · 75.0%')
  })

  it('shows the journeys in a tooltip', async () => {
    renderEdge('filter', 'continue', {}, statsOn({ src: statsOf({ yes: 2, no: 0 }) }))
    fireEvent.mouseOver(label()!)
    expect(await screen.findByText('2 journeys')).toBeInTheDocument()
  })

  it('gives no percentage without any pass', () => {
    renderEdge('filter', 'continue', {}, statsOn({ src: statsOf({}) }))
    expect(label()?.textContent).toBe('Yes')
  })

  it('gives the base label only with stats off, loading or on an unsaved step', () => {
    const nodes = { src: statsOf({ yes: 1 }) }
    for (const stats of [NO_NODE_STATS, statsOn(nodes, { loading: true }), statsOn(nodes, { savedNodeIds: new Set() })]) {
      const { unmount } = renderEdge('filter', 'continue', {}, stats)
      expect(label()?.textContent).toBe('Yes')
      unmount()
    }
  })

  it('names the list status paths', () => {
    for (const [handle, text] of [
      ['not_in_list', 'Not in list'],
      ['active', 'Active'],
      ['non_active', 'Non-active']
    ]) {
      const { unmount } = renderEdge('list_status_branch', handle)
      expect(label()).toHaveTextContent(text)
      unmount()
    }
  })

  it('has no label for a single-exit step or the unnamed handle', () => {
    renderEdge('delay', null)
    expect(label()).toBeNull()
  })
})

describe('AddNodeEdge label', () => {
  it('names the open path the "+" continues', () => {
    const props = {
      id: 'p',
      source: 'src',
      target: 'placeholder',
      sourceX: 0,
      sourceY: 0,
      targetX: 0,
      targetY: 100,
      data: { sourceNodeId: 'src', sourceHandle: 'exit', sourceType: 'filter', sourceConfig: {} }
    } as unknown as EdgeProps
    render(
      <I18nProvider i18n={i18n}>
        <AddNodeEdge {...(props as Parameters<typeof AddNodeEdge>[0])} />
      </I18nProvider>
    )
    expect(screen.getByText('No')).toBeInTheDocument()
  })
})
