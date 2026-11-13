import { useRef, type ReactNode } from 'react'
import { Button, Drawer, type DrawerProps } from 'antd'
import { ArrowLeft } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { CloseButton, CopyLinkButton, DrawerNavButtons, type DrawerNav } from './DrawerControls'
import { useDrawerNavKeys } from './useDrawerNavKeys'

export interface FullDrawerProps {
  open: boolean
  onClose: () => void
  /** Label of the back button, the name of the list ("Broadcasts"). */
  backLabel?: ReactNode
  nav?: DrawerNav
  /** A copy-link button (default). */
  copyLink?: boolean
  /** Left side of the bar, after the back button: an editable name, a status. */
  barTitle?: ReactNode
  /** Right side of the bar, before copy and close. */
  topActions?: ReactNode
  /**
   * A screen that lays out its own columns (a canvas): the content fills the height under the bar,
   * without the page padding.
   */
  fill?: boolean
  /**
   * rc-drawer slides a drawer aside by its own `push` when a drawer opens inside it: `false` for a
   * screen whose fields open drawers of their own.
   */
  push?: DrawerProps['push']
  children?: ReactNode
}

/**
 * A full-width drawer over the whole console, sidebar included, with a 40px navigation bar.
 * Escape closes it (antd). Its content sits on the canvas (antd-overrides.css, `.app-full-drawer`).
 */
export function FullDrawer({
  open,
  onClose,
  backLabel,
  nav,
  copyLink = true,
  barTitle,
  topActions,
  fill = false,
  push,
  children
}: FullDrawerProps) {
  const { t } = useLingui()
  const barRef = useRef<HTMLDivElement>(null)
  useDrawerNavKeys({ enabled: open, onPrev: nav?.onPrev, onNext: nav?.onNext, anchorRef: barRef })

  return (
    <Drawer
      open={open}
      onClose={onClose}
      placement="right"
      size="100%"
      closable={false}
      destroyOnHidden
      rootClassName="app-full-drawer"
      push={push}
      styles={{ body: { padding: 0 } }}
    >
      <div className={fill ? 'flex h-full flex-col' : undefined}>
        <div
          ref={barRef}
          data-drawer-bar
          className="sticky top-0 z-10 flex h-10 shrink-0 items-center justify-between gap-3 border-b border-line-soft bg-surface pr-3 pl-2"
        >
          <div className="flex min-w-0 items-center gap-1">
            <Button type="text" size="small" icon={<ArrowLeft size={14} aria-hidden="true" />} onClick={onClose}>
              {backLabel ?? t`Back`}
            </Button>
            {nav ? (
              <>
                <span aria-hidden="true" className="mx-1 h-4 w-px shrink-0 bg-line" />
                <DrawerNavButtons nav={nav} showPosition />
              </>
            ) : null}
            {barTitle ? (
              <>
                <span aria-hidden="true" className="mx-1 h-4 w-px shrink-0 bg-line" />
                <div className="flex min-w-0 items-center gap-2">{barTitle}</div>
              </>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {topActions ? <div className="mr-1 flex items-center gap-2">{topActions}</div> : null}
            {copyLink ? <CopyLinkButton /> : null}
            <CloseButton onClose={onClose} />
          </div>
        </div>
        <div data-drawer-content className={fill ? 'flex min-h-0 flex-1' : 'flex flex-col gap-3 px-5 pt-4 pb-6'}>
          {children}
        </div>
      </div>
    </Drawer>
  )
}
