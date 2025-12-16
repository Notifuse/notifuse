import React from 'react'
import { Form, Select, Radio, Typography } from 'antd'

const { Text } = Typography

// Available event kinds based on timeline operations + entity types
const EVENT_KIND_OPTIONS = [
  { label: 'Contact Created', value: 'insert_contact', group: 'Contact' },
  { label: 'Contact Updated', value: 'update_contact', group: 'Contact' },
  { label: 'Contact Deleted', value: 'delete_contact', group: 'Contact' },
  { label: 'Added to List', value: 'insert_contact_list', group: 'List' },
  { label: 'List Status Updated', value: 'update_contact_list', group: 'List' },
  { label: 'Removed from List', value: 'delete_contact_list', group: 'List' },
  { label: 'Message Sent', value: 'insert_message_history', group: 'Message' },
  { label: 'Message Status Changed', value: 'update_message_history', group: 'Message' }
]

// Group options for Select
const groupedOptions = EVENT_KIND_OPTIONS.reduce(
  (acc, opt) => {
    if (!acc[opt.group]) {
      acc[opt.group] = []
    }
    acc[opt.group].push({ label: opt.label, value: opt.value })
    return acc
  },
  {} as Record<string, { label: string; value: string }[]>
)

interface TriggerConfig {
  event_kinds?: string[]
  frequency?: 'once' | 'every_time'
}

interface TriggerConfigFormProps {
  config: TriggerConfig
  onChange: (config: TriggerConfig) => void
}

export const TriggerConfigForm: React.FC<TriggerConfigFormProps> = ({ config, onChange }) => {
  const handleEventKindsChange = (value: string[]) => {
    onChange({ ...config, event_kinds: value })
  }

  const handleFrequencyChange = (value: 'once' | 'every_time') => {
    onChange({ ...config, frequency: value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label="Trigger Events"
        required
        extra="Select the events that will trigger this automation"
      >
        <Select
          mode="multiple"
          placeholder="Select events..."
          value={config.event_kinds || []}
          onChange={handleEventKindsChange}
          style={{ width: '100%' }}
          options={Object.entries(groupedOptions).map(([group, options]) => ({
            label: group,
            options
          }))}
        />
      </Form.Item>

      <Form.Item label="Frequency" required>
        <Radio.Group
          value={config.frequency || 'once'}
          onChange={(e) => handleFrequencyChange(e.target.value)}
        >
          <div className="flex flex-col gap-2">
            <Radio value="once">
              <div>
                <div>Once per contact</div>
                <Text type="secondary" className="text-xs">
                  Each contact enters the automation only once
                </Text>
              </div>
            </Radio>
            <Radio value="every_time">
              <div>
                <div>Every time</div>
                <Text type="secondary" className="text-xs">
                  Contact re-enters each time the event occurs
                </Text>
              </div>
            </Radio>
          </div>
        </Radio.Group>
      </Form.Item>
    </Form>
  )
}
