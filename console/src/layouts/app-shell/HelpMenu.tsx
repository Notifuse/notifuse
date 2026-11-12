import { useState } from 'react'
import { Dropdown, Tooltip } from 'antd'
import { BookOpen, Bug, CircleHelp } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'

export interface HelpMenuProps {
  collapsed: boolean
  getPopupContainer: () => HTMLElement
}

/** Documentation and issue tracker, opened above the button. */
export function HelpMenu({ collapsed, getPopupContainer }: HelpMenuProps) {
  const { t } = useLingui()
  const [menuOpen, setMenuOpen] = useState(false)
  const button = (
    <button
      type="button"
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
      onOpenChange={setMenuOpen}
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
      {collapsed ? (
        <Tooltip
          title={t`Help`}
          placement="right"
          getPopupContainer={getPopupContainer}
          // A tooltip left over the menu it opened would cover its first or last row.
          open={menuOpen ? false : undefined}
        >
          {button}
        </Tooltip>
      ) : (
        button
      )}
    </Dropdown>
  )
}
