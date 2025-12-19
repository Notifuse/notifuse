import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Row, Col, Typography, Space, App, Empty, Pagination } from 'antd'
import { useParams } from '@tanstack/react-router'
import { useState } from 'react'
import { PlusOutlined } from '@ant-design/icons'
import { automationApi, Automation } from '../services/api/automation'
import { listsApi } from '../services/api/list'
import { listSegments } from '../services/api/segment'
import { useWorkspacePermissions, useAuth } from '../contexts/AuthContext'
import { AutomationCard } from '../components/automations/AutomationCard'
import { UpsertAutomationDrawer } from '../components/automations/UpsertAutomationDrawer'

const { Title } = Typography

export function AutomationsPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { permissions } = useWorkspacePermissions(workspaceId)
  const { workspaces } = useAuth()
  const queryClient = useQueryClient()
  const { message } = App.useApp()

  // Get current workspace
  const currentWorkspace = workspaces.find((w) => w.id === workspaceId)

  // State for editing automation
  const [editingAutomation, setEditingAutomation] = useState<Automation | undefined>(undefined)

  const [currentPage, setCurrentPage] = useState(1)
  const pageSize = 10

  // Fetch automations
  const {
    data: automationsData,
    isLoading: isLoadingAutomations,
    error: automationsError
  } = useQuery({
    queryKey: ['automations', workspaceId, currentPage, pageSize],
    queryFn: () =>
      automationApi.list({
        workspace_id: workspaceId,
        limit: pageSize,
        offset: (currentPage - 1) * pageSize
      }),
    enabled: !!workspaceId
  })

  // Fetch lists for reference
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: !!workspaceId
  })

  // Fetch segments for reference
  const { data: segmentsData } = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => listSegments({ workspace_id: workspaceId }),
    enabled: !!workspaceId
  })

  const automations = automationsData?.automations || []
  const totalAutomations = automationsData?.total || 0
  const lists = listsData?.lists || []
  const segments = segmentsData?.segments || []

  // Handle activate automation
  const handleActivate = async (automation: Automation) => {
    try {
      await automationApi.activate({
        workspace_id: workspaceId,
        automation_id: automation.id
      })
      message.success('Automation activated successfully')
      queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
    } catch (error) {
      console.error('Failed to activate automation:', error)
      message.error('Failed to activate automation')
    }
  }

  // Handle pause automation
  const handlePause = async (automation: Automation) => {
    try {
      await automationApi.pause({
        workspace_id: workspaceId,
        automation_id: automation.id
      })
      message.success('Automation paused successfully')
      queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
    } catch (error) {
      console.error('Failed to pause automation:', error)
      message.error('Failed to pause automation')
    }
  }

  // Handle delete automation
  const handleDelete = async (automation: Automation) => {
    try {
      await automationApi.delete({
        workspace_id: workspaceId,
        automation_id: automation.id
      })
      message.success('Automation deleted successfully')
      queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
    } catch (error) {
      console.error('Failed to delete automation:', error)
      message.error('Failed to delete automation')
    }
  }

  // Handle edit automation
  const handleEdit = (automation: Automation) => {
    setEditingAutomation(automation)
  }

  // Handle edit drawer close
  const handleEditClose = () => {
    setEditingAutomation(undefined)
  }

  // Handle page change
  const handlePageChange = (page: number) => {
    setCurrentPage(page)
    // Scroll to top smoothly
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  if (automationsError) {
    return (
      <div className="p-6">
        <Title level={4}>Error loading automations</Title>
        <p className="text-red-500">{String(automationsError)}</p>
      </div>
    )
  }

  return (
    <div>
      <Row justify="space-between" align="middle" className="mb-6">
        <Col>
          <Title level={4} style={{ margin: 0 }}>
            Automations
          </Title>
        </Col>
        <Col>
          <Space>
            {currentWorkspace && (
              <UpsertAutomationDrawer
                workspace={currentWorkspace}
                lists={lists}
                segments={segments}
                buttonProps={{
                  type: 'primary',
                  icon: <PlusOutlined />,
                  disabled: !permissions?.automations?.write
                }}
                buttonContent="Create Automation"
              />
            )}
          </Space>
        </Col>
      </Row>

      {isLoadingAutomations ? (
        <div className="text-center py-12 text-gray-500">Loading automations...</div>
      ) : automations.length === 0 ? (
        <Empty
          description="No automations yet"
          className="py-12"
        >
          {currentWorkspace && (
            <UpsertAutomationDrawer
              workspace={currentWorkspace}
              lists={lists}
              segments={segments}
              buttonProps={{
                type: 'primary',
                icon: <PlusOutlined />,
                disabled: !permissions?.automations?.write
              }}
              buttonContent="Create your first automation"
            />
          )}
        </Empty>
      ) : (
        <>
          {automations.map((automation) => (
            <AutomationCard
              key={automation.id}
              automation={automation}
              lists={lists}
              segments={segments}
              permissions={permissions}
              onActivate={handleActivate}
              onPause={handlePause}
              onDelete={handleDelete}
              onEdit={handleEdit}
            />
          ))}

          {totalAutomations > pageSize && (
            <div className="flex justify-center mt-6">
              <Pagination
                current={currentPage}
                total={totalAutomations}
                pageSize={pageSize}
                onChange={handlePageChange}
                showSizeChanger={false}
                showTotal={(total, range) => `${range[0]}-${range[1]} of ${total} automations`}
              />
            </div>
          )}
        </>
      )}

      {/* Edit Automation Drawer (controlled) */}
      {currentWorkspace && editingAutomation && (
        <UpsertAutomationDrawer
          workspace={currentWorkspace}
          automation={editingAutomation}
          lists={lists}
          segments={segments}
          open={!!editingAutomation}
          onOpenChange={(open) => {
            if (!open) handleEditClose()
          }}
          onClose={handleEditClose}
        />
      )}
    </div>
  )
}
