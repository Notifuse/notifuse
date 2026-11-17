import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ReactNode } from 'react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'

const state = vi.hoisted(() => ({ navigate: vi.fn() }))

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
  MainLayoutSidebar: ({ extra, children }: { extra?: ReactNode; children: ReactNode }) => (
    <div>
      {extra}
      {children}
    </div>
  )
}))
vi.mock('../components/settings/SystemSettingsDrawer', () => ({ SystemSettingsDrawer: () => null }))
vi.mock('../components/workspace/PendingInvitations', () => ({ PendingInvitations: () => null }))

import { DashboardPage } from './DashboardPage'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderPage = () =>
  render(
    <I18nProvider i18n={i18n}>
      <DashboardPage />
    </I18nProvider>
  )

describe('DashboardPage (workspace selection)', () => {
  beforeEach(() => state.navigate.mockReset())

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
})
