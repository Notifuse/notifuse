import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { StoredPermissions } from '../services/api/permissions'

let permissions: StoredPermissions = {}

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' })
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace', settings: {} }] }),
  useWorkspacePermissions: () => ({ permissions })
}))

// The icons are the only content of the card's buttons; render something a test can find.
vi.mock('@fortawesome/react-fontawesome', () => ({
  FontAwesomeIcon: ({ icon }: { icon: { iconName: string } }) => (
    <span data-icon={icon.iconName} />
  )
}))

// Keep the page render shallow: these children play no part in showing the test button.
vi.mock('../components/transactional/UpsertTransactionalNotificationDrawer', () => ({
  default: () => null
}))
vi.mock('../components/templates/SendTemplateModal', () => ({ default: () => null }))
vi.mock('../components/templates/TemplatePreviewDrawer', () => ({ default: () => null }))
vi.mock('../components/transactional/ApiCommandModal', () => ({ ApiCommandModal: () => null }))
vi.mock('../services/api/template', () => ({
  templatesApi: { get: vi.fn().mockResolvedValue({}) }
}))
vi.mock('../services/api/analytics', () => ({
  analyticsService: { query: vi.fn().mockResolvedValue({ data: [] }) }
}))
vi.mock('../services/api/transactional_notifications', () => ({
  transactionalNotificationsApi: {
    list: vi.fn().mockResolvedValue({
      notifications: [
        {
          id: 'welcome',
          name: 'Welcome',
          description: '',
          channels: { email: { template_id: 'tpl1' } },
          tracking_settings: {},
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z'
        }
      ],
      total: 1
    }),
    delete: vi.fn()
  }
}))

import { TransactionalNotificationsPage } from './TransactionalNotificationsPage'

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <TransactionalNotificationsPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

describe('TransactionalNotificationsPage test button', () => {
  beforeEach(() => {
    permissions = {}
  })

  it('is offered with templates read and transactional write', async () => {
    permissions = {
      templates: { read: true, write: false },
      transactional: { read: true, write: true }
    }
    const { container } = renderPage()

    await screen.findByText('Welcome')
    expect(container.querySelector('button [data-icon="paper-plane"]')).not.toBeNull()
  })

  // The test reads the notification's template with the member's own permissions.
  it('is not offered without templates read', async () => {
    permissions = {
      templates: { read: false, write: false },
      transactional: { read: true, write: true }
    }
    const { container } = renderPage()

    await screen.findByText('Welcome')
    expect(container.querySelector('button [data-icon="paper-plane"]')).toBeNull()
  })
})
