import { useState } from 'react'
import { Dropdown, Tooltip, type MenuProps } from 'antd'
import { ChevronsUpDown, Plus } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { Workspace } from '../../services/api/types'
import { layout } from '../../theme/tokens'

const NEW_WORKSPACE_KEY = '__new_workspace__'

/** The workspace's logo, or its initial on the brand gradient. `large` is the 40px of the settings. */
export function WorkspaceBadge({
  workspace,
  small,
  large
}: {
  workspace?: Pick<Workspace, 'name'> & { settings?: Pick<Workspace['settings'], 'logo_url'> }
  small?: boolean
  large?: boolean
}) {
  const size = small
    ? 'size-4.5 rounded-sm text-overline'
    : large
      ? 'size-10 rounded-md text-base'
      : 'size-6 rounded-md text-xs'
  const logo = workspace?.settings?.logo_url
  if (logo) {
    return (
      <img
        src={logo}
        alt=""
        data-workspace-logo
        className={`${size} shrink-0 border border-line bg-surface object-contain`}
      />
    )
  }
  return (
    <span
      aria-hidden="true"
      data-workspace-badge
      className={`${size} grid shrink-0 place-items-center bg-linear-135 from-primary-500 to-brand-pink font-semibold text-white`}
    >
      {Array.from(workspace?.name ?? '?')[0]?.toUpperCase()}
    </span>
  )
}

export interface WorkspaceSwitcherProps {
  workspaces: Workspace[]
  workspaceId: string
  /** Only the root user creates workspaces. */
  isRoot: boolean
  collapsed: boolean
  onSelect: (workspaceId: string) => void
  onCreate: () => void
  getPopupContainer: () => HTMLElement
}

/** The current workspace, and the menu that switches to another one. */
export function WorkspaceSwitcher({
  workspaces,
  workspaceId,
  isRoot,
  collapsed,
  onSelect,
  onCreate,
  getPopupContainer
}: WorkspaceSwitcherProps) {
  const { t } = useLingui()
  const [menuOpen, setMenuOpen] = useState(false)
  const current = workspaces.find((workspace) => workspace.id === workspaceId)

  const items: MenuProps['items'] = [
    ...workspaces.map((workspace) => ({
      key: workspace.id,
      label: (
        <span className="flex min-w-0 items-center gap-2">
          <WorkspaceBadge workspace={workspace} small />
          <span className="truncate">{workspace.name}</span>
        </span>
      )
    })),
    // Never greyed by the licence. The workspace ceiling is a quota, not a capability: how many a
    // deployment already holds decides whether the next one is refused, and the server answers
    // that with a 402 naming the number. Guessing it here would grey the control out on a
    // deployment that has room.
    ...(isRoot
      ? [
          { type: 'divider' as const },
          {
            key: NEW_WORKSPACE_KEY,
            icon: <Plus size={14} aria-hidden="true" />,
            label: t`New workspace`
          }
        ]
      : [])
  ]

  const button = (
    <button
      type="button"
      aria-label={collapsed ? current?.name : undefined}
      className={
        collapsed
          ? 'mx-auto grid size-9 cursor-pointer place-items-center rounded-md border-0 bg-transparent hover:bg-fill focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none'
          : 'flex h-9 w-full cursor-pointer items-center gap-2 rounded-md border border-line bg-surface pr-2 pl-1.5 text-left hover:bg-fill focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none'
      }
    >
      <WorkspaceBadge workspace={current} />
      {collapsed ? null : (
        <>
          <span className="min-w-0 flex-1 truncate text-sm font-semibold text-ink">{current?.name}</span>
          <ChevronsUpDown size={14} aria-hidden="true" className="shrink-0 text-ink-3" />
        </>
      )}
    </button>
  )

  return (
    <Dropdown
      trigger={['click']}
      onOpenChange={setMenuOpen}
      placement={collapsed ? 'rightTop' : 'bottomLeft'}
      getPopupContainer={getPopupContainer}
      styles={{ root: { minWidth: layout.sidebar - 20 } }}
      menu={{
        items,
        selectable: true,
        selectedKeys: [workspaceId],
        style: { maxHeight: 360, overflowY: 'auto' },
        onClick: ({ key }) => (key === NEW_WORKSPACE_KEY ? onCreate() : onSelect(key))
      }}
    >
      {collapsed ? (
        <Tooltip
          title={current?.name}
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
