import { useCallback, useEffect, useRef } from 'react'
import { Menu, type MenuProps } from 'antd'
import { Link } from '@tanstack/react-router'
import { PanelLeft } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { NotifuseLogo, NotifuseMark } from '../../components/brand/NotifuseLogo'
import { minusBannerOffset, withBannerOffset } from '../../components/license/bannerOffset'
import type { Workspace } from '../../services/api/types'
import { WorkspaceSwitcher } from './WorkspaceSwitcher'
import { HelpMenu } from './HelpMenu'
import { AccountMenu, type AccountRole } from './AccountMenu'
import type { MenuItem } from './menuItems'

export interface AppSidebarProps {
  /** Shown folded to icons: the stored choice, or always on a rail page unless peeking. */
  collapsed: boolean
  /** Expanded over a rail page for a moment, without moving the content. */
  peeking: boolean
  /** True on a rail page: expanding there peeks instead of changing the stored choice. */
  rail: boolean
  /** Flips the stored choice. */
  onToggle: () => void
  onPeekChange: (peeking: boolean) => void
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
}

/** The console's only chrome: logo, workspace, navigation, help and account, with no top bar. */
export function AppSidebar({
  collapsed,
  peeking,
  rail,
  onToggle,
  onPeekChange,
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
  onSignout
}: AppSidebarProps) {
  const { t } = useLingui()
  const asideRef = useRef<HTMLElement>(null)
  const collapseRef = useRef<HTMLButtonElement>(null)
  const expandRef = useRef<HTMLButtonElement>(null)
  // Which toggle takes the focus once the sidebar has changed width: the one that undoes it.
  const pendingFocus = useRef<'collapse' | 'expand' | null>(null)

  // The sidebar's popups render inside it, so a click in one of them is not a click outside.
  const getPopupContainer = useCallback(() => asideRef.current ?? document.body, [])

  useEffect(() => {
    const target = pendingFocus.current
    if (!target) return
    pendingFocus.current = null
    ;(target === 'collapse' ? collapseRef : expandRef).current?.focus()
  }, [collapsed])

  useEffect(() => {
    if (!peeking) return
    const onMouseDown = (event: MouseEvent) => {
      if (!asideRef.current?.contains(event.target as Node)) onPeekChange(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      // Escape closes an open sidebar menu first; antd handles that on window, after document.
      if (asideRef.current?.querySelector('.ant-dropdown:not(.ant-dropdown-hidden)')) return
      pendingFocus.current = 'expand'
      onPeekChange(false)
    }
    // Tabbing out of the peek would put the focus on the rail hidden under it.
    const onFocusOut = (event: FocusEvent) => {
      const next = event.relatedTarget as Node | null
      if (next && !asideRef.current?.contains(next)) onPeekChange(false)
    }
    const aside = asideRef.current
    document.addEventListener('mousedown', onMouseDown)
    document.addEventListener('keydown', onKeyDown)
    aside?.addEventListener('focusout', onFocusOut)
    return () => {
      document.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('keydown', onKeyDown)
      aside?.removeEventListener('focusout', onFocusOut)
    }
  }, [peeking, onPeekChange])

  const collapse = () => {
    pendingFocus.current = 'expand'
    if (peeking) onPeekChange(false)
    else onToggle()
  }

  const expand = () => {
    pendingFocus.current = 'collapse'
    if (rail) onPeekChange(true)
    else onToggle()
  }

  const focusRing = 'focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none'

  return (
    <aside
      ref={asideRef}
      data-sidebar
      data-collapsed={collapsed}
      className={`fixed left-0 z-10 flex flex-col bg-surface motion-safe:transition-[width] motion-safe:duration-180 ${
        collapsed ? 'w-sidebar-collapsed' : 'w-sidebar'
      } ${peeking ? 'shadow-popup' : ''}`}
      // Both follow the licence banner, fixed at the top of the viewport above this layout.
      style={{ top: withBannerOffset('0px'), height: minusBannerOffset('100vh') }}
    >
      {/* The right border, drawn over the sidebar so its 56px stay usable when folded. */}
      <span aria-hidden="true" className="pointer-events-none absolute inset-y-0 right-0 w-px bg-line" />

      <div
        className={`flex h-sidebar-head shrink-0 items-center ${
          collapsed ? 'justify-center' : 'justify-between pr-3.5 pl-5'
        }`}
      >
        <Link
          to="/console/workspace/$workspaceId"
          params={{ workspaceId }}
          aria-label={t`Dashboard`}
          className={`rounded-sm ${focusRing}`}
        >
          {collapsed ? <NotifuseMark height={24} /> : <NotifuseLogo height={22} />}
        </Link>
        {collapsed ? null : (
          <button
            ref={collapseRef}
            type="button"
            aria-label={t`Collapse sidebar`}
            aria-expanded={true}
            onClick={collapse}
            className={`grid size-7 cursor-pointer place-items-center rounded-md border-0 bg-transparent text-ink-3 hover:bg-fill hover:text-ink ${focusRing}`}
          >
            <PanelLeft size={15} aria-hidden="true" />
          </button>
        )}
      </div>

      <div className={`shrink-0 ${collapsed ? 'mb-2.5' : 'mx-2.5 mb-2.5'}`}>
        <WorkspaceSwitcher
          workspaces={workspaces}
          workspaceId={workspaceId}
          isRoot={isRoot}
          collapsed={collapsed}
          onSelect={onSelectWorkspace}
          onCreate={onCreateWorkspace}
          getPopupContainer={getPopupContainer}
        />
      </div>

      <nav aria-label={t`Main navigation`} className={`app-sidebar-nav ${collapsed ? 'is-collapsed' : ''}`}>
        <Menu
          mode="inline"
          inlineIndent={14}
          inlineCollapsed={collapsed}
          selectedKeys={[selectedKey]}
          // Folded, antd opens the groups as flyouts on hover and keeps their state itself.
          {...(collapsed ? {} : { openKeys })}
          onOpenChange={onOpenChange}
          items={menuItems}
        />
      </nav>

      <div
        className={`flex shrink-0 flex-col gap-1 border-t border-line-soft px-2.5 pt-2 pb-2.5 ${
          collapsed ? 'items-center' : ''
        }`}
      >
        <HelpMenu collapsed={collapsed} getPopupContainer={getPopupContainer} />
        <AccountMenu
          user={user}
          role={role}
          collapsed={collapsed}
          onSignout={onSignout}
          getPopupContainer={getPopupContainer}
        />
        {collapsed ? (
          <button
            ref={expandRef}
            type="button"
            aria-label={t`Expand sidebar`}
            aria-expanded={false}
            onClick={expand}
            className={`mx-auto mt-1 grid size-8.5 cursor-pointer place-items-center rounded-md border-0 bg-transparent text-ink-3 hover:bg-line-soft hover:text-ink ${focusRing}`}
          >
            <PanelLeft size={15} aria-hidden="true" />
          </button>
        ) : null}
      </div>
    </aside>
  )
}
