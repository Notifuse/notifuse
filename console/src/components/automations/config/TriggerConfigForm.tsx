import React, { useMemo } from 'react'
import { Form, Select, Input, Cascader, ConfigProvider } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { OptionSelector } from '../../ui/OptionSelector'
import type { Workspace } from '../../../services/api/types'

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

// Helper to get custom field label with fallback to default
const getFieldLabel = (
  fieldKey: string,
  defaultLabel: string,
  customFieldLabels?: Record<string, string>
): string => {
  const customLabel = customFieldLabels?.[fieldKey]
  if (customLabel) {
    return `${customLabel} (${fieldKey})`
  }
  return defaultLabel
}

// Build contact field options with custom labels from workspace settings
const buildContactFieldOptions = (customFieldLabels?: Record<string, string>) => [
  {
    label: 'Core Fields',
    options: [
      { value: 'first_name', label: 'First Name' },
      { value: 'last_name', label: 'Last Name' },
      { value: 'phone', label: 'Phone' },
      { value: 'photo_url', label: 'Photo URL' },
      { value: 'external_id', label: 'External ID' },
      { value: 'timezone', label: 'Timezone' },
      { value: 'language', label: 'Language' }
    ]
  },
  {
    label: 'Address',
    options: [
      { value: 'address_line_1', label: 'Address Line 1' },
      { value: 'address_line_2', label: 'Address Line 2' },
      { value: 'country', label: 'Country' },
      { value: 'state', label: 'State' },
      { value: 'postcode', label: 'Postcode' }
    ]
  },
  {
    label: 'Custom String Fields',
    options: [
      { value: 'custom_string_1', label: getFieldLabel('custom_string_1', 'Custom String 1', customFieldLabels) },
      { value: 'custom_string_2', label: getFieldLabel('custom_string_2', 'Custom String 2', customFieldLabels) },
      { value: 'custom_string_3', label: getFieldLabel('custom_string_3', 'Custom String 3', customFieldLabels) },
      { value: 'custom_string_4', label: getFieldLabel('custom_string_4', 'Custom String 4', customFieldLabels) },
      { value: 'custom_string_5', label: getFieldLabel('custom_string_5', 'Custom String 5', customFieldLabels) }
    ]
  },
  {
    label: 'Custom Number Fields',
    options: [
      { value: 'custom_number_1', label: getFieldLabel('custom_number_1', 'Custom Number 1', customFieldLabels) },
      { value: 'custom_number_2', label: getFieldLabel('custom_number_2', 'Custom Number 2', customFieldLabels) },
      { value: 'custom_number_3', label: getFieldLabel('custom_number_3', 'Custom Number 3', customFieldLabels) },
      { value: 'custom_number_4', label: getFieldLabel('custom_number_4', 'Custom Number 4', customFieldLabels) },
      { value: 'custom_number_5', label: getFieldLabel('custom_number_5', 'Custom Number 5', customFieldLabels) }
    ]
  },
  {
    label: 'Custom Date Fields',
    options: [
      { value: 'custom_datetime_1', label: getFieldLabel('custom_datetime_1', 'Custom Date 1', customFieldLabels) },
      { value: 'custom_datetime_2', label: getFieldLabel('custom_datetime_2', 'Custom Date 2', customFieldLabels) },
      { value: 'custom_datetime_3', label: getFieldLabel('custom_datetime_3', 'Custom Date 3', customFieldLabels) },
      { value: 'custom_datetime_4', label: getFieldLabel('custom_datetime_4', 'Custom Date 4', customFieldLabels) },
      { value: 'custom_datetime_5', label: getFieldLabel('custom_datetime_5', 'Custom Date 5', customFieldLabels) }
    ]
  },
  {
    label: 'Custom JSON Fields',
    options: [
      { value: 'custom_json_1', label: getFieldLabel('custom_json_1', 'Custom JSON 1', customFieldLabels) },
      { value: 'custom_json_2', label: getFieldLabel('custom_json_2', 'Custom JSON 2', customFieldLabels) },
      { value: 'custom_json_3', label: getFieldLabel('custom_json_3', 'Custom JSON 3', customFieldLabels) },
      { value: 'custom_json_4', label: getFieldLabel('custom_json_4', 'Custom JSON 4', customFieldLabels) },
      { value: 'custom_json_5', label: getFieldLabel('custom_json_5', 'Custom JSON 5', customFieldLabels) }
    ]
  }
]

interface TriggerConfig {
  event_kind?: string
  list_id?: string
  segment_id?: string
  custom_event_name?: string
  updated_fields?: string[]
  frequency?: 'once' | 'every_time'
}

interface TriggerConfigFormProps {
  config: TriggerConfig
  onChange: (config: TriggerConfig) => void
  workspaceId: string
  workspace?: Workspace
}

export const TriggerConfigForm: React.FC<TriggerConfigFormProps> = ({ config, onChange, workspaceId, workspace }) => {
  // Build contact field options with custom labels from workspace settings
  const contactFieldOptions = useMemo(
    () => buildContactFieldOptions(workspace?.settings?.custom_field_labels),
    [workspace?.settings?.custom_field_labels]
  )
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
      custom_event_name: undefined,
      updated_fields: undefined
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

  const handleUpdatedFieldsChange = (value: string[]) => {
    onChange({ ...config, updated_fields: value.length > 0 ? value : undefined })
  }

  const isListEvent = config.event_kind?.startsWith('list.')
  const isSegmentEvent = config.event_kind?.startsWith('segment.')
  const isCustomEvent = config.event_kind === 'custom_event'
  const isContactUpdated = config.event_kind === 'contact.updated'

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

      {/* Updated fields filter for contact.updated events */}
      {isContactUpdated && (
        <Form.Item
          label="Trigger on specific field changes"
          extra="Leave empty to trigger on any field change"
        >
          <Select
            mode="multiple"
            placeholder="Any field change triggers automation"
            value={config.updated_fields || []}
            onChange={handleUpdatedFieldsChange}
            options={contactFieldOptions}
            allowClear
            style={{ width: '100%' }}
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
