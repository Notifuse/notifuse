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
  Modal,
  Tooltip
} from 'antd'
import { Undo2, Redo2 } from 'lucide-react'
import type { Automation } from '../../services/api/automation'
import type { Workspace } from '../../services/api/types'
import type { List } from '../../services/api/list'
import { AutomationProvider, useAutomation } from './context'
import { AutomationFlowEditor } from './AutomationFlowEditor'

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

// Inner component that uses the context
function DrawerContent({ onCloseDrawer }: { onCloseDrawer: () => void }) {
  const {
    isEditing,
    name,
    setName,
    listId,
    setListId,
    lists,
    hasUnsavedChanges,
    isSaving,
    save,
    validate,
    canUndo,
    canRedo,
    undo,
    redo
  } = useAutomation()

  const { modal } = App.useApp()

  // Keyboard shortcuts for undo/redo
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0
    const modifier = isMac ? e.metaKey : e.ctrlKey

    if (modifier && e.key === 'z' && !e.shiftKey) {
      e.preventDefault()
      if (canUndo) undo()
    } else if (modifier && e.key === 'z' && e.shiftKey) {
      e.preventDefault()
      if (canRedo) redo()
    } else if (modifier && e.key === 'y') {
      e.preventDefault()
      if (canRedo) redo()
    }
  }, [canUndo, canRedo, undo, redo])

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  const handleCloseWithConfirm = () => {
    if (hasUnsavedChanges) {
      modal.confirm({
        title: 'Unsaved Changes',
        content: 'You have unsaved changes. Are you sure you want to close?',
        okText: 'Close without saving',
        cancelText: 'Cancel',
        onOk: onCloseDrawer
      })
    } else {
      onCloseDrawer()
    }
  }

  const handleSubmit = async () => {
    // Validate name first
    if (!name.trim()) {
      modal.error({
        title: 'Validation Error',
        content: 'Please enter an automation name'
      })
      return
    }

    // Check for warnings
    const validationErrors = validate()
    const warnings = validationErrors.filter(e => e.message.startsWith('Warning:'))

    if (warnings.length > 0) {
      Modal.confirm({
        title: 'Warning',
        content: warnings.map(w => w.message).join('\n'),
        okText: 'Save Anyway',
        cancelText: 'Cancel',
        onOk: () => save()
      })
      return
    }

    await save()
  }

  return (
    <>
      {/* Header with title and actions */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200">
        <Space>
          <span className="text-lg font-medium">
            {isEditing ? 'Edit Automation' : 'Create Automation'}
          </span>
          {hasUnsavedChanges && (
            <Badge status="warning" text="Unsaved changes" />
          )}
        </Space>
        <Space>
          <Tooltip title="Undo (Ctrl+Z)">
            <Button
              type="text"
              icon={<Undo2 size={16} />}
              disabled={!canUndo}
              onClick={undo}
            />
          </Tooltip>
          <Tooltip title="Redo (Ctrl+Shift+Z)">
            <Button
              type="text"
              icon={<Redo2 size={16} />}
              disabled={!canRedo}
              onClick={redo}
            />
          </Tooltip>
          <Button onClick={handleCloseWithConfirm}>Cancel</Button>
          <Button
            type="primary"
            loading={isSaving}
            onClick={handleSubmit}
          >
            {isEditing ? 'Save Changes' : 'Create'}
          </Button>
        </Space>
      </div>

      {/* Form Header */}
      <div className="p-4 border-b border-gray-200 bg-white">
        <Form layout="inline">
          <Form.Item
            label="Name"
            required
            style={{ marginBottom: 0, minWidth: 300 }}
          >
            <Input
              placeholder="Enter automation name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Form.Item>
          <Form.Item
            label="List"
            style={{ marginBottom: 0, minWidth: 250 }}
          >
            <Select
              placeholder="Select list"
              value={listId}
              onChange={setListId}
              allowClear
              options={lists.map((list) => ({
                label: list.name,
                value: list.id
              }))}
            />
          </Form.Item>
        </Form>
      </div>

      {/* Flow Editor */}
      <div className="flex-1" style={{ height: 'calc(100vh - 180px)' }}>
        <AutomationFlowEditor />
      </div>
    </>
  )
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

  const isEditing = !!automation

  const handleOpen = () => {
    setIsOpen(true)
  }

  const handleClose = () => {
    setIsOpen(false)
    onClose?.()
  }

  const handleSaveSuccess = () => {
    handleClose()
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
        placement="right"
        width="100%"
        onClose={handleClose}
        open={isOpen}
        destroyOnClose
        closable={false}
        styles={{
          body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' }
        }}
      >
        {isOpen && (
          <AutomationProvider
            workspace={workspace}
            automation={automation}
            lists={lists}
            onSaveSuccess={handleSaveSuccess}
            onClose={handleClose}
          >
            <DrawerContent onCloseDrawer={handleClose} />
          </AutomationProvider>
        )}
      </Drawer>
    </>
  )
}
