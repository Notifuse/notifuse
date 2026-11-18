import { describe, it, expect, vi, type Mock } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput } from './input'
import { TableSchemas } from './table_schemas'
import type { TreeNode } from '../../services/api/segment'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const lists = [
  { id: 'newsletter', name: 'Newsletter' },
  { id: 'product', name: 'Product updates' }
]

const twoConditions: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contact_lists',
          contact_list: { operator: 'in', list_id: 'newsletter', status: 'active' }
        }
      },
      {
        kind: 'leaf',
        leaf: {
          source: 'contact_lists',
          contact_list: { operator: 'in', list_id: 'newsletter', status: 'active' }
        }
      }
    ]
  }
}

const renderTree = (onDraftTreeChange: (tree: TreeNode | undefined) => void) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <TreeNodeInput
          value={twoConditions}
          onChange={vi.fn()}
          onDraftTreeChange={onDraftTreeChange}
          schemas={{ contact_lists: TableSchemas.contact_lists }}
          lists={lists}
        />
      </I18nProvider>
    </QueryClientProvider>
  )
}

const editCondition = (index: number) =>
  fireEvent.click(document.querySelectorAll('[data-icon="pen-to-square"]')[index])

const openSelect = (index: number) => fireEvent.mouseDown(screen.getAllByRole('combobox')[index])

const lastCall = (mock: Mock) =>
  mock.mock.calls[mock.mock.calls.length - 1][0] as TreeNode | undefined

describe('TreeNodeInput — draft tree', () => {
  it('reports the whole tree with the edited condition swapped in', async () => {
    const onDraftTreeChange = vi.fn()
    renderTree(onDraftTreeChange)

    editCondition(1)

    // The group's all / any and the condition's in / not in are segmented controls: the list
    // select is the first combobox
    openSelect(0)
    fireEvent.click(await screen.findByTitle('Product updates'))

    await waitFor(() => expect(lastCall(onDraftTreeChange)).toBeDefined())
    const draft = lastCall(onDraftTreeChange)!
    // The edited condition carries the new list...
    expect(draft.branch?.leaves[1].leaf?.contact_list?.list_id).toBe('product')
    // ...and its sibling is untouched, so the count reflects the whole segment
    expect(draft.branch?.leaves[0].leaf?.contact_list?.list_id).toBe('newsletter')
    // The condition keeps its status: its form takes it, so it may be counted
    expect(onDraftTreeChange.mock.lastCall?.[1]).toBe(true)
  })

  it('withdraws the draft once the condition is confirmed', async () => {
    const onDraftTreeChange = vi.fn()
    renderTree(onDraftTreeChange)

    editCondition(0)
    openSelect(0)
    fireEvent.click(await screen.findByTitle('Product updates'))
    await waitFor(() => expect(lastCall(onDraftTreeChange)).toBeDefined())

    fireEvent.click(screen.getByText('Confirm'))

    // The committed tree is authoritative again
    await waitFor(() => expect(lastCall(onDraftTreeChange)).toBeUndefined())
  })

  it('withdraws the draft when the edit is cancelled', async () => {
    const onDraftTreeChange = vi.fn()
    renderTree(onDraftTreeChange)

    editCondition(0)
    openSelect(0)
    fireEvent.click(await screen.findByTitle('Product updates'))
    await waitFor(() => expect(lastCall(onDraftTreeChange)).toBeDefined())

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => expect(lastCall(onDraftTreeChange)).toBeUndefined())
  })

  it('withdraws the draft when another condition is opened', async () => {
    const onDraftTreeChange = vi.fn()
    renderTree(onDraftTreeChange)

    editCondition(0)
    openSelect(0)
    fireEvent.click(await screen.findByTitle('Product updates'))
    await waitFor(() => expect(lastCall(onDraftTreeChange)).toBeDefined())

    // The pen of the condition that is still closed
    editCondition(0)

    await waitFor(() => expect(lastCall(onDraftTreeChange)).toBeUndefined())
  })
})
