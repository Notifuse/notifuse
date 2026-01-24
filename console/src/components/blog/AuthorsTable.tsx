import { useLingui } from '@lingui/react/macro'
import { Avatar, Button, Form, Input, Space, Table, Modal, Popconfirm, Tooltip } from 'antd'
import { EditOutlined, DeleteOutlined, PlusOutlined, UserOutlined } from '@ant-design/icons'
import { useState } from 'react'
import type { BlogAuthor } from '../../services/api/blog'
import { ImageURLInput } from '../common/ImageURLInput'

interface AuthorsTableProps {
  value?: BlogAuthor[]
  onChange?: (value: BlogAuthor[]) => void
}

export function AuthorsTable({ value = [], onChange }: AuthorsTableProps) {
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

  const columns = [
    {
      key: 'avatar',
      width: 60,
      render: (_: unknown, record: BlogAuthor) => (
        <Avatar src={record.avatar_url} icon={<UserOutlined />} />
      )
    },
    {
      key: 'name',
      render: (_: unknown, record: BlogAuthor) => (
        <div>
          <div className="font-medium">
            {record.name || <em className="text-gray-400">{t`No name`}</em>}
          </div>
          {record.avatar_url && (
            <Tooltip title={record.avatar_url}>
              <div className="text-xs text-gray-500 truncate mt-1" style={{ maxWidth: 200 }}>
                {record.avatar_url}
              </div>
            </Tooltip>
          )}
        </div>
      )
    },
    {
      key: 'actions',
      width: 100,
      align: 'right' as const,
      render: (_: unknown, _record: BlogAuthor, index: number) => (
        <Space className="flex justify-end">
          <Button
            type="text"
            size="small"
            icon={<EditOutlined />}
            onClick={() => handleEdit(index)}
          />
          <Popconfirm
            title={t`Remove this author?`}
            description={t`Are you sure you want to remove this author from the post?`}
            onConfirm={() => handleDelete(index)}
            okText={t`Yes`}
            cancelText={t`No`}
          >
            <Button type="text" size="small" icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      )
    }
  ]

  return (
    <>
      {value.length > 0 && (
        <Table
          columns={columns}
          dataSource={value}
          pagination={false}
          showHeader={false}
          rowKey={(_, index) => index?.toString() || '0'}
          size="small"
          className="authors-table mb-2 bg-white rounded-lg"
        />
      )}
      <Button type="primary" ghost onClick={handleAdd} block icon={<PlusOutlined />}>
        {t`Add Author`}
      </Button>

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
