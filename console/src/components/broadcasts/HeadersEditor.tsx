import { useState } from 'react'
import { Button, Input, Table, Modal, Form, Popconfirm } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import type { DataFeedHeader } from '../../services/api/broadcast'

interface HeadersEditorProps {
  value?: DataFeedHeader[]
  onChange?: (headers: DataFeedHeader[]) => void
  disabled?: boolean
}

export function HeadersEditor({ value = [], onChange, disabled = false }: HeadersEditorProps) {
  const { t } = useLingui()
  const [modalOpen, setModalOpen] = useState(false)
  const [form] = Form.useForm()

  const handleAddHeader = () => {
    form.validateFields().then((values) => {
      const newHeaders = [...value, { name: values.name, value: values.value }]
      onChange?.(newHeaders)
      form.resetFields()
      setModalOpen(false)
    })
  }

  const handleRemoveHeader = (index: number) => {
    const newHeaders = value.filter((_, i) => i !== index)
    onChange?.(newHeaders)
  }

  const columns = [
    {
      title: t`Custom header`,
      dataIndex: 'name',
      key: 'name',
      width: 180
    },
    {
      title: t`Value`,
      dataIndex: 'value',
      key: 'value'
    },
    {
      title: (
        <Button
          type="primary"
          ghost
          size="small"
          icon={<PlusOutlined />}
          onClick={() => setModalOpen(true)}
          disabled={disabled}
        >
          {t`Add`}
        </Button>
      ),
      key: 'action',
      width: 80,
      align: 'right' as const,
      render: (_: unknown, __: DataFeedHeader, index: number) => (
        <Popconfirm
          title={t`Delete header`}
          description={t`Are you sure you want to delete this header?`}
          onConfirm={() => handleRemoveHeader(index)}
          okText={t`Yes`}
          cancelText={t`No`}
        >
          <Button
            type="text"
            icon={<DeleteOutlined />}
            disabled={disabled}
          />
        </Popconfirm>
      )
    }
  ]

  return (
    <div className="space-y-2">
      {value.length > 0 ? (
        <Table
          dataSource={value.map((h, i) => ({ ...h, key: i }))}
          columns={columns}
          showHeader={true}
          pagination={false}
          size="small"
        />
      ) : (
        <Button type="primary" ghost block size="small" onClick={() => setModalOpen(true)} disabled={disabled}>
          {t`Add custom header`}
        </Button>
      )}

      <Modal
        title={t`Add Custom Header`}
        open={modalOpen}
        onCancel={() => {
          form.resetFields()
          setModalOpen(false)
        }}
        onOk={handleAddHeader}
        okText={t`Add`}
        cancelText={t`Cancel`}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label={t`Header name`}
            rules={[{ required: true, message: t`Header name is required` }]}
          >
            <Input placeholder="Authorization" />
          </Form.Item>
          <Form.Item
            name="value"
            label={t`Header value`}
            rules={[{ required: true, message: t`Header value is required` }]}
          >
            <Input placeholder="Bearer token123" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
