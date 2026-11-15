import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent
} from 'react'
import { Menu, Tooltip, type MenuProps } from 'antd'
import { Link } from '@tanstack/react-router'
import { Pin } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { NotifuseLogo, NotifuseMark } from '../../components/brand/NotifuseLogo'
import { minusBannerOffset, withBannerOffset } from '../../components/license/bannerOffset'
import type { Workspace } from '../../services/api/types'
import { WorkspaceSwitcher } from './WorkspaceSwitcher'
import { HelpMenu } from './HelpMenu'
import { AccountMenu, type AccountRole } from './AccountMenu'
import type { MenuItem } from './menuItems'
import type { MenuTarget } from './MenuLink'

export interface AppSidebarProps {
  /** Pinned, the sidebar is open in the page flow; unpinned, it is the folded rail. */
  pinned: boolean
  onPinnedChange: (pinned: boolean) => void
  /** Changes on every navigation: an opening held by the focus or a tap ends there. */
  navigationKey: string
  menuItems: MenuItem[]
  selectedKey: string
  openKeys: string[]
  onOpenChange: MenuProps['onOpenChange']
  workspaces: Workspace[]
  workspaceId: string
  isRoot: boolean
  onSelectWorkspace: (workspaceId: string) => void
  onCreateWorkspace: () => void
  user: { email: string; name?: string } | null
  role?: AccountRole
  onSignout: () => void
  /** Where the logo leads, and its name; the workspace dashboard without one. */
  home?: { target: MenuTarget; label: string }
}

type SidebarMenu = 'workspace' | 'help' | 'account'

/**
 * The console's only chrome: logo, workspace, navigation, help and account, with no top bar.
 *
 * Unpinned, it is a 56px rail that opens over the content while the pointer is on it, while the
 * keyboard focus is in it, after a tap on it, or while one of its menus is open. The pin keeps it
 * open in the page flow.
 */
export function AppSidebar({
  pinned,
  onPinnedChange,
  navigationKey,
  menuItems,
  selectedKey,
  openKeys,
  onOpenChange,
  workspaces,
  workspaceId,
  isRoot,
  onSelectWorkspace,
  onCreateWorkspace,
  user,
  role,
  onSignout,
  home
}: AppSidebarProps) {
  const { t } = useLingui()
  const asideRef = useRef<HTMLElement>(null)
  const logoRef = useRef<HTMLAnchorElement>(null)
  const pinRef = useRef<HTMLButtonElement>(null)

  // Each source that holds the sidebar open over the content. The pointer holds it while it is on
  // it, except after a press there led to another page: the entry clicked is reached, the sidebar
  // folds, and it opens again once the pointer has left and come back. A page changing on its own
  // (a redirect, a save) leaves it open.
  const [pointerInside, setPointerInside] = useState(false)
  const [pressedOn, setPressedOn] = useState<string | null>(null)
  const hovered = pointerInside && (pressedOn === null || pressedOn === navigationKey)
  const [focusHeld, setFocusHeld] = useState(false)
  const [touchHeld, setTouchHeld] = useState(false)
  const [openMenus, setOpenMenus] = useState<Record<SidebarMenu, boolean>>({
    workspace: false,
    help: false,
    account: false
  })
  const menuOpen = openMenus.workspace || openMenus.help || openMenus.account

  const overlay = !pinned && (hovered || focusHeld || touchHeld || menuOpen)
  const expanded = pinned || overlay

  // Between a press inside and its release, wherever that happens: the focus the press gives
  // belongs to the pointer, which holds the sidebar by itself. Any other focus (Tab) holds it
  // until the focus leaves.
  const pressingRef = useRef(false)
  const blurTimerRef = useRef<number | undefined>(undefined)
  useEffect(() => {
    const release = () => {
      pressingRef.current = false
    }
    // On window: a mouse pressed in the sidebar may be released over the page.
    window.addEventListener('pointerup', release, true)
    window.addEventListener('pointercancel', release, true)
    return () => {
      window.removeEventListener('pointerup', release, true)
      window.removeEventListener('pointercancel', release, true)
      window.clearTimeout(blurTimerRef.current)
    }
  }, [])

  // Read by the document listeners, registered once per pin state rather than on every hover.
  const liveRef = useRef({ hovered, menuOpen })
  useLayoutEffect(() => {
    liveRef.current = { hovered, menuOpen }
  })

  const menuOpenChange = (menu: SidebarMenu) => (open: boolean) =>
    setOpenMenus((current) => (current[menu] === open ? current : { ...current, [menu]: open }))

  // The sidebar's popups render inside it, so moving into one is neither a leave nor a click outside.
  const getPopupContainer = useCallback(() => asideRef.current ?? document.body, [])

  // A navigation also ends an opening held by the focus or a tap. Adjusted while rendering, as
  // React recommends for state that follows a prop.
  const [seenNavigationKey, setSeenNavigationKey] = useState(navigationKey)
  if (seenNavigationKey !== navigationKey) {
    setSeenNavigationKey(navigationKey)
    setFocusHeld(false)
    setTouchHeld(false)
  }

  /** The pin only exists while the sidebar is open: before it folds, its focus goes to the logo. */
  const rescuePinFocus = () => {
    if (document.activeElement !== pinRef.current) return
    logoRef.current?.focus()
    // That focus is a move, not a hold.
    setFocusHeld(false)
  }

  useEffect(() => {
    if (pinned) return
    const onPointerDown = (event: PointerEvent) => {
      if (!asideRef.current?.contains(event.target as Node)) {
        setTouchHeld(false)
        setFocusHeld(false)
      }
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      // Escape closes an open sidebar menu first; antd handles that on window, after document.
      if (liveRef.current.menuOpen) return
      if (!liveRef.current.hovered) rescuePinFocus()
      setFocusHeld(false)
      setTouchHeld(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [pinned])

  const togglePin = (event: ReactMouseEvent) => {
    const unpinning = pinned
    onPinnedChange(!pinned)
    // Unpinned from the keyboard (a click with no pointer, detail 0), the focus holds it open until
    // it leaves; from the mouse, the pointer does.
    if (unpinning) setFocusHeld(event.detail === 0)
  }

  // jsdom's pointer events carry no pointerType: anything but a touch counts as a mouse.
  const isTouch = (event: ReactPointerEvent) => event.pointerType === 'touch'

  const pinLabel = pinned ? t`Unpin sidebar` : t`Pin sidebar`
  const focusRing = 'focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none'

  return (
    <aside
      ref={asideRef}
      data-sidebar
      data-expanded={expanded}
      data-pinned={pinned}
      onPointerEnter={(event) => {
        // A button held is a drag passing by (a block in the email editor, a node in the automation
        // flow): opening over the canvas would cover the drop target.
        if (!isTouch(event) && !event.buttons) {
          setPointerInside(true)
          setPressedOn(null)
        }
      }}
      onPointerLeave={(event) => {
        if (isTouch(event)) return
        // Nothing else holds it: it folds now, and a focused pin would take the focus with it.
        if (!pinned && !focusHeld && !touchHeld && !menuOpen) rescuePinFocus()
        setPointerInside(false)
        setPressedOn(null)
      }}
      onPointerDownCapture={(event) => {
        pressingRef.current = true
        // A tap on the rail opens it; a tap on an entry or a button acts at once, without first
        // widening the sidebar under the finger.
        const target = event.target as Element
        if (isTouch(event)) {
          if (!target.closest('a, button, [role="menuitem"]')) setTouchHeld(true)
          return
        }
        // A press proves the pointer is on the sidebar, and marks the page it was made on: if it
        // leads elsewhere, the sidebar folds there.
        setPointerInside(true)
        setPressedOn(navigationKey)
      }}
      onFocus={() => {
        window.clearTimeout(blurTimerRef.current)
        // A hold taken while pinned does not outlive it: unpinning with the pin decides the hold
        // anew, and a navigation (Contacts unpins) clears it.
        if (!pressingRef.current) setFocusHeld(true)
      }}
      onBlur={() => {
        // Where the focus went is only known once it has landed: relatedTarget is null for a click
        // into an iframe (the email preview) or a focused element removed. Switching windows keeps
        // the focused element, so the sidebar stays as it was.
        window.clearTimeout(blurTimerRef.current)
        blurTimerRef.current = window.setTimeout(() => {
          const active = document.activeElement
          if (active && asideRef.current && !asideRef.current.contains(active)) setFocusHeld(false)
        })
      }}
      className={`fixed left-0 z-10 flex flex-col bg-surface ${expanded ? 'w-sidebar' : 'w-sidebar-collapsed'} ${
        overlay ? 'shadow-popup' : ''
      }`}
      // Both follow the licence banner, fixed at the top of the viewport above this layout.
      style={{ top: withBannerOffset('0px'), height: minusBannerOffset('100vh') }}
    >
      {/* The right border, drawn over the sidebar so its 56px stay usable when folded. */}
      <span aria-hidden="true" className="pointer-events-none absolute inset-y-0 right-0 w-px bg-line" />

      <div
        className={`flex h-sidebar-head shrink-0 items-center ${
          expanded ? 'justify-between pr-3.5 pl-5' : 'justify-center'
        }`}
      >
        <Link
          ref={logoRef}
          {...(home?.target ?? { to: '/console/workspace/$workspaceId', params: { workspaceId } })}
          aria-label={home?.label ?? t`Dashboard`}
          className={`rounded-sm ${focusRing}`}
        >
          {expanded ? <NotifuseLogo height={22} /> : <NotifuseMark height={24} />}
        </Link>
        {expanded ? (
          <Tooltip title={pinLabel} placement="bottom" getPopupContainer={getPopupContainer}>
            <button
              ref={pinRef}
              type="button"
              // The name says what a click does, as the tooltip does: a voice command reads it.
              aria-label={pinLabel}
              onClick={togglePin}
              className={`grid size-7 cursor-pointer place-items-center rounded-md border-0 ${
                pinned
                  ? 'bg-primary-50 text-primary-600 hover:bg-primary-100'
                  : 'bg-transparent text-ink-3 hover:bg-fill hover:text-ink'
              } ${focusRing}`}
            >
              <Pin size={15} aria-hidden="true" />
            </button>
          </Tooltip>
        ) : null}
      </div>

      {/* Folded, the wrapper centres the button: a margin on the button itself (mx-auto) loses to the
          unlayered `button { margin: 0 }` reset, which beats Tailwind's layered utilities. */}
      <div className={`mt-2 mb-4 shrink-0 ${expanded ? 'mx-2.5' : 'flex justify-center'}`}>
        <WorkspaceSwitcher
          workspaces={workspaces}
          workspaceId={workspaceId}
          isRoot={isRoot}
          collapsed={!expanded}
          onSelect={onSelectWorkspace}
          onCreate={onCreateWorkspace}
          onMenuOpenChange={menuOpenChange('workspace')}
          getPopupContainer={getPopupContainer}
        />
      </div>

      <nav aria-label={t`Main navigation`} className={`app-sidebar-nav ${expanded ? '' : 'is-collapsed'}`}>
        <Menu
          mode="inline"
          inlineIndent={14}
          inlineCollapsed={!expanded}
          // No title tooltip on the folded entries: the pointer that would show it unfolds the
          // sidebar, labels included. antd still started one as the sidebar folded under a
          // leaving pointer, drawn at the top left, never placed, through its leave animation.
          tooltip={false}
          selectedKeys={[selectedKey]}
          // Folded, antd keeps the groups' state itself. It also reports an empty set on folding,
          // through the onOpenChange of that render: none is passed there, so the groups opened by
          // hand are still open on the next hover.
          {...(expanded ? { openKeys } : {})}
          onOpenChange={expanded ? onOpenChange : undefined}
          items={menuItems}
        />
      </nav>

      <div
        className={`flex shrink-0 flex-col gap-1 border-t border-line-soft px-2.5 pt-2 pb-2.5 ${
          expanded ? '' : 'items-center'
        }`}
      >
        <HelpMenu collapsed={!expanded} onMenuOpenChange={menuOpenChange('help')} getPopupContainer={getPopupContainer} />
        <AccountMenu
          user={user}
          role={role}
          collapsed={!expanded}
          onSignout={onSignout}
          onMenuOpenChange={menuOpenChange('account')}
          getPopupContainer={getPopupContainer}
        />
      </div>
    </aside>
  )
}

