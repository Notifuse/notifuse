import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
// A route that needs the width (Contacts, Broadcasts) opens with the sidebar unpinned.
let currentUnpin = false
let currentWorkspaceId = 'ws1'

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet" />,
  Link: ({ children, to, 'aria-label': label }: { children: ReactNode; to?: string; 'aria-label'?: string }) => (
    <a href={to} aria-label={label}>
      {children}
    </a>
  ),
  useParams: () => ({ workspaceId: currentWorkspaceId }),
  // The workspace route declares no layout; the leaf may.
  useMatches: () => [
    { routeId: '/console/workspace/$workspaceId', pathname: '/console/workspace/ws1', staticData: {} },
    {
      // The leaf's route id: one per page, whatever its search.
      routeId: currentPathname,
      pathname: currentPathname,
      staticData: { ...(currentLayout ? { layout: currentLayout } : {}), ...(currentUnpin ? { unpinSidebar: true } : {}) }
    }
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
  currentUnpin = false
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

  it('only opens a group when the sidebar is unpinned, so its other entries stay in reach', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    fireEvent.pointerEnter(document.querySelector<HTMLElement>('[data-sidebar]')!)
    await userEvent.click(await screen.findByText('Content'))

    expect(mockNavigate).not.toHaveBeenCalled()
    expect(await screen.findByText('Blog')).toBeInTheDocument()
    expect(document.querySelector<HTMLElement>('[data-sidebar]')!.dataset.expanded).toBe('true')
  })

  it('keeps the groups as they were, open or closed by hand, when the sidebar folds and opens again', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    currentPathname = '/console/workspace/ws1/templates'
    renderLayout()
    const sidebar = document.querySelector<HTMLElement>('[data-sidebar]')!
    const nav = await screen.findByRole('navigation', { name: 'Main navigation' })
    const contentOpen = () => nav.querySelector('.ant-menu-submenu-open > .ant-menu-submenu-title')

    fireEvent.pointerEnter(sidebar)
    await waitFor(() => expect(contentOpen()).toHaveTextContent('Content'))
    // Folding does not close the active group either.
    fireEvent.pointerLeave(sidebar)
    await act(() => new Promise((resolve) => setTimeout(resolve, 0)))
    fireEvent.pointerEnter(sidebar)
    await act(() => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(contentOpen()).toHaveTextContent('Content')

    await userEvent.click(screen.getByText('Content'))
    await waitFor(() => expect(contentOpen()).toBeNull())

    fireEvent.pointerLeave(sidebar)
    fireEvent.pointerEnter(sidebar)
    expect(sidebar.dataset.expanded).toBe('true')
    await act(() => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(contentOpen()).toBeNull()
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

  it.each([
    ['a page', '/console/workspace/ws1', undefined],
    ['a rail page', '/console/workspace/ws1/settings/team', 'rail' as const]
  ])('puts the pinned sidebar in the flow on %s, and the unpinned one over it', async (_, path, layout) => {
    currentPathname = path
    currentLayout = layout
    const { unmount } = renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(aside().dataset.expanded).toBe('true')
    expect(main().className).toContain('ml-sidebar')
    expect(main().className).not.toContain('ml-sidebar-collapsed')
    unmount()

    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(aside().dataset.expanded).toBe('false')
    expect(main().className).toContain('ml-sidebar-collapsed')
  })

  it('stores the pin, and moves the page only when it changes', async () => {
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    await userEvent.click(screen.getByRole('button', { name: 'Unpin sidebar' }))
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBe('true')
    expect(main().className).toContain('ml-sidebar-collapsed')
    // Unpinned under the pointer, the sidebar stays open over the page.
    expect(aside().dataset.expanded).toBe('true')

    await userEvent.click(screen.getByRole('button', { name: 'Pin sidebar' }))
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBe('false')
    expect(main().className).not.toContain('ml-sidebar-collapsed')
  })

  it('pins and unpins when the storage throws', async () => {
    // The setup replaces window.localStorage with a plain object, so the spy goes on it, not on Storage.
    const denied = () => {
      throw new Error('denied')
    }
    const getItem = vi.spyOn(window.localStorage, 'getItem').mockImplementation(denied)
    const setItem = vi.spyOn(window.localStorage, 'setItem').mockImplementation(denied)
    try {
      renderLayout()
      expect(aside().dataset.pinned).toBe('true')
      await userEvent.click(screen.getByRole('button', { name: 'Unpin sidebar' }))
      expect(aside().dataset.pinned).toBe('false')
      expect(setItem).toHaveBeenCalled()
    } finally {
      getItem.mockRestore()
      setItem.mockRestore()
    }
  })

  it('opens over the Blog with Content open and Blog selected, without moving the page', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    currentPathname = '/console/workspace/ws1/blog'
    currentLayout = 'rail'
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    fireEvent.pointerEnter(aside())
    expect(aside().dataset.expanded).toBe('true')
    const nav = screen.getByRole('navigation', { name: 'Main navigation' })
    await waitFor(() =>
      expect(
        nav.querySelector('.ant-menu-submenu-open > .ant-menu-submenu-title')
      ).toHaveTextContent('Content')
    )
    expect(nav.querySelector('.ant-menu-item-selected')).toHaveTextContent('Blog')
    expect(main().className).toContain('ml-sidebar-collapsed')
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBe('true')
  })

  it('folds an opening held by the focus on navigation', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    currentPathname = '/console/workspace/ws1/blog'
    currentLayout = 'rail'
    const { rerender } = renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    act(() => screen.getByRole('button', { name: 'Help' }).focus())
    expect(aside().dataset.expanded).toBe('true')
    currentPathname = '/console/workspace/ws1/settings/team'
    act(() => rerender(<WorkspaceLayout />))
    expect(aside().dataset.expanded).toBe('false')
  })

  it.each([
    ['Contacts', '/console/workspace/ws1/contacts', 'rail' as const],
    ['Broadcasts', '/console/workspace/ws1/broadcasts', undefined]
  ])('opens %s with the sidebar unpinned, whatever the stored choice, and stores nothing', async (_, path, layout) => {
    currentPathname = path
    currentLayout = layout
    currentUnpin = true
    renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(aside().dataset.pinned).toBe('false')
    expect(aside().dataset.expanded).toBe('false')
    expect(main().className).toContain('ml-sidebar-collapsed')
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBeNull()
  })

  it('pins on Contacts for the visit only, and gives the stored choice back elsewhere', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    currentPathname = '/console/workspace/ws1/contacts'
    currentLayout = 'rail'
    currentUnpin = true
    const { rerender } = renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    fireEvent.pointerEnter(aside())
    await userEvent.click(screen.getByRole('button', { name: 'Pin sidebar' }))
    expect(main().className).not.toContain('ml-sidebar-collapsed')
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBe('true')

    // Leaving: the stored choice (unpinned) applies again.
    currentPathname = '/console/workspace/ws1/broadcasts'
    currentLayout = undefined
    act(() => rerender(<WorkspaceLayout />))
    expect(aside().dataset.pinned).toBe('false')

    // Back on Contacts, a new visit: unpinned again.
    currentPathname = '/console/workspace/ws1/contacts'
    currentLayout = 'rail'
    act(() => rerender(<WorkspaceLayout />))
    expect(aside().dataset.pinned).toBe('false')
  })

  it('folds the sidebar the pointer is still on once Contacts opens from it', async () => {
    const { rerender } = renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    fireEvent.pointerEnter(aside())
    expect(aside().dataset.pinned).toBe('true')
    fireEvent.pointerDown(screen.getByText('Contacts'))

    currentPathname = '/console/workspace/ws1/contacts'
    currentLayout = 'rail'
    currentUnpin = true
    act(() => rerender(<WorkspaceLayout />))
    expect(aside().dataset.pinned).toBe('false')
    expect(aside().dataset.expanded).toBe('false')
    expect(main().className).toContain('ml-sidebar-collapsed')
  })

  it('pins the stored choice on a page that does not need the width', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'false')
    currentPathname = '/console/workspace/ws1/contacts'
    currentLayout = 'rail'
    currentUnpin = true
    const { rerender } = renderLayout()
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(aside().dataset.pinned).toBe('false')
    currentPathname = '/console/workspace/ws1/settings/team'
    currentUnpin = false
    act(() => rerender(<WorkspaceLayout />))
    expect(aside().dataset.pinned).toBe('true')
    expect(main().className).not.toContain('ml-sidebar-collapsed')
  })

  it('moves the page without a transition', () => {
    renderLayout()
    expect(main().className).not.toMatch(/transition/)
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

describe('WorkspaceLayout logo', () => {
  beforeEach(signInAsRoot)

  const logo = () => screen.getByRole('img', { name: 'Notifuse' }).closest('a')!

  it('leads a member who cannot read the messages where the dashboard would send them', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [
        {
          user_id: 'u1',
          permissions: {
            ...grantAll(false),
            contacts: { read: true, write: false },
            message_history: { read: false, write: true }
          }
        }
      ]
    } as never)
    renderLayout()
    await screen.findByText('Contacts')
    expect(logo()).toHaveAttribute('aria-label', 'Contacts')
    expect(logo()).toHaveAttribute('href', '/console/workspace/$workspaceId/contacts')
  })

  it('leads a member who reads the messages to the dashboard', async () => {
    renderLayout()
    await screen.findByText('Settings')
    expect(logo()).toHaveAttribute('aria-label', 'Dashboard')
    expect(logo()).toHaveAttribute('href', '/console/workspace/$workspaceId')
  })
})
