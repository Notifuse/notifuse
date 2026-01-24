import { useLingui } from '@lingui/react/macro'
import { Modal, Typography } from 'antd'
import type { BlogCategory } from '../../services/api/blog'

const { Paragraph } = Typography

interface DeleteCategoryModalProps {
  open: boolean
  category: BlogCategory | null
  onConfirm: () => void
  onCancel: () => void
  loading: boolean
}

export function DeleteCategoryModal({
  open,
  category,
  onConfirm,
  onCancel,
  loading
}: DeleteCategoryModalProps) {
  const { t } = useLingui()
  return (
    <Modal
      title={t`Delete Category`}
      open={open}
      onOk={onConfirm}
      onCancel={onCancel}
      okText={t`Delete`}
      okButtonProps={{ danger: true, loading }}
      cancelButtonProps={{ disabled: loading }}
    >
      <Paragraph>
        {t`Are you sure you want to delete the category`} <strong>{category?.settings.name}</strong>?
      </Paragraph>
      <Paragraph type="secondary">
        {t`Both the category and all its associated posts will be unpublished from the web.`}
      </Paragraph>
    </Modal>
  )
}
