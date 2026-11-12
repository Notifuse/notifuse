import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
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

// Read at render time, so a test can put the layout on any route before rendering.
let currentPathname = '/console/workspace/ws1'
let currentLayout: 'page' | 'rail' | undefined

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet" />,
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
  useParams: () => ({ workspaceId: 'ws1' }),
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
vi.mock('../contexts/LocaleContext', () => ({
  useLocale: () => ({ locale: 'en', setLocale: vi.fn() })
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

const signInAsRoot = () => {
  vi.clearAllMocks()
  window.localStorage.clear()
  currentPathname = '/console/workspace/ws1'
  currentLayout = undefined
  vi.mocked(useNavigate).mockReturnValue(mockNavigate as never)
  vi.mocked(isRootUser).mockReturnValue(true)
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

describe('WorkspaceLayout web analytics entries', () => {
  beforeEach(signInAsRoot)

  const selectedLabel = () => document.querySelector('.ant-menu-item-selected')?.textContent ?? null

  it('lists Annotations under the Web Analytics group', async () => {
    render(<WorkspaceLayout />)
    await userEvent.click(await screen.findByText('Web Analytics'))

    expect(screen.getByText('Annotations')).toBeInTheDocument()
  })

  it('highlights the annotations entry on the annotations route', async () => {
    // WEB_ANALYTICS_SECTIONS is the one registration TypeScript cannot catch:
    // a section missing from it falls through to the dashboard entry, so the
    // sidebar says you are somewhere you are not.
    currentPathname = '/console/workspace/ws1/web-analytics/annotations'

    render(<WorkspaceLayout />)
    await screen.findByText('Annotations')

    expect(selectedLabel()).toBe('Annotations')
  })

  it('still highlights the filters entry on the filters route', async () => {
    currentPathname = '/console/workspace/ws1/web-analytics/filters'

    render(<WorkspaceLayout />)
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
    render(<WorkspaceLayout />)
    await screen.findByText('Contacts')
    expect(main().className.split(' ')).toContain('isolate')
  })

  it('pads a page and leaves a rail page to its own layout', async () => {
    const { unmount } = render(<WorkspaceLayout />)
    await screen.findByText('Contacts')
    expect(content().className).toBe('px-5 pt-4 pb-6')
    unmount()

    currentPathname = '/console/workspace/ws1/settings/team'
    currentLayout = 'rail'
    render(<WorkspaceLayout />)
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(content().className).toBe('')
  })

  it('folds the sidebar on a rail page whatever the stored choice', async () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'false')
    currentPathname = '/console/workspace/ws1/settings/team'
    currentLayout = 'rail'
    render(<WorkspaceLayout />)
    await screen.findByRole('navigation', { name: 'Main navigation' })
    expect(aside().dataset.collapsed).toBe('true')
    expect(main().className).toContain('ml-sidebar-collapsed')
  })

  it('keeps the collapse chosen on a page, and writes it', async () => {
    render(<WorkspaceLayout />)
    expect(main().className).toContain('ml-sidebar')
    expect(main().className).not.toContain('ml-sidebar-collapsed')
    await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBe('true')
    expect(main().className).toContain('ml-sidebar-collapsed')
  })

  it('reads a stored collapse', () => {
    window.localStorage.setItem('notifuse.sidebar.collapsed', 'true')
    render(<WorkspaceLayout />)
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
      render(<WorkspaceLayout />)
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
    render(<WorkspaceLayout />)
    await screen.findByRole('navigation', { name: 'Main navigation' })
    await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
    expect(aside().dataset.collapsed).toBe('false')
    const nav = screen.getByRole('navigation', { name: 'Main navigation' })
    await waitFor(() =>
      expect(nav.querySelector('.ant-menu-submenu-open > .ant-menu-submenu-title')).toHaveTextContent('Content')
    )
    expect(nav.querySelector('.ant-menu-item-selected')).toHaveTextContent('Blog')
    expect(main().className).toContain('ml-sidebar-collapsed')
    expect(window.localStorage.getItem('notifuse.sidebar.collapsed')).toBeNull()
  })

  it('folds the peek back on navigation', async () => {
    currentPathname = '/console/workspace/ws1/blog'
    currentLayout = 'rail'
    const { rerender } = render(<WorkspaceLayout />)
    await screen.findByRole('navigation', { name: 'Main navigation' })
    await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
    expect(aside().dataset.collapsed).toBe('false')
    currentPathname = '/console/workspace/ws1/settings/team'
    act(() => rerender(<WorkspaceLayout />))
    expect(aside().dataset.collapsed).toBe('true')
  })

  it('places the sidebar and the page under the licence banner', () => {
    render(<WorkspaceLayout />)
    expect(aside().style.top).toBe('calc(0px + var(--license-banner-height, 0px))')
    expect(main().style.paddingTop).toBe('calc(0px + var(--license-banner-height, 0px))')
  })

  it('no longer paints the former #F9F9F9 background', () => {
    const { container } = render(<WorkspaceLayout />)
    expect(container.innerHTML.toLowerCase()).not.toContain('#f9f9f9')
    expect(container.firstElementChild?.className).toContain('bg-canvas')
  })

  it('shows no entry while the rights load', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockReturnValue(new Promise(() => {}))
    render(<WorkspaceLayout />)
    await waitFor(() => expect(screen.getByRole('navigation', { name: 'Main navigation' })).toBeInTheDocument())
    expect(document.querySelectorAll('.app-sidebar-nav .ant-menu-item')).toHaveLength(0)
  })

  it('reads the member role on the account line', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [{ user_id: 'u1', role: 'owner', permissions: grantAll(true) }]
    } as never)
    render(<WorkspaceLayout />)
    await waitFor(() => expect(document.querySelector('[data-account-role]')).toHaveTextContent('Owner · EN'))
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
    render(<WorkspaceLayout />)
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
