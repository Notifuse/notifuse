import { useState } from 'react'
import { Dropdown, Tooltip } from 'antd'
import { BookOpen, Bug, CircleHelp } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useFocusBack } from './useFocusBack'

export interface HelpMenuProps {
  collapsed: boolean
  /** Reports the menu opening and closing: an open menu holds the sidebar open. */
  onMenuOpenChange?: (open: boolean) => void
  getPopupContainer: () => HTMLElement
}

/** Documentation and issue tracker, opened above the button. */
export function HelpMenu({ collapsed, onMenuOpenChange, getPopupContainer }: HelpMenuProps) {
  const { t } = useLingui()
  const [menuOpen, setMenuOpen] = useState(false)
  const { ref, focusBack } = useFocusBack<HTMLButtonElement>()
  const button = (
    <button
      ref={ref}
      type="button"
      aria-haspopup="menu"
      aria-expanded={menuOpen}
      aria-label={collapsed ? t`Help` : undefined}
      className={`flex h-8.5 cursor-pointer items-center gap-2.5 rounded-md border-0 bg-transparent text-sm text-ink-2 hover:bg-line-soft focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none ${
        collapsed ? 'mx-auto w-9 justify-center' : 'w-full px-2.5 text-left'
      }`}
    >
      <CircleHelp size={16} aria-hidden="true" className="shrink-0 text-ink-3" />
      {collapsed ? null : <span>{t`Help`}</span>}
    </button>
  )
  return (
    <Dropdown
      trigger={['click']}
      onOpenChange={(open) => {
        setMenuOpen(open)
        onMenuOpenChange?.(open)
        if (!open) focusBack()
      }}
      placement={collapsed ? 'rightBottom' : 'topLeft'}
      getPopupContainer={getPopupContainer}
      menu={{
        items: [
          {
            key: 'docs',
            icon: <BookOpen size={14} aria-hidden="true" />,
            label: (
              <a href="https://docs.notifuse.com/" target="_blank" rel="noopener noreferrer">
                {t`Documentation`}
              </a>
            )
          },
          {
            key: 'report-issue',
            icon: <Bug size={14} aria-hidden="true" />,
            label: (
              <a href="https://github.com/notifuse/notifuse/issues" target="_blank" rel="noopener noreferrer">
                {t`Report an issue`}
              </a>
            )
          }
        ]
      }}
    >
      {/* Always mounted, titled only when folded: swapping the wrapper would remount the button and
          drop its focus when the sidebar opens. The name shows when the sidebar folds under a
          still pointer, after a click on an entry. */}
      <Tooltip
        title={collapsed ? t`Help` : undefined}
        placement="right"
        getPopupContainer={getPopupContainer}
        // A tooltip left over the menu it opened would cover its first or last row.
        open={menuOpen ? false : undefined}
      >
        {button}
      </Tooltip>
    </Dropdown>
  )
}
