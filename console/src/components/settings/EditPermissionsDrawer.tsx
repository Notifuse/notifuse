import { useState, useEffect } from 'react'
import { Drawer, Button, Space, App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { WorkspaceMember, UserPermissions } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import {
  createEmptyPermissions,
  grantUnenforcedPermissions,
  grantsFullPermissions
} from '../../services/api/permissions'
import { PermissionsMatrix } from './PermissionsMatrix'
import { PermissionScopePresets } from './PermissionScopePresets'
import { useLicense } from '../../hooks/useLicense'
import { LicenceGateNotice } from '../license/LicenceGateNotice'

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
  // Advisory, like every licence read in the console: the server still answers 402, and the
  // catch below still shows its sentence. Without the rbac licence the server accepts full access
  // and refuses anything narrower, so the editor offers exactly that: Grant all stays live, the
  // choices that would narrow the set are greyed, and Save waits until the set is full access —
  // which is how a member holding a restricted set is put back on full access once the key lapsed.
  const { has } = useLicense()
  const locked = !has('rbac')
  const savable = !locked || grantsFullPermissions(permissions)

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
      // The server's sentence, not a fixed one. client.ts has already turned a 403 into a
      // permission message and a 402 into the licence one by the time it lands here, and
      // this is the surface where that matters most: granular permissions are a licensed
      // capability and this drawer is the capability. Discarding the message showed the
      // single most-hit paid gate as "Failed to update permissions" — a bug report rather
      // than something to buy, with nothing anywhere else in the console mentioning a
      // licence. The same shape WorkspaceMembers and Integrations already use.
      message.error(
        error instanceof Error && error.message ? error.message : t`Failed to update permissions`
      )
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
            <Button
              type="primary"
              onClick={handleSavePermissions}
              loading={saving}
              disabled={!savable}
            >
              {t`Save Permissions`}
            </Button>
          </Space>
        </div>
      }
    >
      <LicenceGateNotice feature="rbac" workspaceId={workspaceId} />
      <PermissionScopePresets
        value={permissions}
        onChange={setPermissions}
        fullAccessOnly={locked}
      />
      <PermissionsMatrix
        value={permissions}
        onChange={setPermissions}
        fullAccessOnly={locked}
        className="border border-gray-200 rounded-md"
      />
    </Drawer>
  )
}
