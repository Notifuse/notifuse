import { Button, Tooltip } from 'antd'
import type { ButtonProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPaperPlane } from '@fortawesome/free-regular-svg-icons'
import { canSendTestEmail, type StoredPermissions } from '../../services/api/permissions'

interface SendTestEmailButtonProps {
  permissions: StoredPermissions | null | undefined
  onClick: () => void
  size?: ButtonProps['size']
}

// The icon button that opens the test email dialog, enabled only for members the
// transactional.testTemplate endpoint lets send.
export default function SendTestEmailButton({ permissions, onClick, size }: SendTestEmailButtonProps) {
  const { t } = useLingui()
  const allowed = canSendTestEmail(permissions)

  return (
    <Tooltip
      title={
        allowed
          ? t`Send Test Email`
          : t`You need Templates read and Transactional write permissions to send test emails`
      }
    >
      <Button
        size={size}
        type="text"
        icon={<FontAwesomeIcon icon={faPaperPlane} className="opacity-70" />}
        onClick={onClick}
        disabled={!allowed}
      />
    </Tooltip>
  )
}
