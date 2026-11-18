import { useEffect, useRef, useState, type ReactNode } from 'react'
import { cloneDeep, get, set } from 'lodash'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPenToSquare, faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { faPlus, faSitemap } from '@fortawesome/free-solid-svg-icons'
import { Alert, Button, Dropdown, Popconfirm, Segmented, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import {
  EditingNodeLeaf,
  DimensionFilter,
  TreeNode,
  TreeNodeBranch,
  TreeNodeLeaf,
  TableSchema,
  List,
  ContactTimelineCondition,
  CustomEventsGoalCondition,
  BooleanOperator
} from '../../services/api/segment'
import { useLingui } from '@lingui/react/macro'
import { useQuery } from '@tanstack/react-query'
import { templatesApi } from '../../services/api/template'
import {
  LeafActionForm,
  LeafContactForm,
  LeafContactListForm,
  LeafCustomEventsGoalForm
} from './form_leaf'
import { ConditionIcon, type ConditionTone } from './conditionUi'
import { ConditionSentence } from './ConditionSentence'

// Defined in ./tree_completeness so pure callers can use it without importing this module,
// which pulls in antd and the whole editor. Re-exported here for the existing importers.
export { HasLeaf } from './tree_completeness'

export type SegmentSchemas = {
  [key: string]: TableSchema
}

export type TreeNodeInputProps = {
  value?: TreeNode
  onChange?: (updatedValue: TreeNode) => void
  schemas: SegmentSchemas
  lists?: List[]
  workspaceId?: string
  // Without Templates read, templates.list and templates.get answer 403: an email condition then
  // names its template by ID, and its template field is locked. Defaults to true.
  canReadTemplates?: boolean
  customFieldLabels?: Record<string, string>
  // Reports the tree as it would stand if the condition currently open in the form were confirmed,
  // and undefined once no condition is open. Lets the drawer preview an in-progress condition.
  onDraftTreeChange?: (draftTree: TreeNode | undefined, countable?: boolean) => void
  // Told whenever a condition opens in its editor or closes, so a caller can refuse to save, or to
  // close, while one is half-written.
  onEditingChange?: (open: boolean) => void
  // Shown above the add buttons while the root group has no condition yet.
  emptyState?: ReactNode
}

// What each condition type is, in the add menu and above each saved condition.
const SOURCE_TITLES: Record<string, MessageDescriptor> = {
  contacts: msg`Contact property`,
  contact_lists: msg`List subscription`,
  contact_timeline: msg`Activity`,
  custom_events_goals: msg`Goal`
}

const SOURCE_DESCRIPTIONS: Record<string, MessageDescriptor> = {
  contacts: msg`Email, country, language, custom fields, tracking consent…`,
  contact_lists: msg`Is or is not in a list, with a status`,
  contact_timeline: msg`Emails sent, opened, clicked… pages viewed and website visits`,
  custom_events_goals: msg`Purchases, sign-ups, leads… count them or add up their value`
}

// The rail and the AND / OR pill of each nesting level, written whole for Tailwind.
const DEPTH_STYLES = [
  { rail: 'border-primary-300', pill: 'border-primary-300 text-primary-700' },
  { rail: 'border-info-fg/30', pill: 'border-info-fg/30 text-info-fg' },
  { rail: 'border-success-fg/30', pill: 'border-success-fg/30 text-success-fg' },
  { rail: 'border-warning-fg/30', pill: 'border-warning-fg/30 text-warning-fg' }
]

const depthStyle = (depth: number) => DEPTH_STYLES[depth % DEPTH_STYLES.length]

export const TreeNodeInput = (props: TreeNodeInputProps) => {
  const { t, i18n } = useLingui()
  const [editingNodeLeaf, setEditingNodeLeaf] = useState<EditingNodeLeaf | undefined>(undefined)
  // The group whose add menu is open. Controlled: the button turns disabled as soon as a type is
  // picked, and a disabled Dropdown left to itself stayed open over the editor.
  const [openAddMenu, setOpenAddMenu] = useState<string | undefined>(undefined)

  const onDraftTreeChangeRef = useRef(props.onDraftTreeChange)
  onDraftTreeChangeRef.current = props.onDraftTreeChange
  const onEditingChangeRef = useRef(props.onEditingChange)
  onEditingChangeRef.current = props.onEditingChange

  // A draft only ever describes the condition open in the form. Whenever that changes — opened,
  // confirmed, cancelled, or swapped for another condition — the committed tree takes over again.
  useEffect(() => {
    onDraftTreeChangeRef.current?.(undefined)
    onEditingChangeRef.current?.(!!editingNodeLeaf)
  }, [editingNodeLeaf])

  // Where the keyboard goes back to once the open condition closes: its pen button, or the add
  // button of its group when it is gone (a new condition cancelled, a condition deleted).
  const rootRef = useRef<HTMLDivElement>(null)
  const lastOpenedRef = useRef<{ path: string; key: number } | undefined>(undefined)
  useEffect(() => {
    if (editingNodeLeaf) {
      lastOpenedRef.current = { path: editingNodeLeaf.path, key: editingNodeLeaf.key }
      return
    }
    const last = lastOpenedRef.current
    lastOpenedRef.current = undefined
    const root = rootRef.current
    if (!last || !root) return
    // Only when focus went down with the editor that just closed: never steal it from elsewhere.
    const active = document.activeElement
    if (active && active !== document.body) return
    const target =
      root.querySelector<HTMLElement>(`[data-edit-condition="${last.path}[${last.key}]"]`) ??
      root.querySelector<HTMLElement>(`[data-add-condition="${last.path}"]`)
    target?.focus()
  }, [editingNodeLeaf])

  const onDraftLeafChange = (draftLeaf: TreeNode, path: string, pathKey: number, countable: boolean) => {
    if (!props.onDraftTreeChange || !props.value) return

    const clonedTree = cloneDeep(props.value) as TreeNode
    set(clonedTree, path + '[' + pathKey + ']', draftLeaf)

    props.onDraftTreeChange(clonedTree, countable)
  }

  const { data: templatesData } = useQuery({
    queryKey: ['templates', props.workspaceId],
    queryFn: () => templatesApi.list({ workspace_id: props.workspaceId!, channel: 'email' }),
    enabled: !!props.workspaceId && props.canReadTemplates !== false
  })
  const templateName = (id: string) => templatesData?.templates?.find((tpl) => tpl.id === id)?.name

  const isEditing = !!editingNodeLeaf

  // Removes the condition at path[key] from a tree, in place.
  const removeAt = (tree: TreeNode, path: string, key: number) => {
    const target = get(tree, path)
    if (target && target.length) {
      set(
        tree,
        path,
        target.filter((_x: TreeNode, i: number) => i !== key)
      )
    }
  }

  // Cancel of the open condition: a new one goes, an existing one keeps its saved version. Also the
  // delete of a condition or a group, which is only offered while nothing is open.
  const cancelOrDeleteNode = (path: string, key: number) => {
    const clonedTree = cloneDeep(props.value) as TreeNode

    // cancel if edit, and is not new
    if (editingNodeLeaf && !editingNodeLeaf.is_new) {
      setEditingNodeLeaf(undefined)
      props.onChange?.(clonedTree)
      return
    }

    // condition is new and not yet confirmed, or a delete: it leaves the tree
    removeAt(clonedTree, path, key)

    if (editingNodeLeaf && editingNodeLeaf.path === path && editingNodeLeaf.key === key) {
      setEditingNodeLeaf(undefined)
    }

    props.onChange?.(clonedTree)
  }

  const addGroup = (path: string, key: number) => {
    if (!props.value) return
    const clonedTree = cloneDeep(props.value) as TreeNode
    if (!clonedTree.branch) return

    const node: TreeNode = {
      kind: 'branch',
      branch: { operator: 'and', leaves: [] } as TreeNodeBranch
    }

    if (path === '') {
      clonedTree.branch.leaves.push(node)
    } else {
      get(clonedTree, path + '[' + key + ']').branch.leaves.push(node)
    }

    props.onChange?.(clonedTree)
  }

  const addCondition = (path: string, key: number, source: string) => {
    if (!props.value) return

    const clonedTree = cloneDeep(props.value) as TreeNode
    if (!clonedTree.branch) return

    const setPath = path + '[' + key + ']'
    const leaf = { source } as TreeNodeLeaf

    // The values each condition type starts with
    if (source === 'contact_lists') {
      leaf.contact_list = { operator: 'in', list_id: '', status: undefined }
    } else if (source === 'contacts') {
      leaf.contact = { filters: [] as DimensionFilter[] }
    } else if (source === 'custom_events_goals') {
      leaf.custom_events_goal = {
        goal_type: '*', // All goal types by default
        negate: false, // "has", not "has not"
        aggregate_operator: 'count',
        operator: 'gte',
        value: 1, // At least 1 event makes sense as default
        timeframe_operator: 'anytime',
        timeframe_values: []
      } as CustomEventsGoalCondition
    } else {
      leaf.contact_timeline = {
        kind: '',
        timeframe_operator: 'anytime',
        timeframe_values: [],
        count_operator: 'at_least',
        count_value: 1,
        filters: [] as DimensionFilter[]
      } as ContactTimelineCondition
    }

    const node: TreeNode = { kind: 'leaf', leaf }
    let editingNodeKey = 0

    if (path === '') {
      clonedTree.branch.leaves.push(node)
      editingNodeKey = clonedTree.branch.leaves.length - 1
    } else {
      const target = get(clonedTree, setPath)
      target.branch.leaves.push(node)
      editingNodeKey = target.branch.leaves.length - 1
    }

    setEditingNodeLeaf(
      Object.assign(
        {
          path: path === '' ? 'branch.leaves' : setPath + '.branch.leaves',
          key: editingNodeKey,
          is_new: true
        },
        node as object
      ) as EditingNodeLeaf
    )

    props.onChange?.(clonedTree)
  }

  // Opens a saved condition. A new condition still open is dropped first rather than left behind as
  // an empty leaf; it was appended last to its group, so the indexes of the others do not move.
  const editNode = (path: string, key: number) => {
    if (!props.value) return
    if (editingNodeLeaf && editingNodeLeaf.path === path && editingNodeLeaf.key === key) return

    let tree = props.value
    if (editingNodeLeaf?.is_new) {
      tree = cloneDeep(props.value) as TreeNode
      removeAt(tree, editingNodeLeaf.path, editingNodeLeaf.key)
      props.onChange?.(tree)
    }

    const condition = get(tree, path + '[' + key + ']')
    setEditingNodeLeaf(Object.assign({ path, key }, condition) as EditingNodeLeaf)
  }

  const changeBranchOperator = (path: string, pathKey: number, value: BooleanOperator) => {
    const clonedTree = cloneDeep(props.value) as TreeNode
    if (!clonedTree.branch) return

    if (path === '') {
      clonedTree.branch.operator = value
    } else {
      set(clonedTree, path + '[' + pathKey + '].branch.operator', value)
    }

    props.onChange?.(clonedTree)
  }

  const onUpdateNode = (updatedNode: TreeNode, path: string, pathKey: number) => {
    const clonedTree = cloneDeep(props.value) as TreeNode
    set(clonedTree, path + '[' + pathKey + ']', updatedNode)
    props.onChange?.(clonedTree)
  }

  const deleteButton = (path: string, pathKey: number, isBranch: boolean) => (
    // Stops the clicks of the confirmation too, so they never reach a clickable row around it
    <span onClick={(event) => event.stopPropagation()}>
    <Popconfirm
      placement="left"
      title={isBranch ? t`Do you really want to remove this group?` : t`Do you really want to remove this condition?`}
      onConfirm={() => {
        cancelOrDeleteNode(path, pathKey)
        // The button that was focused is gone with its row: the group's add button takes over.
        requestAnimationFrame(() =>
          rootRef.current?.querySelector<HTMLElement>(`[data-add-condition="${path}"]`)?.focus()
        )
      }}
      okText={t`Delete`}
      okButtonProps={{ danger: true }}
      cancelText={t`Cancel`}
    >
      <Tooltip title={isBranch ? t`Remove this group` : t`Remove this condition`}>
        <Button
          type="text"
          size="small"
          aria-label={isBranch ? t`Remove this group` : t`Remove this condition`}
          icon={<FontAwesomeIcon icon={faTrashCan} />}
        />
      </Tooltip>
    </Popconfirm>
    </span>
  )

  const renderLeaf = (node: TreeNode, path: string, pathKey: number) => {
    const isEditingCurrent = !!editingNodeLeaf && editingNodeLeaf.path === path && editingNodeLeaf.key === pathKey
    const source = node.leaf?.source as string
    const schema = props.schemas[source]

    if (!schema) {
      return (
        <div className="flex items-start gap-3 rounded-lg border border-line bg-surface p-3">
          <ConditionIcon tone="group" />
          <Alert style={{ flex: 1 }} type="error" title={t`source ${source} not found`} />
          {!isEditing && deleteButton(path, pathKey, false)}
        </div>
      )
    }

    if (isEditingCurrent && editingNodeLeaf) {
      const formProps = {
        value: node,
        onChange: (updatedLeaf: TreeNode) => onUpdateNode(updatedLeaf, path, pathKey),
        onDraftChange: props.onDraftTreeChange
          ? (draftLeaf: TreeNode, countable: boolean) => onDraftLeafChange(draftLeaf, path, pathKey, countable)
          : undefined,
        source,
        schema,
        editingNodeLeaf: editingNodeLeaf as EditingNodeLeaf,
        setEditingNodeLeaf,
        cancelOrDeleteNode: () => cancelOrDeleteNode(path, pathKey),
        customFieldLabels: props.customFieldLabels
      }

      if (source === 'contacts') return <LeafContactForm {...formProps} />
      if (source === 'contact_lists') return <LeafContactListForm {...formProps} lists={props.lists || []} />
      if (source === 'custom_events_goals') return <LeafCustomEventsGoalForm {...formProps} />
      return (
        <LeafActionForm {...formProps} workspaceId={props.workspaceId} canReadTemplates={props.canReadTemplates} />
      )
    }

    const title = SOURCE_TITLES[source]
    return (
      // A mouse convenience: the pen button is how a keyboard (or a screen reader, which reads the
      // sentence as it is) opens the condition.
      <div
        className="group flex cursor-pointer items-start gap-3 rounded-lg border border-line bg-surface px-3 py-2.5 hover:border-primary-300"
        // Only clicks on the row itself: the delete confirmation renders in a portal, and React
        // bubbles its Delete and Cancel through here, which opened the row on a stale tree.
        onClick={(event) => {
          if (!event.currentTarget.contains(event.target as Node)) return
          editNode(path, pathKey)
        }}
      >
        <ConditionIcon tone={source as ConditionTone} icon={schema.icon} />
        <div className="min-w-0 flex-1">
          <div className="text-2xs text-ink-3">{title ? i18n._(title) : schema.title}</div>
          <ConditionSentence
            leaf={node.leaf as TreeNodeLeaf}
            schemas={props.schemas}
            lists={props.lists}
            workspaceId={props.workspaceId}
            customFieldLabels={props.customFieldLabels}
            templateName={templateName}
          />
        </div>
        <div className="flex shrink-0 gap-0.5">
          <Tooltip title={t`Edit`}>
            <Button
              type="text"
              size="small"
              aria-label={t`Edit`}
              data-edit-condition={`${path}[${pathKey}]`}
              icon={<FontAwesomeIcon icon={faPenToSquare} />}
              onClick={(event) => {
                event.stopPropagation()
                editNode(path, pathKey)
              }}
            />
          </Tooltip>
          {!isEditing && deleteButton(path, pathKey, false)}
        </div>
      </div>
    )
  }

  const addMenu = (path: string, pathKey: number): MenuProps => ({
    items: Object.keys(props.schemas).map((source) => {
      const schema = props.schemas[source]
      const title = SOURCE_TITLES[source]
      const description = SOURCE_DESCRIPTIONS[source]
      return {
        key: source,
        label: (
          <span className="flex max-w-80 items-start gap-2.5 py-1">
            <ConditionIcon tone={source as ConditionTone} icon={schema.icon} />
            <span>
              <span className="block font-semibold text-ink">{title ? i18n._(title) : schema.title}</span>
              <span className="block text-xs whitespace-normal text-ink-3">
                {description ? i18n._(description) : schema.description}
              </span>
            </span>
          </span>
        )
      }
    }),
    onClick: ({ key }) => {
      setOpenAddMenu(undefined)
      addCondition(path, pathKey, key)
    }
  })

  const renderBranch = (node: TreeNode, path: string, pathKey: number, depth: number) => {
    if (!node.branch) return <span>{t`A branch condition is required...`}</span>

    const conditionPath = path === '' ? 'branch.leaves' : path + '[' + pathKey + '].branch.leaves'
    const isRoot = path === ''
    const style = depthStyle(depth)
    const operatorLabel = node.branch.operator === 'or' ? t`OR` : t`AND`

    const operatorControl = (
      <Segmented<BooleanOperator>
        size="small"
        value={node.branch.operator}
        onChange={(value) => changeBranchOperator(path, pathKey, value)}
        options={[
          { value: 'and', label: t`all` },
          { value: 'or', label: t`at least one` }
        ]}
      />
    )

    return (
      <div
        className={
          isRoot
            ? 'flex flex-col'
            : `flex flex-col rounded-r-lg border-l-2 bg-subtle py-2.5 pr-3 pl-4 ${style.rail}`
        }
      >
        <div className="mb-2.5 flex flex-wrap items-center gap-2 text-ink-2">
          {isRoot ? (
            <>
              <span>{t`Contacts matching`}</span>
              {operatorControl}
              <span>{t`of the following conditions`}</span>
            </>
          ) : (
            <>
              <FontAwesomeIcon icon={faSitemap} className="text-ink-3" />
              <span>{t`Group:`}</span>
              {operatorControl}
              <span>{t`of the conditions`}</span>
              <span className="ml-auto">{!isEditing && deleteButton(path, pathKey, true)}</span>
            </>
          )}
        </div>

        <div className={isRoot ? `border-l-2 pl-4 ${style.rail}` : undefined}>
          {node.branch.leaves.length === 0 && !isRoot && (
            <div className="mb-2 text-xs text-ink-3">{t`Empty: add a condition or a subgroup.`}</div>
          )}
          {node.branch.leaves.length === 0 && isRoot && props.emptyState}

          {node.branch.leaves.map((leaf: TreeNode, i: number) => (
            <div key={i}>
              {i > 0 && (
                <div className="-ml-7 my-1.5 flex">
                  <span
                    className={`rounded-full border bg-surface px-1.5 text-overline font-semibold tracking-wide ${style.pill}`}
                  >
                    {operatorLabel}
                  </span>
                </div>
              )}
              {leaf.leaf && renderLeaf(leaf, conditionPath, i)}
              {leaf.branch && renderBranch(leaf, conditionPath, i, depth + 1)}
            </div>
          ))}

          <div className="mt-2.5 flex gap-2">
            <Dropdown
              menu={addMenu(path, pathKey)}
              trigger={['click']}
              disabled={isEditing}
              open={!isEditing && openAddMenu === conditionPath}
              onOpenChange={(open) => setOpenAddMenu(open ? conditionPath : undefined)}
            >
              <Button
                size="small"
                type="primary"
                ghost={node.branch.leaves.length > 0}
                disabled={isEditing}
                data-add-condition={conditionPath}
                icon={<FontAwesomeIcon icon={faPlus} />}
              >
                {t`Condition`}
              </Button>
            </Dropdown>
            <Button
              size="small"
              type="link"
              disabled={isEditing}
              icon={<FontAwesomeIcon icon={faSitemap} />}
              onClick={() => addGroup(path, pathKey)}
            >
              {t`Group`}
            </Button>
          </div>
        </div>
      </div>
    )
  }

  if (!props.value) {
    return <span>{t`A value is required...`}</span>
  }

  return (
    <div ref={rootRef} className="pt-2">
      {renderBranch(props.value, '', 0, 0)}
    </div>
  )
}
