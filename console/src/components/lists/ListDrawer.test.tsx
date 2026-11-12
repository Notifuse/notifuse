import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { CreateListDrawer } from './ListDrawer'
import { templatesApi } from '../../services/api/template'
import { listsApi } from '../../services/api/list'
import type { List } from '../../services/api/types'

vi.mock('../../services/api/template', () => ({
  templatesApi: { list: vi.fn(), get: vi.fn() }
}))
vi.mock('../../services/api/list', () => ({
  listsApi: { create: vi.fn(), update: vi.fn() }
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace', integrations: [] }] })
}))
vi.mock('../templates/TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>
}))
vi.mock('../templates/CreateTemplateDrawer', () => ({ CreateTemplateDrawer: () => null }))

const list = {
  id: 'newsletter',
  name: 'Newsletter',
  is_double_optin: true,
  is_public: false,
  double_optin_template: { id: 'tpl-optin', version: 1 },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
} as unknown as List

const openDrawer = (canReadTemplates: boolean) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>
          <CreateListDrawer workspaceId="ws1" list={list} canReadTemplates={canReadTemplates} />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Edit List' }))
}

describe('CreateListDrawer double opt-in template', () => {
  beforeEach(() => {
    vi.mocked(templatesApi.list).mockReset().mockResolvedValue({ templates: [] } as never)
    vi.mocked(templatesApi.get)
      .mockReset()
      .mockResolvedValue({ template: { id: 'tpl-optin', name: 'Confirm your subscription' } } as never)
    vi.mocked(listsApi.update).mockReset().mockResolvedValue({ list } as never)
  })

  // Without Templates read, templates.list and templates.get answer 403: the field shows the
  // stored ID, locked, and saving keeps the template.
  it("shows the template's ID without Templates read, requests nothing, and saves it unchanged", async () => {
    openDrawer(false)

    const field = await screen.findByDisplayValue('tpl-optin')
    expect(field).toBeDisabled()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(templatesApi.list).not.toHaveBeenCalled()
    expect(templatesApi.get).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(listsApi.update).toHaveBeenCalled())
    expect(vi.mocked(listsApi.update).mock.calls[0][0].double_optin_template).toEqual({
      id: 'tpl-optin',
      version: 1
    })
  })

  it("shows the template's name with Templates read", async () => {
    openDrawer(true)

    expect(await screen.findByDisplayValue('Confirm your subscription')).toBeInTheDocument()
    expect(templatesApi.list).toHaveBeenCalled()
  })
})
