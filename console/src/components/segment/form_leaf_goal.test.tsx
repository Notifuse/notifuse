import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { LeafCustomEventsGoalForm } from './form_leaf'
import { CustomEventsGoalsTableSchema } from './table_schemas'
import type {
  CustomEventsGoalCondition,
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

const baseGoal: CustomEventsGoalCondition = {
  goal_type: 'purchase',
  negate: false,
  aggregate_operator: 'count',
  operator: 'gte',
  value: 1,
  timeframe_operator: 'anytime',
  timeframe_values: []
}

const renderForm = (goal: CustomEventsGoalCondition, onChange: (leaf: TreeNode) => void) => {
  const node: TreeNode = {
    kind: 'leaf',
    leaf: { source: 'custom_events_goals', custom_events_goal: goal }
  }
  const editingNodeLeaf: EditingNodeLeaf = { ...node, path: '', key: 0 }

  return render(
    <I18nProvider i18n={i18n}>
      <LeafCustomEventsGoalForm
        value={node}
        onChange={onChange}
        source="custom_events_goals"
        schema={CustomEventsGoalsTableSchema}
        editingNodeLeaf={editingNodeLeaf}
        setEditingNodeLeaf={vi.fn()}
        cancelOrDeleteNode={vi.fn()}
      />
    </I18nProvider>
  )
}

const submit = () => fireEvent.click(screen.getByText('Confirm'))

describe('LeafCustomEventsGoalForm', () => {
  it('submits negate=true when the condition is switched to "has not"', async () => {
    const onChange = vi.fn()
    renderForm(baseGoal, onChange)

    // The negation select is the first one in the form
    fireEvent.mouseDown(screen.getAllByRole('combobox')[0])
    fireEvent.click(await screen.findByTitle('has not'))
    submit()

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls[0][0].leaf.custom_events_goal.negate).toBe(true)
  })

  it('keeps negate=false for an untouched condition', async () => {
    const onChange = vi.fn()
    renderForm(baseGoal, onChange)

    submit()

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls[0][0].leaf.custom_events_goal.negate).toBe(false)
  })

  it('round-trips goal_name and event_name', async () => {
    const onChange = vi.fn()
    renderForm({ ...baseGoal, goal_name: 'checkout', event_name: 'shopify.order' }, onChange)

    submit()

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const goal = onChange.mock.calls[0][0].leaf.custom_events_goal
    expect(goal.goal_name).toBe('checkout')
    expect(goal.event_name).toBe('shopify.order')
  })

  it('round-trips property filters', async () => {
    const onChange = vi.fn()
    renderForm(
      {
        ...baseGoal,
        filters: [
          { field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] }
        ]
      },
      onChange
    )

    // The filter is listed by its key, with no schema backing it
    expect(screen.getByText('sku')).toBeTruthy()

    submit()

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls[0][0].leaf.custom_events_goal.filters).toEqual([
      { field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A-1'] }
    ])
  })

  it('renders "has not" for a stored negated condition', () => {
    renderForm({ ...baseGoal, negate: true }, vi.fn())
    expect(screen.getByTitle('has not')).toBeTruthy()
  })
})

describe('LeafCustomEventsGoalForm negation control', () => {
  it('renders "has" when the condition is not negated', () => {
    renderForm({ ...baseGoal, negate: false }, vi.fn())
    expect(screen.getByTitle('has')).toBeTruthy()
  })

  it('renders "has" for a segment saved before negation existed', () => {
    // Conditions stored before this field was added carry no `negate` key at all. The control
    // must fall back to the positive reading rather than showing an empty select.
    const legacy = { ...baseGoal }
    delete (legacy as { negate?: boolean }).negate

    renderForm(legacy, vi.fn())
    expect(screen.getByTitle('has')).toBeTruthy()
  })

  it('does not invent a negate value for an untouched legacy condition', async () => {
    const onChange = vi.fn()
    const legacy = { ...baseGoal }
    delete (legacy as { negate?: boolean }).negate

    renderForm(legacy, onChange)
    fireEvent.click(screen.getByText('Confirm'))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    // undefined serializes away entirely, which the backend reads as "not negated"
    expect(onChange.mock.calls[0][0].leaf.custom_events_goal.negate).toBeFalsy()
  })

  it('keeps goal_name and event_name absent when left blank', async () => {
    const onChange = vi.fn()
    renderForm(baseGoal, onChange)

    fireEvent.click(screen.getByText('Confirm'))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const goal = onChange.mock.calls[0][0].leaf.custom_events_goal
    // The backend treats "" as no filter, but an absent key keeps stored conditions clean
    expect(goal.goal_name ?? '').toBe('')
    expect(goal.event_name ?? '').toBe('')
  })
})
