import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactElement } from 'react'
import { UpsertTransactionalNotificationDrawer } from './UpsertTransactionalNotificationDrawer'
import {
  transactionalNotificationsApi,
  TransactionalNotification
} from '../../services/api/transactional_notifications'
import type { Workspace } from '../../services/api/types'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// The template selector pulls in template list queries; replace it with a bare
// input so the required template_id field can be filled without network calls.
vi.mock('../templates/TemplateSelectorInput', () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (value: string) => void }) => (
    <input
      data-testid="template-selector"
      value={value ?? ''}
      onChange={(e) => onChange?.(e.target.value)}
    />
  )
}))

vi.mock('../../services/api/transactional_notifications', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../../services/api/transactional_notifications')>()
  return {
    ...actual,
    transactionalNotificationsApi: {
      list: vi.fn(),
      get: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      send: vi.fn(),
      testTemplate: vi.fn()
    }
  }
})

const workspace = {
  id: 'ws1',
  settings: {
    website_url: 'https://example.com'
  }
} as unknown as Workspace

const makeNotification = (
  trackingMode?: 'inherit' | 'disabled',
  overrides: Partial<TransactionalNotification> = {}
): TransactionalNotification =>
  ({
    id: 'welcome_email',
    name: 'Welcome Email',
    description: '',
    channels: { email: { template_id: 'tmpl1' } },
    tracking_settings: {
      enable_tracking: false,
      ...(trackingMode ? { tracking_mode: trackingMode } : {}),
      utm_source: 'example.com',
      utm_medium: 'email'
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides
  }) as unknown as TransactionalNotification

function renderDrawer(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AntApp>{ui}</AntApp>
    </QueryClientProvider>
  )
}

const openCreateDrawer = async () => {
  renderDrawer(<UpsertTransactionalNotificationDrawer workspace={workspace} />)
  await userEvent.click(screen.getByRole('button', { name: /Create Notification/i }))
}

const fillRequiredFields = async () => {
  // Typing the name auto-generates the API identifier.
  await userEvent.type(screen.getByPlaceholderText('E.g. Password Reset Email'), 'Welcome Email')
  await userEvent.type(screen.getByTestId('template-selector'), 'tmpl1')
}

describe('UpsertTransactionalNotificationDrawer tracking mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(transactionalNotificationsApi.create as ReturnType<typeof vi.fn>).mockResolvedValue({
      notification: {}
    })
    ;(transactionalNotificationsApi.update as ReturnType<typeof vi.fn>).mockResolvedValue({
      notification: {}
    })
  })

  it('renders the tracking select with both options', async () => {
    await openCreateDrawer()

    await userEvent.click(screen.getByRole('combobox'))

    expect(await screen.findByText('Disabled for this notification')).toBeInTheDocument()
    // "Follow workspace setting" appears twice: as the selected value and as an option.
    expect(screen.getAllByText('Follow workspace setting').length).toBeGreaterThanOrEqual(2)
  })

  it('submits tracking_mode "inherit" by default in create mode', async () => {
    await openCreateDrawer()
    await fillRequiredFields()

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalled())
    const payload = (transactionalNotificationsApi.create as ReturnType<typeof vi.fn>).mock
      .calls[0][0]
    expect(payload.notification.tracking_settings.tracking_mode).toBe('inherit')
  })

  it('submits tracking_mode "disabled" when the disabled option is selected', async () => {
    await openCreateDrawer()
    await fillRequiredFields()

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Disabled for this notification'))

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(transactionalNotificationsApi.create).toHaveBeenCalled())
    const payload = (transactionalNotificationsApi.create as ReturnType<typeof vi.fn>).mock
      .calls[0][0]
    expect(payload.notification.tracking_settings.tracking_mode).toBe('disabled')
  })

  it('shows the disabled option selected when editing an opted-out notification', async () => {
    renderDrawer(
      <UpsertTransactionalNotificationDrawer
        workspace={workspace}
        notification={makeNotification('disabled')}
      />
    )

    await userEvent.click(screen.getByRole('button', { name: /Edit Notification/i }))

    expect(await screen.findByText('Disabled for this notification')).toBeInTheDocument()
  })

  it('shows "Follow workspace setting" when the stored mode is absent', async () => {
    renderDrawer(
      <UpsertTransactionalNotificationDrawer
        workspace={workspace}
        notification={makeNotification()}
      />
    )

    await userEvent.click(screen.getByRole('button', { name: /Edit Notification/i }))

    expect(await screen.findByText('Follow workspace setting')).toBeInTheDocument()
  })

  it('submits tracking_mode "inherit" when resetting an opted-out notification', async () => {
    renderDrawer(
      <UpsertTransactionalNotificationDrawer
        workspace={workspace}
        notification={makeNotification('disabled')}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit Notification/i }))

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Follow workspace setting'))

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalled())
    const payload = (transactionalNotificationsApi.update as ReturnType<typeof vi.fn>).mock
      .calls[0][0]
    expect(payload.updates.tracking_settings.tracking_mode).toBe('inherit')
    // Regular (non integration-managed) updates keep sending the channels config.
    expect(payload.updates.channels).toEqual({ email: { template_id: 'tmpl1' } })
  })

  it('submits tracking_mode "disabled" when opting out a mode-less notification', async () => {
    renderDrawer(
      <UpsertTransactionalNotificationDrawer
        workspace={workspace}
        notification={makeNotification()}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit Notification/i }))

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Disabled for this notification'))

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalled())
    const payload = (transactionalNotificationsApi.update as ReturnType<typeof vi.fn>).mock
      .calls[0][0]
    expect(payload.updates.tracking_settings.tracking_mode).toBe('disabled')
  })

  it('omits channels from updates of integration-managed notifications', async () => {
    // The server rejects any channels value on integration-managed notifications,
    // so the drawer must leave it out while still sending the tracking preference.
    renderDrawer(
      <UpsertTransactionalNotificationDrawer
        workspace={workspace}
        notification={makeNotification(undefined, {
          id: 'supabase_magiclink_000001',
          integration_id: 'supabase-integration-1'
        })}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit Notification/i }))

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Disabled for this notification'))

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(transactionalNotificationsApi.update).toHaveBeenCalled())
    const payload = (transactionalNotificationsApi.update as ReturnType<typeof vi.fn>).mock
      .calls[0][0]
    expect(payload.updates.channels).toBeUndefined()
    expect(payload.updates.tracking_settings.tracking_mode).toBe('disabled')
  })
})
