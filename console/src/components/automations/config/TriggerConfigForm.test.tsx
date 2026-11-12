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

// The selectors query the API; here they only need to show their value and report a choice.
vi.mock('../../segment/BroadcastSelectorInput', () => ({
  default: ({ value, onChange }: { value?: string | null; onChange?: (v: string | null) => void }) => (
    <button data-testid="broadcast-selector" onClick={() => onChange?.('bc-1')}>
      broadcast:{value ?? 'any'}
    </button>
  )
}))
vi.mock('../../templates/TemplateSelectorInput', () => ({
  default: ({
    value,
    onChange,
    canReadTemplates
  }: {
    value?: string | null
    onChange?: (v: string | null) => void
    canReadTemplates?: boolean
  }) => (
    <button
      data-testid="template-selector"
      data-can-read-templates={String(canReadTemplates)}
      onClick={() => onChange?.('tpl-1')}
    >
      template:{value ?? 'any'}
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
  link_url?: string
  broadcast_id?: string
  template_id?: string
}

const renderForm = (config: TriggerConfigLike, onChange = vi.fn(), canReadTemplates?: boolean) => {
  render(
    <I18nProvider i18n={i18n}>
      <TriggerConfigForm
        config={config}
        onChange={onChange}
        workspaceId="ws1"
        canReadTemplates={canReadTemplates}
      />
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

describe('TriggerConfigForm tracking consent fields', () => {
  it('offers the tracking consents as fields a contact update can trigger on', async () => {
    renderForm({ event_kind: 'contact.updated', frequency: 'once' })

    const fieldFilter = screen.getByText('Trigger on specific field changes').closest('.ant-form-item') as HTMLElement
    // The option list is virtualised: narrow it so the consent group is rendered.
    await userEvent.type(within(fieldFilter).getByRole('combobox'), 'Tracking')

    expect(await screen.findByText('Opens Tracking Consent')).toBeInTheDocument()
    expect(screen.getByText('Clicks Tracking Consent')).toBeInTheDocument()
  })
})

describe('TriggerConfigForm link click filters', () => {
  it('shows the link, broadcast and template filters for a link click', () => {
    renderForm({ event_kind: 'email.link_clicked', frequency: 'every_time', link_url: '/pricing', broadcast_id: 'bc-9' })

    expect(screen.getByText('Clicked link contains')).toBeInTheDocument()
    expect(screen.getByDisplayValue('/pricing')).toBeInTheDocument()
    expect(screen.getByTestId('broadcast-selector')).toHaveTextContent('broadcast:bc-9')
    expect(screen.getByTestId('template-selector')).toHaveTextContent('template:any')
    expect(screen.getByText(/does not start it again/)).toBeInTheDocument()
  })

  // Without Templates read the template filter shows the stored ID, locked.
  it.each([
    { given: false, passed: 'false' },
    { given: true, passed: 'true' },
    { given: undefined, passed: 'true' }
  ])('gives the template filter Templates read $given', ({ given, passed }) => {
    renderForm({ event_kind: 'email.link_clicked', frequency: 'every_time', template_id: 'tpl-9' }, vi.fn(), given)

    expect(screen.getByTestId('template-selector').getAttribute('data-can-read-templates')).toBe(passed)
  })

  it('shows none of them for a plain email click', () => {
    renderForm({ event_kind: 'email.clicked', frequency: 'once' })

    expect(screen.queryByText('Clicked link contains')).not.toBeInTheDocument()
    expect(screen.queryByTestId('broadcast-selector')).not.toBeInTheDocument()
  })

  it('emits each filter it is given', async () => {
    const onChange = renderForm({ event_kind: 'email.link_clicked', frequency: 'every_time' })

    await userEvent.type(screen.getByPlaceholderText('e.g. /pricing'), 'p')
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ link_url: 'p' }))

    await userEvent.click(screen.getByTestId('broadcast-selector'))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ broadcast_id: 'bc-1' }))

    await userEvent.click(screen.getByTestId('template-selector'))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ template_id: 'tpl-1' }))
  })

  it('warns that a link click fires often', async () => {
    renderForm({ event_kind: 'email.link_clicked', frequency: 'once', conditions: contactsTree })
    await userEvent.click(screen.getByText('Edit'))
    expect(screen.getByText(/evaluated for every matching event/)).toBeInTheDocument()
  })
})

describe('TriggerConfigForm event change', () => {
  it('drops the link click filters when the event changes', async () => {
    // email.clicked takes none of them: the server would refuse the trigger.
    const onChange = renderForm({
      event_kind: 'email.link_clicked',
      frequency: 'every_time',
      link_url: '/pricing',
      broadcast_id: 'bc-9',
      template_id: 'tpl-9'
    })

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByRole('menuitemcheckbox', { name: 'Clicked' }))

    const emitted = onChange.mock.calls[onChange.mock.calls.length - 1][0]
    expect(emitted.event_kind).toBe('email.clicked')
    expect(emitted.link_url).toBeUndefined()
    expect(emitted.broadcast_id).toBeUndefined()
    expect(emitted.template_id).toBeUndefined()
  })
})
