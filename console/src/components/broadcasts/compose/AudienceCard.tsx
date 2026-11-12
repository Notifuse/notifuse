import { Form, Select, Switch, Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Users } from 'lucide-react'
import { Panel, useNumberFormat } from '../../layout'
import type { List } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import type { ComposeEstimate } from '../useAudienceEstimates'
import type { ComposeValues } from './composeValues'
import type { ComposeUpdate } from './types'

export interface AudienceCardProps {
  values: ComposeValues
  onChange: ComposeUpdate
  lists: Pick<List, 'id' | 'name' | 'active_count'>[]
  segments: Pick<Segment, 'id' | 'name' | 'color' | 'users_count'>[]
  /** Without lists.read (or segments.read), the stored value is shown and cannot change. */
  canReadLists: boolean
  canReadSegments: boolean
  estimate: ComposeEstimate
}

/** List, segments and unsubscribed contacts, with the recipients they make. */
export function AudienceCard({ values, onChange, lists, segments, canReadLists, canReadSegments, estimate }: AudienceCardProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()

  const listOptions = canReadLists
    ? lists.map((list) => {
        const active = list.active_count
        return {
          value: list.id,
          label: active === undefined ? list.name : `${list.name} · ${t`${plural(active, { one: '# active', other: '# active' })}`}`
        }
      })
    : values.list
      ? [{ value: values.list, label: values.list }]
      : []

  const segmentLabel = (id: string) => {
    const segment = segments.find((s) => s.id === id)
    if (!segment) return id
    return segment.users_count === undefined ? segment.name : `${segment.name} (${int(segment.users_count)})`
  }

  const count = estimate.count
  const recipients =
    !estimate.available || estimate.error
      ? '—'
      : estimate.counting || count === undefined
        ? t`Counting…`
        : `≈ ${t`${plural(count, { one: '# recipient', other: '# recipients' })}`}`

  return (
    <Panel
      title={t`Audience`}
      icon={<Users size={14} />}
      extra={<span className="text-sm text-ink-2 tabular-nums" data-testid="audience-estimate">{recipients}</span>}
    >
      <Form layout="vertical" component="div">
        <Form.Item label={t`List`} required>
          <Select
            aria-label={t`List`}
            placeholder={t`Select a list`}
            value={values.list}
            disabled={!canReadLists}
            options={listOptions}
            showSearch={{ optionFilterProp: 'label' }}
            onChange={(list: string) => onChange((v) => ({ ...v, list }))}
          />
        </Form.Item>
        <Form.Item
          label={t`And in at least one segment`}
          extra={t`Contacts of the list who belong to any of these segments. Leave empty for the whole list.`}
        >
          <Select
            aria-label={t`Segments`}
            mode="multiple"
            placeholder={t`Select segments (optional)`}
            value={values.segments}
            disabled={!canReadSegments}
            options={(canReadSegments ? segments.map((s) => s.id) : values.segments).map((id) => ({
              value: id,
              label: segmentLabel(id)
            }))}
            showSearch={{ optionFilterProp: 'label' }}
            tagRender={(props) => {
              const segment = segments.find((s) => s.id === props.value)
              return (
                <Tag color={segment?.color} variant="filled" closable={props.closable} onClose={props.onClose}>
                  {props.label}
                </Tag>
              )
            }}
            onChange={(chosen: string[]) => onChange((v) => ({ ...v, segments: chosen }))}
          />
        </Form.Item>
        <Form.Item
          label={t`Exclude unsubscribed recipients`}
          extra={t`Turn off for notices every member must receive, such as security or legal notices: the list's unsubscribed contacts receive it too, and its emails carry no List-Unsubscribe header, so mail clients show no Unsubscribe button. The unsubscribe link in your template is not affected.`}
        >
          <Switch
            aria-label={t`Exclude unsubscribed recipients`}
            checked={values.excludeUnsubscribed}
            onChange={(excludeUnsubscribed) => onChange((v) => ({ ...v, excludeUnsubscribed }))}
          />
        </Form.Item>
      </Form>
    </Panel>
  )
}
