import { useState, useEffect } from 'react'
import { Drawer, Button, Space, App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { WorkspaceMember, UserPermissions } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import {
  createEmptyPermissions,
  grantUnenforcedPermissions
} from '../../services/api/permissions'
import { PermissionsMatrix } from './PermissionsMatrix'

interface EditPermissionsDrawerProps {
  open: boolean
  member: WorkspaceMember | null
  workspaceId: string
  onClose: () => void
  onSuccess: () => void
}

export function EditPermissionsDrawer({
  open,
  member,
  workspaceId,
  onClose,
  onSuccess
}: EditPermissionsDrawerProps) {
  const { t } = useLingui()
  const [permissions, setPermissions] = useState<UserPermissions>(createEmptyPermissions)
  const [saving, setSaving] = useState(false)
  const { message } = App.useApp()

  // Initialize permissions when the drawer opens
  useEffect(() => {
    if (member && open) {
      // Use permissions from member data. The stored map may be partial or null, and a resource
      // it does not mention is denied — which is what the empty base spells out, and what keeps
      // the matrix from rendering a subset the owner could then never widen. The unenforceable
      // verbs are granted on the way in, so saving an untouched form cannot freeze them at false.
      setPermissions(
        grantUnenforcedPermissions({ ...createEmptyPermissions(), ...member.permissions })
      )
    }
  }, [member, open])

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

  return (
    // A drawer rather than a modal: the matrix carries fourteen expandable rows, and an expanded
    // one lists every endpoint the permission gates — far more than a centred dialog can hold.
    <Drawer
      title={t`Edit Permissions for ${member?.email}`}
      open={open}
      onClose={onClose}
      placement="right"
      size={720}
      styles={{ wrapper: { maxWidth: '100%' } }}
      footer={
        <div className="flex justify-end">
          <Space>
            <Button onClick={onClose}>{t`Cancel`}</Button>
            <Button type="primary" onClick={handleSavePermissions} loading={saving}>
              {t`Save Permissions`}
            </Button>
          </Space>
        </div>
      }
    >
      <PermissionsMatrix
        value={permissions}
        onChange={setPermissions}
        className="border border-gray-200 rounded-md"
      />
    </Drawer>
  )
}
