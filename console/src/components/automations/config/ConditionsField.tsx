import React, { useState } from 'react'
import { Button, Drawer, Popconfirm, Space } from 'antd'
import { PlusOutlined, EditOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { Plural } from '@lingui/react/macro'
import { TreeNodeInput } from '../../segment/input'
import { TableSchemas } from '../../segment/table_schemas'
import { HasLeaf, pruneIncompleteConditions } from '../../segment/tree_completeness'
import { useAutomation } from '../context'
import type { TreeNode } from '../../../services/api/segment'

// Shown to TreeNodeInput while the editor is open and still empty. It is never handed to the
// caller: a branch with zero leaves is not "no conditions" to the server, it is a payload it
// rejects, so an untouched editor must leave the config alone.
const EMPTY_TREE: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: []
  }
}

const countLeaves = (node: TreeNode | undefined): number => {
  if (!node) return 0
  if (node.kind === 'leaf') return node.leaf ? 1 : 0
  return (node.branch?.leaves ?? []).reduce((total, child) => total + countLeaves(child), 0)
}

interface ConditionsFieldProps {
  /** Names the field in the panel and titles the drawer. */
  title: string
  /** Shown under the summary in the panel. */
  description: string
  /** Wording of the button that opens an editor with nothing in it yet. */
  addLabel: string
  value?: TreeNode
  onChange: (conditions: TreeNode) => void
  onClear: () => void
  /** Guidance shown above the editor, inside the drawer. */
  notice?: React.ReactNode
}

/**
 * A condition tree edited in a drawer rather than inline.
 *
 * The tree editor is the same component segments use, and segments give it a drawer at 90% of
 * the viewport for a reason: a nested AND/OR tree, each leaf carrying a source, a field, an
 * operator and a value, does not fit a canvas side panel. Inlining it forced the panel to 640px
 * for the Filter node and still left the editor cramped, while making the panel's own fields
 * harder to scan.
 *
 * Edits write through as they happen — there is no draft and no Cancel, because the canvas
 * already snapshots whole nodes for undo/redo and every other field in the panel behaves the
 * same way. A drawer promises nothing more than that; a modal would, which is why this is not
 * a modal.
 */
export const ConditionsField: React.FC<ConditionsFieldProps> = ({
  title,
  description,
  addLabel,
  value,
  onChange,
  onClear,
  notice
}) => {
  const { t } = useLingui()
  const { lists, workspace } = useAutomation()
  const [open, setOpen] = useState(false)

  const configured = HasLeaf(value)
  const leafCount = countLeaves(value)

  // Closing is the commit point. TreeNodeInput hands a leaf back the moment a source is
  // picked, before any filter exists, so a user who opens the editor and changes their mind
  // would otherwise leave behind a condition the server refuses — blocking every later save
  // of the automation, with nothing on screen to explain why.
  const close = () => {
    const pruned = pruneIncompleteConditions(value)

    if (!pruned) {
      if (HasLeaf(value)) onClear()
    } else if (pruned !== value) {
      onChange(pruned)
    }

    setOpen(false)
  }

  return (
    <>
      {!configured && (
        <Button type="link" size="small" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
          {addLabel}
        </Button>
      )}

      {configured && (
        <div className="border border-gray-200 rounded-md p-2">
          <div className="text-sm font-medium text-gray-700">{title}</div>
          <div className="text-xs text-gray-500 mt-0.5">
            {/* A count rather than a rendering of the tree: a partial paraphrase that quietly
                dropped a nested branch would be worse than no paraphrase at all. */}
            <Plural value={leafCount} one="# condition" other="# conditions" />
          </div>
          <div className="text-xs text-gray-400 mt-1">{description}</div>
          <Space size="small" className="mt-2">
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => setOpen(true)}>
              {t`Edit`}
            </Button>
            {/* Confirmed, because one click here discards a whole tree that took real work to
                build, and the summary shows only a count — there is nothing on screen to make the
                loss obvious afterwards. A generic title rather than the field's own `title` prop:
                lowercasing a translated noun to splice into a sentence breaks in languages that
                capitalise them. */}
            <Popconfirm
              title={t`Remove conditions`}
              description={t`The whole condition tree will be deleted.`}
              onConfirm={onClear}
              okText={t`Remove`}
              cancelText={t`Cancel`}
              okButtonProps={{ danger: true }}
            >
              <Button type="link" size="small" danger>
                {t`Remove`}
              </Button>
            </Popconfirm>
          </Space>
        </div>
      )}

      <Drawer
        title={title}
        open={open}
        size={960}
        onClose={close}
        destroyOnHidden
        extra={<Button type="primary" onClick={close}>{t`Done`}</Button>}
      >
        {notice}
        <TreeNodeInput
          value={value ?? EMPTY_TREE}
          onChange={onChange}
          schemas={TableSchemas}
          lists={lists}
          workspaceId={workspace.id}
        />
      </Drawer>
    </>
  )
}
