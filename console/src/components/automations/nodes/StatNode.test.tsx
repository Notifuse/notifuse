import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import {
  StatNode,
  FilterStatNode,
  ListStatusStatNode,
  ABTestStatNode,
  type StatNodeData
} from './StatNode'
import type { NodeType, AutomationNodeStats } from '../../../services/api/automation'

// The stat nodes only need Handle/Position from @xyflow/react. Render each Handle
// as an inert marker carrying its props so the nodes mount without a ReactFlow
// provider and the declared handle ids stay assertable.
vi.mock('@xyflow/react', () => ({
  Handle: ({
    type,
    id,
    style
  }: {
    type: string
    id?: string
    position?: unknown
    style?: React.CSSProperties
  }) => (
    <div
      data-testid="handle"
      data-handle-type={type}
      data-handle-id={id ?? ''}
      data-handle-left={String(style?.left ?? '')}
    />
  ),
  Position: { Top: 'top', Bottom: 'bottom' }
}))

// The NodeType union has no runtime counterpart, so build the list from a map
// typed against it: adding a member to the union without adding it here is a
// compile error, and the test below then proves it renders an icon and a title.
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

const makeStats = (
  overrides: Partial<AutomationNodeStats> = {}
): AutomationNodeStats =>
  ({
    entered: 0,
    completed: 0,
    failed: 0,
    skipped: 0,
    ...overrides
  } as AutomationNodeStats)

// The stat nodes read nothing but `data` off their NodeProps, and NodeProps is
// not writable here — StatNodeData has no index signature, so spelling out
// NodeProps<StatNodeData> is itself a type error in this codebase.
type StatNodeComponent = React.FC<{ data: StatNodeData }>

const asStatNode = (Component: unknown) => Component as StatNodeComponent

const renderNode = (Component: StatNodeComponent, data: Partial<StatNodeData>) => {
  return render(
    <I18nProvider i18n={i18n}>
      <Component data={data as StatNodeData} />
    </I18nProvider>
  )
}

// The header title is the second child of the header row; the first is the icon.
const headerTitle = (container: HTMLElement) =>
  container.querySelector('.text-sm.font-medium.text-gray-800')

const sourceHandles = (container: HTMLElement) =>
  Array.from(container.querySelectorAll('[data-handle-type="source"]'))

describe('StatNode', () => {
  it('renders a non-empty title for a list_status_branch node', () => {
    // The nodeIcons/nodeLabels maps were missing the list_status_branch key, so
    // the drawer rendered the node with a blank header.
    const { container } = renderNode(asStatNode(StatNode), { nodeType: 'list_status_branch' })

    expect(headerTitle(container)?.textContent).toBe('List Status')
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it('resolves an icon and a title for every member of the NodeType union', () => {
    ALL_NODE_TYPES.forEach((nodeType) => {
      const { container, unmount } = renderNode(asStatNode(StatNode), { nodeType })

      expect(container.querySelector('svg'), `${nodeType} icon`).not.toBeNull()
      expect(headerTitle(container)?.textContent?.trim(), `${nodeType} title`).toBeTruthy()

      unmount()
    })
  })

  it('renders zeros when a node has no stats', () => {
    const { container } = renderNode(asStatNode(StatNode), { nodeType: 'delay' })

    const values = Array.from(container.querySelectorAll('.ant-statistic-content')).map(
      (el) => el.textContent
    )
    expect(values).toEqual(['0', '0'])
  })
})

describe('FilterStatNode', () => {
  it('binds its third tile to skipped, not failed', () => {
    // The tile is titled "Failed" but reads .skipped — pin it so renaming the
    // heading is a deliberate change rather than an accident.
    const { container } = renderNode(asStatNode(FilterStatNode), {
      nodeType: 'filter',
      stats: makeStats({ entered: 1, completed: 2, failed: 99, skipped: 7 })
    })

    const tile = screen.getByText('Failed').closest('.ant-statistic')
    expect(tile).toHaveTextContent('7')
    expect(tile).not.toHaveTextContent('99')
    expect(container.querySelectorAll('.ant-statistic')).toHaveLength(3)
  })
})

describe('ListStatusStatNode', () => {
  it('declares the three branch source handles at the offsets layoutNodes assumes', () => {
    // layoutNodes hard-codes 20%/50%/80% for this node type when it computes the
    // X of each child, so a handle moved anywhere else desynchronises the layout.
    const { container } = renderNode(asStatNode(ListStatusStatNode), {
      nodeType: 'list_status_branch'
    })

    const handles = sourceHandles(container).map((el) => ({
      id: el.getAttribute('data-handle-id'),
      left: el.getAttribute('data-handle-left')
    }))

    expect(handles).toEqual([
      { id: 'not_in_list', left: '20%' },
      { id: 'active', left: '50%' },
      { id: 'non_active', left: '80%' }
    ])
  })

  it('renders the branch labels and the entered/completed tiles', () => {
    const { container } = renderNode(asStatNode(ListStatusStatNode), {
      nodeType: 'list_status_branch',
      stats: makeStats({ entered: 12, completed: 9, failed: 3, skipped: 4 })
    })

    expect(headerTitle(container)?.textContent).toBe('List Status')
    expect(screen.getByText('Not in List')).toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('Non-Active')).toBeInTheDocument()

    const values = Array.from(container.querySelectorAll('.ant-statistic-content')).map(
      (el) => el.textContent
    )
    expect(values).toEqual(['12', '9'])
  })

  it('renders zeros when the node has no stats', () => {
    const { container } = renderNode(asStatNode(ListStatusStatNode), {
      nodeType: 'list_status_branch'
    })

    const values = Array.from(container.querySelectorAll('.ant-statistic-content')).map(
      (el) => el.textContent
    )
    expect(values).toEqual(['0', '0'])
  })
})

// Flow Stats used to show no descriptions at all, not even the filter node's, so a flow that read
// clearly in the editor lost its annotations the moment you opened its numbers.
describe('stat node descriptions', () => {
  const variants: Array<[string, StatNodeComponent, Partial<StatNodeData>]> = [
    ['StatNode', asStatNode(StatNode), { nodeType: 'email' }],
    ['FilterStatNode', asStatNode(FilterStatNode), { nodeType: 'filter' }],
    ['ListStatusStatNode', asStatNode(ListStatusStatNode), { nodeType: 'list_status_branch' }],
    ['ABTestStatNode', asStatNode(ABTestStatNode), { nodeType: 'ab_test' }]
  ]

  variants.forEach(([name, Component, data]) => {
    it(`shows the node description on ${name}`, () => {
      renderNode(Component, { ...data, config: { description: 'Welcome — day 1' } })

      const description = screen.getByText('Welcome — day 1')
      expect(description).toBeInTheDocument()
      expect(description).toHaveAttribute('title', 'Welcome — day 1')
    })

    it(`renders no description element on ${name} when the node has none`, () => {
      const { container } = renderNode(Component, { ...data, config: {} })

      expect(container.querySelector('[title]')).toBeNull()
      expect(headerTitle(container)?.textContent?.trim()).toBeTruthy()
    })
  })
})
