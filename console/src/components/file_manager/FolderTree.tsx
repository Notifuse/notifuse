import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { ChevronRight, Folder, FolderOpen } from 'lucide-react'
import { useNumberFormat } from '../layout'
import type { FolderNode } from './storageTree'

interface FolderTreeProps {
  tree: FolderNode[]
  counts: Map<string, number>
  /** The open folder, "" for the root. */
  currentPath: string
  onOpen: (folder: string) => void
}

interface RowProps {
  label: string
  count?: number
  active: boolean
  depth: number
  expanded?: boolean
  onToggle?: () => void
  onOpen: () => void
}

function FolderRow({ label, count, active, depth, expanded, onToggle, onOpen }: RowProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const Icon = active ? FolderOpen : Folder
  return (
    <div
      data-folder-row
      data-depth={depth}
      className={`group flex h-7 items-center gap-1 rounded-md pr-2 text-base focus-within:ring-2 focus-within:ring-primary-500/18 ${
        active ? 'bg-primary-100' : 'hover:bg-fill'
      }`}
    >
      {onToggle ? (
        <button
          type="button"
          aria-label={expanded ? t`Collapse ${label}` : t`Expand ${label}`}
          aria-expanded={expanded}
          onClick={onToggle}
          className="flex size-4 shrink-0 items-center justify-center rounded-sm text-ink-4 hover:text-ink focus-visible:outline-none"
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
      <button
        type="button"
        onClick={onOpen}
        aria-current={active ? 'page' : undefined}
        className={`flex min-w-0 flex-1 items-center gap-2 self-stretch text-left focus-visible:outline-none ${
          active ? 'font-medium text-primary-700' : 'text-ink-2 hover:text-ink'
        }`}
      >
        <span
          aria-hidden="true"
          className={`flex shrink-0 items-center ${active ? 'text-primary-500' : 'text-ink-3'}`}
        >
          <Icon size={15} />
        </span>
        <span className="min-w-0 flex-1 truncate">{label}</span>
        {count !== undefined ? (
          <span
            className={`shrink-0 text-2xs tabular-nums ${active ? 'text-primary-600' : 'text-ink-3'}`}
          >
            {int(count)}
          </span>
        ) : null}
      </button>
    </div>
  )
}

/**
 * The folders of the bucket, at any depth, each indented 12px under its parent. The folders on
 * the way to the open one are expanded; the others expand on their chevron.
 */
export function FolderTree({ tree, counts, currentPath, onOpen }: FolderTreeProps) {
  const { t } = useLingui()
  // Folders the user expanded or collapsed by hand, over the default (expanded on the open path).
  const [toggled, setToggled] = useState<Record<string, boolean>>({})

  const isExpanded = (key: string) => toggled[key] ?? currentPath.startsWith(key)

  const renderLevel = (nodes: FolderNode[], depth: number) => (
    <div role="group" className={depth > 0 ? 'flex flex-col gap-px pl-3' : 'flex flex-col gap-px'}>
      {nodes.map((node) => {
        const expanded = isExpanded(node.key)
        return (
          <div key={node.key} className="flex flex-col gap-px">
            <FolderRow
              label={node.name}
              count={counts.get(node.key)}
              active={currentPath === node.key}
              depth={depth}
              expanded={expanded}
              onToggle={
                node.children.length > 0
                  ? () => setToggled((prev) => ({ ...prev, [node.key]: !expanded }))
                  : undefined
              }
              onOpen={() => onOpen(node.key)}
            />
            {expanded && node.children.length > 0 ? renderLevel(node.children, depth + 1) : null}
          </div>
        )
      })}
    </div>
  )

  return (
    <div className="flex flex-col gap-px">
      <FolderRow
        label={t`Root`}
        count={counts.get('')}
        active={currentPath === ''}
        depth={0}
        onOpen={() => onOpen('')}
      />
      {renderLevel(tree, 1)}
    </div>
  )
}
