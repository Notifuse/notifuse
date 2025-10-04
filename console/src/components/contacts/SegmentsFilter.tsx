import React from 'react'
import { useMutation, useQueryClient, useQuery } from '@tanstack/react-query'
import { Space, Dropdown, Modal, Badge, Tag, Popover, message, Progress } from 'antd'
import { deleteSegment, type Segment } from '../../services/api/segment'
import { taskApi } from '../../services/api/task'
import ButtonUpsertSegment from '../segment/button_upsert'
import numbro from 'numbro'

interface SegmentsFilterProps {
  workspaceId: string
  segments: Segment[]
  selectedSegmentIds?: string[]
  totalContacts?: number
  onSegmentToggle: (segmentId: string) => void
}

// Separate component for each segment button to handle individual task fetching
interface SegmentButtonProps {
  segment: Segment
  workspaceId: string
  isSelected: boolean
  totalContacts?: number
  onToggle: () => void
  onDelete: (segmentId: string) => void
}

function SegmentButton({
  segment,
  workspaceId,
  isSelected,
  totalContacts,
  onToggle,
  onDelete
}: SegmentButtonProps) {
  const queryClient = useQueryClient()

  // Fetch task for building segments
  const { data: task } = useQuery({
    queryKey: ['segment-task', workspaceId, segment.id],
    queryFn: () => taskApi.findBySegmentId(workspaceId, segment.id),
    enabled: segment.status === 'building',
    refetchInterval: segment.status === 'building' ? 3000 : false // Poll every 3 seconds when building
  })

  // Get status badge color and content for popover
  const getStatusBadge = () => {
    switch (segment.status) {
      case 'active':
        return { status: 'success', title: 'Active', content: 'Ready to use' }
      case 'building': {
        if (task?.state?.build_segment) {
          const buildState = task.state.build_segment
          const progress = task.progress || 0
          return {
            status: 'processing',
            title: 'Building segment',
            content: (
              <div>
                <Progress
                  percent={Math.round(progress)}
                  size="small"
                  style={{ marginBottom: '12px' }}
                />
                <div>
                  Processed:{' '}
                  {numbro(buildState.processed_count).format({ thousandSeparated: true })}
                </div>
                <div>
                  Matched: {numbro(buildState.matched_count).format({ thousandSeparated: true })}
                </div>
                {buildState.total_contacts > 0 && (
                  <div>
                    Total: {numbro(buildState.total_contacts).format({ thousandSeparated: true })}
                  </div>
                )}
              </div>
            )
          }
        }
        return { status: 'processing', title: 'Building', content: 'Processing contacts' }
      }
      case 'deleted':
        return { status: 'error', title: 'Deleted', content: 'Will be removed' }
      default:
        return { status: 'default', title: 'Unknown', content: 'Unknown status' }
    }
  }

  const statusBadge = getStatusBadge()

  return (
    <Dropdown.Button
      key={segment.id}
      size="small"
      onClick={onToggle}
      buttonsRender={([leftButton, rightButton]) => [
        React.cloneElement(leftButton as React.ReactElement, {
          color: isSelected ? 'primary' : 'default',
          variant: 'outlined'
        }),
        React.cloneElement(rightButton as React.ReactElement, {
          color: isSelected ? 'primary' : 'default',
          variant: 'outlined'
        })
      ]}
      menu={{
        items: [
          {
            key: 'update',
            label: (
              <ButtonUpsertSegment
                segment={segment}
                totalContacts={totalContacts}
                onSuccess={() => {
                  queryClient.invalidateQueries({ queryKey: ['segments', workspaceId] })
                }}
              >
                <span>Update</span>
              </ButtonUpsertSegment>
            )
          },
          {
            key: 'delete',
            label: <span style={{ color: '#ff4d4f' }}>Delete</span>,
            onClick: () => {
              Modal.confirm({
                title: 'Delete segment',
                content: `Are you sure you want to delete "${segment.name}"?`,
                okText: 'Yes',
                cancelText: 'No',
                okButtonProps: { danger: true },
                onOk: () => {
                  onDelete(segment.id)
                }
              })
            }
          }
        ]
      }}
    >
      <Space size="small">
        <Popover title={statusBadge.title} content={statusBadge.content}>
          <Badge status={statusBadge.status as any} />
        </Popover>
        <Tag bordered={false} color={segment.color} style={{ margin: 0 }}>
          {segment.name}
          {segment.users_count !== undefined && (
            <span style={{ marginLeft: '4px', opacity: 0.8 }}>
              (
              {numbro(segment.users_count).format({
                thousandSeparated: true,
                mantissa: 0
              })}
              )
            </span>
          )}
        </Tag>
      </Space>
    </Dropdown.Button>
  )
}

export function SegmentsFilter({
  workspaceId,
  segments,
  selectedSegmentIds = [],
  totalContacts,
  onSegmentToggle
}: SegmentsFilterProps) {
  const queryClient = useQueryClient()

  // Delete segment mutation
  const deleteSegmentMutation = useMutation({
    mutationFn: (segmentId: string) =>
      deleteSegment({
        workspace_id: workspaceId,
        id: segmentId
      }),
    onSuccess: () => {
      message.success('Segment deleted successfully')
      queryClient.invalidateQueries({ queryKey: ['segments', workspaceId] })
    },
    onError: (error: any) => {
      message.error(error?.message || 'Failed to delete segment')
    }
  })

  return (
    <div className="flex items-center gap-2 mb-6">
      <div className="text-sm font-medium">Segments:</div>
      <Space wrap>
        {segments.map((segment: Segment) => {
          const isSelected = selectedSegmentIds.includes(segment.id)

          return (
            <SegmentButton
              key={segment.id}
              segment={segment}
              workspaceId={workspaceId}
              isSelected={isSelected}
              totalContacts={totalContacts}
              onToggle={() => onSegmentToggle(segment.id)}
              onDelete={(segmentId) => deleteSegmentMutation.mutate(segmentId)}
            />
          )
        })}
        <ButtonUpsertSegment
          btnType="primary"
          btnSize="small"
          totalContacts={totalContacts}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['segments', workspaceId] })
          }}
        />
      </Space>
    </div>
  )
}
