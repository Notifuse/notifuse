import { useState } from 'react'
import {
  Form,
  Input,
  Button,
  App,
  Modal,
  Space,
  Radio,
  Row,
  Col,
  Descriptions,
  Popconfirm,
  Tooltip
} from 'antd'
import { EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SettingsSectionHeader } from './SettingsSectionHeader'

interface CustomFieldsConfigurationProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  isOwner: boolean
}

interface CustomFieldMapping {
  fieldKey: string
  fieldType: string
  label: string
}

// All available custom fields organized by type
const CUSTOM_FIELDS_BY_TYPE = {
  String: Array.from({ length: 5 }, (_, i) => `custom_string_${i + 1}`),
  Number: Array.from({ length: 5 }, (_, i) => `custom_number_${i + 1}`),
  Datetime: Array.from({ length: 5 }, (_, i) => `custom_datetime_${i + 1}`),
  JSON: Array.from({ length: 5 }, (_, i) => `custom_json_${i + 1}`)
}

const ALL_CUSTOM_FIELDS = [
  ...CUSTOM_FIELDS_BY_TYPE.String.map((key) => ({ key, type: 'String' })),
  ...CUSTOM_FIELDS_BY_TYPE.Number.map((key) => ({ key, type: 'Number' })),
  ...CUSTOM_FIELDS_BY_TYPE.Datetime.map((key) => ({ key, type: 'Datetime' })),
  ...CUSTOM_FIELDS_BY_TYPE.JSON.map((key) => ({ key, type: 'JSON' }))
]

export function CustomFieldsConfiguration({
  workspace,
  onWorkspaceUpdate,
  isOwner
}: CustomFieldsConfigurationProps) {
  const { t } = useLingui()
  const [modalVisible, setModalVisible] = useState(false)
  const [editingField, setEditingField] = useState<string | null>(null)
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const { message } = App.useApp()

  const customFieldLabels = workspace?.settings?.custom_field_labels || {}

  // Get mapped fields
  const mappedFields: CustomFieldMapping[] = Object.entries(customFieldLabels).map(
    ([fieldKey, label]) => {
      const field = ALL_CUSTOM_FIELDS.find((f) => f.key === fieldKey)
      return {
        fieldKey,
        fieldType: field?.type || 'Unknown',
        label
      }
    }
  )

  // Get available fields for selection (fields not yet mapped)
  const availableFields = ALL_CUSTOM_FIELDS.filter(
    (field) => !customFieldLabels[field.key] || editingField === field.key
  )

  const handleOpenModal = (fieldKey?: string) => {
    if (fieldKey) {
      setEditingField(fieldKey)
      form.setFieldsValue({
        fieldKey,
        label: customFieldLabels[fieldKey]
      })
    } else {
      setEditingField(null)
      form.resetFields()
    }
    setModalVisible(true)
  }

  const handleCloseModal = () => {
    setModalVisible(false)
    setEditingField(null)
    form.resetFields()
  }

  const handleSave = async (values: { fieldKey: string; label: string }) => {
    if (!workspace) return

    setSaving(true)
    try {
      const updatedLabels = {
        ...customFieldLabels,
        [values.fieldKey]: values.label.trim()
      }

      await workspaceService.update({
        ...workspace,
        settings: {
          ...workspace.settings,
          custom_field_labels: updatedLabels
        }
      })

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)
      onWorkspaceUpdate(response.workspace)

      message.success(t`Custom field label saved successfully`)
      handleCloseModal()
    } catch (error) {
      console.error('Failed to update custom field label', error)
      message.error(t`Failed to update custom field label`)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (fieldKey: string) => {
    if (!workspace) return

    try {
      const updatedLabels = { ...customFieldLabels }
      delete updatedLabels[fieldKey]

      await workspaceService.update({
        ...workspace,
        settings: {
          ...workspace.settings,
          custom_field_labels: updatedLabels
        }
      })

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)
      onWorkspaceUpdate(response.workspace)

      message.success(t`Custom field label removed successfully`)
    } catch (error) {
      console.error('Failed to remove custom field label', error)
      message.error(t`Failed to remove custom field label`)
    }
  }

  return (
    <>
      <SettingsSectionHeader
        title={t`Custom Fields`}
        description={t`Set friendly display names for contact custom fields.`}
      />

      {isOwner && (
        <div style={{ textAlign: 'right', marginBottom: 16 }}>
          <Button type="primary" ghost size="small" onClick={() => handleOpenModal()}>
            {t`Add Label`}
          </Button>
        </div>
      )}

      {mappedFields.length > 0 && (
        <Descriptions bordered size="small" column={1}>
          {mappedFields.map((field) => (
            <Descriptions.Item
              key={field.fieldKey}
              label={
                <span>
                  <code style={{ fontSize: '12px' }}>{field.fieldKey}</code>
                  <span style={{ color: '#888', marginLeft: 8 }}>({field.fieldType})</span>
                </span>
              }
            >
              <div
                style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
              >
                <span>{field.label}</span>
                {isOwner && (
                  <Space size="small">
                    <Button
                      type="text"
                      size="small"
                      icon={<EditOutlined />}
                      onClick={() => handleOpenModal(field.fieldKey)}
                    />
                    <Popconfirm
                      title={t`Remove custom field label`}
                      description={t`Are you sure you want to remove this custom field label?`}
                      onConfirm={() => handleDelete(field.fieldKey)}
                      okText={t`Yes`}
                      cancelText={t`No`}
                    >
                      <Button type="text" size="small" icon={<DeleteOutlined />} />
                    </Popconfirm>
                  </Space>
                )}
              </div>
            </Descriptions.Item>
          ))}
        </Descriptions>
      )}

      <Modal
        title={editingField ? t`Edit Custom Field Label` : t`Add Custom Field Label`}
        open={modalVisible}
        onCancel={handleCloseModal}
        footer={null}
        width={800}
      >
        <div className="py-8">
          <Form form={form} onFinish={handleSave} layout="vertical">
            <Form.Item
              name="fieldKey"
              rules={[{ required: true, message: t`Please select a custom field` }]}
            >
              <Radio.Group disabled={!!editingField} style={{ width: '100%' }}>
                <Row gutter={[24, 24]}>
                  {Object.entries(CUSTOM_FIELDS_BY_TYPE).map(([type, fields]) => (
                    <Col span={6} key={type}>
                      <div style={{ fontWeight: 'bold', marginBottom: 8 }}>{type}</div>
                      <Space direction="vertical">
                        {fields.map((fieldKey) => {
                          const isAvailable =
                            availableFields.some((f) => f.key === fieldKey) ||
                            editingField === fieldKey
                          const radioButton = (
                            <Radio key={fieldKey} value={fieldKey} disabled={!isAvailable}>
                              <code style={{ fontSize: '11px' }}>{fieldKey}</code>
                            </Radio>
                          )

                          if (!isAvailable) {
                            return (
                              <Tooltip key={fieldKey} title={t`already set`}>
                                {radioButton}
                              </Tooltip>
                            )
                          }

                          return radioButton
                        })}
                      </Space>
                    </Col>
                  ))}
                </Row>
              </Radio.Group>
            </Form.Item>

            <Form.Item
              name="label"
              label={t`Custom Label`}
              rules={[
                { required: true, message: t`Please enter a custom label` },
                { max: 100, message: t`Label must be at most 100 characters` }
              ]}
            >
              <Input placeholder={t`e.g., Company Name, Industry, etc.`} maxLength={100} />
            </Form.Item>

            <Form.Item style={{ marginBottom: 0, textAlign: 'right' }}>
              <Space>
                <Button onClick={handleCloseModal}>{t`Cancel`}</Button>
                <Button type="primary" htmlType="submit" loading={saving}>
                  {t`Save`}
                </Button>
              </Space>
            </Form.Item>
          </Form>
        </div>
      </Modal>
    </>
  )
}
