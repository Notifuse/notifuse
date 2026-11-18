import { useState } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput, type SegmentSchemas } from './input'
import { TableSchemas } from './table_schemas'
import type { TreeNode } from '../../services/api/segment'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const listLeaf = (listId: string): TreeNode => ({
  kind: 'leaf',
  leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: listId, status: 'active' } }
})

const treeOf = (...leaves: TreeNode[]): TreeNode => ({ kind: 'branch', branch: { operator: 'and', leaves } })

const renderTree = (initial: TreeNode, schemas: SegmentSchemas = TableSchemas) => {
  const latest: { tree: TreeNode } = { tree: initial }
  const Wrapper = () => {
    const [tree, setTree] = useState(initial)
    return (
      <TreeNodeInput
        value={tree}
        onChange={(next) => {
          latest.tree = next
          setTree(next)
        }}
        schemas={schemas}
        lists={[
          { id: 'news', name: 'Newsletter' },
          { id: 'beta', name: 'Beta testers' }
        ]}
      />
    )
  }
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <Wrapper />
      </I18nProvider>
    </QueryClientProvider>
  )
  return latest
}

const addButton = () => screen.getAllByRole('button', { name: 'Condition' }).at(-1)!
const openAddMenu = async () => {
  fireEvent.click(addButton())
  return screen.findAllByRole('menuitem')
}

describe('TreeNodeInput — adding', () => {
  it('lists the four condition types with their description', async () => {
    renderTree(treeOf())
    const items = await openAddMenu()
    expect(items.map((item) => item.textContent)).toEqual([
      expect.stringContaining('Contact property'),
      expect.stringContaining('List subscription'),
      expect.stringContaining('Activity'),
      expect.stringContaining('Goal')
    ])
    expect(screen.getByText('Is or is not in a list, with a status')).toBeTruthy()
  })

  it('lists only the sources it is given', async () => {
    const { contacts, contact_lists, contact_timeline } = TableSchemas
    renderTree(treeOf(), { contacts, contact_lists, contact_timeline })
    expect(await openAddMenu()).toHaveLength(3)
    expect(screen.queryByText('Goal')).toBeNull()
  })

  it('opens the editor of the type picked', async () => {
    const latest = renderTree(treeOf())
    fireEvent.click((await openAddMenu())[1])
    expect(await screen.findByText('Confirm')).toBeTruthy()
    expect(latest.tree.branch?.leaves[0].leaf?.source).toBe('contact_lists')
  })

  it('adds an empty AND group', () => {
    const latest = renderTree(treeOf(listLeaf('news')))
    fireEvent.click(screen.getByRole('button', { name: 'Group' }))
    expect(latest.tree.branch?.leaves[1]).toEqual({ kind: 'branch', branch: { operator: 'and', leaves: [] } })
    expect(screen.getByText('Empty: add a condition or a subgroup.')).toBeTruthy()
  })

  it('switches a group to « at least one »', () => {
    const latest = renderTree(treeOf(listLeaf('news'), listLeaf('beta')))
    fireEvent.click(screen.getByTitle('at least one'))
    expect(latest.tree.branch?.operator).toBe('or')
    expect(screen.getByText('OR')).toBeTruthy()
  })
})

describe('TreeNodeInput — while a condition is open', () => {
  it('disables both add buttons', async () => {
    renderTree(treeOf(listLeaf('news')))
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    await screen.findByText('Confirm')
    expect(addButton()).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Group' })).toBeDisabled()
  })

  it('hides the delete of the other conditions and of the groups', async () => {
    renderTree(
      treeOf(listLeaf('news'), listLeaf('beta'), { kind: 'branch', branch: { operator: 'or', leaves: [listLeaf('news')] } })
    )
    expect(screen.getAllByRole('button', { name: 'Remove this condition' })).toHaveLength(3)
    expect(screen.getAllByRole('button', { name: 'Remove this group' })).toHaveLength(1)

    fireEvent.click(screen.getAllByRole('button', { name: 'Edit' })[0])
    await screen.findByText('Confirm')

    expect(screen.queryByRole('button', { name: 'Remove this condition' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Remove this group' })).toBeNull()
  })

  it('drops a new, unconfirmed condition when another one is opened', async () => {
    const latest = renderTree(treeOf(listLeaf('news')))
    fireEvent.click((await openAddMenu())[1])
    await waitFor(() => expect(latest.tree.branch?.leaves).toHaveLength(2))

    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))

    await waitFor(() => expect(latest.tree.branch?.leaves).toHaveLength(1))
    expect(latest.tree.branch?.leaves[0]).toEqual(listLeaf('news'))
  })

  it('offers no delete on the root group', () => {
    renderTree(treeOf(listLeaf('news')))
    expect(screen.queryByRole('button', { name: 'Remove this group' })).toBeNull()
  })
})

describe('TreeNodeInput — deleting', () => {
  const confirmDelete = async (index: number) => {
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove this condition' })[index])
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
  }

  // The confirmation renders in a portal, but React bubbles its clicks through the clickable row:
  // they used to open the row being deleted, on the tree from before the delete.
  it('deletes the condition confirmed, and opens nothing', async () => {
    const latest = renderTree(treeOf(listLeaf('news'), listLeaf('beta')))
    await confirmDelete(0)

    await waitFor(() => expect(latest.tree.branch?.leaves).toEqual([listLeaf('beta')]))
    expect(screen.queryByText('Confirm')).toBeNull()
    expect(addButton()).toBeEnabled()
  })

  it('leaves the editor closed after deleting the last condition', async () => {
    const latest = renderTree(treeOf(listLeaf('news')))
    await confirmDelete(0)

    await waitFor(() => expect(latest.tree.branch?.leaves).toEqual([]))
    expect(addButton()).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Group' })).toBeEnabled()
  })

  it('opens nothing when the delete is cancelled', async () => {
    const latest = renderTree(treeOf(listLeaf('news')))
    fireEvent.click(screen.getByRole('button', { name: 'Remove this condition' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }))

    expect(screen.queryByText('Confirm')).toBeNull()
    expect(latest.tree.branch?.leaves).toEqual([listLeaf('news')])
  })
})

describe('TreeNodeInput — keyboard focus', () => {
  it('lands on the first field of a condition as it opens, and back on its pen as it closes', async () => {
    renderTree(treeOf(listLeaf('news')))
    const pen = screen.getByRole('button', { name: 'Edit' })
    pen.focus()
    fireEvent.click(pen)
    await screen.findByText('Confirm')
    await waitFor(() => expect(document.activeElement?.tagName).toBe('INPUT'))

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Edit' })))
  })

  it('goes back to the add button after a delete', async () => {
    renderTree(treeOf(listLeaf('news'), listLeaf('beta')))
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove this condition' })[0])
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(document.activeElement).toBe(addButton()))
  })

  it('goes back to the add button when a new condition is cancelled', async () => {
    renderTree(treeOf())
    fireEvent.click((await openAddMenu())[1])
    await screen.findByText('Confirm')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(document.activeElement).toBe(addButton()))
  })
})

describe('TreeNodeInput — hooks for its caller', () => {
  it('reports when a condition opens and closes', async () => {
    const onEditingChange = vi.fn()
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <I18nProvider i18n={i18n}>
          <TreeNodeInput
            value={treeOf(listLeaf('news'))}
            onChange={vi.fn()}
            schemas={TableSchemas}
            lists={[{ id: 'news', name: 'Newsletter' }]}
            onEditingChange={onEditingChange}
          />
        </I18nProvider>
      </QueryClientProvider>
    )
    expect(onEditingChange).toHaveBeenLastCalledWith(false)
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    await waitFor(() => expect(onEditingChange).toHaveBeenLastCalledWith(true))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(onEditingChange).toHaveBeenLastCalledWith(false))
  })

  it('shows its empty state on an empty root only, the add buttons kept', () => {
    const { rerender } = render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nProvider i18n={i18n}>
          <TreeNodeInput value={treeOf()} schemas={TableSchemas} emptyState={<p>nothing yet</p>} />
        </I18nProvider>
      </QueryClientProvider>
    )
    expect(screen.getByText('nothing yet')).toBeTruthy()
    expect(addButton()).toHaveAttribute('data-add-condition', 'branch.leaves')
    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <I18nProvider i18n={i18n}>
          <TreeNodeInput value={treeOf(listLeaf('news'))} schemas={TableSchemas} emptyState={<p>nothing yet</p>} />
        </I18nProvider>
      </QueryClientProvider>
    )
    expect(screen.queryByText('nothing yet')).toBeNull()
  })
})
