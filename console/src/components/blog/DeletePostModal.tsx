import { Modal, Typography } from 'antd'
import type { BlogPost } from '../../services/api/blog'

const { Paragraph } = Typography

interface DeletePostModalProps {
  open: boolean
  post: BlogPost | null
  onConfirm: () => void
  onCancel: () => void
  loading: boolean
}

export function DeletePostModal({
  open,
  post,
  onConfirm,
  onCancel,
  loading
}: DeletePostModalProps) {
  return (
    <Modal
      title="Delete Post"
      open={open}
      onOk={onConfirm}
      onCancel={onCancel}
      okText="Delete"
      okButtonProps={{ danger: true, loading }}
      cancelButtonProps={{ disabled: loading }}
    >
      <Paragraph>
        Are you sure you want to delete the post <strong>{post?.settings.title}</strong>?
      </Paragraph>
      <Paragraph type="secondary">This action cannot be undone.</Paragraph>
    </Modal>
  )
}


