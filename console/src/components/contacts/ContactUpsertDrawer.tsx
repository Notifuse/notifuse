import React from 'react'
import {
  Button,
  Drawer,
  Form,
  Input,
  Space,
  Select,
  Typography,
  Divider,
  Alert,
  InputNumber,
  DatePicker,
  App,
  Popconfirm,
  Tooltip
} from 'antd'
import { DeleteOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import type { InputProps } from 'antd/es/input'
import type { TextAreaProps } from 'antd/es/input/TextArea'
import type { SelectProps, DefaultOptionType } from 'antd/es/select'
import type { DatePickerProps } from 'antd/es/date-picker'
import type { InputNumberProps } from 'antd/es/input-number'
import { CountriesFormOptions } from '../../lib/countries_timezones'
import { Languages } from '../../lib/languages'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { Contact, UpsertContactOperationAction } from '../../services/api/contacts'
import { contactsApi } from '../../services/api/contacts'
import dayjs from '../../lib/dayjs'
import type { Dayjs } from 'dayjs'
import { Workspace } from '../../services/api/types'

const { Option } = Select
const { Text } = Typography
const { TextArea } = Input

// Custom form input components
const NullableInput: React.FC<InputProps & { name: string; nullLabel: string }> = ({ name, nullLabel, ...props }) => {
  const form = Form.useFormInstance()
  const value = Form.useWatch(name, form)

  return (
    <Space.Compact style={{ width: '100%' }}>
      <Input {...props} />
      <Button
        type={value === null ? 'primary' : 'default'}
        onClick={() => form.setFieldValue(name, null)}
        style={{ padding: '0 8px' }}
      >
        {nullLabel}
      </Button>
    </Space.Compact>
  )
}

const NullableTextArea: React.FC<TextAreaProps & { name: string; nullLabel: string }> = ({ name, nullLabel, ...props }) => {
  const form = Form.useFormInstance()
  const value = Form.useWatch(name, form)

  return (
    <Space.Compact style={{ width: '100%' }}>
      <TextArea {...props} style={{ width: '100%', ...props.style }} />
      <Button
        type={value === null ? 'primary' : 'default'}
        onClick={() => form.setFieldValue(name, null)}
        style={{ padding: '0 8px' }}
      >
        {nullLabel}
      </Button>
    </Space.Compact>
  )
}

const NullableInputNumber: React.FC<InputNumberProps & { name: string; nullLabel: string }> = ({ name, nullLabel, ...props }) => {
  const form = Form.useFormInstance()
  const value = Form.useWatch(name, form)

  return (
    <Space.Compact style={{ width: '100%' }}>
      <InputNumber {...props} style={{ width: '100%', ...props.style }} />
      <Button
        type={value === null ? 'primary' : 'default'}
        onClick={() => form.setFieldValue(name, null)}
        style={{ padding: '0 8px' }}
      >
        {nullLabel}
      </Button>
    </Space.Compact>
  )
}

const NullableDatePicker: React.FC<DatePickerProps & { name: string; nullLabel: string }> = ({ name, nullLabel, ...props }) => {
  const form = Form.useFormInstance()
  const value = Form.useWatch(name, form)

  return (
    <Space.Compact style={{ width: '100%' }}>
      <DatePicker {...props} style={{ width: '100%', ...props.style }} />
      <Button
        type={value === null ? 'primary' : 'default'}
        onClick={() => form.setFieldValue(name, null)}
        style={{ padding: '0 8px' }}
      >
        {nullLabel}
      </Button>
    </Space.Compact>
  )
}

const NullableSelect: React.FC<SelectProps & { name: string; nullLabel: string }> = ({ name, nullLabel, ...props }) => {
  const form = Form.useFormInstance()
  const value = Form.useWatch(name, form)

  return (
    <Space.Compact style={{ width: '100%' }}>
      <Select {...props} style={{ width: '100%', ...props.style }} />
      <Button
        type={value === null ? 'primary' : 'default'}
        onClick={() => form.setFieldValue(name, null)}
        style={{ padding: '0 8px' }}
      >
        {nullLabel}
      </Button>
    </Space.Compact>
  )
}

// Helper to format custom field label with technical name in parentheses
const formatCustomFieldLabel = (fieldKey: string, workspace: Workspace): string => {
  const getDefaultLabel = (key: string): string => {
    const parts = key.split('_')
    if (parts.length >= 3 && parts[0] === 'custom') {
      const type = parts[1].charAt(0).toUpperCase() + parts[1].slice(1)
      const number = parts[2]
      return `Custom ${type} ${number}`
    }
    return key
  }

  const defaultLabel = getDefaultLabel(fieldKey)
  const customLabel = workspace?.settings?.custom_field_labels?.[fieldKey]

  if (customLabel) {
    return `${customLabel} (${fieldKey})`
  }
  return defaultLabel
}

// Helper to get optionalFields with workspace-specific custom labels
const getOptionalFields = (workspace: Workspace) => [
  { key: 'first_name', label: 'First Name' },
  { key: 'last_name', label: 'Last Name' },
  { key: 'full_name', label: 'Full Name' },
  { key: 'phone', label: 'Phone' },
  { key: 'country', label: 'Country' },
  { key: 'external_id', label: 'External ID' },
  { key: 'timezone', label: 'Timezone' },
  { key: 'language', label: 'Language' },
  { key: 'address_line_1', label: 'Address Line 1' },
  { key: 'address_line_2', label: 'Address Line 2' },
  { key: 'postcode', label: 'Postcode' },
  { key: 'state', label: 'State' },
  { key: 'job_title', label: 'Job Title' },
  { key: 'custom_string_1', label: formatCustomFieldLabel('custom_string_1', workspace) },
  { key: 'custom_string_2', label: formatCustomFieldLabel('custom_string_2', workspace) },
  { key: 'custom_string_3', label: formatCustomFieldLabel('custom_string_3', workspace) },
  { key: 'custom_string_4', label: formatCustomFieldLabel('custom_string_4', workspace) },
  { key: 'custom_string_5', label: formatCustomFieldLabel('custom_string_5', workspace) },
  { key: 'custom_number_1', label: formatCustomFieldLabel('custom_number_1', workspace) },
  { key: 'custom_number_2', label: formatCustomFieldLabel('custom_number_2', workspace) },
  { key: 'custom_number_3', label: formatCustomFieldLabel('custom_number_3', workspace) },
  { key: 'custom_number_4', label: formatCustomFieldLabel('custom_number_4', workspace) },
  { key: 'custom_number_5', label: formatCustomFieldLabel('custom_number_5', workspace) },
  { key: 'custom_datetime_1', label: formatCustomFieldLabel('custom_datetime_1', workspace) },
  { key: 'custom_datetime_2', label: formatCustomFieldLabel('custom_datetime_2', workspace) },
  { key: 'custom_datetime_3', label: formatCustomFieldLabel('custom_datetime_3', workspace) },
  { key: 'custom_datetime_4', label: formatCustomFieldLabel('custom_datetime_4', workspace) },
  { key: 'custom_datetime_5', label: formatCustomFieldLabel('custom_datetime_5', workspace) },
  { key: 'custom_json_1', label: formatCustomFieldLabel('custom_json_1', workspace), type: 'json' },
  { key: 'custom_json_2', label: formatCustomFieldLabel('custom_json_2', workspace), type: 'json' },
  { key: 'custom_json_3', label: formatCustomFieldLabel('custom_json_3', workspace), type: 'json' },
  { key: 'custom_json_4', label: formatCustomFieldLabel('custom_json_4', workspace), type: 'json' },
  { key: 'custom_json_5', label: formatCustomFieldLabel('custom_json_5', workspace), type: 'json' }
]

interface ContactUpsertDrawerProps {
  workspace: Workspace
  contact?: Contact
  onSuccess?: (updatedContact: Contact) => void
  onContactUpdate?: (updatedContact: Contact) => void
  open?: boolean
  onClose?: () => void
  buttonProps?: {
    type?: 'primary' | 'default' | 'dashed' | 'link' | 'text'
    icon?: React.ReactNode
    buttonContent?: React.ReactNode
    className?: string
    style?: React.CSSProperties
    size?: 'large' | 'middle' | 'small'
    disabled?: boolean
    loading?: boolean
    danger?: boolean
    ghost?: boolean
    block?: boolean
  }
}

export function ContactUpsertDrawer({
  workspace,
  contact,
  onSuccess,
  onContactUpdate,
  open: externalOpen,
  onClose: externalOnClose,
  buttonProps
}: ContactUpsertDrawerProps) {
  const { t } = useLingui()
  const [internalDrawerVisible, setInternalDrawerVisible] = React.useState(false)
  const [selectedFields, setSelectedFields] = React.useState<string[]>([])
  const [selectedFieldToAdd, setSelectedFieldToAdd] = React.useState<string | null>(null)
  const [form] = Form.useForm()
  const [loading, setLoading] = React.useState(false)
  const { message } = App.useApp()

  // Get optional fields with custom labels - memoize to prevent useEffect from running on every render
  const optionalFields = React.useMemo(() => getOptionalFields(workspace), [workspace])

  // Use external open state if provided, otherwise use internal state
  const isControlled = externalOpen !== undefined
  const drawerVisible = isControlled ? externalOpen : internalDrawerVisible

  React.useEffect(() => {
    if (drawerVisible && contact) {
      // Pre-fill form with contact data
      const fieldsToShow = Object.keys(contact).filter(
        (key) =>
          key !== 'email' &&
          key !== 'workspace_id' &&
          contact[key as keyof Contact] !== undefined &&
          optionalFields.some((field) => field.key === key) // Only include fields that are in our optionalFields array
      )
      setSelectedFields(fieldsToShow)

      // Format JSON fields for display and convert date strings to dayjs objects
      const formattedValues: Record<string, unknown> = { ...contact }
      fieldsToShow.forEach((field) => {
        // Handle JSON fields
        if (field.startsWith('custom_json_')) {
          try {
            formattedValues[field] = JSON.stringify(
              contact[field as keyof Contact],
              null,
              2
            )
          } catch (error) {
            console.error(`Error formatting JSON for field ${field}:`, error)
          }
        }

        // Handle date fields - convert string to dayjs object for DatePicker
        else if (field.startsWith('custom_datetime_')) {
          const dateValue = contact[field as keyof Contact]
          if (dateValue) {
            formattedValues[field] = dayjs(dateValue as string)
          }
        }
      })

      form.setFieldsValue(formattedValues)
    }
  }, [contact, form, drawerVisible, optionalFields])

  const handleRemoveField = (field: string) => {
    setSelectedFields(selectedFields.filter((f) => f !== field))
    form.setFieldValue(field, undefined)
  }

  const handleSubmit = async (values: Record<string, unknown>) => {
    try {
      setLoading(true)
      const contactData: Partial<Contact> & { workspace_id: string } = {
        workspace_id: workspace.id
      }

      // Copy all values from the form
      Object.keys(values).forEach((key) => {
        contactData[key as keyof typeof contactData] = values[key] as never
      })

      // Convert dayjs objects to strings for API submission and parse JSON
      selectedFields.forEach((field) => {
        // Handle JSON fields
        if (field.startsWith('custom_json_')) {
          try {
            const fieldValue = values[field]
            contactData[field as keyof typeof contactData] = JSON.parse(
              String(fieldValue)
            ) as never
          } catch {
            message.error(t`Invalid JSON in field ${field}`)
            return
          }
        }
        // Handle date fields - convert dayjs to ISO string
        else if (field.startsWith('custom_datetime_')) {
          const dateValue = values[field]
          // Check if it's a Dayjs object
          if (dateValue && typeof dateValue === 'object' && 'toISOString' in dateValue) {
            contactData[field as keyof typeof contactData] = (
              dateValue as Dayjs
            ).toISOString() as never
          }
        }
      })

      const response = await contactsApi.upsert({
        workspace_id: workspace.id,
        contact: contactData
      })

      if (response.action === UpsertContactOperationAction.Error) {
        message.error(response.error || t`Failed to save contact`)
        return
      }

      const actionMessage =
        response.action === UpsertContactOperationAction.Create
          ? t`Contact created successfully`
          : t`Contact updated successfully`

      message.success(actionMessage)

      // Close drawer based on controlled/uncontrolled state
      if (isControlled && externalOnClose) {
        externalOnClose()
      } else {
        setInternalDrawerVisible(false)
      }

      form.resetFields()
      setSelectedFields([])

      // Fetch updated contact data
      if (onSuccess || onContactUpdate) {
        // After successful addition, fetch the latest contact data to pass to the parent
        contactsApi
          .list({
            workspace_id: workspace.id,
            email: contactData.email,
            with_contact_lists: true,
            limit: 1
          })
          .then((response) => {
            if (response.contacts && response.contacts.length > 0) {
              if (onSuccess) {
                onSuccess(response.contacts[0])
              }
              if (onContactUpdate) {
                onContactUpdate(response.contacts[0])
              }
            }
          })
      }
    } catch (error) {
      console.error('Failed to upsert contact:', error)
      message.error(t`Failed to save contact. Please try again.`)
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    if (isControlled && externalOnClose) {
      externalOnClose()
    } else {
      setInternalDrawerVisible(false)
    }
    form.resetFields()
    setSelectedFields([])
  }

  // Separate buttonContent from other props
  const { buttonContent, ...otherButtonProps } = buttonProps || {}
  const defaultButtonProps = {
    type: 'primary' as const,
    ...otherButtonProps
  }

  const renderFieldInput = (field: string, fieldInfo: (typeof optionalFields)[0]) => {
    if (field.startsWith('custom_json_')) {
      return (
        <NullableTextArea
          name={field}
          nullLabel={t`Null`}
          rows={4}
          placeholder={t`Enter ${fieldInfo.label.toLowerCase()}`}
          style={{ fontFamily: 'monospace' }}
        />
      )
    }

    if (
      field === 'custom_number_1' ||
      field === 'custom_number_2' ||
      field === 'custom_number_3' ||
      field === 'custom_number_4' ||
      field === 'custom_number_5'
    ) {
      return (
        <NullableInputNumber name={field} nullLabel={t`Null`} placeholder={t`Enter ${fieldInfo.label.toLowerCase()}`} />
      )
    }

    if (
      field === 'custom_datetime_1' ||
      field === 'custom_datetime_2' ||
      field === 'custom_datetime_3' ||
      field === 'custom_datetime_4' ||
      field === 'custom_datetime_5'
    ) {
      return <NullableDatePicker name={field} nullLabel={t`Null`} showTime format="YYYY-MM-DD HH:mm:ss" />
    }

    if (field === 'timezone') {
      return (
        <NullableSelect
          name={field}
          nullLabel={t`Null`}
          placeholder={t`Select timezone`}
          options={TIMEZONE_OPTIONS}
          showSearch
          filterOption={(input: string, option: DefaultOptionType | undefined) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
        />
      )
    }

    if (field === 'country') {
      return (
        <NullableSelect
          name={field}
          nullLabel={t`Null`}
          placeholder={t`Select country`}
          options={CountriesFormOptions}
          showSearch
          filterOption={(input: string, option: DefaultOptionType | undefined) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
        />
      )
    }

    if (field === 'language') {
      return (
        <NullableSelect
          name={field}
          nullLabel={t`Null`}
          placeholder={t`Select language`}
          options={Languages}
          showSearch
          filterOption={(input: string, option: DefaultOptionType | undefined) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
        />
      )
    }

    return <NullableInput name={field} nullLabel={t`Null`} placeholder={t`Enter ${fieldInfo.label.toLowerCase()}`} />
  }

  return (
    <>
      {!isControlled && (
        <Button
          onClick={() => setInternalDrawerVisible(true)}
          {...defaultButtonProps}
          loading={loading}
        >
          {buttonContent || (buttonProps?.icon ? '' : contact ? t`Update Contact` : t`Add Contact`)}
        </Button>
      )}

      <Drawer
        title={contact ? t`Update Contact` : t`Add Contact`}
        width={500}
        open={drawerVisible}
        onClose={handleClose}
        extra={
          <Space>
            <Button onClick={handleClose} disabled={loading}>
              {t`Cancel`}
            </Button>
            <Button type="primary" onClick={() => form.submit()} loading={loading}>
              {t`Save`}
            </Button>
          </Space>
        }
      >
        <Alert
          description={t`If a contact with this email already exists, the provided fields will be overwritten. Fields not included in the form will remain unchanged.`}
          type="info"
          style={{ marginBottom: '16px' }}
        />
        <Form form={form} layout="vertical" onFinish={handleSubmit} disabled={loading}>
          <Form.Item
            name="email"
            label={t`Email`}
            rules={[
              { required: true, message: t`Email is required` },
              { type: 'email', message: t`Please enter a valid email` }
            ]}
          >
            <Input placeholder={t`Enter email address`} disabled={!!contact} />
          </Form.Item>

          {selectedFields.map((field) => {
            const fieldInfo = optionalFields.find((f) => f.key === field)
            if (!fieldInfo) return null // Skip rendering if fieldInfo is undefined

            return (
              <Form.Item
                key={field}
                name={field}
                label={
                  <Space>
                    <span>{fieldInfo.label}</span>
                    <Popconfirm
                      title={t`Remove field`}
                      description={t`Are you sure you want to remove this field?`}
                      onConfirm={() => handleRemoveField(field)}
                      okText={t`Yes`}
                      cancelText={t`No`}
                    >
                      <Button type="text" size="small" icon={<DeleteOutlined />} />
                    </Popconfirm>
                  </Space>
                }
              >
                {renderFieldInput(field, fieldInfo)}
              </Form.Item>
            )
          })}

          <Divider />

          <div>
            <Text strong>{t`Add an optional field`}</Text>
            <div className="mt-2">
              <Select
                placeholder={t`Select a field`}
                style={{ width: '100%' }}
                value={selectedFieldToAdd}
                showSearch
                filterOption={(input: string, option: DefaultOptionType | undefined) =>
                  String(option?.label ?? '')
                    .toLowerCase()
                    .includes(input.toLowerCase())
                }
                onChange={(value) => {
                  if (value && !selectedFields.includes(value)) {
                    setSelectedFields([...selectedFields, value])
                    setSelectedFieldToAdd(null)
                  }
                }}
              >
                {optionalFields
                  .filter((field) => !selectedFields.includes(field.key))
                  .map((field) => {
                    // Check if this is a custom field with a custom label
                    const isCustomField = field.key.startsWith('custom_')
                    const hasCustomLabel =
                      isCustomField && workspace?.settings?.custom_field_labels?.[field.key]

                    if (hasCustomLabel) {
                      return (
                        <Option key={field.key} value={field.key} label={field.label}>
                          <Tooltip title={field.key} placement="left">
                            {field.label}
                          </Tooltip>
                        </Option>
                      )
                    }

                    return (
                      <Option key={field.key} value={field.key} label={field.label}>
                        {field.label}
                      </Option>
                    )
                  })}
              </Select>
            </div>
          </div>
        </Form>
      </Drawer>
    </>
  )
}
