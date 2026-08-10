import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { LeafActionForm, LeafContactListForm } from './form_leaf'
import { TableSchemas } from './table_schemas'
import type {
  ContactListCondition,
  ContactTimelineCondition,
  EditingNodeLeaf,
  TreeNode
} from '../../services/api/segment'

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

const renderListForm = (
  contactList: ContactListCondition,
  onDraftChange: (leaf: TreeNode) => void
) => {
  const node: TreeNode = { kind: 'leaf', leaf: { source: 'contact_lists', contact_list: contactList } }
  const editingNodeLeaf: EditingNodeLeaf = { ...node, path: '', key: 0 }

  return render(
    <I18nProvider i18n={i18n}>
      <LeafContactListForm
        value={node}
        onChange={vi.fn()}
        onDraftChange={onDraftChange}
        source="contact_lists"
        schema={TableSchemas.contact_lists}
        editingNodeLeaf={editingNodeLeaf}
        setEditingNodeLeaf={vi.fn()}
        cancelOrDeleteNode={vi.fn()}
        lists={lists}
      />
    </I18nProvider>
  )
}

const renderActivityForm = (
  timeline: ContactTimelineCondition,
  onDraftChange: (leaf: TreeNode) => void
) => {
  const node: TreeNode = {
    kind: 'leaf',
    leaf: { source: 'contact_timeline', contact_timeline: timeline }
  }
  const editingNodeLeaf: EditingNodeLeaf = { ...node, path: '', key: 0 }

  return render(
    <I18nProvider i18n={i18n}>
      <LeafActionForm
        value={node}
        onChange={vi.fn()}
        onDraftChange={onDraftChange}
        source="contact_timeline"
        schema={TableSchemas.contact_timeline}
        editingNodeLeaf={editingNodeLeaf}
        setEditingNodeLeaf={vi.fn()}
        cancelOrDeleteNode={vi.fn()}
      />
    </I18nProvider>
  )
}

const lastDraft = (onDraftChange: ReturnType<typeof vi.fn>) =>
  onDraftChange.mock.calls[onDraftChange.mock.calls.length - 1][0] as TreeNode

const openSelect = (index: number) => fireEvent.mouseDown(screen.getAllByRole('combobox')[index])

/**
 * The select currently displaying `value`. Both the control and the matching dropdown option
 * carry the value as a title, so the combobox they wrap is what tells them apart.
 */
const selectShowing = (value: string) =>
  screen.getAllByTitle(value).find((element) => element.querySelector('[role="combobox"]'))

describe('leaf forms — draft reporting', () => {
  it('reports a list condition before it is confirmed', async () => {
    const onDraftChange = vi.fn()
    renderListForm({ operator: 'in', list_id: '', status: undefined }, onDraftChange)

    // The list select sits between the operator and the status ones
    openSelect(1)
    fireEvent.click(await screen.findByTitle('Product updates'))

    await waitFor(() => expect(onDraftChange).toHaveBeenCalled())
    expect(lastDraft(onDraftChange).leaf?.contact_list?.list_id).toBe('product')
  })

  it('reports the condition without touching the confirm handler', async () => {
    const onDraftChange = vi.fn()
    const onChange = vi.fn()
    const node: TreeNode = {
      kind: 'leaf',
      leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: '' } }
    }

    render(
      <I18nProvider i18n={i18n}>
        <LeafContactListForm
          value={node}
          onChange={onChange}
          onDraftChange={onDraftChange}
          source="contact_lists"
          schema={TableSchemas.contact_lists}
          editingNodeLeaf={{ ...node, path: '', key: 0 }}
          setEditingNodeLeaf={vi.fn()}
          cancelOrDeleteNode={vi.fn()}
          lists={lists}
        />
      </I18nProvider>
    )

    openSelect(1)
    fireEvent.click(await screen.findByTitle('Newsletter'))

    await waitFor(() => expect(onDraftChange).toHaveBeenCalled())
    // A draft is not a commit: the tree only changes on Confirm
    expect(onChange).not.toHaveBeenCalled()
  })

  it('reports an activity condition as the event kind is picked', async () => {
    const onDraftChange = vi.fn()
    renderActivityForm(
      {
        kind: '',
        count_operator: 'at_least',
        count_value: 1,
        timeframe_operator: 'anytime',
        timeframe_values: []
      },
      onDraftChange
    )

    openSelect(0)
    fireEvent.click(await screen.findByTitle('Open email'))

    await waitFor(() => expect(onDraftChange).toHaveBeenCalled())
    expect(lastDraft(onDraftChange).leaf?.contact_timeline?.kind).toBe('email.opened')
  })

  const dateRange = {
    kind: 'email.opened' as const,
    count_operator: 'at_least' as const,
    count_value: 1,
    timeframe_operator: 'in_date_range' as const,
    timeframe_values: ['2026-01-01T00:00:00.000Z', '2026-02-01T00:00:00.000Z']
  }

  // Selects, in order: event kind, count operator, timeframe operator
  const switchTimeframeTo = async (label: string) => {
    openSelect(2)
    fireEvent.click(await screen.findByTitle(label))
  }

  it('drops a range when the timeframe becomes a day count', async () => {
    const onDraftChange = vi.fn()
    renderActivityForm(dateRange, onDraftChange)

    await switchTimeframeTo('in the last')

    await waitFor(() => expect(onDraftChange).toHaveBeenCalled())
    const timeline = lastDraft(onDraftChange).leaf?.contact_timeline
    expect(timeline?.timeframe_operator).toBe('in_the_last_days')
    // Two ISO dates are not a day count: the compiler wants exactly one value, so leaving them
    // behind made the condition unsavable (and, read as a number, meant "the last 2026 days")
    expect(timeline?.timeframe_values).toEqual([])
  })

  it('drops a range when the timeframe becomes a single date', async () => {
    const onDraftChange = vi.fn()
    renderActivityForm(dateRange, onDraftChange)

    await switchTimeframeTo('before date')

    await waitFor(() => expect(onDraftChange).toHaveBeenCalled())
    const timeline = lastDraft(onDraftChange).leaf?.contact_timeline
    expect(timeline?.timeframe_operator).toBe('before_date')
    // The picker starts empty rather than silently inheriting the range's start date
    expect((timeline?.timeframe_values ?? []).filter(Boolean)).toEqual([])
  })

  it('keeps working without a draft handler', async () => {
    const node: TreeNode = {
      kind: 'leaf',
      leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: '' } }
    }

    render(
      <I18nProvider i18n={i18n}>
        <LeafContactListForm
          value={node}
          onChange={vi.fn()}
          source="contact_lists"
          schema={TableSchemas.contact_lists}
          editingNodeLeaf={{ ...node, path: '', key: 0 }}
          setEditingNodeLeaf={vi.fn()}
          cancelOrDeleteNode={vi.fn()}
          lists={lists}
        />
      </I18nProvider>
    )

    openSelect(1)
    fireEvent.click(await screen.findByTitle('Newsletter'))

    await waitFor(() => expect(selectShowing('Newsletter')).toBeTruthy())
  })
})
