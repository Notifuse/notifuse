import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import type { Workspace } from '../../services/api/types'
import { AppSidebar, type AppSidebarProps } from './AppSidebar'
import { changeLanguage } from '../../i18n/languages'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, params, ...rest }: { children: ReactNode; to: string; params?: object }) => (
    <a href={to} data-params={JSON.stringify(params)} {...rest}>
      {children}
    </a>
  )
}))

const setLocale = vi.fn()
vi.mock('../../contexts/LocaleContext', () => ({
  useLocale: () => ({ locale: 'en', setLocale })
}))

// The real module imports the API client, and with it the router.
vi.mock('../../i18n/languages', async () => {
  const { locales, localeNames } = await import('../../i18n')
  return {
    LANGUAGES: locales.map((locale) => ({ locale, name: localeNames[locale] })),
    changeLanguage: vi.fn()
  }
})

// Gravatar is preloaded with new Image(); each test decides when (and whether) it loads.
let images: { src: string; onload: (() => void) | null; onerror: (() => void) | null }[] = []
const OriginalImage = globalThis.Image

const workspaces = [
  { id: 'ws1', name: 'Acme', settings: {} },
  { id: 'ws2', name: 'Beta', settings: { logo_url: 'https://cdn.example.com/beta.png' } }
] as unknown as Workspace[]

type HarnessProps = Partial<AppSidebarProps> & { startCollapsed?: boolean }

/** The sidebar with the collapse state WorkspaceLayout keeps: a stored choice, and a peek on rail. */
function Harness({ startCollapsed = false, rail = false, ...props }: HarnessProps) {
  const [stored, setStored] = useState(startCollapsed)
  const [peek, setPeek] = useState(false)
  return (
    <>
      <AppSidebar
        collapsed={rail ? !peek : stored}
        peeking={rail && peek}
        rail={rail}
        onToggle={() => setStored((value) => !value)}
        onPeekChange={setPeek}
        menuItems={[{ key: 'contacts', label: 'Contacts', icon: <svg /> }]}
        selectedKey="contacts"
        openKeys={[]}
        onOpenChange={vi.fn()}
        workspaces={workspaces}
        workspaceId="ws1"
        isRoot={false}
        onSelectWorkspace={vi.fn()}
        onCreateWorkspace={vi.fn()}
        user={{ email: 'jane@example.com', name: 'Jane Doe' }}
        role="owner"
        onSignout={vi.fn()}
        {...props}
      />
      <button type="button">Outside</button>
    </>
  )
}

const aside = () => document.querySelector<HTMLElement>('[data-sidebar]')!
const isCollapsed = () => aside().dataset.collapsed === 'true'

describe('AppSidebar', () => {
  beforeEach(() => {
    images = []
    globalThis.Image = class {
      src = ''
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      constructor() {
        images.push(this)
      }
    } as unknown as typeof Image
    window.VERSION = '44.0'
  })

  afterEach(() => {
    globalThis.Image = OriginalImage
    vi.clearAllMocks()
  })

  it('links the Notifuse logo to the workspace dashboard', () => {
    render(<Harness />)
    const logo = screen.getByRole('img', { name: 'Notifuse' })
    const link = logo.closest('a')!
    expect(link).toHaveAttribute('href', '/console/workspace/$workspaceId')
    expect(link.dataset.params).toBe(JSON.stringify({ workspaceId: 'ws1' }))
  })

  it('follows the banner and keeps the chrome under the banner', () => {
    render(<Harness />)
    expect(aside().style.top).toBe('calc(0px + var(--license-banner-height, 0px))')
    expect(aside().style.height).toBe('calc(100vh - var(--license-banner-height, 0px))')
    expect(aside().className).toContain('z-10')
  })

  it('collapses and expands on a page, moving the focus to the other toggle', async () => {
    render(<Harness />)
    const collapse = screen.getByRole('button', { name: 'Collapse sidebar' })
    expect(collapse).toHaveAttribute('aria-expanded', 'true')
    expect(aside().className).toContain('w-sidebar')

    await userEvent.click(collapse)
    expect(isCollapsed()).toBe(true)
    expect(aside().className).toContain('w-sidebar-collapsed')
    const expand = screen.getByRole('button', { name: 'Expand sidebar' })
    expect(expand).toHaveAttribute('aria-expanded', 'false')
    expect(document.activeElement).toBe(expand)

    await userEvent.click(expand)
    expect(isCollapsed()).toBe(false)
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Collapse sidebar' }))
  })

  it('shows the symbol instead of the wordmark when collapsed', () => {
    render(<Harness startCollapsed />)
    expect(screen.queryByRole('button', { name: 'Collapse sidebar' })).toBeNull()
    expect(screen.getByRole('img', { name: 'Notifuse' }).getAttribute('viewBox')).toBe('0 0 480 480')
  })

  describe('on a rail page', () => {
    it('renders collapsed, then peeks over the rail with a shadow', async () => {
      render(<Harness rail />)
      expect(isCollapsed()).toBe(true)
      await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
      expect(isCollapsed()).toBe(false)
      expect(aside().className).toContain('shadow-popup')
      expect(screen.getByText('Contacts')).toBeVisible()
      expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Collapse sidebar' }))
    })

    it('closes the peek on Escape and gives the focus back to the expand button', async () => {
      render(<Harness rail />)
      await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
      fireEvent.keyDown(document, { key: 'Escape' })
      expect(isCollapsed()).toBe(true)
      expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Expand sidebar' }))
    })

    it('closes the peek on a click outside, not inside', async () => {
      render(<Harness rail />)
      await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
      fireEvent.mouseDown(screen.getByText('Contacts'))
      expect(isCollapsed()).toBe(false)
      fireEvent.mouseDown(screen.getByRole('button', { name: 'Outside' }))
      expect(isCollapsed()).toBe(true)
    })

    it('closes the peek when the focus leaves the sidebar, not when it moves inside', async () => {
      render(<Harness rail />)
      await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
      fireEvent.focusOut(screen.getByRole('button', { name: 'Collapse sidebar' }), {
        relatedTarget: screen.getByRole('button', { name: /Acme/ })
      })
      expect(isCollapsed()).toBe(false)
      fireEvent.focusOut(screen.getByRole('button', { name: /Acme/ }), {
        relatedTarget: screen.getByRole('button', { name: 'Outside' })
      })
      expect(isCollapsed()).toBe(true)
    })

    it('closes the peek from the header button', async () => {
      render(<Harness rail />)
      await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
      await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
      expect(isCollapsed()).toBe(true)
    })

    it('keeps the peek open while a sidebar menu takes the Escape', async () => {
      render(<Harness rail />)
      await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
      await userEvent.click(screen.getByRole('button', { name: /Acme/ }))
      await screen.findByRole('menuitem', { name: /Beta/ })
      fireEvent.keyDown(document, { key: 'Escape' })
      expect(isCollapsed()).toBe(false)
    })

    // With changeLanguage mocked: in the app the locale change remounts the layout, which ends the peek.
    it('keeps the peek open while the language submenu opens and takes a click', async () => {
      render(<Harness rail />)
      await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
      await userEvent.click(screen.getByRole('button', { name: /jane@example\.com/ }))
      const language = await screen.findByRole('menuitem', { name: /Language/ })
      await userEvent.hover(language)
      const french = await screen.findByText('Français')
      expect(aside().contains(french)).toBe(true)
      await userEvent.click(french)
      expect(changeLanguage).toHaveBeenCalledWith(setLocale, 'fr')
      expect(isCollapsed()).toBe(false)
    })
  })

  describe('collapsed', () => {
    it.each([
      ['Help', 'Documentation'],
      ['Acme', 'Beta'],
      ['jane@example.com', 'Logout']
    ])('hides the tooltip of %s while its menu is open', async (name, item) => {
      render(<Harness startCollapsed />)
      const button = screen.getByRole('button', { name })
      await userEvent.hover(button)
      await screen.findByRole('tooltip')
      await userEvent.click(button)
      await screen.findByText(item)
      await waitFor(() => expect(document.querySelector('.ant-tooltip:not(.ant-tooltip-hidden)')).toBeNull())
    })

    it('keeps the workspace, help and account reachable by name', () => {
      render(<Harness startCollapsed />)
      expect(screen.getByRole('button', { name: 'Acme' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Help' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'jane@example.com' })).toBeInTheDocument()
    })
  })

  describe('workspace switcher', () => {
    it('switches to the workspace picked, the current one selected', async () => {
      const onSelectWorkspace = vi.fn()
      render(<Harness onSelectWorkspace={onSelectWorkspace} />)
      await userEvent.click(screen.getByRole('button', { name: /Acme/ }))
      const menu = (await screen.findByRole('menuitem', { name: /Beta/ })).closest<HTMLElement>(
        '[role="menu"]'
      )!
      expect(within(menu).getByRole('menuitem', { name: /Acme/ }).className).toContain(
        'ant-dropdown-menu-item-selected'
      )
      await userEvent.click(within(menu).getByRole('menuitem', { name: /Beta/ }))
      expect(onSelectWorkspace).toHaveBeenCalledWith('ws2')
    })

    it('offers New workspace to the root user only', async () => {
      const onCreateWorkspace = vi.fn()
      const { unmount } = render(<Harness />)
      await userEvent.click(screen.getByRole('button', { name: /Acme/ }))
      await screen.findByRole('menuitem', { name: /Beta/ })
      expect(screen.queryByRole('menuitem', { name: /New workspace/ })).toBeNull()
      unmount()

      render(<Harness isRoot onCreateWorkspace={onCreateWorkspace} />)
      await userEvent.click(screen.getByRole('button', { name: /Acme/ }))
      const create = await screen.findByRole('menuitem', { name: /New workspace/ })
      expect(create).not.toHaveAttribute('aria-disabled', 'true')
      await userEvent.click(create)
      expect(onCreateWorkspace).toHaveBeenCalledTimes(1)
    })

    it('shows the logo of a workspace that has one instead of the badge', () => {
      render(<Harness workspaceId="ws2" />)
      const button = screen.getByRole('button', { name: /Beta/ })
      expect(button.querySelector('[data-workspace-logo]')).toHaveAttribute(
        'src',
        'https://cdn.example.com/beta.png'
      )
      expect(button.querySelector('[data-workspace-badge]')).toBeNull()
    })
  })

  describe('account', () => {
    const avatar = () => document.querySelector<HTMLElement>('[data-account-avatar]')!

    it('shows the initials until the Gravatar has loaded, then the photo', () => {
      render(<Harness />)
      expect(avatar()).toHaveTextContent('JD')
      expect(images[0].src).toMatch(/^https:\/\/www\.gravatar\.com\/avatar\/[0-9a-f]{32}\?s=52&d=identicon$/)
      act(() => images[0].onload?.())
      expect(avatar().querySelector('img')).toHaveAttribute('src', images[0].src)
    })

    it('keeps the initials while the Gravatar never loads, from the email without a name', () => {
      render(<Harness user={{ email: 'zoe@example.com' }} />)
      expect(images).toHaveLength(1)
      expect(avatar().querySelector('img')).toBeNull()
      expect(avatar()).toHaveTextContent('ZO')
    })

    it.each([
      ['owner', 'Owner · EN'],
      ['member', 'Member · EN'],
      ['root', 'Root · EN'],
      [undefined, 'EN']
    ] as const)('reads the %s role with the language', (role, line) => {
      render(<Harness role={role} />)
      expect(document.querySelector('[data-account-role]')).toHaveTextContent(line)
    })

    it('lists the version and signs out from the menu', async () => {
      const onSignout = vi.fn()
      render(<Harness onSignout={onSignout} />)
      await userEvent.click(screen.getByRole('button', { name: /jane@example\.com/ }))
      expect(await screen.findByText('Notifuse v44.0')).toBeInTheDocument()
      await userEvent.click(screen.getByRole('menuitem', { name: /Logout/ }))
      expect(onSignout).toHaveBeenCalledTimes(1)
    })
  })

  describe('help', () => {
    it('opens the documentation and the issue tracker in a new tab', async () => {
      render(<Harness />)
      await userEvent.click(screen.getByRole('button', { name: 'Help' }))
      const docs = await screen.findByRole('link', { name: 'Documentation' })
      expect(docs).toHaveAttribute('href', 'https://docs.notifuse.com/')
      expect(docs).toHaveAttribute('target', '_blank')
      expect(docs).toHaveAttribute('rel', 'noopener noreferrer')
      expect(screen.getByRole('link', { name: 'Report an issue' })).toHaveAttribute(
        'href',
        'https://github.com/notifuse/notifuse/issues'
      )
    })
  })

  it('names the main navigation', async () => {
    render(<Harness />)
    await waitFor(() =>
      expect(screen.getByRole('navigation', { name: 'Main navigation' })).toBeInTheDocument()
    )
  })
})
