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
  Popconfirm
} from 'antd'
import type { InputProps } from 'antd/es/input'
import type { TextAreaProps } from 'antd/es/input/TextArea'
import type { SelectProps, DefaultOptionType } from 'antd/es/select'
import type { DatePickerProps } from 'antd/es/date-picker'
import type { InputNumberProps } from 'antd/es/input-number'
import { CountriesFormOptions, TimezonesFormOptions } from '../utils/countries_timezones'
import { Languages } from '../utils/languages'
import { Contact, UpsertContactOperationAction } from '../../services/api/contacts'
import { contactsApi } from '../../services/api/contacts'
import dayjs from '../../lib/dayjs'
import { Workspace } from '../../services/api/types'

const { Option } = Select
const { Text } = Typography
const { TextArea } = Input

// Custom form input components
const NullableInput: React.FC<InputProps & { name: string }> = ({ name, ...props }) => {
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
        Null
      </Button>
    </Space.Compact>
  )
}

const NullableTextArea: React.FC<TextAreaProps & { name: string }> = ({ name, ...props }) => {
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
        Null
      </Button>
    </Space.Compact>
  )
}

const NullableInputNumber: React.FC<InputNumberProps & { name: string }> = ({ name, ...props }) => {
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
        Null
      </Button>
    </Space.Compact>
  )
}

const NullableDatePicker: React.FC<DatePickerProps & { name: string }> = ({ name, ...props }) => {
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
        Null
      </Button>
    </Space.Compact>
  )
}

const NullableSelect: React.FC<SelectProps & { name: string }> = ({ name, ...props }) => {
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
        Null
      </Button>
    </Space.Compact>
  )
}

const optionalFields = [
  { key: 'first_name', label: 'First Name' },
  { key: 'last_name', label: 'Last Name' },
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
  { key: 'lifetime_value', label: 'Lifetime Value' },
  { key: 'orders_count', label: 'Orders Count' },
  { key: 'last_order_at', label: 'Last Order At' },
  { key: 'custom_string_1', label: 'Custom String 1' },
  { key: 'custom_string_2', label: 'Custom String 2' },
  { key: 'custom_string_3', label: 'Custom String 3' },
  { key: 'custom_string_4', label: 'Custom String 4' },
  { key: 'custom_string_5', label: 'Custom String 5' },
  { key: 'custom_number_1', label: 'Custom Number 1' },
  { key: 'custom_number_2', label: 'Custom Number 2' },
  { key: 'custom_number_3', label: 'Custom Number 3' },
  { key: 'custom_number_4', label: 'Custom Number 4' },
  { key: 'custom_number_5', label: 'Custom Number 5' },
  { key: 'custom_datetime_1', label: 'Custom Date 1' },
  { key: 'custom_datetime_2', label: 'Custom Date 2' },
  { key: 'custom_datetime_3', label: 'Custom Date 3' },
  { key: 'custom_datetime_4', label: 'Custom Date 4' },
  { key: 'custom_datetime_5', label: 'Custom Date 5' },
  { key: 'custom_json_1', label: 'Custom JSON 1', type: 'json' },
  { key: 'custom_json_2', label: 'Custom JSON 2', type: 'json' },
  { key: 'custom_json_3', label: 'Custom JSON 3', type: 'json' },
  { key: 'custom_json_4', label: 'Custom JSON 4', type: 'json' },
  { key: 'custom_json_5', label: 'Custom JSON 5', type: 'json' }
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
  const [internalDrawerVisible, setInternalDrawerVisible] = React.useState(false)
  const [selectedFields, setSelectedFields] = React.useState<string[]>([])
  const [selectedFieldToAdd, setSelectedFieldToAdd] = React.useState<string | null>(null)
  const [form] = Form.useForm()
  const [loading, setLoading] = React.useState(false)
  const { message } = App.useApp()

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
      const formattedValues = { ...contact }
      fieldsToShow.forEach((field) => {
        // Handle JSON fields
        if (field.startsWith('custom_json_')) {
          try {
            formattedValues[field as keyof Contact] = JSON.stringify(
              contact[field as keyof Contact],
              null,
              2
            )
          } catch (e) {
            console.error(`Error formatting JSON for field ${field}:`, e)
          }
        }

        // Handle date fields - convert string to dayjs object for DatePicker
        else if (field === 'last_order_at' || field.startsWith('custom_datetime_')) {
          const dateValue = contact[field as keyof Contact]
          if (dateValue) {
            formattedValues[field as keyof Contact] = dayjs(dateValue as string)
          }
        }
      })

      form.setFieldsValue(formattedValues)
    }
  }, [contact, form, drawerVisible])

  const handleRemoveField = (field: string) => {
    setSelectedFields(selectedFields.filter((f) => f !== field))
    form.setFieldValue(field, undefined)
  }

  const handleSubmit = async (values: any) => {
    try {
      setLoading(true)
      const contactData = {
        ...values,
        workspace_id: workspace.id
      }

      // Convert dayjs objects to strings for API submission and parse JSON
      selectedFields.forEach((field) => {
        // Handle JSON fields
        if (field.startsWith('custom_json_')) {
          try {
            contactData[field] = JSON.parse(values[field])
          } catch (e) {
            message.error(`Invalid JSON in field ${field}`)
            return
          }
        }
        // Handle date fields - convert dayjs to ISO string
        else if (field === 'last_order_at' || field.startsWith('custom_datetime_')) {
          const dateValue = values[field]
          if (dateValue && dateValue.$d) {
            contactData[field] = dateValue.toISOString()
          }
        }
      })

      const response = await contactsApi.upsert({
        workspace_id: workspace.id,
        contact: contactData
      })

      if (response.action === UpsertContactOperationAction.Error) {
        message.error(response.error || 'Failed to save contact')
        return
      }

      const actionMessage =
        response.action === UpsertContactOperationAction.Create
          ? 'Contact created successfully'
          : 'Contact updated successfully'

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
      message.error('Failed to save contact. Please try again.')
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
          rows={4}
          placeholder={`Enter ${fieldInfo.label.toLowerCase()}`}
          style={{ fontFamily: 'monospace' }}
        />
      )
    }

    if (
      field === 'lifetime_value' ||
      field === 'orders_count' ||
      field === 'custom_number_1' ||
      field === 'custom_number_2' ||
      field === 'custom_number_3' ||
      field === 'custom_number_4' ||
      field === 'custom_number_5'
    ) {
      return (
        <NullableInputNumber name={field} placeholder={`Enter ${fieldInfo.label.toLowerCase()}`} />
      )
    }

    if (
      field === 'last_order_at' ||
      field === 'custom_datetime_1' ||
      field === 'custom_datetime_2' ||
      field === 'custom_datetime_3' ||
      field === 'custom_datetime_4' ||
      field === 'custom_datetime_5'
    ) {
      return <NullableDatePicker name={field} showTime format="YYYY-MM-DD HH:mm:ss" />
    }

    if (field === 'timezone') {
      return (
        <NullableSelect
          name={field}
          placeholder="Select timezone"
          options={TimezonesFormOptions}
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
          placeholder="Select country"
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
          placeholder="Select language"
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

    return <NullableInput name={field} placeholder={`Enter ${fieldInfo.label.toLowerCase()}`} />
  }

  return (
    <>
      {!isControlled && (
        <Button
          onClick={() => setInternalDrawerVisible(true)}
          {...defaultButtonProps}
          loading={loading}
        >
          {buttonContent || (buttonProps?.icon ? '' : contact ? 'Update Contact' : 'Add Contact')}
        </Button>
      )}

      <Drawer
        title={contact ? 'Update Contact' : 'Add Contact'}
        width={500}
        open={drawerVisible}
        onClose={handleClose}
        extra={
          <Space>
            <Button onClick={handleClose} disabled={loading}>
              Cancel
            </Button>
            <Button type="primary" onClick={() => form.submit()} loading={loading}>
              Save
            </Button>
          </Space>
        }
      >
        <Alert
          description="If a contact with this email already exists, the provided fields will be overwritten. Fields not included in the form will remain unchanged."
          type="info"
          showIcon
          style={{ marginBottom: '16px' }}
        />
        <Form form={form} layout="vertical" onFinish={handleSubmit} disabled={loading}>
          <Form.Item
            name="email"
            label="Email"
            rules={[
              { required: true, message: 'Email is required' },
              { type: 'email', message: 'Please enter a valid email' }
            ]}
          >
            <Input placeholder="Enter email address" disabled={!!contact} />
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
                      title="Remove field"
                      description="Are you sure you want to remove this field?"
                      onConfirm={() => handleRemoveField(field)}
                      okText="Yes"
                      cancelText="No"
                    >
                      <Button type="link" size="small" danger style={{ marginLeft: 'auto' }}>
                        Remove field
                      </Button>
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
            <Text strong>Add an optional field</Text>
            <div className="mt-2">
              <Select
                placeholder="Select a field"
                style={{ width: '100%' }}
                value={selectedFieldToAdd}
                onChange={(value) => {
                  if (value && !selectedFields.includes(value)) {
                    setSelectedFields([...selectedFields, value])
                    setSelectedFieldToAdd(null)
                  }
                }}
              >
                {optionalFields
                  .filter((field) => !selectedFields.includes(field.key))
                  .map((field) => (
                    <Option key={field.key} value={field.key}>
                      {field.label}
                    </Option>
                  ))}
              </Select>
            </div>
          </div>
        </Form>
      </Drawer>
    </>
  )
}
