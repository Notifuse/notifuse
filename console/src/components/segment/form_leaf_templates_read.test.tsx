import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput } from './input'
import { TableSchemas } from './table_schemas'
import { templatesApi } from '../../services/api/template'
import type { TreeNode } from '../../services/api/segment'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('../../services/api/template', () => ({
  templatesApi: { list: vi.fn(), get: vi.fn() }
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace', integrations: [] }] })
}))
vi.mock('../templates/TemplatePreviewDrawer', () => ({ default: () => null }))
vi.mock('../templates/CreateTemplateDrawer', () => ({ CreateTemplateDrawer: () => null }))

const emailTree: TreeNode = {
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

const renderTree = (canReadTemplates?: boolean) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <TreeNodeInput
          value={emailTree}
          onChange={vi.fn()}
          schemas={TableSchemas}
          workspaceId="ws1"
          canReadTemplates={canReadTemplates}
        />
      </I18nProvider>
    </QueryClientProvider>
  )
}

const editFirstCondition = () =>
  fireEvent.click(document.querySelectorAll('[data-icon="pen-to-square"]')[0])

// Without Templates read, templates.list and templates.get answer 403: an email condition names
// its template by ID, and editing it shows that ID locked, without requesting any template.
describe('segment email condition without Templates read', () => {
  beforeEach(() => {
    vi.mocked(templatesApi.list)
      .mockReset()
      .mockResolvedValue({ templates: [{ id: 'tpl-welcome', name: 'Welcome' }] } as never)
    vi.mocked(templatesApi.get)
      .mockReset()
      .mockResolvedValue({ template: { id: 'tpl-welcome', name: 'Welcome' } } as never)
  })

  it('names the template by its ID and requests no template, even while editing', async () => {
    renderTree(false)

    expect(screen.getByText('tpl-welcome')).toBeInTheDocument()
    editFirstCondition()
    const field = await screen.findByDisplayValue('tpl-welcome')
    expect(field).toBeDisabled()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(templatesApi.list).not.toHaveBeenCalled()
    expect(templatesApi.get).not.toHaveBeenCalled()
  })

  it('names the template when the prop is left out', async () => {
    renderTree()

    expect(await screen.findByText('Welcome')).toBeInTheDocument()
    editFirstCondition()
    expect(await screen.findByDisplayValue('Welcome')).not.toBeDisabled()
    expect(templatesApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'tpl-welcome' })
  })
})
