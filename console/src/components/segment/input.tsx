import { useMemo, useState } from 'react'
import { cloneDeep, forEach, get, set } from 'lodash'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPenToSquare, faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { Button, Select, Tag, Alert, Popconfirm, Cascader, Space, Popover, Flex } from 'antd'
import {
  EditingNodeLeaf,
  FieldTypeRendererDictionary,
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
import type { CascaderProps } from 'antd'
import dayjs from 'dayjs'
import { useLingui } from '@lingui/react/macro'

// Format date for display (converts ISO8601 to readable format)
const formatDateDisplay = (dateStr: string | undefined): string => {
  if (!dateStr) return ''
  return dayjs(dateStr).format('YYYY-MM-DD HH:mm:ss')
}

type CascaderOption = NonNullable<CascaderProps['options']>[number]
import { FieldTypeString } from './type_string'
import { FieldTypeTime } from './type_time'
import {
  LeafActionForm,
  LeafContactForm,
  LeafContactListForm,
  LeafCustomEventsGoalForm
} from './form_leaf'
import { FieldTypeNumber } from './type_number'
import { FieldTypeJSON } from './type_json'
import styles from './input.module.css'

export const HasLeaf = (node: TreeNode): boolean => {
  if (node.kind === 'leaf') return true
  if (!node.branch) return false

  return node.branch.leaves.some((child: TreeNode) => {
    return HasLeaf(child)
  })
}

export type SegmentSchemas = {
  [key: string]: TableSchema
}

export type TreeNodeInputProps = {
  value?: TreeNode
  onChange?: (updatedValue: TreeNode) => void
  schemas: SegmentSchemas
  lists?: List[]
  customFieldLabels?: Record<string, string>
}

const fieldTypeRendererDictionary: FieldTypeRendererDictionary = {
  string: new FieldTypeString(),
  time: new FieldTypeTime(),
  number: new FieldTypeNumber(),
  json: new FieldTypeJSON()
}

// Helper function to get color class name
const getColorClass = (colorID: number): string => {
  return styles[`color${colorID}`] || ''
}

// const typeIcon = css({
//   width: '25px',
//   textAlign: 'center',
//   display: 'inline-block',
//   marginRight: '1rem',
//   fontSize: '9px',
//   lineHeight: '23px',
//   borderRadius: '3px',
//   backgroundColor: '#eee',
//   color: '#666'
// })

export const TreeNodeInput = (props: TreeNodeInputProps) => {
  const { t } = useLingui()
  const [editingNodeLeaf, setEditingNodeLeaf] = useState<EditingNodeLeaf | undefined>(undefined)

  const cascaderOptions = useMemo(() => {
    const options: CascaderOption[] = [
      {
        value: 'and',
        label: (
          <span>
            <span
              style={{
                display: 'inline-block',
                width: 18,
                marginRight: 8,
                textAlign: 'center',
                fontWeight: 600
              }}
            >
              L
            </span>
            {t`AND | OR`}
          </span>
        )
      } // AND by default, user can switch to OR after
    ]

    forEach(props.schemas, (schema: TableSchema, tableName: string) => {
      options.push({
        value: tableName,
        label: (
          <span>
            {schema.icon && (
              <FontAwesomeIcon icon={schema.icon} style={{ width: 18, marginRight: 8 }} />
            )}
            {schema.title}
          </span>
        )
      })
    })

    // forEach(props.schemas, (schema: CubeSchema, tableName: string) => {
    //   const measures: any[] = []

    //   // forEach(schema.measures, (measure, key) => {
    //   //   if (measure.shown === false || measure.meta?.hide_from_segmentation === true) {
    //   //     return
    //   //   }

    //   //   // consider count/count_distinct/sum/avg/max... as number type
    //   //   const type = ['string', 'time', 'number'].includes(measure.type) ? measure.type : 'number'

    //   //   measures.push({
    //   //     value: key,
    //   //     label: (
    //   //       <Tooltip title={measure.description}>
    //   //         <span className={typeIcon}>123</span>
    //   //         {measure.title}
    //   //       </Tooltip>
    //   //     ),
    //   //     type: type
    //   //   })
    //   // })

    //   const dimensions: any[] = []

    //   forEach(schema.dimensions, (dimension, key) => {
    //     if (dimension.shown === false || dimension.meta?.hide_from_segmentation === true) {
    //       return
    //     }

    //     let icon = <span className={typeIcon}>123</span>

    //     switch (dimension.type) {
    //       case 'string':
    //         icon = <span className={typeIcon}>Abc</span>
    //         break
    //       case 'number':
    //         if (key.indexOf('is_') !== -1) {
    //           icon = <span className={typeIcon}>0/1</span>
    //         }
    //         break
    //       case 'time':
    //         icon = (
    //           <span className={typeIcon}>
    //             <FontAwesomeIcon icon={faCalendar} />
    //           </span>
    //         )
    //         break
    //       default:
    //     }

    //     dimensions.push({
    //       value: key,
    //       label: (
    //         <Tooltip title={dimension.description}>
    //           {icon} {dimension.title}
    //         </Tooltip>
    //       ),
    //       type: dimension.type
    //     })
    //   })

    //   options.push({
    //     value: tableName,
    //     label: <TableTag table={tableName} />,
    //     children: [...measures, ...dimensions]
    //   })
    // })

    return options
  }, [props.schemas])

  // borderColor incrementer
  let currentColorID = 0
  const getColorID = () => {
    currentColorID++
    return currentColorID
  }

  const cancelOrDeleteNode = (path: string, key: number) => {
    // console.log('path', path);
    // console.log('key', key);
    const clonedTree = cloneDeep(props.value) as TreeNode

    // cancel if edit, and is not new
    if (editingNodeLeaf && !editingNodeLeaf.is_new) {
      setEditingNodeLeaf(undefined)
      props.onChange?.(clonedTree)
      return
    }

    // condition is new, and not yet confirmed, we remove it from the tree
    const target = get(clonedTree, path)

    if (target && target.length) {
      set(
        clonedTree,
        path,
        target.filter((_x: TreeNode, i: number) => i !== key)
      )
    }

    // reset possible edit mode on current field
    if (editingNodeLeaf && editingNodeLeaf.path === path && editingNodeLeaf.key === key) {
      setEditingNodeLeaf(undefined)
    }

    if (props.onChange) props.onChange(clonedTree)
  }

  const addTreeNode = (path: string, key: number, values: (string | number | null)[], selectedOptions: (CascaderOption | undefined)[]) => {
    // console.log('values', values);
    // console.log('selectedOptions', selectedOptions);
    // console.log('path', path);
    // console.log('key', key);
    if (!props.value) return

    const clonedTree = cloneDeep(props.value) as TreeNode
    if (!clonedTree.branch) return

    const setPath = path + '[' + key + ']'

    // Add branch
    if (values[0] === 'and') {
      const node: TreeNode = {
        kind: 'branch',
        branch: {
          operator: 'and',
          leaves: []
        } as TreeNodeBranch
      }

      // node path, if non root
      if (path === '') {
        clonedTree.branch.leaves.push(node)
      } else {
        const target = get(clonedTree, setPath)
        target.branch.leaves.push(node)
      }

      // console.log('tree is', JSON.stringify(clonedTree, undefined, 2))
      props.onChange?.(clonedTree)
      return
    }

    // Add leaf
    const leaf = {
      source: selectedOptions[0]?.value
    } as TreeNodeLeaf

    // Initialize based on source type
    if (leaf.source === 'contact_lists') {
      // Contact lists use ContactListCondition
      leaf.contact_list = {
        operator: 'in',
        list_id: '',
        status: undefined
      }
    } else if (leaf.source === 'contacts') {
      // Contacts use ContactCondition
      leaf.contact = {
        filters: [] as DimensionFilter[]
      }
    } else if (leaf.source === 'custom_events_goals') {
      // Custom events goals use CustomEventsGoalCondition
      leaf.custom_events_goal = {
        goal_type: '*', // All goal types by default
        aggregate_operator: 'count',
        operator: 'gte',
        value: 1, // At least 1 event makes sense as default
        timeframe_operator: 'anytime',
        timeframe_values: []
      } as CustomEventsGoalCondition
    } else {
      // Contact timeline uses ContactTimelineCondition
      leaf.contact_timeline = {
        kind: '',
        timeframe_operator: 'anytime',
        timeframe_values: [],
        count_operator: 'at_least',
        count_value: 1,
        filters: [] as DimensionFilter[]
      } as ContactTimelineCondition
    }

    // console.log('leaf', leaf)

    // // https://cube.dev/docs/product/apis-integrations/rest-api/query-format#filters-operators
    // switch (selectedOptions[1].type) {
    //   case 'string':
    //     leaf.operator = 'equals'
    //     leaf.string_values = []
    //     break
    //   case 'number':
    //     leaf.operator = 'equals'
    //     leaf.number_values = []
    //     break
    //   case 'time':
    //     leaf.operator = 'beforeDate'
    //     leaf.string_values = []
    //     break
    //   default: {
    //     console.error('operator type ' + selectedOptions[1].type + ' is not implemented')
    //     return
    //   }
    // }

    const node: TreeNode = {
      kind: 'leaf',
      leaf: leaf
    }

    let editingNodeKey = 0

    // node path, if non root
    if (path === '') {
      clonedTree.branch.leaves.push(node)
      editingNodeKey = clonedTree.branch.leaves.length - 1
    } else {
      const target = get(clonedTree, setPath)
      target.branch.leaves.push(node)
      editingNodeKey = target.branch.leaves.length - 1
    }

    const editingNodeLeaf = Object.assign(
      {
        path: path === '' ? 'branch.leaves' : setPath + '.branch.leaves',
        key: editingNodeKey,
        is_new: true
      },
      node as object
    ) as EditingNodeLeaf

    setEditingNodeLeaf(editingNodeLeaf)

    // console.log('tree is', JSON.stringify(clonedTree, undefined, 2))
    props.onChange?.(clonedTree)
  }

  const deleteButton = (path: string, pathKey: number, isBranch: boolean) => {
    return (
      <Popconfirm
        placement="left"
        title={isBranch ? t`Do you really want to remove this branch?` : t`Do you really want to remove this condition?`}
        onConfirm={cancelOrDeleteNode.bind(null, path, pathKey)}
        okText={t`Delete`}
        okButtonProps={{ danger: true }}
        cancelText={t`Cancel`}
      >
        <Button size="small">
          <FontAwesomeIcon icon={faTrashCan} />
        </Button>
      </Popconfirm>
    )
  }

  const editNode = (path: string, key: number) => {
    if (!props.value) return

    const condition = get(props.value, path + '[' + key + ']')

    const editingNodeLeaf = Object.assign(
      {
        path: path,
        key: key
      },
      condition
    ) as EditingNodeLeaf

    setEditingNodeLeaf(editingNodeLeaf)
  }

  const changeBranchOperator = (path: string, pathKey: number, value: BooleanOperator) => {
    const clonedTree = cloneDeep(props.value) as TreeNode
    if (!clonedTree.branch) return

    if (path === '') {
      clonedTree.branch.operator = value
    } else {
      set(clonedTree, path + '[' + pathKey + '].branch.operator', value)
    }

    // console.log('new tree', JSON.stringify(clonedTree, undefined, 2))
    props.onChange?.(clonedTree)
  }

  const onUpdateNode = (updatedNode: TreeNode, path: string, pathKey: number) => {
    const fullPath = path + '[' + pathKey + ']'
    // console.log('fullPath', fullPath)
    // const condition = get(props.value, path + '[' + pathKey + ']')

    const clonedTree = cloneDeep(props.value) as TreeNode
    set(clonedTree, fullPath, updatedNode)
    console.log('tree is', JSON.stringify(clonedTree, undefined, 2))
    props.onChange?.(clonedTree)
  }

  const renderLeaf = (node: TreeNode, path: string, pathKey: number) => {
    const isEditingCurrent =
      editingNodeLeaf && editingNodeLeaf.path === path && editingNodeLeaf.key === pathKey
        ? true
        : false

    const schema = props.schemas[node.leaf?.source as string]

    if (!schema) {
      return (
        <div className="py-4 pl-4">
          <Flex gap="small" className="float-right">
            {deleteButton(path, pathKey, false)}
          </Flex>
          <div>
            <Alert type="error" message={t`source ${node.leaf?.source} not found`} />
          </div>
        </div>
      )
    }

    if (isEditingCurrent && editingNodeLeaf) {
      const isContactSource = node.leaf?.source === 'contacts'
      const isContactListSource = node.leaf?.source === 'contact_lists'
      const isCustomEventsGoalSource = node.leaf?.source === 'custom_events_goals'
      const isContactTimelineSource = node.leaf?.source === 'contact_timeline'

      return (
        <div className="py-4 pl-4">
          {isContactSource && (
            <LeafContactForm
              value={node}
              onChange={(updatedLeaf: TreeNode) => {
                onUpdateNode(updatedLeaf, path, pathKey)
              }}
              source={node.leaf?.source as string}
              schema={schema}
              editingNodeLeaf={editingNodeLeaf as EditingNodeLeaf}
              setEditingNodeLeaf={setEditingNodeLeaf}
              cancelOrDeleteNode={cancelOrDeleteNode.bind(null, path, pathKey)}
              customFieldLabels={props.customFieldLabels}
            />
          )}
          {isContactListSource && (
            <LeafContactListForm
              value={node}
              onChange={(updatedLeaf: TreeNode) => {
                onUpdateNode(updatedLeaf, path, pathKey)
              }}
              source={node.leaf?.source as string}
              schema={schema}
              editingNodeLeaf={editingNodeLeaf as EditingNodeLeaf}
              setEditingNodeLeaf={setEditingNodeLeaf}
              cancelOrDeleteNode={cancelOrDeleteNode.bind(null, path, pathKey)}
              lists={props.lists || []}
              customFieldLabels={props.customFieldLabels}
            />
          )}
          {isCustomEventsGoalSource && (
            <LeafCustomEventsGoalForm
              value={node}
              onChange={(updatedLeaf: TreeNode) => {
                onUpdateNode(updatedLeaf, path, pathKey)
              }}
              source={node.leaf?.source as string}
              schema={schema}
              editingNodeLeaf={editingNodeLeaf as EditingNodeLeaf}
              setEditingNodeLeaf={setEditingNodeLeaf}
              cancelOrDeleteNode={cancelOrDeleteNode.bind(null, path, pathKey)}
              customFieldLabels={props.customFieldLabels}
            />
          )}
          {isContactTimelineSource && (
            <LeafActionForm
              value={node}
              onChange={(updatedLeaf: TreeNode) => {
                onUpdateNode(updatedLeaf, path, pathKey)
              }}
              source={node.leaf?.source as string}
              schema={schema}
              editingNodeLeaf={editingNodeLeaf as EditingNodeLeaf}
              setEditingNodeLeaf={setEditingNodeLeaf}
              cancelOrDeleteNode={cancelOrDeleteNode.bind(null, path, pathKey)}
              customFieldLabels={props.customFieldLabels}
            />
          )}
        </div>
      )
    }

    // console.log('node', node)

    // Special rendering for contact_lists
    const isContactListSource = node.leaf?.source === 'contact_lists'
    if (isContactListSource && node.leaf?.contact_list) {
      const contactList = node.leaf.contact_list
      const listName =
        props.lists?.find((l) => l.id === contactList.list_id)?.name || contactList.list_id
      const statusLabel =
        schema.fields['status']?.options?.find((o) => o.value === contactList.status)?.label ||
        contactList.status
      const isInList = contactList.operator === 'in'

      return (
        <div style={{ lineHeight: '32px' }} className="py-4 pl-4">
          <Flex gap="small" className="float-right">
            {deleteButton(path, pathKey, false)}
            <Button size="small" onClick={editNode.bind(null, path, pathKey)}>
              <FontAwesomeIcon icon={faPenToSquare} />
            </Button>
          </Flex>

          <div>
            <Space style={{ alignItems: 'center' }}>
              <Tag bordered={false} color="cyan">
                {schema.icon && <FontAwesomeIcon icon={schema.icon} style={{ marginRight: 8 }} />}
                {t`List subscription`}
              </Tag>
              <span className="opacity-60">{isInList ? t`is in` : t`is not in`}</span>
              <Tag bordered={false} color="green">
                {listName}
              </Tag>
              {isInList && contactList.status && (
                <>
                  <span className="opacity-60">{t`with status`}</span>
                  <Tag bordered={false} color="purple">
                    {statusLabel}
                  </Tag>
                </>
              )}
            </Space>
          </div>
        </div>
      )
    }

    // Special rendering for custom_events_goals
    const isCustomEventsGoalSource = node.leaf?.source === 'custom_events_goals'
    if (isCustomEventsGoalSource && node.leaf?.custom_events_goal) {
      const goal = node.leaf.custom_events_goal
      const goalTypeLabel =
        schema.fields['goal_type']?.options?.find((o) => o.value === goal.goal_type)?.label ||
        goal.goal_type
      const aggregateLabel =
        schema.fields['aggregate_operator']?.options?.find(
          (o) => o.value === goal.aggregate_operator
        )?.label || goal.aggregate_operator
      const operatorLabel =
        schema.fields['operator']?.options?.find((o) => o.value === goal.operator)?.label ||
        goal.operator

      return (
        <div style={{ lineHeight: '32px' }} className="py-4 pl-4">
          <Flex gap="small" className="float-right">
            {deleteButton(path, pathKey, false)}
            <Button size="small" onClick={editNode.bind(null, path, pathKey)}>
              <FontAwesomeIcon icon={faPenToSquare} />
            </Button>
          </Flex>

          <div>
            <Space style={{ alignItems: 'start' }}>
              <Tag bordered={false} color="cyan">
                {schema.icon && <FontAwesomeIcon icon={schema.icon} style={{ marginRight: 8 }} />}
                {t`Goal`}
              </Tag>
              <div>
                <div className="mb-2">
                  <Space>
                    <span className="opacity-60">{t`type`}</span>
                    <Tag bordered={false} color="blue">
                      {goalTypeLabel}
                    </Tag>
                  </Space>
                </div>
                <div className="mb-2">
                  <Space>
                    <Tag bordered={false} color="blue">
                      {aggregateLabel}
                    </Tag>
                    <span className="opacity-60">{t`is`}</span>
                    <Tag bordered={false} color="blue">
                      {operatorLabel}
                    </Tag>
                    <Tag bordered={false} color="blue">
                      {goal.value}
                    </Tag>
                    {goal.operator === 'between' && goal.value_2 !== undefined && (
                      <>
                        <span className="opacity-60">{t`and`}</span>
                        <Tag bordered={false} color="blue">
                          {goal.value_2}
                        </Tag>
                      </>
                    )}
                  </Space>
                </div>
                <div>
                  <Space>
                    <span className="opacity-60">{t`timeframe`}</span>
                    {goal.timeframe_operator === 'anytime' && (
                      <Tag bordered={false} color="blue">
                        {t`anytime`}
                      </Tag>
                    )}
                    {goal.timeframe_operator === 'in_the_last_days' && (
                      <>
                        <span className="opacity-60">{t`in the last`}</span>
                        <Tag bordered={false} color="blue">
                          {goal.timeframe_values?.[0]}
                        </Tag>
                        <span className="opacity-60">{t`days`}</span>
                      </>
                    )}
                    {goal.timeframe_operator === 'in_date_range' && (
                      <>
                        <span className="opacity-60">{t`between`}</span>
                        <Tag bordered={false} color="blue">
                          {formatDateDisplay(goal.timeframe_values?.[0])}
                        </Tag>
                        <span className="opacity-60">&rarr;</span>
                        <Tag bordered={false} color="blue">
                          {formatDateDisplay(goal.timeframe_values?.[1])}
                        </Tag>
                      </>
                    )}
                    {goal.timeframe_operator === 'before_date' && (
                      <>
                        <span className="opacity-60">{t`before`}</span>
                        <Tag bordered={false} color="blue">
                          {formatDateDisplay(goal.timeframe_values?.[0])}
                        </Tag>
                      </>
                    )}
                    {goal.timeframe_operator === 'after_date' && (
                      <>
                        <span className="opacity-60">{t`after`}</span>
                        <Tag bordered={false} color="blue">
                          {formatDateDisplay(goal.timeframe_values?.[0])}
                        </Tag>
                      </>
                    )}
                  </Space>
                </div>
              </div>
            </Space>
          </div>
        </div>
      )
    }

    // Determine filters based on source type
    const isContactSource = node.leaf?.source === 'contacts'
    const isContactTimelineSource = node.leaf?.source === 'contact_timeline'
    const filtersToShow = isContactSource
      ? node.leaf?.contact?.filters
      : isContactTimelineSource
        ? node.leaf?.contact_timeline?.filters
        : undefined

    return (
      <div style={{ lineHeight: '32px' }} className="py-4 pl-4">
        <Space.Compact className="float-right">
          {deleteButton(path, pathKey, false)}
          <Button size="small" onClick={editNode.bind(null, path, pathKey)}>
            <FontAwesomeIcon icon={faPenToSquare} />
          </Button>
        </Space.Compact>

        <div>
          <Space style={{ alignItems: 'start' }}>
            {isContactSource && (
              <Tag bordered={false} color="cyan">
                {schema.icon && <FontAwesomeIcon icon={schema.icon} style={{ marginRight: 8 }} />}
                {t`Contact property`}
              </Tag>
            )}
            {isContactTimelineSource && (
              <Tag bordered={false} color="cyan">
                {schema.icon && <FontAwesomeIcon icon={schema.icon} style={{ marginRight: 8 }} />}
                {t`Activity`}
              </Tag>
            )}
            <div>
              {node.leaf?.contact_timeline && (
                <>
                  <div className="mb-2">
                    <span className="opacity-60 pr-3">{t`type`}</span>
                    <Tag bordered={false} color="blue">
                      {node.leaf?.contact_timeline.kind === 'open_email' && t`Open email`}
                      {node.leaf?.contact_timeline.kind === 'click_email' && t`Click email`}
                      {node.leaf?.contact_timeline.kind === 'bounce_email' && t`Bounce email`}
                      {node.leaf?.contact_timeline.kind === 'complain_email' && t`Complain email`}
                      {node.leaf?.contact_timeline.kind === 'unsubscribe_email' &&
                        t`Unsubscribe from list`}
                      {node.leaf?.contact_timeline.kind === 'insert_message_history' &&
                        t`New message (email...)`}
                    </Tag>
                  </div>
                  <Space>
                    <span className="opacity-60">{t`happened`}</span>
                    <Tag bordered={false} color="blue">
                      {node.leaf?.contact_timeline.count_operator === 'at_least' && t`at least`}
                      {node.leaf?.contact_timeline.count_operator === 'at_most' && t`at most`}
                      {node.leaf?.contact_timeline.count_operator === 'exactly' && t`exactly`}
                    </Tag>
                    <Tag bordered={false} color="blue">
                      {node.leaf?.contact_timeline.count_value}
                    </Tag>
                    <span className="opacity-60">{t`times`}</span>
                  </Space>

                  <div className="mt-2">
                    <Space>
                      <span className="opacity-60">{t`timeframe`}</span>
                      {node.leaf?.contact_timeline.timeframe_operator === 'anytime' && (
                        <Tag bordered={false} color="blue">
                          {t`anytime`}
                        </Tag>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'in_the_last_days' && (
                        <>
                          <span className="opacity-60">{t`in the last`}</span>
                          <Tag bordered={false} color="blue">
                            {node.leaf?.contact_timeline.timeframe_values?.[0]}
                          </Tag>
                          <span className="opacity-60">{t`days`}</span>
                        </>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'in_date_range' && (
                        <>
                          <span className="opacity-60">{t`between`}</span>
                          <Tag bordered={false} color="blue">
                            {formatDateDisplay(node.leaf?.contact_timeline.timeframe_values?.[0])}
                          </Tag>
                          &rarr;
                          <Tag className="ml-3" bordered={false} color="blue">
                            {formatDateDisplay(node.leaf?.contact_timeline.timeframe_values?.[1])}
                          </Tag>
                        </>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'before_date' && (
                        <>
                          <span className="opacity-60">{t`before`}</span>
                          <Tag bordered={false} color="blue">
                            {formatDateDisplay(node.leaf?.contact_timeline.timeframe_values?.[0])}
                          </Tag>
                        </>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'after_date' && (
                        <>
                          <span className="opacity-60">{t`after`}</span>
                          <Tag bordered={false} color="blue">
                            {formatDateDisplay(node.leaf?.contact_timeline.timeframe_values?.[0])}
                          </Tag>
                        </>
                      )}
                    </Space>
                  </div>
                </>
              )}
              {filtersToShow && filtersToShow.length > 0 && (
                <Space style={{ alignItems: 'start' }}>
                  <table>
                    <tbody>
                      {filtersToShow.map((filter, key) => {
                        const field = schema.fields[filter.field_name]
                        // Use JSON renderer if filter has json_path, otherwise use the field_type renderer
                        const rendererType =
                          filter.json_path && filter.json_path.length > 0
                            ? 'json'
                            : filter.field_type
                        const fieldTypeRenderer = fieldTypeRendererDictionary[rendererType]

                        return (
                          <tr key={key}>
                            <td>
                              {!fieldTypeRenderer && (
                                <Alert
                                  type="error"
                                  message={t`type ${rendererType} is not implemented`}
                                />
                              )}
                              {fieldTypeRenderer && (
                                <Space key={key}>
                                  <Popover
                                    title={'field: ' + filter.field_name}
                                    content={field.description}
                                  >
                                    <b>
                                      {props.customFieldLabels?.[filter.field_name] || field.title}
                                    </b>
                                  </Popover>
                                  {fieldTypeRenderer.render(filter, field, props.customFieldLabels)}
                                </Space>
                              )}
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </Space>
              )}
            </div>
          </Space>
        </div>
      </div>
    )
  }

  const renderBranch = (node: TreeNode, path: string, pathKey: number) => {
    if (!node.branch) return <span>{t`A branch condition is required...`}</span>

    const conditionPath = path === '' ? 'branch.leaves' : path + '[' + pathKey + '].branch.leaves'
    // console.log('conditionPath', conditionPath)
    const isEditing = editingNodeLeaf ? true : false
    const borderColorID = getColorID()

    const colorClass = getColorClass(borderColorID)

    return (
      <div className={styles.self}>
        <div className={`${styles.inputGroup} ${colorClass}`}>
          {/* DELETE GROUP BUTTON */}
          {path !== '' && !isEditing && (
            <Flex gap="small" className="float-right">
              {deleteButton(path, pathKey, true)}
            </Flex>
          )}
          {/* SELECT GROUP AND OR */}
          <Select
            size="small"
            className="mr-2"
            style={{ width: '80px' }}
            onChange={changeBranchOperator.bind(null, path, pathKey)}
            value={node.branch.operator}
          >
            <Select.Option value="and">{t`ALL`}</Select.Option>
            <Select.Option value="or">{t`ANY`}</Select.Option>
          </Select>{' '}
          <span className="opacity-60">{t`of the following conditions match:`}</span>
        </div>

        {/* LOOP OVER CONDITIONS */}

        {node.branch.leaves.map((leaf: TreeNode, i: number) => {
          return (
            <div key={i} className={styles.condition}>
              <div className={`${styles.conditionSeparator} ${colorClass}`}></div>
              {i !== 0 && (
                <div className={`${styles.conditionOperatorAndOr} ${colorClass}`}>
                  {node.branch?.operator}
                </div>
              )}

              {/* recursive call to draw the tree */}
              {leaf.leaf && renderLeaf(leaf, conditionPath, i)}
              {leaf.branch && renderBranch(leaf, conditionPath, i)}
            </div>
          )
        })}

        {/* ADD CONDITION BUTTON */}

        <div className={styles.condition}>
          <div
            className={`${styles.conditionSeparator} ${styles.conditionSeparatorHalf} ${colorClass}`}
          ></div>
          {node.branch.leaves.length > 0 && (
            <div className={`${styles.conditionOperatorAndOr} ${colorClass}`}>
              {node.branch.operator}
            </div>
          )}

          <div className="py-4">
            <Cascader
              defaultValue={undefined}
              value={undefined}
              popupClassName={styles.cascaderWide}
              onChange={addTreeNode.bind(null, path, pathKey)}
              expandTrigger="hover"
              options={cascaderOptions}
            >
              <Button
                size="small"
                type="primary"
                ghost={node.branch.leaves.length > 0}
                disabled={editingNodeLeaf ? true : false}
              >
                <FontAwesomeIcon icon={faPlus} />
                &nbsp; {t`Condition`}
              </Button>
            </Cascader>
          </div>
        </div>
      </div>
    )
  }

  if (!props.value) {
    return <span>{t`A value is required...`}</span>
  }

  return <div className="pt-2">{renderBranch(props.value, '', 0)}</div>
}
