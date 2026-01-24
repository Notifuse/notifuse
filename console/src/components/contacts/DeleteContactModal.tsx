import { Modal, Button } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface DeleteContactModalProps {
  visible: boolean
  onCancel: () => void
  onConfirm: () => void
  contactEmail: string
  loading?: boolean
  disabled?: boolean
}

export function DeleteContactModal({
  visible,
  onCancel,
  onConfirm,
  contactEmail,
  loading = false,
  disabled = false
}: DeleteContactModalProps) {
  const { t } = useLingui()

  return (
    <Modal
      title={t`Delete Contact`}
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel} disabled={loading}>
          {t`Cancel`}
        </Button>,
        <Button
          key="delete"
          type="primary"
          danger
          onClick={onConfirm}
          loading={loading}
          disabled={disabled}
        >
          {t`Delete`}
        </Button>
      ]}
      width={500}
    >
      <div className="space-y-4 mt-10 mb-10">
        <p className="text-gray-900">
          {t`Are you sure you want to delete`} <strong>{contactEmail}</strong>?
        </p>
        <div className="text-sm text-gray-600">
          <p>{t`This will permanently remove the contact and their subscriptions.`}</p>
          <p>
            {t`Message history and webhook events will be anonymized (email addresses redacted) but retained for analytics.`}
          </p>
          <p className="font-medium text-red-600">{t`This action cannot be undone.`}</p>
        </div>
      </div>
    </Modal>
  )
}
