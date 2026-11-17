import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LeafActionForm, LeafContactForm, LeafContactListForm, LeafCustomEventsGoalForm } from './form_leaf'
import { TableSchemas } from './table_schemas'
import type { EditingNodeLeaf, TreeNode, TreeNodeLeaf } from '../../services/api/segment'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

const lists = [
  { id: 'news', name: 'Newsletter' },
  { id: 'beta', name: 'Beta testers' }
]

type LeafForm = typeof LeafContactListForm

const renderForm = (Form: LeafForm, leaf: TreeNodeLeaf, schemaKey: string, onChange = vi.fn()) => {
  const node: TreeNode = { kind: 'leaf', leaf }
  const editingNodeLeaf: EditingNodeLeaf = { ...node, path: '', key: 0 }
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <Form
          value={node}
          onChange={onChange}
          source={leaf.source}
          schema={TableSchemas[schemaKey]}
          editingNodeLeaf={editingNodeLeaf}
          setEditingNodeLeaf={vi.fn()}
          cancelOrDeleteNode={vi.fn()}
          lists={lists}
        />
      </I18nProvider>
    </QueryClientProvider>
  )
  return onChange
}

const confirm = () => fireEvent.click(screen.getByText('Confirm'))

describe('LeafContactListForm', () => {
  it('starts on « is in »', () => {
    renderForm(LeafContactListForm, { source: 'contact_lists', contact_list: { list_id: '' } as never }, 'contact_lists')
    expect(document.querySelector('.ant-segmented-item-selected')?.textContent).toBe('is in')
    expect(screen.getByText('with status')).toBeTruthy()
  })

  it('saves « is not in » without a status, and hides the status', async () => {
    const onChange = renderForm(
      LeafContactListForm,
      { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news', status: 'active' } },
      'contact_lists'
    )
    fireEvent.click(screen.getByTitle('is not in'))
    await waitFor(() => expect(screen.queryByText('with status')).toBeNull())
    confirm()
    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const saved = onChange.mock.calls[0][0].leaf.contact_list
    expect(saved.operator).toBe('not_in')
    expect(saved.status).toBeUndefined()
  })

  it('requires a list and, with « is in », a status', async () => {
    const onChange = renderForm(
      LeafContactListForm,
      { source: 'contact_lists', contact_list: { operator: 'in', list_id: '' } },
      'contact_lists'
    )
    confirm()
    expect(await screen.findByText('Please select a list')).toBeTruthy()
    expect(await screen.findByText('Please select a status')).toBeTruthy()
    expect(onChange).not.toHaveBeenCalled()
  })
})

describe('Confirm with an unfinished filter row', () => {
  it('is refused for a contact filter', async () => {
    const onChange = renderForm(
      LeafContactForm,
      {
        source: 'contacts',
        contact: { filters: [{ field_name: 'email', field_type: 'string', operator: 'equals' }] }
      },
      'contacts'
    )
    confirm()
    expect(await screen.findByText('Complete or remove the unfinished filters')).toBeTruthy()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('is refused for a JSON filter without a path', async () => {
    const onChange = renderForm(
      LeafContactForm,
      {
        source: 'contacts',
        contact: {
          filters: [{ field_name: 'custom_json_1', field_type: 'string', operator: 'equals', string_values: ['x'] }]
        }
      },
      'contacts'
    )
    confirm()
    expect(await screen.findByText('Complete or remove the unfinished filters')).toBeTruthy()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('is refused for a web filter', async () => {
    const onChange = renderForm(
      LeafActionForm as LeafForm,
      {
        source: 'contact_timeline',
        contact_timeline: {
          kind: 'web.pageview',
          count_operator: 'at_least',
          count_value: 1,
          timeframe_operator: 'anytime',
          timeframe_values: [],
          filters: [{ field_name: 'path', field_type: 'string', operator: 'contains' }]
        }
      },
      'contact_timeline'
    )
    confirm()
    expect(await screen.findByText('Complete or remove the unfinished filters')).toBeTruthy()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('is refused for a goal property', async () => {
    const onChange = renderForm(
      LeafCustomEventsGoalForm as LeafForm,
      {
        source: 'custom_events_goals',
        custom_events_goal: {
          goal_type: '*',
          aggregate_operator: 'count',
          operator: 'gte',
          value: 1,
          timeframe_operator: 'anytime',
          timeframe_values: [],
          filters: [{ field_name: 'sku', field_type: 'string' } as never]
        }
      },
      'custom_events_goals'
    )
    confirm()
    expect(await screen.findByText('Complete or remove the unfinished filters')).toBeTruthy()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('saves a complete contact filter', async () => {
    const onChange = renderForm(
      LeafContactForm,
      {
        source: 'contacts',
        contact: {
          filters: [{ field_name: 'email', field_type: 'string', operator: 'equals', string_values: ['a@b.c'] }]
        }
      },
      'contacts'
    )
    confirm()
    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls[0][0].leaf.contact.filters).toEqual([
      { field_name: 'email', field_type: 'string', operator: 'equals', string_values: ['a@b.c'] }
    ])
  })
})
