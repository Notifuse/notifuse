import { useState } from 'react'
import {
  Modal,
  Button,
  Form,
  Switch,
  Select,
  DatePicker,
  Row,
  Col,
  message,
  Space,
  Alert
} from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Broadcast } from '../../services/api/broadcast'
import { broadcastApi } from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'
import dayjs from '../../lib/dayjs'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'

// Feature flag for recipient timezone functionality
const ENABLE_RECIPIENT_TIMEZONE = false

interface SendOrScheduleModalProps {
  broadcast: Broadcast | null
  visible: boolean
  onClose: () => void
  workspaceId: string
  workspace?: Workspace
  onSuccess: () => void
}

export function SendOrScheduleModal({
  broadcast,
  visible,
  onClose,
  workspaceId,
  workspace,
  onSuccess
}: SendOrScheduleModalProps) {
  const { t } = useLingui()
  const [form] = Form.useForm()
  const [isScheduled, setIsScheduled] = useState(false)
  const [loading, setLoading] = useState(false)

  const hasMarketingEmailProvider = workspace?.settings?.marketing_email_provider_id

  // Helper function to extract error message from API response
  const getErrorMessage = (error: unknown, defaultMessage: string): string => {
    // Try to extract message from various possible response structures
    // Check for 'error' field first (used by WriteJSONError in backend)
    if (error && typeof error === 'object' && 'response' in error) {
      const response = (error as { response?: { data?: { error?: string; message?: string } } }).response
      if (response?.data?.error) {
        return response.data.error
      }
      // Check for 'message' field (used by some other handlers)
      if (response?.data?.message) {
        return response.data.message
      }
    }
    // Fallback to general error message
    if (error && typeof error === 'object' && 'message' in error && typeof (error as { message: unknown }).message === 'string') {
      return (error as { message: string }).message
    }
    return defaultMessage
  }

  // Reset form when modal opens
  const handleOpen = () => {
    // Get the default timezone from broadcast or workspace or fall back to UTC
    const defaultTimezone = broadcast?.schedule?.timezone || 'UTC'

    form.setFieldsValue({
      is_scheduled: false,
      scheduled_date: null,
      scheduled_time: '11:00',
      timezone: defaultTimezone,
      use_recipient_timezone: false
    })
    setIsScheduled(false)
  }

  // Send broadcast immediately
  const handleSendNow = async () => {
    if (!broadcast) return

    setLoading(true)
    try {
      await broadcastApi.schedule({
        workspace_id: workspaceId,
        id: broadcast.id,
        send_now: true
      })
      message.success(t`Broadcast "${broadcast.name}" sending started`)
      onSuccess()
      onClose()
    } catch (error) {
      console.error(error)
      const errorMessage = getErrorMessage(error, t`Failed to send broadcast`)
      message.error(errorMessage)
    } finally {
      setLoading(false)
    }
  }

  // Schedule broadcast or send immediately based on form state
  const handleSubmit = async () => {
    if (!broadcast) return

    try {
      // Only validate fields if scheduling is enabled
      if (isScheduled) {
        await form.validateFields()
      }

      const values = form.getFieldsValue()

      if (!values.is_scheduled) {
        // If not scheduled, send immediately
        return handleSendNow()
      }

      setLoading(true)

      // For scheduled broadcasts, we need to send the schedule details
      try {
        // Format date and time for API
        const scheduledDate = dayjs(values.scheduled_date).format('YYYY-MM-DD')
        const scheduledTime = values.scheduled_time

        // Now schedule the broadcast
        await broadcastApi.schedule({
          workspace_id: workspaceId,
          id: broadcast.id,
          send_now: false,
          scheduled_date: scheduledDate,
          scheduled_time: scheduledTime,
          timezone: values.timezone,
          use_recipient_timezone: values.use_recipient_timezone
        })

        message.success(t`Broadcast "${broadcast.name}" scheduled successfully`)
        onSuccess()
        onClose()
      } catch (error) {
        console.error(error)
        const errorMessage = getErrorMessage(error, t`Failed to schedule broadcast`)
        message.error(errorMessage)
      }
    } catch (error) {
      console.error(error)
      message.error(t`Please check the form for errors`)
    } finally {
      setLoading(false)
    }
  }

  if (!broadcast) return null

  return (
    <Modal
      title={t`Send or Schedule Broadcast`}
      open={visible}
      onCancel={onClose}
      footer={null}
      destroyOnClose
      afterOpenChange={(visible) => {
        if (visible) handleOpen()
      }}
    >
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        {!hasMarketingEmailProvider && (
          <Alert
            message={t`Marketing Email Provider Required`}
            description={t`You don't have a marketing email provider configured. Please set up an email provider in your workspace settings to send broadcasts.`}
            type="warning"
            showIcon
            className="!mb-4"
            action={
              <Button
                type="link"
                size="small"
                href={`/console/workspace/${workspaceId}/settings/integrations`}
              >
                {t`Configure Provider`}
              </Button>
            }
          />
        )}

        <div className="mb-4">
          <p>{t`Do you want to send "${broadcast.name}" immediately or schedule it for later?`}</p>
        </div>

        <Form.Item name="is_scheduled" valuePropName="checked" label={t`Schedule for later delivery`}>
          <Switch onChange={(checked) => setIsScheduled(checked)} />
        </Form.Item>

        {isScheduled && (
          <>
            <Row gutter={24}>
              <Col span={12}>
                <Form.Item
                  name="scheduled_date"
                  label={t`Date`}
                  rules={[
                    {
                      required: isScheduled,
                      message: t`Please select a date`
                    }
                  ]}
                >
                  <DatePicker
                    format="YYYY-MM-DD"
                    disabledDate={(current) => {
                      // Can't select days before today
                      return current && current < dayjs().startOf('day')
                    }}
                    style={{ width: '100%' }}
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="scheduled_time"
                  label={t`Time`}
                  rules={[
                    {
                      required: isScheduled,
                      message: t`Please select a time`
                    }
                  ]}
                >
                  <Select
                    showSearch
                    style={{ width: '100%' }}
                    placeholder={t`Select time`}
                    optionFilterProp="children"
                  >
                    {Array.from({ length: 24 * 4 }, (_, i) => {
                      const hour = Math.floor(i / 4)
                      const minute = (i % 4) * 15
                      const hourStr = hour.toString().padStart(2, '0')
                      const minuteStr = minute.toString().padStart(2, '0')
                      return {
                        value: `${hourStr}:${minuteStr}`,
                        label: `${hourStr}:${minuteStr}`
                      }
                    }).map((option) => (
                      <Select.Option key={option.value} value={option.value}>
                        {option.label}
                      </Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              </Col>
            </Row>

            <Row gutter={24}>
              <Col span={12}>
                <Form.Item
                  name="timezone"
                  label={t`Timezone`}
                  rules={[
                    {
                      required: isScheduled,
                      message: t`Please select a timezone`
                    }
                  ]}
                >
                  <Select
                    showSearch
                    style={{ width: '100%' }}
                    placeholder={t`Select timezone`}
                    optionFilterProp="label"
                    options={TIMEZONE_OPTIONS}
                  />
                </Form.Item>
              </Col>
              {/* Feature flag for recipient timezone - disabled until backend implementation is complete */}
              {ENABLE_RECIPIENT_TIMEZONE && (
                <Col span={12}>
                  <Form.Item
                    name="use_recipient_timezone"
                    valuePropName="checked"
                    label={t`Use recipient timezone`}
                    tooltip={t`If enabled, the broadcast will be sent according to each recipient's timezone`}
                  >
                    <Switch />
                  </Form.Item>
                </Col>
              )}
            </Row>
          </>
        )}

        <div className="flex justify-end space-x-2 mt-6">
          <Space>
            <Button onClick={onClose}>{t`Cancel`}</Button>
            <Button
              type="primary"
              loading={loading}
              htmlType="submit"
              disabled={!hasMarketingEmailProvider}
            >
              {isScheduled ? t`Schedule` : t`Send Now`}
            </Button>
          </Space>
        </div>
      </Form>
    </Modal>
  )
}
