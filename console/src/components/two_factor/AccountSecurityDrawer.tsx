import { useState } from 'react'
import { Drawer } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { AccountSecurityPanel } from './AccountSecurityPanel'

export interface AccountSecurityDrawerProps {
  open: boolean
  onClose: () => void
}

/**
 * Account security over the workspace, opened from the account menu. It stays open while new
 * recovery codes are on screen and not yet marked as saved.
 */
export function AccountSecurityDrawer({ open, onClose }: AccountSecurityDrawerProps) {
  const { t } = useLingui()
  const [locked, setLocked] = useState(false)

  return (
    <Drawer
      open={open}
      onClose={locked ? undefined : onClose}
      closable={!locked}
      maskClosable={!locked}
      keyboard={!locked}
      title={t`Account security`}
      size={560}
      destroyOnHidden
    >
      <AccountSecurityPanel onLockChange={setLocked} />
    </Drawer>
  )
}
