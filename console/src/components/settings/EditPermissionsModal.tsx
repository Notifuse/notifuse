import { useState, useEffect } from 'react'
import { Modal, Button, Table, Switch, App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { WorkspaceMember, UserPermissions } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'

interface EditPermissionsModalProps {
  visible: boolean
  member: WorkspaceMember | null
  workspaceId: string
  onClose: () => void
  onSuccess: () => void
}

export function EditPermissionsModal({
  visible,
  member,
  workspaceId,
  onClose,
  onSuccess
}: EditPermissionsModalProps) {
  const { t } = useLingui()
  const [permissions, setPermissions] = useState<UserPermissions>({} as UserPermissions)
  const [saving, setSaving] = useState(false)
  const { message } = App.useApp()

  // Initialize permissions when modal opens
  useEffect(() => {
    if (member && visible) {
      // Use permissions from member data
      setPermissions(member.permissions)
    }
  }, [member, visible])

  const handleSavePermissions = async () => {
    if (!member) return

    setSaving(true)
    try {
      await workspaceService.setUserPermissions({
        workspace_id: workspaceId,
        user_id: member.user_id,
        permissions: permissions
      })

      message.success(t`Permissions updated successfully`)
      onSuccess()
      onClose()
    } catch (error) {
      console.error('Failed to update permissions', error)
      message.error(t`Failed to update permissions`)
    } finally {
      setSaving(false)
    }
  }

  const updatePermission = (resource: string, type: 'read' | 'write', value: boolean) => {
    setPermissions((prev) => ({
      ...prev,
      [resource]: {
        ...((prev as unknown as Record<string, { read: boolean; write: boolean }>)[resource]),
        [type]: value
      }
    }))
  }

  // Helper function to create permissions table data
  const createPermissionsTableData = (permissions: UserPermissions) => {
    return Object.entries(permissions).map(([resource, perms]) => ({
      key: resource,
      resource: resource.replace('_', ' ').replace(/\b\w/g, (l) => l.toUpperCase()),
      read: perms.read,
      write: perms.write
    }))
  }

  // Permissions table columns
  const permissionsColumns = [
    {
      title: t`Resource`,
      dataIndex: 'resource',
      key: 'resource',
      width: '40%'
    },
    {
      title: t`Read`,
      dataIndex: 'read',
      key: 'read',
      width: '30%',
      render: (value: boolean, record: { key: string }) => (
        <Switch
          checked={value}
          onChange={(checked) => updatePermission(record.key, 'read', checked)}
          size="small"
        />
      )
    },
    {
      title: t`Write`,
      dataIndex: 'write',
      key: 'write',
      width: '30%',
      render: (value: boolean, record: { key: string }) => (
        <Switch
          checked={value}
          onChange={(checked) => updatePermission(record.key, 'write', checked)}
          size="small"
        />
      )
    }
  ]

  return (
    <Modal
      title={t`Edit Permissions for ${member?.email}`}
      open={visible}
      onCancel={onClose}
      width={600}
      footer={[
        <Button key="cancel" onClick={onClose}>
          {t`Cancel`}
        </Button>,
        <Button key="save" type="primary" onClick={handleSavePermissions} loading={saving}>
          {t`Save Permissions`}
        </Button>
      ]}
    >
      <Table
        dataSource={createPermissionsTableData(permissions)}
        columns={permissionsColumns}
        pagination={false}
        size="small"
        className="border border-gray-200 rounded-md my-8"
      />
    </Modal>
  )
}
