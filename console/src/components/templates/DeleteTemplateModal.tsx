import { Alert, Modal, Spin } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { templatesApi, type TemplateUsageType } from '../../services/api/template'
import type { Template } from '../../services/api/types'
import { Pill } from '../layout'
import { groupReferences, referenceStatusLabel } from './templateUsage'

/** References shown per type before "and {n} more". */
const SHOWN_PER_TYPE = 5

interface DeleteTemplateModalProps {
  workspaceId: string
  /** The template to delete; null keeps the modal closed. */
  template: Template | null
  deleting: boolean
  onConfirm: (template: Template) => void
  onCancel: () => void
}

/**
 * The delete confirmation of a template, with what still uses it (T3, read when it opens). The
 * check never blocks the delete: no send refuses a deleted template, so the member decides.
 */
export function DeleteTemplateModal({
  workspaceId,
  template,
  deleting,
  onConfirm,
  onCancel
}: DeleteTemplateModalProps) {
  const { t, i18n } = useLingui()
  const usageQuery = useQuery({
    queryKey: ['templates-usage', workspaceId, template?.id],
    queryFn: () => templatesApi.usage({ workspace_id: workspaceId, template_id: template?.id }),
    enabled: Boolean(template)
  })

  const groupTitles: Record<TemplateUsageType, string> = {
    broadcasts: t`Newsletters`,
    transactional_notifications: t`Transactional`,
    automations: t`Automations`,
    lists: t`Lists`
  }

  const renderUsage = () => {
    if (usageQuery.isPending) {
      return (
        <div className="flex justify-center py-2">
          <Spin size="small" />
        </div>
      )
    }
    if (usageQuery.isError) {
      return (
        <div className="text-sm text-warning-fg">{t`Couldn't check where this template is used`}</div>
      )
    }
    const data = usageQuery.data
    const counts = template ? data.usage?.[template.id] : undefined
    const groups = groupReferences(data.references ?? [], counts, SHOWN_PER_TYPE)
    const hidden = (data.hidden_types?.length ?? 0) > 0
    return (
      <div className="flex flex-col gap-2">
        {groups.length > 0 ? (
          <Alert
            type="warning"
            showIcon
            title={t`This template is still used`}
            description={
              <div className="flex flex-col gap-2">
                {groups.map((group) => (
                  <div key={group.type} data-usage-group={group.type}>
                    <div className="font-medium">{groupTitles[group.type]}</div>
                    <ul className="m-0 flex list-none flex-col gap-0.5 p-0">
                      {group.items.map((item) => (
                        <li key={item.id} className="flex items-center gap-1.5">
                          <span>{item.name}</span>
                          {item.status ? (
                            <Pill tone="neutral" dot={false}>
                              {referenceStatusLabel(item.status, i18n)}
                            </Pill>
                          ) : null}
                        </li>
                      ))}
                      {group.more > 0 ? (
                        <li className="text-ink-3">
                          {plural(group.more, { one: 'and # more', other: 'and # more' })}
                        </li>
                      ) : null}
                    </ul>
                  </div>
                ))}
                <div>
                  {t`New sends from these items will keep using the deleted template until you pick another one. Messages already sent stay in the logs, but their web version stops working.`}
                </div>
              </div>
            }
          />
        ) : null}
        {hidden ? (
          <div className="text-sm text-ink-3">{t`Some uses may be hidden by your permissions.`}</div>
        ) : null}
      </div>
    )
  }

  return (
    <Modal
      open={Boolean(template)}
      title={t`Delete template?`}
      okText={t`Yes, Delete`}
      okButtonProps={{ danger: true, loading: deleting }}
      cancelText={t`Cancel`}
      onOk={() => template && onConfirm(template)}
      onCancel={onCancel}
      destroyOnHidden
    >
      <div className="flex flex-col gap-3">
        {renderUsage()}
        <p className="m-0">{t`Are you sure you want to delete this template?`}</p>
        <p className="m-0 text-ink-3">
          {t`Note: The template will be hidden from your workspace but preserved to maintain the ability to preview previously sent broadcasts and messages that used this template.`}
        </p>
      </div>
    </Modal>
  )
}
