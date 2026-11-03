import { Button } from 'antd'
import { useLingui } from '@lingui/react/macro'
import {
  OPT_IN_PERMISSION_RESOURCES,
  createEmptyPermissions,
  createFullPermissions,
  grantUnenforcedPermissions
} from '../../services/api/permissions'
import type { StoredPermissions, UserPermissions } from '../../services/api/permissions'

interface PermissionScopePresetsProps {
  // The set being edited. Grant all keeps its opt-in grants, which full access never includes.
  value: StoredPermissions | null | undefined
  onChange: (value: UserPermissions) => void
  // Without the rbac licence only full access can be saved, so Revoke all — which can only
  // produce a narrower set — is greyed, and Grant all stays the way back to full access.
  fullAccessOnly?: boolean
}

// Narrowly scoped keys are the common case for an integration — the Zapier onboarding asks for
// three resources out of the canonical list — and reaching one from the full-access default meant
// flipping every other switch by hand. These two set the floor the owner then edits.
export function PermissionScopePresets({
  value,
  onChange,
  fullAccessOnly = false
}: PermissionScopePresetsProps) {
  const { t } = useLingui()

  const grantAll = () => {
    const next = createFullPermissions()
    // An opt-in grant the set already holds is kept: replacing the map would strip Audit logs read
    // from a member who had it, without anybody choosing to.
    for (const resource of OPT_IN_PERMISSION_RESOURCES) {
      const granted = value?.[resource]
      if (granted) next[resource] = { ...granted }
    }
    onChange(next)
  }

  return (
    <div className="flex justify-end gap-1 mb-2">
      <Button size="small" type="link" onClick={grantAll}>
        {t`Grant all`}
      </Button>
      {/* Revoking still hands back the unenforceable verbs granted: a stored `false` there would
          never be widened by a backfill, which only ever adds the keys a row is missing. */}
      <Button
        size="small"
        type="link"
        disabled={fullAccessOnly}
        onClick={() => onChange(grantUnenforcedPermissions(createEmptyPermissions()))}
      >
        {t`Revoke all`}
      </Button>
    </div>
  )
}
