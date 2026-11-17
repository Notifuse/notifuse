import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput } from './input'
import { TableSchemas } from './table_schemas'
import type { TreeNode } from '../../services/api/segment'

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
