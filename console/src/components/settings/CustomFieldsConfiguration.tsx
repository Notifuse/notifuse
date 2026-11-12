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
  Tooltip,
  Table,
  Dropdown,
  Empty
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { MoreHorizontal, Plus } from 'lucide-react'
import type { MessageDescriptor } from '@lingui/core'
import { msg, plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { Mono, Panel } from '../layout'

interface CustomFieldsConfigurationProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  /** Whether the current user can manage custom field labels (workspace:write or owner). */
  canManage: boolean
}

type CustomFieldType = 'String' | 'Number' | 'Datetime' | 'JSON'

interface CustomFieldSlot {
  key: string
  type: CustomFieldType
}

interface NamedSlot extends CustomFieldSlot {
  label: string
}

// All available custom fields organized by type
const CUSTOM_FIELDS_BY_TYPE: Record<CustomFieldType, string[]> = {
  String: Array.from({ length: 5 }, (_, i) => `custom_string_${i + 1}`),
  Number: Array.from({ length: 5 }, (_, i) => `custom_number_${i + 1}`),
  Datetime: Array.from({ length: 5 }, (_, i) => `custom_datetime_${i + 1}`),
  JSON: Array.from({ length: 5 }, (_, i) => `custom_json_${i + 1}`)
}

export const ALL_CUSTOM_FIELDS: CustomFieldSlot[] = (
  Object.keys(CUSTOM_FIELDS_BY_TYPE) as CustomFieldType[]
).flatMap((type) => CUSTOM_FIELDS_BY_TYPE[type].map((key) => ({ key, type })))

const TYPE_LABELS: Record<CustomFieldType, MessageDescriptor> = {
  String: msg`Text`,
  Number: msg`Number`,
  Datetime: msg`Date`,
  JSON: msg`JSON`
}

export function CustomFieldsConfiguration({
  workspace,
  onWorkspaceUpdate,
  canManage
}: CustomFieldsConfigurationProps) {
  const { t, i18n } = useLingui()
  const [modalVisible, setModalVisible] = useState(false)
  const [editingField, setEditingField] = useState<string | null>(null)
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const { message, modal } = App.useApp()

  const customFieldLabels = workspace?.settings?.custom_field_labels || {}

  // The named slots, in the order of the slots themselves rather than of the stored labels.
  const namedSlots: NamedSlot[] = ALL_CUSTOM_FIELDS.filter(
    (field) => customFieldLabels[field.key]
  ).map((field) => ({ ...field, label: customFieldLabels[field.key] }))
  const total = ALL_CUSTOM_FIELDS.length
  const named = namedSlots.length
  const freeOf = (type: CustomFieldType) =>
    CUSTOM_FIELDS_BY_TYPE[type].filter((key) => !customFieldLabels[key]).length

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

      await workspaceService.setCustomFieldLabels({
        workspace_id: workspace.id,
        custom_field_labels: updatedLabels
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

      await workspaceService.setCustomFieldLabels({
        workspace_id: workspace.id,
        custom_field_labels: updatedLabels
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

  const noPermission = canManage ? undefined : t`You don't have permission to manage custom fields`

  const confirmRemove = (slot: NamedSlot) =>
    modal.confirm({
      title: t`Remove custom field label`,
      content: t`The values stay on your contacts; only the label is removed.`,
      okText: t`Yes`,
      cancelText: t`No`,
      okButtonProps: { danger: true },
      onOk: () => handleDelete(slot.key)
    })

  const columns: ColumnsType<NamedSlot> = [
    {
      title: t`Label`,
      key: 'label',
      render: (_, slot) => <span className="font-medium text-ink">{slot.label}</span>
    },
    {
      title: t`Field`,
      key: 'field',
      width: 200,
      render: (_, slot) => <Mono>{slot.key}</Mono>
    },
    {
      title: t`Type`,
      key: 'type',
      width: 110,
      render: (_, slot) => <span className="text-ink-3">{i18n._(TYPE_LABELS[slot.type])}</span>
    },
    {
      title: '',
      key: 'actions',
      width: 48,
      align: 'right',
      render: (_, slot) => (
        <Tooltip title={noPermission}>
          <span className="inline-block">
            <Dropdown
              trigger={['click']}
              placement="bottomRight"
              disabled={!canManage}
              menu={{
                items: [
                  {
                    key: 'rename',
                    label: t`Rename`,
                    onClick: () => handleOpenModal(slot.key)
                  },
                  {
                    key: 'remove',
                    label: t`Remove label`,
                    danger: true,
                    onClick: () => confirmRemove(slot)
                  }
                ]
              }}
            >
              <Button
                type="text"
                size="small"
                aria-label={t`Actions`}
                disabled={!canManage}
                icon={<MoreHorizontal size={16} aria-hidden="true" />}
              />
            </Dropdown>
          </span>
        </Tooltip>
      )
    }
  ]

  const textFree = plural(freeOf('String'), { one: '# text', other: '# text' })
  const numberFree = plural(freeOf('Number'), {
    one: '# number',
    other: '# number'
  })
  const dateFree = plural(freeOf('Datetime'), {
    one: '# date',
    other: '# date'
  })
  const jsonFree = plural(freeOf('JSON'), { one: '# JSON', other: '# JSON' })

  const footer =
    named === total ? (
      <span
        data-custom-fields-footer
        className="text-ink-3"
      >{t`All ${total} slots are named`}</span>
    ) : (
      <div data-custom-fields-footer className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <Tooltip title={noPermission}>
          <span className="inline-block">
            <Button
              type="link"
              size="small"
              icon={<Plus size={14} aria-hidden="true" />}
              disabled={!canManage}
              onClick={() => handleOpenModal()}
            >
              {t`Name a free slot`}
            </Button>
          </span>
        </Tooltip>
        <span className="text-ink-3">{t`${textFree}, ${numberFree}, ${dateFree}, ${jsonFree} free`}</span>
      </div>
    )

  return (
    <>
      <SettingsSectionHeader
        title={t`Custom fields`}
        description={t`Name the free fields of your contacts`}
      />

      <Panel
        flush
        title={t`Slots`}
        extra={<span className="text-xs text-ink-3">{t`${named} of ${total} slots named`}</span>}
      >
        {named === 0 ? (
          <div className="py-8">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={t`No custom field is named yet`}
            >
              <Tooltip title={noPermission}>
                <span className="inline-block">
                  <Button type="primary" disabled={!canManage} onClick={() => handleOpenModal()}>
                    {t`Name a slot`}
                  </Button>
                </span>
              </Tooltip>
            </Empty>
          </div>
        ) : (
          <>
            <Table<NamedSlot>
              rowKey="key"
              size="middle"
              columns={columns}
              dataSource={namedSlots}
              pagination={false}
            />
            <div className="border-t border-line-soft px-3.5 py-2 text-xs">{footer}</div>
          </>
        )}
      </Panel>

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
                      <Space orientation="vertical">
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
