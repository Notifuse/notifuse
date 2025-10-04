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
  ContactTimelineCondition
} from './interfaces'
import TableTag from './table_tag'
import { FieldTypeString } from './type_string'
import { FieldTypeTime } from './type_time'
import { LeafActionForm, LeafContactForm, LeafContactListForm } from './form_leaf'
import { FieldTypeNumber } from './type_number'
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
}

const fieldTypeRendererDictionary: FieldTypeRendererDictionary = {
  string: new FieldTypeString(),
  time: new FieldTypeTime(),
  number: new FieldTypeNumber()
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
  const [editingNodeLeaf, setEditingNodeLeaf] = useState<EditingNodeLeaf | undefined>(undefined)

  const cascaderOptions = useMemo(() => {
    const options: any[] = [
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
            AND | OR
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
        target.filter((_x: any, i: number) => i !== key)
      )
    }

    // reset possible edit mode on current field
    if (editingNodeLeaf && editingNodeLeaf.path === path && editingNodeLeaf.key === key) {
      setEditingNodeLeaf(undefined)
    }

    if (props.onChange) props.onChange(clonedTree)
  }

  const addTreeNode = (path: string, key: number, values: any[], selectedOptions: any) => {
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
      table: selectedOptions[0].value
    } as TreeNodeLeaf

    // Initialize based on table type
    if (leaf.table === 'contact_lists') {
      // Contact lists use ContactListCondition
      leaf.contact_list = {
        operator: 'in',
        list_id: '',
        status: undefined
      }
    } else if (leaf.table === 'contacts') {
      // Contacts use ContactCondition
      leaf.contact = {
        filters: [] as DimensionFilter[]
      }
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
        title={'Do you really want to remove this ' + (isBranch ? 'branch' : 'condition') + '?'}
        onConfirm={cancelOrDeleteNode.bind(null, path, pathKey)}
        okText="Delete"
        okButtonProps={{ danger: true }}
        cancelText="Cancel"
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

  const changeBranchOperator = (path: string, pathKey: number, value: any) => {
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

    const schema = props.schemas[node.leaf?.table as string]

    if (!schema) {
      return (
        <div className="py-4 pl-4">
          <Flex gap="small" className="float-right">
            {deleteButton(path, pathKey, false)}
          </Flex>
          <div>
            <Alert type="error" message={'table ' + node.leaf?.table + ' not found'} />
          </div>
        </div>
      )
    }

    if (isEditingCurrent && editingNodeLeaf) {
      const isContactTable = node.leaf?.table === 'contacts'
      const isContactListTable = node.leaf?.table === 'contact_lists'

      return (
        <div className="py-4 pl-4">
          {isContactTable && (
            <LeafContactForm
              value={node}
              onChange={(updatedLeaf: TreeNode) => {
                onUpdateNode(updatedLeaf, path, pathKey)
              }}
              table={node.leaf?.table as string}
              schema={schema}
              editingNodeLeaf={editingNodeLeaf as EditingNodeLeaf}
              setEditingNodeLeaf={setEditingNodeLeaf}
              cancelOrDeleteNode={cancelOrDeleteNode.bind(null, path, pathKey)}
            />
          )}
          {isContactListTable && (
            <LeafContactListForm
              value={node}
              onChange={(updatedLeaf: TreeNode) => {
                onUpdateNode(updatedLeaf, path, pathKey)
              }}
              table={node.leaf?.table as string}
              schema={schema}
              editingNodeLeaf={editingNodeLeaf as EditingNodeLeaf}
              setEditingNodeLeaf={setEditingNodeLeaf}
              cancelOrDeleteNode={cancelOrDeleteNode.bind(null, path, pathKey)}
              lists={props.lists || []}
            />
          )}
          {!isContactTable && !isContactListTable && (
            <LeafActionForm
              value={node}
              onChange={(updatedLeaf: TreeNode) => {
                onUpdateNode(updatedLeaf, path, pathKey)
              }}
              table={node.leaf?.table as string}
              schema={schema}
              editingNodeLeaf={editingNodeLeaf as EditingNodeLeaf}
              setEditingNodeLeaf={setEditingNodeLeaf}
              cancelOrDeleteNode={cancelOrDeleteNode.bind(null, path, pathKey)}
            />
          )}
        </div>
      )
    }

    // console.log('node', node)

    // Special rendering for contact_lists
    const isContactListTable = node.leaf?.table === 'contact_lists'
    if (isContactListTable && node.leaf?.contact_list) {
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
              <span style={{ fontWeight: 500 }}>
                {schema.icon && (
                  <FontAwesomeIcon icon={schema.icon} style={{ width: 18, marginRight: 8 }} />
                )}
                {schema.title}
              </span>
              <Tag bordered={false} color="blue">
                {isInList ? 'is in' : 'is not in'}
              </Tag>
              <Tag bordered={false} color="green">
                {listName}
              </Tag>
              {isInList && contactList.status && (
                <>
                  <span className="opacity-60">with status</span>
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

    // Determine filters based on table type
    const isContactTable = node.leaf?.table === 'contacts'
    const isContactTimelineTable = node.leaf?.table === 'contact_timeline'
    const filtersToShow = isContactTable
      ? node.leaf?.contact?.filters
      : isContactTimelineTable
        ? node.leaf?.contact_timeline?.filters
        : undefined

    return (
      <div style={{ lineHeight: '32px' }} className="py-4 pl-4">
        <Button.Group className="float-right">
          {deleteButton(path, pathKey, false)}
          <Button size="small" onClick={editNode.bind(null, path, pathKey)}>
            <FontAwesomeIcon icon={faPenToSquare} />
          </Button>
        </Button.Group>

        <div>
          <Space style={{ alignItems: 'start' }}>
            <TableTag table={node.leaf?.table as string} />
            <div>
              {node.leaf?.contact_timeline && (
                <>
                  <div className="mb-2">
                    <span className="opacity-60 pr-3">type</span>
                    <Tag bordered={false} color="blue">
                      {node.leaf?.contact_timeline.kind === 'open_email' && 'Open email'}
                      {node.leaf?.contact_timeline.kind === 'click_email' && 'Click email'}
                      {node.leaf?.contact_timeline.kind === 'bounce_email' && 'Bounce email'}
                      {node.leaf?.contact_timeline.kind === 'complain_email' && 'Complain email'}
                      {node.leaf?.contact_timeline.kind === 'unsubscribe_email' &&
                        'Unsubscribe email'}
                      {node.leaf?.contact_timeline.kind === 'insert_message_history' && 'New email'}
                    </Tag>
                  </div>
                  <Space>
                    <span className="opacity-60">happened</span>
                    <Tag bordered={false} color="blue">
                      {node.leaf?.contact_timeline.count_operator === 'at_least' && 'at least'}
                      {node.leaf?.contact_timeline.count_operator === 'at_most' && 'at most'}
                      {node.leaf?.contact_timeline.count_operator === 'exactly' && 'exactly'}
                    </Tag>
                    <Tag bordered={false} color="blue">
                      {node.leaf?.contact_timeline.count_value}
                    </Tag>
                    <span className="opacity-60">times</span>
                  </Space>

                  <div className="mt-2">
                    <Space>
                      <span className="opacity-60">timeframe</span>
                      {node.leaf?.contact_timeline.timeframe_operator === 'anytime' && (
                        <Tag bordered={false} color="blue">
                          anytime
                        </Tag>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'in_the_last_days' && (
                        <>
                          <span className="opacity-60">in the last</span>
                          <Tag bordered={false} color="blue">
                            {node.leaf?.contact_timeline.timeframe_values?.[0]}
                          </Tag>
                          <span className="opacity-60">days</span>
                        </>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'in_date_range' && (
                        <>
                          <span className="opacity-60">between</span>
                          <Tag bordered={false} color="blue">
                            {node.leaf?.contact_timeline.timeframe_values?.[0]}
                          </Tag>
                          &rarr;
                          <Tag className="ml-3" bordered={false} color="blue">
                            {node.leaf?.contact_timeline.timeframe_values?.[1]}
                          </Tag>
                        </>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'before_date' && (
                        <>
                          <span className="opacity-60">before</span>
                          <Tag bordered={false} color="blue">
                            {node.leaf?.contact_timeline.timeframe_values?.[0]}
                          </Tag>
                        </>
                      )}
                      {node.leaf?.contact_timeline.timeframe_operator === 'after_date' && (
                        <>
                          <span className="opacity-60">after</span>
                          <Tag bordered={false} color="blue">
                            {node.leaf?.contact_timeline.timeframe_values?.[0]}
                          </Tag>
                        </>
                      )}
                    </Space>
                  </div>
                </>
              )}
              {filtersToShow && filtersToShow.length > 0 && (
                <Space style={{ alignItems: 'start' }}>
                  <span className="opacity-60">with filters</span>
                  <table>
                    <tbody>
                      {filtersToShow.map((filter, key) => {
                        const field = schema.fields[filter.field_name]
                        const fieldTypeRenderer = fieldTypeRendererDictionary[filter.field_type]

                        return (
                          <tr key={key}>
                            <td>
                              {!fieldTypeRenderer && (
                                <Alert
                                  type="error"
                                  message={'type ' + filter.field_type + ' is not implemented'}
                                />
                              )}
                              {fieldTypeRenderer && (
                                <Space key={key}>
                                  <Popover
                                    title={'field: ' + filter.field_name}
                                    content={field.description}
                                  >
                                    <b>{field.title}</b>
                                  </Popover>
                                  {fieldTypeRenderer.render(filter, field)}
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
    if (!node.branch) return <span>A branch condition is required...</span>

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
            <Select.Option value="and">ALL</Select.Option>
            <Select.Option value="or">ANY</Select.Option>
          </Select>{' '}
          <span className="opacity-60">of the following conditions match:</span>
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
                &nbsp; Condition
              </Button>
            </Cascader>
          </div>
        </div>
      </div>
    )
  }

  if (!props.value) {
    return <span>A value is required...</span>
  }

  return <div className="pt-2">{renderBranch(props.value, '', 0)}</div>
}
