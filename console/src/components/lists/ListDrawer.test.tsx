import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { CreateListDrawer } from './ListDrawer'
import { templatesApi } from '../../services/api/template'
import { listsApi } from '../../services/api/list'
import type { List } from '../../services/api/types'

vi.mock('../../services/api/template', () => ({
  // The window counts the uses of each type the API names.
  TEMPLATE_USAGE_TYPES: ['broadcasts', 'transactional_notifications', 'automations', 'lists'],
  templatesApi: { list: vi.fn(), get: vi.fn(), usage: vi.fn(), compile: vi.fn() }
}))
vi.mock('../../services/api/list', () => ({
  listsApi: { create: vi.fn(), update: vi.fn() }
}))
vi.mock('../../contexts/AuthContext', () => ({
  // TemplateSelectorInput reads the member's permissions for its drawers
  useWorkspacePermissions: () => ({ permissions: null }),
  // Both providers set, each with its default sender: the window names the one a send uses.
  useAuth: () => ({
    workspaces: [
      {
        id: 'ws1',
        name: 'Workspace',
        settings: { marketing_email_provider_id: 'mk', transactional_email_provider_id: 'tx' },
        integrations: [
          { id: 'tx', name: 'Postmark', email_provider: { senders: [{ id: 'tx1', name: 'Support', email: 'support@acme.com', is_default: true }] } },
          { id: 'mk', name: 'SES', email_provider: { senders: [{ id: 'mk1', name: 'News', email: 'news@acme.com', is_default: true }] } }
        ]
      }
    ]
  })
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
    vi.mocked(templatesApi.usage).mockReset().mockResolvedValue({ usage: {}, hidden_types: [] } as never)
    vi.mocked(templatesApi.compile).mockReset().mockResolvedValue({ html: '', mjml: '' } as never)
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
  })

  // The window lists the Opt-in templates alone and says what the template is for.
  it('opens the template window on the Opt-in category, saying what the template is for', async () => {
    openDrawer(true)

    fireEvent.click(await screen.findByDisplayValue('Confirm your subscription'))
    // Found by its title: the dialogs around it share the test id antd names them by.
    const dialog = await waitFor(() => {
      const found = screen.getAllByRole('dialog').find((element) => element.textContent?.startsWith('Choose a template'))
      expect(found).toBeDefined()
      return found as HTMLElement
    })
    expect(within(dialog).getByText('The confirmation email each new subscriber receives')).toBeInTheDocument()
    await waitFor(() =>
      expect(templatesApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1', category: 'opt_in', channel: 'email' })
    )
  })

  // The confirmation email goes out through the marketing provider, whatever the template's category.
  it("names the marketing provider's sender in the template window", async () => {
    const optin = { id: 'tpl-optin', name: 'Confirm your subscription', category: 'opt_in', email: { subject: 'Confirm' } }
    vi.mocked(templatesApi.list).mockResolvedValue({ templates: [optin] } as never)
    vi.mocked(templatesApi.get).mockResolvedValue({ template: optin } as never)
    openDrawer(true)

    fireEvent.click(await screen.findByDisplayValue('Confirm your subscription'))
    const preview = await screen.findByRole('region', { name: 'Preview' })
    expect(await within(preview).findByText('News · news@acme.com')).toBeInTheDocument()
    expect(within(preview).queryByText('Support · support@acme.com')).toBeNull()
  })
})
