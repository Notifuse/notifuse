import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { WorkspaceLayout } from '../layouts/WorkspaceLayout'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate } from '@tanstack/react-router'
import { isRootUser } from '../services/api/auth'
import { workspaceService } from '../services/api/workspace'

vi.mock('../contexts/AuthContext', () => ({
  useAuth: vi.fn()
}))

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet" />,
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
  useParams: () => ({ workspaceId: 'ws1' }),
  useMatches: () => [{ pathname: '/console/workspace/ws1' }],
  useNavigate: vi.fn()
}))

vi.mock('../services/api/auth', () => ({
  isRootUser: vi.fn()
}))

vi.mock('../services/api/workspace', () => ({
  workspaceService: {
    getMembers: vi.fn(),
    update: vi.fn()
  }
}))

// Providers pulling in react-query and the file manager are noise here.
vi.mock('../components/contacts/ContactsCsvUploadProvider', () => ({
  ContactsCsvUploadProvider: ({ children }: { children: ReactNode }) => <>{children}</>
}))
vi.mock('../components/file_manager/context', () => ({
  FileManagerProvider: ({ children }: { children: ReactNode }) => <>{children}</>
}))
vi.mock('../components/LanguageSwitcher', () => ({
  LanguageSwitcher: () => null
}))

const mockNavigate = vi.fn()

const grantAll = (value: boolean) => ({
  contacts: { read: value, write: value },
  lists: { read: value, write: value },
  templates: { read: value, write: value },
  broadcasts: { read: value, write: value },
  transactional: { read: value, write: value },
  workspace: { read: value, write: value },
  message_history: { read: value, write: value },
  blog: { read: value, write: value },
  automations: { read: value, write: value },
  llm: { read: value, write: value },
  web_analytics: { read: value, write: value }
})

describe('WorkspaceLayout sidebar groups', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useNavigate).mockReturnValue(mockNavigate as never)
    vi.mocked(isRootUser).mockReturnValue(true)
    vi.mocked(useAuth).mockReturnValue({
      signout: vi.fn(),
      refreshWorkspaces: vi.fn(),
      workspaces: [{ id: 'ws1', name: 'Workspace One', settings: {} }],
      user: { id: 'u1', email: 'root@example.com' }
    } as never)
  })

  const openGroup = async (label: string) => {
    render(<WorkspaceLayout />)
    const title = await screen.findByText(label)
    await userEvent.click(title)
    return title
  }

  it('opens the web analytics dashboard when the collapsed group is clicked', async () => {
    await openGroup('Web Analytics')

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/web-analytics/$tab',
      params: { workspaceId: 'ws1', tab: 'dashboard' }
    })
  })

  it('does not navigate when the open web analytics group is clicked shut', async () => {
    const title = await openGroup('Web Analytics')
    mockNavigate.mockClear()

    await userEvent.click(title)

    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('opens templates when the collapsed content group is clicked', async () => {
    await openGroup('Content')

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/templates',
      params: { workspaceId: 'ws1' }
    })
  })

  it('falls back to the blog when the member cannot read templates', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [
        {
          user_id: 'u1',
          permissions: { ...grantAll(true), templates: { read: false, write: false } }
        }
      ]
    } as never)

    await openGroup('Content')

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/blog',
      params: { workspaceId: 'ws1' }
    })
  })
})
