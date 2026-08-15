import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeProps } from '@xyflow/react'
import { FilterNode } from './FilterNode'
import type { AutomationFlowNode, AutomationNodeData } from '../utils/flowConverter'
import type { TreeNode } from '../../../services/api/segment'

// FilterNode only needs Handle/Position/useConnection from @xyflow/react; render them as inert so
// the node can mount without a ReactFlow provider.
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
  useConnection: () => ({ inProgress: false })
}))

const leaf: TreeNode = {
  kind: 'leaf',
  leaf: {
    table: 'contacts',
    filters: [{ field_name: 'email', field_type: 'string', operator: 'is_set' }]
  }
} as unknown as TreeNode

const treeOf = (count: number): TreeNode =>
  ({
    kind: 'branch',
    branch: { operator: 'and', leaves: Array.from({ length: count }, () => leaf) }
  }) as unknown as TreeNode

const renderNode = (config: Record<string, unknown>) => {
  const data: AutomationNodeData = { nodeType: 'filter', config, label: 'Filter' }
  // FilterNode only reads `data` and `selected`; the rest of NodeProps is inert here.
  const props = { data, selected: false } as NodeProps<AutomationFlowNode>
  return render(
    <I18nProvider i18n={i18n}>
      <FilterNode {...props} />
    </I18nProvider>
  )
}

describe('FilterNode', () => {
  it('summarises the conditions by count whether or not a description is set', () => {
    // The description used to be folded into this line, replacing the count entirely. It is
    // rendered by BaseNode now like every other node type's, so the count always shows.
    renderNode({ conditions: treeOf(3) })
    expect(screen.getByText('3 conditions')).toBeInTheDocument()

    renderNode({ conditions: treeOf(3), description: 'Active users only' })
    expect(screen.getAllByText('3 conditions')).toHaveLength(2)
  })

  it('shows the description once, separately from the count', () => {
    renderNode({ conditions: treeOf(2), description: 'Active users only' })

    expect(screen.getAllByText('Active users only')).toHaveLength(1)
    expect(screen.getByText('2 conditions')).toBeInTheDocument()
    // The old rendering appended the count to the description as "Active users only (2)".
    expect(screen.queryByText(/Active users only \(/)).not.toBeInTheDocument()
  })

  it('says condition in the singular for a lone condition', () => {
    renderNode({ conditions: treeOf(1) })

    expect(screen.getByText('1 condition')).toBeInTheDocument()
  })

  it('warns when the filter has no conditions at all', () => {
    renderNode({ description: 'Active users only' })

    expect(screen.getByText('No conditions')).toBeInTheDocument()
    // A described but unconfigured filter still has to read as unconfigured.
    expect(screen.getByText('Active users only')).toBeInTheDocument()
  })

  it('labels the two outgoing paths', () => {
    renderNode({ conditions: treeOf(1) })

    expect(screen.getByText('Yes')).toBeInTheDocument()
    expect(screen.getByText('No')).toBeInTheDocument()
  })
})
