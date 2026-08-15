import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { TriggerConfigForm } from './TriggerConfigForm'
import type { TreeNode } from '../../../services/api/segment'

// antd's Select and Cascader mount a ResizeObserver; jsdom doesn't provide one.
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

// The tree editor is exercised by its own tests; here it only needs to be identifiable and
// able to report a change back, so the panel's wiring is what gets asserted.
vi.mock('../../segment/input', () => ({
  TreeNodeInput: ({ onChange }: { onChange: (tree: TreeNode) => void }) => (
    <button data-testid="tree-editor" onClick={() => onChange(contactsTree)}>
      tree editor
    </button>
  )
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: { lists: [] } })
}))

const contactsTree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contacts',
          contact: {
            filters: [
              { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['US'] }
            ]
          }
        }
      }
    ]
  }
}

const timelineTree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contact_timeline',
          contact_timeline: { kind: 'email.opened', count_operator: 'at_least', count_value: 3 }
        }
      }
    ]
  }
}

interface TriggerConfigLike {
  event_kind?: string
  frequency?: 'once' | 'every_time'
  conditions?: TreeNode
}

const renderForm = (config: TriggerConfigLike, onChange = vi.fn()) => {
  render(
    <I18nProvider i18n={i18n}>
      <TriggerConfigForm config={config} onChange={onChange} workspaceId="ws1" />
    </I18nProvider>
  )
  return onChange
}

describe('TriggerConfigForm entry conditions', () => {
  it('offers to add conditions when the automation has none', () => {
    renderForm({ event_kind: 'contact.created', frequency: 'once' })

    expect(screen.getByText('Add entry conditions')).toBeInTheDocument()
    expect(screen.queryByTestId('tree-editor')).not.toBeInTheDocument()
  })

  // An automation created through the API carries conditions the console has no other way to
  // show. The panel has to say they exist even though the editor itself is behind a drawer.
  it('summarises the conditions an automation already carries', () => {
    renderForm({ event_kind: 'contact.created', frequency: 'once', conditions: contactsTree })

    expect(screen.getByText('1 condition')).toBeInTheDocument()
    expect(screen.queryByText('Add entry conditions')).not.toBeInTheDocument()
  })

  // A tree with no leaves is not "empty conditions", it is a payload the backend rejects —
  // TreeNodeBranch.Validate refuses a branch with zero leaves and the whole save 400s. So
  // opening the editor must not write anything into the config.
  it('writes nothing to the config when the editor is merely opened', async () => {
    const onChange = renderForm({ event_kind: 'contact.created', frequency: 'once' })

    await userEvent.click(screen.getByText('Add entry conditions'))

    expect(screen.getByTestId('tree-editor')).toBeInTheDocument()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('emits the tree the editor reports', async () => {
    const onChange = renderForm({ event_kind: 'contact.created', frequency: 'once' })

    await userEvent.click(screen.getByText('Add entry conditions'))
    await userEvent.click(screen.getByTestId('tree-editor'))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ conditions: contactsTree }))
  })

  it('clears the conditions when they are removed', async () => {
    const onChange = renderForm({
      event_kind: 'contact.created',
      frequency: 'once',
      conditions: contactsTree
    })

    // Removal is behind a Popconfirm whose OK button repeats the trigger's label, so confirm
    // inside the popup rather than by text alone.
    await userEvent.click(screen.getByText('Remove'))
    const popup = await screen.findByRole('tooltip')
    await userEvent.click(within(popup).getByText('Remove'))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ conditions: undefined }))
  })
})

describe('TriggerConfigForm guidance', () => {
  const inclusiveNote = /includes the event that fires the automation/
  const frequencyNote = /evaluated for every matching event/

  const openEditor = async () => userEvent.click(screen.getByText('Edit'))

  it('explains inclusive counting only when the tree reads the timeline', async () => {
    renderForm({ event_kind: 'contact.created', frequency: 'once', conditions: timelineTree })
    await openEditor()
    expect(screen.getByText(inclusiveNote)).toBeInTheDocument()
  })

  it('says nothing about counting for a condition that reads no timeline', async () => {
    renderForm({ event_kind: 'contact.created', frequency: 'once', conditions: contactsTree })
    await openEditor()
    expect(screen.queryByText(inclusiveNote)).not.toBeInTheDocument()
  })

  it('warns about cost on a frequent event', async () => {
    renderForm({ event_kind: 'email.opened', frequency: 'once', conditions: contactsTree })
    await openEditor()
    expect(screen.getByText(frequencyNote)).toBeInTheDocument()
  })

  it('says nothing about cost on an infrequent event', async () => {
    renderForm({ event_kind: 'contact.created', frequency: 'once', conditions: contactsTree })
    await openEditor()
    expect(screen.queryByText(frequencyNote)).not.toBeInTheDocument()
  })
})

describe('TriggerConfigForm existing controls', () => {
  it('still renders the trigger event and frequency controls', () => {
    renderForm({ event_kind: 'contact.created', frequency: 'once' })

    expect(screen.getByText('Trigger Event')).toBeInTheDocument()
    expect(screen.getByText('Frequency')).toBeInTheDocument()
  })

  // The field filter and the entry conditions are different things, and the panel shows both
  // for contact.updated. Their labels have to stay distinguishable.
  it('keeps the field filter distinct from the entry conditions', () => {
    renderForm({ event_kind: 'contact.updated', frequency: 'once', conditions: contactsTree })

    expect(screen.getByText('Trigger on specific field changes')).toBeInTheDocument()
    expect(screen.getByText('Entry conditions')).toBeInTheDocument()
  })
})
