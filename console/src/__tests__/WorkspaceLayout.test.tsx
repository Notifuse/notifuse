import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WorkspaceLayout } from '../layouts/WorkspaceLayout'
import { AuthContext, useAuth } from '../contexts/AuthContext'
import { useNavigate } from '@tanstack/react-router'
import { isRootUser } from '../services/api/auth'
import { workspaceService } from '../services/api/workspace'

// The layout's own useAuth is mocked; useWorkspacePermissions is the real one, which reads the
// same value from the provider renderLayout puts around the layout.
vi.mock('../contexts/AuthContext', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../contexts/AuthContext')>()),
  useAuth: vi.fn()
}))

// Read at render time, so a test can put the layout on any route before rendering.
let currentPathname = '/console/workspace/ws1'
let currentLayout: 'page' | 'rail' | undefined
let currentWorkspaceId = 'ws1'

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet" />,
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
  useParams: () => ({ workspaceId: currentWorkspaceId }),
  // The workspace route declares no layout; the leaf may.
  useMatches: () => [
    { pathname: '/console/workspace/ws1', staticData: {} },
    { pathname: currentPathname, staticData: currentLayout ? { layout: currentLayout } : {} }
  ],
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
// Its query needs a QueryClient; the access is what the layout reads.
const auditAccess = vi.hoisted(() => ({ canRead: false, isRoot: false, loading: false }))
vi.mock('../hooks/useAuditLogAccess', () => ({
  useAuditLogAccess: () => auditAccess
}))
vi.mock('../contexts/LocaleContext', () => ({
  useLocale: () => ({ locale: 'en', setLocale: vi.fn() })
}))

const mockNavigate = vi.fn()

// A fresh query cache per render, and the mocked auth value in the provider the permissions hook
// reads.
const renderLayout = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<WorkspaceLayout />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <AuthContext.Provider value={vi.mocked(useAuth)()}>{children}</AuthContext.Provider>
      </QueryClientProvider>
    )
  })
}

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

const signInAsRoot = () => {
  vi.clearAllMocks()
  window.localStorage.clear()
  currentPathname = '/console/workspace/ws1'
  currentLayout = undefined
  currentWorkspaceId = 'ws1'
  vi.mocked(useNavigate).mockReturnValue(mockNavigate as never)
  vi.mocked(isRootUser).mockReturnValue(true)
  auditAccess.canRead = false
  vi.mocked(useAuth).mockReturnValue({
    signout: vi.fn(),
    refreshWorkspaces: vi.fn(),
    workspaces: [{ id: 'ws1', name: 'Workspace One', settings: {} }],
    user: { id: 'u1', email: 'root@example.com' }
  } as never)
}

describe('WorkspaceLayout sidebar groups', () => {
  beforeEach(signInAsRoot)

  const openGroup = async (label: string) => {
    renderLayout()
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

describe('WorkspaceLayout web analytics entries', () => {
  beforeEach(signInAsRoot)

  const selectedLabel = () => document.querySelector('.ant-menu-item-selected')?.textContent ?? null

  it('lists Annotations under the Web Analytics group', async () => {
    renderLayout()
    await userEvent.click(await screen.findByText('Web Analytics'))

    expect(screen.getByText('Annotations')).toBeInTheDocument()
  })

  it('highlights the annotations entry on the annotations route', async () => {
    // WEB_ANALYTICS_SECTIONS is the one registration TypeScript cannot catch:
    // a section missing from it falls through to the dashboard entry, so the
    // sidebar says you are somewhere you are not.
    currentPathname = '/console/workspace/ws1/web-analytics/annotations'

    renderLayout()
    await screen.findByText('Annotations')

    expect(selectedLabel()).toBe('Annotations')
  })

  it('still highlights the filters entry on the filters route', async () => {
    currentPathname = '/console/workspace/ws1/web-analytics/filters'

    renderLayout()
    await screen.findByText('Filters')

    expect(selectedLabel()).toBe('Filters')
  })
})

describe('WorkspaceLayout chrome', () => {
  beforeEach(signInAsRoot)

  const aside = () => document.querySelector<HTMLElement>('[data-sidebar]')!
  const main = () => document.querySelector<HTMLElement>('main')!
  const content = () => document.querySelector<HTMLElement>('[data-page-content]')!

  it('isolates the page, so its z-indexes stay under the sidebar and its menus', async () => {
    renderLayout()
    await screen.findByText('Contacts')
    expect(main().className.split(' ')).toContain('isolate')
  })

  it('pads a page and leaves a rail page to its own layout', async () => {
    const { unmount } = renderLayout()
    await screen.findByText('Contacts')
    expect(content().className).toBe('px-5 pt-4 pb-6')
    unmount()

    currentPathname = '/console/workspace/ws1/settings/team'
    currentLayout = 'rail'
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(content().className).toBe('')
  })

  it('folds the sidebar on a rail page whatever the stored choice', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'false')
    currentPathname = '/console/workspace/ws1/settings/team'
    currentLayout = 'rail'
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(aside().dataset.collapsed).toBe('true')
    expect(main().className).toContain('ml-sidebar-collapsed')
  })

  it('keeps the collapse chosen on a page, and writes it', async () => {
    renderLayout()
    expect(main().className).toContain('ml-sidebar')
    expect(main().className).not.toContain('ml-sidebar-collapsed')
    await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBe('true')
    expect(main().className).toContain('ml-sidebar-collapsed')
  })

  it('reads a stored collapse', () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    renderLayout()
    expect(aside().dataset.collapsed).toBe('true')
  })

  it('opens expanded and still collapses when the storage throws', async () => {
    // The setup replaces window.localStorage with a plain object, so the spy goes on it, not on Storage.
    const denied = () => {
      throw new Error('denied')
    }
    const getItem = vi.spyOn(window.localStorage, 'getItem').mockImplementation(denied)
    const setItem = vi.spyOn(window.localStorage, 'setItem').mockImplementation(denied)
    try {
      renderLayout()
      expect(aside().dataset.collapsed).toBe('false')
      await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
      expect(aside().dataset.collapsed).toBe('true')
      expect(setItem).toHaveBeenCalled()
    } finally {
      getItem.mockRestore()
      setItem.mockRestore()
    }
  })

  it('peeks on the Blog with Content open and Blog selected, without moving the content', async () => {
    currentPathname = '/console/workspace/ws1/blog'
    currentLayout = 'rail'
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
    expect(aside().dataset.collapsed).toBe('false')
    const nav = screen.getByRole('navigation', { name: 'Main navigation' })
    await waitFor(() =>
      expect(
        nav.querySelector('.ant-menu-submenu-open > .ant-menu-submenu-title')
      ).toHaveTextContent('Content')
    )
    expect(nav.querySelector('.ant-menu-item-selected')).toHaveTextContent('Blog')
    expect(main().className).toContain('ml-sidebar-collapsed')
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBeNull()
  })

  it('folds the peek back on navigation', async () => {
    currentPathname = '/console/workspace/ws1/blog'
    currentLayout = 'rail'
    const { rerender } = renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
    expect(aside().dataset.collapsed).toBe('false')
    currentPathname = '/console/workspace/ws1/settings/team'
    act(() => rerender(<WorkspaceLayout />))
    expect(aside().dataset.collapsed).toBe('true')
  })

  it('places the sidebar and the page under the licence banner', () => {
    renderLayout()
    expect(aside().style.top).toBe('calc(0px + var(--license-banner-height, 0px))')
    expect(main().style.paddingTop).toBe('calc(0px + var(--license-banner-height, 0px))')
  })

  it('no longer paints the former #F9F9F9 background', () => {
    const { container } = renderLayout()
    expect(container.innerHTML.toLowerCase()).not.toContain('#f9f9f9')
    expect(container.firstElementChild?.className).toContain('bg-canvas')
  })

  it('shows no entry while the rights load', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockReturnValue(new Promise(() => {}))
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('navigation', { name: 'Main navigation' })).toBeInTheDocument()
    )
    expect(document.querySelectorAll('.app-sidebar-nav .ant-menu-item')).toHaveLength(0)
  })

  it('reads the member role on the account line', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [{ user_id: 'u1', role: 'owner', permissions: grantAll(true) }]
    } as never)
    renderLayout()
    await waitFor(() =>
      expect(document.querySelector('[data-account-role]')).toHaveTextContent('Owner · EN')
    )
  })

  // The layout stays mounted on a switch: until the new workspace answers, it shows no entry and
  // no role, never the previous workspace's.
  it("never shows the previous workspace's menu or role after a switch", async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    let answerWs2!: (value: unknown) => void
    vi.mocked(workspaceService.getMembers).mockImplementation(((id: string) =>
      id === 'ws1'
        ? Promise.resolve({
            members: [{ user_id: 'u1', role: 'owner', permissions: grantAll(true) }]
          })
        : new Promise((resolve) => (answerWs2 = resolve))) as never)
    const { rerender } = renderLayout()
    expect(await screen.findByText('Broadcasts')).toBeInTheDocument()
    expect(document.querySelector('[data-account-role]')).toHaveTextContent('Owner')

    currentWorkspaceId = 'ws2'
    rerender(<WorkspaceLayout />)

    expect(screen.queryByText('Broadcasts')).toBeNull()
    expect(document.querySelectorAll('.app-sidebar-nav .ant-menu-item')).toHaveLength(0)
    expect(document.querySelector('[data-account-role]')).not.toHaveTextContent('Owner')

    await act(async () => {
      answerWs2({
        members: [
          {
            user_id: 'u1',
            role: 'member',
            permissions: { ...grantAll(false), workspace: { read: true, write: false } }
          }
        ]
      })
    })
    expect(await screen.findByText('Settings')).toBeInTheDocument()
    expect(screen.queryByText('Broadcasts')).toBeNull()
    expect(document.querySelector('[data-account-role]')).toHaveTextContent('Member')
  })

  it('switches workspace and opens the workspace creation', async () => {
    vi.mocked(useAuth).mockReturnValue({
      signout: vi.fn(),
      refreshWorkspaces: vi.fn(),
      workspaces: [
        { id: 'ws1', name: 'Workspace One', settings: {} },
        { id: 'ws2', name: 'Workspace Two', settings: {} }
      ],
      user: { id: 'u1', email: 'root@example.com' }
    } as never)
    renderLayout()
    await userEvent.click(screen.getByRole('button', { name: /Workspace One/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: /Workspace Two/ }))
    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId',
      params: { workspaceId: 'ws2' }
    })
    await userEvent.click(screen.getByRole('button', { name: /Workspace One/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: /New workspace/ }))
    expect(mockNavigate).toHaveBeenCalledWith({ to: '/console/workspace/create' })
  })
})

describe('WorkspaceLayout Logs entry', () => {
  beforeEach(signInAsRoot)

  const memberWith = (permissions: Record<string, { read: boolean; write: boolean }>) => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [{ user_id: 'u1', permissions: { ...grantAll(false), ...permissions } }]
    } as never)
  }

  // The settings entry needs workspace access, which every case below grants, so the sidebar has
  // drawn once it shows.
  const logsShown = async () => {
    renderLayout()
    await screen.findByText('Settings')
    return screen.queryByText('Logs') !== null
  }

  const workspace = { workspace: { read: true, write: false } }

  it('shows with the audit log access alone', async () => {
    memberWith(workspace)
    auditAccess.canRead = true
    expect(await logsShown()).toBe(true)
  })

  it('shows with the incoming webhooks read alone', async () => {
    memberWith({ ...workspace, webhook_events: { read: true, write: false } })
    expect(await logsShown()).toBe(true)
  })

  it('shows with the outgoing webhooks read alone', async () => {
    memberWith({ ...workspace, webhook_subscriptions: { read: true, write: false } })
    expect(await logsShown()).toBe(true)
  })

  it('stays hidden for a member who may only write webhooks', async () => {
    memberWith({ ...workspace, webhook_subscriptions: { read: false, write: true } })
    expect(await logsShown()).toBe(false)
  })

  it('stays hidden without any logs right', async () => {
    memberWith(workspace)
    expect(await logsShown()).toBe(false)
  })
})
