import React, { useMemo } from 'react'
import { Form, Select, Input, Cascader, ConfigProvider } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { OptionSelector } from '../../ui/OptionSelector'
import type { Workspace } from '../../../services/api/types'

// Event kind cascader options are built inside the component to support i18n

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
  const { t } = useLingui()

  // Cascader options for event kinds
  const EVENT_KIND_CASCADER_OPTIONS = useMemo(() => [
    {
      value: 'contact',
      label: t`Contact`,
      children: [
        { value: 'contact.created', label: t`Created` },
        { value: 'contact.updated', label: t`Updated` },
        { value: 'contact.deleted', label: t`Deleted` }
      ]
    },
    {
      value: 'list',
      label: t`List`,
      children: [
        { value: 'list.subscribed', label: t`Subscribed` },
        { value: 'list.unsubscribed', label: t`Unsubscribed` },
        { value: 'list.confirmed', label: t`Confirmed` },
        { value: 'list.resubscribed', label: t`Resubscribed` },
        { value: 'list.bounced', label: t`Bounced` },
        { value: 'list.complained', label: t`Complained` },
        { value: 'list.pending', label: t`Pending` },
        { value: 'list.removed', label: t`Removed` }
      ]
    },
    {
      value: 'segment',
      label: t`Segment`,
      children: [
        { value: 'segment.joined', label: t`Joined` },
        { value: 'segment.left', label: t`Left` }
      ]
    },
    {
      value: 'email',
      label: t`Email`,
      children: [
        { value: 'email.sent', label: t`Sent` },
        { value: 'email.delivered', label: t`Delivered` },
        { value: 'email.opened', label: t`Opened` },
        { value: 'email.clicked', label: t`Clicked` },
        { value: 'email.bounced', label: t`Bounced` },
        { value: 'email.complained', label: t`Complained` },
        { value: 'email.unsubscribed', label: t`Unsubscribed` }
      ]
    },
    { value: 'custom_event', label: t`Custom Event` }
  ], [t])

  // Build contact field options with custom labels from workspace settings
  const contactFieldOptions = useMemo(
    () => [
      {
        label: t`Core Fields`,
        options: [
          { value: 'first_name', label: t`First Name` },
          { value: 'last_name', label: t`Last Name` },
          { value: 'phone', label: t`Phone` },
          { value: 'photo_url', label: t`Photo URL` },
          { value: 'external_id', label: t`External ID` },
          { value: 'timezone', label: t`Timezone` },
          { value: 'language', label: t`Language` }
        ]
      },
      {
        label: t`Address`,
        options: [
          { value: 'address_line_1', label: t`Address Line 1` },
          { value: 'address_line_2', label: t`Address Line 2` },
          { value: 'country', label: t`Country` },
          { value: 'state', label: t`State` },
          { value: 'postcode', label: t`Postcode` }
        ]
      },
      {
        label: t`Custom String Fields`,
        options: [
          { value: 'custom_string_1', label: getFieldLabel('custom_string_1', t`Custom String 1`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_string_2', label: getFieldLabel('custom_string_2', t`Custom String 2`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_string_3', label: getFieldLabel('custom_string_3', t`Custom String 3`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_string_4', label: getFieldLabel('custom_string_4', t`Custom String 4`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_string_5', label: getFieldLabel('custom_string_5', t`Custom String 5`, workspace?.settings?.custom_field_labels) }
        ]
      },
      {
        label: t`Custom Number Fields`,
        options: [
          { value: 'custom_number_1', label: getFieldLabel('custom_number_1', t`Custom Number 1`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_number_2', label: getFieldLabel('custom_number_2', t`Custom Number 2`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_number_3', label: getFieldLabel('custom_number_3', t`Custom Number 3`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_number_4', label: getFieldLabel('custom_number_4', t`Custom Number 4`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_number_5', label: getFieldLabel('custom_number_5', t`Custom Number 5`, workspace?.settings?.custom_field_labels) }
        ]
      },
      {
        label: t`Custom Date Fields`,
        options: [
          { value: 'custom_datetime_1', label: getFieldLabel('custom_datetime_1', t`Custom Date 1`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_datetime_2', label: getFieldLabel('custom_datetime_2', t`Custom Date 2`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_datetime_3', label: getFieldLabel('custom_datetime_3', t`Custom Date 3`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_datetime_4', label: getFieldLabel('custom_datetime_4', t`Custom Date 4`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_datetime_5', label: getFieldLabel('custom_datetime_5', t`Custom Date 5`, workspace?.settings?.custom_field_labels) }
        ]
      },
      {
        label: t`Custom JSON Fields`,
        options: [
          { value: 'custom_json_1', label: getFieldLabel('custom_json_1', t`Custom JSON 1`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_json_2', label: getFieldLabel('custom_json_2', t`Custom JSON 2`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_json_3', label: getFieldLabel('custom_json_3', t`Custom JSON 3`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_json_4', label: getFieldLabel('custom_json_4', t`Custom JSON 4`, workspace?.settings?.custom_field_labels) },
          { value: 'custom_json_5', label: getFieldLabel('custom_json_5', t`Custom JSON 5`, workspace?.settings?.custom_field_labels) }
        ]
      }
    ],
    [workspace?.settings?.custom_field_labels, t]
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
        label={t`Trigger Event`}
        required
        extra={t`Select the event that will trigger this automation`}
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
            placeholder={t`Select an event...`}
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
          label={t`List`}
          required
          extra={t`Select which list this trigger applies to`}
        >
          <Select
            placeholder={t`Select a list...`}
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
          label={t`Segment`}
          required
          extra={t`Select which segment this trigger applies to`}
        >
          <Select
            placeholder={t`Select a segment...`}
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
          label={t`Event Name`}
          required
          extra={t`Enter the name of the custom event (e.g., 'purchase', 'signup')`}
        >
          <Input
            placeholder={t`e.g., purchase`}
            value={config.custom_event_name}
            onChange={handleCustomEventNameChange}
          />
        </Form.Item>
      )}

      {/* Updated fields filter for contact.updated events */}
      {isContactUpdated && (
        <Form.Item
          label={t`Trigger on specific field changes`}
          extra={t`Leave empty to trigger on any field change`}
        >
          <Select
            mode="multiple"
            placeholder={t`Any field change triggers automation`}
            value={config.updated_fields || []}
            onChange={handleUpdatedFieldsChange}
            options={contactFieldOptions}
            allowClear
            style={{ width: '100%' }}
          />
        </Form.Item>
      )}

      <Form.Item label={t`Frequency`} required>
        <OptionSelector
          value={config.frequency || 'once'}
          onChange={handleFrequencyChange}
          options={[
            {
              value: 'once',
              label: t`Once per contact`,
              description: t`Each contact enters the automation only once`
            },
            {
              value: 'every_time',
              label: t`Every time`,
              description: t`Contact re-enters each time the event occurs`
            }
          ]}
        />
      </Form.Item>
    </Form>
  )
}
