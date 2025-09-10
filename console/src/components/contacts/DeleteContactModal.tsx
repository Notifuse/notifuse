import { Modal, Button } from 'antd'

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
  return (
    <Modal
      title="Delete Contact"
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel} disabled={loading}>
          Cancel
        </Button>,
        <Button
          key="delete"
          type="primary"
          danger
          onClick={onConfirm}
          loading={loading}
          disabled={disabled}
        >
          Delete
        </Button>
      ]}
      width={500}
    >
      <div className="space-y-4 mt-10 mb-10">
        <p className="text-gray-900">
          Are you sure you want to delete <strong>{contactEmail}</strong>?
        </p>
        <div className="text-sm text-gray-600">
          <p>This will permanently remove the contact and their subscriptions.</p>
          <p>
            Message history and webhook events will be anonymized (email addresses redacted) but
            retained for analytics.
          </p>
          <p className="font-medium text-red-600">This action cannot be undone.</p>
        </div>
      </div>
    </Modal>
  )
}
