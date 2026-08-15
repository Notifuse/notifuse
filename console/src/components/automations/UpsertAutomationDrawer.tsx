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
  Switch,
  Tooltip
} from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { Undo2, Redo2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { type Automation, supportsInboundReplies } from '../../services/api/automation'
import type { Workspace, Template } from '../../services/api/types'
import type { List } from '../../services/api/list'
import type { Segment } from '../../services/api/segment'
import { AutomationProvider, useAutomation } from './context'
import { AutomationFlowEditor } from './AutomationFlowEditor'

interface UpsertAutomationDrawerProps {
  workspace: Workspace
  automation?: Automation
  buttonProps?: Record<string, unknown>
  buttonContent?: React.ReactNode
  onClose?: () => void
  lists?: List[]
  segments?: Segment[]
  templates?: Template[]
  // Controlled mode props
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// Inner component that uses the context
function DrawerContent({ onCloseDrawer }: { onCloseDrawer: () => void }) {
  const { t } = useLingui()
  const {
    isEditing,
    name,
    setName,
    listId,
    setListId,
    exitOnReply,
    setExitOnReply,
    lists,
    workspace,
    hasUnsavedChanges,
    isSaving,
    save,
    validate,
    canUndo,
    canRedo,
    undo,
    redo
  } = useAutomation()

  // "Exit on reply" needs an email provider that can ingest inbound replies. Gate the toggle
  // on the workspace having at least one such integration. For SES this is also region-aware:
  // inbound only works in receiving-capable regions, so a sending-only-region SES integration
  // does not enable the toggle (it would never fire).
  // Integration.email_provider is optional on the client type and supportsInboundReplies
  // dereferences it on its first line, so it has to be checked for before it is inspected.
  const hasInboundIntegration = (workspace?.integrations || []).some(
    (i) => !!i.email_provider && supportsInboundReplies(i.email_provider)
  )

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
        title: t`Unsaved Changes`,
        content: t`You have unsaved changes. Are you sure you want to close?`,
        okText: t`Close without saving`,
        cancelText: t`Cancel`,
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
        title: t`Validation Error`,
        content: t`Please enter an automation name`
      })
      return
    }

    // Check for warnings
    const validationErrors = validate()
    const warnings = validationErrors.filter(e => e.message.startsWith('Warning:'))

    if (warnings.length > 0) {
      Modal.confirm({
        title: t`Warning`,
        content: warnings.map(w => w.message).join('\n'),
        okText: t`Save Anyway`,
        cancelText: t`Cancel`,
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
            {isEditing ? t`Edit Automation` : t`Create Automation`}
          </span>
          {hasUnsavedChanges && (
            <Badge status="warning" text={t`Unsaved changes`} />
          )}
        </Space>
        <Space>
          <Tooltip title={t`Undo (Ctrl+Z)`}>
            <Button
              type="text"
              icon={<Undo2 size={16} />}
              disabled={!canUndo}
              onClick={undo}
            />
          </Tooltip>
          <Tooltip title={t`Redo (Ctrl+Shift+Z)`}>
            <Button
              type="text"
              icon={<Redo2 size={16} />}
              disabled={!canRedo}
              onClick={redo}
            />
          </Tooltip>
          <Button onClick={handleCloseWithConfirm}>{t`Cancel`}</Button>
          <Button
            type="primary"
            loading={isSaving}
            onClick={handleSubmit}
          >
            {isEditing ? t`Save Changes` : t`Create`}
          </Button>
        </Space>
      </div>

      {/* Form Header */}
      <div className="p-4 border-b border-gray-200 bg-white">
        <Form layout="inline">
          <Form.Item
            label={t`Name`}
            required
            style={{ marginBottom: 0, minWidth: 300 }}
          >
            <Input
              placeholder={t`Enter automation name`}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Form.Item>
          <Form.Item
            label={t`List`}
            style={{ marginBottom: 0, minWidth: 250 }}
          >
            <Select
              placeholder={t`Select list`}
              value={listId}
              onChange={setListId}
              allowClear
              options={lists.map((list) => ({
                label: list.name,
                value: list.id
              }))}
            />
          </Form.Item>
          <Form.Item
            label={
              <Space size={4}>
                {t`Exit on reply`}
                <Tooltip
                  title={
                    hasInboundIntegration
                      ? t`Stops this automation for a contact as soon as they reply to one of its emails. This requires inbound reply forwarding to be configured at your email provider (ESP): replies to your sending domain must be routed to the provider, which forwards them to Notifuse. Without that setup, replies aren't detected and the sequence won't stop.`
                      : t`Available once you connect an email provider that supports inbound replies (currently Mailgun and Amazon SES — more providers coming soon). It stops this automation for a contact as soon as they reply to one of its emails.`
                  }
                >
                  <InfoCircleOutlined style={{ color: '#8c8c8c' }} />
                </Tooltip>
              </Space>
            }
            style={{ marginBottom: 0 }}
          >
            <Switch
              checked={exitOnReply && hasInboundIntegration}
              onChange={setExitOnReply}
              disabled={!hasInboundIntegration}
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
  segments = [],
  templates = [],
  open: controlledOpen,
  onOpenChange
}: UpsertAutomationDrawerProps) {
  const { t } = useLingui()
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
          {buttonContent || (isEditing ? t`Edit` : t`Create Automation`)}
        </Button>
      )}

      <Drawer
        placement="right"
        size="100%"
        onClose={handleClose}
        open={isOpen}
        destroyOnHidden
        closable={false}
        // The condition editors inside the canvas are drawers of their own, and rc-drawer
        // pushes a parent aside when a child opens — which would slide this full-screen
        // editor 180px off. The transform is applied by the parent, so this is the only
        // place it can be switched off; setting push on the child does nothing.
        push={false}
        styles={{
          body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' }
        }}
      >
        {isOpen && (
          <AutomationProvider
            workspace={workspace}
            automation={automation}
            lists={lists}
            segments={segments}
            templates={templates}
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
