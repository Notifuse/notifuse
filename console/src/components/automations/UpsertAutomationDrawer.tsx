import React, { useState, useEffect, useCallback } from 'react'
import {
  Button,
  Drawer,
  Form,
  Input,
  Select,
  Space,
  App,
  Badge,
  Modal
} from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  type Node,
  type Edge,
  type NodeChange,
  type EdgeChange,
  type Connection,
  applyNodeChanges,
  applyEdgeChanges,
  addEdge
} from '@xyflow/react'
import {
  automationApi,
  type Automation,
  type AutomationNode
} from '../../services/api/automation'
import type { Workspace } from '../../services/api/types'
import type { List } from '../../services/api/list'
import { AutomationFlowEditor } from './AutomationFlowEditor'
import {
  createInitialFlow,
  automationToFlow,
  flowToAutomationNodes,
  buildTriggerConfig,
  findRootNodeId,
  validateFlow,
  generateId,
  getNodeLabel,
  type AutomationNodeData
} from './utils/flowConverter'
import type { NodeType } from '../../services/api/automation'

interface UpsertAutomationDrawerProps {
  workspace: Workspace
  automation?: Automation
  buttonProps?: Record<string, unknown>
  buttonContent?: React.ReactNode
  onClose?: () => void
  lists?: List[]
  // Controlled mode props
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

export function UpsertAutomationDrawer({
  workspace,
  automation,
  buttonProps = {},
  buttonContent,
  onClose,
  lists = [],
  open: controlledOpen,
  onOpenChange
}: UpsertAutomationDrawerProps) {
  const [internalOpen, setInternalOpen] = useState(false)

  // Support both controlled and uncontrolled modes
  const isControlled = controlledOpen !== undefined
  const isOpen = isControlled ? controlledOpen : internalOpen

  const setIsOpen = (newOpen: boolean) => {
    if (isControlled) {
      onOpenChange?.(newOpen)
    } else {
      setInternalOpen(newOpen)
    }
  }
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)
  const { message, modal } = App.useApp()
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)

  // Flow state
  const [nodes, setNodes] = useState<Node<AutomationNodeData>[]>([])
  const [edges, setEdges] = useState<Edge[]>([])
  const [initialTriggerNodeId, setInitialTriggerNodeId] = useState<string | undefined>(undefined)

  const isEditing = !!automation

  // Initialize flow when opening
  useEffect(() => {
    if (isOpen) {
      if (automation) {
        // Load existing automation (don't auto-select)
        const { nodes: flowNodes, edges: flowEdges } = automationToFlow(automation)
        setNodes(flowNodes)
        setEdges(flowEdges)
        setInitialTriggerNodeId(undefined)
        form.setFieldsValue({
          name: automation.name,
          list_id: automation.list_id
        })
      } else {
        // New automation - start with trigger only (no exit node needed)
        const { nodes: initialNodes, edges: initialEdges } = createInitialFlow()
        setNodes(initialNodes)
        setEdges(initialEdges)
        // Auto-select trigger node for new automations
        const triggerNode = initialNodes.find((n) => n.data.nodeType === 'trigger')
        setInitialTriggerNodeId(triggerNode?.id)
        form.resetFields()
      }
      setHasUnsavedChanges(false)
    }
  }, [isOpen, automation, form])

  // ReactFlow change handlers (controlled component pattern)
  const handleNodesChange = useCallback(
    (changes: NodeChange<Node<AutomationNodeData>>[]) => {
      setNodes((nds) => applyNodeChanges(changes, nds))
      // Only mark as unsaved for non-selection changes
      if (changes.some((c) => c.type !== 'select')) {
        setHasUnsavedChanges(true)
      }
    },
    []
  )

  const handleEdgesChange = useCallback(
    (changes: EdgeChange<Edge>[]) => {
      setEdges((eds) => applyEdgeChanges(changes, eds))
      setHasUnsavedChanges(true)
    },
    []
  )

  const handleConnect = useCallback((params: Connection) => {
    setEdges((eds) => addEdge({ ...params, type: 'smoothstep' }, eds))
    setHasUnsavedChanges(true)
  }, [])

  const handleAddNode = useCallback(
    (type: NodeType, position: { x: number; y: number }) => {
      const newNode: Node<AutomationNodeData> = {
        id: generateId(),
        type,
        position,
        data: {
          nodeType: type,
          config: type === 'delay' ? { duration: 0, unit: 'minutes' } : {},
          label: getNodeLabel(type)
        }
      }
      setNodes((nds) => [...nds, newNode])
      setHasUnsavedChanges(true)
    },
    []
  )

  // Add node with edge from source node (used by plus button)
  const handleAddNodeWithEdge = useCallback(
    (sourceNodeId: string, type: NodeType, position: { x: number; y: number }) => {
      const newNodeId = generateId()
      const newNode: Node<AutomationNodeData> = {
        id: newNodeId,
        type,
        position,
        data: {
          nodeType: type,
          config: type === 'delay' ? { duration: 0, unit: 'minutes' } : {},
          label: getNodeLabel(type)
        }
      }
      const newEdge: Edge = {
        id: `${sourceNodeId}-${newNodeId}`,
        source: sourceNodeId,
        target: newNodeId,
        type: 'smoothstep'
      }
      setNodes((nds) => [...nds, newNode])
      setEdges((eds) => [...eds, newEdge])
      setHasUnsavedChanges(true)
    },
    []
  )

  const handleNodeUpdate = useCallback(
    (nodeId: string, data: Partial<AutomationNodeData>) => {
      setNodes((nds) =>
        nds.map((n) => (n.id === nodeId ? { ...n, data: { ...n.data, ...data } } : n))
      )
      setHasUnsavedChanges(true)
    },
    []
  )

  // Create mutation
  const createMutation = useMutation({
    mutationFn: (data: { workspace_id: string; automation: Automation }) =>
      automationApi.create(data),
    onSuccess: () => {
      message.success('Automation created successfully')
      queryClient.invalidateQueries({ queryKey: ['automations', workspace.id] })
      handleClose()
    },
    onError: (error: Error) => {
      message.error(`Failed to create automation: ${error.message}`)
    }
  })

  // Update mutation
  const updateMutation = useMutation({
    mutationFn: (data: { workspace_id: string; automation: Automation }) =>
      automationApi.update(data),
    onSuccess: () => {
      message.success('Automation updated successfully')
      queryClient.invalidateQueries({ queryKey: ['automations', workspace.id] })
      handleClose()
    },
    onError: (error: Error) => {
      message.error(`Failed to update automation: ${error.message}`)
    }
  })

  const handleOpen = () => {
    setIsOpen(true)
  }

  const handleClose = () => {
    setIsOpen(false)
    setHasUnsavedChanges(false)
    form.resetFields()
    setNodes([])
    setEdges([])
    onClose?.()
  }

  const handleCloseWithConfirm = () => {
    if (hasUnsavedChanges) {
      modal.confirm({
        title: 'Unsaved Changes',
        content: 'You have unsaved changes. Are you sure you want to close?',
        okText: 'Close without saving',
        cancelText: 'Cancel',
        onOk: handleClose
      })
    } else {
      handleClose()
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()

      // Validate flow
      const validationErrors = validateFlow(nodes, edges)
      const errors = validationErrors.filter(e => !e.message.startsWith('Warning:'))
      const warnings = validationErrors.filter(e => e.message.startsWith('Warning:'))

      if (errors.length > 0) {
        message.error(errors[0].message)
        return
      }

      if (warnings.length > 0) {
        // Show warning but allow save
        Modal.confirm({
          title: 'Warning',
          content: warnings.map(w => w.message).join('\n'),
          okText: 'Save Anyway',
          cancelText: 'Cancel',
          onOk: () => saveAutomation(values)
        })
        return
      }

      await saveAutomation(values)
    } catch (error) {
      console.error('Validation failed:', error)
    }
  }

  const saveAutomation = async (values: { name: string; list_id: string }) => {
    setLoading(true)
    try {
      const automationId = automation?.id || `auto_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`

      // Convert flow to automation nodes
      const automationNodes: AutomationNode[] = flowToAutomationNodes(nodes, edges, automationId)

      // Build trigger config from trigger node
      const triggerConfig = buildTriggerConfig(nodes)

      // Find root node ID
      const rootNodeId = findRootNodeId(nodes)

      const automationData: Automation = {
        id: automationId,
        workspace_id: workspace.id,
        name: values.name,
        status: automation?.status || 'draft',
        list_id: values.list_id,
        trigger: triggerConfig,
        root_node_id: rootNodeId,
        nodes: automationNodes,
        created_at: automation?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      }

      if (isEditing) {
        await updateMutation.mutateAsync({
          workspace_id: workspace.id,
          automation: automationData
        })
      } else {
        await createMutation.mutateAsync({
          workspace_id: workspace.id,
          automation: automationData
        })
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      {/* Only show button in uncontrolled mode */}
      {!isControlled && (
        <Button type="primary" onClick={handleOpen} {...buttonProps}>
          {buttonContent || (isEditing ? 'Edit' : 'Create Automation')}
        </Button>
      )}

      <Drawer
        title={
          <Space>
            <span>{isEditing ? 'Edit Automation' : 'Create Automation'}</span>
            {hasUnsavedChanges && (
              <Badge status="warning" text="Unsaved changes" />
            )}
          </Space>
        }
        placement="right"
        width="100%"
        onClose={handleCloseWithConfirm}
        open={isOpen}
        destroyOnClose
        styles={{
          body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' }
        }}
        extra={
          <Space>
            <Button onClick={handleCloseWithConfirm}>Cancel</Button>
            <Button
              type="primary"
              loading={loading}
              onClick={handleSubmit}
            >
              {isEditing ? 'Save Changes' : 'Create'}
            </Button>
          </Space>
        }
      >
        {/* Header Form */}
        <div className="p-4 border-b border-gray-200 bg-white">
          <Form form={form} layout="inline">
            <Form.Item
              name="name"
              label="Name"
              rules={[{ required: true, message: 'Please enter automation name' }]}
              style={{ marginBottom: 0, minWidth: 300 }}
            >
              <Input placeholder="Enter automation name" />
            </Form.Item>
            <Form.Item
              name="list_id"
              label="List"
              rules={[{ required: true, message: 'Please select a list' }]}
              style={{ marginBottom: 0, minWidth: 250 }}
            >
              <Select
                placeholder="Select list"
                options={lists.map((list) => ({
                  label: list.name,
                  value: list.id
                }))}
              />
            </Form.Item>
          </Form>
        </div>

        {/* Flow Editor */}
        <div className="flex-1" style={{ height: 'calc(100vh - 130px)' }}>
          <AutomationFlowEditor
            nodes={nodes}
            edges={edges}
            onNodesChange={handleNodesChange}
            onEdgesChange={handleEdgesChange}
            onConnect={handleConnect}
            onAddNode={handleAddNode}
            onNodeUpdate={handleNodeUpdate}
            onAddNodeWithEdge={handleAddNodeWithEdge}
            workspaceId={workspace.id}
            initialSelectedNodeId={initialTriggerNodeId}
          />
        </div>
      </Drawer>
    </>
  )
}
