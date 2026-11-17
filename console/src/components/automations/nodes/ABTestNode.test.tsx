import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeProps } from '@xyflow/react'
import { ABTestNode } from './ABTestNode'
import type { AutomationFlowNode, AutomationNodeData } from '../utils/flowConverter'

const updateNodeInternals = vi.hoisted(() => vi.fn())

// The node mounts without a ReactFlow provider: Handle is inert and the re-measure hook is a spy.
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
  useConnection: () => ({ inProgress: false }),
  useUpdateNodeInternals: () => updateNodeInternals
}))

const variant = (id: string) => ({ id, name: `Variant ${id}`, weight: 0, next_node_id: '' })

const propsFor = (ids: string[]) => {
  const data: AutomationNodeData = { nodeType: 'ab_test', config: { variants: ids.map(variant) }, label: 'A/B test' }
  return { id: 'ab-1', data, selected: false } as NodeProps<AutomationFlowNode>
}

const wrap = (ids: string[]) => (
  <I18nProvider i18n={i18n}>
    <ABTestNode {...propsFor(ids)} />
  </I18nProvider>
)

describe('ABTestNode', () => {
  beforeEach(() => updateNodeInternals.mockClear())

  it('has ReactFlow re-measure its handles when a variant is added', () => {
    // A variant added from the panel keeps the node's size: without a re-measure, ReactFlow never
    // learns the new handle and warns "Couldn't create edge for source handle id" on every render.
    const { rerender } = render(wrap(['A', 'B']))
    updateNodeInternals.mockClear()

    rerender(wrap(['A', 'B', 'C']))

    expect(updateNodeInternals).toHaveBeenCalledWith('ab-1')
  })

  it('does not re-measure when the handles stay the same', () => {
    const { rerender } = render(wrap(['A', 'B']))
    updateNodeInternals.mockClear()

    rerender(wrap(['A', 'B']))

    expect(updateNodeInternals).not.toHaveBeenCalled()
  })
})
