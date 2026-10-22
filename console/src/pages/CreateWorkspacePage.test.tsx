import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import { CreateWorkspacePage } from './CreateWorkspacePage'
import { workspaceService } from '../services/api/workspace'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => vi.fn()
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ refreshWorkspaces: vi.fn() })
}))
vi.mock('../layouts/MainLayout', () => ({
  MainLayout: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  MainLayoutSidebar: ({ children }: { children: ReactNode }) => <div>{children}</div>
}))
vi.mock('../services/api/workspace', () => ({
  workspaceService: {
    create: vi.fn().mockResolvedValue({}),
    detectFavicon: vi.fn()
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderPage = () =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <CreateWorkspacePage />
      </App>
    </I18nProvider>
  )

// The API refuses a workspace name over 32 characters: the field says so as it is typed.
describe('CreateWorkspacePage workspace name', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('limits the workspace name to 32 characters', () => {
    renderPage()
    expect(screen.getByPlaceholderText('Enter a name for your workspace')).toHaveAttribute(
      'maxlength',
      '32'
    )
  })

  it('does not create a workspace whose name is over 32 characters', async () => {
    renderPage()

    // A change event sets the value past the input's maxlength, as a paste can.
    fireEvent.change(screen.getByPlaceholderText('Enter a name for your workspace'), {
      target: { value: 'a'.repeat(33) }
    })
    fireEvent.click(screen.getByRole('button', { name: /Create Workspace/i }))

    expect(
      await screen.findByText('Workspace name must be at most 32 characters long')
    ).toBeInTheDocument()
    expect(workspaceService.create).not.toHaveBeenCalled()
  })
})
