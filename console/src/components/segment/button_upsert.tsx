import {
  Alert,
  Button,
  Col,
  Drawer,
  Form,
  Input,
  Row,
  Select,
  Space,
  Tag,
  Progress,
  Popover,
  message
} from 'antd'
import React, { useMemo, useState, useEffect } from 'react'
import { useParams } from '@tanstack/react-router'
import { useAuth } from '../../contexts/AuthContext'
import { TreeNodeInput, HasLeaf } from './input'
import { useQuery } from '@tanstack/react-query'
import { listsApi } from '../../services/api/list'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus, faInfoCircle } from '@fortawesome/free-solid-svg-icons'
import {
  Segment,
  createSegment,
  updateSegment,
  previewSegment,
  CreateSegmentRequest,
  UpdateSegmentRequest,
  PreviewSegmentRequest
} from '../../services/api/segment'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { TableSchemas } from './table_schemas'
import { Workspace } from '../../services/api/types'

// Helper function to check if a tree contains relative date filters
const treeHasRelativeDates = (tree: any): boolean => {
  if (!tree) return false

  if (tree.kind === 'branch') {
    // Check all child leaves recursively
    if (tree.branch?.leaves) {
      return tree.branch.leaves.some((leaf: any) => treeHasRelativeDates(leaf))
    }
    return false
  }

  if (tree.kind === 'leaf') {
    // Check contact timeline conditions for relative date operators
    if (tree.leaf?.contact_timeline) {
      if (tree.leaf.contact_timeline.timeframe_operator === 'in_the_last_days') {
        return true
      }
    }
    // Check contact property filters for relative date operators
    if (tree.leaf?.contact?.filters) {
      const hasRelativeDateFilter = tree.leaf.contact.filters.some(
        (filter: any) => filter.operator === 'in_the_last_days'
      )
      if (hasRelativeDateFilter) {
        return true
      }
    }
    return false
  }

  return false
}

const ButtonUpsertSegment = (props: {
  segment?: Segment
  btnType?: 'primary' | 'default' | 'dashed' | 'link' | 'text' | undefined
  btnSize?: 'small' | 'middle' | 'large' | undefined
  totalContacts?: number
  onSuccess?: () => void
  children?: React.ReactNode
}) => {
  const [drawserVisible, setDrawserVisible] = useState(false)

  // but the drawer in a separate component to make sure the
  // form is reset when the drawer is closed
  return (
    <>
      {props.children ? (
        <span onClick={() => setDrawserVisible(!drawserVisible)}>{props.children}</span>
      ) : (
        <Button
          type={props.btnType || 'primary'}
          size={props.btnSize || 'small'}
          ghost
          icon={!props.segment ? <FontAwesomeIcon icon={faPlus} /> : undefined}
          onClick={() => setDrawserVisible(!drawserVisible)}
        >
          {props.segment ? 'Edit segment' : 'Segment'}
        </Button>
      )}
      {drawserVisible && (
        <DrawerSegment
          segment={props.segment}
          totalContacts={props.totalContacts}
          setDrawserVisible={setDrawserVisible}
          onSuccess={props.onSuccess}
        />
      )}
    </>
  )
}

const DrawerSegment = (props: {
  segment?: Segment
  totalContacts?: number
  setDrawserVisible: any
  onSuccess?: () => void
}) => {
  const { workspaceId } = useParams({ from: '/workspace/$workspaceId' })
  const { workspaces } = useAuth()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [previewedData, setPreviewedData] = useState<string | undefined>() // track the tree hash to avoid re-render
  const [previewResponse, setPreviewResponse] = useState<any>()

  // Find the current workspace
  useEffect(() => {
    if (workspaceId && workspaces.length > 0) {
      const ws = workspaces.find((w) => w.id === workspaceId)
      if (ws) {
        setWorkspace(ws)
      }
    }
  }, [workspaceId, workspaces])

  // Log the tree when the drawer opens
  useEffect(() => {
    if (props.segment?.tree) {
      console.log('Segment tree:', props.segment.tree)
    } else {
      console.log('New segment - empty tree')
    }
  }, [])

  // Fetch lists for the current workspace
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: !!workspaceId
  })

  const lists = listsData?.lists || []

  const preview = async () => {
    if (loadingPreview || !workspaceId) return
    setLoadingPreview(true)

    const values = form.getFieldsValue()
    const requestData: PreviewSegmentRequest = {
      workspace_id: workspaceId,
      tree: values.tree,
      limit: 100
    }

    // compute data hash
    setPreviewedData(JSON.stringify(requestData))

    try {
      const res = await previewSegment(requestData)
      setPreviewResponse({
        count: res.total_count,
        sql: res.generated_sql,
        args: res.sql_args
      })
      setLoadingPreview(false)
    } catch (error) {
      console.error('Preview error:', error)
      message.error('Failed to preview segment')
      setLoadingPreview(false)
    }
  }

  const initialValues = Object.assign(
    {
      color: 'blue',
      timezone: workspace?.settings.timezone || 'UTC',
      tree: {
        kind: 'branch',
        branch: {
          operator: 'and',
          leaves: []
        }
      }
    },
    props.segment
  )

  const onFinish = async (values: any) => {
    if (loading || !workspaceId) return

    setLoading(true)

    try {
      if (props.segment) {
        // Update existing segment
        const requestData: UpdateSegmentRequest = {
          workspace_id: workspaceId,
          id: props.segment.id,
          name: values.name,
          color: values.color,
          tree: values.tree,
          timezone: values.timezone
        }
        await updateSegment(requestData)
        message.success('The segment has been updated!')
      } else {
        // Create new segment
        // Generate snake_case ID: lowercase, replace spaces/hyphens with underscores, remove invalid chars
        const generatedId = values.name
          .toLowerCase()
          .replace(/[\s-]+/g, '_') // Replace spaces and hyphens with underscores
          .replace(/[^a-z0-9_]/g, '') // Remove any character that's not a-z, 0-9, or underscore
          .replace(/^_+|_+$/g, '') // Remove leading/trailing underscores
          .replace(/_+/g, '_') // Replace multiple consecutive underscores with single underscore

        const requestData: CreateSegmentRequest = {
          workspace_id: workspaceId,
          id: generatedId,
          name: values.name,
          color: values.color,
          tree: values.tree,
          timezone: values.timezone
        }
        await createSegment(requestData)
        message.success('The segment has been created!')
      }

      form.resetFields()
      setLoading(false)
      props.setDrawserVisible(false)

      // Call onSuccess callback to refresh segments list in parent
      if (props.onSuccess) {
        props.onSuccess()
      }
    } catch (error) {
      console.error('Segment operation error:', error)
      message.error(`Failed to ${props.segment ? 'update' : 'create'} segment`)
      setLoading(false)
    }
  }
  // Use the table schemas for segmentation
  const schemas = useMemo(() => {
    return {
      contacts: TableSchemas.contacts,
      contact_lists: TableSchemas.contact_lists,
      contact_timeline: TableSchemas.contact_timeline
    }
  }, [])

  return (
    <Drawer
      title={props.segment ? 'Update segment' : 'New segment'}
      open={true}
      width={'90%'}
      onClose={() => props.setDrawserVisible(false)}
      styles={{ body: { paddingBottom: 80 } }}
      extra={
        <Space>
          <Button loading={loading} onClick={() => props.setDrawserVisible(false)}>
            Cancel
          </Button>
          <Button
            loading={loading}
            onClick={() => {
              form.submit()
            }}
            type="primary"
          >
            Confirm
          </Button>
        </Space>
      }
    >
      <>
        <Form
          form={form}
          initialValues={initialValues}
          labelCol={{ span: 8 }}
          wrapperCol={{ span: 12 }}
          name="groupForm"
          onFinish={onFinish}
        >
          <Row gutter={24}>
            <Col span={18}>
              <Form.Item name="name" label="Name" rules={[{ required: true, type: 'string' }]}>
                <Input
                  placeholder="i.e: Big spenders..."
                  addonAfter={
                    <Form.Item noStyle name="color">
                      <Select
                        style={{ width: 150 }}
                        options={[
                          {
                            label: (
                              <Tag bordered={false} color="magenta">
                                magenta
                              </Tag>
                            ),
                            value: 'magenta'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="red">
                                red
                              </Tag>
                            ),
                            value: 'red'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="volcano">
                                volcano
                              </Tag>
                            ),
                            value: 'volcano'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="orange">
                                orange
                              </Tag>
                            ),
                            value: 'orange'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="gold">
                                gold
                              </Tag>
                            ),
                            value: 'gold'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="lime">
                                lime
                              </Tag>
                            ),
                            value: 'lime'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="green">
                                green
                              </Tag>
                            ),
                            value: 'green'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="cyan">
                                cyan
                              </Tag>
                            ),
                            value: 'cyan'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="blue">
                                blue
                              </Tag>
                            ),
                            value: 'blue'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="geekblue">
                                geekblue
                              </Tag>
                            ),
                            value: 'geekblue'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="purple">
                                purple
                              </Tag>
                            ),
                            value: 'purple'
                          },
                          {
                            label: (
                              <Tag bordered={false} color="grey">
                                grey
                              </Tag>
                            ),
                            value: 'grey'
                          }
                        ]}
                      ></Select>
                    </Form.Item>
                  }
                />
              </Form.Item>

              <Form.Item
                name="timezone"
                label="Timezone used for dates"
                rules={[{ required: true, type: 'string' }]}
                className="mb-12"
              >
                <Select
                  placeholder="Select a time zone"
                  allowClear={false}
                  showSearch={true}
                  filterOption={(input: string, option: any) => {
                    if (!input || !option) return true
                    const label = option.label || option.value || ''
                    return label.toString().toLowerCase().includes(input.toLowerCase())
                  }}
                  optionFilterProp="label"
                  options={TIMEZONE_OPTIONS}
                />
              </Form.Item>

              {/* Alert for segments with relative date filters */}
              <Form.Item noStyle dependencies={['tree', 'timezone']}>
                {() => {
                  const values = form.getFieldsValue()
                  const hasRelativeDates = treeHasRelativeDates(values.tree)
                  const timezone = values.timezone || workspace?.settings.timezone || 'UTC'

                  if (hasRelativeDates) {
                    return (
                      <Alert
                        type="info"
                        showIcon
                        message={`This segment uses relative date filters and will be automatically recomputed daily at 5:00 AM (${timezone})`}
                        style={{ marginBottom: 16 }}
                      />
                    )
                  }
                  return null
                }}
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item noStyle dependencies={['tree']}>
                {() => {
                  if (loadingPreview) {
                    return (
                      <Progress
                        format={() => (
                          <Button type="primary" ghost loading={true}>
                            Preview
                          </Button>
                        )}
                        type="circle"
                        percent={0}
                        size={150}
                      />
                    )
                  }

                  // check if tree has changed
                  const values = form.getFieldsValue()
                  let shouldPreview = false

                  if (values.tree && workspaceId) {
                    const data = {
                      workspace_id: workspaceId,
                      tree: values.tree,
                      limit: 100
                    }

                    // compute data hash
                    const dataHash = JSON.stringify(data)

                    if (!previewedData || previewedData !== dataHash) {
                      shouldPreview = true
                    }
                  }

                  if (shouldPreview) {
                    return (
                      <Progress
                        format={() => (
                          <Button
                            type="primary"
                            ghost
                            onClick={preview}
                            disabled={HasLeaf(values.tree) ? false : true}
                          >
                            Preview
                          </Button>
                        )}
                        type="circle"
                        percent={0}
                        size={150}
                      />
                    )
                  } else if (previewResponse && previewResponse.count >= 0) {
                    const content =
                      previewResponse.count === 0 ? (
                        <>0 contacts</>
                      ) : (
                        <span className="text-base">{previewResponse.count} contacts</span>
                      )

                    // Calculate percentage based on total contacts
                    let percent = 0
                    if (
                      props.totalContacts &&
                      props.totalContacts > 0 &&
                      previewResponse.count > 0
                    ) {
                      percent = Math.min(100, (previewResponse.count / props.totalContacts) * 100)
                    } else if (previewResponse.count > 0) {
                      // Fallback to fixed percentage if total is not available
                      percent = 50
                    }

                    return (
                      <div style={{ position: 'relative', display: 'inline-block' }}>
                        <Progress
                          format={() => content}
                          type="circle"
                          percent={percent}
                          size={150}
                          status="normal"
                          strokeColor={{
                            '0%': '#4e6cff',
                            '100%': '#8E2DE2'
                          }}
                        />
                        <Popover
                          title="Preview Results"
                          placement="left"
                          content={
                            <div style={{ width: 600, maxHeight: 600, overflow: 'auto' }}>
                              <p>
                                <strong>Matching contacts:</strong> {previewResponse.count}
                              </p>
                              {previewResponse.sql && (
                                <>
                                  <p>
                                    <strong>Generated SQL:</strong>
                                  </p>
                                  <pre
                                    style={{
                                      backgroundColor: '#f5f5f5',
                                      padding: '8px',
                                      borderRadius: '4px',
                                      fontSize: '11px',
                                      overflow: 'auto',
                                      maxHeight: '200px'
                                    }}
                                  >
                                    {previewResponse.sql}
                                  </pre>
                                </>
                              )}
                              {previewResponse.args && previewResponse.args.length > 0 && (
                                <>
                                  <p>
                                    <strong>SQL Arguments:</strong>
                                  </p>
                                  <pre
                                    style={{
                                      backgroundColor: '#f5f5f5',
                                      padding: '8px',
                                      borderRadius: '4px',
                                      fontSize: '11px',
                                      overflow: 'auto',
                                      maxHeight: '100px'
                                    }}
                                  >
                                    {JSON.stringify(previewResponse.args, null, 2)}
                                  </pre>
                                </>
                              )}
                            </div>
                          }
                        >
                          <FontAwesomeIcon
                            icon={faInfoCircle}
                            style={{
                              position: 'absolute',
                              top: 0,
                              right: 0,
                              fontSize: '18px',
                              color: '#1890ff',
                              cursor: 'pointer'
                            }}
                          />
                        </Popover>
                      </div>
                    )
                  }

                  return 'No preview available...'
                }}
              </Form.Item>
            </Col>
          </Row>

          <Form.Item
            name="tree"
            noStyle
            rules={[
              {
                required: true,
                validator: (_rule, value) => {
                  // console.log('value', value)
                  return new Promise((resolve, reject) => {
                    if (HasLeaf(value)) {
                      return resolve(undefined)
                    }
                    return reject(new Error('A tree is required'))
                  })
                }
                // message: Messages.RequiredField
              }
            ]}
          >
            <TreeNodeInput schemas={schemas} lists={lists} />
          </Form.Item>
        </Form>
      </>
    </Drawer>
  )
}

export default ButtonUpsertSegment
