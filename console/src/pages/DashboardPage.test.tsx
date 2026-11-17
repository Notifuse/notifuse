import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ReactNode } from 'react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  updates: {
    data: null as null | Record<string, unknown>,
    available: false,
    dismissed: false,
    ignore: () => {}
  }
}))

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => state.navigate }))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { email: 'root@example.com' },
    workspaces: [{ id: 'acme', name: 'Acme', settings: {} }]
  })
}))
vi.mock('../services/api/auth', () => ({ isRootUser: () => true }))
vi.mock('../layouts/MainLayout', () => ({
  MainLayout: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  MainLayoutSidebar: ({
    extra,
    children,
    footer
  }: {
    extra?: ReactNode
    children: ReactNode
    footer?: ReactNode
  }) => (
    <div>
      {extra}
      {children}
      {footer}
    </div>
  )
}))
vi.mock('../components/settings/SystemSettingsDrawer', () => ({ SystemSettingsDrawer: () => null }))
vi.mock('../components/workspace/PendingInvitations', () => ({ PendingInvitations: () => null }))
vi.mock('../hooks/useUpdateCheck', () => ({ useUpdateCheck: () => state.updates }))
vi.mock('../components/updates/UpdateDrawer', () => ({ UpdateDrawer: () => null }))

import { DashboardPage } from './DashboardPage'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderPage = () =>
  render(
    <I18nProvider i18n={i18n}>
      <DashboardPage />
    </I18nProvider>
  )

describe('DashboardPage (workspace selection)', () => {
  beforeEach(() => {
    state.navigate.mockReset()
    state.updates = { data: null, available: false, dismissed: false, ignore: () => {} }
  })

  it('opens a workspace from the keyboard', () => {
    // The card took a click only: Tab never reached it.
    renderPage()
    const card = screen.getByRole('link', { name: 'Acme' })
    expect(card).toHaveAttribute('tabindex', '0')
    fireEvent.keyDown(card, { key: 'Enter' })
    expect(state.navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId',
      params: { workspaceId: 'acme' }
    })
  })

  it('names the button that creates a workspace', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'New workspace' }))
    expect(state.navigate).toHaveBeenCalledWith({ to: '/console/workspace/create' })
  })

  describe('update announcement', () => {
    const data = {
      current: '42.0',
      latest: '44.0',
      update_available: true,
      behind: 2,
      security: false,
      upgrade_url: 'https://docs.notifuse.com/self-hosting/installation',
      releases: []
    }

    it('shows no card when there is nothing to announce, but still the version', () => {
      renderPage()
      expect(screen.queryByText(/is available/)).not.toBeInTheDocument()
      expect(screen.getByText(/^Notifuse v/)).toBeInTheDocument()
    })

    it('shows the card after the workspaces', () => {
      state.updates = { data, available: true, dismissed: false, ignore: () => {} }
      renderPage()
      const workspace = screen.getByRole('link', { name: 'Acme' })
      const card = screen.getByText('Notifuse 44.0 is available')
      expect(workspace.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    })

    it('replaces the card with a link in the footer once dismissed', () => {
      state.updates = { data, available: true, dismissed: true, ignore: () => {} }
      renderPage()
      expect(screen.queryByText('Notifuse 44.0 is available')).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: '44.0 available' })).toBeInTheDocument()
    })
  })
})
