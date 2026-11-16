import { useLingui } from '@lingui/react/macro'
import { Button, Form, Input, Modal, Popconfirm, Tooltip } from 'antd'
import { Pencil, Plus, X } from 'lucide-react'
import { useState } from 'react'
import type { BlogAuthor } from '../../services/api/blog'
import { ImageURLInput } from '../common/ImageURLInput'

const initials = (name: string) =>
  name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')

/** The author's photo, or their initials on the light primary colour. */
function AuthorAvatar({ author }: { author: BlogAuthor }) {
  // The address that failed, so a new one is tried again.
  const [failed, setFailed] = useState<string | null>(null)
  if (author.avatar_url && failed !== author.avatar_url) {
    return (
      <img
        src={author.avatar_url}
        alt=""
        onError={() => setFailed(author.avatar_url ?? null)}
        className="size-8 shrink-0 rounded-full object-cover"
      />
    )
  }
  return (
    <span
      aria-hidden="true"
      className="grid size-8 shrink-0 place-items-center rounded-full bg-primary-100 text-xs font-semibold text-primary-700"
    >
      {initials(author.name) || '?'}
    </span>
  )
}

interface AuthorsFieldProps {
  value?: BlogAuthor[]
  onChange?: (value: BlogAuthor[]) => void
}

/** The post's authors as fixed-width cards in a row, the last one adding an author. */
export function AuthorsField({ value = [], onChange }: AuthorsFieldProps) {
  const { t } = useLingui()
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [editForm] = Form.useForm()

  const handleAdd = () => {
    setEditingIndex(null)
    editForm.resetFields()
    editForm.setFieldsValue({ name: '', avatar_url: '' })
    setIsModalOpen(true)
  }

  const handleDelete = (index: number) => {
    const newValue = value.filter((_, i) => i !== index)
    onChange?.(newValue)
  }

  const handleEdit = (index: number) => {
    setEditingIndex(index)
    editForm.setFieldsValue(value[index])
    setIsModalOpen(true)
  }

  const handleOk = async () => {
    try {
      const values = await editForm.validateFields()

      if (editingIndex !== null) {
        // Edit existing author
        const newValue = [...value]
        newValue[editingIndex] = values
        onChange?.(newValue)
      } else {
        // Add new author
        onChange?.([...value, values])
      }

      setIsModalOpen(false)
      setEditingIndex(null)
      editForm.resetFields()
    } catch {
      // Validation failed
    }
  }

  const handleCancel = () => {
    setIsModalOpen(false)
    setEditingIndex(null)
    editForm.resetFields()
  }

  return (
    <>
      <div className="flex flex-wrap gap-2">
        {value.map((author, index) => {
          const name = author.name
          return (
          <div
            key={index}
            data-author-card
            className="group relative flex h-[52px] w-[200px] items-center rounded-lg border border-line bg-surface hover:border-primary-500 focus-within:border-primary-500"
          >
            <button
              type="button"
              onClick={() => handleEdit(index)}
              aria-label={t`Edit ${name}`}
              className="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-lg border-0 bg-transparent py-0 pr-1 pl-2.5 text-left focus-visible:outline-none"
            >
              <AuthorAvatar author={author} />
              <Tooltip title={author.name}>
                <span className="truncate text-sm font-medium text-ink">
                  {author.name || <em className="text-ink-4">{t`No name`}</em>}
                </span>
              </Tooltip>
            </button>
            {/* On hover or keyboard focus, as the card itself already edits. */}
            <div className="invisible flex shrink-0 items-center gap-0.5 pr-2 group-focus-within:visible group-hover:visible">
              <Button
                type="text"
                size="small"
                aria-label={t`Edit ${name}`}
                icon={<Pencil size={13} aria-hidden="true" />}
                onClick={() => handleEdit(index)}
              />
              <Popconfirm
                title={t`Remove this author?`}
                description={t`Are you sure you want to remove this author from the post?`}
                onConfirm={() => handleDelete(index)}
                okText={t`Yes`}
                cancelText={t`No`}
              >
                <Button
                  type="text"
                  size="small"
                  aria-label={t`Remove ${name}`}
                  icon={<X size={13} aria-hidden="true" />}
                />
              </Popconfirm>
            </div>
          </div>
          )
        })}
        <button
          type="button"
          onClick={handleAdd}
          className="flex h-[52px] w-[200px] cursor-pointer items-center justify-center gap-1.5 rounded-lg border border-dashed border-line bg-subtle text-sm font-medium text-primary-600 hover:border-primary-500 focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none"
        >
          <Plus size={14} aria-hidden="true" />
          {t`Add author`}
        </button>
      </div>

      <Modal
        title={editingIndex !== null ? t`Edit Author` : t`Add Author`}
        open={isModalOpen}
        onOk={handleOk}
        onCancel={handleCancel}
        okText={editingIndex !== null ? t`Save` : t`Add`}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item
            name="name"
            label={t`Name`}
            rules={[{ required: true, message: t`Author name is required` }]}
          >
            <Input placeholder={t`Enter author name`} />
          </Form.Item>
          <Form.Item name="avatar_url" label={t`Avatar URL`}>
            <ImageURLInput placeholder={t`Enter avatar URL (optional)`} buttonText={t`Select Avatar`} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
