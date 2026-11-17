import { Fragment, useState } from 'react'
import { Dropdown, Skeleton } from 'antd'
import type { MenuProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ChevronRight, Folder, FolderOpen, HardDrive, MoreHorizontal, Plus } from 'lucide-react'
import { RailItem, RailSection } from '../layout'
import type { FolderNode } from './storageTree'

interface FolderTreeProps {
  tree: FolderNode[]
  counts: Map<string, number>
  /** The open folder, "" for the root. */
  currentPath: string
  onOpen: (folder: string) => void
  /** The bucket is being listed: placeholder rows instead of the folders. */
  loading: boolean
  /** The listing answered: an empty tree then means a bucket without folders. */
  loaded: boolean
  /** The section's "+", which creates a folder in the open one. */
  onNewFolder: () => void
  readOnly: boolean
  /** The items of a folder's menu (Delete folder). */
  menuFor: (folder: string) => MenuProps['items']
}

function SkeletonRows() {
  return (
    <>
      {[0, 1, 2].map((key) => (
        <div key={key} data-rail-skeleton className="flex h-7 items-center px-2">
          <Skeleton.Input active size="small" block />
        </div>
      ))}
    </>
  )
}

const MENU_BUTTON =
  'grid size-5 cursor-pointer place-items-center rounded-sm border-0 bg-transparent text-ink-3 hover:bg-line-soft hover:text-ink focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none'

/**
 * The file manager's rail, laid out as the Blog and Contacts ones: the root with the bucket's file
 * count, then the folders at any depth, each with its count and menu. A level is indented under
 * its parent; the folders on the way to the open one are expanded, the others expand on their
 * chevron.
 */
export function FolderTree({
  tree,
  counts,
  currentPath,
  onOpen,
  loading,
  loaded,
  onNewFolder,
  readOnly,
  menuFor
}: FolderTreeProps) {
  const { t } = useLingui()
  // Folders the user expanded or collapsed by hand, over the default (expanded on the open path).
  const [toggled, setToggled] = useState<Record<string, boolean>>({})

  const isExpanded = (key: string) => toggled[key] ?? currentPath.startsWith(key)

  const renderLevel = (nodes: FolderNode[], depth: number) =>
    nodes.map((node) => {
      const expanded = isExpanded(node.key)
      const hasChildren = node.children.length > 0
      const active = currentPath === node.key
      const Icon = active ? FolderOpen : Folder
      // Named label, so translators read "Expand {label}" (the msgid the tree has always used).
      const label = node.name
      return (
        <Fragment key={node.key}>
          <RailItem
            leading={
              <>
                {Array.from({ length: depth }, (_, level) => (
                  <span key={level} data-folder-indent className="w-4 shrink-0" aria-hidden="true" />
                ))}
                {hasChildren ? (
                  <button
                    type="button"
                    aria-label={expanded ? t`Collapse ${label}` : t`Expand ${label}`}
                    aria-expanded={expanded}
                    onClick={() => setToggled((prev) => ({ ...prev, [node.key]: !expanded }))}
                    className="flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-sm text-ink-4 hover:text-ink focus-visible:outline-none"
                  >
                    <ChevronRight
                      size={12}
                      aria-hidden="true"
                      className={`transition-transform ${expanded ? 'rotate-90' : ''}`}
                    />
                  </button>
                ) : (
                  <span className="size-4 shrink-0" aria-hidden="true" />
                )}
              </>
            }
            icon={<Icon size={15} aria-hidden="true" />}
            label={node.name}
            count={counts.get(node.key)}
            active={active}
            onClick={() => onOpen(node.key)}
            trailingMode="actions"
            trailing={
              <Dropdown trigger={['click']} menu={{ items: menuFor(node.key) }}>
                <button
                  type="button"
                  aria-label={t`Actions for ${node.name}`}
                  className={MENU_BUTTON}
                >
                  <MoreHorizontal size={14} aria-hidden="true" />
                </button>
              </Dropdown>
            }
          />
          {expanded && hasChildren ? (
            <div role="group" className="flex flex-col gap-px">
              {renderLevel(node.children, depth + 1)}
            </div>
          ) : null}
        </Fragment>
      )
    })

  return (
    <>
      <RailSection>
        <RailItem
          icon={<HardDrive size={15} aria-hidden="true" />}
          label={t`Root`}
          count={counts.get('')}
          active={currentPath === ''}
          onClick={() => onOpen('')}
        />
      </RailSection>
      <RailSection
        title={t`Folders`}
        action={{
          label: t`New folder`,
          icon: <Plus size={14} aria-hidden="true" />,
          onClick: onNewFolder,
          primary: true,
          disabledReason: readOnly ? t`New folder (Read-only mode)` : undefined
        }}
      >
        {loading ? (
          <SkeletonRows />
        ) : loaded && tree.length === 0 ? (
          readOnly ? (
            <div className="px-2 py-1 text-sm text-ink-3">{t`No folders yet`}</div>
          ) : (
            <RailItem
              icon={<Plus size={14} aria-hidden="true" />}
              label={t`Create your first folder`}
              onClick={onNewFolder}
            />
          )
        ) : (
          renderLevel(tree, 0)
        )}
      </RailSection>
    </>
  )
}
