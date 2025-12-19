import React, { useMemo } from 'react'
import { Form, Select, Input, Cascader, ConfigProvider } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { OptionSelector } from '../../ui/OptionSelector'

// Cascader options for event kinds
const EVENT_KIND_CASCADER_OPTIONS = [
  {
    value: 'contact',
    label: 'Contact',
    children: [
      { value: 'contact.created', label: 'Created' },
      { value: 'contact.updated', label: 'Updated' },
      { value: 'contact.deleted', label: 'Deleted' }
    ]
  },
  {
    value: 'list',
    label: 'List',
    children: [
      { value: 'list.subscribed', label: 'Subscribed' },
      { value: 'list.unsubscribed', label: 'Unsubscribed' },
      { value: 'list.confirmed', label: 'Confirmed' },
      { value: 'list.resubscribed', label: 'Resubscribed' },
      { value: 'list.bounced', label: 'Bounced' },
      { value: 'list.complained', label: 'Complained' },
      { value: 'list.pending', label: 'Pending' },
      { value: 'list.removed', label: 'Removed' }
    ]
  },
  {
    value: 'segment',
    label: 'Segment',
    children: [
      { value: 'segment.joined', label: 'Joined' },
      { value: 'segment.left', label: 'Left' }
    ]
  },
  {
    value: 'email',
    label: 'Email',
    children: [
      { value: 'email.sent', label: 'Sent' },
      { value: 'email.delivered', label: 'Delivered' },
      { value: 'email.opened', label: 'Opened' },
      { value: 'email.clicked', label: 'Clicked' },
      { value: 'email.bounced', label: 'Bounced' },
      { value: 'email.complained', label: 'Complained' },
      { value: 'email.unsubscribed', label: 'Unsubscribed' }
    ]
  },
  { value: 'custom_event', label: 'Custom Event' }
]

// Helper to get cascader value from event_kind
const getCascaderValue = (eventKind?: string): string[] => {
  if (!eventKind) return []
  if (eventKind === 'custom_event') return ['custom_event']
  const prefix = eventKind.split('.')[0]
  return [prefix, eventKind]
}

interface TriggerConfig {
  event_kind?: string
  list_id?: string
  segment_id?: string
  custom_event_name?: string
  frequency?: 'once' | 'every_time'
}

interface TriggerConfigFormProps {
  config: TriggerConfig
  onChange: (config: TriggerConfig) => void
  workspaceId: string
}

export const TriggerConfigForm: React.FC<TriggerConfigFormProps> = ({ config, onChange, workspaceId }) => {
  // Fetch lists for list events
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: !!workspaceId && config.event_kind?.startsWith('list.')
  })

  // Fetch segments for segment events
  const { data: segmentsData } = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => listSegments({ workspace_id: workspaceId }),
    enabled: !!workspaceId && config.event_kind?.startsWith('segment.')
  })

  const handleEventKindChange = (value: (string | number)[]) => {
    // Cascader returns array, we want the last value (the actual event kind)
    const eventKind = value.length > 0 ? String(value[value.length - 1]) : undefined
    // Clear related fields when event kind changes
    const newConfig: TriggerConfig = {
      ...config,
      event_kind: eventKind,
      list_id: undefined,
      segment_id: undefined,
      custom_event_name: undefined
    }
    onChange(newConfig)
  }

  const handleListIdChange = (value: string) => {
    onChange({ ...config, list_id: value })
  }

  const handleSegmentIdChange = (value: string) => {
    onChange({ ...config, segment_id: value })
  }

  const handleCustomEventNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onChange({ ...config, custom_event_name: e.target.value })
  }

  const handleFrequencyChange = (value: 'once' | 'every_time') => {
    onChange({ ...config, frequency: value })
  }

  const isListEvent = config.event_kind?.startsWith('list.')
  const isSegmentEvent = config.event_kind?.startsWith('segment.')
  const isCustomEvent = config.event_kind === 'custom_event'

  // Memoize cascader value to prevent flicker on re-render
  const cascaderValue = useMemo(() => getCascaderValue(config.event_kind), [config.event_kind])

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label="Trigger Event"
        required
        extra="Select the event that will trigger this automation"
      >
        <ConfigProvider
          theme={{
            components: {
              Cascader: {
                dropdownHeight: 280
              }
            }
          }}
        >
          <Cascader
            placeholder="Select an event..."
            value={cascaderValue}
            onChange={handleEventKindChange}
            options={EVENT_KIND_CASCADER_OPTIONS}
            expandTrigger="hover"
            style={{ width: '100%' }}
          />
        </ConfigProvider>
      </Form.Item>

      {/* List selector for list events */}
      {isListEvent && (
        <Form.Item
          label="List"
          required
          extra="Select which list this trigger applies to"
        >
          <Select
            placeholder="Select a list..."
            value={config.list_id}
            onChange={handleListIdChange}
            style={{ width: '100%' }}
            options={listsData?.lists?.map((list) => ({
              label: list.name,
              value: list.id
            })) || []}
            loading={!listsData}
          />
        </Form.Item>
      )}

      {/* Segment selector for segment events */}
      {isSegmentEvent && (
        <Form.Item
          label="Segment"
          required
          extra="Select which segment this trigger applies to"
        >
          <Select
            placeholder="Select a segment..."
            value={config.segment_id}
            onChange={handleSegmentIdChange}
            style={{ width: '100%' }}
            options={segmentsData?.segments?.map((segment) => ({
              label: segment.name,
              value: segment.id
            })) || []}
            loading={!segmentsData}
          />
        </Form.Item>
      )}

      {/* Custom event name input */}
      {isCustomEvent && (
        <Form.Item
          label="Event Name"
          required
          extra="Enter the name of the custom event (e.g., 'purchase', 'signup')"
        >
          <Input
            placeholder="e.g., purchase"
            value={config.custom_event_name}
            onChange={handleCustomEventNameChange}
          />
        </Form.Item>
      )}

      <Form.Item label="Frequency" required>
        <OptionSelector
          value={config.frequency || 'once'}
          onChange={handleFrequencyChange}
          options={[
            {
              value: 'once',
              label: 'Once per contact',
              description: 'Each contact enters the automation only once'
            },
            {
              value: 'every_time',
              label: 'Every time',
              description: 'Contact re-enters each time the event occurs'
            }
          ]}
        />
      </Form.Item>
    </Form>
  )
}
