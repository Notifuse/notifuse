import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, createEvent, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { forwardRef, useState, type ReactNode } from 'react'
import type { Workspace } from '../../services/api/types'
import { AppSidebar, type AppSidebarProps } from './AppSidebar'
import { changeLanguage } from '../../i18n/languages'
import { i18n } from '@lingui/core'
import { generateMessageId } from '@lingui/message-utils/generateMessageId'

vi.mock('@tanstack/react-router', () => ({
  // Forwards its ref like the real Link: Escape on the pin moves the focus to the logo.
  Link: forwardRef<HTMLAnchorElement, { children: ReactNode; to: string; params?: object }>(
    ({ children, to, params, ...rest }, ref) => (
      <a ref={ref} href={to} data-params={JSON.stringify(params)} {...rest}>
        {children}
      </a>
    )
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

type HarnessProps = Partial<AppSidebarProps> & { startPinned?: boolean }

/** The sidebar with the pin WorkspaceLayout keeps; the overlay is the sidebar's own state. */
function Harness({ startPinned = true, ...props }: HarnessProps) {
  const [pinned, setPinned] = useState(startPinned)
  return (
    <>
      <AppSidebar
        pinned={pinned}
        onPinnedChange={setPinned}
        navigationKey="/console/workspace/ws1"
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
const isExpanded = () => aside().dataset.expanded === 'true'
const isPinned = () => aside().dataset.pinned === 'true'
// Named by what a click does: Pin sidebar, or Unpin sidebar once pinned.
const pinButton = () => screen.getByRole('button', { name: /^(Pin|Unpin) sidebar$/ })

/** jsdom has no PointerEvent: the touch is written on a plain event. */
const touchDown = (element: Element) => {
  const event = createEvent.pointerDown(element)
  Object.defineProperty(event, 'pointerType', { value: 'touch' })
  fireEvent(element, event)
}

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

  it('links the logo to the page it is given, under that page\'s name', () => {
    render(
      <Harness
        home={{
          target: { to: '/console/workspace/$workspaceId/contacts', params: { workspaceId: 'ws1' } },
          label: 'Contacts'
        }}
      />
    )
    const link = screen.getByRole('img', { name: 'Notifuse' }).closest('a')!
    expect(link).toHaveAttribute('href', '/console/workspace/$workspaceId/contacts')
    expect(link).toHaveAttribute('aria-label', 'Contacts')
  })

  it('follows the banner and keeps the chrome under the banner', () => {
    render(<Harness />)
    expect(aside().style.top).toBe('calc(0px + var(--license-banner-height, 0px))')
    expect(aside().style.height).toBe('calc(100vh - var(--license-banner-height, 0px))')
    expect(aside().className).toContain('z-10')
  })

  it('folds and opens without a transition', () => {
    render(<Harness startPinned={false} />)
    expect(aside().className).not.toMatch(/transition/)
    fireEvent.pointerEnter(aside())
    expect(aside().className).not.toMatch(/transition/)
  })

  describe('pinned', () => {
    it('sits open in the flow, without a shadow, its pin pressed', () => {
      render(<Harness />)
      expect(isExpanded()).toBe(true)
      expect(aside().className).toContain('w-sidebar')
      expect(aside().className).not.toContain('w-sidebar-collapsed')
      expect(aside().className).not.toContain('shadow-popup')
      expect(pinButton()).toHaveAccessibleName('Unpin sidebar')
    })

    it('stays open when the pointer leaves', () => {
      render(<Harness />)
      fireEvent.pointerEnter(aside())
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(true)
    })

    it('unpins from the pin, and stays open over the content until the pointer leaves', async () => {
      render(<Harness />)
      await userEvent.click(pinButton())
      expect(isPinned()).toBe(false)
      expect(isExpanded()).toBe(true)
      expect(aside().className).toContain('shadow-popup')
      expect(pinButton()).toHaveAccessibleName('Pin sidebar')
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(false)
    })
  })

  describe('unpinned', () => {
    it('is the folded rail with the symbol, and no toggle button at all', () => {
      render(<Harness startPinned={false} />)
      expect(isExpanded()).toBe(false)
      expect(aside().className).toContain('w-sidebar-collapsed')
      expect(screen.getByRole('img', { name: 'Notifuse' }).getAttribute('viewBox')).toBe('0 0 480 480')
      for (const name of ['Pin sidebar', 'Unpin sidebar', 'Expand sidebar', 'Collapse sidebar']) {
        expect(screen.queryByRole('button', { name })).toBeNull()
      }
    })

    it('keeps the workspace, help and account reachable by name', () => {
      render(<Harness startPinned={false} />)
      expect(screen.getByRole('button', { name: 'Acme' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Help' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'jane@example.com' })).toBeInTheDocument()
    })

    it('opens over the content while the pointer is on it, and folds when it leaves', () => {
      render(<Harness startPinned={false} />)

      fireEvent.pointerEnter(aside())
      expect(isExpanded()).toBe(true)
      expect(aside().className).toContain('shadow-popup')
      expect(screen.getByText('Contacts')).toBeVisible()
      expect(pinButton()).toHaveAccessibleName('Pin sidebar')

      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(false)
    })

    it('gives its folded entries no title tooltip, which antd drew at the top left as it folded', async () => {
      render(<Harness startPinned={false} />)
      // The entry hovered while the rail is still folded: antd's own tooltip would name it.
      fireEvent.mouseEnter(document.querySelector('.ant-menu-item')!)
      await new Promise((resolve) => setTimeout(resolve, 300))
      expect(document.querySelector('.ant-menu-inline-collapsed-tooltip')).toBeNull()
    })

    it('does not open for a drag passing over it', () => {
      render(<Harness startPinned={false} />)
      const event = createEvent.pointerOver(aside())
      Object.defineProperty(event, 'buttons', { value: 1 })
      fireEvent(aside(), event)
      expect(isExpanded()).toBe(false)
    })

    it('pins from the overlay: in the flow, without the shadow', async () => {
      render(<Harness startPinned={false} />)
      fireEvent.pointerEnter(aside())
      await userEvent.click(pinButton())
      expect(isPinned()).toBe(true)
      expect(aside().className).not.toContain('shadow-popup')
      expect(pinButton()).toHaveAccessibleName('Unpin sidebar')
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(true)
    })

    it('stays open while a sidebar menu is open, and folds once it closes with the pointer outside', async () => {
      render(<Harness startPinned={false} />)
      await userEvent.click(screen.getByRole('button', { name: 'Acme' }))
      await screen.findByRole('menuitem', { name: /Beta/ })
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(true)
      await userEvent.click(screen.getByRole('button', { name: 'Outside' }))
      await waitFor(() => expect(isExpanded()).toBe(false))
    })

    // With changeLanguage mocked: in the app the locale change remounts the layout.
    it('keeps the overlay open while the language submenu opens and takes a click', async () => {
      render(<Harness startPinned={false} />)
      await userEvent.click(screen.getByRole('button', { name: 'jane@example.com' }))
      const language = await screen.findByRole('menuitem', { name: /Language/ })
      await userEvent.hover(language)
      const french = await screen.findByText('Français')
      expect(aside().contains(french)).toBe(true)
      await userEvent.click(french)
      expect(changeLanguage).toHaveBeenCalledWith(setLocale, 'fr')
      expect(isExpanded()).toBe(true)
    })

    it.each(['Acme', 'Help', 'jane@example.com'])(
      'opens on keyboard focus on %s, and the focus stays on it',
      (name) => {
        render(<Harness startPinned={false} />)
        const button = screen.getByRole('button', { name })
        act(() => button.focus())
        expect(isExpanded()).toBe(true)
        expect(button).toBeInTheDocument()
        expect(document.activeElement).toBe(button)
      }
    )

    it('folds when the focus leaves the sidebar, not when it moves inside', async () => {
      render(<Harness startPinned={false} />)
      act(() => screen.getByRole('button', { name: 'Acme' }).focus())
      act(() => screen.getByRole('button', { name: 'Help' }).focus())
      await act(() => new Promise((resolve) => setTimeout(resolve, 0)))
      expect(isExpanded()).toBe(true)
      act(() => screen.getByRole('button', { name: 'Outside' }).focus())
      await waitFor(() => expect(isExpanded()).toBe(false))
    })

    it('folds when the focus goes where blur cannot name it (an iframe), not when the window loses it', async () => {
      render(<Harness startPinned={false} />)
      const workspace = screen.getByRole('button', { name: 'Acme' })
      act(() => workspace.focus())
      // Another window: blur without a relatedTarget, the focused element unchanged.
      fireEvent.blur(workspace)
      await act(() => new Promise((resolve) => setTimeout(resolve, 0)))
      expect(isExpanded()).toBe(true)

      // A click into an iframe: blur without a relatedTarget, the focus now outside the sidebar.
      const frame = document.createElement('iframe')
      document.body.appendChild(frame)
      act(() => frame.focus())
      await waitFor(() => expect(isExpanded()).toBe(false))
      frame.remove()
    })

    it('holds on a keyboard focus even while the mouse rests on it, so the focused pin survives the mouse leaving', () => {
      render(<Harness startPinned={false} />)
      fireEvent.pointerEnter(aside())
      const pin = pinButton()
      act(() => pin.focus())
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(true)
      expect(document.activeElement).toBe(pin)
    })

    it('does not hold on the focus a click gives, even after a navigation under the pointer', async () => {
      const { rerender } = render(<Harness />)
      fireEvent.pointerEnter(aside())
      // An entry clicked: the page changes with the pointer still on the sidebar.
      rerender(<Harness navigationKey="/console/workspace/ws1/templates" />)
      const pin = pinButton()
      fireEvent.pointerDown(pin)
      act(() => pin.focus())
      fireEvent.pointerUp(pin)
      fireEvent.click(pin, { detail: 1 })
      expect(isPinned()).toBe(false)
      // The press shows the pointer is there: open over the content until it leaves.
      expect(isExpanded()).toBe(true)
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(false)
    })

    it('folds on Escape, the focus kept where it was', () => {
      render(<Harness startPinned={false} />)
      const workspace = screen.getByRole('button', { name: 'Acme' })
      act(() => workspace.focus())
      fireEvent.keyDown(document, { key: 'Escape' })
      expect(isExpanded()).toBe(false)
      expect(document.activeElement).toBe(workspace)
    })

    it('moves the focus from the pin to the logo when Escape folds it', () => {
      render(<Harness startPinned={false} />)
      act(() => screen.getByRole('button', { name: 'Acme' }).focus())
      act(() => pinButton().focus())
      fireEvent.keyDown(document, { key: 'Escape' })
      expect(isExpanded()).toBe(false)
      expect(document.activeElement).toBe(screen.getByRole('link', { name: 'Dashboard' }))
    })

    it('stays open while a sidebar menu takes the Escape', async () => {
      render(<Harness startPinned={false} />)
      const workspace = screen.getByRole('button', { name: 'Acme' })
      act(() => workspace.focus())
      fireEvent.click(workspace)
      await screen.findByRole('menuitem', { name: /Beta/ })
      fireEvent.keyDown(document, { key: 'Escape' })
      expect(isExpanded()).toBe(true)
    })

    it('stays open under the pointer when the page changes on its own (a redirect, a save)', () => {
      const { rerender } = render(<Harness startPinned={false} />)
      fireEvent.pointerEnter(aside())
      rerender(<Harness startPinned={false} navigationKey="/console/workspace/ws1/contacts" />)
      expect(isExpanded()).toBe(true)
    })

    it('still opens on keyboard focus after a press inside was released over the page', () => {
      render(<Harness startPinned={false} />)
      fireEvent.pointerDown(screen.getByRole('button', { name: 'Help' }))
      fireEvent.pointerLeave(aside())
      fireEvent.pointerUp(screen.getByRole('button', { name: 'Outside' }))
      expect(isExpanded()).toBe(false)
      act(() => screen.getByRole('button', { name: 'Acme' }).focus())
      expect(isExpanded()).toBe(true)
    })

    it('hands the focus of the pin to the logo when the pointer leaves and it folds', async () => {
      render(<Harness />)
      fireEvent.pointerEnter(aside())
      await userEvent.click(pinButton())
      expect(document.activeElement).toBe(pinButton())
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(false)
      expect(document.activeElement).toBe(screen.getByRole('link', { name: 'Dashboard' }))
    })

    it('does not keep a focus taken while pinned once unpinned with the mouse', async () => {
      render(<Harness />)
      act(() => screen.getByRole('button', { name: /Acme/ }).focus())
      await userEvent.click(pinButton())
      expect(isExpanded()).toBe(true)
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(false)
    })

    it('stays open when unpinned from the keyboard, until the focus leaves', async () => {
      render(<Harness />)
      const pin = pinButton()
      act(() => pin.focus())
      // Enter or Space: a click with no pointer.
      fireEvent.click(pin, { detail: 0 })
      expect(isPinned()).toBe(false)
      expect(isExpanded()).toBe(true)
      act(() => screen.getByRole('button', { name: 'Outside' }).focus())
      await waitFor(() => expect(isExpanded()).toBe(false))
    })

    it.each([
      ['Help', 'Help'],
      ['Acme', 'Acme'],
      ['jane@example.com', 'jane@example.com']
    ])('names the folded %s icon in a tooltip', async (name, title) => {
      render(<Harness startPinned={false} />)
      fireEvent.mouseEnter(screen.getByRole('button', { name }))
      expect(await screen.findByRole('tooltip')).toHaveTextContent(title)
    })

    it('acts on a tapped entry at once, without widening first', () => {
      render(<Harness startPinned={false} />)
      touchDown(screen.getByRole('menuitem', { name: /Contacts/ }))
      expect(isExpanded()).toBe(false)
    })

    it('opens on a tap, not on a touch passing over it, and folds on a tap outside', () => {
      render(<Harness startPinned={false} />)
      const event = createEvent.pointerOver(aside())
      Object.defineProperty(event, 'pointerType', { value: 'touch' })
      fireEvent(aside(), event)
      expect(isExpanded()).toBe(false)
      touchDown(aside())
      expect(isExpanded()).toBe(true)
      touchDown(screen.getByRole('button', { name: 'Outside' }))
      expect(isExpanded()).toBe(false)
    })

    it('folds on a navigation made from it, whether held by the focus or by the pointer', () => {
      const { rerender } = render(<Harness startPinned={false} />)
      act(() => screen.getByRole('button', { name: 'Acme' }).focus())
      rerender(<Harness startPinned={false} navigationKey="/console/workspace/ws1/contacts" />)
      expect(isExpanded()).toBe(false)

      // A click on an entry: the pointer is still on the sidebar when the page changes.
      fireEvent.pointerEnter(aside())
      expect(isExpanded()).toBe(true)
      fireEvent.pointerDown(screen.getByRole('menuitem', { name: /Contacts/ }))
      rerender(<Harness startPinned={false} navigationKey="/console/workspace/ws1/lists" />)
      expect(isExpanded()).toBe(false)
      expect(aside().className).not.toContain('shadow-popup')

      // It opens again once the pointer has left and come back.
      fireEvent.pointerLeave(aside())
      fireEvent.pointerEnter(aside())
      expect(isExpanded()).toBe(true)
    })
  })

  describe('menu buttons', () => {
    const menus = [
      ['workspace', /Acme/, /Beta/],
      ['help', /Help/, /Documentation/],
      ['account', /jane@example\.com/, /Logout/]
    ] as const

    it.each(menus)('says the %s button opens a menu, and whether it is open', async (_, name, item) => {
      render(<Harness />)
      const button = screen.getByRole('button', { name })
      expect(button).toHaveAttribute('aria-haspopup', 'menu')
      expect(button).toHaveAttribute('aria-expanded', 'false')
      await userEvent.click(button)
      await screen.findByRole('menuitem', { name: item })
      expect(button).toHaveAttribute('aria-expanded', 'true')
    })

    it.each(menus)('gives the focus back to the %s button after Escape', async (_, name, item) => {
      render(<Harness />)
      const button = screen.getByRole('button', { name })
      await userEvent.click(button)
      await screen.findByRole('menuitem', { name: item })
      // Where antd leaves it once the menu took the keyboard
      button.blur()
      fireEvent.keyDown(document, { key: 'Escape' })
      await waitFor(() => expect(button).toHaveAttribute('aria-expanded', 'false'))
      await waitFor(() => expect(document.activeElement).toBe(button))
    })

    it('does not take the focus back after an entry picked with the mouse, so the unpinned sidebar still folds', async () => {
      render(<Harness startPinned={false} />)
      fireEvent.pointerEnter(aside())
      const button = screen.getByRole('button', { name: /jane@example\.com/ })
      await userEvent.click(button)
      const logout = await screen.findByRole('menuitem', { name: /Logout/ })
      fireEvent.pointerDown(logout)
      // Where the press leaves the focus once the entry closes the menu
      act(() => button.blur())
      // A modifier held during the click repeats its keydown: still a mouse pick.
      fireEvent.keyDown(document, { key: 'Shift' })
      fireEvent.click(logout)
      await waitFor(() => expect(button).toHaveAttribute('aria-expanded', 'false'))
      await act(() => new Promise((resolve) => setTimeout(resolve, 0)))
      expect(document.activeElement).not.toBe(button)
      fireEvent.pointerLeave(aside())
      expect(isExpanded()).toBe(false)
    })

    it('leaves the focus on the control a click outside moved it to', async () => {
      render(<Harness />)
      await userEvent.click(screen.getByRole('button', { name: /Acme/ }))
      await screen.findByRole('menuitem', { name: /Beta/ })
      const outside = screen.getByRole('button', { name: 'Outside' })
      await userEvent.click(outside)
      await waitFor(() => expect(screen.getByRole('button', { name: /Acme/ })).toHaveAttribute('aria-expanded', 'false'))
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(document.activeElement).toBe(outside)
    })
  })

  describe('workspace switcher', () => {
    it('names the button after the workspace, or "Workspace" when the list does not hold it', () => {
      const { unmount } = render(<Harness startPinned={false} />)
      expect(screen.getByRole('button', { name: 'Acme' })).toBeInTheDocument()
      unmount()
      render(<Harness startPinned={false} workspaceId="gone" />)
      expect(screen.getByRole('button', { name: 'Workspace' })).toBeInTheDocument()
    })

    it('cuts a long name rather than widening the menu', async () => {
      render(<Harness />)
      await userEvent.click(screen.getByRole('button', { name: /Acme/ }))
      const menu = await screen.findByRole('menuitem', { name: /Beta/ })
      expect(menu.closest<HTMLElement>('.ant-dropdown')!.style.maxWidth).toBe('320px')
      expect(within(menu).getByText('Beta').className).toContain('truncate')
    })

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
      expect(images[0].src).toMatch(/^https:\/\/www\.gravatar\.com\/avatar\/[0-9a-f]{32}\?s=52&d=404$/)
      act(() => images[0].onload?.())
      expect(avatar().querySelector('img')).toHaveAttribute('src', images[0].src)
    })

    it('keeps the initials when the email has no Gravatar', () => {
      render(<Harness />)
      act(() => images[0].onerror?.())
      expect(avatar().querySelector('img')).toBeNull()
      expect(avatar()).toHaveTextContent('JD')
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

    it('reads the root role in its own words, apart from the root folder', () => {
      i18n.load('fr', { [generateMessageId('Root', 'role')]: 'Administrateur', Root: 'Racine' })
      i18n.activate('fr')
      try {
        render(<Harness role="root" />)
        expect(document.querySelector('[data-account-role]')).toHaveTextContent('Administrateur · EN')
      } finally {
        i18n.activate('en')
      }
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
