import { Form, Select, Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Users } from 'lucide-react'
import { useId } from 'react'
import { HelpTip, Panel, useNumberFormat } from '../../layout'
import type { List } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import type { ComposeEstimate } from '../useAudienceEstimates'
import type { ComposeValues } from './composeValues'
import type { ComposeUpdate } from './types'
import { FieldsAboveSwitchRow, SwitchRow } from './SwitchRow'

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
  const id = useId()
  const segmentsHelpId = `${id}-segments-help`
  const segmentsSelectId = `${id}-segments`

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
      <FieldsAboveSwitchRow>
        <Form layout="vertical" component="div">
          <div className="grid grid-cols-2 gap-3">
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
              // The label points at the select, not at the "?" it holds.
              htmlFor={segmentsSelectId}
              label={
                <HelpTip title={t`And in at least one segment`} label={t`About segments`} id={segmentsHelpId}>
                  {t`Contacts of the list who belong to any of these segments. Leave empty for the whole list.`}
                </HelpTip>
              }
              extra={t`Optional. Empty means the whole list.`}
            >
              <Select
                id={segmentsSelectId}
                aria-label={t`Segments`}
                aria-describedby={segmentsHelpId}
                mode="multiple"
                placeholder={t`Select segments (optional)`}
                value={values.segments}
                disabled={!canReadSegments}
                options={(canReadSegments ? segments.map((s) => s.id) : values.segments).map((segmentId) => ({
                  value: segmentId,
                  label: segmentLabel(segmentId)
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
          </div>
        </Form>
      </FieldsAboveSwitchRow>
      {/* Its consequences under a "?": the form keeps one line for it. */}
      <SwitchRow
        title={t`Exclude unsubscribed recipients`}
        hint={t`Turn off only for a notice everyone must receive (security, legal).`}
        help={{
          label: t`About unsubscribed recipients`,
          content: t`Turn off for notices every member must receive, such as security or legal notices: the list's unsubscribed contacts receive it too, and its emails carry no List-Unsubscribe header, so mail clients show no Unsubscribe button. The unsubscribe link in your template is not affected.`
        }}
        checked={values.excludeUnsubscribed}
        onChange={(excludeUnsubscribed) => onChange((v) => ({ ...v, excludeUnsubscribed }))}
      />
    </Panel>
  )
}
