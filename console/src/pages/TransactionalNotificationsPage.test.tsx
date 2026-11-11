import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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

// The drawer renders the Create and Edit buttons: stand in a button saying which one it is.
vi.mock('../components/transactional/UpsertTransactionalNotificationDrawer', () => ({
  default: ({
    notification,
    buttonProps
  }: {
    notification?: { id: string }
    buttonProps?: { disabled?: boolean }
  }) => (
    <button
      data-testid={notification ? `edit-${notification.id}` : 'create'}
      disabled={!!buttonProps?.disabled}
    />
  )
}))

// Keep the page render shallow: these children play no part in the buttons under test.
vi.mock('../components/templates/SendTemplateModal', () => ({ default: () => null }))
vi.mock('../components/templates/TemplatePreviewDrawer', () => ({ default: () => null }))
vi.mock('../components/transactional/ApiCommandModal', () => ({ ApiCommandModal: () => null }))
vi.mock('../services/api/template', () => ({
  templatesApi: { get: vi.fn().mockResolvedValue({}) }
}))
vi.mock('../services/api/analytics', () => ({
  analyticsService: { query: vi.fn().mockResolvedValue({ data: [] }) }
}))

const welcome = {
  id: 'welcome',
  name: 'Welcome',
  description: '',
  channels: { email: { template_id: 'tpl1' } },
  tracking_settings: {},
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}
const managed = { ...welcome, id: 'managed', name: 'Managed', integration_id: 'supabase' }

const { listNotifications } = vi.hoisted(() => ({ listNotifications: vi.fn() }))
vi.mock('../services/api/transactional_notifications', () => ({
  transactionalNotificationsApi: {
    list: listNotifications,
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

beforeEach(() => {
  permissions = {}
  listNotifications.mockResolvedValue({ notifications: [welcome], total: 1 })
})

describe('TransactionalNotificationsPage test button', () => {
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

const writeOnly: StoredPermissions = {
  templates: { read: false, write: false },
  transactional: { read: true, write: true }
}
const withTemplates: StoredPermissions = {
  templates: { read: true, write: false },
  transactional: { read: true, write: true }
}

// Creating a notification reads its template with the member's own permissions.
describe('TransactionalNotificationsPage Create button', () => {
  const hover = (element: HTMLElement) => {
    // A disabled button takes no pointer events: hover its tooltip wrapper.
    const wrapper = element.parentElement as HTMLElement
    fireEvent.pointerEnter(wrapper)
    fireEvent.mouseEnter(wrapper)
  }

  it('is disabled without templates read, and says what is needed', async () => {
    permissions = writeOnly
    renderPage()

    await screen.findByText('Welcome')
    const create = screen.getByTestId('create')
    expect(create).toBeDisabled()
    hover(create)
    expect(
      await screen.findByText(
        'You need Transactional write and Templates read permissions to create notifications'
      )
    ).toBeInTheDocument()
  })

  it('is enabled with transactional write and templates read, without the permission hint', async () => {
    permissions = withTemplates
    renderPage()

    await screen.findByText('Welcome')
    const create = screen.getByTestId('create')
    expect(create).toBeEnabled()
    hover(create)
    await new Promise((resolve) => setTimeout(resolve, 200))
    expect(
      screen.queryByText(
        'You need Transactional write and Templates read permissions to create notifications'
      )
    ).toBeNull()
  })

  it('is disabled without templates read on the empty page too', async () => {
    listNotifications.mockResolvedValue({ notifications: [], total: 0 })
    permissions = writeOnly
    renderPage()

    await screen.findByText('No transactional notifications found')
    expect(screen.getByTestId('create')).toBeDisabled()
  })

  it('is enabled with both on the empty page', async () => {
    listNotifications.mockResolvedValue({ notifications: [], total: 0 })
    permissions = withTemplates
    renderPage()

    await screen.findByText('No transactional notifications found')
    expect(screen.getByTestId('create')).toBeEnabled()
  })
})

// The console's edit sends the channels, so it reads the template, unless an integration manages
// the notification: its channels cannot change and are not sent.
describe('TransactionalNotificationsPage Edit button', () => {
  it('is not offered without templates read', async () => {
    permissions = writeOnly
    renderPage()

    await screen.findByText('Welcome')
    expect(screen.queryByTestId('edit-welcome')).toBeNull()
  })

  it('is offered with transactional write and templates read', async () => {
    permissions = withTemplates
    renderPage()

    await screen.findByText('Welcome')
    expect(screen.getByTestId('edit-welcome')).toBeInTheDocument()
  })

  it('is offered without templates read for a notification an integration manages', async () => {
    listNotifications.mockResolvedValue({ notifications: [welcome, managed], total: 2 })
    permissions = writeOnly
    renderPage()

    await screen.findByText('Managed')
    expect(screen.getByTestId('edit-managed')).toBeInTheDocument()
    expect(screen.queryByTestId('edit-welcome')).toBeNull()
  })
})
