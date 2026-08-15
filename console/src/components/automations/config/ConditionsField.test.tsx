import { useState } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ConditionsField } from './ConditionsField'
import type { TreeNode } from '../../../services/api/segment'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('../context', () => ({
  useAutomation: () => ({
    lists: [{ id: 'list123', name: 'My List' }],
    workspace: { id: 'ws1' }
  })
}))

// The real TreeNodeInput commits a leaf the instant a source is picked from the cascader,
// before any filter exists. The mock has to be able to do the same, or the abandonment path
// below is untestable.
vi.mock('../../segment/input', () => ({
  TreeNodeInput: ({ onChange }: { onChange: (tree: TreeNode) => void }) => (
    <>
      <button data-testid="tree-editor" onClick={() => onChange(twoLeafTree)}>
        tree editor
      </button>
      <button data-testid="pick-source" onClick={() => onChange(incompleteTree)}>
        pick a source
      </button>
    </>
  )
}))

// A leaf the server would accept: a source AND a filter carrying everything the query builder
// needs. A leaf with an empty filters array is what the editor commits mid-build, and it is
// exactly what close() must prune — so the two must not be confused in fixtures.
const leaf = (field: string): TreeNode => ({
  kind: 'leaf',
  leaf: {
    source: 'contacts',
    contact: {
      filters: [{ field_name: field, field_type: 'string', operator: 'equals', string_values: ['x'] }]
    }
  }
})

const oneLeafTree: TreeNode = {
  kind: 'branch',
  branch: { operator: 'and', leaves: [leaf('country')] }
}

const twoLeafTree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [leaf('country'), { kind: 'branch', branch: { operator: 'or', leaves: [leaf('first_name')] } }]
  }
}

const emptyTree: TreeNode = { kind: 'branch', branch: { operator: 'and', leaves: [] } }

// A source picked, no filter added: what the editor commits mid-build, and what the server
// refuses with "contact condition must have at least one filter".
const incompleteTree: TreeNode = {
  kind: 'branch',
  branch: { operator: 'and', leaves: [{ kind: 'leaf', leaf: { source: 'contacts', contact: { filters: [] } } }] }
}

// Controlled, like the real panel: what the field reports becomes its next value. Without
// this the component never sees the tree the editor just committed, and the pruning on close
// has nothing to prune.
const Harness = ({
  initial,
  onChange,
  onClear
}: {
  initial?: TreeNode
  onChange: (tree: TreeNode) => void
  onClear: () => void
}) => {
  const [value, setValue] = useState(initial)
  return (
    <ConditionsField
      title="Entry conditions"
      description="Only enroll contacts matching these conditions."
      addLabel="Add entry conditions"
      value={value}
      onChange={(tree) => {
        setValue(tree)
        onChange(tree)
      }}
      onClear={() => {
        setValue(undefined)
        onClear()
      }}
    />
  )
}

const renderField = (value?: TreeNode, handlers: { onChange?: (tree: TreeNode) => void; onClear?: () => void } = {}) => {
  const onChange = handlers.onChange ?? vi.fn()
  const onClear = handlers.onClear ?? vi.fn()
  render(
    <I18nProvider i18n={i18n}>
      <Harness initial={value} onChange={onChange} onClear={onClear} />
    </I18nProvider>
  )
  return { onChange, onClear }
}

// Removing a tree is behind a Popconfirm, whose OK button repeats the trigger's own label — so the
// confirmation has to be clicked inside the popup rather than by text alone.
const confirmRemove = async () => {
  await userEvent.click(screen.getByText('Remove'))
  const popup = await screen.findByRole('tooltip')
  await userEvent.click(within(popup).getByText('Remove'))
}

describe('ConditionsField', () => {
  it('shows the add affordance and no editor when there is nothing configured', () => {
    renderField(undefined)

    expect(screen.getByText('Add entry conditions')).toBeInTheDocument()
    expect(screen.queryByTestId('tree-editor')).not.toBeInTheDocument()
  })

  // A branch with no leaves is what an editor hands back for "nothing configured". Treating it
  // as configured would show a summary of zero conditions and, worse, let it reach the API.
  it('treats a branch with no leaves as nothing configured', () => {
    renderField(emptyTree)

    expect(screen.getByText('Add entry conditions')).toBeInTheDocument()
  })

  it('counts every leaf, including those nested in sub-branches', () => {
    renderField(twoLeafTree)

    expect(screen.getByText('2 conditions')).toBeInTheDocument()
  })

  it('uses the singular for a single condition', () => {
    renderField(oneLeafTree)

    expect(screen.getByText('1 condition')).toBeInTheDocument()
  })

  it('keeps the editor out of the panel until it is asked for', async () => {
    renderField(oneLeafTree)

    expect(screen.queryByTestId('tree-editor')).not.toBeInTheDocument()
    await userEvent.click(screen.getByText('Edit'))
    expect(screen.getByTestId('tree-editor')).toBeInTheDocument()
  })

  // Edits write through as they happen; there is no draft to commit. The drawer's only control
  // closes it, which is why it says Done and not OK.
  it('reports an edit to the caller without waiting for the drawer to close', async () => {
    const { onChange } = renderField(oneLeafTree)

    await userEvent.click(screen.getByText('Edit'))
    await userEvent.click(screen.getByTestId('tree-editor'))

    expect(onChange).toHaveBeenCalledWith(twoLeafTree)
  })

  it('closes the drawer on Done without touching the value', async () => {
    const { onChange, onClear } = renderField(oneLeafTree)

    await userEvent.click(screen.getByText('Edit'))
    await userEvent.click(screen.getByText('Done'))

    expect(onChange).not.toHaveBeenCalled()
    expect(onClear).not.toHaveBeenCalled()
  })

  // Left in place, this condition blocks every later save of the automation — including
  // unrelated edits — with an error the console does not surface.
  it('drops a condition abandoned before any filter was added', async () => {
    const { onChange, onClear } = renderField(undefined)

    await userEvent.click(screen.getByText('Add entry conditions'))
    await userEvent.click(screen.getByTestId('pick-source'))
    await userEvent.click(screen.getByText('Done'))

    // The editor reported the half-built leaf while it was open...
    expect(onChange).toHaveBeenCalledWith(incompleteTree)
    // ...and closing took it back out.
    expect(onClear).toHaveBeenCalled()
  })

  it('drops an abandoned condition when the drawer is dismissed rather than confirmed', async () => {
    const { onClear } = renderField(incompleteTree)

    await userEvent.click(screen.getByText('Edit'))
    await userEvent.click(screen.getByLabelText('Close'))

    expect(onClear).toHaveBeenCalled()
  })

  it('keeps a complete tree untouched on close', async () => {
    const { onChange, onClear } = renderField(oneLeafTree)

    await userEvent.click(screen.getByText('Edit'))
    await userEvent.click(screen.getByText('Done'))

    expect(onChange).not.toHaveBeenCalled()
    expect(onClear).not.toHaveBeenCalled()
  })

  it('clears through the caller rather than emitting an empty tree', async () => {
    const { onClear, onChange } = renderField(oneLeafTree)

    await confirmRemove()

    expect(onClear).toHaveBeenCalled()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('keeps the tree when the removal is not confirmed', async () => {
    const { onClear, onChange } = renderField(oneLeafTree)

    await userEvent.click(screen.getByText('Remove'))
    await userEvent.click(await screen.findByText('Cancel'))

    expect(onClear).not.toHaveBeenCalled()
    expect(onChange).not.toHaveBeenCalled()
  })
})
