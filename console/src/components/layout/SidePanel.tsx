import { useRef, useState, type ReactNode } from 'react'
import { Drawer, Tabs, type TabsProps } from 'antd'
import { layout } from '../../theme/tokens'
import { CloseButton, CopyLinkButton, DrawerNavButtons, type DrawerNav } from './DrawerControls'
import { MetaLine } from './MetaLine'
import { useDrawerNavKeys } from './useDrawerNavKeys'

type TabItem = NonNullable<TabsProps['items']>[number]

/** The panel's side padding (px-4), which the tab bar repeats as a margin. */
const PANEL_GUTTER = 16

export interface SidePanelTabs {
  /** antd tab items; the `children` of the active one fill the scrolling body. */
  items: TabItem[]
  activeKey?: string
  defaultActiveKey?: string
  onChange?: (key: string) => void
}

export interface SidePanelProps {
  open: boolean
  onClose: () => void
  title: ReactNode
  /** Pills after the title. */
  status?: ReactNode
  /** Items of a metadata line under the title, separated by dots. */
  meta?: ReactNode[]
  actions?: ReactNode
  tabs?: SidePanelTabs
  nav?: DrawerNav
  /** A copy-link button (default). */
  copyLink?: boolean
  footer?: ReactNode
  /** The body, when there are no tabs or the active tab has no children. */
  children?: ReactNode
  /** Changing it starts the body afresh, scrolled to the top, while the panel itself stays (J, K). */
  bodyKey?: string
}

/** A 520px panel on the right, over a list that stays readable behind its unblurred mask. */
export function SidePanel({
  open,
  onClose,
  title,
  status,
  meta,
  actions,
  tabs,
  nav,
  copyLink = true,
  footer,
  children,
  bodyKey
}: SidePanelProps) {
  const headerRef = useRef<HTMLDivElement>(null)
  useDrawerNavKeys({ enabled: open, onPrev: nav?.onPrev, onNext: nav?.onNext, anchorRef: headerRef })

  const firstTab = tabs?.defaultActiveKey ?? tabs?.items[0]?.key
  const [ownTab, setOwnTab] = useState(firstTab)
  const activeKey = tabs?.activeKey ?? ownTab

  // A panel opened again starts on its first tab, as one opened for the first time does
  const [wasOpen, setWasOpen] = useState(open)
  if (wasOpen !== open) {
    setWasOpen(open)
    if (!open) setOwnTab(firstTab)
  }

  const scrollingBody = (content: ReactNode) => (
    <div key={bodyKey} data-sidepanel-body className="min-h-0 flex-1 overflow-y-auto px-4 py-3.5">
      {content}
    </div>
  )

  const handleTabChange = (key: string) => {
    setOwnTab(key)
    tabs?.onChange?.(key)
  }

  return (
    <Drawer
      open={open}
      onClose={onClose}
      placement="right"
      size={layout.sidepanel}
      closable={false}
      destroyOnHidden
      mask={{ blur: false }}
      styles={{ body: { padding: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' } }}
    >
      <div
        ref={headerRef}
        data-sidepanel-header
        className="flex shrink-0 items-start justify-between gap-3 px-4 pt-3 pb-2"
      >
        <div className="flex min-w-0 flex-col gap-0.5">
          <div className="flex min-w-0 items-center gap-2">
            <h2
              data-unreset
              className="m-0 truncate text-subtitle font-semibold tracking-[-0.015em] text-ink"
            >
              {title}
            </h2>
            {status ? <div className="flex shrink-0 items-center gap-2">{status}</div> : null}
          </div>
          <MetaLine items={meta} />
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {actions ? <div className="mr-1 flex items-center gap-2">{actions}</div> : null}
          {nav ? <DrawerNavButtons nav={nav} /> : null}
          {copyLink ? <CopyLinkButton /> : null}
          <CloseButton onClose={onClose} />
        </div>
      </div>
      {tabs ? (
        // The active tab's content sits in its own tabpanel, which aria-controls names; the panel
        // fills what the header leaves, and only the body inside it scrolls.
        <Tabs
          activeKey={activeKey}
          onChange={handleTabChange}
          destroyOnHidden
          styles={{
            root: { flex: 1, minHeight: 0 },
            // In line with the title and the body, the tab bar's line included
            header: { margin: `0 ${PANEL_GUTTER}px` },
            body: { height: '100%' },
            content: { height: '100%', display: 'flex', flexDirection: 'column' }
          }}
          items={tabs.items.map((item) => ({
            key: item.key,
            label: item.label,
            icon: item.icon,
            disabled: item.disabled,
            children: scrollingBody(item.children ?? children)
          }))}
        />
      ) : (
        scrollingBody(children)
      )}
      {footer ? (
        <div
          data-sidepanel-footer
          className="flex shrink-0 items-center justify-end gap-2 border-t border-line-soft px-4 py-2.5"
        >
          {footer}
        </div>
      ) : null}
    </Drawer>
  )
}
