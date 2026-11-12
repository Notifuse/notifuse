import { Select, Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useQuery } from '@tanstack/react-query'
import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { formatSender, getMarketingSenders, resolveVariationSender } from './variationSender'

interface VariationSenderSelectProps {
  // Set by the enclosing Form.Item, so its label names the field.
  id?: string
  value?: string
  onChange?: (value: string | undefined) => void
  workspace: Workspace
  // The variation's template, whose sender applies while none is chosen here.
  templateId?: string
  // Without Templates read, templates.list answers 403: the template's sender is not looked up.
  // Defaults to true.
  canReadTemplates?: boolean
}

// VariationSenderSelect picks the sender one broadcast variation leaves from, among the
// marketing integration's senders. Cleared, the variation keeps its template's sender.
export function VariationSenderSelect({
  id,
  value,
  onChange,
  workspace,
  templateId,
  canReadTemplates = true
}: VariationSenderSelectProps) {
  const { t } = useLingui()
  const senders = getMarketingSenders(workspace)

  // Same query as the template selector beside it, so the list comes from its cache.
  const { data: templatesResponse } = useQuery({
    queryKey: ['templates', workspace.id, 'marketing'],
    queryFn: () =>
      templatesApi.list({
        workspace_id: workspace.id,
        category: 'marketing',
        channel: 'email'
      }),
    enabled: !!workspace.id && !!templateId && canReadTemplates
  })
  const template = templatesResponse?.templates?.find((tpl) => tpl.id === templateId)
  // Known once the variation's template is: until then no sender can be said to be its.
  const templateSender = template
    ? resolveVariationSender(senders, undefined, template.email?.sender_id).sender
    : undefined

  const options = senders.map((sender) => ({
    value: sender.id,
    label: (
      <span>
        {formatSender(sender)}
        {templateSender?.id === sender.id && (
          <Tag className="!ml-2" color="purple">
            {t`template's`}
          </Tag>
        )}
        {sender.is_default && <Tag className="!ml-2">{t`default`}</Tag>}
      </span>
    )
  }))
  // A sender chosen before it was removed from the integration stays visible, so the user
  // sees what has to be replaced.
  if (value && !senders.some((s) => s.id === value)) {
    const senderId = value
    options.unshift({ value, label: <span>{t`${senderId} (deleted)`}</span> })
  }

  return (
    <Select
      id={id}
      value={value || undefined}
      onChange={(next) => onChange?.(next || undefined)}
      allowClear
      // A sender kept from before the integration lost its senders can still be cleared.
      disabled={senders.length === 0 && !value}
      placeholder={
        templateSender ? t`Template's sender (${templateSender.name})` : t`Template's sender`
      }
      options={options}
      style={{ width: '100%' }}
    />
  )
}

export default VariationSenderSelect
