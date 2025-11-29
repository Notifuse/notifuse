import { useState, useEffect } from 'react'
import { Modal, Button, Form, Radio, DatePicker, Select, Row, Col, Space, message } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { blogPostsApi, BlogPost } from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'
import dayjs from '../../lib/dayjs'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'

interface PublishModalProps {
  post: BlogPost | null
  visible: boolean
  onClose: () => void
  workspaceId: string
  workspace: Workspace
}

type PublishMode = 'now' | 'custom'

export function PublishModal({ post, visible, onClose, workspaceId, workspace }: PublishModalProps) {
  const [form] = Form.useForm()
  const [publishMode, setPublishMode] = useState<PublishMode>('now')
  const queryClient = useQueryClient()

  const publishMutation = useMutation({
    mutationFn: async (params: { id: string; published_at?: string; timezone?: string }) => {
      return blogPostsApi.publish(workspaceId, params)
    },
    onSuccess: () => {
      message.success('Post published successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })
      onClose()
      form.resetFields()
    },
    onError: (error: Error) => {
      const errorMsg = error?.message || 'Failed to publish post'
      message.error(errorMsg)
    }
  })

  // Reset form when modal opens
  useEffect(() => {
    if (visible) {
      const defaultTimezone = workspace?.settings?.timezone || 'UTC'

      form.setFieldsValue({
        publish_mode: 'now',
        publication_date: null,
        publication_time: '12:00',
        timezone: defaultTimezone
      })
    }
    // Reset publish mode to 'now' when modal opens - this is intentional initial state setup
    if (visible) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setPublishMode('now')
    }
  }, [visible, form, workspace])

  const handleSubmit = async () => {
    if (!post) return

    try {
      const values = form.getFieldsValue()

      if (values.publish_mode === 'now') {
        // Publish immediately - backend will use current timestamp
        await publishMutation.mutateAsync({ id: post.id })
      } else {
        // Validate fields for custom publication date
        await form.validateFields()

        // Combine date, time, and timezone into ISO 8601 timestamp
        const date = dayjs(values.publication_date).format('YYYY-MM-DD')
        const time = values.publication_time
        const timezone = values.timezone

        // Create a dayjs object in the selected timezone
        const dateTimeStr = `${date} ${time}`
        const dateTimeInTz = dayjs.tz(dateTimeStr, timezone)

        // Convert to ISO 8601 format with timezone offset
        const publishedAt = dateTimeInTz.toISOString()

        await publishMutation.mutateAsync({
          id: post.id,
          published_at: publishedAt,
          timezone: timezone
        })
      }
    } catch (error) {
      // Form validation error or other error
      console.error('Publish error:', error)
    }
  }

  if (!post) return null

  return (
    <Modal
      title="Publish Post"
      open={visible}
      onCancel={onClose}
      footer={null}
      destroyOnClose
      width={500}
    >
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <div className="mb-4">
          <p>Choose when to publish "{post.settings.title}"</p>
        </div>

        <Form.Item name="publish_mode" label="Publication">
          <Radio.Group
            onChange={(e) => setPublishMode(e.target.value)}
            className="w-full"
          >
            <Space direction="vertical" className="w-full">
              <Radio value="now">Publish Now</Radio>
              <Radio value="custom">Set Publication Date</Radio>
            </Space>
          </Radio.Group>
        </Form.Item>

        {publishMode === 'custom' && (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="publication_date"
                  label="Date"
                  rules={[
                    {
                      required: publishMode === 'custom',
                      message: 'Please select a date'
                    }
                  ]}
                >
                  <DatePicker
                    format="YYYY-MM-DD"
                    style={{ width: '100%' }}
                    placeholder="Select date"
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="publication_time"
                  label="Time"
                  rules={[
                    {
                      required: publishMode === 'custom',
                      message: 'Please select a time'
                    }
                  ]}
                >
                  <Select
                    showSearch
                    style={{ width: '100%' }}
                    placeholder="Select time"
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

            <Form.Item
              name="timezone"
              label="Timezone"
              rules={[
                {
                  required: publishMode === 'custom',
                  message: 'Please select a timezone'
                }
              ]}
            >
              <Select
                showSearch
                style={{ width: '100%' }}
                placeholder="Select timezone"
                optionFilterProp="label"
                options={TIMEZONE_OPTIONS}
              />
            </Form.Item>
          </>
        )}

        <div className="flex justify-end space-x-2 mt-6">
          <Space>
            <Button onClick={onClose}>Cancel</Button>
            <Button type="primary" htmlType="submit" loading={publishMutation.isPending}>
              {publishMode === 'now' ? 'Publish Now' : 'Publish'}
            </Button>
          </Space>
        </div>
      </Form>
    </Modal>
  )
}
