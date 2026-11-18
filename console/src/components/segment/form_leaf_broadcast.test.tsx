import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput } from './input'
import { LeafActionForm } from './form_leaf'
import { TableSchemas } from './table_schemas'
import type { ContactTimelineCondition, EditingNodeLeaf, TreeNode } from '../../services/api/segment'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('../../services/api/template', () => ({
  TEMPLATE_USAGE_TYPES: ['broadcasts', 'transactional_notifications', 'automations', 'lists'],
  templatesApi: {
    list: vi.fn(async () => ({ templates: [] })),
    get: vi.fn(async () => ({ template: { id: 'tpl-welcome', name: 'Welcome' } })),
    usage: vi.fn(),
    compile: vi.fn()
  }
}))
vi.mock('../../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ permissions: null }),
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace', settings: {}, integrations: [] }] })
}))
vi.mock('../templates/TemplatePreviewDrawer', () => ({ default: () => null }))
vi.mock('../templates/CreateTemplateDrawer', () => ({ CreateTemplateDrawer: () => null }))
// The selector has its own tests: here, only what the condition hands it.
vi.mock('./BroadcastSelectorInput', () => ({
  default: ({ templateId }: { templateId?: string | null }) => (
    <span data-testid="broadcast-selector" data-template-id={templateId ?? ''} />
  )
}))

const tree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contact_timeline',
          contact_timeline: {
            kind: 'email.opened',
            template_id: 'tpl-welcome',
            count_operator: 'at_least',
            count_value: 1,
            timeframe_operator: 'anytime',
            timeframe_values: []
          }
        }
      }
    ]
  }
}

describe('segment email condition broadcast filter', () => {
  it('offers only the broadcasts of the template chosen beside it', async () => {
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <I18nProvider i18n={i18n}>
          <TreeNodeInput value={tree} onChange={vi.fn()} schemas={TableSchemas} workspaceId="ws1" />
        </I18nProvider>
      </QueryClientProvider>
    )
    fireEvent.click(document.querySelectorAll('[data-icon="pen-to-square"]')[0])
    expect(await screen.findByTestId('broadcast-selector')).toHaveAttribute('data-template-id', 'tpl-welcome')
  })
})

describe('segment « Email sent » condition', () => {
  const sent: ContactTimelineCondition = {
    kind: 'email.sent',
    template_id: 'tpl-welcome',
    broadcast_id: 'bc-1',
    count_operator: 'at_least',
    count_value: 1,
    timeframe_operator: 'anytime',
    timeframe_values: []
  }

  const renderSent = (onChange = vi.fn(), onDraftChange = vi.fn()) => {
    const node: TreeNode = { kind: 'leaf', leaf: { source: 'contact_timeline', contact_timeline: sent } }
    const editingNodeLeaf: EditingNodeLeaf = { ...node, path: '', key: 0 }
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <I18nProvider i18n={i18n}>
          <LeafActionForm
            value={node}
            onChange={onChange}
            onDraftChange={onDraftChange}
            source="contact_timeline"
            schema={TableSchemas.contact_timeline}
            editingNodeLeaf={editingNodeLeaf}
            setEditingNodeLeaf={vi.fn()}
            cancelOrDeleteNode={vi.fn()}
            workspaceId="ws1"
          />
        </I18nProvider>
      </QueryClientProvider>
    )
    return { onChange, onDraftChange }
  }

  // The server has always accepted a template and a broadcast on email.sent
  // (ContactTimelineCondition.Validate); the editor used to hide them.
  it('narrows to a template and a broadcast, and saves both', async () => {
    const { onChange } = renderSent()
    expect(screen.getByTestId('broadcast-selector')).toBeTruthy()
    expect(screen.getByText('Template')).toBeTruthy()

    fireEvent.click(screen.getByText('Confirm'))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const saved = onChange.mock.calls[0][0].leaf.contact_timeline
    expect(saved.template_id).toBe('tpl-welcome')
    expect(saved.broadcast_id).toBe('bc-1')
  })

  it('drops them when the kind becomes a web one', async () => {
    const { onDraftChange } = renderSent()
    fireEvent.mouseDown(screen.getAllByRole('combobox')[0])
    fireEvent.click(await screen.findByTitle('View web page'))

    await waitFor(() => expect(onDraftChange).toHaveBeenCalled())
    const draft = onDraftChange.mock.calls[onDraftChange.mock.calls.length - 1][0].leaf.contact_timeline
    expect(draft.kind).toBe('web.pageview')
    expect(draft.template_id).toBeUndefined()
    expect(draft.broadcast_id).toBeUndefined()
  })
})
